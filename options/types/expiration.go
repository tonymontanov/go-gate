/*
FILE: options/types/expiration.go

DESCRIPTION:
Expiration — the SDK's representation of a Gate OPTIONS expiration timestamp
(from GET /options/expirations?underlying=). Gate returns a bare JSON array of
epoch-SECONDS timestamps (e.g. [1711699200, 1712304000]); the SDK normalizes each
to epoch milliseconds and wraps it so the public surface stays typed and stable.

CALIBRATION: the endpoint shape (a flat []int64 of seconds) follows Gate's options
docs; verify live.
*/

package types

// Expiration — one options expiration time.
type Expiration struct {
	// ExpirationMs — expiry/settlement time in epoch milliseconds (Gate returns
	// the raw value in seconds).
	ExpirationMs int64
}
