/*
FILE: spot/market_account_test.go

DESCRIPTION:
Contract tests for the spot MarketDataClient and AccountClient against an
httptest.Server returning real-shaped Gate spot JSON (captured live from prod
2026-06-09). They pin the spot-specific wire shapes:
  - currency_pairs: amount_precision/precision + min_base/min_quote;
  - order_book: ["price","amount"] string levels, epoch-ms current/update;
  - candlesticks: string array column order [t,quote_vol,close,high,low,open,
    base_vol,window_closed];
  - tickers: lowest_ask/highest_bid + base/quote volume;
  - accounts: per-currency available/locked balances.
*/

package spot

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tonymontanov/go-gate/v2/spot/types"
)

func TestGetCurrencyPair_Parse(t *testing.T) {
	var gotPath string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{"id":"BTC_USDT","base":"BTC","base_name":"Bitcoin","quote":"USDT","quote_name":"Tether",
			"fee":"0.2","min_base_amount":"0.000001","min_quote_amount":"3","max_base_amount":"100","max_quote_amount":"5000000",
			"amount_precision":6,"precision":1,"trade_status":"tradable","market_order_max_stock":"63","market_order_max_money":"5000000"}`)
	}))
	defer srv.Close()

	var sp *Client = newSpotTestClient(t, srv.URL)
	var info types.SymbolInfo
	var err error
	info, err = sp.MarketData().GetCurrencyPair(context.Background(), "BTC_USDT")
	if err != nil {
		t.Fatalf("GetCurrencyPair: %v", err)
	}
	if gotPath != "/spot/currency_pairs/BTC_USDT" {
		t.Fatalf("path: %q", gotPath)
	}
	if info.CurrencyPair != "BTC_USDT" || info.Base != "BTC" || info.Quote != "USDT" {
		t.Fatalf("pair/base/quote: %q/%q/%q", info.CurrencyPair, info.Base, info.Quote)
	}
	if info.AmountPrecision != 6 || info.PricePrecision != 1 {
		t.Fatalf("precision: amount=%d price=%d", info.AmountPrecision, info.PricePrecision)
	}
	if !info.MinBaseAmount.Equal(mustDec("0.000001")) || !info.MinQuoteAmount.Equal(mustDec("3")) {
		t.Fatalf("min amounts: base=%s quote=%s", info.MinBaseAmount, info.MinQuoteAmount)
	}
	if !info.MarketOrderMaxBase.Equal(mustDec("63")) || info.TradeStatus != "tradable" {
		t.Fatalf("market max / status: %s / %q", info.MarketOrderMaxBase, info.TradeStatus)
	}
}

func TestGetOrderBook_StringLevels_MsTimestamps(t *testing.T) {
	var gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = io.WriteString(w, `{"id":37576606891,"current":1781017457562,"update":1781017457560,
			"asks":[["61453.4","0.645203"],["61453.9","0.00585"]],"bids":[["61453.3","0.008231"]]}`)
	}))
	defer srv.Close()

	var sp *Client = newSpotTestClient(t, srv.URL)
	var book types.OrderBook
	var err error
	book, err = sp.MarketData().GetOrderBook(context.Background(), "BTC_USDT", 2)
	if err != nil {
		t.Fatalf("GetOrderBook: %v", err)
	}
	if gotQuery != "currency_pair=BTC_USDT&limit=2&with_id=true" {
		t.Fatalf("query: %q", gotQuery)
	}
	if book.ID != 37576606891 {
		t.Fatalf("id: %d", book.ID)
	}
	// current/update are already epoch-ms (>= 1e12) → taken as-is.
	if book.CurrentMs != 1781017457562 || book.UpdateMs != 1781017457560 {
		t.Fatalf("ts: current=%d update=%d", book.CurrentMs, book.UpdateMs)
	}
	if len(book.Asks) != 2 || len(book.Bids) != 1 {
		t.Fatalf("levels: asks=%d bids=%d", len(book.Asks), len(book.Bids))
	}
	if !book.Asks[0].Price.Equal(mustDec("61453.4")) || !book.Asks[0].Amount.Equal(mustDec("0.645203")) {
		t.Fatalf("ask[0]: %s x %s", book.Asks[0].Price, book.Asks[0].Amount)
	}
	if !book.Bids[0].Price.Equal(mustDec("61453.3")) {
		t.Fatalf("bid[0] price: %s", book.Bids[0].Price)
	}
}

func TestGetCandlesticks_ColumnOrder(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[["1781017200","9720096.25868970","61435.6","61553.8","61326.7","61542.9","158.23058500","false"]]`)
	}))
	defer srv.Close()

	var sp *Client = newSpotTestClient(t, srv.URL)
	var candles []types.Candle
	var err error
	candles, err = sp.MarketData().GetCandlesticks(context.Background(), "BTC_USDT", types.CandleInterval5m, 1)
	if err != nil {
		t.Fatalf("GetCandlesticks: %v", err)
	}
	if len(candles) != 1 {
		t.Fatalf("expected 1 candle, got %d", len(candles))
	}
	var c types.Candle = candles[0]
	if c.OpenTimeMs != 1781017200000 {
		t.Fatalf("openTimeMs: %d", c.OpenTimeMs)
	}
	if !c.Close.Equal(mustDec("61435.6")) || !c.High.Equal(mustDec("61553.8")) ||
		!c.Low.Equal(mustDec("61326.7")) || !c.Open.Equal(mustDec("61542.9")) {
		t.Fatalf("OHLC: o=%s h=%s l=%s c=%s", c.Open, c.High, c.Low, c.Close)
	}
	if !c.BaseVolume.Equal(mustDec("158.23058500")) || !c.QuoteVolume.Equal(mustDec("9720096.25868970")) {
		t.Fatalf("volumes: base=%s quote=%s", c.BaseVolume, c.QuoteVolume)
	}
	if c.WindowClosed {
		t.Fatalf("windowClosed should be false")
	}
}

func TestGetTickers_Parse(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{"currency_pair":"BTC_USDT","last":"61446.8","lowest_ask":"61457","lowest_size":"4.648414",
			"highest_bid":"61456.9","highest_size":"2.856168","change_percentage":"-3.96","base_volume":"14576.78",
			"quote_volume":"916379113.29","high_24h":"64212.8","low_24h":"61144.4"}]`)
	}))
	defer srv.Close()

	var sp *Client = newSpotTestClient(t, srv.URL)
	var tickers []types.Ticker
	var err error
	tickers, err = sp.MarketData().GetTickers(context.Background(), "BTC_USDT")
	if err != nil {
		t.Fatalf("GetTickers: %v", err)
	}
	if len(tickers) != 1 {
		t.Fatalf("expected 1 ticker, got %d", len(tickers))
	}
	var tk types.Ticker = tickers[0]
	if tk.CurrencyPair != "BTC_USDT" {
		t.Fatalf("pair: %q", tk.CurrencyPair)
	}
	if !tk.LowestAsk.Equal(mustDec("61457")) || !tk.HighestBid.Equal(mustDec("61456.9")) {
		t.Fatalf("bbo: ask=%s bid=%s", tk.LowestAsk, tk.HighestBid)
	}
	if !tk.BaseVolume.Equal(mustDec("14576.78")) || !tk.High24h.Equal(mustDec("64212.8")) {
		t.Fatalf("vol/high: %s / %s", tk.BaseVolume, tk.High24h)
	}
}

func TestGetBalances_AndGetBalance(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{"currency":"USDT","available":"1000.5","locked":"0.5"},{"currency":"BTC","available":"0.1","locked":"0"}]`)
	}))
	defer srv.Close()

	var sp *Client = newSpotTestClient(t, srv.URL)
	var bals []types.Balance
	var err error
	bals, err = sp.Account().GetBalances(context.Background())
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	if len(bals) != 2 {
		t.Fatalf("expected 2 balances, got %d", len(bals))
	}
	if bals[0].Currency != "USDT" || !bals[0].Available.Equal(mustDec("1000.5")) || !bals[0].Locked.Equal(mustDec("0.5")) {
		t.Fatalf("usdt: %+v", bals[0])
	}

	var b types.Balance
	b, err = sp.Account().GetBalance(context.Background(), "BTC")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if b.Currency != "BTC" || !b.Available.Equal(mustDec("0.1")) {
		t.Fatalf("btc balance: %+v", b)
	}
}

func TestGetBalance_MissingCurrency_ReturnsZero(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	}))
	defer srv.Close()

	var sp *Client = newSpotTestClient(t, srv.URL)
	var b types.Balance
	var err error
	b, err = sp.Account().GetBalance(context.Background(), "DOGE")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if b.Currency != "DOGE" || !b.Available.IsZero() || !b.Locked.IsZero() {
		t.Fatalf("expected zero DOGE balance, got %+v", b)
	}
}
