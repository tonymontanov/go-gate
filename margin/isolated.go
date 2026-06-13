/*
FILE: margin/isolated.go

DESCRIPTION:
Isolated-margin sub-client for the Gate Margin section. Each isolated-margin
account is scoped to a single currency pair. Implements:

  PUBLIC (unsigned):
    - ListCurrencyPairs   : GET  /margin/currency_pairs
    - GetCurrencyPair     : GET  /margin/currency_pairs/{currency_pair}
    - GetFundingBook      : GET  /margin/funding_book?currency=

  PRIVATE (signed):
    - ListAccounts        : GET  /margin/accounts?currency_pair=
    - ListAccountBook     : GET  /margin/account_book?currency_pair=&...
    - ListFundingAccounts : GET  /margin/funding_accounts?currency=
    - ListLoans           : GET  /margin/loans?status=&side=&currency_pair=&...
    - CreateLoan          : POST /margin/loans
    - GetLoan             : GET  /margin/loans/{loan_id}?side=&currency_pair=
    - ListLoanRepayments  : GET  /margin/loans/{loan_id}/repayment
    - RepayLoan           : POST /margin/loans/{loan_id}/repayment
    - ListLoanRecords     : GET  /margin/loan_records?loan_id=&...
    - GetLoanRecord       : GET  /margin/loan_records/{loan_record_id}?loan_id=
    - GetAutoRepayStatus  : GET  /margin/auto_repay
    - SetAutoRepay        : POST /margin/auto_repay
    - GetTransferable     : GET  /margin/transferable?currency=&currency_pair=
    - GetBorrowable       : GET  /margin/borrowable?currency=&currency_pair=

Balance/amount fields decode through codec.FlexDecimal (Gate quotes them as
strings over REST, but a bare-number form must not break the decode). Epoch
seconds are normalized to milliseconds.
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

// IsolatedClient — isolated-margin sub-client.
type IsolatedClient struct {
	c *Client
}

func newIsolatedClient(c *Client) *IsolatedClient {
	return &IsolatedClient{c: c}
}

// ---- query-filter option structs -------------------------------------------

// ListAccountBookParams — optional filters for ListAccountBook.
type ListAccountBookParams struct {
	// CurrencyPair — restrict to one pair, e.g. "BTC_USDT".
	CurrencyPair string
	// Currency — restrict to one currency, e.g. "USDT".
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

// ListLoansParams — optional filters for ListLoans.
type ListLoansParams struct {
	// Status — restrict to one loan status.
	Status types.LoanStatus
	// Side — restrict to lend or borrow.
	Side types.LoanSide
	// CurrencyPair — restrict to one pair, e.g. "BTC_USDT".
	CurrencyPair string
	// Currency — restrict to one currency, e.g. "USDT".
	Currency string
	// Page / Limit — pagination (≤ 0 = Gate default).
	Page  int
	Limit int
}

// ListLoanRecordsParams — optional filters for ListLoanRecords.
type ListLoanRecordsParams struct {
	// LoanID — restrict to one parent loan (required by Gate for some queries).
	LoanID string
	// Status — restrict to one record status.
	Status types.LoanStatus
	// Page / Limit — pagination (≤ 0 = Gate default).
	Page  int
	Limit int
}

// ---- paths -----------------------------------------------------------------

func (i *IsolatedClient) currencyPairsPath() string { return i.c.basePath() + "/currency_pairs" }
func (i *IsolatedClient) currencyPairPath(pair string) string {
	return i.c.basePath() + "/currency_pairs/" + pair
}
func (i *IsolatedClient) fundingBookPath() string     { return i.c.basePath() + "/funding_book" }
func (i *IsolatedClient) accountsPath() string        { return i.c.basePath() + "/accounts" }
func (i *IsolatedClient) accountBookPath() string     { return i.c.basePath() + "/account_book" }
func (i *IsolatedClient) fundingAccountsPath() string { return i.c.basePath() + "/funding_accounts" }
func (i *IsolatedClient) loansPath() string           { return i.c.basePath() + "/loans" }
func (i *IsolatedClient) loanPath(id string) string   { return i.c.basePath() + "/loans/" + id }
func (i *IsolatedClient) loanRepaymentPath(id string) string {
	return i.c.basePath() + "/loans/" + id + "/repayment"
}
func (i *IsolatedClient) loanRecordsPath() string { return i.c.basePath() + "/loan_records" }
func (i *IsolatedClient) loanRecordPath(id string) string {
	return i.c.basePath() + "/loan_records/" + id
}
func (i *IsolatedClient) autoRepayPath() string    { return i.c.basePath() + "/auto_repay" }
func (i *IsolatedClient) transferablePath() string { return i.c.basePath() + "/transferable" }
func (i *IsolatedClient) borrowablePath() string   { return i.c.basePath() + "/borrowable" }

// ---- currency pairs (public) -----------------------------------------------

type currencyPairPayload struct {
	ID             string            `json:"id"`
	Base           string            `json:"base"`
	Quote          string            `json:"quote"`
	Leverage       codec.FlexDecimal `json:"leverage"`
	MinBaseAmount  codec.FlexDecimal `json:"min_base_amount"`
	MinQuoteAmount codec.FlexDecimal `json:"min_quote_amount"`
	MaxQuoteAmount codec.FlexDecimal `json:"max_quote_amount"`
	Status         int64             `json:"status"`
}

func currencyPairFromPayload(p *currencyPairPayload, rateLimits map[string]string) types.MarginCurrencyPair {
	return types.MarginCurrencyPair{
		ID:             p.ID,
		Base:           p.Base,
		Quote:          p.Quote,
		Leverage:       p.Leverage.Decimal,
		MinBaseAmount:  p.MinBaseAmount.Decimal,
		MinQuoteAmount: p.MinQuoteAmount.Decimal,
		MaxQuoteAmount: p.MaxQuoteAmount.Decimal,
		Status:         p.Status,
		RateLimits:     rateLimits,
	}
}

// ListCurrencyPairs returns all isolated-margin currency pairs (public).
func (i *IsolatedClient) ListCurrencyPairs(ctx context.Context) ([]types.MarginCurrencyPair, error) {
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = i.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   i.currencyPairsPath(),
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []currencyPairPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "isolated.ListCurrencyPairs: parse", err)
	}
	var out []types.MarginCurrencyPair = make([]types.MarginCurrencyPair, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, currencyPairFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// GetCurrencyPair returns a single isolated-margin currency pair spec (public).
func (i *IsolatedClient) GetCurrencyPair(ctx context.Context, currencyPair string) (types.MarginCurrencyPair, error) {
	var info types.MarginCurrencyPair
	if currencyPair == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "isolated.GetCurrencyPair: currencyPair is empty", nil)
	}
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = i.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   i.currencyPairPath(currencyPair),
		Meta:   rest.RequestMeta{Symbols: []string{currencyPair}, Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return info, err
	}
	var p currencyPairPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "isolated.GetCurrencyPair: parse", err)
	}
	return currencyPairFromPayload(&p, rateLimits), nil
}

// ---- funding book (public) -------------------------------------------------

type fundingBookEntryPayload struct {
	Rate   codec.FlexDecimal `json:"rate"`
	Amount codec.FlexDecimal `json:"amount"`
	Days   int64             `json:"days"`
}

type fundingBookPayload struct {
	Asks []fundingBookEntryPayload `json:"asks"`
	Bids []fundingBookEntryPayload `json:"bids"`
}

func fundingBookEntriesFromPayload(items []fundingBookEntryPayload) []types.FundingBookEntry {
	var out []types.FundingBookEntry = make([]types.FundingBookEntry, 0, len(items))
	var k int
	for k = 0; k < len(items); k++ {
		out = append(out, types.FundingBookEntry{
			Rate:   items[k].Rate.Decimal,
			Amount: items[k].Amount.Decimal,
			Days:   items[k].Days,
		})
	}
	return out
}

// GetFundingBook returns the isolated-margin funding (lending) book for a
// currency (public).
func (i *IsolatedClient) GetFundingBook(ctx context.Context, currency string) (types.FundingBook, error) {
	var book types.FundingBook
	if currency == "" {
		return book, gate.NewError(gate.ErrorKindInvalidRequest, "", "isolated.GetFundingBook: currency is empty", nil)
	}
	var q = newQuery()
	q.Set("currency", currency)

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = i.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   i.fundingBookPath(),
		Query:  q,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryMarketData)},
	})
	if err != nil {
		return book, err
	}
	var p fundingBookPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return book, gate.NewError(gate.ErrorKindUnknown, "", "isolated.GetFundingBook: parse", err)
	}
	book = types.FundingBook{
		Currency:   currency,
		Asks:       fundingBookEntriesFromPayload(p.Asks),
		Bids:       fundingBookEntriesFromPayload(p.Bids),
		RateLimits: rateLimits,
	}
	return book, nil
}

// ---- accounts --------------------------------------------------------------

type marginBalancePayload struct {
	Currency  string            `json:"currency"`
	Available codec.FlexDecimal `json:"available"`
	Locked    codec.FlexDecimal `json:"locked"`
	Borrowed  codec.FlexDecimal `json:"borrowed"`
	Interest  codec.FlexDecimal `json:"interest"`
}

func marginBalanceFromPayload(p *marginBalancePayload) types.MarginBalance {
	return types.MarginBalance{
		Currency:  p.Currency,
		Available: p.Available.Decimal,
		Locked:    p.Locked.Decimal,
		Borrowed:  p.Borrowed.Decimal,
		Interest:  p.Interest.Decimal,
	}
}

type marginAccountPayload struct {
	CurrencyPair string               `json:"currency_pair"`
	Locked       bool                 `json:"locked"`
	Risk         codec.FlexDecimal    `json:"risk"`
	MarginLevel  codec.FlexDecimal    `json:"margin_level"`
	Base         marginBalancePayload `json:"base"`
	Quote        marginBalancePayload `json:"quote"`
}

func marginAccountFromPayload(p *marginAccountPayload, rateLimits map[string]string) types.MarginAccount {
	return types.MarginAccount{
		CurrencyPair: p.CurrencyPair,
		Locked:       p.Locked,
		Risk:         p.Risk.Decimal,
		MarginLevel:  p.MarginLevel.Decimal,
		Base:         marginBalanceFromPayload(&p.Base),
		Quote:        marginBalanceFromPayload(&p.Quote),
		RateLimits:   rateLimits,
	}
}

// ListAccounts returns the isolated-margin accounts. Pass an empty currencyPair
// for every account the user holds.
func (i *IsolatedClient) ListAccounts(ctx context.Context, currencyPair string) ([]types.MarginAccount, error) {
	var q = newQuery()
	var symbols []string
	if currencyPair != "" {
		q.Set("currency_pair", currencyPair)
		symbols = []string{currencyPair}
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = i.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   i.accountsPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Symbols: symbols, Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []marginAccountPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "isolated.ListAccounts: parse", err)
	}
	var out []types.MarginAccount = make([]types.MarginAccount, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, marginAccountFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// ---- account book ----------------------------------------------------------

type accountBookPayload struct {
	ID           int64             `json:"id"`
	Time         int64             `json:"time"`
	TimeMs       int64             `json:"time_ms"`
	Currency     string            `json:"currency"`
	CurrencyPair string            `json:"currency_pair"`
	Change       codec.FlexDecimal `json:"change"`
	Balance      codec.FlexDecimal `json:"balance"`
	Type         string            `json:"type"`
}

func accountBookEntryFromPayload(p *accountBookPayload, rateLimits map[string]string) types.AccountBookEntry {
	return types.AccountBookEntry{
		ID:           idString(p.ID),
		TimeMs:       epochMs(p.TimeMs, p.Time),
		Currency:     p.Currency,
		CurrencyPair: p.CurrencyPair,
		Change:       p.Change.Decimal,
		Balance:      p.Balance.Decimal,
		Type:         p.Type,
		RateLimits:   rateLimits,
	}
}

// ListAccountBook returns the isolated-margin account-changing history. All
// filters are optional.
func (i *IsolatedClient) ListAccountBook(ctx context.Context, params ListAccountBookParams) ([]types.AccountBookEntry, error) {
	var q = newQuery()
	if params.CurrencyPair != "" {
		q.Set("currency_pair", params.CurrencyPair)
	}
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
	resp, rateLimits, err = i.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   i.accountBookPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []accountBookPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "isolated.ListAccountBook: parse", err)
	}
	var out []types.AccountBookEntry = make([]types.AccountBookEntry, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, accountBookEntryFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// ---- funding accounts ------------------------------------------------------

type fundingAccountPayload struct {
	Currency  string            `json:"currency"`
	Available codec.FlexDecimal `json:"available"`
	Locked    codec.FlexDecimal `json:"locked"`
	Lent      codec.FlexDecimal `json:"lent"`
	TotalLent codec.FlexDecimal `json:"total_lent"`
}

// ListFundingAccounts returns the margin lending (funding) account balances.
// Pass an empty currency for every funding currency.
func (i *IsolatedClient) ListFundingAccounts(ctx context.Context, currency string) ([]types.FundingAccount, error) {
	var q = newQuery()
	if currency != "" {
		q.Set("currency", currency)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = i.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   i.fundingAccountsPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []fundingAccountPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "isolated.ListFundingAccounts: parse", err)
	}
	var out []types.FundingAccount = make([]types.FundingAccount, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, types.FundingAccount{
			Currency:   payloads[k].Currency,
			Available:  payloads[k].Available.Decimal,
			Locked:     payloads[k].Locked.Decimal,
			Lent:       payloads[k].Lent.Decimal,
			TotalLent:  payloads[k].TotalLent.Decimal,
			RateLimits: rateLimits,
		})
	}
	return out, nil
}

// ---- loans -----------------------------------------------------------------

type loanPayload struct {
	ID             int64             `json:"id"`
	CreateTime     int64             `json:"create_time"`
	ExpireTime     int64             `json:"expire_time"`
	Status         string            `json:"status"`
	Side           string            `json:"side"`
	Currency       string            `json:"currency"`
	CurrencyPair   string            `json:"currency_pair"`
	Rate           codec.FlexDecimal `json:"rate"`
	Amount         codec.FlexDecimal `json:"amount"`
	Days           int64             `json:"days"`
	AutoRenew      bool              `json:"auto_renew"`
	Left           codec.FlexDecimal `json:"left"`
	Repaid         codec.FlexDecimal `json:"repaid"`
	PaidInterest   codec.FlexDecimal `json:"paid_interest"`
	UnpaidInterest codec.FlexDecimal `json:"unpaid_interest"`
	FeeRate        codec.FlexDecimal `json:"fee_rate"`
	OriginalID     int64             `json:"original_id"`
}

func loanFromPayload(p *loanPayload, rateLimits map[string]string) types.MarginLoan {
	return types.MarginLoan{
		ID:             idString(p.ID),
		CreatedAtMs:    secondsToMs(p.CreateTime),
		ExpiresAtMs:    secondsToMs(p.ExpireTime),
		Side:           types.LoanSide(p.Side),
		Status:         types.LoanStatus(p.Status),
		Currency:       p.Currency,
		CurrencyPair:   p.CurrencyPair,
		Rate:           p.Rate.Decimal,
		Amount:         p.Amount.Decimal,
		Days:           p.Days,
		AutoRenew:      p.AutoRenew,
		Left:           p.Left.Decimal,
		Repaid:         p.Repaid.Decimal,
		PaidInterest:   p.PaidInterest.Decimal,
		UnpaidInterest: p.UnpaidInterest.Decimal,
		FeeRate:        p.FeeRate.Decimal,
		OriginalID:     idString(p.OriginalID),
		RateLimits:     rateLimits,
	}
}

// ListLoans returns the isolated-margin loans matching the filters.
func (i *IsolatedClient) ListLoans(ctx context.Context, params ListLoansParams) ([]types.MarginLoan, error) {
	var q = newQuery()
	if params.Status != "" {
		q.Set("status", string(params.Status))
	}
	if params.Side != "" {
		q.Set("side", string(params.Side))
	}
	if params.CurrencyPair != "" {
		q.Set("currency_pair", params.CurrencyPair)
	}
	if params.Currency != "" {
		q.Set("currency", params.Currency)
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
	resp, rateLimits, err = i.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   i.loansPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []loanPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "isolated.ListLoans: parse", err)
	}
	var out []types.MarginLoan = make([]types.MarginLoan, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, loanFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// CreateLoan creates an isolated-margin lend or borrow loan.
func (i *IsolatedClient) CreateLoan(ctx context.Context, req types.CreateLoanRequest) (types.MarginLoan, error) {
	var info types.MarginLoan
	if req.Side != types.LoanSideLend && req.Side != types.LoanSideBorrow {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "isolated.CreateLoan: Side must be lend or borrow", nil)
	}
	if req.Currency == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "isolated.CreateLoan: Currency is empty", nil)
	}
	if req.Amount.IsZero() || req.Amount.IsNegative() {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "isolated.CreateLoan: Amount must be positive", nil)
	}

	var body map[string]any = make(map[string]any, 7)
	body["side"] = string(req.Side)
	body["currency"] = req.Currency
	body["amount"] = req.Amount.String()
	if !req.Rate.IsZero() {
		body["rate"] = req.Rate.String()
	}
	if req.Days > 0 {
		body["days"] = req.Days
	}
	if req.AutoRenew {
		body["auto_renew"] = true
	}
	if req.CurrencyPair != "" {
		body["currency_pair"] = req.CurrencyPair
	}
	if !req.FeeRate.IsZero() {
		body["fee_rate"] = req.FeeRate.String()
	}

	var symbols []string
	if req.CurrencyPair != "" {
		symbols = []string{req.CurrencyPair}
	}
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = i.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   i.loansPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Symbols: symbols, Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p loanPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "isolated.CreateLoan: parse", err)
	}
	return loanFromPayload(&p, rateLimits), nil
}

// GetLoan returns a single isolated-margin loan. side and currencyPair are
// optional Gate disambiguators (pass empty to omit).
func (i *IsolatedClient) GetLoan(ctx context.Context, loanID string, side types.LoanSide, currencyPair string) (types.MarginLoan, error) {
	var info types.MarginLoan
	if loanID == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "isolated.GetLoan: loanID is empty", nil)
	}
	var q = newQuery()
	if side != "" {
		q.Set("side", string(side))
	}
	var symbols []string
	if currencyPair != "" {
		q.Set("currency_pair", currencyPair)
		symbols = []string{currencyPair}
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = i.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   i.loanPath(loanID),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Symbols: symbols, Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p loanPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "isolated.GetLoan: parse", err)
	}
	return loanFromPayload(&p, rateLimits), nil
}

// ---- loan repayments -------------------------------------------------------

type loanRepaymentPayload struct {
	ID         int64             `json:"id"`
	CreateTime int64             `json:"create_time"`
	Principal  codec.FlexDecimal `json:"principal"`
	Interest   codec.FlexDecimal `json:"interest"`
}

func loanRepaymentFromPayload(p *loanRepaymentPayload, rateLimits map[string]string) types.LoanRepayment {
	return types.LoanRepayment{
		ID:          idString(p.ID),
		CreatedAtMs: secondsToMs(p.CreateTime),
		Principal:   p.Principal.Decimal,
		Interest:    p.Interest.Decimal,
		RateLimits:  rateLimits,
	}
}

// ListLoanRepayments returns the repayment records of a single loan.
func (i *IsolatedClient) ListLoanRepayments(ctx context.Context, loanID string) ([]types.LoanRepayment, error) {
	if loanID == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "isolated.ListLoanRepayments: loanID is empty", nil)
	}
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = i.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   i.loanRepaymentPath(loanID),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []loanRepaymentPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "isolated.ListLoanRepayments: parse", err)
	}
	var out []types.LoanRepayment = make([]types.LoanRepayment, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, loanRepaymentFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// RepayLoan repays an isolated-margin loan, in full (RepayModeAll) or partially
// (RepayModePartial with Amount). Returns the updated loan.
func (i *IsolatedClient) RepayLoan(ctx context.Context, req types.RepayLoanRequest) (types.MarginLoan, error) {
	var info types.MarginLoan
	if req.LoanID == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "isolated.RepayLoan: LoanID is empty", nil)
	}
	if req.Mode != types.RepayModeAll && req.Mode != types.RepayModePartial {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "isolated.RepayLoan: Mode must be all or partial", nil)
	}
	if req.Mode == types.RepayModePartial && (req.Amount.IsZero() || req.Amount.IsNegative()) {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "isolated.RepayLoan: Amount must be positive for partial mode", nil)
	}

	var body map[string]any = make(map[string]any, 4)
	body["mode"] = string(req.Mode)
	if req.CurrencyPair != "" {
		body["currency_pair"] = req.CurrencyPair
	}
	if req.Currency != "" {
		body["currency"] = req.Currency
	}
	if req.Mode == types.RepayModePartial {
		body["amount"] = req.Amount.String()
	}

	var symbols []string
	if req.CurrencyPair != "" {
		symbols = []string{req.CurrencyPair}
	}
	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = i.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   i.loanRepaymentPath(req.LoanID),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Symbols: symbols, Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p loanPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "isolated.RepayLoan: parse", err)
	}
	return loanFromPayload(&p, rateLimits), nil
}

// ---- loan records ----------------------------------------------------------

type loanRecordPayload struct {
	ID             int64             `json:"id"`
	LoanID         int64             `json:"loan_id"`
	CreateTime     int64             `json:"create_time"`
	ExpireTime     int64             `json:"expire_time"`
	Status         string            `json:"status"`
	BorrowUserID   int64             `json:"borrow_user_id"`
	Currency       string            `json:"currency"`
	CurrencyPair   string            `json:"currency_pair"`
	Rate           codec.FlexDecimal `json:"rate"`
	Amount         codec.FlexDecimal `json:"amount"`
	Days           int64             `json:"days"`
	AutoRenew      bool              `json:"auto_renew"`
	Repaid         codec.FlexDecimal `json:"repaid"`
	PaidInterest   codec.FlexDecimal `json:"paid_interest"`
	UnpaidInterest codec.FlexDecimal `json:"unpaid_interest"`
}

func loanRecordFromPayload(p *loanRecordPayload, rateLimits map[string]string) types.LoanRecord {
	return types.LoanRecord{
		ID:             idString(p.ID),
		LoanID:         idString(p.LoanID),
		CreatedAtMs:    secondsToMs(p.CreateTime),
		ExpiresAtMs:    secondsToMs(p.ExpireTime),
		Status:         types.LoanStatus(p.Status),
		BorrowUserID:   p.BorrowUserID,
		Currency:       p.Currency,
		CurrencyPair:   p.CurrencyPair,
		Rate:           p.Rate.Decimal,
		Amount:         p.Amount.Decimal,
		Days:           p.Days,
		AutoRenew:      p.AutoRenew,
		Repaid:         p.Repaid.Decimal,
		PaidInterest:   p.PaidInterest.Decimal,
		UnpaidInterest: p.UnpaidInterest.Decimal,
		RateLimits:     rateLimits,
	}
}

// ListLoanRecords returns the per-fill loan records matching the filters.
func (i *IsolatedClient) ListLoanRecords(ctx context.Context, params ListLoanRecordsParams) ([]types.LoanRecord, error) {
	var q = newQuery()
	if params.LoanID != "" {
		q.Set("loan_id", params.LoanID)
	}
	if params.Status != "" {
		q.Set("status", string(params.Status))
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
	resp, rateLimits, err = i.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   i.loanRecordsPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []loanRecordPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "isolated.ListLoanRecords: parse", err)
	}
	var out []types.LoanRecord = make([]types.LoanRecord, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, loanRecordFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// GetLoanRecord returns a single loan record. loanID is the Gate-required
// disambiguator (pass empty to omit).
func (i *IsolatedClient) GetLoanRecord(ctx context.Context, loanRecordID, loanID string) (types.LoanRecord, error) {
	var info types.LoanRecord
	if loanRecordID == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "isolated.GetLoanRecord: loanRecordID is empty", nil)
	}
	var q = newQuery()
	if loanID != "" {
		q.Set("loan_id", loanID)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = i.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   i.loanRecordPath(loanRecordID),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p loanRecordPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "isolated.GetLoanRecord: parse", err)
	}
	return loanRecordFromPayload(&p, rateLimits), nil
}

// ---- auto repay ------------------------------------------------------------

type autoRepayPayload struct {
	Status string `json:"status"`
}

// GetAutoRepayStatus returns the current isolated-margin auto-repay setting.
func (i *IsolatedClient) GetAutoRepayStatus(ctx context.Context) (types.AutoRepayStatus, error) {
	var resp rest.Response
	var err error
	resp, _, err = i.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   i.autoRepayPath(),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return "", err
	}
	var p autoRepayPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return "", gate.NewError(gate.ErrorKindUnknown, "", "isolated.GetAutoRepayStatus: parse", err)
	}
	return types.AutoRepayStatus(p.Status), nil
}

// SetAutoRepay enables/disables isolated-margin auto-repay and returns the
// resulting setting.
func (i *IsolatedClient) SetAutoRepay(ctx context.Context, status types.AutoRepayStatus) (types.AutoRepayStatus, error) {
	if status != types.AutoRepayOn && status != types.AutoRepayOff {
		return "", gate.NewError(gate.ErrorKindInvalidRequest, "", "isolated.SetAutoRepay: status must be on or off", nil)
	}
	var body map[string]any = map[string]any{"status": string(status)}

	var resp rest.Response
	var err error
	resp, _, err = i.c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   i.autoRepayPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return "", err
	}
	var p autoRepayPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return "", gate.NewError(gate.ErrorKindUnknown, "", "isolated.SetAutoRepay: parse", err)
	}
	return types.AutoRepayStatus(p.Status), nil
}

// ---- transferable / borrowable ---------------------------------------------

type transferablePayload struct {
	Currency     string            `json:"currency"`
	CurrencyPair string            `json:"currency_pair"`
	Amount       codec.FlexDecimal `json:"amount"`
}

// GetTransferable returns the maximum amount transferable out of an
// isolated-margin account. currencyPair is required by Gate for isolated margin.
func (i *IsolatedClient) GetTransferable(ctx context.Context, currency, currencyPair string) (types.Transferable, error) {
	var info types.Transferable
	if currency == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "isolated.GetTransferable: currency is empty", nil)
	}
	var q = newQuery()
	q.Set("currency", currency)
	var symbols []string
	if currencyPair != "" {
		q.Set("currency_pair", currencyPair)
		symbols = []string{currencyPair}
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = i.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   i.transferablePath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Symbols: symbols, Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p transferablePayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "isolated.GetTransferable: parse", err)
	}
	return types.Transferable{
		Currency:     p.Currency,
		CurrencyPair: p.CurrencyPair,
		Amount:       p.Amount.Decimal,
		RateLimits:   rateLimits,
	}, nil
}

// GetBorrowable returns the maximum amount borrowable into an isolated-margin
// account. currencyPair is required by Gate for isolated margin.
func (i *IsolatedClient) GetBorrowable(ctx context.Context, currency, currencyPair string) (types.Borrowable, error) {
	var info types.Borrowable
	if currency == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "isolated.GetBorrowable: currency is empty", nil)
	}
	var q = newQuery()
	q.Set("currency", currency)
	var symbols []string
	if currencyPair != "" {
		q.Set("currency_pair", currencyPair)
		symbols = []string{currencyPair}
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = i.c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   i.borrowablePath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Symbols: symbols, Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p transferablePayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "isolated.GetBorrowable: parse", err)
	}
	return types.Borrowable{
		Currency:     p.Currency,
		CurrencyPair: p.CurrencyPair,
		Amount:       p.Amount.Decimal,
		RateLimits:   rateLimits,
	}, nil
}
