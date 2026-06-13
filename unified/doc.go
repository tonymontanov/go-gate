/*
Package unified implements the Gate Unified Account section (the unified /
portfolio-margin account) of the go-gate SDK.

It is named after Gate's own terminology ("unified account"). The package owns a
back reference to the root gate.Client (shared signer, REST transport, logger,
config) and exposes a single domain client — unified.Client — built on the same
internal layer (rest/auth/codec). The unified account is one logical domain
(cross-currency / portfolio margin spanning spot, futures and options), so there
is no Trading/Account/Stream split: every endpoint is a method on unified.Client.

The unified section is REST-only (no WebSocket) and is NOT settle-scoped: its
paths live under "/unified/..." (not "/unified/{settle}/..."). It covers the
unified account snapshot, borrow/repay loans, borrowable/transferable quotas,
interest and loan history, the account mode (classic / multi-currency /
portfolio / single-currency), the portfolio-margin calculator, risk units,
collateral, discount / loan-margin tiers and per-currency leverage.

Enable the section with a blank import so its factory registers with the root:

	import (
		gate "github.com/tonymontanov/go-gate/v2"
		_ "github.com/tonymontanov/go-gate/v2/unified"
	)

	client, _ := gate.NewClient(cfg)
	u := client.Unified().(*unified.Client)
	acc, err := u.GetAccount(ctx, "", 0)

Most endpoints are private (signed); only the market/config reads
GET /unified/currencies and GET /unified/currency_discount_tiers are public.

CALIBRATION: endpoint paths and request/response field shapes are modeled on
Gate's unified-account docs. Decimal fields that Gate may quote as either a JSON
number or a string (notably the UnifiedAccount balances/margins) are decoded via
codec.FlexDecimal; verify field exactness against a live unified environment.
*/
package unified
