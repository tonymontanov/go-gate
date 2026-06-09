/*
FILE: spot/account.go

DESCRIPTION:
AccountClient — spot account balances over GET /spot/accounts. Endpoint methods
are added in milestone S3.
*/

package spot

// AccountClient — spot account/balance sub-client.
type AccountClient struct {
	client *Client
}

// newAccountClient constructs the account sub-client.
func newAccountClient(c *Client) *AccountClient {
	return &AccountClient{client: c}
}
