/*
FILE: orderbook/doc.go

DESCRIPTION:
Package orderbook implements a shared local L2 order-book engine for the Gate
SDK, used by both the futures and spot sections. It maintains a sorted local
book from a REST snapshot plus the incremental `*.order_book_update` WebSocket
deltas, detecting sequence gaps so the caller can resync.

GATE PROTOCOL (differs from OKX/Bybit):

  - The authoritative SNAPSHOT comes from REST `GetOrderBook(with_id=true)`,
    NOT over the WebSocket. It carries an `id` — the order-book version.

  - Each `order_book_update` delta carries a RANGE of update ids: `U` (first id
    in the event) and `u` (last id in the event), plus the changed bid/ask
    levels. A level with size 0 means "remove this price".

  - Continuity: after a snapshot of version S, the FIRST applicable delta is the
    one with `U <= S+1 <= u`; every subsequent delta must be contiguous, i.e.
    its `U` must equal `lastU+1`. A delta whose whole range is at or below the
    last applied id (`u <= lastU`) is stale and skipped. A delta that starts
    beyond the next expected id (`U > lastU+1`) means events were missed → gap.

    Unlike OKX, Gate publishes NO CRC32 order-book checksum, so the only gap
    signal is the `U`/`u` sequence (same situation as Bybit). On a gap the engine
    is marked dirty and stops applying deltas until ApplySnapshot / MarkResynced
    re-seeds it (the StreamClient wires this to a fresh REST snapshot fetch).

ENTRY POINTS:
  - NewEngine(symbol, maxDepth) *Engine
  - (*Engine).ApplySnapshot(Snapshot) ApplyResult — re-seeds from a REST book.
  - (*Engine).ApplyDelta(Delta) ApplyResult       — applies one WS update.
  - (*Engine).TopLevels(n)   ([]Level, []Level)
  - (*Engine).BestBidAsk()   (px,sz,px,sz)
  - (*Engine).LastUpdateID() int64
  - (*Engine).IsDirty()      bool
  - (*Engine).MarkStale()    — flags a needed resync (e.g. on WS reconnect).
  - (*Engine).MarkResynced(u, ts) — clears dirty after a manual resync.

THREAD SAFETY:
Safe for concurrent ApplyDelta + readers. Readers take a read lock and copy out
the requested slice; the writer holds a single write lock over the whole apply.

PERFORMANCE:
Levels are stored in two sorted slices with O(log n) binary-search insertion /
deletion. The engine stores shopspring/decimal.Decimal and never converts to
float64; section packages convert at their boundary.
*/
package orderbook
