/*
FILE: options/market_account_test.go

DESCRIPTION:
Contract tests for the Account and MarketData sub-clients against httptest-served
Gate JSON. They pin contract-spec/position/account/ticker parsing (decimal
strings, signed position size, is_call→OptionType, second→ms timestamps), the
flat-position handling on POSITION_NOT_FOUND, and — critically — that the
position/account/ticker decimal fields decode from BOTH the REST string form and
the WS bare-number form via codec.FlexDecimal.
*/

package options

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/options/types"
)

func TestGetContract_ParsesSpec(t *testing.T) {
	var gotPath string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		// Greeks arrive as bare numbers, prices as strings — FlexDecimal handles both.
		_, _ = io.WriteString(w, `{"name":"`+testContract+`","underlying":"BTC_USDT","is_call":true,
			"expiration_time":1711699200,"strike_price":"50000","multiplier":"0.01",
			"order_price_round":"0.1","mark_price_round":"0.001","order_size_min":1,"order_size_max":100000,
			"maker_fee_rate":"0.0003","taker_fee_rate":"0.0005","ref_discount_rate":"0","ref_rebate_rate":"0",
			"mark_price":"123.4","last_price":"120","index_price":"50100","mark_iv":0.65,
			"delta":0.55,"gamma":0.0001,"vega":12.3,"theta":-4.5,"orders_limit":100,"in_delisting":false}`)
	}))
	defer srv.Close()

	var o *Client = newOptionsTestClient(t, srv.URL)
	var info types.SymbolInfo
	var err error
	info, err = o.MarketData().GetContract(context.Background(), testContract)
	if err != nil {
		t.Fatalf("GetContract: %v", err)
	}
	if gotPath != "/options/contracts/"+testContract {
		t.Fatalf("path: %q", gotPath)
	}
	if info.Underlying != "BTC_USDT" || !info.IsCall || info.OptionType != types.OptionTypeCall {
		t.Fatalf("underlying/call: %q / %v / %q", info.Underlying, info.IsCall, info.OptionType)
	}
	if !info.StrikePrice.Equal(mustDec("50000")) || !info.Multiplier.Equal(mustDec("0.01")) {
		t.Fatalf("strike/multiplier: %s / %s", info.StrikePrice, info.Multiplier)
	}
	if !info.OrderPriceRound.Equal(mustDec("0.1")) || info.ExpirationMs != 1711699200000 {
		t.Fatalf("tick/expiry: %s / %d", info.OrderPriceRound, info.ExpirationMs)
	}
	if !info.MakerFeeRate.Equal(mustDec("0.0003")) || !info.TakerFeeRate.Equal(mustDec("0.0005")) {
		t.Fatalf("fees: %s / %s", info.MakerFeeRate, info.TakerFeeRate)
	}
	if !info.Delta.Equal(mustDec("0.55")) || !info.Theta.Equal(mustDec("-4.5")) || !info.MarkIv.Equal(mustDec("0.65")) {
		t.Fatalf("greeks: delta=%s theta=%s iv=%s", info.Delta, info.Theta, info.MarkIv)
	}
}

func TestGetContracts_QueryEncoding(t *testing.T) {
	var gotPath, gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = io.WriteString(w, `[{"name":"`+testContract+`","underlying":"BTC_USDT","is_call":false,
			"expiration_time":1711699200,"strike_price":"50000","multiplier":"0.01","order_price_round":"0.1"}]`)
	}))
	defer srv.Close()

	var o *Client = newOptionsTestClient(t, srv.URL)
	var contracts []types.SymbolInfo
	var err error
	contracts, err = o.MarketData().GetContracts(context.Background(), "BTC_USDT", 1711699200)
	if err != nil {
		t.Fatalf("GetContracts: %v", err)
	}
	if gotPath != "/options/contracts" {
		t.Fatalf("path: %q", gotPath)
	}
	if !strings.Contains(gotQuery, "underlying=BTC_USDT") || !strings.Contains(gotQuery, "expiration=1711699200") {
		t.Fatalf("query: %q", gotQuery)
	}
	if len(contracts) != 1 || contracts[0].OptionType != types.OptionTypePut {
		t.Fatalf("contracts: %+v", contracts)
	}
}

func TestGetTickers_ParsesArray(t *testing.T) {
	var gotPath, gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = io.WriteString(w, `[{"name":"`+testContract+`","last_price":"120","mark_price":"123.4","index_price":"50100",
			"mark_iv":"0.65","bid1_price":"119","bid1_size":"5","bid_iv":"0.64","ask1_price":"121","ask1_size":"7",
			"ask_iv":"0.66","position_size":"42","delta":"0.55","gamma":"0.0001","vega":"12.3","theta":"-4.5"}]`)
	}))
	defer srv.Close()

	var o *Client = newOptionsTestClient(t, srv.URL)
	var tickers []types.Ticker
	var err error
	tickers, err = o.MarketData().GetTickers(context.Background(), "BTC_USDT")
	if err != nil {
		t.Fatalf("GetTickers: %v", err)
	}
	if gotPath != "/options/tickers" || !strings.Contains(gotQuery, "underlying=BTC_USDT") {
		t.Fatalf("path/query: %q / %q", gotPath, gotQuery)
	}
	if len(tickers) != 1 {
		t.Fatalf("expected 1 ticker, got %d", len(tickers))
	}
	if !tickers[0].Bid1Price.Equal(mustDec("119")) || !tickers[0].Ask1Iv.Equal(mustDec("0.66")) {
		t.Fatalf("bbo/iv: %s / %s", tickers[0].Bid1Price, tickers[0].Ask1Iv)
	}
	if !tickers[0].Delta.Equal(mustDec("0.55")) || !tickers[0].PositionSize.Equal(mustDec("42")) {
		t.Fatalf("delta/pos: %s / %s", tickers[0].Delta, tickers[0].PositionSize)
	}
}

func TestGetAccount_ParsesSingleObject(t *testing.T) {
	var gotPath string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{"user":12345,"currency":"USDT","total":"1000","position_value":"250",
			"equity":"1010","unrealised_pnl":"10","init_margin":"100","maint_margin":"50","order_margin":"20",
			"available":"880","bonus":"5"}`)
	}))
	defer srv.Close()

	var o *Client = newOptionsTestClient(t, srv.URL)
	var acc types.AccountInfo
	var err error
	acc, err = o.Account().GetAccount(context.Background())
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if gotPath != "/options/accounts" {
		t.Fatalf("path: %q", gotPath)
	}
	if acc.User != 12345 || acc.Currency != "USDT" {
		t.Fatalf("user/currency: %d / %q", acc.User, acc.Currency)
	}
	if !acc.Total.Equal(mustDec("1000")) || !acc.Available.Equal(mustDec("880")) || !acc.Equity.Equal(mustDec("1010")) {
		t.Fatalf("balances: total=%s avail=%s equity=%s", acc.Total, acc.Available, acc.Equity)
	}
}

func TestGetPositions_ParsesArray(t *testing.T) {
	var gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = io.WriteString(w, `[{"contract":"`+testContract+`","size":-12,"entry_price":"10","mark_price":"12",
			"unrealised_pnl":"-24","realised_pnl":"5","delta":"-6.5","gamma":"0.01","vega":"3.2","theta":"-1.1",
			"mark_iv":"0.6","pending_orders":2,"update_time":1700000000}]`)
	}))
	defer srv.Close()

	var o *Client = newOptionsTestClient(t, srv.URL)
	var positions []types.PositionInfo
	var err error
	positions, err = o.Account().GetPositions(context.Background(), "BTC_USDT")
	if err != nil {
		t.Fatalf("GetPositions: %v", err)
	}
	if !strings.Contains(gotQuery, "underlying=BTC_USDT") {
		t.Fatalf("query: %q", gotQuery)
	}
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}
	if positions[0].Side != types.SideTypeSell || !positions[0].Size.Equal(mustDec("12")) {
		t.Fatalf("side/size: %q / %s", positions[0].Side, positions[0].Size)
	}
	if !positions[0].Delta.Equal(mustDec("-6.5")) || positions[0].PendingOrders != 2 {
		t.Fatalf("delta/pending: %s / %d", positions[0].Delta, positions[0].PendingOrders)
	}
}

func TestGetPosition_FlatOnPositionNotFound(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"label":"POSITION_NOT_FOUND","message":"no position"}`)
	}))
	defer srv.Close()

	var o *Client = newOptionsTestClient(t, srv.URL)
	var pos types.PositionInfo
	var err error
	pos, err = o.Account().GetPosition(context.Background(), testContract)
	if err != nil {
		t.Fatalf("GetPosition should treat POSITION_NOT_FOUND as flat, got %v", err)
	}
	if pos.Contract != testContract || pos.Side != "" || !pos.Size.IsZero() {
		t.Fatalf("flat position: %+v", pos)
	}
}

// TestPositionPayload_DecodesNumberAndStringForms guards the Gate behavior where
// the WebSocket options.positions push sends decimal fields as bare JSON numbers
// while the REST payload quotes them as strings. The shared positionPayload must
// decode BOTH via codec.FlexDecimal (a plain string field silently drops every
// real-time options position update).
func TestPositionPayload_DecodesNumberAndStringForms(t *testing.T) {
	var wsPush = []byte(`[{"contract":"` + testContract + `","size":-12,"entry_price":0.0611,"mark_price":0.061,
		"unrealised_pnl":-0.7,"realised_pnl":1.2,"delta":-6.5,"gamma":0.01,"vega":3.2,"theta":-1.1,
		"mark_iv":0.6,"pending_orders":2,"update_time":1700000000}]`)
	var restBody = []byte(`[{"contract":"` + testContract + `","size":-12,"entry_price":"0.0611","mark_price":"0.061",
		"unrealised_pnl":"-0.7","realised_pnl":"1.2","delta":"-6.5","gamma":"0.01","vega":"3.2","theta":"-1.1",
		"mark_iv":"0.6","pending_orders":2,"update_time":1700000000}]`)

	var form string
	var raw []byte
	for form, raw = range map[string][]byte{"ws-number": wsPush, "rest-string": restBody} {
		var payloads []positionPayload
		if err := codec.Unmarshal(raw, &payloads); err != nil {
			t.Fatalf("%s: unmarshal failed: %v", form, err)
		}
		if len(payloads) != 1 {
			t.Fatalf("%s: want 1 payload, got %d", form, len(payloads))
		}
		var pos types.PositionInfo = positionInfoFromPayload(&payloads[0], nil)
		if pos.Side != types.SideTypeSell || !pos.Size.Equal(mustDec("12")) {
			t.Fatalf("%s: side/size: %q / %s", form, pos.Side, pos.Size)
		}
		if !pos.EntryPrice.Equal(mustDec("0.0611")) || !pos.UnrealisedPnl.Equal(mustDec("-0.7")) {
			t.Fatalf("%s: entry/upnl: %s / %s", form, pos.EntryPrice, pos.UnrealisedPnl)
		}
		if !pos.Delta.Equal(mustDec("-6.5")) || !pos.MarkIv.Equal(mustDec("0.6")) {
			t.Fatalf("%s: delta/iv: %s / %s", form, pos.Delta, pos.MarkIv)
		}
	}
}

// TestTickerPayload_DecodesNumberAndStringForms guards the same number-or-string
// duality on the options ticker (REST strings vs options.contract_tickers numbers).
func TestTickerPayload_DecodesNumberAndStringForms(t *testing.T) {
	var wsPush = []byte(`{"name":"` + testContract + `","last_price":120,"mark_price":123.4,"mark_iv":0.65,
		"bid1_price":119,"bid1_size":5,"ask1_price":121,"ask1_size":7,"delta":0.55,"theta":-4.5,"position_size":42}`)
	var restBody = []byte(`{"name":"` + testContract + `","last_price":"120","mark_price":"123.4","mark_iv":"0.65",
		"bid1_price":"119","bid1_size":"5","ask1_price":"121","ask1_size":"7","delta":"0.55","theta":"-4.5","position_size":"42"}`)

	var form string
	var raw []byte
	for form, raw = range map[string][]byte{"ws-number": wsPush, "rest-string": restBody} {
		var p tickerPayload
		if err := codec.Unmarshal(raw, &p); err != nil {
			t.Fatalf("%s: unmarshal failed: %v", form, err)
		}
		var tk types.Ticker = tickerFromPayload(&p, nil)
		if !tk.MarkPrice.Equal(mustDec("123.4")) || !tk.MarkIv.Equal(mustDec("0.65")) {
			t.Fatalf("%s: mark/iv: %s / %s", form, tk.MarkPrice, tk.MarkIv)
		}
		if !tk.Bid1Size.Equal(mustDec("5")) || !tk.Delta.Equal(mustDec("0.55")) {
			t.Fatalf("%s: bidsize/delta: %s / %s", form, tk.Bid1Size, tk.Delta)
		}
	}
}
