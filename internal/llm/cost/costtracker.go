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
// A model absent from the price map cannot be priced, so it is neither blocked
// nor charged: Allow returns true and Spend returns ErrUnknownModel while
// incrementing Untracked. Budgets constrain priced models only.
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

// Allow reports whether the estimated cost of a request fits both budgets.
func (t *Tracker) Allow(tokensIn, tokensOut int, model string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.resetDailyIfNeeded()

	price, ok := t.priceMap[model]
	if !ok {
		return true
	}

	costUSD := priceOf(tokensIn, tokensOut, price)

	return t.perRunUsedUSD+costUSD <= t.perRunUSD && t.dailyUsedUSD+costUSD <= t.dailyUSD
}

// Spend records the actual cost of a request. It returns ErrUnknownModel when
// the model has no price entry, in which case no budget was charged.
func (t *Tracker) Spend(tokensIn, tokensOut int, model string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.resetDailyIfNeeded()

	price, ok := t.priceMap[model]
	if !ok {
		t.untracked++
		return fmt.Errorf("%w: %q (%d in / %d out tokens were not charged)", ErrUnknownModel, model, tokensIn, tokensOut)
	}

	costUSD := priceOf(tokensIn, tokensOut, price)
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
