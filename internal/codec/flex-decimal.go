/*
FILE: internal/codec/flex-decimal.go

DESCRIPTION:
FlexDecimal is a decimal.Decimal that decodes from EITHER a JSON number or a
JSON string, without losing precision. Gate is inconsistent about this across
transports for the same field: the REST position payload quotes its decimal
fields ("25", "0.0418"), but the WebSocket futures.positions / delivery.positions
push sends those same fields as bare JSON numbers (25, 0.0418). A single shared
payload struct (used by both the REST account client and the WS stream client)
therefore needs a field type that tolerates both forms.

The raw JSON token text is handed straight to decimal.NewFromString, so the
numeric precision is preserved exactly (no float64 round-trip). A null, empty,
or malformed token decodes to Zero rather than erroring: a single bad field must
never abort the whole hot-path decode of a position/order push.
*/

package codec

import "github.com/shopspring/decimal"

// FlexDecimal wraps decimal.Decimal to accept the number-or-string wire forms
// Gate uses interchangeably across REST and WebSocket.
type FlexDecimal struct {
	decimal.Decimal
}

// UnmarshalJSON implements json.Unmarshaler for both the number and the quoted
// string representations. Empty/null/malformed input decodes to Zero.
func (f *FlexDecimal) UnmarshalJSON(b []byte) error {
	f.Decimal = decimal.Zero
	if len(b) == 0 {
		return nil
	}
	// Strip one layer of surrounding quotes for the string form ("0.0418").
	if b[0] == '"' && b[len(b)-1] == '"' && len(b) >= 2 {
		b = b[1 : len(b)-1]
	}
	var s string = string(b)
	if s == "" || s == "null" {
		return nil
	}
	var d decimal.Decimal
	var err error
	d, err = decimal.NewFromString(s)
	if err != nil {
		// Tolerant: leave Zero, do not fail the surrounding decode.
		return nil
	}
	f.Decimal = d
	return nil
}
