/*
FILE: loan/client.go

DESCRIPTION:
Root client for the Gate multi-collateral Loan section. Holds a reference to the
parent gate.Client (REST, Signer, Logger, Config) and exposes the loan REST
methods directly (there is a single domain, so there are no sub-clients).

The Loan section is REST-only (NO WebSocket) and is NOT settle-scoped: there is
no settle field. Paths are built as "/loan/multi_collateral/...".

The init() function registers the factory with the root package so that
gate.Client.Loan() returns *loan.Client (see RegisterLoanFactory).

NOTE (calibration): endpoint/request/response shapes are modeled on Gate's
multi-collateral loan docs; verify field exactness live.
*/

package loan

import (
	gate "github.com/tonymontanov/go-gate/v2"
)

// Client — Gate multi-collateral Loan section client.
type Client struct {
	parent *gate.Client
}

// NewClient creates a Loan client. The parent argument is required.
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

// basePath returns "/loan/multi_collateral". The Loan section is NOT
// settle-scoped, so (unlike futures/delivery) there is no "{settle}" segment.
func (c *Client) basePath() string { return "/loan/multi_collateral" }

// init registers the factory in the root package so that gate.Client.Loan()
// lazily returns *loan.Client. A blank import of this package is still required
// to pull the registration into the binary.
func init() {
	gate.RegisterLoanFactory(func(parent *gate.Client) any {
		return NewClient(parent)
	})
}
