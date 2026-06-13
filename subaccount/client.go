/*
FILE: subaccount/client.go

DESCRIPTION:
Root client for the Gate Sub-Account section (sub-account creation + API-key
management). Holds a reference to the parent gate.Client (REST, Signer, Logger,
Config) and implements every endpoint directly (there are no domain sub-clients —
the Sub-Account section is a single flat namespace).

The Sub-Account section is REST-only (NO WebSocket) and is NOT settle-scoped:
paths are built as "/sub_accounts/...".

The init() function registers the factory with the root package so that
gate.Client.SubAccount() returns *subaccount.Client (see RegisterSubAccountFactory).

NOTE (calibration): endpoint/request/response shapes are modeled on Gate's
sub-account docs; verify field exactness live.
*/

package subaccount

import (
	gate "github.com/tonymontanov/go-gate/v2"
)

// Client — Gate Sub-Account section client.
type Client struct {
	parent *gate.Client
}

// NewClient creates a Sub-Account client. The parent argument is required.
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

// basePath returns "/sub_accounts". The Sub-Account section is NOT settle-scoped,
// so (unlike futures/delivery) there is no "{settle}" path segment.
func (c *Client) basePath() string { return "/sub_accounts" }

// init registers the factory in the root package so that
// gate.Client.SubAccount() lazily returns *subaccount.Client. A blank import of
// this package is still required to pull the registration into the binary.
func init() {
	gate.RegisterSubAccountFactory(func(parent *gate.Client) any {
		return NewClient(parent)
	})
}
