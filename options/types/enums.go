/*
FILE: options/types/enums.go

DESCRIPTION:
Enumerated values for the Gate OPTIONS domain. Values map directly to Gate APIv4
wire strings where Gate has an explicit field (tif), and are SDK-level
conveniences where Gate encodes the concept implicitly:

  - Gate has NO order "side" field — direction is the sign of the integer order
    size. SideType is the SDK's explicit representation; the trading layer encodes
    it into the size sign on the way out and derives it from the sign on the way in.
  - Gate has NO order "type" field — a market order is price="0" + tif="ioc";
    everything else is a limit order. OrderType is an SDK convenience the trading
    layer resolves into (price, tif).
  - OptionType (call/put) is carried on the contract spec; Gate encodes it as the
    boolean "is_call".
*/

package types

// SideType — order direction (SDK-level; encoded into the size sign for Gate).
type SideType string

const (
	// SideTypeBuy — long/buy; encoded as a positive order size.
	SideTypeBuy SideType = "buy"
	// SideTypeSell — short/sell; encoded as a negative order size.
	SideTypeSell SideType = "sell"
)

// OrderType — order type (SDK-level convenience; Gate infers it from price/tif).
type OrderType string

const (
	// OrderTypeLimit — limit order: explicit price, default tif gtc.
	OrderTypeLimit OrderType = "limit"
	// OrderTypeMarket — market order: sent as price="0" with tif="ioc".
	OrderTypeMarket OrderType = "market"
)

// TimeInForceType — Gate time-in-force. Values are the exact Gate wire strings.
type TimeInForceType string

const (
	// TimeInForceGTC — Good-Till-Cancelled.
	TimeInForceGTC TimeInForceType = "gtc"
	// TimeInForceIOC — Immediate-Or-Cancel (taker only). Used for market orders.
	TimeInForceIOC TimeInForceType = "ioc"
	// TimeInForcePOC — Pending-Or-Cancelled: post-only (maker only).
	TimeInForcePOC TimeInForceType = "poc"
	// TimeInForceFOK — Fill-Or-Kill.
	TimeInForceFOK TimeInForceType = "fok"
)

// OptionType — option style (call or put). SDK-level; Gate encodes it as the
// boolean "is_call" on the contract spec.
type OptionType string

const (
	// OptionTypeCall — a call option.
	OptionTypeCall OptionType = "call"
	// OptionTypePut — a put option.
	OptionTypePut OptionType = "put"
)

// Order status strings as returned by Gate in the options order object.
const (
	// OrderStatusOpen — the order is still active on the book.
	OrderStatusOpen string = "open"
	// OrderStatusFinished — the order reached a terminal state (see FinishAs).
	OrderStatusFinished string = "finished"
)
