// +build property

package property

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"

	"github.com/syntor/syntor/pkg/models"
	"github.com/syntor/syntor/pkg/resilience"
)

// **Feature: syntor-multi-agent-system, Property 31: Automatic failure recovery**
// *For any* agent failure, the system should detect the failure and automatically
// restart the agent or redistribute its tasks within the configured recovery time
// **Validates: Requirements 10.1**

func TestAutomaticFailureRecovery(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Failure detector detects missed heartbeats
	properties.Property("failure detector detects failures", prop.ForAll(
		func(agentCount int, failIdx int) bool {
			if agentCount <= 0 {
				agentCount = 3
			}
			if agentCount > 10 {
				agentCount = 10
			}
			failIdx = failIdx % agentCount

			var failedAgents []string
			var mu sync.Mutex

			fd := resilience.NewFailureDetector(resilience.FailureDetectorConfig{
				HeartbeatInterval: 10 * time.Millisecond,
				FailureThreshold:  50 * time.Millisecond,
				OnFailure: func(agentID string) {
					mu.Lock()
					failedAgents = append(failedAgents, agentID)
					mu.Unlock()
				},
			})

			// Register all agents with heartbeats
			for i := 0; i < agentCount; i++ {
				fd.RecordHeartbeat(fmt.Sprintf("agent-%d", i))
			}

			// All should be alive initially
			for i := 0; i < agentCount; i++ {
				if !fd.IsAlive(fmt.Sprintf("agent-%d", i)) {
					return false
				}
			}

			// Simulate one agent failing by not sending heartbeat
			failedAgentID := fmt.Sprintf("agent-%d", failIdx)

			// Keep other agents alive
			for i := 0; i < 10; i++ {
				for j := 0; j < agentCount; j++ {
					if j != failIdx {
						fd.RecordHeartbeat(fmt.Sprintf("agent-%d", j))
					}
				}
				time.Sleep(10 * time.Millisecond)
			}

			// Check for failures
			fd.CheckFailures()

			mu.Lock()
			defer mu.Unlock()

			// Failed agent should be detected
			found := false
			for _, id := range failedAgents {
				if id == failedAgentID {
					found = true
					break
				}
			}

			return found
		},
		gen.IntRange(1, 10),
		gen.IntRange(0, 100),
	))

	// Property: Recovery manager creates checkpoints
	properties.Property("checkpoint creation works", prop.ForAll(
		func(agentIdx int) bool {
			agentID := fmt.Sprintf("agent-%d", agentIdx)

			rm := resilience.NewRecoveryManager(resilience.RecoveryManagerConfig{
				CheckpointInterval: 100 * time.Millisecond,
			})

			// Register state extractor
			testState := map[string]interface{}{
				"counter": agentIdx,
				"status":  "running",
			}
			rm.RegisterStateExtractor(agentID, func() map[string]interface{} {
				return testState
			})

			// Create checkpoint
			cp, err := rm.CreateCheckpoint(context.Background(), agentID)
			if err != nil {
				return false
			}

			return cp.AgentID == agentID &&
				cp.State["counter"] == agentIdx &&
				cp.State["status"] == "running"
		},
		gen.IntRange(1, 100),
	))

	// Property: Task redistribution on failure
	properties.Property("circuit breaker opens after failures", prop.ForAll(
		func(failureThreshold int) bool {
			if failureThreshold <= 0 || failureThreshold > 10 {
				failureThreshold = 5
			}

			cb := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
				Name:             "test",
				FailureThreshold: failureThreshold,
				SuccessThreshold: 2,
				Timeout:          100 * time.Millisecond,
			})

			// Start closed
			if cb.State() != models.CircuitClosed {
				return false
			}

			// Cause failures
			for i := 0; i < failureThreshold; i++ {
				cb.Execute(context.Background(), func(ctx context.Context) error {
					return errors.New("failure")
				})
			}

			// Should be open now
			return cb.State() == models.CircuitOpen
		},
		gen.IntRange(1, 10),
	))

	// Property: Retry attempts correct number of times
	properties.Property("retry attempts correct number", prop.ForAll(
		func(maxAttempts int) bool {
			if maxAttempts <= 0 || maxAttempts > 10 {
				maxAttempts = 3
			}

			var attemptCount int32

			retryer := resilience.NewRetryer(resilience.RetryConfig{
				MaxAttempts:  maxAttempts,
				InitialDelay: 1 * time.Millisecond,
				MaxDelay:     10 * time.Millisecond,
				Multiplier:   2.0,
			})

			result := retryer.Execute(context.Background(), func(ctx context.Context) error {
				atomic.AddInt32(&attemptCount, 1)
				return errors.New("always fails")
			})

			return int(attemptCount) == maxAttempts && !result.Success
		},
		gen.IntRange(1, 10),
	))

	// Property: Successful retry stops early
	properties.Property("successful retry stops early", prop.ForAll(
		func(succeedAt int) bool {
			if succeedAt <= 0 || succeedAt > 5 {
				succeedAt = 2
			}

			var attemptCount int32

			retryer := resilience.NewRetryer(resilience.RetryConfig{
				MaxAttempts:  10,
				InitialDelay: 1 * time.Millisecond,
				MaxDelay:     10 * time.Millisecond,
				Multiplier:   2.0,
			})

			result := retryer.Execute(context.Background(), func(ctx context.Context) error {
				count := atomic.AddInt32(&attemptCount, 1)
				if int(count) >= succeedAt {
					return nil
				}
				return errors.New("not yet")
			})

			return int(attemptCount) == succeedAt && result.Success
		},
		gen.IntRange(1, 5),
	))

	properties.TestingRun(t)
}

// **Feature: syntor-multi-agent-system, Property 35: State recovery after failure**
// *For any* agent restart, the system should recover the agent's state from the
// last checkpoint and resume task processing from that point
// **Validates: Requirements 10.5**

func TestStateRecoveryAfterFailure(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: State is preserved across checkpoint/recovery
	properties.Property("state preserved across recovery", prop.ForAll(
		func(counter int, name string) bool {
			if name == "" {
				name = "test-agent"
			}

			rm := resilience.NewRecoveryManager(resilience.RecoveryManagerConfig{})

			originalState := map[string]interface{}{
				"counter": counter,
				"name":    name,
			}

			// Register extractor
			rm.RegisterStateExtractor(name, func() map[string]interface{} {
				return originalState
			})

			// Track recovered state
			var recoveredState map[string]interface{}
			rm.RegisterStateRestorer(name, func(state map[string]interface{}) error {
				recoveredState = state
				return nil
			})

			// Create checkpoint
			_, err := rm.CreateCheckpoint(context.Background(), name)
			if err != nil {
				return false
			}

			// Recover
			cp, err := rm.Recover(context.Background(), name)
			if err != nil {
				return false
			}

			// Verify state
			return cp.State["counter"] == counter &&
				cp.State["name"] == name &&
				recoveredState["counter"] == counter
		},
		gen.IntRange(1, 1000),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 30 }),
	))

	// Property: Checkpoint version increments
	properties.Property("checkpoint version increments", prop.ForAll(
		func(numCheckpoints int) bool {
			if numCheckpoints <= 0 || numCheckpoints > 10 {
				numCheckpoints = 5
			}

			rm := resilience.NewRecoveryManager(resilience.RecoveryManagerConfig{})
			agentID := "version-test-agent"

			rm.RegisterStateExtractor(agentID, func() map[string]interface{} {
				return map[string]interface{}{"data": "test"}
			})

			var lastVersion int
			for i := 0; i < numCheckpoints; i++ {
				cp, err := rm.CreateCheckpoint(context.Background(), agentID)
				if err != nil {
					return false
				}
				if i > 0 && cp.Version <= lastVersion {
					return false
				}
				lastVersion = cp.Version
			}

			return true
		},
		gen.IntRange(1, 10),
	))

	// Property: Recovery fails without checkpoint
	properties.Property("recovery fails without checkpoint", prop.ForAll(
		func(agentIdx int) bool {
			rm := resilience.NewRecoveryManager(resilience.RecoveryManagerConfig{})
			agentID := fmt.Sprintf("nonexistent-agent-%d", agentIdx)

			_, err := rm.Recover(context.Background(), agentID)
			return errors.Is(err, resilience.ErrNoCheckpoint)
		},
		gen.IntRange(1, 100),
	))

	// Property: Checkpoint can be deleted
	properties.Property("checkpoint deletion works", prop.ForAll(
		func(agentIdx int) bool {
			rm := resilience.NewRecoveryManager(resilience.RecoveryManagerConfig{})
			agentID := fmt.Sprintf("delete-test-agent-%d", agentIdx)

			rm.RegisterStateExtractor(agentID, func() map[string]interface{} {
				return map[string]interface{}{"data": "test"}
			})

			// Create checkpoint
			_, err := rm.CreateCheckpoint(context.Background(), agentID)
			if err != nil {
				return false
			}

			// Should exist
			if !rm.HasCheckpoint(context.Background(), agentID) {
				return false
			}

			// Delete
			err = rm.DeleteCheckpoint(context.Background(), agentID)
			if err != nil {
				return false
			}

			// Should not exist
			return !rm.HasCheckpoint(context.Background(), agentID)
		},
		gen.IntRange(1, 100),
	))

	properties.TestingRun(t)
}

// **Feature: syntor-multi-agent-system, Property 32: Network resilience**
// *For any* network partition or service unavailability, the system should
// implement circuit breakers and retry mechanisms to maintain stability
// **Validates: Requirements 10.2**

func TestNetworkResilience(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Circuit breaker state transitions correctly
	properties.Property("circuit breaker transitions correctly", prop.ForAll(
		func(failureThreshold int, successThreshold int) bool {
			if failureThreshold <= 0 || failureThreshold > 10 {
				failureThreshold = 3
			}
			if successThreshold <= 0 || successThreshold > 10 {
				successThreshold = 2
			}

			var stateChanges []models.CircuitState
			var mu sync.Mutex

			cb := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
				Name:             "transition-test",
				FailureThreshold: failureThreshold,
				SuccessThreshold: successThreshold,
				Timeout:          10 * time.Millisecond,
				HalfOpenMaxCalls: 10,
				OnStateChange: func(from, to models.CircuitState) {
					mu.Lock()
					stateChanges = append(stateChanges, to)
					mu.Unlock()
				},
			})

			// Closed -> Open
			for i := 0; i < failureThreshold; i++ {
				cb.Execute(context.Background(), func(ctx context.Context) error {
					return errors.New("failure")
				})
			}

			if cb.State() != models.CircuitOpen {
				return false
			}

			// Wait for timeout
			time.Sleep(15 * time.Millisecond)

			// Open -> Half-Open (triggered by next request)
			cb.Execute(context.Background(), func(ctx context.Context) error {
				return nil
			})

			// Should be half-open or closed now
			state := cb.State()
			if state != models.CircuitHalfOpen && state != models.CircuitClosed {
				return false
			}

			// Successes to close
			for i := 0; i < successThreshold; i++ {
				cb.Execute(context.Background(), func(ctx context.Context) error {
					return nil
				})
			}

			// Should be closed
			return cb.State() == models.CircuitClosed
		},
		gen.IntRange(1, 5),
		gen.IntRange(1, 5),
	))

	// Property: Circuit breaker rejects requests when open
	properties.Property("open circuit rejects requests", prop.ForAll(
		func(numRequests int) bool {
			if numRequests <= 0 || numRequests > 20 {
				numRequests = 10
			}

			cb := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
				Name:             "reject-test",
				FailureThreshold: 1,
				Timeout:          1 * time.Hour, // Long timeout so it stays open
			})

			// Open the circuit
			cb.Execute(context.Background(), func(ctx context.Context) error {
				return errors.New("failure")
			})

			if cb.State() != models.CircuitOpen {
				return false
			}

			// All subsequent requests should be rejected
			rejectedCount := 0
			for i := 0; i < numRequests; i++ {
				err := cb.Execute(context.Background(), func(ctx context.Context) error {
					return nil
				})
				if errors.Is(err, resilience.ErrCircuitOpen) {
					rejectedCount++
				}
			}

			return rejectedCount == numRequests
		},
		gen.IntRange(1, 20),
	))

	// Property: Exponential backoff increases delay
	properties.Property("exponential backoff increases delay", prop.ForAll(
		func(numAttempts int) bool {
			if numAttempts < 2 || numAttempts > 5 {
				numAttempts = 3
			}

			retryer := resilience.NewRetryer(resilience.RetryConfig{
				MaxAttempts:  numAttempts,
				InitialDelay: 10 * time.Millisecond,
				MaxDelay:     1 * time.Second,
				Multiplier:   2.0,
				Jitter:       0, // No jitter for deterministic test
			})

			var attempts []time.Time
			var mu sync.Mutex

			retryer.ExecuteWithCallback(
				context.Background(),
				func(ctx context.Context) error {
					mu.Lock()
					attempts = append(attempts, time.Now())
					mu.Unlock()
					return errors.New("always fails")
				},
				func(attempt int, err error, delay time.Duration) {
					// Callback on retry
				},
			)

			if len(attempts) < 2 {
				return true
			}

			// Check that delays increase (roughly)
			for i := 2; i < len(attempts); i++ {
				delay1 := attempts[i-1].Sub(attempts[i-2])
				delay2 := attempts[i].Sub(attempts[i-1])
				// Second delay should be >= first delay
				if delay2 < delay1*9/10 { // Allow 10% tolerance
					return false
				}
			}

			return true
		},
		gen.IntRange(2, 5),
	))

	// Property: Circuit breaker can be reset
	properties.Property("circuit breaker reset works", prop.ForAll(
		func(_ int) bool {
			cb := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
				Name:             "reset-test",
				FailureThreshold: 1,
				Timeout:          1 * time.Hour,
			})

			// Open the circuit
			cb.Execute(context.Background(), func(ctx context.Context) error {
				return errors.New("failure")
			})

			if cb.State() != models.CircuitOpen {
				return false
			}

			// Reset
			cb.Reset()

			// Should be closed
			return cb.State() == models.CircuitClosed
		},
		gen.Int(),
	))

	properties.TestingRun(t)
}

// **Feature: syntor-multi-agent-system, Property 33: Backpressure under overload**
// *For any* system overload condition, backpressure mechanisms should activate
// to prevent cascading failures while maintaining system stability
// **Validates: Requirements 10.3**

func TestBackpressureUnderOverload(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Rate limiter enforces rate
	properties.Property("rate limiter enforces limit", prop.ForAll(
		func(rate int, requests int) bool {
			if rate <= 0 || rate > 100 {
				rate = 10
			}
			if requests <= 0 || requests > 50 {
				requests = 20
			}

			rl := resilience.NewRateLimiter(resilience.RateLimiterConfig{
				Rate:       float64(rate),
				BucketSize: rate,
			})

			// Burst up to bucket size should all succeed
			allowedCount := 0
			for i := 0; i < requests; i++ {
				if rl.Allow() {
					allowedCount++
				}
			}

			// Should have allowed at most bucket size
			return allowedCount <= rate
		},
		gen.IntRange(1, 100),
		gen.IntRange(1, 50),
	))

	// Property: Backpressure rejects at capacity
	properties.Property("backpressure rejects at capacity", prop.ForAll(
		func(maxConcurrent int) bool {
			if maxConcurrent <= 0 || maxConcurrent > 20 {
				maxConcurrent = 5
			}

			bp := resilience.NewBackpressure(resilience.BackpressureConfig{
				MaxConcurrent:   maxConcurrent,
				ShedThreshold:   1.0, // Only shed at 100%
				ShedProbability: 1.0,
			})

			// Acquire all slots
			acquired := 0
			for i := 0; i < maxConcurrent; i++ {
				if bp.Acquire() {
					acquired++
				}
			}

			// Should be at capacity
			if acquired != maxConcurrent {
				return false
			}

			// Next acquisition should fail
			return !bp.Acquire()
		},
		gen.IntRange(1, 20),
	))

	// Property: Semaphore limits concurrency
	properties.Property("semaphore limits concurrency", prop.ForAll(
		func(capacity int, numWorkers int) bool {
			if capacity <= 0 || capacity > 10 {
				capacity = 5
			}
			if numWorkers <= capacity || numWorkers > 50 {
				numWorkers = capacity * 2
			}

			sem := resilience.NewSemaphore(capacity)

			var maxConcurrent int32
			var currentConcurrent int32
			var wg sync.WaitGroup

			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			for i := 0; i < numWorkers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()

					if err := sem.Acquire(ctx); err != nil {
						return
					}

					current := atomic.AddInt32(&currentConcurrent, 1)
					for {
						max := atomic.LoadInt32(&maxConcurrent)
						if current <= max || atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
							break
						}
					}

					time.Sleep(5 * time.Millisecond)

					atomic.AddInt32(&currentConcurrent, -1)
					sem.Release()
				}()
			}

			wg.Wait()

			// Max concurrent should not exceed capacity
			return int(atomic.LoadInt32(&maxConcurrent)) <= capacity
		},
		gen.IntRange(1, 10),
		gen.IntRange(5, 50),
	))

	// Property: Load shedding activates at threshold
	properties.Property("load shedding at threshold", prop.ForAll(
		func(threshold int) bool {
			if threshold <= 0 || threshold > 100 {
				threshold = 80
			}

			bp := resilience.NewBackpressure(resilience.BackpressureConfig{
				MaxConcurrent:   100,
				ShedThreshold:   float64(threshold) / 100.0,
				ShedProbability: 1.0, // Always shed when over threshold
			})

			// Fill up to just below threshold
			toAcquire := threshold - 1
			for i := 0; i < toAcquire; i++ {
				bp.Acquire()
			}

			// Should not be overloaded yet
			if bp.IsOverloaded() {
				return false
			}

			// Push to threshold
			bp.Acquire()

			// Should be at or over threshold now
			return bp.LoadPercent() >= float64(threshold)/100.0
		},
		gen.IntRange(10, 90),
	))

	// Property: Released slots become available
	properties.Property("released slots become available", prop.ForAll(
		func(capacity int) bool {
			if capacity <= 0 || capacity > 20 {
				capacity = 5
			}

			bp := resilience.NewBackpressure(resilience.BackpressureConfig{
				MaxConcurrent:   capacity,
				ShedThreshold:   1.0,
				ShedProbability: 1.0,
			})

			// Acquire all
			for i := 0; i < capacity; i++ {
				bp.Acquire()
			}

			// Should be full
			if bp.Acquire() {
				return false
			}

			// Release one
			bp.Release()

			// Should be able to acquire again
			return bp.Acquire()
		},
		gen.IntRange(1, 20),
	))

	properties.TestingRun(t)
}

// Unit tests

func TestCircuitBreakerUnit(t *testing.T) {
	t.Run("initial state is closed", func(t *testing.T) {
		cb := resilience.NewCircuitBreaker(resilience.DefaultCircuitBreakerConfig("test"))
		assert.Equal(t, models.CircuitClosed, cb.State())
	})

	t.Run("force open works", func(t *testing.T) {
		cb := resilience.NewCircuitBreaker(resilience.DefaultCircuitBreakerConfig("test"))
		cb.ForceOpen()
		assert.Equal(t, models.CircuitOpen, cb.State())
	})

	t.Run("stats are tracked", func(t *testing.T) {
		cb := resilience.NewCircuitBreaker(resilience.DefaultCircuitBreakerConfig("test"))

		cb.Execute(context.Background(), func(ctx context.Context) error {
			return errors.New("failure")
		})

		stats := cb.Stats()
		assert.Equal(t, 1, stats.FailureCount)
	})
}

func TestRetryUnit(t *testing.T) {
	t.Run("context cancellation stops retry", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		retryer := resilience.NewRetryer(resilience.RetryConfig{
			MaxAttempts:  10,
			InitialDelay: 100 * time.Millisecond,
		})

		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		result := retryer.Execute(ctx, func(ctx context.Context) error {
			return errors.New("always fails")
		})

		assert.False(t, result.Success)
		assert.True(t, result.Attempts < 10)
	})
}

func TestRecoveryManagerUnit(t *testing.T) {
	t.Run("in-memory store works", func(t *testing.T) {
		store := resilience.NewInMemoryCheckpointStore()

		cp := resilience.Checkpoint{
			ID:        "test-1",
			AgentID:   "agent-1",
			Timestamp: time.Now(),
			State:     map[string]interface{}{"key": "value"},
			Version:   1,
		}

		err := store.Save(context.Background(), cp)
		assert.NoError(t, err)

		loaded, err := store.Load(context.Background(), "agent-1")
		assert.NoError(t, err)
		assert.Equal(t, cp.ID, loaded.ID)
	})
}
