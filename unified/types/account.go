/*
FILE: unified/types/account.go

DESCRIPTION:
UnifiedAccount — the SDK's representation of the Gate UNIFIED account snapshot
(from GET /unified/accounts). The unified account is a single cross-currency /
portfolio-margin book: it carries account-wide USD-denominated totals plus a
per-currency Balance map. All decimal fields are normalized to decimal.Decimal;
the wire layer decodes them from either number or string (Gate varies) via
codec.FlexDecimal.

CALIBRATION: the field set (account-wide totals + per-currency balance fields)
follows Gate's unified-account docs; verify live.
*/

package types

import "github.com/shopspring/decimal"

// UnifiedAccount — normalized unified-account state.
type UnifiedAccount struct {
	// UserID — the Gate user id owning the account.
	UserID int64
	// RefreshTimeMs — server snapshot time in epoch milliseconds.
	RefreshTimeMs int64
	// Locked — whether the account is locked (e.g. during liquidation).
	Locked bool
	// Total — total account value in USD.
	Total decimal.Decimal
	// Borrowed — total borrowed value in USD.
	Borrowed decimal.Decimal
	// Equity — total account equity in USD.
	Equity decimal.Decimal
	// TotalInitialMargin — total initial margin in USD.
	TotalInitialMargin decimal.Decimal
	// TotalMarginBalance — total margin balance in USD.
	TotalMarginBalance decimal.Decimal
	// TotalMaintenanceMargin — total maintenance margin in USD.
	TotalMaintenanceMargin decimal.Decimal
	// TotalInitialMarginRate — total initial margin rate.
	TotalInitialMarginRate decimal.Decimal
	// TotalMaintenanceMarginRate — total maintenance margin rate.
	TotalMaintenanceMarginRate decimal.Decimal
	// TotalAvailableMargin — total available margin in USD.
	TotalAvailableMargin decimal.Decimal
	// UnifiedAccountTotal — unified account total (USD).
	UnifiedAccountTotal decimal.Decimal
	// UnifiedAccountTotalLiab — unified account total liabilities (USD).
	UnifiedAccountTotalLiab decimal.Decimal
	// UnifiedAccountTotalEquity — unified account total equity (USD).
	UnifiedAccountTotalEquity decimal.Decimal
	// Leverage — account-wide leverage, when present.
	Leverage decimal.Decimal
	// SpotOrderLoss — spot order loss in USD, when present.
	SpotOrderLoss decimal.Decimal
	// Balances — per-currency balances keyed by currency (e.g. "BTC", "USDT").
	Balances map[string]Balance
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// Balance — one currency's slice of the unified account.
type Balance struct {
	// Available — balance available for use.
	Available decimal.Decimal
	// Freeze — frozen balance (open orders etc.).
	Freeze decimal.Decimal
	// Borrowed — borrowed amount in this currency.
	Borrowed decimal.Decimal
	// NegativeLiab — negative-balance liability.
	NegativeLiab decimal.Decimal
	// FuturesPosLiab — futures-position liability.
	FuturesPosLiab decimal.Decimal
	// Equity — equity in this currency.
	Equity decimal.Decimal
	// TotalFreeze — total frozen across the account for this currency.
	TotalFreeze decimal.Decimal
	// TotalLiab — total liability for this currency.
	TotalLiab decimal.Decimal
	// SpotInUse — amount in use by spot positions.
	SpotInUse decimal.Decimal
	// Leverage — per-currency leverage, when present.
	Leverage decimal.Decimal
	// FreezeFundingFee — funding fee freeze, when present.
	FreezeFundingFee decimal.Decimal
}
