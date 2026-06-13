/*
FILE: margin/types/cross.go

DESCRIPTION:
Structs for the Gate CROSS-margin section:

  - CrossCurrency        — a currency tradable on cross margin and its
    borrow limits/loanability (GET /margin/cross/currencies[/{currency}]).
  - CrossAccount         — the single cross-margin account with its per-currency
    balance breakdown (GET /margin/cross/accounts).
  - CrossBalance         — one currency's balance within a CrossAccount.
  - CrossLoan            — a cross-margin loan (GET/POST /margin/cross/loans,
    GET /margin/cross/loans/{loan_id}).
  - CrossRepayment       — one cross-margin repayment record
    (GET /margin/cross/repayments).
  - CreateCrossLoanRequest / CrossRepayRequest — request inputs.

Unlike isolated margin, cross margin is NOT pair-scoped: a single account
collateralizes borrowing across every supported currency.
*/

package types

import "github.com/shopspring/decimal"

// CrossCurrency — normalized cross-margin currency spec.
type CrossCurrency struct {
	// Name — the currency, e.g. "BTC".
	Name string
	// Rate — the (daily) borrow interest rate.
	Rate decimal.Decimal
	// Precision — the currency decimal precision Gate reports.
	Precision decimal.Decimal
	// Discount — the collateral discount factor applied to the currency.
	Discount decimal.Decimal
	// MinBorrowAmount — minimum borrowable amount per order.
	MinBorrowAmount decimal.Decimal
	// UserMaxBorrowAmount — maximum the user may borrow of this currency.
	UserMaxBorrowAmount decimal.Decimal
	// TotalMaxBorrowAmount — platform-wide borrow cap for this currency.
	TotalMaxBorrowAmount decimal.Decimal
	// Price — the currency's reference price in the settlement unit.
	Price decimal.Decimal
	// Loanable — whether the currency can currently be borrowed.
	Loanable bool
	// Status — Gate currency status, surfaced as-is.
	Status int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// CrossBalance — one currency's balance within the cross-margin account.
type CrossBalance struct {
	// Currency — the balance currency, e.g. "USDT".
	Currency string
	// Available — balance available for trading/withdrawal.
	Available decimal.Decimal
	// Freeze — balance frozen by open orders.
	Freeze decimal.Decimal
	// Borrowed — outstanding borrowed principal in this currency.
	Borrowed decimal.Decimal
	// Interest — accrued, unpaid interest in this currency.
	Interest decimal.Decimal
}

// CrossAccount — normalized cross-margin account (single, multi-currency).
type CrossAccount struct {
	// UserID — the Gate user id owning the account.
	UserID int64
	// Locked — whether the account is locked (e.g. during liquidation).
	Locked bool
	// Total — total account value in the settlement unit.
	Total decimal.Decimal
	// Borrowed — total borrowed value in the settlement unit.
	Borrowed decimal.Decimal
	// Interest — total accrued interest in the settlement unit.
	Interest decimal.Decimal
	// Risk — the account risk ratio.
	Risk decimal.Decimal
	// TotalInitialMargin — total initial margin requirement.
	TotalInitialMargin decimal.Decimal
	// TotalMarginBalance — total margin balance.
	TotalMarginBalance decimal.Decimal
	// TotalMaintenanceMargin — total maintenance margin requirement.
	TotalMaintenanceMargin decimal.Decimal
	// Balances — per-currency balance breakdown, keyed by currency.
	Balances map[string]CrossBalance
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// CrossLoan — normalized cross-margin loan.
type CrossLoan struct {
	// ID — the Gate loan id.
	ID string
	// CreatedAtMs — loan creation time in epoch milliseconds.
	CreatedAtMs int64
	// UpdatedAtMs — last update time in epoch milliseconds.
	UpdatedAtMs int64
	// Currency — the borrowed currency, e.g. "BTC".
	Currency string
	// Amount — the borrowed principal amount.
	Amount decimal.Decimal
	// Text — the client text/label attached to the loan, when present.
	Text string
	// Status — loan lifecycle state.
	Status LoanStatus
	// Repaid — principal already repaid.
	Repaid decimal.Decimal
	// RepaidInterest — interest already repaid.
	RepaidInterest decimal.Decimal
	// UnpaidInterest — interest accrued but not yet paid.
	UnpaidInterest decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// CrossRepayment — one cross-margin repayment record.
type CrossRepayment struct {
	// ID — the Gate repayment id.
	ID string
	// CreatedAtMs — repayment time in epoch milliseconds.
	CreatedAtMs int64
	// LoanID — the loan this repayment applies to.
	LoanID string
	// Currency — the repaid currency.
	Currency string
	// Principal — principal repaid in this record.
	Principal decimal.Decimal
	// Interest — interest repaid in this record.
	Interest decimal.Decimal
	// RepaymentType — Gate repayment type (e.g. "" manual / "auto"), surfaced as-is.
	RepaymentType string
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// CreateCrossLoanRequest — input for CrossClient.CreateLoan
// (POST /margin/cross/loans).
type CreateCrossLoanRequest struct {
	// Currency — the currency to borrow, e.g. "BTC". Required.
	Currency string
	// Amount — the principal amount to borrow. Required, positive.
	Amount decimal.Decimal
	// Text — an optional client text/label for the loan.
	Text string
}

// CrossRepayRequest — input for CrossClient.Repay (POST /margin/cross/repayments).
type CrossRepayRequest struct {
	// Currency — the currency to repay, e.g. "BTC". Required.
	Currency string
	// Amount — the amount to repay. Required, positive.
	Amount decimal.Decimal
}
