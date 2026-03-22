package proxy

import (
	"sync/atomic"
	"time"
)

// CircuitBreaker tracks capture failures and trips open when too many occur,
// causing the proxy to bypass capture and just forward traffic.
type CircuitBreaker struct {
	failures     atomic.Int64
	lastFailure  atomic.Int64 // unix nano
	tripped      atomic.Bool
	threshold    int64         // failures before tripping
	resetAfter   time.Duration // how long to stay open before retrying
}

func NewCircuitBreaker(threshold int64, resetAfter time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:  threshold,
		resetAfter: resetAfter,
	}
}

// AllowCapture returns true if capture should proceed.
// Returns false if the circuit is open (too many recent failures).
func (cb *CircuitBreaker) AllowCapture() bool {
	if !cb.tripped.Load() {
		return true
	}

	// Check if enough time has passed to retry
	lastFail := time.Unix(0, cb.lastFailure.Load())
	if time.Since(lastFail) > cb.resetAfter {
		cb.tripped.Store(false)
		cb.failures.Store(0)
		return true
	}

	return false
}

// RecordFailure records a capture failure and trips the circuit if threshold is exceeded.
func (cb *CircuitBreaker) RecordFailure() {
	cb.lastFailure.Store(time.Now().UnixNano())
	if cb.failures.Add(1) >= cb.threshold {
		cb.tripped.Store(true)
	}
}

// RecordSuccess resets the failure counter on successful capture.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.failures.Store(0)
}

// IsOpen returns whether the circuit breaker is currently tripped.
func (cb *CircuitBreaker) IsOpen() bool {
	return cb.tripped.Load()
}
