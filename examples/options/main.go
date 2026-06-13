/*
FILE: examples/options/main.go

DESCRIPTION:
A minimal runnable example for the go-gate options section (European-style crypto
options). Because option contracts EXPIRE and are struck, the example discovers a
live contract dynamically: it lists the underlyings, picks one, lists its nearest
expiration and contracts, prints a contract spec (strike/expiry/greeks), fetches
an order-book snapshot, subscribes to the per-contract ticker and the underlying
price, and — when GATE_API_KEY / GATE_API_SECRET are set — reads the options
account and places + cancels a far-from-market post-only order.

Run:

	GATE_API_KEY=... GATE_API_SECRET=... go run ./examples/options

Without credentials it runs the public calls only.

NOTE: the options section is calibration-pending (endpoint/WS field exactness is
modeled on Gate's options docs); verify against a live options environment
(WS host wss://op-ws.gateio.live/v4/ws).
*/

package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/shopspring/decimal"
	gate "github.com/tonymontanov/go-gate/v2"
	"github.com/tonymontanov/go-gate/v2/options"
	otypes "github.com/tonymontanov/go-gate/v2/options/types"
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

	var o *options.Client = client.Options().(*options.Client)

	var ctx context.Context
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// --- public: pick an underlying ---
	var underlyings []otypes.Underlying
	underlyings, err = o.MarketData().GetUnderlyings(ctx)
	if err != nil {
		log.Fatalf("get underlyings: %v", err)
	}
	if len(underlyings) == 0 {
		log.Fatalf("no options underlyings available")
	}
	var underlying string = underlyings[0].Name
	log.Printf("underlying %s: index=%s", underlying, underlyings[0].IndexPrice)

	// --- public: discover a live contract on the nearest expiration ---
	var contracts []otypes.SymbolInfo
	contracts, err = o.MarketData().GetContracts(ctx, underlying, 0)
	if err != nil {
		log.Fatalf("get contracts: %v", err)
	}
	if len(contracts) == 0 {
		log.Fatalf("no options contracts for %s", underlying)
	}
	var contract string = contracts[0].Contract
	log.Printf("%s: strike=%s call=%v expires=%s multiplier=%s tick=%s",
		contract, contracts[0].StrikePrice, contracts[0].IsCall,
		time.UnixMilli(contracts[0].ExpirationMs).UTC(), contracts[0].Multiplier, contracts[0].OrderPriceRound)

	// --- public: order-book snapshot ---
	var book otypes.OrderBook
	book, err = o.MarketData().GetOrderBook(ctx, contract, 5)
	if err != nil {
		log.Printf("get order book: %v", err)
	} else if len(book.Bids) > 0 && len(book.Asks) > 0 {
		log.Printf("top of book: %s / %s", book.Bids[0].Price, book.Asks[0].Price)
	}

	// --- public: per-contract ticker + underlying price streams ---
	err = o.Stream().WatchContractTickers(ctx, contract, func(tk otypes.Ticker) {
		log.Printf("TICKER %s: mark=%s iv=%s delta=%s bid=%s ask=%s",
			tk.Contract, tk.MarkPrice, tk.MarkIv, tk.Delta, tk.Bid1Price, tk.Ask1Price)
	}, func(e error) {
		log.Printf("ticker stream error: %v", e)
	})
	if err != nil {
		log.Printf("watch contract tickers: %v", err)
	}

	err = o.Stream().WatchUnderlyingPrice(ctx, underlying, func(up otypes.UnderlyingPrice) {
		log.Printf("ULPRICE %s: %s", up.Underlying, up.Price)
	}, func(e error) {
		log.Printf("ul_price stream error: %v", e)
	})
	if err != nil {
		log.Printf("watch underlying price: %v", err)
	}

	// --- private: account read + place/cancel a post-only order (with creds) ---
	if os.Getenv("GATE_API_KEY") != "" && os.Getenv("GATE_API_SECRET") != "" {
		var acc otypes.AccountInfo
		acc, err = o.Account().GetAccount(ctx)
		if err != nil {
			log.Printf("get account: %v", err)
		} else {
			log.Printf("account: equity=%s available=%s currency=%s", acc.Equity, acc.Available, acc.Currency)
		}

		var info otypes.OrderInfo
		info, err = o.Trading().CreateOrder(ctx, otypes.CreateOrderRequest{
			Contract:    contract,
			Side:        otypes.SideTypeBuy,
			Size:        decimal.NewFromInt(1),
			Price:       decimal.NewFromFloat(0.0001), // far from market, post-only
			TimeInForce: otypes.TimeInForcePOC,
			Text:        "gate-sdk-example",
		})
		if err != nil {
			log.Printf("create order: %v", err)
		} else {
			log.Printf("placed order id=%s text=%s", info.OrderID, info.ClientOrderID)
			if cerr := o.Trading().CancelOrder(ctx, otypes.CancelOrderRequest{
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

	// Let a few updates arrive.
	<-ctx.Done()
}
