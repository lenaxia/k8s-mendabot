package circuitbreaker

import (
	"sync"
	"time"
)

type Gater interface {
	ShouldAllow() (allowed bool, remaining time.Duration)
}

type CircuitBreaker struct {
	cooldown    time.Duration
	mu          sync.Mutex
	lastAllowed time.Time
}

var _ Gater = (*CircuitBreaker)(nil)

func New(cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{cooldown: cooldown}
}

func (cb *CircuitBreaker) ShouldAllow() (bool, time.Duration) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if !cb.lastAllowed.IsZero() {
		elapsed := time.Since(cb.lastAllowed)
		if elapsed < cb.cooldown {
			return false, cb.cooldown - elapsed
		}
	}

	cb.lastAllowed = time.Now()
	return true, 0
}
