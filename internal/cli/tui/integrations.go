package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"syntor/pkg/checkpoint"
	"syntor/pkg/falkordb"
	"syntor/pkg/herald"
	"syntor/pkg/hooks"
	"syntor/pkg/mcp"
	"syntor/pkg/subagent"
)

// IntegratedServices holds all the new service integrations.
type IntegratedServices struct {
	// Herald client for service gateway
	Herald *herald.Client

	// FalkorDB client for agent routing
	FalkorDB *falkordb.Client

	// MCP client for external tools
	MCP *mcp.Client
	MCPDiscovery *mcp.Discovery

	// Checkpoint manager for session state
	Checkpoint *checkpoint.Manager

	// Sub-agent manager for isolated execution
	SubAgent *subagent.Manager

	// Hooks manager for tool interception
	Hooks *hooks.Manager

	// Status tracking
	HeraldAvailable    bool
	FalkorDBAvailable  bool
	MCPToolCount       int
	LastCheckpointTime time.Time
	ActiveSubAgents    int
}

// NewIntegratedServices initializes all service integrations.
func NewIntegratedServices(ctx context.Context, config *IntegrationConfig) (*IntegratedServices, error) {
	services := &IntegratedServices{}

	// Initialize Herald client
	if config.Herald.Enabled {
		heraldClient, err := herald.New(config.Herald)
		if err == nil {
			services.Herald = heraldClient
			services.HeraldAvailable = heraldClient.IsAvailable(ctx)
		}
	}

	// Initialize FalkorDB client
	if config.FalkorDB.Enabled {
		falkorClient, err := falkordb.New(config.FalkorDB)
		if err == nil {
			services.FalkorDB = falkorClient
			if err := falkorClient.Connect(ctx); err == nil {
				services.FalkorDBAvailable = true
			}
		}
	}

	// Initialize MCP client
	if len(config.MCP.Servers) > 0 {
		mcpClient := mcp.NewClient(config.MCP)
		if err := mcpClient.Start(ctx); err == nil {
			services.MCP = mcpClient
			services.MCPDiscovery = mcp.NewDiscovery(mcpClient)
			services.MCPDiscovery.Refresh(ctx)
			services.MCPToolCount = len(services.MCPDiscovery.GetAllTools())
		}
	}

	// Initialize Checkpoint manager
	checkpointMgr, err := checkpoint.NewManager(config.Checkpoint, config.CheckpointPolicy)
	if err == nil {
		services.Checkpoint = checkpointMgr
	}

	// Initialize Hooks manager
	hooksMgr, err := hooks.NewManager(config.Hooks)
	if err == nil {
		services.Hooks = hooksMgr
	}

	// Sub-agent manager is initialized when needed (requires executor)
	// services.SubAgent will be set later after tool executor is available

	return services, nil
}

// Close shuts down all services.
func (s *IntegratedServices) Close() error {
	if s.Herald != nil {
		s.Herald.Close()
	}
	if s.FalkorDB != nil {
		s.FalkorDB.Close()
	}
	if s.MCP != nil {
		s.MCP.Close()
	}
	if s.Checkpoint != nil {
		s.Checkpoint.Close()
	}
	if s.SubAgent != nil {
		s.SubAgent.Close()
	}
	return nil
}

// IntegrationConfig holds configuration for all integrations.
type IntegrationConfig struct {
	Herald           herald.Config
	FalkorDB         falkordb.Config
	MCP              mcp.Config
	Checkpoint       checkpoint.StorageConfig
	CheckpointPolicy checkpoint.PolicyConfig
	Hooks            hooks.Config
	SubAgent         subagent.Config
}

// DefaultIntegrationConfig returns sensible defaults.
func DefaultIntegrationConfig() IntegrationConfig {
	return IntegrationConfig{
		Herald:           herald.DefaultConfig(),
		FalkorDB:         falkordb.DefaultConfig(),
		MCP:              mcp.DefaultConfig(),
		Checkpoint:       checkpoint.DefaultStorageConfig(),
		CheckpointPolicy: checkpoint.DefaultPolicyConfig(),
		Hooks:            hooks.DefaultConfig(),
		SubAgent:         subagent.DefaultConfig(),
	}
}

// TUI Messages for integration events

// HeraldStatusMsg indicates Herald connection status.
type HeraldStatusMsg struct {
	Available bool
	Session   *herald.Session
	Error     error
}

// FalkorDBStatusMsg indicates FalkorDB connection status.
type FalkorDBStatusMsg struct {
	Available bool
	Stats     *falkordb.GraphStats
	Error     error
}

// MCPStatusMsg indicates MCP status.
type MCPStatusMsg struct {
	ToolCount int
	Servers   []string
}

// CheckpointCreatedMsg indicates a checkpoint was created.
type CheckpointCreatedMsg struct {
	ID          string
	Description string
}

// SubAgentEventMsg indicates a sub-agent event.
type SubAgentEventMsg struct {
	Event subagent.Event
}

// HookResultMsg indicates a hook result.
type HookResultMsg struct {
	Result *hooks.HookResult
}

// RouteResultMsg contains agent routing results.
type RouteResultMsg struct {
	Route *falkordb.RouteResult
	Error error
}

// Commands for integration actions

// checkHeraldStatus checks Herald availability.
func checkHeraldStatus(client *herald.Client) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return HeraldStatusMsg{Available: false}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		available := client.IsAvailable(ctx)
		return HeraldStatusMsg{Available: available}
	}
}

// routeTask routes a task through FalkorDB.
func routeTask(client *falkordb.Client, taskType string) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return RouteResultMsg{Error: fmt.Errorf("FalkorDB not available")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := client.RouteTask(ctx, falkordb.RouteQuery{TaskType: taskType})
		return RouteResultMsg{Route: result, Error: err}
	}
}

// createCheckpoint creates a new checkpoint.
func createCheckpoint(mgr *checkpoint.Manager, sessionID string, files []string) tea.Cmd {
	return func() tea.Msg {
		if mgr == nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cp, err := mgr.Create(ctx, checkpoint.CreateRequest{
			SessionID:    sessionID,
			Type:         checkpoint.TypeManual,
			Description:  "Manual checkpoint",
			IncludeFiles: files,
		})
		if err != nil {
			return nil
		}
		return CheckpointCreatedMsg{
			ID:          cp.ID,
			Description: cp.Description,
		}
	}
}

// callMCPTool calls an MCP tool.
func callMCPTool(client *mcp.Client, fullName string, args map[string]any) tea.Cmd {
	return func() tea.Msg {
		if client == nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		result, err := client.CallToolByFullName(ctx, fullName, args)
		if err != nil {
			return ErrorMsg{Err: err}
		}

		// Convert to standard tool result format
		var output string
		for _, c := range result.Content {
			if c.Type == mcp.ContentTypeText {
				output += c.Text
			}
		}
		return ToolResultMsg{
			Success: result.Success,
			Output:  output,
		}
	}
}

// executePreToolHook runs pre-tool hooks.
func executePreToolHook(mgr *hooks.Manager, toolName string, params map[string]any) tea.Cmd {
	return func() tea.Msg {
		if mgr == nil {
			return HookResultMsg{Result: &hooks.HookResult{Action: hooks.ActionContinue}}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		result, err := mgr.ExecutePreTool(ctx, toolName, params)
		if err != nil {
			return HookResultMsg{Result: &hooks.HookResult{
				Action: hooks.ActionBlock,
				Reason: err.Error(),
			}}
		}
		return HookResultMsg{Result: result}
	}
}

// ToolResultMsg wraps tool execution results.
type ToolResultMsg struct {
	Success bool
	Output  string
	Error   error
}

// Helper methods for the Model

// GetHeraldStatus returns Herald status for display.
func (s *IntegratedServices) GetHeraldStatus() string {
	if s.Herald == nil {
		return "disabled"
	}
	if s.HeraldAvailable {
		return "connected"
	}
	return "unavailable"
}

// GetFalkorDBStatus returns FalkorDB status for display.
func (s *IntegratedServices) GetFalkorDBStatus() string {
	if s.FalkorDB == nil {
		return "disabled"
	}
	if s.FalkorDBAvailable {
		return "connected"
	}
	return "unavailable"
}

// GetMCPStatus returns MCP status for display.
func (s *IntegratedServices) GetMCPStatus() string {
	if s.MCP == nil {
		return "disabled"
	}
	if s.MCPToolCount > 0 {
		return fmt.Sprintf("%d tools", s.MCPToolCount)
	}
	return "no tools"
}

// GetCheckpointStatus returns checkpoint status for display.
func (s *IntegratedServices) GetCheckpointStatus() string {
	if s.Checkpoint == nil {
		return "disabled"
	}
	if s.LastCheckpointTime.IsZero() {
		return "ready"
	}
	return fmt.Sprintf("last: %s ago", time.Since(s.LastCheckpointTime).Truncate(time.Second))
}

// GetSubAgentStatus returns sub-agent status for display.
func (s *IntegratedServices) GetSubAgentStatus() string {
	if s.SubAgent == nil {
		return "disabled"
	}
	if s.ActiveSubAgents > 0 {
		return fmt.Sprintf("%d active", s.ActiveSubAgents)
	}
	return "idle"
}

// RenderStatusBar returns a status bar with integration states.
func (s *IntegratedServices) RenderStatusBar() string {
	if s == nil {
		return ""
	}

	var parts []string

	// Herald status
	if s.HeraldAvailable {
		parts = append(parts, "H:✓")
	} else if s.Herald != nil {
		parts = append(parts, "H:✗")
	}

	// FalkorDB status
	if s.FalkorDBAvailable {
		parts = append(parts, "F:✓")
	} else if s.FalkorDB != nil {
		parts = append(parts, "F:✗")
	}

	// MCP status
	if s.MCPToolCount > 0 {
		parts = append(parts, fmt.Sprintf("MCP:%d", s.MCPToolCount))
	}

	// Sub-agent status
	if s.ActiveSubAgents > 0 {
		parts = append(parts, fmt.Sprintf("SA:%d", s.ActiveSubAgents))
	}

	if len(parts) == 0 {
		return ""
	}

	return "[" + joinStrings(parts, " | ") + "]"
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
