package integration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/syntor/syntor/pkg/models"
	"github.com/syntor/syntor/pkg/performance"
	"github.com/syntor/syntor/pkg/resilience"
)

// TestEndToEndMessageFlow tests message flow through the system
func TestEndToEndMessageFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create message pools for efficient allocation
	pool := performance.NewMessagePool()

	// Simulate message flow
	messageCount := 1000
	receivedCount := int32(0)

	// Create a simple pipeline
	pipeline := performance.NewPipeline[*models.Message](4, 100)

	// Processing stage
	pipeline.AddStage(func(msg *models.Message) (*models.Message, error) {
		// Simulate processing
		time.Sleep(time.Microsecond * 10)
		return msg, nil
	})

	pipeline.Start(ctx)

	// Send messages
	go func() {
		for i := 0; i < messageCount; i++ {
			msg := pool.Get()
			msg.ID = fmt.Sprintf("msg-%d", i)
			msg.Type = "test"
			msg.Source = "sender"
			msg.Target = "receiver"
			pipeline.Submit(msg)
		}
	}()

	// Receive messages
	timeout := time.After(10 * time.Second)
	for {
		select {
		case <-pipeline.Results():
			atomic.AddInt32(&receivedCount, 1)
			if atomic.LoadInt32(&receivedCount) >= int32(messageCount) {
				goto done
			}
		case <-timeout:
			t.Fatalf("Timeout: only received %d/%d messages", receivedCount, messageCount)
		case <-ctx.Done():
			t.Fatal("Context cancelled")
		}
	}
done:

	pipeline.Stop()

	if receivedCount != int32(messageCount) {
		t.Errorf("Expected %d messages, received %d", messageCount, receivedCount)
	}
}

// TestConcurrentTaskExecution tests parallel task execution
func TestConcurrentTaskExecution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	taskPool := performance.NewTaskPool()
	workerCount := 8
	tasksPerWorker := 100
	totalTasks := workerCount * tasksPerWorker

	completedTasks := int32(0)
	var wg sync.WaitGroup

	// Simulate workers
	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for i := 0; i < tasksPerWorker; i++ {
				select {
				case <-ctx.Done():
					return
				default:
					task := taskPool.Get()
					task.ID = fmt.Sprintf("task-%d-%d", workerID, i)
					task.Type = "compute"

					// Simulate task execution
					time.Sleep(time.Microsecond * 100)

					atomic.AddInt32(&completedTasks, 1)
					taskPool.Put(task)
				}
			}
		}(w)
	}

	wg.Wait()

	if completedTasks != int32(totalTasks) {
		t.Errorf("Expected %d completed tasks, got %d", totalTasks, completedTasks)
	}
}

// TestCircuitBreakerIntegration tests circuit breaker under load
func TestCircuitBreakerIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cb := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
		FailureThreshold: 5,
		Timeout:          100 * time.Millisecond,
		HalfOpenMaxCalls: 2,
	})

	// Successful calls
	for i := 0; i < 10; i++ {
		err := cb.Execute(ctx, func(context.Context) error {
			return nil
		})
		if err != nil {
			t.Errorf("Unexpected error on successful call: %v", err)
		}
	}

	// Trigger failures to open circuit
	for i := 0; i < 6; i++ {
		cb.Execute(ctx, func(context.Context) error {
			return fmt.Errorf("simulated failure")
		})
	}

	// Circuit should be open
	err := cb.Execute(ctx, func(context.Context) error {
		return nil
	})
	if err != resilience.ErrCircuitOpen {
		t.Errorf("Expected circuit to be open, got: %v", err)
	}

	// Wait for reset
	time.Sleep(150 * time.Millisecond)

	// Circuit should be half-open, allow test calls
	err = cb.Execute(ctx, func(context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected successful call in half-open state: %v", err)
	}
}

// TestRetryWithBackoff tests retry mechanism
func TestRetryWithBackoff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	retryer := resilience.NewRetryer(resilience.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0.1,
	})

	attemptCount := int32(0)

	// Should succeed after 2 failures
	result := retryer.Execute(ctx, func(context.Context) error {
		attempt := atomic.AddInt32(&attemptCount, 1)
		if attempt < 3 {
			return fmt.Errorf("temporary failure")
		}
		return nil
	})

	if !result.Success {
		t.Errorf("Expected success after retries")
	}

	if result.Attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", result.Attempts)
	}
}

// TestBatchProcessorUnderLoad tests batch processing with high load
func TestBatchProcessorUnderLoad(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	processedBatches := int32(0)
	processedItems := int32(0)

	bp := performance.NewBatchProcessor(performance.BatchProcessorConfig[int]{
		BatchSize:     100,
		FlushInterval: 50 * time.Millisecond,
		Processor: func(items []int) error {
			atomic.AddInt32(&processedBatches, 1)
			atomic.AddInt32(&processedItems, int32(len(items)))
			return nil
		},
	})

	bp.Start(ctx)

	// Add items rapidly
	itemCount := 10000
	for i := 0; i < itemCount; i++ {
		bp.Add(i)
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)
	bp.Stop()

	if processedItems != int32(itemCount) {
		t.Errorf("Expected %d processed items, got %d", itemCount, processedItems)
	}

	// Batches were processed (at least one)
	if processedBatches < 1 {
		t.Errorf("Expected at least 1 batch, got %d", processedBatches)
	}
}

// TestBackpressureUnderOverload tests backpressure mechanism
func TestBackpressureUnderOverload(t *testing.T) {
	bp := resilience.NewBackpressure(resilience.BackpressureConfig{
		MaxConcurrent:   10,
		ShedThreshold:   0.8,
		ShedProbability: 0.5,
	})

	// Fill up to capacity
	acquired := 0
	for i := 0; i < 20; i++ {
		if bp.Acquire() {
			acquired++
		}
	}

	// Should not acquire more than max
	if acquired > 10 {
		t.Errorf("Acquired more than max concurrent: %d", acquired)
	}

	// Release some
	for i := 0; i < 5; i++ {
		bp.Release()
	}

	// Should be able to acquire again
	if !bp.Acquire() {
		t.Error("Should be able to acquire after release")
	}
}

// TestRateLimiterIntegration tests rate limiting
func TestRateLimiterIntegration(t *testing.T) {
	limiter := resilience.NewRateLimiter(resilience.RateLimiterConfig{
		Rate:       100, // 100 per second
		BucketSize: 10,
	})

	// Burst should be allowed
	allowedInBurst := 0
	for i := 0; i < 20; i++ {
		if limiter.Allow() {
			allowedInBurst++
		}
	}

	if allowedInBurst < 10 {
		t.Errorf("Burst should allow at least 10, got %d", allowedInBurst)
	}

	// Wait for tokens to refill
	time.Sleep(200 * time.Millisecond)

	// Should allow more requests
	if !limiter.Allow() {
		t.Error("Should allow requests after waiting")
	}
}

// TestCheckpointRecovery tests checkpoint-based recovery
func TestCheckpointRecovery(t *testing.T) {
	ctx := context.Background()

	rm := resilience.NewRecoveryManager(resilience.RecoveryManagerConfig{
		CheckpointInterval: 100 * time.Millisecond,
		MaxCheckpoints:     5,
	})

	agentID := "test-agent-1"

	// Register state extractor
	testState := map[string]interface{}{
		"counter": 42,
		"status":  "running",
	}

	rm.RegisterStateExtractor(agentID, func() map[string]interface{} {
		return testState
	})

	// Create checkpoint
	checkpoint, err := rm.CreateCheckpoint(ctx, agentID)
	if err != nil {
		t.Fatalf("Failed to create checkpoint: %v", err)
	}

	if checkpoint.AgentID != agentID {
		t.Errorf("Checkpoint agent ID mismatch")
	}

	// Register restorer and recover
	rm.RegisterStateRestorer(agentID, func(state map[string]interface{}) error {
		// State restored successfully
		return nil
	})

	recovered, err := rm.Recover(ctx, agentID)
	if err != nil {
		t.Fatalf("Failed to recover: %v", err)
	}

	// State should have been restored
	if recovered.State["counter"] != 42 {
		t.Errorf("Recovered state mismatch: %v", recovered.State)
	}
}

// TestFailureDetection tests failure detection mechanism
func TestFailureDetection(t *testing.T) {
	failedAgents := make([]string, 0)
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

	// Record heartbeats for agents
	fd.RecordHeartbeat("agent-1")
	fd.RecordHeartbeat("agent-2")

	// Both should be alive
	if !fd.IsAlive("agent-1") {
		t.Error("agent-1 should be alive")
	}
	if !fd.IsAlive("agent-2") {
		t.Error("agent-2 should be alive")
	}

	// Keep agent-1 alive, let agent-2 expire
	for i := 0; i < 6; i++ {
		fd.RecordHeartbeat("agent-1")
		time.Sleep(10 * time.Millisecond)
	}

	// Check failures
	failed := fd.CheckFailures()

	// agent-2 should have failed
	found := false
	for _, id := range failed {
		if id == "agent-2" {
			found = true
			break
		}
	}

	if !found {
		t.Error("agent-2 should be detected as failed")
	}
}

// TestAutoScalerDecisions tests auto-scaler scaling decisions
func TestAutoScalerDecisions(t *testing.T) {
	scaler := performance.NewAutoScaler(performance.AutoScalerConfig{
		HistorySize: 5,
	})

	// Register agent type
	scaler.RegisterAgentType("worker", performance.ScalingPolicy{
		MinInstances:       2,
		MaxInstances:       10,
		TargetCPU:          0.7,
		ScaleUpThreshold:   0.8,
		ScaleDownThreshold: 0.2,
		CooldownPeriod:     0, // No cooldown for testing
		ScaleUpStep:        1,
		ScaleDownStep:      1,
		TargetQueueDepth:   100,
	}, 5)

	// Record high load metrics
	for i := 0; i < 5; i++ {
		scaler.RecordMetrics("worker", performance.ScalingMetrics{
			CPUUtilization:    0.9,
			MemoryUtilization: 0.8,
			QueueDepth:        200,
		})
	}

	// Should recommend scale up
	decisions := scaler.Evaluate()

	found := false
	for _, d := range decisions {
		if d.AgentType == "worker" && d.DesiredCount > d.CurrentCount {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected scale up decision for high load")
	}
}

// TestLoadBalancerDistribution tests load balancer fairness
func TestLoadBalancerDistribution(t *testing.T) {
	lb := performance.NewLoadBalancer(performance.LoadBalancerConfig{
		Algorithm: "round-robin",
	})

	// Add instances
	for i := 0; i < 4; i++ {
		lb.AddInstance(fmt.Sprintf("instance-%d", i), 1.0)
	}

	// Distribute requests
	distribution := make(map[string]int)
	for i := 0; i < 1000; i++ {
		instance := lb.Next()
		distribution[instance]++
	}

	// Check even distribution
	expected := 250
	tolerance := 10 // Allow some deviation

	for instance, count := range distribution {
		if count < expected-tolerance || count > expected+tolerance {
			t.Errorf("Uneven distribution for %s: %d (expected ~%d)", instance, count, expected)
		}
	}
}

// TestLatencyTracking tests latency percentile calculations
func TestLatencyTracking(t *testing.T) {
	tracker := performance.NewLatencyTracker(1000)

	// Add samples with known distribution
	for i := 1; i <= 100; i++ {
		tracker.Record(time.Duration(i) * time.Millisecond)
	}

	p50 := tracker.P50()
	p95 := tracker.P95()
	p99 := tracker.P99()

	// P50 should be around 50ms
	if p50 < 45*time.Millisecond || p50 > 55*time.Millisecond {
		t.Errorf("P50 should be around 50ms, got %v", p50)
	}

	// P95 should be around 95ms
	if p95 < 90*time.Millisecond || p95 > 100*time.Millisecond {
		t.Errorf("P95 should be around 95ms, got %v", p95)
	}

	// P99 should be around 99ms
	if p99 < 95*time.Millisecond || p99 > 100*time.Millisecond {
		t.Errorf("P99 should be around 99ms, got %v", p99)
	}
}

// Benchmark tests
func BenchmarkMessagePooling(b *testing.B) {
	pool := performance.NewMessagePool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := pool.Get()
		msg.ID = "test"
		pool.Put(msg)
	}
}

func BenchmarkBatchProcessing(b *testing.B) {
	ctx := context.Background()
	processed := int64(0)

	bp := performance.NewBatchProcessor(performance.BatchProcessorConfig[int]{
		BatchSize:     100,
		FlushInterval: 10 * time.Millisecond,
		Processor: func(items []int) error {
			atomic.AddInt64(&processed, int64(len(items)))
			return nil
		},
	})

	bp.Start(ctx)
	defer bp.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bp.Add(i)
	}
}

func BenchmarkCircuitBreaker(b *testing.B) {
	ctx := context.Background()
	cb := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
		FailureThreshold: 100,
		Timeout:          time.Minute,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Execute(ctx, func(context.Context) error {
			return nil
		})
	}
}

func BenchmarkRateLimiter(b *testing.B) {
	limiter := resilience.NewRateLimiter(resilience.RateLimiterConfig{
		Rate:       1000000, // High rate to not block
		BucketSize: 1000000,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow()
	}
}
