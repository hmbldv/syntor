package tools

import (
	"context"
	"fmt"
	"time"
)

// ToolCall represents a tool invocation request from the LLM
type ToolCall struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters"`
}

// ToolCallBatch represents multiple tool calls in a single response
type ToolCallBatch struct {
	ToolCalls []ToolCall `json:"tool_calls"`
}

// ToolResult represents the result of executing a tool
type ToolResult struct {
	CallID    string        `json:"call_id"`
	ToolName  string        `json:"tool_name"`
	Success   bool          `json:"success"`
	Output    interface{}   `json:"output,omitempty"`
	Error     *ToolError    `json:"error,omitempty"`
	Duration  time.Duration `json:"duration"`
	Truncated bool          `json:"truncated,omitempty"`
}

// ToolError represents an error during tool execution
type ToolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// Error implements the error interface
func (e *ToolError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ApprovalRequest represents a tool call waiting for user approval
type ApprovalRequest struct {
	ID          string    `json:"id"`
	ToolCall    ToolCall  `json:"tool_call"`
	Reason      string    `json:"reason"`
	Risk        RiskLevel `json:"risk"`
	RequestedAt time.Time `json:"requested_at"`
}

// RiskLevel indicates the risk level of a tool operation
type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// Tool is the interface all tools must implement
type Tool interface {
	// Name returns the tool's unique identifier
	Name() string

	// Description returns a human-readable description
	Description() string

	// Execute runs the tool with the given parameters
	Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error)

	// Validate checks if the parameters are valid
	Validate(params map[string]interface{}) error
}

// ToolDefinition defines a tool's metadata and parameters (from YAML)
type ToolDefinition struct {
	Name        string           `yaml:"name" json:"name"`
	Description string           `yaml:"description" json:"description"`
	Version     string           `yaml:"version,omitempty" json:"version,omitempty"`
	Category    ToolCategory     `yaml:"category" json:"category"`
	Parameters  []ToolParameter  `yaml:"parameters" json:"parameters"`
	Returns     ToolReturnSpec   `yaml:"returns" json:"returns"`
	Security    ToolSecuritySpec `yaml:"security,omitempty" json:"security,omitempty"`
}

// ToolCategory categorizes tools by their function
type ToolCategory string

const (
	ToolCategoryFilesystem ToolCategory = "filesystem"
	ToolCategoryShell      ToolCategory = "shell"
	ToolCategorySearch     ToolCategory = "search"
	ToolCategoryAgent      ToolCategory = "agent"
)

// ToolParameter defines a single parameter for a tool
type ToolParameter struct {
	Name        string      `yaml:"name" json:"name"`
	Type        string      `yaml:"type" json:"type"` // "string", "integer", "boolean", "array", "object"
	Description string      `yaml:"description" json:"description"`
	Required    bool        `yaml:"required" json:"required"`
	Default     interface{} `yaml:"default,omitempty" json:"default,omitempty"`
	Enum        []string    `yaml:"enum,omitempty" json:"enum,omitempty"`
	Validation  string      `yaml:"validation,omitempty" json:"validation,omitempty"` // regex pattern
}

// ToolReturnSpec describes what a tool returns
type ToolReturnSpec struct {
	Type        string `yaml:"type" json:"type"`
	Description string `yaml:"description" json:"description"`
}

// ToolSecuritySpec defines security constraints for a tool
type ToolSecuritySpec struct {
	RequireApproval  bool     `yaml:"requireApproval" json:"requireApproval"`
	AllowedPaths     []string `yaml:"allowedPaths,omitempty" json:"allowedPaths,omitempty"`
	DeniedPaths      []string `yaml:"deniedPaths,omitempty" json:"deniedPaths,omitempty"`
	Sandbox          bool     `yaml:"sandbox" json:"sandbox"`
	MaxExecutionTime string   `yaml:"maxExecutionTime,omitempty" json:"maxExecutionTime,omitempty"`
	Allowlist        []string `yaml:"allowlist,omitempty" json:"allowlist,omitempty"`
	Denylist         []string `yaml:"denylist,omitempty" json:"denylist,omitempty"`
}

// ExecuteOptions configures tool execution
type ExecuteOptions struct {
	RequireApproval bool
	WorkingDir      string
	Timeout         time.Duration
	DryRun          bool
	PlanMode        bool
}

// Common error codes
const (
	ErrCodeToolNotFound     = "TOOL_NOT_FOUND"
	ErrCodeInvalidParams    = "INVALID_PARAMS"
	ErrCodeSecurityViolation = "SECURITY_VIOLATION"
	ErrCodeExecutionError   = "EXECUTION_ERROR"
	ErrCodeTimeout          = "TIMEOUT"
	ErrCodeFileNotFound     = "FILE_NOT_FOUND"
	ErrCodePermissionDenied = "PERMISSION_DENIED"
	ErrCodeApprovalRequired = "APPROVAL_REQUIRED"
)
