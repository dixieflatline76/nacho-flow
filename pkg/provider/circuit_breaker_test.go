package provider

import (
	"sync"
	"testing"
	"time"
)

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	cooldown := 50 * time.Millisecond
	cb := NewCircuitBreaker(2, cooldown)

	if cb.State() != StateClosed {
		t.Fatalf("Expected initial state StateClosed, got %v", cb.State())
	}
	if !cb.AllowRequest() {
		t.Fatalf("Expected AllowRequest to be true in closed state")
	}

	// Failure 1: Still closed
	cb.RecordFailure()
	if cb.State() != StateClosed {
		t.Errorf("Expected state StateClosed after 1 failure, got %v", cb.State())
	}
	if !cb.AllowRequest() {
		t.Errorf("Expected AllowRequest to be true after 1 failure")
	}

	// Failure 2: Trips to StateOpen
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Errorf("Expected state StateOpen after 2 failures, got %v", cb.State())
	}
	if cb.AllowRequest() {
		t.Errorf("Expected AllowRequest to be false in open state during cooldown")
	}

	// Wait for cooldown
	time.Sleep(70 * time.Millisecond)

	// AllowRequest should transition to StateHalfOpen
	if !cb.AllowRequest() {
		t.Fatalf("Expected AllowRequest to be true after cooldown elapsed")
	}
	if cb.State() != StateHalfOpen {
		t.Errorf("Expected state StateHalfOpen after cooldown probe, got %v", cb.State())
	}

	// Probe succeeds -> Closed
	cb.RecordSuccess()
	if cb.State() != StateClosed {
		t.Errorf("Expected state StateClosed after probe success, got %v", cb.State())
	}
	if !cb.AllowRequest() {
		t.Errorf("Expected AllowRequest to be true after recovery")
	}

	// Test HalfOpen failure -> immediately Open
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Fatalf("Expected open state")
	}
	time.Sleep(70 * time.Millisecond)
	if !cb.AllowRequest() {
		t.Fatalf("Expected probe allowed")
	}
	if cb.State() != StateHalfOpen {
		t.Fatalf("Expected half open state")
	}
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Errorf("Expected StateOpen immediately after probe failure in half-open state, got %v", cb.State())
	}
}

func TestCircuitBreaker_ConcurrentRaces(t *testing.T) {
	cb := NewCircuitBreaker(3, 10*time.Millisecond)
	var wg sync.WaitGroup
	workers := 100

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = cb.AllowRequest()
				if j%3 == 0 {
					cb.RecordFailure()
				} else if j%5 == 0 {
					cb.RecordSuccess()
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestCircuitBreaker_ResetAndString(t *testing.T) {
	cb := NewCircuitBreaker(0, 0)
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Errorf("Expected StateOpen")
	}
	if cb.State().String() != "open" {
		t.Errorf("Expected string 'open', got %s", cb.State().String())
	}

	cb.Reset()
	if cb.State() != StateClosed {
		t.Errorf("Expected StateClosed after Reset")
	}
	if StateHalfOpen.String() != "half-open" {
		t.Errorf("Expected 'half-open', got %s", StateHalfOpen.String())
	}
	if CircuitState(99).String() != "unknown" {
		t.Errorf("Expected 'unknown' for invalid state")
	}
}
