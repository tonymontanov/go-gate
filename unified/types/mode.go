/*
FILE: unified/types/mode.go

DESCRIPTION:
Account-mode domain types: the current UnifiedMode (GET /unified/unified_mode)
and the SetModeRequest body (PUT /unified/unified_mode). The AccountMode enum
carries the four Gate margin modes.

CALIBRATION: mode values and the settings flag set follow Gate's unified-account
docs; verify live.
*/

package types

// AccountMode — the unified account's margin mode.
type AccountMode string

const (
	// AccountModeClassic — classic (isolated, non-unified) mode.
	AccountModeClassic AccountMode = "classic"
	// AccountModeMultiCurrency — multi-currency cross-margin mode.
	AccountModeMultiCurrency AccountMode = "multi_currency"
	// AccountModePortfolio — portfolio-margin mode.
	AccountModePortfolio AccountMode = "portfolio"
	// AccountModeSingleCurrency — single-currency margin mode.
	AccountModeSingleCurrency AccountMode = "single_currency"
)

// UnifiedMode — the account's current margin mode plus its boolean feature
// settings (Gate "settings", e.g. "usdt_futures", "spot_hedge", "options",
// "use_funding").
type UnifiedMode struct {
	// Mode — the active account mode.
	Mode AccountMode
	// Settings — feature toggles keyed by Gate setting name.
	Settings map[string]bool
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// SetModeRequest — body for PUT /unified/unified_mode.
type SetModeRequest struct {
	// Mode — the desired account mode (required).
	Mode AccountMode
	// Settings — feature toggles keyed by Gate setting name (optional).
	Settings map[string]bool
}
