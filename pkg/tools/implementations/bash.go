package implementations

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/syntor/syntor/pkg/tools"
)

// BashTool executes shell commands
type BashTool struct {
	workingDir     string
	defaultTimeout time.Duration
	maxOutput      int
}

// NewBashTool creates a new bash tool
func NewBashTool(workingDir string) *BashTool {
	return &BashTool{
		workingDir:     workingDir,
		defaultTimeout: 2 * time.Minute,
		maxOutput:      30000,
	}
}

// Name returns the tool's name
func (t *BashTool) Name() string {
	return "bash"
}

// Description returns the tool's description
func (t *BashTool) Description() string {
	return "Executes a bash command in a shell. Use for system operations, git commands, build tools, etc. Commands have a default 2-minute timeout."
}

// Execute runs the command
func (t *BashTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.ToolResult, error) {
	result := &tools.ToolResult{
		ToolName: t.Name(),
	}

	// Get command
	command, ok := params["command"].(string)
	if !ok || command == "" {
		result.Error = &tools.ToolError{
			Code:    tools.ErrCodeInvalidParams,
			Message: "command is required",
		}
		return result, nil
	}

	// Get timeout
	timeout := t.defaultTimeout
	if v, ok := params["timeout"].(float64); ok && v > 0 {
		timeout = time.Duration(v) * time.Millisecond
		if timeout > 10*time.Minute {
			timeout = 10 * time.Minute // Cap at 10 minutes
		}
	}

	// Get working directory
	workDir := t.workingDir
	if v, ok := params["working_dir"].(string); ok && v != "" {
		workDir = v
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(execCtx, "bash", "-c", command)
	if workDir != "" {
		cmd.Dir = workDir
	}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run command
	err := cmd.Run()

	// Combine output
	var output strings.Builder
	if stdout.Len() > 0 {
		output.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString("[stderr]\n")
		output.WriteString(stderr.String())
	}

	outputStr := output.String()

	// Truncate if too long
	if len(outputStr) > t.maxOutput {
		outputStr = outputStr[:t.maxOutput]
		outputStr += fmt.Sprintf("\n[OUTPUT TRUNCATED - exceeded %d characters]", t.maxOutput)
		result.Truncated = true
	}

	// Handle errors
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			result.Error = &tools.ToolError{
				Code:    tools.ErrCodeTimeout,
				Message: fmt.Sprintf("command timed out after %v", timeout),
			}
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			// Command ran but returned non-zero exit code
			result.Success = false
			result.Output = outputStr
			result.Error = &tools.ToolError{
				Code:    tools.ErrCodeExecutionError,
				Message: fmt.Sprintf("command exited with code %d", exitErr.ExitCode()),
			}
		} else {
			result.Error = &tools.ToolError{
				Code:    tools.ErrCodeExecutionError,
				Message: err.Error(),
			}
		}
		return result, nil
	}

	result.Success = true
	result.Output = outputStr
	if result.Output == "" {
		result.Output = "[command completed successfully with no output]"
	}

	return result, nil
}

// Validate checks the parameters
func (t *BashTool) Validate(params map[string]interface{}) error {
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return fmt.Errorf("command is required")
	}

	return nil
}

// SetWorkingDir updates the working directory
func (t *BashTool) SetWorkingDir(dir string) {
	t.workingDir = dir
}

// SetDefaultTimeout updates the default timeout
func (t *BashTool) SetDefaultTimeout(d time.Duration) {
	t.defaultTimeout = d
}

// Definition returns the tool definition
func (t *BashTool) Definition() *tools.ToolDefinition {
	return &tools.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		Category:    tools.ToolCategoryShell,
		Parameters: []tools.ToolParameter{
			{
				Name:        "command",
				Type:        "string",
				Description: "The bash command to execute",
				Required:    true,
			},
			{
				Name:        "timeout",
				Type:        "integer",
				Description: "Timeout in milliseconds (max 600000, default 120000)",
				Required:    false,
				Default:     120000,
			},
			{
				Name:        "working_dir",
				Type:        "string",
				Description: "Working directory for the command",
				Required:    false,
			},
		},
		Returns: tools.ToolReturnSpec{
			Type:        "string",
			Description: "Command output (stdout and stderr)",
		},
		Security: tools.ToolSecuritySpec{
			RequireApproval: true,
			Sandbox:         true,
		},
	}
}
