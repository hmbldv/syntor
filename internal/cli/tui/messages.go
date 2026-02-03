package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/syntor/syntor/pkg/coordination"
	"github.com/syntor/syntor/pkg/tools"
)

// StreamChunkMsg represents a chunk of streamed response
type StreamChunkMsg struct {
	Content string
	Done    bool
}

// StreamErrorMsg represents an error during streaming
type StreamErrorMsg struct {
	Err error
}

// StreamStartMsg signals the start of streaming
type StreamStartMsg struct{}

// StreamEndMsg signals the end of streaming
type StreamEndMsg struct {
	Interrupted bool
}

// ProviderReadyMsg signals that the provider is ready
type ProviderReadyMsg struct {
	Available bool
	Error     error
}

// CommandExecutedMsg signals that a slash command was executed
type CommandExecutedMsg struct {
	Command string
	Output  string
	Error   error
}

// AgentSwitchedMsg signals that the agent was switched
type AgentSwitchedMsg struct {
	Agent string
	Model string
}

// ErrorMsg represents a general error
type ErrorMsg struct {
	Err error
}

// ClearScreenMsg signals to clear the chat history display
type ClearScreenMsg struct{}

// TickMsg for periodic updates (like spinner animation)
type TickMsg struct{}

// WindowSizeMsg for terminal resize events
type WindowSizeMsg struct {
	Width  int
	Height int
}

// ModeChangedMsg signals that the autonomy mode was changed
type ModeChangedMsg struct {
	Mode AutonomyMode
}

// PlanProposedMsg signals that a plan was proposed by the coordination agent
type PlanProposedMsg struct {
	Plan *coordination.ExecutionPlan
}

// PlanApprovedMsg signals that a pending plan was approved
type PlanApprovedMsg struct {
	Plan *coordination.ExecutionPlan
}

// PlanRejectedMsg signals that a pending plan was rejected
type PlanRejectedMsg struct{}

// HandoffStartedMsg signals that an agent handoff was initiated
type HandoffStartedMsg struct {
	FromAgent string
	ToAgent   string
	Task      string
}

// HandoffCompletedMsg signals that an agent handoff completed
type HandoffCompletedMsg struct {
	Result *coordination.HandoffResult
}

// ActivityStartMsg signals the start of a new activity
type ActivityStartMsg struct {
	Type        string // thinking, streaming, tools, searching, planning, agent, handoff
	Description string
	Agent       string // Which agent is performing
	Tool        string // Which tool (if applicable)
	Nested      bool   // Is this a nested activity?
}

// ActivityUpdateMsg updates the current activity description/progress
type ActivityUpdateMsg struct {
	Description string
	Progress    float64 // 0.0 to 1.0 for progress-based activities
}

// ActivityEndMsg signals the end of an activity
type ActivityEndMsg struct {
	Success bool
	Error   error
}

// ToolActivityMsg signals tool-specific activity for enhanced display
type ToolActivityMsg struct {
	ToolName    string
	Operation   string // read, write, edit, execute, search
	Description string
	Icon        string
}

// AgentHandoffActivityMsg signals agent handoff activity for visualization
type AgentHandoffActivityMsg struct {
	FromAgent string
	ToAgent   string
	Task      string
	Status    string // started, in_progress, completed, failed
}

// ClipboardCopyMsg signals that content was copied to clipboard
type ClipboardCopyMsg struct {
	Success bool
	Index   int
	Error   error
}

// Helper function to create a streaming command
func StreamResponse(provider interface{}, request interface{}, cancelFunc func()) tea.Cmd {
	return func() tea.Msg {
		return StreamStartMsg{}
	}
}

// Helper to create tick command for spinner
func DoTick() tea.Cmd {
	return tea.Tick(spinnerInterval, func(_ time.Time) tea.Msg {
		return TickMsg{}
	})
}

const spinnerInterval = 100 * time.Millisecond

// Tool execution messages

// ToolCallDetectedMsg signals that tool calls were found in the response
type ToolCallDetectedMsg struct {
	Calls       []tools.ToolCall
	TextContent string
}

// ToolApprovalRequestMsg signals that tools need user approval
type ToolApprovalRequestMsg struct {
	Requests []*tools.ApprovalRequest
}

// ToolApprovedMsg signals that a tool call was approved
type ToolApprovedMsg struct {
	CallID string
}

// ToolDeniedMsg signals that a tool call was denied
type ToolDeniedMsg struct {
	CallID string
}

// ToolApproveAllMsg signals that all pending tools were approved
type ToolApproveAllMsg struct{}

// ToolDenyAllMsg signals that all pending tools were denied
type ToolDenyAllMsg struct{}

// ToolExecutionStartMsg signals that tool execution has started
type ToolExecutionStartMsg struct {
	Call tools.ToolCall
}

// ToolExecutionCompleteMsg signals that tool execution completed
type ToolExecutionCompleteMsg struct {
	Results []tools.ToolResult
}

// ToolIterationLimitMsg signals that the tool iteration limit was reached
type ToolIterationLimitMsg struct {
	Iterations int
}
