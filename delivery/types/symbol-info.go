/*
FILE: delivery/types/symbol-info.go

DESCRIPTION:
SymbolInfo — the SDK's representation of a Gate DELIVERY contract specification
(from /delivery/{settle}/contracts). Like the futures spec it carries
QuantoMultiplier (base-asset value of one contract) and OrderPriceRound (tick),
but delivery contracts are DATED — the Contract name encodes the expiry
("BTC_USDT_20240329") and the spec adds ExpireTimeMs / Cycle. Delivery settles at
expiry, so there is NO funding rate (unlike perpetual futures).

CALIBRATION: the delivery-specific field names (expire_time, cycle) follow Gate's
delivery docs; verify live.
*/

package types

import "github.com/shopspring/decimal"

// SymbolInfo — normalized delivery contract specification.
type SymbolInfo struct {
	// Contract — Gate delivery contract name, dated, e.g. "BTC_USDT_20240329".
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
	// ExpireTimeMs — contract expiry/settlement time in epoch milliseconds
	// (delivery-specific; Gate "expire_time").
	ExpireTimeMs int64
	// Cycle — settlement cycle label, e.g. "WEEKLY"/"QUARTERLY" (Gate "cycle").
	Cycle string
	// OrdersLimit — max number of open orders allowed on the contract.
	OrdersLimit int64
	// InDelisting — whether the contract is being delisted.
	InDelisting bool
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
