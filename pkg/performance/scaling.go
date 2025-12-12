package performance

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func randFloat() float64 {
	return rand.Float64()
}

// ScalingDecision represents a scaling action
type ScalingDecision struct {
	AgentType     string
	CurrentCount  int
	DesiredCount  int
	Reason        string
	Timestamp     time.Time
}

// ScalingMetrics holds metrics used for scaling decisions
type ScalingMetrics struct {
	CPUUtilization    float64
	MemoryUtilization float64
	QueueDepth        int
	MessageRate       float64
	AverageLatency    time.Duration
	ErrorRate         float64
	ActiveTasks       int
	Timestamp         time.Time
}

// ScalingPolicy defines scaling behavior
type ScalingPolicy struct {
	MinInstances      int
	MaxInstances      int
	TargetCPU         float64
	TargetMemory      float64
	TargetQueueDepth  int
	TargetLatency     time.Duration
	ScaleUpThreshold  float64
	ScaleDownThreshold float64
	CooldownPeriod    time.Duration
	ScaleUpStep       int
	ScaleDownStep     int
}

// DefaultScalingPolicy returns a default scaling policy
func DefaultScalingPolicy() ScalingPolicy {
	return ScalingPolicy{
		MinInstances:       1,
		MaxInstances:       10,
		TargetCPU:          0.7,
		TargetMemory:       0.8,
		TargetQueueDepth:   100,
		TargetLatency:      100 * time.Millisecond,
		ScaleUpThreshold:   0.8,
		ScaleDownThreshold: 0.3,
		CooldownPeriod:     60 * time.Second,
		ScaleUpStep:        1,
		ScaleDownStep:      1,
	}
}

// AutoScaler manages automatic scaling of agent instances
type AutoScaler struct {
	policies       map[string]ScalingPolicy
	currentCounts  map[string]int
	lastScaleTime  map[string]time.Time
	metricsHistory map[string][]ScalingMetrics
	historySize    int
	mu             sync.RWMutex

	// Callback for executing scaling actions
	onScale func(agentType string, delta int) error
}

// AutoScalerConfig holds configuration for the auto scaler
type AutoScalerConfig struct {
	HistorySize int
	OnScale     func(agentType string, delta int) error
}

// NewAutoScaler creates a new auto scaler
func NewAutoScaler(config AutoScalerConfig) *AutoScaler {
	if config.HistorySize <= 0 {
		config.HistorySize = 10
	}

	return &AutoScaler{
		policies:       make(map[string]ScalingPolicy),
		currentCounts:  make(map[string]int),
		lastScaleTime:  make(map[string]time.Time),
		metricsHistory: make(map[string][]ScalingMetrics),
		historySize:    config.HistorySize,
		onScale:        config.OnScale,
	}
}

// RegisterAgentType registers an agent type with its scaling policy
func (as *AutoScaler) RegisterAgentType(agentType string, policy ScalingPolicy, initialCount int) {
	as.mu.Lock()
	defer as.mu.Unlock()

	as.policies[agentType] = policy
	as.currentCounts[agentType] = initialCount
	as.metricsHistory[agentType] = make([]ScalingMetrics, 0, as.historySize)
}

// RecordMetrics records metrics for an agent type
func (as *AutoScaler) RecordMetrics(agentType string, metrics ScalingMetrics) {
	as.mu.Lock()
	defer as.mu.Unlock()

	metrics.Timestamp = time.Now()
	history := as.metricsHistory[agentType]

	if len(history) >= as.historySize {
		history = history[1:]
	}
	as.metricsHistory[agentType] = append(history, metrics)
}

// Evaluate evaluates scaling decisions for all registered agent types
func (as *AutoScaler) Evaluate() []ScalingDecision {
	as.mu.Lock()
	defer as.mu.Unlock()

	var decisions []ScalingDecision

	for agentType, policy := range as.policies {
		decision := as.evaluateAgentType(agentType, policy)
		if decision != nil {
			decisions = append(decisions, *decision)
		}
	}

	return decisions
}

// EvaluateAgentType evaluates scaling for a specific agent type
func (as *AutoScaler) EvaluateAgentType(agentType string) *ScalingDecision {
	as.mu.Lock()
	defer as.mu.Unlock()

	policy, exists := as.policies[agentType]
	if !exists {
		return nil
	}

	return as.evaluateAgentType(agentType, policy)
}

func (as *AutoScaler) evaluateAgentType(agentType string, policy ScalingPolicy) *ScalingDecision {
	history := as.metricsHistory[agentType]
	if len(history) == 0 {
		return nil
	}

	// Check cooldown
	lastScale, hasScaled := as.lastScaleTime[agentType]
	if hasScaled && time.Since(lastScale) < policy.CooldownPeriod {
		return nil
	}

	// Calculate average metrics
	avgMetrics := as.calculateAverageMetrics(history)
	currentCount := as.currentCounts[agentType]

	// Determine scaling action
	action, reason := as.determineScalingAction(avgMetrics, policy, currentCount)

	if action == 0 {
		return nil
	}

	desiredCount := currentCount + action

	// Clamp to policy limits
	if desiredCount < policy.MinInstances {
		desiredCount = policy.MinInstances
	}
	if desiredCount > policy.MaxInstances {
		desiredCount = policy.MaxInstances
	}

	if desiredCount == currentCount {
		return nil
	}

	return &ScalingDecision{
		AgentType:    agentType,
		CurrentCount: currentCount,
		DesiredCount: desiredCount,
		Reason:       reason,
		Timestamp:    time.Now(),
	}
}

func (as *AutoScaler) calculateAverageMetrics(history []ScalingMetrics) ScalingMetrics {
	var avg ScalingMetrics
	n := float64(len(history))

	for _, m := range history {
		avg.CPUUtilization += m.CPUUtilization
		avg.MemoryUtilization += m.MemoryUtilization
		avg.QueueDepth += m.QueueDepth
		avg.MessageRate += m.MessageRate
		avg.AverageLatency += m.AverageLatency
		avg.ErrorRate += m.ErrorRate
		avg.ActiveTasks += m.ActiveTasks
	}

	avg.CPUUtilization /= n
	avg.MemoryUtilization /= n
	avg.QueueDepth = int(float64(avg.QueueDepth) / n)
	avg.MessageRate /= n
	avg.AverageLatency = time.Duration(float64(avg.AverageLatency) / n)
	avg.ErrorRate /= n
	avg.ActiveTasks = int(float64(avg.ActiveTasks) / n)

	return avg
}

func (as *AutoScaler) determineScalingAction(metrics ScalingMetrics, policy ScalingPolicy, currentCount int) (int, string) {
	// Check for scale up conditions
	if metrics.CPUUtilization > policy.ScaleUpThreshold {
		return policy.ScaleUpStep, "CPU utilization above threshold"
	}
	if metrics.MemoryUtilization > policy.ScaleUpThreshold {
		return policy.ScaleUpStep, "Memory utilization above threshold"
	}
	if metrics.QueueDepth > policy.TargetQueueDepth {
		return policy.ScaleUpStep, "Queue depth above target"
	}
	if metrics.AverageLatency > policy.TargetLatency {
		return policy.ScaleUpStep, "Latency above target"
	}

	// Check for scale down conditions
	if metrics.CPUUtilization < policy.ScaleDownThreshold &&
		metrics.MemoryUtilization < policy.ScaleDownThreshold &&
		metrics.QueueDepth < policy.TargetQueueDepth/2 &&
		currentCount > policy.MinInstances {
		return -policy.ScaleDownStep, "Low resource utilization"
	}

	return 0, ""
}

// ApplyDecision applies a scaling decision
func (as *AutoScaler) ApplyDecision(decision ScalingDecision) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	delta := decision.DesiredCount - decision.CurrentCount

	if as.onScale != nil {
		if err := as.onScale(decision.AgentType, delta); err != nil {
			return err
		}
	}

	as.currentCounts[decision.AgentType] = decision.DesiredCount
	as.lastScaleTime[decision.AgentType] = time.Now()

	return nil
}

// GetCurrentCount returns the current instance count for an agent type
func (as *AutoScaler) GetCurrentCount(agentType string) int {
	as.mu.RLock()
	defer as.mu.RUnlock()
	return as.currentCounts[agentType]
}

// SetCurrentCount sets the current instance count for an agent type
func (as *AutoScaler) SetCurrentCount(agentType string, count int) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.currentCounts[agentType] = count
}

// ThroughputScaler scales based on message throughput
type ThroughputScaler struct {
	targetThroughput  float64
	currentPartitions int
	maxPartitions     int
	minPartitions     int
	scaleThreshold    float64
	mu                sync.RWMutex
}

// ThroughputScalerConfig holds configuration for throughput scaling
type ThroughputScalerConfig struct {
	TargetThroughput float64
	MinPartitions    int
	MaxPartitions    int
	ScaleThreshold   float64
}

// NewThroughputScaler creates a new throughput scaler
func NewThroughputScaler(config ThroughputScalerConfig) *ThroughputScaler {
	if config.MinPartitions <= 0 {
		config.MinPartitions = 1
	}
	if config.MaxPartitions <= 0 {
		config.MaxPartitions = 100
	}
	if config.TargetThroughput <= 0 {
		config.TargetThroughput = 10000
	}
	if config.ScaleThreshold <= 0 {
		config.ScaleThreshold = 0.8
	}

	return &ThroughputScaler{
		targetThroughput:  config.TargetThroughput,
		currentPartitions: config.MinPartitions,
		maxPartitions:     config.MaxPartitions,
		minPartitions:     config.MinPartitions,
		scaleThreshold:    config.ScaleThreshold,
	}
}

// CalculatePartitions calculates the optimal number of partitions for a given throughput
func (ts *ThroughputScaler) CalculatePartitions(currentThroughput float64) int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	// Calculate utilization
	utilizationPerPartition := currentThroughput / float64(ts.currentPartitions)

	// Target utilization per partition
	targetUtilPerPartition := ts.targetThroughput * ts.scaleThreshold / float64(ts.currentPartitions)

	// Calculate needed partitions
	if utilizationPerPartition > targetUtilPerPartition {
		// Need more partitions
		needed := int(math.Ceil(currentThroughput / (ts.targetThroughput * ts.scaleThreshold)))
		if needed > ts.maxPartitions {
			needed = ts.maxPartitions
		}
		return needed
	}

	// Check if we can reduce partitions
	if utilizationPerPartition < targetUtilPerPartition*0.3 && ts.currentPartitions > ts.minPartitions {
		needed := int(math.Ceil(currentThroughput / (ts.targetThroughput * 0.5)))
		if needed < ts.minPartitions {
			needed = ts.minPartitions
		}
		return needed
	}

	return ts.currentPartitions
}

// SetPartitions sets the current partition count
func (ts *ThroughputScaler) SetPartitions(count int) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.currentPartitions = count
}

// GetPartitions returns the current partition count
func (ts *ThroughputScaler) GetPartitions() int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.currentPartitions
}

// LoadBalancer distributes load across instances
type LoadBalancer struct {
	instances     []string
	weights       map[string]float64
	currentIndex  int
	algorithm     string
	mu            sync.RWMutex
}

// LoadBalancerConfig holds configuration for the load balancer
type LoadBalancerConfig struct {
	Algorithm string // "round-robin", "weighted", "least-connections"
}

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer(config LoadBalancerConfig) *LoadBalancer {
	if config.Algorithm == "" {
		config.Algorithm = "round-robin"
	}

	return &LoadBalancer{
		instances:    make([]string, 0),
		weights:      make(map[string]float64),
		algorithm:    config.Algorithm,
		currentIndex: 0,
	}
}

// AddInstance adds an instance to the load balancer
func (lb *LoadBalancer) AddInstance(id string, weight float64) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.instances = append(lb.instances, id)
	lb.weights[id] = weight
}

// RemoveInstance removes an instance from the load balancer
func (lb *LoadBalancer) RemoveInstance(id string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	for i, inst := range lb.instances {
		if inst == id {
			lb.instances = append(lb.instances[:i], lb.instances[i+1:]...)
			break
		}
	}
	delete(lb.weights, id)
}

// Next returns the next instance to use
func (lb *LoadBalancer) Next() string {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if len(lb.instances) == 0 {
		return ""
	}

	switch lb.algorithm {
	case "weighted":
		return lb.weightedSelect()
	default: // round-robin
		instance := lb.instances[lb.currentIndex]
		lb.currentIndex = (lb.currentIndex + 1) % len(lb.instances)
		return instance
	}
}

func (lb *LoadBalancer) weightedSelect() string {
	// Calculate total weight
	var totalWeight float64
	for _, id := range lb.instances {
		totalWeight += lb.weights[id]
	}

	// Select based on weight
	r := randFloat() * totalWeight
	var cumulative float64
	for _, id := range lb.instances {
		cumulative += lb.weights[id]
		if r <= cumulative {
			return id
		}
	}

	return lb.instances[len(lb.instances)-1]
}

// InstanceCount returns the number of instances
func (lb *LoadBalancer) InstanceCount() int {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return len(lb.instances)
}

// LatencyTracker tracks operation latencies
type LatencyTracker struct {
	samples    []time.Duration
	maxSamples int
	mu         sync.RWMutex
}

// NewLatencyTracker creates a new latency tracker
func NewLatencyTracker(maxSamples int) *LatencyTracker {
	if maxSamples <= 0 {
		maxSamples = 1000
	}
	return &LatencyTracker{
		samples:    make([]time.Duration, 0, maxSamples),
		maxSamples: maxSamples,
	}
}

// Record records a latency sample
func (lt *LatencyTracker) Record(latency time.Duration) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	if len(lt.samples) >= lt.maxSamples {
		lt.samples = lt.samples[1:]
	}
	lt.samples = append(lt.samples, latency)
}

// Average returns the average latency
func (lt *LatencyTracker) Average() time.Duration {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	if len(lt.samples) == 0 {
		return 0
	}

	var total time.Duration
	for _, s := range lt.samples {
		total += s
	}
	return total / time.Duration(len(lt.samples))
}

// Percentile returns the p-th percentile latency
func (lt *LatencyTracker) Percentile(p float64) time.Duration {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	if len(lt.samples) == 0 {
		return 0
	}

	// Copy and sort samples
	sorted := make([]time.Duration, len(lt.samples))
	copy(sorted, lt.samples)

	// Simple bubble sort for small datasets
	for i := 0; i < len(sorted)-1; i++ {
		for j := 0; j < len(sorted)-1-i; j++ {
			if sorted[j] > sorted[j+1] {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}

	index := int(float64(len(sorted)-1) * p)
	return sorted[index]
}

// P50 returns the 50th percentile (median) latency
func (lt *LatencyTracker) P50() time.Duration {
	return lt.Percentile(0.5)
}

// P95 returns the 95th percentile latency
func (lt *LatencyTracker) P95() time.Duration {
	return lt.Percentile(0.95)
}

// P99 returns the 99th percentile latency
func (lt *LatencyTracker) P99() time.Duration {
	return lt.Percentile(0.99)
}

// SampleCount returns the number of samples
func (lt *LatencyTracker) SampleCount() int {
	lt.mu.RLock()
	defer lt.mu.RUnlock()
	return len(lt.samples)
}

// Clear clears all samples
func (lt *LatencyTracker) Clear() {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.samples = lt.samples[:0]
}

// PerformanceMonitor monitors overall system performance
type PerformanceMonitor struct {
	latencyTrackers map[string]*LatencyTracker
	throughputRates map[string]*ThroughputCounter
	mu              sync.RWMutex
}

// ThroughputCounter counts throughput over time windows
type ThroughputCounter struct {
	counts     []int64
	timestamps []time.Time
	windowSize time.Duration
	mu         sync.Mutex
}

// NewThroughputCounter creates a new throughput counter
func NewThroughputCounter(windowSize time.Duration) *ThroughputCounter {
	if windowSize <= 0 {
		windowSize = time.Second
	}
	return &ThroughputCounter{
		counts:     make([]int64, 0),
		timestamps: make([]time.Time, 0),
		windowSize: windowSize,
	}
}

// Increment increments the counter
func (tc *ThroughputCounter) Increment(delta int64) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	now := time.Now()
	tc.cleanup(now)
	tc.counts = append(tc.counts, delta)
	tc.timestamps = append(tc.timestamps, now)
}

// Rate returns the current rate per second
func (tc *ThroughputCounter) Rate() float64 {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.cleanup(time.Now())

	if len(tc.counts) == 0 {
		return 0
	}

	var total int64
	for _, c := range tc.counts {
		total += c
	}

	return float64(total) / tc.windowSize.Seconds()
}

func (tc *ThroughputCounter) cleanup(now time.Time) {
	cutoff := now.Add(-tc.windowSize)

	i := 0
	for ; i < len(tc.timestamps); i++ {
		if tc.timestamps[i].After(cutoff) {
			break
		}
	}

	if i > 0 {
		tc.counts = tc.counts[i:]
		tc.timestamps = tc.timestamps[i:]
	}
}

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor() *PerformanceMonitor {
	return &PerformanceMonitor{
		latencyTrackers: make(map[string]*LatencyTracker),
		throughputRates: make(map[string]*ThroughputCounter),
	}
}

// RecordLatency records a latency for an operation
func (pm *PerformanceMonitor) RecordLatency(operation string, latency time.Duration) {
	pm.mu.Lock()
	tracker, exists := pm.latencyTrackers[operation]
	if !exists {
		tracker = NewLatencyTracker(1000)
		pm.latencyTrackers[operation] = tracker
	}
	pm.mu.Unlock()

	tracker.Record(latency)
}

// RecordThroughput records throughput for an operation
func (pm *PerformanceMonitor) RecordThroughput(operation string, count int64) {
	pm.mu.Lock()
	counter, exists := pm.throughputRates[operation]
	if !exists {
		counter = NewThroughputCounter(time.Second)
		pm.throughputRates[operation] = counter
	}
	pm.mu.Unlock()

	counter.Increment(count)
}

// GetLatencyStats returns latency statistics for an operation
func (pm *PerformanceMonitor) GetLatencyStats(operation string) (avg, p50, p95, p99 time.Duration) {
	pm.mu.RLock()
	tracker, exists := pm.latencyTrackers[operation]
	pm.mu.RUnlock()

	if !exists {
		return 0, 0, 0, 0
	}

	return tracker.Average(), tracker.P50(), tracker.P95(), tracker.P99()
}

// GetThroughput returns throughput for an operation
func (pm *PerformanceMonitor) GetThroughput(operation string) float64 {
	pm.mu.RLock()
	counter, exists := pm.throughputRates[operation]
	pm.mu.RUnlock()

	if !exists {
		return 0
	}

	return counter.Rate()
}

// StartMonitoring starts background monitoring
func (as *AutoScaler) StartMonitoring(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			decisions := as.Evaluate()
			for _, d := range decisions {
				as.ApplyDecision(d)
			}
		}
	}
}
