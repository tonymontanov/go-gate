/*
FILE: spot/types/order-book.go

DESCRIPTION:
Order book snapshot types for Gate spot (REST /spot/order_book). Unlike futures
(where a level is {s:contracts, p:price}), Gate spot returns each level as a
["price", "amount"] string pair where amount is in BASE currency. ID is the Gate
order-book version (with_id=true), usable as the snapshot baseline for an
incremental engine in a later iteration.
*/

package types

import "github.com/shopspring/decimal"

// OrderBookLevel — a single price level: price and amount (in base currency).
type OrderBookLevel struct {
	Price  decimal.Decimal
	Amount decimal.Decimal
}

// OrderBook — a spot order book snapshot.
type OrderBook struct {
	// ID — Gate order-book version id (0 if fetched without with_id). Increases on
	// every change.
	ID int64
	// Asks — ascending by price.
	Asks []OrderBookLevel
	// Bids — descending by price.
	Bids []OrderBookLevel
	// CurrentMs — response generation time in epoch milliseconds.
	CurrentMs int64
	// UpdateMs — last order-book change time in epoch milliseconds.
	UpdateMs int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
