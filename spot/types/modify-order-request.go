/*
FILE: spot/types/modify-order-request.go

DESCRIPTION:
ModifyOrderRequest — input for TradingClient.ModifyOrder (Gate
PATCH /spot/orders/{order_id}?currency_pair=...). Note this differs from futures,
which amends via PUT. Gate keeps the order side fixed on amend; only amount and/or
price may change.
*/

package types

import "github.com/shopspring/decimal"

// ModifyOrderRequest — parameters for amending a spot order. Exactly one of
// OrderID / ClientOrderID identifies the order; at least one of NewPrice /
// NewAmount must be set.
type ModifyOrderRequest struct {
	// CurrencyPair — Gate currency pair, e.g. "BTC_USDT". Required.
	CurrencyPair string
	// OrderID — Gate numeric order id. Use this OR ClientOrderID.
	OrderID string
	// ClientOrderID — the order's "text" id (with or without the "t-" prefix).
	// Use this OR OrderID.
	ClientOrderID string
	// NewAmount — new order amount in base currency (positive). Optional.
	NewAmount decimal.Decimal
	// NewPrice — new limit price. Optional.
	NewPrice decimal.Decimal
	// AmendText — custom info recorded with the amendment (Gate "amend_text").
	AmendText string
}
