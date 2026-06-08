/*
FILE: errors.go

DESCRIPTION:
Public re-export of entities from the internal/gateerr package. The type itself,
categories, and Gate mappings live in internal/gateerr (see documentation there);
here — only the type aliases and function wrappers so that callers work through
the familiar root-package import:

	import gate "github.com/tonymontanov/go-gate/v2"

	if gate.IsRateLimit(err) { ... }
*/

package gate

import "github.com/tonymontanov/go-gate/v2/internal/gateerr"

// Error — SDK error type. Alias.
type Error = gateerr.Error

// ErrorKind — SDK error category. Alias.
type ErrorKind = gateerr.ErrorKind

// Categories.
const (
	ErrorKindUnknown        = gateerr.ErrorKindUnknown
	ErrorKindNetwork        = gateerr.ErrorKindNetwork
	ErrorKindRateLimit      = gateerr.ErrorKindRateLimit
	ErrorKindAuth           = gateerr.ErrorKindAuth
	ErrorKindInvalidRequest = gateerr.ErrorKindInvalidRequest
	ErrorKindExchange       = gateerr.ErrorKindExchange
)

// NewError creates a *Error. label is the Gate error label (may be empty).
func NewError(kind ErrorKind, label, msg string, cause error) *Error {
	return gateerr.New(kind, label, msg, cause)
}

// IsNetwork / IsRateLimit / IsAuth / IsInvalidRequest / IsExchange — error
// category predicates.
func IsNetwork(err error) bool        { return gateerr.IsNetwork(err) }
func IsRateLimit(err error) bool      { return gateerr.IsRateLimit(err) }
func IsAuth(err error) bool           { return gateerr.IsAuth(err) }
func IsInvalidRequest(err error) bool { return gateerr.IsInvalidRequest(err) }
func IsExchange(err error) bool       { return gateerr.IsExchange(err) }

// MapLabel returns the SDK error category for a Gate error label (status is the
// rate-limit tie-breaker). Returns ErrorKindUnknown for unrecognized labels.
func MapLabel(label string, status int) ErrorKind { return gateerr.MapLabel(label, status) }

// MapHTTPStatus returns the SDK error category for an HTTP status code.
func MapHTTPStatus(status int) ErrorKind { return gateerr.MapHTTPStatus(status) }
