package registry

import (
	"context"
	"sort"
	"strings"

	"github.com/syntor/syntor/pkg/models"
)

// DefaultCapabilityMatcher implements CapabilityMatcher interface
type DefaultCapabilityMatcher struct{}

// NewCapabilityMatcher creates a new capability matcher
func NewCapabilityMatcher() *DefaultCapabilityMatcher {
	return &DefaultCapabilityMatcher{}
}

// Match checks if an agent has all required capabilities
func (m *DefaultCapabilityMatcher) Match(required []string, available []models.Capability) bool {
	if len(required) == 0 {
		return true
	}

	// Build a set of available capability names
	availableSet := make(map[string]bool)
	for _, cap := range available {
		availableSet[cap.Name] = true
		// Also add lowercase version for case-insensitive matching
		availableSet[strings.ToLower(cap.Name)] = true
	}

	// Check all required capabilities are available
	for _, req := range required {
		if !availableSet[req] && !availableSet[strings.ToLower(req)] {
			return false
		}
	}

	return true
}

// Score calculates how well an agent matches the required capabilities
// Returns a score between 0 and 1, where 1 is a perfect match
func (m *DefaultCapabilityMatcher) Score(required []string, available []models.Capability) float64 {
	if len(required) == 0 {
		return 1.0 // No requirements means perfect match
	}

	if len(available) == 0 {
		return 0.0 // No capabilities means no match
	}

	// Build a map of available capabilities with versions
	availableMap := make(map[string]models.Capability)
	for _, cap := range available {
		availableMap[cap.Name] = cap
		availableMap[strings.ToLower(cap.Name)] = cap
	}

	matchedCount := 0
	for _, req := range required {
		if _, ok := availableMap[req]; ok {
			matchedCount++
		} else if _, ok := availableMap[strings.ToLower(req)]; ok {
			matchedCount++
		}
	}

	// Base score is percentage of matched requirements
	baseScore := float64(matchedCount) / float64(len(required))

	// Bonus for having extra capabilities (up to 10% bonus)
	extraCapabilities := len(available) - matchedCount
	if extraCapabilities > 0 {
		bonus := float64(extraCapabilities) * 0.01
		if bonus > 0.1 {
			bonus = 0.1
		}
		baseScore += bonus
	}

	if baseScore > 1.0 {
		baseScore = 1.0
	}

	return baseScore
}

// MatchWithVersion checks if capabilities match including version requirements
func (m *DefaultCapabilityMatcher) MatchWithVersion(required []models.Capability, available []models.Capability) bool {
	availableMap := make(map[string]models.Capability)
	for _, cap := range available {
		availableMap[cap.Name] = cap
	}

	for _, req := range required {
		avail, ok := availableMap[req.Name]
		if !ok {
			return false
		}

		// If version is specified, check compatibility
		if req.Version != "" && avail.Version != "" {
			if !isVersionCompatible(req.Version, avail.Version) {
				return false
			}
		}
	}

	return true
}

// isVersionCompatible checks if available version satisfies required version
// Simple implementation - can be extended for semver comparison
func isVersionCompatible(required, available string) bool {
	// Simple exact match or prefix match
	if required == available {
		return true
	}

	// Check if available version starts with required major version
	reqParts := strings.Split(required, ".")
	availParts := strings.Split(available, ".")

	if len(reqParts) > 0 && len(availParts) > 0 {
		return reqParts[0] == availParts[0]
	}

	return false
}

// DefaultLoadBalancer implements LoadBalancer interface
type DefaultLoadBalancer struct {
	strategy LoadBalancerStrategy
	matcher  CapabilityMatcher
}

// NewLoadBalancer creates a new load balancer with the specified strategy
func NewLoadBalancer(strategy LoadBalancerStrategy) *DefaultLoadBalancer {
	return &DefaultLoadBalancer{
		strategy: strategy,
		matcher:  NewCapabilityMatcher(),
	}
}

// SelectAgent selects the best agent for a task based on strategy
func (lb *DefaultLoadBalancer) SelectAgent(ctx context.Context, agents []AgentInfo, task models.Task) (*AgentInfo, error) {
	// Filter agents by capability
	eligible := lb.filterByCapability(agents, task.RequiredCapabilities)
	if len(eligible) == 0 {
		return nil, &NoAgentError{
			Capabilities: task.RequiredCapabilities,
			Message:      "no agents available with required capabilities",
		}
	}

	// Filter out unhealthy agents
	healthy := lb.filterByHealth(eligible)
	if len(healthy) == 0 {
		return nil, &NoAgentError{
			Capabilities: task.RequiredCapabilities,
			Message:      "no healthy agents available",
		}
	}

	// Apply load balancing strategy
	switch lb.strategy {
	case LeastLoaded:
		return lb.selectLeastLoaded(healthy), nil
	case Random:
		return lb.selectRandom(healthy), nil
	case RoundRobin:
		return lb.selectRoundRobin(healthy), nil
	case WeightedRound:
		return lb.selectWeighted(healthy), nil
	default:
		return lb.selectLeastLoaded(healthy), nil
	}
}

func (lb *DefaultLoadBalancer) filterByCapability(agents []AgentInfo, required []string) []AgentInfo {
	if len(required) == 0 {
		return agents
	}

	result := make([]AgentInfo, 0)
	for _, agent := range agents {
		if lb.matcher.Match(required, agent.Capabilities) {
			result = append(result, agent)
		}
	}
	return result
}

func (lb *DefaultLoadBalancer) filterByHealth(agents []AgentInfo) []AgentInfo {
	result := make([]AgentInfo, 0)
	for _, agent := range agents {
		if agent.Status.Health == models.HealthHealthy &&
			agent.Status.State == AgentStateRunning {
			result = append(result, agent)
		}
	}
	return result
}

func (lb *DefaultLoadBalancer) selectLeastLoaded(agents []AgentInfo) *AgentInfo {
	if len(agents) == 0 {
		return nil
	}

	// Sort by load
	sorted := make([]AgentInfo, len(agents))
	copy(sorted, agents)

	sort.Slice(sorted, func(i, j int) bool {
		loadI := float64(sorted[i].Load.ActiveTasks) + sorted[i].Load.CPUPercent/100
		loadJ := float64(sorted[j].Load.ActiveTasks) + sorted[j].Load.CPUPercent/100
		return loadI < loadJ
	})

	return &sorted[0]
}

func (lb *DefaultLoadBalancer) selectRandom(agents []AgentInfo) *AgentInfo {
	if len(agents) == 0 {
		return nil
	}
	// Simple pseudo-random based on current time
	idx := int(models.LoadMetrics{}.ActiveTasks+1) % len(agents)
	return &agents[idx]
}

var roundRobinCounter int

func (lb *DefaultLoadBalancer) selectRoundRobin(agents []AgentInfo) *AgentInfo {
	if len(agents) == 0 {
		return nil
	}
	roundRobinCounter++
	return &agents[roundRobinCounter%len(agents)]
}

func (lb *DefaultLoadBalancer) selectWeighted(agents []AgentInfo) *AgentInfo {
	if len(agents) == 0 {
		return nil
	}

	// Calculate weights based on inverse of load
	type weightedAgent struct {
		agent  AgentInfo
		weight float64
	}

	weighted := make([]weightedAgent, len(agents))
	totalWeight := 0.0

	for i, agent := range agents {
		// Weight is inverse of load (lower load = higher weight)
		load := float64(agent.Load.ActiveTasks+1) + agent.Load.CPUPercent/100
		weight := 1.0 / load
		weighted[i] = weightedAgent{agent: agent, weight: weight}
		totalWeight += weight
	}

	// Select based on weight distribution
	// Simple approach: return highest weighted
	best := weighted[0]
	for _, w := range weighted[1:] {
		if w.weight > best.weight {
			best = w
		}
	}

	return &best.agent
}

// NoAgentError is returned when no suitable agent is found
type NoAgentError struct {
	Capabilities []string
	Message      string
}

func (e *NoAgentError) Error() string {
	return e.Message + ": " + strings.Join(e.Capabilities, ", ")
}

// TaskRouter handles routing tasks to appropriate agents
type TaskRouter struct {
	registry     Registry
	loadBalancer *DefaultLoadBalancer
	matcher      *DefaultCapabilityMatcher
}

// NewTaskRouter creates a new task router
func NewTaskRouter(registry Registry, strategy LoadBalancerStrategy) *TaskRouter {
	return &TaskRouter{
		registry:     registry,
		loadBalancer: NewLoadBalancer(strategy),
		matcher:      NewCapabilityMatcher(),
	}
}

// Route finds the best agent for a task
func (tr *TaskRouter) Route(ctx context.Context, task models.Task) (*AgentInfo, error) {
	// Get all agents
	agents, err := tr.registry.ListAllAgents(ctx)
	if err != nil {
		return nil, err
	}

	// Use load balancer to select
	return tr.loadBalancer.SelectAgent(ctx, agents, task)
}

// RouteToType finds the best agent of a specific type for a task
func (tr *TaskRouter) RouteToType(ctx context.Context, task models.Task, agentType models.AgentType) (*AgentInfo, error) {
	// Get agents of specific type
	agents, err := tr.registry.ListAgentsByType(ctx, agentType)
	if err != nil {
		return nil, err
	}

	// Use load balancer to select
	return tr.loadBalancer.SelectAgent(ctx, agents, task)
}
