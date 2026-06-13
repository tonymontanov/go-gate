/*
Package margin implements the Gate Margin section (isolated + cross margin
lending/borrowing) of the go-gate SDK.

It is named after Gate's own terminology ("Margin"). The package owns a back
reference to the root gate.Client (shared signer, REST transport, logger, config)
and exposes two domain sub-clients — Isolated and Cross — built on the same
internal layer (rest/auth/codec). The Margin section is REST-only: it has NO
WebSocket stream.

Unlike futures/delivery, the Margin section is NOT settle-scoped: its REST paths
live under "/margin/..." (isolated) and "/margin/cross/..." (cross), not under a
"{settle}" segment.

Enable the section with a blank import so its factory registers with the root:

	import (
		gate "github.com/tonymontanov/go-gate/v2"
		_ "github.com/tonymontanov/go-gate/v2/margin"
	)

	client, _ := gate.NewClient(cfg)
	m := client.Margin().(*margin.Client)
	pairs, err := m.Isolated().ListCurrencyPairs(ctx)

GATE SPECIFICS encoded by this package:
  - the public discovery endpoints (isolated currency_pairs + funding_book, cross
    currencies) are unsigned; every account/loan/repay endpoint is signed;
  - a loan "side" is lend or borrow (Gate's explicit field) on isolated margin;
  - the isolated repay endpoint takes a mode of "all" or "partial";
  - account balances/amounts may arrive as JSON strings (REST) — decoded through
    codec.FlexDecimal so a future number form does not break the decode;
  - Gate epoch-seconds time fields are normalized to epoch milliseconds (...Ms).

CALIBRATION: endpoint paths follow Gate's margin docs; the exact request/response
field set (balances, loan fields, cross account shape) is modeled on those docs —
verify field exactness against a live margin environment.
*/
package margin
