package subagent

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ContextBuilder helps construct isolated contexts for sub-agents.
type ContextBuilder struct {
	context *Context
}

// NewContextBuilder creates a new context builder.
func NewContextBuilder() *ContextBuilder {
	return &ContextBuilder{
		context: &Context{
			Variables:    make(map[string]any),
			Messages:     nil,
			AllowedTools: nil,
			Constraints:  Constraints{},
		},
	}
}

// WithSessionID sets the parent session ID.
func (b *ContextBuilder) WithSessionID(sessionID string) *ContextBuilder {
	b.context.SessionID = sessionID
	return b
}

// WithWorkingDir sets the working directory.
func (b *ContextBuilder) WithWorkingDir(dir string) *ContextBuilder {
	b.context.WorkingDir = dir
	return b
}

// WithVariable adds a variable to the context.
func (b *ContextBuilder) WithVariable(key string, value any) *ContextBuilder {
	b.context.Variables[key] = value
	return b
}

// WithVariables adds multiple variables to the context.
func (b *ContextBuilder) WithVariables(vars map[string]any) *ContextBuilder {
	for k, v := range vars {
		b.context.Variables[k] = v
	}
	return b
}

// WithTools sets the allowed tools.
func (b *ContextBuilder) WithTools(tools ...string) *ContextBuilder {
	b.context.AllowedTools = tools
	return b
}

// WithReadOnlyTools sets common read-only tools.
func (b *ContextBuilder) WithReadOnlyTools() *ContextBuilder {
	b.context.AllowedTools = []string{
		"read_file",
		"list_directory",
		"glob",
		"grep",
	}
	return b
}

// WithWriteTools adds file modification tools.
func (b *ContextBuilder) WithWriteTools() *ContextBuilder {
	b.context.AllowedTools = append(b.context.AllowedTools,
		"write_file",
		"edit_file",
	)
	return b
}

// WithBashTool adds bash execution capability.
func (b *ContextBuilder) WithBashTool() *ContextBuilder {
	b.context.AllowedTools = append(b.context.AllowedTools, "bash")
	b.context.Constraints.AllowBash = true
	return b
}

// WithConstraints sets the constraints.
func (b *ContextBuilder) WithConstraints(c Constraints) *ContextBuilder {
	b.context.Constraints = c
	return b
}

// WithMaxTokens sets the token limit.
func (b *ContextBuilder) WithMaxTokens(max int) *ContextBuilder {
	b.context.Constraints.MaxTokens = max
	return b
}

// WithMaxToolCalls sets the tool call limit.
func (b *ContextBuilder) WithMaxToolCalls(max int) *ContextBuilder {
	b.context.Constraints.MaxToolCalls = max
	return b
}

// WithAllowedPaths sets allowed file paths.
func (b *ContextBuilder) WithAllowedPaths(paths ...string) *ContextBuilder {
	b.context.Constraints.AllowedPaths = paths
	return b
}

// WithDeniedPaths sets denied file paths.
func (b *ContextBuilder) WithDeniedPaths(paths ...string) *ContextBuilder {
	b.context.Constraints.DeniedPaths = paths
	return b
}

// WithNetworkAccess enables network operations.
func (b *ContextBuilder) WithNetworkAccess() *ContextBuilder {
	b.context.Constraints.AllowNetwork = true
	return b
}

// WithApprovalRequired forces all operations to require approval.
func (b *ContextBuilder) WithApprovalRequired() *ContextBuilder {
	b.context.Constraints.RequireApproval = true
	return b
}

// WithMessage adds an initial message to the context.
func (b *ContextBuilder) WithMessage(role, content string) *ContextBuilder {
	b.context.Messages = append(b.context.Messages, Message{
		Role:    role,
		Content: content,
	})
	return b
}

// WithSystemMessage adds a system message.
func (b *ContextBuilder) WithSystemMessage(content string) *ContextBuilder {
	return b.WithMessage("system", content)
}

// Build returns the constructed context.
func (b *ContextBuilder) Build() *Context {
	return b.context
}

// InheritFrom creates a context that inherits from a parent context.
func InheritFrom(parent *Context) *ContextBuilder {
	builder := NewContextBuilder()

	if parent != nil {
		builder.context.SessionID = parent.SessionID
		builder.context.WorkingDir = parent.WorkingDir

		// Deep copy variables
		for k, v := range parent.Variables {
			builder.context.Variables[k] = v
		}

		// Copy allowed tools (can be restricted further)
		builder.context.AllowedTools = make([]string, len(parent.AllowedTools))
		copy(builder.context.AllowedTools, parent.AllowedTools)

		// Copy constraints (can be made more restrictive)
		builder.context.Constraints = parent.Constraints
	}

	return builder
}

// RestrictedContext creates a highly restricted context.
func RestrictedContext(sessionID, workingDir string) *Context {
	return NewContextBuilder().
		WithSessionID(sessionID).
		WithWorkingDir(workingDir).
		WithReadOnlyTools().
		WithMaxTokens(4096).
		WithMaxToolCalls(10).
		WithAllowedPaths(workingDir).
		WithApprovalRequired().
		Build()
}

// TrustedContext creates a context with broader permissions.
func TrustedContext(sessionID, workingDir string) *Context {
	return NewContextBuilder().
		WithSessionID(sessionID).
		WithWorkingDir(workingDir).
		WithReadOnlyTools().
		WithWriteTools().
		WithBashTool().
		WithMaxTokens(16384).
		WithMaxToolCalls(50).
		WithAllowedPaths(workingDir).
		Build()
}

// ContextValidator validates context constraints.
type ContextValidator struct {
	context *Context
}

// NewContextValidator creates a validator for the given context.
func NewContextValidator(ctx *Context) *ContextValidator {
	return &ContextValidator{context: ctx}
}

// ValidateTool checks if a tool is allowed.
func (v *ContextValidator) ValidateTool(toolName string) error {
	if len(v.context.AllowedTools) == 0 {
		return nil // No restrictions
	}

	for _, allowed := range v.context.AllowedTools {
		if allowed == toolName {
			return nil
		}
	}

	return fmt.Errorf("tool not allowed: %s", toolName)
}

// ValidatePath checks if a path is allowed.
func (v *ContextValidator) ValidatePath(path string) error {
	// Normalize path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Check denied paths first
	for _, denied := range v.context.Constraints.DeniedPaths {
		if strings.HasPrefix(absPath, denied) {
			return fmt.Errorf("path is denied: %s", path)
		}
	}

	// Check allowed paths
	if len(v.context.Constraints.AllowedPaths) > 0 {
		allowed := false
		for _, allowedPath := range v.context.Constraints.AllowedPaths {
			if strings.HasPrefix(absPath, allowedPath) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("path not in allowed paths: %s", path)
		}
	}

	return nil
}

// ValidateBash checks if bash execution is allowed.
func (v *ContextValidator) ValidateBash() error {
	if !v.context.Constraints.AllowBash {
		return fmt.Errorf("bash execution not allowed")
	}
	return nil
}

// ValidateNetwork checks if network access is allowed.
func (v *ContextValidator) ValidateNetwork() error {
	if !v.context.Constraints.AllowNetwork {
		return fmt.Errorf("network access not allowed")
	}
	return nil
}

// TokenTracker tracks token usage against limits.
type TokenTracker struct {
	limit int
	used  int
}

// NewTokenTracker creates a token tracker.
func NewTokenTracker(limit int) *TokenTracker {
	return &TokenTracker{limit: limit}
}

// Add adds tokens to the tracker.
func (t *TokenTracker) Add(tokens int) error {
	if t.limit > 0 && t.used+tokens > t.limit {
		return fmt.Errorf("token limit exceeded: %d + %d > %d", t.used, tokens, t.limit)
	}
	t.used += tokens
	return nil
}

// Remaining returns the remaining token budget.
func (t *TokenTracker) Remaining() int {
	if t.limit <= 0 {
		return -1 // Unlimited
	}
	return t.limit - t.used
}

// Used returns the total tokens used.
func (t *TokenTracker) Used() int {
	return t.used
}

// ToolCallTracker tracks tool call count against limits.
type ToolCallTracker struct {
	limit int
	count int
}

// NewToolCallTracker creates a tool call tracker.
func NewToolCallTracker(limit int) *ToolCallTracker {
	return &ToolCallTracker{limit: limit}
}

// Increment increments the tool call count.
func (t *ToolCallTracker) Increment() error {
	if t.limit > 0 && t.count >= t.limit {
		return fmt.Errorf("tool call limit reached: %d", t.limit)
	}
	t.count++
	return nil
}

// Remaining returns the remaining tool call budget.
func (t *ToolCallTracker) Remaining() int {
	if t.limit <= 0 {
		return -1 // Unlimited
	}
	return t.limit - t.count
}

// Count returns the total tool calls made.
func (t *ToolCallTracker) Count() int {
	return t.count
}
