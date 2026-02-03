// Package herald provides a client for the Herald service gateway.
// Herald is the primary interface for all infrastructure services including
// LLM inference, session management, trust tiers, and approval workflows.
package herald

import (
	"time"
)

// TrustTier represents the trust level for operations.
type TrustTier int

const (
	// T0 is the lowest trust tier - requires explicit user approval for everything.
	T0 TrustTier = iota
	// T1 allows read-only operations without approval.
	T1
	// T2 allows file modifications with approval.
	T2
	// T3 allows system commands with approval.
	T3
	// T4 is full autonomy - no approval required.
	T4
)

func (t TrustTier) String() string {
	switch t {
	case T0:
		return "T0 (Restricted)"
	case T1:
		return "T1 (Read-Only)"
	case T2:
		return "T2 (Modify)"
	case T3:
		return "T3 (Execute)"
	case T4:
		return "T4 (Autonomous)"
	default:
		return "Unknown"
	}
}

// RequiresApproval returns true if this tier requires approval for the given operation.
func (t TrustTier) RequiresApproval(opType OperationType) bool {
	switch opType {
	case OpRead:
		return t < T1
	case OpWrite:
		return t < T2
	case OpExecute:
		return t < T3
	case OpNetwork:
		return t < T4
	default:
		return true
	}
}

// OperationType categorizes operations for trust tier checks.
type OperationType string

const (
	OpRead    OperationType = "read"
	OpWrite   OperationType = "write"
	OpExecute OperationType = "execute"
	OpNetwork OperationType = "network"
)

// Session represents a Herald-managed session.
type Session struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        SessionType       `json:"type"`
	Status      SessionStatus     `json:"status"`
	TrustTier   TrustTier         `json:"trust_tier"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	LastActive  time.Time         `json:"last_active"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	WorkingDir  string            `json:"working_dir,omitempty"`
	AgentName   string            `json:"agent_name,omitempty"`
	ParentID    string            `json:"parent_id,omitempty"` // For forked sessions
	TokensUsed  int64             `json:"tokens_used"`
	MessageCount int              `json:"message_count"`
}

// SessionType indicates what kind of session this is.
type SessionType string

const (
	SessionTypeCLI        SessionType = "cli"        // Claude CLI via tmux
	SessionTypeAgent      SessionType = "agent"      // Agent-based session
	SessionTypeBackground SessionType = "background" // Background task
)

// SessionStatus indicates the current state of a session.
type SessionStatus string

const (
	SessionStatusCreating   SessionStatus = "creating"
	SessionStatusActive     SessionStatus = "active"
	SessionStatusIdle       SessionStatus = "idle"
	SessionStatusSuspended  SessionStatus = "suspended"
	SessionStatusTerminated SessionStatus = "terminated"
	SessionStatusError      SessionStatus = "error"
)

// ApprovalRequest represents a request for user approval.
type ApprovalRequest struct {
	ID          string        `json:"id"`
	SessionID   string        `json:"session_id"`
	Type        ApprovalType  `json:"type"`
	Operation   string        `json:"operation"`
	Description string        `json:"description"`
	Risk        RiskLevel     `json:"risk"`
	Status      ApprovalStatus `json:"status"`
	Context     map[string]any `json:"context,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	ExpiresAt   time.Time     `json:"expires_at"`
	RespondedAt *time.Time    `json:"responded_at,omitempty"`
	RespondedBy string        `json:"responded_by,omitempty"`
	Reason      string        `json:"reason,omitempty"`
}

// ApprovalType categorizes what kind of approval is being requested.
type ApprovalType string

const (
	ApprovalTypeTool     ApprovalType = "tool"
	ApprovalTypePlan     ApprovalType = "plan"
	ApprovalTypeHandoff  ApprovalType = "handoff"
	ApprovalTypeEscalate ApprovalType = "escalate"
)

// RiskLevel indicates the severity of the operation.
type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// ApprovalStatus indicates the current state of an approval request.
type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "pending"
	ApprovalStatusApproved ApprovalStatus = "approved"
	ApprovalStatusDenied   ApprovalStatus = "denied"
	ApprovalStatusExpired  ApprovalStatus = "expired"
)

// InferenceRequest represents a request to the LLM via Herald.
type InferenceRequest struct {
	SessionID    string         `json:"session_id,omitempty"`
	Model        string         `json:"model,omitempty"`
	Messages     []Message      `json:"messages"`
	MaxTokens    int            `json:"max_tokens,omitempty"`
	Temperature  float64        `json:"temperature,omitempty"`
	Stream       bool           `json:"stream,omitempty"`
	Tools        []ToolDef      `json:"tools,omitempty"`
	SystemPrompt string         `json:"system_prompt,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// Message represents a chat message.
type Message struct {
	Role       string         `json:"role"` // user, assistant, system, tool
	Content    string         `json:"content"`
	Name       string         `json:"name,omitempty"`       // For tool messages
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall     `json:"tool_calls,omitempty"`
}

// ToolDef defines a tool available to the model.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCall represents a tool invocation by the model.
type ToolCall struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// InferenceResponse represents the response from inference.
type InferenceResponse struct {
	ID           string    `json:"id"`
	Model        string    `json:"model"`
	Message      Message   `json:"message"`
	FinishReason string    `json:"finish_reason"`
	Usage        Usage     `json:"usage"`
	CreatedAt    time.Time `json:"created_at"`
}

// Usage tracks token consumption.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StreamChunk represents a streaming response chunk.
type StreamChunk struct {
	ID           string  `json:"id"`
	Delta        string  `json:"delta"`
	FinishReason string  `json:"finish_reason,omitempty"`
	ToolCall     *ToolCall `json:"tool_call,omitempty"`
}

// HealthStatus represents Herald service health.
type HealthStatus struct {
	Status      string            `json:"status"` // healthy, degraded, unhealthy
	Services    map[string]string `json:"services"`
	Uptime      time.Duration     `json:"uptime"`
	LastChecked time.Time         `json:"last_checked"`
}

// Error represents an error from Herald.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func (e *Error) Error() string {
	return e.Message
}

// Common error codes
const (
	ErrCodeUnauthorized     = "unauthorized"
	ErrCodeForbidden        = "forbidden"
	ErrCodeNotFound         = "not_found"
	ErrCodeInvalidRequest   = "invalid_request"
	ErrCodeRateLimited      = "rate_limited"
	ErrCodeServiceUnavailable = "service_unavailable"
	ErrCodeInternalError    = "internal_error"
	ErrCodeApprovalRequired = "approval_required"
	ErrCodeApprovalDenied   = "approval_denied"
	ErrCodeSessionExpired   = "session_expired"
)

// CreateSessionRequest is the request to create a new session.
type CreateSessionRequest struct {
	Name       string            `json:"name,omitempty"`
	Type       SessionType       `json:"type"`
	TrustTier  TrustTier         `json:"trust_tier"`
	AgentName  string            `json:"agent_name,omitempty"`
	WorkingDir string            `json:"working_dir,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	ParentID   string            `json:"parent_id,omitempty"` // For forking
}

// UpdateSessionRequest is the request to update a session.
type UpdateSessionRequest struct {
	Name      string            `json:"name,omitempty"`
	TrustTier *TrustTier        `json:"trust_tier,omitempty"`
	Status    SessionStatus     `json:"status,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// ListSessionsFilter provides filtering options for listing sessions.
type ListSessionsFilter struct {
	Status    SessionStatus `json:"status,omitempty"`
	Type      SessionType   `json:"type,omitempty"`
	AgentName string        `json:"agent_name,omitempty"`
	Limit     int           `json:"limit,omitempty"`
	Offset    int           `json:"offset,omitempty"`
}

// ApprovalResponse is the response to an approval request.
type ApprovalResponse struct {
	RequestID string         `json:"request_id"`
	Approved  bool           `json:"approved"`
	Reason    string         `json:"reason,omitempty"`
}
