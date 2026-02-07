package hooks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/syntor/syntor/pkg/tools/security"
)

// SecurityHandler checks for security concerns in tool calls.
type SecurityHandler struct{}

func (h *SecurityHandler) Execute(ctx context.Context, hookCtx *HookContext) (*HookResult, error) {
	// Check for dangerous commands in bash
	if hookCtx.ToolName == "bash" {
		if cmd, ok := hookCtx.ToolParams["command"].(string); ok {
			if isDangerousCommand(cmd) {
				return &HookResult{
					Action:  ActionBlock,
					Reason:  "Command appears to be destructive or dangerous",
					Message: fmt.Sprintf("Blocked potentially dangerous command: %s", truncate(cmd, 100)),
				}, nil
			}
		}
	}

	// Check for sensitive file paths
	if path := getPathFromParams(hookCtx.ToolParams); path != "" {
		if isSensitivePath(path) {
			return &HookResult{
				Action:  ActionBlock,
				Reason:  "Operation on sensitive system path",
				Message: fmt.Sprintf("Blocked access to sensitive path: %s", path),
			}, nil
		}
	}

	return &HookResult{Action: ActionContinue}, nil
}

// isDangerousCommand checks for destructive command patterns.
func isDangerousCommand(cmd string) bool {
	dangerousPatterns := []string{
		`rm\s+-rf\s+/`,           // rm -rf /
		`rm\s+-rf\s+~`,           // rm -rf ~
		`rm\s+-rf\s+\*`,          // rm -rf *
		`mkfs\.`,                 // mkfs
		`:(){.*:\|:&\s*};:`,      // fork bomb
		`dd\s+if=.*of=/dev/`,     // dd to device
		`>\s*/dev/sd`,            // write to disk device
		`chmod\s+-R\s+777\s+/`,   // chmod 777 /
		`curl.*\|\s*sh`,          // curl pipe to shell
		`wget.*\|\s*sh`,          // wget pipe to shell
	}

	for _, pattern := range dangerousPatterns {
		if matched, _ := regexp.MatchString(pattern, cmd); matched {
			return true
		}
	}

	return false
}

// isSensitivePath checks for sensitive system paths.
func isSensitivePath(path string) bool {
	absPath, _ := filepath.Abs(path)

	sensitivePaths := []string{
		"/etc/passwd",
		"/etc/shadow",
		"/etc/sudoers",
		"/root/.ssh",
		"~/.ssh/id_rsa",
		"~/.ssh/id_ed25519",
		"/var/log/auth.log",
		"/etc/hosts",
	}

	for _, sensitive := range sensitivePaths {
		if strings.Contains(absPath, sensitive) {
			return true
		}
	}

	// Block access to user's SSH keys
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(absPath, filepath.Join(home, ".ssh")) {
		if strings.Contains(absPath, "id_") && !strings.HasSuffix(absPath, ".pub") {
			return true
		}
	}

	return false
}

// AuditHandler logs tool usage.
type AuditHandler struct {
	logFile string
	mu      sync.Mutex
}

func (h *AuditHandler) Execute(ctx context.Context, hookCtx *HookContext) (*HookResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Log to file or stdout
	entry := fmt.Sprintf("[%s] Tool: %s, Params: %v\n",
		time.Now().Format(time.RFC3339),
		hookCtx.ToolName,
		hookCtx.ToolParams,
	)

	if h.logFile != "" {
		f, err := os.OpenFile(h.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			defer f.Close()
			f.WriteString(entry)
		}
	}

	// Continue execution - audit doesn't block
	return &HookResult{Action: ActionContinue}, nil
}

// ConfirmHandler prompts for user confirmation.
type ConfirmHandler struct {
	confirmFn func(prompt string) bool
}

func (h *ConfirmHandler) Execute(ctx context.Context, hookCtx *HookContext) (*HookResult, error) {
	// If no confirm function is set, continue
	if h.confirmFn == nil {
		return &HookResult{Action: ActionContinue}, nil
	}

	prompt := fmt.Sprintf("Allow %s with parameters %v?", hookCtx.ToolName, hookCtx.ToolParams)
	if h.confirmFn(prompt) {
		return &HookResult{Action: ActionApprove}, nil
	}

	return &HookResult{
		Action: ActionBlock,
		Reason: "User denied confirmation",
	}, nil
}

// DLPHandler checks for sensitive data patterns.
type DLPHandler struct {
	patterns []*regexp.Regexp
}

// NewDLPHandler creates a DLP handler with default patterns.
func NewDLPHandler() *DLPHandler {
	patterns := []string{
		// API keys
		`(?i)(api[_-]?key|apikey)[\s:=]+['"]?[a-zA-Z0-9_-]{20,}`,
		// AWS keys
		`AKIA[0-9A-Z]{16}`,
		// Private keys
		`-----BEGIN (RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`,
		// Passwords in config
		`(?i)(password|passwd|pwd)[\s:=]+['"]?[^\s'"]{8,}`,
		// JWT tokens
		`eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*`,
		// Credit card numbers (basic)
		`\b\d{4}[- ]?\d{4}[- ]?\d{4}[- ]?\d{4}\b`,
		// SSN (US)
		`\b\d{3}-\d{2}-\d{4}\b`,
	}

	var compiled []*regexp.Regexp
	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			compiled = append(compiled, re)
		}
	}

	return &DLPHandler{patterns: compiled}
}

func (h *DLPHandler) Execute(ctx context.Context, hookCtx *HookContext) (*HookResult, error) {
	// Check tool parameters for sensitive data
	for key, value := range hookCtx.ToolParams {
		if str, ok := value.(string); ok {
			for _, pattern := range h.patterns {
				if pattern.MatchString(str) {
					return &HookResult{
						Action:  ActionBlock,
						Reason:  fmt.Sprintf("Sensitive data detected in parameter '%s'", key),
						Message: "Blocked: potential sensitive data exposure",
					}, nil
				}
			}
		}
	}

	// Check file content for write operations
	if hookCtx.ToolName == "write_file" || hookCtx.ToolName == "edit_file" {
		if content, ok := hookCtx.ToolParams["content"].(string); ok {
			for _, pattern := range h.patterns {
				if pattern.MatchString(content) {
					return &HookResult{
						Action:  ActionBlock,
						Reason:  "Sensitive data detected in file content",
						Message: "Blocked: potential sensitive data in file write",
					}, nil
				}
			}
		}
	}

	return &HookResult{Action: ActionContinue}, nil
}

// RateLimitHandler enforces rate limits.
type RateLimitHandler struct {
	windowSize    time.Duration
	maxOperations int
	history       map[string][]time.Time
	mu            sync.Mutex
}

// NewRateLimitHandler creates a rate limit handler.
func NewRateLimitHandler(windowSize time.Duration, maxOperations int) *RateLimitHandler {
	return &RateLimitHandler{
		windowSize:    windowSize,
		maxOperations: maxOperations,
		history:       make(map[string][]time.Time),
	}
}

func (h *RateLimitHandler) Execute(ctx context.Context, hookCtx *HookContext) (*HookResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	key := hookCtx.ToolName
	now := time.Now()
	cutoff := now.Add(-h.windowSize)

	// Clean old entries
	var recent []time.Time
	for _, t := range h.history[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}

	// Check limit
	if len(recent) >= h.maxOperations {
		return &HookResult{
			Action:  ActionBlock,
			Reason:  fmt.Sprintf("Rate limit exceeded: %d operations in %s", h.maxOperations, h.windowSize),
			Message: "Please wait before performing more operations",
		}, nil
	}

	// Record this operation
	h.history[key] = append(recent, now)

	return &HookResult{Action: ActionContinue}, nil
}

// GitSafetyHandler validates git commands for safety.
type GitSafetyHandler struct{}

func (h *GitSafetyHandler) Execute(ctx context.Context, hookCtx *HookContext) (*HookResult, error) {
	if hookCtx.ToolName != "bash" {
		return &HookResult{Action: ActionContinue}, nil
	}

	cmd, ok := hookCtx.ToolParams["command"].(string)
	if !ok {
		return &HookResult{Action: ActionContinue}, nil
	}

	if !security.IsGitCommand(cmd) {
		return &HookResult{Action: ActionContinue}, nil
	}

	result := security.ValidateGitCommand(cmd)
	if result == nil {
		return &HookResult{Action: ActionContinue}, nil
	}

	// Map security package actions to hook actions
	var action HookAction
	switch result.Action {
	case security.GitActionBlock:
		action = ActionBlock
	case security.GitActionConfirm:
		action = ActionConfirm
	default:
		action = ActionContinue
	}

	return &HookResult{
		Action:  action,
		Reason:  result.Reason,
		Message: result.Message,
	}, nil
}

// Helper functions

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
