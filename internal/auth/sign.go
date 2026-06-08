/*
FILE: internal/auth/sign.go

DESCRIPTION:
sign.go implements request signing for Gate APIv4 REST. Algorithm per the
official documentation and the gateapi-go reference implementation:

	hashedPayload = hex( SHA512(body) )
	signString    = method + "\n" + path + "\n" + rawQuery + "\n" + hashedPayload + "\n" + timestamp
	SIGN          = hex( HMAC_SHA512(secretKey, signString) )

Where:
  - method        — UPPER-CASE HTTP method ("GET" / "POST" / "PUT" / "DELETE").
  - path          — full URL path including the "/api/v4" prefix,
                    e.g. "/api/v4/futures/usdt/orders".
  - rawQuery      — the URL-UNESCAPED query string (no leading '?'),
                    e.g. "contract=BTC_USDT&limit=10". Empty for requests with
                    no query. Gate verifies the unescaped form, matching
                    url.QueryUnescape applied to the encoded query.
  - body          — the exact JSON request body for POST/PUT; empty string for
                    GET/DELETE without a body. SHA512("") is a well-defined
                    constant, so the empty-body case needs no special casing.
  - timestamp     — Unix time in SECONDS as a decimal string.

Three headers are sent with signed REST requests:

	KEY:       apiKey
	SIGN:      SIGN (see above)
	Timestamp: timestamp

SECURITY NOTES:
  - SecretKey is stored inside Signer and is never serialized; String() redacts.
  - Do not log signString or request bodies — that leaks secret-adjacent data.

DEPENDENCIES:
- crypto/hmac, crypto/sha512: signing and payload hashing.
- encoding/hex:               output encoding.
- errors, strconv, time:      errors and timestamp formatting.
*/

package auth

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

// ErrSignerDisabled is returned when Sign is called with empty credentials.
var ErrSignerDisabled = errors.New("auth: signer is disabled (api key/secret not configured)")

// Signer — compact Gate request signer. Safe for concurrent use: contains only
// read-only fields.
type Signer struct {
	apiKey    string
	secretKey []byte
	enabled   bool
}

// NewSigner creates a Signer. If either field is empty, the signer is marked
// disabled (Sign returns ErrSignerDisabled). This lets the same Client serve
// public Gate endpoints without credentials.
func NewSigner(apiKey, secretKey string) *Signer {
	var enabled bool = apiKey != "" && secretKey != ""
	return &Signer{
		apiKey:    apiKey,
		secretKey: []byte(secretKey),
		enabled:   enabled,
	}
}

// Enabled returns true if the signer is ready to sign requests.
func (s *Signer) Enabled() bool { return s != nil && s.enabled }

// APIKey returns the API key (for the KEY header).
func (s *Signer) APIKey() string {
	if s == nil {
		return ""
	}
	return s.apiKey
}

/*
Sign returns hex(HMAC_SHA512(secret, signString)) per Gate APIv4 specification.

Parameters:
  - method:    UPPER-CASE HTTP method, "GET" / "POST" / "PUT" / "DELETE".
  - path:      full request path including the "/api/v4" prefix.
  - rawQuery:  URL-UNESCAPED query string without the leading '?' (may be empty).
  - body:      JSON body for POST/PUT; empty string for GET/DELETE without a body.
  - timestamp: Unix seconds as a decimal string (see Timestamp).

Returns the hex signature string, or ErrSignerDisabled if the signer is disabled.
*/
func (s *Signer) Sign(method, path, rawQuery, body, timestamp string) (string, error) {
	if !s.Enabled() {
		return "", ErrSignerDisabled
	}

	// hashedPayload = hex(SHA512(body)). SHA512("") is well-defined, so an empty
	// body yields the canonical empty-string digest without special casing.
	var payload [sha512.Size]byte = sha512.Sum512([]byte(body))
	var hashedPayload string = hex.EncodeToString(payload[:])

	var sb strings.Builder
	sb.Grow(len(method) + len(path) + len(rawQuery) + len(hashedPayload) + len(timestamp) + 4)
	sb.WriteString(method)
	sb.WriteByte('\n')
	sb.WriteString(path)
	sb.WriteByte('\n')
	sb.WriteString(rawQuery)
	sb.WriteByte('\n')
	sb.WriteString(hashedPayload)
	sb.WriteByte('\n')
	sb.WriteString(timestamp)

	var mac = hmac.New(sha512.New, s.secretKey)
	mac.Write([]byte(sb.String()))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// Timestamp formats now as Unix seconds (decimal string), the format Gate
// expects in the Timestamp header and in the signString. If now is zero,
// time.Now() is used.
func (s *Signer) Timestamp(now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	return strconv.FormatInt(now.Unix(), 10)
}

/*
SignWS returns the signature for a Gate WebSocket channel subscription:

	SIGN = hex(HMAC_SHA512(secret, "channel=<channel>&event=<event>&time=<ts>"))

ts is Unix seconds. The result goes into the subscribe message's auth object
({method:"api_key", KEY, SIGN}). Returns ErrSignerDisabled if the signer is
disabled.
*/
func (s *Signer) SignWS(channel, event string, ts int64) (string, error) {
	if !s.Enabled() {
		return "", ErrSignerDisabled
	}
	var msg string = "channel=" + channel + "&event=" + event + "&time=" + strconv.FormatInt(ts, 10)
	var mac = hmac.New(sha512.New, s.secretKey)
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// String returns a log-safe representation of the Signer — without secrets.
func (s *Signer) String() string {
	if s == nil || !s.enabled {
		return "auth.Signer{disabled}"
	}
	return "auth.Signer{enabled, apiKey=" + redact(s.apiKey) + "}"
}

// redact turns a string into "abcd…wxyz" — first/last 4 characters.
// Used for logging only.
func redact(s string) string {
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "…" + s[len(s)-4:]
}
