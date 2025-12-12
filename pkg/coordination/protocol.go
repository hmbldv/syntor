package coordination

import (
	"time"
)

// HandoffIntent represents a delegation request from the coordination agent
type HandoffIntent struct {
	Action        ActionType             `json:"action"`                  // "delegate", "query", "parallel", "plan"
	Target        string                 `json:"target"`                  // Target agent name
	Task          string                 `json:"task"`                    // Task description
	Context       map[string]interface{} `json:"context,omitempty"`       // Passed context
	Priority      Priority               `json:"priority,omitempty"`      // Task priority
	Timeout       string                 `json:"timeout,omitempty"`       // Duration string
	WaitForResult bool                   `json:"waitForResult,omitempty"` // Wait for completion
}

// ActionType defines the type of handoff action
type ActionType string

const (
	ActionDelegate ActionType = "delegate" // Single agent delegation
	ActionQuery    ActionType = "query"    // Query agent for information
	ActionParallel ActionType = "parallel" // Parallel execution across agents
	ActionPlan     ActionType = "plan"     // Propose an execution plan
)

// Priority defines task priority levels
type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityNormal   Priority = "normal"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

// HandoffResult represents the result of a delegation
type HandoffResult struct {
	ID        string                 `json:"id"`                  // Unique result ID
	AgentName string                 `json:"agentName"`           // Agent that executed
	Status    ResultStatus           `json:"status"`              // "success", "error", "partial", "timeout"
	Result    interface{}            `json:"result,omitempty"`    // Output data
	Error     string                 `json:"error,omitempty"`     // Error message if failed
	Duration  time.Duration          `json:"duration"`            // Execution duration
	Metadata  map[string]interface{} `json:"metadata,omitempty"`  // Additional metadata
	Timestamp time.Time              `json:"timestamp"`           // Completion time
}

// ResultStatus defines the status of a handoff result
type ResultStatus string

const (
	ResultSuccess ResultStatus = "success"
	ResultError   ResultStatus = "error"
	ResultPartial ResultStatus = "partial"
	ResultTimeout ResultStatus = "timeout"
)

// PlanStep represents a step in an execution plan
type PlanStep struct {
	StepNumber  int               `json:"step"`                    // Step number (1-indexed)
	Agent       string            `json:"agent"`                   // Agent to execute
	Action      string            `json:"action"`                  // Action description
	Description string            `json:"description,omitempty"`   // Detailed description
	DependsOn   []int             `json:"dependsOn,omitempty"`     // Steps this depends on
	Estimated   string            `json:"estimated,omitempty"`     // Duration estimate
	Context     map[string]string `json:"context,omitempty"`       // Step-specific context
}

// ExecutionPlan represents a multi-step plan
type ExecutionPlan struct {
	ID               string     `json:"id"`               // Unique plan ID
	Summary          string     `json:"summary"`          // Plan summary
	Steps            []PlanStep `json:"steps"`            // Plan steps
	TotalAgents      int        `json:"totalAgents"`      // Number of unique agents
	Estimated        string     `json:"estimated"`        // Total estimated duration
	RequiresApproval bool       `json:"requiresApproval"` // Whether user approval is needed
	CreatedAt        time.Time  `json:"createdAt"`        // Plan creation time
}

// PlanExecution tracks the execution of a plan
type PlanExecution struct {
	Plan          *ExecutionPlan          `json:"plan"`
	Status        PlanStatus              `json:"status"`
	CurrentStep   int                     `json:"currentStep"`
	StepResults   map[int]*HandoffResult  `json:"stepResults"`
	StartedAt     time.Time               `json:"startedAt"`
	CompletedAt   *time.Time              `json:"completedAt,omitempty"`
	Error         string                  `json:"error,omitempty"`
}

// PlanStatus defines the status of plan execution
type PlanStatus string

const (
	PlanPending    PlanStatus = "pending"    // Awaiting approval
	PlanApproved   PlanStatus = "approved"   // Approved, ready to execute
	PlanExecuting  PlanStatus = "executing"  // Currently executing
	PlanCompleted  PlanStatus = "completed"  // Successfully completed
	PlanFailed     PlanStatus = "failed"     // Failed during execution
	PlanCancelled  PlanStatus = "cancelled"  // Cancelled by user
)

// HandoffStatus tracks the status of an active handoff
type HandoffStatus struct {
	ID        string        `json:"id"`
	FromAgent string        `json:"fromAgent"`
	ToAgent   string        `json:"toAgent"`
	Task      string        `json:"task"`
	Status    HandoffState  `json:"status"`
	StartTime time.Time     `json:"startTime"`
	EndTime   *time.Time    `json:"endTime,omitempty"`
	Result    *HandoffResult `json:"result,omitempty"`
}

// HandoffState defines the state of a handoff
type HandoffState string

const (
	HandoffPending   HandoffState = "pending"
	HandoffExecuting HandoffState = "executing"
	HandoffCompleted HandoffState = "completed"
	HandoffFailed    HandoffState = "failed"
)

// TimelineEvent represents an event in the agent activity timeline
type TimelineEvent struct {
	Time      time.Time `json:"time"`
	Agent     string    `json:"agent"`
	EventType EventType `json:"eventType"`
	Message   string    `json:"message"`
	Details   string    `json:"details,omitempty"`
}

// EventType defines types of timeline events
type EventType string

const (
	EventStarted   EventType = "started"
	EventCompleted EventType = "completed"
	EventHandoff   EventType = "handoff"
	EventError     EventType = "error"
	EventInfo      EventType = "info"
)

// GetAgentNames returns unique agent names from plan steps
func (p *ExecutionPlan) GetAgentNames() []string {
	seen := make(map[string]bool)
	agents := make([]string, 0)

	for _, step := range p.Steps {
		if !seen[step.Agent] {
			seen[step.Agent] = true
			agents = append(agents, step.Agent)
		}
	}

	return agents
}

// IsComplete checks if a plan execution is complete
func (e *PlanExecution) IsComplete() bool {
	return e.Status == PlanCompleted || e.Status == PlanFailed || e.Status == PlanCancelled
}

// GetProgress returns execution progress as a percentage
func (e *PlanExecution) GetProgress() float64 {
	if len(e.Plan.Steps) == 0 {
		return 0
	}
	completed := 0
	for _, result := range e.StepResults {
		if result != nil && result.Status == ResultSuccess {
			completed++
		}
	}
	return float64(completed) / float64(len(e.Plan.Steps)) * 100
}
