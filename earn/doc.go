/*
Package earn implements the Gate Earn "Uni" flexible-lending section of the
go-gate SDK.

It is named after Gate's own terminology (Gate Earn / "Uni" flexible lending).
The package owns a back reference to the root gate.Client (shared signer, REST
transport, logger, config) and exposes the lending domain as methods on a single
earn.Client. The section is REST-only — there is NO WebSocket.

Unlike futures/delivery, the Earn section is NOT settle-scoped: its REST paths
live under "/earn/uni/..." (not "/earn/{settle}/..."). The pool quotes an
estimated annualized rate; lenders add principal ("lend") or withdraw it
("redeem"), set a floor rate they will accept, and optionally auto-reinvest
accrued interest.

Enable the section with a blank import so its factory registers with the root:

	import (
		gate "github.com/tonymontanov/go-gate/v2"
		_ "github.com/tonymontanov/go-gate/v2/earn"
	)

	client, _ := gate.NewClient(cfg)
	e := client.Earn().(*earn.Client)
	currencies, err := e.ListCurrencies(ctx)

ENDPOINTS:
  - ListCurrencies      GET   /earn/uni/currencies                 (public)
  - GetCurrency         GET   /earn/uni/currencies/{currency}      (public)
  - ListUserLends       GET   /earn/uni/lends                      (signed)
  - CreateLend          POST  /earn/uni/lends                      (signed)
  - ChangeLend          PATCH /earn/uni/lends                      (signed)
  - ListLendRecords     GET   /earn/uni/lend_records               (signed)
  - GetInterest         GET   /earn/uni/interests/{currency}       (signed)
  - ListInterestRecords GET   /earn/uni/interest_records           (signed)
  - GetInterestStatus   GET   /earn/uni/interest_status/{currency} (signed)
  - ListChart           GET   /earn/uni/chart                      (public)
  - ListRate            GET   /earn/uni/rate                       (public)

GATE WIRE NOTES encoded by this package:
  - amounts and rates may arrive as JSON numbers OR quoted strings depending on
    the endpoint; the wire payloads use codec.FlexDecimal to tolerate both;
  - epoch-second timestamps (create_time/update_time/chart time) are normalized
    to epoch-millisecond ...Ms fields.

CALIBRATION: endpoint paths and request/response field names are modeled on
Gate's Uni flexible-lending docs; verify field exactness against a live account.
*/
package earn
