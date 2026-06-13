/*
FILE: earn/interest.go

DESCRIPTION:
Signed interest queries for the Gate Earn "Uni" section:
  - GetInterest        : GET /earn/uni/interests/{currency}        (total income)
  - ListInterestRecords: GET /earn/uni/interest_records            (payout history)
  - GetInterestStatus  : GET /earn/uni/interest_status/{currency}  (auto-reinvest)

All three are private (Signed: true). Interest amounts/rates use codec.FlexDecimal
on the wire (number-or-string tolerant).
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

func (c *Client) interestRecordsPath() string { return c.basePath() + "/interest_records" }
func (c *Client) interestPath(currency string) string {
	return c.basePath() + "/interests/" + currency
}
func (c *Client) interestStatusPath(currency string) string {
	return c.basePath() + "/interest_status/" + currency
}

// interestPayload — Gate Uni total-interest wire shape.
type interestPayload struct {
	Currency string            `json:"currency"`
	Interest codec.FlexDecimal `json:"interest"`
}

// GetInterest returns the caller's total accrued interest income for a currency.
func (c *Client) GetInterest(ctx context.Context, currency string) (types.Interest, error) {
	var info types.Interest
	if currency == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.GetInterest: currency is empty", nil)
	}
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.interestPath(currency),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p interestPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "earn.GetInterest: parse", err)
	}
	info.Currency = p.Currency
	if info.Currency == "" {
		info.Currency = currency
	}
	info.Interest = p.Interest.Decimal
	info.RateLimits = rateLimits
	return info, nil
}

// interestRecordPayload — Gate Uni interest-record wire shape.
type interestRecordPayload struct {
	Currency   string            `json:"currency"`
	Interest   codec.FlexDecimal `json:"interest"`
	Status     int64             `json:"status"`
	ActualRate codec.FlexDecimal `json:"actual_rate"`
	CreateTime int64             `json:"create_time"`
}

// ListInterestRecords returns the caller's interest-payout history. Pass an
// empty currency for all currencies. page/limit ≤ 0 let Gate use its defaults.
func (c *Client) ListInterestRecords(ctx context.Context, currency string, page, limit int) ([]types.InterestRecord, error) {
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
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "earn.ListInterestRecords: parse", err)
	}
	var out []types.InterestRecord = make([]types.InterestRecord, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, types.InterestRecord{
			Currency:    payloads[i].Currency,
			Interest:    payloads[i].Interest.Decimal,
			Status:      payloads[i].Status,
			ActualRate:  payloads[i].ActualRate.Decimal,
			CreatedAtMs: secondsToMs(payloads[i].CreateTime),
			RateLimits:  rateLimits,
		})
	}
	return out, nil
}

// interestStatusPayload — Gate Uni interest-status wire shape.
type interestStatusPayload struct {
	Currency       string `json:"currency"`
	InterestStatus string `json:"interest_status"`
}

// GetInterestStatus returns the auto-reinvest (compounding) status for a
// currency.
func (c *Client) GetInterestStatus(ctx context.Context, currency string) (types.InterestStatusInfo, error) {
	var info types.InterestStatusInfo
	if currency == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.GetInterestStatus: currency is empty", nil)
	}
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.interestStatusPath(currency),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p interestStatusPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "earn.GetInterestStatus: parse", err)
	}
	info.Currency = p.Currency
	if info.Currency == "" {
		info.Currency = currency
	}
	info.InterestStatus = types.InterestStatus(p.InterestStatus)
	info.RateLimits = rateLimits
	return info, nil
}
