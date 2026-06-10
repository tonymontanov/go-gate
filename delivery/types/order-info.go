/*
FILE: delivery/types/order-info.go

DESCRIPTION:
OrderInfo — the SDK's stable representation of a Gate futures order, returned by
the trading and account layers. Sizes are in CONTRACTS as positive magnitudes;
Side carries the direction the desk connector converts back to base-asset units.
*/

package types

import "github.com/shopspring/decimal"

// OrderInfo — normalized futures order state.
type OrderInfo struct {
	// OrderID — Gate numeric order id as a string. Empty if the order was not
	// accepted (e.g. a rejected batch element).
	OrderID string
	// ClientOrderID — the order's "text" id as Gate stored it (with "t-" prefix).
	ClientOrderID string
	// Contract — Gate contract, e.g. "BTC_USDT".
	Contract string
	// Side — derived from the sign of the Gate order size.
	Side SideType
	// OrderType — Limit or Market, inferred from price.
	OrderType OrderType
	// Price — order price.
	Price decimal.Decimal
	// Size — absolute order size in contracts.
	Size decimal.Decimal
	// Left — unfilled size in contracts (absolute).
	Left decimal.Decimal
	// FillPrice — average fill price (0 if unfilled).
	FillPrice decimal.Decimal
	// TimeInForce — gtc/ioc/poc/fok.
	TimeInForce TimeInForceType
	// Status — Gate order status: "open" or "finished".
	Status string
	// FinishAs — terminal reason when finished (filled/cancelled/liquidated/ioc/...).
	FinishAs string
	// ReduceOnly — whether the order is reduce-only.
	ReduceOnly bool
	// Close — whether the order is a close-position order.
	Close bool
	// CreatedAtMs — creation time in epoch milliseconds.
	CreatedAtMs int64
	// FinishedAtMs — finish time in epoch milliseconds (0 if still open).
	FinishedAtMs int64
	// RateLimits — Gate rate-limit headers from the response that produced this
	// OrderInfo (may be empty).
	RateLimits map[string]string
}
