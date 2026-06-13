/*
FILE: margin/types/funding-book.go

DESCRIPTION:
FundingBook / FundingBookEntry — the SDK's representation of the Gate ISOLATED
margin lending order book (from GET /margin/funding_book?currency=). The funding
book aggregates outstanding lend (ask) and borrow (bid) offers by rate and term,
exactly as the spot/derivatives order book aggregates price levels.
*/

package types

import "github.com/shopspring/decimal"

// FundingBookEntry — one aggregated level of the margin funding book.
type FundingBookEntry struct {
	// Rate — the (daily) interest rate offered/requested at this level.
	Rate decimal.Decimal
	// Amount — total currency amount available at this rate/term.
	Amount decimal.Decimal
	// Days — the loan term in days at this level.
	Days int64
}

// FundingBook — normalized margin funding book for a single currency.
type FundingBook struct {
	// Currency — the funding currency, e.g. "USDT".
	Currency string
	// Asks — lend offers (liquidity suppliers), ascending by rate.
	Asks []FundingBookEntry
	// Bids — borrow requests (liquidity takers), descending by rate.
	Bids []FundingBookEntry
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
