package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// estimatorCacheMaxEntries caps the cache. Inputs are caller-supplied prompts,
// so an unbounded map would grow with the workload for the life of the process.
const estimatorCacheMaxEntries = 4096

// Estimator counts tokens for budget checks, caching results per (model, input).
//
// Entries are keyed by a digest of the input: the cache must not keep prompt
// text alive, because prompts carry repository content. Eviction is arbitrary,
// not LRU - the cache exists to avoid recomputation, not to guarantee hits.
type Estimator struct {
	mu    sync.RWMutex
	cache map[string]int
}

// NewEstimator creates a new token estimator
func NewEstimator() *Estimator {
	return &Estimator{
		cache: make(map[string]int),
	}
}

// Estimate returns the provider's own token count for input, falling back to
// the character heuristic when the provider cannot count.
func (e *Estimator) Estimate(provider Provider, input string, model string) int {
	key := cacheKey(model, input)
	if count, ok := e.lookup(key); ok {
		return count
	}

	count, err := provider.Tokens(input)
	if err != nil || count <= 0 {
		count = heuristicTokens(input)
	}

	e.store(key, count)

	return count
}

// EstimateTokens returns a model-independent token estimate for input.
func (e *Estimator) EstimateTokens(input string) int {
	key := cacheKey("", input)
	if count, ok := e.lookup(key); ok {
		return count
	}

	count := heuristicTokens(input)
	e.store(key, count)

	return count
}

// heuristicTokens approximates ~4 characters per token, never below one token
// so that a non-empty input can never be budgeted as free.
func heuristicTokens(input string) int {
	if count := len(input) / 4; count > 0 {
		return count
	}
	return 1
}

func cacheKey(model, input string) string {
	sum := sha256.Sum256([]byte(model + "\x00" + input))
	return hex.EncodeToString(sum[:])
}

func (e *Estimator) lookup(key string) (int, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	count, ok := e.cache[key]
	return count, ok
}

func (e *Estimator) store(key string, count int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.cache) >= estimatorCacheMaxEntries {
		for evict := range e.cache {
			delete(e.cache, evict)
			break
		}
	}

	e.cache[key] = count
}
