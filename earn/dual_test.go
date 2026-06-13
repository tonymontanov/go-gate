/*
FILE: earn/dual_test.go

DESCRIPTION:
Contract tests for the Gate Earn Dual Investment sub-client (earn.Client.Dual())
against httptest-served Gate JSON. They pin:
  - public ListInvestmentPlans parsing (base "/earn/dual/investment_plan",
    plan_id query passthrough, apr / strike / delivery normalization);
  - the POST /earn/dual/orders body shape (plan_id, copies) and the parse of the
    returned order;
  - GetBalance parsing, including a codec.FlexDecimal number-OR-string decode on
    the amount field;
  - that a Gate {label,message} error surfaces as *gate.Error.

These reuse the newEarnTestClient harness from earn_test.go (same package).
No network: the parent client's REST base points at the test server.
*/

package earn

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/earn/types"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
)

func TestDual_ListInvestmentPlans_ParsesArray(t *testing.T) {
	var gotPath, gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = io.WriteString(w, `[{"plan_id":"BTC-7D-1","instrument":"BTC_USDT","invest_currency":"USDT",
			"exercise_currency":"BTC","delivery_time":1700600000,"apr":"0.35","min_apr":"0.20",
			"strike_price":"65000","copies":"120","per_value":"100","status":"open"}]`)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	var plans []types.DualInvestmentPlan
	var err error
	plans, err = e.Dual().ListInvestmentPlans(context.Background(), "BTC-7D-1")
	if err != nil {
		t.Fatalf("ListInvestmentPlans: %v", err)
	}
	if gotPath != "/earn/dual/investment_plan" {
		t.Fatalf("path: %q", gotPath)
	}
	if !strings.Contains(gotQuery, "plan_id=BTC-7D-1") {
		t.Fatalf("query: %q", gotQuery)
	}
	if len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(plans))
	}
	if plans[0].ID != "BTC-7D-1" || plans[0].Instrument != "BTC_USDT" {
		t.Fatalf("id/instrument: %q / %q", plans[0].ID, plans[0].Instrument)
	}
	if plans[0].InvestCurrency != "USDT" || plans[0].ExerciseCurrency != "BTC" {
		t.Fatalf("currencies: %q / %q", plans[0].InvestCurrency, plans[0].ExerciseCurrency)
	}
	if !plans[0].APR.Equal(mustDec("0.35")) || !plans[0].StrikePrice.Equal(mustDec("65000")) {
		t.Fatalf("apr/strike: %s / %s", plans[0].APR, plans[0].StrikePrice)
	}
	if plans[0].DeliveryTimeMs != 1700600000000 || !plans[0].Copies.Equal(mustDec("120")) {
		t.Fatalf("delivery/copies: %d / %s", plans[0].DeliveryTimeMs, plans[0].Copies)
	}
}

func TestDual_CreateOrder_BodyAndParse(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		_, _ = io.WriteString(w, `{"id":555111,"plan_id":"BTC-7D-1","copies":"3","invest_currency":"USDT",
			"invest_amount":"300","settlement_currency":"USDT","settlement_amount":"0","apr":"0.35",
			"status":"init","create_time":1700000000}`)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	var order types.DualOrder
	var err error
	order, err = e.Dual().CreateOrder(context.Background(), types.CreateDualOrderRequest{
		PlanID: "BTC-7D-1",
		Copies: 3,
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/earn/dual/orders" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["plan_id"] != "BTC-7D-1" {
		t.Fatalf("body plan_id: %v", gotBody["plan_id"])
	}
	// copies is sent as a JSON number; JSON decodes integers to float64.
	if c, ok := gotBody["copies"].(float64); !ok || c != 3 {
		t.Fatalf("body copies: %v (%T)", gotBody["copies"], gotBody["copies"])
	}
	if _, ok := gotBody["amount"]; ok {
		t.Fatalf("amount should be omitted when zero")
	}
	if order.ID != "555111" || order.PlanID != "BTC-7D-1" {
		t.Fatalf("order id/plan: %q / %q", order.ID, order.PlanID)
	}
	if !order.Copies.Equal(mustDec("3")) || !order.InvestAmount.Equal(mustDec("300")) {
		t.Fatalf("copies/invest: %s / %s", order.Copies, order.InvestAmount)
	}
	if order.Status != "init" || order.CreatedAtMs != 1700000000000 {
		t.Fatalf("status/created: %q / %d", order.Status, order.CreatedAtMs)
	}
}

func TestDual_CreateOrder_Validation(t *testing.T) {
	var e *Client = newEarnTestClient(t, "http://127.0.0.1:0")
	var err error
	_, err = e.Dual().CreateOrder(context.Background(), types.CreateDualOrderRequest{Copies: 1})
	if err == nil || !gate.IsInvalidRequest(err) {
		t.Fatalf("expected invalid-request for empty PlanID, got %v", err)
	}
	_, err = e.Dual().CreateOrder(context.Background(), types.CreateDualOrderRequest{PlanID: "P"})
	if err == nil || !gate.IsInvalidRequest(err) {
		t.Fatalf("expected invalid-request when neither Copies nor Amount positive, got %v", err)
	}
}

func TestDual_GetBalance_Parses(t *testing.T) {
	var gotPath, gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = io.WriteString(w, `{"currency":"USDT","amount":"1234.5","locked":"200"}`)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	var bal types.DualBalance
	var err error
	bal, err = e.Dual().GetBalance(context.Background(), "USDT")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if gotPath != "/earn/dual/balance" || !strings.Contains(gotQuery, "currency=USDT") {
		t.Fatalf("path/query: %q / %q", gotPath, gotQuery)
	}
	if bal.Currency != "USDT" || !bal.Amount.Equal(mustDec("1234.5")) || !bal.Locked.Equal(mustDec("200")) {
		t.Fatalf("balance: %q / %s / %s", bal.Currency, bal.Amount, bal.Locked)
	}
}

// TestDualBalancePayload_DecodesNumberAndStringForms guards the Gate behavior
// where the same decimal field may arrive quoted as a string OR as a bare JSON
// number. The shared payload must decode BOTH via codec.FlexDecimal.
func TestDualBalancePayload_DecodesNumberAndStringForms(t *testing.T) {
	var numberForm = []byte(`{"currency":"USDT","amount":1234.5,"locked":200}`)
	var stringForm = []byte(`{"currency":"USDT","amount":"1234.5","locked":"200"}`)

	var form string
	var raw []byte
	for form, raw = range map[string][]byte{"number": numberForm, "string": stringForm} {
		var p dualBalancePayload
		if err := codec.Unmarshal(raw, &p); err != nil {
			t.Fatalf("%s: unmarshal failed: %v", form, err)
		}
		if !p.Amount.Decimal.Equal(mustDec("1234.5")) || !p.Locked.Decimal.Equal(mustDec("200")) {
			t.Fatalf("%s: amount/locked: %s / %s", form, p.Amount.Decimal, p.Locked.Decimal)
		}
	}
}

func TestDual_RefundPreview_Parses(t *testing.T) {
	var gotPath, gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = io.WriteString(w, `{"order_id":555111,"currency":"USDT","refund_amount":"299.5","fee":"0.5"}`)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	var preview types.DualRefundPreview
	var err error
	preview, err = e.Dual().RefundPreview(context.Background(), "555111")
	if err != nil {
		t.Fatalf("RefundPreview: %v", err)
	}
	if gotPath != "/earn/dual/order-refund-preview" || !strings.Contains(gotQuery, "order_id=555111") {
		t.Fatalf("path/query: %q / %q", gotPath, gotQuery)
	}
	if preview.OrderID != "555111" || !preview.RefundAmount.Equal(mustDec("299.5")) || !preview.Fee.Equal(mustDec("0.5")) {
		t.Fatalf("preview: %q / %s / %s", preview.OrderID, preview.RefundAmount, preview.Fee)
	}
}

func TestDual_ModifyOrderReinvest_Body(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	if err := e.Dual().ModifyOrderReinvest(context.Background(), types.ModifyReinvestRequest{
		OrderID:  "555111",
		Reinvest: true,
	}); err != nil {
		t.Fatalf("ModifyOrderReinvest: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/earn/dual/modify-order-reinvest" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["order_id"] != "555111" || gotBody["reinvest"] != true {
		t.Fatalf("body: order_id=%v reinvest=%v", gotBody["order_id"], gotBody["reinvest"])
	}
}

func TestDual_CreateOrder_ErrorLabel_Surfaces(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"label":"BALANCE_NOT_ENOUGH","message":"insufficient balance"}`)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	var err error
	_, err = e.Dual().CreateOrder(context.Background(), types.CreateDualOrderRequest{
		PlanID: "BTC-7D-1", Copies: 1,
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ge *gate.Error
	if !errors.As(err, &ge) || ge.Label != "BALANCE_NOT_ENOUGH" {
		t.Fatalf("unexpected error: %v", err)
	}
}
