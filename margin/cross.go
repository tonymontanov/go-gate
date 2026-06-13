/*
FILE: margin/cross.go

DESCRIPTION:
Cross-margin sub-client for the Gate Margin section. A SINGLE cross-margin
account collateralizes borrowing across every supported currency (it is not
pair-scoped). Implements:

  PUBLIC (unsigned):
    - ListCurrencies   : GET  /margin/cross/currencies
    - GetCurrency      : GET  /margin/cross/currencies/{currency}

  PRIVATE (signed):
    - GetAccount       : GET  /margin/cross/accounts
    - ListAccountBook  : GET  /margin/cross/account_book?currency=&...
    - ListLoans        : GET  /margin/cross/loans?status=&currency=&...
    - CreateLoan       : POST /margin/cross/loans
    - GetLoan          : GET  /margin/cross/loans/{loan_id}
    - ListRepayments   : GET  /margin/cross/repayments?currency=&...
    - Repay            : POST /margin/cross/repayments
    - GetTransferable  : GET  /margin/cross/transferable?currency=
    - GetBorrowable    : GET  /margin/cross/borrowable?currency=

Balance/amount fields decode through codec.FlexDecimal; epoch seconds are
normalized to milliseconds. The cross account-book reuses the isolated
accountBookPayload (its currency_pair field stays empty for cross).
*/

package margin

import (
	"context"
	"strconv"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
	"github.com/tonymontanov/go-gate/v2/margin/types"
)

// CrossClient — cross-margin sub-client.
type CrossClient struct {
	c *Client
}

func newCrossClient(c *Client) *CrossClient {
	return &CrossClient{c: c}
}

// ---- query-filter option structs -------------------------------------------

// ListCrossLoansParams — optional filters for CrossClient.ListLoans.
type ListCrossLoansParams struct {
	// Status — restrict to one loan status.
	Status types.LoanStatus
	// Currency — restrict to one currency, e.g. "BTC".
	Currency string
	// Limit / Offset — pagination (≤ 0 = Gate default).
	Limit  int
	Offset int
	// Reverse — list newest-first when true.
	Reverse bool
}

// ListCrossAccountBookParams — optional filters for CrossClient.ListAccountBook.
type ListCrossAccountBookParams struct {
	// Currency — restrict to one currency, e.g. "BTC".
	Currency string
	// Type — restrict to one Gate change type.
	Type string
	// From / To — epoch-seconds time window (0 = unset).
	From int64
	To   int64
	// Page / Limit — pagination (≤ 0 = Gate default).
	Page  int
	Limit int
}

// ListCrossRepaymentsParams — optional filters for CrossClient.ListRepayments.
type ListCrossRepaymentsParams struct {
	// Currency — restrict to one currency, e.g. "BTC".
	Currency string
	// LoanID — restrict to one loan id.
	LoanID string
	// Page / Limit — pagination (≤ 0 = Gate default).
	Page  int
	Limit int
}

// ---- paths -----------------------------------------------------------------

func (x *CrossClient) currenciesPath() string { return x.c.crossBasePath() + "/currencies" }
func (x *CrossClient) currencyPath(currency string) string {
	return x.c.crossBasePath() + "/currencies/" + currency
}
func (x *CrossClient) accountsPath() string    { return x.c.crossBasePath() + "/accounts" }
func (x *CrossClient) accountBookPath() string { return x.c.crossBasePath() + "/account_book" }
func (x *CrossClient) loansPath() string       { return x.c.crossBasePath() + "/loans" }
func (x *CrossClient) loanPath(id string) string {
	return x.c.crossBasePath() + "/loans/" + id
}
func (x *CrossClient) repaymentsPath() string   { return x.c.crossBasePath() + "/repayments" }
func (x *CrossClient) transferablePath() string { return x.c.crossBasePath() + "/transferable" }
func (x *CrossClient) borrowablePath() string   { return x.c.crossBasePath() + "/borrowable" }

// ---- currencies (public) ---------------------------------------------------

type crossCurrencyPayload struct {
	Name                 string            `json:"name"`
	Rate                 codec.FlexDecimal `json:"rate"`
	Prec                 codec.FlexDecimal `json:"prec"`
	Discount             codec.FlexDecimal `json:"discount"`
	MinBorrowAmount      codec.FlexDecimal `json:"min_borrow_amount"`
	UserMaxBorrowAmount  codec.FlexDecimal `json:"user_max_borrow_amount"`
	TotalMaxBorrowAmount codec.FlexDecimal `json:"total_max_borrow_amount"`
	Price                codec.FlexDecimal `json:"price"`
	Loanable             bool              `json:"loanable"`
	Status               int64             `json:"status"`
}

func crossCurrencyFromPayload(p *crossCurrencyPayload, rateLimits map[string]string) types.CrossCurrency {
	return types.CrossCurrency{
		Name:                 p.Name,
		Rate:                 p.Rate.Decimal,
		Precision:            p.Prec.Decimal,
		Discount:             p.Discount.Decimal,
		MinBorrowAmount:      p.MinBorrowAmount.Decimal,
		UserMaxBorrowAmount:  p.UserMaxBorrowAmount.Decimal,
		TotalMaxBorrowAmount: p.TotalMaxBorrowAmount.Decimal,
		Price:                p.Price.Decimal,
		Loanable:             p.Loanable,
		Status:               p.Status,
		RateLimits:           rateLimits,
	}
}

// ListCurrencies returns all cross-margin currencies and their borrow limits
// (public).
func (x *CrossClient) ListCurrencies(ctx context.Context) ([]types.CrossCurrency, error) {
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = x.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   x.currenciesPath(),
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []crossCurrencyPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "cross.ListCurrencies: parse", err)
	}
	var out []types.CrossCurrency = make([]types.CrossCurrency, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, crossCurrencyFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// GetCurrency returns a single cross-margin currency spec (public).
func (x *CrossClient) GetCurrency(ctx context.Context, currency string) (types.CrossCurrency, error) {
	var info types.CrossCurrency
	if currency == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "cross.GetCurrency: currency is empty", nil)
	}
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = x.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   x.currencyPath(currency),
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return info, err
	}
	var p crossCurrencyPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "cross.GetCurrency: parse", err)
	}
	return crossCurrencyFromPayload(&p, rateLimits), nil
}

// ---- account ---------------------------------------------------------------

type crossBalancePayload struct {
	Available codec.FlexDecimal `json:"available"`
	Freeze    codec.FlexDecimal `json:"freeze"`
	Borrowed  codec.FlexDecimal `json:"borrowed"`
	Interest  codec.FlexDecimal `json:"interest"`
}

type crossAccountPayload struct {
	UserID                 int64                          `json:"user_id"`
	Locked                 bool                           `json:"locked"`
	Total                  codec.FlexDecimal              `json:"total"`
	Borrowed               codec.FlexDecimal              `json:"borrowed"`
	Interest               codec.FlexDecimal              `json:"interest"`
	Risk                   codec.FlexDecimal              `json:"risk"`
	TotalInitialMargin     codec.FlexDecimal              `json:"total_initial_margin"`
	TotalMarginBalance     codec.FlexDecimal              `json:"total_margin_balance"`
	TotalMaintenanceMargin codec.FlexDecimal              `json:"total_maintenance_margin"`
	Balances               map[string]crossBalancePayload `json:"balances"`
}

// GetAccount returns the single cross-margin account with its per-currency
// balance breakdown.
func (x *CrossClient) GetAccount(ctx context.Context) (types.CrossAccount, error) {
	var info types.CrossAccount
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = x.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   x.accountsPath(),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p crossAccountPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "cross.GetAccount: parse", err)
	}
	var balances map[string]types.CrossBalance = make(map[string]types.CrossBalance, len(p.Balances))
	var currency string
	var bal crossBalancePayload
	for currency, bal = range p.Balances {
		balances[currency] = types.CrossBalance{
			Currency:  currency,
			Available: bal.Available.Decimal,
			Freeze:    bal.Freeze.Decimal,
			Borrowed:  bal.Borrowed.Decimal,
			Interest:  bal.Interest.Decimal,
		}
	}
	return types.CrossAccount{
		UserID:                 p.UserID,
		Locked:                 p.Locked,
		Total:                  p.Total.Decimal,
		Borrowed:               p.Borrowed.Decimal,
		Interest:               p.Interest.Decimal,
		Risk:                   p.Risk.Decimal,
		TotalInitialMargin:     p.TotalInitialMargin.Decimal,
		TotalMarginBalance:     p.TotalMarginBalance.Decimal,
		TotalMaintenanceMargin: p.TotalMaintenanceMargin.Decimal,
		Balances:               balances,
		RateLimits:             rateLimits,
	}, nil
}

// ---- account book ----------------------------------------------------------

// ListAccountBook returns the cross-margin account-changing history. All filters
// are optional. The returned rows reuse types.AccountBookEntry with an empty
// CurrencyPair (cross is not pair-scoped).
func (x *CrossClient) ListAccountBook(ctx context.Context, params ListCrossAccountBookParams) ([]types.AccountBookEntry, error) {
	var q = newQuery()
	if params.Currency != "" {
		q.Set("currency", params.Currency)
	}
	if params.Type != "" {
		q.Set("type", params.Type)
	}
	if params.From > 0 {
		q.Set("from", strconv.FormatInt(params.From, 10))
	}
	if params.To > 0 {
		q.Set("to", strconv.FormatInt(params.To, 10))
	}
	if params.Page > 0 {
		q.Set("page", strconv.Itoa(params.Page))
	}
	if params.Limit > 0 {
		q.Set("limit", strconv.Itoa(params.Limit))
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = x.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   x.accountBookPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []accountBookPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "cross.ListAccountBook: parse", err)
	}
	var out []types.AccountBookEntry = make([]types.AccountBookEntry, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, accountBookEntryFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// ---- loans -----------------------------------------------------------------

type crossLoanPayload struct {
	ID             int64             `json:"id"`
	CreateTime     int64             `json:"create_time"`
	UpdateTime     int64             `json:"update_time"`
	Currency       string            `json:"currency"`
	Amount         codec.FlexDecimal `json:"amount"`
	Text           string            `json:"text"`
	Status         string            `json:"status"`
	Repaid         codec.FlexDecimal `json:"repaid"`
	RepaidInterest codec.FlexDecimal `json:"repaid_interest"`
	UnpaidInterest codec.FlexDecimal `json:"unpaid_interest"`
}

func crossLoanFromPayload(p *crossLoanPayload, rateLimits map[string]string) types.CrossLoan {
	return types.CrossLoan{
		ID:             idString(p.ID),
		CreatedAtMs:    secondsToMs(p.CreateTime),
		UpdatedAtMs:    secondsToMs(p.UpdateTime),
		Currency:       p.Currency,
		Amount:         p.Amount.Decimal,
		Text:           p.Text,
		Status:         types.LoanStatus(p.Status),
		Repaid:         p.Repaid.Decimal,
		RepaidInterest: p.RepaidInterest.Decimal,
		UnpaidInterest: p.UnpaidInterest.Decimal,
		RateLimits:     rateLimits,
	}
}

// ListLoans returns the cross-margin loans matching the filters.
func (x *CrossClient) ListLoans(ctx context.Context, params ListCrossLoansParams) ([]types.CrossLoan, error) {
	var q = newQuery()
	if params.Status != "" {
		q.Set("status", string(params.Status))
	}
	if params.Currency != "" {
		q.Set("currency", params.Currency)
	}
	if params.Limit > 0 {
		q.Set("limit", strconv.Itoa(params.Limit))
	}
	if params.Offset > 0 {
		q.Set("offset", strconv.Itoa(params.Offset))
	}
	if params.Reverse {
		q.Set("reverse", "true")
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = x.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   x.loansPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []crossLoanPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "cross.ListLoans: parse", err)
	}
	var out []types.CrossLoan = make([]types.CrossLoan, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, crossLoanFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// CreateLoan borrows currency on cross margin.
func (x *CrossClient) CreateLoan(ctx context.Context, req types.CreateCrossLoanRequest) (types.CrossLoan, error) {
	var info types.CrossLoan
	if req.Currency == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "cross.CreateLoan: Currency is empty", nil)
	}
	if req.Amount.IsZero() || req.Amount.IsNegative() {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "cross.CreateLoan: Amount must be positive", nil)
	}
	var body map[string]any = make(map[string]any, 3)
	body["currency"] = req.Currency
	body["amount"] = req.Amount.String()
	if req.Text != "" {
		body["text"] = req.Text
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = x.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   x.loansPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p crossLoanPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "cross.CreateLoan: parse", err)
	}
	return crossLoanFromPayload(&p, rateLimits), nil
}

// GetLoan returns a single cross-margin loan by id.
func (x *CrossClient) GetLoan(ctx context.Context, loanID string) (types.CrossLoan, error) {
	var info types.CrossLoan
	if loanID == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "cross.GetLoan: loanID is empty", nil)
	}
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = x.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   x.loanPath(loanID),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p crossLoanPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "cross.GetLoan: parse", err)
	}
	return crossLoanFromPayload(&p, rateLimits), nil
}

// ---- repayments ------------------------------------------------------------

type crossRepaymentPayload struct {
	ID            int64             `json:"id"`
	CreateTime    int64             `json:"create_time"`
	LoanID        int64             `json:"loan_id"`
	Currency      string            `json:"currency"`
	Principal     codec.FlexDecimal `json:"principal"`
	Interest      codec.FlexDecimal `json:"interest"`
	RepaymentType string            `json:"repayment_type"`
}

func crossRepaymentFromPayload(p *crossRepaymentPayload, rateLimits map[string]string) types.CrossRepayment {
	return types.CrossRepayment{
		ID:            idString(p.ID),
		CreatedAtMs:   secondsToMs(p.CreateTime),
		LoanID:        idString(p.LoanID),
		Currency:      p.Currency,
		Principal:     p.Principal.Decimal,
		Interest:      p.Interest.Decimal,
		RepaymentType: p.RepaymentType,
		RateLimits:    rateLimits,
	}
}

// ListRepayments returns the cross-margin repayment history matching the filters.
func (x *CrossClient) ListRepayments(ctx context.Context, params ListCrossRepaymentsParams) ([]types.CrossRepayment, error) {
	var q = newQuery()
	if params.Currency != "" {
		q.Set("currency", params.Currency)
	}
	if params.LoanID != "" {
		q.Set("loan_id", params.LoanID)
	}
	if params.Page > 0 {
		q.Set("page", strconv.Itoa(params.Page))
	}
	if params.Limit > 0 {
		q.Set("limit", strconv.Itoa(params.Limit))
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = x.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   x.repaymentsPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []crossRepaymentPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "cross.ListRepayments: parse", err)
	}
	var out []types.CrossRepayment = make([]types.CrossRepayment, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, crossRepaymentFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// Repay repays borrowed cross-margin currency. Gate applies the repayment across
// the user's outstanding loans and returns the affected loans.
func (x *CrossClient) Repay(ctx context.Context, req types.CrossRepayRequest) ([]types.CrossLoan, error) {
	if req.Currency == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "cross.Repay: Currency is empty", nil)
	}
	if req.Amount.IsZero() || req.Amount.IsNegative() {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "cross.Repay: Amount must be positive", nil)
	}
	var body map[string]any = map[string]any{
		"currency": req.Currency,
		"amount":   req.Amount.String(),
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = x.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   x.repaymentsPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []crossLoanPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "cross.Repay: parse", err)
	}
	var out []types.CrossLoan = make([]types.CrossLoan, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, crossLoanFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// ---- transferable / borrowable ---------------------------------------------

type crossTransferablePayload struct {
	Currency string            `json:"currency"`
	Amount   codec.FlexDecimal `json:"amount"`
}

// GetTransferable returns the maximum amount transferable out of the
// cross-margin account for a currency.
func (x *CrossClient) GetTransferable(ctx context.Context, currency string) (types.Transferable, error) {
	var info types.Transferable
	if currency == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "cross.GetTransferable: currency is empty", nil)
	}
	var q = newQuery()
	q.Set("currency", currency)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = x.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   x.transferablePath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p crossTransferablePayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "cross.GetTransferable: parse", err)
	}
	return types.Transferable{
		Currency:   p.Currency,
		Amount:     p.Amount.Decimal,
		RateLimits: rateLimits,
	}, nil
}

// GetBorrowable returns the maximum amount borrowable into the cross-margin
// account for a currency.
func (x *CrossClient) GetBorrowable(ctx context.Context, currency string) (types.Borrowable, error) {
	var info types.Borrowable
	if currency == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "cross.GetBorrowable: currency is empty", nil)
	}
	var q = newQuery()
	q.Set("currency", currency)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = x.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   x.borrowablePath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p crossTransferablePayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "cross.GetBorrowable: parse", err)
	}
	return types.Borrowable{
		Currency:   p.Currency,
		Amount:     p.Amount.Decimal,
		RateLimits: rateLimits,
	}, nil
}
