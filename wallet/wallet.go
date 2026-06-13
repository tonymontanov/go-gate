/*
FILE: wallet/wallet.go

DESCRIPTION:
The Gate Wallet section endpoints, implemented directly on *wallet.Client. The
section is a single flat namespace under "/wallet/...". Implements:

  PUBLIC (unsigned):
    - ListCurrencyChains              : GET  /wallet/currency_chains?currency=

  PRIVATE (signed):
    - Transfer                        : POST /wallet/transfers
    - ListSubAccountTransfers         : GET  /wallet/sub_account_transfers?...
    - TransferWithSubAccount          : POST /wallet/sub_account_transfers
    - SubAccountToSubAccount          : POST /wallet/sub_account_to_sub_account
    - GetTotalBalance                 : GET  /wallet/total_balance?currency=
    - ListSubAccountBalances          : GET  /wallet/sub_account_balances?sub_uid=
    - ListSubAccountMarginBalances    : GET  /wallet/sub_account_margin_balances?sub_uid=
    - ListSubAccountFuturesBalances   : GET  /wallet/sub_account_futures_balances?sub_uid=&settle=
    - ListSubAccountCrossMarginBalances: GET /wallet/sub_account_cross_margin_balances?sub_uid=
    - GetTradeFee                     : GET  /wallet/fee?currency_pair=&settle=
    - ListDeposits                    : GET  /wallet/deposits?...
    - ListWithdrawals                 : GET  /wallet/withdrawals?...
    - ListWithdrawStatus              : GET  /wallet/withdraw_status?currency=

This section is READ-ONLY with respect to on-chain movement: it never creates or
cancels a withdrawal. Amount fields decode through codec.FlexDecimal (Gate quotes
them as strings over REST, but a bare-number form must not break the decode).
Epoch seconds are normalized to milliseconds.
*/

package wallet

import (
	"context"
	"net/url"
	"strconv"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
	"github.com/tonymontanov/go-gate/v2/wallet/types"
)

// ---- query-filter option structs -------------------------------------------

// ListSubAccountTransfersParams — optional filters for ListSubAccountTransfers.
type ListSubAccountTransfersParams struct {
	// SubUID — restrict to one sub-account user id.
	SubUID string
	// From / To — epoch-seconds time window (0 = unset).
	From int64
	To   int64
	// Limit / Offset — pagination (≤ 0 = Gate default).
	Limit  int
	Offset int
}

// ListHistoryParams — optional filters shared by ListDeposits / ListWithdrawals.
type ListHistoryParams struct {
	// Currency — restrict to one currency, e.g. "USDT".
	Currency string
	// From / To — epoch-seconds time window (0 = unset).
	From int64
	To   int64
	// Limit / Offset — pagination (≤ 0 = Gate default).
	Limit  int
	Offset int
}

// historyQuery builds the shared deposits/withdrawals query string from a
// ListHistoryParams (currency + epoch-seconds window + pagination).
func historyQuery(params ListHistoryParams) url.Values {
	var q = newQuery()
	if params.Currency != "" {
		q.Set("currency", params.Currency)
	}
	if params.From > 0 {
		q.Set("from", strconv.FormatInt(params.From, 10))
	}
	if params.To > 0 {
		q.Set("to", strconv.FormatInt(params.To, 10))
	}
	if params.Limit > 0 {
		q.Set("limit", strconv.Itoa(params.Limit))
	}
	if params.Offset > 0 {
		q.Set("offset", strconv.Itoa(params.Offset))
	}
	return q
}

// ---- paths -----------------------------------------------------------------

func (c *Client) transfersPath() string           { return c.basePath() + "/transfers" }
func (c *Client) subAccountTransfersPath() string { return c.basePath() + "/sub_account_transfers" }
func (c *Client) subToSubPath() string            { return c.basePath() + "/sub_account_to_sub_account" }
func (c *Client) totalBalancePath() string        { return c.basePath() + "/total_balance" }
func (c *Client) subAccountBalancesPath() string  { return c.basePath() + "/sub_account_balances" }
func (c *Client) subAccountMarginBalancesPath() string {
	return c.basePath() + "/sub_account_margin_balances"
}
func (c *Client) subAccountFuturesBalancesPath() string {
	return c.basePath() + "/sub_account_futures_balances"
}
func (c *Client) subAccountCrossMarginBalancesPath() string {
	return c.basePath() + "/sub_account_cross_margin_balances"
}
func (c *Client) feePath() string            { return c.basePath() + "/fee" }
func (c *Client) currencyChainsPath() string { return c.basePath() + "/currency_chains" }
func (c *Client) depositsPath() string       { return c.basePath() + "/deposits" }
func (c *Client) withdrawalsPath() string    { return c.basePath() + "/withdrawals" }
func (c *Client) withdrawStatusPath() string { return c.basePath() + "/withdraw_status" }

// ---- transfers -------------------------------------------------------------

type transferResultPayload struct {
	TxID    int64 `json:"tx_id"`
	OrderID int64 `json:"order_id"`
}

// Transfer moves funds between two of the caller's OWN accounts (e.g. spot →
// futures). A futures/delivery leg requires Settle; a margin leg requires
// CurrencyPair.
func (c *Client) Transfer(ctx context.Context, req types.TransferRequest) (types.TransferResult, error) {
	var info types.TransferResult
	if req.Currency == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "wallet.Transfer: Currency is empty", nil)
	}
	if req.From == "" || req.To == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "wallet.Transfer: From and To are required", nil)
	}
	if req.From == req.To {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "wallet.Transfer: From and To must differ", nil)
	}
	if req.Amount.IsZero() || req.Amount.IsNegative() {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "wallet.Transfer: Amount must be positive", nil)
	}

	var body map[string]any = make(map[string]any, 6)
	body["currency"] = req.Currency
	body["from"] = string(req.From)
	body["to"] = string(req.To)
	body["amount"] = req.Amount.String()
	if req.CurrencyPair != "" {
		body["currency_pair"] = req.CurrencyPair
	}
	if req.Settle != "" {
		body["settle"] = req.Settle
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   c.transfersPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p transferResultPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "wallet.Transfer: parse", err)
	}
	return types.TransferResult{
		TxID:       idString(p.TxID),
		OrderID:    idString(p.OrderID),
		RateLimits: rateLimits,
	}, nil
}

type subAccountTransferRecordPayload struct {
	Currency       string            `json:"currency"`
	SubAccount     string            `json:"sub_account"`
	Direction      string            `json:"direction"`
	Amount         codec.FlexDecimal `json:"amount"`
	UID            string            `json:"uid"`
	ClientOrderID  string            `json:"client_order_id"`
	Timest         string            `json:"timest"`
	Source         string            `json:"source"`
	SubAccountType string            `json:"sub_account_type"`
}

// ListSubAccountTransfers returns the main↔sub transfer history.
func (c *Client) ListSubAccountTransfers(ctx context.Context, params ListSubAccountTransfersParams) ([]types.SubAccountTransferRecord, error) {
	var q = newQuery()
	if params.SubUID != "" {
		q.Set("sub_uid", params.SubUID)
	}
	if params.From > 0 {
		q.Set("from", strconv.FormatInt(params.From, 10))
	}
	if params.To > 0 {
		q.Set("to", strconv.FormatInt(params.To, 10))
	}
	if params.Limit > 0 {
		q.Set("limit", strconv.Itoa(params.Limit))
	}
	if params.Offset > 0 {
		q.Set("offset", strconv.Itoa(params.Offset))
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.subAccountTransfersPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []subAccountTransferRecordPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "wallet.ListSubAccountTransfers: parse", err)
	}
	var out []types.SubAccountTransferRecord = make([]types.SubAccountTransferRecord, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, types.SubAccountTransferRecord{
			Currency:       payloads[k].Currency,
			SubAccount:     payloads[k].SubAccount,
			Direction:      types.TransferDirection(payloads[k].Direction),
			Amount:         payloads[k].Amount.Decimal,
			UID:            payloads[k].UID,
			ClientOrderID:  payloads[k].ClientOrderID,
			TimeMs:         secondsStringToMs(payloads[k].Timest),
			Source:         payloads[k].Source,
			SubAccountType: types.AccountType(payloads[k].SubAccountType),
			RateLimits:     rateLimits,
		})
	}
	return out, nil
}

// TransferWithSubAccount moves funds between the main account and a sub-account.
// Gate returns no content on success. Direction is TransferDirectionTo
// (main→sub) or TransferDirectionFrom (sub→main).
func (c *Client) TransferWithSubAccount(ctx context.Context, req types.SubAccountTransferRequest) error {
	if req.Currency == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "wallet.TransferWithSubAccount: Currency is empty", nil)
	}
	if req.SubAccount == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "wallet.TransferWithSubAccount: SubAccount is empty", nil)
	}
	if req.Direction != types.TransferDirectionTo && req.Direction != types.TransferDirectionFrom {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "wallet.TransferWithSubAccount: Direction must be to or from", nil)
	}
	if req.Amount.IsZero() || req.Amount.IsNegative() {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "wallet.TransferWithSubAccount: Amount must be positive", nil)
	}

	var body map[string]any = make(map[string]any, 5)
	body["currency"] = req.Currency
	body["sub_account"] = req.SubAccount
	body["direction"] = string(req.Direction)
	body["amount"] = req.Amount.String()
	if req.SubAccountType != "" {
		body["sub_account_type"] = string(req.SubAccountType)
	}

	var err error
	_, _, err = c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   c.subAccountTransfersPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	return err
}

// SubAccountToSubAccount moves funds directly between two sub-accounts. Gate
// returns no content on success.
func (c *Client) SubAccountToSubAccount(ctx context.Context, req types.SubToSubTransferRequest) error {
	if req.Currency == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "wallet.SubAccountToSubAccount: Currency is empty", nil)
	}
	if req.SubAccountFrom == "" || req.SubAccountTo == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "wallet.SubAccountToSubAccount: SubAccountFrom and SubAccountTo are required", nil)
	}
	if req.Amount.IsZero() || req.Amount.IsNegative() {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "wallet.SubAccountToSubAccount: Amount must be positive", nil)
	}

	var body map[string]any = make(map[string]any, 6)
	body["currency"] = req.Currency
	body["sub_account_from"] = req.SubAccountFrom
	body["sub_account_to"] = req.SubAccountTo
	body["amount"] = req.Amount.String()
	if req.SubAccountFromType != "" {
		body["sub_account_from_type"] = string(req.SubAccountFromType)
	}
	if req.SubAccountToType != "" {
		body["sub_account_to_type"] = string(req.SubAccountToType)
	}

	var err error
	_, _, err = c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   c.subToSubPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	return err
}

// ---- balances --------------------------------------------------------------

type balanceSnapshotPayload struct {
	Currency string            `json:"currency"`
	Amount   codec.FlexDecimal `json:"amount"`
}

type totalBalancePayload struct {
	Total   balanceSnapshotPayload            `json:"total"`
	Details map[string]balanceSnapshotPayload `json:"details"`
}

// GetTotalBalance returns the account-wide estimated total balance with a
// per-location breakdown. Pass an empty currency for Gate's default ("USDT").
func (c *Client) GetTotalBalance(ctx context.Context, currency string) (types.TotalBalance, error) {
	var info types.TotalBalance
	var q = newQuery()
	if currency != "" {
		q.Set("currency", currency)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.totalBalancePath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p totalBalancePayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "wallet.GetTotalBalance: parse", err)
	}
	var details map[string]types.BalanceSnapshot = make(map[string]types.BalanceSnapshot, len(p.Details))
	var location string
	var snap balanceSnapshotPayload
	for location, snap = range p.Details {
		details[location] = types.BalanceSnapshot{
			Currency: snap.Currency,
			Amount:   snap.Amount.Decimal,
		}
	}
	return types.TotalBalance{
		Total: types.BalanceSnapshot{
			Currency: p.Total.Currency,
			Amount:   p.Total.Amount.Decimal,
		},
		Details:    details,
		RateLimits: rateLimits,
	}, nil
}

type subAccountBalancePayload struct {
	UID       string                       `json:"uid"`
	Available map[string]codec.FlexDecimal `json:"available"`
}

// ListSubAccountBalances returns the spot balances of the caller's sub-accounts.
// Pass an empty subUID for every sub-account.
func (c *Client) ListSubAccountBalances(ctx context.Context, subUID string) ([]types.SubAccountBalance, error) {
	var q = newQuery()
	if subUID != "" {
		q.Set("sub_uid", subUID)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.subAccountBalancesPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []subAccountBalancePayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "wallet.ListSubAccountBalances: parse", err)
	}
	var out []types.SubAccountBalance = make([]types.SubAccountBalance, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, types.SubAccountBalance{
			UID:        payloads[k].UID,
			Available:  flexMapToDecimal(payloads[k].Available),
			RateLimits: rateLimits,
		})
	}
	return out, nil
}

type marginLegPayload struct {
	Currency  string            `json:"currency"`
	Available codec.FlexDecimal `json:"available"`
	Locked    codec.FlexDecimal `json:"locked"`
	Borrowed  codec.FlexDecimal `json:"borrowed"`
	Interest  codec.FlexDecimal `json:"interest"`
}

type marginPairPayload struct {
	CurrencyPair string            `json:"currency_pair"`
	Locked       bool              `json:"locked"`
	Risk         codec.FlexDecimal `json:"risk"`
	Base         marginLegPayload  `json:"base"`
	Quote        marginLegPayload  `json:"quote"`
}

type subAccountMarginBalancePayload struct {
	UID       string              `json:"uid"`
	Available []marginPairPayload `json:"available"`
}

func marginLegFromPayload(p *marginLegPayload) types.MarginPairBalanceLeg {
	return types.MarginPairBalanceLeg{
		Currency:  p.Currency,
		Available: p.Available.Decimal,
		Locked:    p.Locked.Decimal,
		Borrowed:  p.Borrowed.Decimal,
		Interest:  p.Interest.Decimal,
	}
}

// ListSubAccountMarginBalances returns the isolated-margin balances of the
// caller's sub-accounts. Pass an empty subUID for every sub-account.
func (c *Client) ListSubAccountMarginBalances(ctx context.Context, subUID string) ([]types.SubAccountMarginBalance, error) {
	var q = newQuery()
	if subUID != "" {
		q.Set("sub_uid", subUID)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.subAccountMarginBalancesPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []subAccountMarginBalancePayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "wallet.ListSubAccountMarginBalances: parse", err)
	}
	var out []types.SubAccountMarginBalance = make([]types.SubAccountMarginBalance, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		var pairs []types.MarginPairBalance = make([]types.MarginPairBalance, 0, len(payloads[k].Available))
		var j int
		for j = 0; j < len(payloads[k].Available); j++ {
			var pp *marginPairPayload = &payloads[k].Available[j]
			pairs = append(pairs, types.MarginPairBalance{
				CurrencyPair: pp.CurrencyPair,
				Locked:       pp.Locked,
				Risk:         pp.Risk.Decimal,
				Base:         marginLegFromPayload(&pp.Base),
				Quote:        marginLegFromPayload(&pp.Quote),
			})
		}
		out = append(out, types.SubAccountMarginBalance{
			UID:        payloads[k].UID,
			Available:  pairs,
			RateLimits: rateLimits,
		})
	}
	return out, nil
}

type futuresBalancePayload struct {
	Currency       string            `json:"currency"`
	Total          codec.FlexDecimal `json:"total"`
	Available      codec.FlexDecimal `json:"available"`
	UnrealisedPnl  codec.FlexDecimal `json:"unrealised_pnl"`
	PositionMargin codec.FlexDecimal `json:"position_margin"`
	OrderMargin    codec.FlexDecimal `json:"order_margin"`
}

type subAccountFuturesBalancePayload struct {
	UID       string                           `json:"uid"`
	Available map[string]futuresBalancePayload `json:"available"`
}

// ListSubAccountFuturesBalances returns the futures balances of the caller's
// sub-accounts. Pass an empty subUID for every sub-account; settle restricts to
// one settle currency (e.g. "usdt").
func (c *Client) ListSubAccountFuturesBalances(ctx context.Context, subUID, settle string) ([]types.SubAccountFuturesBalance, error) {
	var q = newQuery()
	if subUID != "" {
		q.Set("sub_uid", subUID)
	}
	if settle != "" {
		q.Set("settle", settle)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.subAccountFuturesBalancesPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []subAccountFuturesBalancePayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "wallet.ListSubAccountFuturesBalances: parse", err)
	}
	var out []types.SubAccountFuturesBalance = make([]types.SubAccountFuturesBalance, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		var available map[string]types.FuturesBalance = make(map[string]types.FuturesBalance, len(payloads[k].Available))
		var settleKey string
		var fb futuresBalancePayload
		for settleKey, fb = range payloads[k].Available {
			var currency string = fb.Currency
			if currency == "" {
				currency = settleKey
			}
			available[settleKey] = types.FuturesBalance{
				Currency:       currency,
				Total:          fb.Total.Decimal,
				Available:      fb.Available.Decimal,
				UnrealisedPnl:  fb.UnrealisedPnl.Decimal,
				PositionMargin: fb.PositionMargin.Decimal,
				OrderMargin:    fb.OrderMargin.Decimal,
			}
		}
		out = append(out, types.SubAccountFuturesBalance{
			UID:        payloads[k].UID,
			Available:  available,
			RateLimits: rateLimits,
		})
	}
	return out, nil
}

type crossBalancePayload struct {
	Available codec.FlexDecimal `json:"available"`
	Freeze    codec.FlexDecimal `json:"freeze"`
	Borrowed  codec.FlexDecimal `json:"borrowed"`
	Interest  codec.FlexDecimal `json:"interest"`
}

type crossMarginAccountPayload struct {
	UserID   int64                          `json:"user_id"`
	Locked   bool                           `json:"locked"`
	Total    codec.FlexDecimal              `json:"total"`
	Borrowed codec.FlexDecimal              `json:"borrowed"`
	Interest codec.FlexDecimal              `json:"interest"`
	Risk     codec.FlexDecimal              `json:"risk"`
	Balances map[string]crossBalancePayload `json:"balances"`
}

type subAccountCrossMarginBalancePayload struct {
	UID       string                    `json:"uid"`
	Available crossMarginAccountPayload `json:"available"`
}

// ListSubAccountCrossMarginBalances returns the cross-margin accounts of the
// caller's sub-accounts. Pass an empty subUID for every sub-account.
func (c *Client) ListSubAccountCrossMarginBalances(ctx context.Context, subUID string) ([]types.SubAccountCrossMarginBalance, error) {
	var q = newQuery()
	if subUID != "" {
		q.Set("sub_uid", subUID)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.subAccountCrossMarginBalancesPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []subAccountCrossMarginBalancePayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "wallet.ListSubAccountCrossMarginBalances: parse", err)
	}
	var out []types.SubAccountCrossMarginBalance = make([]types.SubAccountCrossMarginBalance, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		var acc *crossMarginAccountPayload = &payloads[k].Available
		var balances map[string]types.CrossMarginCurrencyBalance = make(map[string]types.CrossMarginCurrencyBalance, len(acc.Balances))
		var currency string
		var bal crossBalancePayload
		for currency, bal = range acc.Balances {
			balances[currency] = types.CrossMarginCurrencyBalance{
				Currency:  currency,
				Available: bal.Available.Decimal,
				Freeze:    bal.Freeze.Decimal,
				Borrowed:  bal.Borrowed.Decimal,
				Interest:  bal.Interest.Decimal,
			}
		}
		out = append(out, types.SubAccountCrossMarginBalance{
			UID:        payloads[k].UID,
			Locked:     acc.Locked,
			Total:      acc.Total.Decimal,
			Borrowed:   acc.Borrowed.Decimal,
			Interest:   acc.Interest.Decimal,
			Risk:       acc.Risk.Decimal,
			Balances:   balances,
			RateLimits: rateLimits,
		})
	}
	return out, nil
}

// ---- trade fee -------------------------------------------------------------

type tradeFeePayload struct {
	UserID           int64             `json:"user_id"`
	TakerFee         codec.FlexDecimal `json:"taker_fee"`
	MakerFee         codec.FlexDecimal `json:"maker_fee"`
	GTDiscount       bool              `json:"gt_discount"`
	GTTakerFee       codec.FlexDecimal `json:"gt_taker_fee"`
	GTMakerFee       codec.FlexDecimal `json:"gt_maker_fee"`
	LoanFee          codec.FlexDecimal `json:"loan_fee"`
	PointType        string            `json:"point_type"`
	FuturesTakerFee  codec.FlexDecimal `json:"futures_taker_fee"`
	FuturesMakerFee  codec.FlexDecimal `json:"futures_maker_fee"`
	DeliveryTakerFee codec.FlexDecimal `json:"delivery_taker_fee"`
	DeliveryMakerFee codec.FlexDecimal `json:"delivery_maker_fee"`
	DebitFee         int64             `json:"debit_fee"`
	CurrencyPair     string            `json:"currency_pair"`
}

// GetTradeFee returns the caller's personal trading-fee rates. currencyPair
// (optional) returns the rates for a specific pair; settle (optional) selects the
// futures settle currency.
func (c *Client) GetTradeFee(ctx context.Context, currencyPair, settle string) (types.TradeFee, error) {
	var info types.TradeFee
	var q = newQuery()
	var symbols []string
	if currencyPair != "" {
		q.Set("currency_pair", currencyPair)
		symbols = []string{currencyPair}
	}
	if settle != "" {
		q.Set("settle", settle)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.feePath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Symbols: symbols, Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p tradeFeePayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "wallet.GetTradeFee: parse", err)
	}
	return types.TradeFee{
		UserID:           p.UserID,
		TakerFee:         p.TakerFee.Decimal,
		MakerFee:         p.MakerFee.Decimal,
		GTDiscount:       p.GTDiscount,
		GTTakerFee:       p.GTTakerFee.Decimal,
		GTMakerFee:       p.GTMakerFee.Decimal,
		LoanFee:          p.LoanFee.Decimal,
		PointType:        p.PointType,
		FuturesTakerFee:  p.FuturesTakerFee.Decimal,
		FuturesMakerFee:  p.FuturesMakerFee.Decimal,
		DeliveryTakerFee: p.DeliveryTakerFee.Decimal,
		DeliveryMakerFee: p.DeliveryMakerFee.Decimal,
		DebitFee:         p.DebitFee,
		CurrencyPair:     p.CurrencyPair,
		RateLimits:       rateLimits,
	}, nil
}

// ---- currency chains (public) ----------------------------------------------

type currencyChainPayload struct {
	Chain              string `json:"chain"`
	NameCN             string `json:"name_cn"`
	NameEN             string `json:"name_en"`
	ContractAddress    string `json:"contract_address"`
	AddrRegex          string `json:"addr_regex"`
	IsDisabled         int64  `json:"is_disabled"`
	IsDepositDisabled  int64  `json:"is_deposit_disabled"`
	IsWithdrawDisabled int64  `json:"is_withdraw_disabled"`
	Decimal            string `json:"decimal"`
}

// ListCurrencyChains returns the supported deposit/withdraw chains for a currency
// (public, unsigned).
func (c *Client) ListCurrencyChains(ctx context.Context, currency string) ([]types.CurrencyChain, error) {
	if currency == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "wallet.ListCurrencyChains: currency is empty", nil)
	}
	var q = newQuery()
	q.Set("currency", currency)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.currencyChainsPath(),
		Query:  q,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []currencyChainPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "wallet.ListCurrencyChains: parse", err)
	}
	var out []types.CurrencyChain = make([]types.CurrencyChain, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, types.CurrencyChain{
			Chain:            payloads[k].Chain,
			NameCN:           payloads[k].NameCN,
			NameEN:           payloads[k].NameEN,
			ContractAddress:  payloads[k].ContractAddress,
			AddrRegex:        payloads[k].AddrRegex,
			Disabled:         flagToBool(payloads[k].IsDisabled),
			DepositDisabled:  flagToBool(payloads[k].IsDepositDisabled),
			WithdrawDisabled: flagToBool(payloads[k].IsWithdrawDisabled),
			Decimals:         payloads[k].Decimal,
			RateLimits:       rateLimits,
		})
	}
	return out, nil
}

// ---- deposits / withdrawals (read-only) ------------------------------------

type depositRecordPayload struct {
	ID        string            `json:"id"`
	TxID      string            `json:"txid"`
	Currency  string            `json:"currency"`
	Chain     string            `json:"chain"`
	Address   string            `json:"address"`
	Amount    codec.FlexDecimal `json:"amount"`
	Status    string            `json:"status"`
	Timestamp string            `json:"timestamp"`
}

// ListDeposits returns the deposit history. All filters are optional.
func (c *Client) ListDeposits(ctx context.Context, params ListHistoryParams) ([]types.DepositRecord, error) {
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.depositsPath(),
		Query:  historyQuery(params),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []depositRecordPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "wallet.ListDeposits: parse", err)
	}
	var out []types.DepositRecord = make([]types.DepositRecord, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, types.DepositRecord{
			ID:         payloads[k].ID,
			TxID:       payloads[k].TxID,
			Currency:   payloads[k].Currency,
			Chain:      payloads[k].Chain,
			Address:    payloads[k].Address,
			Amount:     payloads[k].Amount.Decimal,
			Status:     payloads[k].Status,
			TimeMs:     secondsStringToMs(payloads[k].Timestamp),
			RateLimits: rateLimits,
		})
	}
	return out, nil
}

type withdrawalRecordPayload struct {
	ID              string            `json:"id"`
	TxID            string            `json:"txid"`
	WithdrawOrderID string            `json:"withdraw_order_id"`
	Currency        string            `json:"currency"`
	Chain           string            `json:"chain"`
	Address         string            `json:"address"`
	Memo            string            `json:"memo"`
	Amount          codec.FlexDecimal `json:"amount"`
	Fee             codec.FlexDecimal `json:"fee"`
	Status          string            `json:"status"`
	Timestamp       string            `json:"timestamp"`
}

// ListWithdrawals returns the withdrawal history. All filters are optional.
func (c *Client) ListWithdrawals(ctx context.Context, params ListHistoryParams) ([]types.WithdrawalRecord, error) {
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.withdrawalsPath(),
		Query:  historyQuery(params),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []withdrawalRecordPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "wallet.ListWithdrawals: parse", err)
	}
	var out []types.WithdrawalRecord = make([]types.WithdrawalRecord, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, types.WithdrawalRecord{
			ID:              payloads[k].ID,
			TxID:            payloads[k].TxID,
			WithdrawOrderID: payloads[k].WithdrawOrderID,
			Currency:        payloads[k].Currency,
			Chain:           payloads[k].Chain,
			Address:         payloads[k].Address,
			Memo:            payloads[k].Memo,
			Amount:          payloads[k].Amount.Decimal,
			Fee:             payloads[k].Fee.Decimal,
			Status:          payloads[k].Status,
			TimeMs:          secondsStringToMs(payloads[k].Timestamp),
			RateLimits:      rateLimits,
		})
	}
	return out, nil
}

type withdrawStatusPayload struct {
	Currency               string                       `json:"currency"`
	Name                   string                       `json:"name"`
	WithdrawFix            codec.FlexDecimal            `json:"withdraw_fix"`
	WithdrawPercent        string                       `json:"withdraw_percent"`
	WithdrawDayLimit       codec.FlexDecimal            `json:"withdraw_day_limit"`
	WithdrawDayLimitRemain codec.FlexDecimal            `json:"withdraw_day_limit_remain"`
	WithdrawAmountMini     codec.FlexDecimal            `json:"withdraw_amount_mini"`
	WithdrawEachtimeLimit  codec.FlexDecimal            `json:"withdraw_eachtime_limit"`
	WithdrawFixOnChains    map[string]codec.FlexDecimal `json:"withdraw_fix_on_chains"`
}

// ListWithdrawStatus returns the per-currency withdrawal fees and limits. Pass an
// empty currency for every currency.
func (c *Client) ListWithdrawStatus(ctx context.Context, currency string) ([]types.WithdrawStatus, error) {
	var q = newQuery()
	if currency != "" {
		q.Set("currency", currency)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.withdrawStatusPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []withdrawStatusPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "wallet.ListWithdrawStatus: parse", err)
	}
	var out []types.WithdrawStatus = make([]types.WithdrawStatus, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, types.WithdrawStatus{
			Currency:               payloads[k].Currency,
			Name:                   payloads[k].Name,
			WithdrawFix:            payloads[k].WithdrawFix.Decimal,
			WithdrawPercent:        payloads[k].WithdrawPercent,
			WithdrawDayLimit:       payloads[k].WithdrawDayLimit.Decimal,
			WithdrawDayLimitRemain: payloads[k].WithdrawDayLimitRemain.Decimal,
			WithdrawAmountMini:     payloads[k].WithdrawAmountMini.Decimal,
			WithdrawEachtimeLimit:  payloads[k].WithdrawEachtimeLimit.Decimal,
			WithdrawFixOnChains:    flexMapToDecimal(payloads[k].WithdrawFixOnChains),
			RateLimits:             rateLimits,
		})
	}
	return out, nil
}
