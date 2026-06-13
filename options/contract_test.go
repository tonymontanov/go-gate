/*
FILE: options/contract_test.go

DESCRIPTION:
Contract tests that drive the options TradingClient against an httptest.Server
returning real-shaped Gate options-order JSON. They pin:
  - the Gate signed-size encoding (buy → +size, sell → −size) and market-order
    shaping (price="0", tif="ioc");
  - client-order-id "t-" prefixing in the request body;
  - response parsing into types.OrderInfo (side from size sign, ms timestamps);
  - the PUT amend path + signed size (amend needs Side);
  - Gate {label,message} errors surfacing as *gate.Error.

No network: the parent client's REST base points at the test server.
*/

package options

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
	"github.com/tonymontanov/go-gate/v2/options/types"
)

const testContract = "BTC_USDT-20240329-50000-C"

func mustDec(s string) decimal.Decimal {
	var d decimal.Decimal
	var err error
	d, err = decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func newOptionsTestClient(t *testing.T, baseURL string) *Client {
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
		_, _ = io.WriteString(w, `{"id":123456,"contract":"`+testContract+`","size":3,"left":3,"price":"12.5",
			"tif":"gtc","text":"t-abc","status":"open","is_reduce_only":false,"is_close":false,
			"fill_price":"0","mkfr":"0.0003","tkfr":"0.0005","create_time_ms":1700000000500}`)
	}))
	defer srv.Close()

	var o *Client = newOptionsTestClient(t, srv.URL)
	var info types.OrderInfo
	var err error
	info, err = o.Trading().CreateOrder(context.Background(), types.CreateOrderRequest{
		Contract:    testContract,
		Side:        types.SideTypeBuy,
		Size:        mustDec("3"),
		Price:       mustDec("12.5"),
		TimeInForce: types.TimeInForceGTC,
		Text:        "abc",
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/options/orders" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["size"].(float64) != 3 {
		t.Fatalf("size: got %v want 3", gotBody["size"])
	}
	if gotBody["price"] != "12.5" || gotBody["tif"] != "gtc" {
		t.Fatalf("price/tif: %v / %v", gotBody["price"], gotBody["tif"])
	}
	if gotBody["text"] != "t-abc" {
		t.Fatalf("text: got %v want t-abc", gotBody["text"])
	}
	if info.OrderID != "123456" || info.Side != types.SideTypeBuy {
		t.Fatalf("orderID/side: %q / %q", info.OrderID, info.Side)
	}
	if !info.Size.Equal(mustDec("3")) || !info.Price.Equal(mustDec("12.5")) {
		t.Fatalf("size/price: %s / %s", info.Size, info.Price)
	}
	if info.Status != "open" || info.CreatedAtMs != 1700000000500 {
		t.Fatalf("status/created: %q / %d", info.Status, info.CreatedAtMs)
	}
	if !info.MakerFeeRate.Equal(mustDec("0.0003")) || !info.TakerFeeRate.Equal(mustDec("0.0005")) {
		t.Fatalf("fee rates: %s / %s", info.MakerFeeRate, info.TakerFeeRate)
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
		_, _ = io.WriteString(w, `{"id":7,"contract":"`+testContract+`","size":-2,"left":0,"price":"0",
			"tif":"ioc","status":"finished","finish_as":"filled","fill_price":"11","create_time":1700000000}`)
	}))
	defer srv.Close()

	var o *Client = newOptionsTestClient(t, srv.URL)
	var info types.OrderInfo
	var err error
	info, err = o.Trading().CreateOrder(context.Background(), types.CreateOrderRequest{
		Contract:  testContract,
		Side:      types.SideTypeSell,
		Size:      mustDec("2"),
		OrderType: types.OrderTypeMarket,
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
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
		_, _ = io.WriteString(w, `{"id":1,"contract":"`+testContract+`","size":1,"status":"finished","finish_as":"cancelled","price":"10","fill_price":"0"}`)
	}))
	defer srv.Close()

	var o *Client = newOptionsTestClient(t, srv.URL)
	var err error
	err = o.Trading().CancelOrder(context.Background(), types.CancelOrderRequest{
		Contract:      testContract,
		ClientOrderID: "myid",
	})
	if err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/options/orders/t-myid" {
		t.Fatalf("unexpected cancel request: %s %s", gotMethod, gotPath)
	}
}

func TestCancelAllOrders_NativeDelete(t *testing.T) {
	var gotPath, gotMethod, gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotQuery = r.URL.Path, r.Method, r.URL.RawQuery
		_, _ = io.WriteString(w, `[{"id":1,"contract":"`+testContract+`","size":1,"price":"10","fill_price":"0","status":"finished","finish_as":"cancelled"}]`)
	}))
	defer srv.Close()

	var o *Client = newOptionsTestClient(t, srv.URL)
	if err := o.Trading().CancelAllOrders(context.Background(), "", "BTC_USDT", ""); err != nil {
		t.Fatalf("CancelAllOrders: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/options/orders" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotQuery != "underlying=BTC_USDT" {
		t.Fatalf("unexpected query: %q", gotQuery)
	}
}

func TestGetOrder_ParsesAndMapsID(t *testing.T) {
	var gotPath string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{"id":999,"contract":"`+testContract+`","size":-4,"left":4,"price":"8.5",
			"tif":"gtc","text":"t-zz","status":"open","fill_price":"0","create_time_ms":1700000002000}`)
	}))
	defer srv.Close()

	var o *Client = newOptionsTestClient(t, srv.URL)
	var info types.OrderInfo
	var err error
	info, err = o.Trading().GetOrder(context.Background(), testContract, "999")
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if gotPath != "/options/orders/999" {
		t.Fatalf("path: %q", gotPath)
	}
	if info.Side != types.SideTypeSell || !info.Size.Equal(mustDec("4")) {
		t.Fatalf("side/size: %q / %s", info.Side, info.Size)
	}
	// GetOrder remembers the ClientOrderID ↔ OrderID mapping.
	var id string
	var ok bool
	id, ok = o.Trading().OrderIDByClientID("t-zz")
	if !ok || id != "999" {
		t.Fatalf("mapping not remembered: %q ok=%v", id, ok)
	}
}

func TestGetOpenOrders_ParsesArray(t *testing.T) {
	var gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = io.WriteString(w, `[
			{"id":1,"contract":"`+testContract+`","size":5,"left":5,"price":"10","fill_price":"0","status":"open","text":"t-a","create_time_ms":1700000000000},
			{"id":2,"contract":"`+testContract+`","size":-4,"left":4,"price":"20","fill_price":"0","status":"open","text":"t-b","create_time_ms":1700000001000}
		]`)
	}))
	defer srv.Close()

	var o *Client = newOptionsTestClient(t, srv.URL)
	var orders []types.OrderInfo
	var err error
	orders, err = o.Trading().GetOpenOrders(context.Background(), "", "BTC_USDT")
	if err != nil {
		t.Fatalf("GetOpenOrders: %v", err)
	}
	if len(orders) != 2 {
		t.Fatalf("expected 2 orders, got %d", len(orders))
	}
	if orders[0].Side != types.SideTypeBuy || orders[1].Side != types.SideTypeSell {
		t.Fatalf("side derivation: %q / %q", orders[0].Side, orders[1].Side)
	}
	if !strings.Contains(gotQuery, "status=open") || !strings.Contains(gotQuery, "underlying=BTC_USDT") {
		t.Fatalf("query: %q", gotQuery)
	}
}

func TestModifyOrder_PutPathAndSignedSize(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		_, _ = io.WriteString(w, `{"id":555,"contract":"`+testContract+`","size":-9,"left":9,"price":"13","fill_price":"0","status":"open","tif":"gtc"}`)
	}))
	defer srv.Close()

	var o *Client = newOptionsTestClient(t, srv.URL)
	var info types.OrderInfo
	var err error
	// Amending the size requires Side, because Gate's amend size is signed.
	info, err = o.Trading().ModifyOrder(context.Background(), types.ModifyOrderRequest{
		Contract: testContract,
		OrderID:  "555",
		Side:     types.SideTypeSell,
		NewSize:  mustDec("9"),
		NewPrice: mustDec("13"),
	})
	if err != nil {
		t.Fatalf("ModifyOrder: %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/options/orders/555" {
		t.Fatalf("unexpected amend request: %s %s", gotMethod, gotPath)
	}
	if gotBody["size"].(float64) != -9 {
		t.Fatalf("amend size: got %v want -9 (sell signed)", gotBody["size"])
	}
	if gotBody["price"] != "13" {
		t.Fatalf("amend price: got %v want 13", gotBody["price"])
	}
	if info.OrderID != "555" || info.Side != types.SideTypeSell {
		t.Fatalf("resp: %q / %q", info.OrderID, info.Side)
	}
}

func TestCreateOrder_ErrorLabel_Surfaces(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"label":"INVALID_PARAM_VALUE","message":"size too small"}`)
	}))
	defer srv.Close()

	var o *Client = newOptionsTestClient(t, srv.URL)
	var err error
	_, err = o.Trading().CreateOrder(context.Background(), types.CreateOrderRequest{
		Contract: testContract, Side: types.SideTypeBuy, Size: mustDec("1"), Price: mustDec("10"),
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ge *gate.Error
	if !errors.As(err, &ge) || ge.Label != "INVALID_PARAM_VALUE" || !gate.IsInvalidRequest(err) {
		t.Fatalf("unexpected error: %v", err)
	}
}
