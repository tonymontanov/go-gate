/*
FILE: margin/types/account.go

DESCRIPTION:
Account-side structs for the Gate ISOLATED-margin section:

  - MarginAccount     — an isolated-margin account for a single currency pair,
    with its per-leg (base/quote) balance breakdown (GET /margin/accounts).
  - MarginBalance     — one leg's balance within a MarginAccount: what is
    available, locked by orders, borrowed, and the accrued interest.
  - FundingAccount    — a margin LENDING (funding) account balance for a single
    currency (GET /margin/funding_accounts).
  - AccountBookEntry  — one row of the margin account-changing history
    (GET /margin/account_book, GET /margin/cross/account_book).

Monetary fields use decimal.Decimal. The wire payloads (decoded in the margin
package) use codec.FlexDecimal because Gate may quote a balance as a string or
send it as a bare JSON number.
*/

package types

import "github.com/shopspring/decimal"

// MarginBalance — one leg (base or quote) of an isolated-margin account.
type MarginBalance struct {
	// Currency — the leg currency, e.g. "BTC" (base) or "USDT" (quote).
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

// MarginAccount — normalized isolated-margin account for one currency pair.
type MarginAccount struct {
	// CurrencyPair — the Gate pair, e.g. "BTC_USDT".
	CurrencyPair string
	// Locked — whether the account is locked (e.g. during liquidation).
	Locked bool
	// Risk — the account risk ratio Gate reports for the pair.
	Risk decimal.Decimal
	// MarginLevel — the current margin level, when Gate reports it.
	MarginLevel decimal.Decimal
	// Base — the base-currency leg balance breakdown.
	Base MarginBalance
	// Quote — the quote-currency leg balance breakdown.
	Quote MarginBalance
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// FundingAccount — normalized margin lending (funding) account for one currency.
type FundingAccount struct {
	// Currency — the funding currency, e.g. "USDT".
	Currency string
	// Available — balance available to lend.
	Available decimal.Decimal
	// Locked — balance locked in pending lend orders.
	Locked decimal.Decimal
	// Lent — principal currently lent out and accruing interest.
	Lent decimal.Decimal
	// TotalLent — total principal ever lent (Gate "total_lent").
	TotalLent decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// AccountBookEntry — one row of the margin account-changing history. Shared by
// the isolated (GET /margin/account_book) and cross (GET /margin/cross/account_book)
// histories; CurrencyPair is empty for cross rows.
type AccountBookEntry struct {
	// ID — the Gate ledger entry id.
	ID string
	// TimeMs — change time in epoch milliseconds.
	TimeMs int64
	// Currency — the currency whose balance changed.
	Currency string
	// CurrencyPair — the isolated pair the change applies to (empty for cross).
	CurrencyPair string
	// Change — signed balance change.
	Change decimal.Decimal
	// Balance — balance after the change.
	Balance decimal.Decimal
	// Type — Gate change type (e.g. "lend", "borrow", "repay", "in", "out").
	Type string
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
