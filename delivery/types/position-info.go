/*
FILE: delivery/types/position-info.go

DESCRIPTION:
PositionInfo — the SDK's representation of a Gate futures position. Size is the
absolute number of contracts; Side carries the direction (Gate encodes it as the
sign of the position size). The desk connector converts contracts → base-asset
units via the contract's quanto_multiplier.
*/

package types

import "github.com/shopspring/decimal"

// PositionInfo — normalized futures position state.
type PositionInfo struct {
	// Contract — Gate contract, e.g. "BTC_USDT".
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
	// LiqPrice — estimated liquidation price.
	LiqPrice decimal.Decimal
	// Leverage — position leverage; 0 means cross margin (see CrossLeverageLimit).
	Leverage decimal.Decimal
	// CrossLeverageLimit — cross-margin leverage when Leverage is 0.
	CrossLeverageLimit decimal.Decimal
	// Margin — position margin in settlement currency.
	Margin decimal.Decimal
	// Value — position value in settlement currency.
	Value decimal.Decimal
	// UnrealisedPnl — unrealized PnL.
	UnrealisedPnl decimal.Decimal
	// RealisedPnl — realized PnL.
	RealisedPnl decimal.Decimal
	// MaintenanceRate — maintenance margin rate under the current risk limit.
	MaintenanceRate decimal.Decimal
	// Mode — Gate position mode: "single", "dual_long", or "dual_short".
	Mode string
	// UpdatedAtMs — last update time in epoch milliseconds.
	UpdatedAtMs int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
