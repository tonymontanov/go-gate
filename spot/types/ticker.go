/*
FILE: spot/types/ticker.go

DESCRIPTION:
Ticker — the SDK's representation of a Gate spot ticker (REST /spot/tickers).
Spot tickers carry best bid/ask (with sizes) and 24h stats but, unlike futures,
have no mark/index price or funding. All numeric fields are decimals.
*/

package types

import "github.com/shopspring/decimal"

// Ticker — normalized spot ticker.
type Ticker struct {
	// CurrencyPair — Gate currency pair, e.g. "BTC_USDT".
	CurrencyPair string
	// Last — last traded price.
	Last decimal.Decimal
	// LowestAsk — best ask price.
	LowestAsk decimal.Decimal
	// LowestSize — size at the best ask (base currency).
	LowestSize decimal.Decimal
	// HighestBid — best bid price.
	HighestBid decimal.Decimal
	// HighestSize — size at the best bid (base currency).
	HighestSize decimal.Decimal
	// ChangePercentage — 24h change percentage.
	ChangePercentage decimal.Decimal
	// BaseVolume — 24h trading volume in base currency.
	BaseVolume decimal.Decimal
	// QuoteVolume — 24h trading volume in quote currency.
	QuoteVolume decimal.Decimal
	// High24h — 24h high price.
	High24h decimal.Decimal
	// Low24h — 24h low price.
	Low24h decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
