package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ShellExecutor runs shell command hooks.
type ShellExecutor struct {
	defaultTimeout time.Duration
}

// NewShellExecutor creates a shell executor.
func NewShellExecutor(defaultTimeout time.Duration) *ShellExecutor {
	return &ShellExecutor{
		defaultTimeout: defaultTimeout,
	}
}

// Execute runs a shell hook.
func (e *ShellExecutor) Execute(ctx context.Context, hook *Hook, hookCtx *HookContext) (*HookResult, error) {
	if hook.Command == "" {
		return nil, fmt.Errorf("shell hook has no command")
	}

	// Set timeout
	timeout := hook.Timeout
	if timeout == 0 {
		timeout = e.defaultTimeout
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Prepare command
	cmd := exec.CommandContext(execCtx, "sh", "-c", hook.Command)

	// Set environment variables
	cmd.Env = os.Environ()
	for k, v := range hook.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Pass hook context as JSON via environment
	ctxJSON, _ := json.Marshal(hookCtx)
	cmd.Env = append(cmd.Env, fmt.Sprintf("SYNTOR_HOOK_CONTEXT=%s", string(ctxJSON)))
	cmd.Env = append(cmd.Env, fmt.Sprintf("SYNTOR_TOOL_NAME=%s", hookCtx.ToolName))

	// Pass individual parameters
	if hookCtx.ToolParams != nil {
		paramsJSON, _ := json.Marshal(hookCtx.ToolParams)
		cmd.Env = append(cmd.Env, fmt.Sprintf("SYNTOR_TOOL_PARAMS=%s", string(paramsJSON)))

		// Also pass common parameters individually
		if path := getPathFromParams(hookCtx.ToolParams); path != "" {
			cmd.Env = append(cmd.Env, fmt.Sprintf("SYNTOR_FILE_PATH=%s", path))
		}
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run command
	err := cmd.Run()

	output := &ShellOutput{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			output.ExitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("execute command: %w", err)
		}
	}

	return e.parseOutput(output)
}

// parseOutput converts shell output to HookResult.
func (e *ShellExecutor) parseOutput(output *ShellOutput) (*HookResult, error) {
	result := &HookResult{}

	// Try to parse stdout as JSON
	var jsonResult struct {
		Action  string         `json:"action"`
		Reason  string         `json:"reason"`
		Message string         `json:"message"`
		Params  map[string]any `json:"params"`
	}

	stdout := strings.TrimSpace(output.Stdout)
	if json.Unmarshal([]byte(stdout), &jsonResult) == nil {
		// Valid JSON output
		switch strings.ToLower(jsonResult.Action) {
		case "approve", "allow", "yes", "ok":
			result.Action = ActionApprove
		case "block", "deny", "no", "reject":
			result.Action = ActionBlock
		case "modify", "change":
			result.Action = ActionModify
			result.ModifiedParams = jsonResult.Params
		default:
			result.Action = ActionContinue
		}
		result.Reason = jsonResult.Reason
		result.Message = jsonResult.Message
		return result, nil
	}

	// Non-JSON output: use exit code
	switch output.ExitCode {
	case 0:
		// Exit 0 = approve
		result.Action = ActionApprove
	case 1:
		// Exit 1 = block
		result.Action = ActionBlock
		result.Reason = stdout
		if result.Reason == "" && output.Stderr != "" {
			result.Reason = strings.TrimSpace(output.Stderr)
		}
	case 2:
		// Exit 2 = continue (don't make a decision)
		result.Action = ActionContinue
	default:
		// Other exit codes = error
		return nil, fmt.Errorf("hook exited with code %d: %s", output.ExitCode, output.Stderr)
	}

	if stdout != "" {
		result.Message = stdout
	}

	return result, nil
}
