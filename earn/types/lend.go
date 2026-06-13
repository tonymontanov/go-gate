/*
FILE: earn/types/lend.go

DESCRIPTION:
UniLend — the SDK's representation of the caller's CURRENT flexible-lending
position in a single currency (from GET /earn/uni/lends). It is the live state of
the principal: how much is lent, how much is frozen pending settlement, the
floor rate the caller is willing to accept, and the auto-reinvest status.

CALIBRATION: the field set (currency, current_amount, min_rate, left_amount,
frozen_amount, interest_status, reinvest_left_amount, create/update times)
follows Gate's Uni docs; verify live.
*/

package types

import "github.com/shopspring/decimal"

// UniLend — the caller's current Uni lending position for one currency.
type UniLend struct {
	// Currency — the lent asset code, e.g. "ETH".
	Currency string
	// CurrentAmount — total principal currently lent.
	CurrentAmount decimal.Decimal
	// MinRate — the floor annualized rate the caller will accept; lends below
	// this rate are not matched.
	MinRate decimal.Decimal
	// LeftAmount — principal still actively earning (not yet redeemed).
	LeftAmount decimal.Decimal
	// FrozenAmount — principal frozen pending a redeem/settlement.
	FrozenAmount decimal.Decimal
	// InterestStatus — auto-reinvest / compounding status (verbatim wire value).
	InterestStatus InterestStatus
	// ReinvestLeftAmount — principal queued for auto-reinvest.
	ReinvestLeftAmount decimal.Decimal
	// CreatedAtMs — position creation time in epoch milliseconds.
	CreatedAtMs int64
	// UpdatedAtMs — last update time in epoch milliseconds.
	UpdatedAtMs int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
