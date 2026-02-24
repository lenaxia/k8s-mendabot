package readiness

import (
	"context"
	"sync"
	"time"
)

// CachedChecker wraps any Checker and caches its result for a configurable TTL.
// This prevents the watcher from hammering the K8s API or LLM endpoint on
// every reconcile loop — checks are re-run at most once per TTL window.
//
// Concurrency: the mutex is NOT held while the inner Check runs. Multiple
// callers whose TTL has expired concurrently will each run the inner check
// independently (thundering-herd on TTL boundary), but this is preferable to
// serialising all callers for up to 10 seconds while a network probe is in-flight.
//
// The zero value is not usable; construct with NewCachedChecker.
type CachedChecker struct {
	inner     Checker
	ttl       time.Duration
	mu        sync.Mutex
	lastCheck time.Time
	lastErr   error
}

// NewCachedChecker returns a CachedChecker that re-runs inner at most once per ttl.
// A ttl of 0 disables caching (every call runs the inner checker directly).
func NewCachedChecker(inner Checker, ttl time.Duration) *CachedChecker {
	return &CachedChecker{inner: inner, ttl: ttl}
}

func (c *CachedChecker) Name() string { return c.inner.Name() }

// Check returns the cached result if it is still within the TTL window,
// otherwise re-runs the inner checker and stores the new result.
// The mutex is released before calling the inner checker so that concurrent
// reconcile goroutines are not serialised behind a network I/O operation.
func (c *CachedChecker) Check(ctx context.Context) error {
	// Fast path: return cached result under lock.
	c.mu.Lock()
	if c.ttl > 0 && !c.lastCheck.IsZero() && time.Since(c.lastCheck) < c.ttl {
		err := c.lastErr
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()

	// Slow path: run the inner check without holding the lock.
	// Multiple goroutines may run the inner check concurrently near TTL expiry;
	// this is acceptable — the last writer wins and results are idempotent.
	err := c.inner.Check(ctx)

	c.mu.Lock()
	c.lastErr = err
	c.lastCheck = time.Now()
	c.mu.Unlock()
	return err
}
