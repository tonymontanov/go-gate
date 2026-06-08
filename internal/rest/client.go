/*
FILE: internal/rest/client.go

DESCRIPTION:
Low-level SDK REST client for Gate APIv4. A thin layer over http.Client that:
  1. assembles the URL (BaseURL + path + query);
  2. signs the request (Signer) — KEY / SIGN / Timestamp headers;
  3. executes the HTTP call with a deadline from ctx or Config.RequestTimeout;
  4. collects Gate rate-limit response headers and notifies the observer;
  5. returns the raw success body (Gate has NO {code,msg,data} envelope — the
     payload IS the resource), or maps a Gate error body {label,message} to
     *gateerr.Error with the correct category.

GATE RESPONSE SHAPE (differs from OKX/Binance):
  - Success: HTTP 2xx, body is the resource JSON directly (object or array).
    Response.UnmarshalData unmarshals that raw body into the caller's dest.
  - Failure: HTTP 4xx/5xx, body is {"label":"...","message":"...","detail":"..."}.
    The label is the authoritative error signal (see gateerr.MapLabel); the HTTP
    status is the fallback. Batch endpoints return 2xx with per-element status in
    the array — that partial-failure handling lives in the domain layer, not here.

RATE LIMITS (differs from OKX):
  Gate DOES return rate-limit response headers (X-Gate-RateLimit-*). They are
  collected per call and delivered to RateLimitEventObserver alongside the
  request metadata (OrderCount / Symbols / Category) the domain layer supplies.

IMPORT NOTE:
  Does NOT import the root gate package (avoids an import cycle). Error/Logger
  types come from internal/gateerr and internal/gatelog.
*/

package rest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tonymontanov/go-gate/v2/internal/auth"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
	"github.com/tonymontanov/go-gate/v2/internal/gateerr"
	"github.com/tonymontanov/go-gate/v2/internal/gatelog"
)

// Config — REST transport parameters. Populated from the public gate.RestConfig
// in the root package (explicit struct conversion is done there to avoid an
// import cycle).
type Config struct {
	RequestTimeout      time.Duration
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
	// ChannelID — optional broker/channel id sent as the X-Gate-Channel-Id
	// header on every request. Empty → header omitted.
	ChannelID string
	// RateLimitEventObserver — optional callback (nil → no-op) invoked
	// SYNCHRONOUSLY once per completed REST call with the request metadata and
	// the Gate rate-limit headers parsed from the response. The root gate.Client
	// converts these flat arguments into the public gate.RateLimitEvent.
	//
	// Speed contract: runs in the calling goroutine and blocks the return from
	// Do. Implementations must be O(1) (typically a non-blocking channel send).
	RateLimitEventObserver func(endpoint, method string, headers map[string]string, meta RequestMeta)
}

// RequestMeta — request metadata known at the domain layer (futures/trading.go,
// futures/account.go) that an external rate-limiter needs for accurate Gate
// limit tracking. Forwarded through Options to RateLimitEventObserver. If empty,
// the observer receives zero values.
type RequestMeta struct {
	// OrderCount — number of orders affected by the request. 1 for single, N for
	// batch, 0 for non-trading.
	OrderCount int
	// Symbols — Gate contracts the request applies to (e.g. "BTC_USDT").
	Symbols []string
	// Category — string form of gate.RateLimitCategory
	// ("place"/"amend"/"cancel"/"query"/"market"/""). String to avoid an import
	// cycle internal/rest ↔ root gate.
	Category string
}

// Options — parameters for a single REST request.
type Options struct {
	Method string
	Path   string
	Query  url.Values
	Body   any
	Signed bool
	Meta   RequestMeta
}

// Response — Gate REST response. Holds the raw success body and the HTTP status.
// Gate has no envelope, so the payload is unmarshaled directly into the caller's
// destination type.
type Response struct {
	status int
	raw    []byte
}

// Status returns the HTTP status code of the response.
func (r Response) Status() int { return r.status }

// Raw returns the raw response body bytes. The slice is owned by the caller.
func (r Response) Raw() []byte { return r.raw }

// UnmarshalData unmarshals the raw response body into dest. An empty body or the
// literal null is treated as no-op (dest left unchanged).
func (r Response) UnmarshalData(dest any) error {
	if len(r.raw) == 0 || bytes.Equal(bytes.TrimSpace(r.raw), []byte("null")) {
		return nil
	}
	return codec.Unmarshal(r.raw, dest)
}

// gateErrorBody — Gate APIv4 error envelope.
type gateErrorBody struct {
	Label   string `json:"label"`
	Message string `json:"message"`
	Detail  string `json:"detail"`
}

// Client — low-level REST client.
type Client struct {
	httpClient             *http.Client
	signer                 *auth.Signer
	baseURL                string
	userAgent              string
	channelID              string
	logger                 gatelog.Logger
	rateLimitEventObserver func(endpoint, method string, headers map[string]string, meta RequestMeta)
}

// NewClient creates a REST client.
func NewClient(baseURL string, signer *auth.Signer, cfg Config, ua string, log gatelog.Logger) *Client {
	if log == nil {
		log = gatelog.Noop()
	}
	var transport *http.Transport = &http.Transport{
		MaxIdleConns:        cfg.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
		IdleConnTimeout:     cfg.IdleConnTimeout,
		ForceAttemptHTTP2:   true,
	}
	var httpClient *http.Client = &http.Client{
		Timeout:   cfg.RequestTimeout,
		Transport: transport,
	}
	return &Client{
		httpClient:             httpClient,
		signer:                 signer,
		baseURL:                strings.TrimRight(baseURL, "/"),
		userAgent:              ua,
		channelID:              cfg.ChannelID,
		logger:                 log,
		rateLimitEventObserver: cfg.RateLimitEventObserver,
	}
}

// Close closes idle transport connections.
func (c *Client) Close() {
	if c == nil || c.httpClient == nil {
		return
	}
	if t, ok := c.httpClient.Transport.(*http.Transport); ok {
		t.CloseIdleConnections()
	}
}

/*
Do executes a single REST call and returns the Response, the Gate rate-limit
headers parsed from the response (always non-nil; empty if none present), and an
error.

Error semantics:
  - transport/ctx failures → *gateerr.Error{Kind: Network};
  - HTTP 2xx → success, error is nil, Response.raw holds the payload;
  - HTTP non-2xx → *gateerr.Error with Kind from the Gate label (MapLabel),
    falling back to the HTTP status (MapHTTPStatus).
*/
func (c *Client) Do(ctx context.Context, opts Options) (Response, map[string]string, error) {
	var resp Response

	var u *url.URL
	var bodyStr string
	var err error
	u, bodyStr, err = c.buildRequest(opts)
	if err != nil {
		return resp, map[string]string{}, err
	}

	var method string = strings.ToUpper(opts.Method)
	var req *http.Request
	req, err = http.NewRequestWithContext(ctx, method, u.String(), bytes.NewBufferString(bodyStr))
	if err != nil {
		return resp, map[string]string{}, gateerr.New(gateerr.ErrorKindInvalidRequest, "", "rest: build request", err)
	}
	c.applyHeaders(req, u, opts, bodyStr)

	var httpResp *http.Response
	var started time.Time = time.Now()
	httpResp, err = c.httpClient.Do(req)
	if err != nil {
		return resp, map[string]string{}, classifyTransportError(err)
	}
	defer func() {
		_ = httpResp.Body.Close()
	}()

	var rateLimits map[string]string = collectRateLimitHeaders(httpResp.Header)

	// Notify the observer BEFORE parsing the body: even an unparseable or
	// error response carries fresh rate-limit headers the external limiter must
	// account for (together with the request metadata from Options.Meta).
	if c.rateLimitEventObserver != nil {
		c.rateLimitEventObserver(opts.Path, method, rateLimits, opts.Meta)
	}

	var raw []byte
	raw, err = io.ReadAll(httpResp.Body)
	if err != nil {
		return resp, rateLimits, gateerr.New(gateerr.ErrorKindNetwork, "", "rest: read body", err)
	}

	c.logger.Debug(
		"rest.Do",
		gatelog.Str("method", method),
		gatelog.Str("path", opts.Path),
		gatelog.Int("status", int64(httpResp.StatusCode)),
		gatelog.Int("durationMs", time.Since(started).Milliseconds()),
		gatelog.Int("bytes", int64(len(raw))),
	)

	resp.status = httpResp.StatusCode
	resp.raw = raw

	if httpResp.StatusCode >= 200 && httpResp.StatusCode < 300 {
		return resp, rateLimits, nil
	}

	// Non-2xx: map the Gate error body. Label first, HTTP status as fallback.
	var ge gateErrorBody
	if codec.Unmarshal(raw, &ge) == nil && (ge.Label != "" || ge.Message != "") {
		var kind gateerr.ErrorKind = gateerr.MapLabel(ge.Label, httpResp.StatusCode)
		if kind == gateerr.ErrorKindUnknown {
			kind = gateerr.MapHTTPStatus(httpResp.StatusCode)
		}
		var msg string = ge.Message
		if ge.Detail != "" {
			msg = msg + " (" + ge.Detail + ")"
		}
		return resp, rateLimits, &gateerr.Error{
			Kind:       kind,
			HTTPStatus: httpResp.StatusCode,
			Label:      ge.Label,
			Message:    msg,
		}
	}
	return resp, rateLimits, &gateerr.Error{
		Kind:       gateerr.MapHTTPStatus(httpResp.StatusCode),
		HTTPStatus: httpResp.StatusCode,
		Message:    truncate(string(raw), 256),
	}
}

// buildRequest assembles the URL and serialized body.
func (c *Client) buildRequest(opts Options) (*url.URL, string, error) {
	var u *url.URL
	var err error
	u, err = url.Parse(c.baseURL + opts.Path)
	if err != nil {
		return nil, "", gateerr.New(gateerr.ErrorKindInvalidRequest, "", "rest: invalid url", err)
	}
	if len(opts.Query) > 0 {
		u.RawQuery = opts.Query.Encode()
	}

	var body string
	if opts.Body != nil {
		var raw []byte
		raw, err = codec.Marshal(opts.Body)
		if err != nil {
			return nil, "", gateerr.New(gateerr.ErrorKindInvalidRequest, "", "rest: marshal body", err)
		}
		body = string(raw)
	}
	return u, body, nil
}

// applyHeaders sets standard and (when requested) signed headers. Signing uses
// the full URL path (including the /api/v4 prefix) and the URL-UNESCAPED query
// string, exactly as Gate verifies it.
func (c *Client) applyHeaders(req *http.Request, u *url.URL, opts Options, body string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	if c.channelID != "" {
		req.Header.Set("X-Gate-Channel-Id", c.channelID)
	}

	if !opts.Signed {
		return
	}
	if c.signer == nil || !c.signer.Enabled() {
		return
	}

	var ts string = c.signer.Timestamp(time.Now())
	// Gate signs the unescaped query string; url.QueryUnescape mirrors how the
	// server reconstructs it from the encoded RawQuery.
	var signQuery string = u.RawQuery
	if signQuery != "" {
		var unescaped string
		var err error
		unescaped, err = url.QueryUnescape(signQuery)
		if err == nil {
			signQuery = unescaped
		}
	}

	var signature string
	var err error
	signature, err = c.signer.Sign(strings.ToUpper(opts.Method), u.Path, signQuery, body, ts)
	if err != nil {
		c.logger.Warn("rest: sign skipped", gatelog.Err(err))
		return
	}
	req.Header.Set("KEY", c.signer.APIKey())
	req.Header.Set("SIGN", signature)
	req.Header.Set("Timestamp", ts)
}

// rateLimitHeaderPrefix is the canonical (http.Header) prefix of Gate's
// rate-limit response headers, e.g. "X-Gate-Ratelimit-Requests-Remain".
const rateLimitHeaderPrefix = "X-Gate-Ratelimit"

// collectRateLimitHeaders extracts Gate rate-limit headers into a non-nil map.
// Matching by prefix is future-proof against Gate adding new X-Gate-RateLimit-*
// counters. Keys are kept in their canonical http.Header form.
func collectRateLimitHeaders(h http.Header) map[string]string {
	var out map[string]string = make(map[string]string, 4)
	for k, v := range h {
		if len(v) == 0 {
			continue
		}
		if len(k) >= len(rateLimitHeaderPrefix) && strings.EqualFold(k[:len(rateLimitHeaderPrefix)], rateLimitHeaderPrefix) {
			out[k] = v[0]
		}
	}
	return out
}

// classifyTransportError converts a network/ctx error into a *gateerr.Error.
func classifyTransportError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return gateerr.New(gateerr.ErrorKindNetwork, "", "rest: context canceled", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return gateerr.New(gateerr.ErrorKindNetwork, "", "rest: deadline exceeded", err)
	}
	return gateerr.New(gateerr.ErrorKindNetwork, "", "rest: transport error", err)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
