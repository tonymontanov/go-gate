/*
FILE: wallet/types/chain.go

DESCRIPTION:
CurrencyChain — a single deposit/withdraw chain supported for a currency
(GET /wallet/currency_chains). This PUBLIC endpoint tells callers which chains a
currency can be moved on and whether deposits/withdrawals are currently disabled
on each.

CALIBRATION: the field set follows Gate's wallet docs; verify live.
*/

package types

// CurrencyChain — a normalized deposit/withdraw chain for a currency.
type CurrencyChain struct {
	// Chain — the chain id, e.g. "ETH", "TRX", "BTC".
	Chain string
	// NameCN — the Chinese display name of the chain (Gate "name_cn").
	NameCN string
	// NameEN — the English display name of the chain (Gate "name_en").
	NameEN string
	// ContractAddress — the on-chain token contract address, when applicable.
	ContractAddress string
	// AddrRegex — a regular expression a destination address must match.
	AddrRegex string
	// Disabled — whether the chain is fully disabled (Gate "is_disabled").
	Disabled bool
	// DepositDisabled — whether deposits are disabled on the chain
	// (Gate "is_deposit_disabled").
	DepositDisabled bool
	// WithdrawDisabled — whether withdrawals are disabled on the chain
	// (Gate "is_withdraw_disabled").
	WithdrawDisabled bool
	// Decimals — the chain's decimal precision (Gate "decimal").
	Decimals string
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}
