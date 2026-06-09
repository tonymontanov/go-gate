/*
FILE: futures/stream.go

DESCRIPTION:
WebSocket subscription sub-client for the Gate Futures section. One shared
ws.Conn carries both public and private channels (Gate authenticates per
subscribe). Public: WatchBookTicker (BBO), WatchTickers + WatchMarkPrice/
WatchLastPrice/WatchIndexPrice (tickers), WatchTrades. Private: WatchOrders,
WatchPositions (require credentials; payload needs the account user id, fetched
once lazily from GET /futures/{settle}/accounts).

GENERAL PATTERN PER WATCH*:
  1. Lazily create + Start the shared ws.Conn under ctx (first ctx wins).
  2. Register a subscription whose handler parses the push and filters by
     contract, then invokes the user callback.
  3. Return nil, or a setup error (e.g. private channel without credentials),
     which is also delivered to errHandler.

Local parse errors are logged and dropped (not sent to errHandler) to avoid
noise on a single malformed frame. The stream lives until the ctx passed to the
first Watch* is cancelled, or the section's connection is closed.
*/

package futures

import (
	"context"
	"strconv"
	"sync"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/futures/types"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
	"github.com/tonymontanov/go-gate/v2/internal/ws"
	"github.com/tonymontanov/go-gate/v2/orderbook"

	"github.com/shopspring/decimal"
)

// Futures WS channels (USDT-settled; the "futures." prefix is Gate's).
const (
	chanBookTicker = "futures.book_ticker"
	chanTickers    = "futures.tickers"
	chanTrades     = "futures.trades"
	chanOrders     = "futures.orders"
	chanPositions  = "futures.positions"
	chanOrderBook  = "futures.order_book_update"
	pingChannel    = "futures.ping"
)

// defaultOrderBookInterval / defaultOrderBookLevel are Gate's incremental-depth
// subscribe defaults used when the caller passes zero values.
const (
	defaultOrderBookInterval = "100ms"
	defaultOrderBookLevel    = 100
)

// allContracts is Gate's wildcard for private channels covering every contract.
const allContracts = "!all"

// StreamClient — WebSocket subscriptions sub-client.
type StreamClient struct {
	c *Client

	connOnce sync.Once
	conn     *ws.Conn

	userOnce sync.Once
	userID   string
	userErr  error
}

func newStreamClient(c *Client) *StreamClient {
	return &StreamClient{c: c}
}

// connection lazily creates the shared ws.Conn from the parent config.
func (s *StreamClient) connection() *ws.Conn {
	s.connOnce.Do(func() {
		var cfg gate.Config = s.c.parent.Config()
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
		URL:                     cfg.WS.FuturesURL,
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
	BS int64  `json:"B"`
	A  string `json:"a"`
	AS int64  `json:"A"`
}

func (p *bookTickerPush) toBookTicker() types.BookTicker {
	return types.BookTicker{
		Contract: p.S,
		BidPrice: mustDecimal(p.B),
		BidSize:  decimalAbsInt(p.BS),
		AskPrice: mustDecimal(p.A),
		AskSize:  decimalAbsInt(p.AS),
		UpdateID: p.U,
		Ts:       p.T,
	}
}

// WatchBookTicker subscribes to best bid/ask (BBO) updates for a contract.
func (s *StreamClient) WatchBookTicker(ctx context.Context, contract string, handler func(types.BookTicker), errHandler func(error)) error {
	var conn *ws.Conn = s.connection()
	conn.Start(ctx)
	return conn.Subscribe(&ws.Subscription{
		Channel: chanBookTicker,
		Payload: []string{contract},
		Handler: func(event string, result []byte) {
			var p bookTickerPush
			// book_ticker uses case-distinct keys b/B and a/A.
			if err := codec.UnmarshalCaseSensitive(result, &p); err != nil {
				s.logParse("book_ticker", err)
				return
			}
			if p.S != contract {
				return
			}
			handler(p.toBookTicker())
		},
	})
}

// WatchTickers subscribes to ticker updates for a contract (last/mark/index/funding).
func (s *StreamClient) WatchTickers(ctx context.Context, contract string, handler func(types.Ticker), errHandler func(error)) error {
	var conn *ws.Conn = s.connection()
	conn.Start(ctx)
	return conn.Subscribe(&ws.Subscription{
		Channel: chanTickers,
		Payload: []string{contract},
		Handler: func(event string, result []byte) {
			var payloads []tickerPayload
			if err := codec.Unmarshal(result, &payloads); err != nil {
				s.logParse("tickers", err)
				return
			}
			var i int
			for i = 0; i < len(payloads); i++ {
				if payloads[i].Contract != contract {
					continue
				}
				handler(tickerFromPayload(&payloads[i], nil))
			}
		},
	})
}

// WatchMarkPrice subscribes to mark-price updates for a contract (via tickers).
func (s *StreamClient) WatchMarkPrice(ctx context.Context, contract string, handler func(types.Ticker), errHandler func(error)) error {
	return s.WatchTickers(ctx, contract, handler, errHandler)
}

type tradePush struct {
	ID           int64   `json:"id"`
	CreateTime   float64 `json:"create_time"`
	CreateTimeMs float64 `json:"create_time_ms"`
	Contract     string  `json:"contract"`
	Size         int64   `json:"size"`
	Price        string  `json:"price"`
}

func (p *tradePush) toPublicTrade() types.PublicTrade {
	return types.PublicTrade{
		ID:       p.ID,
		Contract: p.Contract,
		Price:    mustDecimal(p.Price),
		Size:     decimalAbsInt(p.Size),
		Side:     sideFromSize(p.Size),
		Ts:       epochMs(p.CreateTimeMs, p.CreateTime),
	}
}

// WatchTrades subscribes to public trades for a contract.
func (s *StreamClient) WatchTrades(ctx context.Context, contract string, handler func(types.PublicTrade), errHandler func(error)) error {
	var conn *ws.Conn = s.connection()
	conn.Start(ctx)
	return conn.Subscribe(&ws.Subscription{
		Channel: chanTrades,
		Payload: []string{contract},
		Handler: func(event string, result []byte) {
			var pushes []tradePush
			if err := codec.Unmarshal(result, &pushes); err != nil {
				s.logParse("trades", err)
				return
			}
			var i int
			for i = 0; i < len(pushes); i++ {
				if pushes[i].Contract != contract {
					continue
				}
				handler(pushes[i].toPublicTrade())
			}
		},
	})
}

// ---- order book (incremental depth) ----------------------------------------

// obLevelFutures is one level inside a futures.order_book_update push: price as
// a string, size as an integer number of contracts (0 means remove the level).
type obLevelFutures struct {
	P string `json:"p"`
	S int64  `json:"s"`
}

// orderBookUpdatePush is one futures.order_book_update event. U/u are the first
// and last update ids covered by the event and differ only by case, so this push
// MUST be decoded case-sensitively.
//
// CALIBRATION: the exact field names (t/s/U/u/b/a) and the {p,s} level shape are
// taken from Gate's documented futures depth-update format; verify live once a
// reachable environment is available (futures testnet was down at v1.0).
type orderBookUpdatePush struct {
	T      int64            `json:"t"`
	S      string           `json:"s"`
	FirstU int64            `json:"U"`
	LastU  int64            `json:"u"`
	Bids   []obLevelFutures `json:"b"`
	Asks   []obLevelFutures `json:"a"`
}

// engineLevelsFromPush converts wire levels to engine levels (contracts → decimal).
func engineLevelsFromPush(src []obLevelFutures) []orderbook.Level {
	var out []orderbook.Level = make([]orderbook.Level, len(src))
	var i int
	for i = 0; i < len(src); i++ {
		out[i] = orderbook.Level{Price: mustDecimal(src[i].P), Size: decimal.NewFromInt(src[i].S)}
	}
	return out
}

// engineLevelsFromTypes converts REST OrderBook levels (decimal contracts) to
// engine levels — used to seed the engine from a REST snapshot.
func engineLevelsFromTypes(src []types.OrderBookLevel) []orderbook.Level {
	var out []orderbook.Level = make([]orderbook.Level, len(src))
	var i int
	for i = 0; i < len(src); i++ {
		out[i] = orderbook.Level{Price: src[i].Price, Size: src[i].Size}
	}
	return out
}

// typesLevelsFromEngine converts engine levels back to public OrderBook levels.
func typesLevelsFromEngine(src []orderbook.Level) []types.OrderBookLevel {
	var out []types.OrderBookLevel = make([]types.OrderBookLevel, len(src))
	var i int
	for i = 0; i < len(src); i++ {
		out[i] = types.OrderBookLevel{Price: src[i].Price, Size: src[i].Size}
	}
	return out
}

/*
WatchOrderBook maintains a local L2 order book for a contract from the
futures.order_book_update incremental channel, backed by REST snapshots for
priming and resync (see the orderbook package). The handler receives the full
top-`level` book (sizes in contracts, matching GetOrderBook) on every clean
update; gaps trigger an automatic REST resync.

interval is the Gate push frequency ("100ms"/"1000ms"; default "100ms"); level is
the Gate depth (20/50/100; default 100) and also caps the delivered depth. The
stream lives until ctx is cancelled.
*/
func (s *StreamClient) WatchOrderBook(ctx context.Context, contract string, interval string, level int, handler func(types.OrderBook), errHandler func(error)) error {
	if interval == "" {
		interval = defaultOrderBookInterval
	}
	if level <= 0 {
		level = defaultOrderBookLevel
	}
	var cfg gate.Config = s.c.parent.Config()
	var eng *orderbook.Engine = orderbook.NewEngine(contract, cfg.Orderbook.MaxDepth)

	var snapFn func(context.Context) (orderbook.Snapshot, error) = func(ctx context.Context) (orderbook.Snapshot, error) {
		var book types.OrderBook
		var err error
		book, err = s.c.MarketData().GetOrderBook(ctx, contract, level)
		if err != nil {
			return orderbook.Snapshot{}, err
		}
		return orderbook.Snapshot{
			Symbol: contract,
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
		Payload: []string{contract, interval, strconv.Itoa(level)},
		Reset:   func() { drv.Reset(ctx) },
		Handler: func(event string, result []byte) {
			var p orderBookUpdatePush
			// U/u differ only by case → case-sensitive decode.
			if err := codec.UnmarshalCaseSensitive(result, &p); err != nil {
				s.logParse("order_book_update", err)
				return
			}
			if p.S != "" && p.S != contract {
				return
			}
			drv.PushDelta(ctx, orderbook.Delta{
				Symbol: contract,
				FirstU: p.FirstU,
				LastU:  p.LastU,
				Bids:   engineLevelsFromPush(p.Bids),
				Asks:   engineLevelsFromPush(p.Asks),
				TsMs:   p.T,
			})
		},
	})
}

// ---- private streams -------------------------------------------------------

// WatchOrders subscribes to the account's order updates for a contract. Pass an
// empty contract to receive updates for all contracts.
func (s *StreamClient) WatchOrders(ctx context.Context, contract string, handler func([]types.OrderInfo), errHandler func(error)) error {
	var payload []string
	var err error
	payload, err = s.privatePayload(ctx, contract)
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
			var payloads []futuresOrderPayload
			if err := codec.Unmarshal(result, &payloads); err != nil {
				s.logParse("orders", err)
				return
			}
			var out []types.OrderInfo = make([]types.OrderInfo, 0, len(payloads))
			var i int
			for i = 0; i < len(payloads); i++ {
				if contract != "" && payloads[i].Contract != contract {
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

// WatchPositions subscribes to the account's position updates for a contract.
// Pass an empty contract to receive updates for all contracts.
func (s *StreamClient) WatchPositions(ctx context.Context, contract string, handler func([]types.PositionInfo), errHandler func(error)) error {
	var payload []string
	var err error
	payload, err = s.privatePayload(ctx, contract)
	if err != nil {
		invokeErr(errHandler, err)
		return err
	}
	var conn *ws.Conn = s.connection()
	conn.Start(ctx)
	return conn.Subscribe(&ws.Subscription{
		Channel: chanPositions,
		Payload: payload,
		Private: true,
		Handler: func(event string, result []byte) {
			var payloads []positionPayload
			if err := codec.Unmarshal(result, &payloads); err != nil {
				s.logParse("positions", err)
				return
			}
			var out []types.PositionInfo = make([]types.PositionInfo, 0, len(payloads))
			var i int
			for i = 0; i < len(payloads); i++ {
				if contract != "" && payloads[i].Contract != contract {
					continue
				}
				out = append(out, positionInfoFromPayload(&payloads[i], nil))
			}
			if len(out) > 0 {
				handler(out)
			}
		},
	})
}

// privatePayload builds the [user_id, contract] payload for a private channel,
// resolving the account user id once. Uses "!all" when contract is empty.
func (s *StreamClient) privatePayload(ctx context.Context, contract string) ([]string, error) {
	if !s.c.signerEnabled() {
		return nil, gate.NewError(gate.ErrorKindAuth, "", "stream: private channel requires API credentials", nil)
	}
	var uid string
	var err error
	uid, err = s.ensureUserID(ctx)
	if err != nil {
		return nil, err
	}
	var c string = contract
	if c == "" {
		c = allContracts
	}
	return []string{uid, c}, nil
}

// ensureUserID fetches and caches the account user id (GET /futures/{settle}/accounts).
func (s *StreamClient) ensureUserID(ctx context.Context) (string, error) {
	s.userOnce.Do(func() {
		var resp rest.Response
		var err error
		resp, _, err = s.c.rest().Do(ctx, rest.Options{
			Method: "GET",
			Path:   s.c.basePath() + "/accounts",
			Signed: true,
			Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
		})
		if err != nil {
			s.userErr = err
			return
		}
		var acc struct {
			User int64 `json:"user"`
		}
		if err = resp.UnmarshalData(&acc); err != nil {
			s.userErr = gate.NewError(gate.ErrorKindUnknown, "", "stream.ensureUserID: parse", err)
			return
		}
		if acc.User == 0 {
			s.userErr = gate.NewError(gate.ErrorKindUnknown, "", "stream.ensureUserID: empty user id", nil)
			return
		}
		s.userID = strconv.FormatInt(acc.User, 10)
	})
	return s.userID, s.userErr
}

func (s *StreamClient) logParse(channel string, err error) {
	s.c.logger().Warn("stream: parse push failed", gate.Str("channel", channel), gate.Err(err))
}

func invokeErr(errHandler func(error), err error) {
	if errHandler != nil {
		errHandler(err)
	}
}
