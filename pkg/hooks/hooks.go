package hooks

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Manager handles hook registration and execution.
type Manager struct {
	config Config

	// Registered hooks by type
	hooks   map[HookType][]*Hook
	hooksMu sync.RWMutex

	// Built-in handlers
	builtins map[BuiltinHook]BuiltinHandler

	// Shell executor
	shellExecutor *ShellExecutor

	// Prompt executor
	promptExecutor *PromptExecutor
}

// BuiltinHandler is the interface for built-in hooks.
type BuiltinHandler interface {
	Execute(ctx context.Context, hookCtx *HookContext) (*HookResult, error)
}

// NewManager creates a new hooks manager.
func NewManager(config Config) (*Manager, error) {
	m := &Manager{
		config:        config,
		hooks:         make(map[HookType][]*Hook),
		builtins:      make(map[BuiltinHook]BuiltinHandler),
		shellExecutor: NewShellExecutor(config.GlobalTimeout),
	}

	// Register built-in handlers
	if config.EnableBuiltins {
		m.registerBuiltins()
	}

	// Load hooks from config
	for i := range config.Hooks {
		if err := m.Register(&config.Hooks[i]); err != nil {
			return nil, fmt.Errorf("register hook %s: %w", config.Hooks[i].ID, err)
		}
	}

	return m, nil
}

// SetPromptExecutor sets the prompt executor (requires inference client).
func (m *Manager) SetPromptExecutor(executor *PromptExecutor) {
	m.promptExecutor = executor
}

// Register adds a hook to the manager.
func (m *Manager) Register(hook *Hook) error {
	if hook.ID == "" {
		return fmt.Errorf("hook ID is required")
	}

	if hook.Type == "" {
		return fmt.Errorf("hook type is required")
	}

	m.hooksMu.Lock()
	defer m.hooksMu.Unlock()

	m.hooks[hook.Type] = append(m.hooks[hook.Type], hook)

	// Sort by order
	sort.Slice(m.hooks[hook.Type], func(i, j int) bool {
		return m.hooks[hook.Type][i].Order < m.hooks[hook.Type][j].Order
	})

	return nil
}

// Unregister removes a hook by ID.
func (m *Manager) Unregister(hookID string) {
	m.hooksMu.Lock()
	defer m.hooksMu.Unlock()

	for hookType, hooks := range m.hooks {
		for i, h := range hooks {
			if h.ID == hookID {
				m.hooks[hookType] = append(hooks[:i], hooks[i+1:]...)
				return
			}
		}
	}
}

// Execute runs all matching hooks of the given type.
func (m *Manager) Execute(ctx context.Context, hookType HookType, hookCtx *HookContext) (*HookResult, error) {
	m.hooksMu.RLock()
	hooks := m.hooks[hookType]
	m.hooksMu.RUnlock()

	// Find matching hooks
	var matching []*Hook
	for _, h := range hooks {
		if !h.Enabled {
			continue
		}
		if m.matches(h, hookCtx) {
			matching = append(matching, h)
		}
	}

	if len(matching) == 0 {
		return &HookResult{Action: ActionContinue}, nil
	}

	// Execute hooks in order
	for _, hook := range matching {
		result, err := m.executeHook(ctx, hook, hookCtx)
		if err != nil {
			if m.config.FailOpen {
				continue
			}
			return &HookResult{
				Action: ActionBlock,
				Reason: fmt.Sprintf("hook %s failed: %v", hook.ID, err),
				HookID: hook.ID,
			}, nil
		}

		// Check if we should stop processing
		switch result.Action {
		case ActionApprove, ActionBlock, ActionModify:
			return result, nil
		case ActionContinue:
			// Continue to next hook
		}
	}

	return &HookResult{Action: ActionContinue}, nil
}

// ExecutePreTool runs pre-tool hooks.
func (m *Manager) ExecutePreTool(ctx context.Context, toolName string, params map[string]any) (*HookResult, error) {
	hookCtx := &HookContext{
		ToolName:   toolName,
		ToolParams: params,
	}
	return m.Execute(ctx, HookPreToolUse, hookCtx)
}

// ExecutePostTool runs post-tool hooks.
func (m *Manager) ExecutePostTool(ctx context.Context, toolName string, params map[string]any, result string, err error) (*HookResult, error) {
	hookCtx := &HookContext{
		ToolName:    toolName,
		ToolParams:  params,
		ToolResult:  result,
		ToolSuccess: err == nil,
	}
	if err != nil {
		hookCtx.ToolError = err.Error()
	}
	return m.Execute(ctx, HookPostToolUse, hookCtx)
}

// matches checks if a hook matches the context.
func (m *Manager) matches(hook *Hook, ctx *HookContext) bool {
	// Check tool name matching
	if len(hook.ToolMatch) > 0 && ctx.ToolName != "" {
		matched := false
		for _, pattern := range hook.ToolMatch {
			if match, _ := filepath.Match(pattern, ctx.ToolName); match {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check tool exclusion
	for _, pattern := range hook.ToolExclude {
		if match, _ := filepath.Match(pattern, ctx.ToolName); match {
			return false
		}
	}

	// Check path matching (for file tools)
	if len(hook.PathMatch) > 0 {
		path := getPathFromParams(ctx.ToolParams)
		if path != "" {
			matched := false
			for _, pattern := range hook.PathMatch {
				if match, _ := filepath.Match(pattern, path); match {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
	}

	// Check path exclusion
	path := getPathFromParams(ctx.ToolParams)
	if path != "" {
		for _, pattern := range hook.PathExclude {
			if match, _ := filepath.Match(pattern, path); match {
				return false
			}
		}
	}

	return true
}

// executeHook runs a single hook.
func (m *Manager) executeHook(ctx context.Context, hook *Hook, hookCtx *HookContext) (*HookResult, error) {
	start := time.Now()

	var result *HookResult
	var err error

	switch hook.Handler {
	case HandlerShell:
		result, err = m.shellExecutor.Execute(ctx, hook, hookCtx)
	case HandlerPrompt:
		if m.promptExecutor == nil {
			return nil, fmt.Errorf("prompt executor not configured")
		}
		result, err = m.promptExecutor.Execute(ctx, hook, hookCtx)
	case HandlerBuiltin:
		result, err = m.executeBuiltin(ctx, hook, hookCtx)
	default:
		return nil, fmt.Errorf("unknown handler type: %s", hook.Handler)
	}

	if err != nil {
		return nil, err
	}

	result.Duration = time.Since(start)
	result.HookID = hook.ID
	return result, nil
}

// executeBuiltin runs a built-in hook handler.
func (m *Manager) executeBuiltin(ctx context.Context, hook *Hook, hookCtx *HookContext) (*HookResult, error) {
	// The hook ID should contain the builtin name
	builtinName := BuiltinHook(hook.ID)
	handler, ok := m.builtins[builtinName]
	if !ok {
		return nil, fmt.Errorf("unknown builtin handler: %s", hook.ID)
	}
	return handler.Execute(ctx, hookCtx)
}

// registerBuiltins registers the default built-in hooks.
func (m *Manager) registerBuiltins() {
	m.builtins[BuiltinSecurity] = &SecurityHandler{}
	m.builtins[BuiltinAudit] = &AuditHandler{}
	m.builtins[BuiltinConfirm] = &ConfirmHandler{}
	// DLP and RateLimit require more complex configuration
}

// getPathFromParams extracts a file path from tool parameters.
func getPathFromParams(params map[string]any) string {
	// Common parameter names for file paths
	pathKeys := []string{"path", "file_path", "filepath", "file", "filename"}
	for _, key := range pathKeys {
		if v, ok := params[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

// List returns all registered hooks.
func (m *Manager) List() map[HookType][]*Hook {
	m.hooksMu.RLock()
	defer m.hooksMu.RUnlock()

	result := make(map[HookType][]*Hook)
	for k, v := range m.hooks {
		hooks := make([]*Hook, len(v))
		copy(hooks, v)
		result[k] = hooks
	}
	return result
}

// Get retrieves a hook by ID.
func (m *Manager) Get(hookID string) *Hook {
	m.hooksMu.RLock()
	defer m.hooksMu.RUnlock()

	for _, hooks := range m.hooks {
		for _, h := range hooks {
			if h.ID == hookID {
				return h
			}
		}
	}
	return nil
}

// Enable enables a hook by ID.
func (m *Manager) Enable(hookID string) error {
	m.hooksMu.Lock()
	defer m.hooksMu.Unlock()

	for _, hooks := range m.hooks {
		for _, h := range hooks {
			if h.ID == hookID {
				h.Enabled = true
				return nil
			}
		}
	}
	return fmt.Errorf("hook not found: %s", hookID)
}

// Disable disables a hook by ID.
func (m *Manager) Disable(hookID string) error {
	m.hooksMu.Lock()
	defer m.hooksMu.Unlock()

	for _, hooks := range m.hooks {
		for _, h := range hooks {
			if h.ID == hookID {
				h.Enabled = false
				return nil
			}
		}
	}
	return fmt.Errorf("hook not found: %s", hookID)
}
