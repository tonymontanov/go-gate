/*
FILE: wallet/types/fee.go

DESCRIPTION:
TradeFee — the caller's personal trading-fee rates as Gate reports them
(GET /wallet/fee). It covers the spot taker/maker rates, the GT-deduction
variants, and the futures/delivery taker/maker rates, plus the point type and
loan fee.

CALIBRATION: the field set follows Gate's wallet fee docs; verify live.
*/

package types

import "github.com/shopspring/decimal"

// TradeFee — normalized personal trading-fee rates (GET /wallet/fee).
type TradeFee struct {
	// UserID — the account user id.
	UserID int64
	// TakerFee — the spot taker fee rate.
	TakerFee decimal.Decimal
	// MakerFee — the spot maker fee rate.
	MakerFee decimal.Decimal
	// GTDiscount — whether GT-point fee deduction is enabled.
	GTDiscount bool
	// GTTakerFee — the spot taker fee rate when paying with GT.
	GTTakerFee decimal.Decimal
	// GTMakerFee — the spot maker fee rate when paying with GT.
	GTMakerFee decimal.Decimal
	// LoanFee — the margin loan fee rate.
	LoanFee decimal.Decimal
	// PointType — the Gate point-type tier of the account.
	PointType string
	// FuturesTakerFee — the perpetual-futures taker fee rate.
	FuturesTakerFee decimal.Decimal
	// FuturesMakerFee — the perpetual-futures maker fee rate.
	FuturesMakerFee decimal.Decimal
	// DeliveryTakerFee — the delivery-futures taker fee rate.
	DeliveryTakerFee decimal.Decimal
	// DeliveryMakerFee — the delivery-futures maker fee rate.
	DeliveryMakerFee decimal.Decimal
	// DebitFee — the Gate debit-fee tier indicator.
	DebitFee int64
	// CurrencyPair — the pair the fee applies to, when queried per pair.
	CurrencyPair string
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
