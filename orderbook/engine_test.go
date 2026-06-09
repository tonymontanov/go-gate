/*
FILE: orderbook/engine_test.go

DESCRIPTION:
Offline unit tests for the shared order-book engine: snapshot seeding, in-order
delta application, size-0 deletion, stale-event skipping, sequence-gap detection,
resync, top-N reads, and concurrency safety (-race).
*/

package orderbook

import (
	"strconv"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
)

func lvl(price, size string) Level {
	return Level{Price: decimal.RequireFromString(price), Size: decimal.RequireFromString(size)}
}

func newPrimedEngine(t *testing.T) *Engine {
	t.Helper()
	var e *Engine = NewEngine("BTC_USDT", 100)
	var res ApplyResult = e.ApplySnapshot(Snapshot{
		Symbol: "BTC_USDT",
		Bids:   []Level{lvl("100", "1"), lvl("99", "2"), lvl("98", "3")},
		Asks:   []Level{lvl("101", "1"), lvl("102", "2"), lvl("103", "3")},
		ID:     1000,
		TsMs:   1700000000000,
	})
	if res.Gap != GapNone || res.LastU != 1000 {
		t.Fatalf("snapshot: got gap=%v lastU=%d", res.Gap, res.LastU)
	}
	if !e.IsPrimed() || e.IsDirty() {
		t.Fatalf("after snapshot: primed=%v dirty=%v", e.IsPrimed(), e.IsDirty())
	}
	return e
}

func TestSnapshotSortsSides(t *testing.T) {
	var e *Engine = NewEngine("BTC_USDT", 100)
	// Deliberately unsorted input.
	e.ApplySnapshot(Snapshot{
		Bids: []Level{lvl("98", "3"), lvl("100", "1"), lvl("99", "2")},
		Asks: []Level{lvl("103", "3"), lvl("101", "1"), lvl("102", "2")},
		ID:   1,
	})
	var bids, asks = e.TopLevels(0)
	// Bids descending, asks ascending.
	if bids[0].Price.String() != "100" || bids[2].Price.String() != "98" {
		t.Errorf("bids not sorted desc: %v", bids)
	}
	if asks[0].Price.String() != "101" || asks[2].Price.String() != "103" {
		t.Errorf("asks not sorted asc: %v", asks)
	}
}

func TestDeltaBeforeSnapshot_Initial(t *testing.T) {
	var e *Engine = NewEngine("BTC_USDT", 100)
	var res ApplyResult = e.ApplyDelta(Delta{FirstU: 1, LastU: 2, Bids: []Level{lvl("100", "1")}})
	if res.Gap != GapInitial {
		t.Errorf("expected GapInitial, got %v", res.Gap)
	}
	if !e.IsDirty() {
		t.Error("engine should be dirty after initial-gap delta")
	}
}

func TestDeltaInOrder_AppliesAndUpdatesLevels(t *testing.T) {
	var e *Engine = newPrimedEngine(t)
	// Contiguous delta: U = lastU+1 = 1001.
	var res ApplyResult = e.ApplyDelta(Delta{
		FirstU: 1001, LastU: 1001,
		Bids: []Level{lvl("100", "5"), lvl("97", "4")}, // update top bid, add a new level
		Asks: []Level{lvl("101", "0")},                 // delete best ask
		TsMs: 1700000000001,
	})
	if res.Gap != GapNone || res.Stale {
		t.Fatalf("expected clean apply, got gap=%v stale=%v", res.Gap, res.Stale)
	}
	if e.LastUpdateID() != 1001 {
		t.Errorf("lastU = %d, want 1001", e.LastUpdateID())
	}
	var bids, asks = e.TopLevels(0)
	// Top bid size updated to 5; new bid 97 inserted in sorted position (after 98).
	if bids[0].Price.String() != "100" || bids[0].Size.String() != "5" {
		t.Errorf("top bid = %v, want 100@5", bids[0])
	}
	if bids[len(bids)-1].Price.String() != "97" {
		t.Errorf("expected 97 inserted at tail, bids=%v", bids)
	}
	// Best ask 101 deleted → new best ask is 102.
	if asks[0].Price.String() != "102" {
		t.Errorf("best ask = %v, want 102 (101 deleted)", asks[0])
	}
}

func TestDeltaStale_Skipped(t *testing.T) {
	var e *Engine = newPrimedEngine(t)
	// Whole event id range <= lastU (1000) → stale, no change, not dirty.
	var res ApplyResult = e.ApplyDelta(Delta{FirstU: 998, LastU: 1000, Bids: []Level{lvl("100", "99")}})
	if res.Gap != GapNone || !res.Stale {
		t.Errorf("expected stale GapNone, got gap=%v stale=%v", res.Gap, res.Stale)
	}
	if e.IsDirty() {
		t.Error("stale delta must not mark engine dirty")
	}
	var bids, _ = e.TopLevels(1)
	if bids[0].Size.String() != "1" {
		t.Errorf("stale delta must not modify book, top bid size = %s", bids[0].Size.String())
	}
}

func TestDeltaGap_MarksDirty(t *testing.T) {
	var e *Engine = newPrimedEngine(t)
	// FirstU (1003) > lastU+1 (1001) → missed events.
	var res ApplyResult = e.ApplyDelta(Delta{FirstU: 1003, LastU: 1005, Bids: []Level{lvl("100", "9")}})
	if res.Gap != GapSequence {
		t.Errorf("expected GapSequence, got %v", res.Gap)
	}
	if !e.IsDirty() {
		t.Error("engine must be dirty after a sequence gap")
	}
	// State unchanged.
	var bids, _ = e.TopLevels(1)
	if bids[0].Size.String() != "1" {
		t.Errorf("gap delta must not modify book")
	}
}

func TestFirstDeltaAfterSnapshot_RangeAlignment(t *testing.T) {
	var e *Engine = newPrimedEngine(t) // lastU = 1000
	// First delta covers [999, 1002]; since U=999 <= lastU+1=1001 <= u=1002 it applies.
	var res ApplyResult = e.ApplyDelta(Delta{FirstU: 999, LastU: 1002, Bids: []Level{lvl("100", "7")}})
	if res.Gap != GapNone || res.Stale {
		t.Fatalf("range-aligned first delta should apply, got gap=%v stale=%v", res.Gap, res.Stale)
	}
	if e.LastUpdateID() != 1002 {
		t.Errorf("lastU = %d, want 1002", e.LastUpdateID())
	}
}

func TestResyncClearsDirty(t *testing.T) {
	var e *Engine = newPrimedEngine(t)
	e.ApplyDelta(Delta{FirstU: 5000, LastU: 5001}) // gap
	if !e.IsDirty() {
		t.Fatal("precondition: engine should be dirty")
	}
	// Re-seed with a fresh snapshot.
	e.ApplySnapshot(Snapshot{Bids: []Level{lvl("100", "1")}, Asks: []Level{lvl("101", "1")}, ID: 6000})
	if e.IsDirty() {
		t.Error("snapshot should clear dirty")
	}
	if e.LastUpdateID() != 6000 {
		t.Errorf("lastU = %d, want 6000", e.LastUpdateID())
	}
}

func TestMarkStale_RequiresResnapshot(t *testing.T) {
	var e *Engine = newPrimedEngine(t)
	e.MarkStale()
	if !e.IsDirty() || e.IsPrimed() {
		t.Fatalf("after MarkStale: dirty=%v primed=%v", e.IsDirty(), e.IsPrimed())
	}
	// A delta now yields GapInitial until a fresh snapshot arrives.
	var res ApplyResult = e.ApplyDelta(Delta{FirstU: 1001, LastU: 1001})
	if res.Gap != GapInitial {
		t.Errorf("after MarkStale a delta should be GapInitial, got %v", res.Gap)
	}
}

func TestTopLevels_Limit(t *testing.T) {
	var e *Engine = newPrimedEngine(t)
	var bids, asks = e.TopLevels(2)
	if len(bids) != 2 || len(asks) != 2 {
		t.Errorf("TopLevels(2) = %d/%d, want 2/2", len(bids), len(asks))
	}
	// Returned slice is a copy: mutating it must not affect the engine.
	bids[0].Size = decimal.NewFromInt(999)
	var again, _ = e.TopLevels(1)
	if again[0].Size.String() == "999" {
		t.Error("TopLevels must return a copy, not the engine's backing slice")
	}
}

func TestBestBidAsk(t *testing.T) {
	var e *Engine = newPrimedEngine(t)
	var bidPx, bidSz, askPx, askSz = e.BestBidAsk()
	if bidPx.String() != "100" || bidSz.String() != "1" || askPx.String() != "101" || askSz.String() != "1" {
		t.Errorf("BestBidAsk = %s@%s / %s@%s", bidPx, bidSz, askPx, askSz)
	}
}

// TestConcurrentApplyAndRead exercises the RWMutex under -race.
func TestConcurrentApplyAndRead(t *testing.T) {
	var e *Engine = newPrimedEngine(t)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		var i int
		for i = 0; i < 2000; i++ {
			var id int64 = int64(1001 + i)
			e.ApplyDelta(Delta{
				FirstU: id, LastU: id,
				Bids: []Level{lvl(strconv.Itoa(90+i%10), "1")},
				Asks: []Level{lvl(strconv.Itoa(110+i%10), "1")},
			})
		}
	}()
	go func() {
		defer wg.Done()
		var i int
		for i = 0; i < 2000; i++ {
			e.TopLevels(10)
			e.BestBidAsk()
			e.IsDirty()
		}
	}()
	wg.Wait()
}
