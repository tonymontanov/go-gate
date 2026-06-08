/*
FILE: internal/rest/client_test.go

DESCRIPTION:
REST transport tests against httptest.Server (no network). Cover:
  - success path: Gate returns the resource JSON directly (no envelope), and
    UnmarshalData parses it;
  - signed requests set KEY/SIGN/Timestamp, and SIGN matches an independent
    recomputation of the Gate signString;
  - error path: a Gate {label,message} body maps to *gateerr.Error with the
    category derived from the label;
  - rate-limit headers are collected and the RateLimitEventObserver fires once
    with the request metadata.
*/

package rest

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/tonymontanov/go-gate/v2/internal/auth"
	"github.com/tonymontanov/go-gate/v2/internal/gateerr"
)

const (
	testKey    = "test-key"
	testSecret = "test-secret"
)

func newTestClient(t *testing.T, baseURL string, cfg Config) *Client {
	t.Helper()
	var signer *auth.Signer = auth.NewSigner(testKey, testSecret)
	return NewClient(baseURL, signer, cfg, "go-gate-test", nil)
}

func TestDo_Success_NoEnvelope(t *testing.T) {
	// Gate returns the resource directly (here, an array) — no {code,msg,data}.
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"id":1,"contract":"BTC_USDT"},{"id":2,"contract":"ETH_USDT"}]`)
	}))
	defer srv.Close()

	var c *Client = newTestClient(t, srv.URL, Config{})

	var resp Response
	var err error
	resp, _, err = c.Do(context.Background(), Options{Method: "GET", Path: "/futures/usdt/orders"})
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	type order struct {
		ID       int64  `json:"id"`
		Contract string `json:"contract"`
	}
	var orders []order
	if err = resp.UnmarshalData(&orders); err != nil {
		t.Fatalf("UnmarshalData failed: %v", err)
	}
	if len(orders) != 2 || orders[0].Contract != "BTC_USDT" || orders[1].ID != 2 {
		t.Fatalf("unexpected parse result: %+v", orders)
	}
}

func TestDo_SignedHeaders_MatchReference(t *testing.T) {
	var gotKey, gotSign, gotTs string
	var gotMethod, gotPath, gotRawQuery, gotBody string

	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("KEY")
		gotSign = r.Header.Get("SIGN")
		gotTs = r.Header.Get("Timestamp")
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotRawQuery = r.URL.RawQuery
		var b []byte
		b, _ = io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = io.WriteString(w, `{"id":"42"}`)
	}))
	defer srv.Close()

	var c *Client = newTestClient(t, srv.URL, Config{})

	var q url.Values = url.Values{}
	q.Set("contract", "BTC_USDT")
	var _, _, err = c.Do(context.Background(), Options{
		Method: "POST",
		Path:   "/futures/usdt/orders",
		Query:  q,
		Body:   map[string]any{"contract": "BTC_USDT", "size": 1},
		Signed: true,
	})
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	if gotKey != testKey {
		t.Fatalf("KEY header mismatch: got %q", gotKey)
	}
	if gotTs == "" || gotSign == "" {
		t.Fatalf("missing SIGN/Timestamp headers")
	}

	// Recompute the expected signature independently. Gate signs the unescaped
	// query string.
	var signQuery string = gotRawQuery
	if unescaped, uerr := url.QueryUnescape(gotRawQuery); uerr == nil {
		signQuery = unescaped
	}
	var payload [sha512.Size]byte = sha512.Sum512([]byte(gotBody))
	var signString string = gotMethod + "\n" + gotPath + "\n" + signQuery + "\n" +
		hex.EncodeToString(payload[:]) + "\n" + gotTs
	var mac = hmac.New(sha512.New, []byte(testSecret))
	mac.Write([]byte(signString))
	var want string = hex.EncodeToString(mac.Sum(nil))

	if gotSign != want {
		t.Fatalf("SIGN mismatch:\n got=%q\nwant=%q", gotSign, want)
	}
}

func TestDo_ErrorLabel_MapsToKind(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"label":"INVALID_PARAM_VALUE","message":"invalid contract","detail":"contract=FOO"}`)
	}))
	defer srv.Close()

	var c *Client = newTestClient(t, srv.URL, Config{})

	var _, _, err = c.Do(context.Background(), Options{Method: "GET", Path: "/futures/usdt/orders"})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var ge *gateerr.Error
	if !errors.As(err, &ge) {
		t.Fatalf("expected *gateerr.Error, got %T", err)
	}
	if ge.Kind != gateerr.ErrorKindInvalidRequest {
		t.Fatalf("expected InvalidRequest, got %s", ge.Kind)
	}
	if ge.Label != "INVALID_PARAM_VALUE" || ge.HTTPStatus != 400 {
		t.Fatalf("unexpected error fields: %+v", ge)
	}
}

func TestDo_RateLimitHeaders_AndObserver(t *testing.T) {
	var srv *httptest.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Gate-RateLimit-Requests-Remain", "9")
		w.Header().Set("X-Gate-RateLimit-Limit", "10")
		w.Header().Set("X-Gate-RateLimit-Reset-Timestamp", "1700000000000")
		_, _ = io.WriteString(w, `{"id":"1"}`)
	}))
	defer srv.Close()

	var observed int
	var gotHeaders map[string]string
	var gotMeta RequestMeta
	var cfg Config = Config{
		RateLimitEventObserver: func(endpoint, method string, headers map[string]string, meta RequestMeta) {
			observed++
			gotHeaders = headers
			gotMeta = meta
		},
	}
	var c *Client = newTestClient(t, srv.URL, cfg)

	var resp Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.Do(context.Background(), Options{
		Method: "POST",
		Path:   "/futures/usdt/orders",
		Body:   map[string]any{"x": 1},
		Signed: true,
		Meta:   RequestMeta{OrderCount: 1, Symbols: []string{"BTC_USDT"}, Category: "place"},
	})
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	_ = resp

	if observed != 1 {
		t.Fatalf("observer must fire exactly once, fired %d", observed)
	}
	if gotMeta.OrderCount != 1 || len(gotMeta.Symbols) != 1 || gotMeta.Category != "place" {
		t.Fatalf("observer meta mismatch: %+v", gotMeta)
	}
	// Headers collected by prefix, canonicalized by net/http.
	var remain string = findHeaderValue(rateLimits, "requests-remain")
	if remain != "9" {
		t.Fatalf("expected requests-remain=9, got %q (all=%v)", remain, rateLimits)
	}
	if len(gotHeaders) != 3 {
		t.Fatalf("expected 3 rate-limit headers, got %d: %v", len(gotHeaders), gotHeaders)
	}
}

// findHeaderValue does a case-insensitive suffix match over collected headers,
// independent of net/http canonicalization of "RateLimit".
func findHeaderValue(headers map[string]string, suffix string) string {
	for k, v := range headers {
		if len(k) >= len(suffix) {
			var tail string = k[len(k)-len(suffix):]
			if equalFold(tail, suffix) {
				return v
			}
		}
	}
	return ""
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var i int
	for i = 0; i < len(a); i++ {
		var ca, cb byte = a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
