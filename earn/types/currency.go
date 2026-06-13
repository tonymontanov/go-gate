/*
FILE: earn/types/currency.go

DESCRIPTION:
UniCurrency — the SDK's representation of a single lendable currency in the Gate
Earn "Uni" flexible-lending pool (from GET /earn/uni/currencies and
GET /earn/uni/currencies/{currency}). It pins the per-currency lend amount bounds
and the estimated rate band the pool is currently quoting.

CALIBRATION: the field set (currency, min/max lend amount, available,
total_lend_available, min/max rate) follows Gate's Uni docs; verify live.
*/

package types

import "github.com/shopspring/decimal"

// UniCurrency — a lendable currency and its current Uni-pool parameters.
type UniCurrency struct {
	// Currency — the asset code, e.g. "ETH".
	Currency string
	// MinLendAmount — minimum principal a single lend may add.
	MinLendAmount decimal.Decimal
	// MaxLendAmount — maximum principal a single lend may add (0 = no cap).
	MaxLendAmount decimal.Decimal
	// Available — amount the caller currently has available to lend (Gate
	// "available"); may be zero on the public listing.
	Available decimal.Decimal
	// TotalLendAvailable — total principal the pool can still accept.
	TotalLendAvailable decimal.Decimal
	// MinRate — lower bound of the estimated annualized rate band.
	MinRate decimal.Decimal
	// MaxRate — upper bound of the estimated annualized rate band.
	MaxRate decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
