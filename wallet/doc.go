/*
Package wallet implements the Gate Wallet section (cross-account transfers,
balances, and trading fees) of the go-gate SDK.

It is named after Gate's own terminology ("Wallet"). The package owns a back
reference to the root gate.Client (shared signer, REST transport, logger, config)
and exposes a single REST client whose paths live under "/wallet/...". The Wallet
section is REST-only: it has NO WebSocket stream, and it is NOT settle-scoped
(there is no "{settle}" path segment; futures/delivery reads carry "settle" only
as a query parameter).

Enable the section with a blank import so its factory registers with the root:

	import (
		gate "github.com/tonymontanov/go-gate/v2"
		_ "github.com/tonymontanov/go-gate/v2/wallet"
	)

	client, _ := gate.NewClient(cfg)
	w := client.Wallet().(*wallet.Client)
	total, err := w.GetTotalBalance(ctx, "USDT")

SCOPE: this section deliberately covers transfers, balance reads, fees, and
deposit/withdrawal HISTORY only. It does NOT create or cancel withdrawals — those
balance-moving, irreversible actions are intentionally out of scope.

GATE SPECIFICS encoded by this package:
  - every endpoint is signed EXCEPT ListCurrencyChains
    (GET /wallet/currency_chains), which is public;
  - a transfer's "from"/"to" are AccountType wire strings (spot/margin/futures/
    delivery/options/unified/cross_margin); a futures/delivery transfer also needs
    a settle currency, and a margin transfer needs a currency_pair;
  - a main↔sub transfer carries a TransferDirection ("to" funds the sub-account,
    "from" pulls funds back);
  - amounts may arrive as JSON strings (REST) — decoded through codec.FlexDecimal
    so a future number form does not break the decode;
  - Gate epoch-seconds time fields are normalized to epoch milliseconds (...Ms).

CALIBRATION: endpoint paths follow Gate's wallet docs; the exact request/response
field set (balances, fee fields, chain/withdraw-status shapes) is modeled on those
docs — verify field exactness against a live environment.
*/
package wallet
