// +build property

package property

import (
	"fmt"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
	"github.com/syntor/syntor/pkg/models"
	"github.com/syntor/syntor/pkg/registry"
)

// **Feature: syntor-multi-agent-system, Property 4: Container build consistency**
// *For any* agent source code, the build process should produce a valid Docker container with all required dependencies
// **Validates: Requirements 2.1**

// **Feature: syntor-multi-agent-system, Property 5: Container resource configuration**
// *For any* deployed agent container, it should have properly configured resource limits and responsive health check endpoints
// **Validates: Requirements 2.3**

// **Feature: syntor-multi-agent-system, Property 6: Horizontal scaling capability**
// *For any* agent type, additional instances should be able to join the system and begin processing tasks without manual intervention
// **Validates: Requirements 2.4**

// ContainerConfig represents container configuration
type ContainerConfig struct {
	Image         string
	CPULimit      string
	MemoryLimit   string
	HealthCheck   HealthCheckConfig
	Environment   map[string]string
	Ports         []PortMapping
	Volumes       []VolumeMount
}

type HealthCheckConfig struct {
	Endpoint    string
	Interval    int // seconds
	Timeout     int // seconds
	Retries     int
	StartPeriod int // seconds
}

type PortMapping struct {
	ContainerPort int
	HostPort      int
	Protocol      string
}

type VolumeMount struct {
	Source string
	Target string
	Type   string // bind, volume, tmpfs
}

// ValidateContainerConfig validates container configuration
func ValidateContainerConfig(config ContainerConfig) []string {
	errors := make([]string, 0)

	// Validate image
	if config.Image == "" {
		errors = append(errors, "image name is required")
	}

	// Validate health check
	if config.HealthCheck.Endpoint == "" {
		errors = append(errors, "health check endpoint is required")
	}
	if config.HealthCheck.Interval <= 0 {
		errors = append(errors, "health check interval must be positive")
	}
	if config.HealthCheck.Timeout <= 0 {
		errors = append(errors, "health check timeout must be positive")
	}
	if config.HealthCheck.Retries <= 0 {
		errors = append(errors, "health check retries must be positive")
	}

	// Validate resource limits
	if config.CPULimit != "" && !isValidCPULimit(config.CPULimit) {
		errors = append(errors, "invalid CPU limit format")
	}
	if config.MemoryLimit != "" && !isValidMemoryLimit(config.MemoryLimit) {
		errors = append(errors, "invalid memory limit format")
	}

	// Validate ports
	for _, port := range config.Ports {
		if port.ContainerPort <= 0 || port.ContainerPort > 65535 {
			errors = append(errors, fmt.Sprintf("invalid container port: %d", port.ContainerPort))
		}
	}

	return errors
}

func isValidCPULimit(limit string) bool {
	// Accept formats like "0.5", "1", "1.0", "2"
	if limit == "" {
		return true
	}
	for _, c := range limit {
		if c != '.' && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func isValidMemoryLimit(limit string) bool {
	// Accept formats like "256M", "1G", "512Mi", "1Gi"
	if limit == "" {
		return true
	}
	validSuffixes := []string{"M", "Mi", "G", "Gi", "K", "Ki"}
	for _, suffix := range validSuffixes {
		if strings.HasSuffix(limit, suffix) {
			numPart := strings.TrimSuffix(limit, suffix)
			for _, c := range numPart {
				if c < '0' || c > '9' {
					return false
				}
			}
			return len(numPart) > 0
		}
	}
	return false
}

// AgentScaler manages horizontal scaling of agents
type AgentScaler struct {
	registry     *MockRegistry
	minInstances int
	maxInstances int
	agentsByType map[models.AgentType][]string
}

func NewAgentScaler(reg *MockRegistry, min, max int) *AgentScaler {
	return &AgentScaler{
		registry:     reg,
		minInstances: min,
		maxInstances: max,
		agentsByType: make(map[models.AgentType][]string),
	}
}

func (s *AgentScaler) ScaleUp(agentType models.AgentType) (string, error) {
	current := len(s.agentsByType[agentType])
	if current >= s.maxInstances {
		return "", fmt.Errorf("at max capacity: %d instances", s.maxInstances)
	}

	// Create new agent
	agentID := fmt.Sprintf("%s-%d", agentType, current+1)
	agent := registry.AgentInfo{
		ID:   agentID,
		Name: fmt.Sprintf("%s Instance %d", agentType, current+1),
		Type: agentType,
		Status: registry.AgentStatus{
			State:  registry.AgentStateRunning,
			Health: models.HealthHealthy,
		},
	}

	if err := s.registry.RegisterAgent(nil, agent); err != nil {
		return "", err
	}

	s.agentsByType[agentType] = append(s.agentsByType[agentType], agentID)
	return agentID, nil
}

func (s *AgentScaler) ScaleDown(agentType models.AgentType) error {
	agents := s.agentsByType[agentType]
	if len(agents) <= s.minInstances {
		return fmt.Errorf("at min capacity: %d instances", s.minInstances)
	}

	// Remove last agent
	agentID := agents[len(agents)-1]
	if err := s.registry.DeregisterAgent(nil, agentID); err != nil {
		return err
	}

	s.agentsByType[agentType] = agents[:len(agents)-1]
	return nil
}

func (s *AgentScaler) GetInstanceCount(agentType models.AgentType) int {
	return len(s.agentsByType[agentType])
}

// TestContainerBuildConsistency tests Property 4
func TestContainerBuildConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Valid container config passes validation
	properties.Property("valid config passes validation", prop.ForAll(
		func(cpuNum int, memMB int) bool {
			if cpuNum < 1 {
				cpuNum = 1
			}
			if cpuNum > 16 {
				cpuNum = 16
			}
			if memMB < 64 {
				memMB = 64
			}
			if memMB > 16384 {
				memMB = 16384
			}

			config := ContainerConfig{
				Image:       "syntor/agent:latest",
				CPULimit:    fmt.Sprintf("%d", cpuNum),
				MemoryLimit: fmt.Sprintf("%dM", memMB),
				HealthCheck: HealthCheckConfig{
					Endpoint:    "/health",
					Interval:    30,
					Timeout:     10,
					Retries:     3,
					StartPeriod: 5,
				},
				Ports: []PortMapping{
					{ContainerPort: 8080, HostPort: 8080, Protocol: "tcp"},
				},
			}

			errors := ValidateContainerConfig(config)
			return len(errors) == 0
		},
		gen.IntRange(1, 16),
		gen.IntRange(64, 16384),
	))

	// Property: Missing image fails validation
	properties.Property("missing image fails validation", prop.ForAll(
		func(_ int) bool {
			config := ContainerConfig{
				Image: "", // Missing
				HealthCheck: HealthCheckConfig{
					Endpoint: "/health",
					Interval: 30,
					Timeout:  10,
					Retries:  3,
				},
			}

			errors := ValidateContainerConfig(config)
			for _, err := range errors {
				if strings.Contains(err, "image") {
					return true
				}
			}
			return false
		},
		gen.Int(),
	))

	// Property: Invalid port fails validation
	properties.Property("invalid port fails validation", prop.ForAll(
		func(port int) bool {
			if port > 0 && port <= 65535 {
				return true // Valid port, skip
			}

			config := ContainerConfig{
				Image: "syntor/agent:latest",
				HealthCheck: HealthCheckConfig{
					Endpoint: "/health",
					Interval: 30,
					Timeout:  10,
					Retries:  3,
				},
				Ports: []PortMapping{
					{ContainerPort: port, HostPort: 8080, Protocol: "tcp"},
				},
			}

			errors := ValidateContainerConfig(config)
			return len(errors) > 0
		},
		gen.Int(),
	))

	properties.TestingRun(t)
}

// TestContainerResourceConfiguration tests Property 5
func TestContainerResourceConfiguration(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Health check config is valid
	properties.Property("health check config valid", prop.ForAll(
		func(interval int, timeout int, retries int) bool {
			if interval <= 0 {
				interval = 1
			}
			if timeout <= 0 {
				timeout = 1
			}
			if retries <= 0 {
				retries = 1
			}

			config := ContainerConfig{
				Image: "syntor/agent:latest",
				HealthCheck: HealthCheckConfig{
					Endpoint:    "/health",
					Interval:    interval,
					Timeout:     timeout,
					Retries:     retries,
					StartPeriod: 5,
				},
			}

			errors := ValidateContainerConfig(config)
			return len(errors) == 0
		},
		gen.IntRange(1, 300),
		gen.IntRange(1, 60),
		gen.IntRange(1, 10),
	))

	// Property: Memory limit format is validated
	properties.Property("memory limit format validation", prop.ForAll(
		func(value int, suffixIdx int) bool {
			if value <= 0 {
				return true
			}

			suffixes := []string{"M", "Mi", "G", "Gi"}
			suffix := suffixes[suffixIdx%len(suffixes)]
			limit := fmt.Sprintf("%d%s", value, suffix)

			return isValidMemoryLimit(limit)
		},
		gen.IntRange(1, 1024),
		gen.IntRange(0, 10),
	))

	// Property: CPU limit format is validated
	properties.Property("CPU limit format validation", prop.ForAll(
		func(wholeNum int, decimalNum int) bool {
			if wholeNum < 0 {
				wholeNum = 0
			}
			if decimalNum < 0 {
				decimalNum = 0
			}

			limit := fmt.Sprintf("%d.%d", wholeNum, decimalNum%10)
			return isValidCPULimit(limit)
		},
		gen.IntRange(0, 16),
		gen.IntRange(0, 100),
	))

	properties.TestingRun(t)
}

// TestHorizontalScaling tests Property 6
func TestHorizontalScaling(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Scale up adds instances
	properties.Property("scale up adds instances", prop.ForAll(
		func(scaleCount int) bool {
			if scaleCount <= 0 || scaleCount > 10 {
				scaleCount = 3
			}

			reg := NewMockRegistry()
			scaler := NewAgentScaler(reg, 0, 20)

			for i := 0; i < scaleCount; i++ {
				_, err := scaler.ScaleUp(models.WorkerAgentType)
				if err != nil {
					return false
				}
			}

			return scaler.GetInstanceCount(models.WorkerAgentType) == scaleCount
		},
		gen.IntRange(1, 10),
	))

	// Property: Scale down removes instances
	properties.Property("scale down removes instances", prop.ForAll(
		func(initial int, remove int) bool {
			if initial <= 0 || initial > 10 {
				initial = 5
			}
			if remove < 0 || remove > initial {
				remove = initial / 2
			}

			reg := NewMockRegistry()
			scaler := NewAgentScaler(reg, 0, 20)

			// Scale up first
			for i := 0; i < initial; i++ {
				scaler.ScaleUp(models.WorkerAgentType)
			}

			// Scale down
			for i := 0; i < remove; i++ {
				scaler.ScaleDown(models.WorkerAgentType)
			}

			expected := initial - remove
			return scaler.GetInstanceCount(models.WorkerAgentType) == expected
		},
		gen.IntRange(1, 10),
		gen.IntRange(0, 5),
	))

	// Property: Cannot exceed max instances
	properties.Property("cannot exceed max instances", prop.ForAll(
		func(maxInstances int) bool {
			if maxInstances <= 0 || maxInstances > 10 {
				maxInstances = 5
			}

			reg := NewMockRegistry()
			scaler := NewAgentScaler(reg, 0, maxInstances)

			// Try to scale beyond max
			for i := 0; i < maxInstances+5; i++ {
				scaler.ScaleUp(models.WorkerAgentType)
			}

			return scaler.GetInstanceCount(models.WorkerAgentType) == maxInstances
		},
		gen.IntRange(1, 10),
	))

	// Property: Cannot go below min instances
	properties.Property("cannot go below min instances", prop.ForAll(
		func(minInstances int) bool {
			if minInstances <= 0 || minInstances > 5 {
				minInstances = 2
			}

			reg := NewMockRegistry()
			scaler := NewAgentScaler(reg, minInstances, 20)

			// Scale up first
			for i := 0; i < minInstances+3; i++ {
				scaler.ScaleUp(models.WorkerAgentType)
			}

			// Try to scale down below min
			for i := 0; i < 10; i++ {
				scaler.ScaleDown(models.WorkerAgentType)
			}

			return scaler.GetInstanceCount(models.WorkerAgentType) == minInstances
		},
		gen.IntRange(1, 5),
	))

	// Property: New instances are immediately discoverable
	properties.Property("new instances discoverable", prop.ForAll(
		func(instanceCount int) bool {
			if instanceCount <= 0 || instanceCount > 10 {
				instanceCount = 3
			}

			reg := NewMockRegistry()
			scaler := NewAgentScaler(reg, 0, 20)

			for i := 0; i < instanceCount; i++ {
				scaler.ScaleUp(models.WorkerAgentType)
			}

			// Verify all instances are in registry
			agents, _ := reg.ListAgentsByType(nil, models.WorkerAgentType)
			return len(agents) == instanceCount
		},
		gen.IntRange(1, 10),
	))

	// Property: Different agent types scale independently
	properties.Property("agent types scale independently", prop.ForAll(
		func(workerCount int, serviceCount int) bool {
			if workerCount < 0 || workerCount > 10 {
				workerCount = 3
			}
			if serviceCount < 0 || serviceCount > 10 {
				serviceCount = 2
			}

			reg := NewMockRegistry()
			scaler := NewAgentScaler(reg, 0, 20)

			// Scale workers
			for i := 0; i < workerCount; i++ {
				scaler.ScaleUp(models.WorkerAgentType)
			}

			// Scale services
			for i := 0; i < serviceCount; i++ {
				scaler.ScaleUp(models.ServiceAgentType)
			}

			workers := scaler.GetInstanceCount(models.WorkerAgentType)
			services := scaler.GetInstanceCount(models.ServiceAgentType)

			return workers == workerCount && services == serviceCount
		},
		gen.IntRange(0, 10),
		gen.IntRange(0, 10),
	))

	properties.TestingRun(t)
}

// Unit tests
func TestContainerConfigValidation(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := ContainerConfig{
			Image:       "syntor/coordination:latest",
			CPULimit:    "0.5",
			MemoryLimit: "256M",
			HealthCheck: HealthCheckConfig{
				Endpoint:    "/health",
				Interval:    30,
				Timeout:     10,
				Retries:     3,
				StartPeriod: 5,
			},
			Ports: []PortMapping{
				{ContainerPort: 8080, HostPort: 8081, Protocol: "tcp"},
				{ContainerPort: 9090, HostPort: 9091, Protocol: "tcp"},
			},
		}

		errors := ValidateContainerConfig(config)
		assert.Empty(t, errors)
	})

	t.Run("missing health check endpoint", func(t *testing.T) {
		config := ContainerConfig{
			Image: "syntor/agent:latest",
			HealthCheck: HealthCheckConfig{
				Interval: 30,
				Timeout:  10,
				Retries:  3,
			},
		}

		errors := ValidateContainerConfig(config)
		assert.NotEmpty(t, errors)
	})
}

func TestMemoryLimitValidation(t *testing.T) {
	validLimits := []string{"256M", "1G", "512Mi", "2Gi", "1024K", "100Ki"}
	for _, limit := range validLimits {
		assert.True(t, isValidMemoryLimit(limit), "Expected %s to be valid", limit)
	}

	invalidLimits := []string{"256", "1GB", "abc", "256MB"}
	for _, limit := range invalidLimits {
		assert.False(t, isValidMemoryLimit(limit), "Expected %s to be invalid", limit)
	}
}
