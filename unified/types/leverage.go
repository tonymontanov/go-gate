/*
FILE: unified/types/leverage.go

DESCRIPTION:
Per-currency leverage domain types: the allowed leverage range
(GET /unified/leverage/user_currency_config), the current setting
(GET /unified/leverage/user_currency_setting) and the SetLeverageRequest body
(POST /unified/leverage/user_currency_setting).

CALIBRATION: field sets follow Gate's unified-account docs; verify live.
*/

package types

import "github.com/shopspring/decimal"

// LeverageConfig — the allowed leverage range for a currency.
type LeverageConfig struct {
	// Currency — the currency, e.g. "BTC".
	Currency string
	// MinLeverage — minimum settable leverage.
	MinLeverage decimal.Decimal
	// MaxLeverage — maximum settable leverage.
	MaxLeverage decimal.Decimal
	// CurrentLeverage — the currently applied leverage, when reported.
	CurrentLeverage decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// LeverageSetting — the currently configured leverage for a currency.
type LeverageSetting struct {
	// Currency — the currency, e.g. "BTC".
	Currency string
	// Leverage — the configured leverage.
	Leverage decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// SetLeverageRequest — body for POST /unified/leverage/user_currency_setting.
type SetLeverageRequest struct {
	// Currency — the currency to configure (required).
	Currency string
	// Leverage — the desired leverage (required).
	Leverage decimal.Decimal
}
