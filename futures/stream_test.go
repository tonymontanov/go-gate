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
