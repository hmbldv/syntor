// +build property

package property

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
	"github.com/syntor/syntor/pkg/models"
	"github.com/syntor/syntor/pkg/registry"
)

// **Feature: syntor-multi-agent-system, Property 23: Automatic agent registration**
// *For any* agent startup, the agent should automatically register itself with its capabilities and endpoints in the service registry
// **Validates: Requirements 8.1**

// **Feature: syntor-multi-agent-system, Property 16: Capability-based task routing**
// *For any* task with required capabilities, the system should route it to agents that possess those capabilities
// **Validates: Requirements 5.4**

// **Feature: syntor-multi-agent-system, Property 24: Registry status updates**
// *For any* change in agent status or load, the service registry should be updated to reflect the current state
// **Validates: Requirements 8.2**

// MockRegistry implements Registry interface for testing
type MockRegistry struct {
	mu       sync.RWMutex
	agents   map[string]registry.AgentInfo
	statuses map[string]registry.AgentStatus
	loads    map[string]models.LoadMetrics
	health   models.HealthStatus
}

func NewMockRegistry() *MockRegistry {
	return &MockRegistry{
		agents:   make(map[string]registry.AgentInfo),
		statuses: make(map[string]registry.AgentStatus),
		loads:    make(map[string]models.LoadMetrics),
		health:   models.HealthHealthy,
	}
}

func (m *MockRegistry) Connect(ctx context.Context) error {
	return nil
}

func (m *MockRegistry) Close() error {
	return nil
}

func (m *MockRegistry) Health() models.HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.health
}

func (m *MockRegistry) RegisterAgent(ctx context.Context, agent registry.AgentInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent.RegisteredAt = time.Now()
	agent.LastSeen = time.Now()
	m.agents[agent.ID] = agent
	m.statuses[agent.ID] = registry.AgentStatus{
		State:       registry.AgentStateRunning,
		Health:      models.HealthHealthy,
		LastUpdated: time.Now(),
	}
	return nil
}

func (m *MockRegistry) DeregisterAgent(ctx context.Context, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.agents, agentID)
	delete(m.statuses, agentID)
	delete(m.loads, agentID)
	return nil
}

func (m *MockRegistry) UpdateAgentStatus(ctx context.Context, agentID string, status registry.AgentStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.agents[agentID]; !ok {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	m.statuses[agentID] = status

	agent := m.agents[agentID]
	agent.Status = status
	agent.LastSeen = time.Now()
	m.agents[agentID] = agent

	return nil
}

func (m *MockRegistry) UpdateAgentLoad(ctx context.Context, agentID string, load models.LoadMetrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.agents[agentID]; !ok {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	m.loads[agentID] = load

	agent := m.agents[agentID]
	agent.Load = load
	agent.LastSeen = time.Now()
	m.agents[agentID] = agent

	return nil
}

func (m *MockRegistry) GetAgent(ctx context.Context, agentID string) (*registry.AgentInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, ok := m.agents[agentID]
	if !ok {
		return nil, nil
	}
	return &agent, nil
}

func (m *MockRegistry) GetAgentStatus(ctx context.Context, agentID string) (*registry.AgentStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status, ok := m.statuses[agentID]
	if !ok {
		return nil, nil
	}
	return &status, nil
}

func (m *MockRegistry) ListAllAgents(ctx context.Context) ([]registry.AgentInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]registry.AgentInfo, 0, len(m.agents))
	for _, agent := range m.agents {
		agents = append(agents, agent)
	}
	return agents, nil
}

func (m *MockRegistry) ListAgentsByType(ctx context.Context, agentType models.AgentType) ([]registry.AgentInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]registry.AgentInfo, 0)
	for _, agent := range m.agents {
		if agent.Type == agentType {
			agents = append(agents, agent)
		}
	}
	return agents, nil
}

func (m *MockRegistry) FindAgentsByCapability(ctx context.Context, capability string) ([]registry.AgentInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]registry.AgentInfo, 0)
	for _, agent := range m.agents {
		for _, cap := range agent.Capabilities {
			if cap.Name == capability {
				agents = append(agents, agent)
				break
			}
		}
	}
	return agents, nil
}

func (m *MockRegistry) Heartbeat(ctx context.Context, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if agent, ok := m.agents[agentID]; ok {
		agent.LastSeen = time.Now()
		m.agents[agentID] = agent
	}
	return nil
}

func (m *MockRegistry) GetStaleAgents(ctx context.Context, threshold time.Duration) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stale := make([]string, 0)
	now := time.Now()
	for id, agent := range m.agents {
		if now.Sub(agent.LastSeen) > threshold {
			stale = append(stale, id)
		}
	}
	return stale, nil
}

func (m *MockRegistry) CleanupStaleAgents(ctx context.Context, threshold time.Duration) error {
	stale, _ := m.GetStaleAgents(ctx, threshold)
	for _, id := range stale {
		m.DeregisterAgent(ctx, id)
	}
	return nil
}

// TestAutomaticAgentRegistration tests Property 23
func TestAutomaticAgentRegistration(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Registered agents can be retrieved
	properties.Property("registered agents are retrievable", prop.ForAll(
		func(agentIdx int, nameIdx int) bool {
			agentID := fmt.Sprintf("agent-%d", agentIdx)
			agentName := fmt.Sprintf("Agent-%d", nameIdx)

			reg := NewMockRegistry()
			ctx := context.Background()

			agent := registry.AgentInfo{
				ID:   agentID,
				Name: agentName,
				Type: models.ServiceAgentType,
				Capabilities: []models.Capability{
					{Name: "test-capability", Version: "1.0"},
				},
				Status: registry.AgentStatus{
					State:  registry.AgentStateRunning,
					Health: models.HealthHealthy,
				},
			}

			err := reg.RegisterAgent(ctx, agent)
			if err != nil {
				return false
			}

			retrieved, err := reg.GetAgent(ctx, agentID)
			if err != nil || retrieved == nil {
				return false
			}

			return retrieved.ID == agentID && retrieved.Name == agentName
		},
		gen.IntRange(1, 10000),
		gen.IntRange(1, 10000),
	))

	// Property: Agent capabilities are preserved after registration
	properties.Property("capabilities preserved after registration", prop.ForAll(
		func(capCount int) bool {
			if capCount <= 0 || capCount > 10 {
				capCount = 3
			}

			reg := NewMockRegistry()
			ctx := context.Background()

			caps := make([]models.Capability, capCount)
			for i := 0; i < capCount; i++ {
				caps[i] = models.Capability{
					Name:    fmt.Sprintf("cap-%d", i),
					Version: "1.0",
				}
			}

			agent := registry.AgentInfo{
				ID:           "test-agent",
				Name:         "Test Agent",
				Type:         models.WorkerAgentType,
				Capabilities: caps,
			}

			reg.RegisterAgent(ctx, agent)
			retrieved, _ := reg.GetAgent(ctx, "test-agent")

			if retrieved == nil {
				return false
			}

			return len(retrieved.Capabilities) == capCount
		},
		gen.IntRange(1, 10),
	))

	// Property: Registration timestamp is set
	properties.Property("registration timestamp is set", prop.ForAll(
		func(agentID string) bool {
			if agentID == "" {
				return true
			}

			reg := NewMockRegistry()
			ctx := context.Background()

			before := time.Now()
			agent := registry.AgentInfo{
				ID:   agentID,
				Name: "Test Agent",
				Type: models.WorkerAgentType,
			}

			reg.RegisterAgent(ctx, agent)
			after := time.Now()

			retrieved, _ := reg.GetAgent(ctx, agentID)
			if retrieved == nil {
				return false
			}

			return !retrieved.RegisteredAt.Before(before) && !retrieved.RegisteredAt.After(after)
		},
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
	))

	// Property: Deregistered agents are not retrievable
	properties.Property("deregistered agents not retrievable", prop.ForAll(
		func(agentID string) bool {
			if agentID == "" {
				return true
			}

			reg := NewMockRegistry()
			ctx := context.Background()

			agent := registry.AgentInfo{
				ID:   agentID,
				Name: "Test Agent",
				Type: models.WorkerAgentType,
			}

			reg.RegisterAgent(ctx, agent)
			reg.DeregisterAgent(ctx, agentID)

			retrieved, _ := reg.GetAgent(ctx, agentID)
			return retrieved == nil
		},
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
	))

	properties.TestingRun(t)
}

// TestCapabilityBasedRouting tests Property 16
func TestCapabilityBasedRouting(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Agents are found by capability
	properties.Property("agents found by capability", prop.ForAll(
		func(capName string, agentCount int) bool {
			if capName == "" || agentCount <= 0 || agentCount > 20 {
				return true
			}

			reg := NewMockRegistry()
			ctx := context.Background()

			// Register agents with the capability
			for i := 0; i < agentCount; i++ {
				agent := registry.AgentInfo{
					ID:   fmt.Sprintf("agent-%d", i),
					Name: fmt.Sprintf("Agent %d", i),
					Type: models.WorkerAgentType,
					Capabilities: []models.Capability{
						{Name: capName, Version: "1.0"},
					},
					Status: registry.AgentStatus{
						State:  registry.AgentStateRunning,
						Health: models.HealthHealthy,
					},
				}
				reg.RegisterAgent(ctx, agent)
			}

			found, err := reg.FindAgentsByCapability(ctx, capName)
			if err != nil {
				return false
			}

			return len(found) == agentCount
		},
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" && len(s) < 30 }),
		gen.IntRange(1, 20),
	))

	// Property: Capability matcher correctly matches requirements
	properties.Property("capability matcher works correctly", prop.ForAll(
		func(requiredCount int, availableCount int) bool {
			if requiredCount < 0 || availableCount < 0 {
				return true
			}
			if requiredCount > 10 {
				requiredCount = 10
			}
			if availableCount > 10 {
				availableCount = 10
			}

			matcher := registry.NewCapabilityMatcher()

			required := make([]string, requiredCount)
			for i := 0; i < requiredCount; i++ {
				required[i] = fmt.Sprintf("cap-%d", i)
			}

			available := make([]models.Capability, availableCount)
			for i := 0; i < availableCount; i++ {
				available[i] = models.Capability{Name: fmt.Sprintf("cap-%d", i)}
			}

			matched := matcher.Match(required, available)

			// Should match if all required capabilities are available
			expectedMatch := requiredCount <= availableCount
			return matched == expectedMatch
		},
		gen.IntRange(0, 10),
		gen.IntRange(0, 10),
	))

	// Property: Score is between 0 and 1
	properties.Property("capability score is normalized", prop.ForAll(
		func(requiredCount int, availableCount int) bool {
			if requiredCount < 0 {
				requiredCount = 0
			}
			if availableCount < 0 {
				availableCount = 0
			}
			if requiredCount > 10 {
				requiredCount = 10
			}
			if availableCount > 10 {
				availableCount = 10
			}

			matcher := registry.NewCapabilityMatcher()

			required := make([]string, requiredCount)
			for i := 0; i < requiredCount; i++ {
				required[i] = fmt.Sprintf("cap-%d", i)
			}

			available := make([]models.Capability, availableCount)
			for i := 0; i < availableCount; i++ {
				available[i] = models.Capability{Name: fmt.Sprintf("cap-%d", i)}
			}

			score := matcher.Score(required, available)

			return score >= 0.0 && score <= 1.0
		},
		gen.IntRange(0, 10),
		gen.IntRange(0, 10),
	))

	// Property: Load balancer selects least loaded agent
	properties.Property("load balancer selects least loaded", prop.ForAll(
		func(agentCount int) bool {
			if agentCount <= 1 || agentCount > 10 {
				return true
			}

			lb := registry.NewLoadBalancer(registry.LeastLoaded)

			agents := make([]registry.AgentInfo, agentCount)
			minLoad := agentCount // Start with max possible
			minLoadIdx := 0

			for i := 0; i < agentCount; i++ {
				load := (i * 7) % agentCount // Pseudo-random load distribution
				if load < minLoad {
					minLoad = load
					minLoadIdx = i
				}

				agents[i] = registry.AgentInfo{
					ID:   fmt.Sprintf("agent-%d", i),
					Type: models.WorkerAgentType,
					Capabilities: []models.Capability{
						{Name: "test-cap"},
					},
					Status: registry.AgentStatus{
						State:  registry.AgentStateRunning,
						Health: models.HealthHealthy,
					},
					Load: models.LoadMetrics{
						ActiveTasks: load,
					},
				}
			}

			task := models.Task{
				RequiredCapabilities: []string{"test-cap"},
			}

			selected, err := lb.SelectAgent(context.Background(), agents, task)
			if err != nil {
				return false
			}

			return selected.ID == fmt.Sprintf("agent-%d", minLoadIdx)
		},
		gen.IntRange(2, 10),
	))

	properties.TestingRun(t)
}

// TestRegistryStatusUpdates tests Property 24
func TestRegistryStatusUpdates(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Status updates are reflected immediately
	properties.Property("status updates reflected immediately", prop.ForAll(
		func(stateIdx int) bool {
			states := []registry.AgentState{
				registry.AgentStateStarting,
				registry.AgentStateRunning,
				registry.AgentStateStopping,
				registry.AgentStateStopped,
				registry.AgentStateFailed,
			}

			idx := stateIdx % len(states)
			if idx < 0 {
				idx = -idx
			}

			reg := NewMockRegistry()
			ctx := context.Background()

			agent := registry.AgentInfo{
				ID:   "test-agent",
				Name: "Test Agent",
				Type: models.WorkerAgentType,
			}
			reg.RegisterAgent(ctx, agent)

			newStatus := registry.AgentStatus{
				State:       states[idx],
				Health:      models.HealthHealthy,
				LastUpdated: time.Now(),
			}

			err := reg.UpdateAgentStatus(ctx, "test-agent", newStatus)
			if err != nil {
				return false
			}

			retrieved, _ := reg.GetAgentStatus(ctx, "test-agent")
			if retrieved == nil {
				return false
			}

			return retrieved.State == states[idx]
		},
		gen.Int(),
	))

	// Property: Load updates are reflected
	properties.Property("load updates reflected", prop.ForAll(
		func(activeTasks int, cpuPercent float64) bool {
			if activeTasks < 0 {
				activeTasks = 0
			}
			if activeTasks > 1000 {
				activeTasks = 1000
			}
			if cpuPercent < 0 {
				cpuPercent = 0
			}
			if cpuPercent > 100 {
				cpuPercent = 100
			}

			reg := NewMockRegistry()
			ctx := context.Background()

			agent := registry.AgentInfo{
				ID:   "test-agent",
				Name: "Test Agent",
				Type: models.WorkerAgentType,
			}
			reg.RegisterAgent(ctx, agent)

			newLoad := models.LoadMetrics{
				ActiveTasks: activeTasks,
				CPUPercent:  cpuPercent,
			}

			err := reg.UpdateAgentLoad(ctx, "test-agent", newLoad)
			if err != nil {
				return false
			}

			retrieved, _ := reg.GetAgent(ctx, "test-agent")
			if retrieved == nil {
				return false
			}

			return retrieved.Load.ActiveTasks == activeTasks
		},
		gen.IntRange(0, 1000),
		gen.Float64Range(0, 100),
	))

	// Property: Heartbeat updates last seen time
	properties.Property("heartbeat updates last seen", prop.ForAll(
		func(_ int) bool {
			reg := NewMockRegistry()
			ctx := context.Background()

			agent := registry.AgentInfo{
				ID:   "test-agent",
				Name: "Test Agent",
				Type: models.WorkerAgentType,
			}
			reg.RegisterAgent(ctx, agent)

			// Wait a bit
			time.Sleep(time.Millisecond)

			beforeHeartbeat := time.Now()
			reg.Heartbeat(ctx, "test-agent")
			afterHeartbeat := time.Now()

			retrieved, _ := reg.GetAgent(ctx, "test-agent")
			if retrieved == nil {
				return false
			}

			return !retrieved.LastSeen.Before(beforeHeartbeat) &&
				!retrieved.LastSeen.After(afterHeartbeat)
		},
		gen.Int(),
	))

	// Property: Stale agents are detected
	properties.Property("stale agents detected", prop.ForAll(
		func(agentCount int, staleCount int) bool {
			if agentCount <= 0 || agentCount > 20 {
				agentCount = 5
			}
			if staleCount < 0 || staleCount > agentCount {
				staleCount = agentCount / 2
			}

			reg := NewMockRegistry()
			ctx := context.Background()

			threshold := 100 * time.Millisecond

			// Register agents
			for i := 0; i < agentCount; i++ {
				agent := registry.AgentInfo{
					ID:       fmt.Sprintf("agent-%d", i),
					Name:     fmt.Sprintf("Agent %d", i),
					Type:     models.WorkerAgentType,
					LastSeen: time.Now(),
				}
				reg.RegisterAgent(ctx, agent)
			}

			// Make some agents stale by setting old LastSeen
			reg.mu.Lock()
			count := 0
			for id := range reg.agents {
				if count < staleCount {
					agent := reg.agents[id]
					agent.LastSeen = time.Now().Add(-2 * threshold)
					reg.agents[id] = agent
					count++
				}
			}
			reg.mu.Unlock()

			stale, err := reg.GetStaleAgents(ctx, threshold)
			if err != nil {
				return false
			}

			return len(stale) == staleCount
		},
		gen.IntRange(1, 20),
		gen.IntRange(0, 10),
	))

	properties.TestingRun(t)
}

// TestAgentTypeFiltering tests agent type filtering
func TestAgentTypeFiltering(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Agents filtered by type correctly
	properties.Property("type filtering works", prop.ForAll(
		func(serviceCount int, workerCount int) bool {
			if serviceCount < 0 {
				serviceCount = 0
			}
			if workerCount < 0 {
				workerCount = 0
			}
			if serviceCount > 20 {
				serviceCount = 20
			}
			if workerCount > 20 {
				workerCount = 20
			}

			reg := NewMockRegistry()
			ctx := context.Background()

			// Register service agents
			for i := 0; i < serviceCount; i++ {
				agent := registry.AgentInfo{
					ID:   fmt.Sprintf("service-%d", i),
					Type: models.ServiceAgentType,
				}
				reg.RegisterAgent(ctx, agent)
			}

			// Register worker agents
			for i := 0; i < workerCount; i++ {
				agent := registry.AgentInfo{
					ID:   fmt.Sprintf("worker-%d", i),
					Type: models.WorkerAgentType,
				}
				reg.RegisterAgent(ctx, agent)
			}

			services, _ := reg.ListAgentsByType(ctx, models.ServiceAgentType)
			workers, _ := reg.ListAgentsByType(ctx, models.WorkerAgentType)

			return len(services) == serviceCount && len(workers) == workerCount
		},
		gen.IntRange(0, 20),
		gen.IntRange(0, 20),
	))

	properties.TestingRun(t)
}

// Unit tests for specific functionality
func TestCapabilityMatcherUnit(t *testing.T) {
	matcher := registry.NewCapabilityMatcher()

	t.Run("empty requirements match any agent", func(t *testing.T) {
		available := []models.Capability{{Name: "cap1"}}
		assert.True(t, matcher.Match([]string{}, available))
	})

	t.Run("exact match works", func(t *testing.T) {
		available := []models.Capability{{Name: "cap1"}, {Name: "cap2"}}
		assert.True(t, matcher.Match([]string{"cap1", "cap2"}, available))
	})

	t.Run("missing capability fails", func(t *testing.T) {
		available := []models.Capability{{Name: "cap1"}}
		assert.False(t, matcher.Match([]string{"cap1", "cap2"}, available))
	})

	t.Run("score is 1.0 for perfect match", func(t *testing.T) {
		available := []models.Capability{{Name: "cap1"}}
		score := matcher.Score([]string{"cap1"}, available)
		assert.Equal(t, 1.0, score)
	})

	t.Run("score is low for no match", func(t *testing.T) {
		available := []models.Capability{{Name: "other"}}
		score := matcher.Score([]string{"cap1"}, available)
		// Score may have a small bonus for extra capabilities but base score is 0
		assert.Less(t, score, 0.2)
	})
}

func TestLoadBalancerUnit(t *testing.T) {
	t.Run("no agents returns error", func(t *testing.T) {
		lb := registry.NewLoadBalancer(registry.LeastLoaded)
		task := models.Task{RequiredCapabilities: []string{"cap1"}}

		_, err := lb.SelectAgent(context.Background(), []registry.AgentInfo{}, task)
		assert.Error(t, err)
	})

	t.Run("selects only healthy agents", func(t *testing.T) {
		lb := registry.NewLoadBalancer(registry.LeastLoaded)

		agents := []registry.AgentInfo{
			{
				ID:           "unhealthy",
				Capabilities: []models.Capability{{Name: "cap1"}},
				Status:       registry.AgentStatus{State: registry.AgentStateFailed, Health: models.HealthUnhealthy},
			},
			{
				ID:           "healthy",
				Capabilities: []models.Capability{{Name: "cap1"}},
				Status:       registry.AgentStatus{State: registry.AgentStateRunning, Health: models.HealthHealthy},
			},
		}

		task := models.Task{RequiredCapabilities: []string{"cap1"}}
		selected, err := lb.SelectAgent(context.Background(), agents, task)

		assert.NoError(t, err)
		assert.Equal(t, "healthy", selected.ID)
	})
}
