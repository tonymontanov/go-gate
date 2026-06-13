/*
FILE: options/types/account-info.go

DESCRIPTION:
AccountInfo — the SDK's representation of the Gate OPTIONS account (from
GET /options/accounts). Unlike futures, the options account endpoint returns a
SINGLE account object (not a list of currency balances): the options account is a
single margined book. All monetary fields are in the settlement currency.

CALIBRATION: the field set (total, position_value, equity, init_margin,
maint_margin, order_margin, available, bonus) follows Gate's options docs; verify
live.
*/

package types

import "github.com/shopspring/decimal"

// AccountInfo — normalized options account state.
type AccountInfo struct {
	// User — the Gate user id owning the account.
	User int64
	// Currency — settlement currency, e.g. "USDT".
	Currency string
	// Total — total account balance.
	Total decimal.Decimal
	// PositionValue — total value of open positions.
	PositionValue decimal.Decimal
	// Equity — account equity (total + unrealised pnl).
	Equity decimal.Decimal
	// UnrealisedPnl — unrealized PnL across all positions.
	UnrealisedPnl decimal.Decimal
	// InitMargin — initial margin requirement.
	InitMargin decimal.Decimal
	// MaintMargin — maintenance margin requirement.
	MaintMargin decimal.Decimal
	// OrderMargin — margin frozen by open orders.
	OrderMargin decimal.Decimal
	// Available — balance available for new orders.
	Available decimal.Decimal
	// Bonus — bonus/credit balance (Gate "bonus").
	Bonus decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// AccountBookEntry — one row of the options account-changing history (from
// GET /options/account_book): a balance change with its cause.
type AccountBookEntry struct {
	// TimeMs — change time in epoch milliseconds.
	TimeMs int64
	// Change — signed balance change.
	Change decimal.Decimal
	// Balance — balance after the change.
	Balance decimal.Decimal
	// Type — Gate change type (e.g. "dnw", "prem", "fee", "set", "point").
	Type string
	// Text — human-readable description (Gate "text").
	Text string
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
