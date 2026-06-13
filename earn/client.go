/*
FILE: earn/client.go

DESCRIPTION:
Root client for the Gate Earn section. The Uni flexible-lending domain is exposed
DIRECTLY as methods on earn.Client (base "/earn/uni"), unchanged. Two additional
domains hang off sub-client accessors (mirroring margin's Isolated()/Cross()):

  - FixedTerm() *FixedTermClient — Earn Fixed-Term lending (base "/earn/fixed-term").
  - Dual()      *DualClient      — Dual Investment             (base "/earn/dual").

Each sub-client builds its own paths under its own base; they do NOT reuse
earn.Client.basePath() ("/earn/uni").

The Earn section is REST-only (no WebSocket) and is NOT settle-scoped: there is
no settle field.

The init() function registers the factory with the root package so that
gate.Client.Earn() returns *earn.Client (see RegisterEarnFactory).
*/

package earn

import (
	gate "github.com/tonymontanov/go-gate/v2"
)

// Client — Gate Earn section client. The Uni flexible-lending methods live
// directly on this type; the Fixed-Term and Dual domains are reached via the
// FixedTerm() and Dual() sub-client accessors.
type Client struct {
	parent *gate.Client

	fixedTerm *FixedTermClient
	dual      *DualClient
}

// NewClient creates an Earn client. The parent argument is required.
func NewClient(parent *gate.Client) *Client {
	if parent == nil {
		return nil
	}
	var c *Client = &Client{
		parent: parent,
	}
	c.fixedTerm = newFixedTermClient(c)
	c.dual = newDualClient(c)
	return c
}

// Parent returns the root gate.Client.
func (c *Client) Parent() *gate.Client { return c.parent }

// FixedTerm returns the Earn Fixed-Term lending sub-client ("/earn/fixed-term").
func (c *Client) FixedTerm() *FixedTermClient { return c.fixedTerm }

// Dual returns the Dual Investment sub-client ("/earn/dual").
func (c *Client) Dual() *DualClient { return c.dual }

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
