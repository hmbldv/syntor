package resilience

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"
)

var (
	ErrMaxRetriesExceeded = errors.New("maximum retries exceeded")
	ErrContextCanceled    = errors.New("context canceled during retry")
)

// RetryConfig holds configuration for retry behavior
type RetryConfig struct {
	MaxAttempts     int
	InitialDelay    time.Duration
	MaxDelay        time.Duration
	Multiplier      float64
	Jitter          float64 // 0-1, percentage of delay to randomize
	RetryableErrors []error
	ShouldRetry     func(error) bool
}

// DefaultRetryConfig returns sensible defaults
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
	}
}

// Retryer implements retry logic with exponential backoff
type Retryer struct {
	config RetryConfig
}

// NewRetryer creates a new retryer with the given configuration
func NewRetryer(config RetryConfig) *Retryer {
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if config.InitialDelay <= 0 {
		config.InitialDelay = 100 * time.Millisecond
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = 30 * time.Second
	}
	if config.Multiplier <= 0 {
		config.Multiplier = 2.0
	}

	return &Retryer{config: config}
}

// RetryResult contains the result of a retry operation
type RetryResult struct {
	Attempts   int
	LastError  error
	TotalDelay time.Duration
	Success    bool
}

// Execute runs the given function with retry logic
func (r *Retryer) Execute(ctx context.Context, fn func(context.Context) error) RetryResult {
	result := RetryResult{}
	var totalDelay time.Duration

	for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
		result.Attempts = attempt

		// Check context before attempting
		select {
		case <-ctx.Done():
			result.LastError = ErrContextCanceled
			result.TotalDelay = totalDelay
			return result
		default:
		}

		// Execute the function
		err := fn(ctx)
		if err == nil {
			result.Success = true
			result.TotalDelay = totalDelay
			return result
		}

		result.LastError = err

		// Check if we should retry this error
		if !r.shouldRetry(err) {
			result.TotalDelay = totalDelay
			return result
		}

		// Don't delay after the last attempt
		if attempt < r.config.MaxAttempts {
			delay := r.calculateDelay(attempt)
			totalDelay += delay

			select {
			case <-ctx.Done():
				result.LastError = ErrContextCanceled
				result.TotalDelay = totalDelay
				return result
			case <-time.After(delay):
				// Continue to next attempt
			}
		}
	}

	result.TotalDelay = totalDelay
	return result
}

// ExecuteWithCallback runs the function with retry and calls back on each attempt
func (r *Retryer) ExecuteWithCallback(
	ctx context.Context,
	fn func(context.Context) error,
	onRetry func(attempt int, err error, delay time.Duration),
) RetryResult {
	result := RetryResult{}
	var totalDelay time.Duration

	for attempt := 1; attempt <= r.config.MaxAttempts; attempt++ {
		result.Attempts = attempt

		select {
		case <-ctx.Done():
			result.LastError = ErrContextCanceled
			result.TotalDelay = totalDelay
			return result
		default:
		}

		err := fn(ctx)
		if err == nil {
			result.Success = true
			result.TotalDelay = totalDelay
			return result
		}

		result.LastError = err

		if !r.shouldRetry(err) {
			result.TotalDelay = totalDelay
			return result
		}

		if attempt < r.config.MaxAttempts {
			delay := r.calculateDelay(attempt)
			totalDelay += delay

			if onRetry != nil {
				onRetry(attempt, err, delay)
			}

			select {
			case <-ctx.Done():
				result.LastError = ErrContextCanceled
				result.TotalDelay = totalDelay
				return result
			case <-time.After(delay):
			}
		}
	}

	result.TotalDelay = totalDelay
	return result
}

// shouldRetry determines if an error should trigger a retry
func (r *Retryer) shouldRetry(err error) bool {
	if r.config.ShouldRetry != nil {
		return r.config.ShouldRetry(err)
	}

	// Check against retryable errors list
	if len(r.config.RetryableErrors) > 0 {
		for _, retryable := range r.config.RetryableErrors {
			if errors.Is(err, retryable) {
				return true
			}
		}
		return false
	}

	// By default, retry all errors
	return true
}

// calculateDelay calculates the delay for the given attempt
func (r *Retryer) calculateDelay(attempt int) time.Duration {
	// Exponential backoff: initialDelay * multiplier^(attempt-1)
	delay := float64(r.config.InitialDelay) * math.Pow(r.config.Multiplier, float64(attempt-1))

	// Add jitter
	if r.config.Jitter > 0 {
		jitterRange := delay * r.config.Jitter
		delay += (rand.Float64()*2 - 1) * jitterRange
	}

	// Cap at max delay
	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}

	return time.Duration(delay)
}

// Retry is a convenience function for simple retry operations
func Retry(ctx context.Context, maxAttempts int, fn func(context.Context) error) RetryResult {
	config := DefaultRetryConfig()
	config.MaxAttempts = maxAttempts
	return NewRetryer(config).Execute(ctx, fn)
}

// RetryWithBackoff is a convenience function with configurable backoff
func RetryWithBackoff(
	ctx context.Context,
	maxAttempts int,
	initialDelay time.Duration,
	fn func(context.Context) error,
) RetryResult {
	config := DefaultRetryConfig()
	config.MaxAttempts = maxAttempts
	config.InitialDelay = initialDelay
	return NewRetryer(config).Execute(ctx, fn)
}
