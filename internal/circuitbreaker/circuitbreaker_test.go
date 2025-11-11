package circuitbreaker

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := New(DefaultConfig())

	if cb.GetState() != StateClosed {
		t.Errorf("expected initial state to be Closed, got %v", cb.GetState())
	}

	if cb.GetFailures() != 0 {
		t.Errorf("expected initial failures to be 0, got %d", cb.GetFailures())
	}
}

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	config := Config{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := New(config)

	testErr := errors.New("test error")

	// First two failures should keep circuit closed
	for i := 0; i < 2; i++ {
		err := cb.Call(func() error {
			return testErr
		})
		if err != testErr {
			t.Errorf("expected test error, got %v", err)
		}
		if cb.GetState() != StateClosed {
			t.Errorf("expected state to be Closed after %d failures, got %v", i+1, cb.GetState())
		}
	}

	// Third failure should open the circuit
	err := cb.Call(func() error {
		return testErr
	})
	if err != testErr {
		t.Errorf("expected test error, got %v", err)
	}

	if cb.GetState() != StateOpen {
		t.Errorf("expected state to be Open after threshold, got %v", cb.GetState())
	}
}

func TestCircuitBreaker_OpenRejectsRequests(t *testing.T) {
	config := Config{
		FailureThreshold: 2,
		SuccessThreshold: 1,
		Timeout:          100 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := New(config)

	testErr := errors.New("test error")

	// Trigger open state
	for i := 0; i < 2; i++ {
		cb.Call(func() error {
			return testErr
		})
	}

	if cb.GetState() != StateOpen {
		t.Fatalf("expected state to be Open, got %v", cb.GetState())
	}

	// Next call should be rejected without executing the function
	callExecuted := false
	err := cb.Call(func() error {
		callExecuted = true
		return nil
	})

	if err != ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}

	if callExecuted {
		t.Error("function should not have been executed when circuit is open")
	}
}

func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
	config := Config{
		FailureThreshold: 2,
		SuccessThreshold: 1,
		Timeout:          50 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := New(config)

	testErr := errors.New("test error")

	// Trigger open state
	for i := 0; i < 2; i++ {
		cb.Call(func() error {
			return testErr
		})
	}

	if cb.GetState() != StateOpen {
		t.Fatalf("expected state to be Open, got %v", cb.GetState())
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Next call should transition to half-open
	cb.Call(func() error {
		return nil
	})

	if cb.GetState() != StateHalfOpen && cb.GetState() != StateClosed {
		t.Errorf("expected state to be HalfOpen or Closed after timeout, got %v", cb.GetState())
	}
}

func TestCircuitBreaker_HalfOpenToClosed(t *testing.T) {
	config := Config{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := New(config)

	testErr := errors.New("test error")

	// Trigger open state
	for i := 0; i < 2; i++ {
		cb.Call(func() error {
			return testErr
		})
	}

	// Wait for timeout and transition to half-open
	time.Sleep(60 * time.Millisecond)

	// First success in half-open
	err := cb.Call(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Second success should close the circuit
	err = cb.Call(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if cb.GetState() != StateClosed {
		t.Errorf("expected state to be Closed after success threshold, got %v", cb.GetState())
	}
}

func TestCircuitBreaker_HalfOpenToOpen(t *testing.T) {
	config := Config{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := New(config)

	testErr := errors.New("test error")

	// Trigger open state
	for i := 0; i < 2; i++ {
		cb.Call(func() error {
			return testErr
		})
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Failure in half-open should reopen the circuit
	err := cb.Call(func() error {
		return testErr
	})
	if err != testErr {
		t.Errorf("expected test error, got %v", err)
	}

	if cb.GetState() != StateOpen {
		t.Errorf("expected state to be Open after half-open failure, got %v", cb.GetState())
	}
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	config := Config{
		FailureThreshold: 3,
		SuccessThreshold: 1,
		Timeout:          100 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := New(config)

	testErr := errors.New("test error")

	// Two failures
	for i := 0; i < 2; i++ {
		cb.Call(func() error {
			return testErr
		})
	}

	if cb.GetFailures() != 2 {
		t.Errorf("expected 2 failures, got %d", cb.GetFailures())
	}

	// Success should reset failure count
	cb.Call(func() error {
		return nil
	})

	if cb.GetFailures() != 0 {
		t.Errorf("expected failures to be reset to 0, got %d", cb.GetFailures())
	}

	if cb.GetState() != StateClosed {
		t.Errorf("expected state to remain Closed, got %v", cb.GetState())
	}
}

func TestCircuitBreaker_HalfOpenMaxCalls(t *testing.T) {
	config := Config{
		FailureThreshold: 2,
		SuccessThreshold: 1,
		Timeout:          50 * time.Millisecond,
		HalfOpenMaxCalls: 2,
	}
	cb := New(config)

	testErr := errors.New("test error")

	// Trigger open state
	for i := 0; i < 2; i++ {
		cb.Call(func() error {
			return testErr
		})
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Use a channel to control timing of concurrent calls
	callStarted := make(chan struct{})
	callComplete := make(chan struct{})

	// Start first call and hold it
	go func() {
		cb.Call(func() error {
			callStarted <- struct{}{}
			<-callComplete
			return nil
		})
	}()

	// Wait for first call to start
	<-callStarted

	// Start second call and hold it
	go func() {
		cb.Call(func() error {
			callStarted <- struct{}{}
			<-callComplete
			return nil
		})
	}()

	// Wait for second call to start
	<-callStarted

	// Third call should be rejected
	callExecuted := false
	err := cb.Call(func() error {
		callExecuted = true
		return nil
	})

	if err != ErrCircuitOpen {
		t.Errorf("expected ErrCircuitOpen for third call, got %v", err)
	}

	if callExecuted {
		t.Error("third call should not have been executed")
	}

	// Release the held calls
	callComplete <- struct{}{}
	callComplete <- struct{}{}
}

func TestCircuitBreaker_ThreadSafety(t *testing.T) {
	config := Config{
		FailureThreshold: 100,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := New(config)

	var wg sync.WaitGroup
	iterations := 100

	// Launch many concurrent calls
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cb.Call(func() error {
				if n%2 == 0 {
					return errors.New("test error")
				}
				return nil
			})
		}(i)
	}

	wg.Wait()

	// Verify state is still valid
	state := cb.GetState()
	if state != StateClosed && state != StateOpen && state != StateHalfOpen {
		t.Errorf("invalid state after concurrent calls: %v", state)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := Config{
		FailureThreshold: 2,
		SuccessThreshold: 1,
		Timeout:          100 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := New(config)

	testErr := errors.New("test error")

	// Trigger open state
	for i := 0; i < 2; i++ {
		cb.Call(func() error {
			return testErr
		})
	}

	if cb.GetState() != StateOpen {
		t.Fatalf("expected state to be Open, got %v", cb.GetState())
	}

	// Reset should close the circuit
	cb.Reset()

	if cb.GetState() != StateClosed {
		t.Errorf("expected state to be Closed after reset, got %v", cb.GetState())
	}

	if cb.GetFailures() != 0 {
		t.Errorf("expected failures to be 0 after reset, got %d", cb.GetFailures())
	}
}

func TestCircuitBreaker_DefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.FailureThreshold != 5 {
		t.Errorf("expected default FailureThreshold to be 5, got %d", config.FailureThreshold)
	}

	if config.SuccessThreshold != 2 {
		t.Errorf("expected default SuccessThreshold to be 2, got %d", config.SuccessThreshold)
	}

	if config.Timeout != 30*time.Second {
		t.Errorf("expected default Timeout to be 30s, got %v", config.Timeout)
	}

	if config.HalfOpenMaxCalls != 1 {
		t.Errorf("expected default HalfOpenMaxCalls to be 1, got %d", config.HalfOpenMaxCalls)
	}
}

func TestCircuitBreaker_ConfigDefaults(t *testing.T) {
	// Test that New applies defaults for missing/invalid config values
	// Note: FailureThreshold of 0 is valid (means disabled), so we use -1 to test default application
	config := Config{
		FailureThreshold: -1, // Invalid, should use default
		SuccessThreshold: 0,  // Invalid, should use default
		Timeout:          0,  // Invalid, should use default
		HalfOpenMaxCalls: 0,  // Invalid, should use default
	}

	cb := New(config)

	if cb.config.FailureThreshold != DefaultConfig().FailureThreshold {
		t.Errorf("expected default FailureThreshold, got %d", cb.config.FailureThreshold)
	}

	if cb.config.SuccessThreshold != DefaultConfig().SuccessThreshold {
		t.Errorf("expected default SuccessThreshold, got %d", cb.config.SuccessThreshold)
	}

	if cb.config.Timeout != DefaultConfig().Timeout {
		t.Errorf("expected default Timeout, got %v", cb.config.Timeout)
	}

	if cb.config.HalfOpenMaxCalls != DefaultConfig().HalfOpenMaxCalls {
		t.Errorf("expected default HalfOpenMaxCalls, got %d", cb.config.HalfOpenMaxCalls)
	}
}

func TestCircuitBreaker_Disabled(t *testing.T) {
	// Test that circuit breaker is disabled when FailureThreshold is 0
	config := Config{
		FailureThreshold: 0,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := New(config)

	testErr := errors.New("test error")

	// Even with many failures, circuit should remain closed
	for i := 0; i < 100; i++ {
		err := cb.Call(func() error {
			return testErr
		})
		if err != testErr {
			t.Errorf("expected test error, got %v", err)
		}
		if cb.GetState() != StateClosed {
			t.Errorf("expected state to remain Closed when disabled, got %v", cb.GetState())
		}
	}

	// Failures should not be tracked when disabled
	if cb.GetFailures() != 0 {
		t.Errorf("expected failures to be 0 when disabled, got %d", cb.GetFailures())
	}
}

func TestCircuitBreaker_GetSuccesses(t *testing.T) {
	config := Config{
		FailureThreshold: 2,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := New(config)

	testErr := errors.New("test error")

	// Trigger open state
	for i := 0; i < 2; i++ {
		cb.Call(func() error {
			return testErr
		})
	}

	// Wait for timeout to transition to half-open
	time.Sleep(60 * time.Millisecond)

	// First success in half-open
	err := cb.Call(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Check successes count
	if cb.GetSuccesses() != 1 {
		t.Errorf("expected 1 success in half-open state, got %d", cb.GetSuccesses())
	}
}

func TestState_String(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("State.String() = %v, want %v", got, tt.expected)
		}
	}
}
