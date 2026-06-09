/*
FILE: spot/account.go

DESCRIPTION:
Account sub-client for the Gate Spot section. Spot has per-currency balances
(available/locked), not positions, so this exposes:
  - GetBalances : GET /spot/accounts            → []types.Balance
  - GetBalance  : GET /spot/accounts?currency=  → types.Balance (one currency)

Both are private (signed).
*/

package spot

import (
	"context"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
	"github.com/tonymontanov/go-gate/v2/spot/types"
)

// AccountClient — account/balance sub-client.
type AccountClient struct {
	c *Client
}

func newAccountClient(c *Client) *AccountClient {
	return &AccountClient{c: c}
}

func (a *AccountClient) accountsPath() string { return "/spot/accounts" }

type spotAccountPayload struct {
	Currency  string `json:"currency"`
	Available string `json:"available"`
	Locked    string `json:"locked"`
}

func balanceFromPayload(p *spotAccountPayload, rateLimits map[string]string) types.Balance {
	return types.Balance{
		Currency:   p.Currency,
		Available:  mustDecimal(p.Available),
		Locked:     mustDecimal(p.Locked),
		RateLimits: rateLimits,
	}
}

// GetBalances returns all spot balances (one entry per currency Gate reports).
func (a *AccountClient) GetBalances(ctx context.Context) ([]types.Balance, error) {
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = a.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   a.accountsPath(),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []spotAccountPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "account.GetBalances: parse", err)
	}
	var out []types.Balance = make([]types.Balance, 0, len(payloads))
	var i int
	for i = 0; i < len(payloads); i++ {
		var rl map[string]string
		if i == 0 {
			rl = rateLimits
		}
		out = append(out, balanceFromPayload(&payloads[i], rl))
	}
	return out, nil
}

// GetBalance returns the balance for a single currency (e.g. "USDT"). If Gate
// returns no entry for the currency, a zero Balance with that currency is
// returned (not an error).
func (a *AccountClient) GetBalance(ctx context.Context, currency string) (types.Balance, error) {
	var bal types.Balance = types.Balance{Currency: currency}
	if currency == "" {
		return bal, gate.NewError(gate.ErrorKindInvalidRequest, "", "account.GetBalance: currency is empty", nil)
	}
	var q = newQuery()
	q.Set("currency", currency)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = a.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   a.accountsPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return bal, err
	}
	var payloads []spotAccountPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return bal, gate.NewError(gate.ErrorKindUnknown, "", "account.GetBalance: parse", err)
	}
	var i int
	for i = 0; i < len(payloads); i++ {
		if payloads[i].Currency == currency {
			return balanceFromPayload(&payloads[i], rateLimits), nil
		}
	}
	bal.RateLimits = rateLimits
	return bal, nil
}
