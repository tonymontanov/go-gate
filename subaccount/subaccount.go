/*
FILE: subaccount/subaccount.go

DESCRIPTION:
The Gate Sub-Account section endpoints, implemented directly on
*subaccount.Client. The section is a single flat namespace under
"/sub_accounts/...". Every endpoint is signed. Implements:

  - List      : GET    /sub_accounts?type=
  - Create    : POST   /sub_accounts
  - Get       : GET    /sub_accounts/{user_id}
  - ListKeys  : GET    /sub_accounts/{user_id}/keys
  - CreateKey : POST   /sub_accounts/{user_id}/keys
  - GetKey    : GET    /sub_accounts/{user_id}/keys/{key}
  - UpdateKey : PUT    /sub_accounts/{user_id}/keys/{key}
  - DeleteKey : DELETE /sub_accounts/{user_id}/keys/{key}
  - Lock      : POST   /sub_accounts/{user_id}/lock
  - Unlock    : POST   /sub_accounts/{user_id}/unlock

Epoch seconds are normalized to milliseconds. A created key's secret is returned
ONCE (by CreateKey) and never again.
*/

package subaccount

import (
	"context"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/internal/rest"
	"github.com/tonymontanov/go-gate/v2/subaccount/types"
)

// ---- paths -----------------------------------------------------------------

func (c *Client) subAccountsPath() string { return c.basePath() }
func (c *Client) subAccountPath(userID string) string {
	return c.basePath() + "/" + userID
}
func (c *Client) keysPath(userID string) string {
	return c.basePath() + "/" + userID + "/keys"
}
func (c *Client) keyPath(userID, key string) string {
	return c.basePath() + "/" + userID + "/keys/" + key
}
func (c *Client) lockPath(userID string) string {
	return c.basePath() + "/" + userID + "/lock"
}
func (c *Client) unlockPath(userID string) string {
	return c.basePath() + "/" + userID + "/unlock"
}

// ---- sub-accounts ----------------------------------------------------------

type subAccountPayload struct {
	UserID     int64  `json:"user_id"`
	LoginName  string `json:"login_name"`
	Remark     string `json:"remark"`
	Email      string `json:"email"`
	State      int64  `json:"state"`
	Type       int64  `json:"type"`
	CreateTime int64  `json:"create_time"`
}

func subAccountFromPayload(p *subAccountPayload, rateLimits map[string]string) types.SubAccount {
	return types.SubAccount{
		UserID:      idString(p.UserID),
		LoginName:   p.LoginName,
		Remark:      p.Remark,
		Email:       p.Email,
		State:       p.State,
		Type:        p.Type,
		CreatedAtMs: secondsToMs(p.CreateTime),
		RateLimits:  rateLimits,
	}
}

// List returns the caller's sub-accounts. Pass an empty typeFilter for every
// sub-account; otherwise restrict to a Gate sub-account type value.
func (c *Client) List(ctx context.Context, typeFilter string) ([]types.SubAccount, error) {
	var q = newQuery()
	if typeFilter != "" {
		q.Set("type", typeFilter)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.subAccountsPath(),
		Query:  q,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []subAccountPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "subaccount.List: parse", err)
	}
	var out []types.SubAccount = make([]types.SubAccount, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, subAccountFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// Create creates a new sub-account under the main account.
func (c *Client) Create(ctx context.Context, req types.CreateSubAccountRequest) (types.SubAccount, error) {
	var info types.SubAccount
	if req.LoginName == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "subaccount.Create: LoginName is empty", nil)
	}

	var body map[string]any = make(map[string]any, 4)
	body["login_name"] = req.LoginName
	if req.Remark != "" {
		body["remark"] = req.Remark
	}
	if req.Password != "" {
		body["password"] = req.Password
	}
	if req.Email != "" {
		body["email"] = req.Email
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   c.subAccountsPath(),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p subAccountPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "subaccount.Create: parse", err)
	}
	return subAccountFromPayload(&p, rateLimits), nil
}

// Get returns a single sub-account by user id.
func (c *Client) Get(ctx context.Context, userID string) (types.SubAccount, error) {
	var info types.SubAccount
	if userID == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "subaccount.Get: userID is empty", nil)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.subAccountPath(userID),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p subAccountPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "subaccount.Get: parse", err)
	}
	return subAccountFromPayload(&p, rateLimits), nil
}

// ---- API keys --------------------------------------------------------------

type subAccountKeyPayload struct {
	UserID      string              `json:"user_id"`
	Name        string              `json:"name"`
	Key         string              `json:"key"`
	Secret      string              `json:"secret"`
	Perms       []permissionPayload `json:"perms"`
	IPWhitelist []string            `json:"ip_whitelist"`
	State       int64               `json:"state"`
	Mode        int64               `json:"mode"`
	CreatedAt   int64               `json:"created_at"`
	UpdatedAt   int64               `json:"updated_at"`
	LastAccess  int64               `json:"last_access"`
}

func keyFromPayload(p *subAccountKeyPayload, rateLimits map[string]string) types.SubAccountKey {
	return types.SubAccountKey{
		UserID:       p.UserID,
		Name:         p.Name,
		Key:          p.Key,
		Secret:       p.Secret,
		Perms:        permsFromPayload(p.Perms),
		IPWhitelist:  p.IPWhitelist,
		State:        p.State,
		Mode:         p.Mode,
		CreatedAtMs:  secondsToMs(p.CreatedAt),
		UpdatedAtMs:  secondsToMs(p.UpdatedAt),
		LastAccessMs: secondsToMs(p.LastAccess),
		RateLimits:   rateLimits,
	}
}

// ListKeys returns the API keys of a sub-account.
func (c *Client) ListKeys(ctx context.Context, userID string) ([]types.SubAccountKey, error) {
	if userID == "" {
		return nil, gate.NewError(gate.ErrorKindInvalidRequest, "", "subaccount.ListKeys: userID is empty", nil)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.keysPath(userID),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return nil, err
	}
	var payloads []subAccountKeyPayload
	if err = resp.UnmarshalData(&payloads); err != nil {
		return nil, gate.NewError(gate.ErrorKindUnknown, "", "subaccount.ListKeys: parse", err)
	}
	var out []types.SubAccountKey = make([]types.SubAccountKey, 0, len(payloads))
	var k int
	for k = 0; k < len(payloads); k++ {
		out = append(out, keyFromPayload(&payloads[k], rateLimits))
	}
	return out, nil
}

// CreateKey creates an API key on a sub-account. The returned SubAccountKey is
// the ONLY time Gate reveals the key's Secret.
func (c *Client) CreateKey(ctx context.Context, userID string, req types.CreateKeyRequest) (types.SubAccountKey, error) {
	var info types.SubAccountKey
	if userID == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "subaccount.CreateKey: userID is empty", nil)
	}
	if req.Name == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "subaccount.CreateKey: Name is empty", nil)
	}
	if len(req.Perms) == 0 {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "subaccount.CreateKey: Perms is empty", nil)
	}

	var body map[string]any = make(map[string]any, 3)
	body["name"] = req.Name
	body["perms"] = permsToBody(req.Perms)
	if len(req.IPWhitelist) > 0 {
		body["ip_whitelist"] = req.IPWhitelist
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   c.keysPath(userID),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p subAccountKeyPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "subaccount.CreateKey: parse", err)
	}
	return keyFromPayload(&p, rateLimits), nil
}

// GetKey returns a single API key of a sub-account (Secret is never populated).
func (c *Client) GetKey(ctx context.Context, userID, key string) (types.SubAccountKey, error) {
	var info types.SubAccountKey
	if userID == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "subaccount.GetKey: userID is empty", nil)
	}
	if key == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "subaccount.GetKey: key is empty", nil)
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "GET",
		Path:   c.keyPath(userID, key),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p subAccountKeyPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "subaccount.GetKey: parse", err)
	}
	return keyFromPayload(&p, rateLimits), nil
}

// UpdateKey replaces the permissions (and optionally the IP allow list) of a
// sub-account API key. Gate may return no content; the returned SubAccountKey is
// the parsed body when present.
func (c *Client) UpdateKey(ctx context.Context, userID, key string, req types.UpdateKeyRequest) (types.SubAccountKey, error) {
	var info types.SubAccountKey
	if userID == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "subaccount.UpdateKey: userID is empty", nil)
	}
	if key == "" {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "subaccount.UpdateKey: key is empty", nil)
	}
	if len(req.Perms) == 0 {
		return info, gate.NewError(gate.ErrorKindInvalidRequest, "", "subaccount.UpdateKey: Perms is empty", nil)
	}

	var body map[string]any = make(map[string]any, 2)
	body["perms"] = permsToBody(req.Perms)
	if len(req.IPWhitelist) > 0 {
		body["ip_whitelist"] = req.IPWhitelist
	}

	var resp rest.Response
	var rateLimits map[string]string
	var err error
	resp, rateLimits, err = c.rest().Do(ctx, rest.Options{
		Method: "PUT",
		Path:   c.keyPath(userID, key),
		Body:   body,
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	if err != nil {
		return info, err
	}
	var p subAccountKeyPayload
	if err = resp.UnmarshalData(&p); err != nil {
		return info, gate.NewError(gate.ErrorKindUnknown, "", "subaccount.UpdateKey: parse", err)
	}
	return keyFromPayload(&p, rateLimits), nil
}

// DeleteKey deletes a sub-account API key. Gate returns no content on success.
func (c *Client) DeleteKey(ctx context.Context, userID, key string) error {
	if userID == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "subaccount.DeleteKey: userID is empty", nil)
	}
	if key == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "subaccount.DeleteKey: key is empty", nil)
	}

	var err error
	_, _, err = c.rest().Do(ctx, rest.Options{
		Method: "DELETE",
		Path:   c.keyPath(userID, key),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	return err
}

// ---- lock / unlock ---------------------------------------------------------

// Lock freezes a sub-account (it can no longer trade or transfer). Gate returns
// no content on success.
func (c *Client) Lock(ctx context.Context, userID string) error {
	if userID == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "subaccount.Lock: userID is empty", nil)
	}

	var err error
	_, _, err = c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   c.lockPath(userID),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	return err
}

// Unlock unfreezes a previously locked sub-account. Gate returns no content on
// success.
func (c *Client) Unlock(ctx context.Context, userID string) error {
	if userID == "" {
		return gate.NewError(gate.ErrorKindInvalidRequest, "", "subaccount.Unlock: userID is empty", nil)
	}

	var err error
	_, _, err = c.rest().Do(ctx, rest.Options{
		Method: "POST",
		Path:   c.unlockPath(userID),
		Signed: true,
		Meta:   rest.RequestMeta{Category: string(gate.RateLimitCategoryQuery)},
	})
	return err
}
