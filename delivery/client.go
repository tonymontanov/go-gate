/*
FILE: delivery/client.go

DESCRIPTION:
Root client for the Gate Delivery section (dated/quarterly USD-M futures,
settle=usdt). Holds a reference to the parent gate.Client (REST, Signer, Logger,
Config) and exposes domain sub-clients (Trading/Account/MarketData/Stream). The
delivery API mirrors the perpetual Futures API but lives under
"/delivery/{settle}/..." and trades DATED contracts (e.g. "BTC_USDT_20240329",
ASSET_SETTLE_YYYYMMDD) that settle at expiry (no funding).

The settle currency is read once from the parent config (shared with the futures
section; "usdt"); all delivery paths are built as "/delivery/{settle}/...".

The init() function registers the factory with the root package so that
gate.Client.Delivery() returns *delivery.Client (see RegisterDeliveryFactory).

NOTE (calibration): the SDK is naming-agnostic — callers pass the full dated
contract name. Endpoint/request/response shapes are modeled on the futures
section and Gate's delivery docs; verify field exactness live.
*/

package delivery

import (
	gate "github.com/tonymontanov/go-gate/v2"
)

// Client — Gate Delivery section client.
type Client struct {
	parent *gate.Client
	settle string

	trading    *TradingClient
	account    *AccountClient
	marketData *MarketDataClient
	stream     *StreamClient
}

// NewClient creates a Delivery client. The parent argument is required.
func NewClient(parent *gate.Client) *Client {
	if parent == nil {
		return nil
	}
	var c *Client = &Client{
		parent: parent,
		settle: parent.Config().Settle,
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

// Settle returns the settlement currency this section operates on (e.g. "usdt").
func (c *Client) Settle() string { return c.settle }

// logger / rest / signerEnabled — internal shortcuts for sub-clients.
func (c *Client) logger() gate.Logger { return c.parent.Logger() }
func (c *Client) rest() restDoer      { return c.parent.REST() }
func (c *Client) signerEnabled() bool { return c.parent.Signer().Enabled() }

// basePath returns "/delivery/{settle}".
func (c *Client) basePath() string { return "/delivery/" + c.settle }

// init registers the factory in the root package so that gate.Client.Delivery()
// lazily returns *delivery.Client. A blank import of this package is still
// required to pull the registration into the binary.
func init() {
	gate.RegisterDeliveryFactory(func(parent *gate.Client) any {
		return NewClient(parent)
	})
}
