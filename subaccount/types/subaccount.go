/*
FILE: subaccount/types/subaccount.go

DESCRIPTION:
Sub-account structs and request inputs for the Gate SUB-ACCOUNT section:

  - SubAccount               — a single sub-account of the main account
    (GET/POST /sub_accounts, GET /sub_accounts/{user_id}).
  - CreateSubAccountRequest  — input for Client.Create.

Time fields are normalized to epoch milliseconds (...Ms). The numeric Gate
"state"/"type" enums are surfaced as-is (int64) since Gate documents them only by
value.
*/

package types

// SubAccount — a normalized Gate sub-account.
type SubAccount struct {
	// UserID — the sub-account user id.
	UserID string
	// LoginName — the sub-account login name.
	LoginName string
	// Remark — a free-form note attached to the sub-account.
	Remark string
	// Email — the sub-account email, when set.
	Email string
	// State — the Gate sub-account state (e.g. 1 normal, 2 locked), surfaced
	// as-is.
	State int64
	// Type — the Gate sub-account type, surfaced as-is.
	Type int64
	// CreatedAtMs — creation time in epoch milliseconds.
	CreatedAtMs int64
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// CreateSubAccountRequest — input for Client.Create (POST /sub_accounts).
type CreateSubAccountRequest struct {
	// LoginName — the new sub-account login name. Required.
	LoginName string
	// Remark — an optional free-form note.
	Remark string
	// Password — an optional login password for the sub-account.
	Password string
	// Email — an optional sub-account email.
	Email string
}
