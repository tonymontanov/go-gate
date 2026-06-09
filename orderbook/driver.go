/*
FILE: orderbook/driver.go

DESCRIPTION:
Driver orchestrates an Engine against Gate's REST-snapshot + WS-delta model so
the section StreamClients don't each re-implement the (subtle) priming and
resync dance. It is transport-agnostic: the caller supplies a snapshot function
(typically a REST GetOrderBook call) and an emit callback; the caller feeds raw
deltas via PushDelta from the WS read goroutine.

WHY A DRIVER:
Gate delivers the order-book snapshot over REST, not over the WS stream (unlike
OKX/Bybit). So when a `*.order_book_update` subscription starts, deltas begin
arriving immediately while the snapshot is still being fetched. Applying a delta
before the snapshot — or applying a stale snapshot against newer deltas — would
corrupt the book. The Driver:

  - never blocks the WS read goroutine: the REST snapshot fetch runs in a
    background goroutine (PushDelta returns immediately);
  - buffers deltas that arrive while priming, then drains them in order against
    the freshly applied snapshot (skipping stale ones, honoring the [U,u]
    alignment in Engine.ApplyDelta);
  - re-primes automatically on a detected sequence gap (Engine returns
    GapSequence) or after a WS reconnect (caller invokes Reset).

CONCURRENCY:
A single mutex guards the priming state machine. The Engine has its own lock, so
reads (TopLevels) stay safe throughout. `primed` flips true only once the buffer
is fully drained, guaranteeing live deltas are never applied out of order with
respect to buffered ones.
*/

package orderbook

import (
	"context"
	"sync"
)

// maxPendingDeltas caps the priming buffer so a stuck/slow snapshot fetch cannot
// grow it without bound. If exceeded, oldest buffered deltas are dropped; the
// alignment/gap logic in the drain still resyncs correctly afterwards.
const maxPendingDeltas = 4096

// Driver drives an Engine from a Gate order_book_update stream.
type Driver struct {
	eng      *Engine
	snapshot func(ctx context.Context) (Snapshot, error)
	onBook   func(*Engine)
	onErr    func(error)

	mu      sync.Mutex
	primed  bool
	priming bool
	pending []Delta
}

// NewDriver creates a Driver for eng. snapshot fetches a fresh REST snapshot;
// onBook is called (outside the lock) after every clean apply that leaves the
// engine non-dirty, so the caller can read TopLevels and emit; onErr (optional)
// receives snapshot-fetch errors. onBook and onErr may be nil.
func NewDriver(eng *Engine, snapshot func(ctx context.Context) (Snapshot, error), onBook func(*Engine), onErr func(error)) *Driver {
	return &Driver{
		eng:      eng,
		snapshot: snapshot,
		onBook:   onBook,
		onErr:    onErr,
	}
}

// Engine returns the underlying engine (for reads).
func (d *Driver) Engine() *Engine { return d.eng }

// PushDelta feeds one parsed delta. Safe to call from the WS read goroutine: it
// never blocks on the REST snapshot fetch. Before the engine is primed the delta
// is buffered and a background prime is kicked off once.
func (d *Driver) PushDelta(ctx context.Context, dl Delta) {
	d.mu.Lock()
	if !d.primed {
		d.bufferLocked(dl)
		if !d.priming {
			d.priming = true
			d.mu.Unlock()
			go d.prime(ctx)
			return
		}
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()

	var res ApplyResult = d.eng.ApplyDelta(dl)
	if res.Gap != GapNone {
		// Buffer the gap-triggering delta so it is replayed against the fresh
		// snapshot the resync fetches (it aligns if the snapshot's id falls
		// within [U,u], and is harmlessly skipped as stale otherwise).
		d.mu.Lock()
		d.bufferLocked(dl)
		d.mu.Unlock()
		d.triggerResync(ctx)
		return
	}
	if res.Stale {
		return
	}
	d.emit()
}

// Reset marks the engine stale and re-primes from a fresh snapshot. Called on WS
// reconnect (via Subscription.Reset): deltas missed while disconnected break
// continuity, so the local book must be rebuilt.
func (d *Driver) Reset(ctx context.Context) {
	d.triggerResync(ctx)
}

// bufferLocked appends a delta to the priming buffer (mu held), dropping the
// oldest if the cap is hit.
func (d *Driver) bufferLocked(dl Delta) {
	if len(d.pending) >= maxPendingDeltas {
		d.pending = d.pending[1:]
	}
	d.pending = append(d.pending, dl)
}

// triggerResync flags the engine for a rebuild and launches a prime if one is
// not already running.
func (d *Driver) triggerResync(ctx context.Context) {
	d.mu.Lock()
	d.eng.MarkStale()
	d.primed = false
	if d.priming {
		d.mu.Unlock()
		return
	}
	d.priming = true
	d.mu.Unlock()
	go d.prime(ctx)
}

// prime fetches a snapshot, applies it, drains buffered deltas, then flips primed
// once the buffer is empty. Runs in its own goroutine.
func (d *Driver) prime(ctx context.Context) {
	var snap Snapshot
	var err error
	snap, err = d.snapshot(ctx)
	if err != nil {
		if d.onErr != nil {
			d.onErr(err)
		}
		// Allow a later delta (or Reset) to retry priming.
		d.mu.Lock()
		d.priming = false
		d.mu.Unlock()
		return
	}

	d.eng.ApplySnapshot(snap)

	for {
		d.mu.Lock()
		if len(d.pending) == 0 {
			d.primed = true
			d.priming = false
			d.mu.Unlock()
			break
		}
		var batch []Delta = d.pending
		d.pending = nil
		d.mu.Unlock()

		var i int
		for i = 0; i < len(batch); i++ {
			var res ApplyResult = d.eng.ApplyDelta(batch[i])
			if res.Gap == GapSequence {
				// Snapshot is older than the buffered deltas (or a gap inside the
				// buffer): fetch a newer one and start over.
				d.mu.Lock()
				d.priming = false
				d.mu.Unlock()
				d.triggerResync(ctx)
				return
			}
		}
	}

	if !d.eng.IsDirty() {
		d.emit()
	}
}

// emit invokes onBook if set.
func (d *Driver) emit() {
	if d.onBook != nil {
		d.onBook(d.eng)
	}
}
