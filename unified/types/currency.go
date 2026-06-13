/*
FILE: unified/types/currency.go

DESCRIPTION:
UnifiedCurrency — one entry of the public unified borrowing-currency list
(GET /unified/currencies): the borrow limits, precision and loan status of a
currency that may be borrowed in the unified account.

CALIBRATION: field set follows Gate's unified-account docs; verify live.
*/

package types

import "github.com/shopspring/decimal"

// UnifiedCurrency — a borrowable currency in the unified account.
type UnifiedCurrency struct {
	// Name — the currency, e.g. "BTC".
	Name string
	// PrecMode — Gate precision/loan mode flag, when present.
	PrecMode int64
	// MinBorrowAmount — minimum single-borrow amount.
	MinBorrowAmount decimal.Decimal
	// UserMaxBorrowAmount — per-user maximum borrowable amount.
	UserMaxBorrowAmount decimal.Decimal
	// TotalMaxBorrowAmount — platform-wide maximum borrowable amount.
	TotalMaxBorrowAmount decimal.Decimal
	// PriceChange30d — 30-day price change ratio, when present.
	PriceChange30d decimal.Decimal
	// Discount — the collateral discount factor for this currency.
	Discount decimal.Decimal
	// LoanStatus — whether borrowing is enabled (Gate "loan_status").
	LoanStatus string
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
