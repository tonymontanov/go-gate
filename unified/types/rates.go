/*
FILE: unified/types/rates.go

DESCRIPTION:
Interest-rate domain types for the unified account: the per-currency estimated
next rate (GET /unified/estimate_rate) and the historical loan-rate series
(GET /unified/history_loan_rate).

CALIBRATION: field sets follow Gate's unified-account docs; verify live.
*/

package types

import "github.com/shopspring/decimal"

// EstimateRate — estimated next-period borrow rates keyed by currency.
type EstimateRate struct {
	// Rates — currency → estimated rate (e.g. "BTC" → 0.0001).
	Rates map[string]decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// RatePoint — one (time, rate) sample of a historical loan-rate series.
type RatePoint struct {
	// TimeMs — sample time in epoch milliseconds.
	TimeMs int64
	// Rate — the loan rate at that time.
	Rate decimal.Decimal
}

// HistoryLoanRate — the historical loan-rate series for a currency/tier.
type HistoryLoanRate struct {
	// Currency — the currency, e.g. "BTC".
	Currency string
	// Tier — the VIP/loan tier the series is for.
	Tier string
	// Rate — the current rate, when reported alongside the series.
	Rate decimal.Decimal
	// Rates — the historical (time, rate) samples, oldest-first.
	Rates []RatePoint
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
