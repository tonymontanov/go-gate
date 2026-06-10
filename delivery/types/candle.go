/*
FILE: delivery/types/candle.go

DESCRIPTION:
Candle and CandleInterval types for Gate futures candlesticks
(REST /delivery/{settle}/candlesticks). Gate timestamps are in seconds; the SDK
normalizes the open time to milliseconds. Volume is in contracts.
*/

package types

import "github.com/shopspring/decimal"

// CandleInterval — Gate candlestick interval. Values are the exact Gate wire
// strings accepted by the candlesticks endpoint.
type CandleInterval string

const (
	CandleInterval10s CandleInterval = "10s"
	CandleInterval1m  CandleInterval = "1m"
	CandleInterval5m  CandleInterval = "5m"
	CandleInterval15m CandleInterval = "15m"
	CandleInterval30m CandleInterval = "30m"
	CandleInterval1h  CandleInterval = "1h"
	CandleInterval4h  CandleInterval = "4h"
	CandleInterval8h  CandleInterval = "8h"
	CandleInterval1d  CandleInterval = "1d"
	CandleInterval7d  CandleInterval = "7d"
	CandleInterval30d CandleInterval = "30d"
)

// Candle — a single futures candlestick.
type Candle struct {
	// OpenTimeMs — bucket open time in epoch milliseconds.
	OpenTimeMs int64
	// Open / High / Low / Close — OHLC prices.
	Open  decimal.Decimal
	High  decimal.Decimal
	Low   decimal.Decimal
	Close decimal.Decimal
	// Volume — traded volume in contracts.
	Volume decimal.Decimal
	// QuoteVolume — traded volume in quote currency (Gate "sum").
	QuoteVolume decimal.Decimal
}
