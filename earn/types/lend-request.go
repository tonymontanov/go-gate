/*
FILE: earn/types/lend-request.go

DESCRIPTION:
Request inputs for the Gate Earn "Uni" lending mutations:
  - CreateLendRequest — POST  /earn/uni/lends  (lend or redeem principal).
  - ChangeLendRequest — PATCH /earn/uni/lends  (adjust the floor rate / auto-renew
    of an existing position WITHOUT moving principal).

UNITS & CONVENTIONS:
  - Amount is the principal to lend or redeem, in the asset's native units.
  - MinRate is the floor annualized rate the caller will accept (optional;
    omitted when zero so Gate applies its default).
  - AutoRenew controls auto-reinvest of accrued interest. It is a tri-state
    pointer because false is a meaningful value distinct from "leave unchanged".
*/

package types

import "github.com/shopspring/decimal"

// CreateLendRequest — parameters for POST /earn/uni/lends.
type CreateLendRequest struct {
	// Currency — the asset to lend or redeem, e.g. "ETH". Required.
	Currency string
	// Amount — principal to lend or redeem (positive magnitude). Required.
	Amount decimal.Decimal
	// Type — LendTypeLend (add principal) or LendTypeRedeem (withdraw). Required.
	Type LendType
	// MinRate — floor annualized rate the caller will accept. Optional: omitted
	// from the request body when zero.
	MinRate decimal.Decimal
	// AutoRenew — auto-reinvest accrued interest. Optional tri-state: nil leaves
	// the field out of the body; a non-nil pointer sends the explicit bool.
	AutoRenew *bool
}

// ChangeLendRequest — parameters for PATCH /earn/uni/lends. Adjusts an existing
// position's floor rate and/or auto-renew flag; it does NOT move principal.
type ChangeLendRequest struct {
	// Currency — the position's asset code, e.g. "ETH". Required.
	Currency string
	// MinRate — new floor annualized rate. Optional: omitted when zero.
	MinRate decimal.Decimal
	// AutoRenew — new auto-reinvest flag. Optional tri-state: nil leaves the
	// field out of the body; a non-nil pointer sends the explicit bool.
	AutoRenew *bool
}
