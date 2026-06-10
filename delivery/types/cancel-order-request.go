/*
FILE: delivery/types/cancel-order-request.go

DESCRIPTION:
CancelOrderRequest — input for TradingClient.CancelOrder / CancelBatchOrders /
CancelForgottenOrders. The order is identified by either its Gate numeric OrderID
or its client "text" id (ClientOrderID); Gate accepts the text in place of the id.
*/

package types

// CancelOrderRequest — parameters for cancelling a futures order.
type CancelOrderRequest struct {
	// Contract — Gate contract, e.g. "BTC_USDT". Required (for rate-limit scope).
	Contract string
	// OrderID — Gate numeric order id. Use this OR ClientOrderID.
	OrderID string
	// ClientOrderID — the order's "text" id (with or without the "t-" prefix).
	// Use this OR OrderID.
	ClientOrderID string
}
