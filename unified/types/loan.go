/*
FILE: unified/types/loan.go

DESCRIPTION:
Loan-related domain types for the unified account: the outstanding UnifiedLoan
(GET /unified/loans), the borrow/repay LoanRecord history
(GET /unified/loan_records), the InterestRecord history
(GET /unified/interest_records), and the CreateLoanRequest used to borrow/repay
(POST /unified/loans). The LoanType enum carries the borrow/repay direction.

CALIBRATION: field sets follow Gate's unified-account docs; verify live.
*/

package types

import "github.com/shopspring/decimal"

// LoanType — the direction of a unified loan operation.
type LoanType string

const (
	// LoanTypeBorrow — borrow currency into the unified account.
	LoanTypeBorrow LoanType = "borrow"
	// LoanTypeRepay — repay borrowed currency.
	LoanTypeRepay LoanType = "repay"
)

// UnifiedLoan — one outstanding loan in the unified account.
type UnifiedLoan struct {
	// Currency — the borrowed currency, e.g. "USDT".
	Currency string
	// CurrencyPair — the associated currency pair, when applicable.
	CurrencyPair string
	// Amount — the outstanding principal amount.
	Amount decimal.Decimal
	// Type — the loan type/category (Gate "type", e.g. "platform").
	Type string
	// CreateTimeMs — loan creation time in epoch milliseconds.
	CreateTimeMs int64
	// UpdateTimeMs — last update time in epoch milliseconds.
	UpdateTimeMs int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// LoanRecord — one borrow/repay history entry.
type LoanRecord struct {
	// ID — the record id, when present.
	ID string
	// Type — "borrow" or "repay".
	Type LoanType
	// Currency — the affected currency.
	Currency string
	// Amount — the borrowed/repaid amount.
	Amount decimal.Decimal
	// CreateTimeMs — record time in epoch milliseconds.
	CreateTimeMs int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// InterestRecord — one interest-accrual history entry.
type InterestRecord struct {
	// Currency — the currency interest was charged in.
	Currency string
	// CurrencyPair — the associated currency pair, when applicable.
	CurrencyPair string
	// ActualRate — the actual interest rate applied.
	ActualRate decimal.Decimal
	// Interest — the interest amount charged.
	Interest decimal.Decimal
	// Status — Gate status code for the record.
	Status int64
	// Type — the interest type/category (Gate "type").
	Type string
	// CreateTimeMs — record time in epoch milliseconds.
	CreateTimeMs int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// CreateLoanRequest — body for POST /unified/loans (borrow or repay).
type CreateLoanRequest struct {
	// Currency — the currency to borrow/repay (required).
	Currency string
	// Amount — the amount to borrow/repay (required unless RepaidAll).
	Amount decimal.Decimal
	// Type — "borrow" or "repay" (required).
	Type LoanType
	// RepaidAll — when true (repay only), repays the full outstanding balance;
	// Amount is then ignored by Gate.
	RepaidAll bool
}
