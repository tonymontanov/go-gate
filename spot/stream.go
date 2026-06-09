/*
FILE: spot/stream.go

DESCRIPTION:
StreamClient — spot WebSocket subscriptions (public: book_ticker / tickers /
trades / order_book(_update) / candlesticks; private: orders / usertrades /
balances) over the Gate spot WS host. Watch methods are added in milestone S4.
*/

package spot

// StreamClient — spot WebSocket subscription sub-client.
type StreamClient struct {
	client *Client
}

// newStreamClient constructs the stream sub-client.
func newStreamClient(c *Client) *StreamClient {
	return &StreamClient{client: c}
}
