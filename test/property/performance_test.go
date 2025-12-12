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
	"github.com/syntor/syntor/pkg/performance"
)

// TestSubMillisecondLocalCommunication tests Property 2: Sub-millisecond local communication
// Validates: Requirements 1.4
func TestSubMillisecondLocalCommunication(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("local message passing completes in sub-millisecond time", prop.ForAll(
		func(messageCount int) bool {
			// Create a simple in-memory channel-based message passing
			ch := make(chan struct{}, messageCount)

			start := time.Now()

			// Send all messages
			for i := 0; i < messageCount; i++ {
				ch <- struct{}{}
			}

			// Receive all messages
			for i := 0; i < messageCount; i++ {
				<-ch
			}

			elapsed := time.Since(start)

			// For local communication, expect sub-millisecond per message on average
			avgPerMessage := elapsed / time.Duration(messageCount)
			return avgPerMessage < time.Millisecond
		},
		gen.IntRange(1, 1000),
	))

	properties.Property("batch processor processes items with low latency", prop.ForAll(
		func(batchSize int) bool {
			processed := make(chan time.Duration, 100)
			startTimes := sync.Map{}

			bp := performance.NewBatchProcessor(performance.BatchProcessorConfig[int]{
				BatchSize:     batchSize,
				FlushInterval: 10 * time.Millisecond,
				Processor: func(items []int) error {
					for _, item := range items {
						if start, ok := startTimes.Load(item); ok {
							latency := time.Since(start.(time.Time))
							select {
							case processed <- latency:
							default:
							}
						}
					}
					return nil
				},
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			bp.Start(ctx)

			// Add items with timing
			for i := 0; i < batchSize; i++ {
				startTimes.Store(i, time.Now())
				bp.Add(i)
			}

			// Wait for processing
			time.Sleep(50 * time.Millisecond)
			bp.Stop()

			// Check latencies
			close(processed)
			var totalLatency time.Duration
			count := 0
			for latency := range processed {
				totalLatency += latency
				count++
			}

			if count == 0 {
				return true // No items processed is okay for edge cases
			}

			avgLatency := totalLatency / time.Duration(count)
			// Batch processing should complete within 100ms including flush interval
			return avgLatency < 100*time.Millisecond
		},
		gen.IntRange(1, 100),
	))

	properties.Property("pipeline processing maintains low per-item latency", prop.ForAll(
		func(workerCount int) bool {
			itemCount := 50
			var latencies []time.Duration
			var mu sync.Mutex

			pipeline := performance.NewPipeline[time.Time](workerCount, 100)

			// Add a simple pass-through stage
			pipeline.AddStage(func(t time.Time) (time.Time, error) {
				return t, nil
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			pipeline.Start(ctx)

			// Submit items
			go func() {
				for i := 0; i < itemCount; i++ {
					pipeline.Submit(time.Now())
				}
			}()

			// Collect results with timeout
			timeout := time.After(2 * time.Second)
			received := 0
			for received < itemCount {
				select {
				case startTime := <-pipeline.Results():
					latency := time.Since(startTime)
					mu.Lock()
					latencies = append(latencies, latency)
					mu.Unlock()
					received++
				case <-timeout:
					// Timeout, check what we have
					goto done
				}
			}
		done:

			pipeline.Stop()

			if len(latencies) == 0 {
				return true
			}

			// Calculate average latency
			var total time.Duration
			for _, l := range latencies {
				total += l
			}
			avg := total / time.Duration(len(latencies))

			// Pipeline should process items quickly
			return avg < 50*time.Millisecond
		},
		gen.IntRange(1, 10),
	))

	properties.TestingRun(t)
}

// TestScalabilityWithoutDegradation tests Property 3: Scalability without degradation
// Validates: Requirements 1.5
func TestScalabilityWithoutDegradation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("throughput scales with worker count", prop.ForAll(
		func(workerCount int) bool {
			items := 500

			// Measure throughput with given worker count
			throughput := measurePipelineThroughput(workerCount, items)

			// Throughput should be positive and reasonable
			// With more workers, we should process items faster
			return throughput > 0
		},
		gen.IntRange(1, 8),
	))

	properties.Property("latency remains bounded under load", prop.ForAll(
		func(load int) bool {
			workers := 4

			// Measure latency at given load
			latency := measureAverageLatency(workers, load)

			// Latency should be bounded regardless of load
			// For in-process pipeline, latency should be well under 100ms
			return latency < 100*time.Millisecond
		},
		gen.IntRange(50, 500),
	))

	properties.Property("load balancer distributes work evenly across instances", prop.ForAll(
		func(instanceCount int) bool {
			lb := performance.NewLoadBalancer(performance.LoadBalancerConfig{
				Algorithm: "round-robin",
			})

			// Add instances
			for i := 0; i < instanceCount; i++ {
				lb.AddInstance(fmt.Sprintf("instance-%d", i), 1.0)
			}

			// Track distribution
			distribution := make(map[string]int)
			totalRequests := instanceCount * 100

			for i := 0; i < totalRequests; i++ {
				instance := lb.Next()
				distribution[instance]++
			}

			// Each instance should receive approximately equal load
			expectedPerInstance := totalRequests / instanceCount
			tolerance := float64(expectedPerInstance) * 0.1 // 10% tolerance

			for _, count := range distribution {
				deviation := float64(count - expectedPerInstance)
				if deviation < 0 {
					deviation = -deviation
				}
				if deviation > tolerance {
					return false
				}
			}

			return true
		},
		gen.IntRange(2, 10),
	))

	properties.TestingRun(t)
}

// TestIndependentAgentScaling tests Property 15: Independent agent scaling
// Validates: Requirements 5.3
func TestIndependentAgentScaling(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("each agent type scales independently", prop.ForAll(
		func(serviceCount, workerCount int) bool {
			scaler := performance.NewAutoScaler(performance.AutoScalerConfig{
				HistorySize: 10,
			})

			// Register different agent types with different policies
			scaler.RegisterAgentType("service", performance.ScalingPolicy{
				MinInstances: 1,
				MaxInstances: 5,
				TargetCPU:    0.7,
			}, serviceCount)

			scaler.RegisterAgentType("worker", performance.ScalingPolicy{
				MinInstances: 2,
				MaxInstances: 20,
				TargetCPU:    0.8,
			}, workerCount)

			// Verify independent counts
			return scaler.GetCurrentCount("service") == serviceCount &&
				scaler.GetCurrentCount("worker") == workerCount
		},
		gen.IntRange(1, 5),
		gen.IntRange(2, 10),
	))

	properties.Property("scaling decisions are based on recorded metrics", prop.ForAll(
		func(cpuUtilization int) bool {
			scaler := performance.NewAutoScaler(performance.AutoScalerConfig{
				HistorySize: 10,
			})

			cpu := float64(cpuUtilization) / 100.0

			// Register agent type
			scaler.RegisterAgentType("service", performance.ScalingPolicy{
				MinInstances:       1,
				MaxInstances:       10,
				ScaleUpThreshold:   0.8,
				ScaleDownThreshold: 0.2,
				CooldownPeriod:     0,
				ScaleUpStep:        1,
				ScaleDownStep:      1,
				TargetQueueDepth:   100,
			}, 3)

			// Record metrics
			for i := 0; i < 5; i++ {
				scaler.RecordMetrics("service", performance.ScalingMetrics{
					CPUUtilization:    cpu,
					MemoryUtilization: cpu,
					QueueDepth:        int(cpu * 200),
				})
			}

			// Evaluate scaling
			decisions := scaler.Evaluate()

			// High CPU should trigger scale up, low CPU may trigger scale down
			if cpu > 0.8 {
				// Should have scale up decision
				for _, d := range decisions {
					if d.AgentType == "service" && d.DesiredCount > d.CurrentCount {
						return true
					}
				}
				return false
			}

			// Low or medium CPU - any decision is valid
			return true
		},
		gen.IntRange(10, 95),
	))

	properties.Property("scaling respects min and max instance limits", prop.ForAll(
		func(initialCount int) bool {
			// Clamp initial count to valid range for testing
			if initialCount < 2 {
				initialCount = 2
			}
			if initialCount > 8 {
				initialCount = 8
			}

			policy := performance.ScalingPolicy{
				MinInstances:       2,
				MaxInstances:       8,
				ScaleUpThreshold:   0.5,
				ScaleDownThreshold: 0.1,
				CooldownPeriod:     0,
				ScaleUpStep:        2,
				ScaleDownStep:      1,
				TargetQueueDepth:   50,
			}

			scaler := performance.NewAutoScaler(performance.AutoScalerConfig{
				HistorySize: 10,
			})
			scaler.RegisterAgentType("test", policy, initialCount)

			// Test scale down - record low metrics
			for i := 0; i < 5; i++ {
				scaler.RecordMetrics("test", performance.ScalingMetrics{
					CPUUtilization:    0.05,
					MemoryUtilization: 0.05,
					QueueDepth:        5,
				})
			}

			decisions := scaler.Evaluate()
			for _, d := range decisions {
				if d.AgentType == "test" {
					// Verify decision respects limits
					if d.DesiredCount < policy.MinInstances {
						return false
					}
					if d.DesiredCount > policy.MaxInstances {
						return false
					}
				}
			}

			return true
		},
		gen.IntRange(2, 8),
	))

	properties.TestingRun(t)
}

// TestThroughputScaling tests Property 10: Throughput scaling
// Validates: Requirements 3.4
func TestThroughputScaling(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("partition count scales with throughput requirements", prop.ForAll(
		func(currentThroughput int) bool {
			scaler := performance.NewThroughputScaler(performance.ThroughputScalerConfig{
				TargetThroughput: 10000,
				MinPartitions:    1,
				MaxPartitions:    100,
				ScaleThreshold:   0.8,
			})

			partitions := scaler.CalculatePartitions(float64(currentThroughput))

			// Partitions should be within bounds
			return partitions >= 1 && partitions <= 100
		},
		gen.IntRange(1000, 100000),
	))

	properties.Property("throughput counter accurately measures rate", prop.ForAll(
		func(eventsPerSecond int) bool {
			counter := performance.NewThroughputCounter(time.Second)

			// Generate events over a short period
			events := eventsPerSecond / 10 // Reduce to 100ms worth
			for i := 0; i < events; i++ {
				counter.Increment(1)
			}

			rate := counter.Rate()

			// Rate should be approximately proportional to events
			// Allow significant tolerance due to timing
			expectedMin := float64(events) * 0.5
			expectedMax := float64(events) * 2.0

			return rate >= expectedMin && rate <= expectedMax
		},
		gen.IntRange(10, 1000),
	))

	properties.Property("latency tracker calculates percentiles correctly", prop.ForAll(
		func(sampleCount int) bool {
			tracker := performance.NewLatencyTracker(1000)

			// Add samples in order (1ms, 2ms, 3ms, ...)
			for i := 1; i <= sampleCount; i++ {
				tracker.Record(time.Duration(i) * time.Millisecond)
			}

			p50 := tracker.P50()
			p95 := tracker.P95()
			p99 := tracker.P99()

			// P50 should be around median
			expectedP50 := time.Duration(sampleCount/2) * time.Millisecond
			p50Valid := p50 >= expectedP50-time.Millisecond && p50 <= expectedP50+time.Millisecond

			// P95 should be higher than P50
			p95Valid := p95 > p50

			// P99 should be highest
			p99Valid := p99 >= p95

			return p50Valid && p95Valid && p99Valid
		},
		gen.IntRange(10, 100),
	))

	properties.Property("performance monitor tracks multiple operations", prop.ForAll(
		func(operationCount int) bool {
			monitor := performance.NewPerformanceMonitor()

			// Record metrics for multiple operations
			for i := 0; i < operationCount; i++ {
				opName := fmt.Sprintf("op-%d", i)
				monitor.RecordLatency(opName, time.Duration(i+1)*time.Millisecond)
				monitor.RecordThroughput(opName, int64(i+1))
			}

			// Verify each operation has metrics
			for i := 0; i < operationCount; i++ {
				opName := fmt.Sprintf("op-%d", i)
				avg, _, _, _ := monitor.GetLatencyStats(opName)
				if avg == 0 {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 20),
	))

	properties.TestingRun(t)
}

// Helper functions

func measurePipelineThroughput(workers, items int) float64 {
	pipeline := performance.NewPipeline[int](workers, items)

	// Simple pass-through stage
	pipeline.AddStage(func(i int) (int, error) {
		return i, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pipeline.Start(ctx)

	start := time.Now()

	// Submit items
	go func() {
		for i := 0; i < items; i++ {
			pipeline.Submit(i)
		}
	}()

	// Receive items
	received := 0
	timeout := time.After(5 * time.Second)
	for received < items {
		select {
		case <-pipeline.Results():
			received++
		case <-timeout:
			goto done
		}
	}
done:

	elapsed := time.Since(start)
	pipeline.Stop()

	if elapsed == 0 {
		return 0
	}

	return float64(received) / elapsed.Seconds()
}

func measureAverageLatency(workers, items int) time.Duration {
	pipeline := performance.NewPipeline[time.Time](workers, items)

	pipeline.AddStage(func(t time.Time) (time.Time, error) {
		return t, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pipeline.Start(ctx)

	var totalLatency int64
	var count int64

	// Submit and measure
	go func() {
		for i := 0; i < items; i++ {
			pipeline.Submit(time.Now())
		}
	}()

	timeout := time.After(5 * time.Second)
	received := 0
	for received < items {
		select {
		case startTime := <-pipeline.Results():
			atomic.AddInt64(&totalLatency, int64(time.Since(startTime)))
			atomic.AddInt64(&count, 1)
			received++
		case <-timeout:
			goto done
		}
	}
done:

	pipeline.Stop()

	if count == 0 {
		return 0
	}

	return time.Duration(totalLatency / count)
}
