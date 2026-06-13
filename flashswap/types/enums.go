/*
FILE: flashswap/types/enums.go

DESCRIPTION:
Enumerated values for the Gate FLASH SWAP domain. Gate reports a flash-swap
order's lifecycle as a small integer status code; FlashSwapOrderStatus carries
those codes with named constants and is also accepted as the optional "status"
filter on ListOrders.
*/

package types

// FlashSwapOrderStatus — lifecycle state of a flash-swap order. Gate encodes
// this as an integer on the wire.
type FlashSwapOrderStatus int64

const (
	// FlashSwapOrderStatusInit — order accepted/initiated, not yet settled. Used
	// as the "unset" sentinel for the ListOrders status filter (0 = all).
	FlashSwapOrderStatusInit FlashSwapOrderStatus = 0
	// FlashSwapOrderStatusSuccess — the swap completed successfully.
	FlashSwapOrderStatusSuccess FlashSwapOrderStatus = 1
	// FlashSwapOrderStatusFailed — the swap failed.
	FlashSwapOrderStatusFailed FlashSwapOrderStatus = 2
)
