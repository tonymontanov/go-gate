/*
FILE: margin/types/loan.go

DESCRIPTION:
Isolated-margin LOAN structs and their request inputs:

  - MarginLoan        — a margin loan order, lend or borrow (GET/POST /margin/loans,
    GET /margin/loans/{loan_id}).
  - LoanRepayment     — one repayment record against a loan
    (GET /margin/loans/{loan_id}/repayment).
  - LoanRecord        — a per-fill loan record / sub-loan
    (GET /margin/loan_records[/{loan_record_id}]).
  - CreateLoanRequest — input for IsolatedClient.CreateLoan.
  - RepayLoanRequest  — input for IsolatedClient.RepayLoan.

Monetary fields use decimal.Decimal; time fields are normalized to epoch
milliseconds (...Ms).
*/

package types

import "github.com/shopspring/decimal"

// MarginLoan — normalized isolated-margin loan order.
type MarginLoan struct {
	// ID — the Gate loan id.
	ID string
	// CreatedAtMs — loan creation time in epoch milliseconds.
	CreatedAtMs int64
	// ExpiresAtMs — loan expiry time in epoch milliseconds (0 if none).
	ExpiresAtMs int64
	// Side — lend or borrow.
	Side LoanSide
	// Status — loan lifecycle state.
	Status LoanStatus
	// Currency — the loaned currency, e.g. "USDT".
	Currency string
	// CurrencyPair — the isolated pair the loan is scoped to, e.g. "BTC_USDT".
	CurrencyPair string
	// Rate — the (daily) interest rate.
	Rate decimal.Decimal
	// Amount — the loan principal amount.
	Amount decimal.Decimal
	// Days — the loan term in days.
	Days int64
	// AutoRenew — whether the loan auto-renews at expiry.
	AutoRenew bool
	// Left — principal still outstanding / unfilled.
	Left decimal.Decimal
	// Repaid — principal already repaid.
	Repaid decimal.Decimal
	// PaidInterest — interest already paid.
	PaidInterest decimal.Decimal
	// UnpaidInterest — interest accrued but not yet paid.
	UnpaidInterest decimal.Decimal
	// FeeRate — the lending fee rate (lend side).
	FeeRate decimal.Decimal
	// OriginalID — the parent loan id when this loan was created by a renewal.
	OriginalID string
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// LoanRepayment — one repayment record against an isolated-margin loan.
type LoanRepayment struct {
	// ID — the Gate repayment id.
	ID string
	// CreatedAtMs — repayment time in epoch milliseconds.
	CreatedAtMs int64
	// Principal — principal repaid in this record.
	Principal decimal.Decimal
	// Interest — interest repaid in this record.
	Interest decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// LoanRecord — a per-fill loan record (sub-loan) of an isolated-margin loan.
type LoanRecord struct {
	// ID — the Gate loan-record id.
	ID string
	// LoanID — the parent loan id.
	LoanID string
	// CreatedAtMs — record creation time in epoch milliseconds.
	CreatedAtMs int64
	// ExpiresAtMs — record expiry time in epoch milliseconds (0 if none).
	ExpiresAtMs int64
	// Status — record lifecycle state.
	Status LoanStatus
	// BorrowUserID — the counterparty (borrower) user id, when present.
	BorrowUserID int64
	// Currency — the loaned currency.
	Currency string
	// CurrencyPair — the isolated pair the record is scoped to.
	CurrencyPair string
	// Rate — the (daily) interest rate.
	Rate decimal.Decimal
	// Amount — the record principal amount.
	Amount decimal.Decimal
	// Days — the term in days.
	Days int64
	// AutoRenew — whether the record auto-renews at expiry.
	AutoRenew bool
	// Repaid — principal already repaid on this record.
	Repaid decimal.Decimal
	// PaidInterest — interest already paid on this record.
	PaidInterest decimal.Decimal
	// UnpaidInterest — interest accrued but not yet paid on this record.
	UnpaidInterest decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// CreateLoanRequest — input for IsolatedClient.CreateLoan (POST /margin/loans).
type CreateLoanRequest struct {
	// Side — lend or borrow. Required.
	Side LoanSide
	// Currency — the currency to lend/borrow, e.g. "USDT". Required.
	Currency string
	// Amount — the principal amount. Required, positive.
	Amount decimal.Decimal
	// Rate — the (daily) interest rate. Optional; Gate uses the market rate when
	// omitted (zero).
	Rate decimal.Decimal
	// Days — the loan term in days. Optional (0 → Gate default).
	Days int64
	// AutoRenew — whether the loan should auto-renew at expiry. Optional.
	AutoRenew bool
	// CurrencyPair — the isolated pair to scope the loan to, e.g. "BTC_USDT".
	// Optional for lend, required by Gate for borrow.
	CurrencyPair string
	// FeeRate — the lending fee rate (lend side). Optional (zero omitted).
	FeeRate decimal.Decimal
}

// RepayLoanRequest — input for IsolatedClient.RepayLoan
// (POST /margin/loans/{loan_id}/repayment).
type RepayLoanRequest struct {
	// LoanID — the loan to repay. Required.
	LoanID string
	// CurrencyPair — the isolated pair the loan is scoped to. Required by Gate.
	CurrencyPair string
	// Currency — the loan currency. Optional, depending on the loan.
	Currency string
	// Mode — RepayModeAll (whole balance) or RepayModePartial (Amount). Required.
	Mode RepayMode
	// Amount — the amount to repay when Mode is partial. Ignored for "all".
	Amount decimal.Decimal
}
