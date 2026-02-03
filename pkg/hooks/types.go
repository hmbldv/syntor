// Package hooks provides pre/post tool execution hooks.
// Hooks allow users to approve, modify, or block tool usage
// based on custom logic or shell commands.
package hooks

import (
	"time"
)

// HookType defines when a hook is executed.
type HookType string

const (
	// HookPreToolUse runs before a tool is executed.
	HookPreToolUse HookType = "pre_tool_use"

	// HookPostToolUse runs after a tool completes.
	HookPostToolUse HookType = "post_tool_use"

	// HookPreSession runs when a session starts.
	HookPreSession HookType = "pre_session"

	// HookPostSession runs when a session ends.
	HookPostSession HookType = "post_session"

	// HookOnError runs when an error occurs.
	HookOnError HookType = "on_error"

	// HookPromptSubmit runs when user submits a prompt.
	HookPromptSubmit HookType = "prompt_submit"
)

// HookAction specifies what to do with tool execution.
type HookAction string

const (
	// ActionApprove allows the tool to execute.
	ActionApprove HookAction = "approve"

	// ActionBlock prevents the tool from executing.
	ActionBlock HookAction = "block"

	// ActionModify allows execution with modified parameters.
	ActionModify HookAction = "modify"

	// ActionContinue proceeds to the next hook (default).
	ActionContinue HookAction = "continue"
)

// Hook defines a hook that can intercept tool execution.
type Hook struct {
	// Identity
	ID          string   `json:"id" yaml:"id"`
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty"`

	// When to run
	Type    HookType `json:"type" yaml:"type"`
	Enabled bool     `json:"enabled" yaml:"enabled"`
	Order   int      `json:"order" yaml:"order"` // Lower runs first

	// Matching criteria
	ToolMatch    []string `json:"tool_match,omitempty" yaml:"tool_match,omitempty"`       // Tool names to match (glob patterns)
	ToolExclude  []string `json:"tool_exclude,omitempty" yaml:"tool_exclude,omitempty"`   // Tool names to exclude
	PathMatch    []string `json:"path_match,omitempty" yaml:"path_match,omitempty"`       // File paths to match
	PathExclude  []string `json:"path_exclude,omitempty" yaml:"path_exclude,omitempty"`   // File paths to exclude

	// Execution
	Handler HandlerType `json:"handler" yaml:"handler"`

	// Shell handler config
	Command   string            `json:"command,omitempty" yaml:"command,omitempty"`
	Timeout   time.Duration     `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Env       map[string]string `json:"env,omitempty" yaml:"env,omitempty"`

	// Prompt handler config
	PromptTemplate string `json:"prompt_template,omitempty" yaml:"prompt_template,omitempty"`
}

// HandlerType defines how the hook is executed.
type HandlerType string

const (
	// HandlerShell runs a shell command.
	HandlerShell HandlerType = "shell"

	// HandlerPrompt uses an LLM prompt to decide.
	HandlerPrompt HandlerType = "prompt"

	// HandlerBuiltin uses a built-in handler.
	HandlerBuiltin HandlerType = "builtin"
)

// HookContext provides context for hook execution.
type HookContext struct {
	// Session information
	SessionID   string `json:"session_id"`
	WorkingDir  string `json:"working_dir"`

	// Tool information (for tool hooks)
	ToolName    string         `json:"tool_name,omitempty"`
	ToolParams  map[string]any `json:"tool_params,omitempty"`

	// Result information (for post hooks)
	ToolResult  string `json:"tool_result,omitempty"`
	ToolError   string `json:"tool_error,omitempty"`
	ToolSuccess bool   `json:"tool_success,omitempty"`

	// Prompt information (for prompt hooks)
	UserPrompt  string `json:"user_prompt,omitempty"`

	// Error information (for error hooks)
	ErrorMessage string `json:"error_message,omitempty"`
	ErrorType    string `json:"error_type,omitempty"`

	// Additional context
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// HookResult is the outcome of a hook execution.
type HookResult struct {
	// Action to take
	Action  HookAction `json:"action"`
	Reason  string     `json:"reason,omitempty"`

	// Modified parameters (if Action == ActionModify)
	ModifiedParams map[string]any `json:"modified_params,omitempty"`

	// Feedback message to show user
	Message string `json:"message,omitempty"`

	// Execution details
	Duration time.Duration `json:"duration"`
	HookID   string        `json:"hook_id"`
}

// BuiltinHook identifies built-in hook implementations.
type BuiltinHook string

const (
	// BuiltinDLP runs DLP (Data Loss Prevention) checks.
	BuiltinDLP BuiltinHook = "dlp"

	// BuiltinSecurity runs security validation.
	BuiltinSecurity BuiltinHook = "security"

	// BuiltinRateLimit enforces rate limiting.
	BuiltinRateLimit BuiltinHook = "rate_limit"

	// BuiltinAudit logs tool usage for auditing.
	BuiltinAudit BuiltinHook = "audit"

	// BuiltinConfirm prompts for user confirmation.
	BuiltinConfirm BuiltinHook = "confirm"
)

// Config holds hooks configuration.
type Config struct {
	// Hooks is the list of configured hooks
	Hooks []Hook `json:"hooks" yaml:"hooks"`

	// GlobalTimeout is the default timeout for hooks
	GlobalTimeout time.Duration `json:"global_timeout" yaml:"global_timeout"`

	// FailOpen determines behavior when a hook errors
	// If true, tool execution proceeds on hook error
	// If false, tool execution is blocked on hook error
	FailOpen bool `json:"fail_open" yaml:"fail_open"`

	// EnableBuiltins enables default built-in hooks
	EnableBuiltins bool `json:"enable_builtins" yaml:"enable_builtins"`

	// ConfigPath is the path to the hooks config file
	ConfigPath string `json:"config_path" yaml:"config_path"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		GlobalTimeout:  10 * time.Second,
		FailOpen:       false,
		EnableBuiltins: true,
		ConfigPath:     "~/.syntor/hooks.yaml",
	}
}

// ShellOutput captures the output of a shell hook.
type ShellOutput struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// PromptOutput captures the output of a prompt hook.
type PromptOutput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}
