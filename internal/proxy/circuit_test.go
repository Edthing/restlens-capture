package proxy

import (
	"testing"
	"time"
)

func TestCircuitBreaker_AllowsInitially(t *testing.T) {
	cb := NewCircuitBreaker(5, time.Second)
	if !cb.AllowCapture() {
		t.Error("expected circuit to be closed initially")
	}
}

func TestCircuitBreaker_TripsAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)

	cb.RecordFailure()
	cb.RecordFailure()
	if !cb.AllowCapture() {
		t.Error("should still allow after 2 failures (threshold=3)")
	}

	cb.RecordFailure()
	if cb.AllowCapture() {
		t.Error("should be tripped after 3 failures")
	}
	if !cb.IsOpen() {
		t.Error("expected circuit to be open")
	}
}

func TestCircuitBreaker_ResetsAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()

	if cb.AllowCapture() {
		t.Error("should be tripped")
	}

	time.Sleep(150 * time.Millisecond)

	if !cb.AllowCapture() {
		t.Error("should have reset after timeout")
	}
	if cb.IsOpen() {
		t.Error("circuit should be closed after reset")
	}
}

func TestCircuitBreaker_SuccessResetsFailures(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess()

	// Failures reset, so 1 more failure shouldn't trip
	cb.RecordFailure()
	if !cb.AllowCapture() {
		t.Error("success should have reset failure count")
	}
}

func TestCircuitBreaker_StaysOpenDuringTimeout(t *testing.T) {
	cb := NewCircuitBreaker(1, time.Second)

	cb.RecordFailure()

	for i := 0; i < 10; i++ {
		if cb.AllowCapture() {
			t.Error("should stay open during timeout period")
		}
	}
}

func TestCircuitBreaker_TrafficFlowsDuringOpen(t *testing.T) {
	// This tests the concept: when circuit is open, proxy should still forward.
	// We test that AllowCapture returns false but that's all — the proxy
	// handles the forwarding in RoundTrip.
	cb := NewCircuitBreaker(1, time.Second)
	cb.RecordFailure()

	if cb.AllowCapture() {
		t.Error("circuit should be open")
	}

	// Even though circuit is open, the proxy still works — this is
	// verified in the integration tests, not here.
}
