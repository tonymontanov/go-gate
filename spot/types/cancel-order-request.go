/*
FILE: spot/types/cancel-order-request.go

DESCRIPTION:
CancelOrderRequest — input for TradingClient.CancelOrder / CancelBatchOrders.
Gate spot identifies an order by its numeric OrderID or its client "text" id
(ClientOrderID), and ALWAYS requires the currency pair (passed as a query param
on the single-cancel endpoint, and per-item on the batch endpoint).
*/

package types

// CancelOrderRequest — parameters for cancelling a spot order.
type CancelOrderRequest struct {
	// CurrencyPair — Gate currency pair, e.g. "BTC_USDT". Required.
	CurrencyPair string
	// OrderID — Gate numeric order id. Use this OR ClientOrderID.
	OrderID string
	// ClientOrderID — the order's "text" id (with or without the "t-" prefix).
	// Use this OR OrderID.
	ClientOrderID string
}
