/*
FILE: unified/helpers.go

DESCRIPTION:
Shared decimal/timestamp helpers used across the unified sub-client methods.
These are local to the unified package (the section never imports another
section's helpers).

GATE WIRE NOTES:
  - monetary/rate fields are decimal strings over REST; the UnifiedAccount
    balances/margins may also arrive as bare JSON numbers and so use
    codec.FlexDecimal at the payload layer (see types/account.go usage).
  - time fields (refresh_time, create_time, update_time) are epoch seconds;
    some endpoints may quote epoch milliseconds. secondsToMs scales plain
    seconds, while epochToMs auto-detects ms-vs-seconds for float timestamps.
*/

package unified

import "github.com/shopspring/decimal"

// mustDecimal parses a Gate decimal string, treating "" as zero and silently
// falling back to zero on malformed input (a single bad field must not abort the
// whole decode).
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

// epochToMs normalizes a Gate float timestamp to epoch milliseconds. Values that
// already look like milliseconds (>= 1e12) are taken as-is; smaller positive
// values are treated as seconds and scaled.
func epochToMs(v float64) int64 {
	if v <= 0 {
		return 0
	}
	if v >= 1e12 {
		return int64(v)
	}
	return int64(v * 1000)
}
