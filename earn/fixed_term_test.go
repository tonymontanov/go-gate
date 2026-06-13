/*
FILE: earn/fixed_term_test.go

DESCRIPTION:
Contract tests for the Gate Earn Fixed-Term sub-client (earn.Client.FixedTerm())
against httptest-served Gate JSON. They pin:
  - public ListProducts parsing (base "/earn/fixed-term/product", apr band,
    amount bounds, duration, ladder tiers);
  - the POST /earn/fixed-term/user/lend body shape (pid, amount) and the parse of
    the returned lend;
  - that a Gate {label,message} error surfaces as *gate.Error.

These reuse the newEarnTestClient harness from earn_test.go (same package).
No network: the parent client's REST base points at the test server.
*/

package earn

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/earn/types"
)

func TestFixedTerm_ListProducts_ParsesArray(t *testing.T) {
	var gotPath string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `[{"pid":"USDT30D","asset":"USDT","type":"fixed","apr":"0.08",
			"min_apr":"0.06","max_apr":"0.10","min_amount":"10","max_amount":"100000",
			"duration_days":30,"start_time":1700000000,"end_time":1700600000,"status":"in_process",
			"tiers":[{"min_amount":"10","max_amount":"1000","apr":"0.06"},
			{"min_amount":"1000","max_amount":"100000","apr":"0.10"}]}]`)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	var products []types.FixedTermProduct
	var err error
	products, err = e.FixedTerm().ListProducts(context.Background())
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if gotPath != "/earn/fixed-term/product" {
		t.Fatalf("path: %q", gotPath)
	}
	if len(products) != 1 {
		t.Fatalf("expected 1 product, got %d", len(products))
	}
	if products[0].ID != "USDT30D" || products[0].Asset != "USDT" {
		t.Fatalf("id/asset: %q / %q", products[0].ID, products[0].Asset)
	}
	if !products[0].MinAPR.Equal(mustDec("0.06")) || !products[0].MaxAPR.Equal(mustDec("0.10")) {
		t.Fatalf("apr band: %s / %s", products[0].MinAPR, products[0].MaxAPR)
	}
	if !products[0].MaxAmount.Equal(mustDec("100000")) || products[0].DurationDays != 30 {
		t.Fatalf("maxAmount/duration: %s / %d", products[0].MaxAmount, products[0].DurationDays)
	}
	if products[0].StartTimeMs != 1700000000000 || products[0].EndTimeMs != 1700600000000 {
		t.Fatalf("times: %d / %d", products[0].StartTimeMs, products[0].EndTimeMs)
	}
	if len(products[0].Tiers) != 2 || !products[0].Tiers[1].APR.Equal(mustDec("0.10")) {
		t.Fatalf("tiers: %+v", products[0].Tiers)
	}
}

func TestFixedTerm_ListProductsByAsset_Path(t *testing.T) {
	var gotPath string
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `[{"pid":"BTC90D","asset":"BTC","apr":"0.05","min_amount":"0.01"}]`)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	var products []types.FixedTermProduct
	var err error
	products, err = e.FixedTerm().ListProductsByAsset(context.Background(), "BTC")
	if err != nil {
		t.Fatalf("ListProductsByAsset: %v", err)
	}
	if gotPath != "/earn/fixed-term/product/BTC/list" {
		t.Fatalf("path: %q", gotPath)
	}
	if len(products) != 1 || products[0].ID != "BTC90D" {
		t.Fatalf("products: %+v", products)
	}
}

func TestFixedTerm_CreateLend_BodyAndParse(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		_, _ = io.WriteString(w, `{"id":987654,"pid":"USDT30D","asset":"USDT","amount":"500","apr":"0.08",
			"create_time":1700000000,"settle_time":1700300000,"redeem_time":1702592000,"status":"holding"}`)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	var lend types.FixedTermLend
	var err error
	lend, err = e.FixedTerm().CreateLend(context.Background(), types.CreateFixedLendRequest{
		ProductID: "USDT30D",
		Amount:    mustDec("500"),
	})
	if err != nil {
		t.Fatalf("CreateLend: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/earn/fixed-term/user/lend" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["pid"] != "USDT30D" || gotBody["amount"] != "500" {
		t.Fatalf("body pid/amount: %v / %v", gotBody["pid"], gotBody["amount"])
	}
	if lend.ID != "987654" || lend.ProductID != "USDT30D" {
		t.Fatalf("lend id/pid: %q / %q", lend.ID, lend.ProductID)
	}
	if !lend.Amount.Equal(mustDec("500")) || !lend.APR.Equal(mustDec("0.08")) {
		t.Fatalf("amount/apr: %s / %s", lend.Amount, lend.APR)
	}
	if lend.CreatedAtMs != 1700000000000 || lend.RedeemTimeMs != 1702592000000 || lend.Status != "holding" {
		t.Fatalf("created/redeem/status: %d / %d / %q", lend.CreatedAtMs, lend.RedeemTimeMs, lend.Status)
	}
}

func TestFixedTerm_CreateLend_Validation(t *testing.T) {
	var e *Client = newEarnTestClient(t, "http://127.0.0.1:0")
	var err error
	_, err = e.FixedTerm().CreateLend(context.Background(), types.CreateFixedLendRequest{Amount: mustDec("1")})
	if err == nil || !gate.IsInvalidRequest(err) {
		t.Fatalf("expected invalid-request for empty ProductID, got %v", err)
	}
	_, err = e.FixedTerm().CreateLend(context.Background(), types.CreateFixedLendRequest{ProductID: "X"})
	if err == nil || !gate.IsInvalidRequest(err) {
		t.Fatalf("expected invalid-request for non-positive Amount, got %v", err)
	}
}

func TestFixedTerm_CreatePreRedeem_Body(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotBody = decodeBody(t, r)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	if err := e.FixedTerm().CreatePreRedeem(context.Background(), types.PreRedeemRequest{ID: "987654"}); err != nil {
		t.Fatalf("CreatePreRedeem: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/earn/fixed-term/user/pre-redeem" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody["id"] != "987654" {
		t.Fatalf("body id: %v", gotBody["id"])
	}
}

func TestFixedTerm_CreateLend_ErrorLabel_Surfaces(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"label":"INVALID_PARAM_VALUE","message":"amount too small"}`)
	}))
	defer srv.Close()

	var e *Client = newEarnTestClient(t, srv.URL)
	var err error
	_, err = e.FixedTerm().CreateLend(context.Background(), types.CreateFixedLendRequest{
		ProductID: "USDT30D", Amount: mustDec("1"),
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ge *gate.Error
	if !errors.As(err, &ge) || ge.Label != "INVALID_PARAM_VALUE" || !gate.IsInvalidRequest(err) {
		t.Fatalf("unexpected error: %v", err)
	}
}
