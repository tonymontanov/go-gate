/*
FILE: unified/account.go

DESCRIPTION:
Account/quota/risk reads for the Gate Unified Account section. Implements
(all private, signed):
  - GetAccount        : GET /unified/accounts?currency=&sub_uid=  (single object)
  - GetBorrowable     : GET /unified/borrowable?currency=
  - BatchBorrowable   : GET /unified/batch_borrowable?currencies=  (comma-joined)
  - GetTransferable   : GET /unified/transferable?currency=
  - BatchTransferable : GET /unified/transferables?currencies=     (comma-joined)
  - GetRiskUnits      : GET /unified/risk_units

The UnifiedAccount snapshot's monetary fields use codec.FlexDecimal because Gate
quotes them inconsistently as JSON numbers or strings (notably across the
per-currency balances and the account-wide margins).
*/

package unified

import (
	"context"
	"strconv"
	"strings"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
	"github.com/tonymontanov/go-gate/v2/unified/types"
)

func (c *Client) accountPath() string         { return c.basePath() + "/accounts" }
func (c *Client) borrowablePath() string      { return c.basePath() + "/borrowable" }
func (c *Client) batchBorrowablePath() string { return c.basePath() + "/batch_borrowable" }
func (c *Client) transferablePath() string    { return c.basePath() + "/transferable" }
func (c *Client) transferablesPath() string   { return c.basePath() + "/transferables" }
func (c *Client) riskUnitsPath() string       { return c.basePath() + "/risk_units" }

// ---- account ---------------------------------------------------------------

// balancePayload — Gate per-currency unified balance wire shape. Decimal fields
// use codec.FlexDecimal (Gate quotes them as numbers or strings).
type balancePayload struct {
	Available        codec.FlexDecimal `json:"available"`
	Freeze           codec.FlexDecimal `json:"freeze"`
	Borrowed         codec.FlexDecimal `json:"borrowed"`
	NegativeLiab     codec.FlexDecimal `json:"negative_liab"`
	FuturesPosLiab   codec.FlexDecimal `json:"futures_pos_liab"`
	Equity           codec.FlexDecimal `json:"equity"`
	TotalFreeze      codec.FlexDecimal `json:"total_freeze"`
	TotalLiab        codec.FlexDecimal `json:"total_liab"`
	SpotInUse        codec.FlexDecimal `json:"spot_in_use"`
	Leverage         codec.FlexDecimal `json:"leverage"`
	FreezeFundingFee codec.FlexDecimal `json:"freeze_funding_fee"`
}

// accountPayload — Gate unified account wire shape. Decimal fields use
// codec.FlexDecimal (number-or-string).
type accountPayload struct {
	UserID                     int64                     `json:"user_id"`
	RefreshTime                int64                     `json:"refresh_time"`
	Locked                     bool                      `json:"locked"`
	Total                      codec.FlexDecimal         `json:"total"`
	Borrowed                   codec.FlexDecimal         `json:"borrowed"`
	Equity                     codec.FlexDecimal         `json:"equity"`
	TotalInitialMargin         codec.FlexDecimal         `json:"total_initial_margin"`
	TotalMarginBalance         codec.FlexDecimal         `json:"total_margin_balance"`
	TotalMaintenanceMargin     codec.FlexDecimal         `json:"total_maintenance_margin"`
	TotalInitialMarginRate     codec.FlexDecimal         `json:"total_initial_margin_rate"`
	TotalMaintenanceMarginRate codec.FlexDecimal         `json:"total_maintenance_margin_rate"`
	TotalAvailableMargin       codec.FlexDecimal         `json:"total_available_margin"`
	UnifiedAccountTotal        codec.FlexDecimal         `json:"unified_account_total"`
	UnifiedAccountTotalLiab    codec.FlexDecimal         `json:"unified_account_total_liab"`
	UnifiedAccountTotalEquity  codec.FlexDecimal         `json:"unified_account_total_equity"`
	Leverage                   codec.FlexDecimal         `json:"leverage"`
	SpotOrderLoss              codec.FlexDecimal         `json:"spot_order_loss"`
	Balances                   map[string]balancePayload `json:"balances"`
}

func accountFromPayload(p *accountPayload, rateLimits map[string]string) types.UnifiedAccount {
	var acc types.UnifiedAccount = types.UnifiedAccount{
		UserID:                     p.UserID,
		RefreshTimeMs:              secondsToMs(p.RefreshTime),
		Locked:                     p.Locked,
		Total:                      p.Total.Decimal,
		Borrowed:                   p.Borrowed.Decimal,
		Equity:                     p.Equity.Decimal,
		TotalInitialMargin:         p.TotalInitialMargin.Decimal,
		TotalMarginBalance:         p.TotalMarginBalance.Decimal,
		TotalMaintenanceMargin:     p.TotalMaintenanceMargin.Decimal,
		TotalInitialMarginRate:     p.TotalInitialMarginRate.Decimal,
		TotalMaintenanceMarginRate: p.TotalMaintenanceMarginRate.Decimal,
		TotalAvailableMargin:       p.TotalAvailableMargin.Decimal,
		UnifiedAccountTotal:        p.UnifiedAccountTotal.Decimal,
		UnifiedAccountTotalLiab:    p.UnifiedAccountTotalLiab.Decimal,
		UnifiedAccountTotalEquity:  p.UnifiedAccountTotalEquity.Decimal,
		Leverage:                   p.Leverage.Decimal,
		SpotOrderLoss:              p.SpotOrderLoss.Decimal,
		RateLimits:                 rateLimits,
	}
	if len(p.Balances) > 0 {
		acc.Balances = make(map[string]types.Balance, len(p.Balances))
		var ccy string
		var b balancePayload
		for ccy, b = range p.Balances {
			acc.Balances[ccy] = types.Balance{
				Available:        b.Available.Decimal,
				Freeze:           b.Freeze.Decimal,
				Borrowed:         b.Borrowed.Decimal,
				NegativeLiab:     b.NegativeLiab.Decimal,
				FuturesPosLiab:   b.FuturesPosLiab.Decimal,
				Equity:           b.Equity.Decimal,
				TotalFreeze:      b.TotalFreeze.Decimal,
				TotalLiab:        b.TotalLiab.Decimal,
				SpotInUse:        b.SpotInUse.Decimal,
				Leverage:         b.Leverage.Decimal,
				FreezeFundingFee: b.FreezeFundingFee.Decimal,
			}
		}
	}
	return acc
}

// GetAccount returns the unified account snapshot. currency narrows the balances
// to a single currency (empty = all); subUID reads a sub-account (0 = self).
func (c *Client) GetAccount(ctx context.Context, currency string, subUID int64) (types.UnifiedAccount, error) {
	var acc types.UnifiedAccount
	var q = newQuery()
	if currency != "" {
		q.Set("currency", currency)
	}
	if subUID > 0 {
		q.Set("sub_uid", strconv.FormatInt(subUID, 10))
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.accountPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return acc, err
	}
	var p accountPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return acc, gate.NewError(gate.ErrorKindUnknown, "", "unified.GetAccount: parse", err)
	}
	return accountFromPayload(&p, rateLimits), nil
}

// ---- borrowable / transferable ---------------------------------------------

type quotaPayload struct {
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
}

// GetBorrowable returns the maximum borrowable amount for a currency.
func (c *Client) GetBorrowable(ctx context.Context, currency string) (types.Borrowable, error) {
	var out types.Borrowable
	if currency == "" {
		return out, gate.NewError(gate.ErrorKindInvalidRequest, "", "unified.GetBorrowable: currency is empty", nil)
	}
	var q = newQuery()
	q.Set("currency", currency)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.borrowablePath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return out, err
	}
	var p quotaPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return out, gate.NewError(gate.ErrorKindUnknown, "", "unified.GetBorrowable: parse", err)
	}
	return types.Borrowable{
		Currency:   p.Currency,
		Amount:     mustDecimal(p.Amount),
		RateLimits: rateLimits,
	}, nil
}

// BatchBorrowable returns the borrowable amounts for several currencies in one
// call (Gate comma-joins the currencies query parameter).
func (c *Client) BatchBorrowable(ctx context.Context, currencies []string) ([]types.Borrowable, error) {
	if len(currencies) == 0 {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "unified.BatchBorrowable: currencies is empty", nil)
	}
	var q = newQuery()
	q.Set("currencies", strings.Join(currencies, ","))

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.batchBorrowablePath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []quotaPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "unified.BatchBorrowable: parse", err)
	}
	var out []types.Borrowable = make([]types.Borrowable, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, types.Borrowable{
			Currency:   payloads[i].Currency,
			Amount:     mustDecimal(payloads[i].Amount),
			RateLimits: rateLimits,
		})
	}
	return out, nil
}

// GetTransferable returns the maximum transferable-out amount for a currency.
func (c *Client) GetTransferable(ctx context.Context, currency string) (types.Transferable, error) {
	var out types.Transferable
	if currency == "" {
		return out, gate.NewError(gate.ErrorKindInvalidRequest, "", "unified.GetTransferable: currency is empty", nil)
	}
	var q = newQuery()
	q.Set("currency", currency)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.transferablePath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return out, err
	}
	var p quotaPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return out, gate.NewError(gate.ErrorKindUnknown, "", "unified.GetTransferable: parse", err)
	}
	return types.Transferable{
		Currency:   p.Currency,
		Amount:     mustDecimal(p.Amount),
		RateLimits: rateLimits,
	}, nil
}

// BatchTransferable returns the transferable-out amounts for several currencies
// in one call (Gate comma-joins the currencies query parameter).
func (c *Client) BatchTransferable(ctx context.Context, currencies []string) ([]types.Transferable, error) {
	if len(currencies) == 0 {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "unified.BatchTransferable: currencies is empty", nil)
	}
	var q = newQuery()
	q.Set("currencies", strings.Join(currencies, ","))

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.transferablesPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []quotaPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "unified.BatchTransferable: parse", err)
	}
	var out []types.Transferable = make([]types.Transferable, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, types.Transferable{
			Currency:   payloads[i].Currency,
			Amount:     mustDecimal(payloads[i].Amount),
			RateLimits: rateLimits,
		})
	}
	return out, nil
}

// ---- risk units ------------------------------------------------------------

type riskUnitEntryPayload struct {
	Symbol         string            `json:"symbol"`
	SpotInUse      codec.FlexDecimal `json:"spot_in_use"`
	MaintainMargin codec.FlexDecimal `json:"maintain_margin"`
	InitialMargin  codec.FlexDecimal `json:"initial_margin"`
	Delta          codec.FlexDecimal `json:"delta"`
	Gamma          codec.FlexDecimal `json:"gamma"`
	Theta          codec.FlexDecimal `json:"theta"`
	Vega           codec.FlexDecimal `json:"vega"`
}

type riskUnitsPayload struct {
	UserID    int64                  `json:"user_id"`
	SpotHedge bool                   `json:"spot_hedge"`
	RiskUnits []riskUnitEntryPayload `json:"risk_units"`
}

func riskUnitEntryFromPayload(p *riskUnitEntryPayload) types.RiskUnitEntry {
	return types.RiskUnitEntry{
		Symbol:         p.Symbol,
		SpotInUse:      p.SpotInUse.Decimal,
		MaintainMargin: p.MaintainMargin.Decimal,
		InitialMargin:  p.InitialMargin.Decimal,
		Delta:          p.Delta.Decimal,
		Gamma:          p.Gamma.Decimal,
		Theta:          p.Theta.Decimal,
		Vega:           p.Vega.Decimal,
	}
}

// GetRiskUnits returns the account's risk-unit breakdown.
func (c *Client) GetRiskUnits(ctx context.Context) (types.RiskUnit, error) {
	var out types.RiskUnit
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.riskUnitsPath(),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return out, err
	}
	var p riskUnitsPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return out, gate.NewError(gate.ErrorKindUnknown, "", "unified.GetRiskUnits: parse", err)
	}
	out = types.RiskUnit{
		UserID:     p.UserID,
		SpotHedge:  p.SpotHedge,
		RateLimits: rateLimits,
	}
	if len(p.RiskUnits) > 0 {
		out.Units = make([]types.RiskUnitEntry, 0, len(p.RiskUnits))
		var i int
		for i = 0; i < len(p.RiskUnits); i++ {
			out.Units = append(out.Units, riskUnitEntryFromPayload(&p.RiskUnits[i]))
		}
	}
	return out, nil
}
