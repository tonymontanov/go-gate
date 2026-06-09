/*
FILE: spot/types/stream-types.go

DESCRIPTION:
Types delivered by the spot WebSocket StreamClient that are not already covered by
the REST domain types (OrderInfo, Balance, Ticker). All amounts are in base
currency; spot trades carry an explicit taker side string.
*/

package types

import "github.com/shopspring/decimal"

// BookTicker — a best-bid/offer (BBO) update from the spot.book_ticker channel.
type BookTicker struct {
	// CurrencyPair — Gate currency pair, e.g. "BTC_USDT".
	CurrencyPair string
	// BidPrice / BidSize — best bid price and size (base currency).
	BidPrice decimal.Decimal
	BidSize  decimal.Decimal
	// AskPrice / AskSize — best ask price and size (base currency).
	AskPrice decimal.Decimal
	AskSize  decimal.Decimal
	// UpdateID — Gate order-book update id at this BBO ("u").
	UpdateID int64
	// Ts — update time in epoch milliseconds.
	Ts int64
}

// PublicTrade — a public trade from the spot.trades channel.
type PublicTrade struct {
	// ID — Gate trade id.
	ID int64
	// CurrencyPair — Gate currency pair.
	CurrencyPair string
	// Price — trade price.
	Price decimal.Decimal
	// Amount — trade amount in base currency.
	Amount decimal.Decimal
	// Side — taker side (explicit Gate "side": buy/sell).
	Side SideType
	// Ts — trade time in epoch milliseconds.
	Ts int64
}

// BalanceUpdate — a balance change from the spot.balances private channel.
type BalanceUpdate struct {
	// Currency — currency code, e.g. "USDT".
	Currency string
	// Available — available balance after the change.
	Available decimal.Decimal
	// Total — total balance after the change.
	Total decimal.Decimal
	// Change — the delta applied (signed).
	Change decimal.Decimal
	// Ts — event time in epoch milliseconds.
	Ts int64
}

// UserTrade — a private fill from the spot.usertrades channel.
type UserTrade struct {
	// ID — Gate trade id.
	ID int64
	// OrderID — the order this fill belongs to.
	OrderID string
	// CurrencyPair — Gate currency pair.
	CurrencyPair string
	// Side — fill side (buy/sell).
	Side SideType
	// Role — "maker" or "taker".
	Role string
	// Price — fill price.
	Price decimal.Decimal
	// Amount — fill amount in base currency.
	Amount decimal.Decimal
	// Fee — fee charged for this fill.
	Fee decimal.Decimal
	// FeeCurrency — currency the fee was charged in.
	FeeCurrency string
	// Text — client order id ("text") echoed by Gate.
	Text string
	// Ts — fill time in epoch milliseconds.
	Ts int64
}
