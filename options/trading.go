/*
FILE: options/trading.go

DESCRIPTION:
Trading sub-client for the Gate Options section. Implements:
  - CreateOrder         : POST   /options/orders
  - ModifyOrder         : PUT    /options/orders/{order_id}
  - CancelOrder         : DELETE /options/orders/{order_id}
  - CancelAllOrders     : DELETE /options/orders?contract=&underlying=&side=  (native)
  - CountdownCancelAll  : POST   /options/countdown_cancel_all
  - GetOrder            : GET    /options/orders/{order_id}
  - GetOpenOrders       : GET    /options/orders?status=open&contract=&underlying=
  - GetMyTrades         : GET    /options/my_trades
  - GetMMP / SetMMP / ResetMMP : GET/POST /options/mmp[/reset]  (Market-Maker Protection)

GATE SPECIFICS encoded here:
  - direction is the sign of the integer order size (no side field), like futures;
  - market order = price="0" + tif="ioc"; limit defaults to tif="gtc";
  - the client order id ("text") is auto-prefixed "t-" and validated;
  - Gate options has NO batch order endpoints (no batch create/amend/cancel).

It also keeps a ClientOrderID ↔ OrderID map, populated on create and on
SyncOrderMappings, for the desk's idempotency/mapping needs.
*/

package options

import (
	"context"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
	"github.com/tonymontanov/go-gate/v2/options/types"
)

// newQuery returns an empty url.Values for building REST query strings.
func newQuery() url.Values { return url.Values{} }

// TradingClient — trading sub-client.
type TradingClient struct {
	c *Client

	mu          sync.RWMutex
	clOrdToOrd  map[string]string
	ordToClOrd  map[string]string
	createdAtMs map[string]int64
}

func newTradingClient(c *Client) *TradingClient {
	return &TradingClient{
		c:           c,
		clOrdToOrd:  make(map[string]string, 1024),
		ordToClOrd:  make(map[string]string, 1024),
		createdAtMs: make(map[string]int64, 1024),
	}
}

// ---- paths -----------------------------------------------------------------

func (t *TradingClient) ordersPath() string    { return t.c.basePath() + "/orders" }
func (t *TradingClient) countdownPath() string { return t.c.basePath() + "/countdown_cancel_all" }
func (t *TradingClient) myTradesPath() string  { return t.c.basePath() + "/my_trades" }
func (t *TradingClient) mmpPath() string       { return t.c.basePath() + "/mmp" }
func (t *TradingClient) mmpResetPath() string  { return t.c.basePath() + "/mmp/reset" }
func (t *TradingClient) orderPath(id string) string {
	return t.c.basePath() + "/orders/" + id
}

// ---- CreateOrder -----------------------------------------------------------

/*
CreateOrder creates an options order.

req must have Contract and (unless Close) Side and a whole-contract Size. Price is
required for limit orders and ignored for market orders. Returns an OrderInfo with
OrderID/ClientOrderID/CreatedAtMs/RateLimits populated. SDK validation errors are
returned WITHOUT sending a request.
*/
func (t *TradingClient) CreateOrder(ctx context.Context, req types.CreateOrderRequest) (types.OrderInfo, error) {
	var info types.OrderInfo
	var body map[string]any
	var err error
	body, err = t.buildCreateOrderBody(req)
	if err != nil {
		return info, err
	}

	var resp rest.Response
	var rateLimits map[string]string
	resp, rateLimits, err = t.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   t.ordersPath(),
		Body:   body,
		Signed: true,
		Meta: rest.RequestMeta{
			OrderCount: 1,
			Symbols:    []string{req.Contract},
			Category:   string(gate.RateLimitCategoryPlace),
		},
	})
	if err != nil {
		return info, err
	}

	var p optionsOrderPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "trading.CreateOrder: parse", err)
	}
	info = orderInfoFromPayload(&p, rateLimits)
	t.rememberMapping(info.ClientOrderID, info.OrderID, info.CreatedAtMs)
	return info, nil
}

// buildCreateOrderBody assembles the Gate request body. A map is used because
// Gate omits absent optional fields; the required ones are always present.
func (t *TradingClient) buildCreateOrderBody(req types.CreateOrderRequest) (map[string]any, error) {
	if req.Contract == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CreateOrder: Contract is empty", nil)
	}

	var sizeI int64
	var err error
	if !req.Close {
		sizeI, err = encodeSignedSize(req.Side, req.Size)
		if err != nil {
			return nil, err
		}
	}

	var priceStr string
	var tif types.TimeInForceType
	priceStr, tif, err = resolvePriceAndTIF(req)
	if err != nil {
		return nil, err
	}

	var text string
	text, err = normalizeClientID(req.Text)
	if err != nil {
		return nil, err
	}

	var body map[string]any = make(map[string]any, 9)
	body["contract"] = req.Contract
	body["size"] = sizeI
	body["price"] = priceStr
	body["tif"] = string(tif)
	if text != "" {
		body["text"] = text
	}
	if req.ReduceOnly {
		body["reduce_only"] = true
	}
	if req.Close {
		body["close"] = true
	}
	if req.Mmp {
		body["mmp"] = true
	}
	if req.StpAct != "" {
		body["stp_act"] = req.StpAct
	}
	return body, nil
}

// encodeSignedSize validates the size (whole, positive) and applies the Gate
// sign convention from the side.
func encodeSignedSize(side types.SideType, size decimal.Decimal) (int64, error) {
	if size.IsZero() {
		return 0, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CreateOrder: Size is zero", nil)
	}
	if !size.Equal(size.Truncate(0)) {
		return 0, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CreateOrder: Size must be a whole number of contracts", nil)
	}
	var n int64 = size.IntPart()
	if n < 0 {
		n = -n
	}
	switch side {
	case types.SideTypeBuy:
		return n, nil
	case types.SideTypeSell:
		return -n, nil
	default:
		return 0, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CreateOrder: Side must be buy or sell", nil)
	}
}

// resolvePriceAndTIF decides the Gate price string and tif from the request,
// inferring market vs limit when OrderType is empty.
func resolvePriceAndTIF(req types.CreateOrderRequest) (string, types.TimeInForceType, error) {
	var isMarket bool
	switch req.OrderType {
	case types.OrderTypeMarket:
		isMarket = true
	case types.OrderTypeLimit:
		isMarket = false
	default:
		isMarket = req.Price.IsZero()
	}

	var tif types.TimeInForceType = req.TimeInForce
	if isMarket {
		if tif == "" {
			tif = types.TimeInForceIOC
		}
		if tif != types.TimeInForceIOC && tif != types.TimeInForceFOK {
			return "", "", gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CreateOrder: market order requires tif ioc or fok", nil)
		}
		return "0", tif, nil
	}

	if req.Price.IsZero() {
		return "", "", gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CreateOrder: Price is required for a limit order", nil)
	}
	if tif == "" {
		tif = types.TimeInForceGTC
	}
	return req.Price.String(), tif, nil
}

// ---- ModifyOrder -----------------------------------------------------------

/*
ModifyOrder amends an order's price and/or size (Gate keeps the side fixed). One
of OrderID / ClientOrderID identifies the order; when NewSize is set, Side is
required to re-apply the Gate signed-size convention (same model as futures).
*/
func (t *TradingClient) ModifyOrder(ctx context.Context, req types.ModifyOrderRequest) (types.OrderInfo, error) {
	var info types.OrderInfo
	var body map[string]any
	var err error
	body, err = buildAmendBody(req)
	if err != nil {
		return info, err
	}
	var id string = orderIDPath(req.OrderID, req.ClientOrderID)
	if id == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.ModifyOrder: OrderID or ClientOrderID is required", nil)
	}

	var resp rest.Response
	var rateLimits map[string]string
	resp, rateLimits, err = t.c.rest().Do(ctx, rest.Options{
		Method: "PUT",
		Path:   t.orderPath(id),
		Body:   body,
		Signed: true,
		Meta: rest.RequestMeta{
			OrderCount: 1,
			Symbols:    []string{req.Contract},
			Category:   string(gate.RateLimitCategoryAmend),
		},
	})
	if err != nil {
		return info, err
	}

	var p optionsOrderPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "trading.ModifyOrder: parse", err)
	}
	info = orderInfoFromPayload(&p, rateLimits)
	return info, nil
}

// buildAmendBody constructs the Gate amend body {size?, price?, amend_text?}.
// Gate's amend size is signed, so a size change requires Side.
func buildAmendBody(req types.ModifyOrderRequest) (map[string]any, error) {
	if (req.OrderID == "") == (req.ClientOrderID == "") {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.ModifyOrder: exactly one of OrderID/ClientOrderID must be set", nil)
	}
	if req.NewSize.IsZero() && req.NewPrice.IsZero() {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.ModifyOrder: NewSize or NewPrice must be set", nil)
	}
	var body map[string]any = make(map[string]any, 3)
	if !req.NewSize.IsZero() {
		var sizeI int64
		var err error
		sizeI, err = encodeSignedSize(req.Side, req.NewSize)
		if err != nil {
			return nil, err
		}
		body["size"] = sizeI
	}
	if !req.NewPrice.IsZero() {
		body["price"] = req.NewPrice.String()
	}
	if req.AmendText != "" {
		body["amend_text"] = req.AmendText
	}
	return body, nil
}

// ---- CancelOrder / CancelAllOrders -----------------------------------------

// CancelOrder cancels a single order by OrderID or ClientOrderID.
func (t *TradingClient) CancelOrder(ctx context.Context, req types.CancelOrderRequest) error {
	var id string = orderIDPath(req.OrderID, req.ClientOrderID)
	if id == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CancelOrder: OrderID or ClientOrderID is required", nil)
	}
	var err error
	_, _, err = t.c.rest().Do(ctx, rest.Options{
		Method: "DELETE",
		Path:   t.orderPath(id),
		Signed: true,
		Meta: rest.RequestMeta{
			OrderCount: 1,
			Symbols:    []string{req.Contract},
			Category:   string(gate.RateLimitCategoryCancel),
		},
	})
	if err != nil {
		return err
	}
	t.forgetMapping(req.ClientOrderID, req.OrderID)
	return nil
}

/*
CancelAllOrders cancels all open orders via Gate's native DELETE /options/orders.
The cancellation is scoped by any combination of contract, underlying and side
(side "ask"/"bid" — empty cancels both); all three empty cancels every open order.
*/
func (t *TradingClient) CancelAllOrders(ctx context.Context, contract, underlying, side string) error {
	var q = newQuery()
	var symbols []string
	if contract != "" {
		q.Set("contract", contract)
		symbols = []string{contract}
	}
	if underlying != "" {
		q.Set("underlying", underlying)
	}
	if side != "" {
		q.Set("side", side)
	}
	var err error
	_, _, err = t.c.rest().Do(ctx, rest.Options{
		Method: "DELETE",
		Path:   t.ordersPath(),
		Query:  q,
		Signed: true,
		Meta: rest.RequestMeta{
			Symbols:  symbols,
			Category: string(gate.RateLimitCategoryCancel),
		},
	})
	return err
}

/*
CountdownCancelAll arms (or disarms) Gate's server-side dead-man's switch: if the
client does not call again within timeout, Gate cancels all open orders (optionally
scoped to a contract). timeout=0 disarms. Returns the trigger time in epoch ms.
*/
func (t *TradingClient) CountdownCancelAll(ctx context.Context, timeout time.Duration, contract string) (int64, error) {
	if timeout < 0 {
		return 0, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CountdownCancelAll: timeout must be >= 0", nil)
	}
	var body map[string]any = map[string]any{"timeout": int64(timeout / time.Second)}
	if contract != "" {
		body["contract"] = contract
	}

	var resp rest.Response
	var err error
	resp, _, err = t.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   t.countdownPath(),
		Body:   body,
		Signed: true,
		Meta: rest.RequestMeta{
			Category: string(gate.RateLimitCategoryCancel),
		},
	})
	if err != nil {
		return 0, err
	}
	var out struct {
		TriggerTime float64 `json:"trigger_time"`
	}
	if err = resp.UnmarshalData(&out); err != nil {
		return 0, gate.NewError(gate.ErrorKindUnknown, "", "trading.CountdownCancelAll: parse", err)
	}
	return int64(out.TriggerTime * 1000), nil
}

// ---- GetOrder / GetOpenOrders ----------------------------------------------

// GetOrder fetches a single order by its numeric id or client text.
func (t *TradingClient) GetOrder(ctx context.Context, contract, idOrText string) (types.OrderInfo, error) {
	var info types.OrderInfo
	if idOrText == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.GetOrder: id is empty", nil)
	}
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = t.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   t.orderPath(idOrText),
		Signed: true,
		Meta: rest.RequestMeta{
			Symbols:  []string{contract},
			Category: string(gate.RateLimitCategoryQuery),
		},
	})
	if err != nil {
		return info, err
	}
	var p optionsOrderPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "trading.GetOrder: parse", err)
	}
	info = orderInfoFromPayload(&p, rateLimits)
	t.rememberMapping(info.ClientOrderID, info.OrderID, info.CreatedAtMs)
	return info, nil
}

/*
GetOpenOrders lists open orders (GET /options/orders?status=open). The query is
scoped by contract and/or underlying; pass both empty for every open order.
*/
func (t *TradingClient) GetOpenOrders(ctx context.Context, contract, underlying string) ([]types.OrderInfo, error) {
	var q = newQuery()
	q.Set("status", types.OrderStatusOpen)
	var symbols []string
	if contract != "" {
		q.Set("contract", contract)
		symbols = []string{contract}
	}
	if underlying != "" {
		q.Set("underlying", underlying)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = t.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   t.ordersPath(),
		Query:  q,
		Signed: true,
		Meta: rest.RequestMeta{
			Symbols:  symbols,
			Category: string(gate.RateLimitCategoryQuery),
		},
	})
	if err != nil {
		return nil, err
	}

	var payloads []optionsOrderPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "trading.GetOpenOrders: parse", err)
	}
	var out []types.OrderInfo = make([]types.OrderInfo, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, orderInfoFromPayload(&payloads[i], rateLimits))
	}
	return out, nil
}

// ---- GetMyTrades -----------------------------------------------------------

type myTradePayload struct {
	ID         int64   `json:"id"`
	CreateTime float64 `json:"create_time"`
	Contract   string  `json:"contract"`
	OrderID    int64   `json:"order_id"`
	Size       int64   `json:"size"`
	Price      string  `json:"price"`
	Role       string  `json:"role"`
	Text       string  `json:"text"`
}

/*
GetMyTrades returns the account's own fills. The query is scoped by contract
and/or underlying; limit ≤ 0 / offset ≤ 0 let Gate use its defaults.
*/
func (t *TradingClient) GetMyTrades(ctx context.Context, contract, underlying string, limit, offset int) ([]types.UserTrade, error) {
	var q = newQuery()
	var symbols []string
	if contract != "" {
		q.Set("contract", contract)
		symbols = []string{contract}
	}
	if underlying != "" {
		q.Set("underlying", underlying)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = t.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   t.myTradesPath(),
		Query:  q,
		Signed: true,
		Meta: rest.RequestMeta{
			Symbols:  symbols,
			Category: string(gate.RateLimitCategoryQuery),
		},
	})
	if err != nil {
		return nil, err
	}
	var payloads []myTradePayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "trading.GetMyTrades: parse", err)
	}
	_ = rateLimits
	var out []types.UserTrade = make([]types.UserTrade, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		var orderID string
		if payloads[i].OrderID != 0 {
			orderID = strconv.FormatInt(payloads[i].OrderID, 10)
		}
		var tradeID string
		if payloads[i].ID != 0 {
			tradeID = strconv.FormatInt(payloads[i].ID, 10)
		}
		out = append(out, types.UserTrade{
			ID:            tradeID,
			Contract:      payloads[i].Contract,
			OrderID:       orderID,
			ClientOrderID: payloads[i].Text,
			Price:         mustDecimal(payloads[i].Price),
			Size:          decimalAbsInt(payloads[i].Size),
			Side:          sideFromSize(payloads[i].Size),
			Role:          payloads[i].Role,
			Ts:            floatSecondsOrMsToMs(payloads[i].CreateTime),
		})
	}
	return out, nil
}

// ---- MMP (Market-Maker Protection) -----------------------------------------

type mmpPayload struct {
	Underlying     string `json:"underlying"`
	Window         int64  `json:"window"`
	FrozenPeriod   int64  `json:"frozen_period"`
	QtyLimit       string `json:"qty_limit"`
	DeltaLimit     string `json:"delta_limit"`
	MmpFrozenUntil int64  `json:"mmp_frozen_until"`
}

func mmpInfoFromPayload(p *mmpPayload, rateLimits map[string]string) types.MMPInfo {
	return types.MMPInfo{
		Underlying:       p.Underlying,
		Window:           p.Window,
		FrozenPeriod:     p.FrozenPeriod,
		QtyLimit:         mustDecimal(p.QtyLimit),
		DeltaLimit:       mustDecimal(p.DeltaLimit),
		MmpFrozenUntilMs: p.MmpFrozenUntil,
		RateLimits:       rateLimits,
	}
}

// GetMMP returns the Market-Maker Protection settings/state for an underlying.
func (t *TradingClient) GetMMP(ctx context.Context, underlying string) (types.MMPInfo, error) {
	var info types.MMPInfo
	if underlying == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.GetMMP: underlying is empty", nil)
	}
	var q = newQuery()
	q.Set("underlying", underlying)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = t.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   t.mmpPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p mmpPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "trading.GetMMP: parse", err)
	}
	return mmpInfoFromPayload(&p, rateLimits), nil
}

// SetMMP configures Market-Maker Protection for an underlying and returns the
// resulting state.
func (t *TradingClient) SetMMP(ctx context.Context, req types.MMPSettings) (types.MMPInfo, error) {
	var info types.MMPInfo
	if req.Underlying == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.SetMMP: Underlying is empty", nil)
	}
	var body map[string]any = make(map[string]any, 5)
	body["underlying"] = req.Underlying
	body["window"] = req.Window
	body["frozen_period"] = req.FrozenPeriod
	if !req.QtyLimit.IsZero() {
		body["qty_limit"] = req.QtyLimit.String()
	}
	if !req.DeltaLimit.IsZero() {
		body["delta_limit"] = req.DeltaLimit.String()
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = t.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   t.mmpPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p mmpPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "trading.SetMMP: parse", err)
	}
	return mmpInfoFromPayload(&p, rateLimits), nil
}

// ResetMMP clears a triggered MMP freeze for an underlying, re-enabling order
// placement.
func (t *TradingClient) ResetMMP(ctx context.Context, underlying string) error {
	if underlying == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.ResetMMP: underlying is empty", nil)
	}
	var body map[string]any = map[string]any{"underlying": underlying}
	var err error
	_, _, err = t.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   t.mmpResetPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	return err
}

// ---- ID mapping ------------------------------------------------------------

// SyncOrderMappings reloads ClientOrderID ↔ OrderID mappings from the exchange's
// open orders and drops mappings no longer present.
func (t *TradingClient) SyncOrderMappings(ctx context.Context, contract, underlying string) error {
	var open []types.OrderInfo
	var err error
	open, err = t.GetOpenOrders(ctx, contract, underlying)
	if err != nil {
		return err
	}
	var seen map[string]struct{} = make(map[string]struct{}, len(open))
	var i int
	for i = 0; i < len(open); i++ {
		if open[i].ClientOrderID != "" && open[i].OrderID != "" {
			t.rememberMapping(open[i].ClientOrderID, open[i].OrderID, open[i].CreatedAtMs)
			seen[open[i].ClientOrderID] = struct{}{}
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	var clOrdID, ordID string
	for clOrdID, ordID = range t.clOrdToOrd {
		if _, ok := seen[clOrdID]; !ok {
			delete(t.clOrdToOrd, clOrdID)
			delete(t.ordToClOrd, ordID)
			delete(t.createdAtMs, clOrdID)
		}
	}
	return nil
}

// OrderIDByClientID returns the OrderID for a known ClientOrderID.
func (t *TradingClient) OrderIDByClientID(clientOrderID string) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var v string
	var ok bool
	v, ok = t.clOrdToOrd[clientOrderID]
	return v, ok
}

// ClientIDByOrderID returns the ClientOrderID for a known OrderID.
func (t *TradingClient) ClientIDByOrderID(orderID string) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var v string
	var ok bool
	v, ok = t.ordToClOrd[orderID]
	return v, ok
}

func (t *TradingClient) rememberMapping(clientOrderID, orderID string, createdAtMs int64) {
	if clientOrderID == "" || orderID == "" {
		return
	}
	t.mu.Lock()
	t.clOrdToOrd[clientOrderID] = orderID
	t.ordToClOrd[orderID] = clientOrderID
	t.createdAtMs[clientOrderID] = createdAtMs
	t.mu.Unlock()
}

func (t *TradingClient) forgetMapping(clientOrderID, orderID string) {
	if clientOrderID == "" && orderID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if clientOrderID == "" {
		clientOrderID = t.ordToClOrd[orderID]
	}
	if orderID == "" {
		orderID = t.clOrdToOrd[clientOrderID]
	}
	delete(t.clOrdToOrd, clientOrderID)
	delete(t.ordToClOrd, orderID)
	delete(t.createdAtMs, clientOrderID)
}
