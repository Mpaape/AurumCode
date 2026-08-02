package cost

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// TestAllowDoesNotMutateUnderReadLock exercises the daily-reset path from many
// goroutines at once. resetDailyIfNeeded writes shared state, so every caller
// that can reach it must hold the write lock.
func TestAllowDoesNotMutateUnderReadLock(t *testing.T) {
	prices := map[string]PriceMap{"m": {InputPer1K: 0.001, OutputPer1K: 0.001}}

	for round := 0; round < 50; round++ {
		tracker := NewTracker(1000.0, 1000.0, prices)
		tracker.lastReset = time.Now().AddDate(0, 0, -1)

		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				if i%2 == 0 {
					tracker.Allow(10, 10, "m")
					return
				}
				tracker.Remaining()
			}(i)
		}
		close(start)
		wg.Wait()
	}
}

// TestSpendReportsUnpricedModel: an unpriced model is not charged to any
// budget. Returning nil makes that indistinguishable from a recorded spend, so
// a caller can loop forever against a budget that never moves.
func TestSpendReportsUnpricedModel(t *testing.T) {
	tracker := NewTracker(10.0, 100.0, map[string]PriceMap{
		"priced": {InputPer1K: 1.0, OutputPer1K: 1.0},
	})

	err := tracker.Spend(1_000_000, 1_000_000, "not-in-price-map")
	if err == nil {
		t.Fatal("Spend reported success for a model it never charged")
	}
	if !errors.Is(err, ErrUnknownModel) {
		t.Fatalf("expected ErrUnknownModel, got: %v", err)
	}

	perRun, daily := tracker.Remaining()
	if perRun != 10.0 || daily != 100.0 {
		t.Fatalf("unpriced spend must not move budgets, got perRun=%f daily=%f", perRun, daily)
	}
	if got := tracker.Untracked(); got != 1 {
		t.Fatalf("Untracked() = %d, want 1", got)
	}
}

// TestDailyBudgetResetsOnDayChange covers the rollover branch, which decides
// whether a run gets a fresh daily allowance.
func TestDailyBudgetResetsOnDayChange(t *testing.T) {
	prices := map[string]PriceMap{"m": {InputPer1K: 1.0, OutputPer1K: 1.0}}
	tracker := NewTracker(10.0, 10.0, prices)

	clock := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return clock }
	tracker.lastReset = clock

	if err := tracker.Spend(4000, 0, "m"); err != nil {
		t.Fatalf("Spend: %v", err)
	}
	perRun, daily := tracker.Remaining()
	if perRun != 6.0 || daily != 6.0 {
		t.Fatalf("after spend: perRun=%f daily=%f, want 6 and 6", perRun, daily)
	}

	clock = clock.AddDate(0, 0, 1)

	perRun, daily = tracker.Remaining()
	if daily != 10.0 {
		t.Errorf("daily budget did not reset on day change: %f", daily)
	}
	if perRun != 6.0 {
		t.Errorf("day change must not refill the per-run budget: %f", perRun)
	}
	if !tracker.Allow(4000, 0, "m") {
		t.Error("a request within the refreshed daily budget was blocked")
	}
}
