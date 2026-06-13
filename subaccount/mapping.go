/*
FILE: subaccount/mapping.go

DESCRIPTION:
Shared wire ↔ domain helpers used across the subaccount endpoints: query-string
construction, Gate timestamp normalization, and the permission wire ↔ domain
projection. These mirror the equivalent helpers in the other sections
(secondsToMs/epochMs/idString) but are defined LOCALLY in the subaccount package
so it never imports another section.

GATE WIRE NOTES:
  - create_time / created_at / updated_at / last_access are epoch SECONDS; a
    parallel *_ms field, when present, is epoch milliseconds and takes precedence;
  - a key's user_id is a string on the key endpoints but a number on the
    sub-account endpoints; idString normalizes the numeric form.
*/

package subaccount

import (
	"net/url"
	"strconv"

	"github.com/tonymontanov/go-gate/v2/subaccount/types"
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

// permissionPayload — Gate API-key permission wire shape.
type permissionPayload struct {
	Name     string `json:"name"`
	ReadOnly bool   `json:"read_only"`
}

// permsFromPayload projects the wire permission list to the domain type.
func permsFromPayload(in []permissionPayload) []types.Permission {
	if in == nil {
		return nil
	}
	var out []types.Permission = make([]types.Permission, 0, len(in))
	var k int
	for k = 0; k < len(in); k++ {
		out = append(out, types.Permission{
			Name:     in[k].Name,
			ReadOnly: in[k].ReadOnly,
		})
	}
	return out
}

// permsToBody projects the domain permission list to the wire form used in
// request bodies ([]map[string]any so the JSON keys are Gate-exact).
func permsToBody(perms []types.Permission) []map[string]any {
	var out []map[string]any = make([]map[string]any, 0, len(perms))
	var k int
	for k = 0; k < len(perms); k++ {
		out = append(out, map[string]any{
			"name":      perms[k].Name,
			"read_only": perms[k].ReadOnly,
		})
	}
	return out
}
