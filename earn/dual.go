/*
FILE: earn/dual.go

DESCRIPTION:
DualClient — the Gate Earn Dual Investment sub-client, reachable via
earn.Client.Dual(). Like Fixed-Term (and unlike the Uni flexible-lending methods
on earn.Client, base "/earn/uni"), it builds its paths under a DIFFERENT base,
"/earn/dual", so it does NOT reuse earn.Client.basePath().

ENDPOINTS:
  - ListInvestmentPlans  GET  /earn/dual/investment_plan?plan_id=   (public)
  - ProjectRecommend     GET  /earn/dual/project-recommend          (public)
  - ListOrders           GET  /earn/dual/orders?from=&to=&page=&limit=  (signed)
  - CreateOrder          POST /earn/dual/orders                     (signed)
  - GetBalance           GET  /earn/dual/balance?currency=          (signed)
  - RefundPreview        GET  /earn/dual/order-refund-preview?order_id=  (signed)
  - Refund               POST /earn/dual/order-refund               (signed)
  - ModifyOrderReinvest  POST /earn/dual/modify-order-reinvest      (signed)

Amounts/APRs/prices decode through codec.FlexDecimal (Gate may quote them as JSON
numbers or strings); epoch-second timestamps are normalized to milliseconds.
*/

package earn

import (
	"context"
	"strconv"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/earn/types"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
)

// DualClient — Gate Earn Dual Investment sub-client.
type DualClient struct {
	c *Client
}

func newDualClient(c *Client) *DualClient {
	return &DualClient{c: c}
}

// dualBasePath returns "/earn/dual". It is deliberately separate from
// earn.Client.basePath() ("/earn/uni").
func (d *DualClient) dualBasePath() string { return "/earn/dual" }

func (d *DualClient) investmentPlanPath() string {
	return d.dualBasePath() + "/investment_plan"
}
func (d *DualClient) projectRecommendPath() string { return d.dualBasePath() + "/project-recommend" }
func (d *DualClient) ordersPath() string           { return d.dualBasePath() + "/orders" }
func (d *DualClient) balancePath() string          { return d.dualBasePath() + "/balance" }
func (d *DualClient) refundPreviewPath() string {
	return d.dualBasePath() + "/order-refund-preview"
}
func (d *DualClient) refundPath() string { return d.dualBasePath() + "/order-refund" }
func (d *DualClient) modifyReinvestPath() string {
	return d.dualBasePath() + "/modify-order-reinvest"
}

// ---- investment plans (public) ---------------------------------------------

// dualPlanPayload — Gate Dual Investment plan wire shape.
type dualPlanPayload struct {
	PlanID           string            `json:"plan_id"`
	ID               int64             `json:"id"`
	Instrument       string            `json:"instrument"`
	InvestCurrency   string            `json:"invest_currency"`
	ExerciseCurrency string            `json:"exercise_currency"`
	DeliveryTime     int64             `json:"delivery_time"`
	APR              codec.FlexDecimal `json:"apr"`
	MinAPR           codec.FlexDecimal `json:"min_apr"`
	StrikePrice      codec.FlexDecimal `json:"strike_price"`
	Copies           codec.FlexDecimal `json:"copies"`
	PerValue         codec.FlexDecimal `json:"per_value"`
	Status           string            `json:"status"`
}

func dualPlanFromPayload(p *dualPlanPayload, rateLimits map[string]string) types.DualInvestmentPlan {
	var id string = p.PlanID
	if id == "" {
		id = idString(p.ID)
	}
	return types.DualInvestmentPlan{
		ID:               id,
		Instrument:       p.Instrument,
		InvestCurrency:   p.InvestCurrency,
		ExerciseCurrency: p.ExerciseCurrency,
		DeliveryTimeMs:   secondsToMs(p.DeliveryTime),
		APR:              p.APR.Decimal,
		MinAPR:           p.MinAPR.Decimal,
		StrikePrice:      p.StrikePrice.Decimal,
		Copies:           p.Copies.Decimal,
		PerValue:         p.PerValue.Decimal,
		Status:           p.Status,
		RateLimits:       rateLimits,
	}
}

// ListInvestmentPlans returns the dual-investment plans. Pass an empty planID
// for every plan, or a specific plan id to fetch one. Public: no signature.
func (d *DualClient) ListInvestmentPlans(ctx context.Context, planID string) ([]types.DualInvestmentPlan, error) {
	var q = newQuery()
	if planID != "" {
		q.Set("plan_id", planID)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = d.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   d.investmentPlanPath(),
		Query:  q,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []dualPlanPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "earn.Dual.ListInvestmentPlans: parse", err)
	}
	var out []types.DualInvestmentPlan = make([]types.DualInvestmentPlan, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, dualPlanFromPayload(&payloads[i], rateLimits))
	}
	return out, nil
}

// ProjectRecommend returns Gate's recommended dual-investment plans. Public: no
// signature required.
func (d *DualClient) ProjectRecommend(ctx context.Context) ([]types.DualInvestmentPlan, error) {
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = d.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   d.projectRecommendPath(),
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []dualPlanPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "earn.Dual.ProjectRecommend: parse", err)
	}
	var out []types.DualInvestmentPlan = make([]types.DualInvestmentPlan, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, dualPlanFromPayload(&payloads[i], rateLimits))
	}
	return out, nil
}

// ---- orders (signed) -------------------------------------------------------

// dualOrderPayload — Gate Dual Investment order wire shape.
type dualOrderPayload struct {
	ID                 int64             `json:"id"`
	PlanID             string            `json:"plan_id"`
	Copies             codec.FlexDecimal `json:"copies"`
	InvestCurrency     string            `json:"invest_currency"`
	InvestAmount       codec.FlexDecimal `json:"invest_amount"`
	SettlementCurrency string            `json:"settlement_currency"`
	SettlementAmount   codec.FlexDecimal `json:"settlement_amount"`
	APR                codec.FlexDecimal `json:"apr"`
	Status             string            `json:"status"`
	CreateTime         int64             `json:"create_time"`
}

func dualOrderFromPayload(p *dualOrderPayload, rateLimits map[string]string) types.DualOrder {
	return types.DualOrder{
		ID:                 idString(p.ID),
		PlanID:             p.PlanID,
		Copies:             p.Copies.Decimal,
		InvestCurrency:     p.InvestCurrency,
		InvestAmount:       p.InvestAmount.Decimal,
		SettlementCurrency: p.SettlementCurrency,
		SettlementAmount:   p.SettlementAmount.Decimal,
		APR:                p.APR.Decimal,
		Status:             p.Status,
		CreatedAtMs:        secondsToMs(p.CreateTime),
		RateLimits:         rateLimits,
	}
}

// ListOrders returns the caller's dual-investment orders. All filters are
// optional: from==to==0 / page==limit≤0 let Gate apply its defaults. from/to are
// epoch SECONDS (Gate's native unit).
func (d *DualClient) ListOrders(ctx context.Context, from, to int64, page, limit int) ([]types.DualOrder, error) {
	var q = newQuery()
	if from > 0 {
		q.Set("from", strconv.FormatInt(from, 10))
	}
	if to > 0 {
		q.Set("to", strconv.FormatInt(to, 10))
	}
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = d.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   d.ordersPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []dualOrderPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "earn.Dual.ListOrders: parse", err)
	}
	var out []types.DualOrder = make([]types.DualOrder, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, dualOrderFromPayload(&payloads[i], rateLimits))
	}
	return out, nil
}

// CreateOrder subscribes a dual-investment order. req must have a PlanID and at
// least one positive sizing field (Copies and/or Amount). Returns the created
// order. SDK validation errors are returned WITHOUT sending a request.
func (d *DualClient) CreateOrder(ctx context.Context, req types.CreateDualOrderRequest) (types.DualOrder, error) {
	var info types.DualOrder
	if req.PlanID == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.Dual.CreateOrder: PlanID is empty", nil)
	}
	if req.Copies <= 0 && (req.Amount.IsZero() || req.Amount.IsNegative()) {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.Dual.CreateOrder: Copies or Amount must be positive", nil)
	}

	var body map[string]any = make(map[string]any, 3)
	body["plan_id"] = req.PlanID
	if req.Copies > 0 {
		body["copies"] = req.Copies
	}
	if !req.Amount.IsZero() && !req.Amount.IsNegative() {
		body["amount"] = req.Amount.String()
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = d.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   d.ordersPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryPlace)},
	})
	if err != nil {
		return info, err
	}
	var p dualOrderPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "earn.Dual.CreateOrder: parse", err)
	}
	return dualOrderFromPayload(&p, rateLimits), nil
}

// ---- balance (signed) ------------------------------------------------------

// dualBalancePayload — Gate Dual Investment balance wire shape.
type dualBalancePayload struct {
	Currency string            `json:"currency"`
	Amount   codec.FlexDecimal `json:"amount"`
	Locked   codec.FlexDecimal `json:"locked"`
}

// GetBalance returns the caller's dual-investment balance for a currency.
func (d *DualClient) GetBalance(ctx context.Context, currency string) (types.DualBalance, error) {
	var info types.DualBalance
	if currency == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.Dual.GetBalance: currency is empty", nil)
	}
	var q = newQuery()
	q.Set("currency", currency)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = d.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   d.balancePath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p dualBalancePayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "earn.Dual.GetBalance: parse", err)
	}
	info.Currency = p.Currency
	if info.Currency == "" {
		info.Currency = currency
	}
	info.Amount = p.Amount.Decimal
	info.Locked = p.Locked.Decimal
	info.RateLimits = rateLimits
	return info, nil
}

// ---- refunds (signed) ------------------------------------------------------

// dualRefundPreviewPayload — Gate Dual Investment refund-preview wire shape.
type dualRefundPreviewPayload struct {
	OrderID      int64             `json:"order_id"`
	Currency     string            `json:"currency"`
	RefundAmount codec.FlexDecimal `json:"refund_amount"`
	Fee          codec.FlexDecimal `json:"fee"`
}

// RefundPreview returns the projected refund for an order without refunding it.
// orderID is required.
func (d *DualClient) RefundPreview(ctx context.Context, orderID string) (types.DualRefundPreview, error) {
	var info types.DualRefundPreview
	if orderID == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.Dual.RefundPreview: orderID is empty", nil)
	}
	var q = newQuery()
	q.Set("order_id", orderID)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = d.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   d.refundPreviewPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p dualRefundPreviewPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "earn.Dual.RefundPreview: parse", err)
	}
	var id string = idString(p.OrderID)
	if id == "" {
		id = orderID
	}
	return types.DualRefundPreview{
		OrderID:      id,
		Currency:     p.Currency,
		RefundAmount: p.RefundAmount.Decimal,
		Fee:          p.Fee.Decimal,
		RateLimits:   rateLimits,
	}, nil
}

// Refund refunds (cancels) a dual-investment order. orderID is required. SDK
// validation errors are returned WITHOUT sending a request.
func (d *DualClient) Refund(ctx context.Context, orderID string) error {
	if orderID == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.Dual.Refund: orderID is empty", nil)
	}
	var body map[string]any = map[string]any{"order_id": orderID}
	_, _, err := d.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   d.refundPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryPlace)},
	})
	return err
}

// ModifyOrderReinvest enables/disables auto-reinvest on a dual-investment order.
// req must have an OrderID. SDK validation errors are returned WITHOUT sending a
// request.
func (d *DualClient) ModifyOrderReinvest(ctx context.Context, req types.ModifyReinvestRequest) error {
	if req.OrderID == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.Dual.ModifyOrderReinvest: OrderID is empty", nil)
	}
	var body map[string]any = make(map[string]any, 2)
	body["order_id"] = req.OrderID
	body["reinvest"] = req.Reinvest
	_, _, err := d.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   d.modifyReinvestPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryAmend)},
	})
	return err
}
