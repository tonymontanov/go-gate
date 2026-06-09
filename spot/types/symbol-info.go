/*
FILE: spot/types/symbol-info.go

DESCRIPTION:
SymbolInfo — the SDK's representation of a Gate spot currency pair specification
(from /spot/currency_pairs/{currency_pair}). Spot sizing differs from futures:
amounts are in BASE currency (not contracts), and precision is expressed as a
number of decimal places (AmountPrecision for base amount, PricePrecision for
price) rather than a tick/quanto multiplier.
*/

package types

import "github.com/shopspring/decimal"

// SymbolInfo — normalized spot currency-pair specification.
type SymbolInfo struct {
	// CurrencyPair — Gate currency pair / "id", e.g. "BTC_USDT".
	CurrencyPair string
	// Base — base currency, e.g. "BTC".
	Base string
	// Quote — quote currency, e.g. "USDT".
	Quote string
	// Fee — trading fee rate in percent (Gate "fee").
	Fee decimal.Decimal
	// MinBaseAmount — minimum order amount in base currency.
	MinBaseAmount decimal.Decimal
	// MinQuoteAmount — minimum order amount in quote currency.
	MinQuoteAmount decimal.Decimal
	// MaxBaseAmount — maximum order amount in base currency (0 if unset).
	MaxBaseAmount decimal.Decimal
	// MaxQuoteAmount — maximum order amount in quote currency (0 if unset).
	MaxQuoteAmount decimal.Decimal
	// AmountPrecision — number of decimal places allowed for the base amount.
	AmountPrecision int32
	// PricePrecision — number of decimal places allowed for the price
	// (Gate "precision").
	PricePrecision int32
	// TradeStatus — "tradable", "untradable", "buyable", "sellable", etc.
	TradeStatus string
	// MarketOrderMaxBase — max base amount for a market order (Gate
	// "market_order_max_stock"; 0 if unset).
	MarketOrderMaxBase decimal.Decimal
	// MarketOrderMaxQuote — max quote amount for a market order (Gate
	// "market_order_max_money"; 0 if unset).
	MarketOrderMaxQuote decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
