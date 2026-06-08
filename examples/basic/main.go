/*
FILE: examples/basic/main.go

DESCRIPTION:
A minimal runnable example for the go-gate futures section. It fetches a contract
spec and order-book snapshot (public), subscribes to the best bid/ask stream, and
— when GATE_API_KEY / GATE_API_SECRET are set — places and cancels a far-from-market
post-only order to demonstrate the private flow.

Run:

	GATE_API_KEY=... GATE_API_SECRET=... go run ./examples/basic

Without credentials it runs the public calls only. Set GATE_TESTNET=1 to target
the Gate futures testnet.
*/

package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/shopspring/decimal"
	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/futures"
	ftypes "github.com/tonymontanov/go-gate/v2/futures/types"
)

func main() {
	var contract string = "BTC_USDT"

	var client *gate.Client
	var err error
	client, err = gate.NewClient(gate.Config{
		APIKey:    os.Getenv("GATE_API_KEY"),
		SecretKey: os.Getenv("GATE_API_SECRET"),
		Testnet:   os.Getenv("GATE_TESTNET") == "1",
	})
	if err != nil {
		log.Fatalf("new client: %v", err)
	}
	defer func() { _ = client.Close() }()

	var fut *futures.Client = client.Futures().(*futures.Client)

	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// --- public: contract spec + order-book snapshot ---
	var spec ftypes.SymbolInfo
	spec, err = fut.MarketData().GetContract(ctx, contract)
	if err != nil {
		log.Fatalf("get contract: %v", err)
	}
	log.Printf("%s: quanto_multiplier=%s tick=%s order_size_min=%s",
		spec.Contract, spec.QuantoMultiplier, spec.OrderPriceRound, spec.OrderSizeMin)

	var book ftypes.OrderBook
	book, err = fut.MarketData().GetOrderBook(ctx, contract, 5)
	if err != nil {
		log.Fatalf("get order book: %v", err)
	}
	if len(book.Bids) > 0 && len(book.Asks) > 0 {
		log.Printf("top of book: %s / %s", book.Bids[0].Price, book.Asks[0].Price)
	}

	// --- public: best bid/ask stream ---
	err = fut.Stream().WatchBookTicker(ctx, contract, func(bt ftypes.BookTicker) {
		log.Printf("BBO %s: %s x %s | %s x %s", bt.Contract, bt.BidPrice, bt.BidSize, bt.AskPrice, bt.AskSize)
	}, func(e error) {
		log.Printf("stream error: %v", e)
	})
	if err != nil {
		log.Printf("watch book ticker: %v", err)
	}

	// --- private: place + cancel a post-only order (only with credentials) ---
	if os.Getenv("GATE_API_KEY") != "" && os.Getenv("GATE_API_SECRET") != "" {
		var info ftypes.OrderInfo
		info, err = fut.Trading().CreateOrder(ctx, ftypes.CreateOrderRequest{
			Contract:    contract,
			Side:        ftypes.SideTypeBuy,
			Size:        decimal.NewFromInt(1),
			Price:       decimal.NewFromInt(1000), // far from market, post-only
			TimeInForce: ftypes.TimeInForcePOC,
			Text:        "gate-sdk-example",
		})
		if err != nil {
			log.Printf("create order: %v", err)
		} else {
			log.Printf("placed order id=%s text=%s", info.OrderID, info.ClientOrderID)
			if cerr := fut.Trading().CancelOrder(ctx, ftypes.CancelOrderRequest{
				Contract: contract, OrderID: info.OrderID,
			}); cerr != nil {
				log.Printf("cancel order: %v", cerr)
			} else {
				log.Printf("cancelled order id=%s", info.OrderID)
			}
		}
	} else {
		log.Printf("no credentials set — skipping private order demo")
	}

	// Let a few BBO updates arrive.
	<-ctx.Done()
}
