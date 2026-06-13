/*
FILE: earn/lends.go

DESCRIPTION:
Signed lending positions and mutations for the Gate Earn "Uni" section:
  - ListUserLends   : GET   /earn/uni/lends         (current positions)
  - CreateLend      : POST  /earn/uni/lends         (lend / redeem principal)
  - ChangeLend      : PATCH /earn/uni/lends         (adjust rate / auto-renew)
  - ListLendRecords : GET   /earn/uni/lend_records  (lend/redeem history)

All four are private (Signed: true). Request bodies are built as maps so Gate
omits absent optional fields; the required ones are always present. Decimal
amounts/rates use codec.FlexDecimal on the wire (number-or-string tolerant).
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

func (c *Client) lendsPath() string       { return c.basePath() + "/lends" }
func (c *Client) lendRecordsPath() string { return c.basePath() + "/lend_records" }

// uniLendPayload — Gate Uni lend (current position) wire shape.
type uniLendPayload struct {
	Currency           string            `json:"currency"`
	CurrentAmount      codec.FlexDecimal `json:"current_amount"`
	MinRate            codec.FlexDecimal `json:"min_rate"`
	LeftAmount         codec.FlexDecimal `json:"left_amount"`
	FrozenAmount       codec.FlexDecimal `json:"frozen_amount"`
	InterestStatus     string            `json:"interest_status"`
	ReinvestLeftAmount codec.FlexDecimal `json:"reinvest_left_amount"`
	CreateTime         int64             `json:"create_time"`
	UpdateTime         int64             `json:"update_time"`
}

func uniLendFromPayload(p *uniLendPayload, rateLimits map[string]string) types.UniLend {
	return types.UniLend{
		Currency:           p.Currency,
		CurrentAmount:      p.CurrentAmount.Decimal,
		MinRate:            p.MinRate.Decimal,
		LeftAmount:         p.LeftAmount.Decimal,
		FrozenAmount:       p.FrozenAmount.Decimal,
		InterestStatus:     types.InterestStatus(p.InterestStatus),
		ReinvestLeftAmount: p.ReinvestLeftAmount.Decimal,
		CreatedAtMs:        secondsToMs(p.CreateTime),
		UpdatedAtMs:        secondsToMs(p.UpdateTime),
		RateLimits:         rateLimits,
	}
}

// ListUserLends returns the caller's current Uni lending positions. Pass an
// empty currency for all currencies. page/limit ≤ 0 let Gate use its defaults.
func (c *Client) ListUserLends(ctx context.Context, currency string, page, limit int) ([]types.UniLend, error) {
	var q = newQuery()
	if currency != "" {
		q.Set("currency", currency)
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
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.lendsPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []uniLendPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "earn.ListUserLends: parse", err)
	}
	var out []types.UniLend = make([]types.UniLend, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, uniLendFromPayload(&payloads[i], rateLimits))
	}
	return out, nil
}

// CreateLend lends (adds) or redeems (withdraws) principal in the Uni pool.
// req must have Currency, a positive Amount, and a Type (lend|redeem). MinRate
// and AutoRenew are optional. SDK validation errors are returned WITHOUT sending
// a request.
func (c *Client) CreateLend(ctx context.Context, req types.CreateLendRequest) error {
	var body map[string]any
	var err error
	body, err = buildCreateLendBody(req)
	if err != nil {
		return err
	}
	_, _, err = c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   c.lendsPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryPlace)},
	})
	return err
}

// buildCreateLendBody assembles the POST /earn/uni/lends request body.
func buildCreateLendBody(req types.CreateLendRequest) (map[string]any, error) {
	if req.Currency == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.CreateLend: Currency is empty", nil)
	}
	if req.Amount.IsZero() || req.Amount.IsNegative() {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.CreateLend: Amount must be positive", nil)
	}
	switch req.Type {
	case types.LendTypeLend, types.LendTypeRedeem:
	default:
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.CreateLend: Type must be lend or redeem", nil)
	}

	var body map[string]any = make(map[string]any, 5)
	body["currency"] = req.Currency
	body["amount"] = req.Amount.String()
	body["type"] = string(req.Type)
	if !req.MinRate.IsZero() {
		body["min_rate"] = req.MinRate.String()
	}
	if req.AutoRenew != nil {
		body["auto_renew"] = *req.AutoRenew
	}
	return body, nil
}

// ChangeLend adjusts an existing position's floor rate and/or auto-renew flag
// (PATCH /earn/uni/lends). It does NOT move principal. req must have Currency.
func (c *Client) ChangeLend(ctx context.Context, req types.ChangeLendRequest) error {
	var body map[string]any
	var err error
	body, err = buildChangeLendBody(req)
	if err != nil {
		return err
	}
	_, _, err = c.rest().Do(ctx, rest.Options{
		Method: "PATCH",
		Path:   c.lendsPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryAmend)},
	})
	return err
}

// buildChangeLendBody assembles the PATCH /earn/uni/lends request body.
func buildChangeLendBody(req types.ChangeLendRequest) (map[string]any, error) {
	if req.Currency == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.ChangeLend: Currency is empty", nil)
	}
	var body map[string]any = make(map[string]any, 3)
	body["currency"] = req.Currency
	if !req.MinRate.IsZero() {
		body["min_rate"] = req.MinRate.String()
	}
	if req.AutoRenew != nil {
		body["auto_renew"] = *req.AutoRenew
	}
	return body, nil
}

// lendRecordPayload — Gate Uni lend-record wire shape.
type lendRecordPayload struct {
	Currency         string            `json:"currency"`
	Amount           codec.FlexDecimal `json:"amount"`
	LastWalletAmount codec.FlexDecimal `json:"last_wallet_amount"`
	LastLentAmount   codec.FlexDecimal `json:"last_lent_amount"`
	LastFrozenAmount codec.FlexDecimal `json:"last_frozen_amount"`
	Type             string            `json:"type"`
	CreateTime       int64             `json:"create_time"`
}

// ListLendRecords returns the caller's lend/redeem history. All filters are
// optional: empty currency / empty lendType / from==to==0 / page==limit<=0 let
// Gate apply its defaults. from/to are epoch SECONDS (Gate's native unit).
func (c *Client) ListLendRecords(ctx context.Context, currency string, from, to int64, lendType types.LendType, page, limit int) ([]types.LendRecord, error) {
	var q = newQuery()
	if currency != "" {
		q.Set("currency", currency)
	}
	if from > 0 {
		q.Set("from", strconv.FormatInt(from, 10))
	}
	if to > 0 {
		q.Set("to", strconv.FormatInt(to, 10))
	}
	if lendType != "" {
		q.Set("type", string(lendType))
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
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.lendRecordsPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []lendRecordPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "earn.ListLendRecords: parse", err)
	}
	var out []types.LendRecord = make([]types.LendRecord, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, types.LendRecord{
			Currency:         payloads[i].Currency,
			Amount:           payloads[i].Amount.Decimal,
			LastWalletAmount: payloads[i].LastWalletAmount.Decimal,
			LastLentAmount:   payloads[i].LastLentAmount.Decimal,
			LastFrozenAmount: payloads[i].LastFrozenAmount.Decimal,
			Type:             types.LendType(payloads[i].Type),
			CreatedAtMs:      secondsToMs(payloads[i].CreateTime),
			RateLimits:       rateLimits,
		})
	}
	return out, nil
}
