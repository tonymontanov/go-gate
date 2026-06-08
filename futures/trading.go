/*
FILE: futures/trading.go

DESCRIPTION:
Trading sub-client for the Gate Futures section. Implements:
  - CreateOrder          : POST   /futures/{settle}/orders
  - CreateBatchOrders    : POST   /futures/{settle}/batch_orders
  - ModifyOrder          : PUT    /futures/{settle}/orders/{order_id}
  - ModifyBatchOrders    : sequential ModifyOrder (native batch_amend deferred)
  - CancelOrder          : DELETE /futures/{settle}/orders/{order_id}
  - CancelBatchOrders    : POST   /futures/{settle}/batch_cancel_orders
  - CancelAllOrders      : DELETE /futures/{settle}/orders?contract=  (native)
  - CancelForgottenOrders: GetOpenOrders + age filter + CancelBatchOrders
  - CountdownCancelAll   : POST   /futures/{settle}/countdown_cancel_all
  - GetOrder             : GET    /futures/{settle}/orders/{order_id}
  - GetOpenOrders        : GET    /futures/{settle}/orders?status=open&contract=

GATE SPECIFICS encoded here:
  - direction is the sign of the integer order size (no side field);
  - market order = price="0" + tif="ioc"; limit defaults to tif="gtc";
  - the client order id ("text") is auto-prefixed "t-" and validated;
  - single endpoints return the FuturesOrder object directly (no per-entry sCode
    envelope); batch endpoints return arrays with per-element succeeded/label.

It also keeps a ClientOrderID ↔ OrderID map, populated on create and on
SyncOrderMappings, for the desk's idempotency/mapping needs.
*/

package futures

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/futures/types"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
)

// newQuery returns an empty url.Values for building REST query strings.
func newQuery() url.Values { return url.Values{} }

// TradingClient — trading sub-client.
type TradingClient struct {
	c *Client

	mu          sync.RWMutex
	clOrdToOrd  map[string]string
	ordToClOrd  map[string]string
	createdAtMs map[string]int64 // clientOrderID -> ms, for CancelForgottenOrders
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

func (t *TradingClient) ordersPath() string      { return t.c.basePath() + "/orders" }
func (t *TradingClient) batchOrdersPath() string { return t.c.basePath() + "/batch_orders" }
func (t *TradingClient) batchCancelPath() string { return t.c.basePath() + "/batch_cancel_orders" }
func (t *TradingClient) countdownPath() string   { return t.c.basePath() + "/countdown_cancel_all" }
func (t *TradingClient) orderPath(id string) string {
	return t.c.basePath() + "/orders/" + id
}

// ---- CreateOrder -----------------------------------------------------------

/*
CreateOrder creates a futures order.

req must have Contract and (unless Close/AutoSize) Side and a whole-contract Size.
Price is required for limit orders and ignored for market orders. Returns an
OrderInfo with OrderID/ClientOrderID/CreatedAtMs/RateLimits populated. SDK
validation errors are returned WITHOUT sending a request.
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

	var p futuresOrderPayload
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

	var isClose bool = req.Close || req.AutoSize != ""

	var sizeI int64
	var err error
	if !isClose {
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

	var body map[string]any = make(map[string]any, 10)
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
	if req.AutoSize != "" {
		body["auto_size"] = req.AutoSize
	}
	if !req.Iceberg.IsZero() {
		body["iceberg"] = req.Iceberg.IntPart()
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

// ---- CreateBatchOrders -----------------------------------------------------

/*
CreateBatchOrders creates a batch of orders via POST /futures/{settle}/batch_orders.
Returns an OrderInfo per input request, in the same order. For elements Gate
rejected (succeeded=false) the OrderInfo carries the contract/side/size echoed
from the request with an empty OrderID, and an aggregated error (errors.Join of
each rejected element's label) is returned so the caller can react.
*/
func (t *TradingClient) CreateBatchOrders(ctx context.Context, reqs []types.CreateOrderRequest) ([]types.OrderInfo, error) {
	if len(reqs) == 0 {
		return nil, nil
	}

	var bodies []map[string]any = make([]map[string]any, 0, len(reqs))
	var bodyErrs []error
	var err error
	var i int
	for i = 0; i < len(reqs); i++ {
		var b map[string]any
		b, err = t.buildCreateOrderBody(reqs[i])
		if err != nil {
			bodyErrs = append(bodyErrs, fmt.Errorf("batch[%d]: %w", i, err))
			continue
		}
		bodies = append(bodies, b)
	}
	if len(bodies) == 0 {
		return placeholderCreateInfos(reqs), errors.Join(bodyErrs...)
	}

	var resp rest.Response
	var rateLimits map[string]string
	resp, rateLimits, err = t.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   t.batchOrdersPath(),
		Body:   bodies,
		Signed: true,
		Meta: rest.RequestMeta{
			OrderCount: len(bodies),
			Symbols:    uniqueContractsCreate(reqs),
			Category:   string(gate.RateLimitCategoryPlace),
		},
	})
	if err != nil {
		return placeholderCreateInfos(reqs), err
	}

	var entries []batchFuturesOrderPayload
	if err = resp.UnmarshalData(&entries); err != nil {
		return placeholderCreateInfos(reqs), gate.NewError(gate.ErrorKindUnknown, "", "trading.CreateBatchOrders: parse", err)
	}

	var infos []types.OrderInfo = make([]types.OrderInfo, 0, len(entries))
	var aggErrs []error = bodyErrs
	var now int64 = time.Now().UnixMilli()
	for i = 0; i < len(entries); i++ {
		var e batchFuturesOrderPayload = entries[i]
		if e.Succeeded != nil && !*e.Succeeded {
			aggErrs = append(aggErrs, &gate.Error{
				Kind:    gate.MapLabel(e.Label, 400),
				Label:   e.Label,
				Message: e.Detail,
			})
			infos = append(infos, types.OrderInfo{Contract: e.Contract, ClientOrderID: e.Text})
			continue
		}
		var info types.OrderInfo = orderInfoFromPayload(&e.futuresOrderPayload, rateLimits)
		if info.CreatedAtMs == 0 {
			info.CreatedAtMs = now
		}
		t.rememberMapping(info.ClientOrderID, info.OrderID, info.CreatedAtMs)
		infos = append(infos, info)
	}

	if len(aggErrs) > 0 {
		return infos, errors.Join(aggErrs...)
	}
	return infos, nil
}

// placeholderCreateInfos echoes request fields for the case where nothing was
// sent (so the caller keeps positional correspondence).
func placeholderCreateInfos(reqs []types.CreateOrderRequest) []types.OrderInfo {
	var out []types.OrderInfo = make([]types.OrderInfo, 0, len(reqs))
	var i int
	for i = 0; i < len(reqs); i++ {
		out = append(out, types.OrderInfo{
			Contract:      reqs[i].Contract,
			Side:          reqs[i].Side,
			Price:         reqs[i].Price,
			Size:          reqs[i].Size,
			ClientOrderID: reqs[i].Text,
		})
	}
	return out
}

// ---- ModifyOrder -----------------------------------------------------------

/*
ModifyOrder amends an order's price and/or size (Gate keeps the side fixed). One
of OrderID / ClientOrderID identifies the order; when NewSize is set, Side is
required to re-apply the Gate signed-size convention.
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

	var p futuresOrderPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "trading.ModifyOrder: parse", err)
	}
	info = orderInfoFromPayload(&p, rateLimits)
	return info, nil
}

// buildAmendBody constructs the Gate amend body {size?, price?, amend_text?}.
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

/*
ModifyBatchOrders amends several orders. v1.0 issues sequential ModifyOrder calls
(Gate's native batch_amend_orders item shape — size vs amount naming — is pending
fixture calibration; sequential amend is functionally equivalent). Returns an
OrderInfo per request with an aggregated error for any failures.
*/
func (t *TradingClient) ModifyBatchOrders(ctx context.Context, reqs []types.ModifyOrderRequest) ([]types.OrderInfo, error) {
	if len(reqs) == 0 {
		return nil, nil
	}
	var out []types.OrderInfo = make([]types.OrderInfo, 0, len(reqs))
	var aggErrs []error
	var i int
	for i = 0; i < len(reqs); i++ {
		var info types.OrderInfo
		var err error
		info, err = t.ModifyOrder(ctx, reqs[i])
		if err != nil {
			aggErrs = append(aggErrs, fmt.Errorf("amend[%d]: %w", i, err))
		}
		out = append(out, info)
	}
	if len(aggErrs) > 0 {
		return out, errors.Join(aggErrs...)
	}
	return out, nil
}

// ---- CancelOrder / CancelAllOrders / CancelBatchOrders ---------------------

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

// CancelAllOrders cancels all open orders for a contract via Gate's native
// DELETE /futures/{settle}/orders?contract=.
func (t *TradingClient) CancelAllOrders(ctx context.Context, contract string) error {
	if contract == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CancelAllOrders: contract is empty", nil)
	}
	var q = newQuery()
	q.Set("contract", contract)
	var err error
	_, _, err = t.c.rest().Do(ctx, rest.Options{
		Method: "DELETE",
		Path:   t.ordersPath(),
		Query:  q,
		Signed: true,
		Meta: rest.RequestMeta{
			Symbols:  []string{contract},
			Category: string(gate.RateLimitCategoryCancel),
		},
	})
	return err
}

// CancelBatchOrders cancels a batch of orders by id via Gate's native
// POST /futures/{settle}/batch_cancel_orders (body: array of order ids). Returns
// an aggregated error for any element Gate reports as not succeeded.
func (t *TradingClient) CancelBatchOrders(ctx context.Context, reqs []types.CancelOrderRequest) error {
	if len(reqs) == 0 {
		return nil
	}
	var ids []string = make([]string, 0, len(reqs))
	var i int
	for i = 0; i < len(reqs); i++ {
		var id string = orderIDPath(reqs[i].OrderID, reqs[i].ClientOrderID)
		if id == "" {
			return gate.NewError(gate.ErrorKindInvalidRequest, "", fmt.Sprintf("trading.CancelBatchOrders: req[%d] has no OrderID/ClientOrderID", i), nil)
		}
		ids = append(ids, id)
	}

	var resp rest.Response
	var err error
	resp, _, err = t.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   t.batchCancelPath(),
		Body:   ids,
		Signed: true,
		Meta: rest.RequestMeta{
			OrderCount: len(ids),
			Symbols:    uniqueContractsCancel(reqs),
			Category:   string(gate.RateLimitCategoryCancel),
		},
	})
	if err != nil {
		return err
	}

	var results []cancelResultPayload
	if err = resp.UnmarshalData(&results); err != nil {
		return gate.NewError(gate.ErrorKindUnknown, "", "trading.CancelBatchOrders: parse", err)
	}
	var aggErrs []error
	for i = 0; i < len(results); i++ {
		if results[i].Succeeded != nil && !*results[i].Succeeded {
			var msg string = results[i].Message
			if msg == "" {
				msg = results[i].Detail
			}
			aggErrs = append(aggErrs, &gate.Error{
				Kind:    gate.MapLabel(results[i].Label, 400),
				Label:   results[i].Label,
				Message: msg,
			})
			continue
		}
		t.forgetMapping("", results[i].ID)
	}
	if len(aggErrs) > 0 {
		return errors.Join(aggErrs...)
	}
	return nil
}

/*
CancelForgottenOrders cancels open orders older than maxAge, using CreatedAtMs
from GetOpenOrders. Returns the list of cancelled orders.
*/
func (t *TradingClient) CancelForgottenOrders(ctx context.Context, contract string, maxAge time.Duration) ([]types.OrderInfo, error) {
	if contract == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CancelForgottenOrders: contract is empty", nil)
	}
	if maxAge <= 0 {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CancelForgottenOrders: maxAge must be positive", nil)
	}

	var open []types.OrderInfo
	var err error
	open, err = t.GetOpenOrders(ctx, contract)
	if err != nil {
		return nil, err
	}

	var thresholdMs int64 = time.Now().UnixMilli() - maxAge.Milliseconds()
	var stale []types.OrderInfo
	var reqs []types.CancelOrderRequest
	var i int
	for i = 0; i < len(open); i++ {
		if open[i].CreatedAtMs > 0 && open[i].CreatedAtMs <= thresholdMs {
			stale = append(stale, open[i])
			reqs = append(reqs, types.CancelOrderRequest{Contract: contract, OrderID: open[i].OrderID})
		}
	}
	if len(reqs) == 0 {
		return nil, nil
	}
	err = t.CancelBatchOrders(ctx, reqs)
	return stale, err
}

/*
CountdownCancelAll arms (or disarms) Gate's server-side dead-man's switch: if the
client does not call again within timeout, Gate cancels all open orders (optionally
scoped to contract). timeout=0 disarms. Returns the trigger time in epoch ms.

HFT usage: refresh every ~⅓ of timeout in the hot loop.
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
	var p futuresOrderPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "trading.GetOrder: parse", err)
	}
	return orderInfoFromPayload(&p, rateLimits), nil
}

// GetOpenOrders lists open orders for a contract
// (GET /futures/{settle}/orders?status=open&contract=).
func (t *TradingClient) GetOpenOrders(ctx context.Context, contract string) ([]types.OrderInfo, error) {
	if contract == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.GetOpenOrders: contract is empty", nil)
	}
	var q = newQuery()
	q.Set("status", types.OrderStatusOpen)
	q.Set("contract", contract)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = t.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   t.ordersPath(),
		Query:  q,
		Signed: true,
		Meta: rest.RequestMeta{
			Symbols:  []string{contract},
			Category: string(gate.RateLimitCategoryQuery),
		},
	})
	if err != nil {
		return nil, err
	}

	var payloads []futuresOrderPayload
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

// ---- ID mapping ------------------------------------------------------------

// SyncOrderMappings reloads ClientOrderID ↔ OrderID mappings from the exchange's
// open orders and drops mappings no longer present.
func (t *TradingClient) SyncOrderMappings(ctx context.Context, contract string) error {
	var open []types.OrderInfo
	var err error
	open, err = t.GetOpenOrders(ctx, contract)
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

// ---- helpers ---------------------------------------------------------------

// uniqueContractsCreate returns the sorted unique contract set of a create batch,
// for the rate-limit observer's per-contract accounting.
func uniqueContractsCreate(reqs []types.CreateOrderRequest) []string {
	var set map[string]struct{} = make(map[string]struct{}, len(reqs))
	var i int
	for i = 0; i < len(reqs); i++ {
		if reqs[i].Contract != "" {
			set[reqs[i].Contract] = struct{}{}
		}
	}
	return sortedKeys(set)
}

// uniqueContractsCancel — same for a cancel batch.
func uniqueContractsCancel(reqs []types.CancelOrderRequest) []string {
	var set map[string]struct{} = make(map[string]struct{}, len(reqs))
	var i int
	for i = 0; i < len(reqs); i++ {
		if reqs[i].Contract != "" {
			set[reqs[i].Contract] = struct{}{}
		}
	}
	return sortedKeys(set)
}

func sortedKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	var out []string = make([]string, 0, len(set))
	var s string
	for s = range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
