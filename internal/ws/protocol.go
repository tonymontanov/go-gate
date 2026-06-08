/*
FILE: internal/ws/protocol.go

DESCRIPTION:
Gate WebSocket v4 wire protocol structures and message builders.

OUTGOING — a subscribe/unsubscribe/ping request:

	{ "time": <unix-sec>, "channel": "<channel>", "event": "<event>",
	  "payload": ["BTC_USDT", ...], "auth": { "method":"api_key","KEY":..,"SIGN":.. } }

The auth object is present only for private channels. Its SIGN is
hex(HMAC_SHA512(secret, "channel=<channel>&event=<event>&time=<ts>")).

INCOMING — an ack or a data push:

	{ "time": <unix-sec>, "channel": "<channel>", "event": "<event>",
	  "error": null | {"code":..,"message":..}, "result": <object|array> }

event is "subscribe"/"unsubscribe" for acks and "update"/"all" for data.
*/

package ws

import "github.com/tonymontanov/go-gate/v2/internal/codec"

// authBlock — the per-subscribe authentication object for private channels.
type authBlock struct {
	Method string `json:"method"`
	Key    string `json:"KEY"`
	Sign   string `json:"SIGN"`
}

// request — an outgoing subscribe/unsubscribe/ping message.
type request struct {
	Time    int64      `json:"time"`
	Channel string     `json:"channel"`
	Event   string     `json:"event,omitempty"`
	Payload []string   `json:"payload,omitempty"`
	Auth    *authBlock `json:"auth,omitempty"`
}

// responseError — the error object in an incoming message.
type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// response — an incoming ack or data push. Result is deferred so the domain
// layer parses it into the concrete typed shape.
type response struct {
	Time    int64            `json:"time"`
	Channel string           `json:"channel"`
	Event   string           `json:"event"`
	Error   *responseError   `json:"error"`
	Result  codec.RawMessage `json:"result"`
}

// Event names.
const (
	eventSubscribe   = "subscribe"
	eventUnsubscribe = "unsubscribe"
	eventUpdate      = "update"
	eventAll         = "all"
)
