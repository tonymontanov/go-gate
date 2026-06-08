/*
Package futures implements the Gate USD-M perpetual futures section (settle=usdt
in v1.0) of the go-gate SDK.

It is named after Gate's own terminology ("Futures"). The package owns a back
reference to the root gate.Client (shared signer, REST transport, logger, config)
and exposes domain sub-clients — Trading in v1.0, with Account/MarketData/Stream
added in later milestones.

Enable the section with a blank import so its factory registers with the root:

	import (
		gate "github.com/tonymontanov/go-gate/v2"
		_ "github.com/tonymontanov/go-gate/v2/futures"
	)

	client, _ := gate.NewClient(cfg)
	fut := client.Futures().(*futures.Client)
	info, err := fut.Trading().CreateOrder(ctx, types.CreateOrderRequest{ ... })

GATE SPECIFICS encoded by this package:
  - order direction is the sign of the integer contract size (no side field);
  - market orders are price="0" + tif="ioc";
  - the client order id ("text") is auto-prefixed "t-" and validated;
  - cancel-all and batch cancel use Gate's native endpoints.
*/
package futures
