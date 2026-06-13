/*
FILE: options/mapping.go

DESCRIPTION:
Wire ↔ domain mapping for Gate options orders: the JSON payload struct, the
conversion into types.OrderInfo (decoding Gate's signed-size and second-based
timestamps), and client-order-id ("text") normalization/validation. Shared
decimal/size/timestamp helpers used across the options sub-clients live here too.

GATE WIRE NOTES:
  - size/left are signed integers (contracts); positive=buy, negative=sell.
  - create_time/finish_time are float seconds; create_time_ms/finish_time_ms,
    when present, are epoch milliseconds and take precedence.
  - price/fill_price are decimal strings; mkfr/tkfr are decimal strings.
*/

package options

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/options/types"
)

// optionsOrderPayload — Gate OptionsOrder wire shape (the fields the SDK consumes).
type optionsOrderPayload struct {
	ID           int64   `json:"id"`
	Contract     string  `json:"contract"`
	CreateTime   float64 `json:"create_time"`
	CreateTimeMs float64 `json:"create_time_ms"`
	FinishTime   float64 `json:"finish_time"`
	FinishTimeMs float64 `json:"finish_time_ms"`
	FinishAs     string  `json:"finish_as"`
	Status       string  `json:"status"`
	Size         int64   `json:"size"`
	Price        string  `json:"price"`
	Left         int64   `json:"left"`
	FillPrice    string  `json:"fill_price"`
	Tif          string  `json:"tif"`
	Text         string  `json:"text"`
	IsClose      bool    `json:"is_close"`
	IsReduceOnly bool    `json:"is_reduce_only"`
	Mkfr         string  `json:"mkfr"`
	Tkfr         string  `json:"tkfr"`
}

// orderInfoFromPayload converts a Gate OptionsOrder payload into types.OrderInfo.
// rateLimits is attached for the rate-limit-aware caller (may be nil/empty).
func orderInfoFromPayload(p *optionsOrderPayload, rateLimits map[string]string) types.OrderInfo {
	var info types.OrderInfo
	if p.ID != 0 {
		info.OrderID = strconv.FormatInt(p.ID, 10)
	}
	info.ClientOrderID = p.Text
	info.Contract = p.Contract
	info.Side = sideFromSize(p.Size)
	info.Size = decimalAbsInt(p.Size)
	info.Left = decimalAbsInt(p.Left)

	var price decimal.Decimal = mustDecimal(p.Price)
	info.Price = price
	info.FillPrice = mustDecimal(p.FillPrice)
	if price.IsZero() {
		info.OrderType = types.OrderTypeMarket
	} else {
		info.OrderType = types.OrderTypeLimit
	}

	info.TimeInForce = types.TimeInForceType(p.Tif)
	info.Status = p.Status
	info.FinishAs = p.FinishAs
	info.ReduceOnly = p.IsReduceOnly
	info.Close = p.IsClose
	info.MakerFeeRate = mustDecimal(p.Mkfr)
	info.TakerFeeRate = mustDecimal(p.Tkfr)
	info.CreatedAtMs = epochMs(p.CreateTimeMs, p.CreateTime)
	info.FinishedAtMs = epochMs(p.FinishTimeMs, p.FinishTime)
	info.RateLimits = rateLimits
	return info
}

// mustDecimal parses a Gate decimal string, treating "" as zero and silently
// falling back to zero on malformed input (a single bad field must not abort the
// whole order decode on a hot path).
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

// decimalAbsInt returns the absolute value of a signed Gate size as a decimal.
func decimalAbsInt(v int64) decimal.Decimal {
	if v < 0 {
		v = -v
	}
	return decimal.NewFromInt(v)
}

// sideFromSize derives the order side from the Gate signed size.
func sideFromSize(size int64) types.SideType {
	switch {
	case size > 0:
		return types.SideTypeBuy
	case size < 0:
		return types.SideTypeSell
	default:
		return ""
	}
}

// epochMs returns milliseconds, preferring the millisecond field when present,
// otherwise converting the float-seconds field.
func epochMs(ms float64, sec float64) int64 {
	if ms > 0 {
		return int64(ms)
	}
	if sec > 0 {
		return int64(sec * 1000)
	}
	return 0
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

// clientIDBodyPattern — allowed characters/length of the client order id AFTER
// the mandatory "t-" prefix: 1..28 chars of [0-9A-Za-z._-] (Gate "text" rule).
var clientIDBodyPattern = regexp.MustCompile(`^[0-9A-Za-z._-]{1,28}$`)

// normalizeClientID validates and normalizes a client order id to Gate's "text"
// format. Empty input returns "" (no text sent). Any input is normalized to a
// single leading "t-" prefix; the remainder must match clientIDBodyPattern.
func normalizeClientID(text string) (string, error) {
	if text == "" {
		return "", nil
	}
	var body string = text
	if strings.HasPrefix(body, "t-") {
		body = body[2:]
	}
	if !clientIDBodyPattern.MatchString(body) {
		return "", gate.NewError(gate.ErrorKindInvalidRequest, "",
			"invalid client order id (Gate text must be 1..28 chars of [0-9A-Za-z._-] after the t- prefix)", nil)
	}
	return "t-" + body, nil
}

// orderIDPath returns the path identifier for single-order endpoints: the numeric
// OrderID when set, otherwise the normalized client text. Returns "" if neither
// is usable.
func orderIDPath(orderID, clientOrderID string) string {
	if orderID != "" {
		return orderID
	}
	if clientOrderID == "" {
		return ""
	}
	var normalized string
	var err error
	normalized, err = normalizeClientID(clientOrderID)
	if err != nil {
		return ""
	}
	return normalized
}
