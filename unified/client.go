/*
FILE: unified/client.go

DESCRIPTION:
Root client for the Gate Unified Account section (unified / portfolio-margin
account). Holds a reference to the parent gate.Client (REST, Signer, Logger,
Config) and exposes the section's endpoints directly as methods.

The unified account is a single logical domain (cross-currency / portfolio
margin spanning spot, futures and options), so — unlike futures/spot/options —
there is no Trading/Account/Stream sub-client split: every endpoint is a method
on this Client. The section is REST-only (no WebSocket).

Like options (and unlike futures/delivery), the unified section is NOT
settle-scoped: there is no settle field and all paths are built as "/unified/...".

The init() function registers the factory with the root package so that
gate.Client.Unified() returns *unified.Client (see RegisterUnifiedFactory).

NOTE (calibration): endpoint/request/response shapes are modeled on Gate's
unified-account docs; verify field exactness live.
*/

package unified

import (
	"net/url"

	gate "github.com/tonymontanov/go-gate/v2"
)

// Client — Gate Unified Account section client.
type Client struct {
	parent *gate.Client
}

// NewClient creates a Unified client. The parent argument is required.
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

// basePath returns "/unified". The unified section is NOT settle-scoped, so
// (unlike futures/delivery) there is no "{settle}" path segment.
func (c *Client) basePath() string { return "/unified" }

// newQuery returns an empty url.Values for building REST query strings.
func newQuery() url.Values { return url.Values{} }

// init registers the factory in the root package so that gate.Client.Unified()
// lazily returns *unified.Client. A blank import of this package is still
// required to pull the registration into the binary.
func init() {
	gate.RegisterUnifiedFactory(func(parent *gate.Client) any {
		return NewClient(parent)
	})
}
