/*
FILE: loan/mapping.go

DESCRIPTION:
Shared wire ↔ domain helpers for the loan client: query-string construction and
Gate timestamp normalization. These mirror the equivalent helpers in the other
sections but are defined LOCALLY in the loan package so it never imports another
section.

GATE WIRE NOTES:
  - create_time / update_time / expire_time arrive as epoch SECONDS; they are
    normalized to epoch milliseconds (...Ms);
  - decimal fields are usually quoted strings over REST; the payload structs use
    codec.FlexDecimal so a bare-number form also decodes.
*/

package loan

import (
	"net/url"
	"strconv"
)

// newQuery returns an empty url.Values for building REST query strings.
func newQuery() url.Values { return url.Values{} }

// secondsToMs converts an epoch-seconds timestamp to milliseconds (0 stays 0).
func secondsToMs(sec int64) int64 {
	if sec <= 0 {
		return 0
	}
	return sec * 1000
}

// idString formats a non-zero numeric id as a string ("" for 0), so zero ids do
// not leak as "0" into the domain types.
func idString(id int64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}
