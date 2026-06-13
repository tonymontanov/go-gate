/*
FILE: options/client.go

DESCRIPTION:
Root client for the Gate Options section (European-style crypto options). Holds a
reference to the parent gate.Client (REST, Signer, Logger, Config) and exposes
domain sub-clients (Trading/Account/MarketData/Stream).

Unlike futures/delivery, the Options section is NOT settle-scoped: there is no
settle field and all paths are built as "/options/...". The WebSocket uses a
dedicated options host (Config.WS.OptionsURL) and the "options.*" channel
namespace.

The init() function registers the factory with the root package so that
gate.Client.Options() returns *options.Client (see RegisterOptionsFactory).

NOTE (calibration): the SDK is naming-agnostic — callers pass the full options
contract name (ASSET-YYYYMMDD-STRIKE-C/P). Endpoint/request/response shapes are
modeled on Gate's options docs; verify field exactness live.
*/

package options

import (
	gate "github.com/tonymontanov/go-gate/v2"
)

// Client — Gate Options section client.
type Client struct {
	parent *gate.Client

	trading    *TradingClient
	account    *AccountClient
	marketData *MarketDataClient
	stream     *StreamClient
}

// NewClient creates an Options client. The parent argument is required.
func NewClient(parent *gate.Client) *Client {
	if parent == nil {
		return nil
	}
	var c *Client = &Client{
		parent: parent,
	}
	c.trading = newTradingClient(c)
	c.account = newAccountClient(c)
	c.marketData = newMarketDataClient(c)
	c.stream = newStreamClient(c)
	return c
}

// Parent returns the root gate.Client.
func (c *Client) Parent() *gate.Client { return c.parent }

// Trading returns the trading sub-client.
func (c *Client) Trading() *TradingClient { return c.trading }

// Account returns the account/position sub-client.
func (c *Client) Account() *AccountClient { return c.account }

// MarketData returns the market-data sub-client.
func (c *Client) MarketData() *MarketDataClient { return c.marketData }

// Stream returns the WebSocket subscription sub-client.
func (c *Client) Stream() *StreamClient { return c.stream }

// logger / rest / signerEnabled — internal shortcuts for sub-clients.
func (c *Client) logger() gate.Logger { return c.parent.Logger() }
func (c *Client) rest() restDoer      { return c.parent.REST() }
func (c *Client) signerEnabled() bool { return c.parent.Signer().Enabled() }

// basePath returns "/options". Options is NOT settle-scoped, so (unlike
// futures/delivery) there is no "{settle}" path segment.
func (c *Client) basePath() string { return "/options" }

// init registers the factory in the root package so that gate.Client.Options()
// lazily returns *options.Client. A blank import of this package is still
// required to pull the registration into the binary.
func init() {
	gate.RegisterOptionsFactory(func(parent *gate.Client) any {
		return NewClient(parent)
	})
}
