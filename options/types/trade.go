/*
FILE: options/types/trade.go

DESCRIPTION:
Trade — the SDK's representation of a Gate OPTIONS public trade (from
GET /options/trades). Size is signed on the wire (positive taker buy, negative
taker sell); the SDK exposes an absolute Size plus a derived Side. Prices are
decimals; timestamps are normalized to epoch milliseconds.
*/

package types

import "github.com/shopspring/decimal"

// Trade — one options public trade.
type Trade struct {
	// ID — Gate trade id.
	ID int64
	// Contract — Gate options contract.
	Contract string
	// Price — trade price.
	Price decimal.Decimal
	// Size — absolute trade size in contracts.
	Size decimal.Decimal
	// Side — taker side, derived from the sign of the wire size
	// (Gate: positive = taker buy, negative = taker sell).
	Side SideType
	// IsCall — whether the traded contract is a call (Gate "is_call"), when the
	// trade feed includes it.
	IsCall bool
	// Ts — trade time in epoch milliseconds.
	Ts int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
