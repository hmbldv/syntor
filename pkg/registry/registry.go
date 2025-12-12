package registry

import (
	"context"
	"time"

	"github.com/syntor/syntor/pkg/models"
)

// Registry manages agent discovery and capability matching
type Registry interface {
	// Agent registration
	RegisterAgent(ctx context.Context, agent AgentInfo) error
	DeregisterAgent(ctx context.Context, agentID string) error
	UpdateAgentStatus(ctx context.Context, agentID string, status AgentStatus) error
	UpdateAgentLoad(ctx context.Context, agentID string, load models.LoadMetrics) error

	// Agent discovery
	GetAgent(ctx context.Context, agentID string) (*AgentInfo, error)
	GetAgentStatus(ctx context.Context, agentID string) (*AgentStatus, error)
	ListAllAgents(ctx context.Context) ([]AgentInfo, error)
	ListAgentsByType(ctx context.Context, agentType models.AgentType) ([]AgentInfo, error)
	FindAgentsByCapability(ctx context.Context, capability string) ([]AgentInfo, error)

	// Health and monitoring
	Heartbeat(ctx context.Context, agentID string) error
	GetStaleAgents(ctx context.Context, threshold time.Duration) ([]string, error)
	CleanupStaleAgents(ctx context.Context, threshold time.Duration) error

	// Lifecycle
	Connect(ctx context.Context) error
	Close() error
	Health() models.HealthStatus
}

// AgentInfo contains agent registration information
type AgentInfo struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Type         models.AgentType    `json:"type"`
	Capabilities []models.Capability `json:"capabilities"`
	Endpoint     string              `json:"endpoint"`
	Status       AgentStatus         `json:"status"`
	Load         models.LoadMetrics  `json:"load"`
	RegisteredAt time.Time           `json:"registered_at"`
	LastSeen     time.Time           `json:"last_seen"`
	Metadata     map[string]string   `json:"metadata,omitempty"`
}

// AgentStatus represents the current status of an agent
type AgentStatus struct {
	State        AgentState          `json:"state"`
	Health       models.HealthStatus `json:"health"`
	Message      string              `json:"message,omitempty"`
	LastUpdated  time.Time           `json:"last_updated"`
}

// AgentState represents the operational state of an agent
type AgentState string

const (
	AgentStateStarting   AgentState = "starting"
	AgentStateRunning    AgentState = "running"
	AgentStateStopping   AgentState = "stopping"
	AgentStateStopped    AgentState = "stopped"
	AgentStateFailed     AgentState = "failed"
	AgentStateDraining   AgentState = "draining"
)

// RegistryConfig holds configuration for the registry
type RegistryConfig struct {
	Redis           RedisConfig   `json:"redis"`
	HeartbeatTTL    time.Duration `json:"heartbeat_ttl"`
	CleanupInterval time.Duration `json:"cleanup_interval"`
	StaleThreshold  time.Duration `json:"stale_threshold"`
}

// RedisConfig holds Redis connection configuration for the registry
type RedisConfig struct {
	Address   string `json:"address"`
	Password  string `json:"password,omitempty"`
	DB        int    `json:"db"`
	KeyPrefix string `json:"key_prefix"`
}

// DefaultRegistryConfig returns default registry configuration
func DefaultRegistryConfig() RegistryConfig {
	return RegistryConfig{
		Redis: RedisConfig{
			Address:   "localhost:6379",
			DB:        0,
			KeyPrefix: "syntor:registry:",
		},
		HeartbeatTTL:    30 * time.Second,
		CleanupInterval: 60 * time.Second,
		StaleThreshold:  90 * time.Second,
	}
}

// CapabilityMatcher provides capability matching functionality
type CapabilityMatcher interface {
	Match(required []string, available []models.Capability) bool
	Score(required []string, available []models.Capability) float64
}

// LoadBalancer provides load balancing functionality for agent selection
type LoadBalancer interface {
	SelectAgent(ctx context.Context, agents []AgentInfo, task models.Task) (*AgentInfo, error)
}

// LoadBalancerStrategy defines load balancing strategies
type LoadBalancerStrategy string

const (
	RoundRobin    LoadBalancerStrategy = "round_robin"
	LeastLoaded   LoadBalancerStrategy = "least_loaded"
	Random        LoadBalancerStrategy = "random"
	WeightedRound LoadBalancerStrategy = "weighted_round"
)
