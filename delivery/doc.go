/*
Package delivery implements the Gate Delivery section (dated/quarterly USD-M
futures, settle=usdt) of the go-gate SDK.

It is named after Gate's own terminology ("Delivery"). The package owns a back
reference to the root gate.Client (shared signer, REST transport, logger, config)
and exposes domain sub-clients — Trading, Account, MarketData, Stream — built on
the same internal layer (rest/ws/auth/codec) and the shared orderbook engine.

Delivery mirrors the perpetual Futures API but lives under "/delivery/{settle}/"
and trades DATED contracts whose name encodes the expiry, e.g.
"BTC_USDT_20240329" (ASSET_SETTLE_YYYYMMDD). The contracts settle at expiry, so
there is no funding rate; the section adds expiry/cycle on the contract spec and
a GetSettlements query for past settlements.

Enable the section with a blank import so its factory registers with the root:

	import (
		gate "github.com/tonymontanov/go-gate/v2"
		_ "github.com/tonymontanov/go-gate/v2/delivery"
	)

	client, _ := gate.NewClient(cfg)
	d := client.Delivery().(*delivery.Client)
	info, err := d.Trading().CreateOrder(ctx, types.CreateOrderRequest{ ... })

GATE SPECIFICS encoded by this package:
  - order direction is the sign of the integer contract size (no side field);
  - market orders are price="0" + tif="ioc";
  - the client order id ("text") is auto-prefixed "t-" and validated;
  - cancel-all and batch cancel use Gate's native endpoints.

CALIBRATION: endpoint/request/response shapes and the delivery WS host + channel
names are modeled on the futures section and Gate's delivery docs; verify field
exactness against a live delivery environment.
*/
package delivery
