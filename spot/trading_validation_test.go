/*
FILE: spot/trading_validation_test.go

DESCRIPTION:
Pure (no-network) tests for spot request validation in buildCreateOrderBody /
buildAmendBody / normalizeClientID. They pin the Gate spot rules the SDK enforces
before sending a request: explicit side, positive amount, limit needs a price,
market needs ioc/fok, account/type defaulting, and amend identity/field rules.
*/

package spot

import (
	"testing"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/spot/types"
)

func TestBuildCreateOrderBody_LimitDefaults(t *testing.T) {
	var body map[string]any
	var err error
	body, err = buildCreateOrderBody(types.CreateOrderRequest{
		CurrencyPair: "BTC_USDT",
		Side:         types.SideTypeSell,
		Amount:       mustDec("0.1"),
		Price:        mustDec("50000"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body["type"] != "limit" || body["time_in_force"] != "gtc" || body["account"] != "spot" {
		t.Fatalf("defaults: type=%v tif=%v account=%v", body["type"], body["time_in_force"], body["account"])
	}
	if body["price"] != "50000" {
		t.Fatalf("price: %v", body["price"])
	}
}

func TestBuildCreateOrderBody_MarketRequiresIocFok(t *testing.T) {
	var _, err = buildCreateOrderBody(types.CreateOrderRequest{
		CurrencyPair: "BTC_USDT",
		Side:         types.SideTypeBuy,
		Amount:       mustDec("100"),
		OrderType:    types.OrderTypeMarket,
		TimeInForce:  types.TimeInForceGTC, // invalid for market
	})
	if err == nil || !gate.IsInvalidRequest(err) {
		t.Fatalf("expected invalid-request error, got %v", err)
	}
}

func TestBuildCreateOrderBody_MarketDefaultsIoc_NoPrice(t *testing.T) {
	var body map[string]any
	var err error
	body, err = buildCreateOrderBody(types.CreateOrderRequest{
		CurrencyPair: "BTC_USDT",
		Side:         types.SideTypeBuy,
		Amount:       mustDec("100"),
		OrderType:    types.OrderTypeMarket,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body["time_in_force"] != "ioc" {
		t.Fatalf("market tif default: %v", body["time_in_force"])
	}
	if _, has := body["price"]; has {
		t.Fatalf("market order must omit price")
	}
}

func TestBuildCreateOrderBody_Rejections(t *testing.T) {
	var cases = []struct {
		name string
		req  types.CreateOrderRequest
	}{
		{"empty pair", types.CreateOrderRequest{Side: types.SideTypeBuy, Amount: mustDec("1"), Price: mustDec("1")}},
		{"no side", types.CreateOrderRequest{CurrencyPair: "BTC_USDT", Amount: mustDec("1"), Price: mustDec("1")}},
		{"zero amount", types.CreateOrderRequest{CurrencyPair: "BTC_USDT", Side: types.SideTypeBuy, Price: mustDec("1")}},
		{"limit no price", types.CreateOrderRequest{CurrencyPair: "BTC_USDT", Side: types.SideTypeBuy, Amount: mustDec("1"), OrderType: types.OrderTypeLimit}},
	}
	for _, c := range cases {
		if _, err := buildCreateOrderBody(c.req); err == nil || !gate.IsInvalidRequest(err) {
			t.Errorf("%s: expected invalid-request error, got %v", c.name, err)
		}
	}
}

func TestBuildAmendBody_Rules(t *testing.T) {
	// Both ids set → error.
	if _, err := buildAmendBody(types.ModifyOrderRequest{OrderID: "1", ClientOrderID: "x", NewPrice: mustDec("1")}); err == nil {
		t.Fatal("expected error when both ids set")
	}
	// Neither field set → error.
	if _, err := buildAmendBody(types.ModifyOrderRequest{OrderID: "1"}); err == nil {
		t.Fatal("expected error when no NewPrice/NewAmount")
	}
	// Valid: one id + price.
	var body map[string]any
	var err error
	body, err = buildAmendBody(types.ModifyOrderRequest{OrderID: "1", NewPrice: mustDec("123"), NewAmount: mustDec("0.5")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body["price"] != "123" || body["amount"] != "0.5" {
		t.Fatalf("amend body: %v", body)
	}
}

func TestNormalizeClientID(t *testing.T) {
	var got string
	var err error
	got, err = normalizeClientID("abc")
	if err != nil || got != "t-abc" {
		t.Fatalf("normalize abc: %q %v", got, err)
	}
	got, err = normalizeClientID("t-abc")
	if err != nil || got != "t-abc" {
		t.Fatalf("normalize t-abc: %q %v", got, err)
	}
	if _, err = normalizeClientID("bad id!"); err == nil {
		t.Fatal("expected error for invalid chars")
	}
}

// TestOrderInfoFromPayload_WSDerivesTerminalStatus — the spot WS order push has
// no "status" field; it signals lifecycle via "finish_as". orderInfoFromPayload
// must derive a usable terminal Status from finish_as when status is absent
// (calibrated live 2026-06-12), so terminal detection works for WS pushes too.
func TestOrderInfoFromPayload_WSDerivesTerminalStatus(t *testing.T) {
	type tc struct {
		finishAs   string
		wantStatus string
	}
	var cases = []tc{
		{"open", types.OrderStatusOpen},
		{"filled", types.OrderStatusClosed},
		{"cancelled", types.OrderStatusCancelled},
		{"ioc", types.OrderStatusCancelled},
	}
	var i int
	for i = 0; i < len(cases); i++ {
		// WS-shaped payload: status empty, finish_as set.
		var p = spotOrderPayload{CurrencyPair: "BTC_USDT", FinishAs: cases[i].finishAs}
		var info = orderInfoFromPayload(&p, nil)
		if string(info.Status) != cases[i].wantStatus {
			t.Errorf("finish_as=%q → Status=%q, want %q", cases[i].finishAs, info.Status, cases[i].wantStatus)
		}
	}
	// REST-shaped payload keeps its explicit status (no override).
	var p = spotOrderPayload{CurrencyPair: "BTC_USDT", Status: types.OrderStatusOpen, FinishAs: "open"}
	if string(orderInfoFromPayload(&p, nil).Status) != types.OrderStatusOpen {
		t.Error("explicit status must be preserved")
	}
}
