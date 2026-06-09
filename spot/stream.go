/*
FILE: spot/stream.go

DESCRIPTION:
WebSocket subscription sub-client for the Gate Spot section. One shared ws.Conn
(on the spot WS host) carries both public and private channels; Gate
authenticates per subscribe. Public: WatchBookTicker (BBO), WatchTickers,
WatchTrades. Private (require credentials): WatchOrders, WatchUserTrades,
WatchBalances.

Unlike futures, spot private channels take just [currency_pair] (or "!all") as the
payload — there is no account user-id in the payload; the per-subscribe auth
signature identifies the account.

GENERAL PATTERN PER WATCH*:
  1. Lazily create + Start the shared ws.Conn under ctx (first ctx wins).
  2. Register a subscription whose handler parses the push and (where applicable)
     filters by currency pair, then invokes the user callback.
  3. Return nil, or a setup error (e.g. a private channel without credentials),
     which is also delivered to errHandler.

Local parse errors are logged and dropped (not sent to errHandler) to avoid noise
on a single malformed frame. The stream lives until the ctx passed to the first
Watch* is cancelled, or the section's connection is closed.

WatchOrderBook maintains a local L2 book from the spot.order_book_update
incremental channel, backed by REST snapshots (shared orderbook engine).
*/

package spot

import (
	"context"
	"strconv"
	"sync"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/internal/ws"
	"github.com/tonymontanov/go-gate/v2/orderbook"
	"github.com/tonymontanov/go-gate/v2/spot/types"
)

// Spot WS channels (the "spot." prefix is Gate's).
const (
	chanBookTicker = "spot.book_ticker"
	chanTickers    = "spot.tickers"
	chanTrades     = "spot.trades"
	chanOrders     = "spot.orders"
	chanUserTrades = "spot.usertrades"
	chanBalances   = "spot.balances"
	chanOrderBook  = "spot.order_book_update"
	pingChannel    = "spot.ping"
)

// Gate spot incremental-depth subscribe defaults used when the caller passes
// zero values.
const (
	defaultOrderBookInterval = "100ms"
	defaultOrderBookLevel    = 100
)

// allPairs is Gate's wildcard for private channels covering every currency pair.
const allPairs = "!all"

// StreamClient — WebSocket subscriptions sub-client.
type StreamClient struct {
	c *Client

	connOnce sync.Once
	conn     *ws.Conn
}

func newStreamClient(c *Client) *StreamClient {
	return &StreamClient{c: c}
}

// connection lazily creates the shared ws.Conn from the parent config.
func (s *StreamClient) connection() *ws.Conn {
	s.connOnce.Do(func() {
		var cfg gate.Config = s.c.config()
		s.conn = ws.NewConn(toWsConfig(cfg), s.c.parent.Signer(), cfg.Logger)
	})
	return s.conn
}

// Close shuts down the WebSocket connection, terminating all subscriptions.
func (s *StreamClient) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

func toWsConfig(cfg gate.Config) ws.Config {
	return ws.Config{
		URL:                     cfg.WS.SpotURL,
		PingChannel:             pingChannel,
		HandshakeTimeout:        cfg.WS.HandshakeTimeout,
		ReadTimeout:             cfg.WS.ReadTimeout,
		WriteTimeout:            cfg.WS.WriteTimeout,
		PingInterval:            cfg.WS.PingInterval,
		ReconnectInitialBackoff: cfg.WS.ReconnectInitialBackoff,
		ReconnectMaxBackoff:     cfg.WS.ReconnectMaxBackoff,
		ReconnectJitter:         cfg.WS.ReconnectJitter,
		ReadBufferSize:          cfg.WS.ReadBufferSize,
		WriteBufferSize:         cfg.WS.WriteBufferSize,
	}
}

// ---- public streams --------------------------------------------------------

type bookTickerPush struct {
	T  int64  `json:"t"`
	U  int64  `json:"u"`
	S  string `json:"s"`
	B  string `json:"b"`
	BS string `json:"B"`
	A  string `json:"a"`
	AS string `json:"A"`
}

func (p *bookTickerPush) toBookTicker() types.BookTicker {
	return types.BookTicker{
		CurrencyPair: p.S,
		BidPrice:     mustDecimal(p.B),
		BidSize:      mustDecimal(p.BS),
		AskPrice:     mustDecimal(p.A),
		AskSize:      mustDecimal(p.AS),
		UpdateID:     p.U,
		Ts:           p.T,
	}
}

// WatchBookTicker subscribes to best bid/ask (BBO) updates for a currency pair.
func (s *StreamClient) WatchBookTicker(ctx context.Context, currencyPair string, handler func(types.BookTicker), errHandler func(error)) error {
	var conn *ws.Conn = s.connection()
	conn.Start(ctx)
	return conn.Subscribe(&ws.Subscription{
		Channel: chanBookTicker,
		Payload: []string{currencyPair},
		Handler: func(event string, result []byte) {
			var p bookTickerPush
			// book_ticker uses case-distinct keys b/B and a/A.
			if err := codec.UnmarshalCaseSensitive(result, &p); err != nil {
				s.logParse("book_ticker", err)
				return
			}
			if p.S != currencyPair {
				return
			}
			handler(p.toBookTicker())
		},
	})
}

// WatchTickers subscribes to ticker updates for a currency pair.
func (s *StreamClient) WatchTickers(ctx context.Context, currencyPair string, handler func(types.Ticker), errHandler func(error)) error {
	var conn *ws.Conn = s.connection()
	conn.Start(ctx)
	return conn.Subscribe(&ws.Subscription{
		Channel: chanTickers,
		Payload: []string{currencyPair},
		Handler: func(event string, result []byte) {
			// spot.tickers pushes a single ticker object.
			var p spotTickerPayload
			if err := codec.Unmarshal(result, &p); err != nil {
				s.logParse("tickers", err)
				return
			}
			if p.CurrencyPair != currencyPair {
				return
			}
			handler(tickerFromPayload(&p, nil))
		},
	})
}

type tradePush struct {
	ID           int64     `json:"id"`
	CreateTime   float64   `json:"create_time"`
	CreateTimeMs flexFloat `json:"create_time_ms"`
	Side         string    `json:"side"`
	CurrencyPair string    `json:"currency_pair"`
	Amount       string    `json:"amount"`
	Price        string    `json:"price"`
}

func (p *tradePush) toPublicTrade() types.PublicTrade {
	return types.PublicTrade{
		ID:           p.ID,
		CurrencyPair: p.CurrencyPair,
		Price:        mustDecimal(p.Price),
		Amount:       mustDecimal(p.Amount),
		Side:         types.SideType(p.Side),
		Ts:           epochMsFromParts(float64(p.CreateTimeMs), p.CreateTime),
	}
}

// WatchTrades subscribes to public trades for a currency pair.
func (s *StreamClient) WatchTrades(ctx context.Context, currencyPair string, handler func(types.PublicTrade), errHandler func(error)) error {
	var conn *ws.Conn = s.connection()
	conn.Start(ctx)
	return conn.Subscribe(&ws.Subscription{
		Channel: chanTrades,
		Payload: []string{currencyPair},
		Handler: func(event string, result []byte) {
			// spot.trades pushes a single trade object.
			var p tradePush
			if err := codec.Unmarshal(result, &p); err != nil {
				s.logParse("trades", err)
				return
			}
			if p.CurrencyPair != currencyPair {
				return
			}
			handler(p.toPublicTrade())
		},
	})
}

// ---- order book (incremental depth) ----------------------------------------

// spotOrderBookUpdatePush is one spot.order_book_update event. Levels are
// ["price","amount"] base-currency string pairs (amount "0" removes the level).
// U/u are the first/last update ids (differ only by case → case-sensitive decode).
//
// CALIBRATION: field names (t/s/U/u/b/a) and the [price,amount] level shape are
// taken from Gate's documented spot depth-update format; verify live against prod
// (Gate has no spot testnet).
type spotOrderBookUpdatePush struct {
	T      int64      `json:"t"`
	S      string     `json:"s"`
	FirstU int64      `json:"U"`
	LastU  int64      `json:"u"`
	Bids   [][]string `json:"b"`
	Asks   [][]string `json:"a"`
}

// engineLevelsFromPairs converts ["price","amount"] string pairs to engine levels.
func engineLevelsFromPairs(src [][]string) []orderbook.Level {
	var out []orderbook.Level = make([]orderbook.Level, 0, len(src))
	var i int
	for i = 0; i < len(src); i++ {
		if len(src[i]) < 2 {
			continue
		}
		out = append(out, orderbook.Level{Price: mustDecimal(src[i][0]), Size: mustDecimal(src[i][1])})
	}
	return out
}

// engineLevelsFromTypes converts REST OrderBook levels (decimal base amounts) to
// engine levels — used to seed the engine from a REST snapshot.
func engineLevelsFromTypes(src []types.OrderBookLevel) []orderbook.Level {
	var out []orderbook.Level = make([]orderbook.Level, len(src))
	var i int
	for i = 0; i < len(src); i++ {
		out[i] = orderbook.Level{Price: src[i].Price, Size: src[i].Amount}
	}
	return out
}

// typesLevelsFromEngine converts engine levels back to public OrderBook levels.
func typesLevelsFromEngine(src []orderbook.Level) []types.OrderBookLevel {
	var out []types.OrderBookLevel = make([]types.OrderBookLevel, len(src))
	var i int
	for i = 0; i < len(src); i++ {
		out[i] = types.OrderBookLevel{Price: src[i].Price, Amount: src[i].Size}
	}
	return out
}

/*
WatchOrderBook maintains a local L2 order book for a currency pair from the
spot.order_book_update incremental channel, backed by REST snapshots for priming
and resync (see the orderbook package). The handler receives the full top-`level`
book (amounts in base currency, matching GetOrderBook) on every clean update;
gaps trigger an automatic REST resync.

interval is the Gate push frequency ("100ms"/"1000ms"; default "100ms"); level is
the Gate depth (20/50/100; default 100) and also caps the delivered depth. The
stream lives until ctx is cancelled.
*/
func (s *StreamClient) WatchOrderBook(ctx context.Context, currencyPair string, interval string, level int, handler func(types.OrderBook), errHandler func(error)) error {
	if interval == "" {
		interval = defaultOrderBookInterval
	}
	if level <= 0 {
		level = defaultOrderBookLevel
	}
	var cfg gate.Config = s.c.parent.Config()
	var eng *orderbook.Engine = orderbook.NewEngine(currencyPair, cfg.Orderbook.MaxDepth)

	var snapFn func(context.Context) (orderbook.Snapshot, error) = func(ctx context.Context) (orderbook.Snapshot, error) {
		var book types.OrderBook
		var err error
		book, err = s.c.MarketData().GetOrderBook(ctx, currencyPair, level)
		if err != nil {
			return orderbook.Snapshot{}, err
		}
		return orderbook.Snapshot{
			Symbol: currencyPair,
			Bids:   engineLevelsFromTypes(book.Bids),
			Asks:   engineLevelsFromTypes(book.Asks),
			ID:     book.ID,
			TsMs:   book.UpdateMs,
		}, nil
	}
	var onBook func(*orderbook.Engine) = func(e *orderbook.Engine) {
		var ebids, easks = e.TopLevels(level)
		handler(types.OrderBook{
			ID:   e.LastUpdateID(),
			Bids: typesLevelsFromEngine(ebids),
			Asks: typesLevelsFromEngine(easks),
		})
	}
	var drv *orderbook.Driver = orderbook.NewDriver(eng, snapFn, onBook, func(err error) { invokeErr(errHandler, err) })

	var conn *ws.Conn = s.connection()
	conn.Start(ctx)
	return conn.Subscribe(&ws.Subscription{
		Channel: chanOrderBook,
		Payload: []string{currencyPair, interval, strconv.Itoa(level)},
		Reset:   func() { drv.Reset(ctx) },
		Handler: func(event string, result []byte) {
			var p spotOrderBookUpdatePush
			// U/u differ only by case → case-sensitive decode.
			if err := codec.UnmarshalCaseSensitive(result, &p); err != nil {
				s.logParse("order_book_update", err)
				return
			}
			if p.S != "" && p.S != currencyPair {
				return
			}
			drv.PushDelta(ctx, orderbook.Delta{
				Symbol: currencyPair,
				FirstU: p.FirstU,
				LastU:  p.LastU,
				Bids:   engineLevelsFromPairs(p.Bids),
				Asks:   engineLevelsFromPairs(p.Asks),
				TsMs:   p.T,
			})
		},
	})
}

// ---- private streams -------------------------------------------------------

// WatchOrders subscribes to the account's order updates for a currency pair. Pass
// an empty currencyPair to receive updates for all pairs.
func (s *StreamClient) WatchOrders(ctx context.Context, currencyPair string, handler func([]types.OrderInfo), errHandler func(error)) error {
	var payload []string
	var err error
	payload, err = s.privatePayload(currencyPair)
	if err != nil {
		invokeErr(errHandler, err)
		return err
	}
	var conn *ws.Conn = s.connection()
	conn.Start(ctx)
	return conn.Subscribe(&ws.Subscription{
		Channel: chanOrders,
		Payload: payload,
		Private: true,
		Handler: func(event string, result []byte) {
			var payloads []spotOrderPayload
			if err := codec.Unmarshal(result, &payloads); err != nil {
				s.logParse("orders", err)
				return
			}
			var out []types.OrderInfo = make([]types.OrderInfo, 0, len(payloads))
			var i int
			for i = 0; i < len(payloads); i++ {
				if currencyPair != "" && payloads[i].CurrencyPair != currencyPair {
					continue
				}
				out = append(out, orderInfoFromPayload(&payloads[i], nil))
			}
			if len(out) > 0 {
				handler(out)
			}
		},
	})
}

type userTradePush struct {
	ID           int64     `json:"id"`
	CreateTime   float64   `json:"create_time"`
	CreateTimeMs flexFloat `json:"create_time_ms"`
	CurrencyPair string    `json:"currency_pair"`
	Side         string    `json:"side"`
	Role         string    `json:"role"`
	Amount       string    `json:"amount"`
	Price        string    `json:"price"`
	OrderID      string    `json:"order_id"`
	Fee          string    `json:"fee"`
	FeeCurrency  string    `json:"fee_currency"`
	Text         string    `json:"text"`
}

func (p *userTradePush) toUserTrade() types.UserTrade {
	return types.UserTrade{
		ID:           p.ID,
		OrderID:      p.OrderID,
		CurrencyPair: p.CurrencyPair,
		Side:         types.SideType(p.Side),
		Role:         p.Role,
		Price:        mustDecimal(p.Price),
		Amount:       mustDecimal(p.Amount),
		Fee:          mustDecimal(p.Fee),
		FeeCurrency:  p.FeeCurrency,
		Text:         p.Text,
		Ts:           epochMsFromParts(float64(p.CreateTimeMs), p.CreateTime),
	}
}

// WatchUserTrades subscribes to the account's private fills for a currency pair.
// Pass an empty currencyPair to receive fills for all pairs.
func (s *StreamClient) WatchUserTrades(ctx context.Context, currencyPair string, handler func([]types.UserTrade), errHandler func(error)) error {
	var payload []string
	var err error
	payload, err = s.privatePayload(currencyPair)
	if err != nil {
		invokeErr(errHandler, err)
		return err
	}
	var conn *ws.Conn = s.connection()
	conn.Start(ctx)
	return conn.Subscribe(&ws.Subscription{
		Channel: chanUserTrades,
		Payload: payload,
		Private: true,
		Handler: func(event string, result []byte) {
			var pushes []userTradePush
			if err := codec.Unmarshal(result, &pushes); err != nil {
				s.logParse("usertrades", err)
				return
			}
			var out []types.UserTrade = make([]types.UserTrade, 0, len(pushes))
			var i int
			for i = 0; i < len(pushes); i++ {
				if currencyPair != "" && pushes[i].CurrencyPair != currencyPair {
					continue
				}
				out = append(out, pushes[i].toUserTrade())
			}
			if len(out) > 0 {
				handler(out)
			}
		},
	})
}

type balancePush struct {
	Timestamp   string    `json:"timestamp"`
	TimestampMs flexFloat `json:"timestamp_ms"`
	Currency    string    `json:"currency"`
	Change      string    `json:"change"`
	Total       string    `json:"total"`
	Available   string    `json:"available"`
}

func (p *balancePush) toBalanceUpdate() types.BalanceUpdate {
	return types.BalanceUpdate{
		Currency:  p.Currency,
		Available: mustDecimal(p.Available),
		Total:     mustDecimal(p.Total),
		Change:    mustDecimal(p.Change),
		Ts:        spotEpochMs(float64(p.TimestampMs), p.Timestamp),
	}
}

// WatchBalances subscribes to the account's spot balance updates (all currencies).
func (s *StreamClient) WatchBalances(ctx context.Context, handler func([]types.BalanceUpdate), errHandler func(error)) error {
	if !s.c.signerEnabled() {
		var err error = gate.NewError(gate.ErrorKindAuth, "", "stream: private channel requires API credentials", nil)
		invokeErr(errHandler, err)
		return err
	}
	var conn *ws.Conn = s.connection()
	conn.Start(ctx)
	return conn.Subscribe(&ws.Subscription{
		Channel: chanBalances,
		Private: true,
		Handler: func(event string, result []byte) {
			var pushes []balancePush
			if err := codec.Unmarshal(result, &pushes); err != nil {
				s.logParse("balances", err)
				return
			}
			var out []types.BalanceUpdate = make([]types.BalanceUpdate, 0, len(pushes))
			var i int
			for i = 0; i < len(pushes); i++ {
				out = append(out, pushes[i].toBalanceUpdate())
			}
			if len(out) > 0 {
				handler(out)
			}
		},
	})
}

// ---- helpers ---------------------------------------------------------------

// privatePayload builds the [currency_pair] payload for a private channel,
// using "!all" when currencyPair is empty. Requires credentials.
func (s *StreamClient) privatePayload(currencyPair string) ([]string, error) {
	if !s.c.signerEnabled() {
		return nil, gate.NewError(gate.ErrorKindAuth, "", "stream: private channel requires API credentials", nil)
	}
	var cp string = currencyPair
	if cp == "" {
		cp = allPairs
	}
	return []string{cp}, nil
}

// epochMsFromParts returns epoch ms, preferring the millisecond value when
// present, otherwise converting the float-seconds value.
func epochMsFromParts(ms float64, sec float64) int64 {
	if ms > 0 {
		return int64(ms)
	}
	if sec > 0 {
		return int64(sec * 1000)
	}
	return 0
}

func (s *StreamClient) logParse(channel string, err error) {
	s.c.logger().Warn("stream: parse push failed", gate.Str("channel", channel), gate.Err(err))
}

func invokeErr(errHandler func(error), err error) {
	if errHandler != nil {
		errHandler(err)
	}
}
