/*
FILE: spot/trading.go

DESCRIPTION:
Trading sub-client for the Gate Spot section. Implements:
  - CreateOrder          : POST   /spot/orders
  - CreateBatchOrders    : POST   /spot/batch_orders
  - ModifyOrder          : PATCH  /spot/orders/{order_id}?currency_pair=
  - ModifyBatchOrders    : POST   /spot/amend_batch_orders (native, ≤5/req)
  - CancelOrder          : DELETE /spot/orders/{order_id}?currency_pair=
  - CancelBatchOrders    : POST   /spot/cancel_batch_orders  (body: [{currency_pair,id}])
  - CancelAllOrders      : DELETE /spot/orders?currency_pair=
  - CancelForgottenOrders: GetOpenOrders + age filter + CancelBatchOrders
  - CountdownCancelAll   : POST   /spot/countdown_cancel_all
  - GetOrder             : GET    /spot/orders/{order_id}?currency_pair=
  - GetOpenOrders        : GET    /spot/orders?currency_pair=&status=open

GATE SPOT SPECIFICS encoded here (vs futures):
  - side and type are explicit fields; amount is in base currency;
  - a MARKET BUY's amount is the QUOTE amount, and market orders require
    tif ioc/fok and carry no price;
  - amend is PATCH (futures uses PUT); cancel needs the currency pair;
  - the client order id ("text") is auto-prefixed "t-" and validated.

It also keeps a ClientOrderID ↔ OrderID map, populated on create and on
SyncOrderMappings, for the desk's idempotency/mapping needs.
*/

package spot

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"sync"
	"time"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
	"github.com/tonymontanov/go-gate/v2/spot/types"
)

// newQuery returns an empty url.Values for building REST query strings.
func newQuery() url.Values { return url.Values{} }

// TradingClient — spot trading sub-client.
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

func (t *TradingClient) ordersPath() string      { return "/spot/orders" }
func (t *TradingClient) batchOrdersPath() string { return "/spot/batch_orders" }
func (t *TradingClient) batchAmendPath() string  { return "/spot/amend_batch_orders" }
func (t *TradingClient) batchCancelPath() string { return "/spot/cancel_batch_orders" }
func (t *TradingClient) countdownPath() string   { return "/spot/countdown_cancel_all" }

// maxBatchAmend is Gate's per-request cap for POST /spot/amend_batch_orders
// ("up to 5 orders per request"). ModifyBatchOrders chunks larger inputs.
const maxBatchAmend = 5

func (t *TradingClient) orderPath(id string) string {
	return "/spot/orders/" + id
}

// ---- CreateOrder -----------------------------------------------------------

/*
CreateOrder creates a spot order.

req must have CurrencyPair, Side, and a positive Amount (base currency; QUOTE
amount for a market BUY). Price is required for limit orders and omitted for
market orders. Returns an OrderInfo with OrderID/ClientOrderID/CreatedAtMs/
RateLimits populated. SDK validation errors are returned WITHOUT sending a request.
*/
func (t *TradingClient) CreateOrder(ctx context.Context, req types.CreateOrderRequest) (types.OrderInfo, error) {
	var info types.OrderInfo
	var body map[string]any
	var err error
	body, err = buildCreateOrderBody(req)
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
			Symbols:    []string{req.CurrencyPair},
			Category:   string(gate.RateLimitCategoryPlace),
		},
	})
	if err != nil {
		return info, err
	}

	var p spotOrderPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "trading.CreateOrder: parse", err)
	}
	info = orderInfoFromPayload(&p, rateLimits)
	t.rememberMapping(info.ClientOrderID, info.OrderID, info.CreatedAtMs)
	return info, nil
}

// buildCreateOrderBody assembles the Gate request body. A map is used because
// Gate omits absent optional fields; the required ones are always present.
func buildCreateOrderBody(req types.CreateOrderRequest) (map[string]any, error) {
	if req.CurrencyPair == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CreateOrder: CurrencyPair is empty", nil)
	}
	if req.Side != types.SideTypeBuy && req.Side != types.SideTypeSell {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CreateOrder: Side must be buy or sell", nil)
	}
	if req.Amount.Sign() <= 0 {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CreateOrder: Amount must be positive", nil)
	}

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
			return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CreateOrder: market order requires tif ioc or fok", nil)
		}
	} else {
		if req.Price.Sign() <= 0 {
			return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CreateOrder: Price is required for a limit order", nil)
		}
		if tif == "" {
			tif = types.TimeInForceGTC
		}
	}

	var account types.AccountType = req.Account
	if account == "" {
		account = types.AccountSpot
	}

	var text string
	var err error
	text, err = normalizeClientID(req.Text)
	if err != nil {
		return nil, err
	}

	var orderType types.OrderType = types.OrderTypeLimit
	if isMarket {
		orderType = types.OrderTypeMarket
	}

	var body map[string]any = make(map[string]any, 9)
	body["currency_pair"] = req.CurrencyPair
	body["side"] = string(req.Side)
	body["amount"] = req.Amount.String()
	body["type"] = string(orderType)
	body["time_in_force"] = string(tif)
	body["account"] = string(account)
	if !isMarket {
		body["price"] = req.Price.String()
	}
	if text != "" {
		body["text"] = text
	}
	if req.Iceberg.Sign() > 0 {
		body["iceberg"] = req.Iceberg.String()
	}
	return body, nil
}

// ---- CreateBatchOrders -----------------------------------------------------

/*
CreateBatchOrders creates a batch of orders via POST /spot/batch_orders. Returns
an OrderInfo per input request, in the same order. For elements Gate rejected
(succeeded=false) the OrderInfo carries the pair/side/amount echoed from the
request with an empty OrderID, and an aggregated error (errors.Join of each
rejected element's label) is returned so the caller can react.
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
		b, err = buildCreateOrderBody(reqs[i])
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

	var entries []batchSpotOrderPayload
	if err = resp.UnmarshalData(&entries); err != nil {
		return placeholderCreateInfos(reqs), gate.NewError(gate.ErrorKindUnknown, "", "trading.CreateBatchOrders: parse", err)
	}

	var infos []types.OrderInfo = make([]types.OrderInfo, 0, len(entries))
	var aggErrs []error = bodyErrs
	var now int64 = time.Now().UnixMilli()
	for i = 0; i < len(entries); i++ {
		var e batchSpotOrderPayload = entries[i]
		if e.Succeeded != nil && !*e.Succeeded {
			var msg string = e.Message
			if msg == "" {
				msg = e.Detail
			}
			aggErrs = append(aggErrs, &gate.Error{
				Kind:    gate.MapLabel(e.Label, 400),
				Label:   e.Label,
				Message: msg,
			})
			infos = append(infos, types.OrderInfo{CurrencyPair: e.CurrencyPair, ClientOrderID: e.Text})
			continue
		}
		var info types.OrderInfo = orderInfoFromPayload(&e.spotOrderPayload, rateLimits)
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
			CurrencyPair:  reqs[i].CurrencyPair,
			Side:          reqs[i].Side,
			Price:         reqs[i].Price,
			Amount:        reqs[i].Amount,
			ClientOrderID: reqs[i].Text,
		})
	}
	return out
}

// ---- ModifyOrder -----------------------------------------------------------

/*
ModifyOrder amends an order's price and/or amount via PATCH /spot/orders/{id}.
One of OrderID / ClientOrderID identifies the order; CurrencyPair is required
(sent as a query parameter).
*/
func (t *TradingClient) ModifyOrder(ctx context.Context, req types.ModifyOrderRequest) (types.OrderInfo, error) {
	var info types.OrderInfo
	if req.CurrencyPair == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.ModifyOrder: CurrencyPair is empty", nil)
	}
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

	var q = newQuery()
	q.Set("currency_pair", req.CurrencyPair)

	var resp rest.Response
	var rateLimits map[string]string
	resp, rateLimits, err = t.c.rest().Do(ctx, rest.Options{
		Method: "PATCH",
		Path:   t.orderPath(id),
		Query:  q,
		Body:   body,
		Signed: true,
		Meta: rest.RequestMeta{
			OrderCount: 1,
			Symbols:    []string{req.CurrencyPair},
			Category:   string(gate.RateLimitCategoryAmend),
		},
	})
	if err != nil {
		return info, err
	}

	var p spotOrderPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "trading.ModifyOrder: parse", err)
	}
	info = orderInfoFromPayload(&p, rateLimits)
	return info, nil
}

// buildAmendBody constructs the Gate amend body {amount?, price?, amend_text?}.
func buildAmendBody(req types.ModifyOrderRequest) (map[string]any, error) {
	if (req.OrderID == "") == (req.ClientOrderID == "") {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.ModifyOrder: exactly one of OrderID/ClientOrderID must be set", nil)
	}
	if req.NewAmount.IsZero() && req.NewPrice.IsZero() {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.ModifyOrder: NewAmount or NewPrice must be set", nil)
	}
	var body map[string]any = make(map[string]any, 3)
	if !req.NewAmount.IsZero() {
		body["amount"] = req.NewAmount.String()
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
ModifyBatchOrders amends several orders via Gate's native
POST /spot/amend_batch_orders. Inputs larger than Gate's per-request cap (5) are
split into sequential chunks. The response is an array aligned with the request
body; rejected elements (succeeded=false) carry their Gate label and contribute
to the aggregated error, while accepted ones return a populated OrderInfo.
Returns one OrderInfo per request, in order.

A native batch amend is one HTTP request per chunk instead of one per order,
which is materially cheaper against Gate's per-UID order rate limit (spot's order
bucket is small) than the old sequential-amend emulation.
*/
func (t *TradingClient) ModifyBatchOrders(ctx context.Context, reqs []types.ModifyOrderRequest) ([]types.OrderInfo, error) {
	if len(reqs) == 0 {
		return nil, nil
	}
	var out []types.OrderInfo = make([]types.OrderInfo, 0, len(reqs))
	var aggErrs []error
	var start int
	for start = 0; start < len(reqs); start += maxBatchAmend {
		var end int = start + maxBatchAmend
		if end > len(reqs) {
			end = len(reqs)
		}
		var infos []types.OrderInfo
		var err error
		infos, err = t.modifyBatchChunk(ctx, reqs[start:end])
		out = append(out, infos...)
		if err != nil {
			aggErrs = append(aggErrs, err)
		}
	}
	if len(aggErrs) > 0 {
		return out, errors.Join(aggErrs...)
	}
	return out, nil
}

// modifyBatchChunk sends one amend_batch_orders request for up to maxBatchAmend
// orders and maps the per-element results.
func (t *TradingClient) modifyBatchChunk(ctx context.Context, reqs []types.ModifyOrderRequest) ([]types.OrderInfo, error) {
	var bodies []map[string]any = make([]map[string]any, 0, len(reqs))
	var bodyErrs []error
	var err error
	var i int
	for i = 0; i < len(reqs); i++ {
		var b map[string]any
		b, err = buildBatchAmendItem(reqs[i])
		if err != nil {
			bodyErrs = append(bodyErrs, fmt.Errorf("amend[%d]: %w", i, err))
			continue
		}
		bodies = append(bodies, b)
	}
	if len(bodies) == 0 {
		return placeholderAmendInfos(reqs), errors.Join(bodyErrs...)
	}

	var resp rest.Response
	var rateLimits map[string]string
	resp, rateLimits, err = t.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   t.batchAmendPath(),
		Body:   bodies,
		Signed: true,
		Meta: rest.RequestMeta{
			OrderCount: len(bodies),
			Symbols:    uniqueContractsModify(reqs),
			Category:   string(gate.RateLimitCategoryAmend),
		},
	})
	if err != nil {
		return placeholderAmendInfos(reqs), err
	}

	var entries []batchSpotOrderPayload
	if err = resp.UnmarshalData(&entries); err != nil {
		return placeholderAmendInfos(reqs), gate.NewError(gate.ErrorKindUnknown, "", "trading.ModifyBatchOrders: parse", err)
	}

	var infos []types.OrderInfo = make([]types.OrderInfo, 0, len(entries))
	var aggErrs []error = bodyErrs
	for i = 0; i < len(entries); i++ {
		var e batchSpotOrderPayload = entries[i]
		if e.Succeeded != nil && !*e.Succeeded {
			var msg string = e.Message
			if msg == "" {
				msg = e.Detail
			}
			aggErrs = append(aggErrs, &gate.Error{
				Kind:    gate.MapLabel(e.Label, 400),
				Label:   e.Label,
				Message: msg,
			})
			infos = append(infos, types.OrderInfo{CurrencyPair: e.CurrencyPair, ClientOrderID: e.Text})
			continue
		}
		infos = append(infos, orderInfoFromPayload(&e.spotOrderPayload, rateLimits))
	}
	if len(aggErrs) > 0 {
		return infos, errors.Join(aggErrs...)
	}
	return infos, nil
}

// buildBatchAmendItem assembles one element of the amend_batch_orders body:
// {order_id, currency_pair, amount?, price?, amend_text?}. Spot keeps side fixed;
// amount is in base currency (no signed convention, unlike futures).
func buildBatchAmendItem(req types.ModifyOrderRequest) (map[string]any, error) {
	if req.CurrencyPair == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.ModifyBatchOrders: CurrencyPair is empty", nil)
	}
	if (req.OrderID == "") == (req.ClientOrderID == "") {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.ModifyBatchOrders: exactly one of OrderID/ClientOrderID must be set", nil)
	}
	if req.NewAmount.IsZero() && req.NewPrice.IsZero() {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.ModifyBatchOrders: NewAmount or NewPrice must be set", nil)
	}

	var id string = orderIDPath(req.OrderID, req.ClientOrderID)
	if id == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.ModifyBatchOrders: OrderID or ClientOrderID is required", nil)
	}

	var item map[string]any = make(map[string]any, 5)
	item["order_id"] = id
	item["currency_pair"] = req.CurrencyPair
	if !req.NewAmount.IsZero() {
		item["amount"] = req.NewAmount.String()
	}
	if !req.NewPrice.IsZero() {
		item["price"] = req.NewPrice.String()
	}
	if req.AmendText != "" {
		item["amend_text"] = req.AmendText
	}
	return item, nil
}

// placeholderAmendInfos echoes request fields for the case where nothing was
// sent (so the caller keeps positional correspondence).
func placeholderAmendInfos(reqs []types.ModifyOrderRequest) []types.OrderInfo {
	var out []types.OrderInfo = make([]types.OrderInfo, 0, len(reqs))
	var i int
	for i = 0; i < len(reqs); i++ {
		out = append(out, types.OrderInfo{
			CurrencyPair:  reqs[i].CurrencyPair,
			Price:         reqs[i].NewPrice,
			Amount:        reqs[i].NewAmount,
			OrderID:       reqs[i].OrderID,
			ClientOrderID: reqs[i].ClientOrderID,
		})
	}
	return out
}

// uniqueContractsModify returns the sorted unique currency-pair set of an amend
// batch, for the rate-limit observer's per-pair accounting.
func uniqueContractsModify(reqs []types.ModifyOrderRequest) []string {
	var set map[string]struct{} = make(map[string]struct{}, len(reqs))
	var i int
	for i = 0; i < len(reqs); i++ {
		if reqs[i].CurrencyPair != "" {
			set[reqs[i].CurrencyPair] = struct{}{}
		}
	}
	return sortedKeys(set)
}

// ---- CancelOrder / CancelAllOrders / CancelBatchOrders ---------------------

// CancelOrder cancels a single order by OrderID or ClientOrderID. CurrencyPair
// is required (Gate sends it as a query parameter).
func (t *TradingClient) CancelOrder(ctx context.Context, req types.CancelOrderRequest) error {
	if req.CurrencyPair == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CancelOrder: CurrencyPair is empty", nil)
	}
	var id string = orderIDPath(req.OrderID, req.ClientOrderID)
	if id == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CancelOrder: OrderID or ClientOrderID is required", nil)
	}
	var q = newQuery()
	q.Set("currency_pair", req.CurrencyPair)
	var err error
	_, _, err = t.c.rest().Do(ctx, rest.Options{
		Method: "DELETE",
		Path:   t.orderPath(id),
		Query:  q,
		Signed: true,
		Meta: rest.RequestMeta{
			OrderCount: 1,
			Symbols:    []string{req.CurrencyPair},
			Category:   string(gate.RateLimitCategoryCancel),
		},
	})
	if err != nil {
		return err
	}
	t.forgetMapping(req.ClientOrderID, req.OrderID)
	return nil
}

// CancelAllOrders cancels all open orders for a currency pair via Gate's native
// DELETE /spot/orders?currency_pair=.
func (t *TradingClient) CancelAllOrders(ctx context.Context, currencyPair string) error {
	if currencyPair == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CancelAllOrders: currencyPair is empty", nil)
	}
	var q = newQuery()
	q.Set("currency_pair", currencyPair)
	var err error
	_, _, err = t.c.rest().Do(ctx, rest.Options{
		Method: "DELETE",
		Path:   t.ordersPath(),
		Query:  q,
		Signed: true,
		Meta: rest.RequestMeta{
			Symbols:  []string{currencyPair},
			Category: string(gate.RateLimitCategoryCancel),
		},
	})
	return err
}

// CancelBatchOrders cancels a batch of orders via Gate's native
// POST /spot/cancel_batch_orders (body: array of {currency_pair, id}). Returns an
// aggregated error for any element Gate reports as not succeeded.
func (t *TradingClient) CancelBatchOrders(ctx context.Context, reqs []types.CancelOrderRequest) error {
	if len(reqs) == 0 {
		return nil
	}
	var items []map[string]any = make([]map[string]any, 0, len(reqs))
	var i int
	for i = 0; i < len(reqs); i++ {
		if reqs[i].CurrencyPair == "" {
			return gate.NewError(gate.ErrorKindInvalidRequest, "", fmt.Sprintf("trading.CancelBatchOrders: req[%d] has no CurrencyPair", i), nil)
		}
		var id string = orderIDPath(reqs[i].OrderID, reqs[i].ClientOrderID)
		if id == "" {
			return gate.NewError(gate.ErrorKindInvalidRequest, "", fmt.Sprintf("trading.CancelBatchOrders: req[%d] has no OrderID/ClientOrderID", i), nil)
		}
		items = append(items, map[string]any{"currency_pair": reqs[i].CurrencyPair, "id": id})
	}

	var resp rest.Response
	var err error
	resp, _, err = t.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   t.batchCancelPath(),
		Body:   items,
		Signed: true,
		Meta: rest.RequestMeta{
			OrderCount: len(items),
			Symbols:    uniqueContractsCancel(reqs),
			Category:   string(gate.RateLimitCategoryCancel),
		},
	})
	if err != nil {
		return err
	}

	var results []batchSpotOrderPayload
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
		t.forgetMapping(results[i].Text, results[i].ID)
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
func (t *TradingClient) CancelForgottenOrders(ctx context.Context, currencyPair string, maxAge time.Duration) ([]types.OrderInfo, error) {
	if currencyPair == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CancelForgottenOrders: currencyPair is empty", nil)
	}
	if maxAge <= 0 {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CancelForgottenOrders: maxAge must be positive", nil)
	}

	var open []types.OrderInfo
	var err error
	open, err = t.GetOpenOrders(ctx, currencyPair)
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
			reqs = append(reqs, types.CancelOrderRequest{CurrencyPair: currencyPair, OrderID: open[i].OrderID})
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
scoped to currencyPair). timeout=0 disarms. Returns the trigger time in epoch ms.
*/
func (t *TradingClient) CountdownCancelAll(ctx context.Context, timeout time.Duration, currencyPair string) (int64, error) {
	if timeout < 0 {
		return 0, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.CountdownCancelAll: timeout must be >= 0", nil)
	}
	var body map[string]any = map[string]any{"timeout": int64(timeout / time.Second)}
	if currencyPair != "" {
		body["currency_pair"] = currencyPair
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
		TriggerTime float64 `json:"triggerTime"`
	}
	if err = resp.UnmarshalData(&out); err != nil {
		return 0, gate.NewError(gate.ErrorKindUnknown, "", "trading.CountdownCancelAll: parse", err)
	}
	return int64(out.TriggerTime), nil
}

// ---- GetOrder / GetOpenOrders ----------------------------------------------

// GetOrder fetches a single order by its id or client text for a currency pair.
func (t *TradingClient) GetOrder(ctx context.Context, currencyPair, idOrText string) (types.OrderInfo, error) {
	var info types.OrderInfo
	if currencyPair == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.GetOrder: currencyPair is empty", nil)
	}
	if idOrText == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.GetOrder: id is empty", nil)
	}
	var q = newQuery()
	q.Set("currency_pair", currencyPair)
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = t.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   t.orderPath(idOrText),
		Query:  q,
		Signed: true,
		Meta: rest.RequestMeta{
			Symbols:  []string{currencyPair},
			Category: string(gate.RateLimitCategoryQuery),
		},
	})
	if err != nil {
		return info, err
	}
	var p spotOrderPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "trading.GetOrder: parse", err)
	}
	return orderInfoFromPayload(&p, rateLimits), nil
}

// GetOpenOrders lists open orders for a currency pair
// (GET /spot/orders?currency_pair=&status=open).
func (t *TradingClient) GetOpenOrders(ctx context.Context, currencyPair string) ([]types.OrderInfo, error) {
	if currencyPair == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "trading.GetOpenOrders: currencyPair is empty", nil)
	}
	var q = newQuery()
	q.Set("currency_pair", currencyPair)
	q.Set("status", types.OrderStatusOpen)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = t.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   t.ordersPath(),
		Query:  q,
		Signed: true,
		Meta: rest.RequestMeta{
			Symbols:  []string{currencyPair},
			Category: string(gate.RateLimitCategoryQuery),
		},
	})
	if err != nil {
		return nil, err
	}

	var payloads []spotOrderPayload
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
func (t *TradingClient) SyncOrderMappings(ctx context.Context, currencyPair string) error {
	var open []types.OrderInfo
	var err error
	open, err = t.GetOpenOrders(ctx, currencyPair)
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

// uniqueContractsCreate returns the sorted unique currency-pair set of a create
// batch, for the rate-limit observer's per-pair accounting.
func uniqueContractsCreate(reqs []types.CreateOrderRequest) []string {
	var set map[string]struct{} = make(map[string]struct{}, len(reqs))
	var i int
	for i = 0; i < len(reqs); i++ {
		if reqs[i].CurrencyPair != "" {
			set[reqs[i].CurrencyPair] = struct{}{}
		}
	}
	return sortedKeys(set)
}

// uniqueContractsCancel — same for a cancel batch.
func uniqueContractsCancel(reqs []types.CancelOrderRequest) []string {
	var set map[string]struct{} = make(map[string]struct{}, len(reqs))
	var i int
	for i = 0; i < len(reqs); i++ {
		if reqs[i].CurrencyPair != "" {
			set[reqs[i].CurrencyPair] = struct{}{}
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
