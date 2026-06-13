/*
FILE: options/market.go

DESCRIPTION:
Market-data sub-client for the Gate Options section. Implements (all public, no
signing):
  - GetUnderlyings            : GET /options/underlyings
  - GetExpirations            : GET /options/expirations?underlying=
  - GetContracts / GetContract: GET /options/contracts[?underlying=&expiration=][/{contract}]
  - GetSettlements/GetSettlement: GET /options/settlements[?underlying=][/{contract}]
  - GetOrderBook              : GET /options/order_book (with_id=true)
  - GetTickers                : GET /options/tickers?underlying=
  - GetUnderlyingTicker       : GET /options/underlying/tickers/{underlying}
  - GetCandlesticks           : GET /options/candlesticks
  - GetUnderlyingCandlesticks : GET /options/underlying/candlesticks
  - GetTrades                 : GET /options/trades

Decimal fields that Gate may send as number OR string (ticker/contract prices and
greeks) decode through codec.FlexDecimal so the SAME payload struct works for both
the REST string form and the options.contract_tickers WS number form. Timestamps
in seconds are normalized to milliseconds.
*/

package options

import (
	"context"
	"net/url"
	"strconv"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
	"github.com/tonymontanov/go-gate/v2/options/types"
)

// MarketDataClient — market-data sub-client.
type MarketDataClient struct {
	c *Client
}

func newMarketDataClient(c *Client) *MarketDataClient {
	return &MarketDataClient{c: c}
}

func (m *MarketDataClient) underlyingsPath() string { return m.c.basePath() + "/underlyings" }
func (m *MarketDataClient) expirationsPath() string { return m.c.basePath() + "/expirations" }
func (m *MarketDataClient) contractsPath() string   { return m.c.basePath() + "/contracts" }
func (m *MarketDataClient) contractPath(contract string) string {
	return m.c.basePath() + "/contracts/" + contract
}
func (m *MarketDataClient) settlementsPath() string { return m.c.basePath() + "/settlements" }
func (m *MarketDataClient) settlementPath(contract string) string {
	return m.c.basePath() + "/settlements/" + contract
}
func (m *MarketDataClient) orderBookPath() string    { return m.c.basePath() + "/order_book" }
func (m *MarketDataClient) tickersPath() string      { return m.c.basePath() + "/tickers" }
func (m *MarketDataClient) candlesticksPath() string { return m.c.basePath() + "/candlesticks" }
func (m *MarketDataClient) underlyingTickerPath(underlying string) string {
	return m.c.basePath() + "/underlying/tickers/" + underlying
}
func (m *MarketDataClient) underlyingCandlesticksPath() string {
	return m.c.basePath() + "/underlying/candlesticks"
}
func (m *MarketDataClient) tradesPath() string { return m.c.basePath() + "/trades" }

// ---- underlyings -----------------------------------------------------------

type underlyingPayload struct {
	Name       string            `json:"name"`
	IndexPrice codec.FlexDecimal `json:"index_price"`
}

// GetUnderlyings returns all options underlying indices and their index prices.
func (m *MarketDataClient) GetUnderlyings(ctx context.Context) ([]types.Underlying, error) {
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = m.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   m.underlyingsPath(),
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []underlyingPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "market.GetUnderlyings: parse", err)
	}
	var out []types.Underlying = make([]types.Underlying, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, types.Underlying{
			Name:       payloads[i].Name,
			IndexPrice: payloads[i].IndexPrice.Decimal,
			RateLimits: rateLimits,
		})
	}
	return out, nil
}

// ---- expirations -----------------------------------------------------------

// GetExpirations returns the expiration timestamps available for an underlying.
// Gate returns a flat array of epoch-SECONDS timestamps; the SDK normalizes each
// to milliseconds.
func (m *MarketDataClient) GetExpirations(ctx context.Context, underlying string) ([]types.Expiration, error) {
	if underlying == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "market.GetExpirations: underlying is empty", nil)
	}
	var q = newQuery()
	q.Set("underlying", underlying)

	var resp rest.Response
	var err error
	resp, _, err = m.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   m.expirationsPath(),
		Query:  q,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var seconds []int64
	if err = resp.UnmarshalData(&seconds); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "market.GetExpirations: parse", err)
	}
	var out []types.Expiration = make([]types.Expiration, 0, len(seconds))
	var i int
	for i = 0; i < len(seconds); i++ {
		out = append(out, types.Expiration{ExpirationMs: secondsToMs(seconds[i])})
	}
	return out, nil
}

// ---- contracts -------------------------------------------------------------

// contractPayload — Gate options contract spec wire shape. Decimal fields use
// codec.FlexDecimal: the REST /options/contracts response quotes prices as
// strings but sends greeks as bare numbers, and the same shape feeds nowhere
// else — FlexDecimal tolerates both forms without aborting the decode.
type contractPayload struct {
	Name            string            `json:"name"`
	Underlying      string            `json:"underlying"`
	IsCall          bool              `json:"is_call"`
	ExpirationTime  int64             `json:"expiration_time"`
	StrikePrice     codec.FlexDecimal `json:"strike_price"`
	Multiplier      codec.FlexDecimal `json:"multiplier"`
	OrderPriceRound codec.FlexDecimal `json:"order_price_round"`
	MarkPriceRound  codec.FlexDecimal `json:"mark_price_round"`
	OrderSizeMin    int64             `json:"order_size_min"`
	OrderSizeMax    int64             `json:"order_size_max"`
	MakerFeeRate    codec.FlexDecimal `json:"maker_fee_rate"`
	TakerFeeRate    codec.FlexDecimal `json:"taker_fee_rate"`
	RefDiscountRate codec.FlexDecimal `json:"ref_discount_rate"`
	RefRebateRate   codec.FlexDecimal `json:"ref_rebate_rate"`
	MarkPrice       codec.FlexDecimal `json:"mark_price"`
	LastPrice       codec.FlexDecimal `json:"last_price"`
	IndexPrice      codec.FlexDecimal `json:"index_price"`
	MarkIv          codec.FlexDecimal `json:"mark_iv"`
	Delta           codec.FlexDecimal `json:"delta"`
	Gamma           codec.FlexDecimal `json:"gamma"`
	Vega            codec.FlexDecimal `json:"vega"`
	Theta           codec.FlexDecimal `json:"theta"`
	OrdersLimit     int64             `json:"orders_limit"`
	InDelisting     bool              `json:"in_delisting"`
}

func symbolInfoFromPayload(p *contractPayload, rateLimits map[string]string) types.SymbolInfo {
	var opt types.OptionType = types.OptionTypePut
	if p.IsCall {
		opt = types.OptionTypeCall
	}
	return types.SymbolInfo{
		Contract:        p.Name,
		Underlying:      p.Underlying,
		ExpirationMs:    secondsToMs(p.ExpirationTime),
		StrikePrice:     p.StrikePrice.Decimal,
		IsCall:          p.IsCall,
		OptionType:      opt,
		Multiplier:      p.Multiplier.Decimal,
		OrderPriceRound: p.OrderPriceRound.Decimal,
		MarkPriceRound:  p.MarkPriceRound.Decimal,
		OrderSizeMin:    decimalAbsInt(p.OrderSizeMin),
		OrderSizeMax:    decimalAbsInt(p.OrderSizeMax),
		MakerFeeRate:    p.MakerFeeRate.Decimal,
		TakerFeeRate:    p.TakerFeeRate.Decimal,
		RefDiscountRate: p.RefDiscountRate.Decimal,
		RefRebateRate:   p.RefRebateRate.Decimal,
		MarkPrice:       p.MarkPrice.Decimal,
		LastPrice:       p.LastPrice.Decimal,
		IndexPrice:      p.IndexPrice.Decimal,
		MarkIv:          p.MarkIv.Decimal,
		Delta:           p.Delta.Decimal,
		Gamma:           p.Gamma.Decimal,
		Vega:            p.Vega.Decimal,
		Theta:           p.Theta.Decimal,
		OrdersLimit:     p.OrdersLimit,
		InDelisting:     p.InDelisting,
		RateLimits:      rateLimits,
	}
}

// GetContracts returns the option contracts for an underlying. expiration is
// optional (0 = all expiries); when set it filters to that expiry (epoch seconds,
// as Gate expects on the wire).
func (m *MarketDataClient) GetContracts(ctx context.Context, underlying string, expiration int64) ([]types.SymbolInfo, error) {
	if underlying == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "market.GetContracts: underlying is empty", nil)
	}
	var q = newQuery()
	q.Set("underlying", underlying)
	if expiration > 0 {
		q.Set("expiration", strconv.FormatInt(expiration, 10))
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = m.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   m.contractsPath(),
		Query:  q,
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

// GetContract returns the specification of a single options contract.
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

// ---- settlements (public) --------------------------------------------------

type settlementPayload struct {
	Time        int64  `json:"time"`
	Contract    string `json:"contract"`
	Profit      string `json:"profit"`
	Fee         string `json:"fee"`
	StrikePrice string `json:"strike_price"`
	SettlePrice string `json:"settle_price"`
}

func settlementFromPayload(p *settlementPayload, rateLimits map[string]string) types.Settlement {
	return types.Settlement{
		TimeMs:      secondsToMs(p.Time),
		Contract:    p.Contract,
		Profit:      mustDecimal(p.Profit),
		Fee:         mustDecimal(p.Fee),
		StrikePrice: mustDecimal(p.StrikePrice),
		SettlePrice: mustDecimal(p.SettlePrice),
		RateLimits:  rateLimits,
	}
}

// GetSettlements returns public settlement records for an underlying.
func (m *MarketDataClient) GetSettlements(ctx context.Context, underlying string, limit int) ([]types.Settlement, error) {
	if underlying == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "market.GetSettlements: underlying is empty", nil)
	}
	var q = newQuery()
	q.Set("underlying", underlying)
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = m.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   m.settlementsPath(),
		Query:  q,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []settlementPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "market.GetSettlements: parse", err)
	}
	var out []types.Settlement = make([]types.Settlement, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, settlementFromPayload(&payloads[i], rateLimits))
	}
	return out, nil
}

// GetSettlement returns the public settlement record for a single contract.
func (m *MarketDataClient) GetSettlement(ctx context.Context, contract string) (types.Settlement, error) {
	var s types.Settlement
	if contract == "" {
		return s, gate.NewError(gate.ErrorKindInvalidRequest, "", "market.GetSettlement: contract is empty", nil)
	}
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = m.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   m.settlementPath(contract),
		Meta:   rest.RequestMeta{Symbols: []string{contract}, Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return s, err
	}
	var p settlementPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return s, gate.NewError(gate.ErrorKindUnknown, "", "market.GetSettlement: parse", err)
	}
	return settlementFromPayload(&p, rateLimits), nil
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
// baseline the incremental engine.
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

// ---- tickers ---------------------------------------------------------------

// tickerPayload — Gate options ticker wire shape, shared by REST GetTickers
// (string decimals) and the options.contract_tickers WS push (number decimals).
// codec.FlexDecimal tolerates both forms.
type tickerPayload struct {
	Name         string            `json:"name"`
	LastPrice    codec.FlexDecimal `json:"last_price"`
	MarkPrice    codec.FlexDecimal `json:"mark_price"`
	IndexPrice   codec.FlexDecimal `json:"index_price"`
	MarkIv       codec.FlexDecimal `json:"mark_iv"`
	Bid1Price    codec.FlexDecimal `json:"bid1_price"`
	Bid1Size     codec.FlexDecimal `json:"bid1_size"`
	Bid1Iv       codec.FlexDecimal `json:"bid_iv"`
	Ask1Price    codec.FlexDecimal `json:"ask1_price"`
	Ask1Size     codec.FlexDecimal `json:"ask1_size"`
	Ask1Iv       codec.FlexDecimal `json:"ask_iv"`
	PositionSize codec.FlexDecimal `json:"position_size"`
	Delta        codec.FlexDecimal `json:"delta"`
	Gamma        codec.FlexDecimal `json:"gamma"`
	Vega         codec.FlexDecimal `json:"vega"`
	Theta        codec.FlexDecimal `json:"theta"`
}

func tickerFromPayload(p *tickerPayload, rateLimits map[string]string) types.Ticker {
	return types.Ticker{
		Contract:     p.Name,
		LastPrice:    p.LastPrice.Decimal,
		MarkPrice:    p.MarkPrice.Decimal,
		IndexPrice:   p.IndexPrice.Decimal,
		MarkIv:       p.MarkIv.Decimal,
		Bid1Price:    p.Bid1Price.Decimal,
		Bid1Size:     p.Bid1Size.Decimal,
		Bid1Iv:       p.Bid1Iv.Decimal,
		Ask1Price:    p.Ask1Price.Decimal,
		Ask1Size:     p.Ask1Size.Decimal,
		Ask1Iv:       p.Ask1Iv.Decimal,
		PositionSize: p.PositionSize.Decimal,
		Delta:        p.Delta.Decimal,
		Gamma:        p.Gamma.Decimal,
		Vega:         p.Vega.Decimal,
		Theta:        p.Theta.Decimal,
		RateLimits:   rateLimits,
	}
}

// GetTickers returns the tickers for all contracts on an underlying.
func (m *MarketDataClient) GetTickers(ctx context.Context, underlying string) ([]types.Ticker, error) {
	if underlying == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "market.GetTickers: underlying is empty", nil)
	}
	var q = newQuery()
	q.Set("underlying", underlying)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = m.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   m.tickersPath(),
		Query:  q,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
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

// ---- underlying ticker -----------------------------------------------------

type underlyingTickerPayload struct {
	TradePut   codec.FlexDecimal `json:"trade_put"`
	TradeCall  codec.FlexDecimal `json:"trade_call"`
	IndexPrice codec.FlexDecimal `json:"index_price"`
}

// GetUnderlyingTicker returns the aggregate ticker (index price + put/call trade
// activity) for an underlying index.
func (m *MarketDataClient) GetUnderlyingTicker(ctx context.Context, underlying string) (types.UnderlyingTicker, error) {
	var t types.UnderlyingTicker
	if underlying == "" {
		return t, gate.NewError(gate.ErrorKindInvalidRequest, "", "market.GetUnderlyingTicker: underlying is empty", nil)
	}
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = m.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   m.underlyingTickerPath(underlying),
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return t, err
	}
	var p underlyingTickerPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return t, gate.NewError(gate.ErrorKindUnknown, "", "market.GetUnderlyingTicker: parse", err)
	}
	return types.UnderlyingTicker{
		Underlying: underlying,
		IndexPrice: p.IndexPrice.Decimal,
		TradePut:   p.TradePut.Decimal,
		TradeCall:  p.TradeCall.Decimal,
		RateLimits: rateLimits,
	}, nil
}

// ---- candlesticks ----------------------------------------------------------

type candlePayload struct {
	T     float64 `json:"t"`
	V     int64   `json:"v"`
	C     string  `json:"c"`
	H     string  `json:"h"`
	L     string  `json:"l"`
	O     string  `json:"o"`
	Sum   string  `json:"sum"`
	Close string  `json:"close"`
}

func candleFromPayload(p *candlePayload) types.Candle {
	// The contract candle series uses single-letter OHLC keys (o/h/l/c); the
	// underlying candle series uses "close" plus the same h/l/o set.
	var closePrice string = p.C
	if closePrice == "" {
		closePrice = p.Close
	}
	return types.Candle{
		OpenTimeMs: int64(p.T * 1000),
		Open:       mustDecimal(p.O),
		High:       mustDecimal(p.H),
		Low:        mustDecimal(p.L),
		Close:      mustDecimal(closePrice),
		Volume:     decimalAbsInt(p.V),
	}
}

func (m *MarketDataClient) getCandles(ctx context.Context, path string, q url.Values) ([]types.Candle, error) {
	var resp rest.Response
	var err error
	resp, _, err = m.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   path,
		Query:  q,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
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
		out = append(out, candleFromPayload(&payloads[i]))
	}
	return out, nil
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
	return m.getCandles(ctx, m.candlesticksPath(), q)
}

// GetUnderlyingCandlesticks returns up to limit candles for an UNDERLYING index
// at the given interval (the index price series, no volume).
func (m *MarketDataClient) GetUnderlyingCandlesticks(ctx context.Context, underlying string, interval types.CandleInterval, limit int) ([]types.Candle, error) {
	if underlying == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "market.GetUnderlyingCandlesticks: underlying is empty", nil)
	}
	var q = newQuery()
	q.Set("underlying", underlying)
	if interval != "" {
		q.Set("interval", string(interval))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	return m.getCandles(ctx, m.underlyingCandlesticksPath(), q)
}

// ---- trades ----------------------------------------------------------------

type tradePayload struct {
	ID         int64   `json:"id"`
	CreateTime float64 `json:"create_time"`
	Contract   string  `json:"contract"`
	Size       int64   `json:"size"`
	Price      string  `json:"price"`
	IsCall     bool    `json:"is_call"`
}

func tradeFromPayload(p *tradePayload, rateLimits map[string]string) types.Trade {
	return types.Trade{
		ID:         p.ID,
		Contract:   p.Contract,
		Price:      mustDecimal(p.Price),
		Size:       decimalAbsInt(p.Size),
		Side:       sideFromSize(p.Size),
		IsCall:     p.IsCall,
		Ts:         floatSecondsOrMsToMs(p.CreateTime),
		RateLimits: rateLimits,
	}
}

// GetTrades returns recent public trades for a contract. limit ≤ 0 / offset ≤ 0
// let Gate use its defaults.
func (m *MarketDataClient) GetTrades(ctx context.Context, contract string, limit, offset int) ([]types.Trade, error) {
	if contract == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "market.GetTrades: contract is empty", nil)
	}
	var q = newQuery()
	q.Set("contract", contract)
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = m.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   m.tradesPath(),
		Query:  q,
		Meta:   rest.RequestMeta{Symbols: []string{contract}, Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []tradePayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "market.GetTrades: parse", err)
	}
	var out []types.Trade = make([]types.Trade, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, tradeFromPayload(&payloads[i], rateLimits))
	}
	return out, nil
}
