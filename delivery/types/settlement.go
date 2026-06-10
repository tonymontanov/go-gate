/*
FILE: delivery/types/settlement.go

DESCRIPTION:
Settlement — the SDK's representation of a Gate DELIVERY settlement record (from
GET /delivery/{settle}/settlements). Delivery contracts settle at expiry; each
settlement record captures the realized outcome for a position at that moment.
This type has no perpetual-futures analogue (perpetuals never settle).

CALIBRATION: the field set follows Gate's delivery settlements docs; verify the
exact keys and units live.
*/

package types

import "github.com/shopspring/decimal"

// Settlement — one delivery settlement record.
type Settlement struct {
	// TimeMs — settlement time in epoch milliseconds.
	TimeMs int64
	// Contract — the (dated) delivery contract that settled, e.g. "BTC_USDT_20240329".
	Contract string
	// Size — absolute position size at settlement (contracts).
	Size decimal.Decimal
	// Side — position direction at settlement (derived from the signed wire size).
	Side SideType
	// Leverage — position leverage at settlement.
	Leverage decimal.Decimal
	// Margin — position margin at settlement.
	Margin decimal.Decimal
	// Profit — settlement profit.
	Profit decimal.Decimal
	// Pnl — realized PnL at settlement.
	Pnl decimal.Decimal
	// Fee — settlement fee.
	Fee decimal.Decimal
	// SettlePrice — the price the contract settled at.
	SettlePrice decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
