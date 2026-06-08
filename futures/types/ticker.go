/*
FILE: futures/types/ticker.go

DESCRIPTION:
Ticker — the SDK's representation of a Gate futures ticker
(REST /futures/{settle}/tickers). All numeric fields are decimals.
*/

package types

import "github.com/shopspring/decimal"

// Ticker — normalized futures ticker.
type Ticker struct {
	// Contract — Gate contract, e.g. "BTC_USDT".
	Contract string
	// Last — last traded price.
	Last decimal.Decimal
	// MarkPrice — current mark price.
	MarkPrice decimal.Decimal
	// IndexPrice — current index price.
	IndexPrice decimal.Decimal
	// HighestBid — best bid price.
	HighestBid decimal.Decimal
	// LowestAsk — best ask price.
	LowestAsk decimal.Decimal
	// ChangePercentage — 24h change percentage.
	ChangePercentage decimal.Decimal
	// TotalSize — total position size of the contract (open interest, contracts).
	TotalSize decimal.Decimal
	// Volume24h — 24h trading volume in contracts.
	Volume24h decimal.Decimal
	// FundingRate — current funding rate.
	FundingRate decimal.Decimal
	// FundingRateIndicative — indicative (next) funding rate.
	FundingRateIndicative decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
