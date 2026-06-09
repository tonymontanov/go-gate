/*
FILE: spot/mapping.go

DESCRIPTION:
Wire ↔ domain mapping for Gate spot orders: the JSON payload structs, the
conversion into types.OrderInfo, and client-order-id ("text") normalization.

GATE SPOT WIRE NOTES (vs futures):
  - the order id is a STRING (futures: numeric);
  - side / type / time_in_force are explicit string fields (no signed size);
  - amount / price / left / filled_total / avg_deal_price are decimal strings;
  - create_time / update_time are second strings; create_time_ms / update_time_ms
    are numeric epoch milliseconds and take precedence;
  - batch responses add succeeded/label/message per element.
*/

package spot

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/spot/types"
)

// spotOrderPayload — Gate spot Order wire shape (the fields the SDK consumes).
type spotOrderPayload struct {
	ID           string  `json:"id"`
	Text         string  `json:"text"`
	CurrencyPair string  `json:"currency_pair"`
	Type         string  `json:"type"`
	Account      string  `json:"account"`
	Side         string  `json:"side"`
	Amount       string  `json:"amount"`
	Price        string  `json:"price"`
	TimeInForce  string  `json:"time_in_force"`
	Left         string  `json:"left"`
	FilledTotal  string  `json:"filled_total"`
	AvgDealPrice string  `json:"avg_deal_price"`
	FillPrice    string  `json:"fill_price"`
	Status       string  `json:"status"`
	FinishAs     string  `json:"finish_as"`
	CreateTime   string  `json:"create_time"`
	UpdateTime   string  `json:"update_time"`
	CreateTimeMs float64 `json:"create_time_ms"`
	UpdateTimeMs float64 `json:"update_time_ms"`
}

// batchSpotOrderPayload — a single element of a batch_orders response: a spot
// Order plus the per-element status fields Gate adds for batch results.
type batchSpotOrderPayload struct {
	spotOrderPayload
	Succeeded *bool  `json:"succeeded"`
	Label     string `json:"label"`
	Message   string `json:"message"`
	Detail    string `json:"detail"`
}

// orderInfoFromPayload converts a Gate spot Order payload into types.OrderInfo.
// rateLimits is attached for the rate-limit-aware caller (may be nil/empty).
func orderInfoFromPayload(p *spotOrderPayload, rateLimits map[string]string) types.OrderInfo {
	var info types.OrderInfo
	info.OrderID = p.ID
	info.ClientOrderID = p.Text
	info.CurrencyPair = p.CurrencyPair
	info.Side = types.SideType(p.Side)
	if p.Type == "" {
		info.OrderType = types.OrderTypeLimit
	} else {
		info.OrderType = types.OrderType(p.Type)
	}
	info.Account = types.AccountType(p.Account)
	info.Price = mustDecimal(p.Price)
	info.Amount = mustDecimal(p.Amount)
	info.Left = mustDecimal(p.Left)
	// FilledAmount = Amount − Left (guard against an unexpected Left > Amount, e.g.
	// the market-buy case where amount is denominated in quote).
	if !info.Left.IsZero() && info.Left.LessThanOrEqual(info.Amount) {
		info.FilledAmount = info.Amount.Sub(info.Left)
	} else if info.Left.IsZero() {
		info.FilledAmount = info.Amount
	}
	info.AvgDealPrice = mustDecimal(p.AvgDealPrice)
	info.FilledTotal = mustDecimal(p.FilledTotal)
	info.TimeInForce = types.TimeInForceType(p.TimeInForce)
	info.Status = p.Status
	info.FinishAs = p.FinishAs
	info.CreatedAtMs = spotEpochMs(p.CreateTimeMs, p.CreateTime)
	info.UpdatedAtMs = spotEpochMs(p.UpdateTimeMs, p.UpdateTime)
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

// spotEpochMs returns epoch milliseconds, preferring the numeric millisecond
// field when present, otherwise converting the second-string field.
func spotEpochMs(ms float64, secStr string) int64 {
	if ms > 0 {
		return int64(ms)
	}
	if secStr != "" {
		var f float64
		var err error
		f, err = strconv.ParseFloat(secStr, 64)
		if err == nil && f > 0 {
			return int64(f * 1000)
		}
	}
	return 0
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

// orderIDPath returns the path identifier for single-order endpoints: the order
// id when set, otherwise the normalized client text (Gate accepts the text in
// place of the id). Returns "" if neither is usable.
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
