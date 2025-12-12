package models

import (
	"time"
)

// AgentType defines the category of agent
type AgentType string

const (
	ServiceAgentType AgentType = "service"
	WorkerAgentType  AgentType = "worker"
)

// MessageType defines the category of message
type MessageType string

const (
	MsgTaskAssignment    MessageType = "task.assignment"
	MsgTaskStatus        MessageType = "task.status"
	MsgTaskComplete      MessageType = "task.complete"
	MsgAgentRegistration MessageType = "agent.registration"
	MsgAgentHeartbeat    MessageType = "agent.heartbeat"
	MsgServiceRequest    MessageType = "service.request"
	MsgServiceResponse   MessageType = "service.response"
)

// TaskStatus represents the current state of a task
type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskAssigned  TaskStatus = "assigned"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskCancelled TaskStatus = "cancelled"
)

// TaskPriority defines task execution priority
type TaskPriority int

const (
	LowPriority      TaskPriority = 1
	NormalPriority   TaskPriority = 5
	HighPriority     TaskPriority = 10
	CriticalPriority TaskPriority = 15
)

// HealthStatus represents the health state of a component
type HealthStatus string

const (
	HealthHealthy   HealthStatus = "healthy"
	HealthUnhealthy HealthStatus = "unhealthy"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnknown   HealthStatus = "unknown"
)

// CircuitState represents circuit breaker state
type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half-open"
)

// Capability represents a capability that an agent can handle
type Capability struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Parameters  map[string]string `json:"parameters,omitempty"`
}

// Message represents the standard message format for inter-agent communication
type Message struct {
	ID            string                 `json:"id"`
	Type          MessageType            `json:"type"`
	Source        string                 `json:"source"`
	Target        string                 `json:"target"`
	Payload       map[string]interface{} `json:"payload"`
	Timestamp     time.Time              `json:"timestamp"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	ReplyTo       string                 `json:"reply_to,omitempty"`
	TTL           time.Duration          `json:"ttl,omitempty"`
}

// Task represents a unit of work to be executed by an agent
type Task struct {
	ID                   string                 `json:"id"`
	Type                 string                 `json:"type"`
	Priority             TaskPriority           `json:"priority"`
	RequiredCapabilities []string               `json:"required_capabilities"`
	Payload              map[string]interface{} `json:"payload"`
	AssignedAgent        string                 `json:"assigned_agent,omitempty"`
	Status               TaskStatus             `json:"status"`
	CreatedAt            time.Time              `json:"created_at"`
	StartedAt            *time.Time             `json:"started_at,omitempty"`
	CompletedAt          *time.Time             `json:"completed_at,omitempty"`
	Result               *TaskResult            `json:"result,omitempty"`
	Error                *TaskError             `json:"error,omitempty"`
	Timeout              time.Duration          `json:"timeout"`
	RetryCount           int                    `json:"retry_count"`
	MaxRetries           int                    `json:"max_retries"`
}

// TaskResult contains the result of a completed task
type TaskResult struct {
	Data     map[string]interface{} `json:"data"`
	Duration time.Duration          `json:"duration"`
}

// TaskError contains error information for a failed task
type TaskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// AgentMetrics contains performance and health metrics for an agent
type AgentMetrics struct {
	AgentID        string        `json:"agent_id"`
	TasksProcessed int64         `json:"tasks_processed"`
	TasksSucceeded int64         `json:"tasks_succeeded"`
	TasksFailed    int64         `json:"tasks_failed"`
	AverageLatency time.Duration `json:"average_latency"`
	CurrentLoad    float64       `json:"current_load"`
	MemoryUsage    int64         `json:"memory_usage"`
	CPUUsage       float64       `json:"cpu_usage"`
	LastUpdated    time.Time     `json:"last_updated"`
}

// LoadMetrics contains load information for an agent
type LoadMetrics struct {
	ActiveTasks    int     `json:"active_tasks"`
	QueuedTasks    int     `json:"queued_tasks"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryPercent  float64 `json:"memory_percent"`
	MessageRate    float64 `json:"message_rate"`
	ErrorRate      float64 `json:"error_rate"`
}

// TraceContext contains distributed tracing context
type TraceContext struct {
	TraceID  string `json:"trace_id"`
	SpanID   string `json:"span_id"`
	ParentID string `json:"parent_id,omitempty"`
}

// RetryPolicy defines how retries should be handled
type RetryPolicy struct {
	MaxAttempts     int           `json:"max_attempts"`
	InitialDelay    time.Duration `json:"initial_delay"`
	MaxDelay        time.Duration `json:"max_delay"`
	BackoffFactor   float64       `json:"backoff_factor"`
	RetryableErrors []string      `json:"retryable_errors"`
}
