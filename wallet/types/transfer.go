/*
FILE: wallet/types/transfer.go

DESCRIPTION:
Transfer request inputs and result structs for the Gate WALLET section:

  - TransferRequest            — input for Client.Transfer (between the caller's
    OWN accounts, POST /wallet/transfers).
  - TransferResult             — the result Gate returns for a transfer (a
    transaction id and/or order id, when present).
  - SubAccountTransferRequest  — input for Client.TransferWithSubAccount (main↔sub,
    POST /wallet/sub_account_transfers).
  - SubToSubTransferRequest    — input for Client.SubAccountToSubAccount
    (sub→sub, POST /wallet/sub_account_to_sub_account).
  - SubAccountTransferRecord   — one row of the main↔sub transfer history
    (GET /wallet/sub_account_transfers).

Monetary fields use decimal.Decimal; time fields are normalized to epoch
milliseconds (...Ms).
*/

package types

import "github.com/shopspring/decimal"

// TransferRequest — input for Client.Transfer, moving funds between the caller's
// own accounts (POST /wallet/transfers).
type TransferRequest struct {
	// Currency — the currency to move, e.g. "USDT". Required.
	Currency string
	// From — the source account. Required.
	From AccountType
	// To — the destination account. Required.
	To AccountType
	// Amount — the amount to transfer. Required, positive.
	Amount decimal.Decimal
	// CurrencyPair — the isolated-margin pair, required by Gate when From or To
	// is AccountTypeMargin. Optional otherwise.
	CurrencyPair string
	// Settle — the settle currency, required by Gate when From or To is
	// AccountTypeFutures or AccountTypeDelivery (e.g. "usdt", "btc"). Optional
	// otherwise.
	Settle string
}

// TransferResult — result of a wallet transfer. Gate returns a transaction id
// (and, for some account transfers, an order id); either may be empty.
type TransferResult struct {
	// TxID — the Gate transfer transaction id, when returned.
	TxID string
	// OrderID — the Gate transfer order id, when returned.
	OrderID string
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// SubAccountTransferRequest — input for Client.TransferWithSubAccount, moving
// funds between the main account and one of its sub-accounts
// (POST /wallet/sub_account_transfers).
type SubAccountTransferRequest struct {
	// Currency — the currency to move, e.g. "USDT". Required.
	Currency string
	// SubAccount — the sub-account user id. Required.
	SubAccount string
	// Direction — TransferDirectionTo (main→sub) or TransferDirectionFrom
	// (sub→main). Required.
	Direction TransferDirection
	// Amount — the amount to transfer. Required, positive.
	Amount decimal.Decimal
	// SubAccountType — the sub-account wallet to move into/out of (e.g.
	// AccountTypeSpot). Optional (Gate defaults to spot).
	SubAccountType AccountType
}

// SubToSubTransferRequest — input for Client.SubAccountToSubAccount, moving funds
// directly between two sub-accounts (POST /wallet/sub_account_to_sub_account).
type SubToSubTransferRequest struct {
	// Currency — the currency to move, e.g. "USDT". Required.
	Currency string
	// SubAccountFrom — the source sub-account user id. Required.
	SubAccountFrom string
	// SubAccountFromType — the source sub-account wallet (e.g. AccountTypeSpot).
	// Optional (Gate defaults to spot).
	SubAccountFromType AccountType
	// SubAccountTo — the destination sub-account user id. Required.
	SubAccountTo string
	// SubAccountToType — the destination sub-account wallet. Optional.
	SubAccountToType AccountType
	// Amount — the amount to transfer. Required, positive.
	Amount decimal.Decimal
}

// SubAccountTransferRecord — one row of the main↔sub transfer history
// (GET /wallet/sub_account_transfers).
type SubAccountTransferRecord struct {
	// Currency — the transferred currency.
	Currency string
	// SubAccount — the sub-account user id involved.
	SubAccount string
	// Direction — main→sub ("to") or sub→main ("from").
	Direction TransferDirection
	// Amount — the transferred amount.
	Amount decimal.Decimal
	// UID — the main account user id.
	UID string
	// ClientOrderID — the client-supplied transfer id, when present.
	ClientOrderID string
	// TimeMs — the transfer time in epoch milliseconds.
	TimeMs int64
	// Source — the Gate-reported transfer source (e.g. "web", "api").
	Source string
	// SubAccountType — the sub-account wallet involved.
	SubAccountType AccountType
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
