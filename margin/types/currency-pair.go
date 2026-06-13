/*
FILE: margin/types/currency-pair.go

DESCRIPTION:
MarginCurrencyPair — the SDK's representation of a Gate ISOLATED-margin currency
pair (from GET /margin/currency_pairs[/{currency_pair}]). It describes which
pairs are tradable on isolated margin and the per-pair limits/leverage Gate
enforces.

CALIBRATION: the field set (leverage, min/max amounts, status) follows Gate's
margin docs; verify live.
*/

package types

import "github.com/shopspring/decimal"

// MarginCurrencyPair — normalized isolated-margin currency-pair spec.
type MarginCurrencyPair struct {
	// ID — the Gate pair id, e.g. "BTC_USDT".
	ID string
	// Base — base currency, e.g. "BTC".
	Base string
	// Quote — quote currency, e.g. "USDT".
	Quote string
	// Leverage — maximum leverage available on the pair.
	Leverage decimal.Decimal
	// MinBaseAmount — minimum borrowable/tradable base amount.
	MinBaseAmount decimal.Decimal
	// MinQuoteAmount — minimum borrowable/tradable quote amount.
	MinQuoteAmount decimal.Decimal
	// MaxQuoteAmount — maximum borrowable quote amount.
	MaxQuoteAmount decimal.Decimal
	// Status — Gate pair status (0 disabled / 1 enabled), surfaced as-is.
	Status int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
