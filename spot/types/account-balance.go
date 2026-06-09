/*
FILE: spot/types/account-balance.go

DESCRIPTION:
Balance — the SDK's representation of a single spot account balance entry (from
GET /spot/accounts). Spot has balances per currency, not positions; this replaces
the futures PositionInfo type for the spot section.
*/

package types

import "github.com/shopspring/decimal"

// Balance — a spot balance for one currency.
type Balance struct {
	// Currency — currency code, e.g. "USDT".
	Currency string
	// Available — amount available for trading/withdrawal.
	Available decimal.Decimal
	// Locked — amount locked in open orders.
	Locked decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty). Set
	// on the first element of a multi-currency response.
	RateLimits map[string]string
}
