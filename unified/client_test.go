/*
FILE: unified/client_test.go

DESCRIPTION:
Contract tests that drive the unified.Client against an httptest.Server returning
real-shaped Gate unified-account JSON. They pin:
  - the unified account snapshot parse, including codec.FlexDecimal decoding the
    balances/margins from BOTH the number and the string wire forms;
  - borrowable/loans/leverage read parsing and query encoding;
  - the CreateLoan POST body shape and the SetMode PUT body shape;
  - the GetMode parse;
  - Gate {label,message} errors surfacing as *gate.Error.

No network: the parent client's REST base points at the test server.
*/

package unified

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/unified/types"
)

func mustDec(s string) decimal.Decimal {
	var d decimal.Decimal
	var err error
	d, err = decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func newUnifiedTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	var parent *gate.Client
	var err error
	parent, err = gate.NewClient(gate.Config{
		APIKey:    "test-key",
		SecretKey: "test-secret",
		REST:      gate.RestConfig{BaseURL: baseURL},
	})
	if err != nil {
		t.Fatalf("gate.NewClient: %v", err)
	}
	return NewClient(parent)
}

// decodeBody reads and JSON-decodes the request body into a generic map.
func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	var raw []byte
	raw, _ = io.ReadAll(r.Body)
	var m map[string]any = map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("decode body: %v (raw=%s)", err, string(raw))
		}
	}
	return m
}

func TestGetAccount_ParsesSnapshot(t *testing.T) {
	var gotPath, gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = io.WriteString(w, `{"user_id":12345,"refresh_time":1614620745,"locked":false,
			"total":"230.94","borrowed":"161.66","equity":"232.1","unified_account_total":"230.94",
			"total_initial_margin":"100","total_maintenance_margin":"50","total_available_margin":"880",
			"balances":{"BTC":{"available":"1.5","freeze":"0.1","borrowed":"0.2","equity":"1.4","spot_in_use":"0"},
				"USDT":{"available":"1000","freeze":"0","borrowed":"50","equity":"950","spot_in_use":"0"}}}`)
	}))
	defer srv.Close()

	var u *Client = newUnifiedTestClient(t, srv.URL)
	var acc types.UnifiedAccount
	var err error
	acc, err = u.GetAccount(context.Background(), "BTC", 0)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if gotPath != "/unified/accounts" {
		t.Fatalf("path: %q", gotPath)
	}
	if !strings.Contains(gotQuery, "currency=BTC") {
		t.Fatalf("query: %q", gotQuery)
	}
	if acc.UserID != 12345 || acc.RefreshTimeMs != 1614620745000 {
		t.Fatalf("user/refresh: %d / %d", acc.UserID, acc.RefreshTimeMs)
	}
	if !acc.Total.Equal(mustDec("230.94")) || !acc.Borrowed.Equal(mustDec("161.66")) {
		t.Fatalf("total/borrowed: %s / %s", acc.Total, acc.Borrowed)
	}
	if len(acc.Balances) != 2 {
		t.Fatalf("balances: want 2, got %d", len(acc.Balances))
	}
	if !acc.Balances["BTC"].Available.Equal(mustDec("1.5")) || !acc.Balances["USDT"].Borrowed.Equal(mustDec("50")) {
		t.Fatalf("balance fields: %s / %s", acc.Balances["BTC"].Available, acc.Balances["USDT"].Borrowed)
	}
}

// TestAccountPayload_DecodesNumberAndStringForms guards that the unified account
// balances/margins decode from BOTH the quoted-string REST form and the bare
// JSON-number form via codec.FlexDecimal.
func TestAccountPayload_DecodesNumberAndStringForms(t *testing.T) {
	var numberForm = []byte(`{"user_id":1,"total":230.94,"borrowed":161.66,"equity":232.1,
		"total_initial_margin":100,"balances":{"BTC":{"available":1.5,"borrowed":0.2,"equity":1.4}}}`)
	var stringForm = []byte(`{"user_id":1,"total":"230.94","borrowed":"161.66","equity":"232.1",
		"total_initial_margin":"100","balances":{"BTC":{"available":"1.5","borrowed":"0.2","equity":"1.4"}}}`)

	var form string
	var raw []byte
	for form, raw = range map[string][]byte{"number": numberForm, "string": stringForm} {
		var p accountPayload
		if err := codec.Unmarshal(raw, &p); err != nil {
			t.Fatalf("%s: unmarshal failed: %v", form, err)
		}
		var acc types.UnifiedAccount = accountFromPayload(&p, nil)
		if !acc.Total.Equal(mustDec("230.94")) || !acc.TotalInitialMargin.Equal(mustDec("100")) {
			t.Fatalf("%s: total/imargin: %s / %s", form, acc.Total, acc.TotalInitialMargin)
		}
		if !acc.Balances["BTC"].Available.Equal(mustDec("1.5")) || !acc.Balances["BTC"].Borrowed.Equal(mustDec("0.2")) {
			t.Fatalf("%s: btc balance: %s / %s", form, acc.Balances["BTC"].Available, acc.Balances["BTC"].Borrowed)
		}
	}
}

func TestGetBorrowable_ParsesAndQuery(t *testing.T) {
	var gotPath, gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = io.WriteString(w, `{"currency":"BTC","amount":"1.2345"}`)
	}))
	defer srv.Close()

	var u *Client = newUnifiedTestClient(t, srv.URL)
	var b types.Borrowable
	var err error
	b, err = u.GetBorrowable(context.Background(), "BTC")
	if err != nil {
		t.Fatalf("GetBorrowable: %v", err)
	}
	if gotPath != "/unified/borrowable" || !strings.Contains(gotQuery, "currency=BTC") {
		t.Fatalf("path/query: %q / %q", gotPath, gotQuery)
	}
	if b.Currency != "BTC" || !b.Amount.Equal(mustDec("1.2345")) {
		t.Fatalf("borrowable: %q / %s", b.Currency, b.Amount)
	}
}

func TestListLoans_ParsesArray(t *testing.T) {
	var gotPath, gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = io.WriteString(w, `[{"currency":"USDT","amount":"100.5","type":"platform","create_time":1614620745,"update_time":1614620800},
			{"currency":"BTC","amount":"0.2","type":"platform","create_time":1614620745}]`)
	}))
	defer srv.Close()

	var u *Client = newUnifiedTestClient(t, srv.URL)
	var loans []types.UnifiedLoan
	var err error
	loans, err = u.ListLoans(context.Background(), "", 1, 10)
	if err != nil {
		t.Fatalf("ListLoans: %v", err)
	}
	if gotPath != "/unified/loans" {
		t.Fatalf("path: %q", gotPath)
	}
	if !strings.Contains(gotQuery, "page=1") || !strings.Contains(gotQuery, "limit=10") {
		t.Fatalf("query: %q", gotQuery)
	}
	if len(loans) != 2 {
		t.Fatalf("want 2 loans, got %d", len(loans))
	}
	if loans[0].Currency != "USDT" || !loans[0].Amount.Equal(mustDec("100.5")) || loans[0].CreateTimeMs != 1614620745000 {
		t.Fatalf("loan[0]: %+v", loans[0])
	}
}

func TestCreateLoan_BodyShapeAndParse(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		_, _ = io.WriteString(w, `{"currency":"USDT","amount":"100","type":"borrow","create_time":1614620745}`)
	}))
	defer srv.Close()

	var u *Client = newUnifiedTestClient(t, srv.URL)
	var loan types.UnifiedLoan
	var err error
	loan, err = u.CreateLoan(context.Background(), types.CreateLoanRequest{
		Currency: "USDT",
		Amount:   mustDec("100"),
		Type:     types.LoanTypeBorrow,
	})
	if err != nil {
		t.Fatalf("CreateLoan: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/unified/loans" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["currency"] != "USDT" || gotBody["type"] != "borrow" || gotBody["amount"] != "100" {
		t.Fatalf("body: %+v", gotBody)
	}
	if _, ok := gotBody["repaid_all"]; ok {
		t.Fatalf("repaid_all must be omitted when false: %+v", gotBody)
	}
	if loan.Currency != "USDT" || !loan.Amount.Equal(mustDec("100")) {
		t.Fatalf("loan parse: %+v", loan)
	}
}

func TestCreateLoan_RepayAllBody(t *testing.T) {
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody = decodeBody(t, r)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	var u *Client = newUnifiedTestClient(t, srv.URL)
	var err error
	_, err = u.CreateLoan(context.Background(), types.CreateLoanRequest{
		Currency:  "USDT",
		Type:      types.LoanTypeRepay,
		RepaidAll: true,
	})
	if err != nil {
		t.Fatalf("CreateLoan: %v", err)
	}
	if gotBody["repaid_all"] != true || gotBody["type"] != "repay" {
		t.Fatalf("repay-all body: %+v", gotBody)
	}
}

func TestGetMode_Parses(t *testing.T) {
	var gotPath string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{"mode":"multi_currency","settings":{"usdt_futures":true,"spot_hedge":false}}`)
	}))
	defer srv.Close()

	var u *Client = newUnifiedTestClient(t, srv.URL)
	var mode types.UnifiedMode
	var err error
	mode, err = u.GetMode(context.Background())
	if err != nil {
		t.Fatalf("GetMode: %v", err)
	}
	if gotPath != "/unified/unified_mode" {
		t.Fatalf("path: %q", gotPath)
	}
	if mode.Mode != types.AccountModeMultiCurrency {
		t.Fatalf("mode: %q", mode.Mode)
	}
	if !mode.Settings["usdt_futures"] || mode.Settings["spot_hedge"] {
		t.Fatalf("settings: %+v", mode.Settings)
	}
}

func TestSetMode_PutBody(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	var u *Client = newUnifiedTestClient(t, srv.URL)
	var err error
	err = u.SetMode(context.Background(), types.SetModeRequest{
		Mode:     types.AccountModePortfolio,
		Settings: map[string]bool{"usdt_futures": true},
	})
	if err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/unified/unified_mode" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["mode"] != "portfolio" {
		t.Fatalf("mode body: %+v", gotBody)
	}
	var settings map[string]any
	var ok bool
	settings, ok = gotBody["settings"].(map[string]any)
	if !ok || settings["usdt_futures"] != true {
		t.Fatalf("settings body: %+v", gotBody["settings"])
	}
}

func TestGetLeverageSetting_ParsesAndQuery(t *testing.T) {
	var gotPath, gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = io.WriteString(w, `{"currency":"BTC","leverage":"5"}`)
	}))
	defer srv.Close()

	var u *Client = newUnifiedTestClient(t, srv.URL)
	var s types.LeverageSetting
	var err error
	s, err = u.GetLeverageSetting(context.Background(), "BTC")
	if err != nil {
		t.Fatalf("GetLeverageSetting: %v", err)
	}
	if gotPath != "/unified/leverage/user_currency_setting" || !strings.Contains(gotQuery, "currency=BTC") {
		t.Fatalf("path/query: %q / %q", gotPath, gotQuery)
	}
	if s.Currency != "BTC" || !s.Leverage.Equal(mustDec("5")) {
		t.Fatalf("setting: %q / %s", s.Currency, s.Leverage)
	}
}

func TestGetAccount_ErrorLabel_Surfaces(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"label":"INVALID_PARAM_VALUE","message":"bad currency"}`)
	}))
	defer srv.Close()

	var u *Client = newUnifiedTestClient(t, srv.URL)
	var err error
	_, err = u.GetAccount(context.Background(), "BTC", 0)
	if err == nil {
		t.Fatalf("expected error")
	}
	var ge *gate.Error
	if !errors.As(err, &ge) || ge.Label != "INVALID_PARAM_VALUE" || !gate.IsInvalidRequest(err) {
		t.Fatalf("unexpected error: %v", err)
	}
}
