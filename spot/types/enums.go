/*
FILE: spot/types/enums.go

DESCRIPTION:
Enumerated values for the Gate Spot domain. Unlike futures (where side and type
are implicit in the signed integer size), Gate Spot has EXPLICIT wire fields:

  - "side": "buy" | "sell";
  - "type": "limit" | "market";
  - "time_in_force": "gtc" | "ioc" | "poc" | "fok";
  - "account": "spot" | "margin" | "cross_margin" | "unified".

These enums map directly to those wire strings.
*/

package types

// SideType — order direction. Gate Spot has an explicit "side" field.
type SideType string

const (
	// SideTypeBuy — buy side.
	SideTypeBuy SideType = "buy"
	// SideTypeSell — sell side.
	SideTypeSell SideType = "sell"
)

// OrderType — Gate Spot order type ("type" wire field).
type OrderType string

const (
	// OrderTypeLimit — limit order (explicit price).
	OrderTypeLimit OrderType = "limit"
	// OrderTypeMarket — market order. Gate requires tif ∈ {ioc, fok}; for a market
	// BUY the "amount" field is the QUOTE amount to spend, not the base amount.
	OrderTypeMarket OrderType = "market"
)

// TimeInForceType — Gate time-in-force. Values are the exact Gate wire strings.
type TimeInForceType string

const (
	// TimeInForceGTC — Good-Till-Cancelled.
	TimeInForceGTC TimeInForceType = "gtc"
	// TimeInForceIOC — Immediate-Or-Cancel (taker only). Required for market orders.
	TimeInForceIOC TimeInForceType = "ioc"
	// TimeInForcePOC — Pending-Or-Cancelled: post-only (maker only).
	TimeInForcePOC TimeInForceType = "poc"
	// TimeInForceFOK — Fill-Or-Kill.
	TimeInForceFOK TimeInForceType = "fok"
)

// AccountType — Gate Spot settlement account ("account" wire field). v2.0 supports
// only the plain spot account; margin/cross_margin/unified are reserved for a
// later iteration.
type AccountType string

const (
	// AccountSpot — the plain spot account (default).
	AccountSpot AccountType = "spot"
)

// Order status strings as returned by Gate in the spot order "status" field.
const (
	// OrderStatusOpen — the order is still active on the book.
	OrderStatusOpen string = "open"
	// OrderStatusClosed — the order filled completely.
	OrderStatusClosed string = "closed"
	// OrderStatusCancelled — the order was cancelled (fully or partially filled).
	OrderStatusCancelled string = "cancelled"
)
