/*
FILE: earn/types/market.go

DESCRIPTION:
Public market-data shapes for the Gate Earn "Uni" lending section:
  - ChartPoint — one point of the historical rate chart (GET /earn/uni/chart).
  - RatePoint  — the current estimated annualized rate (GET /earn/uni/rate).

These endpoints are public (no signature). The chart timestamp Gate sends in
epoch seconds is normalized to an epoch-millisecond TimeMs field.

CALIBRATION: field names follow Gate's Uni docs; verify live.
*/

package types

import "github.com/shopspring/decimal"

// ChartPoint — one sample of the Uni historical lending-rate chart.
type ChartPoint struct {
	// TimeMs — sample time in epoch milliseconds.
	TimeMs int64
	// Value — the sampled annualized lending rate at TimeMs.
	Value decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// RatePoint — the current estimated annualized lending rate for a currency.
type RatePoint struct {
	// Currency — the asset code, e.g. "ETH".
	Currency string
	// EstimateRate — the estimated annualized rate the pool is quoting now.
	EstimateRate decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
