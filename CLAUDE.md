# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repository is

`go-gate` is a high-performance Go SDK for the **Gate** exchange (REST + WebSocket), built for an
in-house HFT / market-making desk. It is a **single-exchange** SDK: cross-exchange unification lives
in the desk core (`sleipnir/core`), not here. The SDK exposes idiomatic, low-latency primitives that a
desk `ExchangeConnector` adapter delegates to.

The repo is currently greenfield — only the spec (`docs/`) and `LICENSE` exist. Implementation is built
from scratch following the conventions of the sibling SDKs `go-okx` and `go-bybit` (see below). The
authoritative requirements are in `docs/TS-SINGLE-EXCHANGE-SDK.md` (EN) / `-RU.md`.

Delivery is staged:
- **v1.0** — MVP: Gate USD-M perpetual **Futures** only.
- **v2.0** — add **Spot**.
- **v2.5** — cover the remaining Gate sections as fully as practical.

## Sibling reference SDKs (read these before writing code)

These are the canonical patterns to copy. They live in the same monorepo:
- `../go-okx/` — module `github.com/tonymontanov/go-okx/v2`
- `../go-bybit/` — module `github.com/tonymontanov/go-bybit/v2`
- `../go-binance/` — the original style reference (adshao/go-binance fork).

The primary consumer is `../core/` (the desk). When integrating, add a new Gate connector there on a
branch named `gate-connector` based off `qa`, and **only** add Gate — do not touch other connectors.
Agree the core change plan with the user before editing core.

## Architecture conventions (must follow)

**Three-layer client composition** (identical in go-okx and go-bybit — replicate it):

1. **Root client** (`gate.Client`) — owns shared infrastructure: config, `auth.Signer`, REST client,
   logger, metrics. It does not know about specific sections.
2. **Section client** (e.g. `futures.Client`, later `spot.Client`) — one Go package per Gate trading
   section. Holds a `parent *gate.Client` back-reference and lazily builds its sub-clients.
3. **Domain sub-clients** inside each section — `TradingClient`, `AccountClient`, `MarketDataClient`,
   `StreamClient` — which hold the actual endpoint logic.

Section packages register themselves with the root via a factory at `init()` time to avoid import
cycles; the root exposes them through `sync.Once`-guarded accessors returning `any`
(e.g. `client.Futures().(*futures.Client)`). Section sub-clients reach shared resources through thin
shortcut methods on the parent (`rest()`, `logger()`, `config()`, `signer()`).

**Two-layer "unify then specialize" rule (non-negotiable, per user):** Build *one* unified primitive
and specialize it per section. Never copy-paste a section's logic into another section, and never have
one section's function call another section's function. The shared work lives in the root/internal
layer (`internal/rest`, `internal/ws`, `internal/auth`, shared `types/`); each section calls that
unified primitive with its own specifics.
- WRONG: `spotRequest()` internally calls `futuresRequest()`.
- RIGHT: both `spotRequest()` and `futuresRequest()` call a unified `request()` with their own params.

**Endpoint implementation pattern** (see `go-okx/swap/trading.go`, `go-bybit/linears/trading.go`):
- Public method takes a plain request struct: `CreateOrder(ctx, types.CreateOrderRequest) (types.OrderInfo, error)`.
- A `buildXxxBody(req)` helper does validation + defaulting + body assembly (no builder methods on the
  struct itself).
- Call the shared `rest().Do(ctx, rest.Options{Method, Path, Body, Signed, Meta})`.
- Parse the typed response and map into stable domain types (`OrderInfo`, `PositionInfo`, `SymbolInfo`).

**WebSocket / streams:** lazy public/private `ws.Conn` per section via `sync.Once`; `StreamClient`
exposes `Watch*` methods `(ctx, ..., handler, errHandler) error` cancelled via `ctx.Done()`; reconnect
with backoff + jitter and automatic resubscribe handled in `internal/ws`. The order book engine
(snapshot + delta + seq + gap detection + resync) lives in a shared `orderbook/` package.

## Section naming (must follow, per user)

Trading-section package names must match **Gate's own naming**, not a generic abstraction. If Gate calls
USD-M perpetuals "Futures", the package is `futures` and the core connector section is `gate_futures`.
Mirror Gate's API terminology (e.g. `futures`, `spot`, `delivery`, `options`) exactly. Compare with how
go-okx uses `swap`/`spot` and go-bybit uses `linears`/`spot` for their own exchanges' terms.

## Code style (must follow)

- All SDK code comments and GoDoc in **English** (this is a public project).
- `camelCase`; **explicit `var` declarations** with explicit types in the body (the reference SDKs write
  `var d decimal.Decimal; d, err = ...` rather than `:=` in non-trivial spots — match the surrounding
  style of go-okx/go-bybit).
- Exported funcs/types get GoDoc comments.
- HFT hot-path discipline: avoid allocations on critical paths (parse one WS message, apply one delta);
  reuse buffers / `sync.Pool`; target ≤100 µs software latency on those paths. Use fast JSON
  (`json-iterator`) where it matters and `shopspring/decimal` for prices/sizes.
- Never log secrets.

## Errors

Single error model like the references (`gate.NewError(kind, code, msg, wrapped)`): `ErrorKind` ∈
{Network, RateLimit, Auth, InvalidRequest, Exchange, Unknown}, carrying the Gate code/message and a
wrapped error for `errors.Is`/`As`. Map Gate HTTP statuses / labels (e.g. 429 → RateLimit) to these.

## Rate limits

The SDK does **not** implement a cross-exchange limiter. It surfaces rate-limit metadata so an external
limiter can budget: each REST `Do` carries `rest.RequestMeta{OrderCount, Symbols, Category}` and a
`RateLimitEvent` observer hook (see `go-okx/rate-limit-event.go`). Propagate Gate's rate-limit response
headers where Gate provides them, and map 429/limit codes to `RateLimit` errors.

## Expected dependencies (match the sibling SDKs)

- `go 1.24`
- `github.com/gorilla/websocket` — WS client
- `github.com/json-iterator/go` — fast JSON
- `github.com/shopspring/decimal` — prices/quantities
- Module path follows the sibling convention (`github.com/tonymontanov/go-gate/v2` or as the user
  confirms) — confirm before committing `go.mod`.

## Commands

```bash
go test ./...            # all tests
go test ./futures        # one package
go test -v -run TestName ./futures   # a single test
go test -cover ./...     # coverage
go vet ./...
gofmt -l .               # list unformatted files (must be empty)
```

Tests are unit + **contract tests**: drive endpoint parsing against `httptest.Server` using real Gate
JSON fixtures (copied from Gate API docs), so a contract change fails the build. No live network in
tests. See `go-okx/swap/contract_test.go` for the template.

## Handoff document (must maintain)

After each significant change (architecture, new module, refactor) update `handoff.md` at the repo root
(create it if missing). It carries context across sessions: role & stack, architecture (folder map +
module interaction), roadmap (done ✅ / in progress 🔧 / planned 📋), code-style rules, and integration
secrets (config paths, env var **names** only — never real keys).
