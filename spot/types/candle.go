/*
FILE: spot/types/candle.go

DESCRIPTION:
Candle and CandleInterval types for Gate spot candlesticks
(REST /spot/candlesticks). Gate returns each candle as an ARRAY whose column
order differs from futures:

	[ t, quote_volume, close, high, low, open, base_volume, window_closed ]

(futures order is t, volume, close, high, low, open, sum). The SDK normalizes
the open time to milliseconds; Gate's spot candle timestamp is in seconds.
*/

package types

import "github.com/shopspring/decimal"

// CandleInterval — Gate candlestick interval. Values are the exact Gate wire
// strings accepted by the spot candlesticks endpoint.
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

// Candle — a single spot candlestick.
type Candle struct {
	// OpenTimeMs — bucket open time in epoch milliseconds.
	OpenTimeMs int64
	// Open / High / Low / Close — OHLC prices.
	Open  decimal.Decimal
	High  decimal.Decimal
	Low   decimal.Decimal
	Close decimal.Decimal
	// BaseVolume — traded volume in base currency (Gate "amount").
	BaseVolume decimal.Decimal
	// QuoteVolume — traded volume in quote currency.
	QuoteVolume decimal.Decimal
	// WindowClosed — whether this candle's time window has closed (Gate's trailing
	// boolean; false means the bucket is still forming).
	WindowClosed bool
}
