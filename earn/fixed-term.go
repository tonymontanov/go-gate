/*
FILE: earn/fixed-term.go

DESCRIPTION:
FixedTermClient — the Gate Earn Fixed-Term lending sub-client, reachable via
earn.Client.FixedTerm(). Unlike the Uni flexible-lending methods that hang
directly off earn.Client (base "/earn/uni"), Fixed-Term builds its paths under a
DIFFERENT base, "/earn/fixed-term", so it does NOT reuse earn.Client.basePath().

ENDPOINTS:
  - ListProducts        GET  /earn/fixed-term/product               (public)
  - ListProductsByAsset GET  /earn/fixed-term/product/{asset}/list  (public)
  - CreateLend          POST /earn/fixed-term/user/lend             (signed)
  - ListLends           GET  /earn/fixed-term/user/lend             (signed)
  - CreatePreRedeem     POST /earn/fixed-term/user/pre-redeem       (signed)
  - ListHistory         GET  /earn/fixed-term/user/history          (signed)

Amounts/APRs decode through codec.FlexDecimal (Gate may quote them as JSON
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

// FixedTermClient — Gate Earn Fixed-Term lending sub-client.
type FixedTermClient struct {
	c *Client
}

func newFixedTermClient(c *Client) *FixedTermClient {
	return &FixedTermClient{c: c}
}

// fixedTermBasePath returns "/earn/fixed-term". It is deliberately separate from
// earn.Client.basePath() ("/earn/uni").
func (f *FixedTermClient) fixedTermBasePath() string { return "/earn/fixed-term" }

func (f *FixedTermClient) productPath() string { return f.fixedTermBasePath() + "/product" }
func (f *FixedTermClient) productByAssetPath(asset string) string {
	return f.fixedTermBasePath() + "/product/" + asset + "/list"
}
func (f *FixedTermClient) userLendPath() string  { return f.fixedTermBasePath() + "/user/lend" }
func (f *FixedTermClient) preRedeemPath() string { return f.fixedTermBasePath() + "/user/pre-redeem" }
func (f *FixedTermClient) historyPath() string   { return f.fixedTermBasePath() + "/user/history" }

// ---- products (public) -----------------------------------------------------

// fixedTermTierPayload — one rung of a product's amount→APR ladder.
type fixedTermTierPayload struct {
	MinAmount codec.FlexDecimal `json:"min_amount"`
	MaxAmount codec.FlexDecimal `json:"max_amount"`
	APR       codec.FlexDecimal `json:"apr"`
}

// fixedTermProductPayload — Gate Fixed-Term product wire shape.
type fixedTermProductPayload struct {
	Pid          string                 `json:"pid"`
	ID           int64                  `json:"id"`
	Asset        string                 `json:"asset"`
	Type         string                 `json:"type"`
	APR          codec.FlexDecimal      `json:"apr"`
	MinAPR       codec.FlexDecimal      `json:"min_apr"`
	MaxAPR       codec.FlexDecimal      `json:"max_apr"`
	MinAmount    codec.FlexDecimal      `json:"min_amount"`
	MaxAmount    codec.FlexDecimal      `json:"max_amount"`
	DurationDays int64                  `json:"duration_days"`
	Tiers        []fixedTermTierPayload `json:"tiers"`
	StartTime    int64                  `json:"start_time"`
	EndTime      int64                  `json:"end_time"`
	Status       string                 `json:"status"`
}

func fixedTermTiersFromPayload(items []fixedTermTierPayload) []types.FixedTermTier {
	var out []types.FixedTermTier = make([]types.FixedTermTier, 0, len(items))
	var k int
	for k = 0; k < len(items); k++ {
		out = append(out, types.FixedTermTier{
			MinAmount: items[k].MinAmount.Decimal,
			MaxAmount: items[k].MaxAmount.Decimal,
			APR:       items[k].APR.Decimal,
		})
	}
	return out
}

func fixedTermProductFromPayload(p *fixedTermProductPayload, rateLimits map[string]string) types.FixedTermProduct {
	var id string = p.Pid
	if id == "" {
		id = idString(p.ID)
	}
	return types.FixedTermProduct{
		ID:           id,
		Asset:        p.Asset,
		Type:         p.Type,
		APR:          p.APR.Decimal,
		MinAPR:       p.MinAPR.Decimal,
		MaxAPR:       p.MaxAPR.Decimal,
		MinAmount:    p.MinAmount.Decimal,
		MaxAmount:    p.MaxAmount.Decimal,
		DurationDays: p.DurationDays,
		Tiers:        fixedTermTiersFromPayload(p.Tiers),
		StartTimeMs:  secondsToMs(p.StartTime),
		EndTimeMs:    secondsToMs(p.EndTime),
		Status:       p.Status,
		RateLimits:   rateLimits,
	}
}

// ListProducts returns every fixed-term lending product Gate currently offers.
// Public: no signature required.
func (f *FixedTermClient) ListProducts(ctx context.Context) ([]types.FixedTermProduct, error) {
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = f.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   f.productPath(),
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []fixedTermProductPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "earn.FixedTerm.ListProducts: parse", err)
	}
	var out []types.FixedTermProduct = make([]types.FixedTermProduct, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, fixedTermProductFromPayload(&payloads[i], rateLimits))
	}
	return out, nil
}

// ListProductsByAsset returns the fixed-term products available for a single
// asset (e.g. "USDT"). Public: no signature required.
func (f *FixedTermClient) ListProductsByAsset(ctx context.Context, asset string) ([]types.FixedTermProduct, error) {
	if asset == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.FixedTerm.ListProductsByAsset: asset is empty", nil)
	}
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = f.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   f.productByAssetPath(asset),
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []fixedTermProductPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "earn.FixedTerm.ListProductsByAsset: parse", err)
	}
	var out []types.FixedTermProduct = make([]types.FixedTermProduct, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, fixedTermProductFromPayload(&payloads[i], rateLimits))
	}
	return out, nil
}

// ---- lends (signed) --------------------------------------------------------

// fixedTermLendPayload — Gate Fixed-Term lend wire shape.
type fixedTermLendPayload struct {
	ID         int64             `json:"id"`
	Pid        string            `json:"pid"`
	Asset      string            `json:"asset"`
	Amount     codec.FlexDecimal `json:"amount"`
	APR        codec.FlexDecimal `json:"apr"`
	CreateTime int64             `json:"create_time"`
	SettleTime int64             `json:"settle_time"`
	RedeemTime int64             `json:"redeem_time"`
	Status     string            `json:"status"`
}

func fixedTermLendFromPayload(p *fixedTermLendPayload, rateLimits map[string]string) types.FixedTermLend {
	return types.FixedTermLend{
		ID:           idString(p.ID),
		ProductID:    p.Pid,
		Asset:        p.Asset,
		Amount:       p.Amount.Decimal,
		APR:          p.APR.Decimal,
		CreatedAtMs:  secondsToMs(p.CreateTime),
		SettleTimeMs: secondsToMs(p.SettleTime),
		RedeemTimeMs: secondsToMs(p.RedeemTime),
		Status:       p.Status,
		RateLimits:   rateLimits,
	}
}

// CreateLend subscribes principal to a fixed-term product. req must have a
// ProductID and a positive Amount. Returns the created lend. SDK validation
// errors are returned WITHOUT sending a request.
func (f *FixedTermClient) CreateLend(ctx context.Context, req types.CreateFixedLendRequest) (types.FixedTermLend, error) {
	var info types.FixedTermLend
	if req.ProductID == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.FixedTerm.CreateLend: ProductID is empty", nil)
	}
	if req.Amount.IsZero() || req.Amount.IsNegative() {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.FixedTerm.CreateLend: Amount must be positive", nil)
	}

	var body map[string]any = make(map[string]any, 2)
	body["pid"] = req.ProductID
	body["amount"] = req.Amount.String()

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = f.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   f.userLendPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryPlace)},
	})
	if err != nil {
		return info, err
	}
	var p fixedTermLendPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "earn.FixedTerm.CreateLend: parse", err)
	}
	return fixedTermLendFromPayload(&p, rateLimits), nil
}

// ListLends returns the caller's fixed-term lend positions. Pass an empty asset
// for all assets; page/limit ≤ 0 let Gate use its defaults.
func (f *FixedTermClient) ListLends(ctx context.Context, asset string, page, limit int) ([]types.FixedTermLend, error) {
	var q = newQuery()
	if asset != "" {
		q.Set("asset", asset)
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
	resp, rateLimits, err = f.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   f.userLendPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []fixedTermLendPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "earn.FixedTerm.ListLends: parse", err)
	}
	var out []types.FixedTermLend = make([]types.FixedTermLend, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, fixedTermLendFromPayload(&payloads[i], rateLimits))
	}
	return out, nil
}

// CreatePreRedeem queues an early redemption of a fixed-term lend. req must have
// an ID (the lend id). SDK validation errors are returned WITHOUT sending a
// request.
func (f *FixedTermClient) CreatePreRedeem(ctx context.Context, req types.PreRedeemRequest) error {
	if req.ID == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.FixedTerm.CreatePreRedeem: ID is empty", nil)
	}
	var body map[string]any = map[string]any{"id": req.ID}
	_, _, err := f.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   f.preRedeemPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryPlace)},
	})
	return err
}

// ---- history (signed) ------------------------------------------------------

// fixedTermHistoryPayload — Gate Fixed-Term history-record wire shape.
type fixedTermHistoryPayload struct {
	ID         int64             `json:"id"`
	Pid        string            `json:"pid"`
	Asset      string            `json:"asset"`
	Type       string            `json:"type"`
	Amount     codec.FlexDecimal `json:"amount"`
	APR        codec.FlexDecimal `json:"apr"`
	CreateTime int64             `json:"create_time"`
	Status     string            `json:"status"`
}

// ListHistory returns the caller's fixed-term lend/redeem history. All filters
// are optional: empty asset / from==to==0 / page==limit≤0 let Gate apply its
// defaults. from/to are epoch SECONDS (Gate's native unit).
func (f *FixedTermClient) ListHistory(ctx context.Context, asset string, from, to int64, page, limit int) ([]types.FixedTermHistory, error) {
	var q = newQuery()
	if asset != "" {
		q.Set("asset", asset)
	}
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
	resp, rateLimits, err = f.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   f.historyPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []fixedTermHistoryPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "earn.FixedTerm.ListHistory: parse", err)
	}
	var out []types.FixedTermHistory = make([]types.FixedTermHistory, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, types.FixedTermHistory{
			ID:          idString(payloads[i].ID),
			ProductID:   payloads[i].Pid,
			Asset:       payloads[i].Asset,
			Type:        payloads[i].Type,
			Amount:      payloads[i].Amount.Decimal,
			APR:         payloads[i].APR.Decimal,
			CreatedAtMs: secondsToMs(payloads[i].CreateTime),
			Status:      payloads[i].Status,
			RateLimits:  rateLimits,
		})
	}
	return out, nil
}
