/*
FILE: unified/types/tiers.go

DESCRIPTION:
Tier domain types: the public per-currency collateral discount tiers
(GET /unified/currency_discount_tiers) and the loan-margin tiers
(GET /unified/loan_margin_tiers).

CALIBRATION: field sets follow Gate's unified-account docs; verify live.
*/

package types

import "github.com/shopspring/decimal"

// DiscountTierEntry — one collateral-discount tier band.
type DiscountTierEntry struct {
	// Tier — the tier identifier/index.
	Tier string
	// Discount — the collateral discount factor for this band.
	Discount decimal.Decimal
	// LowerLimit — lower amount bound of the band.
	LowerLimit decimal.Decimal
	// UpperLimit — upper amount bound of the band.
	UpperLimit decimal.Decimal
}

// DiscountTier — the collateral discount tiers for a currency.
type DiscountTier struct {
	// Currency — the currency, e.g. "BTC".
	Currency string
	// Tiers — the discount tier bands, ascending.
	Tiers []DiscountTierEntry
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// LoanMarginTierEntry — one loan-margin tier band.
type LoanMarginTierEntry struct {
	// Tier — the tier identifier/index.
	Tier string
	// MarginRate — the margin rate for this band.
	MarginRate decimal.Decimal
	// LowerLimit — lower amount bound of the band.
	LowerLimit decimal.Decimal
	// UpperLimit — upper amount bound of the band.
	UpperLimit decimal.Decimal
}

// LoanMarginTier — the loan-margin tiers for a currency.
type LoanMarginTier struct {
	// Currency — the currency, e.g. "BTC".
	Currency string
	// Tiers — the loan-margin tier bands, ascending.
	Tiers []LoanMarginTierEntry
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
