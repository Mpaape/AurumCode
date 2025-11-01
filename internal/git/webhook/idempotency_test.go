package webhook

import (
	"sync"
	"testing"
	"time"
)

func TestIdempotencyCache_SeenOrAdd(t *testing.T) {
	cache := NewIdempotencyCache(1*time.Second, 0)

	// First time should return false (not seen)
	if cache.SeenOrAdd("delivery-1") {
		t.Error("expected false for first occurrence")
	}

	// Second time should return true (duplicate)
	if !cache.SeenOrAdd("delivery-1") {
		t.Error("expected true for duplicate")
	}

	// Different ID should return false
	if cache.SeenOrAdd("delivery-2") {
		t.Error("expected false for different ID")
	}
}

func TestIdempotencyCache_Contains(t *testing.T) {
	cache := NewIdempotencyCache(1*time.Second, 0)

	// Should not contain before adding
	if cache.Contains("delivery-1") {
		t.Error("expected false before adding")
	}

	// Add entry
	cache.SeenOrAdd("delivery-1")

	// Should contain after adding
	if !cache.Contains("delivery-1") {
		t.Error("expected true after adding")
	}
}

func TestIdempotencyCache_Expiry(t *testing.T) {
	cache := NewIdempotencyCache(50*time.Millisecond, 0)

	// Add entry
	cache.SeenOrAdd("delivery-1")

	// Should be seen immediately
	if !cache.SeenOrAdd("delivery-1") {
		t.Error("expected duplicate immediately after adding")
	}

	// Wait for expiry
	time.Sleep(60 * time.Millisecond)

	// Should no longer be seen
	if cache.SeenOrAdd("delivery-1") {
		t.Error("expected false after expiry")
	}
}

func TestIdempotencyCache_MaxSize(t *testing.T) {
	cache := NewIdempotencyCache(1*time.Second, 2)

	// Add 2 entries (at capacity)
	cache.SeenOrAdd("delivery-1")
	cache.SeenOrAdd("delivery-2")

	if cache.Size() != 2 {
		t.Errorf("expected size 2, got %d", cache.Size())
	}

	// Add 3rd entry (should evict oldest)
	cache.SeenOrAdd("delivery-3")

	if cache.Size() != 2 {
		t.Errorf("expected size 2 after eviction, got %d", cache.Size())
	}

	// delivery-1 should have been evicted
	if cache.Contains("delivery-1") {
		t.Error("expected delivery-1 to be evicted")
	}

	// delivery-2 and delivery-3 should still be present
	if !cache.Contains("delivery-2") {
		t.Error("expected delivery-2 to be present")
	}
	if !cache.Contains("delivery-3") {
		t.Error("expected delivery-3 to be present")
	}
}

func TestIdempotencyCache_Clear(t *testing.T) {
	cache := NewIdempotencyCache(1*time.Second, 0)

	cache.SeenOrAdd("delivery-1")
	cache.SeenOrAdd("delivery-2")

	if cache.Size() != 2 {
		t.Errorf("expected size 2, got %d", cache.Size())
	}

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", cache.Size())
	}

	if cache.Contains("delivery-1") {
		t.Error("expected delivery-1 to be cleared")
	}
}

func TestIdempotencyCache_Concurrent(t *testing.T) {
	cache := NewIdempotencyCache(1*time.Second, 0)

	var wg sync.WaitGroup
	concurrency := 100

	// Concurrently add same ID
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.SeenOrAdd("concurrent-delivery")
		}()
	}

	wg.Wait()

	// Should only have one entry
	if cache.Size() != 1 {
		t.Errorf("expected size 1, got %d", cache.Size())
	}
}

func TestIdempotencyCache_ConcurrentDifferentIDs(t *testing.T) {
	cache := NewIdempotencyCache(1*time.Second, 0)

	var wg sync.WaitGroup
	concurrency := 100

	// Concurrently add different IDs
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cache.SeenOrAdd(string(rune('A' + id)))
		}(i)
	}

	wg.Wait()

	// Should have concurrency entries
	size := cache.Size()
	if size != concurrency {
		t.Errorf("expected size %d, got %d", concurrency, size)
	}
}

func TestIdempotencyCache_AutoCleanup(t *testing.T) {
	cache := NewIdempotencyCache(100*time.Millisecond, 0)

	// Add entries
	cache.SeenOrAdd("delivery-1")
	cache.SeenOrAdd("delivery-2")

	if cache.Size() != 2 {
		t.Errorf("expected size 2, got %d", cache.Size())
	}

	// Wait for cleanup (runs at half TTL interval)
	time.Sleep(200 * time.Millisecond)

	// Entries should be cleaned up
	if cache.Size() != 0 {
		t.Errorf("expected size 0 after cleanup, got %d", cache.Size())
	}
}

func TestIdempotencyCache_Size(t *testing.T) {
	cache := NewIdempotencyCache(1*time.Second, 0)

	if cache.Size() != 0 {
		t.Errorf("expected initial size 0, got %d", cache.Size())
	}

	cache.SeenOrAdd("delivery-1")
	if cache.Size() != 1 {
		t.Errorf("expected size 1, got %d", cache.Size())
	}

	cache.SeenOrAdd("delivery-2")
	if cache.Size() != 2 {
		t.Errorf("expected size 2, got %d", cache.Size())
	}

	// Duplicate should not increase size
	cache.SeenOrAdd("delivery-1")
	if cache.Size() != 2 {
		t.Errorf("expected size 2 after duplicate, got %d", cache.Size())
	}
}

// BenchmarkIdempotencyCache_SeenOrAdd measures performance
func BenchmarkIdempotencyCache_SeenOrAdd(b *testing.B) {
	cache := NewIdempotencyCache(1*time.Minute, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.SeenOrAdd("benchmark-delivery")
	}
}

// BenchmarkIdempotencyCache_Contains measures lookup performance
func BenchmarkIdempotencyCache_Contains(b *testing.B) {
	cache := NewIdempotencyCache(1*time.Minute, 0)
	cache.SeenOrAdd("benchmark-delivery")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Contains("benchmark-delivery")
	}
}
