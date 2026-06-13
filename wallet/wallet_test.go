/*
FILE: wallet/wallet_test.go

DESCRIPTION:
Contract tests for the wallet client against httptest-served Gate JSON.
newWalletTestClient points the parent gate.Client's REST transport at an
httptest.Server so the client issues real HTTP requests with no network. The
tests pin: the Transfer request-body shape + parse, the GetTotalBalance parse
(including a codec.FlexDecimal number-or-string decode of an amount field), the
public ListCurrencyChains call (unsigned), GetTradeFee, and a Gate {label,message}
error surfacing as *gate.Error.
*/

package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/wallet/types"
)

func mustDec(s string) decimal.Decimal {
	var d decimal.Decimal
	var err error
	d, err = decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func newWalletTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	var parent *gate.Client
	var err error
	parent, err = gate.NewClient(gate.Config{
		APIKey:    "test-key",
		SecretKey: "test-secret",
		REST:      gate.RestConfig{BaseURL: baseURL},
	})
	if err != nil {
		t.Fatalf("gate.NewClient: %v", err)
	}
	return NewClient(parent)
}

// decodeBody reads and JSON-decodes the request body into a generic map.
func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var raw []byte
	raw, _ = io.ReadAll(r.Body)
	var m map[string]any = map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("decode body: %v (raw=%s)", err, string(raw))
		}
	}
	return m
}

func TestTransfer_BodyAndParse(t *testing.T) {
	var gotPath, gotMethod, gotKey string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotKey = r.URL.Path, r.Method, r.Header.Get("KEY")
		gotBody = decodeBody(t, r)
		_, _ = io.WriteString(w, `{"tx_id":59636381}`)
	}))
	defer srv.Close()

	var wc *Client = newWalletTestClient(t, srv.URL)
	var res types.TransferResult
	var err error
	res, err = wc.Transfer(context.Background(), types.TransferRequest{
		Currency: "USDT",
		From:     types.AccountTypeSpot,
		To:       types.AccountTypeFutures,
		Amount:   mustDec("100"),
		Settle:   "usdt",
	})
	if err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/wallet/transfers" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	// Private endpoint: must be signed.
	if gotKey == "" {
		t.Fatalf("private call should be signed (KEY header missing)")
	}
	if gotBody["currency"] != "USDT" || gotBody["from"] != "spot" || gotBody["to"] != "futures" {
		t.Fatalf("body core: %+v", gotBody)
	}
	if gotBody["amount"] != "100" || gotBody["settle"] != "usdt" {
		t.Fatalf("body opts: %+v", gotBody)
	}
	if res.TxID != "59636381" {
		t.Fatalf("tx id: %q", res.TxID)
	}
}

func TestGetTotalBalance_Parse(t *testing.T) {
	var gotPath, gotQuery, gotKey string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotKey = r.URL.Path, r.URL.RawQuery, r.Header.Get("KEY")
		_, _ = io.WriteString(w, `{"total":{"amount":"123.45","currency":"USDT"},
			"details":{"spot":{"amount":"100","currency":"USDT"},
			"futures":{"amount":"23.45","currency":"USDT"}}}`)
	}))
	defer srv.Close()

	var wc *Client = newWalletTestClient(t, srv.URL)
	var tb types.TotalBalance
	var err error
	tb, err = wc.GetTotalBalance(context.Background(), "USDT")
	if err != nil {
		t.Fatalf("GetTotalBalance: %v", err)
	}
	if gotPath != "/wallet/total_balance" || !strings.Contains(gotQuery, "currency=USDT") {
		t.Fatalf("path/query: %q / %q", gotPath, gotQuery)
	}
	if gotKey == "" {
		t.Fatalf("private call should be signed (KEY header missing)")
	}
	if tb.Total.Currency != "USDT" || !tb.Total.Amount.Equal(mustDec("123.45")) {
		t.Fatalf("total: %+v", tb.Total)
	}
	if !tb.Details["spot"].Amount.Equal(mustDec("100")) || !tb.Details["futures"].Amount.Equal(mustDec("23.45")) {
		t.Fatalf("details: %+v", tb.Details)
	}
}

// TestTotalBalance_DecodesNumberAndStringForms guards the Gate behavior where an
// amount may arrive as a quoted string (REST) or a bare JSON number: the shared
// payload must decode BOTH via codec.FlexDecimal.
func TestTotalBalance_DecodesNumberAndStringForms(t *testing.T) {
	var stringForm = []byte(`{"total":{"amount":"123.45","currency":"USDT"},
		"details":{"spot":{"amount":"100","currency":"USDT"}}}`)
	var numberForm = []byte(`{"total":{"amount":123.45,"currency":"USDT"},
		"details":{"spot":{"amount":100,"currency":"USDT"}}}`)

	var form string
	var raw []byte
	for form, raw = range map[string][]byte{"rest-string": stringForm, "ws-number": numberForm} {
		var p totalBalancePayload
		if err := codec.Unmarshal(raw, &p); err != nil {
			t.Fatalf("%s: unmarshal failed: %v", form, err)
		}
		if !p.Total.Amount.Decimal.Equal(mustDec("123.45")) {
			t.Fatalf("%s: total amount: %s", form, p.Total.Amount.Decimal)
		}
		if !p.Details["spot"].Amount.Decimal.Equal(mustDec("100")) {
			t.Fatalf("%s: spot amount: %s", form, p.Details["spot"].Amount.Decimal)
		}
	}
}

func TestListCurrencyChains_Public(t *testing.T) {
	var gotPath, gotQuery, gotKey string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotKey = r.URL.Path, r.URL.RawQuery, r.Header.Get("KEY")
		_, _ = io.WriteString(w, `[{"chain":"ETH","name_cn":"以太坊","name_en":"ETH",
			"contract_address":"","addr_regex":"^0x","is_disabled":0,"is_deposit_disabled":0,
			"is_withdraw_disabled":1,"decimal":"18"}]`)
	}))
	defer srv.Close()

	var wc *Client = newWalletTestClient(t, srv.URL)
	var chains []types.CurrencyChain
	var err error
	chains, err = wc.ListCurrencyChains(context.Background(), "ETH")
	if err != nil {
		t.Fatalf("ListCurrencyChains: %v", err)
	}
	if gotPath != "/wallet/currency_chains" || !strings.Contains(gotQuery, "currency=ETH") {
		t.Fatalf("path/query: %q / %q", gotPath, gotQuery)
	}
	// Public endpoint: must NOT be signed (no KEY header).
	if gotKey != "" {
		t.Fatalf("public call should be unsigned, got KEY=%q", gotKey)
	}
	if len(chains) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(chains))
	}
	if chains[0].Chain != "ETH" || chains[0].NameEN != "ETH" || chains[0].Decimals != "18" {
		t.Fatalf("chain: %+v", chains[0])
	}
	if chains[0].Disabled || chains[0].DepositDisabled || !chains[0].WithdrawDisabled {
		t.Fatalf("flags: %+v", chains[0])
	}
}

func TestGetTradeFee_Parse(t *testing.T) {
	var gotPath, gotKey string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey = r.URL.Path, r.Header.Get("KEY")
		_, _ = io.WriteString(w, `{"user_id":10001,"taker_fee":"0.002","maker_fee":"0.0018",
			"gt_discount":true,"gt_taker_fee":"0.0015","gt_maker_fee":"0.0013","loan_fee":"0.18",
			"point_type":"1","futures_taker_fee":"0.00045","futures_maker_fee":"0.0002","debit_fee":3}`)
	}))
	defer srv.Close()

	var wc *Client = newWalletTestClient(t, srv.URL)
	var fee types.TradeFee
	var err error
	fee, err = wc.GetTradeFee(context.Background(), "", "")
	if err != nil {
		t.Fatalf("GetTradeFee: %v", err)
	}
	if gotPath != "/wallet/fee" {
		t.Fatalf("path: %q", gotPath)
	}
	if gotKey == "" {
		t.Fatalf("private call should be signed (KEY header missing)")
	}
	if fee.UserID != 10001 || !fee.TakerFee.Equal(mustDec("0.002")) || !fee.MakerFee.Equal(mustDec("0.0018")) {
		t.Fatalf("fee core: %+v", fee)
	}
	if !fee.GTDiscount || !fee.FuturesTakerFee.Equal(mustDec("0.00045")) || fee.DebitFee != 3 {
		t.Fatalf("fee opts: %+v", fee)
	}
}

func TestTransfer_ErrorLabel_Surfaces(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"label":"BALANCE_NOT_ENOUGH","message":"insufficient balance"}`)
	}))
	defer srv.Close()

	var wc *Client = newWalletTestClient(t, srv.URL)
	var err error
	_, err = wc.Transfer(context.Background(), types.TransferRequest{
		Currency: "USDT", From: types.AccountTypeSpot, To: types.AccountTypeMargin, Amount: mustDec("1"),
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ge *gate.Error
	if !errors.As(err, &ge) || ge.Label != "BALANCE_NOT_ENOUGH" {
		t.Fatalf("unexpected error: %v", err)
	}
}
