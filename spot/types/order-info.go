/*
FILE: spot/types/order-info.go

DESCRIPTION:
OrderInfo — the SDK's stable representation of a Gate spot order, returned by the
trading and account layers. Amounts are in BASE currency; Side is an explicit
Gate field.
*/

package types

import "github.com/shopspring/decimal"

// OrderInfo — normalized spot order state.
type OrderInfo struct {
	// OrderID — Gate numeric order id as a string. Empty if the order was not
	// accepted (e.g. a rejected batch element).
	OrderID string
	// ClientOrderID — the order's "text" id as Gate stored it (with "t-" prefix).
	ClientOrderID string
	// CurrencyPair — Gate currency pair, e.g. "BTC_USDT".
	CurrencyPair string
	// Side — buy or sell.
	Side SideType
	// OrderType — limit or market.
	OrderType OrderType
	// Account — settlement account ("spot").
	Account AccountType
	// Price — order price.
	Price decimal.Decimal
	// Amount — order amount in base currency.
	Amount decimal.Decimal
	// Left — unfilled amount in base currency.
	Left decimal.Decimal
	// FilledAmount — filled amount in base currency (Amount − Left).
	FilledAmount decimal.Decimal
	// AvgDealPrice — average fill price (0 if unfilled).
	AvgDealPrice decimal.Decimal
	// FilledTotal — total filled value in quote currency (Gate "filled_total").
	FilledTotal decimal.Decimal
	// TimeInForce — gtc/ioc/poc/fok.
	TimeInForce TimeInForceType
	// Status — Gate order status: "open", "closed", or "cancelled".
	Status string
	// FinishAs — terminal reason when not open (filled/cancelled/ioc/...). Optional.
	FinishAs string
	// CreatedAtMs — creation time in epoch milliseconds.
	CreatedAtMs int64
	// UpdatedAtMs — last update time in epoch milliseconds.
	UpdatedAtMs int64
	// RateLimits — Gate rate-limit headers from the response that produced this
	// OrderInfo (may be empty).
	RateLimits map[string]string
}
