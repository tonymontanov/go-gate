/*
FILE: unified/loans.go

DESCRIPTION:
Loan / interest endpoints for the Gate Unified Account section. Implements
(all private, signed):
  - ListLoans           : GET  /unified/loans?currency=&page=&limit=
  - CreateLoan          : POST /unified/loans          (borrow/repay)
  - ListLoanRecords     : GET  /unified/loan_records?type=&currency=&page=&limit=
  - ListInterestRecords : GET  /unified/interest_records?currency=&page=&limit=
  - GetEstimateRate     : GET  /unified/estimate_rate?currencies=
  - GetHistoryLoanRate  : GET  /unified/history_loan_rate?currency=&tier=

CreateLoan POSTs a map body (currency, amount, type, repaid_all) so Gate's
optional fields are omitted when unset.
*/

package unified

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
	"github.com/tonymontanov/go-gate/v2/unified/types"
)

func (c *Client) loansPath() string           { return c.basePath() + "/loans" }
func (c *Client) loanRecordsPath() string     { return c.basePath() + "/loan_records" }
func (c *Client) interestRecordsPath() string { return c.basePath() + "/interest_records" }
func (c *Client) estimateRatePath() string    { return c.basePath() + "/estimate_rate" }
func (c *Client) historyLoanRatePath() string { return c.basePath() + "/history_loan_rate" }

// setPaging adds Gate's page/limit query parameters when positive.
func setPaging(q interface{ Set(string, string) }, page, limit int) {
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
}

// ---- loans -----------------------------------------------------------------

type loanPayload struct {
	Currency     string `json:"currency"`
	CurrencyPair string `json:"currency_pair"`
	Amount       string `json:"amount"`
	Type         string `json:"type"`
	CreateTime   int64  `json:"create_time"`
	UpdateTime   int64  `json:"update_time"`
}

func loanFromPayload(p *loanPayload, rateLimits map[string]string) types.UnifiedLoan {
	return types.UnifiedLoan{
		Currency:     p.Currency,
		CurrencyPair: p.CurrencyPair,
		Amount:       mustDecimal(p.Amount),
		Type:         p.Type,
		CreateTimeMs: secondsToMs(p.CreateTime),
		UpdateTimeMs: secondsToMs(p.UpdateTime),
		RateLimits:   rateLimits,
	}
}

// ListLoans returns the account's outstanding unified loans. currency narrows to
// a single currency (empty = all); page/limit ≤ 0 use Gate defaults.
func (c *Client) ListLoans(ctx context.Context, currency string, page, limit int) ([]types.UnifiedLoan, error) {
	var q = newQuery()
	if currency != "" {
		q.Set("currency", currency)
	}
	setPaging(q, page, limit)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.loansPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []loanPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "unified.ListLoans: parse", err)
	}
	var out []types.UnifiedLoan = make([]types.UnifiedLoan, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, loanFromPayload(&payloads[i], rateLimits))
	}
	return out, nil
}

// CreateLoan borrows or repays a currency in the unified account. For a repay
// with RepaidAll set, Amount is ignored by Gate.
func (c *Client) CreateLoan(ctx context.Context, req types.CreateLoanRequest) (types.UnifiedLoan, error) {
	var out types.UnifiedLoan
	if req.Currency == "" {
		return out, gate.NewError(gate.ErrorKindInvalidRequest, "", "unified.CreateLoan: currency is empty", nil)
	}
	if req.Type != types.LoanTypeBorrow && req.Type != types.LoanTypeRepay {
		return out, gate.NewError(gate.ErrorKindInvalidRequest, "", "unified.CreateLoan: type must be borrow or repay", nil)
	}
	if !req.RepaidAll && req.Amount.Sign() <= 0 {
		return out, gate.NewError(gate.ErrorKindInvalidRequest, "", "unified.CreateLoan: amount must be positive (or set RepaidAll)", nil)
	}

	var body map[string]any = map[string]any{
		"currency": req.Currency,
		"type":     string(req.Type),
		"amount":   req.Amount.String(),
	}
	if req.RepaidAll {
		body["repaid_all"] = true
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   c.loansPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return out, err
	}
	var p loanPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return out, gate.NewError(gate.ErrorKindUnknown, "", "unified.CreateLoan: parse", err)
	}
	// Gate may echo an empty body on success; fall back to the request shape.
	if p.Currency == "" {
		return types.UnifiedLoan{
			Currency:   req.Currency,
			Amount:     req.Amount,
			Type:       string(req.Type),
			RateLimits: rateLimits,
		}, nil
	}
	return loanFromPayload(&p, rateLimits), nil
}

// ---- loan records ----------------------------------------------------------

type loanRecordPayload struct {
	ID         json.Number `json:"id"`
	Type       string      `json:"type"`
	Currency   string      `json:"currency"`
	Amount     string      `json:"amount"`
	CreateTime int64       `json:"create_time"`
}

// ListLoanRecords returns the account's borrow/repay history. loanType narrows to
// "borrow"/"repay" (empty = both); currency narrows to one currency; page/limit ≤
// 0 use Gate defaults.
func (c *Client) ListLoanRecords(ctx context.Context, loanType types.LoanType, currency string, page, limit int) ([]types.LoanRecord, error) {
	var q = newQuery()
	if loanType != "" {
		q.Set("type", string(loanType))
	}
	if currency != "" {
		q.Set("currency", currency)
	}
	setPaging(q, page, limit)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.loanRecordsPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []loanRecordPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "unified.ListLoanRecords: parse", err)
	}
	var out []types.LoanRecord = make([]types.LoanRecord, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, types.LoanRecord{
			ID:           payloads[i].ID.String(),
			Type:         types.LoanType(payloads[i].Type),
			Currency:     payloads[i].Currency,
			Amount:       mustDecimal(payloads[i].Amount),
			CreateTimeMs: secondsToMs(payloads[i].CreateTime),
			RateLimits:   rateLimits,
		})
	}
	return out, nil
}

// ---- interest records ------------------------------------------------------

type interestRecordPayload struct {
	Currency     string `json:"currency"`
	CurrencyPair string `json:"currency_pair"`
	ActualRate   string `json:"actual_rate"`
	Interest     string `json:"interest"`
	Status       int64  `json:"status"`
	Type         string `json:"type"`
	CreateTime   int64  `json:"create_time"`
}

// ListInterestRecords returns the account's interest-accrual history. currency
// narrows to one currency (empty = all); page/limit ≤ 0 use Gate defaults.
func (c *Client) ListInterestRecords(ctx context.Context, currency string, page, limit int) ([]types.InterestRecord, error) {
	var q = newQuery()
	if currency != "" {
		q.Set("currency", currency)
	}
	setPaging(q, page, limit)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.interestRecordsPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []interestRecordPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "unified.ListInterestRecords: parse", err)
	}
	var out []types.InterestRecord = make([]types.InterestRecord, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, types.InterestRecord{
			Currency:     payloads[i].Currency,
			CurrencyPair: payloads[i].CurrencyPair,
			ActualRate:   mustDecimal(payloads[i].ActualRate),
			Interest:     mustDecimal(payloads[i].Interest),
			Status:       payloads[i].Status,
			Type:         payloads[i].Type,
			CreateTimeMs: secondsToMs(payloads[i].CreateTime),
			RateLimits:   rateLimits,
		})
	}
	return out, nil
}

// ---- estimate rate ---------------------------------------------------------

// GetEstimateRate returns the estimated next-period borrow rate for each of the
// requested currencies (Gate comma-joins the currencies query parameter and
// returns a currency→rate object).
func (c *Client) GetEstimateRate(ctx context.Context, currencies []string) (types.EstimateRate, error) {
	var out types.EstimateRate
	if len(currencies) == 0 {
		return out, gate.NewError(gate.ErrorKindInvalidRequest, "", "unified.GetEstimateRate: currencies is empty", nil)
	}
	var q = newQuery()
	q.Set("currencies", strings.Join(currencies, ","))

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.estimateRatePath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return out, err
	}
	var raw map[string]string
	if err = resp.UnmarshalData(&raw); err != nil {
		return out, gate.NewError(gate.ErrorKindUnknown, "", "unified.GetEstimateRate: parse", err)
	}
	out.Rates = make(map[string]decimal.Decimal, len(raw))
	var ccy, rate string
	for ccy, rate = range raw {
		out.Rates[ccy] = mustDecimal(rate)
	}
	out.RateLimits = rateLimits
	return out, nil
}

// ---- history loan rate -----------------------------------------------------

type ratePointPayload struct {
	Time int64  `json:"time"`
	Rate string `json:"rate"`
}

type historyLoanRatePayload struct {
	Currency string             `json:"currency"`
	Tier     string             `json:"tier"`
	Rate     string             `json:"rate"`
	Rates    []ratePointPayload `json:"rates"`
}

// GetHistoryLoanRate returns the historical loan-rate series for a currency at a
// VIP/loan tier (tier may be empty for the account's default).
func (c *Client) GetHistoryLoanRate(ctx context.Context, currency, tier string) (types.HistoryLoanRate, error) {
	var out types.HistoryLoanRate
	if currency == "" {
		return out, gate.NewError(gate.ErrorKindInvalidRequest, "", "unified.GetHistoryLoanRate: currency is empty", nil)
	}
	var q = newQuery()
	q.Set("currency", currency)
	if tier != "" {
		q.Set("tier", tier)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.historyLoanRatePath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return out, err
	}
	var p historyLoanRatePayload
	if err = resp.UnmarshalData(&p); err != nil {
		return out, gate.NewError(gate.ErrorKindUnknown, "", "unified.GetHistoryLoanRate: parse", err)
	}
	out = types.HistoryLoanRate{
		Currency:   p.Currency,
		Tier:       p.Tier,
		Rate:       mustDecimal(p.Rate),
		RateLimits: rateLimits,
	}
	if out.Currency == "" {
		out.Currency = currency
	}
	if len(p.Rates) > 0 {
		out.Rates = make([]types.RatePoint, 0, len(p.Rates))
		var i int
		for i = 0; i < len(p.Rates); i++ {
			out.Rates = append(out.Rates, types.RatePoint{
				TimeMs: secondsToMs(p.Rates[i].Time),
				Rate:   mustDecimal(p.Rates[i].Rate),
			})
		}
	}
	return out, nil
}
