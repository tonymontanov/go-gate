/*
FILE: options/types/stream-types.go

DESCRIPTION:
Types delivered by the options WebSocket StreamClient that are not already
covered by the REST domain types (Ticker, Trade, OrderInfo, PositionInfo). The
options socket adds an underlying-price feed (options.ul_price) and a private
user-trades feed (options.usertrades).
*/

package types

import "github.com/shopspring/decimal"

// UnderlyingPrice — an underlying index-price update from the options.ul_price
// channel.
type UnderlyingPrice struct {
	// Underlying — the underlying index, e.g. "BTC_USDT".
	Underlying string
	// Price — the current underlying index price.
	Price decimal.Decimal
	// Ts — update time in epoch milliseconds.
	Ts int64
}

// UserTrade — one of the account's own fills from the options.usertrades channel.
type UserTrade struct {
	// ID — Gate trade id.
	ID string
	// Contract — Gate options contract.
	Contract string
	// OrderID — the order id this fill belongs to.
	OrderID string
	// ClientOrderID — the order's "text" id (Gate "text").
	ClientOrderID string
	// Price — fill price.
	Price decimal.Decimal
	// Size — absolute fill size in contracts.
	Size decimal.Decimal
	// Side — fill side, derived from the sign of the wire size.
	Side SideType
	// Role — "maker" or "taker" (Gate "role"), when present.
	Role string
	// Ts — fill time in epoch milliseconds.
	Ts int64
}
