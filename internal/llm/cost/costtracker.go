package cost

import (
	"sync"
	"time"
)

// PriceMap represents the cost per 1k tokens for input and output
type PriceMap struct {
	InputPer1K  float64 `json:"input_per_1k" yaml:"input_per_1k"`   // $ per 1k input tokens
	OutputPer1K float64 `json:"output_per_1k" yaml:"output_per_1k"` // $ per 1k output tokens
}

// Tracker manages LLM cost tracking with per-run and daily budgets
type Tracker struct {
	mu                sync.RWMutex
	priceMap          map[string]PriceMap // model key -> prices
	perRunUSD         float64
	perRunUsedUSD     float64
	dailyUSD          float64
	dailyUsedUSD      float64
	lastReset         time.Time
}

// NewTracker creates a new cost tracker with the given budgets and prices
func NewTracker(perRunUSD, dailyUSD float64, prices map[string]PriceMap) *Tracker {
	return &Tracker{
		priceMap:          prices,
		perRunUSD:         perRunUSD,
		perRunUsedUSD:     0.0,
		dailyUSD:          dailyUSD,
		dailyUsedUSD:      0.0,
		lastReset:         time.Now(),
	}
}

// Allow checks if the estimated cost is within budget
func (t *Tracker) Allow(tokensIn, tokensOut int, model string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	// Reset daily budget if needed
	t.resetDailyIfNeeded()
	
	// Get price for model
	price, ok := t.priceMap[model]
	if !ok {
		// Unknown model - be conservative and allow
		return true
	}
	
	// Calculate estimated cost
	costUSD := (float64(tokensIn)/1000.0)*price.InputPer1K + (float64(tokensOut)/1000.0)*price.OutputPer1K
	
	// Check both budgets
	if t.perRunUsedUSD+costUSD > t.perRunUSD {
		return false
	}
	
	if t.dailyUsedUSD+costUSD > t.dailyUSD {
		return false
	}
	
	return true
}

// Spend records the actual cost of a request
func (t *Tracker) Spend(tokensIn, tokensOut int, model string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	// Reset daily budget if needed
	t.resetDailyIfNeeded()
	
	// Get price for model
	price, ok := t.priceMap[model]
	if !ok {
		// Unknown model - skip accounting but allow
		return nil
	}
	
	// Calculate cost
	costUSD := (float64(tokensIn)/1000.0)*price.InputPer1K + (float64(tokensOut)/1000.0)*price.OutputPer1K
	
	t.perRunUsedUSD += costUSD
	t.dailyUsedUSD += costUSD
	
	return nil
}

// Remaining returns the remaining budget as a tuple (perRun, daily)
func (t *Tracker) Remaining() (float64, float64) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	t.resetDailyIfNeeded()
	
	return t.perRunUSD - t.perRunUsedUSD, t.dailyUSD - t.dailyUsedUSD
}

// resetDailyIfNeeded checks if we need to reset the daily budget
// Must be called with lock held
func (t *Tracker) resetDailyIfNeeded() {
	now := time.Now()
	if now.Day() != t.lastReset.Day() || now.Month() != t.lastReset.Month() || now.Year() != t.lastReset.Year() {
		t.dailyUsedUSD = 0.0
		t.lastReset = now
	}
}

// ResetPerRun resets the per-run counter
func (t *Tracker) ResetPerRun() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.perRunUsedUSD = 0.0
}

