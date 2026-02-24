package readiness_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lenaxia/k8s-mendabot/internal/readiness"
)

// countingChecker counts how many times Check is called and returns a fixed error.
type countingChecker struct {
	name  string
	err   error
	calls atomic.Int32
}

func (c *countingChecker) Name() string { return c.name }
func (c *countingChecker) Check(_ context.Context) error {
	c.calls.Add(1)
	return c.err
}

// --- NopChecker ---

func TestNopChecker_AlwaysPasses(t *testing.T) {
	nop := readiness.NewNopChecker("test-nop")
	if nop.Name() != "test-nop" {
		t.Errorf("Name() = %q, want %q", nop.Name(), "test-nop")
	}
	if err := nop.Check(context.Background()); err != nil {
		t.Errorf("NopChecker.Check() returned unexpected error: %v", err)
	}
}

// --- All ---

func TestAll_PassesWhenAllPass(t *testing.T) {
	a := &countingChecker{name: "a", err: nil}
	b := &countingChecker{name: "b", err: nil}
	combined := readiness.All(a, b)

	if err := combined.Check(context.Background()); err != nil {
		t.Errorf("All.Check() returned unexpected error: %v", err)
	}
	if a.calls.Load() != 1 {
		t.Errorf("checker a called %d times, want 1", a.calls.Load())
	}
	if b.calls.Load() != 1 {
		t.Errorf("checker b called %d times, want 1", b.calls.Load())
	}
}

func TestAll_FailsWhenOneCheckerFails(t *testing.T) {
	errA := errors.New("a failed")
	a := &countingChecker{name: "a", err: errA}
	b := &countingChecker{name: "b", err: nil}
	combined := readiness.All(a, b)

	err := combined.Check(context.Background())
	if err == nil {
		t.Fatal("All.Check() should have returned an error")
	}
	if !errors.Is(err, errA) {
		t.Errorf("expected error to contain errA, got: %v", err)
	}
}

func TestAll_JoinsAllErrors(t *testing.T) {
	errA := errors.New("a failed")
	errB := errors.New("b failed")
	a := &countingChecker{name: "a", err: errA}
	b := &countingChecker{name: "b", err: errB}
	combined := readiness.All(a, b)

	err := combined.Check(context.Background())
	if err == nil {
		t.Fatal("All.Check() should have returned an error")
	}
	if !errors.Is(err, errA) {
		t.Errorf("expected joined error to contain errA, got: %v", err)
	}
	if !errors.Is(err, errB) {
		t.Errorf("expected joined error to contain errB, got: %v", err)
	}
}

func TestAll_RunsAllCheckersEvenOnFailure(t *testing.T) {
	a := &countingChecker{name: "a", err: errors.New("a failed")}
	b := &countingChecker{name: "b", err: nil}
	readiness.All(a, b).Check(context.Background()) //nolint:errcheck

	if a.calls.Load() != 1 {
		t.Errorf("checker a called %d times, want 1", a.calls.Load())
	}
	if b.calls.Load() != 1 {
		t.Errorf("checker b called %d times, want 1", b.calls.Load())
	}
}

// --- CachedChecker ---

func TestCachedChecker_CachesResult(t *testing.T) {
	inner := &countingChecker{name: "inner", err: nil}
	cached := readiness.NewCachedChecker(inner, 10*time.Second)

	for i := 0; i < 5; i++ {
		if err := cached.Check(context.Background()); err != nil {
			t.Fatalf("unexpected error on call %d: %v", i, err)
		}
	}
	if inner.calls.Load() != 1 {
		t.Errorf("inner checker called %d times, want 1 (result should be cached)", inner.calls.Load())
	}
}

func TestCachedChecker_RefreshesAfterTTL(t *testing.T) {
	inner := &countingChecker{name: "inner", err: nil}
	cached := readiness.NewCachedChecker(inner, 10*time.Millisecond)

	cached.Check(context.Background()) //nolint:errcheck
	time.Sleep(20 * time.Millisecond)
	cached.Check(context.Background()) //nolint:errcheck

	if inner.calls.Load() != 2 {
		t.Errorf("inner checker called %d times after TTL expiry, want 2", inner.calls.Load())
	}
}

func TestCachedChecker_ZeroTTLDisablesCache(t *testing.T) {
	inner := &countingChecker{name: "inner", err: nil}
	cached := readiness.NewCachedChecker(inner, 0)

	for i := 0; i < 3; i++ {
		cached.Check(context.Background()) //nolint:errcheck
	}
	if inner.calls.Load() != 3 {
		t.Errorf("inner checker called %d times with zero TTL, want 3", inner.calls.Load())
	}
}

func TestCachedChecker_CachesErrors(t *testing.T) {
	expectedErr := errors.New("probe failed")
	inner := &countingChecker{name: "inner", err: expectedErr}
	cached := readiness.NewCachedChecker(inner, 10*time.Second)

	for i := 0; i < 3; i++ {
		err := cached.Check(context.Background())
		if !errors.Is(err, expectedErr) {
			t.Errorf("call %d: expected %v, got %v", i, expectedErr, err)
		}
	}
	if inner.calls.Load() != 1 {
		t.Errorf("inner checker called %d times, want 1 (error should be cached)", inner.calls.Load())
	}
}

func TestCachedChecker_Name(t *testing.T) {
	inner := &countingChecker{name: "my-checker"}
	cached := readiness.NewCachedChecker(inner, time.Minute)
	if cached.Name() != "my-checker" {
		t.Errorf("Name() = %q, want %q", cached.Name(), "my-checker")
	}
}
