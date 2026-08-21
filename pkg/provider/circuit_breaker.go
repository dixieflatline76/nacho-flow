package provider

import (
	"sync"
	"sync/atomic"
	"time"
)

// CircuitState represents the operational state of a provider circuit breaker.
type CircuitState int32

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

const (
	DefaultFailureThreshold = 2
	DefaultCooldownDuration = 20 * time.Second
)

// CircuitBreaker manages provider health transitions and fast-fail bypasses.
type CircuitBreaker struct {
	state            atomic.Int32
	failures         atomic.Int32
	lastFailureUnix  atomic.Int64
	failureThreshold int32
	cooldown         time.Duration
	mu               sync.Mutex
}

// NewCircuitBreaker initializes a circuit breaker with threshold and cooldown.
func NewCircuitBreaker(failureThreshold int, cooldown time.Duration) *CircuitBreaker {
	if failureThreshold <= 0 {
		failureThreshold = DefaultFailureThreshold
	}
	if cooldown <= 0 {
		cooldown = DefaultCooldownDuration
	}
	cb := &CircuitBreaker{
		failureThreshold: int32(failureThreshold),
		cooldown:         cooldown,
	}
	cb.state.Store(int32(StateClosed))
	return cb
}

// State returns the current CircuitState.
func (cb *CircuitBreaker) State() CircuitState {
	return CircuitState(cb.state.Load())
}

// AllowRequest returns true if requests to this provider are permitted.
func (cb *CircuitBreaker) AllowRequest() bool {
	st := CircuitState(cb.state.Load())
	if st == StateClosed {
		return true
	}
	if st == StateHalfOpen {
		return true
	}

	// StateOpen: check if cooldown has elapsed
	lastFailNano := cb.lastFailureUnix.Load()
	if time.Since(time.Unix(0, lastFailNano)) >= cb.cooldown {
		cb.mu.Lock()
		defer cb.mu.Unlock()
		if CircuitState(cb.state.Load()) == StateOpen {
			cb.state.Store(int32(StateHalfOpen))
			return true
		}
		return CircuitState(cb.state.Load()) != StateOpen
	}
	return false
}

// RecordSuccess records a successful provider request, resetting the breaker to closed state.
func (cb *CircuitBreaker) RecordSuccess() {
	if CircuitState(cb.state.Load()) != StateClosed {
		cb.mu.Lock()
		defer cb.mu.Unlock()
		cb.state.Store(int32(StateClosed))
		cb.failures.Store(0)
		return
	}
	cb.failures.Store(0)
}

// RecordFailure records a failed provider request, tripping the circuit if threshold is reached.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	fails := cb.failures.Add(1)
	cb.lastFailureUnix.Store(time.Now().UnixNano())

	if fails >= cb.failureThreshold || CircuitState(cb.state.Load()) == StateHalfOpen {
		cb.state.Store(int32(StateOpen))
	}
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state.Store(int32(StateClosed))
	cb.failures.Store(0)
	cb.lastFailureUnix.Store(0)
}
