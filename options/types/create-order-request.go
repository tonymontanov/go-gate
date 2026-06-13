/*
FILE: options/types/create-order-request.go

DESCRIPTION:
CreateOrderRequest — input for TradingClient.CreateOrder.

UNITS & CONVENTIONS:
  - Size is in CONTRACTS (Gate-native), a positive magnitude. Direction comes
    from Side; the trading layer encodes the Gate signed-integer size. The desk
    connector converts base-asset quantity → contracts (multiplier) before
    populating Size.
  - Price is the limit price. Leave zero for a market order (OrderType=Market or
    omitted with a zero price).
  - Text is the client order id. The SDK auto-prefixes "t-" (Gate requirement)
    and validates the remainder.

NOTE: Gate options has NO batch order endpoints — orders are placed one at a time.
*/

package types

import "github.com/shopspring/decimal"

// CreateOrderRequest — parameters for creating an options order.
type CreateOrderRequest struct {
	// Contract — Gate options contract, e.g. "BTC_USDT-20240329-50000-C". Required.
	Contract string
	// Side — Buy or Sell. Required unless Close is set. Encoded into the size sign.
	Side SideType
	// Size — order size in contracts (positive magnitude). Required unless Close.
	// Must be a whole number of contracts.
	Size decimal.Decimal
	// Price — limit price. Required for limit orders; ignored for market orders.
	Price decimal.Decimal
	// OrderType — Limit or Market. Optional: when empty it is inferred from Price
	// (zero → Market) and TimeInForce.
	OrderType OrderType
	// TimeInForce — gtc/ioc/poc/fok. Optional: defaults to gtc for limit orders
	// and ioc for market orders. Use poc for post-only.
	TimeInForce TimeInForceType
	// Text — client order id. Optional. The SDK sends it as Gate's "text" field,
	// auto-prefixing "t-" and validating ≤28 trailing chars of [0-9A-Za-z._-].
	Text string
	// ReduceOnly — set true for a reduce-only order.
	ReduceOnly bool
	// Close — set true to close the position; Gate requires size=0 in this case,
	// so Size is ignored.
	Close bool
	// Mmp — set true to flag the order as eligible for Market-Maker Protection.
	Mmp bool
	// StpAct — self-trading-prevention action: "cn" (cancel newest),
	// "co" (cancel oldest), "cb" (cancel both). Optional.
	StpAct string
}
