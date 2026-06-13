/*
FILE: examples/unified/main.go

DESCRIPTION:
A minimal runnable example for the go-gate unified-account section (the unified /
portfolio-margin account). It performs the public currency-list read and the
public collateral discount tiers, and — when GATE_API_KEY / GATE_API_SECRET are
set — reads the signed unified account snapshot, the account mode and the
estimated borrow rate.

Run:

	GATE_API_KEY=... GATE_API_SECRET=... go run ./examples/unified

Without credentials it runs the public calls only.

NOTE: the unified section is calibration-pending (endpoint/field exactness is
modeled on Gate's unified-account docs); verify against a live environment.
*/

package main

import (
	"context"
	"log"
	"os"
	"time"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/unified"
	utypes "github.com/tonymontanov/go-gate/v2/unified/types"
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

	var u *unified.Client = client.Unified().(*unified.Client)

	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// --- public: borrowable currencies ---
	var currencies []utypes.UnifiedCurrency
	currencies, err = u.ListCurrencies(ctx)
	if err != nil {
		log.Fatalf("list currencies: %v", err)
	}
	log.Printf("unified borrowable currencies: %d", len(currencies))
	var n int = len(currencies)
	if n > 5 {
		n = 5
	}
	var i int
	for i = 0; i < n; i++ {
		log.Printf("  %s: loan_status=%s user_max_borrow=%s discount=%s",
			currencies[i].Name, currencies[i].LoanStatus,
			currencies[i].UserMaxBorrowAmount, currencies[i].Discount)
	}

	// --- public: collateral discount tiers ---
	var tiers []utypes.DiscountTier
	tiers, err = u.ListCurrencyDiscountTiers(ctx)
	if err != nil {
		log.Printf("list discount tiers: %v", err)
	} else {
		log.Printf("discount-tier currencies: %d", len(tiers))
	}

	// --- private: signed reads (with credentials) ---
	if os.Getenv("GATE_API_KEY") != "" && os.Getenv("GATE_API_SECRET") != "" {
		var acc utypes.UnifiedAccount
		acc, err = u.GetAccount(ctx, "", 0)
		if err != nil {
			log.Printf("get account: %v", err)
		} else {
			log.Printf("account: user=%d total=%s borrowed=%s equity=%s balances=%d",
				acc.UserID, acc.Total, acc.Borrowed, acc.Equity, len(acc.Balances))
		}

		var mode utypes.UnifiedMode
		mode, err = u.GetMode(ctx)
		if err != nil {
			log.Printf("get mode: %v", err)
		} else {
			log.Printf("account mode: %s settings=%v", mode.Mode, mode.Settings)
		}

		if len(currencies) > 0 {
			var rate utypes.EstimateRate
			rate, err = u.GetEstimateRate(ctx, []string{currencies[0].Name})
			if err != nil {
				log.Printf("estimate rate: %v", err)
			} else {
				log.Printf("estimate rate %s: %s", currencies[0].Name, rate.Rates[currencies[0].Name])
			}
		}
	} else {
		log.Printf("no credentials set — skipping private reads")
	}
}
