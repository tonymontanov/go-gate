/*
FILE: futures/contract_test.go

DESCRIPTION:
Contract tests that drive the futures TradingClient against an httptest.Server
returning real-shaped Gate FuturesOrder JSON. They pin:
  - the Gate signed-size encoding (buy → +size, sell → −size) and market-order
    shaping (price="0", tif="ioc");
  - client-order-id "t-" prefixing in the request body;
  - response parsing into types.OrderInfo (side from size sign, ms timestamps);
  - native cancel-all (DELETE ?contract=) and batch create per-element status;
  - Gate {label,message} errors surfacing as *gate.Error.

No network: the parent client's REST base points at the test server.
*/

package futures

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
	"github.com/tonymontanov/go-gate/v2/futures/types"
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

func newFuturesTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	var parent *gate.Client
	var err error
	parent, err = gate.NewClient(gate.Config{
		APIKey:    "test-key",
		SecretKey: "test-secret",
		Settle:    "usdt",
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
		_, _ = io.WriteString(w, `{"id":123456,"contract":"BTC_USDT","size":3,"left":3,"price":"30000",
			"tif":"gtc","text":"t-abc","status":"open","is_reduce_only":false,"is_close":false,
			"fill_price":"0","create_time_ms":1700000000500}`)
	}))
	defer srv.Close()

	var fut *Client = newFuturesTestClient(t, srv.URL)
	var info types.OrderInfo
	var err error
	info, err = fut.Trading().CreateOrder(context.Background(), types.CreateOrderRequest{
		Contract:    "BTC_USDT",
		Side:        types.SideTypeBuy,
		Size:        mustDec("3"),
		Price:       mustDec("30000"),
		TimeInForce: types.TimeInForceGTC,
		Text:        "abc",
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/futures/usdt/orders" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	// Signed-size: positive for buy; price string; tif; auto t- prefix.
	if gotBody["size"].(float64) != 3 {
		t.Fatalf("size: got %v want 3", gotBody["size"])
	}
	if gotBody["price"] != "30000" || gotBody["tif"] != "gtc" {
		t.Fatalf("price/tif: %v / %v", gotBody["price"], gotBody["tif"])
	}
	if gotBody["text"] != "t-abc" {
		t.Fatalf("text: got %v want t-abc", gotBody["text"])
	}
	// Response mapping.
	if info.OrderID != "123456" || info.Side != types.SideTypeBuy {
		t.Fatalf("orderID/side: %q / %q", info.OrderID, info.Side)
	}
	if !info.Size.Equal(mustDec("3")) || !info.Price.Equal(mustDec("30000")) {
		t.Fatalf("size/price: %s / %s", info.Size, info.Price)
	}
	if info.Status != "open" || info.CreatedAtMs != 1700000000500 {
		t.Fatalf("status/created: %q / %d", info.Status, info.CreatedAtMs)
	}
	if info.ClientOrderID != "t-abc" {
		t.Fatalf("clientOrderID: %q", info.ClientOrderID)
	}
}

func TestCreateOrder_MarketSell_Encoding(t *testing.T) {
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody = decodeBody(t, r)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":7,"contract":"ETH_USDT","size":-2,"left":0,"price":"0",
			"tif":"ioc","status":"finished","finish_as":"filled","fill_price":"2500","create_time":1700000000}`)
	}))
	defer srv.Close()

	var fut *Client = newFuturesTestClient(t, srv.URL)
	var info types.OrderInfo
	var err error
	info, err = fut.Trading().CreateOrder(context.Background(), types.CreateOrderRequest{
		Contract:  "ETH_USDT",
		Side:      types.SideTypeSell,
		Size:      mustDec("2"),
		OrderType: types.OrderTypeMarket,
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	// Sell → negative size; market → price "0", tif "ioc".
	if gotBody["size"].(float64) != -2 {
		t.Fatalf("size: got %v want -2", gotBody["size"])
	}
	if gotBody["price"] != "0" || gotBody["tif"] != "ioc" {
		t.Fatalf("market shaping: price=%v tif=%v", gotBody["price"], gotBody["tif"])
	}
	if info.Side != types.SideTypeSell || !info.Size.Equal(mustDec("2")) {
		t.Fatalf("resp side/size: %q / %s", info.Side, info.Size)
	}
	if info.OrderType != types.OrderTypeMarket || info.FinishAs != "filled" {
		t.Fatalf("type/finishAs: %q / %q", info.OrderType, info.FinishAs)
	}
	if info.CreatedAtMs != 1700000000000 {
		t.Fatalf("created from seconds: got %d", info.CreatedAtMs)
	}
}

func TestCancelOrder_ByClientID_PathAndMethod(t *testing.T) {
	var gotPath, gotMethod string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_, _ = io.WriteString(w, `{"id":1,"contract":"BTC_USDT","size":1,"status":"finished","finish_as":"cancelled","price":"100","fill_price":"0"}`)
	}))
	defer srv.Close()

	var fut *Client = newFuturesTestClient(t, srv.URL)
	var err error
	err = fut.Trading().CancelOrder(context.Background(), types.CancelOrderRequest{
		Contract:      "BTC_USDT",
		ClientOrderID: "myid",
	})
	if err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/futures/usdt/orders/t-myid" {
		t.Fatalf("unexpected cancel request: %s %s", gotMethod, gotPath)
	}
}

func TestCancelAllOrders_NativeDelete(t *testing.T) {
	var gotPath, gotMethod, gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotQuery = r.URL.Path, r.Method, r.URL.RawQuery
		_, _ = io.WriteString(w, `[{"id":1,"contract":"BTC_USDT","size":1,"price":"100","fill_price":"0","status":"finished","finish_as":"cancelled"}]`)
	}))
	defer srv.Close()

	var fut *Client = newFuturesTestClient(t, srv.URL)
	if err := fut.Trading().CancelAllOrders(context.Background(), "BTC_USDT"); err != nil {
		t.Fatalf("CancelAllOrders: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/futures/usdt/orders" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotQuery != "contract=BTC_USDT" {
		t.Fatalf("unexpected query: %q", gotQuery)
	}
}

func TestGetOpenOrders_ParsesArray(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[
			{"id":1,"contract":"BTC_USDT","size":5,"left":5,"price":"100","fill_price":"0","status":"open","text":"t-a","create_time_ms":1700000000000},
			{"id":2,"contract":"BTC_USDT","size":-4,"left":4,"price":"200","fill_price":"0","status":"open","text":"t-b","create_time_ms":1700000001000}
		]`)
	}))
	defer srv.Close()

	var fut *Client = newFuturesTestClient(t, srv.URL)
	var orders []types.OrderInfo
	var err error
	orders, err = fut.Trading().GetOpenOrders(context.Background(), "BTC_USDT")
	if err != nil {
		t.Fatalf("GetOpenOrders: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("expected 2 orders, got %d", len(orders))
	}
	if orders[0].Side != types.SideTypeBuy || orders[1].Side != types.SideTypeSell {
		t.Fatalf("side derivation: %q / %q", orders[0].Side, orders[1].Side)
	}
	if !orders[1].Size.Equal(mustDec("4")) {
		t.Fatalf("abs size: %s", orders[1].Size)
	}
}

func TestCreateBatchOrders_PartialSuccess(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `[
			{"succeeded":true,"id":10,"contract":"BTC_USDT","size":1,"left":1,"price":"100","fill_price":"0","status":"open","text":"t-ok"},
			{"succeeded":false,"label":"BALANCE_NOT_ENOUGH","detail":"insufficient","contract":"BTC_USDT","text":"t-bad"}
		]`)
	}))
	defer srv.Close()

	var fut *Client = newFuturesTestClient(t, srv.URL)
	var infos []types.OrderInfo
	var err error
	infos, err = fut.Trading().CreateBatchOrders(context.Background(), []types.CreateOrderRequest{
		{Contract: "BTC_USDT", Side: types.SideTypeBuy, Size: mustDec("1"), Price: mustDec("100"), Text: "ok"},
		{Contract: "BTC_USDT", Side: types.SideTypeBuy, Size: mustDec("1"), Price: mustDec("100"), Text: "bad"},
	})
	if err == nil {
		t.Fatalf("expected aggregated error for the rejected element")
	}
	if !gate.IsExchange(err) {
		t.Fatalf("expected Exchange-kind error, got %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("expected 2 infos, got %d", len(infos))
	}
	if infos[0].OrderID != "10" || infos[1].OrderID != "" {
		t.Fatalf("batch infos: %q / %q", infos[0].OrderID, infos[1].OrderID)
	}
}

// TestModifyBatchOrders_NativeBatchAmend pins the native batch_amend_orders path:
// the request hits POST .../batch_amend_orders, each item carries the signed size
// (sell → negative) plus the numeric order_id or client text, and the per-element
// succeeded/label response maps like the batch-create path.
func TestModifyBatchOrders_NativeBatchAmend(t *testing.T) {
	var gotPath string
	var gotBody []byte
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, `[
			{"succeeded":true,"id":10,"contract":"BTC_USDT","size":-5,"left":-5,"price":"101","fill_price":"0","status":"open","text":"t-a"},
			{"succeeded":false,"label":"ORDER_NOT_FOUND","detail":"gone","contract":"BTC_USDT","text":"t-b"}
		]`)
	}))
	defer srv.Close()

	var fut *Client = newFuturesTestClient(t, srv.URL)
	var infos []types.OrderInfo
	var err error
	infos, err = fut.Trading().ModifyBatchOrders(context.Background(), []types.ModifyOrderRequest{
		{Contract: "BTC_USDT", OrderID: "10", Side: types.SideTypeSell, NewSize: mustDec("5"), NewPrice: mustDec("101")},
		{Contract: "BTC_USDT", ClientOrderID: "b", Side: types.SideTypeSell, NewSize: mustDec("3")},
	})
	if err == nil {
		t.Fatalf("expected aggregated error for the rejected element")
	}
	if gotPath[len(gotPath)-len("/batch_amend_orders"):] != "/batch_amend_orders" {
		t.Fatalf("path=%s, want suffix /batch_amend_orders", gotPath)
	}

	var items []map[string]any
	if err = json.Unmarshal(gotBody, &items); err != nil {
		t.Fatalf("body not a JSON array: %v (%s)", err, gotBody)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0]["order_id"] != float64(10) || items[0]["size"] != float64(-5) {
		t.Fatalf("item0 order_id/size: %v / %v", items[0]["order_id"], items[0]["size"])
	}
	if items[1]["text"] != "t-b" || items[1]["size"] != float64(-3) {
		t.Fatalf("item1 text/size: %v / %v", items[1]["text"], items[1]["size"])
	}
	if len(infos) != 2 || infos[0].OrderID != "10" || infos[1].OrderID != "" {
		t.Fatalf("infos: %+v", infos)
	}
}

func TestCreateOrder_ErrorLabel_Surfaces(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"label":"INVALID_PARAM_VALUE","message":"size too small"}`)
	}))
	defer srv.Close()

	var fut *Client = newFuturesTestClient(t, srv.URL)
	var err error
	_, err = fut.Trading().CreateOrder(context.Background(), types.CreateOrderRequest{
		Contract: "BTC_USDT", Side: types.SideTypeBuy, Size: mustDec("1"), Price: mustDec("100"),
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ge *gate.Error
	if !errors.As(err, &ge) || ge.Label != "INVALID_PARAM_VALUE" || !gate.IsInvalidRequest(err) {
		t.Fatalf("unexpected error: %v", err)
	}
}
