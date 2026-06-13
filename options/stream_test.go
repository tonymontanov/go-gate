/*
FILE: options/stream_test.go

DESCRIPTION:
WebSocket integration tests for the options StreamClient against a local
gorilla/websocket server (httptest). They verify the subscribe message format
(channel + payload) and end-to-end dispatch for one public channel
(options.contract_tickers) and one private channel (options.positions, including
the lazy user-id fetch via GET /options/accounts and the auth requirement). No
external network.
*/

package options

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
	"github.com/tonymontanov/go-gate/v2/options/types"
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
		WS: gate.WsConfig{OptionsURL: wsEndpoint},
	})
	if err != nil {
		t.Fatalf("gate.NewClient: %v", err)
	}
	return NewClient(parent)
}

// newStreamTestClientPrivate builds a credentialed Client pointed at both a WS
// endpoint and a REST base URL (the private channels resolve the user id from
// GET /options/accounts).
func newStreamTestClientPrivate(t *testing.T, wsEndpoint, restBase string) *Client {
	t.Helper()
	var parent *gate.Client
	var err error
	parent, err = gate.NewClient(gate.Config{
		APIKey:    "test-key",
		SecretKey: "test-secret",
		REST:      gate.RestConfig{BaseURL: restBase},
		WS:        gate.WsConfig{OptionsURL: wsEndpoint},
	})
	if err != nil {
		t.Fatalf("gate.NewClient: %v", err)
	}
	return NewClient(parent)
}

func TestWatchContractTickers_Dispatch(t *testing.T) {
	var subCh chan string = make(chan string, 1)
	var push string = `{"time":1700000000,"channel":"options.contract_tickers","event":"update",` +
		`"result":{"name":"` + testContract + `","last_price":120,"mark_price":123.4,"mark_iv":0.65,` +
		`"bid1_price":119,"bid1_size":5,"ask1_price":121,"ask1_size":7,"delta":0.55,"theta":-4.5,"position_size":42}}`
	var srv *httptest.Server = wsTestServer(subCh, push)
	defer srv.Close()

	var o *Client = newStreamTestClient(t, wsURL(srv.URL))
	defer func() { _ = o.Stream().Close() }()

	var ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	var got chan types.Ticker = make(chan types.Ticker, 1)
	var err error
	err = o.Stream().WatchContractTickers(ctx, testContract, func(tk types.Ticker) {
		select {
		case got <- tk:
		default:
		}
	}, func(error) {})
	if err != nil {
		t.Fatalf("WatchContractTickers: %v", err)
	}

	select {
	case sub := <-subCh:
		if !strings.Contains(sub, `"channel":"options.contract_tickers"`) || !strings.Contains(sub, testContract) {
			t.Fatalf("unexpected subscribe message: %s", sub)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("server did not receive subscribe")
	}

	select {
	case tk := <-got:
		if tk.Contract != testContract || !tk.MarkPrice.Equal(mustDec("123.4")) || !tk.MarkIv.Equal(mustDec("0.65")) {
			t.Fatalf("ticker mismatch: %+v", tk)
		}
		if !tk.Bid1Price.Equal(mustDec("119")) || !tk.Delta.Equal(mustDec("0.55")) {
			t.Fatalf("bid/delta mismatch: %s / %s", tk.Bid1Price, tk.Delta)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("handler not invoked")
	}
}

func TestWatchPositions_PrivateRoundTrip(t *testing.T) {
	// REST: the private channel resolves the account user id once.
	var restSrv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"user":12345,"currency":"USDT"}`))
	}))
	defer restSrv.Close()

	// WS: a positions push with bare-number decimals (the live WS form).
	var subCh chan string = make(chan string, 1)
	var push string = `{"time":1700000000,"channel":"options.positions","event":"update",` +
		`"result":[{"contract":"` + testContract + `","size":-12,"entry_price":0.0611,"mark_price":0.061,` +
		`"unrealised_pnl":-0.7,"realised_pnl":1.2,"delta":-6.5,"mark_iv":0.6,"update_time":1700000000}]}`
	var wsSrv *httptest.Server = wsTestServer(subCh, push)
	defer wsSrv.Close()

	var o *Client = newStreamTestClientPrivate(t, wsURL(wsSrv.URL), restSrv.URL)
	defer func() { _ = o.Stream().Close() }()

	var ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	var got chan []types.PositionInfo = make(chan []types.PositionInfo, 1)
	var err error
	err = o.Stream().WatchPositions(ctx, testContract, func(p []types.PositionInfo) {
		select {
		case got <- p:
		default:
		}
	}, func(error) {})
	if err != nil {
		t.Fatalf("WatchPositions: %v", err)
	}

	// Subscribe message format: channel + [user_id, contract].
	select {
	case sub := <-subCh:
		if !strings.Contains(sub, `"channel":"options.positions"`) ||
			!strings.Contains(sub, `"12345"`) || !strings.Contains(sub, testContract) {
			t.Fatalf("unexpected subscribe message: %s", sub)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("server did not receive subscribe")
	}

	select {
	case ps := <-got:
		if len(ps) != 1 || ps[0].Side != types.SideTypeSell || !ps[0].Size.Equal(mustDec("12")) {
			t.Fatalf("position mismatch: %+v", ps)
		}
		if !ps[0].EntryPrice.Equal(mustDec("0.0611")) || !ps[0].Delta.Equal(mustDec("-6.5")) {
			t.Fatalf("entry/delta mismatch: %s / %s", ps[0].EntryPrice, ps[0].Delta)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("positions handler not invoked")
	}
}

func TestWatchPositions_RequiresCredentials(t *testing.T) {
	var o *Client = newStreamTestClient(t, "ws://127.0.0.1:0")
	defer func() { _ = o.Stream().Close() }()

	var gotErr error
	var err error
	err = o.Stream().WatchPositions(context.Background(), testContract,
		func([]types.PositionInfo) {}, func(e error) { gotErr = e })
	if err == nil || !gate.IsAuth(err) {
		t.Fatalf("expected auth error without credentials, got %v", err)
	}
	if gotErr == nil {
		t.Fatalf("errHandler should have been invoked")
	}
}
