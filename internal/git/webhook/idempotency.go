package webhook

import (
	"sync"
	"time"
)

// IdempotencyCache provides thread-safe deduplication of webhook delivery IDs
type IdempotencyCache struct {
	mu      sync.RWMutex
	entries map[string]time.Time
	ttl     time.Duration
	maxSize int
}

// NewIdempotencyCache creates a new idempotency cache
// ttl is how long entries are kept
// maxSize is the maximum number of entries (0 = unlimited)
func NewIdempotencyCache(ttl time.Duration, maxSize int) *IdempotencyCache {
	cache := &IdempotencyCache{
		entries: make(map[string]time.Time),
		ttl:     ttl,
		maxSize: maxSize,
	}

	// Start background cleanup goroutine
	go cache.cleanup()

	return cache
}

// SeenOrAdd checks if an ID has been seen and adds it if not
// Returns true if the ID was already seen (duplicate)
// Returns false if this is the first time seeing it
func (c *IdempotencyCache) SeenOrAdd(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Check if ID exists and is not expired
	if expiry, exists := c.entries[id]; exists {
		if now.Before(expiry) {
			// Still valid - this is a duplicate
			return true
		}
		// Expired - remove it
		delete(c.entries, id)
	}

	// Enforce max size if set
	if c.maxSize > 0 && len(c.entries) >= c.maxSize {
		// Evict oldest entry
		c.evictOldest()
	}

	// Add new entry
	c.entries[id] = now.Add(c.ttl)

	return false
}

// Contains checks if an ID is in the cache without adding it
func (c *IdempotencyCache) Contains(id string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	expiry, exists := c.entries[id]
	if !exists {
		return false
	}

	// Check if expired
	if time.Now().After(expiry) {
		return false
	}

	return true
}

// Size returns the current number of entries
func (c *IdempotencyCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Clear removes all entries
func (c *IdempotencyCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]time.Time)
}

// evictOldest removes the oldest entry (must be called with lock held)
func (c *IdempotencyCache) evictOldest() {
	var oldestID string
	var oldestTime time.Time

	for id, expiry := range c.entries {
		if oldestID == "" || expiry.Before(oldestTime) {
			oldestID = id
			oldestTime = expiry
		}
	}

	if oldestID != "" {
		delete(c.entries, oldestID)
	}
}

// cleanup runs periodically to remove expired entries
func (c *IdempotencyCache) cleanup() {
	ticker := time.NewTicker(c.ttl / 2) // Cleanup at half TTL interval
	defer ticker.Stop()

	for range ticker.C {
		c.removeExpired()
	}
}

// removeExpired removes all expired entries
func (c *IdempotencyCache) removeExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for id, expiry := range c.entries {
		if now.After(expiry) {
			delete(c.entries, id)
		}
	}
}
