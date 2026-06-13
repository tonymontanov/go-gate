/*
FILE: margin/client.go

DESCRIPTION:
Root client for the Gate Margin section (isolated + cross margin). Holds a
reference to the parent gate.Client (REST, Signer, Logger, Config) and exposes
the two domain sub-clients (Isolated/Cross).

The Margin section is REST-only (NO WebSocket) and is NOT settle-scoped: there is
no settle field. Isolated paths are built as "/margin/..." and cross paths as
"/margin/cross/...".

The init() function registers the factory with the root package so that
gate.Client.Margin() returns *margin.Client (see RegisterMarginFactory).

NOTE (calibration): endpoint/request/response shapes are modeled on Gate's margin
docs; verify field exactness live.
*/

package margin

import (
	gate "github.com/tonymontanov/go-gate/v2"
)

// Client — Gate Margin section client.
type Client struct {
	parent *gate.Client

	isolated *IsolatedClient
	cross    *CrossClient
}

// NewClient creates a Margin client. The parent argument is required.
func NewClient(parent *gate.Client) *Client {
	if parent == nil {
		return nil
	}
	var c *Client = &Client{
		parent: parent,
	}
	c.isolated = newIsolatedClient(c)
	c.cross = newCrossClient(c)
	return c
}

// Parent returns the root gate.Client.
func (c *Client) Parent() *gate.Client { return c.parent }

// Isolated returns the isolated-margin sub-client ("/margin/...").
func (c *Client) Isolated() *IsolatedClient { return c.isolated }

// Cross returns the cross-margin sub-client ("/margin/cross/...").
func (c *Client) Cross() *CrossClient { return c.cross }

// logger / rest / signerEnabled — internal shortcuts for sub-clients.
func (c *Client) logger() gate.Logger { return c.parent.Logger() }
func (c *Client) rest() restDoer      { return c.parent.REST() }
func (c *Client) signerEnabled() bool { return c.parent.Signer().Enabled() }

// basePath returns "/margin". Margin is NOT settle-scoped, so (unlike
// futures/delivery) there is no "{settle}" path segment.
func (c *Client) basePath() string { return "/margin" }

// crossBasePath returns "/margin/cross", the prefix for cross-margin endpoints.
func (c *Client) crossBasePath() string { return "/margin/cross" }

// init registers the factory in the root package so that gate.Client.Margin()
// lazily returns *margin.Client. A blank import of this package is still
// required to pull the registration into the binary.
func init() {
	gate.RegisterMarginFactory(func(parent *gate.Client) any {
		return NewClient(parent)
	})
}
