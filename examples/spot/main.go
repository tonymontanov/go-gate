/*
FILE: examples/spot/main.go

DESCRIPTION:
A minimal runnable example for the go-gate spot section. It fetches a currency
pair spec and order-book snapshot (public), subscribes to the best bid/ask
stream, and — when GATE_API_KEY / GATE_API_SECRET are set — places and cancels a
far-from-market post-only order to demonstrate the private flow.

Run:

	GATE_API_KEY=... GATE_API_SECRET=... go run ./examples/spot

Without credentials it runs the public calls only. Gate has no spot testnet, so
the private demo targets production — use a throwaway key with tiny size, or omit
credentials to run public-only.
*/

package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/shopspring/decimal"
	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/spot"
	stypes "github.com/tonymontanov/go-gate/v2/spot/types"
)

func main() {
	var pair string = "BTC_USDT"

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

	var sp *spot.Client = client.Spot().(*spot.Client)

	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// --- public: currency pair spec + order-book snapshot ---
	var info stypes.SymbolInfo
	info, err = sp.MarketData().GetCurrencyPair(ctx, pair)
	if err != nil {
		log.Fatalf("get currency pair: %v", err)
	}
	log.Printf("%s: amount_precision=%d price_precision=%d min_base=%s min_quote=%s",
		info.CurrencyPair, info.AmountPrecision, info.PricePrecision, info.MinBaseAmount, info.MinQuoteAmount)

	var book stypes.OrderBook
	book, err = sp.MarketData().GetOrderBook(ctx, pair, 5)
	if err != nil {
		log.Fatalf("get order book: %v", err)
	}
	if len(book.Bids) > 0 && len(book.Asks) > 0 {
		log.Printf("top of book: %s / %s", book.Bids[0].Price, book.Asks[0].Price)
	}

	// --- public: best bid/ask stream ---
	err = sp.Stream().WatchBookTicker(ctx, pair, func(bt stypes.BookTicker) {
		log.Printf("BBO %s: %s x %s | %s x %s", bt.CurrencyPair, bt.BidPrice, bt.BidSize, bt.AskPrice, bt.AskSize)
	}, func(e error) {
		log.Printf("stream error: %v", e)
	})
	if err != nil {
		log.Printf("watch book ticker: %v", err)
	}

	// --- private: place + cancel a post-only order (only with credentials) ---
	if os.Getenv("GATE_API_KEY") != "" && os.Getenv("GATE_API_SECRET") != "" {
		var ord stypes.OrderInfo
		ord, err = sp.Trading().CreateOrder(ctx, stypes.CreateOrderRequest{
			CurrencyPair: pair,
			Side:         stypes.SideTypeBuy,
			Amount:       decimal.RequireFromString("0.01"),
			Price:        decimal.NewFromInt(1000), // far from market, post-only
			TimeInForce:  stypes.TimeInForcePOC,
			Text:         "gate-sdk-example",
		})
		if err != nil {
			log.Printf("create order: %v", err)
		} else {
			log.Printf("placed order id=%s text=%s", ord.OrderID, ord.ClientOrderID)
			if cerr := sp.Trading().CancelOrder(ctx, stypes.CancelOrderRequest{
				CurrencyPair: pair, OrderID: ord.OrderID,
			}); cerr != nil {
				log.Printf("cancel order: %v", cerr)
			} else {
				log.Printf("cancelled order id=%s", ord.OrderID)
			}
		}
	} else {
		log.Printf("no credentials set — skipping private order demo")
	}

	// Let a few BBO updates arrive.
	<-ctx.Done()
}
