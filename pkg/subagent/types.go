// Package subagent provides sub-agent lifecycle management.
// Sub-agents are isolated execution contexts that can run tasks
// in parallel or sequentially with varying levels of visibility.
package subagent

import (
	"context"
	"time"
)

// TrustLevel defines the visibility and autonomy of a sub-agent.
type TrustLevel int

const (
	// Level0Visible - Sub-agent is visible in TaskList, results require acknowledgment.
	// This is the default for new or untrusted workflows.
	Level0Visible TrustLevel = iota

	// Level1Trusted - Sub-agent is visible, results auto-accepted.
	// Failures still surface to user.
	Level1Trusted

	// Level2Background - Sub-agent runs in background, not visible unless requested.
	// Only failures surface to user. Requires "promotion ceremony".
	Level2Background
)

func (t TrustLevel) String() string {
	switch t {
	case Level0Visible:
		return "Level0 (Visible)"
	case Level1Trusted:
		return "Level1 (Trusted)"
	case Level2Background:
		return "Level2 (Background)"
	default:
		return "Unknown"
	}
}

// Status represents the current state of a sub-agent.
type Status string

const (
	StatusPending    Status = "pending"
	StatusRunning    Status = "running"
	StatusWaiting    Status = "waiting"   // Waiting for input/approval
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
	StatusCancelled  Status = "cancelled"
)

// SubAgent represents an isolated execution context.
type SubAgent struct {
	// Identity
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	AgentType   string    `json:"agent_type"` // The manifest agent type
	ParentID    string    `json:"parent_id,omitempty"` // Parent sub-agent if nested

	// Configuration
	TrustLevel  TrustLevel `json:"trust_level"`
	Timeout     time.Duration `json:"timeout,omitempty"`
	MaxRetries  int        `json:"max_retries,omitempty"`

	// State
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Task information
	Task        string    `json:"task"`
	Context     *Context  `json:"context,omitempty"`
	Result      *Result   `json:"result,omitempty"`

	// Metrics
	SuccessCount int      `json:"success_count"`
	FailureCount int      `json:"failure_count"`
}

// Context represents the isolated context for a sub-agent.
type Context struct {
	// SessionID links to the parent session
	SessionID   string `json:"session_id"`

	// WorkingDir is the directory context for file operations
	WorkingDir  string `json:"working_dir"`

	// Variables available to the sub-agent
	Variables   map[string]any `json:"variables,omitempty"`

	// Messages is the conversation history for this sub-agent
	Messages    []Message `json:"messages,omitempty"`

	// Tools available to this sub-agent (subset of parent's tools)
	AllowedTools []string `json:"allowed_tools,omitempty"`

	// Constraints on this sub-agent
	Constraints Constraints `json:"constraints,omitempty"`
}

// Message represents a message in the sub-agent's conversation.
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// Constraints define limits on sub-agent behavior.
type Constraints struct {
	// MaxTokens limits the total tokens used
	MaxTokens   int `json:"max_tokens,omitempty"`

	// MaxToolCalls limits the number of tool invocations
	MaxToolCalls int `json:"max_tool_calls,omitempty"`

	// AllowedPaths restricts file access
	AllowedPaths []string `json:"allowed_paths,omitempty"`

	// DeniedPaths blocks file access
	DeniedPaths []string `json:"denied_paths,omitempty"`

	// AllowBash controls whether bash execution is allowed
	AllowBash bool `json:"allow_bash"`

	// AllowNetwork controls whether network access is allowed
	AllowNetwork bool `json:"allow_network"`

	// RequireApproval forces all operations to require approval
	RequireApproval bool `json:"require_approval"`
}

// Result represents the outcome of a sub-agent's execution.
type Result struct {
	// Success indicates whether the task completed successfully
	Success     bool   `json:"success"`

	// Output is the main result content
	Output      string `json:"output"`

	// Error contains error details if failed
	Error       string `json:"error,omitempty"`

	// Artifacts are any files or data produced
	Artifacts   []Artifact `json:"artifacts,omitempty"`

	// Metrics about the execution
	TokensUsed  int       `json:"tokens_used"`
	ToolCalls   int       `json:"tool_calls"`
	Duration    time.Duration `json:"duration"`
}

// Artifact represents a file or data produced by a sub-agent.
type Artifact struct {
	Type    ArtifactType `json:"type"`
	Name    string       `json:"name"`
	Path    string       `json:"path,omitempty"`
	Content string       `json:"content,omitempty"`
	Size    int64        `json:"size,omitempty"`
}

// ArtifactType categorizes artifacts.
type ArtifactType string

const (
	ArtifactFile   ArtifactType = "file"
	ArtifactCode   ArtifactType = "code"
	ArtifactReport ArtifactType = "report"
	ArtifactData   ArtifactType = "data"
)

// SpawnRequest contains parameters for creating a new sub-agent.
type SpawnRequest struct {
	// Name is a human-readable identifier
	Name        string `json:"name"`

	// AgentType is the manifest agent type to use
	AgentType   string `json:"agent_type"`

	// Task is the task description
	Task        string `json:"task"`

	// TrustLevel for this sub-agent
	TrustLevel  TrustLevel `json:"trust_level"`

	// Context configuration
	WorkingDir  string            `json:"working_dir,omitempty"`
	Variables   map[string]any    `json:"variables,omitempty"`
	AllowedTools []string         `json:"allowed_tools,omitempty"`
	Constraints Constraints       `json:"constraints,omitempty"`

	// Execution options
	Timeout     time.Duration     `json:"timeout,omitempty"`
	MaxRetries  int               `json:"max_retries,omitempty"`
	RunInBackground bool          `json:"run_in_background,omitempty"`
}

// ParallelRequest contains parameters for parallel sub-agent execution.
type ParallelRequest struct {
	// Agents to spawn and run in parallel
	Agents []SpawnRequest `json:"agents"`

	// WaitAll waits for all agents to complete
	WaitAll bool `json:"wait_all"`

	// FailFast stops all agents if one fails
	FailFast bool `json:"fail_fast"`

	// Timeout for the entire parallel operation
	Timeout time.Duration `json:"timeout,omitempty"`
}

// ParallelResult contains results from parallel execution.
type ParallelResult struct {
	// Results maps agent ID to result
	Results map[string]*Result `json:"results"`

	// Completed lists IDs of completed agents
	Completed []string `json:"completed"`

	// Failed lists IDs of failed agents
	Failed []string `json:"failed"`

	// Cancelled lists IDs of cancelled agents
	Cancelled []string `json:"cancelled"`

	// Duration of the entire parallel operation
	Duration time.Duration `json:"duration"`
}

// PromotionCriteria defines requirements for trust level promotion.
type PromotionCriteria struct {
	// MinSuccessCount is the minimum consecutive successes required
	MinSuccessCount int `json:"min_success_count"`

	// MaxFailureCount is the maximum failures allowed
	MaxFailureCount int `json:"max_failure_count"`

	// NoUserIntervention requires no user overrides
	NoUserIntervention bool `json:"no_user_intervention"`

	// LowRiskOnly restricts to low-risk operations
	LowRiskOnly bool `json:"low_risk_only"`
}

// DefaultPromotionCriteria returns the standard promotion criteria.
func DefaultPromotionCriteria() PromotionCriteria {
	return PromotionCriteria{
		MinSuccessCount:    5,
		MaxFailureCount:    0,
		NoUserIntervention: true,
		LowRiskOnly:        true,
	}
}

// Event represents a sub-agent lifecycle event.
type Event struct {
	Type      EventType  `json:"type"`
	AgentID   string     `json:"agent_id"`
	Timestamp time.Time  `json:"timestamp"`
	Data      any        `json:"data,omitempty"`
}

// EventType categorizes sub-agent events.
type EventType string

const (
	EventSpawned    EventType = "spawned"
	EventStarted    EventType = "started"
	EventProgress   EventType = "progress"
	EventWaiting    EventType = "waiting"
	EventCompleted  EventType = "completed"
	EventFailed     EventType = "failed"
	EventCancelled  EventType = "cancelled"
	EventPromoted   EventType = "promoted"
	EventDemoted    EventType = "demoted"
)

// Observer receives sub-agent events.
type Observer interface {
	OnEvent(ctx context.Context, event Event)
}

// ObserverFunc is a function adapter for Observer.
type ObserverFunc func(ctx context.Context, event Event)

func (f ObserverFunc) OnEvent(ctx context.Context, event Event) {
	f(ctx, event)
}
