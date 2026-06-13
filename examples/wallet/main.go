/*
FILE: examples/wallet/main.go

DESCRIPTION:
A minimal runnable example for the go-gate wallet section (cross-account
transfers, balances, fees). It exercises the public discovery endpoint
(currency chains) and — when GATE_API_KEY / GATE_API_SECRET are set — performs
read-only signed calls (total balance, trade fee, sub-account balances). It does
NOT perform any transfer: moving funds is a balance-changing action that should
be opt-in.

Run:

	GATE_API_KEY=... GATE_API_SECRET=... go run ./examples/wallet

Without credentials it runs the public call only.

NOTE: the wallet section is calibration-pending (endpoint/field exactness is
modeled on Gate's wallet docs); verify against a live environment.
*/

package main

import (
	"context"
	"log"
	"os"
	"time"

	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/wallet"
	wtypes "github.com/tonymontanov/go-gate/v2/wallet/types"
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

	var w *wallet.Client = client.Wallet().(*wallet.Client)

	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// --- public: deposit/withdraw chains for a currency ---
	var chains []wtypes.CurrencyChain
	chains, err = w.ListCurrencyChains(ctx, "USDT")
	if err != nil {
		log.Fatalf("list currency chains: %v", err)
	}
	log.Printf("USDT chains: %d", len(chains))
	var i int
	for i = 0; i < len(chains); i++ {
		log.Printf("  chain %s (deposit_disabled=%v withdraw_disabled=%v)",
			chains[i].Chain, chains[i].DepositDisabled, chains[i].WithdrawDisabled)
	}

	// --- private: read-only signed calls (with creds) ---
	if os.Getenv("GATE_API_KEY") != "" && os.Getenv("GATE_API_SECRET") != "" {
		var total wtypes.TotalBalance
		total, err = w.GetTotalBalance(ctx, "USDT")
		if err != nil {
			log.Printf("total balance: %v", err)
		} else {
			log.Printf("total balance: %s %s across %d locations",
				total.Total.Amount, total.Total.Currency, len(total.Details))
		}

		var fee wtypes.TradeFee
		fee, err = w.GetTradeFee(ctx, "", "")
		if err != nil {
			log.Printf("trade fee: %v", err)
		} else {
			log.Printf("trade fee: taker=%s maker=%s (gt_discount=%v)",
				fee.TakerFee, fee.MakerFee, fee.GTDiscount)
		}

		var balances []wtypes.SubAccountBalance
		balances, err = w.ListSubAccountBalances(ctx, "")
		if err != nil {
			log.Printf("sub-account balances: %v", err)
		} else {
			log.Printf("sub-account spot balances: %d sub-accounts", len(balances))
		}
	} else {
		log.Printf("no credentials set — skipping private read demo")
	}
}
