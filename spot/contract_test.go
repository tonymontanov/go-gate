/*
FILE: spot/contract_test.go

DESCRIPTION:
Contract tests that drive the spot TradingClient against an httptest.Server
returning real-shaped Gate spot Order JSON. They pin:
  - the explicit side/type/amount/account/time_in_force request shaping;
  - market-order shaping (type="market", no price, tif="ioc") and the market-BUY
    quote-amount convention;
  - client-order-id "t-" prefixing in the request body;
  - response parsing into types.OrderInfo (string id, ms timestamps, FilledAmount);
  - PATCH amend with the currency_pair query, native cancel (currency_pair query),
    batch create per-element status, and Gate {label,message} errors as *gate.Error.

No network: the parent client's REST base points at the test server.
*/

package spot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/spot/types"
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

func newSpotTestClient(t *testing.T, baseURL string) *Client {
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

func TestCreateOrder_LimitBuy_Encoding(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"123456","currency_pair":"BTC_USDT","side":"buy","type":"limit",
			"account":"spot","amount":"0.5","price":"30000","left":"0.5","filled_total":"0",
			"avg_deal_price":"0","time_in_force":"gtc","status":"open","text":"t-abc",
			"create_time_ms":1700000000500,"update_time_ms":1700000000500}`)
	}))
	defer srv.Close()

	var sp *Client = newSpotTestClient(t, srv.URL)
	var info types.OrderInfo
	var err error
	info, err = sp.Trading().CreateOrder(context.Background(), types.CreateOrderRequest{
		CurrencyPair: "BTC_USDT",
		Side:         types.SideTypeBuy,
		Amount:       mustDec("0.5"),
		Price:        mustDec("30000"),
		TimeInForce:  types.TimeInForceGTC,
		Text:         "abc",
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/spot/orders" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["side"] != "buy" || gotBody["type"] != "limit" || gotBody["account"] != "spot" {
		t.Fatalf("side/type/account: %v / %v / %v", gotBody["side"], gotBody["type"], gotBody["account"])
	}
	if gotBody["amount"] != "0.5" || gotBody["price"] != "30000" || gotBody["time_in_force"] != "gtc" {
		t.Fatalf("amount/price/tif: %v / %v / %v", gotBody["amount"], gotBody["price"], gotBody["time_in_force"])
	}
	if gotBody["text"] != "t-abc" {
		t.Fatalf("text: got %v want t-abc", gotBody["text"])
	}
	// Response mapping.
	if info.OrderID != "123456" || info.Side != types.SideTypeBuy {
		t.Fatalf("orderID/side: %q / %q", info.OrderID, info.Side)
	}
	if !info.Amount.Equal(mustDec("0.5")) || !info.Price.Equal(mustDec("30000")) {
		t.Fatalf("amount/price: %s / %s", info.Amount, info.Price)
	}
	if info.Status != "open" || info.CreatedAtMs != 1700000000500 {
		t.Fatalf("status/created: %q / %d", info.Status, info.CreatedAtMs)
	}
	if info.ClientOrderID != "t-abc" {
		t.Fatalf("clientOrderID: %q", info.ClientOrderID)
	}
}

func TestCreateOrder_MarketBuy_Encoding(t *testing.T) {
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody = decodeBody(t, r)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"7","currency_pair":"ETH_USDT","side":"buy","type":"market",
			"account":"spot","amount":"100","price":"0","left":"0","filled_total":"100","avg_deal_price":"2500",
			"time_in_force":"ioc","status":"closed","finish_as":"filled","create_time":"1700000000"}`)
	}))
	defer srv.Close()

	var sp *Client = newSpotTestClient(t, srv.URL)
	var info types.OrderInfo
	var err error
	// Market BUY: Amount is the QUOTE amount to spend.
	info, err = sp.Trading().CreateOrder(context.Background(), types.CreateOrderRequest{
		CurrencyPair: "ETH_USDT",
		Side:         types.SideTypeBuy,
		Amount:       mustDec("100"),
		OrderType:    types.OrderTypeMarket,
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	// Market → type "market", tif "ioc", no price field sent.
	if gotBody["type"] != "market" || gotBody["time_in_force"] != "ioc" {
		t.Fatalf("market shaping: type=%v tif=%v", gotBody["type"], gotBody["time_in_force"])
	}
	if _, hasPrice := gotBody["price"]; hasPrice {
		t.Fatalf("market order must not send price, got %v", gotBody["price"])
	}
	if gotBody["amount"] != "100" {
		t.Fatalf("amount: got %v want 100", gotBody["amount"])
	}
	if info.OrderType != types.OrderTypeMarket || info.FinishAs != "filled" {
		t.Fatalf("type/finishAs: %q / %q", info.OrderType, info.FinishAs)
	}
	if info.CreatedAtMs != 1700000000000 {
		t.Fatalf("created from seconds: got %d", info.CreatedAtMs)
	}
}

func TestCancelOrder_ByClientID_PathQueryMethod(t *testing.T) {
	var gotPath, gotMethod, gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotQuery = r.URL.Path, r.Method, r.URL.RawQuery
		_, _ = io.WriteString(w, `{"id":"1","currency_pair":"BTC_USDT","side":"buy","amount":"1","price":"100","status":"cancelled","finish_as":"cancelled"}`)
	}))
	defer srv.Close()

	var sp *Client = newSpotTestClient(t, srv.URL)
	var err error
	err = sp.Trading().CancelOrder(context.Background(), types.CancelOrderRequest{
		CurrencyPair:  "BTC_USDT",
		ClientOrderID: "myid",
	})
	if err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/spot/orders/t-myid" {
		t.Fatalf("unexpected cancel request: %s %s", gotMethod, gotPath)
	}
	if gotQuery != "currency_pair=BTC_USDT" {
		t.Fatalf("unexpected query: %q", gotQuery)
	}
}

func TestModifyOrder_PatchPriceAmount(t *testing.T) {
	var gotPath, gotMethod, gotQuery string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotQuery = r.URL.Path, r.Method, r.URL.RawQuery
		gotBody = decodeBody(t, r)
		_, _ = io.WriteString(w, `{"id":"55","currency_pair":"BTC_USDT","side":"buy","type":"limit","amount":"0.7","price":"31000","left":"0.7","status":"open"}`)
	}))
	defer srv.Close()

	var sp *Client = newSpotTestClient(t, srv.URL)
	var info types.OrderInfo
	var err error
	info, err = sp.Trading().ModifyOrder(context.Background(), types.ModifyOrderRequest{
		CurrencyPair: "BTC_USDT",
		OrderID:      "55",
		NewAmount:    mustDec("0.7"),
		NewPrice:     mustDec("31000"),
	})
	if err != nil {
		t.Fatalf("ModifyOrder: %v", err)
	}
	if gotMethod != http.MethodPatch || gotPath != "/spot/orders/55" {
		t.Fatalf("unexpected amend request: %s %s", gotMethod, gotPath)
	}
	if gotQuery != "currency_pair=BTC_USDT" {
		t.Fatalf("unexpected query: %q", gotQuery)
	}
	if gotBody["amount"] != "0.7" || gotBody["price"] != "31000" {
		t.Fatalf("amend body: amount=%v price=%v", gotBody["amount"], gotBody["price"])
	}
	if !info.Price.Equal(mustDec("31000")) || !info.Amount.Equal(mustDec("0.7")) {
		t.Fatalf("amend resp: price=%s amount=%s", info.Price, info.Amount)
	}
}

func TestCancelAllOrders_NativeDelete(t *testing.T) {
	var gotPath, gotMethod, gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotQuery = r.URL.Path, r.Method, r.URL.RawQuery
		_, _ = io.WriteString(w, `[{"id":"1","currency_pair":"BTC_USDT","side":"buy","amount":"1","price":"100","status":"cancelled"}]`)
	}))
	defer srv.Close()

	var sp *Client = newSpotTestClient(t, srv.URL)
	if err := sp.Trading().CancelAllOrders(context.Background(), "BTC_USDT"); err != nil {
		t.Fatalf("CancelAllOrders: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/spot/orders" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotQuery != "currency_pair=BTC_USDT" {
		t.Fatalf("unexpected query: %q", gotQuery)
	}
}

func TestGetOpenOrders_ParsesArray(t *testing.T) {
	var gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = io.WriteString(w, `[
			{"id":"1","currency_pair":"BTC_USDT","side":"buy","amount":"5","left":"5","price":"100","status":"open","text":"t-a","create_time_ms":1700000000000},
			{"id":"2","currency_pair":"BTC_USDT","side":"sell","amount":"4","left":"1","price":"200","status":"open","text":"t-b","create_time_ms":1700000001000}
		]`)
	}))
	defer srv.Close()

	var sp *Client = newSpotTestClient(t, srv.URL)
	var orders []types.OrderInfo
	var err error
	orders, err = sp.Trading().GetOpenOrders(context.Background(), "BTC_USDT")
	if err != nil {
		t.Fatalf("GetOpenOrders: %v", err)
	}
	if gotQuery != "currency_pair=BTC_USDT&status=open" {
		t.Fatalf("unexpected query: %q", gotQuery)
	}
	if len(orders) != 2 {
		t.Fatalf("expected 2 orders, got %d", len(orders))
	}
	if orders[0].Side != types.SideTypeBuy || orders[1].Side != types.SideTypeSell {
		t.Fatalf("side: %q / %q", orders[0].Side, orders[1].Side)
	}
	// FilledAmount = amount − left: order[1] = 4 − 1 = 3.
	if !orders[1].FilledAmount.Equal(mustDec("3")) {
		t.Fatalf("filled amount: %s", orders[1].FilledAmount)
	}
}

func TestCreateBatchOrders_PartialSuccess(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `[
			{"succeeded":true,"id":"10","currency_pair":"BTC_USDT","side":"buy","amount":"1","left":"1","price":"100","status":"open","text":"t-ok"},
			{"succeeded":false,"label":"BALANCE_NOT_ENOUGH","message":"insufficient","currency_pair":"BTC_USDT","text":"t-bad"}
		]`)
	}))
	defer srv.Close()

	var sp *Client = newSpotTestClient(t, srv.URL)
	var infos []types.OrderInfo
	var err error
	infos, err = sp.Trading().CreateBatchOrders(context.Background(), []types.CreateOrderRequest{
		{CurrencyPair: "BTC_USDT", Side: types.SideTypeBuy, Amount: mustDec("1"), Price: mustDec("100"), Text: "ok"},
		{CurrencyPair: "BTC_USDT", Side: types.SideTypeBuy, Amount: mustDec("1"), Price: mustDec("100"), Text: "bad"},
	})
	if err == nil {
		t.Fatalf("expected aggregated error for the rejected element")
	}
	if len(infos) != 2 {
		t.Fatalf("expected 2 infos, got %d", len(infos))
	}
	if infos[0].OrderID != "10" || infos[1].OrderID != "" {
		t.Fatalf("batch infos: %q / %q", infos[0].OrderID, infos[1].OrderID)
	}
}

func TestCreateOrder_ErrorLabel_Surfaces(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"label":"INVALID_PARAM_VALUE","message":"amount too small"}`)
	}))
	defer srv.Close()

	var sp *Client = newSpotTestClient(t, srv.URL)
	var err error
	_, err = sp.Trading().CreateOrder(context.Background(), types.CreateOrderRequest{
		CurrencyPair: "BTC_USDT", Side: types.SideTypeBuy, Amount: mustDec("1"), Price: mustDec("100"),
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ge *gate.Error
	if !errors.As(err, &ge) || ge.Label != "INVALID_PARAM_VALUE" || !gate.IsInvalidRequest(err) {
		t.Fatalf("unexpected error: %v", err)
	}
}
