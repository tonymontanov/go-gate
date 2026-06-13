/*
FILE: examples/loan/main.go

DESCRIPTION:
A minimal runnable example for the go-gate multi-collateral loan section. It
exercises the public discovery endpoints (supported currencies, current/fixed
rates) and — when GATE_API_KEY / GATE_API_SECRET are set — performs read-only
signed calls (list orders, list repay records). It does NOT create, repay, or
adjust a loan: borrowing and collateral changes are balance-moving actions that
should be opt-in.

Run:

	GATE_API_KEY=... GATE_API_SECRET=... go run ./examples/loan

Without credentials it runs the public calls only.

NOTE: the loan section is calibration-pending (endpoint/field exactness is
modeled on Gate's multi-collateral loan docs); verify against a live environment.
*/

package main

import (
	"context"
	"log"
	"os"
	"time"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/loan"
	ltypes "github.com/tonymontanov/go-gate/v2/loan/types"
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

	var l *loan.Client = client.Loan().(*loan.Client)

	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// --- public: supported currencies ---
	var currencies []ltypes.MultiLoanCurrency
	currencies, err = l.ListCurrencies(ctx, "")
	if err != nil {
		log.Fatalf("list currencies: %v", err)
	}
	if len(currencies) == 0 {
		log.Fatalf("no multi-collateral loan currencies available")
	}
	log.Printf("loan currencies: %d (first %s, ltv=%s)",
		len(currencies), currencies[0].Currency, currencies[0].Ltv)

	// --- public: current (floating) borrow rate ---
	var rates []ltypes.CurrentRate
	rates, err = l.GetCurrentRate(ctx, []string{currencyOrDefault(currencies)})
	if err != nil {
		log.Printf("current rate: %v", err)
	} else if len(rates) > 0 {
		log.Printf("current rate %s: %s", rates[0].Currency, rates[0].Rate)
	}

	// --- private: read-only signed calls (with creds) ---
	if os.Getenv("GATE_API_KEY") != "" && os.Getenv("GATE_API_SECRET") != "" {
		var orders []ltypes.MultiLoanOrder
		orders, err = l.ListOrders(ctx, loan.ListOrdersParams{Limit: 10})
		if err != nil {
			log.Printf("list orders: %v", err)
		} else {
			log.Printf("loan orders: %d", len(orders))
		}

		var repays []ltypes.RepayRecord
		repays, err = l.ListRepayRecords(ctx, loan.ListRepayRecordsParams{Limit: 10})
		if err != nil {
			log.Printf("list repay records: %v", err)
		} else {
			log.Printf("repay records: %d", len(repays))
		}
	} else {
		log.Printf("no credentials set — skipping private read demo")
	}
}

// currencyOrDefault returns a sensible currency to query, preferring the first
// listed loan currency and falling back to "USDT".
func currencyOrDefault(currencies []ltypes.MultiLoanCurrency) string {
	if len(currencies) > 0 && currencies[0].Currency != "" {
		return currencies[0].Currency
	}
	return "USDT"
}
