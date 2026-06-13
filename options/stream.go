/*
FILE: options/stream.go

DESCRIPTION:
WebSocket subscription sub-client for the Gate Options section. One shared
ws.Conn carries both public and private channels (Gate authenticates per
subscribe). Public: WatchContractTickers (per-contract IV/greeks ticker),
WatchUnderlyingPrice (index price), WatchTrades, WatchOrderBook (L2 via the
shared orderbook engine). Private: WatchOrders, WatchPositions, WatchUserTrades
(require credentials; payload needs the account user id, fetched once lazily from
GET /options/accounts).

WS HOST & CHANNELS (CALIBRATION): the options socket is a dedicated host
(Config.WS.OptionsURL, default wss://op-ws.gateio.live/v4/ws) and the channels
live in the "options.*" namespace:
  - options.contract_tickers  (payload [contract])
  - options.ul_price          (payload [underlying])
  - options.trades            (payload [contract])
  - options.order_book_update (payload [contract, interval, level])
  - options.orders / options.positions / options.usertrades (private,
    payload [user_id, contract|!all])
Channel names, payload shapes and push field names follow Gate's options WS docs
and MUST be verified against a live options environment.

GENERAL PATTERN PER WATCH*:
  1. Lazily create + Start the shared ws.Conn under ctx (first ctx wins).
  2. Register a subscription whose handler parses the push and filters by
     contract/underlying, then invokes the user callback.
  3. Return nil, or a setup error (e.g. private channel without credentials),
     which is also delivered to errHandler.

Local parse errors are logged and dropped (not sent to errHandler) to avoid noise
on a single malformed frame. The stream lives until the ctx passed to the first
Watch* is cancelled, or the section's connection is closed.
*/

package options

import (
	"context"
	"strconv"
	"sync"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
	"github.com/tonymontanov/go-gate/v2/internal/ws"
	"github.com/tonymontanov/go-gate/v2/options/types"
	"github.com/tonymontanov/go-gate/v2/orderbook"

	"github.com/shopspring/decimal"
)

// Options WS channels (options.* namespace). CALIBRATION: confirm the names and
// payload shapes live (host wss://op-ws.gateio.live/v4/ws).
const (
	chanContractTickers = "options.contract_tickers"
	chanUnderlyingPrice = "options.ul_price"
	chanTrades          = "options.trades"
	chanOrderBook       = "options.order_book_update"
	chanOrders          = "options.orders"
	chanPositions       = "options.positions"
	chanUserTrades      = "options.usertrades"
	pingChannel         = "options.ping"
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
		URL:                     cfg.WS.OptionsURL,
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

// WatchContractTickers subscribes to the per-contract ticker (last/mark price,
// the IV surface and greeks) for a contract.
func (s *StreamClient) WatchContractTickers(ctx context.Context, contract string, handler func(types.Ticker), errHandler func(error)) error {
	var conn *ws.Conn = s.connection()
	conn.Start(ctx)
	return conn.Subscribe(&ws.Subscription{
		Channel: chanContractTickers,
		Payload: []string{contract},
		Handler: func(event string, result []byte) {
			// options.contract_tickers pushes a single ticker object.
			var p tickerPayload
			if err := codec.Unmarshal(result, &p); err != nil {
				s.logParse("contract_tickers", err)
				return
			}
			if p.Name != "" && p.Name != contract {
				return
			}
			handler(tickerFromPayload(&p, nil))
		},
	})
}

// underlyingPricePush — one options.ul_price event.
//
// CALIBRATION: the field names (underlying/price/time) follow Gate's options docs;
// verify live.
type underlyingPricePush struct {
	Underlying string            `json:"underlying"`
	Price      codec.FlexDecimal `json:"price"`
	Time       int64             `json:"time"`
	TimeMs     int64             `json:"time_ms"`
}

// WatchUnderlyingPrice subscribes to underlying index-price updates.
func (s *StreamClient) WatchUnderlyingPrice(ctx context.Context, underlying string, handler func(types.UnderlyingPrice), errHandler func(error)) error {
	var conn *ws.Conn = s.connection()
	conn.Start(ctx)
	return conn.Subscribe(&ws.Subscription{
		Channel: chanUnderlyingPrice,
		Payload: []string{underlying},
		Handler: func(event string, result []byte) {
			var p underlyingPricePush
			if err := codec.Unmarshal(result, &p); err != nil {
				s.logParse("ul_price", err)
				return
			}
			if p.Underlying != "" && p.Underlying != underlying {
				return
			}
			var ts int64 = p.TimeMs
			if ts == 0 {
				ts = secondsToMs(p.Time)
			}
			handler(types.UnderlyingPrice{
				Underlying: underlying,
				Price:      p.Price.Decimal,
				Ts:         ts,
			})
		},
	})
}

// WatchTrades subscribes to public trades for a contract.
func (s *StreamClient) WatchTrades(ctx context.Context, contract string, handler func(types.Trade), errHandler func(error)) error {
	var conn *ws.Conn = s.connection()
	conn.Start(ctx)
	return conn.Subscribe(&ws.Subscription{
		Channel: chanTrades,
		Payload: []string{contract},
		Handler: func(event string, result []byte) {
			var pushes []tradePayload
			if err := codec.Unmarshal(result, &pushes); err != nil {
				s.logParse("trades", err)
				return
			}
			var i int
			for i = 0; i < len(pushes); i++ {
				if pushes[i].Contract != "" && pushes[i].Contract != contract {
					continue
				}
				handler(tradeFromPayload(&pushes[i], nil))
			}
		},
	})
}

// ---- order book (incremental depth) ----------------------------------------

// obLevel is one level inside an options.order_book_update push: price as a
// string, size as an integer number of contracts (0 means remove the level).
type obLevel struct {
	P string `json:"p"`
	S int64  `json:"s"`
}

// orderBookUpdatePush is one options.order_book_update event. U/u are the first
// and last update ids covered by the event and differ only by case, so this push
// MUST be decoded case-sensitively.
//
// CALIBRATION: the field names (t/s/U/u/b/a) and the {p,s} level shape follow
// Gate's documented depth-update format (shared with futures); verify live.
type orderBookUpdatePush struct {
	T      int64     `json:"t"`
	S      string    `json:"s"`
	FirstU int64     `json:"U"`
	LastU  int64     `json:"u"`
	Bids   []obLevel `json:"b"`
	Asks   []obLevel `json:"a"`
}

func engineLevelsFromPush(src []obLevel) []orderbook.Level {
	var out []orderbook.Level = make([]orderbook.Level, len(src))
	var i int
	for i = 0; i < len(src); i++ {
		out[i] = orderbook.Level{Price: mustDecimal(src[i].P), Size: decimal.NewFromInt(src[i].S)}
	}
	return out
}

func engineLevelsFromTypes(src []types.OrderBookLevel) []orderbook.Level {
	var out []orderbook.Level = make([]orderbook.Level, len(src))
	var i int
	for i = 0; i < len(src); i++ {
		out[i] = orderbook.Level{Price: src[i].Price, Size: src[i].Size}
	}
	return out
}

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
options.order_book_update incremental channel, backed by REST snapshots for
priming and resync (see the orderbook package). The handler receives the full
top-`level` book (sizes in contracts, matching GetOrderBook) on every clean
update; gaps trigger an automatic REST resync.

interval is the Gate push frequency ("100ms"/"1000ms"; default "100ms"); level is
the Gate depth (default 100) and also caps the delivered depth. The stream lives
until ctx is cancelled.
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
			var payloads []optionsOrderPayload
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

// userTradePush — one element of an options.usertrades push. Over WS Gate sends
// the trade and order ids as strings. CALIBRATION: verify field names live.
type userTradePush struct {
	ID           string  `json:"id"`
	CreateTime   float64 `json:"create_time"`
	CreateTimeMs float64 `json:"create_time_ms"`
	Contract     string  `json:"contract"`
	OrderID      string  `json:"order"`
	Size         int64   `json:"size"`
	Price        string  `json:"price"`
	Role         string  `json:"role"`
	Text         string  `json:"text"`
}

// WatchUserTrades subscribes to the account's own fills for a contract. Pass an
// empty contract to receive fills for all contracts.
func (s *StreamClient) WatchUserTrades(ctx context.Context, contract string, handler func([]types.UserTrade), errHandler func(error)) error {
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
				if contract != "" && pushes[i].Contract != contract {
					continue
				}
				out = append(out, types.UserTrade{
					ID:            pushes[i].ID,
					Contract:      pushes[i].Contract,
					OrderID:       pushes[i].OrderID,
					ClientOrderID: pushes[i].Text,
					Price:         mustDecimal(pushes[i].Price),
					Size:          decimalAbsInt(pushes[i].Size),
					Side:          sideFromSize(pushes[i].Size),
					Role:          pushes[i].Role,
					Ts:            epochMs(pushes[i].CreateTimeMs, pushes[i].CreateTime),
				})
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

// ensureUserID fetches and caches the account user id (GET /options/accounts).
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
