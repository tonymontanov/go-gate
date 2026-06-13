/*
FILE: options/types/ticker.go

DESCRIPTION:
Ticker — the SDK's representation of a Gate OPTIONS ticker (REST /options/tickers
and the options.contract_tickers WS channel). Beyond last/mark prices it carries
the implied-volatility surface (mark/bid/ask IV), the top-of-book quote with its
IV, the open position size, and the option greeks (delta/gamma/vega/theta).
*/

package types

import "github.com/shopspring/decimal"

// Ticker — normalized options ticker.
type Ticker struct {
	// Contract — Gate options contract, e.g. "BTC_USDT-20240329-50000-C".
	Contract string
	// LastPrice — last traded price.
	LastPrice decimal.Decimal
	// MarkPrice — current mark price.
	MarkPrice decimal.Decimal
	// IndexPrice — current price of the underlying index (may be 0).
	IndexPrice decimal.Decimal
	// MarkIv — mark implied volatility.
	MarkIv decimal.Decimal
	// Bid1Price / Bid1Size / Bid1Iv — best bid price, size and its implied vol.
	Bid1Price decimal.Decimal
	Bid1Size  decimal.Decimal
	Bid1Iv    decimal.Decimal
	// Ask1Price / Ask1Size / Ask1Iv — best ask price, size and its implied vol.
	Ask1Price decimal.Decimal
	Ask1Size  decimal.Decimal
	Ask1Iv    decimal.Decimal
	// PositionSize — total open position size of the contract (contracts).
	PositionSize decimal.Decimal
	// Delta / Gamma / Vega / Theta — option greeks.
	Delta decimal.Decimal
	Gamma decimal.Decimal
	Vega  decimal.Decimal
	Theta decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
