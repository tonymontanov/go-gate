/*
FILE: unified/types/risk.go

DESCRIPTION:
Risk/portfolio-margin domain types: the account's RiskUnit breakdown
(GET /unified/risk_units) and the portfolio-margin calculator request/result
(POST /unified/portfolio_calculator). The calculator request is intentionally
flexible (map-valued legs) because Gate accepts a rich, evolving set of
hypothetical spot/futures/options balances, positions and orders.

CALIBRATION: field sets follow Gate's unified-account docs; verify live.
*/

package types

import "github.com/shopspring/decimal"

// RiskUnitEntry — one risk unit (a margin-grouped symbol family) of the account.
type RiskUnitEntry struct {
	// Symbol — the risk-unit symbol (Gate "symbol", e.g. "BTC").
	Symbol string
	// SpotInUse — spot amount in use by the risk unit.
	SpotInUse decimal.Decimal
	// MaintainMargin — maintenance margin for the risk unit.
	MaintainMargin decimal.Decimal
	// InitialMargin — initial margin for the risk unit.
	InitialMargin decimal.Decimal
	// Delta — aggregate delta.
	Delta decimal.Decimal
	// Gamma — aggregate gamma.
	Gamma decimal.Decimal
	// Theta — aggregate theta.
	Theta decimal.Decimal
	// Vega — aggregate vega.
	Vega decimal.Decimal
}

// RiskUnit — the account's risk-unit breakdown.
type RiskUnit struct {
	// UserID — the Gate user id owning the account.
	UserID int64
	// SpotHedge — whether spot hedging is enabled.
	SpotHedge bool
	// Units — the per-risk-unit breakdown.
	Units []RiskUnitEntry
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// PortfolioCalcRequest — body for POST /unified/portfolio_calculator. Each leg
// is a slice of free-form maps forwarded verbatim to Gate, so callers can model
// hypothetical books without the SDK pinning a calibration-pending schema.
type PortfolioCalcRequest struct {
	// SpotBalances — hypothetical spot balances.
	SpotBalances []map[string]any
	// SpotOrders — hypothetical spot orders.
	SpotOrders []map[string]any
	// FuturesPositions — hypothetical futures positions.
	FuturesPositions []map[string]any
	// FuturesOrders — hypothetical futures orders.
	FuturesOrders []map[string]any
	// OptionsPositions — hypothetical options positions.
	OptionsPositions []map[string]any
	// OptionsOrders — hypothetical options orders.
	OptionsOrders []map[string]any
	// SpotHedge — whether to assume spot hedging.
	SpotHedge bool
}

// PortfolioMarginResult — the portfolio-margin calculator result.
type PortfolioMarginResult struct {
	// MaintainMarginTotal — total maintenance margin (USD).
	MaintainMarginTotal decimal.Decimal
	// InitialMarginTotal — total initial margin (USD).
	InitialMarginTotal decimal.Decimal
	// CalculateTimeMs — calculation time in epoch milliseconds.
	CalculateTimeMs int64
	// Units — the per-risk-unit breakdown of the calculation.
	Units []RiskUnitEntry
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
