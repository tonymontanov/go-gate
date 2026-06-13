/*
FILE: margin/isolated_test.go

DESCRIPTION:
Contract tests for the isolated-margin sub-client against httptest-served Gate
JSON. They pin: the public currency_pairs list parse, ListAccounts parsing with a
codec.FlexDecimal number-or-string decode of a balance field, CreateLoan
request-body shaping + response parse, the RepayLoan path + body, the
GetBorrowable/GetTransferable shapes, and a Gate {label,message} error surfacing
as *gate.Error. No network: the parent client's REST base points at the test
server.
*/

package margin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/margin/types"
)

func TestIsolated_ListCurrencyPairs_Public(t *testing.T) {
	var gotPath string
	var gotKey string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("KEY")
		_, _ = io.WriteString(w, `[{"id":"BTC_USDT","base":"BTC","quote":"USDT","leverage":"3",
			"min_base_amount":"0.001","min_quote_amount":"1","max_quote_amount":"1000000","status":1}]`)
	}))
	defer srv.Close()

	var m *Client = newMarginTestClient(t, srv.URL)
	var pairs []types.MarginCurrencyPair
	var err error
	pairs, err = m.Isolated().ListCurrencyPairs(context.Background())
	if err != nil {
		t.Fatalf("ListCurrencyPairs: %v", err)
	}
	if gotPath != "/margin/currency_pairs" {
		t.Fatalf("path: %q", gotPath)
	}
	// Public endpoint: must NOT be signed (no KEY header).
	if gotKey != "" {
		t.Fatalf("public call should be unsigned, got KEY=%q", gotKey)
	}
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(pairs))
	}
	if pairs[0].ID != "BTC_USDT" || !pairs[0].Leverage.Equal(mustDec("3")) || pairs[0].Status != 1 {
		t.Fatalf("pair: %+v", pairs[0])
	}
	if !pairs[0].MaxQuoteAmount.Equal(mustDec("1000000")) {
		t.Fatalf("max quote: %s", pairs[0].MaxQuoteAmount)
	}
}

func TestIsolated_ListAccounts_ParsesBalances(t *testing.T) {
	var gotPath, gotQuery, gotKey string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotKey = r.URL.Path, r.URL.RawQuery, r.Header.Get("KEY")
		_, _ = io.WriteString(w, `[{"currency_pair":"BTC_USDT","locked":false,"risk":"9.5",
			"base":{"currency":"BTC","available":"0.5","locked":"0","borrowed":"0.1","interest":"0.0001"},
			"quote":{"currency":"USDT","available":"1000","locked":"0","borrowed":"200","interest":"0.05"}}]`)
	}))
	defer srv.Close()

	var m *Client = newMarginTestClient(t, srv.URL)
	var accounts []types.MarginAccount
	var err error
	accounts, err = m.Isolated().ListAccounts(context.Background(), "BTC_USDT")
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if gotPath != "/margin/accounts" || !strings.Contains(gotQuery, "currency_pair=BTC_USDT") {
		t.Fatalf("path/query: %q / %q", gotPath, gotQuery)
	}
	// Private endpoint: must be signed.
	if gotKey == "" {
		t.Fatalf("private call should be signed (KEY header missing)")
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(accounts))
	}
	if accounts[0].CurrencyPair != "BTC_USDT" || !accounts[0].Risk.Equal(mustDec("9.5")) {
		t.Fatalf("pair/risk: %q / %s", accounts[0].CurrencyPair, accounts[0].Risk)
	}
	if !accounts[0].Base.Available.Equal(mustDec("0.5")) || !accounts[0].Quote.Borrowed.Equal(mustDec("200")) {
		t.Fatalf("balances: base.avail=%s quote.borrowed=%s", accounts[0].Base.Available, accounts[0].Quote.Borrowed)
	}
}

// TestIsolated_AccountPayload_DecodesNumberAndStringForms guards the Gate
// behavior where a balance may arrive as a quoted string (REST) or a bare JSON
// number: the shared payload must decode BOTH via codec.FlexDecimal.
func TestIsolated_AccountPayload_DecodesNumberAndStringForms(t *testing.T) {
	var stringForm = []byte(`{"currency_pair":"BTC_USDT","locked":false,"risk":"9.5",
		"base":{"currency":"BTC","available":"0.5","borrowed":"0.1"},
		"quote":{"currency":"USDT","available":"1000","borrowed":"200"}}`)
	var numberForm = []byte(`{"currency_pair":"BTC_USDT","locked":false,"risk":9.5,
		"base":{"currency":"BTC","available":0.5,"borrowed":0.1},
		"quote":{"currency":"USDT","available":1000,"borrowed":200}}`)

	var form string
	var raw []byte
	for form, raw = range map[string][]byte{"rest-string": stringForm, "ws-number": numberForm} {
		var p marginAccountPayload
		if err := codec.Unmarshal(raw, &p); err != nil {
			t.Fatalf("%s: unmarshal failed: %v", form, err)
		}
		var acc types.MarginAccount = marginAccountFromPayload(&p, nil)
		if !acc.Risk.Equal(mustDec("9.5")) {
			t.Fatalf("%s: risk: %s", form, acc.Risk)
		}
		if !acc.Base.Available.Equal(mustDec("0.5")) || !acc.Quote.Borrowed.Equal(mustDec("200")) {
			t.Fatalf("%s: base.avail=%s quote.borrowed=%s", form, acc.Base.Available, acc.Quote.Borrowed)
		}
	}
}

func TestIsolated_CreateLoan_BodyAndParse(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":987654,"create_time":1700000000,"status":"loaned","side":"borrow",
			"currency":"USDT","currency_pair":"BTC_USDT","rate":"0.0002","amount":"500","days":10,
			"auto_renew":true,"left":"0","repaid":"0","paid_interest":"0","unpaid_interest":"0.01"}`)
	}))
	defer srv.Close()

	var m *Client = newMarginTestClient(t, srv.URL)
	var loan types.MarginLoan
	var err error
	loan, err = m.Isolated().CreateLoan(context.Background(), types.CreateLoanRequest{
		Side:         types.LoanSideBorrow,
		Currency:     "USDT",
		Amount:       mustDec("500"),
		Rate:         mustDec("0.0002"),
		Days:         10,
		AutoRenew:    true,
		CurrencyPair: "BTC_USDT",
	})
	if err != nil {
		t.Fatalf("CreateLoan: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/margin/loans" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["side"] != "borrow" || gotBody["currency"] != "USDT" || gotBody["amount"] != "500" {
		t.Fatalf("body core: %+v", gotBody)
	}
	if gotBody["rate"] != "0.0002" || gotBody["auto_renew"] != true || gotBody["currency_pair"] != "BTC_USDT" {
		t.Fatalf("body opts: %+v", gotBody)
	}
	if gotBody["days"].(float64) != 10 {
		t.Fatalf("days: %v", gotBody["days"])
	}
	if loan.ID != "987654" || loan.Side != types.LoanSideBorrow || loan.Status != types.LoanStatusLoaned {
		t.Fatalf("parsed loan: %+v", loan)
	}
	if loan.CreatedAtMs != 1700000000000 || !loan.Amount.Equal(mustDec("500")) || !loan.AutoRenew {
		t.Fatalf("parsed loan fields: created=%d amount=%s renew=%v", loan.CreatedAtMs, loan.Amount, loan.AutoRenew)
	}
}

func TestIsolated_RepayLoan_PathAndBody(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		_, _ = io.WriteString(w, `{"id":987654,"status":"finished","side":"borrow","currency":"USDT",
			"currency_pair":"BTC_USDT","amount":"500","repaid":"500","unpaid_interest":"0"}`)
	}))
	defer srv.Close()

	var m *Client = newMarginTestClient(t, srv.URL)
	var loan types.MarginLoan
	var err error
	loan, err = m.Isolated().RepayLoan(context.Background(), types.RepayLoanRequest{
		LoanID:       "987654",
		CurrencyPair: "BTC_USDT",
		Mode:         types.RepayModePartial,
		Amount:       mustDec("250"),
	})
	if err != nil {
		t.Fatalf("RepayLoan: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/margin/loans/987654/repayment" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["mode"] != "partial" || gotBody["amount"] != "250" || gotBody["currency_pair"] != "BTC_USDT" {
		t.Fatalf("body: %+v", gotBody)
	}
	if loan.ID != "987654" || loan.Status != types.LoanStatusFinished {
		t.Fatalf("parsed loan: %+v", loan)
	}
}

func TestIsolated_RepayLoan_AllOmitsAmount(t *testing.T) {
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody = decodeBody(t, r)
		_, _ = io.WriteString(w, `{"id":1,"status":"finished","currency_pair":"BTC_USDT"}`)
	}))
	defer srv.Close()

	var m *Client = newMarginTestClient(t, srv.URL)
	var err error
	_, err = m.Isolated().RepayLoan(context.Background(), types.RepayLoanRequest{
		LoanID:       "1",
		CurrencyPair: "BTC_USDT",
		Mode:         types.RepayModeAll,
	})
	if err != nil {
		t.Fatalf("RepayLoan: %v", err)
	}
	if gotBody["mode"] != "all" {
		t.Fatalf("mode: %v", gotBody["mode"])
	}
	if _, present := gotBody["amount"]; present {
		t.Fatalf("amount must be omitted for mode=all, got %v", gotBody["amount"])
	}
}

func TestIsolated_GetBorrowableAndTransferable(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/margin/borrowable":
			_, _ = io.WriteString(w, `{"currency":"USDT","currency_pair":"BTC_USDT","amount":"800"}`)
		case "/margin/transferable":
			_, _ = io.WriteString(w, `{"currency":"USDT","currency_pair":"BTC_USDT","amount":"123.45"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var m *Client = newMarginTestClient(t, srv.URL)

	var b types.Borrowable
	var err error
	b, err = m.Isolated().GetBorrowable(context.Background(), "USDT", "BTC_USDT")
	if err != nil {
		t.Fatalf("GetBorrowable: %v", err)
	}
	if b.Currency != "USDT" || b.CurrencyPair != "BTC_USDT" || !b.Amount.Equal(mustDec("800")) {
		t.Fatalf("borrowable: %+v", b)
	}

	var tr types.Transferable
	tr, err = m.Isolated().GetTransferable(context.Background(), "USDT", "BTC_USDT")
	if err != nil {
		t.Fatalf("GetTransferable: %v", err)
	}
	if !tr.Amount.Equal(mustDec("123.45")) {
		t.Fatalf("transferable amount: %s", tr.Amount)
	}
}

func TestIsolated_ErrorLabel_Surfaces(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"label":"INVALID_PARAM_VALUE","message":"amount too small"}`)
	}))
	defer srv.Close()

	var m *Client = newMarginTestClient(t, srv.URL)
	var err error
	_, err = m.Isolated().CreateLoan(context.Background(), types.CreateLoanRequest{
		Side: types.LoanSideBorrow, Currency: "USDT", Amount: mustDec("1"), CurrencyPair: "BTC_USDT",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ge *gate.Error
	if !errors.As(err, &ge) || ge.Label != "INVALID_PARAM_VALUE" || !gate.IsInvalidRequest(err) {
		t.Fatalf("unexpected error: %v", err)
	}
}
