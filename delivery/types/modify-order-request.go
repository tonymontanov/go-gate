/*
FILE: delivery/types/modify-order-request.go

DESCRIPTION:
ModifyOrderRequest — input for TradingClient.ModifyOrder / ModifyBatchOrders
(Gate PUT /delivery/{settle}/orders/{order_id} and batch_amend_orders).

Gate keeps the order side fixed on amend; only price and/or size may change. When
amending the size, Side is required so the SDK can re-apply the Gate signed-size
convention (the new size must keep the original direction).
*/

package types

import "github.com/shopspring/decimal"

// ModifyOrderRequest — parameters for amending a futures order. Exactly one of
// OrderID / ClientOrderID identifies the order; at least one of NewPrice / NewSize
// must be set.
type ModifyOrderRequest struct {
	// Contract — Gate contract, e.g. "BTC_USDT". Required (for rate-limit scope).
	Contract string
	// OrderID — Gate numeric order id. Use this OR ClientOrderID.
	OrderID string
	// ClientOrderID — the order's "text" id (with or without the "t-" prefix).
	// Gate accepts the text in place of the numeric id. Use this OR OrderID.
	ClientOrderID string
	// Side — original order side. Required when NewSize is set, to encode the
	// Gate signed-size convention (side must match the original order).
	Side SideType
	// NewSize — new total order size in contracts (positive magnitude). Optional.
	NewSize decimal.Decimal
	// NewPrice — new limit price. Optional.
	NewPrice decimal.Decimal
	// AmendText — custom info recorded with the amendment (Gate "amend_text").
	AmendText string
}
