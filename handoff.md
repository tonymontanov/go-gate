# go-gate — Handoff

Context bridge across sessions. Update after every significant change.

## Role & stack
High-performance Go SDK for the **Gate** exchange (REST + WebSocket) for an in-house HFT desk.
Single-exchange SDK; cross-exchange unification lives in `sleipnir/core`, not here.
- Go 1.24. Module `github.com/tonymontanov/go-gate/v2`.
- Deps: `gorilla/websocket`, `json-iterator/go`, `shopspring/decimal` (same as sibling SDKs go-okx/go-bybit).
- Style: English comments/GoDoc; `camelCase`; explicit `var` declarations with types; no allocations on hot paths.

## Architecture
Three-layer composition (mirrors go-okx):
- **Root** `gate` package: `Client` (shared signer/rest/logger/config), `Config`, error/logger re-exports,
  `RateLimitEvent`. Sections registered via `RegisterFuturesFactory` in their `init()`; accessed lazily
  via `client.Futures().(*futures.Client)` (returns `any` to avoid an import cycle).
- **Internal** (`internal/*`): `auth` (HMAC-SHA512 hex signer), `rest` (transport, no-envelope Gate
  responses, error/label mapping, rate-header collection), `codec` (json-iterator + decimal helpers),
  `gateerr` (Error/ErrorKind/MapLabel/MapHTTPStatus), `gatelog` (Logger).
- **Sections** (per Gate terminology): `futures/` (USD-M perp, settle=usdt) — NOT YET CREATED. Later:
  `spot/`, `delivery/`, `options/`. Each gets `client.go` + `trading.go`/`account.go`/`market.go`/`stream.go` + `types/`.

### Gate-specific facts baked in
- REST base `https://api.gateio.ws/api/v4` (testnet `fx-api-testnet`). WS futures `wss://fx-ws.gateio.ws/v4/ws/usdt`.
- Signing: `SIGN = hex(HMAC_SHA512(secret, "METHOD\npath\nrawQuery\nhex(sha512(body))\nts"))`,
  headers `KEY`/`SIGN`/`Timestamp` (Unix **seconds**). rawQuery signed **unescaped**.
- No `{code,msg,data}` envelope: success body is the resource; errors are `{label,message,detail}` with HTTP status.
- Order model: NO side field — direction = sign of integer `size` (contracts); market = `price="0"`+`tif=ioc`;
  clientOrderId = `text` (must be `t-`-prefixed, ≤28 chars after prefix); tif gtc/ioc/poc/fok.
- Gate DOES return rate headers `X-Gate-RateLimit-*` → collected and passed to `RateLimitEventObserver`.

## Roadmap
- ✅ **M1** — module + internal scaffolding (auth/rest/codec/gateerr/gatelog) + root package
  (client/config/errors/logger/rate-limit-event). Build/vet/gofmt clean; unit + transport tests green.
- ✅ **M2** — `futures/` section: `Client` + factory (`gate_futures` terminology), `types/`
  (CreateOrderRequest/ModifyOrderRequest/CancelOrderRequest/OrderInfo/enums), `TradingClient`
  (CreateOrder, CreateBatchOrders, ModifyOrder, ModifyBatchOrders [seq], CancelOrder, CancelBatchOrders
  [native], CancelAllOrders [native DELETE ?contract=], CancelForgottenOrders, CountdownCancelAll,
  GetOrder, GetOpenOrders) + signed-size/market/text encoding + ID-mapping + contract & validation tests.
  Build/vet/gofmt clean, tests green. NOTE: native `batch_amend_orders` deferred (size/amount field
  naming needs fixture calibration) — ModifyBatchOrders loops single amends meanwhile.
- ✅ **M3** — futures Account (GetPositions/GetPosition/SetLeverage/SetPositionMode/ClosePosition) +
  Market-data REST (GetContracts/GetContract→SymbolInfo w/ quanto_multiplier, GetOrderBook[with_id],
  GetCandlesticks, GetTickers) + types (PositionInfo/SymbolInfo/OrderBook/Candle/Ticker). Account/Market
  wired into futures.Client. Contract tests green; build/vet/gofmt clean.
- ✅ **M4** — WebSocket. `internal/ws` (Gate `{time,channel,event,payload,auth}` protocol, per-subscribe
  HMAC-SHA512 auth, reconnect+backoff+jitter, multi-handler-per-key dispatch, app-level ping). Signer.SignWS
  + codec.UnmarshalCaseSensitive (for book_ticker b/B,a/A). futures/stream.go: WatchBookTicker(BBO),
  WatchTickers/WatchMarkPrice, WatchTrades (public); WatchOrders, WatchPositions (private; lazy user_id via
  GET .../accounts). types BookTicker/PublicTrade. Stream() wired in. WS integration tests (gorilla httptest)
  + -race clean. NOTE: no `internal/gatemet` — ws uses logger only (metrics deferred). Conn lifetime = first
  Watch ctx (mirrors okx); per-Watch cancel not supported.
- ✅ **M5** — README.md (overview, architecture, quick start, errors), runnable `examples/basic`, GoDoc.
  Build/vet/gofmt clean; 42 test functions green; `go test -race` clean.

**v1.0 (futures MVP) COMPLETE.** Pending calibration (do when testnet keys available): live signature
check; native `batch_amend_orders` item shape; `order_book` current/update timestamp unit; private-channel
push field exactness (orders/positions/trades). All flagged in code comments.
- 📋 **core integration** (separate branch `gate-connector` off `qa`): `gate_futures` connector
  (`baseToContracts` via quanto_multiplier, `RateLimitEventObserver`→channel, factory registration). Agree plan first.

### v1.0 scope decisions (approved)
- Trading transport: **REST-only** (WS order entry deferred).
- Orderbook: **BBO (book_ticker) + REST snapshot** only (no incremental engine in v1.0).
- Order size units: **contract-native** (Side + positive Size in contracts); base→contracts conversion is the connector's job.

## Code-style rules
camelCase; explicit `var x T = ...`; English GoDoc on exported symbols; file-header block comment per file;
fast JSON via `internal/codec`; `decimal.Decimal` for prices/sizes; never log secrets (Signer redacts).

## Integration secrets
No secrets in repo. Credentials via `gate.Config{APIKey, SecretKey}` (caller supplies from env). Testnet via
`Config.Testnet=true`. No config files in the SDK; the desk owns key sourcing.

## Commands
`go build ./...` · `go test ./...` · `go test -run TestName ./futures` · `go vet ./...` · `gofmt -l .` (must be empty).

## Where things are
Working in git worktree `init-claude-md` (branch `worktree-init-claude-md`); contains `CLAUDE.md`, `handoff.md`,
and all v1.0 SDK code. Merge to `main` when ready. Reference SDKs: `../go-okx/`, `../go-bybit/`. Consumer: `../core/`.
