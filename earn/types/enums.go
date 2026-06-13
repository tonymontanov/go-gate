/*
FILE: earn/types/enums.go

DESCRIPTION:
Enumerated values for the Gate Earn "Uni" flexible-lending domain. Values map
directly to Gate APIv4 wire strings.

  - LendType is the action carried by POST /earn/uni/lends and the type recorded
    on a lend record: "lend" (subscribe / add principal) or "redeem" (withdraw
    principal). It is the only sign-bearing field in the section (Uni lending has
    no buy/sell side).
  - InterestStatus is the on/off compounding (auto-reinvest) flag Gate returns on
    a Uni lend and via GET /earn/uni/interest_status/{currency}. Gate's exact
    wire vocabulary is calibration-pending; the SDK keeps the raw string and
    offers the documented "on"/"off" constants for convenience.
*/

package types

// LendType — Uni lending action (lend = add principal, redeem = withdraw).
// Values are the exact Gate wire strings sent in the POST /earn/uni/lends body
// and returned on a lend record.
type LendType string

const (
	// LendTypeLend — subscribe to / add principal to the flexible-lending pool.
	LendTypeLend LendType = "lend"
	// LendTypeRedeem — withdraw principal from the flexible-lending pool.
	LendTypeRedeem LendType = "redeem"
)

// InterestStatus — Uni interest (auto-reinvest / compounding) status. The wire
// value is kept verbatim; the constants below are the documented on/off forms.
//
// CALIBRATION: Gate's exact interest_status vocabulary is modeled on the Uni
// docs; verify the live string values.
type InterestStatus string

const (
	// InterestStatusOn — interest is being accrued / auto-reinvested.
	InterestStatusOn InterestStatus = "on"
	// InterestStatusOff — interest accrual / auto-reinvest is disabled.
	InterestStatusOff InterestStatus = "off"
)
