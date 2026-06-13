/*
FILE: config.go

DESCRIPTION:
config.go defines the SDK configuration structs (spec §5.5) plus the production
and testnet Gate endpoints and default values.

MAIN FUNCTIONS:
  - DefaultConfig():        Config with production endpoints and default timeouts.
  - (Config).withDefaults(): fills empty fields with defaults. Inside the SDK the
    config is ALWAYS passed through withDefaults() first; the user's Config is
    never mutated.
  - (Config).validate():    checks required URL fields.

ENDPOINTS:
Production:
  - REST:         https://api.gateio.ws/api/v4
  - WS futures:   wss://fx-ws.gateio.ws/v4/ws/usdt   (USDT-settled perpetuals)
Testnet (Config.Testnet = true):
  - REST:         https://fx-api-testnet.gateio.ws/api/v4
  - WS futures:   wss://fx-ws-testnet.gateio.ws/v4/ws/usdt

DEPENDENCIES:
- time: timeouts and reconnect/keepalive intervals.
*/

package gate

import "time"

// Gate transport URLs. Declared as vars (not const) so tests can point them at a
// mock server.
var (
	// DefaultRestBaseURL — production REST endpoint, includes the /api/v4 prefix.
	DefaultRestBaseURL string = "https://api.gateio.ws/api/v4"
	// DefaultWsFuturesURL — production WS endpoint for USDT-settled futures.
	DefaultWsFuturesURL string = "wss://fx-ws.gateio.ws/v4/ws/usdt"
	// DefaultWsSpotURL — production WS endpoint for the spot section. Gate serves
	// spot on a different host than futures (api.gateio.ws vs fx-ws.gateio.ws).
	// Gate has NO public spot testnet (the testnet is futures-only), so there is
	// no Testnet* counterpart — the spot section always uses this host.
	DefaultWsSpotURL string = "wss://api.gateio.ws/ws/v4/"
	// DefaultWsDeliveryURL — production WS endpoint for the delivery section
	// (dated USDT-settled futures). Gate routes delivery to a delivery-specific
	// socket; the channel names are shared with the futures namespace
	// ("futures.*"). CALIBRATION: confirm host + channel prefix live.
	DefaultWsDeliveryURL string = "wss://fx-ws.gateio.ws/v4/ws/delivery/usdt"
	// DefaultWsOptionsURL — production WS endpoint for the options section. Gate
	// routes options to a dedicated socket (note the ".live" host) and the
	// channels live in the "options.*" namespace. Options is NOT settle-scoped
	// (REST paths are "/options/...", not "/options/{settle}/...").
	// CALIBRATION: confirm host + channel names live.
	DefaultWsOptionsURL string = "wss://op-ws.gateio.live/v4/ws"

	// TestnetRestBaseURL — futures testnet REST endpoint.
	TestnetRestBaseURL string = "https://fx-api-testnet.gateio.ws/api/v4"
	// TestnetWsFuturesURL — futures testnet WS endpoint (USDT-settled).
	TestnetWsFuturesURL string = "wss://fx-ws-testnet.gateio.ws/v4/ws/usdt"
	// TestnetWsDeliveryURL — delivery testnet WS endpoint. CALIBRATION: verify.
	TestnetWsDeliveryURL string = "wss://fx-ws-testnet.gateio.ws/v4/ws/delivery/usdt"
	// TestnetWsOptionsURL — options testnet WS endpoint. CALIBRATION: verify.
	TestnetWsOptionsURL string = "wss://ws-testnet.gate.com/v4/ws/options"
)

// DefaultSettle — default settlement currency for the futures section.
const DefaultSettle string = "usdt"

// Config — public SDK configuration. Passed to NewClient.
type Config struct {
	// APIKey — Gate API key (KEY header).
	APIKey string
	// SecretKey — Gate API secret used for HMAC-SHA512 signing (SIGN header).
	SecretKey string

	// Settle — settlement currency for the futures section ("usdt" in v1.0).
	// Lower-case; used to build "/futures/{settle}/..." paths. Default: "usdt".
	Settle string

	// REST — REST transport settings. Empty fields fall back to DefaultConfig().
	REST RestConfig
	// WS — WebSocket transport settings. Empty fields fall back to DefaultConfig().
	WS WsConfig
	// Orderbook — local order-book engine settings (used by the WatchOrderBook
	// streams). Empty fields fall back to DefaultConfig().
	Orderbook OrderbookConfig

	// Logger — optional logger. If nil, NoopLogger() is used.
	Logger Logger

	// UserAgent — User-Agent for REST requests. Default: "go-gate/v2".
	UserAgent string

	// ChannelID — optional broker/channel id sent as X-Gate-Channel-Id on every
	// request. Empty → header omitted.
	ChannelID string

	// RateLimitEventObserver — optional hook invoked SYNCHRONOUSLY once per
	// completed REST call with a structured RateLimitEvent (see rate-limit-event.go).
	// The SDK does not throttle internally; this feeds an external limiter.
	//
	// Speed contract: runs in the calling goroutine and blocks the return from a
	// REST call. Implementations must be O(1) (typically a non-blocking channel
	// send). nil → no-op, zero overhead.
	RateLimitEventObserver func(RateLimitEvent)

	// Testnet — switch REST/WS default endpoints to the Gate futures testnet.
	// Explicit URLs in REST.BaseURL / WS.FuturesURL are NOT overridden.
	// Testnet uses separate API keys.
	Testnet bool
}

// RestConfig — HTTP transport settings.
type RestConfig struct {
	// BaseURL — base URL for the Gate REST API, including /api/v4. Default:
	// DefaultRestBaseURL (or TestnetRestBaseURL when Config.Testnet is set).
	BaseURL string
	// RequestTimeout — timeout for a single REST request. Default: 10s. For
	// latency-critical calls pass a ctx with its own deadline — it overrides this.
	RequestTimeout time.Duration
	// MaxIdleConns — idle connection pool size for http.Transport. Default: 100.
	MaxIdleConns int
	// MaxIdleConnsPerHost — pool size per host. Default: 100.
	MaxIdleConnsPerHost int
	// IdleConnTimeout — keep-alive idle timeout. Default: 90s.
	IdleConnTimeout time.Duration
}

// WsConfig — WebSocket transport settings. Consumed by the futures section in M4;
// declared here so the config surface is stable across milestones.
type WsConfig struct {
	// FuturesURL — USDT-settled futures WS URL. Default: DefaultWsFuturesURL
	// (or TestnetWsFuturesURL when Config.Testnet is set).
	FuturesURL string
	// SpotURL — spot WS URL. Default: DefaultWsSpotURL. Gate has no spot testnet,
	// so Config.Testnet does NOT switch this (spot always targets prod).
	SpotURL string
	// DeliveryURL — delivery (dated futures) WS URL. Default: DefaultWsDeliveryURL
	// (or TestnetWsDeliveryURL when Config.Testnet is set).
	DeliveryURL string
	// OptionsURL — options WS URL. Default: DefaultWsOptionsURL (or
	// TestnetWsOptionsURL when Config.Testnet is set).
	OptionsURL string
	// HandshakeTimeout — connection handshake timeout. Default: 10s.
	HandshakeTimeout time.Duration
	// ReadTimeout — read timeout for a single frame. Default: 35s.
	ReadTimeout time.Duration
	// WriteTimeout — write timeout for a single frame. Default: 5s.
	WriteTimeout time.Duration
	// PingInterval — client-side ping interval. Default: 15s (Gate disconnects
	// idle connections; a sub-interval ping keeps them alive).
	PingInterval time.Duration
	// ReconnectInitialBackoff — initial delay between reconnect attempts. Default: 200ms.
	ReconnectInitialBackoff time.Duration
	// ReconnectMaxBackoff — upper bound of backoff. Default: 10s.
	ReconnectMaxBackoff time.Duration
	// ReconnectJitter — relative jitter [0..1] added to backoff. Default: 0.2.
	ReconnectJitter float64
	// ReadBufferSize — gorilla/websocket read buffer size. Default: 64KB.
	ReadBufferSize int
	// WriteBufferSize — gorilla/websocket write buffer size. Default: 16KB.
	WriteBufferSize int
}

// OrderbookConfig — local order-book engine settings. Consumed by the
// WatchOrderBook streams of each section (futures, spot).
type OrderbookConfig struct {
	// MaxDepth — maximum number of price levels kept per side in the local book.
	// Incoming deltas beyond this depth are trimmed. Default: 400.
	MaxDepth int
}

// DefaultConfig returns a Config with production endpoints and sensible defaults.
func DefaultConfig() Config {
	return Config{
		Settle: DefaultSettle,
		REST: RestConfig{
			BaseURL:             DefaultRestBaseURL,
			RequestTimeout:      10 * time.Second,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
		WS: WsConfig{
			FuturesURL:              DefaultWsFuturesURL,
			SpotURL:                 DefaultWsSpotURL,
			DeliveryURL:             DefaultWsDeliveryURL,
			OptionsURL:              DefaultWsOptionsURL,
			HandshakeTimeout:        10 * time.Second,
			ReadTimeout:             35 * time.Second,
			WriteTimeout:            5 * time.Second,
			PingInterval:            15 * time.Second,
			ReconnectInitialBackoff: 200 * time.Millisecond,
			ReconnectMaxBackoff:     10 * time.Second,
			ReconnectJitter:         0.2,
			ReadBufferSize:          64 * 1024,
			WriteBufferSize:         16 * 1024,
		},
		Orderbook: OrderbookConfig{
			MaxDepth: 400,
		},
		Logger:    NoopLogger(),
		UserAgent: "go-gate/v2",
	}
}

// withDefaults returns a Config where empty fields are filled from DefaultConfig().
// The user-supplied Config is never mutated (value receiver).
func (c Config) withDefaults() Config {
	var def Config = DefaultConfig()

	if c.Settle == "" {
		c.Settle = def.Settle
	}

	// REST endpoint: testnet swaps the default base URL unless the user set one.
	var defRestBase string = def.REST.BaseURL
	if c.Testnet {
		defRestBase = TestnetRestBaseURL
	}
	if c.REST.BaseURL == "" {
		c.REST.BaseURL = defRestBase
	}
	if c.REST.RequestTimeout == 0 {
		c.REST.RequestTimeout = def.REST.RequestTimeout
	}
	if c.REST.MaxIdleConns == 0 {
		c.REST.MaxIdleConns = def.REST.MaxIdleConns
	}
	if c.REST.MaxIdleConnsPerHost == 0 {
		c.REST.MaxIdleConnsPerHost = def.REST.MaxIdleConnsPerHost
	}
	if c.REST.IdleConnTimeout == 0 {
		c.REST.IdleConnTimeout = def.REST.IdleConnTimeout
	}

	// WS endpoint: testnet swaps the default futures URL unless the user set one.
	var defWsFutures string = def.WS.FuturesURL
	if c.Testnet {
		defWsFutures = TestnetWsFuturesURL
	}
	if c.WS.FuturesURL == "" {
		c.WS.FuturesURL = defWsFutures
	}
	// Spot WS host has no testnet variant (Gate testnet is futures-only).
	if c.WS.SpotURL == "" {
		c.WS.SpotURL = def.WS.SpotURL
	}
	// Delivery WS endpoint: testnet swaps the default unless the user set one.
	var defWsDelivery string = def.WS.DeliveryURL
	if c.Testnet {
		defWsDelivery = TestnetWsDeliveryURL
	}
	if c.WS.DeliveryURL == "" {
		c.WS.DeliveryURL = defWsDelivery
	}
	// Options WS endpoint: testnet swaps the default unless the user set one.
	var defWsOptions string = def.WS.OptionsURL
	if c.Testnet {
		defWsOptions = TestnetWsOptionsURL
	}
	if c.WS.OptionsURL == "" {
		c.WS.OptionsURL = defWsOptions
	}
	if c.WS.HandshakeTimeout == 0 {
		c.WS.HandshakeTimeout = def.WS.HandshakeTimeout
	}
	if c.WS.ReadTimeout == 0 {
		c.WS.ReadTimeout = def.WS.ReadTimeout
	}
	if c.WS.WriteTimeout == 0 {
		c.WS.WriteTimeout = def.WS.WriteTimeout
	}
	if c.WS.PingInterval == 0 {
		c.WS.PingInterval = def.WS.PingInterval
	}
	if c.WS.ReconnectInitialBackoff == 0 {
		c.WS.ReconnectInitialBackoff = def.WS.ReconnectInitialBackoff
	}
	if c.WS.ReconnectMaxBackoff == 0 {
		c.WS.ReconnectMaxBackoff = def.WS.ReconnectMaxBackoff
	}
	if c.WS.ReconnectJitter == 0 {
		c.WS.ReconnectJitter = def.WS.ReconnectJitter
	}
	if c.WS.ReadBufferSize == 0 {
		c.WS.ReadBufferSize = def.WS.ReadBufferSize
	}
	if c.WS.WriteBufferSize == 0 {
		c.WS.WriteBufferSize = def.WS.WriteBufferSize
	}

	if c.Orderbook.MaxDepth == 0 {
		c.Orderbook.MaxDepth = def.Orderbook.MaxDepth
	}

	if c.Logger == nil {
		c.Logger = NoopLogger()
	}
	if c.UserAgent == "" {
		c.UserAgent = def.UserAgent
	}

	return c
}

// validate checks required URL fields. Credentials are not enforced here:
// public REST/WS work without keys; the Signer tracks an `enabled` flag and
// enforces credentials at call time.
func (c Config) validate() error {
	if c.REST.BaseURL == "" {
		return NewError(ErrorKindInvalidRequest, "", "config: REST.BaseURL is empty", nil)
	}
	if c.WS.FuturesURL == "" {
		return NewError(ErrorKindInvalidRequest, "", "config: WS.FuturesURL is empty", nil)
	}
	return nil
}
