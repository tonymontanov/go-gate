/*
FILE: options/types/symbol-info.go

DESCRIPTION:
SymbolInfo — the SDK's representation of a Gate OPTIONS contract specification
(from /options/contracts). An options contract is dated and struck: its name
encodes the underlying, expiry and strike (e.g. "BTC_USDT-20240329-50000-C")
and the spec carries ExpirationMs, StrikePrice and IsCall (call vs put), plus the
contract Multiplier (base-asset value of one contract), the price tick, the
maker/taker fee rates, and — when Gate includes them on the spec feed — the live
mark/last prices and option greeks.

CALIBRATION: the options-specific field names (underlying, expiration_time,
strike_price, is_call, multiplier, order_price_round) follow Gate's options docs;
verify field exactness live (host wss://op-ws.gateio.live/v4/ws).
*/

package types

import "github.com/shopspring/decimal"

// SymbolInfo — normalized options contract specification.
type SymbolInfo struct {
	// Contract — Gate options contract name, e.g. "BTC_USDT-20240329-50000-C".
	Contract string
	// Underlying — the underlying index this option is written on, e.g.
	// "BTC_USDT" (Gate "underlying").
	Underlying string
	// ExpirationMs — contract expiry/settlement time in epoch milliseconds
	// (Gate "expiration_time", seconds).
	ExpirationMs int64
	// StrikePrice — the option strike price (Gate "strike_price").
	StrikePrice decimal.Decimal
	// IsCall — true for a call option, false for a put (Gate "is_call").
	IsCall bool
	// OptionType — call/put, derived from IsCall for convenience.
	OptionType OptionType
	// Multiplier — base-asset amount represented by ONE contract. Used to convert
	// base quantity → contracts (qty / multiplier) and back (Gate "multiplier").
	Multiplier decimal.Decimal
	// OrderPriceRound — minimum price increment (tick size; Gate "order_price_round").
	OrderPriceRound decimal.Decimal
	// MarkPriceRound — minimum mark-price increment (Gate "mark_price_round").
	MarkPriceRound decimal.Decimal
	// OrderSizeMin — minimum order size in contracts.
	OrderSizeMin decimal.Decimal
	// OrderSizeMax — maximum order size in contracts.
	OrderSizeMax decimal.Decimal
	// MakerFeeRate / TakerFeeRate — fee rates for maker/taker fills.
	MakerFeeRate decimal.Decimal
	TakerFeeRate decimal.Decimal
	// RefDiscountRate — referrer commission discount rate.
	RefDiscountRate decimal.Decimal
	// RefRebateRate — referee rebate rate.
	RefRebateRate decimal.Decimal
	// MarkPrice / LastPrice — current prices from the contract feed (may be 0 on
	// a spec-only response).
	MarkPrice decimal.Decimal
	LastPrice decimal.Decimal
	// IndexPrice — current price of the underlying index (may be 0).
	IndexPrice decimal.Decimal
	// MarkIv — mark implied volatility (may be 0).
	MarkIv decimal.Decimal
	// Delta / Gamma / Vega / Theta — option greeks (may be 0 when Gate omits them
	// from the spec response).
	Delta decimal.Decimal
	Gamma decimal.Decimal
	Vega  decimal.Decimal
	Theta decimal.Decimal
	// OrdersLimit — max number of open orders allowed on the contract.
	OrdersLimit int64
	// InDelisting — whether the contract is being delisted.
	InDelisting bool
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
