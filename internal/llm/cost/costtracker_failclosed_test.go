package cost

import "testing"

// A budget ceiling that cannot price a request cannot enforce anything about
// it. Admitting such a request means spending an unknown amount against a limit
// that is, for that request, permanently inert. The tracker must refuse.
//
// These tests are red on a tracker whose Allow returns true for a model missing
// from a configured price table.

func TestTrackerAllow_FailsClosedOnUnknownModel(t *testing.T) {
	tracker := NewTracker(10.0, 100.0, map[string]PriceMap{
		"priced": {InputPer1K: 0.01, OutputPer1K: 0.02},
	})

	if tracker.Allow(1_000_000, 1_000_000, "not-in-price-map") {
		t.Fatal("Allow admitted a request it cannot price against a configured price table")
	}

	// The refusal must not be a side effect of an exhausted budget: the priced
	// model of the same size is still admitted.
	if !tracker.Allow(1000, 1000, "priced") {
		t.Fatal("Allow refused a priced request that fits, so the test above proves nothing")
	}
}

func TestTrackerAllow_FailsClosedOnEmptyModelKey(t *testing.T) {
	tracker := NewTracker(10.0, 100.0, map[string]PriceMap{
		"priced": {InputPer1K: 0.01, OutputPer1K: 0.02},
	})

	if tracker.Allow(1000, 1000, "") {
		t.Fatal("Allow admitted an empty model key against a configured price table")
	}
}

// An unconfigured tracker (no price table at all) has nothing to enforce, so it
// must not block the run. It must still make the missing accounting visible
// rather than pretending the spend was booked.
func TestTrackerWithoutPrices_AdmitsButReportsEverySpendAsUntracked(t *testing.T) {
	tracker := NewTracker(10.0, 100.0, map[string]PriceMap{})

	if !tracker.Allow(1_000_000, 1_000_000, "anything") {
		t.Fatal("a tracker with no price table blocked a request it has no ceiling for")
	}

	if err := tracker.Spend(1_000_000, 1_000_000, "anything"); err == nil {
		t.Fatal("Spend reported success for a charge it never booked")
	}

	if got := tracker.Untracked(); got != 1 {
		t.Fatalf("Untracked() = %d, want 1", got)
	}

	perRun, daily := tracker.Remaining()
	if perRun != 10.0 || daily != 100.0 {
		t.Fatalf("an unbookable spend moved a budget: perRun=%f daily=%f", perRun, daily)
	}
}
