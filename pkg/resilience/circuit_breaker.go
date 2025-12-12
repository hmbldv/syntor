package resilience

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/syntor/syntor/pkg/models"
)

var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrTooManyRequests = errors.New("too many requests")
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	name          string
	state         models.CircuitState
	failureCount  int
	successCount  int
	lastFailure   time.Time
	lastStateChange time.Time

	// Configuration
	failureThreshold int
	successThreshold int
	timeout          time.Duration
	halfOpenMaxCalls int

	// Half-open state tracking
	halfOpenCalls int

	// Callbacks
	onStateChange func(from, to models.CircuitState)

	mu sync.RWMutex
}

// CircuitBreakerConfig holds configuration for a circuit breaker
type CircuitBreakerConfig struct {
	Name             string
	FailureThreshold int           // Number of failures before opening
	SuccessThreshold int           // Number of successes in half-open to close
	Timeout          time.Duration // Time to wait before transitioning to half-open
	HalfOpenMaxCalls int           // Max concurrent calls in half-open state
	OnStateChange    func(from, to models.CircuitState)
}

// DefaultCircuitBreakerConfig returns sensible defaults
func DefaultCircuitBreakerConfig(name string) CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Name:             name,
		FailureThreshold: 5,
		SuccessThreshold: 3,
		Timeout:          30 * time.Second,
		HalfOpenMaxCalls: 1,
	}
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		name:             config.Name,
		state:            models.CircuitClosed,
		failureThreshold: config.FailureThreshold,
		successThreshold: config.SuccessThreshold,
		timeout:          config.Timeout,
		halfOpenMaxCalls: config.HalfOpenMaxCalls,
		onStateChange:    config.OnStateChange,
		lastStateChange:  time.Now(),
	}
}

// Execute runs the given function with circuit breaker protection
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(context.Context) error) error {
	if err := cb.allowRequest(); err != nil {
		return err
	}

	// Execute the function
	err := fn(ctx)

	// Record the result
	cb.recordResult(err)

	return err
}

// allowRequest checks if a request should be allowed
func (cb *CircuitBreaker) allowRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case models.CircuitClosed:
		return nil

	case models.CircuitOpen:
		// Check if timeout has passed
		if time.Since(cb.lastStateChange) >= cb.timeout {
			cb.transitionTo(models.CircuitHalfOpen)
			cb.halfOpenCalls = 1
			return nil
		}
		return ErrCircuitOpen

	case models.CircuitHalfOpen:
		if cb.halfOpenCalls >= cb.halfOpenMaxCalls {
			return ErrTooManyRequests
		}
		cb.halfOpenCalls++
		return nil
	}

	return nil
}

// recordResult records the result of an operation
func (cb *CircuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}
}

// recordFailure records a failed operation
func (cb *CircuitBreaker) recordFailure() {
	cb.failureCount++
	cb.lastFailure = time.Now()

	switch cb.state {
	case models.CircuitClosed:
		if cb.failureCount >= cb.failureThreshold {
			cb.transitionTo(models.CircuitOpen)
		}

	case models.CircuitHalfOpen:
		// Any failure in half-open returns to open
		cb.transitionTo(models.CircuitOpen)
	}
}

// recordSuccess records a successful operation
func (cb *CircuitBreaker) recordSuccess() {
	switch cb.state {
	case models.CircuitClosed:
		// Reset failure count on success
		cb.failureCount = 0

	case models.CircuitHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.successThreshold {
			cb.transitionTo(models.CircuitClosed)
		}
	}
}

// transitionTo transitions to a new state
func (cb *CircuitBreaker) transitionTo(newState models.CircuitState) {
	oldState := cb.state
	cb.state = newState
	cb.lastStateChange = time.Now()

	// Reset counters on state change
	switch newState {
	case models.CircuitClosed:
		cb.failureCount = 0
		cb.successCount = 0
	case models.CircuitOpen:
		cb.successCount = 0
	case models.CircuitHalfOpen:
		cb.halfOpenCalls = 0
		cb.successCount = 0
	}

	if cb.onStateChange != nil {
		cb.onStateChange(oldState, newState)
	}
}

// State returns the current circuit breaker state
func (cb *CircuitBreaker) State() models.CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Name returns the circuit breaker name
func (cb *CircuitBreaker) Name() string {
	return cb.name
}

// FailureCount returns the current failure count
func (cb *CircuitBreaker) FailureCount() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failureCount
}

// SuccessCount returns the current success count
func (cb *CircuitBreaker) SuccessCount() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.successCount
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.transitionTo(models.CircuitClosed)
}

// ForceOpen manually opens the circuit breaker
func (cb *CircuitBreaker) ForceOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.transitionTo(models.CircuitOpen)
}

// Stats returns current circuit breaker statistics
type CircuitBreakerStats struct {
	Name            string
	State           models.CircuitState
	FailureCount    int
	SuccessCount    int
	LastFailure     time.Time
	LastStateChange time.Time
}

func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		Name:            cb.name,
		State:           cb.state,
		FailureCount:    cb.failureCount,
		SuccessCount:    cb.successCount,
		LastFailure:     cb.lastFailure,
		LastStateChange: cb.lastStateChange,
	}
}
