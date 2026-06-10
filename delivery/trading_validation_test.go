/*
FILE: delivery/trading_validation_test.go

DESCRIPTION:
Pure request-build validation tests (no HTTP). They assert the SDK rejects
malformed requests BEFORE sending, with ErrorKindInvalidRequest, covering the
Gate-specific encoding rules: side required, whole-contract size, market tif,
client-id "t-" format, and amend identifier/field rules.
*/

package delivery

import (
	"testing"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/delivery/types"
)

func buildClient() *Client {
	var parent *gate.Client
	parent, _ = gate.NewClient(gate.Config{Settle: "usdt"})
	return NewClient(parent)
}

func TestBuildCreate_MissingSide(t *testing.T) {
	var tc *TradingClient = buildClient().Trading()
	var _, err = tc.buildCreateOrderBody(types.CreateOrderRequest{
		Contract: "BTC_USDT", Size: mustDec("1"), Price: mustDec("100"),
	})
	if err == nil || !gate.IsInvalidRequest(err) {
		t.Fatalf("expected InvalidRequest for missing side, got %v", err)
	}
}

func TestBuildCreate_NonIntegerSize(t *testing.T) {
	var tc *TradingClient = buildClient().Trading()
	var _, err = tc.buildCreateOrderBody(types.CreateOrderRequest{
		Contract: "BTC_USDT", Side: types.SideTypeBuy, Size: mustDec("1.5"), Price: mustDec("100"),
	})
	if err == nil || !gate.IsInvalidRequest(err) {
		t.Fatalf("expected InvalidRequest for fractional size, got %v", err)
	}
}

func TestBuildCreate_LimitRequiresPrice(t *testing.T) {
	var tc *TradingClient = buildClient().Trading()
	var _, err = tc.buildCreateOrderBody(types.CreateOrderRequest{
		Contract: "BTC_USDT", Side: types.SideTypeBuy, Size: mustDec("1"), OrderType: types.OrderTypeLimit,
	})
	if err == nil || !gate.IsInvalidRequest(err) {
		t.Fatalf("expected InvalidRequest for limit without price, got %v", err)
	}
}

func TestBuildCreate_MarketBadTIF(t *testing.T) {
	var tc *TradingClient = buildClient().Trading()
	var _, err = tc.buildCreateOrderBody(types.CreateOrderRequest{
		Contract: "BTC_USDT", Side: types.SideTypeBuy, Size: mustDec("1"),
		OrderType: types.OrderTypeMarket, TimeInForce: types.TimeInForceGTC,
	})
	if err == nil || !gate.IsInvalidRequest(err) {
		t.Fatalf("expected InvalidRequest for market+gtc, got %v", err)
	}
}

func TestBuildCreate_BadText(t *testing.T) {
	var tc *TradingClient = buildClient().Trading()
	var _, err = tc.buildCreateOrderBody(types.CreateOrderRequest{
		Contract: "BTC_USDT", Side: types.SideTypeBuy, Size: mustDec("1"), Price: mustDec("100"),
		Text: "bad id with spaces",
	})
	if err == nil || !gate.IsInvalidRequest(err) {
		t.Fatalf("expected InvalidRequest for malformed text, got %v", err)
	}
}

func TestBuildCreate_CloseZeroSize(t *testing.T) {
	var tc *TradingClient = buildClient().Trading()
	// Close orders need no Side/Size; size must be sent as 0.
	var body, err = tc.buildCreateOrderBody(types.CreateOrderRequest{
		Contract: "BTC_USDT", Close: true, Price: mustDec("100"),
	})
	if err != nil {
		t.Fatalf("close order should build, got %v", err)
	}
	if body["size"].(int64) != 0 || body["close"] != true {
		t.Fatalf("close body wrong: %+v", body)
	}
}

func TestNormalizeClientID(t *testing.T) {
	var got string
	var err error

	got, err = normalizeClientID("abc")
	if err != nil || got != "t-abc" {
		t.Fatalf("plain id: got %q err %v", got, err)
	}
	got, err = normalizeClientID("t-abc")
	if err != nil || got != "t-abc" {
		t.Fatalf("prefixed id: got %q err %v", got, err)
	}
	got, err = normalizeClientID("")
	if err != nil || got != "" {
		t.Fatalf("empty id: got %q err %v", got, err)
	}
	_, err = normalizeClientID("has space")
	if err == nil {
		t.Fatalf("expected error for invalid charset")
	}
}

func TestBuildAmend_RequiresOneIdentifier(t *testing.T) {
	var _, err = buildAmendBody(types.ModifyOrderRequest{NewPrice: mustDec("100")})
	if err == nil || !gate.IsInvalidRequest(err) {
		t.Fatalf("expected InvalidRequest with no identifier, got %v", err)
	}
	_, err = buildAmendBody(types.ModifyOrderRequest{OrderID: "1", ClientOrderID: "t-a", NewPrice: mustDec("100")})
	if err == nil || !gate.IsInvalidRequest(err) {
		t.Fatalf("expected InvalidRequest with both identifiers, got %v", err)
	}
}

func TestBuildAmend_SizeNeedsSide(t *testing.T) {
	var _, err = buildAmendBody(types.ModifyOrderRequest{OrderID: "1", NewSize: mustDec("5")})
	if err == nil || !gate.IsInvalidRequest(err) {
		t.Fatalf("expected InvalidRequest for size amend without side, got %v", err)
	}
}
