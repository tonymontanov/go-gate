/*
FILE: internal/auth/sign_test.go

DESCRIPTION:
Gate signing tests. Cover:
  - disabled-mode behavior (empty keys) and ErrSignerDisabled;
  - the signString layout (method\npath\nrawQuery\nhex(sha512(body))\nts) and
    hex(HMAC_SHA512) output, computed independently;
  - a known-answer check that the empty body hashes to the canonical SHA512("")
    digest — this pins the algorithm to Gate's, not merely to itself;
  - signature dependence on body and query;
  - Timestamp() format (Unix seconds).
*/

package auth

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"testing"
	"time"
)

// sha512EmptyHex is the canonical hex digest of SHA512 over an empty input.
// Gate hashes the (possibly empty) request body with SHA512; this constant
// pins our implementation to the same well-known value.
const sha512EmptyHex = "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce" +
	"47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e"

func TestSigner_Disabled_NoSign(t *testing.T) {
	var s *Signer = NewSigner("", "")
	if s.Enabled() {
		t.Fatalf("signer must be disabled when keys are empty")
	}
	var sig string
	var err error
	sig, err = s.Sign("GET", "/api/v4/futures/usdt/orders", "", "", "1700000000")
	if !errors.Is(err, ErrSignerDisabled) {
		t.Fatalf("expected ErrSignerDisabled, got %v", err)
	}
	if sig != "" {
		t.Fatalf("signature must be empty when disabled")
	}
}

func TestSigner_Sign_MatchesReference(t *testing.T) {
	var secret string = "test-secret-key"
	var s *Signer = NewSigner("test-api-key", secret)
	if !s.Enabled() {
		t.Fatalf("signer must be enabled")
	}

	var method string = "POST"
	var path string = "/api/v4/futures/usdt/orders"
	var rawQuery string = ""
	var body string = `{"contract":"BTC_USDT","size":1,"price":"30000"}`
	var ts string = "1700000000"

	var got string
	var err error
	got, err = s.Sign(method, path, rawQuery, body, ts)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Reference computed independently from the implementation under test.
	var payload [sha512.Size]byte = sha512.Sum512([]byte(body))
	var signString string = method + "\n" + path + "\n" + rawQuery + "\n" +
		hex.EncodeToString(payload[:]) + "\n" + ts
	var mac = hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(signString))
	var want string = hex.EncodeToString(mac.Sum(nil))

	if got != want {
		t.Fatalf("signature mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestSigner_EmptyBody_KnownDigest(t *testing.T) {
	// Verify the empty-body branch uses the canonical SHA512("") digest by
	// reconstructing the expected signature with that constant inlined.
	var secret string = "s"
	var s *Signer = NewSigner("k", secret)

	var method string = "GET"
	var path string = "/api/v4/futures/usdt/positions"
	var rawQuery string = "holding=true"
	var ts string = "1700000001"

	var got string
	var err error
	got, err = s.Sign(method, path, rawQuery, "", ts)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	var signString string = method + "\n" + path + "\n" + rawQuery + "\n" + sha512EmptyHex + "\n" + ts
	var mac = hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(signString))
	var want string = hex.EncodeToString(mac.Sum(nil))

	if got != want {
		t.Fatalf("empty-body signature mismatch:\n got=%q\nwant=%q", got, want)
	}
}

func TestSigner_Sign_DependsOnBodyAndQuery(t *testing.T) {
	var s *Signer = NewSigner("k", "s")
	var sigA, sigB, sigC string
	var err error
	sigA, err = s.Sign("POST", "/p", "", `{"a":1}`, "ts")
	if err != nil {
		t.Fatal(err)
	}
	sigB, err = s.Sign("POST", "/p", "", `{"a":2}`, "ts")
	if err != nil {
		t.Fatal(err)
	}
	sigC, err = s.Sign("GET", "/p", "x=1", "", "ts")
	if err != nil {
		t.Fatal(err)
	}
	if sigA == sigB {
		t.Fatalf("signature must depend on body")
	}
	if sigA == sigC {
		t.Fatalf("signature must depend on query")
	}
}

func TestSigner_Timestamp_UnixSeconds(t *testing.T) {
	var s *Signer = NewSigner("k", "s")
	var now time.Time = time.Date(2026, 5, 15, 16, 0, 0, 500_000_000, time.UTC)
	var ts string = s.Timestamp(now)
	// Sub-second component must be dropped (seconds granularity).
	var want string = "1778860800"
	if ts != want {
		t.Fatalf("timestamp mismatch: got=%q want=%q", ts, want)
	}
}

func TestSigner_String_Redacts(t *testing.T) {
	var s *Signer = NewSigner("ABCDEFGHIJ", "secret-secret")
	var got string = s.String()
	if got == "" {
		t.Fatalf("String must return non-empty value")
	}
	if contains(got, "secret-secret") || contains(got, "EFGH") {
		t.Fatalf("String must redact sensitive fields, got %q", got)
	}
}

// contains — without strings.Contains to avoid extra imports in the test.
func contains(s, sub string) bool {
	var i int
	for i = 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
