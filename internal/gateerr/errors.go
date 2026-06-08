/*
FILE: internal/gateerr/errors.go

DESCRIPTION:
SDK error type + categories + Gate error mapping. Placed in an internal package
so that any internal/* package (rest, ws, codec) can use it without an import
cycle on the root gate package. The root gate package re-exports these entities
via type aliases.

Gate APIv4 reports failures in two complementary ways that this package maps
into a single ErrorKind taxonomy:
  - the HTTP status code (Gate uses real 4xx/5xx statuses), and
  - a JSON error body {"label":"INVALID_PARAM_VALUE","message":"...","detail":"..."}.
MapLabel is consulted first (it is the most specific signal); MapHTTPStatus is
the fallback when the label is unknown or absent.

See also: errors.go in the root (re-export).
*/

package gateerr

import (
	"errors"
	"fmt"
)

// ErrorKind — SDK error category (spec §5.6).
type ErrorKind uint8

const (
	ErrorKindUnknown ErrorKind = iota
	ErrorKindNetwork
	ErrorKindRateLimit
	ErrorKindAuth
	ErrorKindInvalidRequest
	ErrorKindExchange
)

// String — human-readable category name.
func (k ErrorKind) String() string {
	switch k {
	case ErrorKindNetwork:
		return "network"
	case ErrorKindRateLimit:
		return "rate_limit"
	case ErrorKindAuth:
		return "auth"
	case ErrorKindInvalidRequest:
		return "invalid_request"
	case ErrorKindExchange:
		return "exchange"
	default:
		return "unknown"
	}
}

// Error — unified SDK error type. Label holds the Gate error label
// (e.g. "INVALID_PARAM_VALUE"); HTTPStatus holds the response status code.
type Error struct {
	Kind       ErrorKind
	HTTPStatus int
	Label      string
	Message    string
	Cause      error
}

// Error implements the error interface.
func (e *Error) Error() string {
	switch {
	case e.Label != "" && e.Cause != nil:
		return fmt.Sprintf("gate %s: label=%s status=%d msg=%q: %v", e.Kind, e.Label, e.HTTPStatus, e.Message, e.Cause)
	case e.Label != "":
		return fmt.Sprintf("gate %s: label=%s status=%d msg=%q", e.Kind, e.Label, e.HTTPStatus, e.Message)
	case e.Cause != nil:
		return fmt.Sprintf("gate %s: status=%d msg=%q: %v", e.Kind, e.HTTPStatus, e.Message, e.Cause)
	default:
		return fmt.Sprintf("gate %s: status=%d msg=%q", e.Kind, e.HTTPStatus, e.Message)
	}
}

// Unwrap — for errors.Is/As.
func (e *Error) Unwrap() error { return e.Cause }

// New creates a *Error. label is the Gate error label (may be empty).
func New(kind ErrorKind, label, msg string, cause error) *Error {
	return &Error{Kind: kind, Label: label, Message: msg, Cause: cause}
}

// IsNetwork returns true if err has category Network.
func IsNetwork(err error) bool { return matchKind(err, ErrorKindNetwork) }

// IsRateLimit returns true if err has category RateLimit.
func IsRateLimit(err error) bool { return matchKind(err, ErrorKindRateLimit) }

// IsAuth returns true if err has category Auth.
func IsAuth(err error) bool { return matchKind(err, ErrorKindAuth) }

// IsInvalidRequest returns true if err has category InvalidRequest.
func IsInvalidRequest(err error) bool { return matchKind(err, ErrorKindInvalidRequest) }

// IsExchange returns true if err has category Exchange.
func IsExchange(err error) bool { return matchKind(err, ErrorKindExchange) }

func matchKind(err error, kind ErrorKind) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind == kind
	}
	return false
}

/*
MapLabel returns the SDK error category for a Gate error label. Labels are the
authoritative, status-independent failure signal in Gate APIv4. The status is
passed as a tie-breaker for rate-limit detection (429) when the label is generic.

Returns ErrorKindUnknown when the label is empty or unrecognized — the caller
then falls back to MapHTTPStatus(status).
*/
func MapLabel(label string, status int) ErrorKind {
	if status == 429 {
		return ErrorKindRateLimit
	}
	switch label {
	case "":
		return ErrorKindUnknown

	// Rate limiting.
	case "TOO_MANY_REQUESTS", "REQUEST_RATE_LIMIT", "TOO_FAST", "FREQUENCY_LIMITED":
		return ErrorKindRateLimit

	// Authentication / authorization / signing / clock skew.
	case "INVALID_KEY", "INVALID_SIGNATURE", "SIGNATURE_MISMATCH",
		"INVALID_CREDENTIALS", "INVALID_REQUEST_TIME", "REQUEST_EXPIRED",
		"MISSING_REQUIRED_HEADER", "FORBIDDEN", "API_KEY_NOT_FOUND",
		"READ_ONLY", "IP_FORBIDDEN", "INVALID_API_KEY_TYPE":
		return ErrorKindAuth

	// Client-side validation / not-found / business-rule rejections that the
	// caller can fix by changing the request.
	case "INVALID_PARAM_VALUE", "MISSING_REQUIRED_PARAM", "INVALID_PARAM",
		"INVALID_REQUEST_BODY", "ORDER_NOT_FOUND", "CONTRACT_NOT_FOUND",
		"POSITION_NOT_FOUND", "INVALID_CURRENCY", "INVALID_CONTRACT",
		"INVALID_ORDER_SIZE", "INVALID_ORDER_PRICE", "ORDER_SIZE_TOO_SMALL",
		"ORDER_SIZE_TOO_LARGE", "TINY_LIQUIDATION", "INVALID_CLIENT_ORDER_ID",
		"CLIENT_ID_NOT_FOUND", "DUPLICATE_REQUEST":
		return ErrorKindInvalidRequest

	// Exchange-side state: accepted-but-rejected by matching/risk engine.
	case "BALANCE_NOT_ENOUGH", "MARGIN_NOT_ENOUGH", "POSITION_EMPTY",
		"ORDER_FINISHED", "ORDER_CANCELLED", "LIQUIDATE_IMMEDIATELY",
		"POC_FILL_IMMEDIATELY", "RISK_LIMIT_EXCEEDED", "POSITION_HOLDING",
		"POSITION_IN_LIQUIDATION", "INSUFFICIENT_AVAILABLE", "AUTO_BORROW_TOO_MUCH":
		return ErrorKindExchange

	default:
		return ErrorKindExchange
	}
}

// MapHTTPStatus returns the SDK error category for an HTTP status code (used
// when the response body has no recognizable Gate label).
func MapHTTPStatus(status int) ErrorKind {
	switch {
	case status == 429:
		return ErrorKindRateLimit
	case status == 401 || status == 403:
		return ErrorKindAuth
	case status >= 500:
		return ErrorKindNetwork
	case status >= 400:
		return ErrorKindInvalidRequest
	default:
		return ErrorKindUnknown
	}
}
