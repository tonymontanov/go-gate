/*
FILE: margin/types/enums.go

DESCRIPTION:
Enumerated values for the Gate MARGIN domain (isolated + cross). Values map
directly to Gate APIv4 wire strings:

  - LoanSide — a margin loan is either a "lend" (you supply liquidity and earn
    interest) or a "borrow" (you take liquidity and pay interest). Gate carries
    this as the explicit "side" field on the isolated-margin loan endpoints.
  - LoanStatus — lifecycle state of a margin loan as Gate reports it.
  - RepayMode — the repayment mode accepted by the isolated repay endpoint:
    "all" repays the whole outstanding balance, "partial" repays a given amount.
  - AutoRepayStatus — the on/off toggle of the isolated auto-repay setting.
*/

package types

// LoanSide — direction of a margin loan (lend or borrow). Wire-exact strings.
type LoanSide string

const (
	// LoanSideLend — supply liquidity to the margin lending pool and earn interest.
	LoanSideLend LoanSide = "lend"
	// LoanSideBorrow — borrow liquidity from the margin pool and pay interest.
	LoanSideBorrow LoanSide = "borrow"
)

// LoanStatus — lifecycle state of a margin loan. Wire-exact strings.
type LoanStatus string

const (
	// LoanStatusOpen — the loan order is open and (partially) unfilled.
	LoanStatusOpen LoanStatus = "open"
	// LoanStatusLoaned — the loan has been (fully) loaned out / drawn down.
	LoanStatusLoaned LoanStatus = "loaned"
	// LoanStatusFinished — the loan reached a terminal state (repaid/closed).
	LoanStatusFinished LoanStatus = "finished"
	// LoanStatusAutoRepaid — the loan was closed by the auto-repay mechanism.
	LoanStatusAutoRepaid LoanStatus = "auto_repaid"
)

// RepayMode — repayment mode for the isolated loan repay endpoint. Wire-exact.
type RepayMode string

const (
	// RepayModeAll — repay the entire outstanding balance; Amount is ignored.
	RepayModeAll RepayMode = "all"
	// RepayModePartial — repay a specific Amount of the outstanding balance.
	RepayModePartial RepayMode = "partial"
)

// AutoRepayStatus — on/off state of the isolated auto-repay setting. Wire-exact.
type AutoRepayStatus string

const (
	// AutoRepayOn — auto-repay is enabled.
	AutoRepayOn AutoRepayStatus = "on"
	// AutoRepayOff — auto-repay is disabled.
	AutoRepayOff AutoRepayStatus = "off"
)
