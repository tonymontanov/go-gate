/*
FILE: earn/currencies.go

DESCRIPTION:
Public lendable-currency discovery for the Gate Earn "Uni" section:
  - ListCurrencies : GET /earn/uni/currencies            (public)
  - GetCurrency    : GET /earn/uni/currencies/{currency}  (public)

Both are unsigned (public). Amounts/rates use codec.FlexDecimal on the wire
because Gate may quote them as JSON numbers or strings.
*/

package earn

import (
	"context"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/earn/types"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
)

func (c *Client) currenciesPath() string { return c.basePath() + "/currencies" }
func (c *Client) currencyPath(currency string) string {
	return c.basePath() + "/currencies/" + currency
}

// uniCurrencyPayload — Gate Uni currency wire shape. Decimal fields use
// codec.FlexDecimal (Gate may quote amounts/rates as numbers or strings).
type uniCurrencyPayload struct {
	Currency           string            `json:"currency"`
	MinLendAmount      codec.FlexDecimal `json:"min_lend_amount"`
	MaxLendAmount      codec.FlexDecimal `json:"max_lend_amount"`
	Available          codec.FlexDecimal `json:"available"`
	TotalLendAvailable codec.FlexDecimal `json:"total_lend_available"`
	MinRate            codec.FlexDecimal `json:"min_rate"`
	MaxRate            codec.FlexDecimal `json:"max_rate"`
}

func uniCurrencyFromPayload(p *uniCurrencyPayload, rateLimits map[string]string) types.UniCurrency {
	return types.UniCurrency{
		Currency:           p.Currency,
		MinLendAmount:      p.MinLendAmount.Decimal,
		MaxLendAmount:      p.MaxLendAmount.Decimal,
		Available:          p.Available.Decimal,
		TotalLendAvailable: p.TotalLendAvailable.Decimal,
		MinRate:            p.MinRate.Decimal,
		MaxRate:            p.MaxRate.Decimal,
		RateLimits:         rateLimits,
	}
}

// ListCurrencies returns every currency the Uni flexible-lending pool accepts.
// Public: no signature required.
func (c *Client) ListCurrencies(ctx context.Context) ([]types.UniCurrency, error) {
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
	var payloads []uniCurrencyPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "earn.ListCurrencies: parse", err)
	}
	var out []types.UniCurrency = make([]types.UniCurrency, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, uniCurrencyFromPayload(&payloads[i], rateLimits))
	}
	return out, nil
}

// GetCurrency returns the Uni-pool parameters for a single currency. Public: no
// signature required.
func (c *Client) GetCurrency(ctx context.Context, currency string) (types.UniCurrency, error) {
	var info types.UniCurrency
	if currency == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "earn.GetCurrency: currency is empty", nil)
	}
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.currencyPath(currency),
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return info, err
	}
	var p uniCurrencyPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "earn.GetCurrency: parse", err)
	}
	return uniCurrencyFromPayload(&p, rateLimits), nil
}
