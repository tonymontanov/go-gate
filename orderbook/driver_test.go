/*
FILE: orderbook/driver_test.go

DESCRIPTION:
Offline tests for the Driver priming/buffering/resync orchestration, using a fake
in-memory snapshotter (no transport). Emits are observed through a channel so the
background prime goroutine can be awaited deterministically.
*/

package orderbook

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// obsDriver wires a Driver to a fake snapshotter and an emit channel.
type obsDriver struct {
	drv       *Driver
	snapCalls int64
	emits     chan int64 // sends LastUpdateID on each emit

	mu   sync.Mutex
	snap Snapshot
	err  error
}

func newObsDriver(symbol string) *obsDriver {
	var o *obsDriver = &obsDriver{emits: make(chan int64, 64)}
	var eng *Engine = NewEngine(symbol, 100)
	o.drv = NewDriver(eng,
		func(ctx context.Context) (Snapshot, error) {
			atomic.AddInt64(&o.snapCalls, 1)
			o.mu.Lock()
			defer o.mu.Unlock()
			return o.snap, o.err
		},
		func(e *Engine) { o.emits <- e.LastUpdateID() },
		nil,
	)
	return o
}

func (o *obsDriver) setSnapshot(s Snapshot) {
	o.mu.Lock()
	o.snap = s
	o.err = nil
	o.mu.Unlock()
}

func (o *obsDriver) waitEmit(t *testing.T) int64 {
	t.Helper()
	select {
	case v := <-o.emits:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for book emit")
		return 0
	}
}

func obLvl(price, size string) Level {
	return Level{Price: decimal.RequireFromString(price), Size: decimal.RequireFromString(size)}
}

func TestDriver_PrimesAndDrainsBufferedDeltas(t *testing.T) {
	var o *obsDriver = newObsDriver("BTC_USDT")
	o.setSnapshot(Snapshot{
		Bids: []Level{obLvl("100", "1")},
		Asks: []Level{obLvl("101", "1")},
		ID:   1000,
	})
	var ctx context.Context = context.Background()

	// Deltas arrive before the snapshot is applied → buffered, prime kicked off.
	o.drv.PushDelta(ctx, Delta{FirstU: 1001, LastU: 1001, Bids: []Level{obLvl("100", "5")}})
	o.drv.PushDelta(ctx, Delta{FirstU: 1002, LastU: 1002, Asks: []Level{obLvl("101", "9")}})

	var lastU int64 = o.waitEmit(t)
	if lastU != 1002 {
		t.Errorf("after drain lastU = %d, want 1002", lastU)
	}
	if atomic.LoadInt64(&o.snapCalls) != 1 {
		t.Errorf("snapshot fetched %d times, want 1", o.snapCalls)
	}
	var bids, asks = o.drv.Engine().TopLevels(0)
	if bids[0].Size.String() != "5" {
		t.Errorf("buffered bid delta not applied: %v", bids[0])
	}
	if asks[0].Size.String() != "9" {
		t.Errorf("buffered ask delta not applied: %v", asks[0])
	}
}

func TestDriver_LiveDeltaAfterPrime(t *testing.T) {
	var o *obsDriver = newObsDriver("BTC_USDT")
	o.setSnapshot(Snapshot{Bids: []Level{obLvl("100", "1")}, Asks: []Level{obLvl("101", "1")}, ID: 2000})
	var ctx context.Context = context.Background()

	// Prime via a first (aligned) delta.
	o.drv.PushDelta(ctx, Delta{FirstU: 2001, LastU: 2001, Bids: []Level{obLvl("100", "2")}})
	o.waitEmit(t)

	// A live, contiguous delta should now emit directly (no extra snapshot).
	o.drv.PushDelta(ctx, Delta{FirstU: 2002, LastU: 2002, Bids: []Level{obLvl("99", "3")}})
	var lastU int64 = o.waitEmit(t)
	if lastU != 2002 {
		t.Errorf("live delta lastU = %d, want 2002", lastU)
	}
	if atomic.LoadInt64(&o.snapCalls) != 1 {
		t.Errorf("snapshot fetched %d times, want 1 (no resync expected)", o.snapCalls)
	}
}

func TestDriver_GapTriggersResync(t *testing.T) {
	var o *obsDriver = newObsDriver("BTC_USDT")
	o.setSnapshot(Snapshot{Bids: []Level{obLvl("100", "1")}, Asks: []Level{obLvl("101", "1")}, ID: 3000})
	var ctx context.Context = context.Background()

	o.drv.PushDelta(ctx, Delta{FirstU: 3001, LastU: 3001, Bids: []Level{obLvl("100", "2")}})
	o.waitEmit(t)
	if atomic.LoadInt64(&o.snapCalls) != 1 {
		t.Fatalf("precondition: snapCalls=%d", o.snapCalls)
	}

	// Bump the snapshot the resync will fetch so the post-resync book is fresh.
	o.setSnapshot(Snapshot{Bids: []Level{obLvl("100", "7")}, Asks: []Level{obLvl("101", "1")}, ID: 4000})

	// A gap (FirstU far beyond lastU+1) must trigger a fresh snapshot fetch.
	o.drv.PushDelta(ctx, Delta{FirstU: 3999, LastU: 4001, Bids: []Level{obLvl("100", "8")}})
	var lastU int64 = o.waitEmit(t) // emit from the resync prime
	if atomic.LoadInt64(&o.snapCalls) != 2 {
		t.Errorf("expected a resync snapshot fetch, snapCalls=%d", o.snapCalls)
	}
	// The buffered gap delta [3999,4001] aligns against snapshot id 4000 and applies.
	if lastU != 4001 {
		t.Errorf("post-resync lastU = %d, want 4001", lastU)
	}
	var bids, _ = o.drv.Engine().TopLevels(1)
	if bids[0].Size.String() != "8" {
		t.Errorf("post-resync book wrong: top bid %v", bids[0])
	}
}

func TestDriver_ResetRePrimes(t *testing.T) {
	var o *obsDriver = newObsDriver("BTC_USDT")
	o.setSnapshot(Snapshot{Bids: []Level{obLvl("100", "1")}, Asks: []Level{obLvl("101", "1")}, ID: 5000})
	var ctx context.Context = context.Background()

	o.drv.PushDelta(ctx, Delta{FirstU: 5001, LastU: 5001})
	o.waitEmit(t)

	// Simulate reconnect.
	o.setSnapshot(Snapshot{Bids: []Level{obLvl("100", "1")}, Asks: []Level{obLvl("101", "1")}, ID: 6000})
	o.drv.Reset(ctx)
	o.waitEmit(t)
	if atomic.LoadInt64(&o.snapCalls) != 2 {
		t.Errorf("Reset should fetch a fresh snapshot, snapCalls=%d", o.snapCalls)
	}
	if o.drv.Engine().LastUpdateID() != 6000 {
		t.Errorf("after reset lastU = %d, want 6000", o.drv.Engine().LastUpdateID())
	}
}
