/*
Package options implements the Gate Options section (European-style crypto
options) of the go-gate SDK.

It is named after Gate's own terminology ("Options"). The package owns a back
reference to the root gate.Client (shared signer, REST transport, logger, config)
and exposes domain sub-clients — Trading, Account, MarketData, Stream — built on
the same internal layer (rest/ws/auth/codec) and the shared orderbook engine.

Unlike futures/delivery, the Options section is NOT settle-scoped: its REST paths
live under "/options/..." (not "/options/{settle}/..."). Options are written on an
UNDERLYING index (e.g. "BTC_USDT") and a contract name encodes the underlying,
expiry and strike (e.g. "BTC_USDT-20240329-50000-C"). The section adds
underlying/expiration discovery, an implied-volatility + greeks ticker, public and
private settlement history, and Market-Maker Protection (MMP).

Enable the section with a blank import so its factory registers with the root:

	import (
		gate "github.com/tonymontanov/go-gate/v2"
		_ "github.com/tonymontanov/go-gate/v2/options"
	)

	client, _ := gate.NewClient(cfg)
	o := client.Options().(*options.Client)
	info, err := o.Trading().CreateOrder(ctx, types.CreateOrderRequest{ ... })

GATE SPECIFICS encoded by this package:
  - order direction is the sign of the integer contract size (no side field),
    exactly like futures;
  - market orders are price="0" + tif="ioc";
  - the client order id ("text") is auto-prefixed "t-" and validated;
  - Gate options has NO batch order endpoints (no batch create/amend/cancel);
  - cancel-all uses Gate's native DELETE /options/orders (scoped by
    contract/underlying/side).

CALIBRATION: endpoint/request/response shapes and the options WS host + channel
names are modeled on Gate's options docs (host wss://op-ws.gateio.live/v4/ws,
channels in the "options.*" namespace); verify field exactness against a live
options environment.
*/
package options
