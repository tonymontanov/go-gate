/*
FILE: wallet/types/record.go

DESCRIPTION:
Deposit/withdrawal history and withdrawal-limit structs for the Gate WALLET
section:

  - DepositRecord    — one row of the deposit history (GET /wallet/deposits).
  - WithdrawalRecord — one row of the withdrawal history (GET /wallet/withdrawals).
  - WithdrawStatus   — the per-currency withdrawal fees and limits
    (GET /wallet/withdraw_status).

This SDK section is READ-ONLY: it lists deposits/withdrawals but never creates or
cancels a withdrawal. Monetary fields use decimal.Decimal; time fields are
normalized to epoch milliseconds (...Ms).
*/

package types

import "github.com/shopspring/decimal"

// DepositRecord — one row of the deposit history (GET /wallet/deposits).
type DepositRecord struct {
	// ID — the Gate deposit record id.
	ID string
	// TxID — the on-chain transaction hash.
	TxID string
	// Currency — the deposited currency.
	Currency string
	// Chain — the chain the deposit arrived on, e.g. "TRX".
	Chain string
	// Address — the destination deposit address.
	Address string
	// Amount — the deposited amount.
	Amount decimal.Decimal
	// Status — the Gate deposit status (e.g. "DONE", "PEND").
	Status string
	// TimeMs — the deposit time in epoch milliseconds.
	TimeMs int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// WithdrawalRecord — one row of the withdrawal history (GET /wallet/withdrawals).
type WithdrawalRecord struct {
	// ID — the Gate withdrawal record id.
	ID string
	// TxID — the on-chain transaction hash, when broadcast.
	TxID string
	// WithdrawOrderID — the client-supplied withdrawal order id, when present.
	WithdrawOrderID string
	// Currency — the withdrawn currency.
	Currency string
	// Chain — the chain the withdrawal was sent on, e.g. "TRX".
	Chain string
	// Address — the destination address.
	Address string
	// Memo — the destination memo/tag, when applicable.
	Memo string
	// Amount — the withdrawn amount.
	Amount decimal.Decimal
	// Fee — the withdrawal fee charged.
	Fee decimal.Decimal
	// Status — the Gate withdrawal status (e.g. "DONE", "CANCEL").
	Status string
	// TimeMs — the withdrawal time in epoch milliseconds.
	TimeMs int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// WithdrawStatus — per-currency withdrawal fees and limits
// (GET /wallet/withdraw_status).
type WithdrawStatus struct {
	// Currency — the currency the status describes.
	Currency string
	// Name — the currency display name.
	Name string
	// WithdrawFix — the fixed withdrawal fee (default chain).
	WithdrawFix decimal.Decimal
	// WithdrawPercent — the percentage withdrawal fee, as Gate reports it
	// (e.g. "0%").
	WithdrawPercent string
	// WithdrawDayLimit — the per-day withdrawal limit.
	WithdrawDayLimit decimal.Decimal
	// WithdrawDayLimitRemain — the remaining per-day withdrawal limit.
	WithdrawDayLimitRemain decimal.Decimal
	// WithdrawAmountMini — the minimum withdrawal amount.
	WithdrawAmountMini decimal.Decimal
	// WithdrawEachtimeLimit — the per-transaction withdrawal limit.
	WithdrawEachtimeLimit decimal.Decimal
	// WithdrawFixOnChains — the fixed withdrawal fee per chain (e.g. "TRX" → 1).
	WithdrawFixOnChains map[string]decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
