package security

import (
	"fmt"
	"sync"

	"github.com/syntor/syntor/pkg/tools"
)

// Manager handles security policy enforcement for tools
type Manager struct {
	pathValidator    *PathValidator
	commandValidator *CommandValidator
	planMode         bool
	mu               sync.RWMutex

	// Tool-specific approval requirements
	approvalRequired map[string]bool
}

// NewManager creates a new security manager
func NewManager(workingDir string) *Manager {
	return &Manager{
		pathValidator:    NewPathValidator(workingDir),
		commandValidator: NewCommandValidator(),
		approvalRequired: map[string]bool{
			"write_file": true,
			"edit_file":  true,
			"bash":       true,
		},
	}
}

// SetPlanMode enables or disables plan mode (affects approval requirements)
func (m *Manager) SetPlanMode(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.planMode = enabled
}

// IsPlanMode returns whether plan mode is enabled
func (m *Manager) IsPlanMode() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.planMode
}

// SetWorkingDir updates the working directory for path validation
func (m *Manager) SetWorkingDir(dir string) {
	m.pathValidator.SetWorkingDir(dir)
}

// Check validates a tool call against security policies
func (m *Manager) Check(call tools.ToolCall, def *tools.ToolDefinition) error {
	switch call.Name {
	case "read_file":
		return m.checkReadFile(call)
	case "write_file":
		return m.checkWriteFile(call)
	case "edit_file":
		return m.checkEditFile(call)
	case "bash":
		return m.checkBash(call)
	case "glob":
		return m.checkGlob(call)
	case "grep":
		return m.checkGrep(call)
	case "list_directory":
		return m.checkListDirectory(call)
	default:
		// Unknown tools require approval by default
		return nil
	}
}

// RequiresApproval determines if a tool call needs user approval
func (m *Manager) RequiresApproval(call tools.ToolCall, def *tools.ToolDefinition, planMode bool) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// In plan mode, destructive operations always require approval
	if planMode {
		if required, ok := m.approvalRequired[call.Name]; ok && required {
			return true
		}
	}

	// Check tool definition for explicit approval requirement
	if def != nil && def.Security.RequireApproval {
		return true
	}

	// Bash commands that aren't read-only require approval
	if call.Name == "bash" {
		if cmd, ok := call.Parameters["command"].(string); ok {
			if !m.commandValidator.IsReadOnlyCommand(cmd) {
				return true
			}
		}
	}

	return false
}

// checkReadFile validates read_file tool calls
func (m *Manager) checkReadFile(call tools.ToolCall) error {
	path, ok := call.Parameters["file_path"].(string)
	if !ok {
		return fmt.Errorf("missing file_path parameter")
	}

	return m.pathValidator.ValidatePath(path, false)
}

// checkWriteFile validates write_file tool calls
func (m *Manager) checkWriteFile(call tools.ToolCall) error {
	path, ok := call.Parameters["file_path"].(string)
	if !ok {
		return fmt.Errorf("missing file_path parameter")
	}

	return m.pathValidator.ValidatePath(path, true)
}

// checkEditFile validates edit_file tool calls
func (m *Manager) checkEditFile(call tools.ToolCall) error {
	path, ok := call.Parameters["file_path"].(string)
	if !ok {
		return fmt.Errorf("missing file_path parameter")
	}

	return m.pathValidator.ValidatePath(path, true)
}

// checkBash validates bash tool calls
func (m *Manager) checkBash(call tools.ToolCall) error {
	command, ok := call.Parameters["command"].(string)
	if !ok {
		return fmt.Errorf("missing command parameter")
	}

	return m.commandValidator.ValidateCommand(command)
}

// checkGlob validates glob tool calls
func (m *Manager) checkGlob(call tools.ToolCall) error {
	path, ok := call.Parameters["path"].(string)
	if ok && path != "" {
		return m.pathValidator.ValidatePath(path, false)
	}
	return nil
}

// checkGrep validates grep tool calls
func (m *Manager) checkGrep(call tools.ToolCall) error {
	path, ok := call.Parameters["path"].(string)
	if ok && path != "" {
		return m.pathValidator.ValidatePath(path, false)
	}
	return nil
}

// checkListDirectory validates list_directory tool calls
func (m *Manager) checkListDirectory(call tools.ToolCall) error {
	path, ok := call.Parameters["path"].(string)
	if !ok {
		return fmt.Errorf("missing path parameter")
	}

	return m.pathValidator.ValidatePath(path, false)
}

// AddApprovalRequired adds a tool to the approval-required list
func (m *Manager) AddApprovalRequired(toolName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.approvalRequired[toolName] = true
}

// RemoveApprovalRequired removes a tool from the approval-required list
func (m *Manager) RemoveApprovalRequired(toolName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.approvalRequired, toolName)
}

// GetPathValidator returns the path validator for external configuration
func (m *Manager) GetPathValidator() *PathValidator {
	return m.pathValidator
}

// GetCommandValidator returns the command validator for external configuration
func (m *Manager) GetCommandValidator() *CommandValidator {
	return m.commandValidator
}

// SecurityReport contains information about a security check
type SecurityReport struct {
	ToolName    string
	Allowed     bool
	Reason      string
	RiskLevel   tools.RiskLevel
	RequiresApproval bool
}

// Analyze provides a security analysis of a tool call without blocking
func (m *Manager) Analyze(call tools.ToolCall, def *tools.ToolDefinition) SecurityReport {
	report := SecurityReport{
		ToolName: call.Name,
		Allowed:  true,
	}

	// Check security
	if err := m.Check(call, def); err != nil {
		report.Allowed = false
		report.Reason = err.Error()
		report.RiskLevel = tools.RiskHigh
		return report
	}

	// Determine risk level
	switch call.Name {
	case "bash":
		cmd, _ := call.Parameters["command"].(string)
		if m.commandValidator.IsReadOnlyCommand(cmd) {
			report.RiskLevel = tools.RiskLow
		} else {
			report.RiskLevel = tools.RiskHigh
		}
	case "write_file", "edit_file":
		report.RiskLevel = tools.RiskMedium
	case "read_file", "glob", "grep", "list_directory":
		report.RiskLevel = tools.RiskLow
	default:
		report.RiskLevel = tools.RiskMedium
	}

	// Check approval requirement
	report.RequiresApproval = m.RequiresApproval(call, def, m.IsPlanMode())

	return report
}
