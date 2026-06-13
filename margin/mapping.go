/*
FILE: margin/mapping.go

DESCRIPTION:
Shared wire ↔ domain helpers used across the margin sub-clients: query-string
construction, decimal parsing, and Gate timestamp normalization. These mirror the
equivalent helpers in the other sections (options secondsToMs/floatSeconds) but
are defined LOCALLY in the margin package so it never imports another section.

GATE WIRE NOTES:
  - many list endpoints carry create_time / time as epoch SECONDS; a parallel
    *_ms field, when present, is epoch milliseconds and takes precedence;
  - decimal fields are usually quoted strings over REST; the payload structs use
    codec.FlexDecimal so a bare-number form also decodes.
*/

package margin

import (
	"net/url"
	"strconv"

	"github.com/shopspring/decimal"
)

// newQuery returns an empty url.Values for building REST query strings.
func newQuery() url.Values { return url.Values{} }

// mustDecimal parses a Gate decimal string, treating "" as zero and silently
// falling back to zero on malformed input (a single bad field must not abort the
// whole decode on a hot path).
func mustDecimal(s string) decimal.Decimal {
	if s == "" {
		return decimal.Zero
	}
	var d decimal.Decimal
	var err error
	d, err = decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}

// secondsToMs converts an epoch-seconds timestamp to milliseconds (0 stays 0).
func secondsToMs(sec int64) int64 {
	if sec <= 0 {
		return 0
	}
	return sec * 1000
}

// floatSecondsOrMsToMs normalizes a Gate float timestamp to epoch milliseconds.
// Values that already look like milliseconds (>= 1e12) are taken as-is; smaller
// values are treated as seconds and scaled.
func floatSecondsOrMsToMs(v float64) int64 {
	if v <= 0 {
		return 0
	}
	if v >= 1e12 {
		return int64(v)
	}
	return int64(v * 1000)
}

// epochMs returns milliseconds, preferring the millisecond field when present,
// otherwise converting the epoch-seconds field.
func epochMs(ms int64, sec int64) int64 {
	if ms > 0 {
		return ms
	}
	return secondsToMs(sec)
}

// idString formats a non-zero numeric id as a string ("" for 0), so zero ids do
// not leak as "0" into the domain types.
func idString(id int64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}
