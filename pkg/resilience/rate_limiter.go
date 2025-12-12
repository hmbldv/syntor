package resilience

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
)

// RateLimiter implements a token bucket rate limiter
type RateLimiter struct {
	rate       float64 // tokens per second
	bucketSize int     // maximum burst size
	tokens     float64
	lastUpdate time.Time
	mu         sync.Mutex
}

// RateLimiterConfig holds configuration for a rate limiter
type RateLimiterConfig struct {
	Rate       float64 // requests per second
	BucketSize int     // burst capacity
}

// NewRateLimiter creates a new token bucket rate limiter
func NewRateLimiter(config RateLimiterConfig) *RateLimiter {
	return &RateLimiter{
		rate:       config.Rate,
		bucketSize: config.BucketSize,
		tokens:     float64(config.BucketSize),
		lastUpdate: time.Now(),
	}
}

// Allow checks if a request should be allowed
func (rl *RateLimiter) Allow() bool {
	return rl.AllowN(1)
}

// AllowN checks if n requests should be allowed
func (rl *RateLimiter) AllowN(n int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refill()

	if rl.tokens >= float64(n) {
		rl.tokens -= float64(n)
		return true
	}

	return false
}

// Wait blocks until a request is allowed or context is canceled
func (rl *RateLimiter) Wait(ctx context.Context) error {
	return rl.WaitN(ctx, 1)
}

// WaitN blocks until n requests are allowed or context is canceled
func (rl *RateLimiter) WaitN(ctx context.Context, n int) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if rl.AllowN(n) {
			return nil
		}

		// Calculate wait time
		waitTime := rl.waitTime(n)
		if waitTime <= 0 {
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// Try again
		}
	}
}

// refill adds tokens based on elapsed time
func (rl *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(rl.lastUpdate).Seconds()
	rl.lastUpdate = now

	rl.tokens += elapsed * rl.rate
	if rl.tokens > float64(rl.bucketSize) {
		rl.tokens = float64(rl.bucketSize)
	}
}

// waitTime calculates how long to wait for n tokens
func (rl *RateLimiter) waitTime(n int) time.Duration {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refill()

	if rl.tokens >= float64(n) {
		return 0
	}

	needed := float64(n) - rl.tokens
	return time.Duration(needed / rl.rate * float64(time.Second))
}

// Tokens returns the current number of available tokens
func (rl *RateLimiter) Tokens() float64 {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.refill()
	return rl.tokens
}

// Backpressure implements backpressure control with load shedding
type Backpressure struct {
	maxConcurrent   int
	currentLoad     int
	shedThreshold   float64 // 0-1, load percentage at which to start shedding
	shedProbability float64 // 0-1, probability of shedding when over threshold
	mu              sync.Mutex
}

// BackpressureConfig holds configuration for backpressure control
type BackpressureConfig struct {
	MaxConcurrent   int
	ShedThreshold   float64 // e.g., 0.8 = start shedding at 80% load
	ShedProbability float64 // e.g., 0.5 = 50% chance of shedding
}

// NewBackpressure creates a new backpressure controller
func NewBackpressure(config BackpressureConfig) *Backpressure {
	return &Backpressure{
		maxConcurrent:   config.MaxConcurrent,
		shedThreshold:   config.ShedThreshold,
		shedProbability: config.ShedProbability,
	}
}

// Acquire attempts to acquire a slot, returns false if load shedding
func (bp *Backpressure) Acquire() bool {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	load := float64(bp.currentLoad) / float64(bp.maxConcurrent)

	// Always reject if at capacity
	if bp.currentLoad >= bp.maxConcurrent {
		return false
	}

	// Load shedding logic
	if load >= bp.shedThreshold {
		// Probabilistic shedding
		if randFloat() < bp.shedProbability {
			return false
		}
	}

	bp.currentLoad++
	return true
}

// Release releases a slot
func (bp *Backpressure) Release() {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if bp.currentLoad > 0 {
		bp.currentLoad--
	}
}

// CurrentLoad returns the current load
func (bp *Backpressure) CurrentLoad() int {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	return bp.currentLoad
}

// LoadPercent returns the current load as a percentage
func (bp *Backpressure) LoadPercent() float64 {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	return float64(bp.currentLoad) / float64(bp.maxConcurrent)
}

// IsOverloaded returns true if load is at or above the shed threshold
func (bp *Backpressure) IsOverloaded() bool {
	return bp.LoadPercent() >= bp.shedThreshold
}

// Execute runs the function with backpressure control
func (bp *Backpressure) Execute(ctx context.Context, fn func(context.Context) error) error {
	if !bp.Acquire() {
		return ErrRateLimitExceeded
	}
	defer bp.Release()

	return fn(ctx)
}

// randFloat returns a random float between 0 and 1
func randFloat() float64 {
	return float64(time.Now().UnixNano()%1000) / 1000.0
}

// Semaphore implements a counting semaphore for concurrency control
type Semaphore struct {
	sem chan struct{}
}

// NewSemaphore creates a new semaphore with the given capacity
func NewSemaphore(capacity int) *Semaphore {
	return &Semaphore{
		sem: make(chan struct{}, capacity),
	}
}

// Acquire acquires a semaphore slot, blocking if necessary
func (s *Semaphore) Acquire(ctx context.Context) error {
	select {
	case s.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TryAcquire attempts to acquire without blocking
func (s *Semaphore) TryAcquire() bool {
	select {
	case s.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

// Release releases a semaphore slot
func (s *Semaphore) Release() {
	select {
	case <-s.sem:
	default:
		// Already empty, shouldn't happen in correct usage
	}
}

// Available returns the number of available slots
func (s *Semaphore) Available() int {
	return cap(s.sem) - len(s.sem)
}
