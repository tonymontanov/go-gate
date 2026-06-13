/*
FILE: options/account.go

DESCRIPTION:
Account/position sub-client for the Gate Options section. Implements (all private,
signed):
  - GetAccount       : GET /options/accounts        (single account object)
  - GetAccountBook   : GET /options/account_book     (balance-change history)
  - GetPositions     : GET /options/positions?underlying=
  - GetPosition      : GET /options/positions/{contract}
  - GetPositionClose : GET /options/position_close?underlying=
  - GetMySettlements : GET /options/my_settlements

Position size is signed on the wire (positive long / negative short); the SDK
exposes an absolute Size plus a derived Side. Position/account decimal fields use
codec.FlexDecimal because the SAME payload struct decodes BOTH the REST string
form and the options.positions WS number form.
*/

package options

import (
	"context"
	"errors"
	"strconv"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
	"github.com/tonymontanov/go-gate/v2/options/types"
)

// AccountClient — account/position sub-client.
type AccountClient struct {
	c *Client
}

func newAccountClient(c *Client) *AccountClient {
	return &AccountClient{c: c}
}

// labelPositionNotFound is the Gate error label (HTTP 400) for a FLAT contract;
// GetPosition surfaces it as a zero position rather than an error (mirrors the
// futures/delivery sections).
const labelPositionNotFound = "POSITION_NOT_FOUND"

func (a *AccountClient) accountPath() string       { return a.c.basePath() + "/accounts" }
func (a *AccountClient) accountBookPath() string   { return a.c.basePath() + "/account_book" }
func (a *AccountClient) positionsPath() string     { return a.c.basePath() + "/positions" }
func (a *AccountClient) positionClosePath() string { return a.c.basePath() + "/position_close" }
func (a *AccountClient) mySettlementsPath() string { return a.c.basePath() + "/my_settlements" }
func (a *AccountClient) positionPath(contract string) string {
	return a.c.basePath() + "/positions/" + contract
}

// ---- account ---------------------------------------------------------------

// accountPayload — Gate options account wire shape. Decimal fields use
// codec.FlexDecimal (Gate quotes them as strings over REST, numbers over WS).
type accountPayload struct {
	User          int64             `json:"user"`
	Currency      string            `json:"currency"`
	Total         codec.FlexDecimal `json:"total"`
	PositionValue codec.FlexDecimal `json:"position_value"`
	Equity        codec.FlexDecimal `json:"equity"`
	UnrealisedPnl codec.FlexDecimal `json:"unrealised_pnl"`
	InitMargin    codec.FlexDecimal `json:"init_margin"`
	MaintMargin   codec.FlexDecimal `json:"maint_margin"`
	OrderMargin   codec.FlexDecimal `json:"order_margin"`
	Available     codec.FlexDecimal `json:"available"`
	Bonus         codec.FlexDecimal `json:"bonus"`
}

func accountInfoFromPayload(p *accountPayload, rateLimits map[string]string) types.AccountInfo {
	return types.AccountInfo{
		User:          p.User,
		Currency:      p.Currency,
		Total:         p.Total.Decimal,
		PositionValue: p.PositionValue.Decimal,
		Equity:        p.Equity.Decimal,
		UnrealisedPnl: p.UnrealisedPnl.Decimal,
		InitMargin:    p.InitMargin.Decimal,
		MaintMargin:   p.MaintMargin.Decimal,
		OrderMargin:   p.OrderMargin.Decimal,
		Available:     p.Available.Decimal,
		Bonus:         p.Bonus.Decimal,
		RateLimits:    rateLimits,
	}
}

// GetAccount returns the single options account object for the credentials.
func (a *AccountClient) GetAccount(ctx context.Context) (types.AccountInfo, error) {
	var info types.AccountInfo
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = a.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   a.accountPath(),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p accountPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "account.GetAccount: parse", err)
	}
	return accountInfoFromPayload(&p, rateLimits), nil
}

// ---- account book ----------------------------------------------------------

type accountBookPayload struct {
	Time    int64  `json:"time"`
	Change  string `json:"change"`
	Balance string `json:"balance"`
	Type    string `json:"type"`
	Text    string `json:"text"`
}

// GetAccountBook returns the account-changing history. limit ≤ 0 lets Gate use
// its default page size.
func (a *AccountClient) GetAccountBook(ctx context.Context, limit int) ([]types.AccountBookEntry, error) {
	var q = newQuery()
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = a.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   a.accountBookPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []accountBookPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "account.GetAccountBook: parse", err)
	}
	var out []types.AccountBookEntry = make([]types.AccountBookEntry, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, types.AccountBookEntry{
			TimeMs:     secondsToMs(payloads[i].Time),
			Change:     mustDecimal(payloads[i].Change),
			Balance:    mustDecimal(payloads[i].Balance),
			Type:       payloads[i].Type,
			Text:       payloads[i].Text,
			RateLimits: rateLimits,
		})
	}
	return out, nil
}

// ---- positions -------------------------------------------------------------

// positionPayload — Gate options Position wire shape (the fields the SDK
// consumes). Decimal fields use codec.FlexDecimal because this single struct is
// shared by the REST account client (quoted strings) and the WebSocket
// options.positions push (bare JSON numbers).
type positionPayload struct {
	Contract      string            `json:"contract"`
	Size          int64             `json:"size"`
	EntryPrice    codec.FlexDecimal `json:"entry_price"`
	MarkPrice     codec.FlexDecimal `json:"mark_price"`
	MarkIv        codec.FlexDecimal `json:"mark_iv"`
	RealisedPnl   codec.FlexDecimal `json:"realised_pnl"`
	UnrealisedPnl codec.FlexDecimal `json:"unrealised_pnl"`
	Delta         codec.FlexDecimal `json:"delta"`
	Gamma         codec.FlexDecimal `json:"gamma"`
	Vega          codec.FlexDecimal `json:"vega"`
	Theta         codec.FlexDecimal `json:"theta"`
	PendingOrders int64             `json:"pending_orders"`
	UpdateTime    int64             `json:"update_time"`
}

func positionInfoFromPayload(p *positionPayload, rateLimits map[string]string) types.PositionInfo {
	return types.PositionInfo{
		Contract:      p.Contract,
		Side:          sideFromSize(p.Size),
		Size:          decimalAbsInt(p.Size),
		EntryPrice:    p.EntryPrice.Decimal,
		MarkPrice:     p.MarkPrice.Decimal,
		MarkIv:        p.MarkIv.Decimal,
		RealisedPnl:   p.RealisedPnl.Decimal,
		UnrealisedPnl: p.UnrealisedPnl.Decimal,
		Delta:         p.Delta.Decimal,
		Gamma:         p.Gamma.Decimal,
		Vega:          p.Vega.Decimal,
		Theta:         p.Theta.Decimal,
		PendingOrders: p.PendingOrders,
		UpdatedAtMs:   secondsToMs(p.UpdateTime),
		RateLimits:    rateLimits,
	}
}

// GetPositions returns all option positions for an underlying. Pass an empty
// underlying for every position the account holds.
func (a *AccountClient) GetPositions(ctx context.Context, underlying string) ([]types.PositionInfo, error) {
	var q = newQuery()
	if underlying != "" {
		q.Set("underlying", underlying)
	}

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

// GetPosition returns the position for a single contract. A FLAT contract
// (Gate POSITION_NOT_FOUND, HTTP 400) is surfaced as a zero position.
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
		var gerr *gate.Error
		if errors.As(err, &gerr) && gerr.Label == labelPositionNotFound {
			return types.PositionInfo{Contract: contract}, nil
		}
		return info, err
	}
	var p positionPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "account.GetPosition: parse", err)
	}
	return positionInfoFromPayload(&p, rateLimits), nil
}

// ---- position close history ------------------------------------------------

type positionClosePayload struct {
	Time       int64  `json:"time"`
	Contract   string `json:"contract"`
	Side       string `json:"side"`
	Pnl        string `json:"pnl"`
	SettleSize int64  `json:"settle_size"`
}

// GetPositionClose returns the account's position-close (realized PnL) history
// for an underlying. Pass an empty underlying for every contract.
func (a *AccountClient) GetPositionClose(ctx context.Context, underlying string) ([]types.PositionClose, error) {
	var q = newQuery()
	if underlying != "" {
		q.Set("underlying", underlying)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = a.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   a.positionClosePath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []positionClosePayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "account.GetPositionClose: parse", err)
	}
	var out []types.PositionClose = make([]types.PositionClose, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, types.PositionClose{
			TimeMs:     secondsToMs(payloads[i].Time),
			Contract:   payloads[i].Contract,
			Side:       closeSide(payloads[i].Side, payloads[i].SettleSize),
			Pnl:        mustDecimal(payloads[i].Pnl),
			SettleSize: decimalAbsInt(payloads[i].SettleSize),
			RateLimits: rateLimits,
		})
	}
	return out, nil
}

// closeSide derives the closed-leg side from Gate's explicit "side" string
// ("long"/"short") when present, otherwise from the sign of settle_size.
func closeSide(side string, settleSize int64) types.SideType {
	switch side {
	case "long":
		return types.SideTypeBuy
	case "short":
		return types.SideTypeSell
	}
	return sideFromSize(settleSize)
}

// ---- my settlements --------------------------------------------------------

type mySettlementPayload struct {
	Time         int64  `json:"time"`
	Contract     string `json:"contract"`
	Size         int64  `json:"size"`
	SettlePrice  string `json:"settle_price"`
	SettleProfit string `json:"settle_profit"`
	Fee          string `json:"fee"`
	RealisedPnl  string `json:"realised_pnl"`
}

// GetMySettlements returns the account's own settlement records. limit ≤ 0 lets
// Gate use its default page size.
func (a *AccountClient) GetMySettlements(ctx context.Context, underlying string, limit int) ([]types.MySettlement, error) {
	var q = newQuery()
	if underlying != "" {
		q.Set("underlying", underlying)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = a.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   a.mySettlementsPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []mySettlementPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "account.GetMySettlements: parse", err)
	}
	var out []types.MySettlement = make([]types.MySettlement, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		out = append(out, types.MySettlement{
			TimeMs:       secondsToMs(payloads[i].Time),
			Contract:     payloads[i].Contract,
			Side:         sideFromSize(payloads[i].Size),
			Size:         decimalAbsInt(payloads[i].Size),
			SettlePrice:  mustDecimal(payloads[i].SettlePrice),
			SettleProfit: mustDecimal(payloads[i].SettleProfit),
			Fee:          mustDecimal(payloads[i].Fee),
			RealisedPnl:  mustDecimal(payloads[i].RealisedPnl),
			RateLimits:   rateLimits,
		})
	}
	return out, nil
}
