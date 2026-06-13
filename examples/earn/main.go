/*
FILE: examples/earn/main.go

DESCRIPTION:
A minimal runnable example for the go-gate Earn "Uni" flexible lending section.
It lists the lendable currencies, prints a currency's amount/rate band, fetches
the current estimated annualized rate and a slice of the rate trend chart, and —
when GATE_API_KEY / GATE_API_SECRET are set — reads the caller's lending
positions and accrued interest (all read-only; no funds are moved).

Run:

	GATE_API_KEY=... GATE_API_SECRET=... go run ./examples/earn

Without credentials it runs the public calls only.

NOTE: the earn section is calibration-pending (endpoint/field exactness is
modeled on Gate's Uni-lending docs); verify against a live environment.
*/

package main

import (
	"context"
	"log"
	"os"
	"time"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/earn"
	etypes "github.com/tonymontanov/go-gate/v2/earn/types"
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

	var e *earn.Client = client.Earn().(*earn.Client)

	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// --- public: list lendable currencies ---
	var currencies []etypes.UniCurrency
	currencies, err = e.ListCurrencies(ctx)
	if err != nil {
		log.Fatalf("list currencies: %v", err)
	}
	if len(currencies) == 0 {
		log.Fatalf("no Uni lending currencies available")
	}
	var currency string = currencies[0].Currency
	log.Printf("currency %s: minLend=%s maxLend=%s available=%s rate=[%s..%s]",
		currency, currencies[0].MinLendAmount, currencies[0].MaxLendAmount,
		currencies[0].Available, currencies[0].MinRate, currencies[0].MaxRate)

	// --- public: current estimated annualized rate ---
	var rate etypes.RatePoint
	rate, err = e.ListRate(ctx, currency)
	if err != nil {
		log.Printf("list rate: %v", err)
	} else {
		log.Printf("%s estimated annualized rate: %s", currency, rate.EstimateRate)
	}

	// --- public: rate trend chart (last 7 days) ---
	var now int64 = time.Now().Unix()
	var chart []etypes.ChartPoint
	chart, err = e.ListChart(ctx, currency, now-7*24*3600, now)
	if err != nil {
		log.Printf("list chart: %v", err)
	} else {
		log.Printf("%s rate chart: %d points", currency, len(chart))
	}

	// --- private: lending positions + accrued interest (with creds) ---
	if os.Getenv("GATE_API_KEY") != "" && os.Getenv("GATE_API_SECRET") != "" {
		var lends []etypes.UniLend
		lends, err = e.ListUserLends(ctx, "", 0, 0)
		if err != nil {
			log.Printf("list user lends: %v", err)
		} else {
			log.Printf("open lending positions: %d", len(lends))
			var i int
			for i = 0; i < len(lends); i++ {
				log.Printf("  %s: current=%s left=%s minRate=%s status=%s",
					lends[i].Currency, lends[i].CurrentAmount, lends[i].LeftAmount,
					lends[i].MinRate, lends[i].InterestStatus)
			}
		}

		var interest etypes.Interest
		interest, err = e.GetInterest(ctx, currency)
		if err != nil {
			log.Printf("get interest: %v", err)
		} else {
			log.Printf("%s total interest income: %s", interest.Currency, interest.Interest)
		}
	} else {
		log.Printf("no credentials set — skipping private lending reads")
	}
}
