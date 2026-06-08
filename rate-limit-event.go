/*
FILE: rate-limit-event.go

DESCRIPTION:
Public RateLimitEvent type that the SDK delivers to subscribers via
gate.Config.RateLimitEventObserver. The SDK does NOT throttle internally
(per spec §12): it surfaces the data an external rate-limiter needs and maps
429/limit responses to ErrorKindRateLimit.

WHY THE EXTRA FIELDS:
Gate's rate-limit model cannot be reconstructed from (endpoint, headers) alone:

  1. Batch order endpoints charge per ORDER, not per request. One POST
     /futures/{settle}/batch_orders for 20 orders consumes 20 units. Without
     OrderCount an external counter underestimates usage by up to 20x.
  2. Limits are per (UID + endpoint) and effectively per contract for trading.
     Symbols lets the subscriber debit usage to the right contract state.
  3. Place/Amend share a tighter budget than Cancel/Query on some endpoints.
     Category lets the subscriber model that plane.

Unlike OKX, Gate DOES return rate-limit response headers
(X-Gate-RateLimit-Requests-Remain / -Limit / -Reset-Timestamp); they are
delivered verbatim in Headers so the subscriber can calibrate against the
server's own counters.

DEPENDENCIES:
None — this is a plain data struct.
*/

package gate

// RateLimitCategory — REST call classification from the Gate rate-limit model
// perspective. Used by external rate-limiters to distribute usage across the
// different limit planes Gate enforces.
type RateLimitCategory string

const (
	// RateLimitCategoryPlace — order creation.
	// Endpoints: POST /futures/{settle}/orders, /futures/{settle}/batch_orders.
	RateLimitCategoryPlace RateLimitCategory = "place"

	// RateLimitCategoryAmend — order modification.
	// Endpoints: PUT /futures/{settle}/orders/{order_id} and batch amend.
	RateLimitCategoryAmend RateLimitCategory = "amend"

	// RateLimitCategoryCancel — order cancellation.
	// Endpoints: DELETE /futures/{settle}/orders, /futures/{settle}/orders/{order_id}.
	RateLimitCategoryCancel RateLimitCategory = "cancel"

	// RateLimitCategoryQuery — private GET or non-trading POST (positions,
	// leverage, account configuration).
	RateLimitCategoryQuery RateLimitCategory = "query"

	// RateLimitCategoryMarketData — public GET (contracts, order_book,
	// candlesticks, tickers); Gate meters these per IP.
	RateLimitCategoryMarketData RateLimitCategory = "market"

	// RateLimitCategoryUnknown — fallback for uncategorized requests. The
	// external limiter may ignore these or count them conservatively as Query.
	RateLimitCategoryUnknown RateLimitCategory = ""
)

// String returns the string representation of the category.
func (c RateLimitCategory) String() string { return string(c) }

// RateLimitEvent — structured rate-limit event delivered to subscribers via
// gate.Config.RateLimitEventObserver, exactly once per completed REST call
// (including calls that return a Gate error).
type RateLimitEvent struct {
	// Endpoint — request path (e.g. "/futures/usdt/batch_orders"). Never empty.
	Endpoint string

	// Method — HTTP method in upper case (GET / POST / PUT / DELETE).
	Method string

	// Headers — Gate rate-limit response headers for this call
	// (X-Gate-RateLimit-Requests-Remain / -Limit / -Reset-Timestamp). Always
	// non-nil; empty when the endpoint returned none.
	Headers map[string]string

	// OrderCount — number of orders created/amended/cancelled by this request:
	// 1 for single trading endpoints, len(orders) for batch, 0 for non-trading.
	OrderCount int

	// Symbols — Gate contracts the request relates to (e.g. "BTC_USDT").
	// 1 element for single trading/query methods, 1+ for batch, empty for
	// requests without a contract scope.
	Symbols []string

	// Category — classification by the Gate rate-limit model.
	Category RateLimitCategory
}
