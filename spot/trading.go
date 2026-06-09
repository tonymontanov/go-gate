/*
FILE: spot/trading.go

DESCRIPTION:
TradingClient — spot order management (create / batch / amend / cancel / query)
over the Gate /spot/orders endpoints. Endpoint methods are added in milestone S2.
*/

package spot

// TradingClient — spot trading sub-client.
type TradingClient struct {
	client *Client
}

// newTradingClient constructs the trading sub-client.
func newTradingClient(c *Client) *TradingClient {
	return &TradingClient{client: c}
}
