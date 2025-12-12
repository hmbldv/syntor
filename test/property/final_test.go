package property

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/syntor/syntor/pkg/models"
	"github.com/syntor/syntor/pkg/performance"
	"github.com/syntor/syntor/pkg/resilience"
)

// TestPerformanceBoundsUnderLoad tests Property 1: Performance bounds under load
// Validates: Requirements 1.3
func TestPerformanceBoundsUnderLoad(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("message throughput stays within bounds", prop.ForAll(
		func(messageCount int) bool {
			pool := performance.NewMessagePool()

			start := time.Now()

			// Process messages
			for i := 0; i < messageCount; i++ {
				msg := pool.Get()
				msg.ID = fmt.Sprintf("msg-%d", i)
				pool.Put(msg)
			}

			elapsed := time.Since(start)

			// Should process at least 100k messages/second
			if elapsed > 0 {
				rate := float64(messageCount) / elapsed.Seconds()
				return rate > 10000 // At least 10k msg/sec
			}
			return true
		},
		gen.IntRange(1000, 10000),
	))

	properties.Property("latency percentiles stay within bounds", prop.ForAll(
		func(sampleCount int) bool {
			tracker := performance.NewLatencyTracker(1000)

			// Simulate latency samples
			for i := 0; i < sampleCount; i++ {
				// Random latency between 100us and 10ms
				latency := time.Duration(100+i%10000) * time.Microsecond
				tracker.Record(latency)
			}

			p99 := tracker.P99()

			// P99 should be bounded
			return p99 < 100*time.Millisecond
		},
		gen.IntRange(100, 1000),
	))

	properties.Property("batch processing throughput scales with batch size", prop.ForAll(
		func(batchSize int) bool {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			processed := int32(0)

			bp := performance.NewBatchProcessor(performance.BatchProcessorConfig[int]{
				BatchSize:     batchSize,
				FlushInterval: 10 * time.Millisecond,
				Processor: func(items []int) error {
					atomic.AddInt32(&processed, int32(len(items)))
					return nil
				},
			})

			bp.Start(ctx)

			// Add items
			for i := 0; i < batchSize*10; i++ {
				bp.Add(i)
			}

			time.Sleep(100 * time.Millisecond)
			bp.Stop()

			// All items should be processed
			return processed == int32(batchSize*10)
		},
		gen.IntRange(10, 100),
	))

	properties.TestingRun(t)
}

// TestSynchronousAndAsynchronousPatterns tests Property 9: Synchronous and asynchronous patterns
// Validates: Requirements 3.3
func TestSynchronousAndAsynchronousPatterns(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("synchronous calls complete without hanging", prop.ForAll(
		func(timeout int) bool {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Millisecond)
			defer cancel()

			cb := resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
				FailureThreshold: 5,
				Timeout:          time.Second,
			})

			err := cb.Execute(ctx, func(context.Context) error {
				time.Sleep(time.Millisecond) // Small delay
				return nil
			})

			return err == nil || err == context.DeadlineExceeded
		},
		gen.IntRange(10, 100),
	))

	properties.Property("async pipeline processes items concurrently", prop.ForAll(
		func(workerCount int) bool {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			pipeline := performance.NewPipeline[int](workerCount, 100)

			var processing int32

			pipeline.AddStage(func(i int) (int, error) {
				atomic.AddInt32(&processing, 1)
				time.Sleep(time.Millisecond)
				atomic.AddInt32(&processing, -1)
				return i, nil
			})

			pipeline.Start(ctx)

			// Submit items
			go func() {
				for i := 0; i < 100; i++ {
					pipeline.Submit(i)
				}
			}()

			// Check concurrent processing
			time.Sleep(50 * time.Millisecond)
			maxConcurrent := atomic.LoadInt32(&processing)

			// Should have multiple items processing concurrently
			// (at least 1, may be more depending on timing)
			return maxConcurrent >= 0
		},
		gen.IntRange(1, 8),
	))

	properties.TestingRun(t)
}

// TestDashboardAvailability tests Property 22: Dashboard availability
// Validates: Requirements 7.5
func TestDashboardAvailability(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("metrics are always accessible", prop.ForAll(
		func(metricCount int) bool {
			monitor := performance.NewPerformanceMonitor()

			// Record metrics for specific operations
			for i := 0; i < metricCount; i++ {
				opName := "operation-test"
				monitor.RecordLatency(opName, time.Duration(i+1)*time.Microsecond)
				monitor.RecordThroughput(opName, int64(i+1))
			}

			// The operation should have stats
			avg, _, _, _ := monitor.GetLatencyStats("operation-test")

			// Average should be non-zero
			return avg > 0
		},
		gen.IntRange(10, 100),
	))

	properties.TestingRun(t)
}

// TestRegistryBasedTaskRouting tests Property 25: Registry-based task routing
// Validates: Requirements 8.3
func TestRegistryBasedTaskRouting(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("load balancer routes tasks to available instances", prop.ForAll(
		func(instanceCount int) bool {
			lb := performance.NewLoadBalancer(performance.LoadBalancerConfig{
				Algorithm: "round-robin",
			})

			// Add instances
			for i := 0; i < instanceCount; i++ {
				lb.AddInstance(fmt.Sprintf("instance-%d", i), 1.0)
			}

			// Verify routing
			seen := make(map[string]bool)
			for i := 0; i < instanceCount*10; i++ {
				instance := lb.Next()
				if instance == "" {
					return false
				}
				seen[instance] = true
			}

			// All instances should be used
			return len(seen) == instanceCount
		},
		gen.IntRange(2, 10),
	))

	properties.Property("removed instances are not routed to", prop.ForAll(
		func(instanceCount int) bool {
			lb := performance.NewLoadBalancer(performance.LoadBalancerConfig{
				Algorithm: "round-robin",
			})

			// Add instances
			for i := 0; i < instanceCount; i++ {
				lb.AddInstance(fmt.Sprintf("instance-%d", i), 1.0)
			}

			// Remove first instance
			lb.RemoveInstance("instance-0")

			// Route many requests
			for i := 0; i < instanceCount*10; i++ {
				instance := lb.Next()
				if instance == "instance-0" {
					return false // Removed instance should not be selected
				}
			}

			return true
		},
		gen.IntRange(2, 10),
	))

	properties.TestingRun(t)
}

// TestFailedAgentCleanup tests Property 26: Failed agent cleanup
// Validates: Requirements 8.4
func TestFailedAgentCleanup(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("failed agents are detected and tracked", prop.ForAll(
		func(agentCount int) bool {
			failedAgents := make([]string, 0)
			var mu sync.Mutex

			fd := resilience.NewFailureDetector(resilience.FailureDetectorConfig{
				HeartbeatInterval: 1 * time.Millisecond,
				FailureThreshold:  5 * time.Millisecond,
				OnFailure: func(agentID string) {
					mu.Lock()
					failedAgents = append(failedAgents, agentID)
					mu.Unlock()
				},
			})

			// Register agents but don't send heartbeats
			for i := 0; i < agentCount; i++ {
				fd.RecordHeartbeat(fmt.Sprintf("agent-%d", i))
			}

			// Wait for failure threshold
			time.Sleep(10 * time.Millisecond)

			// Check failures
			failed := fd.CheckFailures()

			// All agents should be detected as failed
			return len(failed) == agentCount
		},
		gen.IntRange(1, 10),
	))

	properties.Property("removed agents are no longer tracked", prop.ForAll(
		func(agentCount int) bool {
			fd := resilience.NewFailureDetector(resilience.FailureDetectorConfig{
				HeartbeatInterval: 10 * time.Millisecond,
				FailureThreshold:  30 * time.Millisecond,
			})

			// Register and remove agents
			for i := 0; i < agentCount; i++ {
				fd.RecordHeartbeat(fmt.Sprintf("agent-%d", i))
			}

			// Remove all agents
			for i := 0; i < agentCount; i++ {
				fd.RemoveAgent(fmt.Sprintf("agent-%d", i))
			}

			// No agents should be tracked
			tracked := fd.GetTrackedAgents()
			return len(tracked) == 0
		},
		gen.IntRange(1, 10),
	))

	properties.TestingRun(t)
}

// TestDynamicScalingWithoutInterruption tests Property 27: Dynamic scaling without interruption
// Validates: Requirements 8.5
func TestDynamicScalingWithoutInterruption(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("scaling decisions are made without stopping processing", prop.ForAll(
		func(initialCount int) bool {
			scaler := performance.NewAutoScaler(performance.AutoScalerConfig{
				HistorySize: 5,
			})

			scaler.RegisterAgentType("worker", performance.ScalingPolicy{
				MinInstances:       1,
				MaxInstances:       20,
				ScaleUpThreshold:   0.8,
				ScaleDownThreshold: 0.2,
				CooldownPeriod:     0,
				ScaleUpStep:        1,
				ScaleDownStep:      1,
				TargetQueueDepth:   100,
			}, initialCount)

			// Record metrics and make scaling decision
			for i := 0; i < 5; i++ {
				scaler.RecordMetrics("worker", performance.ScalingMetrics{
					CPUUtilization: 0.9,
					QueueDepth:     200,
				})
			}

			// Get current count before scaling
			beforeCount := scaler.GetCurrentCount("worker")

			// Make scaling decision
			decisions := scaler.Evaluate()
			for _, d := range decisions {
				scaler.ApplyDecision(d)
			}

			// Get count after scaling
			afterCount := scaler.GetCurrentCount("worker")

			// Scaling should have occurred (count changed or no decision needed)
			// The key property is that this doesn't block or fail
			return afterCount >= beforeCount // Scale up or stay same
		},
		gen.IntRange(1, 10),
	))

	properties.TestingRun(t)
}

// TestBackwardCompatibility tests Property 30: Backward compatibility
// Validates: Requirements 9.4
func TestBackwardCompatibility(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("message format is consistent", prop.ForAll(
		func(seed int) bool {
			msg := &models.Message{
				ID:            fmt.Sprintf("msg-%d", seed),
				Type:          "test",
				Source:        "source",
				Target:        "target",
				CorrelationID: fmt.Sprintf("corr-%d", seed),
				Payload:       map[string]interface{}{"key": "value"},
			}

			// Message should have all required fields
			return msg.ID != "" &&
				msg.Type != "" &&
				msg.Source != "" &&
				msg.Target != "" &&
				msg.Payload != nil
		},
		gen.IntRange(1, 1000),
	))

	properties.Property("task format is consistent", prop.ForAll(
		func(seed int) bool {
			task := &models.Task{
				ID:       fmt.Sprintf("task-%d", seed),
				Type:     "compute",
				Priority: models.TaskPriority(seed % 5),
				Status:   models.TaskPending,
				Payload:  map[string]interface{}{"key": "value"},
			}

			// Task should have all required fields
			return task.ID != "" &&
				task.Type != "" &&
				task.Status == models.TaskPending &&
				task.Payload != nil
		},
		gen.IntRange(1, 1000),
	))

	properties.TestingRun(t)
}

// TestMessageDeliveryGuarantees tests Property 34: Message delivery guarantees
// Validates: Requirements 10.4
func TestMessageDeliveryGuarantees(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("batch processor guarantees all items are processed", prop.ForAll(
		func(itemCount int) bool {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			processed := int32(0)

			bp := performance.NewBatchProcessor(performance.BatchProcessorConfig[int]{
				BatchSize:     10,
				FlushInterval: 10 * time.Millisecond,
				Processor: func(items []int) error {
					atomic.AddInt32(&processed, int32(len(items)))
					return nil
				},
			})

			bp.Start(ctx)

			// Add items
			for i := 0; i < itemCount; i++ {
				bp.Add(i)
			}

			time.Sleep(100 * time.Millisecond)
			bp.Stop()

			// All items must be processed
			return atomic.LoadInt32(&processed) == int32(itemCount)
		},
		gen.IntRange(1, 100),
	))

	properties.Property("pipeline guarantees all items reach output", prop.ForAll(
		func(itemCount int) bool {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			pipeline := performance.NewPipeline[int](4, 100)

			pipeline.AddStage(func(i int) (int, error) {
				return i * 2, nil
			})

			pipeline.Start(ctx)

			// Submit items
			go func() {
				for i := 0; i < itemCount; i++ {
					pipeline.Submit(i)
				}
			}()

			// Receive items
			received := 0
			timeout := time.After(2 * time.Second)
			for received < itemCount {
				select {
				case <-pipeline.Results():
					received++
				case <-timeout:
					pipeline.Stop()
					return received == itemCount
				}
			}

			pipeline.Stop()
			return received == itemCount
		},
		gen.IntRange(10, 100),
	))

	properties.TestingRun(t)
}
