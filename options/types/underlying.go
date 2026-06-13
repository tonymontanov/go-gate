/*
FILE: options/types/underlying.go

DESCRIPTION:
Underlying — the SDK's representation of a Gate OPTIONS underlying index (from
GET /options/underlyings). Each underlying (e.g. "BTC_USDT") groups the option
contracts written on it; the expirations and the strike chain hang off the
underlying. The list endpoint returns the underlying name plus its current index
price.

CALIBRATION: field names (name, index_price) follow Gate's options docs; verify
live.
*/

package types

import "github.com/shopspring/decimal"

// Underlying — one options underlying index.
type Underlying struct {
	// Name — the underlying index, e.g. "BTC_USDT" (Gate "name").
	Name string
	// IndexPrice — current price of the underlying index (Gate "index_price").
	IndexPrice decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// UnderlyingTicker — the aggregate ticker for an underlying index (from
// GET /options/underlying/tickers/{underlying}): its current index price and the
// recent put/call trade activity.
type UnderlyingTicker struct {
	// Underlying — the underlying index the ticker is for, e.g. "BTC_USDT".
	Underlying string
	// IndexPrice — current price of the underlying index (Gate "index_price").
	IndexPrice decimal.Decimal
	// TradePut — recent put-option trade volume/count (Gate "trade_put").
	TradePut decimal.Decimal
	// TradeCall — recent call-option trade volume/count (Gate "trade_call").
	TradeCall decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
