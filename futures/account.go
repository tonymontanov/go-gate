/*
FILE: futures/account.go

DESCRIPTION:
Account/position sub-client for the Gate Futures section. Implements:
  - GetPositions      : GET  /futures/{settle}/positions
  - GetPosition       : GET  /futures/{settle}/positions/{contract}
  - SetLeverage       : POST /futures/{settle}/positions/{contract}/leverage?leverage=
  - SetPositionMode   : POST /futures/{settle}/dual_mode?dual_mode=
  - ClosePosition     : market close via Trading().CreateOrder(close=true)

All endpoints are private (signed). Position size is signed on the wire (positive
long / negative short); the SDK exposes an absolute Size plus a derived Side.
*/

package futures

import (
	"context"
	"strconv"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/futures/types"
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
func (a *AccountClient) dualModePath() string { return a.c.basePath() + "/dual_mode" }

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
