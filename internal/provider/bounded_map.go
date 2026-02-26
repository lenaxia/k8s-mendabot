package provider

import (
	"sync"
	"time"
)

// BoundedMap is a thread-safe map with TTL-based cleanup and size limits.
// It's designed to store firstSeen timestamps for fingerprint stabilization.
type BoundedMap struct {
	mu sync.RWMutex

	// data stores the actual entries
	data map[string]time.Time

	// maxSize is the maximum number of entries allowed (0 = unlimited)
	maxSize int

	// ttl is the time-to-live for entries. Entries older than ttl are cleaned up.
	// If 0, no TTL cleanup is performed.
	ttl time.Duration

	// lastCleanup tracks when we last performed cleanup
	lastCleanup time.Time

	// cleanupInterval is how often to run cleanup
	cleanupInterval time.Duration
}

// NewBoundedMap creates a new BoundedMap with the given configuration.
// If maxSize <= 0, size is unlimited.
// If ttl <= 0, no TTL cleanup is performed.
// cleanupInterval determines how often cleanup runs (default: ttl/2 if ttl > 0, else 5 minutes).
func NewBoundedMap(maxSize int, ttl, cleanupInterval time.Duration) *BoundedMap {
	if cleanupInterval <= 0 {
		if ttl > 0 {
			cleanupInterval = ttl / 2
		} else {
			cleanupInterval = 5 * time.Minute
		}
	}

	return &BoundedMap{
		data:            make(map[string]time.Time),
		maxSize:         maxSize,
		ttl:             ttl,
		lastCleanup:     time.Now(),
		cleanupInterval: cleanupInterval,
	}
}

// Set adds or updates an entry with the current timestamp.
// Returns true if the entry was added/updated, false if the map is at capacity
// and the entry is not more recent than existing entries.
func (m *BoundedMap) Set(key string) bool {
	return m.SetWithTime(key, time.Now())
}

// SetWithTime adds or updates an entry with the given timestamp.
// Returns true if the entry was added/updated, false if the map is at capacity
// and the entry is not more recent than existing entries.
func (m *BoundedMap) SetWithTime(key string, timestamp time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Run cleanup if needed
	m.cleanupIfNeeded()

	// Check if we're at capacity
	if m.maxSize > 0 && len(m.data) >= m.maxSize {
		// If key already exists, update it (this doesn't increase size)
		if _, exists := m.data[key]; exists {
			m.data[key] = timestamp
			return true
		}

		// Map is at capacity and key doesn't exist
		// Find and remove the oldest entry to make room
		var oldestKey string
		var oldestTime time.Time
		first := true

		for k, v := range m.data {
			if first || v.Before(oldestTime) {
				oldestKey = k
				oldestTime = v
				first = false
			}
		}

		if !first {
			delete(m.data, oldestKey)
		} else {
			// Should not happen since len(m.data) >= maxSize > 0
			return false
		}
	}

	m.data[key] = timestamp
	return true
}

// Get returns the timestamp for a key and whether it exists.
// If the entry exists but is older than the TTL, it is treated as absent.
func (m *BoundedMap) Get(key string) (time.Time, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ts, exists := m.data[key]
	if !exists {
		return time.Time{}, false
	}
	// Always enforce TTL on reads — do not wait for the cleanup interval.
	// This ensures that a genuinely recovered object (entry older than TTL)
	// is treated as unseen, restarting the stabilisation window correctly.
	if m.ttl > 0 && time.Since(ts) > m.ttl {
		return time.Time{}, false
	}
	return ts, true
}

// Delete removes an entry from the map.
func (m *BoundedMap) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, key)
}

// Len returns the number of entries in the map.
func (m *BoundedMap) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.data)
}

// Clear removes all entries from the map.
func (m *BoundedMap) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data = make(map[string]time.Time)
	m.lastCleanup = time.Now()
}

// NeedsCleanup returns true if cleanup is needed.
func (m *BoundedMap) needsCleanup() bool {
	if m.ttl <= 0 {
		return false
	}
	return time.Since(m.lastCleanup) > m.cleanupInterval
}

// cleanupIfNeeded performs TTL cleanup if needed.
// Caller must hold write lock.
func (m *BoundedMap) cleanupIfNeeded() {
	if !m.needsCleanup() {
		return
	}

	if m.ttl > 0 {
		cutoff := time.Now().Add(-m.ttl)
		for k, v := range m.data {
			if v.Before(cutoff) {
				delete(m.data, k)
			}
		}
	}

	m.lastCleanup = time.Now()
}

// Cleanup forces a cleanup operation.
func (m *BoundedMap) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cleanupIfNeeded()
}

// Keys returns all keys in the map (for testing).
func (m *BoundedMap) Keys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	return keys
}

// Copy returns a copy of the map (for testing).
func (m *BoundedMap) Copy() map[string]time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()

	copy := make(map[string]time.Time, len(m.data))
	for k, v := range m.data {
		copy[k] = v
	}
	return copy
}

// SetForTest sets an entry with a specific timestamp for testing.
// This should only be used in tests.
func (m *BoundedMap) SetForTest(key string, timestamp time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data[key] = timestamp
	// Reset lastCleanup to avoid immediate cleanup of test data
	m.lastCleanup = time.Now()
}
