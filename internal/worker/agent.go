package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/syntor/syntor/pkg/agent"
	"github.com/syntor/syntor/pkg/kafka"
	"github.com/syntor/syntor/pkg/models"
	"github.com/syntor/syntor/pkg/registry"
)

// Agent implements the WorkerAgent interface for task execution
type Agent struct {
	*agent.BaseAgent

	// Dependencies
	registry   registry.Registry
	messageBus *kafka.Client

	// Task execution
	taskHandlers  map[string]TaskHandler
	activeTasks   map[string]*TaskExecution
	tasksMu       sync.RWMutex
	maxConcurrent int
	semaphore     chan struct{}

	// Configuration
	config Config

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// TaskHandler is a function that executes a specific task type
type TaskHandler func(ctx context.Context, task models.Task) (*models.TaskResult, error)

// TaskExecution tracks an executing task
type TaskExecution struct {
	Task      models.Task
	StartTime time.Time
	Cancel    context.CancelFunc
}

// Config holds configuration for the Worker Agent
type Config struct {
	AgentConfig       agent.Config
	MaxConcurrentTasks int
	TaskTimeout       time.Duration
	HeartbeatInterval time.Duration
	WorkPath          string
}

// DefaultConfig returns default worker agent configuration
func DefaultConfig() Config {
	return Config{
		MaxConcurrentTasks: 10,
		TaskTimeout:        5 * time.Minute,
		HeartbeatInterval:  30 * time.Second,
		WorkPath:           "/app/work",
	}
}

// New creates a new Worker Agent
func New(config Config, reg registry.Registry, bus *kafka.Client) *Agent {
	baseConfig := config.AgentConfig
	baseConfig.Type = models.WorkerAgentType

	if config.MaxConcurrentTasks <= 0 {
		config.MaxConcurrentTasks = 10
	}

	a := &Agent{
		BaseAgent:     agent.NewBaseAgent(baseConfig),
		registry:      reg,
		messageBus:    bus,
		taskHandlers:  make(map[string]TaskHandler),
		activeTasks:   make(map[string]*TaskExecution),
		maxConcurrent: config.MaxConcurrentTasks,
		semaphore:     make(chan struct{}, config.MaxConcurrentTasks),
		config:        config,
	}

	// Register default message handlers
	a.RegisterHandler(models.MsgTaskAssignment, a.handleTaskAssignment)

	return a
}

// RegisterTaskHandler registers a handler for a specific task type
func (a *Agent) RegisterTaskHandler(taskType string, handler TaskHandler) {
	a.tasksMu.Lock()
	defer a.tasksMu.Unlock()
	a.taskHandlers[taskType] = handler

	// Add as capability
	a.AddCapability(models.Capability{
		Name:        taskType,
		Version:     "1.0",
		Description: fmt.Sprintf("Handler for %s tasks", taskType),
	})
}

// Start begins the worker agent operation
func (a *Agent) Start(ctx context.Context) error {
	if err := a.BaseAgent.Start(ctx); err != nil {
		return err
	}

	a.ctx, a.cancel = context.WithCancel(ctx)

	// Subscribe to task assignments
	if err := a.messageBus.Subscribe(ctx, kafka.TopicTaskAssignment, a.handleMessage); err != nil {
		return fmt.Errorf("failed to subscribe to task assignments: %w", err)
	}

	// Register with registry
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

	// Start heartbeat
	a.wg.Add(1)
	go a.runHeartbeat()

	return nil
}

// Stop gracefully shuts down the worker agent
func (a *Agent) Stop(ctx context.Context) error {
	// Cancel all active tasks
	a.tasksMu.Lock()
	for _, exec := range a.activeTasks {
		if exec.Cancel != nil {
			exec.Cancel()
		}
	}
	a.tasksMu.Unlock()

	if a.cancel != nil {
		a.cancel()
	}

	// Wait for background workers
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

	a.registry.DeregisterAgent(ctx, a.ID())
	return a.BaseAgent.Stop(ctx)
}

// ExecuteTask executes a task and returns the result
func (a *Agent) ExecuteTask(ctx context.Context, task models.Task) (*models.TaskResult, error) {
	// Check capacity
	select {
	case a.semaphore <- struct{}{}:
		defer func() { <-a.semaphore }()
	default:
		return nil, fmt.Errorf("agent at capacity, max concurrent tasks: %d", a.maxConcurrent)
	}

	// Get handler
	a.tasksMu.RLock()
	handler, ok := a.taskHandlers[task.Type]
	a.tasksMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no handler for task type: %s", task.Type)
	}

	// Setup execution context with timeout
	timeout := task.Timeout
	if timeout == 0 {
		timeout = a.config.TaskTimeout
	}

	taskCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Track active task
	exec := &TaskExecution{
		Task:      task,
		StartTime: time.Now(),
		Cancel:    cancel,
	}

	a.tasksMu.Lock()
	a.activeTasks[task.ID] = exec
	a.tasksMu.Unlock()

	defer func() {
		a.tasksMu.Lock()
		delete(a.activeTasks, task.ID)
		a.tasksMu.Unlock()
	}()

	// Execute task
	result, err := handler(taskCtx, task)
	if err != nil {
		return nil, err
	}

	if result != nil {
		result.Duration = time.Since(exec.StartTime)
	}

	return result, nil
}

// GetTaskStatus returns the status of an active task
func (a *Agent) GetTaskStatus(ctx context.Context, taskID string) (models.TaskStatus, error) {
	a.tasksMu.RLock()
	defer a.tasksMu.RUnlock()

	if _, ok := a.activeTasks[taskID]; ok {
		return models.TaskRunning, nil
	}
	return models.TaskStatus("unknown"), fmt.Errorf("task not found: %s", taskID)
}

// CancelTask cancels an active task
func (a *Agent) CancelTask(ctx context.Context, taskID string) error {
	a.tasksMu.Lock()
	defer a.tasksMu.Unlock()

	exec, ok := a.activeTasks[taskID]
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if exec.Cancel != nil {
		exec.Cancel()
	}

	return nil
}

// GetActiveTaskCount returns the number of currently executing tasks
func (a *Agent) GetActiveTaskCount() int {
	a.tasksMu.RLock()
	defer a.tasksMu.RUnlock()
	return len(a.activeTasks)
}

// GetLoad returns current load metrics
func (a *Agent) GetLoad() models.LoadMetrics {
	return models.LoadMetrics{
		ActiveTasks:   a.GetActiveTaskCount(),
		QueuedTasks:   0,
		CPUPercent:    0, // Would need system metrics
		MemoryPercent: 0,
	}
}

// Message handlers

func (a *Agent) handleMessage(ctx context.Context, msg models.Message) error {
	return a.HandleMessage(ctx, msg)
}

func (a *Agent) handleTaskAssignment(ctx context.Context, msg models.Message) error {
	task, err := models.TaskFromPayload(msg.Payload)
	if err != nil {
		return err
	}

	// Check if task is for us
	if task.AssignedAgent != a.ID() {
		return nil
	}

	// Execute asynchronously
	go a.executeAndReport(task)

	return nil
}

func (a *Agent) executeAndReport(task models.Task) {
	// Send running status
	statusMsg := models.NewTaskStatusMessage(a.ID(), task.ID, models.TaskRunning)
	a.messageBus.PublishAsync(kafka.TopicTaskStatus, statusMsg)

	// Execute
	result, err := a.ExecuteTask(a.ctx, task)

	// Send completion
	var taskErr *models.TaskError
	if err != nil {
		taskErr = &models.TaskError{
			Code:    "EXECUTION_FAILED",
			Message: err.Error(),
		}
	}

	completeMsg := models.NewTaskCompleteMessage(a.ID(), task.ID, result, taskErr)
	a.messageBus.Publish(a.ctx, kafka.TopicTaskComplete, completeMsg)
}

func (a *Agent) runHeartbeat() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			// Update load metrics
			a.registry.UpdateAgentLoad(a.ctx, a.ID(), a.GetLoad())

			// Send heartbeat
			msg := models.NewAgentHeartbeatMessage(a.ID(), a.GetMetrics())
			a.messageBus.PublishAsync(kafka.TopicAgentHeartbeat, msg)
			a.registry.Heartbeat(a.ctx, a.ID())
		}
	}
}

// Ensure Agent implements WorkerAgent interface
var _ agent.WorkerAgent = (*Agent)(nil)
