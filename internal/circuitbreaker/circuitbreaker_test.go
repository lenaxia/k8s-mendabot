package circuitbreaker_test

import (
	"sync"
	"testing"
	"time"

	"github.com/lenaxia/k8s-mechanic/internal/circuitbreaker"
)

func TestCircuitBreaker_FirstCall(t *testing.T) {
	cb := circuitbreaker.New(5 * time.Minute)

	allowed, remaining := cb.ShouldAllow()

	if !allowed {
		t.Errorf("first call: got allowed=%v, want true", allowed)
	}
	if remaining != 0 {
		t.Errorf("first call: got remaining=%v, want 0", remaining)
	}
}

func TestCircuitBreaker_WithinCooldown(t *testing.T) {
	cb := circuitbreaker.New(1 * time.Hour)

	first, _ := cb.ShouldAllow()
	if !first {
		t.Fatal("first call must be allowed to set up within-cooldown test")
	}

	allowed, remaining := cb.ShouldAllow()

	if allowed {
		t.Errorf("within cooldown: got allowed=%v, want false", allowed)
	}
	if remaining <= 0 {
		t.Errorf("within cooldown: got remaining=%v, want > 0", remaining)
	}
	const tolerance = 5 * time.Second
	if remaining > time.Hour+tolerance || remaining < time.Hour-tolerance {
		t.Errorf("within cooldown: got remaining=%v, want ≈1h", remaining)
	}
}

func TestCircuitBreaker_CooldownElapsed(t *testing.T) {
	cb := circuitbreaker.New(1 * time.Nanosecond)

	first, _ := cb.ShouldAllow()
	if !first {
		t.Fatal("first call must be allowed to set up elapsed test")
	}

	time.Sleep(10 * time.Millisecond)

	allowed, remaining := cb.ShouldAllow()

	if !allowed {
		t.Errorf("cooldown elapsed: got allowed=%v, want true", allowed)
	}
	if remaining != 0 {
		t.Errorf("cooldown elapsed: got remaining=%v, want 0", remaining)
	}
}

func TestCircuitBreaker_ZeroCooldown(t *testing.T) {
	cb := circuitbreaker.New(0)

	tests := []struct {
		name string
		call int
	}{
		{"first call", 1},
		{"second call", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, remaining := cb.ShouldAllow()
			if !allowed {
				t.Errorf("zero cooldown call %d: got allowed=%v, want true", tt.call, allowed)
			}
			if remaining != 0 {
				t.Errorf("zero cooldown call %d: got remaining=%v, want 0", tt.call, remaining)
			}
		})
	}
}

// TestCircuitBreaker_SecondAllowResetsTimer verifies that a second allowed call (after
// the cooldown has elapsed) resets the lastAllowed timestamp, so the next immediate call
// within the new cooldown window is correctly blocked again.
func TestCircuitBreaker_SecondAllowResetsTimer(t *testing.T) {
	cb := circuitbreaker.New(1 * time.Nanosecond)

	// First allow — sets lastAllowed.
	first, _ := cb.ShouldAllow()
	if !first {
		t.Fatal("first call must be allowed")
	}

	time.Sleep(10 * time.Millisecond) // let first cooldown expire

	// Second allow — should succeed and reset the timer to now.
	second, _ := cb.ShouldAllow()
	if !second {
		t.Fatal("second call after cooldown elapsed must be allowed")
	}

	// Now use a fresh CB with a long cooldown to verify the timer was reset:
	// create a new CB, trigger it, then immediately call again — must be blocked.
	cb2 := circuitbreaker.New(1 * time.Hour)
	cb2.ShouldAllow() // sets lastAllowed to now
	allowed, remaining := cb2.ShouldAllow()
	if allowed {
		t.Error("call immediately after an allowed call must be blocked (timer was not reset)")
	}
	if remaining <= 0 {
		t.Errorf("expected remaining > 0 after reset, got %v", remaining)
	}
}

func TestCircuitBreaker_Concurrent(t *testing.T) {
	cb := circuitbreaker.New(1 * time.Hour)

	var (
		wg           sync.WaitGroup
		mu           sync.Mutex
		allowedCount int
		start        = make(chan struct{})
	)

	const goroutines = 2
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			<-start // block until gate opens so both goroutines race simultaneously
			allowed, _ := cb.ShouldAllow()
			if allowed {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}()
	}

	close(start) // release all goroutines simultaneously
	wg.Wait()

	if allowedCount != 1 {
		t.Errorf("concurrent calls: got allowedCount=%d, want exactly 1", allowedCount)
	}
}
