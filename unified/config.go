/*
FILE: unified/config.go

DESCRIPTION:
Configuration / market / margin endpoints for the Gate Unified Account section:
  - ListCurrencies            : GET  /unified/currencies                 (public)
  - GetMode                   : GET  /unified/unified_mode
  - SetMode                   : PUT  /unified/unified_mode
  - CalculatePortfolioMargin  : POST /unified/portfolio_calculator
  - SetCollateralCurrencies   : POST /unified/collateral_currencies
  - ListCurrencyDiscountTiers : GET  /unified/currency_discount_tiers    (public)
  - ListLoanMarginTiers       : GET  /unified/loan_margin_tiers
  - GetLeverageConfig         : GET  /unified/leverage/user_currency_config?currency=
  - GetLeverageSetting        : GET  /unified/leverage/user_currency_setting?currency=
  - SetLeverageSetting        : POST /unified/leverage/user_currency_setting

Only ListCurrencies and ListCurrencyDiscountTiers are public (unsigned); the
rest are signed.
*/

package unified

import (
	"context"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
	"github.com/tonymontanov/go-gate/v2/unified/types"
)

func (c *Client) currenciesPath() string      { return c.basePath() + "/currencies" }
func (c *Client) modePath() string            { return c.basePath() + "/unified_mode" }
func (c *Client) portfolioCalcPath() string   { return c.basePath() + "/portfolio_calculator" }
func (c *Client) collateralPath() string      { return c.basePath() + "/collateral_currencies" }
func (c *Client) discountTiersPath() string   { return c.basePath() + "/currency_discount_tiers" }
func (c *Client) loanMarginTiersPath() string { return c.basePath() + "/loan_margin_tiers" }
func (c *Client) leverageConfigPath() string  { return c.basePath() + "/leverage/user_currency_config" }
func (c *Client) leverageSettingPath() string {
	return c.basePath() + "/leverage/user_currency_setting"
}

// ---- currencies (public) ---------------------------------------------------

type currencyPayload struct {
	Name                 string `json:"name"`
	PrecMode             int64  `json:"prec_mode"`
	MinBorrowAmount      string `json:"min_borrow_amount"`
	UserMaxBorrowAmount  string `json:"user_max_borrow_amount"`
	TotalMaxBorrowAmount string `json:"total_max_borrow_amount"`
	PriceChange30d       string `json:"price_change_30d"`
	Discount             string `json:"discount"`
	LoanStatus           string `json:"loan_status"`
}

// ListCurrencies returns the public list of currencies borrowable in the unified
// account, with their borrow limits and loan status.
func (c *Client) ListCurrencies(ctx context.Context) ([]types.UnifiedCurrency, error) {
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
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "unified.ListCurrencies: parse", err)
	}
	var out []types.UnifiedCurrency = make([]types.UnifiedCurrency, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, types.UnifiedCurrency{
			Name:                 payloads[i].Name,
			PrecMode:             payloads[i].PrecMode,
			MinBorrowAmount:      mustDecimal(payloads[i].MinBorrowAmount),
			UserMaxBorrowAmount:  mustDecimal(payloads[i].UserMaxBorrowAmount),
			TotalMaxBorrowAmount: mustDecimal(payloads[i].TotalMaxBorrowAmount),
			PriceChange30d:       mustDecimal(payloads[i].PriceChange30d),
			Discount:             mustDecimal(payloads[i].Discount),
			LoanStatus:           payloads[i].LoanStatus,
			RateLimits:           rateLimits,
		})
	}
	return out, nil
}

// ---- account mode ----------------------------------------------------------

type modePayload struct {
	Mode     string          `json:"mode"`
	Settings map[string]bool `json:"settings"`
}

// GetMode returns the account's current margin mode and feature settings.
func (c *Client) GetMode(ctx context.Context) (types.UnifiedMode, error) {
	var out types.UnifiedMode
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.modePath(),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return out, err
	}
	var p modePayload
	if err = resp.UnmarshalData(&p); err != nil {
		return out, gate.NewError(gate.ErrorKindUnknown, "", "unified.GetMode: parse", err)
	}
	return types.UnifiedMode{
		Mode:       types.AccountMode(p.Mode),
		Settings:   p.Settings,
		RateLimits: rateLimits,
	}, nil
}

// SetMode switches the account's margin mode (and optional feature settings).
func (c *Client) SetMode(ctx context.Context, req types.SetModeRequest) error {
	if req.Mode == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "unified.SetMode: mode is empty", nil)
	}
	var body map[string]any = map[string]any{
		"mode": string(req.Mode),
	}
	if len(req.Settings) > 0 {
		body["settings"] = req.Settings
	}

	var err error
	_, _, err = c.rest().Do(ctx, rest.Options{
		Method: "PUT",
		Path:   c.modePath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	return err
}

// ---- portfolio calculator --------------------------------------------------

type portfolioResultPayload struct {
	MaintainMarginTotal codec.FlexDecimal      `json:"maintain_margin_total"`
	InitialMarginTotal  codec.FlexDecimal      `json:"initial_margin_total"`
	CalculateTime       int64                  `json:"calculate_time"`
	RiskUnit            []riskUnitEntryPayload `json:"risk_unit"`
}

// CalculatePortfolioMargin runs Gate's portfolio-margin calculator over a set of
// hypothetical balances/positions/orders and returns the resulting margins. Empty
// legs are omitted from the request body.
func (c *Client) CalculatePortfolioMargin(ctx context.Context, req types.PortfolioCalcRequest) (types.PortfolioMarginResult, error) {
	var out types.PortfolioMarginResult
	var body map[string]any = map[string]any{}
	if len(req.SpotBalances) > 0 {
		body["spot_balances"] = req.SpotBalances
	}
	if len(req.SpotOrders) > 0 {
		body["spot_orders"] = req.SpotOrders
	}
	if len(req.FuturesPositions) > 0 {
		body["futures_positions"] = req.FuturesPositions
	}
	if len(req.FuturesOrders) > 0 {
		body["futures_orders"] = req.FuturesOrders
	}
	if len(req.OptionsPositions) > 0 {
		body["options_positions"] = req.OptionsPositions
	}
	if len(req.OptionsOrders) > 0 {
		body["options_orders"] = req.OptionsOrders
	}
	if req.SpotHedge {
		body["spot_hedge"] = true
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   c.portfolioCalcPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return out, err
	}
	var p portfolioResultPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return out, gate.NewError(gate.ErrorKindUnknown, "", "unified.CalculatePortfolioMargin: parse", err)
	}
	out = types.PortfolioMarginResult{
		MaintainMarginTotal: p.MaintainMarginTotal.Decimal,
		InitialMarginTotal:  p.InitialMarginTotal.Decimal,
		CalculateTimeMs:     secondsToMs(p.CalculateTime),
		RateLimits:          rateLimits,
	}
	if len(p.RiskUnit) > 0 {
		out.Units = make([]types.RiskUnitEntry, 0, len(p.RiskUnit))
		var i int
		for i = 0; i < len(p.RiskUnit); i++ {
			out.Units = append(out.Units, riskUnitEntryFromPayload(&p.RiskUnit[i]))
		}
	}
	return out, nil
}

// ---- collateral currencies -------------------------------------------------

// SetCollateralCurrencies sets the currencies enabled as collateral for the
// unified account. The currencies slice is sent as Gate's "currencies" body
// field.
//
// CALIBRATION: the exact body shape is modeled on Gate's docs; verify live.
func (c *Client) SetCollateralCurrencies(ctx context.Context, currencies []string) error {
	if len(currencies) == 0 {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "unified.SetCollateralCurrencies: currencies is empty", nil)
	}
	var body map[string]any = map[string]any{
		"currencies": currencies,
	}
	var err error
	_, _, err = c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   c.collateralPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	return err
}

// ---- discount / loan-margin tiers ------------------------------------------

type tierBandPayload struct {
	Tier       string `json:"tier"`
	Discount   string `json:"discount"`
	MarginRate string `json:"margin_rate"`
	LowerLimit string `json:"lower_limit"`
	UpperLimit string `json:"upper_limit"`
}

type discountTierPayload struct {
	Currency      string            `json:"currency"`
	DiscountTiers []tierBandPayload `json:"discount_tiers"`
}

type loanMarginTierPayload struct {
	Currency    string            `json:"currency"`
	MarginTiers []tierBandPayload `json:"margin_tiers"`
}

// ListCurrencyDiscountTiers returns the public per-currency collateral discount
// tiers.
func (c *Client) ListCurrencyDiscountTiers(ctx context.Context) ([]types.DiscountTier, error) {
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.discountTiersPath(),
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []discountTierPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "unified.ListCurrencyDiscountTiers: parse", err)
	}
	var out []types.DiscountTier = make([]types.DiscountTier, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		var dt types.DiscountTier = types.DiscountTier{
			Currency:   payloads[i].Currency,
			RateLimits: rateLimits,
		}
		var j int
		for j = 0; j < len(payloads[i].DiscountTiers); j++ {
			dt.Tiers = append(dt.Tiers, types.DiscountTierEntry{
				Tier:       payloads[i].DiscountTiers[j].Tier,
				Discount:   mustDecimal(payloads[i].DiscountTiers[j].Discount),
				LowerLimit: mustDecimal(payloads[i].DiscountTiers[j].LowerLimit),
				UpperLimit: mustDecimal(payloads[i].DiscountTiers[j].UpperLimit),
			})
		}
		out = append(out, dt)
	}
	return out, nil
}

// ListLoanMarginTiers returns the per-currency loan-margin tiers.
func (c *Client) ListLoanMarginTiers(ctx context.Context) ([]types.LoanMarginTier, error) {
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.loanMarginTiersPath(),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []loanMarginTierPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "unified.ListLoanMarginTiers: parse", err)
	}
	var out []types.LoanMarginTier = make([]types.LoanMarginTier, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		var lt types.LoanMarginTier = types.LoanMarginTier{
			Currency:   payloads[i].Currency,
			RateLimits: rateLimits,
		}
		var j int
		for j = 0; j < len(payloads[i].MarginTiers); j++ {
			lt.Tiers = append(lt.Tiers, types.LoanMarginTierEntry{
				Tier:       payloads[i].MarginTiers[j].Tier,
				MarginRate: mustDecimal(payloads[i].MarginTiers[j].MarginRate),
				LowerLimit: mustDecimal(payloads[i].MarginTiers[j].LowerLimit),
				UpperLimit: mustDecimal(payloads[i].MarginTiers[j].UpperLimit),
			})
		}
		out = append(out, lt)
	}
	return out, nil
}

// ---- leverage --------------------------------------------------------------

type leverageConfigPayload struct {
	Currency        string `json:"currency"`
	MinLeverage     string `json:"min_leverage"`
	MaxLeverage     string `json:"max_leverage"`
	CurrentLeverage string `json:"current_leverage"`
}

type leverageSettingPayload struct {
	Currency string `json:"currency"`
	Leverage string `json:"leverage"`
}

// GetLeverageConfig returns the allowed leverage range for a currency.
func (c *Client) GetLeverageConfig(ctx context.Context, currency string) (types.LeverageConfig, error) {
	var out types.LeverageConfig
	if currency == "" {
		return out, gate.NewError(gate.ErrorKindInvalidRequest, "", "unified.GetLeverageConfig: currency is empty", nil)
	}
	var q = newQuery()
	q.Set("currency", currency)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.leverageConfigPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return out, err
	}
	var p leverageConfigPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return out, gate.NewError(gate.ErrorKindUnknown, "", "unified.GetLeverageConfig: parse", err)
	}
	out = types.LeverageConfig{
		Currency:        p.Currency,
		MinLeverage:     mustDecimal(p.MinLeverage),
		MaxLeverage:     mustDecimal(p.MaxLeverage),
		CurrentLeverage: mustDecimal(p.CurrentLeverage),
		RateLimits:      rateLimits,
	}
	if out.Currency == "" {
		out.Currency = currency
	}
	return out, nil
}

// GetLeverageSetting returns the currently configured leverage for a currency.
func (c *Client) GetLeverageSetting(ctx context.Context, currency string) (types.LeverageSetting, error) {
	var out types.LeverageSetting
	if currency == "" {
		return out, gate.NewError(gate.ErrorKindInvalidRequest, "", "unified.GetLeverageSetting: currency is empty", nil)
	}
	var q = newQuery()
	q.Set("currency", currency)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.leverageSettingPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return out, err
	}
	var p leverageSettingPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return out, gate.NewError(gate.ErrorKindUnknown, "", "unified.GetLeverageSetting: parse", err)
	}
	out = types.LeverageSetting{
		Currency:   p.Currency,
		Leverage:   mustDecimal(p.Leverage),
		RateLimits: rateLimits,
	}
	if out.Currency == "" {
		out.Currency = currency
	}
	return out, nil
}

// SetLeverageSetting configures the leverage for a currency.
func (c *Client) SetLeverageSetting(ctx context.Context, req types.SetLeverageRequest) error {
	if req.Currency == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "unified.SetLeverageSetting: currency is empty", nil)
	}
	var body map[string]any = map[string]any{
		"currency": req.Currency,
		"leverage": req.Leverage.String(),
	}
	var err error
	_, _, err = c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   c.leverageSettingPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	return err
}
