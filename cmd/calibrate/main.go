/*
FILE: cmd/calibrate/main.go

DESCRIPTION:
A staged, POSITION-SAFE live-calibration harness for the go-gate SDK against a
REAL Gate account. Calibration verifies the SDK's raw wire mapping (exact JSON
field names / WS frame shapes / signing) — so it dumps RAW bytes next to the
SDK's parsed view: a wrong field tag silently parses to zero in a typed struct,
which only the raw body reveals.

SCOPE: futures + spot.

STAGES (run each deliberately via -stage):
  - public : zero risk, no keys. Raw+parsed REST (contracts/order_book/candles/
             tickers) and RAW public WS frames (book_ticker / order_book_update /
             tickers / trades).
  - read   : zero risk, keys. Signed READ only (positions/open orders/balances) —
             confirms live HMAC-SHA512 signing and private REST field shapes
             WITHOUT placing anything. LOUDLY warns if any position is open.
  - write  : tiny, controlled, CANNOT FILL. Requires -confirm-write. Places ONE
             post-only (poc) LIMIT far from market (min size), captures the
             private WS pushes + raw order JSON, amends (incl. native batch
             amend), then cancels. Armed with a CountdownCancelAll deadman switch.

SAFETY RAILS (independent → no position possible):
  - post-only (poc) only; Gate rejects (never fills) anything that would cross;
  - price set far from market (×0.5) and size = OrderSizeMin / MinBaseAmount;
  - NEVER a market order (it would fill); spot market-buy is NOT sent;
  - pre-flight requires a FLAT position; aborts otherwise;
  - CountdownCancelAll(30s) deadman, refreshed every 10s — if this process dies,
    Gate auto-cancels everything;
  - explicit -confirm-write guard on the write stage.

USAGE:

	go run ./cmd/calibrate -stage=public -section=futures
	go run ./cmd/calibrate -stage=public -section=spot
	GATE_API_KEY=... GATE_API_SECRET=... go run ./cmd/calibrate -stage=read -section=futures
	GATE_API_KEY=... GATE_API_SECRET=... go run ./cmd/calibrate -stage=read -section=spot
	# only after read looks correct:
	GATE_API_KEY=... GATE_API_SECRET=... go run ./cmd/calibrate -stage=write -section=futures -confirm-write
	GATE_API_KEY=... GATE_API_SECRET=... go run ./cmd/calibrate -stage=write -section=spot -confirm-write

Share the printed RAW-vs-parsed output; mismatched/zero fields are candidate
wrong tags to patch in the SDK.
*/

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/futures"
	ftypes "github.com/tonymontanov/go-gate/v2/futures/types"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
	"github.com/tonymontanov/go-gate/v2/internal/ws"
	"github.com/tonymontanov/go-gate/v2/spot"
	stypes "github.com/tonymontanov/go-gate/v2/spot/types"
)

const settle = "usdt"

func main() {
	var stage = flag.String("stage", "public", "public | read | write")
	var section = flag.String("section", "futures", "futures | spot")
	var contract = flag.String("contract", "BTC_USDT", "Gate contract / currency pair")
	var confirmWrite = flag.Bool("confirm-write", false, "required guard for -stage=write")
	var wsSeconds = flag.Int("ws-seconds", 8, "how long to capture WS frames")
	flag.Parse()

	if *section != "futures" && *section != "spot" {
		log.Fatalf("invalid -section %q (futures|spot)", *section)
	}

	var client = buildClient()
	defer func() { _ = client.Close() }()

	var ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	switch *stage {
	case "public":
		runPublic(ctx, client, *section, *contract, *wsSeconds)
	case "read":
		requireKeys()
		runRead(ctx, client, *section, *contract)
	case "write":
		requireKeys()
		if !*confirmWrite {
			log.Fatalf("-stage=write requires -confirm-write (safety guard). " +
				"It places ONE post-only order far from market and cancels it; it cannot fill.")
		}
		runWrite(ctx, client, *section, *contract, *wsSeconds)
	default:
		log.Fatalf("invalid -stage %q (public|read|write)", *stage)
	}
}

// ---------------------------------------------------------------------------
// client + logging
// ---------------------------------------------------------------------------

// stderrLogger surfaces SDK reconnect/parse/ratelimit diagnostics (debug muted).
type stderrLogger struct{}

func (stderrLogger) Debug(string, ...gate.Field)      {}
func (stderrLogger) Info(msg string, _ ...gate.Field) { log.Printf("[gate INFO]  %s", msg) }
func (stderrLogger) Warn(msg string, _ ...gate.Field) { log.Printf("[gate WARN]  %s", msg) }
func (stderrLogger) Error(msg string, _ ...gate.Field) {
	log.Printf("[gate ERROR] %s", msg)
}

func buildClient() *gate.Client {
	var cfg = gate.Config{
		APIKey:    os.Getenv("GATE_API_KEY"),
		SecretKey: os.Getenv("GATE_API_SECRET"),
		Settle:    settle,
		Logger:    stderrLogger{},
		// Surface live rate-limit headers (helps validate the header model).
		RateLimitEventObserver: func(ev gate.RateLimitEvent) {
			if len(ev.Headers) == 0 {
				return
			}
			log.Printf("[ratelimit] %s %s cat=%s headers=%v", ev.Method, ev.Endpoint, ev.Category, ev.Headers)
		},
	}
	var client, err = gate.NewClient(cfg)
	if err != nil {
		log.Fatalf("gate.NewClient: %v", err)
	}
	return client
}

func requireKeys() {
	if os.Getenv("GATE_API_KEY") == "" || os.Getenv("GATE_API_SECRET") == "" {
		log.Fatalf("this stage needs GATE_API_KEY / GATE_API_SECRET")
	}
}

// ---------------------------------------------------------------------------
// raw helpers
// ---------------------------------------------------------------------------

// rawGET performs a (optionally signed) GET and prints the raw JSON body + its
// top-level keys, so wrong/extra field names are visible. Returns the raw bytes.
func rawGET(ctx context.Context, client *gate.Client, label, path string, query map[string]string, signed bool, category gate.RateLimitCategory) []byte {
	var q = make(map[string][]string, len(query))
	for k, v := range query {
		q[k] = []string{v}
	}
	var resp rest.Response
	var err error
	resp, _, err = client.REST().Do(ctx, rest.Options{
		Method: "GET",
		Path:   path,
		Query:  q,
		Signed: signed,
		Meta:   rest.RequestMeta{Category: string(category)},
	})
	fmt.Printf("\n===== RAW %s  (GET %s) =====\n", label, path)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return nil
	}
	var raw = resp.Raw()
	printRawWithKeys(raw)
	return raw
}

// printRawWithKeys pretty-prints raw JSON and lists the top-level keys of the
// first object (so a human can compare against the SDK struct's fields).
func printRawWithKeys(raw []byte) {
	fmt.Printf("  body: %s\n", string(raw))
	var first = raw
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil && len(arr) > 0 {
		first = arr[0]
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(first, &obj) == nil && len(obj) > 0 {
		var keys = make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		fmt.Printf("  wire keys: %v\n", keys)
	}
}

func printParsed(label string, v any) {
	fmt.Printf("----- PARSED %s (SDK struct) -----\n  %+v\n", label, v)
}

// ---------------------------------------------------------------------------
// raw WebSocket capture
// ---------------------------------------------------------------------------

func wsConfigFor(client *gate.Client, section string) ws.Config {
	var c = client.Config()
	var url = c.WS.FuturesURL
	var ping = "futures.ping"
	if section == "spot" {
		url = c.WS.SpotURL
		ping = "spot.ping"
	}
	return ws.Config{
		URL:                     url,
		PingChannel:             ping,
		HandshakeTimeout:        c.WS.HandshakeTimeout,
		ReadTimeout:             c.WS.ReadTimeout,
		WriteTimeout:            c.WS.WriteTimeout,
		PingInterval:            c.WS.PingInterval,
		ReconnectInitialBackoff: c.WS.ReconnectInitialBackoff,
		ReconnectMaxBackoff:     c.WS.ReconnectMaxBackoff,
		ReconnectJitter:         c.WS.ReconnectJitter,
		ReadBufferSize:          c.WS.ReadBufferSize,
		WriteBufferSize:         c.WS.WriteBufferSize,
	}
}

// rawWatch subscribes to a channel and prints up to `maxFrames` raw frames,
// listing wire keys of each. Private subscriptions are signed per-subscribe by
// the ws layer.
func rawWatch(conn *ws.Conn, channel string, payload []string, private bool, maxFrames int) {
	var seen int
	_ = conn.Subscribe(&ws.Subscription{
		Channel: channel,
		Payload: payload,
		Private: private,
		Handler: func(event string, result []byte) {
			if seen >= maxFrames {
				return
			}
			seen++
			fmt.Printf("\n[WS %s] event=%s frame#%d\n", channel, event, seen)
			printRawWithKeys(result)
		},
	})
}

// ---------------------------------------------------------------------------
// STAGE: public
// ---------------------------------------------------------------------------

func runPublic(ctx context.Context, client *gate.Client, section, contract string, wsSeconds int) {
	fmt.Printf("### STAGE public — section=%s contract=%s ###\n", section, contract)

	if section == "futures" {
		var fut = client.Futures().(*futures.Client)
		rawGET(ctx, client, "futures contract", "/futures/"+settle+"/contracts/"+contract, nil, false, gate.RateLimitCategoryMarketData)
		if spec, err := fut.MarketData().GetContract(ctx, contract); err == nil {
			printParsed("futures SymbolInfo", spec)
		}
		rawGET(ctx, client, "futures order_book", "/futures/"+settle+"/order_book",
			map[string]string{"contract": contract, "with_id": "true", "limit": "5"}, false, gate.RateLimitCategoryMarketData)
		if book, err := fut.MarketData().GetOrderBook(ctx, contract, 5); err == nil {
			printParsed("futures OrderBook", book)
		}
		rawGET(ctx, client, "futures candlesticks", "/futures/"+settle+"/candlesticks",
			map[string]string{"contract": contract, "interval": "1m", "limit": "3"}, false, gate.RateLimitCategoryMarketData)
		rawGET(ctx, client, "futures tickers", "/futures/"+settle+"/tickers",
			map[string]string{"contract": contract}, false, gate.RateLimitCategoryMarketData)
	} else {
		var sp = client.Spot().(*spot.Client)
		rawGET(ctx, client, "spot currency_pair", "/spot/currency_pairs/"+contract, nil, false, gate.RateLimitCategoryMarketData)
		if spec, err := sp.MarketData().GetCurrencyPair(ctx, contract); err == nil {
			printParsed("spot SymbolInfo", spec)
		}
		rawGET(ctx, client, "spot order_book", "/spot/order_book",
			map[string]string{"currency_pair": contract, "with_id": "true", "limit": "5"}, false, gate.RateLimitCategoryMarketData)
		if book, err := sp.MarketData().GetOrderBook(ctx, contract, 5); err == nil {
			printParsed("spot OrderBook", book)
		}
		rawGET(ctx, client, "spot candlesticks", "/spot/candlesticks",
			map[string]string{"currency_pair": contract, "interval": "1m", "limit": "3"}, false, gate.RateLimitCategoryMarketData)
		rawGET(ctx, client, "spot tickers", "/spot/tickers",
			map[string]string{"currency_pair": contract}, false, gate.RateLimitCategoryMarketData)
	}

	// --- RAW public WS frames ---
	fmt.Printf("\n### capturing public WS frames for %ds ###\n", wsSeconds)
	var conn = ws.NewConn(wsConfigFor(client, section), client.Signer(), client.Logger())
	conn.Start(ctx)
	defer func() { _ = conn.Close() }()
	var prefix = section // "futures" or "spot"
	rawWatch(conn, prefix+".book_ticker", []string{contract}, false, 3)
	// order_book_update payload differs: futures = [contract, interval, level];
	// spot = [pair, interval] (calibrated live — spot rejects a 3-element payload).
	var obPayload = []string{contract, "100ms", "20"}
	if section == "spot" {
		obPayload = []string{contract, "100ms"}
	}
	rawWatch(conn, prefix+".order_book_update", obPayload, false, 3)
	rawWatch(conn, prefix+".tickers", []string{contract}, false, 2)
	rawWatch(conn, prefix+".trades", []string{contract}, false, 3)
	sleep(ctx, time.Duration(wsSeconds)*time.Second)
	fmt.Printf("\n### public stage done ###\n")
}

// ---------------------------------------------------------------------------
// STAGE: read (signed, zero orders)
// ---------------------------------------------------------------------------

func runRead(ctx context.Context, client *gate.Client, section, contract string) {
	fmt.Printf("### STAGE read — section=%s (signed; confirms auth/signing; places NOTHING) ###\n", section)

	if section == "futures" {
		var fut = client.Futures().(*futures.Client)
		rawGET(ctx, client, "futures accounts", "/futures/"+settle+"/accounts", nil, true, gate.RateLimitCategoryQuery)
		rawGET(ctx, client, "futures positions", "/futures/"+settle+"/positions",
			map[string]string{"holding": "true"}, true, gate.RateLimitCategoryQuery)
		var positions, err = fut.Account().GetPositions(ctx)
		if err != nil {
			log.Printf("GetPositions error (signing?): %v", err)
		} else {
			warnIfFuturesPositions(positions)
			printParsed("futures positions", positions)
		}
		rawGET(ctx, client, "futures open orders", "/futures/"+settle+"/orders",
			map[string]string{"contract": contract, "status": "open"}, true, gate.RateLimitCategoryQuery)
	} else {
		var sp = client.Spot().(*spot.Client)
		rawGET(ctx, client, "spot accounts", "/spot/accounts", nil, true, gate.RateLimitCategoryQuery)
		var balances, err = sp.Account().GetBalances(ctx)
		if err != nil {
			log.Printf("GetBalances error (signing?): %v", err)
		} else {
			printParsed("spot balances", balances)
		}
		rawGET(ctx, client, "spot open orders", "/spot/orders",
			map[string]string{"currency_pair": contract, "status": "open"}, true, gate.RateLimitCategoryQuery)
	}
	fmt.Printf("\n### read stage done (no orders placed) ###\n")
}

func warnIfFuturesPositions(positions []ftypes.PositionInfo) {
	for i := range positions {
		if !positions[i].Size.IsZero() {
			log.Printf("!!! WARNING: open position %s size=%s side=%s — resolve before -stage=write",
				positions[i].Contract, positions[i].Size, positions[i].Side)
		}
	}
}

// ---------------------------------------------------------------------------
// STAGE: write (post-only, far from market, cannot fill; deadman-armed)
// ---------------------------------------------------------------------------

func runWrite(ctx context.Context, client *gate.Client, section, contract string, wsSeconds int) {
	fmt.Printf("### STAGE write — section=%s contract=%s ###\n", section, contract)
	fmt.Printf("Placing ONE post-only LIMIT far from market (cannot fill), then amend + cancel.\n")

	// Capture private WS pushes BEFORE placing.
	var conn = ws.NewConn(wsConfigFor(client, section), client.Signer(), client.Logger())
	conn.Start(ctx)
	defer func() { _ = conn.Close() }()
	subscribePrivateRaw(ctx, client, conn, section, contract)
	sleep(ctx, 1*time.Second) // let private subs settle

	if section == "futures" {
		runWriteFutures(ctx, client, contract)
	} else {
		runWriteSpot(ctx, client, contract)
	}

	// Let trailing private pushes arrive.
	sleep(ctx, time.Duration(wsSeconds)*time.Second)
	fmt.Printf("\n### write stage done ###\n")
}

func subscribePrivateRaw(ctx context.Context, client *gate.Client, conn *ws.Conn, section, contract string) {
	if section == "futures" {
		// Futures private channels need the account user id in the payload.
		var raw = rawGET(ctx, client, "futures accounts (for user id)", "/futures/"+settle+"/accounts", nil, true, gate.RateLimitCategoryQuery)
		var acc struct {
			User int64 `json:"user"`
		}
		_ = json.Unmarshal(raw, &acc)
		var uid = strconv.FormatInt(acc.User, 10)
		rawWatch(conn, "futures.orders", []string{uid, contract}, true, 10)
		rawWatch(conn, "futures.positions", []string{uid, contract}, true, 10)
		rawWatch(conn, "futures.usertrades", []string{uid, contract}, true, 10)
	} else {
		rawWatch(conn, "spot.orders", []string{contract}, true, 10)
		rawWatch(conn, "spot.usertrades", []string{contract}, true, 10)
	}
}

func runWriteFutures(ctx context.Context, client *gate.Client, contract string) {
	var fut = client.Futures().(*futures.Client)

	// Pre-flight: must be FLAT on this contract.
	var pos, err = fut.Account().GetPosition(ctx, contract)
	if err != nil {
		log.Fatalf("pre-flight GetPosition failed (abort): %v", err)
	}
	if !pos.Size.IsZero() {
		log.Fatalf("ABORT: open position on %s (size=%s). Will not place orders.", contract, pos.Size)
	}
	var spec ftypes.SymbolInfo
	spec, err = fut.MarketData().GetContract(ctx, contract)
	if err != nil {
		log.Fatalf("GetContract failed (abort): %v", err)
	}
	var book ftypes.OrderBook
	book, err = fut.MarketData().GetOrderBook(ctx, contract, 1)
	if err != nil || len(book.Bids) == 0 {
		log.Fatalf("GetOrderBook failed (abort): %v", err)
	}
	var bestBid = book.Bids[0].Price
	var price = roundDownToTick(bestBid.Mul(decimal.NewFromFloat(0.5)), spec.OrderPriceRound)
	var size = spec.OrderSizeMin
	fmt.Printf("best bid=%s → post-only buy price=%s size=%s (far below market, cannot fill)\n", bestBid, price, size)

	// Deadman: cancel everything in 30s unless refreshed.
	armDeadman(ctx, func(c context.Context, d time.Duration) error {
		_, e := fut.Trading().CountdownCancelAll(c, d, contract)
		return e
	})

	// Place post-only.
	var info ftypes.OrderInfo
	info, err = fut.Trading().CreateOrder(ctx, ftypes.CreateOrderRequest{
		Contract:    contract,
		Side:        ftypes.SideTypeBuy,
		Size:        size,
		Price:       price,
		OrderType:   ftypes.OrderTypeLimit,
		TimeInForce: ftypes.TimeInForcePOC,
		Text:        "calib-" + nowID(),
	})
	if err != nil {
		log.Printf("CreateOrder error: %v", err)
		_ = fut.Trading().CancelAllOrders(ctx, contract)
		return
	}
	printParsed("futures CreateOrder result", info)
	// Raw order JSON (calibrate response fields).
	rawGET(ctx, client, "futures order by id", "/futures/"+settle+"/orders/"+info.OrderID, nil, true, gate.RateLimitCategoryQuery)

	// Confirm still flat.
	if p2, e := fut.Account().GetPosition(ctx, contract); e == nil && !p2.Size.IsZero() {
		log.Printf("!!! unexpected non-zero position after post-only place: %s — cancelling", p2.Size)
	}

	// Amend (single + native batch) to calibrate amend wire shapes.
	var newPrice = roundDownToTick(price.Mul(decimal.NewFromFloat(0.99)), spec.OrderPriceRound)
	if amended, e := fut.Trading().ModifyOrder(ctx, ftypes.ModifyOrderRequest{
		Contract: contract, OrderID: info.OrderID, NewPrice: newPrice,
	}); e != nil {
		log.Printf("ModifyOrder error: %v", e)
	} else {
		printParsed("futures ModifyOrder result", amended)
	}
	if batch, e := fut.Trading().ModifyBatchOrders(ctx, []ftypes.ModifyOrderRequest{
		{Contract: contract, OrderID: info.OrderID, NewPrice: roundDownToTick(newPrice.Mul(decimal.NewFromFloat(0.99)), spec.OrderPriceRound)},
	}); e != nil {
		log.Printf("ModifyBatchOrders error (native batch_amend shape?): %v", e)
	} else {
		printParsed("futures ModifyBatchOrders result", batch)
	}

	// Cancel + belt-and-suspenders cancel-all + disarm deadman.
	if e := fut.Trading().CancelOrder(ctx, ftypes.CancelOrderRequest{Contract: contract, OrderID: info.OrderID}); e != nil {
		log.Printf("CancelOrder error: %v", e)
	}
	_ = fut.Trading().CancelAllOrders(ctx, contract)
	_, _ = fut.Trading().CountdownCancelAll(ctx, 0, contract) // disarm
	assertFlatFutures(ctx, fut, contract)
}

func runWriteSpot(ctx context.Context, client *gate.Client, contract string) {
	var sp = client.Spot().(*spot.Client)

	var spec, err = sp.MarketData().GetCurrencyPair(ctx, contract)
	if err != nil {
		log.Fatalf("GetCurrencyPair failed (abort): %v", err)
	}
	var book stypes.OrderBook
	book, err = sp.MarketData().GetOrderBook(ctx, contract, 1)
	if err != nil || len(book.Bids) == 0 {
		log.Fatalf("GetOrderBook failed (abort): %v", err)
	}
	var bestBid = book.Bids[0].Price
	// Price tick = 10^-PricePrecision.
	var tick = decimal.New(1, -spec.PricePrecision)
	var price = roundDownToTick(bestBid.Mul(decimal.NewFromFloat(0.5)), tick)
	var amount = spec.MinBaseAmount
	fmt.Printf("best bid=%s → post-only buy price=%s amount=%s (far below market, cannot fill)\n", bestBid, price, amount)

	// Deadman.
	armDeadman(ctx, func(c context.Context, d time.Duration) error {
		_, e := sp.Trading().CountdownCancelAll(c, d, contract)
		return e
	})

	var info stypes.OrderInfo
	info, err = sp.Trading().CreateOrder(ctx, stypes.CreateOrderRequest{
		CurrencyPair: contract,
		Side:         stypes.SideTypeBuy,
		Amount:       amount,
		Price:        price,
		OrderType:    stypes.OrderTypeLimit,
		TimeInForce:  stypes.TimeInForcePOC,
		Text:         "calib-" + nowID(),
	})
	if err != nil {
		log.Printf("CreateOrder error: %v", err)
		_ = sp.Trading().CancelAllOrders(ctx, contract)
		return
	}
	printParsed("spot CreateOrder result", info)
	rawGET(ctx, client, "spot order by id", "/spot/orders/"+info.OrderID,
		map[string]string{"currency_pair": contract}, true, gate.RateLimitCategoryQuery)

	var newPrice = roundDownToTick(price.Mul(decimal.NewFromFloat(0.99)), tick)
	if amended, e := sp.Trading().ModifyOrder(ctx, stypes.ModifyOrderRequest{
		CurrencyPair: contract, OrderID: info.OrderID, NewPrice: newPrice,
	}); e != nil {
		log.Printf("ModifyOrder error: %v", e)
	} else {
		printParsed("spot ModifyOrder result", amended)
	}

	if e := sp.Trading().CancelOrder(ctx, stypes.CancelOrderRequest{CurrencyPair: contract, OrderID: info.OrderID}); e != nil {
		log.Printf("CancelOrder error: %v", e)
	}
	_ = sp.Trading().CancelAllOrders(ctx, contract)
	_, _ = sp.Trading().CountdownCancelAll(ctx, 0, contract) // disarm

	// NOTE: spot MARKET-BUY (amount = QUOTE) is NOT sent here — a market order
	// fills. Its quote-amount semantics are documented in spot/types and the
	// connector; validate by code inspection, not live.
	fmt.Printf("(spot market-buy quote-amount semantics intentionally NOT live-tested — would fill)\n")
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// armDeadman starts a goroutine that arms CountdownCancelAll(30s) and refreshes
// it every 10s, so a process crash leaves no live orders.
func armDeadman(ctx context.Context, refresh func(context.Context, time.Duration) error) {
	if err := refresh(ctx, 30*time.Second); err != nil {
		log.Printf("deadman arm failed: %v", err)
		return
	}
	log.Printf("deadman armed: CountdownCancelAll(30s)")
	go func() {
		var t = time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = refresh(ctx, 30*time.Second)
			}
		}
	}()
}

func assertFlatFutures(ctx context.Context, fut *futures.Client, contract string) {
	if p, e := fut.Account().GetPosition(ctx, contract); e == nil {
		if p.Size.IsZero() {
			fmt.Printf("FINAL: position flat ✓\n")
		} else {
			log.Printf("!!! FINAL: position NOT flat: size=%s — investigate", p.Size)
		}
	}
	if orders, e := fut.Trading().GetOpenOrders(ctx, contract); e == nil {
		fmt.Printf("FINAL: open orders=%d (want 0)\n", len(orders))
	}
}

// roundDownToTick rounds price down to a multiple of tick (tick<=0 → unchanged).
func roundDownToTick(price, tick decimal.Decimal) decimal.Decimal {
	if tick.LessThanOrEqual(decimal.Zero) {
		return price
	}
	return price.Div(tick).Floor().Mul(tick)
}

func nowID() string {
	// Unique-ish suffix from the wall clock; only used as the client order text.
	return strconv.FormatInt(time.Now().UnixNano()%1_000_000_000, 10)
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
