// Package circuitbreaker implements the circuit breaker pattern for fault tolerance.
// It prevents cascading failures by temporarily stopping requests to a failing service
// and allows time for recovery.
package circuitbreaker

import (
	"errors"
	"log/slog"
	"sync"
	"time"
)

var (
	// ErrCircuitOpen is returned when the circuit breaker is open and rejecting requests.
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

// State represents the current state of the circuit breaker
type State int

const (
	// StateClosed means the circuit breaker is allowing all requests through
	StateClosed State = iota
	// StateOpen means the circuit breaker is rejecting all requests
	StateOpen
	// StateHalfOpen means the circuit breaker is testing if the service has recovered
	StateHalfOpen
)

// String returns the string representation of the state
func (s State) String() string {
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

// Config contains configuration for the circuit breaker
type Config struct {
	// FailureThreshold is the number of consecutive failures before opening the circuit
	FailureThreshold int
	// SuccessThreshold is the number of consecutive successes in half-open state before closing
	SuccessThreshold int
	// Timeout is the duration to wait before transitioning from open to half-open
	Timeout time.Duration
	// HalfOpenMaxCalls is the maximum number of concurrent calls allowed in half-open state
	HalfOpenMaxCalls int
}

// DefaultConfig returns the default circuit breaker configuration
func DefaultConfig() Config {
	return Config{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
		HalfOpenMaxCalls: 1,
	}
}

// CircuitBreaker implements the circuit breaker pattern.
// It tracks failures and successes, automatically transitioning between states
// to protect downstream services from overload.
//
// CircuitBreaker is safe for concurrent use by multiple goroutines.
type CircuitBreaker struct {
	mu                sync.RWMutex
	state             State
	failures          int
	successes         int
	lastFailureTime   time.Time
	lastStateChange   time.Time
	config            Config
	halfOpenCalls     int
	halfOpenCallsLock sync.Mutex
}

// New creates a new circuit breaker with the given configuration
// Note: A FailureThreshold of 0 means the circuit breaker is disabled
func New(config Config) *CircuitBreaker {
	// FailureThreshold of 0 is valid and means disabled
	// Only apply defaults for negative values
	if config.FailureThreshold < 0 {
		config.FailureThreshold = DefaultConfig().FailureThreshold
	}
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = DefaultConfig().SuccessThreshold
	}
	if config.Timeout <= 0 {
		config.Timeout = DefaultConfig().Timeout
	}
	if config.HalfOpenMaxCalls <= 0 {
		config.HalfOpenMaxCalls = DefaultConfig().HalfOpenMaxCalls
	}

	return &CircuitBreaker{
		state:           StateClosed,
		config:          config,
		lastStateChange: time.Now(),
	}
}

// Call executes the given function if the circuit breaker allows it.
// If the circuit is open, it returns ErrCircuitOpen without calling the function.
// The result of the function call is recorded to update the circuit breaker state.
func (cb *CircuitBreaker) Call(fn func() error) error {
	if !cb.canAttempt() {
		return ErrCircuitOpen
	}

	err := fn()
	cb.recordResult(err)
	return err
}

// canAttempt determines if a call can be attempted based on the current state
func (cb *CircuitBreaker) canAttempt() bool {
	cb.mu.RLock()
	state := cb.state
	lastStateChange := cb.lastStateChange
	cb.mu.RUnlock()

	switch state {
	case StateClosed:
		return true
	case StateOpen:
		// Check if enough time has passed to transition to half-open
		if time.Since(lastStateChange) >= cb.config.Timeout {
			cb.transitionToHalfOpen()
			return cb.canAttemptHalfOpen()
		}
		return false
	case StateHalfOpen:
		return cb.canAttemptHalfOpen()
	default:
		return false
	}
}

// canAttemptHalfOpen checks if a call can be made in half-open state
func (cb *CircuitBreaker) canAttemptHalfOpen() bool {
	cb.halfOpenCallsLock.Lock()
	defer cb.halfOpenCallsLock.Unlock()

	if cb.halfOpenCalls < cb.config.HalfOpenMaxCalls {
		cb.halfOpenCalls++
		return true
	}
	return false
}

// recordResult records the result of a call and updates the circuit breaker state
func (cb *CircuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Release half-open call slot if in half-open state
	if cb.state == StateHalfOpen {
		cb.halfOpenCallsLock.Lock()
		cb.halfOpenCalls--
		cb.halfOpenCallsLock.Unlock()
	}

	if err == nil {
		cb.onSuccess()
	} else {
		cb.onFailure()
	}
}

// onSuccess handles a successful call
func (cb *CircuitBreaker) onSuccess() {
	switch cb.state {
	case StateClosed:
		// Reset failure counter on success
		cb.failures = 0
	case StateHalfOpen:
		cb.successes++
		if cb.successes >= cb.config.SuccessThreshold {
			cb.transitionToClosed()
		}
	}
}

// onFailure handles a failed call
func (cb *CircuitBreaker) onFailure() {
	// If circuit breaker is disabled (threshold = 0), don't track failures
	if cb.config.FailureThreshold == 0 {
		return
	}

	cb.lastFailureTime = time.Now()
	cb.failures++

	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.config.FailureThreshold {
			cb.transitionToOpen()
		}
	case StateHalfOpen:
		// Any failure in half-open state reopens the circuit
		cb.transitionToOpen()
	}
}

// transitionToOpen transitions the circuit breaker to the open state
func (cb *CircuitBreaker) transitionToOpen() {
	cb.state = StateOpen
	cb.lastStateChange = time.Now()
	cb.successes = 0
	slog.Warn("circuit breaker opened",
		"consecutive_failures", cb.failures,
		"threshold", cb.config.FailureThreshold)
}

// transitionToHalfOpen transitions the circuit breaker to the half-open state
func (cb *CircuitBreaker) transitionToHalfOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateHalfOpen
	cb.lastStateChange = time.Now()
	cb.successes = 0
	cb.failures = 0
	cb.halfOpenCalls = 0
	slog.Info("circuit breaker half-open, testing recovery")
}

// transitionToClosed transitions the circuit breaker to the closed state
func (cb *CircuitBreaker) transitionToClosed() {
	cb.state = StateClosed
	cb.lastStateChange = time.Now()
	cb.failures = 0
	cb.successes = 0
	slog.Info("circuit breaker closed, HEC recovered")
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetFailures returns the current failure count
func (cb *CircuitBreaker) GetFailures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}

// GetSuccesses returns the current success count (only relevant in half-open state)
func (cb *CircuitBreaker) GetSuccesses() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.successes
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenCalls = 0
	cb.lastStateChange = time.Now()
}
