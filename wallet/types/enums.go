/*
FILE: wallet/types/enums.go

DESCRIPTION:
Enumerated values for the Gate WALLET domain (cross-account transfers, balances,
fees). Values map directly to Gate APIv4 wire strings:

  - AccountType — the per-account "wallet" a transfer moves funds into or out of
    (spot, margin, futures, delivery, options, unified, cross_margin). Gate
    carries it as the "from"/"to" fields on POST /wallet/transfers and as the
    "sub_account_type" field on the sub-account transfer endpoints.
  - TransferDirection — the direction of a main↔sub-account transfer relative to
    the MAIN account: "to" funds the sub-account, "from" pulls funds back.
*/

package types

// AccountType — a Gate account ("wallet") a transfer moves funds into or out of.
// Wire-exact strings.
type AccountType string

const (
	// AccountTypeSpot — the spot trading account.
	AccountTypeSpot AccountType = "spot"
	// AccountTypeMargin — the isolated-margin account.
	AccountTypeMargin AccountType = "margin"
	// AccountTypeFutures — the perpetual-futures account.
	AccountTypeFutures AccountType = "futures"
	// AccountTypeDelivery — the delivery-futures account.
	AccountTypeDelivery AccountType = "delivery"
	// AccountTypeOptions — the options account.
	AccountTypeOptions AccountType = "options"
	// AccountTypeUnified — the unified account.
	AccountTypeUnified AccountType = "unified"
	// AccountTypeCrossMargin — the cross-margin account.
	AccountTypeCrossMargin AccountType = "cross_margin"
)

// TransferDirection — direction of a main↔sub-account transfer, relative to the
// MAIN account. Wire-exact strings.
type TransferDirection string

const (
	// TransferDirectionTo — move funds from the main account TO the sub-account.
	TransferDirectionTo TransferDirection = "to"
	// TransferDirectionFrom — move funds FROM the sub-account back to the main.
	TransferDirectionFrom TransferDirection = "from"
)
