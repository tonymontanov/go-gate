/*
FILE: wallet/types/balance.go

DESCRIPTION:
Balance-side structs for the Gate WALLET section:

  - BalanceSnapshot           — a (currency, amount) pair Gate uses for the
    aggregate total-balance figures.
  - TotalBalance              — the account-wide estimated total plus a per-location
    breakdown (GET /wallet/total_balance).
  - SubAccountBalance         — a sub-account's spot balances by currency
    (GET /wallet/sub_account_balances).
  - SubAccountMarginBalance   — a sub-account's isolated-margin balances
    (GET /wallet/sub_account_margin_balances).
  - SubAccountFuturesBalance  — a sub-account's futures balances by settle currency
    (GET /wallet/sub_account_futures_balances).
  - SubAccountCrossMarginBalance — a sub-account's cross-margin account
    (GET /wallet/sub_account_cross_margin_balances).

Monetary fields use decimal.Decimal. The wire payloads (decoded in the wallet
package) use codec.FlexDecimal because Gate may quote an amount as a string or
send it as a bare JSON number.
*/

package types

import "github.com/shopspring/decimal"

// BalanceSnapshot — a currency/amount pair (a single converted figure).
type BalanceSnapshot struct {
	// Currency — the unit currency of the amount, e.g. "USDT".
	Currency string
	// Amount — the converted amount in Currency.
	Amount decimal.Decimal
}

// TotalBalance — the account-wide estimated total balance with a per-location
// breakdown (GET /wallet/total_balance).
type TotalBalance struct {
	// Total — the aggregate estimated total across all locations.
	Total BalanceSnapshot
	// Details — per-location estimated totals, keyed by Gate location name
	// (e.g. "spot", "margin", "futures", "delivery", "cross_margin", "finance").
	Details map[string]BalanceSnapshot
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// SubAccountBalance — a sub-account's spot balances, keyed by currency
// (GET /wallet/sub_account_balances).
type SubAccountBalance struct {
	// UID — the sub-account user id.
	UID string
	// Available — available spot balance per currency (e.g. "USDT" → 100).
	Available map[string]decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// MarginPairBalanceLeg — one leg (base or quote) of a sub-account isolated-margin
// pair balance.
type MarginPairBalanceLeg struct {
	// Currency — the leg currency.
	Currency string
	// Available — balance available for trading/withdrawal.
	Available decimal.Decimal
	// Locked — balance locked by open orders.
	Locked decimal.Decimal
	// Borrowed — outstanding borrowed principal on this leg.
	Borrowed decimal.Decimal
	// Interest — accrued, unpaid interest on the borrowed principal.
	Interest decimal.Decimal
}

// MarginPairBalance — a sub-account's isolated-margin balance for one pair.
type MarginPairBalance struct {
	// CurrencyPair — the Gate pair, e.g. "BTC_USDT".
	CurrencyPair string
	// Locked — whether the account is locked.
	Locked bool
	// Risk — the account risk ratio Gate reports for the pair.
	Risk decimal.Decimal
	// Base — the base-currency leg balance breakdown.
	Base MarginPairBalanceLeg
	// Quote — the quote-currency leg balance breakdown.
	Quote MarginPairBalanceLeg
}

// SubAccountMarginBalance — a sub-account's isolated-margin balances
// (GET /wallet/sub_account_margin_balances).
type SubAccountMarginBalance struct {
	// UID — the sub-account user id.
	UID string
	// Available — the per-pair isolated-margin balances.
	Available []MarginPairBalance
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// FuturesBalance — a sub-account's futures account figures for one settle
// currency.
type FuturesBalance struct {
	// Currency — the settle currency, e.g. "USDT".
	Currency string
	// Total — total account equity.
	Total decimal.Decimal
	// Available — balance available for new positions/orders.
	Available decimal.Decimal
	// UnrealisedPnl — unrealized profit and loss of open positions.
	UnrealisedPnl decimal.Decimal
	// PositionMargin — margin currently locked by open positions.
	PositionMargin decimal.Decimal
	// OrderMargin — margin currently locked by open orders.
	OrderMargin decimal.Decimal
}

// SubAccountFuturesBalance — a sub-account's futures balances keyed by settle
// currency (GET /wallet/sub_account_futures_balances).
type SubAccountFuturesBalance struct {
	// UID — the sub-account user id.
	UID string
	// Available — futures balances per settle currency (e.g. "USDT" → figures).
	Available map[string]FuturesBalance
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// CrossMarginCurrencyBalance — a sub-account cross-margin balance for one
// currency.
type CrossMarginCurrencyBalance struct {
	// Currency — the currency.
	Currency string
	// Available — balance available for trading/withdrawal.
	Available decimal.Decimal
	// Freeze — balance locked by open orders.
	Freeze decimal.Decimal
	// Borrowed — outstanding borrowed principal.
	Borrowed decimal.Decimal
	// Interest — accrued, unpaid interest.
	Interest decimal.Decimal
}

// SubAccountCrossMarginBalance — a sub-account's cross-margin account
// (GET /wallet/sub_account_cross_margin_balances).
type SubAccountCrossMarginBalance struct {
	// UID — the sub-account user id.
	UID string
	// Locked — whether the cross-margin account is locked.
	Locked bool
	// Total — total account value.
	Total decimal.Decimal
	// Borrowed — total outstanding borrowed principal.
	Borrowed decimal.Decimal
	// Interest — total accrued, unpaid interest.
	Interest decimal.Decimal
	// Risk — the account risk ratio.
	Risk decimal.Decimal
	// Balances — per-currency cross-margin balances.
	Balances map[string]CrossMarginCurrencyBalance
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
