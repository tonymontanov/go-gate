/*
FILE: unified/types/borrowable.go

DESCRIPTION:
Borrowable and Transferable — the per-currency quota results returned by the
unified borrow/transfer quota endpoints (GET /unified/borrowable,
/unified/batch_borrowable, /unified/transferable, /unified/transferables).

CALIBRATION: field set (currency + amount) follows Gate's unified-account docs;
verify live.
*/

package types

import "github.com/shopspring/decimal"

// Borrowable — the maximum amount of a currency the account may borrow.
type Borrowable struct {
	// Currency — the currency, e.g. "BTC".
	Currency string
	// Amount — the maximum borrowable amount.
	Amount decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// Transferable — the maximum amount of a currency the account may transfer out.
type Transferable struct {
	// Currency — the currency, e.g. "BTC".
	Currency string
	// Amount — the maximum transferable amount.
	Amount decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
