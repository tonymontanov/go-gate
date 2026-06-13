/*
Package flashswap implements the Gate Flash Swap section of the go-gate SDK.

Flash Swap is Gate's instant currency-conversion product: a user previews a
conversion between a sell currency and a buy currency, then creates an order
against that preview to execute the swap at the quoted price. The package owns a
back reference to the root gate.Client (shared signer, REST transport, logger,
config) and exposes a single flashswap.Client. The Flash Swap section is
REST-only: it has NO WebSocket stream.

Unlike futures/delivery, the Flash Swap section is NOT settle-scoped: its REST
paths live under "/flash_swap/...", not under a "{settle}" segment.

Enable the section with a blank import so its factory registers with the root:

	import (
		gate "github.com/tonymontanov/go-gate/v2"
		_ "github.com/tonymontanov/go-gate/v2/flashswap"
	)

	client, _ := gate.NewClient(cfg)
	fs := client.FlashSwap().(*flashswap.Client)
	currencies, err := fs.ListCurrencies(ctx)

GATE SPECIFICS encoded by this package:
  - the currency-discovery endpoint (GET /flash_swap/currencies) is unsigned;
    the preview/create/list/get order endpoints are signed;
  - a preview must be created first (PreviewOrder) to obtain a preview_id, which
    is then passed to CreateOrder to execute the swap;
  - exactly one of sell_amount / buy_amount is supplied to PreviewOrder; Gate
    computes the other side and the price;
  - amount/price fields may arrive as JSON strings (REST) — decoded through
    codec.FlexDecimal so a future number form does not break the decode;
  - Gate epoch-seconds time fields are normalized to epoch milliseconds (...Ms).

CALIBRATION: endpoint paths follow Gate's flash-swap docs; the exact
request/response field set (currency limits, order/preview fields, status codes)
is modeled on those docs — verify field exactness against a live environment.
*/
package flashswap
