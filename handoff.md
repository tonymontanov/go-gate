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
- **Sections** (per Gate terminology), all IMPLEMENTED: `futures/` (USD-M perp, settle=usdt),
  `spot/`, `delivery/` (dated futures), `options/` (trading sections: `client.go` +
  `trading.go`/`account.go`/`market.go`/`stream.go` + `types/`); plus REST-only account/
  lending sections `margin/` (isolated+cross), `unified/` (portfolio-margin account),
  `earn/` (Uni flexible lending). Each registers a factory via `Register*Factory` in `init()`.

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
- ✅ **v2.0 — `spot/` section** (full parity, SDK only). Branch `v2.0-spot`. Reuses the entire
  root/internal layer. Root `Spot()`/`RegisterSpotFactory`; config `WS.SpotURL`
  (`wss://api.gateio.ws/ws/v4/`, no testnet — Gate spot has none; REST host shared). Spot-specific
  types (currency_pairs spec with amount_precision/precision; order-book `[price,amount]` base levels;
  ticker w/ bid/ask, no mark/funding; candle column order t,quote_vol,c,h,l,o,base_vol,closed;
  balances not positions). Trading: create/batch/amend(**PATCH**)/cancel/cancel-all/cancel-batch
  (`[{currency_pair,id}]`)/countdown/get + ID-mapping; explicit side/type, amount in base,
  **market BUY amount = quote**, market omits price + needs ioc/fok. Account: GetBalances/GetBalance.
  Market: currency pairs/order book/candles/tickers. Stream: book_ticker/tickers/trades (public) +
  orders/usertrades/balances (private; payload `[currency_pair]`/`!all`, no user-id). `flexFloat`
  tolerates number-or-string `*_ms`. Contract+validation+WS(-race) tests green; `examples/spot`.
  Live spot calibration PENDING (Gate spot testnet does not exist; validate against prod with prod
  keys). Published: tag `v2.1.0`.
- ✅ **v2.5 — incremental order-book engine** (SDK). Branch `v2.5-orderbook` off `v2.0-spot`. New
  shared `orderbook/` package (mirrors `go-bybit/orderbook`): `Engine` (snapshot + delta apply,
  sorted sides, size-0 delete, top-N) + `Driver` (REST-snapshot priming, delta buffering during
  prime, automatic resync on gap / reconnect). Gate-specific: snapshot comes from REST
  `GetOrderBook(with_id)` (NOT over WS); each `*.order_book_update` delta carries `[U,u]` ids; gap =
  `U > lastU+1`; stale = `u <= lastU`; first post-snapshot delta aligns when `U <= snapId+1 <= u`. No
  CRC32 (Gate, like Bybit, has none). Config `Orderbook.MaxDepth` (default 400). Wired
  `WatchOrderBook(ctx, symbol, interval, level, handler, errHandler)` on BOTH `futures/stream.go`
  (`futures.order_book_update`, `{p,s}` contract levels) and `spot/stream.go`
  (`spot.order_book_update`, `[price,amount]` base levels); handler gets full top-N `types.OrderBook`
  per clean update. Offline engine+driver unit tests + WS/REST integration tests (-race) green;
  examples updated. CALIBRATION: exact depth-update field names (`t/s/U/u/b/a`), level shape, and the
  freq/level subscribe-payload syntax follow Gate docs — verify live (futures testnet was down; spot
  has no testnet). WS order entry + native batch_amend still deferred.
- ✅ **v2.5 — `delivery/` section** (SDK only). Branch `v2.5-delivery` off `v2.5-orderbook`. Gate
  Delivery = DATED/quarterly USD-M futures under `/delivery/{settle}/...`; contract names encode the
  expiry (`BTC_USDT_20240329`, ASSET_SETTLE_YYYYMMDD) and settle at expiry (no funding). Structural
  mirror of `futures/` (same internal layer + shared orderbook engine), specialized: SymbolInfo adds
  `ExpireTimeMs`/`Cycle` and drops funding; Ticker drops funding; Account adds `GetSettlements`
  (`/settlements`). Root `Delivery()`/`RegisterDeliveryFactory`; config `WS.DeliveryURL`
  (default `wss://fx-ws.gateio.ws/v4/ws/delivery/usdt`, +testnet variant), reuses `Settle`. The SDK is
  naming-agnostic — callers pass the full dated contract name. Contract/market/account/stream tests
  (incl. settlements + dated-contract expiry parse) + `examples/delivery` green; build/vet/gofmt/-race
  clean. CALIBRATION (untested live; delivery has no reachable env here): delivery WS host + whether
  channels are `delivery.*` vs the reused `futures.*` namespace; exact `expire_time`/`cycle` keys +
  units; settlement-record shape; confirm order/position bodies match futures.
- ✅ **v2.5 — `options/` section** (SDK only, does NOT touch core). Gate Options =
  European-style crypto options written on an UNDERLYING index; contract names encode
  underlying+expiry+strike (`BTC_USDT-20240329-50000-C`). Structural mirror of
  `futures/`/`delivery/` (same internal layer + shared orderbook engine), with the
  key differences: **NOT settle-scoped** — REST paths are `/options/...` (no
  `{settle}`), `basePath()` returns `"/options"`; **NO batch endpoints** (no batch
  create/amend/cancel); orders use the **futures signed-size model** (direction = sign
  of int `size`, market = `price="0"`+`tif=ioc`, `text` `t-`-prefixed; amend size is
  signed → needs `Side`). Root `Options()`/`RegisterOptionsFactory` + config
  `WS.OptionsURL` were pre-wired; `options/client.go` `init()` registers the factory.
  MarketData (public): GetUnderlyings, GetExpirations (`[]int64` secs → ms), GetContracts/
  GetContract (SymbolInfo w/ underlying/expiry/strike/is_call/multiplier/greeks),
  GetSettlements/GetSettlement, GetOrderBook(with_id), GetTickers (IV surface + greeks),
  GetUnderlyingTicker, GetCandlesticks, GetUnderlyingCandlesticks, GetTrades. Account
  (signed): GetAccount (single object), GetAccountBook, GetPositions, GetPosition
  (POSITION_NOT_FOUND→flat), GetPositionClose, GetMySettlements. Trading (signed):
  CreateOrder, ModifyOrder (PUT, signed size), CancelOrder, CancelAllOrders (native
  DELETE `?contract=&underlying=&side=`), CountdownCancelAll, GetOrder, GetOpenOrders,
  GetMyTrades, MMP (GetMMP/SetMMP/ResetMMP) + ClientOrderID↔OrderID mapping. Stream:
  WatchContractTickers (`options.contract_tickers`), WatchUnderlyingPrice
  (`options.ul_price`), WatchTrades (`options.trades`), WatchOrderBook (shared engine,
  `options.order_book_update`); private WatchOrders/WatchPositions/WatchUserTrades
  (`options.orders`/`.positions`/`.usertrades`, lazy user-id via GET /options/accounts).
  `codec.FlexDecimal` on all number-or-string payload fields (contract/ticker/position/
  account decimals + greeks: REST quotes, WS sends bare numbers). Contract/market/account/
  validation/stream (-race) tests green + `examples/options`; build/vet/gofmt clean.
  CALIBRATION (untested live; no reachable options env here): WS host
  `wss://op-ws.gateio.live/v4/ws` + the `options.*` channel names and payload shapes;
  ul_price/usertrades push field names; exact contract-spec keys (expiration_time/
  strike_price/is_call/multiplier) + units; position_close/my_settlements/account/MMP
  field sets. SDK-only — no core change. Published: tag `v2.5.0`.
- ✅ **v2.6 — `margin/` + `unified/` + `earn/` sections** (SDK only, REST-only, do NOT
  touch core). Three additional Gate sections, all NOT settle-scoped, each its own
  package mirroring the section pattern (root `Margin()`/`Unified()`/`Earn()` +
  `Register*Factory` pre-wired in `client.go`; per-section `init()` registers). No WS
  (these are account/lending domains). `codec.FlexDecimal` on number-or-string money
  fields; epoch-seconds→ms via per-section local helpers (no cross-section imports).
  • **margin/** (`/margin/...` isolated + `/margin/cross/...` cross): sub-clients
    `Isolated()` (currency_pairs, funding_book, accounts, account_book, funding_accounts,
    loans CRUD + repayment, loan_records, auto_repay, transferable, borrowable) and
    `Cross()` (currencies, accounts, account_book, loans, repayments, transferable,
    borrowable). `examples/margin`.
  • **unified/** (`/unified/...` portfolio/cross-margin account): single `unified.Client`
    — accounts, borrowable/batch_borrowable, transferable/transferables, loans (list/
    create borrow|repay), loan_records, interest_records, estimate_rate, history_loan_rate,
    currencies, unified_mode (get/set PUT), risk_units, portfolio_calculator,
    collateral_currencies, currency_discount_tiers, loan_margin_tiers, leverage config/
    setting (get/set). `examples/unified`.
  • **earn/** (`/earn/uni/...` flexible lending): single `earn.Client` — currencies
    (list/get), lends (create lend|redeem POST, change PATCH, list), lend_records,
    interests/{ccy}, interest_records, interest_status/{ccy}, chart, rate.
    `examples/earn`.
  httptest contract tests per section (incl. FlexDecimal number-or-string decode +
  {label,message}→*gate.Error) green; build/vet/gofmt/-race clean. CALIBRATION (no
  reachable env here; modeled on Gate v4 docs): exact field names/sets for margin
  account/loan/cross bodies, unified account-wide margins + balance keys + mode
  settings map + tier shapes, and earn lend/interest field vocab; verify live.
  Published: tag `v2.6.0`. Remaining Gate surface NOT yet covered: flash_swap,
  multi_collateral loan, earn fixed-term/dual, sub-account & wallet transfers.
- ✅ **LIVE CALIBRATION — futures + spot (prod, 2026-06-12).** Verified raw-vs-parsed across public
  REST/WS, signed reads, and the full write path (post-only place→amend→cancel; never filled). Futures
  fully clean (signing, order_book_update `{t,s,U,u,b:[{p,s}],a}` contiguous, book_ticker `b/B/a/A`,
  positions, PUT amend). Spot clean except three fixes that landed:
  (1) `spot.order_book_update` subscribe payload is `[pair, interval]` (2 elements; futures stays 3);
  (2) futures/delivery `GetPosition` on a FLAT contract returns a zero position (Gate sends
  `POSITION_NOT_FOUND` HTTP 400, no longer propagated);
  (3) spot `orders` WS push has no `status` — Status is derived from `finish_as` (open/filled/cancelled).
  Native `batch_amend_orders` still deferred (`ModifyBatchOrders` loops single PUT amends — works). The
  diagnostic harness lives on branch `calibration-harness` (`cmd/calibrate`), kept OUT of the release.
- ✅ **LIVE BUGFIX — futures (prod, 2026-06-13).** First live run with a real
  filled position + active re-quoting surfaced two bugs, both fixed:
  (1) **WS positions never parsed** → inventory was not real-time. The shared
  `positionPayload` typed its decimal fields as `string`, but the
  `futures.positions` / `delivery.positions` WS push sends them as bare JSON
  numbers (`cross_leverage_limit:25`, `liq_price:0.0418`) while REST quotes them.
  Added `codec.FlexDecimal` (number-or-string → decimal, precision-preserving,
  tolerant Zero) and switched all decimal fields of `positionPayload` in
  `futures/account.go` + `delivery/account.go` to it. Test:
  `TestPositionPayload_DecodesNumberAndStringForms` + `internal/codec` test.
  (2) **futures amend always failed** with `trading.CreateOrder: Side must be buy
  or sell`. The SDK is correct (Gate's amend `size` is signed, so `buildAmendBody`
  needs `Side`); the **core** `gate_futures` connector built the
  `ModifyOrderRequest` WITHOUT `Side`. Fixed in
  `core/.../gate/futures/connector.go` (`ModifyOrder` + `ModifyBatchOrders`) by
  passing `Side: coreSideToGate(req.Side)`. Test:
  `TestModifyOrder_PropagatesSignedSize` (httptest-backed, asserts signed size).
  Spot was unaffected (spot amend uses unsigned base `amount`). NOTE: spot's Gate
  `order` rate-limit bucket is tiny (limit≈10/window) → the header-driven limiter
  legitimately throttles batch place/amend; native batch-amend (below) reduces this.
- ✅ **NATIVE BATCH AMEND (2026-06-13).** `ModifyBatchOrders` no longer loops single
  amends — it now uses Gate's native endpoints, chunked to Gate's per-request caps:
  futures `POST /futures/{settle}/batch_amend_orders` (≤10/req, item
  `{order_id|text, size [signed], price, amend_text}`, response `[]BatchFuturesOrder`
  reusing `batchFuturesOrderPayload`); spot `POST /spot/amend_batch_orders` (≤5/req,
  item `{order_id, currency_pair, amount, price, amend_text}`, response `[]BatchOrder`
  reusing `batchSpotOrderPayload`). Per-element `succeeded/label` aggregation mirrors
  CreateBatchOrders; `RateLimitCategoryAmend` with `OrderCount=len(chunk)`. One HTTP
  request per chunk instead of one per order — materially cheaper on Gate's per-UID
  order limit. No core change needed (connector already calls `ModifyBatchOrders` and
  passes `Side`). Tests: `TestModifyBatchOrders_NativeBatchAmend` (futures + spot,
  assert path + signed/unsigned item shape + per-element mapping). Delivery still
  loops single amends (Gate delivery has no batch-amend; out of scope, not core-wired).
  CALIBRATION: verify live the exact futures item id field (`order_id` int vs `text`)
  and that response is `BatchFuturesOrder` with `succeeded` (mirrors batch create).
- 📋 **core integration** (branch `gate-connector` off `qa`): `gate_futures` connector DONE
  (`baseToContracts` via quanto_multiplier, `RateLimitEventObserver`→channel, factory registration,
  runtime wiring, header-driven rate-limiter). `gate_spot` connector DONE (consumes published
  `v2.1.0`; base-amount sizing, balance-as-position, no-op leverage/mark/index, factory/config/app
  wiring + rate-limiter). The incremental order book is SDK-only for now: the core
  `ExchangeConnector` has no deep-book Watch method (uses `WatchSpread` for BBO), so exposing
  `WatchOrderBook` to core needs a core interface change — agree plan first.

### v1.0 scope decisions (approved)
- Trading transport: **REST-only** (WS order entry deferred — still deferred).
- Orderbook: **BBO (book_ticker) + REST snapshot** in v1.0/v2.0; the incremental engine
  (`WatchOrderBook`) landed in v2.5 (branch `v2.5-orderbook`).
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
