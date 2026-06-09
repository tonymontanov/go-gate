# go-gate

A high-performance Go SDK for the [Gate](https://www.gate.com/docs/developers/apiv4/en/)
exchange (REST + WebSocket), built for HFT / algorithmic trading.

`go-gate` is a **single-exchange** SDK: it exposes idiomatic, low-latency primitives
for one exchange and leaves cross-exchange unification to the caller (a trading desk).
Its public surface mirrors the sibling SDKs `go-okx` and `go-bybit`, so a desk
connector can wrap it with minimal glue.

> **Status:** USD-M perpetual **Futures** (settle = `usdt`, v1.0) and **Spot**
> (v2.0) are implemented. The remaining sections land in later versions. See
> `docs/` for the full spec.

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
  ├─ Futures() → futures.Client   one package per Gate section (named per Gate's own terms)
  │    ├─ Trading()    REST: create/amend/cancel (single + batch), cancel-all, countdown
  │    ├─ Account()    REST: positions, leverage, position mode, close
  │    ├─ MarketData() REST: contracts, order book, candlesticks, tickers
  │    └─ Stream()     WS:  book ticker (BBO), order book (L2), tickers, trades, orders, positions
  └─ Spot() → spot.Client         Gate Spot (currency pairs, base amounts)
       ├─ Trading()    REST: create/amend(PATCH)/cancel (single + batch), cancel-all, countdown
       ├─ Account()    REST: per-currency balances (available/locked)
       ├─ MarketData() REST: currency pairs, order book, candlesticks, tickers
       └─ Stream()     WS:  book ticker (BBO), order book (L2), tickers, trades, orders, usertrades, balances

A shared `orderbook/` package maintains a local L2 book per symbol from a REST
snapshot plus the incremental `*.order_book_update` deltas (sequence-gap detection
and automatic resync), exposed as `Stream().WatchOrderBook(...)` on both sections.
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

### Spot

The spot section mirrors the same shape; enable it with a blank import of
`github.com/tonymontanov/go-gate/v2/spot` and use `client.Spot().(*spot.Client)`.
Spot conventions differ from futures: amounts are in **base currency** (a market
BUY's amount is the **quote** amount), `Side`/`OrderType` are explicit, amend is a
`PATCH`, and the account exposes per-currency balances instead of positions.

```go
sp := client.Spot().(*spot.Client)
ord, _ := sp.Trading().CreateOrder(ctx, stypes.CreateOrderRequest{
	CurrencyPair: "BTC_USDT",
	Side:         stypes.SideTypeBuy,
	Amount:       decimal.RequireFromString("0.01"), // base currency
	Price:        decimal.RequireFromString("30000"),
	TimeInForce:  stypes.TimeInForcePOC,
})
```

A runnable version is in [`examples/spot`](examples/spot).

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
