package cost

import (
	"testing"
	"time"
)

// A reservation books its estimate into the daily counter and gives it back
// when it settles. If the day rolls over in between, the daily counter has
// already been zeroed and the hold no longer exists in it. Subtracting the hold
// anyway drives dailyUsedUSD below zero, which makes Remaining report MORE than
// the configured daily budget - a free allowance, renewed at every midnight
// that lands inside an in-flight request.
//
// The clock is injected, so the window is exercised deterministically instead
// of being left to whether a test happens to run at midnight.

const (
	rolloverPerRun = 1000.0
	rolloverDaily  = 10.0
)

func rolloverTracker(t *testing.T) (*Tracker, *time.Time) {
	t.Helper()

	clock := time.Date(2026, 3, 1, 23, 59, 59, 0, time.UTC)
	tracker := NewTracker(rolloverPerRun, rolloverDaily, map[string]PriceMap{
		"m": {InputPer1K: 1.0, OutputPer1K: 1.0},
	})
	tracker.now = func() time.Time { return clock }
	tracker.lastReset = clock

	return tracker, &clock
}

func TestReservationRelease_DayRolloverDoesNotCreateFreeDailyBudget(t *testing.T) {
	tracker, clock := rolloverTracker(t)

	// $1.00 held: 1000 input tokens at $1.00/1k.
	res, ok := tracker.Reserve(1000, 0, "m")
	if !ok {
		t.Fatal("Reserve refused a request that fits")
	}
	if _, daily := tracker.Remaining(); !nearlyUSD(daily, rolloverDaily-1.0) {
		t.Fatalf("the hold was not booked: daily remaining %f", daily)
	}

	// Midnight passes, and something observes the tracker, which clears the
	// daily counter the hold was booked into.
	*clock = clock.AddDate(0, 0, 1)
	if _, daily := tracker.Remaining(); !nearlyUSD(daily, rolloverDaily) {
		t.Fatalf("daily budget did not reset on day change: %f", daily)
	}

	res.Release()

	perRun, daily := tracker.Remaining()
	if daily > rolloverDaily {
		t.Errorf("release after a day rollover handed out %f of daily budget, more than the configured %f",
			daily, rolloverDaily)
	}
	if !nearlyUSD(daily, rolloverDaily) {
		t.Errorf("daily remaining = %f, want %f", daily, rolloverDaily)
	}
	// The per-run counter has no rollover, so the hold must come back in full.
	if !nearlyUSD(perRun, rolloverPerRun) {
		t.Errorf("per-run remaining = %f, want %f", perRun, rolloverPerRun)
	}
}

func TestReservationCommit_DayRolloverDoesNotCreateFreeDailyBudget(t *testing.T) {
	tracker, clock := rolloverTracker(t)

	res, ok := tracker.Reserve(1000, 0, "m")
	if !ok {
		t.Fatal("Reserve refused a request that fits")
	}

	*clock = clock.AddDate(0, 0, 1)
	tracker.Remaining() // clears the counter the hold lives in

	// The request really cost $2.00: 1000 in + 1000 out at $1.00/1k each.
	if err := res.Commit(1000, 1000, "m"); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	perRun, daily := tracker.Remaining()
	if daily > rolloverDaily {
		t.Errorf("commit after a day rollover handed out %f of daily budget, more than the configured %f",
			daily, rolloverDaily)
	}
	// Only the actual cost is booked into the new day; the stale hold is not
	// refunded out of it.
	if !nearlyUSD(daily, rolloverDaily-2.0) {
		t.Errorf("daily remaining = %f, want %f", daily, rolloverDaily-2.0)
	}
	if !nearlyUSD(perRun, rolloverPerRun-2.0) {
		t.Errorf("per-run remaining = %f, want %f", perRun, rolloverPerRun-2.0)
	}
}

// Without a rollover the refund must still happen in full. Without this case,
// a settle path that simply never refunds the daily counter would pass the two
// tests above.
func TestReservationRelease_WithoutRolloverRefundsBothCounters(t *testing.T) {
	tracker, _ := rolloverTracker(t)

	res, ok := tracker.Reserve(1000, 0, "m")
	if !ok {
		t.Fatal("Reserve refused a request that fits")
	}

	res.Release()

	perRun, daily := tracker.Remaining()
	if !nearlyUSD(daily, rolloverDaily) {
		t.Errorf("daily remaining = %f, want the hold fully refunded (%f)", daily, rolloverDaily)
	}
	if !nearlyUSD(perRun, rolloverPerRun) {
		t.Errorf("per-run remaining = %f, want the hold fully refunded (%f)", perRun, rolloverPerRun)
	}
}

// Settling twice must not refund twice.
func TestReservationSettlesOnce(t *testing.T) {
	tracker, _ := rolloverTracker(t)

	res, ok := tracker.Reserve(1000, 1000, "m")
	if !ok {
		t.Fatal("Reserve refused a request that fits")
	}

	if err := res.Commit(1000, 1000, "m"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	res.Release()
	res.Release()
	if err := res.Commit(1000, 1000, "m"); err != nil {
		t.Fatalf("second Commit: %v", err)
	}

	perRun, daily := tracker.Remaining()
	if !nearlyUSD(daily, rolloverDaily-2.0) {
		t.Errorf("daily remaining = %f, want %f", daily, rolloverDaily-2.0)
	}
	if !nearlyUSD(perRun, rolloverPerRun-2.0) {
		t.Errorf("per-run remaining = %f, want %f", perRun, rolloverPerRun-2.0)
	}
}

func nearlyUSD(got, want float64) bool {
	const epsilon = 1e-9
	d := got - want
	if d < 0 {
		d = -d
	}
	return d < epsilon
}
