/*
FILE: flashswap/flashswap_test.go

DESCRIPTION:
Contract tests for the flash-swap client against httptest-served Gate JSON. They
pin: the public currencies list parse (unsigned), PreviewOrder request-body
shaping + response parse, CreateOrder body (preview_id) + response parse, GetOrder
path/parse, and a Gate {label,message} error surfacing as *gate.Error. No
network: the parent client's REST base points at the test server.
*/

package flashswap

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
	"github.com/tonymontanov/go-gate/v2/flashswap/types"
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

func newFlashSwapTestClient(t *testing.T, baseURL string) *Client {
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

func TestFlashSwap_ListCurrencies_Public(t *testing.T) {
	var gotPath, gotKey string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey = r.URL.Path, r.Header.Get("KEY")
		_, _ = io.WriteString(w, `[{"currency":"BTC","min_amount":"0.001","max_amount":"5",
			"buy_currencies":[{"currency":"USDT","min_amount":"10","max_amount":"200000"}]}]`)
	}))
	defer srv.Close()

	var fs *Client = newFlashSwapTestClient(t, srv.URL)
	var currencies []types.FlashSwapCurrency
	var err error
	currencies, err = fs.ListCurrencies(context.Background())
	if err != nil {
		t.Fatalf("ListCurrencies: %v", err)
	}
	if gotPath != "/flash_swap/currencies" {
		t.Fatalf("path: %q", gotPath)
	}
	if gotKey != "" {
		t.Fatalf("public call should be unsigned, got KEY=%q", gotKey)
	}
	if len(currencies) != 1 || currencies[0].Currency != "BTC" {
		t.Fatalf("currencies: %+v", currencies)
	}
	if !currencies[0].MaxAmount.Equal(mustDec("5")) || len(currencies[0].BuyCurrencies) != 1 {
		t.Fatalf("currency fields: %+v", currencies[0])
	}
	if currencies[0].BuyCurrencies[0].Currency != "USDT" || !currencies[0].BuyCurrencies[0].MaxAmount.Equal(mustDec("200000")) {
		t.Fatalf("buy currency: %+v", currencies[0].BuyCurrencies[0])
	}
}

func TestFlashSwap_PreviewOrder_BodyAndParse(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		_, _ = io.WriteString(w, `{"preview_id":"pv-123","sell_currency":"BTC","sell_amount":"0.5",
			"buy_currency":"USDT","buy_amount":"30000","price":"60000"}`)
	}))
	defer srv.Close()

	var fs *Client = newFlashSwapTestClient(t, srv.URL)
	var preview types.FlashSwapPreview
	var err error
	preview, err = fs.PreviewOrder(context.Background(), types.PreviewRequest{
		SellCurrency: "BTC",
		BuyCurrency:  "USDT",
		SellAmount:   mustDec("0.5"),
	})
	if err != nil {
		t.Fatalf("PreviewOrder: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/flash_swap/orders/preview" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["sell_currency"] != "BTC" || gotBody["buy_currency"] != "USDT" || gotBody["sell_amount"] != "0.5" {
		t.Fatalf("body: %+v", gotBody)
	}
	if _, present := gotBody["buy_amount"]; present {
		t.Fatalf("buy_amount must be omitted when sell_amount supplied, got %v", gotBody["buy_amount"])
	}
	if preview.PreviewID != "pv-123" || !preview.BuyAmount.Equal(mustDec("30000")) || !preview.Price.Equal(mustDec("60000")) {
		t.Fatalf("parsed preview: %+v", preview)
	}
}

func TestFlashSwap_PreviewOrder_RequiresExactlyOneAmount(t *testing.T) {
	var fs *Client = newFlashSwapTestClient(t, "http://127.0.0.1:0")
	var err error
	_, err = fs.PreviewOrder(context.Background(), types.PreviewRequest{
		SellCurrency: "BTC",
		BuyCurrency:  "USDT",
	})
	if err == nil || !gate.IsInvalidRequest(err) {
		t.Fatalf("expected invalid-request error when neither amount supplied, got %v", err)
	}
	_, err = fs.PreviewOrder(context.Background(), types.PreviewRequest{
		SellCurrency: "BTC",
		BuyCurrency:  "USDT",
		SellAmount:   mustDec("1"),
		BuyAmount:    mustDec("1"),
	})
	if err == nil || !gate.IsInvalidRequest(err) {
		t.Fatalf("expected invalid-request error when both amounts supplied, got %v", err)
	}
}

func TestFlashSwap_CreateOrder_BodyAndParse(t *testing.T) {
	var gotPath, gotMethod, gotKey string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotKey = r.URL.Path, r.Method, r.Header.Get("KEY")
		gotBody = decodeBody(t, r)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":556677,"create_time":1700000000,"update_time":1700000001,"user_id":42,
			"sell_currency":"BTC","sell_amount":"0.5","buy_currency":"USDT","buy_amount":"30000",
			"price":"60000","status":1}`)
	}))
	defer srv.Close()

	var fs *Client = newFlashSwapTestClient(t, srv.URL)
	var order types.FlashSwapOrder
	var err error
	order, err = fs.CreateOrder(context.Background(), types.CreateOrderRequest{
		PreviewID:    "pv-123",
		SellCurrency: "BTC",
		SellAmount:   mustDec("0.5"),
		BuyCurrency:  "USDT",
		BuyAmount:    mustDec("30000"),
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/flash_swap/orders" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotKey == "" {
		t.Fatalf("private call should be signed (KEY header missing)")
	}
	if gotBody["preview_id"] != "pv-123" || gotBody["sell_currency"] != "BTC" || gotBody["buy_amount"] != "30000" {
		t.Fatalf("body: %+v", gotBody)
	}
	if order.ID != "556677" || order.Status != types.FlashSwapOrderStatusSuccess || order.UserID != 42 {
		t.Fatalf("parsed order: %+v", order)
	}
	if order.CreatedAtMs != 1700000000000 || !order.Price.Equal(mustDec("60000")) {
		t.Fatalf("parsed order fields: created=%d price=%s", order.CreatedAtMs, order.Price)
	}
}

func TestFlashSwap_GetOrder_PathAndParse(t *testing.T) {
	var gotPath string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{"id":556677,"sell_currency":"BTC","sell_amount":"0.5",
			"buy_currency":"USDT","buy_amount":"30000","price":"60000","status":1}`)
	}))
	defer srv.Close()

	var fs *Client = newFlashSwapTestClient(t, srv.URL)
	var order types.FlashSwapOrder
	var err error
	order, err = fs.GetOrder(context.Background(), "556677")
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if gotPath != "/flash_swap/orders/556677" {
		t.Fatalf("path: %q", gotPath)
	}
	if order.ID != "556677" || !order.SellAmount.Equal(mustDec("0.5")) {
		t.Fatalf("parsed order: %+v", order)
	}
}

func TestFlashSwap_ErrorLabel_Surfaces(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"label":"INVALID_PARAM_VALUE","message":"preview expired"}`)
	}))
	defer srv.Close()

	var fs *Client = newFlashSwapTestClient(t, srv.URL)
	var err error
	_, err = fs.CreateOrder(context.Background(), types.CreateOrderRequest{
		PreviewID:    "pv-123",
		SellCurrency: "BTC",
		SellAmount:   mustDec("0.5"),
		BuyCurrency:  "USDT",
		BuyAmount:    mustDec("30000"),
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ge *gate.Error
	if !errors.As(err, &ge) || ge.Label != "INVALID_PARAM_VALUE" || !gate.IsInvalidRequest(err) {
		t.Fatalf("unexpected error: %v", err)
	}
}
