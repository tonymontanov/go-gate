# go-gate

A high-performance Go SDK for the [Gate](https://www.gate.com/docs/developers/apiv4/en/)
exchange (REST + WebSocket), built for HFT / algorithmic trading.

`go-gate` is a **single-exchange** SDK: it exposes idiomatic, low-latency primitives
for one exchange and leaves cross-exchange unification to the caller (a trading desk).
Its public surface mirrors the sibling SDKs `go-okx` and `go-bybit`, so a desk
connector can wrap it with minimal glue.

> **Status — v1.0:** USD-M perpetual **Futures** (settle = `usdt`). Spot and the
> remaining sections land in later versions. See `docs/` for the full spec.

## Install

```bash
go get github.com/tonymontanov/go-gate/v2
```

Requires Go 1.24+. Dependencies: `gorilla/websocket`, `json-iterator/go`,
`shopspring/decimal`.

## Architecture

Three layers, composed so each trading section stays independent and testable:

```
gate.Client                       root: shared signer, REST transport, logger, config
  └─ Futures() → futures.Client   one package per Gate section (named per Gate's own terms)
       ├─ Trading()    REST: create/amend/cancel (single + batch), cancel-all, countdown
       ├─ Account()    REST: positions, leverage, position mode, close
       ├─ MarketData() REST: contracts, order book, candlesticks, tickers
       └─ Stream()     WS:  book ticker (BBO), tickers, trades, orders, positions
```

Sections register with the root via an `init()` factory (no import cycle); enable
one with a blank import. Shared transport/auth/codec live in `internal/`, and each
section calls those unified primitives with its own specifics — no copy-paste
between sections.

### Gate specifics handled for you

- **Signing:** `SIGN = hex(HMAC_SHA512(secret, METHOD\npath\nrawQuery\nSHA512(body)\nts))`.
- **No order side field:** direction is the sign of the integer contract size — the
  SDK takes an explicit `Side` + positive `Size` and encodes the sign.
- **Market orders:** `price="0"` + `tif="ioc"`.
- **Client order id:** Gate's `text` field, auto-prefixed `t-` and validated.
- **Sizes in contracts:** convert base-asset quantity ↔ contracts with the
  contract's `quanto_multiplier` (the desk connector's job).
- **Rate limits:** the SDK does not throttle; it surfaces Gate's `X-Gate-RateLimit-*`
  headers plus request metadata via `Config.RateLimitEventObserver`.

## Quick start

```go
package main

import (
	"context"
	"fmt"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/futures" // importing the section registers it
	ftypes "github.com/tonymontanov/go-gate/v2/futures/types"
	"github.com/shopspring/decimal"
)

func main() {
	client, err := gate.NewClient(gate.Config{
		APIKey:    "YOUR_KEY",
		SecretKey: "YOUR_SECRET",
		// Testnet: true,
	})
	if err != nil {
		panic(err)
	}
	defer client.Close()

	fut := client.Futures().(*futures.Client)
	ctx := context.Background()

	// Public: contract spec + best bid/ask stream.
	spec, _ := fut.MarketData().GetContract(ctx, "BTC_USDT")
	fmt.Println("quanto multiplier:", spec.QuantoMultiplier)

	_ = fut.Stream().WatchBookTicker(ctx, "BTC_USDT", func(bt ftypes.BookTicker) {
		fmt.Printf("BBO %s: %s x %s | %s x %s\n",
			bt.Contract, bt.BidPrice, bt.BidSize, bt.AskPrice, bt.AskSize)
	}, func(err error) { fmt.Println("ws err:", err) })

	// Private: place a post-only limit order (size in contracts).
	info, err := fut.Trading().CreateOrder(ctx, ftypes.CreateOrderRequest{
		Contract:    "BTC_USDT",
		Side:        ftypes.SideTypeBuy,
		Size:        decimal.NewFromInt(1),
		Price:       decimal.RequireFromString("30000"),
		TimeInForce: ftypes.TimeInForcePOC,
		Text:        "myorder1",
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("order:", info.OrderID, info.ClientOrderID)

	_ = fut.Trading().CancelOrder(ctx, ftypes.CancelOrderRequest{
		Contract: "BTC_USDT", OrderID: info.OrderID,
	})

	select {} // keep the stream alive
}
```

A runnable version is in [`examples/basic`](examples/basic).

## Errors

All errors are `*gate.Error` with a `Kind` ∈ {Network, RateLimit, Auth,
InvalidRequest, Exchange, Unknown}, the Gate `label`, and the HTTP status. Use the
predicates `gate.IsRateLimit(err)`, `gate.IsAuth(err)`, etc., or `errors.As`.

## Testing

```bash
go test ./...          # unit + contract tests (httptest fixtures, no network)
go test -race ./...    # concurrency checks (WebSocket)
```

## License

See [LICENSE](LICENSE).
