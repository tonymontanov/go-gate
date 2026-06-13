/*
FILE: earn/types/dual.go

DESCRIPTION:
SDK shapes for the Gate Earn Dual Investment section (base "/earn/dual"). A dual
investment subscribes "copies" of a structured product that delivers either the
invested asset or the exercise asset at a fixed delivery time, depending on
whether the strike price is reached.

  - DualInvestmentPlan — a subscribable dual-investment plan
    (GET /earn/dual/investment_plan, GET /earn/dual/project-recommend).
  - DualOrder          — the caller's subscribed order
    (GET/POST /earn/dual/orders).
  - DualBalance        — the caller's dual-investment balance for a currency
    (GET /earn/dual/balance).
  - DualRefundPreview  — the projected refund of an order
    (GET /earn/dual/order-refund-preview).
  - CreateDualOrderRequest — POST /earn/dual/orders body.
  - ModifyReinvestRequest  — POST /earn/dual/modify-order-reinvest body.

CALIBRATION: field names follow Gate's Earn Dual-Investment docs; epoch-second
timestamps are normalized to epoch-millisecond ...Ms fields. Verify the exact
field set and the status vocabulary against a live account.
*/

package types

import "github.com/shopspring/decimal"

// DualInvestmentPlan — a subscribable dual-investment plan.
type DualInvestmentPlan struct {
	// ID — Gate plan id (the "plan_id" subscribed via CreateOrder).
	ID string
	// Instrument — the underlying instrument/symbol, e.g. "BTC_USDT".
	Instrument string
	// InvestCurrency — the asset the caller invests.
	InvestCurrency string
	// ExerciseCurrency — the asset delivered if the option is exercised.
	ExerciseCurrency string
	// DeliveryTimeMs — settlement/delivery time in epoch milliseconds.
	DeliveryTimeMs int64
	// APR — headline annualized yield of the plan.
	APR decimal.Decimal
	// MinAPR — lower bound of the plan's yield band.
	MinAPR decimal.Decimal
	// StrikePrice — the strike at which delivery currency flips.
	StrikePrice decimal.Decimal
	// Copies — total copies still available to subscribe.
	Copies decimal.Decimal
	// PerValue — invested value of a single copy.
	PerValue decimal.Decimal
	// Status — plan status (verbatim wire value).
	Status string
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// DualOrder — the caller's subscribed dual-investment order.
type DualOrder struct {
	// ID — Gate order id.
	ID string
	// PlanID — the subscribed plan id.
	PlanID string
	// Copies — number of copies subscribed.
	Copies decimal.Decimal
	// InvestCurrency — the invested asset code.
	InvestCurrency string
	// InvestAmount — total invested principal.
	InvestAmount decimal.Decimal
	// SettlementCurrency — the asset delivered at settlement.
	SettlementCurrency string
	// SettlementAmount — amount delivered at settlement (0 until settled).
	SettlementAmount decimal.Decimal
	// APR — annualized yield locked for this order.
	APR decimal.Decimal
	// Status — order status (verbatim wire value).
	Status string
	// CreatedAtMs — subscription time in epoch milliseconds.
	CreatedAtMs int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// DualBalance — the caller's dual-investment balance for a single currency.
type DualBalance struct {
	// Currency — the asset code, e.g. "USDT".
	Currency string
	// Amount — total balance held in the dual-investment account.
	Amount decimal.Decimal
	// Locked — portion currently locked in active orders.
	Locked decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// DualRefundPreview — the projected refund for cancelling/refunding an order.
type DualRefundPreview struct {
	// OrderID — the order the preview applies to.
	OrderID string
	// Currency — the asset that would be refunded.
	Currency string
	// RefundAmount — the projected refund amount.
	RefundAmount decimal.Decimal
	// Fee — the refund fee deducted (0 if none).
	Fee decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// CreateDualOrderRequest — parameters for POST /earn/dual/orders.
type CreateDualOrderRequest struct {
	// PlanID — the dual-investment plan to subscribe (sent as "plan_id"). Required.
	PlanID string
	// Copies — number of copies to subscribe. Provide Copies (> 0) and/or Amount;
	// at least one must be positive.
	Copies int64
	// Amount — principal to invest. Optional alternative/companion to Copies.
	Amount decimal.Decimal
}

// ModifyReinvestRequest — parameters for POST /earn/dual/modify-order-reinvest.
type ModifyReinvestRequest struct {
	// OrderID — the order whose auto-reinvest flag is being changed. Required.
	OrderID string
	// Reinvest — desired auto-reinvest state (sent as "reinvest").
	Reinvest bool
}
