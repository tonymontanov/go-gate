/*
FILE: spot/client.go

DESCRIPTION:
Root client for the Gate Spot section. Holds a reference to the parent gate.Client
(REST, Signer, Logger, Config) and exposes domain sub-clients
(Trading/Account/MarketData/Stream). Spot REST paths are absolute "/spot/..."
(there is no settle segment like futures).

The init() function registers the factory with the root package so that
gate.Client.Spot() returns *spot.Client (see RegisterSpotFactory).
*/

package spot

import (
	gate "github.com/tonymontanov/go-gate/v2"
)

// Client — Gate Spot section client.
type Client struct {
	parent *gate.Client

	trading    *TradingClient
	account    *AccountClient
	marketData *MarketDataClient
	stream     *StreamClient
}

// NewClient creates a Spot client. The parent argument is required.
func NewClient(parent *gate.Client) *Client {
	if parent == nil {
		return nil
	}
	var c *Client = &Client{parent: parent}
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

// Account returns the account/balance sub-client.
func (c *Client) Account() *AccountClient { return c.account }

// MarketData returns the market-data sub-client.
func (c *Client) MarketData() *MarketDataClient { return c.marketData }

// Stream returns the WebSocket subscription sub-client.
func (c *Client) Stream() *StreamClient { return c.stream }

// logger / rest / config / signer — internal shortcuts for sub-clients.
func (c *Client) logger() gate.Logger { return c.parent.Logger() }
func (c *Client) rest() restDoer      { return c.parent.REST() }
func (c *Client) config() gate.Config { return c.parent.Config() }
func (c *Client) signerEnabled() bool { return c.parent.Signer().Enabled() }

// init registers the factory in the root package so that gate.Client.Spot()
// lazily returns *spot.Client. A blank import of this package is still required
// to pull the registration into the binary.
func init() {
	gate.RegisterSpotFactory(func(parent *gate.Client) any {
		return NewClient(parent)
	})
}
