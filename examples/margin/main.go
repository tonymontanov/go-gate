/*
FILE: examples/margin/main.go

DESCRIPTION:
A minimal runnable example for the go-gate margin section (isolated + cross
margin lending/borrowing). It exercises the public discovery endpoints
(isolated currency pairs + funding book, cross currencies) and — when
GATE_API_KEY / GATE_API_SECRET are set — performs read-only signed calls
(isolated accounts, cross account, borrowable). It does NOT create or repay any
loan: borrowing is a balance-moving action that should be opt-in.

Run:

	GATE_API_KEY=... GATE_API_SECRET=... go run ./examples/margin

Without credentials it runs the public calls only.

NOTE: the margin section is calibration-pending (endpoint/field exactness is
modeled on Gate's margin docs); verify against a live environment.
*/

package main

import (
	"context"
	"log"
	"os"
	"time"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/margin"
	mtypes "github.com/tonymontanov/go-gate/v2/margin/types"
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

	var m *margin.Client = client.Margin().(*margin.Client)

	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// --- public: isolated-margin currency pairs ---
	var pairs []mtypes.MarginCurrencyPair
	pairs, err = m.Isolated().ListCurrencyPairs(ctx)
	if err != nil {
		log.Fatalf("list currency pairs: %v", err)
	}
	if len(pairs) == 0 {
		log.Fatalf("no margin currency pairs available")
	}
	var pair string = pairs[0].ID
	log.Printf("isolated pairs: %d (first %s, leverage=%s)", len(pairs), pair, pairs[0].Leverage)

	// --- public: isolated funding (lending) book for the quote currency ---
	var book mtypes.FundingBook
	book, err = m.Isolated().GetFundingBook(ctx, pairs[0].Quote)
	if err != nil {
		log.Printf("funding book: %v", err)
	} else {
		log.Printf("funding book %s: %d lend / %d borrow levels", pairs[0].Quote, len(book.Asks), len(book.Bids))
	}

	// --- public: cross-margin currencies ---
	var currencies []mtypes.CrossCurrency
	currencies, err = m.Cross().ListCurrencies(ctx)
	if err != nil {
		log.Printf("cross currencies: %v", err)
	} else {
		log.Printf("cross currencies: %d", len(currencies))
	}

	// --- private: read-only signed calls (with creds) ---
	if os.Getenv("GATE_API_KEY") != "" && os.Getenv("GATE_API_SECRET") != "" {
		var accounts []mtypes.MarginAccount
		accounts, err = m.Isolated().ListAccounts(ctx, pair)
		if err != nil {
			log.Printf("isolated accounts: %v", err)
		} else {
			log.Printf("isolated accounts for %s: %d", pair, len(accounts))
		}

		var cross mtypes.CrossAccount
		cross, err = m.Cross().GetAccount(ctx)
		if err != nil {
			log.Printf("cross account: %v", err)
		} else {
			log.Printf("cross account: user=%d total=%s risk=%s currencies=%d",
				cross.UserID, cross.Total, cross.Risk, len(cross.Balances))
		}

		var borrowable mtypes.Borrowable
		borrowable, err = m.Cross().GetBorrowable(ctx, currencyOrDefault(currencies))
		if err != nil {
			log.Printf("cross borrowable: %v", err)
		} else {
			log.Printf("cross borrowable %s: %s", borrowable.Currency, borrowable.Amount)
		}
	} else {
		log.Printf("no credentials set — skipping private read demo")
	}
}

// currencyOrDefault returns a sensible currency to query, preferring the first
// listed cross-margin currency and falling back to "USDT".
func currencyOrDefault(currencies []mtypes.CrossCurrency) string {
	if len(currencies) > 0 && currencies[0].Name != "" {
		return currencies[0].Name
	}
	return "USDT"
}
