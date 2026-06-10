/*
FILE: delivery/types/stream-types.go

DESCRIPTION:
Types delivered by the futures WebSocket StreamClient that are not already
covered by the REST domain types (OrderInfo, PositionInfo, Ticker).
*/

package types

import "github.com/shopspring/decimal"

// BookTicker — a best-bid/offer (BBO) update from the futures.book_ticker channel.
// Sizes are in contracts.
type BookTicker struct {
	// Contract — Gate contract, e.g. "BTC_USDT".
	Contract string
	// BidPrice / BidSize — best bid price and size.
	BidPrice decimal.Decimal
	BidSize  decimal.Decimal
	// AskPrice / AskSize — best ask price and size.
	AskPrice decimal.Decimal
	AskSize  decimal.Decimal
	// UpdateID — Gate order-book update id at this BBO ("u").
	UpdateID int64
	// Ts — update time in epoch milliseconds.
	Ts int64
}

// PublicTrade — a public trade from the futures.trades channel.
type PublicTrade struct {
	// ID — Gate trade id.
	ID int64
	// Contract — Gate contract.
	Contract string
	// Price — trade price.
	Price decimal.Decimal
	// Size — absolute trade size in contracts.
	Size decimal.Decimal
	// Side — taker side, derived from the sign of the wire size
	// (Gate: positive = taker buy, negative = taker sell).
	Side SideType
	// Ts — trade time in epoch milliseconds.
	Ts int64
}
