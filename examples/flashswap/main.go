/*
FILE: examples/flashswap/main.go

DESCRIPTION:
A minimal runnable example for the go-gate flash-swap section (instant currency
conversion). It exercises the public currency-discovery endpoint and — when
GATE_API_KEY / GATE_API_SECRET are set — performs read-only signed calls (list
orders, preview a tiny swap quote). It does NOT create any swap order: a swap is
a balance-moving action that should be opt-in.

Run:

	GATE_API_KEY=... GATE_API_SECRET=... go run ./examples/flashswap

Without credentials it runs the public calls only.

NOTE: the flash-swap section is calibration-pending (endpoint/field exactness is
modeled on Gate's flash-swap docs); verify against a live environment.
*/

package main

import (
	"context"
	"log"
	"os"
	"time"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/flashswap"
	fstypes "github.com/tonymontanov/go-gate/v2/flashswap/types"
)

func main() {
	var client *gate.Client
	var err error
	client, err = gate.NewClient(gate.Config{
		APIKey:    os.Getenv("GATE_API_KEY"),
		SecretKey: os.Getenv("GATE_API_SECRET"),
	})
	if err != nil {
		log.Fatalf("new client: %v", err)
	}
	defer func() { _ = client.Close() }()

	var fs *flashswap.Client = client.FlashSwap().(*flashswap.Client)

	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// --- public: swappable currencies ---
	var currencies []fstypes.FlashSwapCurrency
	currencies, err = fs.ListCurrencies(ctx)
	if err != nil {
		log.Fatalf("list currencies: %v", err)
	}
	if len(currencies) == 0 {
		log.Fatalf("no flash-swap currencies available")
	}
	var first fstypes.FlashSwapCurrency = currencies[0]
	log.Printf("flash-swap currencies: %d (first %s -> %d buy currencies)",
		len(currencies), first.Currency, len(first.BuyCurrencies))

	// --- private: read-only signed calls (with creds) ---
	if os.Getenv("GATE_API_KEY") != "" && os.Getenv("GATE_API_SECRET") != "" {
		var orders []fstypes.FlashSwapOrder
		orders, err = fs.ListOrders(ctx, flashswap.ListOrdersParams{Limit: 10})
		if err != nil {
			log.Printf("list orders: %v", err)
		} else {
			log.Printf("flash-swap orders: %d", len(orders))
		}

		// A preview is a read-only quote (no balance moves). Only attempt it when
		// the first currency advertises at least one buy currency with a minimum.
		if len(first.BuyCurrencies) > 0 && !first.MinAmount.IsZero() {
			var preview fstypes.FlashSwapPreview
			preview, err = fs.PreviewOrder(ctx, fstypes.PreviewRequest{
				SellCurrency: first.Currency,
				BuyCurrency:  first.BuyCurrencies[0].Currency,
				SellAmount:   first.MinAmount,
			})
			if err != nil {
				log.Printf("preview order: %v", err)
			} else {
				log.Printf("preview %s->%s: sell=%s buy=%s price=%s (preview_id=%s)",
					preview.SellCurrency, preview.BuyCurrency, preview.SellAmount,
					preview.BuyAmount, preview.Price, preview.PreviewID)
			}
		}
	} else {
		log.Printf("no credentials set — skipping private read demo")
	}
}
