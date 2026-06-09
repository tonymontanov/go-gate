/*
FILE: orderbook/engine.go

DESCRIPTION:
Shared local L2 order-book engine for the Gate SDK. Package overview and the
exact Gate sequence rules live in doc.go.

ENTITIES:
  - Engine      — one instance per symbol (contract / currency pair).
  - Snapshot    — full book state from a REST GetOrderBook(with_id) call.
  - Delta       — one incremental `order_book_update` event (range [U,u]).
  - ApplyResult — outcome of an apply call: Gap classifier + post-apply metrics
                  so callers can log/observe without re-locking.
  - GapKind     — gap categories (None / Initial / Sequence).

ALGORITHM (see doc.go for the wire rationale):
  1. ApplySnapshot drops local state, copies+sorts bids/asks, stamps lastU=ID,
     marks the engine primed and clean. Always Gap=GapNone.
  2. ApplyDelta:
       a. Not primed yet → Gap=GapInitial, dirty=true, state unchanged.
       b. Stale (d.LastU <= lastU) → Gap=GapNone, state unchanged (already seen).
       c. Missed events (d.FirstU > lastU+1) → Gap=GapSequence, dirty=true,
          state unchanged.
       d. Otherwise apply: size-0 levels are removed, others inserted/updated;
          entries past maxDepth are dropped; lastU = d.LastU.

NUMERICS:
All prices and sizes are shopspring/decimal.Decimal; the engine never converts
to float64.
*/

package orderbook

import (
	"sort"
	"sync"

	"github.com/shopspring/decimal"
)

// Level — one order-book level inside the engine. Section packages (futures,
// spot) own their own public OrderBookLevel struct and convert at the package
// boundary; keeping the engine's level type independent of any section package
// avoids the import cycle that would otherwise prevent both sections from
// sharing the engine.
type Level struct {
	Price decimal.Decimal
	Size  decimal.Decimal
}

// SideKind identifies the order-book side in internal helpers.
type SideKind uint8

const (
	// SideBid — bids (sorted descending by price).
	SideBid SideKind = iota
	// SideAsk — asks (sorted ascending by price).
	SideAsk
)

// GapKind categorises gap detections returned by ApplyDelta.
type GapKind uint8

const (
	// GapNone — the delta was applied cleanly (or was a harmless stale repeat).
	GapNone GapKind = iota
	// GapInitial — engine has not received a snapshot yet; the delta was dropped.
	GapInitial
	// GapSequence — the delta's first id is beyond lastU+1: events were missed.
	GapSequence
)

// String returns a short human-readable label for logs/metrics.
func (g GapKind) String() string {
	switch g {
	case GapInitial:
		return "initial"
	case GapSequence:
		return "sequence"
	default:
		return "none"
	}
}

// Snapshot — a full order-book state. Created by the dispatcher from a REST
// GetOrderBook(with_id) call. ID is the Gate order-book version (the snapshot's
// `id`); the engine treats it as the last applied update id.
type Snapshot struct {
	Symbol string
	Bids   []Level
	Asks   []Level
	ID     int64
	TsMs   int64
}

// Delta — one incremental `order_book_update` event. FirstU / LastU are Gate's
// `U` / `u` (first / last update id covered by this event).
type Delta struct {
	Symbol string
	Bids   []Level
	Asks   []Level
	FirstU int64
	LastU  int64
	TsMs   int64
}

// ApplyResult describes the outcome of applying a snapshot/delta. LastU is the
// engine's last applied update id after the call.
type ApplyResult struct {
	Gap     GapKind
	LastU   int64
	TsMs    int64
	BidsLen int
	AsksLen int
	// Stale is true when ApplyDelta skipped an already-seen event (Gap=GapNone
	// but nothing changed) — lets callers avoid re-publishing an unchanged book.
	Stale bool
}

// Engine — local order-book engine for one symbol.
type Engine struct {
	symbol   string
	maxDepth int

	mu       sync.RWMutex
	bids     []Level
	asks     []Level
	lastU    int64
	lastTsMs int64
	primed   bool
	dirty    bool
}

// NewEngine creates an empty engine. maxDepth caps the local book depth
// (default 400 when maxDepth <= 0).
func NewEngine(symbol string, maxDepth int) *Engine {
	if maxDepth <= 0 {
		maxDepth = 400
	}
	return &Engine{
		symbol:   symbol,
		maxDepth: maxDepth,
		bids:     make([]Level, 0, maxDepth),
		asks:     make([]Level, 0, maxDepth),
	}
}

// Symbol returns the symbol associated with the engine.
func (e *Engine) Symbol() string { return e.symbol }

// IsDirty reports whether the engine needs a resync (after Gap* or MarkStale).
func (e *Engine) IsDirty() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.dirty
}

// IsPrimed reports whether a snapshot has been applied since the last reset.
func (e *Engine) IsPrimed() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.primed
}

// LastUpdateID returns the last applied `u` value (0 before the first snapshot).
func (e *Engine) LastUpdateID() int64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastU
}

// ApplySnapshot replaces the local state with the snapshot. Always returns
// Gap=GapNone — a snapshot is the canonical way to clear a dirty engine.
func (e *Engine) ApplySnapshot(s Snapshot) ApplyResult {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.bids = copyLevelsSortedDesc(s.Bids, e.maxDepth)
	e.asks = copyLevelsSortedAsc(s.Asks, e.maxDepth)
	e.lastU = s.ID
	e.lastTsMs = s.TsMs
	e.primed = true
	e.dirty = false

	return ApplyResult{
		Gap:     GapNone,
		LastU:   s.ID,
		TsMs:    s.TsMs,
		BidsLen: len(e.bids),
		AsksLen: len(e.asks),
	}
}

// ApplyDelta applies one incremental update. See doc.go for the exact
// gap-detection rules.
func (e *Engine) ApplyDelta(d Delta) ApplyResult {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.primed {
		e.dirty = true
		return ApplyResult{
			Gap:     GapInitial,
			LastU:   e.lastU,
			TsMs:    e.lastTsMs,
			BidsLen: len(e.bids),
			AsksLen: len(e.asks),
		}
	}
	// Stale: the whole event is at or below what we've already applied.
	if d.LastU != 0 && d.LastU <= e.lastU {
		return ApplyResult{
			Gap:     GapNone,
			LastU:   e.lastU,
			TsMs:    e.lastTsMs,
			BidsLen: len(e.bids),
			AsksLen: len(e.asks),
			Stale:   true,
		}
	}
	// Gap: the event starts beyond the next expected id — events were missed.
	if d.FirstU > e.lastU+1 {
		e.dirty = true
		return ApplyResult{
			Gap:     GapSequence,
			LastU:   e.lastU,
			TsMs:    e.lastTsMs,
			BidsLen: len(e.bids),
			AsksLen: len(e.asks),
		}
	}

	var i int
	for i = 0; i < len(d.Bids); i++ {
		e.applyLevelLocked(SideBid, d.Bids[i])
	}
	for i = 0; i < len(d.Asks); i++ {
		e.applyLevelLocked(SideAsk, d.Asks[i])
	}
	e.trimLocked()
	e.lastU = d.LastU
	e.lastTsMs = d.TsMs

	return ApplyResult{
		Gap:     GapNone,
		LastU:   d.LastU,
		TsMs:    d.TsMs,
		BidsLen: len(e.bids),
		AsksLen: len(e.asks),
	}
}

// MarkStale flags the engine as needing a resync without touching the stored
// levels. Called on WS reconnect: the next applicable thing must be a fresh
// snapshot (ApplySnapshot), since deltas missed while disconnected break
// continuity.
func (e *Engine) MarkStale() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.primed = false
	e.dirty = true
}

// MarkResynced clears the dirty flag and stamps the engine with a new (u, ts).
// Prefer ApplySnapshot when you have the full state in hand; MarkResynced exists
// for callers that re-seed the levels separately.
func (e *Engine) MarkResynced(updateID, tsMs int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastU = updateID
	e.lastTsMs = tsMs
	e.primed = true
	e.dirty = false
}

// TopLevels returns a copy of the top-n bid/ask levels. n <= 0 returns the full
// local state.
func (e *Engine) TopLevels(n int) (bids, asks []Level) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var nb int = len(e.bids)
	var na int = len(e.asks)
	if n > 0 {
		if n < nb {
			nb = n
		}
		if n < na {
			na = n
		}
	}
	bids = make([]Level, nb)
	asks = make([]Level, na)
	copy(bids, e.bids[:nb])
	copy(asks, e.asks[:na])
	return bids, asks
}

// BestBidAsk returns the best (top-of-book) bid/ask price + size pairs. All four
// values are decimal.Zero when the corresponding side is empty.
func (e *Engine) BestBidAsk() (bidPx, bidSz, askPx, askSz decimal.Decimal) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if len(e.bids) > 0 {
		bidPx = e.bids[0].Price
		bidSz = e.bids[0].Size
	}
	if len(e.asks) > 0 {
		askPx = e.asks[0].Price
		askSz = e.asks[0].Size
	}
	return bidPx, bidSz, askPx, askSz
}

// applyLevelLocked applies one level. Size 0 → remove. e.mu must be held.
func (e *Engine) applyLevelLocked(side SideKind, lvl Level) {
	var slice *[]Level
	var less func(a, b decimal.Decimal) bool
	if side == SideBid {
		slice = &e.bids
		less = func(a, b decimal.Decimal) bool { return a.GreaterThan(b) }
	} else {
		slice = &e.asks
		less = func(a, b decimal.Decimal) bool { return a.LessThan(b) }
	}
	var arr []Level = *slice
	var idx int = sort.Search(len(arr), func(i int) bool {
		return !less(arr[i].Price, lvl.Price)
	})
	if idx < len(arr) && arr[idx].Price.Equal(lvl.Price) {
		if lvl.Size.IsZero() {
			arr = append(arr[:idx], arr[idx+1:]...)
		} else {
			arr[idx].Size = lvl.Size
		}
	} else if !lvl.Size.IsZero() {
		arr = append(arr, Level{})
		copy(arr[idx+1:], arr[idx:])
		arr[idx] = lvl
	}
	*slice = arr
}

// trimLocked clamps both sides to maxDepth.
func (e *Engine) trimLocked() {
	if len(e.bids) > e.maxDepth {
		e.bids = e.bids[:e.maxDepth]
	}
	if len(e.asks) > e.maxDepth {
		e.asks = e.asks[:e.maxDepth]
	}
}

// copyLevelsSortedDesc copies non-zero levels and sorts by price descending
// (bids). Truncates to max levels.
func copyLevelsSortedDesc(src []Level, max int) []Level {
	var out []Level = make([]Level, 0, len(src))
	var i int
	for i = 0; i < len(src); i++ {
		if src[i].Size.IsZero() {
			continue
		}
		out = append(out, src[i])
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Price.GreaterThan(out[j].Price)
	})
	if len(out) > max {
		out = out[:max]
	}
	return out
}

// copyLevelsSortedAsc copies non-zero levels and sorts by price ascending
// (asks). Truncates to max levels.
func copyLevelsSortedAsc(src []Level, max int) []Level {
	var out []Level = make([]Level, 0, len(src))
	var i int
	for i = 0; i < len(src); i++ {
		if src[i].Size.IsZero() {
			continue
		}
		out = append(out, src[i])
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Price.LessThan(out[j].Price)
	})
	if len(out) > max {
		out = out[:max]
	}
	return out
}
