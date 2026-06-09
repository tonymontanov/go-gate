/*
FILE: client.go

DESCRIPTION:
The main public SDK Client. Holds shared resources (REST client, signer, config,
logger) and provides lazy trading-section sub-clients on demand. In v1.0 only the
Futures section (USD-M perpetuals) is implemented; spot/delivery/options are
reserved for later iterations and register through the same factory mechanism.

MAIN FUNCTIONS:
  - NewClient(cfg)        : constructor with Config validation and defaults.
  - (Client).Futures()    : returns the Futures-section sub-client. Created lazily
                            on first access via the registered factory.
  - (Client).Close()      : releases idle HTTP connections. WS streams terminate
                            on cancellation of their contexts.

IMPORT-CYCLE NOTE:
The futures package imports the root gate package (for gate.Config / gate.NewError
etc.), so the root cannot import futures. Sections register a factory in their
init() via RegisterFuturesFactory; Futures() returns `any`, which the caller
type-asserts to *futures.Client.

DEPENDENCIES:
- internal/auth, internal/rest: signing and REST transport.
- sync: lazy sub-client initialization.
*/

package gate

import (
	"sync"

	"github.com/tonymontanov/go-gate/v2/internal/auth"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
)

// Client — root SDK object.
type Client struct {
	cfg    Config
	signer *auth.Signer
	rest   *rest.Client
	logger Logger

	futuresOnce sync.Once
	futuresVal  any

	spotOnce sync.Once
	spotVal  any
}

// NewClient creates the root SDK client. cfg goes through withDefaults + validate.
// If credentials are set, the Signer is enabled and signs private calls; otherwise
// the client can only access public endpoints.
func NewClient(cfg Config) (*Client, error) {
	cfg = cfg.withDefaults()
	var err error = cfg.validate()
	if err != nil {
		return nil, err
	}

	var signer *auth.Signer = auth.NewSigner(cfg.APIKey, cfg.SecretKey)
	var restCfg rest.Config = rest.Config{
		RequestTimeout:      cfg.REST.RequestTimeout,
		MaxIdleConns:        cfg.REST.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.REST.MaxIdleConnsPerHost,
		IdleConnTimeout:     cfg.REST.IdleConnTimeout,
		ChannelID:           cfg.ChannelID,
	}
	// Forward the event observer through a thin adapter: RateLimitEvent lives in
	// the root gate package and cannot be referenced from internal/rest (import
	// cycle), so rest calls back with flat arguments which we assemble here.
	if cfg.RateLimitEventObserver != nil {
		var userObserver func(RateLimitEvent) = cfg.RateLimitEventObserver
		restCfg.RateLimitEventObserver = func(endpoint, method string, headers map[string]string, meta rest.RequestMeta) {
			userObserver(RateLimitEvent{
				Endpoint:   endpoint,
				Method:     method,
				Headers:    headers,
				OrderCount: meta.OrderCount,
				Symbols:    meta.Symbols,
				Category:   RateLimitCategory(meta.Category),
			})
		}
	}
	var restClient *rest.Client = rest.NewClient(cfg.REST.BaseURL, signer, restCfg, cfg.UserAgent, cfg.Logger)

	return &Client{
		cfg:    cfg,
		signer: signer,
		rest:   restClient,
		logger: cfg.Logger,
	}, nil
}

// Config returns a copy of the final config (after withDefaults). Useful for
// diagnostics and for sections that need transport/WS settings.
func (c *Client) Config() Config { return c.cfg }

// Logger returns the current logger.
func (c *Client) Logger() Logger { return c.logger }

// Signer returns the internal/auth.Signer (for internal SDK sub-packages).
// User code should not access the signer directly.
func (c *Client) Signer() *auth.Signer { return c.signer }

// REST returns the internal/rest.Client (for internal SDK sub-packages).
func (c *Client) REST() *rest.Client { return c.rest }

// Close releases resources (idle HTTP connections). Safe to call multiple times.
// WS streams terminate on cancellation of their contexts.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.rest.Close()
	return nil
}

// futuresClientFactory — futures client builder, registered by the futures
// package in init() to avoid the import cycle.
var futuresClientFactory func(c *Client) any

// RegisterFuturesFactory registers the futures client factory. Must be called
// from the futures package's init(). Idempotent.
//
// Importing the futures package for its side effect enables the section:
//
//	import _ "github.com/tonymontanov/go-gate/v2/futures"
//
// The root gate package does NOT import futures — this avoids the import cycle
// (futures imports the root for gate.Config / gate.NewError etc.).
func RegisterFuturesFactory(f func(c *Client) any) {
	if futuresClientFactory == nil {
		futuresClientFactory = f
	}
}

// Futures returns the futures sub-client. The return type is any because the
// root package cannot import futures (which imports the root). The caller
// immediately type-asserts to *futures.Client.
//
// Usage idiom:
//
//	var fut *futures.Client = client.Futures().(*futures.Client)
//
// Lazy: created on first access via the registered factory. If the futures
// package is not imported (factory not registered) — returns nil and warns.
func (c *Client) Futures() any {
	c.futuresOnce.Do(func() {
		if futuresClientFactory == nil {
			c.logger.Warn("gate.Client.Futures: futures factory is not registered; import _ \"github.com/tonymontanov/go-gate/v2/futures\"")
			return
		}
		c.futuresVal = futuresClientFactory(c)
	})
	return c.futuresVal
}

// spotClientFactory — spot client builder, registered by the spot package in
// init() to avoid the import cycle (the spot package imports the root gate
// package, so the root cannot import spot).
var spotClientFactory func(c *Client) any

// RegisterSpotFactory registers the spot client factory. Must be called from the
// spot package's init(). Idempotent.
//
// Importing the spot package for its side effect enables the section:
//
//	import _ "github.com/tonymontanov/go-gate/v2/spot"
func RegisterSpotFactory(f func(c *Client) any) {
	if spotClientFactory == nil {
		spotClientFactory = f
	}
}

// Spot returns the spot sub-client. The return type is any because the root
// package cannot import spot (which imports the root). The caller immediately
// type-asserts to *spot.Client.
//
// Usage idiom:
//
//	var sp *spot.Client = client.Spot().(*spot.Client)
//
// Lazy: created on first access via the registered factory. If the spot package
// is not imported (factory not registered) — returns nil and warns.
func (c *Client) Spot() any {
	c.spotOnce.Do(func() {
		if spotClientFactory == nil {
			c.logger.Warn("gate.Client.Spot: spot factory is not registered; import _ \"github.com/tonymontanov/go-gate/v2/spot\"")
			return
		}
		c.spotVal = spotClientFactory(c)
	})
	return c.spotVal
}
