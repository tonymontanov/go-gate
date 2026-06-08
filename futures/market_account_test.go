/*
FILE: futures/market_account_test.go

DESCRIPTION:
Contract tests for the Account and MarketData sub-clients against httptest-served
Gate JSON. They pin contract-spec/position/orderbook/candle/ticker parsing
(decimal strings, signed position size, second→ms timestamps), the leverage and
dual_mode query encoding, and that ClosePosition issues a market close order.
*/

package futures

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tonymontanov/go-gate/v2/futures/types"
)

func TestGetContract_ParsesSpec(t *testing.T) {
	var gotPath string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{"name":"BTC_USDT","type":"direct","quanto_multiplier":"0.0001",
			"leverage_min":"1","leverage_max":"100","maintenance_rate":"0.005","mark_price":"30000.5",
			"index_price":"30001","last_price":"30000","order_price_round":"0.1","mark_price_round":"0.01",
			"order_size_min":1,"order_size_max":1000000,"order_price_deviate":"0.5","orders_limit":100,
			"funding_rate":"0.0001","funding_interval":28800,"in_delisting":false}`)
	}))
	defer srv.Close()

	var fut *Client = newFuturesTestClient(t, srv.URL)
	var info types.SymbolInfo
	var err error
	info, err = fut.MarketData().GetContract(context.Background(), "BTC_USDT")
	if err != nil {
		t.Fatalf("GetContract: %v", err)
	}
	if gotPath != "/futures/usdt/contracts/BTC_USDT" {
		t.Fatalf("path: %q", gotPath)
	}
	if !info.QuantoMultiplier.Equal(mustDec("0.0001")) {
		t.Fatalf("quanto: %s", info.QuantoMultiplier)
	}
	if !info.OrderPriceRound.Equal(mustDec("0.1")) || !info.OrderSizeMin.Equal(mustDec("1")) {
		t.Fatalf("tick/min: %s / %s", info.OrderPriceRound, info.OrderSizeMin)
	}
	if info.FundingIntervalSec != 28800 || info.OrdersLimit != 100 {
		t.Fatalf("funding/orders: %d / %d", info.FundingIntervalSec, info.OrdersLimit)
	}
}

func TestGetOrderBook_ParsesLevels(t *testing.T) {
	var gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = io.WriteString(w, `{"id":99,"current":1700000000.5,"update":1700000000.4,
			"asks":[{"p":"30001","s":5},{"p":"30002","s":7}],
			"bids":[{"p":"30000","s":3},{"p":"29999","s":9}]}`)
	}))
	defer srv.Close()

	var fut *Client = newFuturesTestClient(t, srv.URL)
	var book types.OrderBook
	var err error
	book, err = fut.MarketData().GetOrderBook(context.Background(), "BTC_USDT", 10)
	if err != nil {
		t.Fatalf("GetOrderBook: %v", err)
	}
	if book.ID != 99 || len(book.Asks) != 2 || len(book.Bids) != 2 {
		t.Fatalf("book shape: id=%d asks=%d bids=%d", book.ID, len(book.Asks), len(book.Bids))
	}
	if !book.Asks[0].Price.Equal(mustDec("30001")) || !book.Asks[0].Size.Equal(mustDec("5")) {
		t.Fatalf("ask[0]: %s/%s", book.Asks[0].Price, book.Asks[0].Size)
	}
	if book.CurrentMs != 1700000000500 {
		t.Fatalf("current ms: %d", book.CurrentMs)
	}
	// with_id=true must always be sent.
	if !strings.Contains(gotQuery, "with_id=true") || !strings.Contains(gotQuery, "contract=BTC_USDT") {
		t.Fatalf("query: %q", gotQuery)
	}
}

func TestGetCandlesticks_ParsesOHLC(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[
			{"t":1700000000,"v":120,"o":"100","h":"110","l":"95","c":"105","sum":"12600"},
			{"t":1700000060,"v":80,"o":"105","h":"108","l":"104","c":"107","sum":"8500"}
		]`)
	}))
	defer srv.Close()

	var fut *Client = newFuturesTestClient(t, srv.URL)
	var candles []types.Candle
	var err error
	candles, err = fut.MarketData().GetCandlesticks(context.Background(), "BTC_USDT", types.CandleInterval1m, 2)
	if err != nil {
		t.Fatalf("GetCandlesticks: %v", err)
	}
	if len(candles) != 2 {
		t.Fatalf("expected 2 candles, got %d", len(candles))
	}
	if candles[0].OpenTimeMs != 1700000000000 || !candles[0].Close.Equal(mustDec("105")) {
		t.Fatalf("candle0: %d / %s", candles[0].OpenTimeMs, candles[0].Close)
	}
	if !candles[0].Volume.Equal(mustDec("120")) {
		t.Fatalf("volume: %s", candles[0].Volume)
	}
}

func TestGetTickers_ParsesArray(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{"contract":"BTC_USDT","last":"30000","mark_price":"30001","index_price":"30002",
			"highest_bid":"29999","lowest_ask":"30001","change_percentage":"-1.5","total_size":"123456",
			"volume_24h":"1000","funding_rate":"0.0001","funding_rate_indicative":"0.00012"}]`)
	}))
	defer srv.Close()

	var fut *Client = newFuturesTestClient(t, srv.URL)
	var tickers []types.Ticker
	var err error
	tickers, err = fut.MarketData().GetTickers(context.Background(), "BTC_USDT")
	if err != nil {
		t.Fatalf("GetTickers: %v", err)
	}
	if len(tickers) != 1 {
		t.Fatalf("expected 1 ticker, got %d", len(tickers))
	}
	if !tickers[0].HighestBid.Equal(mustDec("29999")) || !tickers[0].LowestAsk.Equal(mustDec("30001")) {
		t.Fatalf("bbo: %s / %s", tickers[0].HighestBid, tickers[0].LowestAsk)
	}
}

func TestGetPosition_SignedSizeToSide(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"contract":"BTC_USDT","size":-12,"leverage":"10","entry_price":"30000",
			"mark_price":"30100","liq_price":"33000","margin":"360","value":"3600","unrealised_pnl":"-12",
			"realised_pnl":"5","maintenance_rate":"0.005","mode":"single","update_time":1700000000}`)
	}))
	defer srv.Close()

	var fut *Client = newFuturesTestClient(t, srv.URL)
	var pos types.PositionInfo
	var err error
	pos, err = fut.Account().GetPosition(context.Background(), "BTC_USDT")
	if err != nil {
		t.Fatalf("GetPosition: %v", err)
	}
	if pos.Side != types.SideTypeSell || !pos.Size.Equal(mustDec("12")) {
		t.Fatalf("side/size: %q / %s", pos.Side, pos.Size)
	}
	if !pos.EntryPrice.Equal(mustDec("30000")) || pos.UpdatedAtMs != 1700000000000 {
		t.Fatalf("entry/updated: %s / %d", pos.EntryPrice, pos.UpdatedAtMs)
	}
}

func TestSetLeverage_QueryEncoding(t *testing.T) {
	var gotPath, gotQuery, gotMethod string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotMethod = r.URL.Path, r.URL.RawQuery, r.Method
		_, _ = io.WriteString(w, `{"contract":"BTC_USDT","size":0,"leverage":"20","entry_price":"0","mark_price":"0","liq_price":"0","margin":"0","value":"0","unrealised_pnl":"0","realised_pnl":"0","mode":"single"}`)
	}))
	defer srv.Close()

	var fut *Client = newFuturesTestClient(t, srv.URL)
	if err := fut.Account().SetLeverage(context.Background(), "BTC_USDT", 20); err != nil {
		t.Fatalf("SetLeverage: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/futures/usdt/positions/BTC_USDT/leverage" {
		t.Fatalf("request: %s %s", gotMethod, gotPath)
	}
	if gotQuery != "leverage=20" {
		t.Fatalf("query: %q", gotQuery)
	}
}

func TestSetPositionMode_DualModeQuery(t *testing.T) {
	var gotPath, gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	var fut *Client = newFuturesTestClient(t, srv.URL)
	// oneWayMode=true → dual_mode=false
	if err := fut.Account().SetPositionMode(context.Background(), true); err != nil {
		t.Fatalf("SetPositionMode: %v", err)
	}
	if gotPath != "/futures/usdt/dual_mode" || gotQuery != "dual_mode=false" {
		t.Fatalf("request: %s ? %s", gotPath, gotQuery)
	}
}

func TestClosePosition_IssuesMarketClose(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBody = decodeBody(t, r)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":5,"contract":"BTC_USDT","size":0,"price":"0","fill_price":"30000","status":"finished","finish_as":"filled","is_close":true}`)
	}))
	defer srv.Close()

	var fut *Client = newFuturesTestClient(t, srv.URL)
	if err := fut.Account().ClosePosition(context.Background(), "BTC_USDT"); err != nil {
		t.Fatalf("ClosePosition: %v", err)
	}
	if gotPath != "/futures/usdt/orders" {
		t.Fatalf("path: %q", gotPath)
	}
	if gotBody["close"] != true || gotBody["price"] != "0" || gotBody["tif"] != "ioc" {
		t.Fatalf("close order body: %+v", gotBody)
	}
}
