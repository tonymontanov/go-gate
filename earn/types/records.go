/*
FILE: earn/types/records.go

DESCRIPTION:
History and status records for the Gate Earn "Uni" lending section:
  - LendRecord     — one lend/redeem action (GET /earn/uni/lend_records).
  - InterestRecord — one interest payout (GET /earn/uni/interest_records).
  - Interest       — total accrued interest for a currency (GET /earn/uni/interests/{currency}).
  - InterestStatusInfo — the auto-reinvest status for a currency
    (GET /earn/uni/interest_status/{currency}).

All epoch-second timestamps Gate sends are normalized to epoch-millisecond
...Ms fields by the earn package.

CALIBRATION: field names follow Gate's Uni docs; verify live.
*/

package types

import "github.com/shopspring/decimal"

// LendRecord — one historical lend or redeem action on the Uni pool.
type LendRecord struct {
	// Currency — the asset code, e.g. "ETH".
	Currency string
	// Amount — principal moved by this action.
	Amount decimal.Decimal
	// LastWalletAmount — wallet balance snapshot right before the action.
	LastWalletAmount decimal.Decimal
	// LastLentAmount — lent principal snapshot right before the action.
	LastLentAmount decimal.Decimal
	// LastFrozenAmount — frozen principal snapshot right before the action.
	LastFrozenAmount decimal.Decimal
	// Type — LendTypeLend or LendTypeRedeem.
	Type LendType
	// CreatedAtMs — action time in epoch milliseconds.
	CreatedAtMs int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// InterestRecord — one interest payout on a Uni lending position.
type InterestRecord struct {
	// Currency — the asset code, e.g. "ETH".
	Currency string
	// Interest — interest amount credited by this payout.
	Interest decimal.Decimal
	// Status — Gate payout status code (verbatim wire value).
	Status int64
	// ActualRate — the annualized rate actually applied for this payout.
	ActualRate decimal.Decimal
	// CreatedAtMs — payout time in epoch milliseconds.
	CreatedAtMs int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// Interest — the total accrued interest income for a single currency.
type Interest struct {
	// Currency — the asset code, e.g. "ETH".
	Currency string
	// Interest — cumulative interest income.
	Interest decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// InterestStatusInfo — the auto-reinvest (compounding) status for a currency.
type InterestStatusInfo struct {
	// Currency — the asset code, e.g. "ETH".
	Currency string
	// InterestStatus — auto-reinvest status (verbatim wire value).
	InterestStatus InterestStatus
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
