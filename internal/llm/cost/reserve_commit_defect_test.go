package cost

import "testing"

// TestReserve_FailsClosedOnUnknownModel exercises the fail-closed branch that
// Allow's tests do not reach: Reserve is the production path (Complete calls
// Reserve, not Allow), and a model missing from a configured price table must
// be refused, not admitted with an unbounded, unpriced hold.
//
// This test is RED against a Reserve whose unknown-model branch (costtracker.go
// 144-147) is mutated to fail OPEN, e.g. replacing
//
//	costUSD, ok := t.costOf(tokensIn, tokensOut, model)
//	if !ok {
//		return nil, false
//	}
//
// with an unconditional `return &Reservation{tracker: t, resetEpoch: t.lastReset}, true`.
func TestReserve_FailsClosedOnUnknownModel(t *testing.T) {
	tracker := NewTracker(10.0, 100.0, map[string]PriceMap{
		"priced": {InputPer1K: 0.01, OutputPer1K: 0.02},
	})

	res, ok := tracker.Reserve(1_000_000, 1_000_000, "not-in-price-map")
	if ok {
		t.Fatal("Reserve admitted a request against a model it cannot price")
	}
	if res != nil {
		t.Fatal("Reserve returned a non-nil reservation alongside ok=false")
	}

	// The refusal must not be a side effect of an exhausted budget, and it must
	// not have booked anything against either ceiling.
	perRun, daily := tracker.Remaining()
	if perRun != 10.0 || daily != 100.0 {
		t.Fatalf("a refused reservation moved a budget: perRun=%f daily=%f", perRun, daily)
	}

	res2, ok2 := tracker.Reserve(1000, 1000, "priced")
	if !ok2 || res2 == nil {
		t.Fatal("Reserve refused a priced request that fits, so the assertions above prove nothing")
	}
}

// TestReservationCommit_SecondCallDoesNotDoubleCharge covers the idempotency
// guard `if r.settled { return nil }` at costtracker.go 193-195.
//
// TestReservationSettlesOnce in reservation_rollover_test.go calls Commit with
// the SAME token counts used at Reserve time, so the guard's removal is
// mathematically invisible there: releaseHeld() gives back exactly what the
// re-run of the commit body then re-adds (-heldUSD +costUSD, with
// heldUSD == costUSD), netting to zero on every extra call regardless of
// whether the guard fires. This test instead settles with a DIFFERENT actual
// cost than the reservation held, so a second, unguarded settle changes the
// ledger: it subtracts the (already-refunded) hold again and re-adds the full
// actual cost again, in addition to the first settle.
//
// RED against a Commit whose idempotency check (costtracker.go 193-195) is
// removed, e.g. deleting:
//
//	if r.settled {
//		return nil
//	}
func TestReservationCommit_SecondCallDoesNotDoubleCharge(t *testing.T) {
	tracker := NewTracker(1000.0, 1000.0, map[string]PriceMap{
		"m": {InputPer1K: 1.0, OutputPer1K: 1.0},
	})

	// Reserve holds $1.00 (1000 input tokens @ $1.00/1k, 0 output tokens).
	res, ok := tracker.Reserve(1000, 0, "m")
	if !ok {
		t.Fatal("Reserve refused a request that fits")
	}

	// The request actually used more than estimated: settle at $4.00
	// (2000 in + 2000 out @ $1.00/1k each), deliberately different from the
	// $1.00 hold so a duplicated settle is numerically visible.
	if err := res.Commit(2000, 2000, "m"); err != nil {
		t.Fatalf("first Commit: %v", err)
	}

	wantPerRun := 1000.0 - 4.0
	wantDaily := 1000.0 - 4.0
	if perRun, daily := tracker.Remaining(); !nearlyUSD(perRun, wantPerRun) || !nearlyUSD(daily, wantDaily) {
		t.Fatalf("after first Commit: perRun=%f daily=%f, want perRun=%f daily=%f",
			perRun, daily, wantPerRun, wantDaily)
	}

	// A second Commit on an already-settled reservation must be a no-op.
	if err := res.Commit(2000, 2000, "m"); err != nil {
		t.Fatalf("second Commit: %v", err)
	}

	perRun, daily := tracker.Remaining()
	if !nearlyUSD(perRun, wantPerRun) {
		t.Errorf("perRun remaining = %f after a second Commit, want unchanged %f (settle-once was not enforced)",
			perRun, wantPerRun)
	}
	if !nearlyUSD(daily, wantDaily) {
		t.Errorf("daily remaining = %f after a second Commit, want unchanged %f (settle-once was not enforced)",
			daily, wantDaily)
	}
}

// TestReserve_RefusesRequestOverBudgetCeiling covers the fits() check inside
// Reserve (costtracker.go 149-151). Every other test that calls Reserve asks
// for an amount that comfortably fits, so nothing in the existing suite would
// notice if this specific check were deleted: Reserve would still price the
// model correctly and still book the cost, it would just never say no to a
// request the ledger cannot afford.
//
// RED against a Reserve whose ceiling check
//
//	if !t.fits(costUSD) {
//		return nil, false
//	}
//
// is removed, leaving the priced-but-unaffordable request booked and
// admitted instead of refused.
func TestReserve_RefusesRequestOverBudgetCeiling(t *testing.T) {
	tracker := NewTracker(1.0, 1.0, map[string]PriceMap{
		"m": {InputPer1K: 1.0, OutputPer1K: 1.0},
	})

	// 2000 input tokens at $1.00/1k costs $2.00, over both the $1.00 per-run
	// and $1.00 daily ceilings.
	res, ok := tracker.Reserve(2000, 0, "m")
	if ok {
		t.Fatal("Reserve admitted a request that costs more than either budget ceiling")
	}
	if res != nil {
		t.Fatal("Reserve returned a non-nil reservation alongside ok=false")
	}

	perRun, daily := tracker.Remaining()
	if !nearlyUSD(perRun, 1.0) || !nearlyUSD(daily, 1.0) {
		t.Fatalf("a refused reservation moved a budget: perRun=%f daily=%f, want 1.0 and 1.0", perRun, daily)
	}

	// A request that does fit must still be admitted, so the refusal above is
	// not a symptom of a tracker that blocks everything.
	res2, ok2 := tracker.Reserve(500, 0, "m")
	if !ok2 || res2 == nil {
		t.Fatal("Reserve refused a request that fits, so the assertions above prove nothing")
	}
}
