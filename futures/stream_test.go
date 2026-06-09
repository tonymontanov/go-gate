/*
FILE: futures/stream_test.go

DESCRIPTION:
WebSocket integration tests for the futures StreamClient against a local
gorilla/websocket server (httptest). They verify the subscribe message format
(channel + payload), end-to-end dispatch, and — critically — that the
case-distinct book_ticker keys b/B/a/A are decoded correctly (bid vs ask price
and size are not aliased). No external network.
*/

package futures

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/futures/types"
)

var testUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// wsTestServer upgrades to WS, captures the first subscribe message into subCh,
// then pushes each frame in pushes. It keeps reading (draining pings) until the
// client disconnects.
func wsTestServer(subCh chan<- string, pushes ...string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var conn *websocket.Conn
		var err error
		conn, err = testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		for {
			var _, msg, rerr = conn.ReadMessage()
			if rerr != nil {
				return
			}
			if bytes.Contains(msg, []byte(`"event":"subscribe"`)) {
				select {
				case subCh <- string(msg):
				default:
				}
				var i int
				for i = 0; i < len(pushes); i++ {
					if werr := conn.WriteMessage(websocket.TextMessage, []byte(pushes[i])); werr != nil {
						return
					}
				}
			}
		}
	}))
}

func wsURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http")
}

func newStreamTestClient(t *testing.T, wsEndpoint string) *Client {
	t.Helper()
	var parent *gate.Client
	var err error
	parent, err = gate.NewClient(gate.Config{
		Settle: "usdt",
		WS:     gate.WsConfig{FuturesURL: wsEndpoint},
	})
	if err != nil {
		t.Fatalf("gate.NewClient: %v", err)
	}
	return NewClient(parent)
}

// newStreamTestClientREST builds a Client pointed at both a WS endpoint and a
// REST base URL (the order-book stream primes from REST).
func newStreamTestClientREST(t *testing.T, wsEndpoint, restBase string) *Client {
	t.Helper()
	var parent *gate.Client
	var err error
	parent, err = gate.NewClient(gate.Config{
		Settle: "usdt",
		REST:   gate.RestConfig{BaseURL: restBase},
		WS:     gate.WsConfig{FuturesURL: wsEndpoint},
	})
	if err != nil {
		t.Fatalf("gate.NewClient: %v", err)
	}
	return NewClient(parent)
}

// TestWatchOrderBook_PrimesAndAppliesDelta exercises the full incremental path:
// REST snapshot prime + a buffered order_book_update delta applied via the engine.
func TestWatchOrderBook_PrimesAndAppliesDelta(t *testing.T) {
	// REST snapshot: id=100, two levels per side (sizes in contracts).
	var restSrv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":100,"current":1700000000.5,"update":1700000000.4,` +
			`"asks":[{"p":"30001","s":5},{"p":"30002","s":7}],` +
			`"bids":[{"p":"30000","s":3},{"p":"29999","s":9}]}`))
	}))
	defer restSrv.Close()

	// WS delta: U=u=101 — update top bid to 50, delete best ask (s:0).
	var subCh chan string = make(chan string, 1)
	var push string = `{"time":1700000000,"channel":"futures.order_book_update","event":"update",` +
		`"result":{"t":1700000000600,"s":"BTC_USDT","U":101,"u":101,` +
		`"b":[{"p":"30000","s":50}],"a":[{"p":"30001","s":0}]}}`
	var wsSrv *httptest.Server = wsTestServer(subCh, push)
	defer wsSrv.Close()

	var fut *Client = newStreamTestClientREST(t, wsURL(wsSrv.URL), restSrv.URL)
	defer func() { _ = fut.Stream().Close() }()

	var ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	var got chan types.OrderBook = make(chan types.OrderBook, 4)
	var err error
	err = fut.Stream().WatchOrderBook(ctx, "BTC_USDT", "100ms", 10, func(ob types.OrderBook) {
		select {
		case got <- ob:
		default:
		}
	}, func(error) {})
	if err != nil {
		t.Fatalf("WatchOrderBook: %v", err)
	}

	// Subscribe format: channel + [contract, interval, level].
	select {
	case sub := <-subCh:
		if !strings.Contains(sub, `"channel":"futures.order_book_update"`) ||
			!strings.Contains(sub, `"BTC_USDT"`) || !strings.Contains(sub, `"100ms"`) {
			t.Fatalf("unexpected subscribe message: %s", sub)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("server did not receive subscribe")
	}

	// The book is delivered progressively: depending on whether the REST snapshot
	// or the WS delta lands first, the handler may receive the snapshot-only book
	// (id=100) before the post-delta book (id=101). Wait for the delta-applied one.
	var deadline = time.After(3 * time.Second)
	for {
		select {
		case ob := <-got:
			if ob.ID != 101 {
				continue // snapshot-only book; keep waiting for the delta-applied one
			}
			if len(ob.Bids) == 0 || !ob.Bids[0].Price.Equal(mustDec("30000")) || !ob.Bids[0].Size.Equal(mustDec("50")) {
				t.Fatalf("top bid = %+v, want 30000@50", ob.Bids)
			}
			if len(ob.Asks) == 0 || !ob.Asks[0].Price.Equal(mustDec("30002")) {
				t.Fatalf("best ask = %+v, want 30002 (30001 deleted)", ob.Asks)
			}
			return
		case <-deadline:
			t.Fatalf("post-delta order book (id=101) not delivered")
		}
	}
}

func TestWatchBookTicker_DispatchAndCaseSensitiveDecode(t *testing.T) {
	var subCh chan string = make(chan string, 1)
	var push string = `{"time":1700000000,"channel":"futures.book_ticker","event":"update",` +
		`"result":{"t":1700000000500,"u":42,"s":"BTC_USDT","b":"30000.1","B":5,"a":"30001.2","A":7}}`
	var srv *httptest.Server = wsTestServer(subCh, push)
	defer srv.Close()

	var fut *Client = newStreamTestClient(t, wsURL(srv.URL))
	defer func() { _ = fut.Stream().Close() }()

	var ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	var got chan types.BookTicker = make(chan types.BookTicker, 1)
	var err error
	err = fut.Stream().WatchBookTicker(ctx, "BTC_USDT", func(bt types.BookTicker) {
		select {
		case got <- bt:
		default:
		}
	}, func(error) {})
	if err != nil {
		t.Fatalf("WatchBookTicker: %v", err)
	}

	// Subscribe message format.
	select {
	case sub := <-subCh:
		if !strings.Contains(sub, `"channel":"futures.book_ticker"`) || !strings.Contains(sub, `"BTC_USDT"`) {
			t.Fatalf("unexpected subscribe message: %s", sub)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("server did not receive subscribe")
	}

	// Dispatched BBO with correctly de-aliased b/B/a/A.
	select {
	case bt := <-got:
		if bt.Contract != "BTC_USDT" || bt.UpdateID != 42 || bt.Ts != 1700000000500 {
			t.Fatalf("meta mismatch: %+v", bt)
		}
		if !bt.BidPrice.Equal(mustDec("30000.1")) || !bt.BidSize.Equal(mustDec("5")) {
			t.Fatalf("bid mismatch: %s / %s", bt.BidPrice, bt.BidSize)
		}
		if !bt.AskPrice.Equal(mustDec("30001.2")) || !bt.AskSize.Equal(mustDec("7")) {
			t.Fatalf("ask mismatch: %s / %s", bt.AskPrice, bt.AskSize)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("handler not invoked")
	}
}

func TestWatchTrades_Dispatch(t *testing.T) {
	var subCh chan string = make(chan string, 1)
	var push string = `{"time":1700000000,"channel":"futures.trades","event":"update",` +
		`"result":[{"id":1,"create_time_ms":1700000000500,"contract":"BTC_USDT","size":-3,"price":"30000"}]}`
	var srv *httptest.Server = wsTestServer(subCh, push)
	defer srv.Close()

	var fut *Client = newStreamTestClient(t, wsURL(srv.URL))
	defer func() { _ = fut.Stream().Close() }()
	var ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	var got chan types.PublicTrade = make(chan types.PublicTrade, 1)
	if err := fut.Stream().WatchTrades(ctx, "BTC_USDT", func(tr types.PublicTrade) {
		select {
		case got <- tr:
		default:
		}
	}, func(error) {}); err != nil {
		t.Fatalf("WatchTrades: %v", err)
	}

	select {
	case tr := <-got:
		if tr.Side != types.SideTypeSell || !tr.Size.Equal(mustDec("3")) || !tr.Price.Equal(mustDec("30000")) {
			t.Fatalf("trade mismatch: %+v", tr)
		}
		if tr.Ts != 1700000000500 {
			t.Fatalf("ts: %d", tr.Ts)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("trade handler not invoked")
	}
}

func TestWatchPositions_RequiresCredentials(t *testing.T) {
	var fut *Client = newStreamTestClient(t, "ws://127.0.0.1:0")
	defer func() { _ = fut.Stream().Close() }()

	var gotErr error
	var err error
	err = fut.Stream().WatchPositions(context.Background(), "BTC_USDT",
		func([]types.PositionInfo) {}, func(e error) { gotErr = e })
	if err == nil || !gate.IsAuth(err) {
		t.Fatalf("expected auth error without credentials, got %v", err)
	}
	if gotErr == nil {
		t.Fatalf("errHandler should have been invoked")
	}
}
