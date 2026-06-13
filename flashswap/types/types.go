/*
FILE: flashswap/types/types.go

DESCRIPTION:
Domain structs for the Gate FLASH SWAP section (instant currency conversion):

  - FlashSwapCurrency — one swappable sell currency with its eligible buy
    currencies and per-currency amount limits (GET /flash_swap/currencies).
  - FlashSwapBuyCurrency — an eligible buy currency (and its amount limits) for a
    given sell currency.
  - FlashSwapOrder      — a flash-swap order (GET/POST /flash_swap/orders,
    GET /flash_swap/orders/{order_id}).
  - FlashSwapPreview    — a quote computed by POST /flash_swap/orders/preview; its
    PreviewID is passed to CreateOrder to execute the swap.
  - PreviewRequest      — input for Client.PreviewOrder.
  - CreateOrderRequest  — input for Client.CreateOrder.

Monetary fields use decimal.Decimal; the wire payloads (decoded in the flashswap
package) use codec.FlexDecimal because Gate may quote an amount/price as a string
or send it as a bare JSON number. Time fields are normalized to epoch
milliseconds (...Ms).
*/

package types

import "github.com/shopspring/decimal"

// FlashSwapBuyCurrency — an eligible buy currency for a given sell currency,
// with its swap amount limits.
type FlashSwapBuyCurrency struct {
	// Currency — the buy currency, e.g. "USDT".
	Currency string
	// MinAmount — minimum swappable amount of this buy currency (0 if unset).
	MinAmount decimal.Decimal
	// MaxAmount — maximum swappable amount of this buy currency (0 if unset).
	MaxAmount decimal.Decimal
}

// FlashSwapCurrency — a swappable sell currency, its amount limits, and the buy
// currencies it can be converted into.
type FlashSwapCurrency struct {
	// Currency — the sell currency, e.g. "BTC".
	Currency string
	// MinAmount — minimum swappable amount of the sell currency (0 if unset).
	MinAmount decimal.Decimal
	// MaxAmount — maximum swappable amount of the sell currency (0 if unset).
	MaxAmount decimal.Decimal
	// BuyCurrencies — the currencies this sell currency can be converted into.
	BuyCurrencies []FlashSwapBuyCurrency
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// FlashSwapOrder — a normalized flash-swap order.
type FlashSwapOrder struct {
	// ID — the Gate order id.
	ID string
	// CreatedAtMs — order creation time in epoch milliseconds.
	CreatedAtMs int64
	// UpdatedAtMs — order last-update time in epoch milliseconds.
	UpdatedAtMs int64
	// UserID — the owning user id.
	UserID int64
	// SellCurrency — the currency sold, e.g. "BTC".
	SellCurrency string
	// SellAmount — the amount of SellCurrency sold.
	SellAmount decimal.Decimal
	// BuyCurrency — the currency bought, e.g. "USDT".
	BuyCurrency string
	// BuyAmount — the amount of BuyCurrency received.
	BuyAmount decimal.Decimal
	// Price — the swap price (BuyCurrency per SellCurrency).
	Price decimal.Decimal
	// Status — order lifecycle state.
	Status FlashSwapOrderStatus
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// FlashSwapPreview — a quote computed by PreviewOrder. Its PreviewID must be
// passed to CreateOrder to execute the swap at the quoted price.
type FlashSwapPreview struct {
	// PreviewID — the Gate preview id, required by CreateOrder.
	PreviewID string
	// SellCurrency — the currency to sell, e.g. "BTC".
	SellCurrency string
	// SellAmount — the (possibly computed) amount of SellCurrency.
	SellAmount decimal.Decimal
	// BuyCurrency — the currency to buy, e.g. "USDT".
	BuyCurrency string
	// BuyAmount — the (possibly computed) amount of BuyCurrency.
	BuyAmount decimal.Decimal
	// Price — the quoted swap price (BuyCurrency per SellCurrency).
	Price decimal.Decimal
	// RateLimits — Gate rate-limit headers from the response (may be empty).
	RateLimits map[string]string
}

// PreviewRequest — input for Client.PreviewOrder
// (POST /flash_swap/orders/preview). Exactly one of SellAmount / BuyAmount is
// supplied; Gate computes the other side and the price.
type PreviewRequest struct {
	// SellCurrency — the currency to sell, e.g. "BTC". Required.
	SellCurrency string
	// BuyCurrency — the currency to buy, e.g. "USDT". Required.
	BuyCurrency string
	// SellAmount — the amount of SellCurrency to sell. Supply this OR BuyAmount.
	SellAmount decimal.Decimal
	// BuyAmount — the amount of BuyCurrency to receive. Supply this OR SellAmount.
	BuyAmount decimal.Decimal
}

// CreateOrderRequest — input for Client.CreateOrder (POST /flash_swap/orders).
// The fields are normally copied from a FlashSwapPreview returned by
// PreviewOrder.
type CreateOrderRequest struct {
	// PreviewID — the preview id obtained from PreviewOrder. Required.
	PreviewID string
	// SellCurrency — the currency to sell, e.g. "BTC". Required.
	SellCurrency string
	// SellAmount — the amount of SellCurrency to sell. Required.
	SellAmount decimal.Decimal
	// BuyCurrency — the currency to buy, e.g. "USDT". Required.
	BuyCurrency string
	// BuyAmount — the amount of BuyCurrency to receive. Required.
	BuyAmount decimal.Decimal
}
