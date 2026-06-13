/*
FILE: options/types/mmp.go

DESCRIPTION:
Market-Maker Protection (MMP) types for the Gate OPTIONS section. MMP is a
risk-control switch a market maker arms PER UNDERLYING: if the cumulative filled
quantity (or net delta) over a rolling window breaches a configured limit, Gate
freezes new orders on that underlying for a cooldown period. MMPSettings is the
configuration sent to POST /options/mmp; MMPInfo is the state returned by
GET /options/mmp and the set/reset calls.

CALIBRATION: the field set (window, frozen_period, qty_limit, delta_limit,
mmp_frozen_until) follows Gate's options MMP docs; verify live.
*/

package types

import "github.com/shopspring/decimal"

// MMPSettings — Market-Maker Protection configuration for one underlying.
type MMPSettings struct {
	// Underlying — the underlying index the protection applies to, e.g. "BTC_USDT".
	// Required.
	Underlying string
	// Window — rolling time window in milliseconds over which fills accumulate
	// (Gate "window"). 0 disables MMP.
	Window int64
	// FrozenPeriod — freeze cooldown in milliseconds after a trigger (Gate
	// "frozen_period"). 0 freezes until manually reset.
	FrozenPeriod int64
	// QtyLimit — cumulative filled-quantity limit that triggers the freeze
	// (Gate "qty_limit").
	QtyLimit decimal.Decimal
	// DeltaLimit — cumulative net-delta limit that triggers the freeze
	// (Gate "delta_limit").
	DeltaLimit decimal.Decimal
}

// MMPInfo — the current Market-Maker Protection state for one underlying.
type MMPInfo struct {
	// Underlying — the underlying index the protection applies to.
	Underlying string
	// Window — configured rolling window in milliseconds.
	Window int64
	// FrozenPeriod — configured freeze cooldown in milliseconds.
	FrozenPeriod int64
	// QtyLimit — configured cumulative filled-quantity limit.
	QtyLimit decimal.Decimal
	// DeltaLimit — configured cumulative net-delta limit.
	DeltaLimit decimal.Decimal
	// MmpFrozenUntilMs — epoch milliseconds the underlying is frozen until
	// (0 when not frozen; Gate "mmp_frozen_until").
	MmpFrozenUntilMs int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
