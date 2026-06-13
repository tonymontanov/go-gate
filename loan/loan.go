/*
FILE: loan/loan.go

DESCRIPTION:
REST methods for the Gate multi-collateral Loan section. Implements:

  PUBLIC (unsigned):
    - ListCurrencies   : GET  /loan/multi_collateral/currencies?loan_type=
    - GetFixedRate     : GET  /loan/multi_collateral/fixed_rate?currency=
    - GetCurrentRate   : GET  /loan/multi_collateral/current_rate?currencies=

  PRIVATE (signed):
    - ListOrders        : GET  /loan/multi_collateral/orders?page=&limit=&sort=
    - CreateOrder       : POST /loan/multi_collateral/orders
    - GetOrder          : GET  /loan/multi_collateral/orders/{order_id}
    - ListRepayRecords  : GET  /loan/multi_collateral/repay?page=&limit=&...
    - Repay             : POST /loan/multi_collateral/repay
    - ListMortgageRecords : GET  /loan/multi_collateral/mortgage?page=&limit=&...
    - OperateMortgage   : POST /loan/multi_collateral/mortgage
    - GetCurrencyQuota  : GET  /loan/multi_collateral/currency_quota?type=&currency=
    - GetLtv            : GET  /loan/multi_collateral/ltv

Amount/rate/ltv fields decode through codec.FlexDecimal (Gate quotes them as
strings over REST, but a bare-number form must not break the decode). Epoch
seconds are normalized to milliseconds.
*/

package loan

import (
	"context"
	"strconv"
	"strings"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
	"github.com/tonymontanov/go-gate/v2/loan/types"
)

// ---- query-filter option structs -------------------------------------------

// ListOrdersParams — optional filters for ListOrders.
type ListOrdersParams struct {
	// Page / Limit — pagination (≤ 0 = Gate default).
	Page  int
	Limit int
	// Sort — Gate sort directive (empty = Gate default).
	Sort string
}

// ListRepayRecordsParams — optional filters for ListRepayRecords.
type ListRepayRecordsParams struct {
	// OrderID — restrict to one loan order.
	OrderID string
	// Currency — restrict to one repaid currency.
	Currency string
	// Page / Limit — pagination (≤ 0 = Gate default).
	Page  int
	Limit int
}

// ListMortgageRecordsParams — optional filters for ListMortgageRecords.
type ListMortgageRecordsParams struct {
	// OrderID — restrict to one loan order.
	OrderID string
	// Currency — restrict to one collateral currency.
	Currency string
	// Page / Limit — pagination (≤ 0 = Gate default).
	Page  int
	Limit int
}

// ---- paths -----------------------------------------------------------------

func (c *Client) ordersPath() string { return c.basePath() + "/orders" }
func (c *Client) orderPath(id string) string {
	return c.basePath() + "/orders/" + id
}
func (c *Client) repayPath() string         { return c.basePath() + "/repay" }
func (c *Client) mortgagePath() string      { return c.basePath() + "/mortgage" }
func (c *Client) currencyQuotaPath() string { return c.basePath() + "/currency_quota" }
func (c *Client) currenciesPath() string    { return c.basePath() + "/currencies" }
func (c *Client) ltvPath() string           { return c.basePath() + "/ltv" }
func (c *Client) fixedRatePath() string     { return c.basePath() + "/fixed_rate" }
func (c *Client) currentRatePath() string   { return c.basePath() + "/current_rate" }

// ---- shared collateral / borrow item payloads ------------------------------

type collateralItemPayload struct {
	Currency string            `json:"currency"`
	Amount   codec.FlexDecimal `json:"amount"`
}

func collateralItemsFromPayload(items []collateralItemPayload) []types.CollateralItem {
	var out []types.CollateralItem = make([]types.CollateralItem, 0, len(items))
	var k int
	for k = 0; k < len(items); k++ {
		out = append(out, types.CollateralItem{
			Currency: items[k].Currency,
			Amount:   items[k].Amount.Decimal,
		})
	}
	return out
}

func borrowItemsFromPayload(items []collateralItemPayload) []types.BorrowItem {
	var out []types.BorrowItem = make([]types.BorrowItem, 0, len(items))
	var k int
	for k = 0; k < len(items); k++ {
		out = append(out, types.BorrowItem{
			Currency: items[k].Currency,
			Amount:   items[k].Amount.Decimal,
		})
	}
	return out
}

// ---- orders ----------------------------------------------------------------

type orderPayload struct {
	OrderID              int64                   `json:"order_id"`
	OrderType            string                  `json:"order_type"`
	FixedType            string                  `json:"fixed_type"`
	FixedRate            codec.FlexDecimal       `json:"fixed_rate"`
	ExpireTime           int64                   `json:"expire_time"`
	AutoRenew            bool                    `json:"auto_renew"`
	AutoRepay            bool                    `json:"auto_repay"`
	Currencies           []collateralItemPayload `json:"currencies"`
	CollateralCurrencies []collateralItemPayload `json:"collateral_currencies"`
	BorrowCurrency       string                  `json:"borrow_currency"`
	BorrowAmount         codec.FlexDecimal       `json:"borrow_amount"`
	CurrentLtv           codec.FlexDecimal       `json:"current_ltv"`
	Status               string                  `json:"status"`
	CreateTime           int64                   `json:"create_time"`
	UpdateTime           int64                   `json:"update_time"`
}

func orderFromPayload(p *orderPayload, rateLimits map[string]string) types.MultiLoanOrder {
	return types.MultiLoanOrder{
		OrderID:              idString(p.OrderID),
		OrderType:            types.MultiLoanOrderType(p.OrderType),
		FixedType:            types.MultiLoanFixedType(p.FixedType),
		FixedRate:            p.FixedRate.Decimal,
		ExpireTimeMs:         secondsToMs(p.ExpireTime),
		AutoRenew:            p.AutoRenew,
		AutoRepay:            p.AutoRepay,
		Currencies:           borrowItemsFromPayload(p.Currencies),
		CollateralCurrencies: collateralItemsFromPayload(p.CollateralCurrencies),
		BorrowCurrency:       p.BorrowCurrency,
		BorrowAmount:         p.BorrowAmount.Decimal,
		CurrentLtv:           p.CurrentLtv.Decimal,
		Status:               types.MultiLoanOrderStatus(p.Status),
		CreatedAtMs:          secondsToMs(p.CreateTime),
		UpdatedAtMs:          secondsToMs(p.UpdateTime),
		RateLimits:           rateLimits,
	}
}

// ListOrders returns the user's multi-collateral loan orders.
func (c *Client) ListOrders(ctx context.Context, params ListOrdersParams) ([]types.MultiLoanOrder, error) {
	var q = newQuery()
	if params.Page > 0 {
		q.Set("page", strconv.Itoa(params.Page))
	}
	if params.Limit > 0 {
		q.Set("limit", strconv.Itoa(params.Limit))
	}
	if params.Sort != "" {
		q.Set("sort", params.Sort)
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
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "loan.ListOrders: parse", err)
	}
	var out []types.MultiLoanOrder = make([]types.MultiLoanOrder, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, orderFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// CreateOrder opens a multi-collateral loan: borrow BorrowCurrency against the
// CollateralCurrencies basket.
func (c *Client) CreateOrder(ctx context.Context, req types.CreateOrderRequest) (types.MultiLoanOrder, error) {
	var info types.MultiLoanOrder
	if req.OrderType != types.MultiLoanOrderTypeCurrent && req.OrderType != types.MultiLoanOrderTypeFixed {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "loan.CreateOrder: OrderType must be current or fixed", nil)
	}
	if req.BorrowCurrency == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "loan.CreateOrder: BorrowCurrency is empty", nil)
	}
	if req.BorrowAmount.IsZero() || req.BorrowAmount.IsNegative() {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "loan.CreateOrder: BorrowAmount must be positive", nil)
	}
	if len(req.CollateralCurrencies) == 0 {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "loan.CreateOrder: CollateralCurrencies is empty", nil)
	}

	var collaterals []map[string]any = make([]map[string]any, 0, len(req.CollateralCurrencies))
	var k int
	for k = 0; k < len(req.CollateralCurrencies); k++ {
		collaterals = append(collaterals, map[string]any{
			"currency": req.CollateralCurrencies[k].Currency,
			"amount":   req.CollateralCurrencies[k].Amount.String(),
		})
	}

	var body map[string]any = make(map[string]any, 8)
	body["order_type"] = string(req.OrderType)
	body["borrow_currency"] = req.BorrowCurrency
	body["borrow_amount"] = req.BorrowAmount.String()
	body["collateral_currencies"] = collaterals
	if req.OrderID != "" {
		body["order_id"] = req.OrderID
	}
	if req.FixedType != "" {
		body["fixed_type"] = string(req.FixedType)
	}
	if !req.FixedRate.IsZero() {
		body["fixed_rate"] = req.FixedRate.String()
	}
	if req.AutoRenew {
		body["auto_renew"] = true
	}
	if req.AutoRepay {
		body["auto_repay"] = true
	}

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
		return info, gate.NewError(gate.ErrorKindUnknown, "", "loan.CreateOrder: parse", err)
	}
	return orderFromPayload(&p, rateLimits), nil
}

// GetOrder returns a single multi-collateral loan order by id.
func (c *Client) GetOrder(ctx context.Context, orderID string) (types.MultiLoanOrder, error) {
	var info types.MultiLoanOrder
	if orderID == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "loan.GetOrder: orderID is empty", nil)
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
		return info, gate.NewError(gate.ErrorKindUnknown, "", "loan.GetOrder: parse", err)
	}
	return orderFromPayload(&p, rateLimits), nil
}

// ---- repay -----------------------------------------------------------------

type repayRecordPayload struct {
	OrderID   int64             `json:"order_id"`
	RecordID  int64             `json:"record_id"`
	RepayTime int64             `json:"repay_time"`
	Currency  string            `json:"currency"`
	Principal codec.FlexDecimal `json:"principal"`
	Interest  codec.FlexDecimal `json:"interest"`
	RepaidAll bool              `json:"repaid_all"`
}

func repayRecordFromPayload(p *repayRecordPayload, rateLimits map[string]string) types.RepayRecord {
	return types.RepayRecord{
		OrderID:    idString(p.OrderID),
		RecordID:   idString(p.RecordID),
		RepaidAtMs: secondsToMs(p.RepayTime),
		Currency:   p.Currency,
		Principal:  p.Principal.Decimal,
		Interest:   p.Interest.Decimal,
		RepaidAll:  p.RepaidAll,
		RateLimits: rateLimits,
	}
}

// ListRepayRecords returns the multi-collateral repayment history.
func (c *Client) ListRepayRecords(ctx context.Context, params ListRepayRecordsParams) ([]types.RepayRecord, error) {
	var q = newQuery()
	if params.OrderID != "" {
		q.Set("order_id", params.OrderID)
	}
	if params.Currency != "" {
		q.Set("currency", params.Currency)
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
		Path:   c.repayPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []repayRecordPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "loan.ListRepayRecords: parse", err)
	}
	var out []types.RepayRecord = make([]types.RepayRecord, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, repayRecordFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// Repay repays a multi-collateral loan with one or more per-currency
// instructions. Returns the updated loan order.
func (c *Client) Repay(ctx context.Context, req types.RepayRequest) (types.MultiLoanOrder, error) {
	var info types.MultiLoanOrder
	if req.OrderID == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "loan.Repay: OrderID is empty", nil)
	}
	if len(req.RepayItems) == 0 {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "loan.Repay: RepayItems is empty", nil)
	}

	var items []map[string]any = make([]map[string]any, 0, len(req.RepayItems))
	var k int
	for k = 0; k < len(req.RepayItems); k++ {
		if req.RepayItems[k].Currency == "" {
			return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "loan.Repay: RepayItems currency is empty", nil)
		}
		var item map[string]any = make(map[string]any, 3)
		item["currency"] = req.RepayItems[k].Currency
		if req.RepayItems[k].RepaidAll {
			item["repaid_all"] = true
		} else {
			item["amount"] = req.RepayItems[k].Amount.String()
		}
		items = append(items, item)
	}

	var body map[string]any = make(map[string]any, 2)
	body["order_id"] = req.OrderID
	body["repay_items"] = items

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   c.repayPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p orderPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "loan.Repay: parse", err)
	}
	return orderFromPayload(&p, rateLimits), nil
}

// ---- mortgage --------------------------------------------------------------

type mortgageRecordPayload struct {
	OrderID    int64             `json:"order_id"`
	OperatTime int64             `json:"operat_time"`
	Type       string            `json:"type"`
	Currency   string            `json:"currency"`
	Amount     codec.FlexDecimal `json:"amount"`
}

func mortgageRecordFromPayload(p *mortgageRecordPayload, rateLimits map[string]string) types.MortgageRecord {
	return types.MortgageRecord{
		OrderID:      idString(p.OrderID),
		OperatedAtMs: secondsToMs(p.OperatTime),
		Type:         types.MortgageType(p.Type),
		Currency:     p.Currency,
		Amount:       p.Amount.Decimal,
		RateLimits:   rateLimits,
	}
}

// ListMortgageRecords returns the multi-collateral collateral-operation history.
func (c *Client) ListMortgageRecords(ctx context.Context, params ListMortgageRecordsParams) ([]types.MortgageRecord, error) {
	var q = newQuery()
	if params.OrderID != "" {
		q.Set("order_id", params.OrderID)
	}
	if params.Currency != "" {
		q.Set("currency", params.Currency)
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
		Path:   c.mortgagePath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []mortgageRecordPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "loan.ListMortgageRecords: parse", err)
	}
	var out []types.MortgageRecord = make([]types.MortgageRecord, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, mortgageRecordFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// OperateMortgage adds (append) or withdraws (redeem) collateral on a
// multi-collateral loan.
func (c *Client) OperateMortgage(ctx context.Context, req types.MortgageRequest) error {
	if req.OrderID == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "loan.OperateMortgage: OrderID is empty", nil)
	}
	if req.Type != types.MortgageTypeAppend && req.Type != types.MortgageTypeRedeem {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "loan.OperateMortgage: Type must be append or redeem", nil)
	}
	if len(req.Collaterals) == 0 {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "loan.OperateMortgage: Collaterals is empty", nil)
	}

	var collaterals []map[string]any = make([]map[string]any, 0, len(req.Collaterals))
	var k int
	for k = 0; k < len(req.Collaterals); k++ {
		if req.Collaterals[k].Currency == "" {
			return gate.NewError(gate.ErrorKindInvalidRequest, "", "loan.OperateMortgage: collateral currency is empty", nil)
		}
		collaterals = append(collaterals, map[string]any{
			"currency": req.Collaterals[k].Currency,
			"amount":   req.Collaterals[k].Amount.String(),
		})
	}

	var body map[string]any = make(map[string]any, 3)
	body["order_id"] = req.OrderID
	body["type"] = string(req.Type)
	body["collaterals"] = collaterals

	var err error
	_, _, err = c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   c.mortgagePath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	return err
}

// ---- currency quota --------------------------------------------------------

type currencyQuotaPayload struct {
	Currency string            `json:"currency"`
	Index    string            `json:"index"`
	Quota    codec.FlexDecimal `json:"quota"`
}

// GetCurrencyQuota returns the borrow/collateral quota for a currency.
// quotaType is the Gate quota kind (e.g. "borrow" or "collateral").
func (c *Client) GetCurrencyQuota(ctx context.Context, quotaType, currency string) ([]types.CurrencyQuota, error) {
	if quotaType == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "loan.GetCurrencyQuota: quotaType is empty", nil)
	}
	if currency == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "loan.GetCurrencyQuota: currency is empty", nil)
	}
	var q = newQuery()
	q.Set("type", quotaType)
	q.Set("currency", currency)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.currencyQuotaPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []currencyQuotaPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "loan.GetCurrencyQuota: parse", err)
	}
	var out []types.CurrencyQuota = make([]types.CurrencyQuota, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, types.CurrencyQuota{
			Currency:   payloads[k].Currency,
			Index:      payloads[k].Index,
			Quota:      payloads[k].Quota.Decimal,
			RateLimits: rateLimits,
		})
	}
	return out, nil
}

// ---- currencies (public) ---------------------------------------------------

type currencyPayload struct {
	Currency        string            `json:"currency"`
	PrecisionAmount int64             `json:"precision_amount"`
	MinBorrowAmount codec.FlexDecimal `json:"min_borrow_amount"`
	Ltv             codec.FlexDecimal `json:"ltv"`
	LoanType        string            `json:"loan_type"`
}

// ListCurrencies returns the supported borrow/collateral currencies (public).
// loanType is an optional Gate filter (e.g. "borrow"/"collateral"); pass empty
// to omit.
func (c *Client) ListCurrencies(ctx context.Context, loanType string) ([]types.MultiLoanCurrency, error) {
	var q = newQuery()
	if loanType != "" {
		q.Set("loan_type", loanType)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.currenciesPath(),
		Query:  q,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []currencyPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "loan.ListCurrencies: parse", err)
	}
	var out []types.MultiLoanCurrency = make([]types.MultiLoanCurrency, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, types.MultiLoanCurrency{
			Currency:        payloads[k].Currency,
			PrecisionAmount: payloads[k].PrecisionAmount,
			MinBorrowAmount: payloads[k].MinBorrowAmount.Decimal,
			Ltv:             payloads[k].Ltv.Decimal,
			LoanType:        payloads[k].LoanType,
			RateLimits:      rateLimits,
		})
	}
	return out, nil
}

// ---- ltv -------------------------------------------------------------------

type ltvPayload struct {
	CurrentLtv     codec.FlexDecimal `json:"current_ltv"`
	LiquidationLtv codec.FlexDecimal `json:"liquidate_ltv"`
	AlertLtv       codec.FlexDecimal `json:"alert_ltv"`
}

// GetLtv returns the current / liquidation / alert loan-to-value ratios. orderID
// is an optional Gate disambiguator (pass empty to omit).
func (c *Client) GetLtv(ctx context.Context, orderID string) (types.Ltv, error) {
	var info types.Ltv
	var q = newQuery()
	if orderID != "" {
		q.Set("order_id", orderID)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.ltvPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p ltvPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "loan.GetLtv: parse", err)
	}
	return types.Ltv{
		CurrentLtv:     p.CurrentLtv.Decimal,
		LiquidationLtv: p.LiquidationLtv.Decimal,
		AlertLtv:       p.AlertLtv.Decimal,
		RateLimits:     rateLimits,
	}, nil
}

// ---- fixed / current rate (public) -----------------------------------------

type fixedRatePayload struct {
	Currency  string            `json:"currency"`
	FixedType string            `json:"fixed_type"`
	Rate      codec.FlexDecimal `json:"rate"`
}

// GetFixedRate returns the fixed borrow rates for a currency (public).
func (c *Client) GetFixedRate(ctx context.Context, currency string) ([]types.FixedRate, error) {
	if currency == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "loan.GetFixedRate: currency is empty", nil)
	}
	var q = newQuery()
	q.Set("currency", currency)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.fixedRatePath(),
		Query:  q,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []fixedRatePayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "loan.GetFixedRate: parse", err)
	}
	var out []types.FixedRate = make([]types.FixedRate, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, types.FixedRate{
			Currency:   payloads[k].Currency,
			FixedType:  types.MultiLoanFixedType(payloads[k].FixedType),
			Rate:       payloads[k].Rate.Decimal,
			RateLimits: rateLimits,
		})
	}
	return out, nil
}

type currentRatePayload struct {
	Currency string            `json:"currency"`
	Rate     codec.FlexDecimal `json:"rate"`
}

// GetCurrentRate returns the current (floating) borrow rates for one or more
// currencies (public). currencies are comma-joined into the Gate query.
func (c *Client) GetCurrentRate(ctx context.Context, currencies []string) ([]types.CurrentRate, error) {
	if len(currencies) == 0 {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "loan.GetCurrentRate: currencies is empty", nil)
	}
	var q = newQuery()
	q.Set("currencies", strings.Join(currencies, ","))

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.currentRatePath(),
		Query:  q,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []currentRatePayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "loan.GetCurrentRate: parse", err)
	}
	var out []types.CurrentRate = make([]types.CurrentRate, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, types.CurrentRate{
			Currency:   payloads[k].Currency,
			Rate:       payloads[k].Rate.Decimal,
			RateLimits: rateLimits,
		})
	}
	return out, nil
}
