/*
FILE: loan/loan_test.go

DESCRIPTION:
Contract tests for the multi-collateral loan client against httptest-served Gate
JSON. They pin: the public currencies list parse (unsigned), CreateOrder
request-body shaping (nested collateral_currencies array) + response parse, the
Repay body (nested repay_items), GetCurrencyQuota path/query/parse, GetLtv parse
including a codec.FlexDecimal number-or-string decode of a ratio field, and a
Gate {label,message} error surfacing as *gate.Error. No network: the parent
client's REST base points at the test server.
*/

package loan

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
	"github.com/tonymontanov/go-gate/v2/loan/types"
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

func newLoanTestClient(t *testing.T, baseURL string) *Client {
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

func TestLoan_ListCurrencies_Public(t *testing.T) {
	var gotPath, gotQuery, gotKey string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotKey = r.URL.Path, r.URL.RawQuery, r.Header.Get("KEY")
		_, _ = io.WriteString(w, `[{"currency":"BTC","precision_amount":8,"min_borrow_amount":"0",
			"ltv":"0.7","loan_type":"collateral"}]`)
	}))
	defer srv.Close()

	var l *Client = newLoanTestClient(t, srv.URL)
	var currencies []types.MultiLoanCurrency
	var err error
	currencies, err = l.ListCurrencies(context.Background(), "collateral")
	if err != nil {
		t.Fatalf("ListCurrencies: %v", err)
	}
	if gotPath != "/loan/multi_collateral/currencies" || !strings.Contains(gotQuery, "loan_type=collateral") {
		t.Fatalf("path/query: %q / %q", gotPath, gotQuery)
	}
	if gotKey != "" {
		t.Fatalf("public call should be unsigned, got KEY=%q", gotKey)
	}
	if len(currencies) != 1 || currencies[0].Currency != "BTC" || currencies[0].PrecisionAmount != 8 {
		t.Fatalf("currencies: %+v", currencies)
	}
	if !currencies[0].Ltv.Equal(mustDec("0.7")) || currencies[0].LoanType != "collateral" {
		t.Fatalf("currency fields: %+v", currencies[0])
	}
}

func TestLoan_CreateOrder_BodyAndParse(t *testing.T) {
	var gotPath, gotMethod, gotKey string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotKey = r.URL.Path, r.Method, r.Header.Get("KEY")
		gotBody = decodeBody(t, r)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"order_id":112233,"order_type":"current","borrow_currency":"USDT",
			"borrow_amount":"1000","current_ltv":"0.45","status":"borrowed","create_time":1700000000,
			"collateral_currencies":[{"currency":"BTC","amount":"0.05"},{"currency":"ETH","amount":"1"}]}`)
	}))
	defer srv.Close()

	var l *Client = newLoanTestClient(t, srv.URL)
	var order types.MultiLoanOrder
	var err error
	order, err = l.CreateOrder(context.Background(), types.CreateOrderRequest{
		OrderType:      types.MultiLoanOrderTypeCurrent,
		BorrowCurrency: "USDT",
		BorrowAmount:   mustDec("1000"),
		CollateralCurrencies: []types.CollateralItem{
			{Currency: "BTC", Amount: mustDec("0.05")},
			{Currency: "ETH", Amount: mustDec("1")},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/loan/multi_collateral/orders" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotKey == "" {
		t.Fatalf("private call should be signed (KEY header missing)")
	}
	if gotBody["order_type"] != "current" || gotBody["borrow_currency"] != "USDT" || gotBody["borrow_amount"] != "1000" {
		t.Fatalf("body core: %+v", gotBody)
	}
	// Nested collateral_currencies array shape.
	var rawColl any = gotBody["collateral_currencies"]
	var coll []any
	var ok bool
	coll, ok = rawColl.([]any)
	if !ok || len(coll) != 2 {
		t.Fatalf("collateral_currencies must be a 2-element array, got %#v", rawColl)
	}
	var first map[string]any = coll[0].(map[string]any)
	if first["currency"] != "BTC" || first["amount"] != "0.05" {
		t.Fatalf("collateral[0]: %#v", first)
	}
	if order.OrderID != "112233" || order.Status != types.MultiLoanOrderStatusBorrowed {
		t.Fatalf("parsed order: %+v", order)
	}
	if !order.CurrentLtv.Equal(mustDec("0.45")) || len(order.CollateralCurrencies) != 2 {
		t.Fatalf("parsed order fields: ltv=%s collaterals=%d", order.CurrentLtv, len(order.CollateralCurrencies))
	}
	if order.CollateralCurrencies[1].Currency != "ETH" || !order.CollateralCurrencies[1].Amount.Equal(mustDec("1")) {
		t.Fatalf("collateral parse: %+v", order.CollateralCurrencies[1])
	}
}

func TestLoan_Repay_BodyAndParse(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		_, _ = io.WriteString(w, `{"order_id":112233,"status":"repaying","borrow_currency":"USDT","borrow_amount":"1000"}`)
	}))
	defer srv.Close()

	var l *Client = newLoanTestClient(t, srv.URL)
	var order types.MultiLoanOrder
	var err error
	order, err = l.Repay(context.Background(), types.RepayRequest{
		OrderID: "112233",
		RepayItems: []types.RepayItem{
			{Currency: "USDT", Amount: mustDec("250")},
			{Currency: "ETH", RepaidAll: true},
		},
	})
	if err != nil {
		t.Fatalf("Repay: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/loan/multi_collateral/repay" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["order_id"] != "112233" {
		t.Fatalf("body order_id: %+v", gotBody)
	}
	var items []any
	var ok bool
	items, ok = gotBody["repay_items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("repay_items must be a 2-element array, got %#v", gotBody["repay_items"])
	}
	var first map[string]any = items[0].(map[string]any)
	if first["currency"] != "USDT" || first["amount"] != "250" {
		t.Fatalf("repay_items[0]: %#v", first)
	}
	var second map[string]any = items[1].(map[string]any)
	if second["repaid_all"] != true {
		t.Fatalf("repay_items[1] must carry repaid_all=true, got %#v", second)
	}
	if _, present := second["amount"]; present {
		t.Fatalf("repaid_all item must omit amount, got %v", second["amount"])
	}
	if order.OrderID != "112233" || order.Status != types.MultiLoanOrderStatusRepaying {
		t.Fatalf("parsed order: %+v", order)
	}
}

func TestLoan_GetCurrencyQuota_PathAndParse(t *testing.T) {
	var gotPath, gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = io.WriteString(w, `[{"currency":"USDT","index":"1","quota":"50000"}]`)
	}))
	defer srv.Close()

	var l *Client = newLoanTestClient(t, srv.URL)
	var quotas []types.CurrencyQuota
	var err error
	quotas, err = l.GetCurrencyQuota(context.Background(), "borrow", "USDT")
	if err != nil {
		t.Fatalf("GetCurrencyQuota: %v", err)
	}
	if gotPath != "/loan/multi_collateral/currency_quota" {
		t.Fatalf("path: %q", gotPath)
	}
	if !strings.Contains(gotQuery, "type=borrow") || !strings.Contains(gotQuery, "currency=USDT") {
		t.Fatalf("query: %q", gotQuery)
	}
	if len(quotas) != 1 || quotas[0].Currency != "USDT" || !quotas[0].Quota.Equal(mustDec("50000")) {
		t.Fatalf("quotas: %+v", quotas)
	}
}

func TestLoan_GetLtv_Parse(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"current_ltv":"0.45","liquidate_ltv":"0.85","alert_ltv":"0.75"}`)
	}))
	defer srv.Close()

	var l *Client = newLoanTestClient(t, srv.URL)
	var ltv types.Ltv
	var err error
	ltv, err = l.GetLtv(context.Background(), "112233")
	if err != nil {
		t.Fatalf("GetLtv: %v", err)
	}
	if !ltv.CurrentLtv.Equal(mustDec("0.45")) || !ltv.LiquidationLtv.Equal(mustDec("0.85")) || !ltv.AlertLtv.Equal(mustDec("0.75")) {
		t.Fatalf("parsed ltv: %+v", ltv)
	}
}

// TestLoan_LtvPayload_DecodesNumberAndStringForms guards the Gate behavior where
// a ratio may arrive as a quoted string (REST) or a bare JSON number: the shared
// payload must decode BOTH via codec.FlexDecimal.
func TestLoan_LtvPayload_DecodesNumberAndStringForms(t *testing.T) {
	var stringForm = []byte(`{"current_ltv":"0.45","liquidate_ltv":"0.85","alert_ltv":"0.75"}`)
	var numberForm = []byte(`{"current_ltv":0.45,"liquidate_ltv":0.85,"alert_ltv":0.75}`)

	var form string
	var raw []byte
	for form, raw = range map[string][]byte{"rest-string": stringForm, "number": numberForm} {
		var p ltvPayload
		if err := codec.Unmarshal(raw, &p); err != nil {
			t.Fatalf("%s: unmarshal failed: %v", form, err)
		}
		if !p.CurrentLtv.Decimal.Equal(mustDec("0.45")) || !p.LiquidationLtv.Decimal.Equal(mustDec("0.85")) {
			t.Fatalf("%s: current=%s liquidate=%s", form, p.CurrentLtv.Decimal, p.LiquidationLtv.Decimal)
		}
	}
}

func TestLoan_ErrorLabel_Surfaces(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"label":"INVALID_PARAM_VALUE","message":"insufficient collateral"}`)
	}))
	defer srv.Close()

	var l *Client = newLoanTestClient(t, srv.URL)
	var err error
	_, err = l.CreateOrder(context.Background(), types.CreateOrderRequest{
		OrderType:            types.MultiLoanOrderTypeCurrent,
		BorrowCurrency:       "USDT",
		BorrowAmount:         mustDec("1000"),
		CollateralCurrencies: []types.CollateralItem{{Currency: "BTC", Amount: mustDec("0.01")}},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ge *gate.Error
	if !errors.As(err, &ge) || ge.Label != "INVALID_PARAM_VALUE" || !gate.IsInvalidRequest(err) {
		t.Fatalf("unexpected error: %v", err)
	}
}
