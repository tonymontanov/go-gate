/*
FILE: margin/cross_test.go

DESCRIPTION:
Contract tests for the cross-margin sub-client against httptest-served Gate JSON.
They pin: the public currencies list parse, the single cross account parse with
its per-currency balance map, CreateLoan request-body + parse, the Repay path +
body returning the affected loans, GetTransferable/GetBorrowable, and a Gate
{label,message} error surfacing as *gate.Error.
*/

package margin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/margin/types"
)

func TestCross_ListCurrencies_Public(t *testing.T) {
	var gotPath, gotKey string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey = r.URL.Path, r.Header.Get("KEY")
		_, _ = io.WriteString(w, `[{"name":"BTC","rate":"0.0002","prec":"0.000001","discount":"0.95",
			"min_borrow_amount":"0.001","user_max_borrow_amount":"5","total_max_borrow_amount":"100",
			"price":"60000","loanable":true,"status":1}]`)
	}))
	defer srv.Close()

	var m *Client = newMarginTestClient(t, srv.URL)
	var currencies []types.CrossCurrency
	var err error
	currencies, err = m.Cross().ListCurrencies(context.Background())
	if err != nil {
		t.Fatalf("ListCurrencies: %v", err)
	}
	if gotPath != "/margin/cross/currencies" {
		t.Fatalf("path: %q", gotPath)
	}
	if gotKey != "" {
		t.Fatalf("public call should be unsigned, got KEY=%q", gotKey)
	}
	if len(currencies) != 1 || currencies[0].Name != "BTC" || !currencies[0].Loanable {
		t.Fatalf("currencies: %+v", currencies)
	}
	if !currencies[0].Rate.Equal(mustDec("0.0002")) || !currencies[0].Price.Equal(mustDec("60000")) {
		t.Fatalf("rate/price: %s / %s", currencies[0].Rate, currencies[0].Price)
	}
}

func TestCross_GetAccount_ParsesBalances(t *testing.T) {
	var gotPath, gotKey string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey = r.URL.Path, r.Header.Get("KEY")
		// "total" arrives as a bare number, the leg balances as strings —
		// FlexDecimal must decode both.
		_, _ = io.WriteString(w, `{"user_id":42,"locked":false,"total":12345.67,"borrowed":"200","interest":"0.5","risk":"3.2",
			"total_initial_margin":"100","total_margin_balance":"12000","total_maintenance_margin":"50",
			"balances":{"BTC":{"available":"0.5","freeze":"0","borrowed":"0.1","interest":"0.0001"},
				"USDT":{"available":"1000","freeze":"10","borrowed":"200","interest":"0.5"}}}`)
	}))
	defer srv.Close()

	var m *Client = newMarginTestClient(t, srv.URL)
	var acc types.CrossAccount
	var err error
	acc, err = m.Cross().GetAccount(context.Background())
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if gotPath != "/margin/cross/accounts" {
		t.Fatalf("path: %q", gotPath)
	}
	if gotKey == "" {
		t.Fatalf("private call should be signed (KEY header missing)")
	}
	if acc.UserID != 42 || !acc.Total.Equal(mustDec("12345.67")) || !acc.Risk.Equal(mustDec("3.2")) {
		t.Fatalf("account scalars: user=%d total=%s risk=%s", acc.UserID, acc.Total, acc.Risk)
	}
	if len(acc.Balances) != 2 {
		t.Fatalf("expected 2 balances, got %d", len(acc.Balances))
	}
	if !acc.Balances["BTC"].Available.Equal(mustDec("0.5")) || acc.Balances["BTC"].Currency != "BTC" {
		t.Fatalf("BTC balance: %+v", acc.Balances["BTC"])
	}
	if !acc.Balances["USDT"].Borrowed.Equal(mustDec("200")) {
		t.Fatalf("USDT borrowed: %s", acc.Balances["USDT"].Borrowed)
	}
}

func TestCross_CreateLoan_BodyAndParse(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":555,"create_time":1700000000,"update_time":1700000001,"currency":"BTC",
			"amount":"0.5","text":"desk-1","status":"loaned","repaid":"0","repaid_interest":"0","unpaid_interest":"0"}`)
	}))
	defer srv.Close()

	var m *Client = newMarginTestClient(t, srv.URL)
	var loan types.CrossLoan
	var err error
	loan, err = m.Cross().CreateLoan(context.Background(), types.CreateCrossLoanRequest{
		Currency: "BTC",
		Amount:   mustDec("0.5"),
		Text:     "desk-1",
	})
	if err != nil {
		t.Fatalf("CreateLoan: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/margin/cross/loans" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["currency"] != "BTC" || gotBody["amount"] != "0.5" || gotBody["text"] != "desk-1" {
		t.Fatalf("body: %+v", gotBody)
	}
	if loan.ID != "555" || loan.Currency != "BTC" || !loan.Amount.Equal(mustDec("0.5")) {
		t.Fatalf("parsed loan: %+v", loan)
	}
	if loan.CreatedAtMs != 1700000000000 || loan.UpdatedAtMs != 1700000001000 {
		t.Fatalf("timestamps: created=%d updated=%d", loan.CreatedAtMs, loan.UpdatedAtMs)
	}
}

func TestCross_Repay_PathBodyAndParsesArray(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		_, _ = io.WriteString(w, `[{"id":555,"currency":"BTC","amount":"0.5","status":"finished","repaid":"0.5","unpaid_interest":"0"}]`)
	}))
	defer srv.Close()

	var m *Client = newMarginTestClient(t, srv.URL)
	var loans []types.CrossLoan
	var err error
	loans, err = m.Cross().Repay(context.Background(), types.CrossRepayRequest{
		Currency: "BTC",
		Amount:   mustDec("0.5"),
	})
	if err != nil {
		t.Fatalf("Repay: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/margin/cross/repayments" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["currency"] != "BTC" || gotBody["amount"] != "0.5" {
		t.Fatalf("body: %+v", gotBody)
	}
	if len(loans) != 1 || loans[0].ID != "555" || loans[0].Status != types.LoanStatusFinished {
		t.Fatalf("parsed loans: %+v", loans)
	}
}

func TestCross_ListRepayments_QueryAndParse(t *testing.T) {
	var gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = io.WriteString(w, `[{"id":7,"create_time":1700000000,"loan_id":555,"currency":"BTC",
			"principal":"0.5","interest":"0.0001","repayment_type":"auto"}]`)
	}))
	defer srv.Close()

	var m *Client = newMarginTestClient(t, srv.URL)
	var reps []types.CrossRepayment
	var err error
	reps, err = m.Cross().ListRepayments(context.Background(), ListCrossRepaymentsParams{Currency: "BTC", Limit: 10})
	if err != nil {
		t.Fatalf("ListRepayments: %v", err)
	}
	if !strings.Contains(gotQuery, "currency=BTC") || !strings.Contains(gotQuery, "limit=10") {
		t.Fatalf("query: %q", gotQuery)
	}
	if len(reps) != 1 || reps[0].LoanID != "555" || !reps[0].Principal.Equal(mustDec("0.5")) {
		t.Fatalf("parsed repayments: %+v", reps)
	}
}

func TestCross_GetTransferableAndBorrowable(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/margin/cross/transferable":
			_, _ = io.WriteString(w, `{"currency":"BTC","amount":"1.25"}`)
		case "/margin/cross/borrowable":
			_, _ = io.WriteString(w, `{"currency":"BTC","amount":"5"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var m *Client = newMarginTestClient(t, srv.URL)

	var tr types.Transferable
	var err error
	tr, err = m.Cross().GetTransferable(context.Background(), "BTC")
	if err != nil {
		t.Fatalf("GetTransferable: %v", err)
	}
	if tr.Currency != "BTC" || tr.CurrencyPair != "" || !tr.Amount.Equal(mustDec("1.25")) {
		t.Fatalf("transferable: %+v", tr)
	}

	var b types.Borrowable
	b, err = m.Cross().GetBorrowable(context.Background(), "BTC")
	if err != nil {
		t.Fatalf("GetBorrowable: %v", err)
	}
	if !b.Amount.Equal(mustDec("5")) {
		t.Fatalf("borrowable amount: %s", b.Amount)
	}
}

func TestCross_ErrorLabel_Surfaces(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"label":"BALANCE_NOT_ENOUGH","message":"insufficient balance"}`)
	}))
	defer srv.Close()

	var m *Client = newMarginTestClient(t, srv.URL)
	var err error
	_, err = m.Cross().CreateLoan(context.Background(), types.CreateCrossLoanRequest{Currency: "BTC", Amount: mustDec("1")})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ge *gate.Error
	if !errors.As(err, &ge) || ge.Label != "BALANCE_NOT_ENOUGH" {
		t.Fatalf("unexpected error: %v", err)
	}
}
