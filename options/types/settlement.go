/*
FILE: options/types/settlement.go

DESCRIPTION:
Settlement types for the Gate OPTIONS section. Settlement covers the public
per-contract settlement records (GET /options/settlements[/{contract}]) and the
account's own realized settlements (GET /options/my_settlements). Options contracts
expire and settle at their mark/strike outcome; each record captures the settle
profit and the settlement price.

CALIBRATION: the field set follows Gate's options settlements docs; verify the
exact keys and units live.
*/

package types

import "github.com/shopspring/decimal"

// Settlement — one public options settlement record.
type Settlement struct {
	// TimeMs — settlement time in epoch milliseconds.
	TimeMs int64
	// Contract — the options contract that settled.
	Contract string
	// Profit — settlement profit per contract.
	Profit decimal.Decimal
	// Fee — settlement fee.
	Fee decimal.Decimal
	// StrikePrice — the option strike price.
	StrikePrice decimal.Decimal
	// SettlePrice — the price the underlying settled at.
	SettlePrice decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// MySettlement — one of the account's own settlement records (the position that
// was settled, its size, realized PnL and the settlement price).
type MySettlement struct {
	// TimeMs — settlement time in epoch milliseconds.
	TimeMs int64
	// Contract — the options contract that settled.
	Contract string
	// Side — position direction at settlement (derived from the signed size).
	Side SideType
	// Size — absolute position size at settlement (contracts).
	Size decimal.Decimal
	// SettlePrice — the price the underlying settled at.
	SettlePrice decimal.Decimal
	// SettleProfit — realized settlement profit.
	SettleProfit decimal.Decimal
	// Fee — settlement fee.
	Fee decimal.Decimal
	// RealisedPnl — realized PnL booked at settlement.
	RealisedPnl decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// PositionClose — one row of the account's position-close history (from
// GET /options/position_close): the realized PnL of a closed position leg.
type PositionClose struct {
	// TimeMs — close time in epoch milliseconds.
	TimeMs int64
	// Contract — the options contract that was closed.
	Contract string
	// Side — direction of the closed leg (derived from the signed size).
	Side SideType
	// Pnl — realized PnL of the close (Gate "pnl").
	Pnl decimal.Decimal
	// SettleSize — the size that was closed (contracts; Gate "settle_size").
	SettleSize decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
