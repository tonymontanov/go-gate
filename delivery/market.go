/*
FILE: delivery/market.go

DESCRIPTION:
Market-data sub-client for the Gate Delivery section. Implements (all public, no
signing):
  - GetContracts / GetContract : GET /delivery/{settle}/contracts[/{contract}] → SymbolInfo
  - GetOrderBook               : GET /delivery/{settle}/order_book (with_id=true)
  - GetCandlesticks            : GET /delivery/{settle}/candlesticks
  - GetTickers                 : GET /delivery/{settle}/tickers

Decimal fields arrive as Gate strings; sizes are in contracts; timestamps in
seconds are normalized to milliseconds.
*/

package delivery

import (
	"context"
	"strconv"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/delivery/types"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
)

// MarketDataClient — market-data sub-client.
type MarketDataClient struct {
	c *Client
}

func newMarketDataClient(c *Client) *MarketDataClient {
	return &MarketDataClient{c: c}
}

func (m *MarketDataClient) contractsPath() string { return m.c.basePath() + "/contracts" }
func (m *MarketDataClient) contractPath(contract string) string {
	return m.c.basePath() + "/contracts/" + contract
}
func (m *MarketDataClient) orderBookPath() string    { return m.c.basePath() + "/order_book" }
func (m *MarketDataClient) candlesticksPath() string { return m.c.basePath() + "/candlesticks" }
func (m *MarketDataClient) tickersPath() string      { return m.c.basePath() + "/tickers" }

// ---- contracts -------------------------------------------------------------

type contractPayload struct {
	Name              string `json:"name"`
	Type              string `json:"type"`
	QuantoMultiplier  string `json:"quanto_multiplier"`
	LeverageMin       string `json:"leverage_min"`
	LeverageMax       string `json:"leverage_max"`
	MaintenanceRate   string `json:"maintenance_rate"`
	MarkPrice         string `json:"mark_price"`
	IndexPrice        string `json:"index_price"`
	LastPrice         string `json:"last_price"`
	OrderPriceRound   string `json:"order_price_round"`
	MarkPriceRound    string `json:"mark_price_round"`
	OrderSizeMin      int64  `json:"order_size_min"`
	OrderSizeMax      int64  `json:"order_size_max"`
	OrderPriceDeviate string `json:"order_price_deviate"`
	OrdersLimit       int64  `json:"orders_limit"`
	// Delivery-specific (no funding): expiry + settlement cycle. CALIBRATION:
	// confirm the exact JSON keys (expire_time in seconds vs ms; cycle).
	ExpireTime  int64  `json:"expire_time"`
	Cycle       string `json:"cycle"`
	InDelisting bool   `json:"in_delisting"`
}

func symbolInfoFromPayload(p *contractPayload, rateLimits map[string]string) types.SymbolInfo {
	return types.SymbolInfo{
		Contract:          p.Name,
		Type:              p.Type,
		QuantoMultiplier:  mustDecimal(p.QuantoMultiplier),
		OrderSizeMin:      decimalAbsInt(p.OrderSizeMin),
		OrderSizeMax:      decimalAbsInt(p.OrderSizeMax),
		OrderPriceRound:   mustDecimal(p.OrderPriceRound),
		MarkPriceRound:    mustDecimal(p.MarkPriceRound),
		OrderPriceDeviate: mustDecimal(p.OrderPriceDeviate),
		LeverageMin:       mustDecimal(p.LeverageMin),
		LeverageMax:       mustDecimal(p.LeverageMax),
		MaintenanceRate:   mustDecimal(p.MaintenanceRate),
		MarkPrice:         mustDecimal(p.MarkPrice),
		IndexPrice:        mustDecimal(p.IndexPrice),
		LastPrice:         mustDecimal(p.LastPrice),
		// Gate delivery returns expire_time in epoch SECONDS; normalize to ms.
		ExpireTimeMs: secondsToMs(p.ExpireTime),
		Cycle:        p.Cycle,
		OrdersLimit:  p.OrdersLimit,
		InDelisting:  p.InDelisting,
		RateLimits:   rateLimits,
	}
}

// secondsToMs converts an epoch-seconds timestamp to milliseconds (0 stays 0).
func secondsToMs(sec int64) int64 {
	if sec <= 0 {
		return 0
	}
	return sec * 1000
}

// GetContracts returns the full contract list for this settle currency.
func (m *MarketDataClient) GetContracts(ctx context.Context) ([]types.SymbolInfo, error) {
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = m.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   m.contractsPath(),
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []contractPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "market.GetContracts: parse", err)
	}
	var out []types.SymbolInfo = make([]types.SymbolInfo, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, symbolInfoFromPayload(&payloads[i], rateLimits))
	}
	return out, nil
}

// GetContract returns the specification of a single contract.
func (m *MarketDataClient) GetContract(ctx context.Context, contract string) (types.SymbolInfo, error) {
	var info types.SymbolInfo
	if contract == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "market.GetContract: contract is empty", nil)
	}
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = m.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   m.contractPath(contract),
		Meta:   rest.RequestMeta{Symbols: []string{contract}, Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return info, err
	}
	var p contractPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "market.GetContract: parse", err)
	}
	return symbolInfoFromPayload(&p, rateLimits), nil
}

// ---- order book ------------------------------------------------------------

type orderBookItemPayload struct {
	Price string `json:"p"`
	Size  int64  `json:"s"`
}

type orderBookPayload struct {
	ID      int64                  `json:"id"`
	Current float64                `json:"current"`
	Update  float64                `json:"update"`
	Asks    []orderBookItemPayload `json:"asks"`
	Bids    []orderBookItemPayload `json:"bids"`
}

func levelsFromPayload(items []orderBookItemPayload) []types.OrderBookLevel {
	var out []types.OrderBookLevel = make([]types.OrderBookLevel, 0, len(items))
	var i int
	for i = 0; i < len(items); i++ {
		out = append(out, types.OrderBookLevel{
			Price: mustDecimal(items[i].Price),
			Size:  decimalAbsInt(items[i].Size),
		})
	}
	return out
}

// GetOrderBook returns an order book snapshot for a contract. limit ≤ 0 lets Gate
// use its default depth. with_id=true is always requested so OrderBook.ID can
// baseline an incremental engine in a later iteration.
func (m *MarketDataClient) GetOrderBook(ctx context.Context, contract string, limit int) (types.OrderBook, error) {
	var book types.OrderBook
	if contract == "" {
		return book, gate.NewError(gate.ErrorKindInvalidRequest, "", "market.GetOrderBook: contract is empty", nil)
	}
	var q = newQuery()
	q.Set("contract", contract)
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
		Meta:   rest.RequestMeta{Symbols: []string{contract}, Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return book, err
	}
	var p orderBookPayload
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

type candlePayload struct {
	T   float64 `json:"t"`
	V   int64   `json:"v"`
	C   string  `json:"c"`
	H   string  `json:"h"`
	L   string  `json:"l"`
	O   string  `json:"o"`
	Sum string  `json:"sum"`
}

// GetCandlesticks returns up to limit candles for a contract at the given interval.
func (m *MarketDataClient) GetCandlesticks(ctx context.Context, contract string, interval types.CandleInterval, limit int) ([]types.Candle, error) {
	if contract == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "market.GetCandlesticks: contract is empty", nil)
	}
	var q = newQuery()
	q.Set("contract", contract)
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
		Meta:   rest.RequestMeta{Symbols: []string{contract}, Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []candlePayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "market.GetCandlesticks: parse", err)
	}
	var out []types.Candle = make([]types.Candle, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, types.Candle{
			OpenTimeMs:  int64(payloads[i].T * 1000),
			Open:        mustDecimal(payloads[i].O),
			High:        mustDecimal(payloads[i].H),
			Low:         mustDecimal(payloads[i].L),
			Close:       mustDecimal(payloads[i].C),
			Volume:      decimalAbsInt(payloads[i].V),
			QuoteVolume: mustDecimal(payloads[i].Sum),
		})
	}
	return out, nil
}

// ---- tickers ---------------------------------------------------------------

type tickerPayload struct {
	Contract         string `json:"contract"`
	Last             string `json:"last"`
	MarkPrice        string `json:"mark_price"`
	IndexPrice       string `json:"index_price"`
	HighestBid       string `json:"highest_bid"`
	LowestAsk        string `json:"lowest_ask"`
	ChangePercentage string `json:"change_percentage"`
	TotalSize        string `json:"total_size"`
	Volume24h        string `json:"volume_24h"`
}

// GetTickers returns tickers. Pass an empty contract for all contracts.
func (m *MarketDataClient) GetTickers(ctx context.Context, contract string) ([]types.Ticker, error) {
	var q = newQuery()
	if contract != "" {
		q.Set("contract", contract)
	}
	var symbols []string
	if contract != "" {
		symbols = []string{contract}
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
	var payloads []tickerPayload
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

// tickerFromPayload maps a Gate ticker payload (REST or WS) into types.Ticker.
func tickerFromPayload(p *tickerPayload, rateLimits map[string]string) types.Ticker {
	return types.Ticker{
		Contract:         p.Contract,
		Last:             mustDecimal(p.Last),
		MarkPrice:        mustDecimal(p.MarkPrice),
		IndexPrice:       mustDecimal(p.IndexPrice),
		HighestBid:       mustDecimal(p.HighestBid),
		LowestAsk:        mustDecimal(p.LowestAsk),
		ChangePercentage: mustDecimal(p.ChangePercentage),
		TotalSize:        mustDecimal(p.TotalSize),
		Volume24h:        mustDecimal(p.Volume24h),
		RateLimits:       rateLimits,
	}
}

// floatSecondsOrMsToMs normalizes a Gate float timestamp to epoch milliseconds.
// Values that already look like milliseconds (>= 1e12) are taken as-is; smaller
// values are treated as seconds and scaled.
func floatSecondsOrMsToMs(v float64) int64 {
	if v <= 0 {
		return 0
	}
	if v >= 1e12 {
		return int64(v)
	}
	return int64(v * 1000)
}
