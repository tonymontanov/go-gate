/*
FILE: wallet/client.go

DESCRIPTION:
Root client for the Gate Wallet section (cross-account transfers, balances,
fees). Holds a reference to the parent gate.Client (REST, Signer, Logger, Config)
and implements every wallet endpoint directly (there are no domain sub-clients —
the Wallet section is a single flat namespace).

The Wallet section is REST-only (NO WebSocket) and is NOT settle-scoped: paths are
built as "/wallet/...". futures/delivery balance reads carry "settle" only as a
query parameter.

The init() function registers the factory with the root package so that
gate.Client.Wallet() returns *wallet.Client (see RegisterWalletFactory).

NOTE (calibration): endpoint/request/response shapes are modeled on Gate's wallet
docs; verify field exactness live.
*/

package wallet

import (
	gate "github.com/tonymontanov/go-gate/v2"
)

// Client — Gate Wallet section client.
type Client struct {
	parent *gate.Client
}

// NewClient creates a Wallet client. The parent argument is required.
func NewClient(parent *gate.Client) *Client {
	if parent == nil {
		return nil
	}
	var c *Client = &Client{
		parent: parent,
	}
	return c
}

// Parent returns the root gate.Client.
func (c *Client) Parent() *gate.Client { return c.parent }

// logger / rest / signerEnabled — internal shortcuts.
func (c *Client) logger() gate.Logger { return c.parent.Logger() }
func (c *Client) rest() restDoer      { return c.parent.REST() }
func (c *Client) signerEnabled() bool { return c.parent.Signer().Enabled() }

// basePath returns "/wallet". The Wallet section is NOT settle-scoped, so (unlike
// futures/delivery) there is no "{settle}" path segment.
func (c *Client) basePath() string { return "/wallet" }

// init registers the factory in the root package so that gate.Client.Wallet()
// lazily returns *wallet.Client. A blank import of this package is still required
// to pull the registration into the binary.
func init() {
	gate.RegisterWalletFactory(func(parent *gate.Client) any {
		return NewClient(parent)
	})
}
