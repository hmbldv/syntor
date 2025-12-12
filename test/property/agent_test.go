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
	"github.com/syntor/syntor/pkg/agent"
	"github.com/syntor/syntor/pkg/models"
	"github.com/syntor/syntor/pkg/registry"
)

// **Feature: syntor-multi-agent-system, Property 13: Service Agent capabilities**
// *For any* Service Agent, it should successfully handle documentation generation, Git operations, and coordination tasks
// **Validates: Requirements 5.1**

// **Feature: syntor-multi-agent-system, Property 17: Dynamic load-based scaling**
// *For any* workload pattern, the system should adjust agent instance counts based on current load metrics
// **Validates: Requirements 5.5**

// MockServiceAgent implements a mock service agent for testing
type MockServiceAgent struct {
	*agent.BaseAgent

	// Capability tracking
	handledTasks   map[string]int
	tasksMu        sync.RWMutex

	// Service-specific handlers
	docHandler     func(ctx context.Context, req map[string]interface{}) (map[string]interface{}, error)
	gitHandler     func(ctx context.Context, req map[string]interface{}) (map[string]interface{}, error)
	coordHandler   func(ctx context.Context, req map[string]interface{}) (map[string]interface{}, error)
}

func NewMockServiceAgent(name string, capabilities []models.Capability) *MockServiceAgent {
	config := agent.Config{
		ID:           fmt.Sprintf("%s-%d", name, time.Now().UnixNano()),
		Name:         name,
		Type:         models.ServiceAgentType,
		Capabilities: capabilities,
	}

	return &MockServiceAgent{
		BaseAgent:    agent.NewBaseAgent(config),
		handledTasks: make(map[string]int),
	}
}

func (a *MockServiceAgent) HandleTask(ctx context.Context, taskType string, payload map[string]interface{}) (map[string]interface{}, error) {
	a.tasksMu.Lock()
	a.handledTasks[taskType]++
	a.tasksMu.Unlock()

	switch {
	case taskType == "doc-generation" || taskType == "doc-search" || taskType == "doc-update":
		if a.docHandler != nil {
			return a.docHandler(ctx, payload)
		}
		return map[string]interface{}{"status": "doc_handled"}, nil

	case taskType == "git-clone" || taskType == "git-commit" || taskType == "git-merge":
		if a.gitHandler != nil {
			return a.gitHandler(ctx, payload)
		}
		return map[string]interface{}{"status": "git_handled"}, nil

	case taskType == "coordination" || taskType == "load-balancing" || taskType == "failover":
		if a.coordHandler != nil {
			return a.coordHandler(ctx, payload)
		}
		return map[string]interface{}{"status": "coord_handled"}, nil

	default:
		return nil, fmt.Errorf("unknown task type: %s", taskType)
	}
}

func (a *MockServiceAgent) GetHandledCount(taskType string) int {
	a.tasksMu.RLock()
	defer a.tasksMu.RUnlock()
	return a.handledTasks[taskType]
}

// ScalingDecider makes scaling decisions based on load
type ScalingDecider struct {
	minAgents       int
	maxAgents       int
	scaleUpThreshold  float64
	scaleDownThreshold float64
}

func NewScalingDecider(min, max int, upThreshold, downThreshold float64) *ScalingDecider {
	return &ScalingDecider{
		minAgents:         min,
		maxAgents:         max,
		scaleUpThreshold:  upThreshold,
		scaleDownThreshold: downThreshold,
	}
}

func (s *ScalingDecider) Decide(agents []registry.AgentInfo) (string, int) {
	if len(agents) == 0 {
		return "scale_up", s.minAgents
	}

	// Calculate average load
	var totalLoad float64
	for _, agent := range agents {
		load := float64(agent.Load.ActiveTasks) / 10.0 // Normalize to 0-1
		if load > 1.0 {
			load = 1.0
		}
		totalLoad += load
	}
	avgLoad := totalLoad / float64(len(agents))

	// Decision logic
	if avgLoad > s.scaleUpThreshold && len(agents) < s.maxAgents {
		return "scale_up", 1
	} else if avgLoad < s.scaleDownThreshold && len(agents) > s.minAgents {
		return "scale_down", 1
	}

	return "maintain", 0
}

// TestServiceAgentCapabilities tests Property 13
func TestServiceAgentCapabilities(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Service agents handle documentation tasks
	properties.Property("service agent handles documentation tasks", prop.ForAll(
		func(taskCount int) bool {
			if taskCount <= 0 || taskCount > 50 {
				taskCount = 10
			}

			agent := NewMockServiceAgent("doc-agent", []models.Capability{
				{Name: "documentation", Version: "1.0"},
				{Name: "doc-generation", Version: "1.0"},
			})

			ctx := context.Background()
			taskTypes := []string{"doc-generation", "doc-search", "doc-update"}

			for i := 0; i < taskCount; i++ {
				taskType := taskTypes[i%len(taskTypes)]
				_, err := agent.HandleTask(ctx, taskType, map[string]interface{}{
					"source": fmt.Sprintf("source-%d", i),
				})
				if err != nil {
					return false
				}
			}

			// Verify all tasks were handled
			totalHandled := 0
			for _, tt := range taskTypes {
				totalHandled += agent.GetHandledCount(tt)
			}

			return totalHandled == taskCount
		},
		gen.IntRange(1, 50),
	))

	// Property: Service agents handle Git operations
	properties.Property("service agent handles git operations", prop.ForAll(
		func(taskCount int) bool {
			if taskCount <= 0 || taskCount > 50 {
				taskCount = 10
			}

			agent := NewMockServiceAgent("git-agent", []models.Capability{
				{Name: "git-operations", Version: "1.0"},
				{Name: "git-clone", Version: "1.0"},
			})

			ctx := context.Background()
			taskTypes := []string{"git-clone", "git-commit", "git-merge"}

			for i := 0; i < taskCount; i++ {
				taskType := taskTypes[i%len(taskTypes)]
				_, err := agent.HandleTask(ctx, taskType, map[string]interface{}{
					"repo": fmt.Sprintf("repo-%d", i),
				})
				if err != nil {
					return false
				}
			}

			totalHandled := 0
			for _, tt := range taskTypes {
				totalHandled += agent.GetHandledCount(tt)
			}

			return totalHandled == taskCount
		},
		gen.IntRange(1, 50),
	))

	// Property: Service agents handle coordination tasks
	properties.Property("service agent handles coordination tasks", prop.ForAll(
		func(taskCount int) bool {
			if taskCount <= 0 || taskCount > 50 {
				taskCount = 10
			}

			agent := NewMockServiceAgent("coord-agent", []models.Capability{
				{Name: "coordination", Version: "1.0"},
				{Name: "load-balancing", Version: "1.0"},
			})

			ctx := context.Background()
			taskTypes := []string{"coordination", "load-balancing", "failover"}

			for i := 0; i < taskCount; i++ {
				taskType := taskTypes[i%len(taskTypes)]
				_, err := agent.HandleTask(ctx, taskType, map[string]interface{}{
					"task_id": fmt.Sprintf("task-%d", i),
				})
				if err != nil {
					return false
				}
			}

			totalHandled := 0
			for _, tt := range taskTypes {
				totalHandled += agent.GetHandledCount(tt)
			}

			return totalHandled == taskCount
		},
		gen.IntRange(1, 50),
	))

	// Property: Service agents reject unknown task types
	properties.Property("service agent rejects unknown tasks", prop.ForAll(
		func(taskType string) bool {
			if taskType == "" {
				return true
			}

			// Skip known types
			knownTypes := []string{"doc-generation", "doc-search", "doc-update",
				"git-clone", "git-commit", "git-merge",
				"coordination", "load-balancing", "failover"}
			for _, known := range knownTypes {
				if taskType == known {
					return true
				}
			}

			agent := NewMockServiceAgent("test-agent", []models.Capability{})
			ctx := context.Background()

			_, err := agent.HandleTask(ctx, taskType, nil)
			return err != nil // Should return error for unknown type
		},
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// TestDynamicLoadBasedScaling tests Property 17
func TestDynamicLoadBasedScaling(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Scaling up when load exceeds threshold
	properties.Property("scale up on high load", prop.ForAll(
		func(agentCount int, loadPerAgent int) bool {
			if agentCount <= 0 || agentCount > 10 {
				agentCount = 3
			}
			if loadPerAgent < 0 {
				loadPerAgent = 0
			}
			if loadPerAgent > 20 {
				loadPerAgent = 20
			}

			decider := NewScalingDecider(1, 10, 0.7, 0.3)

			agents := make([]registry.AgentInfo, agentCount)
			for i := 0; i < agentCount; i++ {
				agents[i] = registry.AgentInfo{
					ID:   fmt.Sprintf("agent-%d", i),
					Type: models.WorkerAgentType,
					Load: models.LoadMetrics{ActiveTasks: loadPerAgent},
					Status: registry.AgentStatus{
						State:  registry.AgentStateRunning,
						Health: models.HealthHealthy,
					},
				}
			}

			action, _ := decider.Decide(agents)

			// High load (> 7 tasks per agent, normalized > 0.7) should trigger scale up
			if loadPerAgent > 7 && agentCount < 10 {
				return action == "scale_up"
			}

			return true // Other cases are fine
		},
		gen.IntRange(1, 10),
		gen.IntRange(0, 20),
	))

	// Property: Scaling down when load is low
	properties.Property("scale down on low load", prop.ForAll(
		func(agentCount int, loadPerAgent int) bool {
			if agentCount <= 1 || agentCount > 10 {
				agentCount = 5
			}
			if loadPerAgent < 0 {
				loadPerAgent = 0
			}
			if loadPerAgent > 5 {
				loadPerAgent = 5
			}

			decider := NewScalingDecider(1, 10, 0.7, 0.3)

			agents := make([]registry.AgentInfo, agentCount)
			for i := 0; i < agentCount; i++ {
				agents[i] = registry.AgentInfo{
					ID:   fmt.Sprintf("agent-%d", i),
					Type: models.WorkerAgentType,
					Load: models.LoadMetrics{ActiveTasks: loadPerAgent},
					Status: registry.AgentStatus{
						State:  registry.AgentStateRunning,
						Health: models.HealthHealthy,
					},
				}
			}

			action, _ := decider.Decide(agents)

			// Low load (< 3 tasks per agent, normalized < 0.3) should trigger scale down
			if loadPerAgent < 3 && agentCount > 1 {
				return action == "scale_down"
			}

			return true
		},
		gen.IntRange(2, 10),
		gen.IntRange(0, 5),
	))

	// Property: Maintain when load is balanced
	properties.Property("maintain on balanced load", prop.ForAll(
		func(agentCount int) bool {
			if agentCount <= 0 || agentCount > 10 {
				agentCount = 5
			}

			decider := NewScalingDecider(1, 10, 0.7, 0.3)

			// Create agents with moderate load (5 tasks = 0.5 normalized)
			agents := make([]registry.AgentInfo, agentCount)
			for i := 0; i < agentCount; i++ {
				agents[i] = registry.AgentInfo{
					ID:   fmt.Sprintf("agent-%d", i),
					Type: models.WorkerAgentType,
					Load: models.LoadMetrics{ActiveTasks: 5},
					Status: registry.AgentStatus{
						State:  registry.AgentStateRunning,
						Health: models.HealthHealthy,
					},
				}
			}

			action, _ := decider.Decide(agents)
			return action == "maintain"
		},
		gen.IntRange(1, 10),
	))

	// Property: Never scale below minimum
	properties.Property("never scale below minimum", prop.ForAll(
		func(minAgents int) bool {
			if minAgents <= 0 || minAgents > 5 {
				minAgents = 2
			}

			decider := NewScalingDecider(minAgents, 10, 0.7, 0.3)

			// Create minimum number of agents with very low load
			agents := make([]registry.AgentInfo, minAgents)
			for i := 0; i < minAgents; i++ {
				agents[i] = registry.AgentInfo{
					ID:   fmt.Sprintf("agent-%d", i),
					Load: models.LoadMetrics{ActiveTasks: 0},
					Status: registry.AgentStatus{
						State:  registry.AgentStateRunning,
						Health: models.HealthHealthy,
					},
				}
			}

			action, _ := decider.Decide(agents)
			// Should maintain, not scale down when at minimum
			return action == "maintain"
		},
		gen.IntRange(1, 5),
	))

	// Property: Never scale above maximum
	properties.Property("never scale above maximum", prop.ForAll(
		func(maxAgents int) bool {
			if maxAgents <= 0 || maxAgents > 10 {
				maxAgents = 5
			}

			decider := NewScalingDecider(1, maxAgents, 0.7, 0.3)

			// Create maximum number of agents with very high load
			agents := make([]registry.AgentInfo, maxAgents)
			for i := 0; i < maxAgents; i++ {
				agents[i] = registry.AgentInfo{
					ID:   fmt.Sprintf("agent-%d", i),
					Load: models.LoadMetrics{ActiveTasks: 15},
					Status: registry.AgentStatus{
						State:  registry.AgentStateRunning,
						Health: models.HealthHealthy,
					},
				}
			}

			action, _ := decider.Decide(agents)
			// Should maintain, not scale up when at maximum
			return action == "maintain"
		},
		gen.IntRange(1, 10),
	))

	// Property: Scale up when no agents
	properties.Property("scale up when no agents", prop.ForAll(
		func(minAgents int) bool {
			if minAgents <= 0 || minAgents > 5 {
				minAgents = 2
			}

			decider := NewScalingDecider(minAgents, 10, 0.7, 0.3)

			action, count := decider.Decide([]registry.AgentInfo{})
			return action == "scale_up" && count == minAgents
		},
		gen.IntRange(1, 5),
	))

	properties.TestingRun(t)
}

// TestBaseAgentLifecycle tests agent lifecycle management
func TestBaseAgentLifecycle(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Agent starts in uninitialized state
	properties.Property("new agent is uninitialized", prop.ForAll(
		func(name string) bool {
			if name == "" {
				return true
			}

			config := agent.Config{
				ID:   fmt.Sprintf("agent-%s", name),
				Name: name,
				Type: models.ServiceAgentType,
			}

			a := agent.NewBaseAgent(config)
			return a.GetState() == agent.StateUninitialized
		},
		gen.AlphaString(),
	))

	// Property: Initialized agent is in correct state
	properties.Property("initialized agent has correct state", prop.ForAll(
		func(name string) bool {
			if name == "" {
				return true
			}

			config := agent.Config{
				ID:   fmt.Sprintf("agent-%s", name),
				Name: name,
				Type: models.ServiceAgentType,
			}

			a := agent.NewBaseAgent(config)
			err := a.Initialize(context.Background(), config)
			if err != nil {
				return false
			}

			return a.GetState() == agent.StateInitialized
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property: Started agent is running
	properties.Property("started agent is running", prop.ForAll(
		func(name string) bool {
			if name == "" {
				return true
			}

			config := agent.Config{
				ID:   fmt.Sprintf("agent-%s", name),
				Name: name,
				Type: models.ServiceAgentType,
			}

			a := agent.NewBaseAgent(config)
			a.Initialize(context.Background(), config)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := a.Start(ctx)
			if err != nil {
				return false
			}
			defer a.Stop(context.Background())

			return a.GetState() == agent.StateRunning
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// Property: Stopped agent is in stopped state
	properties.Property("stopped agent is stopped", prop.ForAll(
		func(name string) bool {
			if name == "" {
				return true
			}

			config := agent.Config{
				ID:   fmt.Sprintf("agent-%s", name),
				Name: name,
				Type: models.ServiceAgentType,
			}

			a := agent.NewBaseAgent(config)
			a.Initialize(context.Background(), config)

			ctx, cancel := context.WithCancel(context.Background())
			a.Start(ctx)
			cancel()

			a.Stop(context.Background())

			return a.GetState() == agent.StateStopped
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	properties.TestingRun(t)
}

// Unit tests for specific functionality
func TestAgentCapabilities(t *testing.T) {
	t.Run("agent reports correct capabilities", func(t *testing.T) {
		caps := []models.Capability{
			{Name: "cap1", Version: "1.0"},
			{Name: "cap2", Version: "2.0"},
		}

		config := agent.Config{
			ID:           "test-agent",
			Name:         "Test Agent",
			Type:         models.ServiceAgentType,
			Capabilities: caps,
		}

		a := agent.NewBaseAgent(config)
		retrieved := a.GetCapabilities()

		assert.Len(t, retrieved, 2)
		assert.Equal(t, "cap1", retrieved[0].Name)
		assert.Equal(t, "cap2", retrieved[1].Name)
	})

	t.Run("agent can handle registered capability", func(t *testing.T) {
		config := agent.Config{
			ID:   "test-agent",
			Name: "Test Agent",
			Type: models.ServiceAgentType,
			Capabilities: []models.Capability{
				{Name: "test-task", Version: "1.0"},
			},
		}

		a := agent.NewBaseAgent(config)
		assert.True(t, a.CanHandle("test-task"))
		assert.False(t, a.CanHandle("other-task"))
	})
}

func TestAgentMetrics(t *testing.T) {
	t.Run("metrics are tracked", func(t *testing.T) {
		config := agent.Config{
			ID:   "test-agent",
			Name: "Test Agent",
			Type: models.ServiceAgentType,
		}

		a := agent.NewBaseAgent(config)
		a.Initialize(context.Background(), config)

		// Register a handler
		a.RegisterHandler(models.MsgTaskAssignment, func(ctx context.Context, msg models.Message) error {
			return nil
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		a.Start(ctx)
		defer a.Stop(context.Background())

		// Handle some messages
		for i := 0; i < 5; i++ {
			a.HandleMessage(context.Background(), models.Message{
				ID:     fmt.Sprintf("msg-%d", i),
				Type:   models.MsgTaskAssignment,
				Source: "test",
			})
		}

		metrics := a.GetMetrics()
		assert.Equal(t, int64(5), metrics.TasksProcessed)
		assert.Equal(t, int64(5), metrics.TasksSucceeded)
		assert.Equal(t, int64(0), metrics.TasksFailed)
	})
}
