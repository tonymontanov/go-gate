/*
FILE: earn/market.go

DESCRIPTION:
Public market-data queries for the Gate Earn "Uni" section:
  - ListChart : GET /earn/uni/chart?currency=&from=&to=  (historical rate chart)
  - ListRate  : GET /earn/uni/rate?currency=             (current estimated rate)

Both are unsigned (public). The chart timestamp Gate sends in epoch SECONDS is
normalized to epoch milliseconds; values/rates use codec.FlexDecimal (Gate may
quote them as numbers or strings).
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

func (c *Client) chartPath() string { return c.basePath() + "/chart" }
func (c *Client) ratePath() string  { return c.basePath() + "/rate" }

// chartPointPayload — Gate Uni chart-point wire shape.
type chartPointPayload struct {
	Time  int64             `json:"time"`
	Value codec.FlexDecimal `json:"value"`
}

// ListChart returns the historical lending-rate chart for a currency. currency
// is required; from/to are epoch SECONDS (Gate's native unit) and may be 0 to
// let Gate use its default window.
func (c *Client) ListChart(ctx context.Context, currency string, from, to int64) ([]types.ChartPoint, error) {
	if currency == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.ListChart: currency is empty", nil)
	}
	var q = newQuery()
	q.Set("currency", currency)
	if from > 0 {
		q.Set("from", strconv.FormatInt(from, 10))
	}
	if to > 0 {
		q.Set("to", strconv.FormatInt(to, 10))
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.chartPath(),
		Query:  q,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []chartPointPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "earn.ListChart: parse", err)
	}
	var out []types.ChartPoint = make([]types.ChartPoint, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, types.ChartPoint{
			TimeMs:     secondsToMs(payloads[i].Time),
			Value:      payloads[i].Value.Decimal,
			RateLimits: rateLimits,
		})
	}
	return out, nil
}

// ratePayload — Gate Uni estimated-rate wire shape.
type ratePayload struct {
	Currency     string            `json:"currency"`
	EstimateRate codec.FlexDecimal `json:"estimate_rate"`
}

// ListRate returns the current estimated annualized lending rate for a currency.
// Public: no signature required.
func (c *Client) ListRate(ctx context.Context, currency string) (types.RatePoint, error) {
	var info types.RatePoint
	if currency == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.ListRate: currency is empty", nil)
	}
	var q = newQuery()
	q.Set("currency", currency)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.ratePath(),
		Query:  q,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return info, err
	}
	var p ratePayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "earn.ListRate: parse", err)
	}
	info.Currency = p.Currency
	if info.Currency == "" {
		info.Currency = currency
	}
	info.EstimateRate = p.EstimateRate.Decimal
	info.RateLimits = rateLimits
	return info, nil
}
