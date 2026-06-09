/*
Package spot implements the Gate Spot section of the go-gate SDK.

It is named after Gate's own terminology ("Spot"). The package owns a back
reference to the root gate.Client (shared signer, REST transport, logger, config)
and exposes domain sub-clients — Trading, Account, MarketData, and Stream.

Enable the section with a blank import so its factory registers with the root:

	import (
		gate "github.com/tonymontanov/go-gate/v2"
		_ "github.com/tonymontanov/go-gate/v2/spot"
	)

	client, _ := gate.NewClient(cfg)
	sp := client.Spot().(*spot.Client)
	info, err := sp.Trading().CreateOrder(ctx, types.CreateOrderRequest{ ... })

GATE SPOT SPECIFICS encoded by this package (vs the futures section):
  - amounts are in BASE currency, not contracts; sizing uses decimal precision
    (amount_precision / precision) rather than a quanto multiplier;
  - "side" and "type" are explicit Gate fields (no signed-size convention);
  - a market BUY's "amount" is the QUOTE amount to spend;
  - orders are amended via PATCH (futures uses PUT);
  - the WS host is api.gateio.ws (futures uses fx-ws.gateio.ws); channels are
    "spot.*"; Gate has no public spot testnet;
  - the account holds per-currency balances, not positions.
*/
package spot
