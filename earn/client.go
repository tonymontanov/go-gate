/*
FILE: earn/client.go

DESCRIPTION:
Root client for the Gate Earn "Uni" flexible-lending section. Holds a reference
to the parent gate.Client (REST, Signer, Logger, Config) and exposes the lending
domain directly as methods on earn.Client (one logical domain, no sub-clients).

The Earn section is REST-only (no WebSocket) and is NOT settle-scoped: there is
no settle field and all paths are built under "/earn/uni/...".

The init() function registers the factory with the root package so that
gate.Client.Earn() returns *earn.Client (see RegisterEarnFactory).
*/

package earn

import (
	gate "github.com/tonymontanov/go-gate/v2"
)

// Client — Gate Earn "Uni" flexible-lending section client.
type Client struct {
	parent *gate.Client
}

// NewClient creates an Earn client. The parent argument is required.
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

// logger / rest / signerEnabled — internal shortcuts for the section methods.
func (c *Client) logger() gate.Logger { return c.parent.Logger() }
func (c *Client) rest() restDoer      { return c.parent.REST() }
func (c *Client) signerEnabled() bool { return c.parent.Signer().Enabled() }

// basePath returns "/earn/uni". The Earn section is NOT settle-scoped, so
// (unlike futures/delivery) there is no "{settle}" path segment.
func (c *Client) basePath() string { return "/earn/uni" }

// init registers the factory in the root package so that gate.Client.Earn()
// lazily returns *earn.Client. A blank import of this package is still required
// to pull the registration into the binary.
func init() {
	gate.RegisterEarnFactory(func(parent *gate.Client) any {
		return NewClient(parent)
	})
}
