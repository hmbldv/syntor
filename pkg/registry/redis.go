package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/syntor/syntor/pkg/models"
)

// RedisRegistry implements the Registry interface using Redis
type RedisRegistry struct {
	client    *redis.Client
	config    RegistryConfig
	mu        sync.RWMutex
	connected bool
	health    models.HealthStatus

	// Background cleanup
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Key prefixes for Redis
const (
	keyPrefixAgent       = "agent:"
	keyPrefixAgentStatus = "agent:status:"
	keyPrefixAgentLoad   = "agent:load:"
	keyPrefixCapability  = "capability:"
	keyAgentSet          = "agents:all"
	keyAgentTypeSet      = "agents:type:"
)

// NewRedisRegistry creates a new Redis-based registry
func NewRedisRegistry(config RegistryConfig) *RedisRegistry {
	return &RedisRegistry{
		config: config,
		health: models.HealthUnknown,
	}
}

// Connect establishes connection to Redis
func (r *RedisRegistry) Connect(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.connected {
		return nil
	}

	r.client = redis.NewClient(&redis.Options{
		Addr:     r.config.Redis.Address,
		Password: r.config.Redis.Password,
		DB:       r.config.Redis.DB,
	})

	// Test connection
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to redis: %w", err)
	}

	r.ctx, r.cancel = context.WithCancel(ctx)
	r.connected = true
	r.health = models.HealthHealthy

	// Start background cleanup
	r.wg.Add(1)
	go r.runCleanup()

	return nil
}

// Close shuts down the registry
func (r *RedisRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.connected {
		return nil
	}

	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()

	if r.client != nil {
		if err := r.client.Close(); err != nil {
			return fmt.Errorf("failed to close redis connection: %w", err)
		}
	}

	r.connected = false
	r.health = models.HealthUnknown

	return nil
}

// Health returns the current health status
func (r *RedisRegistry) Health() models.HealthStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.health
}

// RegisterAgent registers an agent with its capabilities
func (r *RedisRegistry) RegisterAgent(ctx context.Context, agent AgentInfo) error {
	r.mu.RLock()
	if !r.connected {
		r.mu.RUnlock()
		return fmt.Errorf("registry not connected")
	}
	r.mu.RUnlock()

	// Set registration time
	agent.RegisteredAt = time.Now()
	agent.LastSeen = time.Now()

	// Serialize agent info
	data, err := json.Marshal(agent)
	if err != nil {
		return fmt.Errorf("failed to serialize agent info: %w", err)
	}

	prefix := r.config.Redis.KeyPrefix

	// Use pipeline for atomic operations
	pipe := r.client.Pipeline()

	// Store agent info
	pipe.Set(ctx, prefix+keyPrefixAgent+agent.ID, data, 0)

	// Add to agent set
	pipe.SAdd(ctx, prefix+keyAgentSet, agent.ID)

	// Add to type-specific set
	pipe.SAdd(ctx, prefix+keyAgentTypeSet+string(agent.Type), agent.ID)

	// Index by capabilities
	for _, cap := range agent.Capabilities {
		pipe.SAdd(ctx, prefix+keyPrefixCapability+cap.Name, agent.ID)
	}

	// Set heartbeat with TTL
	pipe.Set(ctx, prefix+keyPrefixAgentStatus+agent.ID, "running", r.config.HeartbeatTTL)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}

	return nil
}

// DeregisterAgent removes an agent from the registry
func (r *RedisRegistry) DeregisterAgent(ctx context.Context, agentID string) error {
	r.mu.RLock()
	if !r.connected {
		r.mu.RUnlock()
		return fmt.Errorf("registry not connected")
	}
	r.mu.RUnlock()

	prefix := r.config.Redis.KeyPrefix

	// Get agent info first (to remove from capability sets)
	agent, err := r.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	if agent == nil {
		return nil // Already deregistered
	}

	pipe := r.client.Pipeline()

	// Remove agent info
	pipe.Del(ctx, prefix+keyPrefixAgent+agentID)
	pipe.Del(ctx, prefix+keyPrefixAgentStatus+agentID)
	pipe.Del(ctx, prefix+keyPrefixAgentLoad+agentID)

	// Remove from sets
	pipe.SRem(ctx, prefix+keyAgentSet, agentID)
	pipe.SRem(ctx, prefix+keyAgentTypeSet+string(agent.Type), agentID)

	// Remove from capability indexes
	for _, cap := range agent.Capabilities {
		pipe.SRem(ctx, prefix+keyPrefixCapability+cap.Name, agentID)
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to deregister agent: %w", err)
	}

	return nil
}

// UpdateAgentStatus updates an agent's status
func (r *RedisRegistry) UpdateAgentStatus(ctx context.Context, agentID string, status AgentStatus) error {
	r.mu.RLock()
	if !r.connected {
		r.mu.RUnlock()
		return fmt.Errorf("registry not connected")
	}
	r.mu.RUnlock()

	prefix := r.config.Redis.KeyPrefix

	// Get current agent info
	agent, err := r.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	if agent == nil {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	// Update status
	agent.Status = status
	agent.LastSeen = time.Now()

	// Serialize and store
	data, err := json.Marshal(agent)
	if err != nil {
		return fmt.Errorf("failed to serialize agent info: %w", err)
	}

	pipe := r.client.Pipeline()
	pipe.Set(ctx, prefix+keyPrefixAgent+agentID, data, 0)
	pipe.Set(ctx, prefix+keyPrefixAgentStatus+agentID, string(status.State), r.config.HeartbeatTTL)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update agent status: %w", err)
	}

	return nil
}

// UpdateAgentLoad updates an agent's load metrics
func (r *RedisRegistry) UpdateAgentLoad(ctx context.Context, agentID string, load models.LoadMetrics) error {
	r.mu.RLock()
	if !r.connected {
		r.mu.RUnlock()
		return fmt.Errorf("registry not connected")
	}
	r.mu.RUnlock()

	prefix := r.config.Redis.KeyPrefix

	// Serialize load metrics
	data, err := json.Marshal(load)
	if err != nil {
		return fmt.Errorf("failed to serialize load metrics: %w", err)
	}

	// Store with TTL
	err = r.client.Set(ctx, prefix+keyPrefixAgentLoad+agentID, data, r.config.HeartbeatTTL).Err()
	if err != nil {
		return fmt.Errorf("failed to update agent load: %w", err)
	}

	// Update agent info
	agent, err := r.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	if agent != nil {
		agent.Load = load
		agent.LastSeen = time.Now()

		agentData, _ := json.Marshal(agent)
		r.client.Set(ctx, prefix+keyPrefixAgent+agentID, agentData, 0)
	}

	return nil
}

// GetAgent retrieves an agent by ID
func (r *RedisRegistry) GetAgent(ctx context.Context, agentID string) (*AgentInfo, error) {
	r.mu.RLock()
	if !r.connected {
		r.mu.RUnlock()
		return nil, fmt.Errorf("registry not connected")
	}
	r.mu.RUnlock()

	prefix := r.config.Redis.KeyPrefix

	data, err := r.client.Get(ctx, prefix+keyPrefixAgent+agentID).Bytes()
	if err == redis.Nil {
		return nil, nil // Not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	var agent AgentInfo
	if err := json.Unmarshal(data, &agent); err != nil {
		return nil, fmt.Errorf("failed to deserialize agent info: %w", err)
	}

	return &agent, nil
}

// GetAgentStatus retrieves an agent's status
func (r *RedisRegistry) GetAgentStatus(ctx context.Context, agentID string) (*AgentStatus, error) {
	agent, err := r.GetAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return nil, nil
	}
	return &agent.Status, nil
}

// ListAllAgents returns all registered agents
func (r *RedisRegistry) ListAllAgents(ctx context.Context) ([]AgentInfo, error) {
	r.mu.RLock()
	if !r.connected {
		r.mu.RUnlock()
		return nil, fmt.Errorf("registry not connected")
	}
	r.mu.RUnlock()

	prefix := r.config.Redis.KeyPrefix

	// Get all agent IDs
	agentIDs, err := r.client.SMembers(ctx, prefix+keyAgentSet).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	agents := make([]AgentInfo, 0, len(agentIDs))
	for _, id := range agentIDs {
		agent, err := r.GetAgent(ctx, id)
		if err != nil {
			continue // Skip on error
		}
		if agent != nil {
			agents = append(agents, *agent)
		}
	}

	return agents, nil
}

// ListAgentsByType returns agents of a specific type
func (r *RedisRegistry) ListAgentsByType(ctx context.Context, agentType models.AgentType) ([]AgentInfo, error) {
	r.mu.RLock()
	if !r.connected {
		r.mu.RUnlock()
		return nil, fmt.Errorf("registry not connected")
	}
	r.mu.RUnlock()

	prefix := r.config.Redis.KeyPrefix

	// Get agent IDs for type
	agentIDs, err := r.client.SMembers(ctx, prefix+keyAgentTypeSet+string(agentType)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list agents by type: %w", err)
	}

	agents := make([]AgentInfo, 0, len(agentIDs))
	for _, id := range agentIDs {
		agent, err := r.GetAgent(ctx, id)
		if err != nil {
			continue
		}
		if agent != nil {
			agents = append(agents, *agent)
		}
	}

	return agents, nil
}

// FindAgentsByCapability returns agents with a specific capability
func (r *RedisRegistry) FindAgentsByCapability(ctx context.Context, capability string) ([]AgentInfo, error) {
	r.mu.RLock()
	if !r.connected {
		r.mu.RUnlock()
		return nil, fmt.Errorf("registry not connected")
	}
	r.mu.RUnlock()

	prefix := r.config.Redis.KeyPrefix

	// Get agent IDs with capability
	agentIDs, err := r.client.SMembers(ctx, prefix+keyPrefixCapability+capability).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to find agents by capability: %w", err)
	}

	agents := make([]AgentInfo, 0, len(agentIDs))
	for _, id := range agentIDs {
		agent, err := r.GetAgent(ctx, id)
		if err != nil {
			continue
		}
		if agent != nil {
			// Verify agent is still active
			if agent.Status.State == AgentStateRunning {
				agents = append(agents, *agent)
			}
		}
	}

	return agents, nil
}

// Heartbeat updates agent's last seen time
func (r *RedisRegistry) Heartbeat(ctx context.Context, agentID string) error {
	r.mu.RLock()
	if !r.connected {
		r.mu.RUnlock()
		return fmt.Errorf("registry not connected")
	}
	r.mu.RUnlock()

	prefix := r.config.Redis.KeyPrefix

	// Refresh heartbeat TTL
	err := r.client.Expire(ctx, prefix+keyPrefixAgentStatus+agentID, r.config.HeartbeatTTL).Err()
	if err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}

	// Update last seen in agent info
	agent, err := r.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	if agent != nil {
		agent.LastSeen = time.Now()
		data, _ := json.Marshal(agent)
		r.client.Set(ctx, prefix+keyPrefixAgent+agentID, data, 0)
	}

	return nil
}

// GetStaleAgents returns agents that haven't sent a heartbeat within threshold
func (r *RedisRegistry) GetStaleAgents(ctx context.Context, threshold time.Duration) ([]string, error) {
	agents, err := r.ListAllAgents(ctx)
	if err != nil {
		return nil, err
	}

	stale := make([]string, 0)
	now := time.Now()

	for _, agent := range agents {
		if now.Sub(agent.LastSeen) > threshold {
			stale = append(stale, agent.ID)
		}
	}

	return stale, nil
}

// CleanupStaleAgents removes agents that haven't sent a heartbeat
func (r *RedisRegistry) CleanupStaleAgents(ctx context.Context, threshold time.Duration) error {
	stale, err := r.GetStaleAgents(ctx, threshold)
	if err != nil {
		return err
	}

	for _, agentID := range stale {
		if err := r.DeregisterAgent(ctx, agentID); err != nil {
			// Log error but continue cleanup
			continue
		}
	}

	return nil
}

// runCleanup periodically removes stale agents
func (r *RedisRegistry) runCleanup() {
	defer r.wg.Done()

	ticker := time.NewTicker(r.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.CleanupStaleAgents(r.ctx, r.config.StaleThreshold)
		}
	}
}

// FindAgentsByMultipleCapabilities returns agents matching all specified capabilities
func (r *RedisRegistry) FindAgentsByMultipleCapabilities(ctx context.Context, capabilities []string) ([]AgentInfo, error) {
	if len(capabilities) == 0 {
		return r.ListAllAgents(ctx)
	}

	r.mu.RLock()
	if !r.connected {
		r.mu.RUnlock()
		return nil, fmt.Errorf("registry not connected")
	}
	r.mu.RUnlock()

	prefix := r.config.Redis.KeyPrefix

	// Build capability keys
	keys := make([]string, len(capabilities))
	for i, cap := range capabilities {
		keys[i] = prefix + keyPrefixCapability + cap
	}

	// Intersect all capability sets
	agentIDs, err := r.client.SInter(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to find agents by capabilities: %w", err)
	}

	agents := make([]AgentInfo, 0, len(agentIDs))
	for _, id := range agentIDs {
		agent, err := r.GetAgent(ctx, id)
		if err != nil {
			continue
		}
		if agent != nil && agent.Status.State == AgentStateRunning {
			agents = append(agents, *agent)
		}
	}

	return agents, nil
}

// SelectBestAgent selects the best available agent based on load balancing
func (r *RedisRegistry) SelectBestAgent(ctx context.Context, capabilities []string, strategy LoadBalancerStrategy) (*AgentInfo, error) {
	agents, err := r.FindAgentsByMultipleCapabilities(ctx, capabilities)
	if err != nil {
		return nil, err
	}

	if len(agents) == 0 {
		return nil, fmt.Errorf("no agents available with required capabilities: %v", capabilities)
	}

	switch strategy {
	case LeastLoaded:
		return r.selectLeastLoaded(agents), nil
	case Random:
		return r.selectRandom(agents), nil
	case RoundRobin:
		return r.selectRoundRobin(ctx, agents, strings.Join(capabilities, ",")), nil
	default:
		return r.selectLeastLoaded(agents), nil
	}
}

func (r *RedisRegistry) selectLeastLoaded(agents []AgentInfo) *AgentInfo {
	if len(agents) == 0 {
		return nil
	}

	best := &agents[0]
	lowestLoad := agents[0].Load.ActiveTasks

	for i := 1; i < len(agents); i++ {
		if agents[i].Load.ActiveTasks < lowestLoad {
			lowestLoad = agents[i].Load.ActiveTasks
			best = &agents[i]
		}
	}

	return best
}

func (r *RedisRegistry) selectRandom(agents []AgentInfo) *AgentInfo {
	if len(agents) == 0 {
		return nil
	}
	// Simple selection - in production use proper randomization
	idx := time.Now().UnixNano() % int64(len(agents))
	return &agents[idx]
}

func (r *RedisRegistry) selectRoundRobin(ctx context.Context, agents []AgentInfo, key string) *AgentInfo {
	if len(agents) == 0 {
		return nil
	}

	prefix := r.config.Redis.KeyPrefix
	counterKey := prefix + "rr:" + key

	// Increment counter
	idx, err := r.client.Incr(ctx, counterKey).Result()
	if err != nil {
		idx = 0
	}

	// Set expiry on counter
	r.client.Expire(ctx, counterKey, time.Hour)

	return &agents[idx%int64(len(agents))]
}
