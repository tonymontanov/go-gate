/*
Package gate is a high-performance Go SDK for the Gate exchange (REST + WebSocket),
built for HFT / algorithmic trading.

It is a single-exchange SDK: cross-exchange unification is expected to live in the
caller (the trading desk), not here. The public surface mirrors the sibling SDKs
go-okx and go-bybit so a desk connector can wrap it with minimal glue.

# Layout

The root package (gate) owns shared infrastructure — Config, the request Signer,
the REST transport, and the Logger — and exposes trading sections lazily:

	import (
		gate "github.com/tonymontanov/go-gate/v2"
		_ "github.com/tonymontanov/go-gate/v2/futures" // register the futures section
	)

	client, err := gate.NewClient(gate.Config{APIKey: k, SecretKey: s})
	futuresClient := client.Futures().(*futures.Client)

Each trading section is a separate package named after Gate's own terminology
(futures for USD-M perpetuals; spot, delivery, options in later iterations). A
section registers a factory with the root via init(), which keeps the root free
of any dependency on the sections and avoids an import cycle.

# Scope

v1.0 covers USD-M perpetual Futures (settle=usdt): REST trading, account/position,
market data, and WebSocket market-data + user-data streams. See docs/ for the
full specification and milestone breakdown.
*/
package gate
