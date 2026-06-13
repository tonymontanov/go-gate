/*
FILE: flashswap/client.go

DESCRIPTION:
Root client for the Gate Flash Swap section (instant currency conversion). Holds
a reference to the parent gate.Client (REST, Signer, Logger, Config) and exposes
the flash-swap REST methods directly (there is a single domain, so there are no
sub-clients).

The Flash Swap section is REST-only (NO WebSocket) and is NOT settle-scoped:
there is no settle field. Paths are built as "/flash_swap/...".

The init() function registers the factory with the root package so that
gate.Client.FlashSwap() returns *flashswap.Client (see RegisterFlashSwapFactory).

NOTE (calibration): endpoint/request/response shapes are modeled on Gate's
flash-swap docs; verify field exactness live.
*/

package flashswap

import (
	gate "github.com/tonymontanov/go-gate/v2"
)

// Client — Gate Flash Swap section client.
type Client struct {
	parent *gate.Client
}

// NewClient creates a Flash Swap client. The parent argument is required.
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

// basePath returns "/flash_swap". Flash Swap is NOT settle-scoped, so (unlike
// futures/delivery) there is no "{settle}" path segment.
func (c *Client) basePath() string { return "/flash_swap" }

// init registers the factory in the root package so that gate.Client.FlashSwap()
// lazily returns *flashswap.Client. A blank import of this package is still
// required to pull the registration into the binary.
func init() {
	gate.RegisterFlashSwapFactory(func(parent *gate.Client) any {
		return NewClient(parent)
	})
}
