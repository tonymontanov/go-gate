/*
FILE: earn/types/fixed-term.go

DESCRIPTION:
SDK shapes for the Gate Earn Fixed-Term lending section (base "/earn/fixed-term").
Unlike Uni flexible lending, a fixed-term lend locks principal for a fixed
duration at a fixed (or laddered) annualized rate and settles on a fixed date.

  - FixedTermProduct — a fixed-term offering (GET /earn/fixed-term/product and
    GET /earn/fixed-term/product/{asset}/list).
  - FixedTermTier    — one rung of a product's amount→APR ladder.
  - FixedTermLend    — the caller's subscribed lend (GET/POST /earn/fixed-term/user/lend).
  - FixedTermHistory — one entry of the lend/redeem history
    (GET /earn/fixed-term/user/history).
  - CreateFixedLendRequest — POST /earn/fixed-term/user/lend body.
  - PreRedeemRequest        — POST /earn/fixed-term/user/pre-redeem body.

CALIBRATION: field names follow Gate's Earn Fixed-Term docs; epoch-second
timestamps are normalized to epoch-millisecond ...Ms fields. Verify the exact
field set and the status vocabulary against a live account.
*/

package types

import "github.com/shopspring/decimal"

// FixedTermTier — one rung of a fixed-term product's amount→APR ladder. Gate
// quotes a higher APR for larger principal tiers.
type FixedTermTier struct {
	// MinAmount — lower principal bound at which this tier's APR applies.
	MinAmount decimal.Decimal
	// MaxAmount — upper principal bound for this tier (0 = no cap).
	MaxAmount decimal.Decimal
	// APR — annualized percentage rate for principal in this tier.
	APR decimal.Decimal
}

// FixedTermProduct — a single fixed-term lending offering.
type FixedTermProduct struct {
	// ID — Gate product id (the "pid"/"id" subscribed via CreateLend).
	ID string
	// Asset — the lendable asset code, e.g. "USDT".
	Asset string
	// Type — Gate product category/type (verbatim wire value).
	Type string
	// APR — headline annualized rate (when the product is not laddered).
	APR decimal.Decimal
	// MinAPR / MaxAPR — annualized rate band across the ladder.
	MinAPR decimal.Decimal
	MaxAPR decimal.Decimal
	// MinAmount — minimum principal a single lend may subscribe.
	MinAmount decimal.Decimal
	// MaxAmount — maximum principal a single lend may subscribe (0 = no cap).
	MaxAmount decimal.Decimal
	// DurationDays — lock duration of the product, in days.
	DurationDays int64
	// Tiers — the amount→APR ladder (may be empty for flat-rate products).
	Tiers []FixedTermTier
	// StartTimeMs — subscription window open time in epoch milliseconds.
	StartTimeMs int64
	// EndTimeMs — subscription window close time in epoch milliseconds.
	EndTimeMs int64
	// Status — product status (verbatim wire value).
	Status string
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// FixedTermLend — the caller's subscribed fixed-term lend position.
type FixedTermLend struct {
	// ID — Gate lend id (used as the pre-redeem target).
	ID string
	// ProductID — the underlying product id (the "pid").
	ProductID string
	// Asset — the lent asset code, e.g. "USDT".
	Asset string
	// Amount — principal subscribed.
	Amount decimal.Decimal
	// APR — annualized rate locked for this lend.
	APR decimal.Decimal
	// CreatedAtMs — subscription time in epoch milliseconds.
	CreatedAtMs int64
	// SettleTimeMs — interest-settlement time in epoch milliseconds.
	SettleTimeMs int64
	// RedeemTimeMs — principal-redeem (maturity) time in epoch milliseconds.
	RedeemTimeMs int64
	// Status — lend status (verbatim wire value).
	Status string
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// FixedTermHistory — one entry of the caller's fixed-term lend/redeem history.
type FixedTermHistory struct {
	// ID — Gate history-record id.
	ID string
	// ProductID — the underlying product id (the "pid").
	ProductID string
	// Asset — the asset code, e.g. "USDT".
	Asset string
	// Type — record type (subscribe / redeem / interest, verbatim wire value).
	Type string
	// Amount — principal or interest amount carried by this record.
	Amount decimal.Decimal
	// APR — annualized rate associated with the record.
	APR decimal.Decimal
	// CreatedAtMs — record time in epoch milliseconds.
	CreatedAtMs int64
	// Status — record status (verbatim wire value).
	Status string
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// CreateFixedLendRequest — parameters for POST /earn/fixed-term/user/lend.
type CreateFixedLendRequest struct {
	// ProductID — the fixed-term product to subscribe (sent as "pid"). Required.
	ProductID string
	// Amount — principal to lend (positive magnitude). Required.
	Amount decimal.Decimal
}

// PreRedeemRequest — parameters for POST /earn/fixed-term/user/pre-redeem. It
// queues an early redemption of an existing fixed-term lend.
type PreRedeemRequest struct {
	// ID — the fixed-term lend id to pre-redeem. Required.
	ID string
}
