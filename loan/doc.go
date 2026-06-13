/*
Package loan implements the Gate multi-collateral crypto Loan section of the
go-gate SDK.

A multi-collateral loan lets a user borrow one currency against a basket of
collateral currencies. The package owns a back reference to the root gate.Client
(shared signer, REST transport, logger, config) and exposes a single
loan.Client. The Loan section is REST-only: it has NO WebSocket stream.

Unlike futures/delivery, the Loan section is NOT settle-scoped: its REST paths
live under "/loan/multi_collateral/...", not under a "{settle}" segment.

Enable the section with a blank import so its factory registers with the root:

	import (
		gate "github.com/tonymontanov/go-gate/v2"
		_ "github.com/tonymontanov/go-gate/v2/loan"
	)

	client, _ := gate.NewClient(cfg)
	l := client.Loan().(*loan.Client)
	currencies, err := l.ListCurrencies(ctx, "")

GATE SPECIFICS encoded by this package:
  - the discovery endpoints (currencies, fixed_rate, current_rate) are unsigned;
    every order/repay/mortgage/quota/ltv endpoint is signed;
  - a loan order carries a basket of collateral currencies (each a
    currency+amount pair) backing a single borrow currency;
  - the mortgage endpoint adjusts collateral with an "append" (add) or "redeem"
    (withdraw) type; the repay endpoint takes per-item amounts with an optional
    repaid_all flag;
  - amount/rate/ltv fields may arrive as JSON strings (REST) — decoded through
    codec.FlexDecimal so a future number form does not break the decode;
  - Gate epoch-seconds time fields are normalized to epoch milliseconds (...Ms).

CALIBRATION: endpoint paths follow Gate's multi-collateral loan docs; the exact
request/response field set (order/collateral fields, quota, ltv, rate shapes) is
modeled on those docs — verify field exactness against a live environment.
*/
package loan
