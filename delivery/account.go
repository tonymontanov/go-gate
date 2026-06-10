/*
FILE: delivery/account.go

DESCRIPTION:
Account/position sub-client for the Gate Delivery section. Implements:
  - GetPositions      : GET  /delivery/{settle}/positions
  - GetPosition       : GET  /delivery/{settle}/positions/{contract}
  - SetLeverage       : POST /delivery/{settle}/positions/{contract}/leverage?leverage=
  - SetPositionMode   : POST /delivery/{settle}/dual_mode?dual_mode=
  - ClosePosition     : market close via Trading().CreateOrder(close=true)
  - GetSettlements    : GET  /delivery/{settle}/settlements (delivery-specific:
                        records of expired-contract settlements)

All endpoints are private (signed). Position size is signed on the wire (positive
long / negative short); the SDK exposes an absolute Size plus a derived Side.
*/

package delivery

import (
	"context"
	"strconv"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/delivery/types"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
)

// AccountClient — account/position sub-client.
type AccountClient struct {
	c *Client
}

func newAccountClient(c *Client) *AccountClient {
	return &AccountClient{c: c}
}

func (a *AccountClient) positionsPath() string { return a.c.basePath() + "/positions" }
func (a *AccountClient) positionPath(contract string) string {
	return a.c.basePath() + "/positions/" + contract
}
func (a *AccountClient) leveragePath(contract string) string {
	return a.c.basePath() + "/positions/" + contract + "/leverage"
}
func (a *AccountClient) dualModePath() string    { return a.c.basePath() + "/dual_mode" }
func (a *AccountClient) settlementsPath() string { return a.c.basePath() + "/settlements" }

// positionPayload — Gate Position wire shape (the fields the SDK consumes).
type positionPayload struct {
	Contract           string `json:"contract"`
	Size               int64  `json:"size"`
	Leverage           string `json:"leverage"`
	CrossLeverageLimit string `json:"cross_leverage_limit"`
	MaintenanceRate    string `json:"maintenance_rate"`
	Value              string `json:"value"`
	Margin             string `json:"margin"`
	EntryPrice         string `json:"entry_price"`
	LiqPrice           string `json:"liq_price"`
	MarkPrice          string `json:"mark_price"`
	UnrealisedPnl      string `json:"unrealised_pnl"`
	RealisedPnl        string `json:"realised_pnl"`
	Mode               string `json:"mode"`
	UpdateTime         int64  `json:"update_time"`
}

func positionInfoFromPayload(p *positionPayload, rateLimits map[string]string) types.PositionInfo {
	return types.PositionInfo{
		Contract:           p.Contract,
		Side:               sideFromSize(p.Size),
		Size:               decimalAbsInt(p.Size),
		EntryPrice:         mustDecimal(p.EntryPrice),
		MarkPrice:          mustDecimal(p.MarkPrice),
		LiqPrice:           mustDecimal(p.LiqPrice),
		Leverage:           mustDecimal(p.Leverage),
		CrossLeverageLimit: mustDecimal(p.CrossLeverageLimit),
		Margin:             mustDecimal(p.Margin),
		Value:              mustDecimal(p.Value),
		UnrealisedPnl:      mustDecimal(p.UnrealisedPnl),
		RealisedPnl:        mustDecimal(p.RealisedPnl),
		MaintenanceRate:    mustDecimal(p.MaintenanceRate),
		Mode:               p.Mode,
		UpdatedAtMs:        p.UpdateTime * 1000,
		RateLimits:         rateLimits,
	}
}

// GetPositions returns all positions held by the account on this settle currency.
func (a *AccountClient) GetPositions(ctx context.Context) ([]types.PositionInfo, error) {
	var q = newQuery()
	q.Set("holding", "true")

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = a.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   a.positionsPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []positionPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "account.GetPositions: parse", err)
	}
	var out []types.PositionInfo = make([]types.PositionInfo, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, positionInfoFromPayload(&payloads[i], rateLimits))
	}
	return out, nil
}

// GetPosition returns the position for a single contract.
func (a *AccountClient) GetPosition(ctx context.Context, contract string) (types.PositionInfo, error) {
	var info types.PositionInfo
	if contract == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "account.GetPosition: contract is empty", nil)
	}
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = a.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   a.positionPath(contract),
		Signed: true,
		Meta:   rest.RequestMeta{Symbols: []string{contract}, Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p positionPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "account.GetPosition: parse", err)
	}
	return positionInfoFromPayload(&p, rateLimits), nil
}

// SetLeverage sets the leverage for a contract. leverage=0 selects cross margin.
func (a *AccountClient) SetLeverage(ctx context.Context, contract string, leverage int64) error {
	if contract == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "account.SetLeverage: contract is empty", nil)
	}
	if leverage < 0 {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "account.SetLeverage: leverage must be >= 0", nil)
	}
	var q = newQuery()
	q.Set("leverage", strconv.FormatInt(leverage, 10))

	var err error
	_, _, err = a.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   a.leveragePath(contract),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Symbols: []string{contract}, Category: string(gate.RateLimitCategoryQuery)},
	})
	return err
}

// SetPositionMode switches between one-way (single) and dual position mode for
// the whole futures account. oneWayMode=true → dual_mode=false.
func (a *AccountClient) SetPositionMode(ctx context.Context, oneWayMode bool) error {
	var q = newQuery()
	q.Set("dual_mode", strconv.FormatBool(!oneWayMode))

	var err error
	_, _, err = a.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   a.dualModePath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	return err
}

// ClosePosition market-closes the position on a contract. Gate closes a position
// with a close-order (size=0, close=true); a market order is price="0" + tif=ioc.
func (a *AccountClient) ClosePosition(ctx context.Context, contract string) error {
	if contract == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "account.ClosePosition: contract is empty", nil)
	}
	var _, err = a.c.Trading().CreateOrder(ctx, types.CreateOrderRequest{
		Contract:  contract,
		Close:     true,
		OrderType: types.OrderTypeMarket,
	})
	return err
}

// settlementPayload — Gate delivery settlement wire shape. CALIBRATION: confirm
// the exact field set/keys live (modeled on Gate's /delivery/{settle}/settlements).
type settlementPayload struct {
	Time        int64  `json:"time"`
	Contract    string `json:"contract"`
	Size        int64  `json:"size"`
	Leverage    string `json:"leverage"`
	Margin      string `json:"margin"`
	Profit      string `json:"profit"`
	Pnl         string `json:"pnl"`
	Fee         string `json:"fee"`
	SettlePrice string `json:"settle_price"`
}

func settlementFromPayload(p *settlementPayload, rateLimits map[string]string) types.Settlement {
	return types.Settlement{
		TimeMs:      p.Time * 1000,
		Contract:    p.Contract,
		Size:        decimalAbsInt(p.Size),
		Side:        sideFromSize(p.Size),
		Leverage:    mustDecimal(p.Leverage),
		Margin:      mustDecimal(p.Margin),
		Profit:      mustDecimal(p.Profit),
		Pnl:         mustDecimal(p.Pnl),
		Fee:         mustDecimal(p.Fee),
		SettlePrice: mustDecimal(p.SettlePrice),
		RateLimits:  rateLimits,
	}
}

// GetSettlements returns settlement records for expired delivery contracts. Pass
// an empty contract for all; limit<=0 lets Gate use its default page size. This
// endpoint is delivery-specific (perpetual futures never settle).
func (a *AccountClient) GetSettlements(ctx context.Context, contract string, limit int) ([]types.Settlement, error) {
	var q = newQuery()
	var symbols []string
	if contract != "" {
		q.Set("contract", contract)
		symbols = []string{contract}
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = a.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   a.settlementsPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Symbols: symbols, Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []settlementPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "account.GetSettlements: parse", err)
	}
	var out []types.Settlement = make([]types.Settlement, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, settlementFromPayload(&payloads[i], rateLimits))
	}
	return out, nil
}
