package llm

import (
	"sync"
)

// Estimator provides token counting with LRU caching
type Estimator struct {
	cache map[string]int
	mu    sync.RWMutex
}

// NewEstimator creates a new token estimator
func NewEstimator() *Estimator {
	return &Estimator{
		cache: make(map[string]int),
	}
}

// Estimate estimates tokens using provider.Tokens if available, else heuristics
func (e *Estimator) Estimate(provider Provider, input string, model string) int {
	// Try provider first
	count, err := provider.Tokens(input)
	if err == nil && count > 0 {
		return count
	}
	
	// Fallback to heuristic approximation
	// Simple heuristic: ~4 chars per token for most models
	heuristicCount := len(input) / 4
	
	// Cache the result
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cache[input] = heuristicCount
	
	return heuristicCount
}

// GetCached returns cached estimate if available
func (e *Estimator) GetCached(input string) (int, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	count, ok := e.cache[input]
	return count, ok
}

// EstimateTokens estimates tokens using heuristics and caching
func (e *Estimator) EstimateTokens(input string) (int, error) {
	// Check cache first
	if count, ok := e.GetCached(input); ok {
		return count, nil
	}

	// Simple heuristic: ~4 chars per token for most models
	count := len(input) / 4
	if count == 0 {
		count = 1 // minimum 1 token
	}

	// Cache the result
	e.mu.Lock()
	e.cache[input] = count
	e.mu.Unlock()

	return count, nil
}

