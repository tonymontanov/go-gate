/*
FILE: futures/types/symbol-info.go

DESCRIPTION:
SymbolInfo — the SDK's representation of a Gate futures contract specification
(from /futures/{settle}/contracts). It carries the fields the desk needs to size
and price orders, most importantly QuantoMultiplier (base-asset value of one
contract) used to convert base quantity ↔ contracts, and OrderPriceRound (tick).
*/

package types

import "github.com/shopspring/decimal"

// SymbolInfo — normalized futures contract specification.
type SymbolInfo struct {
	// Contract — Gate contract name, e.g. "BTC_USDT".
	Contract string
	// Type — Gate contract type ("direct" for linear USDT-settled, "inverse").
	Type string
	// QuantoMultiplier — base-asset amount represented by ONE contract. Used to
	// convert base quantity → contracts (qty / multiplier) and back.
	QuantoMultiplier decimal.Decimal
	// OrderSizeMin — minimum order size in contracts.
	OrderSizeMin decimal.Decimal
	// OrderSizeMax — maximum order size in contracts.
	OrderSizeMax decimal.Decimal
	// OrderPriceRound — minimum price increment (tick size).
	OrderPriceRound decimal.Decimal
	// MarkPriceRound — minimum mark-price increment.
	MarkPriceRound decimal.Decimal
	// OrderPriceDeviate — max allowed deviation of order price from mark price.
	OrderPriceDeviate decimal.Decimal
	// LeverageMin / LeverageMax — allowable leverage bounds.
	LeverageMin decimal.Decimal
	LeverageMax decimal.Decimal
	// MaintenanceRate — maintenance margin rate.
	MaintenanceRate decimal.Decimal
	// MarkPrice / IndexPrice / LastPrice — current prices from the contract feed.
	MarkPrice  decimal.Decimal
	IndexPrice decimal.Decimal
	LastPrice  decimal.Decimal
	// FundingRate — current funding rate.
	FundingRate decimal.Decimal
	// FundingIntervalSec — funding interval in seconds.
	FundingIntervalSec int64
	// OrdersLimit — max number of open orders allowed on the contract.
	OrdersLimit int64
	// InDelisting — whether the contract is being delisted.
	InDelisting bool
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
