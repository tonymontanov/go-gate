/*
FILE: earn/mapping.go

DESCRIPTION:
Shared helpers for the Gate Earn "Uni" lending section: query construction and
the decimal / timestamp normalizers used across the section methods.

GATE WIRE NOTES:
  - decimal amounts/rates are parsed leniently (bad/empty → zero) on read paths;
  - create_time/update_time/chart time are epoch SECONDS and are converted to
    epoch milliseconds (...Ms) by secondsToMs. These helpers are local to the
    earn package so the section does not import another section.
*/

package earn

import (
	"net/url"
	"strconv"

	"github.com/shopspring/decimal"
)

// newQuery returns an empty url.Values for building REST query strings.
func newQuery() url.Values { return url.Values{} }

// idString formats a non-zero numeric id as a string ("" for 0), so zero ids do
// not leak as "0" into the domain types. Used by the Fixed-Term and Dual
// sub-clients whose record ids arrive as bare JSON numbers.
func idString(id int64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}

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
