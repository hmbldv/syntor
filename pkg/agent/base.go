package agent

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/syntor/syntor/pkg/models"
)

// BaseAgent provides a common implementation of the Agent interface
// that can be embedded by specific agent implementations
type BaseAgent struct {
	id           string
	name         string
	agentType    models.AgentType
	capabilities []models.Capability
	config       Config

	// State management
	state      atomic.Value // AgentState
	health     atomic.Value // models.HealthStatus
	startTime  time.Time
	mu         sync.RWMutex

	// Metrics
	metrics     *agentMetrics
	traceCtx    models.TraceContext

	// Message handling
	messageBus  MessageBus
	handlers    map[models.MessageType]MessageHandler

	// Lifecycle
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	started    atomic.Bool
}

// AgentState represents the operational state of an agent
type AgentState string

const (
	StateUninitialized AgentState = "uninitialized"
	StateInitialized   AgentState = "initialized"
	StateStarting      AgentState = "starting"
	StateRunning       AgentState = "running"
	StateStopping      AgentState = "stopping"
	StateStopped       AgentState = "stopped"
	StateFailed        AgentState = "failed"
)

// MessageBus interface for agent communication
type MessageBus interface {
	Publish(ctx context.Context, topic string, msg models.Message) error
	Subscribe(ctx context.Context, topic string, handler MessageHandler) error
	Close() error
}

// agentMetrics tracks agent performance metrics
type agentMetrics struct {
	mu             sync.RWMutex
	tasksProcessed int64
	tasksSucceeded int64
	tasksFailed    int64
	totalLatency   time.Duration
	latencyCount   int64
	currentLoad    float64
}

// NewBaseAgent creates a new base agent with the given configuration
func NewBaseAgent(config Config) *BaseAgent {
	agent := &BaseAgent{
		id:           config.ID,
		name:         config.Name,
		agentType:    config.Type,
		capabilities: config.Capabilities,
		config:       config,
		handlers:     make(map[models.MessageType]MessageHandler),
		metrics:      &agentMetrics{},
	}

	// Set initial state
	agent.state.Store(StateUninitialized)
	agent.health.Store(models.HealthUnknown)

	// Generate ID if not provided
	if agent.id == "" {
		agent.id = fmt.Sprintf("%s-%s", config.Name, uuid.New().String()[:8])
	}

	return agent
}

// Initialize prepares the agent for operation
func (a *BaseAgent) Initialize(ctx context.Context, config Config) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.getState() != StateUninitialized {
		return fmt.Errorf("agent already initialized, current state: %s", a.getState())
	}

	a.config = config
	a.state.Store(StateInitialized)
	a.health.Store(models.HealthHealthy)

	return nil
}

// Start begins agent operation
func (a *BaseAgent) Start(ctx context.Context) error {
	if !a.started.CompareAndSwap(false, true) {
		return fmt.Errorf("agent already started")
	}

	a.mu.Lock()
	currentState := a.getState()
	if currentState != StateInitialized && currentState != StateStopped {
		a.mu.Unlock()
		a.started.Store(false)
		return fmt.Errorf("cannot start agent in state: %s", currentState)
	}

	a.state.Store(StateStarting)
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.startTime = time.Now()
	a.mu.Unlock()

	// Start background tasks
	a.wg.Add(1)
	go a.runHealthCheck()

	a.state.Store(StateRunning)
	a.health.Store(models.HealthHealthy)

	return nil
}

// Stop gracefully shuts down the agent
func (a *BaseAgent) Stop(ctx context.Context) error {
	if !a.started.Load() {
		return nil // Already stopped
	}

	a.state.Store(StateStopping)

	// Cancel context to signal shutdown
	if a.cancel != nil {
		a.cancel()
	}

	// Wait for background tasks with timeout
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Clean shutdown
	case <-ctx.Done():
		return fmt.Errorf("shutdown timeout exceeded")
	}

	a.state.Store(StateStopped)
	a.health.Store(models.HealthUnknown)
	a.started.Store(false)

	return nil
}

// Health returns the current health status
func (a *BaseAgent) Health() models.HealthStatus {
	return a.health.Load().(models.HealthStatus)
}

// HandleMessage processes an incoming message
func (a *BaseAgent) HandleMessage(ctx context.Context, msg models.Message) error {
	if a.getState() != StateRunning {
		return fmt.Errorf("agent not running, current state: %s", a.getState())
	}

	handler, ok := a.handlers[msg.Type]
	if !ok {
		return fmt.Errorf("no handler registered for message type: %s", msg.Type)
	}

	start := time.Now()
	err := handler(ctx, msg)
	a.recordTaskMetrics(err == nil, time.Since(start))

	return err
}

// SendMessage sends a message to the specified target
func (a *BaseAgent) SendMessage(ctx context.Context, target string, msg models.Message) error {
	if a.messageBus == nil {
		return fmt.Errorf("message bus not configured")
	}

	// Set message metadata
	msg.Source = a.id
	msg.Target = target
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	if msg.CorrelationID == "" {
		msg.CorrelationID = a.traceCtx.TraceID
	}

	return a.messageBus.Publish(ctx, target, msg)
}

// GetCapabilities returns the agent's capabilities
func (a *BaseAgent) GetCapabilities() []models.Capability {
	a.mu.RLock()
	defer a.mu.RUnlock()

	caps := make([]models.Capability, len(a.capabilities))
	copy(caps, a.capabilities)
	return caps
}

// CanHandle checks if the agent can handle a specific task type
func (a *BaseAgent) CanHandle(taskType string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, cap := range a.capabilities {
		if cap.Name == taskType {
			return true
		}
	}
	return false
}

// GetMetrics returns current agent metrics
func (a *BaseAgent) GetMetrics() models.AgentMetrics {
	a.metrics.mu.RLock()
	defer a.metrics.mu.RUnlock()

	avgLatency := time.Duration(0)
	if a.metrics.latencyCount > 0 {
		avgLatency = a.metrics.totalLatency / time.Duration(a.metrics.latencyCount)
	}

	return models.AgentMetrics{
		AgentID:        a.id,
		TasksProcessed: a.metrics.tasksProcessed,
		TasksSucceeded: a.metrics.tasksSucceeded,
		TasksFailed:    a.metrics.tasksFailed,
		AverageLatency: avgLatency,
		CurrentLoad:    a.metrics.currentLoad,
		LastUpdated:    time.Now(),
	}
}

// GetTraceContext returns the current trace context
func (a *BaseAgent) GetTraceContext() models.TraceContext {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.traceCtx
}

// ID returns the agent's unique identifier
func (a *BaseAgent) ID() string {
	return a.id
}

// Name returns the agent's name
func (a *BaseAgent) Name() string {
	return a.name
}

// Type returns the agent's type
func (a *BaseAgent) Type() models.AgentType {
	return a.agentType
}

// SetMessageBus sets the message bus for communication
func (a *BaseAgent) SetMessageBus(bus MessageBus) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.messageBus = bus
}

// RegisterHandler registers a message handler for a specific message type
func (a *BaseAgent) RegisterHandler(msgType models.MessageType, handler MessageHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.handlers[msgType] = handler
}

// AddCapability adds a capability to the agent
func (a *BaseAgent) AddCapability(cap models.Capability) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.capabilities = append(a.capabilities, cap)
}

// SetHealth updates the agent's health status
func (a *BaseAgent) SetHealth(status models.HealthStatus) {
	a.health.Store(status)
}

// GetState returns the current agent state
func (a *BaseAgent) GetState() AgentState {
	return a.getState()
}

// Uptime returns how long the agent has been running
func (a *BaseAgent) Uptime() time.Duration {
	if a.startTime.IsZero() {
		return 0
	}
	return time.Since(a.startTime)
}

// Internal helper methods

func (a *BaseAgent) getState() AgentState {
	return a.state.Load().(AgentState)
}

func (a *BaseAgent) recordTaskMetrics(success bool, latency time.Duration) {
	a.metrics.mu.Lock()
	defer a.metrics.mu.Unlock()

	a.metrics.tasksProcessed++
	if success {
		a.metrics.tasksSucceeded++
	} else {
		a.metrics.tasksFailed++
	}
	a.metrics.totalLatency += latency
	a.metrics.latencyCount++
}

func (a *BaseAgent) runHealthCheck() {
	defer a.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.performHealthCheck()
		}
	}
}

func (a *BaseAgent) performHealthCheck() {
	// Basic health check - can be overridden by specific agents
	if a.getState() == StateRunning {
		a.health.Store(models.HealthHealthy)
	} else {
		a.health.Store(models.HealthDegraded)
	}
}
