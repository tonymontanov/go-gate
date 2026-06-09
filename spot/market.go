/*
FILE: spot/market.go

DESCRIPTION:
MarketDataClient — spot public market data (currency pairs, order book,
candlesticks, tickers). Endpoint methods are added in milestone S3.
*/

package spot

// MarketDataClient — spot market-data sub-client.
type MarketDataClient struct {
	client *Client
}

// newMarketDataClient constructs the market-data sub-client.
func newMarketDataClient(c *Client) *MarketDataClient {
	return &MarketDataClient{client: c}
}
