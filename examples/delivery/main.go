/*
FILE: examples/delivery/main.go

DESCRIPTION:
A minimal runnable example for the go-gate delivery section (dated/quarterly
USD-M futures). Because delivery contracts EXPIRE, the example discovers a live
contract via GetContracts (instead of hard-coding a dated name), prints its
spec + expiry, fetches an order-book snapshot, subscribes to the best bid/ask
and the maintained L2 book, and — when GATE_API_KEY / GATE_API_SECRET are set —
places and cancels a far-from-market post-only order.

Run:

	GATE_API_KEY=... GATE_API_SECRET=... go run ./examples/delivery

Without credentials it runs the public calls only.

NOTE: the delivery section is calibration-pending (endpoint/WS field exactness is
modeled on the futures section + Gate's delivery docs); verify against a live
delivery environment.
*/

package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/shopspring/decimal"
	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/delivery"
	dtypes "github.com/tonymontanov/go-gate/v2/delivery/types"
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

	var d *delivery.Client = client.Delivery().(*delivery.Client)

	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// --- public: discover a live (dated) delivery contract ---
	var contracts []dtypes.SymbolInfo
	contracts, err = d.MarketData().GetContracts(ctx)
	if err != nil {
		log.Fatalf("get contracts: %v", err)
	}
	if len(contracts) == 0 {
		log.Fatalf("no delivery contracts available")
	}
	var contract string = contracts[0].Contract
	log.Printf("%s: quanto_multiplier=%s tick=%s expires=%s cycle=%s",
		contract, contracts[0].QuantoMultiplier, contracts[0].OrderPriceRound,
		time.UnixMilli(contracts[0].ExpireTimeMs).UTC(), contracts[0].Cycle)

	// --- public: order-book snapshot ---
	var book dtypes.OrderBook
	book, err = d.MarketData().GetOrderBook(ctx, contract, 5)
	if err != nil {
		log.Fatalf("get order book: %v", err)
	}
	if len(book.Bids) > 0 && len(book.Asks) > 0 {
		log.Printf("top of book: %s / %s", book.Bids[0].Price, book.Asks[0].Price)
	}

	// --- public: best bid/ask + maintained L2 book streams ---
	err = d.Stream().WatchBookTicker(ctx, contract, func(bt dtypes.BookTicker) {
		log.Printf("BBO %s: %s x %s | %s x %s", bt.Contract, bt.BidPrice, bt.BidSize, bt.AskPrice, bt.AskSize)
	}, func(e error) {
		log.Printf("stream error: %v", e)
	})
	if err != nil {
		log.Printf("watch book ticker: %v", err)
	}

	err = d.Stream().WatchOrderBook(ctx, contract, "100ms", 20, func(ob dtypes.OrderBook) {
		if len(ob.Bids) > 0 && len(ob.Asks) > 0 {
			log.Printf("BOOK %s (id=%d): bid %s x %s | ask %s x %s",
				contract, ob.ID, ob.Bids[0].Price, ob.Bids[0].Size, ob.Asks[0].Price, ob.Asks[0].Size)
		}
	}, func(e error) {
		log.Printf("order book error: %v", e)
	})
	if err != nil {
		log.Printf("watch order book: %v", err)
	}

	// --- private: place + cancel a post-only order (only with credentials) ---
	if os.Getenv("GATE_API_KEY") != "" && os.Getenv("GATE_API_SECRET") != "" {
		var info dtypes.OrderInfo
		info, err = d.Trading().CreateOrder(ctx, dtypes.CreateOrderRequest{
			Contract:    contract,
			Side:        dtypes.SideTypeBuy,
			Size:        decimal.NewFromInt(1),
			Price:       decimal.NewFromInt(1000), // far from market, post-only
			TimeInForce: dtypes.TimeInForcePOC,
			Text:        "gate-sdk-example",
		})
		if err != nil {
			log.Printf("create order: %v", err)
		} else {
			log.Printf("placed order id=%s text=%s", info.OrderID, info.ClientOrderID)
			if cerr := d.Trading().CancelOrder(ctx, dtypes.CancelOrderRequest{
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
