/*
FILE: margin/types/transferable.go

DESCRIPTION:
Transferable / Borrowable — small response structs shared by the isolated and
cross margin "how much can I ...?" endpoints:

  - Transferable — the maximum amount transferable out of a margin account
    (GET /margin/transferable, GET /margin/cross/transferable).
  - Borrowable   — the maximum amount borrowable into a margin account
    (GET /margin/borrowable, GET /margin/cross/borrowable).

For cross margin the CurrencyPair field is empty (cross is not pair-scoped).
*/

package types

import "github.com/shopspring/decimal"

// Transferable — the maximum amount transferable out of a margin account.
type Transferable struct {
	// Currency — the currency queried, e.g. "USDT".
	Currency string
	// CurrencyPair — the isolated pair queried (empty for cross).
	CurrencyPair string
	// Amount — the maximum transferable amount.
	Amount decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// Borrowable — the maximum amount borrowable into a margin account.
type Borrowable struct {
	// Currency — the currency queried, e.g. "USDT".
	Currency string
	// CurrencyPair — the isolated pair queried (empty for cross).
	CurrencyPair string
	// Amount — the maximum borrowable amount.
	Amount decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
