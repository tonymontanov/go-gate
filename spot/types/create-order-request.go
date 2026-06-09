/*
FILE: spot/types/create-order-request.go

DESCRIPTION:
CreateOrderRequest — input for TradingClient.CreateOrder / CreateBatchOrders.

UNITS & CONVENTIONS:
  - Amount is in BASE currency (Gate-native spot amount). EXCEPTION: for a MARKET
    BUY, Gate interprets "amount" as the QUOTE amount to spend — set Amount to the
    quote total in that case (the trading layer documents/validates this).
  - Side is an explicit Gate field (buy/sell), unlike futures.
  - Price is the limit price; omit (zero) for a market order.
  - Text is the client order id. The SDK auto-prefixes "t-" (Gate requirement)
    and validates the remainder.
*/

package types

import "github.com/shopspring/decimal"

// CreateOrderRequest — parameters for creating a spot order.
type CreateOrderRequest struct {
	// CurrencyPair — Gate currency pair, e.g. "BTC_USDT". Required.
	CurrencyPair string
	// Side — Buy or Sell. Required.
	Side SideType
	// Amount — order amount in base currency (positive). For a MARKET BUY this is
	// the QUOTE amount to spend instead. Required.
	Amount decimal.Decimal
	// Price — limit price. Required for limit orders; ignored for market orders.
	Price decimal.Decimal
	// OrderType — Limit or Market. Optional: defaults to Limit (or Market when
	// Price is zero).
	OrderType OrderType
	// TimeInForce — gtc/ioc/poc/fok. Optional: defaults to gtc for limit orders.
	// Market orders require ioc or fok (the trading layer defaults to ioc).
	TimeInForce TimeInForceType
	// Text — client order id. Optional. Sent as Gate's "text" field, auto-prefixed
	// "t-" and validated (≤28 trailing chars of [0-9A-Za-z._-]).
	Text string
	// Account — settlement account. Optional: defaults to "spot".
	Account AccountType
	// Iceberg — optional iceberg display amount in base currency (0 = disabled).
	Iceberg decimal.Decimal
}
