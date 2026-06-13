/*
FILE: earn/rest-doer.go

DESCRIPTION:
Minimal restDoer interface for REST calls. Using an interface (rather than
*rest.Client directly) keeps the earn package decoupled from the transport
implementation and lets unit tests inject a fake REST without a real http.Client.
It matches the public signature of *internal/rest.Client.Do exactly.
*/

package earn

import (
	"context"

	"github.com/tonymontanov/go-gate/v2/internal/rest"
)

// restDoer — minimal REST transport contract.
type restDoer interface {
	Do(ctx context.Context, opts rest.Options) (rest.Response, map[string]string, error)
}
