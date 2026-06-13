/*
FILE: wallet/mapping.go

DESCRIPTION:
Shared wire ↔ domain helpers used across the wallet endpoints: query-string
construction and Gate timestamp normalization. These mirror the equivalent
helpers in the other sections (secondsToMs/epochMs/idString) but are defined
LOCALLY in the wallet package so it never imports another section.

GATE WIRE NOTES:
  - transfer/history endpoints carry time / timestamp / timest as epoch SECONDS;
    a parallel *_ms field, when present, is epoch milliseconds and takes precedence;
  - decimal fields are usually quoted strings over REST; the payload structs use
    codec.FlexDecimal so a bare-number form also decodes;
  - several disabled flags arrive as 0/1 integers; flagToBool normalizes them.
*/

package wallet

import (
	"net/url"
	"strconv"

	"github.com/shopspring/decimal"
	"github.com/tonymontanov/go-gate/v2/internal/codec"
)

// newQuery returns an empty url.Values for building REST query strings.
func newQuery() url.Values { return url.Values{} }

// flexMapToDecimal projects a map of codec.FlexDecimal values to plain
// decimal.Decimal, preserving the keys. A nil input yields nil.
func flexMapToDecimal(in map[string]codec.FlexDecimal) map[string]decimal.Decimal {
	if in == nil {
		return nil
	}
	var out map[string]decimal.Decimal = make(map[string]decimal.Decimal, len(in))
	var key string
	var val codec.FlexDecimal
	for key, val = range in {
		out[key] = val.Decimal
	}
	return out
}

// secondsToMs converts an epoch-seconds timestamp to milliseconds (0 stays 0).
func secondsToMs(sec int64) int64 {
	if sec <= 0 {
		return 0
	}
	return sec * 1000
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

// flagToBool normalizes a Gate 0/1 integer flag to a bool (non-zero → true).
func flagToBool(v int64) bool { return v != 0 }

// secondsStringToMs parses a Gate epoch-seconds string ("1631811846") and
// converts it to epoch milliseconds. "" or malformed input yields 0.
func secondsStringToMs(s string) int64 {
	if s == "" {
		return 0
	}
	var sec int64
	var err error
	sec, err = strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return secondsToMs(sec)
}
