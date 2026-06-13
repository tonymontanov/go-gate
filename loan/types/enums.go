/*
FILE: loan/types/enums.go

DESCRIPTION:
Enumerated values for the Gate multi-collateral LOAN domain. Values map directly
to Gate APIv4 wire strings:

  - MultiLoanOrderType — whether a loan borrows at the floating ("current") rate
    or a "fixed" rate (with a fixed term).
  - MultiLoanFixedType — the fixed-rate term bucket on a fixed-rate loan.
  - MultiLoanOrderStatus — lifecycle state of a multi-collateral loan order.
  - MortgageType — the collateral operation: "append" adds collateral, "redeem"
    withdraws it.
*/

package types

// MultiLoanOrderType — borrow rate mode of a multi-collateral loan. Wire-exact.
type MultiLoanOrderType string

const (
	// MultiLoanOrderTypeCurrent — borrow at the floating ("current") rate.
	MultiLoanOrderTypeCurrent MultiLoanOrderType = "current"
	// MultiLoanOrderTypeFixed — borrow at a fixed rate for a fixed term.
	MultiLoanOrderTypeFixed MultiLoanOrderType = "fixed"
)

// MultiLoanFixedType — fixed-rate term bucket of a fixed-rate loan. Wire-exact.
type MultiLoanFixedType string

const (
	// MultiLoanFixedType7d — a 7-day fixed-rate term.
	MultiLoanFixedType7d MultiLoanFixedType = "7d"
	// MultiLoanFixedType30d — a 30-day fixed-rate term.
	MultiLoanFixedType30d MultiLoanFixedType = "30d"
)

// MultiLoanOrderStatus — lifecycle state of a multi-collateral loan order.
// Wire-exact strings.
type MultiLoanOrderStatus string

const (
	// MultiLoanOrderStatusInitial — the order is being created.
	MultiLoanOrderStatusInitial MultiLoanOrderStatus = "initial"
	// MultiLoanOrderStatusBorrowed — the loan has been disbursed and is active.
	MultiLoanOrderStatusBorrowed MultiLoanOrderStatus = "borrowed"
	// MultiLoanOrderStatusRepaying — the loan is being repaid.
	MultiLoanOrderStatusRepaying MultiLoanOrderStatus = "repaying"
	// MultiLoanOrderStatusFinished — the loan reached a terminal (repaid) state.
	MultiLoanOrderStatusFinished MultiLoanOrderStatus = "finished"
	// MultiLoanOrderStatusLiquidated — the loan was force-liquidated.
	MultiLoanOrderStatusLiquidated MultiLoanOrderStatus = "liquidated"
)

// MortgageType — collateral operation on a multi-collateral loan. Wire-exact.
type MortgageType string

const (
	// MortgageTypeAppend — add collateral to the loan.
	MortgageTypeAppend MortgageType = "append"
	// MortgageTypeRedeem — withdraw collateral from the loan.
	MortgageTypeRedeem MortgageType = "redeem"
)
