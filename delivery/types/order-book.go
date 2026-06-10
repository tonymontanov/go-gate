/*
FILE: delivery/types/order-book.go

DESCRIPTION:
Order book snapshot types for Gate futures (REST /delivery/{settle}/order_book).
Prices and sizes are decimals; sizes are in contracts. ID is the Gate order-book
version (with_id=true), usable as the snapshot baseline for an incremental engine
in a later iteration.
*/

package types

import "github.com/shopspring/decimal"

// OrderBookLevel — a single price level: price and size (in contracts).
type OrderBookLevel struct {
	Price decimal.Decimal
	Size  decimal.Decimal
}

// OrderBook — a futures order book snapshot.
type OrderBook struct {
	// ID — Gate order-book version id (0 if the snapshot was fetched without
	// with_id). Increases by 1 on every change.
	ID int64
	// Asks — ascending by price.
	Asks []OrderBookLevel
	// Bids — descending by price.
	Bids []OrderBookLevel
	// CurrentMs — response generation time in epoch milliseconds (best effort).
	CurrentMs int64
	// UpdateMs — last order-book change time in epoch milliseconds (best effort).
	UpdateMs int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
