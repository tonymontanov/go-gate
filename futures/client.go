/*
FILE: futures/client.go

DESCRIPTION:
Root client for the Gate Futures section (USD-M perpetuals, settle=usdt in v1.0).
Holds a reference to the parent gate.Client (REST, Signer, Logger, Config) and
exposes domain sub-clients. v1.0 ships Trading; Account/MarketData/Stream are
added in later milestones and slot in via the same construction.

The settle currency is read once from the parent config; all futures paths are
built as "/futures/{settle}/...".

The init() function registers the factory with the root package so that
gate.Client.Futures() returns *futures.Client (see RegisterFuturesFactory).
*/

package futures

import (
	gate "github.com/tonymontanov/go-gate/v2"
)

// Client — Gate Futures section client.
type Client struct {
	parent *gate.Client
	settle string

	trading *TradingClient
}

// NewClient creates a Futures client. The parent argument is required.
func NewClient(parent *gate.Client) *Client {
	if parent == nil {
		return nil
	}
	var c *Client = &Client{
		parent: parent,
		settle: parent.Config().Settle,
	}
	c.trading = newTradingClient(c)
	return c
}

// Parent returns the root gate.Client.
func (c *Client) Parent() *gate.Client { return c.parent }

// Trading returns the trading sub-client.
func (c *Client) Trading() *TradingClient { return c.trading }

// Settle returns the settlement currency this section operates on (e.g. "usdt").
func (c *Client) Settle() string { return c.settle }

// logger / rest / signerEnabled — internal shortcuts for sub-clients.
func (c *Client) logger() gate.Logger { return c.parent.Logger() }
func (c *Client) rest() restDoer      { return c.parent.REST() }
func (c *Client) signerEnabled() bool { return c.parent.Signer().Enabled() }

// basePath returns "/futures/{settle}".
func (c *Client) basePath() string { return "/futures/" + c.settle }

// init registers the factory in the root package so that gate.Client.Futures()
// lazily returns *futures.Client. A blank import of this package is still
// required to pull the registration into the binary.
func init() {
	gate.RegisterFuturesFactory(func(parent *gate.Client) any {
		return NewClient(parent)
	})
}
