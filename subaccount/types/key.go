/*
FILE: subaccount/types/key.go

DESCRIPTION:
API-key structs and request inputs for the Gate SUB-ACCOUNT section:

  - Permission        — one capability grant on an API key (a named scope plus its
    read-only flag).
  - SubAccountKey     — a sub-account API key with its permissions and IP allow
    list (GET/POST/PUT /sub_accounts/{user_id}/keys[/{key}]). The Secret is only
    populated by Create — Gate never returns it again.
  - CreateKeyRequest  — input for Client.CreateKey.
  - UpdateKeyRequest  — input for Client.UpdateKey.

Time fields are normalized to epoch milliseconds (...Ms).
*/

package types

// Permission — one capability grant on a sub-account API key.
type Permission struct {
	// Name — the Gate permission scope, e.g. "wallet", "spot", "futures",
	// "margin", "account".
	Name string
	// ReadOnly — whether the scope is granted read-only (no write actions).
	ReadOnly bool
}

// SubAccountKey — a sub-account API key.
type SubAccountKey struct {
	// UserID — the owning sub-account user id.
	UserID string
	// Name — the key label.
	Name string
	// Key — the API key (public part).
	Key string
	// Secret — the API secret. Populated ONLY by CreateKey; empty on reads,
	// because Gate never returns the secret again.
	Secret string
	// Perms — the permission grants on the key.
	Perms []Permission
	// IPWhitelist — the allowed source IPs (empty = unrestricted).
	IPWhitelist []string
	// State — the Gate key state, surfaced as-is.
	State int64
	// Mode — the Gate key mode, surfaced as-is.
	Mode int64
	// CreatedAtMs — creation time in epoch milliseconds.
	CreatedAtMs int64
	// UpdatedAtMs — last-update time in epoch milliseconds.
	UpdatedAtMs int64
	// LastAccessMs — last-access time in epoch milliseconds.
	LastAccessMs int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// CreateKeyRequest — input for Client.CreateKey
// (POST /sub_accounts/{user_id}/keys).
type CreateKeyRequest struct {
	// Name — the key label. Required.
	Name string
	// Perms — the permission grants to attach. Required (non-empty).
	Perms []Permission
	// IPWhitelist — optional allowed source IPs (empty = unrestricted).
	IPWhitelist []string
}

// UpdateKeyRequest — input for Client.UpdateKey
// (PUT /sub_accounts/{user_id}/keys/{key}).
type UpdateKeyRequest struct {
	// Perms — the replacement permission grants. Required (non-empty).
	Perms []Permission
	// IPWhitelist — optional replacement allowed source IPs.
	IPWhitelist []string
}
