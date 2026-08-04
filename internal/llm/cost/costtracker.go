package cost

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrUnknownModel reports a spend that was not charged to any budget because
// the model has no entry in the price map. A caller that ignores it is running
// without cost control.
var ErrUnknownModel = errors.New("model has no price entry")

// PriceMap represents the cost per 1k tokens for input and output
type PriceMap struct {
	InputPer1K  float64 `json:"input_per_1k" yaml:"input_per_1k"`   // $ per 1k input tokens
	OutputPer1K float64 `json:"output_per_1k" yaml:"output_per_1k"` // $ per 1k output tokens
}

// Tracker manages LLM cost tracking with per-run and daily budgets.
//
// Two different situations are deliberately not treated the same way:
//
//   - No price table at all. The tracker was never configured with anything to
//     enforce, so it blocks nothing. Every spend is still reported as
//     ErrUnknownModel and counted by Untracked, so a run with no cost control
//     is visible instead of silent.
//   - A price table that does not contain this model. Pricing WAS configured
//     and this request falls outside it. The ceiling admits it, so it must fail
//     closed: admitting it means spending an unknown amount against a limit
//     that, for that request, can never be reached.
type Tracker struct {
	mu            sync.Mutex
	priceMap      map[string]PriceMap // model key -> prices
	perRunUSD     float64
	perRunUsedUSD float64
	dailyUSD      float64
	dailyUsedUSD  float64
	lastReset     time.Time
	untracked     int
	now           func() time.Time
}

// NewTracker creates a new cost tracker with the given budgets and prices
func NewTracker(perRunUSD, dailyUSD float64, prices map[string]PriceMap) *Tracker {
	return &Tracker{
		priceMap:  prices,
		perRunUSD: perRunUSD,
		dailyUSD:  dailyUSD,
		lastReset: time.Now(),
		now:       time.Now,
	}
}

// enforcing reports whether this tracker was given anything to enforce.
// Callers must hold t.mu.
func (t *Tracker) enforcing() bool {
	return len(t.priceMap) > 0
}

// costOf prices a request, reporting ok=false when the model has no entry in
// the price table. Callers must hold t.mu.
func (t *Tracker) costOf(tokensIn, tokensOut int, model string) (float64, bool) {
	price, ok := t.priceMap[model]
	if !ok {
		return 0, false
	}
	return priceOf(tokensIn, tokensOut, price), true
}

// fits reports whether costUSD still sits under both ceilings.
// Callers must hold t.mu.
func (t *Tracker) fits(costUSD float64) bool {
	return t.perRunUsedUSD+costUSD <= t.perRunUSD && t.dailyUsedUSD+costUSD <= t.dailyUSD
}

// Allow reports whether the estimated cost of a request fits both budgets.
//
// It FAILS CLOSED: against a configured price table, a model with no price is
// refused rather than admitted, because a request that cannot be priced cannot
// be capped.
//
// Allow reads the ledger and returns; it books nothing. Two callers can
// therefore both be told yes for a cost only one of them fits under. It has no
// production caller in this repository for exactly that reason - Complete uses
// Reserve, which makes the decision and the booking one atomic step. Allow
// survives as a non-binding preview for callers that want to know whether a
// request would fit right now.
func (t *Tracker) Allow(tokensIn, tokensOut int, model string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.resetDailyIfNeeded()

	if !t.enforcing() {
		return true
	}

	costUSD, ok := t.costOf(tokensIn, tokensOut, model)
	if !ok {
		return false
	}

	return t.fits(costUSD)
}

// Reservation is a booking held against both budgets while a request is in
// flight. Exactly one of Commit or Release settles it; both are idempotent.
type Reservation struct {
	tracker *Tracker
	heldUSD float64

	// resetEpoch is the tracker's lastReset at the moment the hold was booked.
	// If a day rollover clears the daily counter before the reservation is
	// settled, the held amount was already wiped by that reset and must not be
	// subtracted a second time - doing so drives dailyUsedUSD negative and
	// hands the run a daily budget larger than the configured one.
	resetEpoch time.Time

	settled bool
}

// Reserve atomically checks the estimated cost against both ceilings and, if it
// fits, books it immediately.
//
// Booking up front is what closes the check-then-act hole: with a bare
// Allow-then-Spend pair every concurrent request reads a ledger that no
// in-flight request has written to yet, so all of them clear a ceiling only one
// of them fits under.
//
// Like Allow, Reserve fails closed on a model absent from a configured price
// table, and holds nothing when no price table was configured at all.
func (t *Tracker) Reserve(tokensIn, tokensOut int, model string) (*Reservation, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.resetDailyIfNeeded()

	if !t.enforcing() {
		return &Reservation{tracker: t, resetEpoch: t.lastReset}, true
	}

	costUSD, ok := t.costOf(tokensIn, tokensOut, model)
	if !ok {
		return nil, false
	}

	if !t.fits(costUSD) {
		return nil, false
	}

	t.perRunUsedUSD += costUSD
	t.dailyUsedUSD += costUSD

	return &Reservation{tracker: t, heldUSD: costUSD, resetEpoch: t.lastReset}, true
}

// releaseHeld gives the held estimate back. Callers must hold t.mu.
//
// The daily reset is applied FIRST. Doing it after the subtraction would let a
// midnight rollover zero dailyUsedUSD and then have the refund subtract from
// that zero.
func (r *Reservation) releaseHeld() {
	t := r.tracker

	t.resetDailyIfNeeded()

	t.perRunUsedUSD -= r.heldUSD

	// Only refund the daily counter if it is still the same counter the hold
	// was booked into. After a rollover that booking no longer exists.
	if t.lastReset.Equal(r.resetEpoch) {
		t.dailyUsedUSD -= r.heldUSD
	}
}

// Commit swaps the held estimate for the actual cost of the completed request,
// in one critical section so no concurrent reservation can slip between the two.
//
// The actual cost is booked even if it overshoots the ceiling: the money was
// already spent upstream, and a ledger that hides real spend is worse than one
// that reports an overrun.
func (r *Reservation) Commit(tokensIn, tokensOut int, model string) error {
	if r == nil {
		return nil
	}

	t := r.tracker
	t.mu.Lock()
	defer t.mu.Unlock()

	if r.settled {
		return nil
	}
	r.settled = true

	r.releaseHeld()

	costUSD, ok := t.costOf(tokensIn, tokensOut, model)
	if !ok {
		t.untracked++
		return fmt.Errorf("%w: %q (%d in / %d out tokens were not charged)", ErrUnknownModel, model, tokensIn, tokensOut)
	}

	t.perRunUsedUSD += costUSD
	t.dailyUsedUSD += costUSD

	return nil
}

// Release cancels the reservation, booking nothing. Used when the request never
// produced billable work: a provider error, or a timeout.
func (r *Reservation) Release() {
	if r == nil {
		return
	}

	t := r.tracker
	t.mu.Lock()
	defer t.mu.Unlock()

	if r.settled {
		return
	}
	r.settled = true

	r.releaseHeld()
}

// Spend records the actual cost of a request. It returns ErrUnknownModel when
// the model has no price entry, in which case no budget was charged.
func (t *Tracker) Spend(tokensIn, tokensOut int, model string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.resetDailyIfNeeded()

	costUSD, ok := t.costOf(tokensIn, tokensOut, model)
	if !ok {
		t.untracked++
		return fmt.Errorf("%w: %q (%d in / %d out tokens were not charged)", ErrUnknownModel, model, tokensIn, tokensOut)
	}

	t.perRunUsedUSD += costUSD
	t.dailyUsedUSD += costUSD

	return nil
}

// Remaining returns the remaining budget as a tuple (perRun, daily)
func (t *Tracker) Remaining() (float64, float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.resetDailyIfNeeded()

	return t.perRunUSD - t.perRunUsedUSD, t.dailyUSD - t.dailyUsedUSD
}

// Untracked returns how many spends were not charged to any budget.
func (t *Tracker) Untracked() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.untracked
}

// ResetPerRun resets the per-run counter
func (t *Tracker) ResetPerRun() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.perRunUsedUSD = 0.0
}

func priceOf(tokensIn, tokensOut int, price PriceMap) float64 {
	return (float64(tokensIn)/1000.0)*price.InputPer1K + (float64(tokensOut)/1000.0)*price.OutputPer1K
}

// resetDailyIfNeeded mutates the daily counters, so callers must hold t.mu.
func (t *Tracker) resetDailyIfNeeded() {
	now := t.now()
	if now.Day() != t.lastReset.Day() || now.Month() != t.lastReset.Month() || now.Year() != t.lastReset.Year() {
		t.dailyUsedUSD = 0.0
		t.lastReset = now
	}
}
