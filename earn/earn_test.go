/*
FILE: earn/earn_test.go

DESCRIPTION:
Contract tests for the Gate Earn "Uni" flexible-lending client against
httptest-served Gate JSON. They pin:
  - public currency parsing (amount bounds + rate band);
  - the POST /earn/uni/lends body shape (currency, amount, type lend/redeem) and
    a no-content success;
  - the PATCH /earn/uni/lends body shape (min_rate, auto_renew);
  - ListUserLends parsing, including a codec.FlexDecimal number-OR-string decode
    on an amount/rate field;
  - GetInterest parsing;
  - Gate {label,message} errors surfacing as *gate.Error.

No network: the parent client's REST base points at the test server.
*/

package earn

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
	"github.com/tonymontanov/go-gate/v2/earn/types"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
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

func newEarnTestClient(t *testing.T, baseURL string) *Client {
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

func TestListCurrencies_ParsesArray(t *testing.T) {
	var gotPath string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `[{"currency":"BTC","min_lend_amount":"0.001","max_lend_amount":"100",
			"available":"42.5","total_lend_available":"1000","min_rate":"0.0001","max_rate":"0.01"}]`)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	var currencies []types.UniCurrency
	var err error
	currencies, err = e.ListCurrencies(context.Background())
	if err != nil {
		t.Fatalf("ListCurrencies: %v", err)
	}
	if gotPath != "/earn/uni/currencies" {
		t.Fatalf("path: %q", gotPath)
	}
	if len(currencies) != 1 {
		t.Fatalf("expected 1 currency, got %d", len(currencies))
	}
	if currencies[0].Currency != "BTC" {
		t.Fatalf("currency: %q", currencies[0].Currency)
	}
	if !currencies[0].MinLendAmount.Equal(mustDec("0.001")) || !currencies[0].MaxLendAmount.Equal(mustDec("100")) {
		t.Fatalf("lend bounds: %s / %s", currencies[0].MinLendAmount, currencies[0].MaxLendAmount)
	}
	if !currencies[0].Available.Equal(mustDec("42.5")) || !currencies[0].MaxRate.Equal(mustDec("0.01")) {
		t.Fatalf("avail/maxrate: %s / %s", currencies[0].Available, currencies[0].MaxRate)
	}
}

func TestCreateLend_BodyShapeAndNoContent(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	var autoRenew bool = true
	var err error
	err = e.CreateLend(context.Background(), types.CreateLendRequest{
		Currency:  "BTC",
		Amount:    mustDec("1.5"),
		Type:      types.LendTypeLend,
		MinRate:   mustDec("0.0002"),
		AutoRenew: &autoRenew,
	})
	if err != nil {
		t.Fatalf("CreateLend: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/earn/uni/lends" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["currency"] != "BTC" || gotBody["type"] != "lend" {
		t.Fatalf("currency/type: %v / %v", gotBody["currency"], gotBody["type"])
	}
	if gotBody["amount"] != "1.5" || gotBody["min_rate"] != "0.0002" {
		t.Fatalf("amount/min_rate: %v / %v", gotBody["amount"], gotBody["min_rate"])
	}
	if gotBody["auto_renew"] != true {
		t.Fatalf("auto_renew: %v", gotBody["auto_renew"])
	}
}

func TestCreateLend_RedeemType(t *testing.T) {
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody = decodeBody(t, r)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	if err := e.CreateLend(context.Background(), types.CreateLendRequest{
		Currency: "USDT",
		Amount:   mustDec("100"),
		Type:     types.LendTypeRedeem,
	}); err != nil {
		t.Fatalf("CreateLend redeem: %v", err)
	}
	if gotBody["type"] != "redeem" || gotBody["amount"] != "100" {
		t.Fatalf("redeem body: type=%v amount=%v", gotBody["type"], gotBody["amount"])
	}
	// auto_renew/min_rate omitted when unset.
	if _, ok := gotBody["auto_renew"]; ok {
		t.Fatalf("auto_renew should be omitted when nil")
	}
	if _, ok := gotBody["min_rate"]; ok {
		t.Fatalf("min_rate should be omitted when zero")
	}
}

func TestChangeLend_PatchBody(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	var autoRenew bool = false
	if err := e.ChangeLend(context.Background(), types.ChangeLendRequest{
		Currency:  "BTC",
		MinRate:   mustDec("0.0005"),
		AutoRenew: &autoRenew,
	}); err != nil {
		t.Fatalf("ChangeLend: %v", err)
	}
	if gotMethod != http.MethodPatch || gotPath != "/earn/uni/lends" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["currency"] != "BTC" || gotBody["min_rate"] != "0.0005" {
		t.Fatalf("currency/min_rate: %v / %v", gotBody["currency"], gotBody["min_rate"])
	}
	if gotBody["auto_renew"] != false {
		t.Fatalf("auto_renew: %v", gotBody["auto_renew"])
	}
}

func TestListUserLends_ParsesArray(t *testing.T) {
	var gotPath, gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = io.WriteString(w, `[{"currency":"BTC","current_amount":"2.0","left_amount":"1.5",
			"frozen_amount":"0.5","min_rate":"0.0001","interest_status":"on","reinvest_left_amount":"0.1",
			"create_time":1700000000,"update_time":1700000100}]`)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	var lends []types.UniLend
	var err error
	lends, err = e.ListUserLends(context.Background(), "BTC", 1, 10)
	if err != nil {
		t.Fatalf("ListUserLends: %v", err)
	}
	if gotPath != "/earn/uni/lends" {
		t.Fatalf("path: %q", gotPath)
	}
	if !strings.Contains(gotQuery, "currency=BTC") || !strings.Contains(gotQuery, "limit=10") {
		t.Fatalf("query: %q", gotQuery)
	}
	if len(lends) != 1 {
		t.Fatalf("expected 1 lend, got %d", len(lends))
	}
	if !lends[0].CurrentAmount.Equal(mustDec("2.0")) || !lends[0].LeftAmount.Equal(mustDec("1.5")) {
		t.Fatalf("amounts: %s / %s", lends[0].CurrentAmount, lends[0].LeftAmount)
	}
	if lends[0].InterestStatus != types.InterestStatusOn || lends[0].CreatedAtMs != 1700000000000 {
		t.Fatalf("status/created: %q / %d", lends[0].InterestStatus, lends[0].CreatedAtMs)
	}
	if lends[0].UpdatedAtMs != 1700000100000 {
		t.Fatalf("updated: %d", lends[0].UpdatedAtMs)
	}
}

// TestUniLendPayload_DecodesNumberAndStringForms guards the Gate behavior where
// the same decimal fields may arrive quoted as strings OR as bare JSON numbers.
// The shared payload must decode BOTH via codec.FlexDecimal.
func TestUniLendPayload_DecodesNumberAndStringForms(t *testing.T) {
	var numberForm = []byte(`[{"currency":"BTC","current_amount":2.0,"left_amount":1.5,"frozen_amount":0.5,
		"min_rate":0.0001,"interest_status":"on","reinvest_left_amount":0.1,"create_time":1700000000}]`)
	var stringForm = []byte(`[{"currency":"BTC","current_amount":"2.0","left_amount":"1.5","frozen_amount":"0.5",
		"min_rate":"0.0001","interest_status":"on","reinvest_left_amount":"0.1","create_time":1700000000}]`)

	var form string
	var raw []byte
	for form, raw = range map[string][]byte{"number": numberForm, "string": stringForm} {
		var payloads []uniLendPayload
		if err := codec.Unmarshal(raw, &payloads); err != nil {
			t.Fatalf("%s: unmarshal failed: %v", form, err)
		}
		if len(payloads) != 1 {
			t.Fatalf("%s: want 1 payload, got %d", form, len(payloads))
		}
		var lend types.UniLend = uniLendFromPayload(&payloads[0], nil)
		if !lend.CurrentAmount.Equal(mustDec("2")) || !lend.LeftAmount.Equal(mustDec("1.5")) {
			t.Fatalf("%s: amounts: %s / %s", form, lend.CurrentAmount, lend.LeftAmount)
		}
		if !lend.MinRate.Equal(mustDec("0.0001")) {
			t.Fatalf("%s: min_rate: %s", form, lend.MinRate)
		}
	}
}

func TestGetInterest_Parses(t *testing.T) {
	var gotPath string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{"currency":"BTC","interest":"0.0123"}`)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	var interest types.Interest
	var err error
	interest, err = e.GetInterest(context.Background(), "BTC")
	if err != nil {
		t.Fatalf("GetInterest: %v", err)
	}
	if gotPath != "/earn/uni/interests/BTC" {
		t.Fatalf("path: %q", gotPath)
	}
	if interest.Currency != "BTC" || !interest.Interest.Equal(mustDec("0.0123")) {
		t.Fatalf("interest: %q / %s", interest.Currency, interest.Interest)
	}
}

func TestListRate_Parses(t *testing.T) {
	var gotPath, gotQuery string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = io.WriteString(w, `{"currency":"BTC","estimate_rate":"0.0088"}`)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	var rate types.RatePoint
	var err error
	rate, err = e.ListRate(context.Background(), "BTC")
	if err != nil {
		t.Fatalf("ListRate: %v", err)
	}
	if gotPath != "/earn/uni/rate" || !strings.Contains(gotQuery, "currency=BTC") {
		t.Fatalf("path/query: %q / %q", gotPath, gotQuery)
	}
	if !rate.EstimateRate.Equal(mustDec("0.0088")) {
		t.Fatalf("estimate_rate: %s", rate.EstimateRate)
	}
}

func TestCreateLend_ErrorLabel_Surfaces(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"label":"INVALID_PARAM_VALUE","message":"amount too small"}`)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	var err error
	err = e.CreateLend(context.Background(), types.CreateLendRequest{
		Currency: "BTC", Amount: mustDec("1"), Type: types.LendTypeLend,
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ge *gate.Error
	if !errors.As(err, &ge) || ge.Label != "INVALID_PARAM_VALUE" || !gate.IsInvalidRequest(err) {
		t.Fatalf("unexpected error: %v", err)
	}
}
