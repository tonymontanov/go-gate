/*
FILE: spot/market.go

DESCRIPTION:
Market-data sub-client for the Gate Spot section. Implements (all public, no
signing):
  - GetCurrencyPairs / GetCurrencyPair : GET /spot/currency_pairs[/{cp}] → SymbolInfo
  - GetOrderBook                       : GET /spot/order_book (with_id=true)
  - GetCandlesticks                    : GET /spot/candlesticks
  - GetTickers                         : GET /spot/tickers

Gate spot wire shapes differ from futures: order-book levels are ["price",
"amount"] string pairs (amount in base currency); candlesticks are string arrays
with the column order [t, quote_volume, close, high, low, open, base_volume,
window_closed]; precision is a number of decimal places.
*/

package spot

import (
	"context"
	"strconv"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
	"github.com/tonymontanov/go-gate/v2/spot/types"
)

// MarketDataClient — market-data sub-client.
type MarketDataClient struct {
	c *Client
}

func newMarketDataClient(c *Client) *MarketDataClient {
	return &MarketDataClient{c: c}
}

func (m *MarketDataClient) currencyPairsPath() string { return "/spot/currency_pairs" }
func (m *MarketDataClient) currencyPairPath(cp string) string {
	return "/spot/currency_pairs/" + cp
}
func (m *MarketDataClient) orderBookPath() string    { return "/spot/order_book" }
func (m *MarketDataClient) candlesticksPath() string { return "/spot/candlesticks" }
func (m *MarketDataClient) tickersPath() string      { return "/spot/tickers" }

// ---- currency pairs --------------------------------------------------------

type currencyPairPayload struct {
	ID                  string `json:"id"`
	Base                string `json:"base"`
	Quote               string `json:"quote"`
	Fee                 string `json:"fee"`
	MinBaseAmount       string `json:"min_base_amount"`
	MinQuoteAmount      string `json:"min_quote_amount"`
	MaxBaseAmount       string `json:"max_base_amount"`
	MaxQuoteAmount      string `json:"max_quote_amount"`
	AmountPrecision     int32  `json:"amount_precision"`
	Precision           int32  `json:"precision"`
	TradeStatus         string `json:"trade_status"`
	MarketOrderMaxStock string `json:"market_order_max_stock"`
	MarketOrderMaxMoney string `json:"market_order_max_money"`
}

func symbolInfoFromPayload(p *currencyPairPayload, rateLimits map[string]string) types.SymbolInfo {
	return types.SymbolInfo{
		CurrencyPair:        p.ID,
		Base:                p.Base,
		Quote:               p.Quote,
		Fee:                 mustDecimal(p.Fee),
		MinBaseAmount:       mustDecimal(p.MinBaseAmount),
		MinQuoteAmount:      mustDecimal(p.MinQuoteAmount),
		MaxBaseAmount:       mustDecimal(p.MaxBaseAmount),
		MaxQuoteAmount:      mustDecimal(p.MaxQuoteAmount),
		AmountPrecision:     p.AmountPrecision,
		PricePrecision:      p.Precision,
		TradeStatus:         p.TradeStatus,
		MarketOrderMaxBase:  mustDecimal(p.MarketOrderMaxStock),
		MarketOrderMaxQuote: mustDecimal(p.MarketOrderMaxMoney),
		RateLimits:          rateLimits,
	}
}

// GetCurrencyPairs returns the full list of spot currency pairs.
func (m *MarketDataClient) GetCurrencyPairs(ctx context.Context) ([]types.SymbolInfo, error) {
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = m.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   m.currencyPairsPath(),
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []currencyPairPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "market.GetCurrencyPairs: parse", err)
	}
	var out []types.SymbolInfo = make([]types.SymbolInfo, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, symbolInfoFromPayload(&payloads[i], rateLimits))
	}
	return out, nil
}

// GetCurrencyPair returns the specification of a single currency pair.
func (m *MarketDataClient) GetCurrencyPair(ctx context.Context, currencyPair string) (types.SymbolInfo, error) {
	var info types.SymbolInfo
	if currencyPair == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "market.GetCurrencyPair: currencyPair is empty", nil)
	}
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = m.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   m.currencyPairPath(currencyPair),
		Meta:   rest.RequestMeta{Symbols: []string{currencyPair}, Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return info, err
	}
	var p currencyPairPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "market.GetCurrencyPair: parse", err)
	}
	return symbolInfoFromPayload(&p, rateLimits), nil
}

// ---- order book ------------------------------------------------------------

type spotOrderBookPayload struct {
	ID      int64      `json:"id"`
	Current float64    `json:"current"`
	Update  float64    `json:"update"`
	Asks    [][]string `json:"asks"`
	Bids    [][]string `json:"bids"`
}

// levelsFromPayload converts Gate spot ["price","amount"] string pairs into
// typed levels. Malformed rows (fewer than 2 fields) are skipped.
func levelsFromPayload(items [][]string) []types.OrderBookLevel {
	var out []types.OrderBookLevel = make([]types.OrderBookLevel, 0, len(items))
	var i int
	for i = 0; i < len(items); i++ {
		if len(items[i]) < 2 {
			continue
		}
		out = append(out, types.OrderBookLevel{
			Price:  mustDecimal(items[i][0]),
			Amount: mustDecimal(items[i][1]),
		})
	}
	return out
}

// GetOrderBook returns an order book snapshot for a currency pair. limit ≤ 0 lets
// Gate use its default depth. with_id=true is always requested so OrderBook.ID can
// baseline an incremental engine in a later iteration.
func (m *MarketDataClient) GetOrderBook(ctx context.Context, currencyPair string, limit int) (types.OrderBook, error) {
	var book types.OrderBook
	if currencyPair == "" {
		return book, gate.NewError(gate.ErrorKindInvalidRequest, "", "market.GetOrderBook: currencyPair is empty", nil)
	}
	var q = newQuery()
	q.Set("currency_pair", currencyPair)
	q.Set("with_id", "true")
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = m.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   m.orderBookPath(),
		Query:  q,
		Meta:   rest.RequestMeta{Symbols: []string{currencyPair}, Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return book, err
	}
	var p spotOrderBookPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return book, gate.NewError(gate.ErrorKindUnknown, "", "market.GetOrderBook: parse", err)
	}
	book = types.OrderBook{
		ID:         p.ID,
		Asks:       levelsFromPayload(p.Asks),
		Bids:       levelsFromPayload(p.Bids),
		CurrentMs:  floatSecondsOrMsToMs(p.Current),
		UpdateMs:   floatSecondsOrMsToMs(p.Update),
		RateLimits: rateLimits,
	}
	return book, nil
}

// ---- candlesticks ----------------------------------------------------------

// GetCandlesticks returns up to limit candles for a currency pair at the given
// interval. Gate returns each candle as a string array; the column order is
// [t, quote_volume, close, high, low, open, base_volume, window_closed].
func (m *MarketDataClient) GetCandlesticks(ctx context.Context, currencyPair string, interval types.CandleInterval, limit int) ([]types.Candle, error) {
	if currencyPair == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "market.GetCandlesticks: currencyPair is empty", nil)
	}
	var q = newQuery()
	q.Set("currency_pair", currencyPair)
	if interval != "" {
		q.Set("interval", string(interval))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}

	var resp rest.Response
	var err error
	resp, _, err = m.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   m.candlesticksPath(),
		Query:  q,
		Meta:   rest.RequestMeta{Symbols: []string{currencyPair}, Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var rows [][]string
	if err = resp.UnmarshalData(&rows); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "market.GetCandlesticks: parse", err)
	}
	var out []types.Candle = make([]types.Candle, 0, len(rows))
	var i int
	for i = 0; i < len(rows); i++ {
		out = append(out, candleFromRow(rows[i]))
	}
	return out, nil
}

// candleFromRow maps one Gate spot candlestick string array into types.Candle.
// Rows with fewer than 7 fields are returned with whatever could be parsed.
func candleFromRow(row []string) types.Candle {
	var c types.Candle
	if len(row) > 0 {
		c.OpenTimeMs = spotEpochMs(0, row[0])
	}
	if len(row) > 1 {
		c.QuoteVolume = mustDecimal(row[1])
	}
	if len(row) > 2 {
		c.Close = mustDecimal(row[2])
	}
	if len(row) > 3 {
		c.High = mustDecimal(row[3])
	}
	if len(row) > 4 {
		c.Low = mustDecimal(row[4])
	}
	if len(row) > 5 {
		c.Open = mustDecimal(row[5])
	}
	if len(row) > 6 {
		c.BaseVolume = mustDecimal(row[6])
	}
	if len(row) > 7 {
		c.WindowClosed = row[7] == "true"
	}
	return c
}

// ---- tickers ---------------------------------------------------------------

type spotTickerPayload struct {
	CurrencyPair     string `json:"currency_pair"`
	Last             string `json:"last"`
	LowestAsk        string `json:"lowest_ask"`
	LowestSize       string `json:"lowest_size"`
	HighestBid       string `json:"highest_bid"`
	HighestSize      string `json:"highest_size"`
	ChangePercentage string `json:"change_percentage"`
	BaseVolume       string `json:"base_volume"`
	QuoteVolume      string `json:"quote_volume"`
	High24h          string `json:"high_24h"`
	Low24h           string `json:"low_24h"`
}

// GetTickers returns tickers. Pass an empty currencyPair for all pairs.
func (m *MarketDataClient) GetTickers(ctx context.Context, currencyPair string) ([]types.Ticker, error) {
	var q = newQuery()
	var symbols []string
	if currencyPair != "" {
		q.Set("currency_pair", currencyPair)
		symbols = []string{currencyPair}
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = m.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   m.tickersPath(),
		Query:  q,
		Meta:   rest.RequestMeta{Symbols: symbols, Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []spotTickerPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "market.GetTickers: parse", err)
	}
	var out []types.Ticker = make([]types.Ticker, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, tickerFromPayload(&payloads[i], rateLimits))
	}
	return out, nil
}

// tickerFromPayload maps a Gate spot ticker payload (REST or WS) into types.Ticker.
func tickerFromPayload(p *spotTickerPayload, rateLimits map[string]string) types.Ticker {
	return types.Ticker{
		CurrencyPair:     p.CurrencyPair,
		Last:             mustDecimal(p.Last),
		LowestAsk:        mustDecimal(p.LowestAsk),
		LowestSize:       mustDecimal(p.LowestSize),
		HighestBid:       mustDecimal(p.HighestBid),
		HighestSize:      mustDecimal(p.HighestSize),
		ChangePercentage: mustDecimal(p.ChangePercentage),
		BaseVolume:       mustDecimal(p.BaseVolume),
		QuoteVolume:      mustDecimal(p.QuoteVolume),
		High24h:          mustDecimal(p.High24h),
		Low24h:           mustDecimal(p.Low24h),
		RateLimits:       rateLimits,
	}
}

// floatSecondsOrMsToMs normalizes a Gate float timestamp to epoch milliseconds.
// Values that already look like milliseconds (>= 1e12) are taken as-is; smaller
// values are treated as seconds and scaled. (Spot order books report ms;
// futures report float seconds — this handles both.)
func floatSecondsOrMsToMs(v float64) int64 {
	if v <= 0 {
		return 0
	}
	if v >= 1e12 {
		return int64(v)
	}
	return int64(v * 1000)
}
