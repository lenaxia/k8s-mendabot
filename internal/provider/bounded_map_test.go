package provider

import (
	"testing"
	"time"
)

// TestBoundedMap_GetAlwaysEnforcesTTL verifies that Get() returns (time.Time{}, false)
// for an entry that is older than the TTL, regardless of whether the cleanup interval
// has elapsed.  This is the core invariant that allows BoundedMap to serve as a
// reliable firstSeen store for the stabilisation window.
//
// Prior to the fix, Get() only enforced TTL when needsCleanup() was true (i.e., the
// cleanup interval had elapsed).  If SetForTest reset lastCleanup to time.Now(), the
// cleanup interval would not have elapsed and Get() would return the expired entry as
// present — causing the stabilisation window to never restart after a genuine recovery.
func TestBoundedMap_GetAlwaysEnforcesTTL(t *testing.T) {
	ttl := 100 * time.Millisecond
	m := NewBoundedMap(100, ttl, 0)

	// Insert an entry with a timestamp well past the TTL.
	key := "expired-key"
	m.SetForTest(key, time.Now().Add(-200*time.Millisecond)) // 200ms > ttl(100ms)

	// Immediately call Get — cleanup interval has NOT elapsed (SetForTest resets it).
	ts, exists := m.Get(key)
	if exists {
		t.Errorf("Get returned exists=true for TTL-expired entry (ts=%v); expected false — TTL not enforced on read", ts)
	}
}

// TestBoundedMap_GetReturnsEntryWithinTTL verifies that Get() returns a valid entry
// that has not yet exceeded the TTL.
func TestBoundedMap_GetReturnsEntryWithinTTL(t *testing.T) {
	ttl := 10 * time.Second
	m := NewBoundedMap(100, ttl, 0)

	key := "live-key"
	m.Set(key) // set to now

	ts, exists := m.Get(key)
	if !exists {
		t.Error("Get returned exists=false for live entry within TTL")
	}
	if ts.IsZero() {
		t.Error("Get returned zero timestamp for live entry")
	}
}

// TestBoundedMap_GetReturnsNotFoundForMissingKey verifies that Get() returns false
// for a key that has never been set.
func TestBoundedMap_GetReturnsNotFoundForMissingKey(t *testing.T) {
	m := NewBoundedMap(100, 10*time.Second, 0)

	_, exists := m.Get("no-such-key")
	if exists {
		t.Error("Get returned exists=true for a key that was never set")
	}
}

// TestBoundedMap_GetNoTTL_NeverExpires verifies that when ttl==0, entries never
// expire via Get() regardless of their age.
func TestBoundedMap_GetNoTTL_NeverExpires(t *testing.T) {
	m := NewBoundedMap(100, 0, 0) // no TTL

	key := "ancient-key"
	m.SetForTest(key, time.Now().Add(-24*time.Hour)) // very old

	_, exists := m.Get(key)
	if !exists {
		t.Error("Get returned exists=false for ancient entry with ttl==0; entries should never expire when ttl is 0")
	}
}

// TestBoundedMap_SetOverwritesExistingEntry verifies that a second Set() for the same
// key updates the timestamp (does not create a duplicate entry).
func TestBoundedMap_SetOverwritesExistingEntry(t *testing.T) {
	m := NewBoundedMap(100, 10*time.Second, 0)

	key := "dup-key"
	before := time.Now().Add(-5 * time.Second)
	m.SetForTest(key, before)

	time.Sleep(2 * time.Millisecond)
	m.Set(key) // update to now

	ts, exists := m.Get(key)
	if !exists {
		t.Fatal("expected entry to exist after second Set()")
	}
	if !ts.After(before) {
		t.Errorf("expected timestamp to be updated to a later time; got %v (before=%v)", ts, before)
	}
}

// TestBoundedMap_CleanupIfNeeded_RemovesExpiredEntries verifies that the periodic
// write-path cleanup (cleanupIfNeeded) actually removes expired entries from the
// underlying data map, not just hides them from Get().
func TestBoundedMap_CleanupIfNeeded_RemovesExpiredEntries(t *testing.T) {
	ttl := 50 * time.Millisecond
	cleanupInterval := 10 * time.Millisecond
	m := NewBoundedMap(100, ttl, cleanupInterval)

	key := "cleanup-key"
	m.SetForTest(key, time.Now().Add(-100*time.Millisecond)) // already expired

	// Force lastCleanup to be old so cleanupIfNeeded triggers.
	m.lastCleanup = time.Now().Add(-cleanupInterval - time.Millisecond)

	// Trigger cleanup via a write.
	m.Set("trigger-key")

	// The expired key should have been removed from the underlying map.
	m.mu.RLock()
	_, stillPresent := m.data[key]
	m.mu.RUnlock()
	if stillPresent {
		t.Error("cleanupIfNeeded did not remove the expired entry from the underlying data map")
	}
}

// TestBoundedMap_MaxSize_EvictsOldestOnCapacity verifies that when the map is at
// capacity, inserting a new entry evicts the oldest one.
func TestBoundedMap_MaxSize_EvictsOldestOnCapacity(t *testing.T) {
	m := NewBoundedMap(2, 0, 0) // max 2 entries, no TTL

	oldest := time.Now().Add(-10 * time.Second)
	m.SetForTest("old-key", oldest)

	newer := time.Now().Add(-5 * time.Second)
	m.SetForTest("new-key", newer)

	// Adding a third entry should evict old-key (the oldest).
	m.Set("third-key")

	if m.Len() > 2 {
		t.Errorf("map size %d exceeds maxSize 2", m.Len())
	}
	_, oldStillPresent := m.Get("old-key")
	if oldStillPresent {
		t.Error("expected oldest entry to be evicted when map is at capacity")
	}
}

// TestBoundedMap_Delete_RemovesEntry verifies Delete() removes the entry.
func TestBoundedMap_Delete_RemovesEntry(t *testing.T) {
	m := NewBoundedMap(100, 10*time.Second, 0)
	m.Set("del-key")

	m.Delete("del-key")

	_, exists := m.Get("del-key")
	if exists {
		t.Error("expected entry to be absent after Delete()")
	}
}

// TestBoundedMap_Clear_EmptiesMap verifies Clear() removes all entries.
func TestBoundedMap_Clear_EmptiesMap(t *testing.T) {
	m := NewBoundedMap(100, 10*time.Second, 0)
	m.Set("a")
	m.Set("b")
	m.Set("c")

	m.Clear()

	if m.Len() != 0 {
		t.Errorf("expected 0 entries after Clear(), got %d", m.Len())
	}
}

// TestBoundedMap_Len_ReflectsActualCount verifies Len() returns the correct count.
func TestBoundedMap_Len_ReflectsActualCount(t *testing.T) {
	m := NewBoundedMap(100, 0, 0)

	if m.Len() != 0 {
		t.Errorf("expected 0 before any inserts, got %d", m.Len())
	}
	m.Set("a")
	m.Set("b")
	if m.Len() != 2 {
		t.Errorf("expected 2 after two inserts, got %d", m.Len())
	}
	m.Delete("a")
	if m.Len() != 1 {
		t.Errorf("expected 1 after delete, got %d", m.Len())
	}
}
