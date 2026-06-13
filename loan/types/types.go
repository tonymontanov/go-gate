/*
FILE: loan/types/types.go

DESCRIPTION:
Domain structs for the Gate multi-collateral LOAN section:

  - MultiLoanOrder    — a multi-collateral loan order (GET/POST
    /loan/multi_collateral/orders, GET .../orders/{order_id}).
  - CollateralItem    — one collateral currency+amount backing a loan.
  - BorrowItem        — one borrowed currency+amount of a loan.
  - RepayRecord       — one repayment record (GET .../repay).
  - MortgageRecord    — one collateral add/withdraw record (GET .../mortgage).
  - CurrencyQuota     — borrow/collateral quota for a currency
    (GET .../currency_quota).
  - MultiLoanCurrency — a supported borrow/collateral currency
    (GET .../currencies).
  - Ltv               — current and liquidation loan-to-value ratios
    (GET .../ltv).
  - FixedRate         — fixed borrow rate for a currency (GET .../fixed_rate).
  - CurrentRate       — current (floating) borrow rate for a currency
    (GET .../current_rate).
  - CreateOrderRequest / RepayRequest / MortgageRequest — request inputs.
  - RepayItem         — one per-currency repayment instruction.

Monetary fields use decimal.Decimal; the wire payloads (decoded in the loan
package) use codec.FlexDecimal because Gate may quote an amount/rate/ltv as a
string or send it as a bare JSON number. Time fields are normalized to epoch
milliseconds (...Ms).
*/

package types

import "github.com/shopspring/decimal"

// CollateralItem — one collateral currency and its amount backing a loan.
type CollateralItem struct {
	// Currency — the collateral currency, e.g. "BTC".
	Currency string
	// Amount — the collateral amount.
	Amount decimal.Decimal
}

// BorrowItem — one borrowed currency and its amount within a loan order.
type BorrowItem struct {
	// Currency — the borrowed currency, e.g. "USDT".
	Currency string
	// Amount — the borrowed amount.
	Amount decimal.Decimal
}

// MultiLoanOrder — a normalized multi-collateral loan order.
type MultiLoanOrder struct {
	// OrderID — the Gate order id.
	OrderID string
	// OrderType — floating ("current") or fixed-rate borrow mode.
	OrderType MultiLoanOrderType
	// FixedType — the fixed-rate term bucket (empty for floating loans).
	FixedType MultiLoanFixedType
	// FixedRate — the fixed (daily) borrow rate (0 for floating loans).
	FixedRate decimal.Decimal
	// ExpireTimeMs — fixed-loan expiry time in epoch milliseconds (0 if none).
	ExpireTimeMs int64
	// AutoRenew — whether a fixed loan auto-renews at expiry.
	AutoRenew bool
	// AutoRepay — whether the loan auto-repays from the spot account.
	AutoRepay bool
	// Currencies — the borrowed currencies and amounts.
	Currencies []BorrowItem
	// CollateralCurrencies — the collateral currencies and amounts.
	CollateralCurrencies []CollateralItem
	// BorrowCurrency — the primary borrowed currency, e.g. "USDT".
	BorrowCurrency string
	// BorrowAmount — the primary borrowed amount.
	BorrowAmount decimal.Decimal
	// CurrentLtv — the current loan-to-value ratio.
	CurrentLtv decimal.Decimal
	// Status — order lifecycle state.
	Status MultiLoanOrderStatus
	// CreatedAtMs — order creation time in epoch milliseconds.
	CreatedAtMs int64
	// UpdatedAtMs — order last-update time in epoch milliseconds.
	UpdatedAtMs int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// RepayRecord — one repayment record against a multi-collateral loan.
type RepayRecord struct {
	// OrderID — the loan order id this repayment applies to.
	OrderID string
	// RecordID — the Gate repayment record id.
	RecordID string
	// RepaidAtMs — repayment time in epoch milliseconds.
	RepaidAtMs int64
	// Currency — the repaid currency.
	Currency string
	// Principal — principal repaid in this record.
	Principal decimal.Decimal
	// Interest — interest repaid in this record.
	Interest decimal.Decimal
	// RepaidAll — whether this record cleared the outstanding balance.
	RepaidAll bool
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// MortgageRecord — one collateral add/withdraw record on a multi-collateral
// loan.
type MortgageRecord struct {
	// OrderID — the loan order id this record applies to.
	OrderID string
	// OperatedAtMs — operation time in epoch milliseconds.
	OperatedAtMs int64
	// Type — the collateral operation (append / redeem).
	Type MortgageType
	// Currency — the collateral currency operated on.
	Currency string
	// Amount — the collateral amount added or withdrawn.
	Amount decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// CurrencyQuota — borrow/collateral quota for a single currency.
type CurrencyQuota struct {
	// Currency — the currency, e.g. "USDT".
	Currency string
	// Index — the Gate quota index/identifier, when present.
	Index string
	// Quota — the remaining quota amount.
	Quota decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// MultiLoanCurrency — a supported borrow or collateral currency.
type MultiLoanCurrency struct {
	// Currency — the currency, e.g. "BTC".
	Currency string
	// PrecisionAmount — the decimal precision Gate accepts for amounts.
	PrecisionAmount int64
	// MinBorrowAmount — the minimum borrowable amount (0 if unset).
	MinBorrowAmount decimal.Decimal
	// Ltv — the loan-to-value ratio applied when this currency is collateral.
	Ltv decimal.Decimal
	// LoanType — whether the currency is usable as a borrow and/or collateral.
	LoanType string
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// Ltv — current and liquidation loan-to-value ratios for a loan/account.
type Ltv struct {
	// CurrentLtv — the current loan-to-value ratio.
	CurrentLtv decimal.Decimal
	// LiquidationLtv — the ratio at which the loan is force-liquidated.
	LiquidationLtv decimal.Decimal
	// AlertLtv — the warning ratio Gate surfaces before liquidation.
	AlertLtv decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// FixedRate — fixed borrow rate for a currency over a term.
type FixedRate struct {
	// Currency — the currency, e.g. "USDT".
	Currency string
	// FixedType — the fixed-rate term bucket.
	FixedType MultiLoanFixedType
	// Rate — the fixed (daily) borrow rate.
	Rate decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// CurrentRate — current (floating) borrow rate for a currency.
type CurrentRate struct {
	// Currency — the currency, e.g. "USDT".
	Currency string
	// Rate — the current (daily) floating borrow rate.
	Rate decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// CreateOrderRequest — input for Client.CreateOrder
// (POST /loan/multi_collateral/orders).
type CreateOrderRequest struct {
	// OrderID — optional client order id to reuse/extend an existing order.
	OrderID string
	// OrderType — floating ("current") or fixed-rate borrow mode. Required.
	OrderType MultiLoanOrderType
	// FixedType — the fixed-rate term bucket (required for fixed loans).
	FixedType MultiLoanFixedType
	// FixedRate — the fixed (daily) borrow rate (fixed loans). Optional (zero
	// omitted).
	FixedRate decimal.Decimal
	// AutoRenew — whether a fixed loan should auto-renew at expiry. Optional.
	AutoRenew bool
	// AutoRepay — whether the loan should auto-repay from spot. Optional.
	AutoRepay bool
	// BorrowCurrency — the currency to borrow, e.g. "USDT". Required.
	BorrowCurrency string
	// BorrowAmount — the amount to borrow. Required, positive.
	BorrowAmount decimal.Decimal
	// CollateralCurrencies — the collateral basket backing the loan. Required,
	// non-empty.
	CollateralCurrencies []CollateralItem
}

// RepayItem — one per-currency repayment instruction for RepayRequest.
type RepayItem struct {
	// Currency — the currency to repay, e.g. "USDT". Required.
	Currency string
	// Amount — the amount to repay. Ignored when RepaidAll is true.
	Amount decimal.Decimal
	// RepaidAll — when true, repay the whole outstanding balance of Currency.
	RepaidAll bool
}

// RepayRequest — input for Client.Repay (POST /loan/multi_collateral/repay).
type RepayRequest struct {
	// OrderID — the loan order to repay. Required.
	OrderID string
	// RepayItems — the per-currency repayment instructions. Required, non-empty.
	RepayItems []RepayItem
}

// MortgageRequest — input for Client.OperateMortgage
// (POST /loan/multi_collateral/mortgage).
type MortgageRequest struct {
	// OrderID — the loan order to adjust collateral on. Required.
	OrderID string
	// Type — MortgageTypeAppend (add) or MortgageTypeRedeem (withdraw). Required.
	Type MortgageType
	// Collaterals — the collateral currencies and amounts to add/withdraw.
	// Required, non-empty.
	Collaterals []CollateralItem
}
