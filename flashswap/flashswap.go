/*
FILE: flashswap/flashswap.go

DESCRIPTION:
Flash-swap REST methods for the Gate Flash Swap section (instant currency
conversion). Implements:

  PUBLIC (unsigned):
    - ListCurrencies : GET  /flash_swap/currencies

  PRIVATE (signed):
    - ListOrders     : GET  /flash_swap/orders?status=&sell_currency=&buy_currency=&page=&limit=
    - PreviewOrder   : POST /flash_swap/orders/preview
    - CreateOrder    : POST /flash_swap/orders
    - GetOrder       : GET  /flash_swap/orders/{order_id}

Amount/price fields decode through codec.FlexDecimal (Gate quotes them as strings
over REST, but a bare-number form must not break the decode). Epoch seconds are
normalized to milliseconds.
*/

package flashswap

import (
	"context"
	"strconv"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/flashswap/types"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
)

// ---- query-filter option structs -------------------------------------------

// ListOrdersParams — optional filters for ListOrders.
type ListOrdersParams struct {
	// Status — restrict to one order status (FlashSwapOrderStatusInit / 0 = all).
	Status types.FlashSwapOrderStatus
	// SellCurrency — restrict to one sell currency, e.g. "BTC".
	SellCurrency string
	// BuyCurrency — restrict to one buy currency, e.g. "USDT".
	BuyCurrency string
	// Page / Limit — pagination (≤ 0 = Gate default).
	Page  int
	Limit int
}

// ---- paths -----------------------------------------------------------------

func (c *Client) currenciesPath() string   { return c.basePath() + "/currencies" }
func (c *Client) ordersPath() string       { return c.basePath() + "/orders" }
func (c *Client) orderPreviewPath() string { return c.basePath() + "/orders/preview" }
func (c *Client) orderPath(id string) string {
	return c.basePath() + "/orders/" + id
}

// ---- currencies (public) ---------------------------------------------------

type buyCurrencyPayload struct {
	Currency  string            `json:"currency"`
	MinAmount codec.FlexDecimal `json:"min_amount"`
	MaxAmount codec.FlexDecimal `json:"max_amount"`
}

type currencyPayload struct {
	Currency      string               `json:"currency"`
	MinAmount     codec.FlexDecimal    `json:"min_amount"`
	MaxAmount     codec.FlexDecimal    `json:"max_amount"`
	BuyCurrencies []buyCurrencyPayload `json:"buy_currencies"`
}

func currencyFromPayload(p *currencyPayload, rateLimits map[string]string) types.FlashSwapCurrency {
	var buys []types.FlashSwapBuyCurrency = make([]types.FlashSwapBuyCurrency, 0, len(p.BuyCurrencies))
	var k int
	for k = 0; k < len(p.BuyCurrencies); k++ {
		buys = append(buys, types.FlashSwapBuyCurrency{
			Currency:  p.BuyCurrencies[k].Currency,
			MinAmount: p.BuyCurrencies[k].MinAmount.Decimal,
			MaxAmount: p.BuyCurrencies[k].MaxAmount.Decimal,
		})
	}
	return types.FlashSwapCurrency{
		Currency:      p.Currency,
		MinAmount:     p.MinAmount.Decimal,
		MaxAmount:     p.MaxAmount.Decimal,
		BuyCurrencies: buys,
		RateLimits:    rateLimits,
	}
}

// ListCurrencies returns the swappable currencies, each with its eligible buy
// currencies and amount limits (public).
func (c *Client) ListCurrencies(ctx context.Context) ([]types.FlashSwapCurrency, error) {
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.currenciesPath(),
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []currencyPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "flashswap.ListCurrencies: parse", err)
	}
	var out []types.FlashSwapCurrency = make([]types.FlashSwapCurrency, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, currencyFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// ---- orders ----------------------------------------------------------------

type orderPayload struct {
	ID           int64             `json:"id"`
	CreateTime   int64             `json:"create_time"`
	UpdateTime   int64             `json:"update_time"`
	UserID       int64             `json:"user_id"`
	SellCurrency string            `json:"sell_currency"`
	SellAmount   codec.FlexDecimal `json:"sell_amount"`
	BuyCurrency  string            `json:"buy_currency"`
	BuyAmount    codec.FlexDecimal `json:"buy_amount"`
	Price        codec.FlexDecimal `json:"price"`
	Status       int64             `json:"status"`
}

func orderFromPayload(p *orderPayload, rateLimits map[string]string) types.FlashSwapOrder {
	return types.FlashSwapOrder{
		ID:           idString(p.ID),
		CreatedAtMs:  secondsToMs(p.CreateTime),
		UpdatedAtMs:  secondsToMs(p.UpdateTime),
		UserID:       p.UserID,
		SellCurrency: p.SellCurrency,
		SellAmount:   p.SellAmount.Decimal,
		BuyCurrency:  p.BuyCurrency,
		BuyAmount:    p.BuyAmount.Decimal,
		Price:        p.Price.Decimal,
		Status:       types.FlashSwapOrderStatus(p.Status),
		RateLimits:   rateLimits,
	}
}

// ListOrders returns the user's flash-swap orders matching the filters.
func (c *Client) ListOrders(ctx context.Context, params ListOrdersParams) ([]types.FlashSwapOrder, error) {
	var q = newQuery()
	if params.Status != types.FlashSwapOrderStatusInit {
		q.Set("status", strconv.FormatInt(int64(params.Status), 10))
	}
	if params.SellCurrency != "" {
		q.Set("sell_currency", params.SellCurrency)
	}
	if params.BuyCurrency != "" {
		q.Set("buy_currency", params.BuyCurrency)
	}
	if params.Page > 0 {
		q.Set("page", strconv.Itoa(params.Page))
	}
	if params.Limit > 0 {
		q.Set("limit", strconv.Itoa(params.Limit))
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.ordersPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []orderPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "flashswap.ListOrders: parse", err)
	}
	var out []types.FlashSwapOrder = make([]types.FlashSwapOrder, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, orderFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// ---- preview ---------------------------------------------------------------

type previewPayload struct {
	PreviewID    string            `json:"preview_id"`
	SellCurrency string            `json:"sell_currency"`
	SellAmount   codec.FlexDecimal `json:"sell_amount"`
	BuyCurrency  string            `json:"buy_currency"`
	BuyAmount    codec.FlexDecimal `json:"buy_amount"`
	Price        codec.FlexDecimal `json:"price"`
}

func previewFromPayload(p *previewPayload, rateLimits map[string]string) types.FlashSwapPreview {
	return types.FlashSwapPreview{
		PreviewID:    p.PreviewID,
		SellCurrency: p.SellCurrency,
		SellAmount:   p.SellAmount.Decimal,
		BuyCurrency:  p.BuyCurrency,
		BuyAmount:    p.BuyAmount.Decimal,
		Price:        p.Price.Decimal,
		RateLimits:   rateLimits,
	}
}

// PreviewOrder quotes a flash-swap conversion. Exactly one of SellAmount /
// BuyAmount must be supplied; Gate computes the other side and the price and
// returns a PreviewID for CreateOrder.
func (c *Client) PreviewOrder(ctx context.Context, req types.PreviewRequest) (types.FlashSwapPreview, error) {
	var info types.FlashSwapPreview
	if req.SellCurrency == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "flashswap.PreviewOrder: SellCurrency is empty", nil)
	}
	if req.BuyCurrency == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "flashswap.PreviewOrder: BuyCurrency is empty", nil)
	}
	var hasSell bool = !req.SellAmount.IsZero()
	var hasBuy bool = !req.BuyAmount.IsZero()
	if hasSell == hasBuy {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "flashswap.PreviewOrder: supply exactly one of SellAmount or BuyAmount", nil)
	}

	var body map[string]any = make(map[string]any, 4)
	body["sell_currency"] = req.SellCurrency
	body["buy_currency"] = req.BuyCurrency
	if hasSell {
		body["sell_amount"] = req.SellAmount.String()
	}
	if hasBuy {
		body["buy_amount"] = req.BuyAmount.String()
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   c.orderPreviewPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p previewPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "flashswap.PreviewOrder: parse", err)
	}
	return previewFromPayload(&p, rateLimits), nil
}

// ---- create / get ----------------------------------------------------------

// CreateOrder executes a flash-swap against a preview obtained from
// PreviewOrder. PreviewID is required.
func (c *Client) CreateOrder(ctx context.Context, req types.CreateOrderRequest) (types.FlashSwapOrder, error) {
	var info types.FlashSwapOrder
	if req.PreviewID == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "flashswap.CreateOrder: PreviewID is empty", nil)
	}
	if req.SellCurrency == "" || req.BuyCurrency == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "flashswap.CreateOrder: SellCurrency and BuyCurrency are required", nil)
	}
	if req.SellAmount.IsZero() || req.SellAmount.IsNegative() {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "flashswap.CreateOrder: SellAmount must be positive", nil)
	}
	if req.BuyAmount.IsZero() || req.BuyAmount.IsNegative() {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "flashswap.CreateOrder: BuyAmount must be positive", nil)
	}

	var body map[string]any = make(map[string]any, 5)
	body["preview_id"] = req.PreviewID
	body["sell_currency"] = req.SellCurrency
	body["sell_amount"] = req.SellAmount.String()
	body["buy_currency"] = req.BuyCurrency
	body["buy_amount"] = req.BuyAmount.String()

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   c.ordersPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p orderPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "flashswap.CreateOrder: parse", err)
	}
	return orderFromPayload(&p, rateLimits), nil
}

// GetOrder returns a single flash-swap order by id.
func (c *Client) GetOrder(ctx context.Context, orderID string) (types.FlashSwapOrder, error) {
	var info types.FlashSwapOrder
	if orderID == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "flashswap.GetOrder: orderID is empty", nil)
	}
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.orderPath(orderID),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p orderPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "flashswap.GetOrder: parse", err)
	}
	return orderFromPayload(&p, rateLimits), nil
}
