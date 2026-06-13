/*
FILE: options/types/position-info.go

DESCRIPTION:
PositionInfo — the SDK's representation of a Gate OPTIONS position. Size is the
absolute number of contracts; Side carries the direction (Gate encodes it as the
sign of the position size). Options positions also carry the per-position greeks
(delta/gamma/vega/theta) Gate computes against the live mark surface. The desk
connector converts contracts → base-asset units via the contract's multiplier.
*/

package types

import "github.com/shopspring/decimal"

// PositionInfo — normalized options position state.
type PositionInfo struct {
	// Contract — Gate options contract, e.g. "BTC_USDT-20240329-50000-C".
	Contract string
	// Side — Buy (long) / Sell (short), derived from the position size sign.
	// Empty for a flat position.
	Side SideType
	// Size — absolute position size in contracts (0 when flat).
	Size decimal.Decimal
	// EntryPrice — average entry price.
	EntryPrice decimal.Decimal
	// MarkPrice — current mark price.
	MarkPrice decimal.Decimal
	// UnrealisedPnl — unrealized PnL.
	UnrealisedPnl decimal.Decimal
	// RealisedPnl — realized PnL.
	RealisedPnl decimal.Decimal
	// MarkIv — mark implied volatility of the contract (may be 0).
	MarkIv decimal.Decimal
	// Delta / Gamma / Vega / Theta — position greeks.
	Delta decimal.Decimal
	Gamma decimal.Decimal
	Vega  decimal.Decimal
	Theta decimal.Decimal
	// PendingOrders — number of open orders on the contract (Gate "pending_orders").
	PendingOrders int64
	// UpdatedAtMs — last update time in epoch milliseconds.
	UpdatedAtMs int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
