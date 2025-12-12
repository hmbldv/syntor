package coordination

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/syntor/syntor/pkg/agent"
	"github.com/syntor/syntor/pkg/kafka"
	"github.com/syntor/syntor/pkg/models"
	"github.com/syntor/syntor/pkg/registry"
)

// Agent implements the CoordinationAgent interface
// It manages task routing, agent coordination, and system health
type Agent struct {
	*agent.BaseAgent

	// Dependencies
	registry    registry.Registry
	messageBus  *kafka.Client
	taskRouter  *registry.TaskRouter

	// Task management
	pendingTasks   map[string]*models.Task
	taskAssignments map[string]string // taskID -> agentID
	taskMu         sync.RWMutex

	// Agent monitoring
	agentHealth    map[string]time.Time // agentID -> lastHeartbeat
	healthMu       sync.RWMutex

	// Configuration
	config         Config

	// Background workers
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// Config holds configuration for the Coordination Agent
type Config struct {
	AgentConfig          agent.Config
	HeartbeatInterval    time.Duration
	HealthCheckInterval  time.Duration
	TaskTimeoutCheck     time.Duration
	FailoverThreshold    time.Duration
	MaxPendingTasks      int
	ScalingThreshold     float64 // Load threshold for scaling decisions
}

// DefaultConfig returns default coordination agent configuration
func DefaultConfig() Config {
	return Config{
		HeartbeatInterval:   30 * time.Second,
		HealthCheckInterval: 10 * time.Second,
		TaskTimeoutCheck:    60 * time.Second,
		FailoverThreshold:   90 * time.Second,
		MaxPendingTasks:     1000,
		ScalingThreshold:    0.8,
	}
}

// New creates a new Coordination Agent
func New(config Config, reg registry.Registry, bus *kafka.Client) *Agent {
	baseConfig := config.AgentConfig
	baseConfig.Type = models.ServiceAgentType
	baseConfig.Capabilities = []models.Capability{
		{Name: "coordination", Version: "1.0", Description: "Task routing and coordination"},
		{Name: "load-balancing", Version: "1.0", Description: "Load balancing across agents"},
		{Name: "failover", Version: "1.0", Description: "Agent failover handling"},
		{Name: "scaling", Version: "1.0", Description: "Dynamic scaling decisions"},
	}

	a := &Agent{
		BaseAgent:       agent.NewBaseAgent(baseConfig),
		registry:        reg,
		messageBus:      bus,
		taskRouter:      registry.NewTaskRouter(reg, registry.LeastLoaded),
		pendingTasks:    make(map[string]*models.Task),
		taskAssignments: make(map[string]string),
		agentHealth:     make(map[string]time.Time),
		config:          config,
	}

	// Register message handlers
	a.RegisterHandler(models.MsgTaskAssignment, a.handleTaskAssignment)
	a.RegisterHandler(models.MsgTaskStatus, a.handleTaskStatus)
	a.RegisterHandler(models.MsgTaskComplete, a.handleTaskComplete)
	a.RegisterHandler(models.MsgAgentRegistration, a.handleAgentRegistration)
	a.RegisterHandler(models.MsgAgentHeartbeat, a.handleAgentHeartbeat)
	a.RegisterHandler(models.MsgServiceRequest, a.handleServiceRequest)

	return a
}

// Start begins the coordination agent operation
func (a *Agent) Start(ctx context.Context) error {
	if err := a.BaseAgent.Start(ctx); err != nil {
		return err
	}

	a.ctx, a.cancel = context.WithCancel(ctx)

	// Subscribe to relevant topics
	topics := []string{
		kafka.TopicTaskAssignment,
		kafka.TopicTaskStatus,
		kafka.TopicTaskComplete,
		kafka.TopicAgentRegistration,
		kafka.TopicAgentHeartbeat,
		kafka.TopicServiceRequest,
	}

	for _, topic := range topics {
		if err := a.messageBus.Subscribe(ctx, topic, a.handleMessage); err != nil {
			return fmt.Errorf("failed to subscribe to %s: %w", topic, err)
		}
	}

	// Register self with registry
	agentInfo := registry.AgentInfo{
		ID:           a.ID(),
		Name:         a.Name(),
		Type:         a.Type(),
		Capabilities: a.GetCapabilities(),
		Status: registry.AgentStatus{
			State:  registry.AgentStateRunning,
			Health: models.HealthHealthy,
		},
	}
	if err := a.registry.RegisterAgent(ctx, agentInfo); err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}

	// Start background workers
	a.wg.Add(3)
	go a.runHeartbeat()
	go a.runHealthMonitor()
	go a.runTaskTimeoutChecker()

	return nil
}

// Stop gracefully shuts down the coordination agent
func (a *Agent) Stop(ctx context.Context) error {
	// Cancel background workers
	if a.cancel != nil {
		a.cancel()
	}

	// Wait for workers to finish
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Deregister from registry
	if err := a.registry.DeregisterAgent(ctx, a.ID()); err != nil {
		// Log but don't fail
	}

	return a.BaseAgent.Stop(ctx)
}

// RouteTask routes a task to an appropriate agent
func (a *Agent) RouteTask(ctx context.Context, task models.Task) error {
	// Find best agent for the task
	targetAgent, err := a.taskRouter.Route(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to find agent for task: %w", err)
	}

	// Update task with assignment
	task.AssignedAgent = targetAgent.ID
	task.Status = models.TaskAssigned
	now := time.Now()
	task.StartedAt = &now

	// Store task assignment
	a.taskMu.Lock()
	a.pendingTasks[task.ID] = &task
	a.taskAssignments[task.ID] = targetAgent.ID
	a.taskMu.Unlock()

	// Send task assignment message
	msg := models.NewTaskAssignmentMessage(a.ID(), targetAgent.ID, task)
	if err := a.messageBus.Publish(ctx, kafka.TopicTaskAssignment, msg); err != nil {
		return fmt.Errorf("failed to publish task assignment: %w", err)
	}

	return nil
}

// MonitorAgents monitors the health of all registered agents
func (a *Agent) MonitorAgents(ctx context.Context) error {
	agents, err := a.registry.ListAllAgents(ctx)
	if err != nil {
		return err
	}

	now := time.Now()

	for _, agentInfo := range agents {
		a.healthMu.RLock()
		lastSeen, ok := a.agentHealth[agentInfo.ID]
		a.healthMu.RUnlock()

		if !ok {
			lastSeen = agentInfo.LastSeen
		}

		// Check if agent is stale
		if now.Sub(lastSeen) > a.config.FailoverThreshold {
			// Trigger failover
			a.HandleFailover(ctx, agentInfo.ID)
		}
	}

	return nil
}

// HandleFailover handles agent failure and task redistribution
func (a *Agent) HandleFailover(ctx context.Context, failedAgentID string) error {
	// Get tasks assigned to failed agent
	a.taskMu.RLock()
	tasksToReassign := make([]*models.Task, 0)
	for taskID, agentID := range a.taskAssignments {
		if agentID == failedAgentID {
			if task, ok := a.pendingTasks[taskID]; ok {
				tasksToReassign = append(tasksToReassign, task)
			}
		}
	}
	a.taskMu.RUnlock()

	// Deregister failed agent
	if err := a.registry.DeregisterAgent(ctx, failedAgentID); err != nil {
		// Log error but continue
	}

	// Reassign tasks
	for _, task := range tasksToReassign {
		task.RetryCount++
		if task.RetryCount <= task.MaxRetries {
			// Route to another agent
			task.Status = models.TaskPending
			task.AssignedAgent = ""
			if err := a.RouteTask(ctx, *task); err != nil {
				// Mark task as failed if cannot reassign
				task.Status = models.TaskFailed
				task.Error = &models.TaskError{
					Code:    "FAILOVER_FAILED",
					Message: "Failed to reassign task after agent failure",
				}
			}
		} else {
			// Max retries exceeded
			task.Status = models.TaskFailed
			task.Error = &models.TaskError{
				Code:    "MAX_RETRIES",
				Message: "Maximum retry attempts exceeded",
			}
		}
	}

	// Publish failover event
	msg := models.NewMessage(
		models.MsgTaskStatus,
		a.ID(),
		"",
		map[string]interface{}{
			"event":          "agent_failover",
			"failed_agent":   failedAgentID,
			"tasks_affected": len(tasksToReassign),
		},
	)
	a.messageBus.PublishAsync(kafka.TopicSystemEvents, msg)

	return nil
}

// BalanceLoad checks and rebalances load across agents
func (a *Agent) BalanceLoad(ctx context.Context) error {
	agents, err := a.registry.ListAllAgents(ctx)
	if err != nil {
		return err
	}

	if len(agents) == 0 {
		return nil
	}

	// Calculate average load
	var totalLoad float64
	for _, agent := range agents {
		totalLoad += float64(agent.Load.ActiveTasks)
	}
	avgLoad := totalLoad / float64(len(agents))

	// Check for scaling needs
	scalingNeeded := false
	for _, agent := range agents {
		loadRatio := float64(agent.Load.ActiveTasks) / avgLoad
		if loadRatio > a.config.ScalingThreshold*2 || loadRatio < a.config.ScalingThreshold/2 {
			scalingNeeded = true
			break
		}
	}

	if scalingNeeded {
		// Publish scaling event
		msg := models.NewMessage(
			models.MsgTaskStatus,
			a.ID(),
			"",
			map[string]interface{}{
				"event":          "scaling_recommendation",
				"average_load":   avgLoad,
				"agent_count":    len(agents),
				"recommendation": a.getScalingRecommendation(agents, avgLoad),
			},
		)
		a.messageBus.PublishAsync(kafka.TopicSystemEvents, msg)
	}

	return nil
}

// SubmitTask accepts a new task for routing
func (a *Agent) SubmitTask(ctx context.Context, task models.Task) (string, error) {
	// Generate task ID if not set
	if task.ID == "" {
		task.ID = uuid.New().String()
	}

	// Set defaults
	if task.Priority == 0 {
		task.Priority = models.NormalPriority
	}
	if task.MaxRetries == 0 {
		task.MaxRetries = 3
	}
	if task.Timeout == 0 {
		task.Timeout = 5 * time.Minute
	}

	task.Status = models.TaskPending
	task.CreatedAt = time.Now()

	// Check capacity
	a.taskMu.RLock()
	pendingCount := len(a.pendingTasks)
	a.taskMu.RUnlock()

	if pendingCount >= a.config.MaxPendingTasks {
		return "", fmt.Errorf("task queue full, max pending tasks: %d", a.config.MaxPendingTasks)
	}

	// Route the task
	if err := a.RouteTask(ctx, task); err != nil {
		return "", err
	}

	return task.ID, nil
}

// GetTaskStatus returns the status of a task
func (a *Agent) GetTaskStatus(ctx context.Context, taskID string) (*models.Task, error) {
	a.taskMu.RLock()
	defer a.taskMu.RUnlock()

	task, ok := a.pendingTasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	return task, nil
}

// Message handlers

func (a *Agent) handleMessage(ctx context.Context, msg models.Message) error {
	return a.HandleMessage(ctx, msg)
}

func (a *Agent) handleTaskAssignment(ctx context.Context, msg models.Message) error {
	// This is for tasks submitted directly to coordination agent
	task, err := models.TaskFromPayload(msg.Payload)
	if err != nil {
		return err
	}

	_, err = a.SubmitTask(ctx, task)
	return err
}

func (a *Agent) handleTaskStatus(ctx context.Context, msg models.Message) error {
	taskID, ok := msg.Payload["task_id"].(string)
	if !ok {
		return fmt.Errorf("missing task_id in status message")
	}

	statusStr, ok := msg.Payload["status"].(string)
	if !ok {
		return fmt.Errorf("missing status in status message")
	}

	a.taskMu.Lock()
	defer a.taskMu.Unlock()

	if task, ok := a.pendingTasks[taskID]; ok {
		task.Status = models.TaskStatus(statusStr)
	}

	return nil
}

func (a *Agent) handleTaskComplete(ctx context.Context, msg models.Message) error {
	taskID, ok := msg.Payload["task_id"].(string)
	if !ok {
		return fmt.Errorf("missing task_id in complete message")
	}

	a.taskMu.Lock()
	defer a.taskMu.Unlock()

	if task, ok := a.pendingTasks[taskID]; ok {
		now := time.Now()
		task.CompletedAt = &now
		task.Status = models.TaskCompleted

		// Check for error
		if errData, ok := msg.Payload["error"]; ok && errData != nil {
			task.Status = models.TaskFailed
		}

		// Clean up
		delete(a.pendingTasks, taskID)
		delete(a.taskAssignments, taskID)
	}

	return nil
}

func (a *Agent) handleAgentRegistration(ctx context.Context, msg models.Message) error {
	agentID, ok := msg.Payload["agent_id"].(string)
	if !ok {
		return fmt.Errorf("missing agent_id in registration message")
	}

	a.healthMu.Lock()
	a.agentHealth[agentID] = time.Now()
	a.healthMu.Unlock()

	return nil
}

func (a *Agent) handleAgentHeartbeat(ctx context.Context, msg models.Message) error {
	agentID, ok := msg.Payload["agent_id"].(string)
	if !ok {
		return fmt.Errorf("missing agent_id in heartbeat message")
	}

	a.healthMu.Lock()
	a.agentHealth[agentID] = time.Now()
	a.healthMu.Unlock()

	// Update registry
	a.registry.Heartbeat(ctx, agentID)

	return nil
}

func (a *Agent) handleServiceRequest(ctx context.Context, msg models.Message) error {
	serviceType, ok := msg.Payload["service_type"].(string)
	if !ok {
		return fmt.Errorf("missing service_type in request")
	}

	var response map[string]interface{}
	var err error

	switch serviceType {
	case "get_agents":
		agents, e := a.registry.ListAllAgents(ctx)
		if e != nil {
			err = e
		} else {
			response = map[string]interface{}{"agents": agents}
		}

	case "get_pending_tasks":
		a.taskMu.RLock()
		tasks := make([]*models.Task, 0, len(a.pendingTasks))
		for _, task := range a.pendingTasks {
			tasks = append(tasks, task)
		}
		a.taskMu.RUnlock()
		response = map[string]interface{}{"tasks": tasks}

	case "get_system_stats":
		agents, _ := a.registry.ListAllAgents(ctx)
		a.taskMu.RLock()
		pendingCount := len(a.pendingTasks)
		a.taskMu.RUnlock()

		response = map[string]interface{}{
			"total_agents":   len(agents),
			"pending_tasks":  pendingCount,
			"uptime":         a.Uptime().String(),
		}

	default:
		err = fmt.Errorf("unknown service type: %s", serviceType)
	}

	// Send response
	var taskErr *models.TaskError
	if err != nil {
		taskErr = &models.TaskError{Code: "SERVICE_ERROR", Message: err.Error()}
	}

	respMsg := models.NewServiceResponseMessage(a.ID(), msg.Source, msg.CorrelationID, response, taskErr)
	return a.messageBus.Publish(ctx, kafka.TopicServiceResponse, respMsg)
}

// Background workers

func (a *Agent) runHeartbeat() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			// Send heartbeat
			msg := models.NewAgentHeartbeatMessage(a.ID(), a.GetMetrics())
			a.messageBus.PublishAsync(kafka.TopicAgentHeartbeat, msg)

			// Update registry
			a.registry.Heartbeat(a.ctx, a.ID())
		}
	}
}

func (a *Agent) runHealthMonitor() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.MonitorAgents(a.ctx)
			a.BalanceLoad(a.ctx)
		}
	}
}

func (a *Agent) runTaskTimeoutChecker() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.TaskTimeoutCheck)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.checkTaskTimeouts()
		}
	}
}

func (a *Agent) checkTaskTimeouts() {
	now := time.Now()

	a.taskMu.Lock()
	defer a.taskMu.Unlock()

	for taskID, task := range a.pendingTasks {
		if task.StartedAt != nil && now.Sub(*task.StartedAt) > task.Timeout {
			// Task timed out
			task.Status = models.TaskFailed
			task.Error = &models.TaskError{
				Code:    "TIMEOUT",
				Message: fmt.Sprintf("Task exceeded timeout of %s", task.Timeout),
			}

			// Clean up
			delete(a.taskAssignments, taskID)

			// Publish timeout event
			msg := models.NewTaskCompleteMessage(a.ID(), taskID, nil, task.Error)
			a.messageBus.PublishAsync(kafka.TopicTaskComplete, msg)
		}
	}
}

func (a *Agent) getScalingRecommendation(agents []registry.AgentInfo, avgLoad float64) string {
	if avgLoad > 10 {
		return "scale_up"
	} else if avgLoad < 2 && len(agents) > 1 {
		return "scale_down"
	}
	return "maintain"
}
