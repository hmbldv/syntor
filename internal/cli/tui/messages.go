package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/syntor/syntor/pkg/coordination"
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
