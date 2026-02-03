package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/syntor/syntor/pkg/checkpoint"
	"github.com/syntor/syntor/pkg/config"
	"github.com/syntor/syntor/pkg/coordination"
	"github.com/syntor/syntor/pkg/falkordb"
	"github.com/syntor/syntor/pkg/inference"
	"github.com/syntor/syntor/pkg/manifest"
	"github.com/syntor/syntor/pkg/prompt"
	"github.com/syntor/syntor/pkg/setup"
	"github.com/syntor/syntor/pkg/skills"
	"github.com/syntor/syntor/pkg/stats"
	"github.com/syntor/syntor/pkg/tools"
	"github.com/syntor/syntor/pkg/tools/implementations"
	"github.com/syntor/syntor/pkg/tools/security"
)

// ChatMessage represents a message in the chat history
type ChatMessage struct {
	Role    string // "user", "assistant", "system"
	Content string
	Agent   string
}

// ActivityStatus represents what the system is currently doing
type ActivityStatus struct {
	Active      bool
	Type        string // "thinking", "streaming", "tools", "searching", "planning", "agent", "handoff"
	Description string
	StartTime   time.Time
	Icon        string           // Nerd Font icon for this activity
	Agent       string           // Which agent is performing the activity
	Tool        string           // Which tool is being executed (if applicable)
	Parent      *ActivityStatus  // For nested activities (e.g., tool within agent task)
}

// AutonomyMode defines how the coordination agent handles tasks
type AutonomyMode int

const (
	AutoMode AutonomyMode = iota // Automatically dispatch to agents
	PlanMode                     // Propose plan, wait for approval
)

// DetailLevel defines how much detail to show
type DetailLevel int

const (
	SummaryDetail DetailLevel = iota
	FullDetail
)

// Model is the main Bubbletea model for the TUI
type Model struct {
	// UI components
	input    textinput.Model
	viewport viewport.Model
	styles   Styles

	// State
	messages      []ChatMessage
	streaming     bool
	streamBuffer  *strings.Builder
	currentAgent  inference.AgentType
	activity      ActivityStatus
	chunkCount    int       // Count chunks for batched updates
	lastUIUpdate  time.Time // Track last UI update for throttling

	// Autonomy mode
	autonomyMode    AutonomyMode
	pendingPlan     *coordination.ExecutionPlan
	planDetailLevel DetailLevel

	// Agent orchestration
	activeHandoffs   []coordination.HandoffStatus
	agentTimeline    []coordination.TimelineEvent
	intentParser     *coordination.Parser
	handoffExecutor  *coordination.Executor

	// Manifest and prompt system
	manifestStore *manifest.ManifestStore
	promptBuilder *prompt.Builder

	// Autocomplete
	showSuggestions    bool
	suggestions        []Command
	selectedSuggestion int
	cmdRegistry        *CommandRegistry

	// Code block tracking for copy functionality
	codeBlocks []*CodeBlock

	// Markdown rendering
	mdRenderer *MarkdownRenderer

	// Tool system
	toolRegistry      *tools.Registry
	toolExecutor      *tools.Executor
	toolParser        *tools.Parser
	toolFormatter     *tools.Formatter
	securityMgr       *security.Manager
	pendingApprovals  []*tools.ApprovalRequest
	toolIterations    int
	maxToolIterations int
	conversationHistory []inference.Message
	workingDir        string

	// Infrastructure
	config        *config.SyntorConfig
	registry      *inference.Registry
	cancelFunc    context.CancelFunc
	providerReady bool

	// Service integrations
	services       *IntegratedServices
	projectContext string // Content from SYNTOR.md

	// Skills system
	skillManager *skills.SkillManager

	// Stats tracking
	stats *stats.Stats

	// Session state
	sessionInitialized bool
	sessionStartTime   time.Time

	// Token tracking
	sessionInputTokens  int64
	sessionOutputTokens int64
	globalContext       string // Content from CENTAUR.md

	// Terminal
	width  int
	height int
	ready  bool

	// Quitting
	quitting bool
	err      error
}

// New creates a new TUI model
func New(cfg *config.SyntorConfig) (*Model, error) {
	registry, err := setup.InitializeInference(&cfg.Inference)
	if err != nil {
		return nil, err
	}

	// Initialize manifest store
	manifestStore, err := manifest.NewManifestStore(manifest.GetDefaultPaths())
	if err != nil {
		// Non-fatal, continue without manifests
		manifestStore = nil
	}

	// Initialize prompt builder
	var promptBuilder *prompt.Builder
	if manifestStore != nil {
		gatherer := prompt.NewContextGatherer(manifestStore, "")
		promptBuilder = prompt.NewBuilder(manifestStore, gatherer)
	}

	// Create text input
	ti := textinput.New()
	ti.Placeholder = "Type a message or /command..."
	ti.Prompt = "" // We render our own prompt
	ti.Focus()
	ti.CharLimit = 4096
	ti.Width = 80

	// Create markdown renderer with default width
	mdRenderer, _ := NewMarkdownRenderer(80)

	// Get working directory
	workingDir, err := os.Getwd()
	if err != nil {
		workingDir = "."
	}

	// Initialize tool system
	toolRegistry := tools.NewRegistry()
	if err := implementations.RegisterAll(toolRegistry, workingDir); err != nil {
		// Non-fatal, continue without tools
		toolRegistry = nil
	}

	// Initialize security manager
	securityMgr := security.NewManager(workingDir)

	// Initialize tool executor with security
	var toolExecutor *tools.Executor
	if toolRegistry != nil {
		toolExecutor = tools.NewExecutor(toolRegistry, securityMgr)
	}

	// Initialize service integrations
	ctx := context.Background()
	intConfig := IntegrationConfigFromYAML(&cfg.Integrations)
	services, _ := NewIntegratedServices(ctx, &intConfig)

	// Load project context from SYNTOR.md
	projectContext, _ := config.GetProjectContext()

	// Load global context from CENTAUR.md
	globalContext, _ := config.GetGlobalContext()

	// Initialize skill manager
	skillManager := skills.NewSkillManager()
	skillManager.LoadAll()

	// Initialize stats tracking
	statsTracker, _ := stats.Load()

	// Initialize handoff executor for real agent delegation
	// Uses FalkorDB for dynamic model lookup when available
	var handoffExecutor *coordination.Executor
	if manifestStore != nil && promptBuilder != nil {
		var falkorClient *falkordb.Client
		if services != nil && services.FalkorDB != nil {
			falkorClient = services.FalkorDB
		}
		handoffExecutor = coordination.NewExecutor(registry, manifestStore, promptBuilder, falkorClient)
	}

	// Initial messages will be populated on first render when we know the width
	initialMessages := []ChatMessage{}

	m := &Model{
		input:           ti,
		styles:          DefaultStyles(),
		messages:        initialMessages,
		streamBuffer:    &strings.Builder{},
		currentAgent:    inference.AgentSNTR,
		autonomyMode:    PlanMode, // Default to Plan mode (safer)
		planDetailLevel: SummaryDetail,
		activeHandoffs:  make([]coordination.HandoffStatus, 0),
		agentTimeline:   make([]coordination.TimelineEvent, 0),
		intentParser:    coordination.NewParser(),
		handoffExecutor: handoffExecutor,
		manifestStore:   manifestStore,
		promptBuilder:   promptBuilder,
		cmdRegistry:     NewCommandRegistry(),
		mdRenderer:      mdRenderer,
		toolRegistry:    toolRegistry,
		toolExecutor:    toolExecutor,
		toolParser:      tools.NewParser(),
		toolFormatter:   tools.NewFormatter(),
		securityMgr:     securityMgr,
		pendingApprovals: make([]*tools.ApprovalRequest, 0),
		maxToolIterations: 25,
		conversationHistory: make([]inference.Message, 0),
		workingDir:      workingDir,
		config:          cfg,
		registry:        registry,
		providerReady:   false,
		services:        services,
		projectContext:  projectContext,
		globalContext:   globalContext,
		skillManager:    skillManager,
		stats:           statsTracker,
	}

	return m, nil
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.checkProvider(),
	)
}

// checkProvider checks if the inference provider is available
func (m *Model) checkProvider() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		provider, ok := m.registry.GetDefaultProvider()
		if !ok {
			return ProviderReadyMsg{Available: false, Error: fmt.Errorf("no default provider")}
		}

		available := provider.IsAvailable(ctx)
		return ProviderReadyMsg{Available: available}
	}
}

// ModelWarmupMsg signals model warmup completion
type ModelWarmupMsg struct {
	Success bool
}

// warmupModel sends a minimal request to pre-load the model into memory
func (m *Model) warmupModel() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		provider, modelID, err := setup.GetProviderForAgent(m.registry, m.currentAgent)
		if err != nil {
			return ModelWarmupMsg{Success: false}
		}

		// Send a minimal request to load the model
		req := inference.ChatRequest{
			Model: modelID,
			Messages: []inference.Message{
				{Role: "user", Content: "hi"},
			},
			MaxTokens: 1, // Only need 1 token to warm up
		}

		_, err = provider.Chat(ctx, req)
		return ModelWarmupMsg{Success: err == nil}
	}
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle scroll keys - pass to viewport first
		switch msg.Type {
		case tea.KeyPgUp, tea.KeyPgDown, tea.KeyUp, tea.KeyDown:
			// Only scroll if not in autocomplete mode and input is not focused on these keys
			if !m.showSuggestions && !m.streaming {
				var vpCmd tea.Cmd
				m.viewport, vpCmd = m.viewport.Update(msg)
				if vpCmd != nil {
					cmds = append(cmds, vpCmd)
				}
				return m, tea.Batch(cmds...)
			}
		}
		return m.handleKeyMsg(msg)

	case tea.WindowSizeMsg:
		firstRender := !m.ready
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		// Set viewport size (leave room for input, status, and separators)
		headerHeight := 3
		inputHeight := 3
		statusHeight := 2
		separatorHeight := 2 // Two separator lines around input
		m.viewport = viewport.New(msg.Width, msg.Height-headerHeight-inputHeight-statusHeight-separatorHeight)

		// Add startup banner on first render
		if firstRender && len(m.messages) == 0 {
			m.messages = append(m.messages, ChatMessage{
				Role:    "system",
				Content: GetStartupBanner("v1.0.0", msg.Width) + GetWelcomeMessage(),
			})
		}

		m.viewport.SetContent(m.renderMessages())
		m.input.Width = msg.Width - 4

		// Update markdown renderer width
		if m.mdRenderer != nil {
			m.mdRenderer.UpdateWidth(msg.Width - 10)
		}

	case ProviderReadyMsg:
		m.providerReady = msg.Available
		if msg.Error != nil {
			m.addSystemMessage(fmt.Sprintf("Provider error: %v", msg.Error))
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
		} else if msg.Available {
			m.addSystemMessage("Warming up model...")
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
			// Warm up model in background
			return m, m.warmupModel()
		} else {
			m.addSystemMessage("Provider not available. Start Ollama with: make ollama-up")
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
		}

	case StreamStartMsg:
		m.streaming = true
		m.streamBuffer.Reset()

	case streamChunkWithContinuation:
		m.streaming = true
		m.chunkCount++
		// Update activity to streaming (only on first chunk)
		if m.chunkCount == 1 {
			agentName := getAgentDisplayName(m.currentAgent)
			m.setActivity("streaming", fmt.Sprintf("%s is responding...", agentName))
		}
		m.streamBuffer.WriteString(msg.Content)
		// Update the last message content
		if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant" {
			m.messages[len(m.messages)-1].Content = m.streamBuffer.String()
		}
		// Throttle UI updates: every 5 chunks or 50ms, whichever comes first
		if m.chunkCount%5 == 0 || time.Since(m.lastUIUpdate) > 50*time.Millisecond {
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
			m.lastUIUpdate = time.Now()
		}
		// Chain the next wait command
		return m, waitForChunk(msg.chunkChan)

	case StreamChunkMsg:
		m.streaming = true
		m.chunkCount++
		// Update activity to streaming (only on first chunk)
		if m.chunkCount == 1 {
			agentName := getAgentDisplayName(m.currentAgent)
			m.setActivity("streaming", fmt.Sprintf("%s is responding...", agentName))
		}
		m.streamBuffer.WriteString(msg.Content)
		// Update the last message content
		if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant" {
			m.messages[len(m.messages)-1].Content = m.streamBuffer.String()
		}
		// Always update on final chunk, otherwise throttle
		if msg.Done || m.chunkCount%5 == 0 || time.Since(m.lastUIUpdate) > 50*time.Millisecond {
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
			m.lastUIUpdate = time.Now()
		}
		if msg.Done {
			m.streaming = false
			m.chunkCount = 0
			m.clearActivity()
		}

	case StreamEndMsg:
		m.streaming = false
		m.clearActivity()
		if msg.Interrupted {
			if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant" {
				m.messages[len(m.messages)-1].Content += "\n[interrupted]"
			}
		}
		m.viewport.SetContent(m.renderMessages())

	case StreamErrorMsg:
		m.streaming = false
		m.clearActivity()
		m.addSystemMessage(fmt.Sprintf("Error: %v", msg.Err))
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

	case ErrorMsg:
		m.addSystemMessage(fmt.Sprintf("Error: %v", msg.Err))
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

	case ClearScreenMsg:
		m.messages = make([]ChatMessage, 0)
		m.viewport.SetContent("")

	case TickMsg:
		// Refresh view to update activity duration
		if m.activity.Active {
			return m, DoTick()
		}

	case ModelWarmupMsg:
		if msg.Success {
			m.addSystemMessage("Model loaded and ready.")
		}
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

	case ChatResponseMsg:
		m.clearActivity()
		if msg.Error != nil {
			m.addSystemMessage(fmt.Sprintf("Error: %v", msg.Error))
		} else {
			// Add assistant response
			m.messages = append(m.messages, ChatMessage{
				Role:    "assistant",
				Content: msg.Content,
				Agent:   getAgentDisplayName(m.currentAgent),
			})

			// Add to conversation history
			m.conversationHistory = append(m.conversationHistory, inference.Message{
				Role:    "assistant",
				Content: msg.Content,
			})

			// Parse response for tool calls
			if m.toolParser != nil && m.toolParser.ContainsToolCalls(msg.Content) {
				parseResult, err := m.toolParser.Parse(msg.Content)
				if err == nil && parseResult.HasTools {
					// Check for tool iteration limit
					m.toolIterations++
					if m.toolIterations > m.maxToolIterations {
						return m, func() tea.Msg { return ToolIterationLimitMsg{Iterations: m.toolIterations} }
					}
					// Return tool detected message
					return m, func() tea.Msg {
						return ToolCallDetectedMsg{
							Calls:       parseResult.ToolCalls,
							TextContent: parseResult.TextContent,
						}
					}
				}
			}

			// Parse response for handoff intents if coordination agent
			if m.currentAgent == "coordination" && m.intentParser != nil {
				if result, err := m.intentParser.ParseResponse(msg.Content); err == nil {
					if result.HasPlan && m.autonomyMode == PlanMode {
						// In Plan mode, queue plan for approval
						m.pendingPlan = result.Plan
						return m, func() tea.Msg { return PlanProposedMsg{Plan: result.Plan} }
					} else if result.HasIntent && m.autonomyMode == AutoMode {
						// In Auto mode, execute intent immediately using the real executor
						intent := result.GetFirstIntent()
						if intent != nil && m.handoffExecutor != nil {
							// Set activity to show handoff is in progress
							m.setActivityWithAgent("handoff", fmt.Sprintf("Handing off to %s", intent.Target), intent.Target)
							// Execute the real handoff
							return m, m.executeHandoff(intent)
						}
					}
				}
			}

			// Also check for delegation intents from any agent (SNTR is also an orchestrator)
			if m.intentParser != nil && m.handoffExecutor != nil {
				if result, err := m.intentParser.ParseResponse(msg.Content); err == nil && result.HasIntent {
					intent := result.GetFirstIntent()
					if intent != nil && intent.Target != "" && intent.Target != string(m.currentAgent) {
						m.setActivityWithAgent("handoff", fmt.Sprintf("Delegating to %s", intent.Target), intent.Target)
						return m, m.executeHandoff(intent)
					}
				}
			}
		}
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

	case ModeChangedMsg:
		modeName := "Auto"
		if msg.Mode == PlanMode {
			modeName = "Plan"
		}
		m.addSystemMessage(fmt.Sprintf("Switched to %s mode", modeName))
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

	case PlanProposedMsg:
		m.pendingPlan = msg.Plan
		planDisplay := coordination.FormatPlanForDisplay(msg.Plan, m.planDetailLevel == FullDetail)
		m.addSystemMessage(fmt.Sprintf("📋 Plan proposed:\n%s\nCtrl+Y approve | Ctrl+N reject | Ctrl+D toggle details", planDisplay))
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

	case PlanApprovedMsg:
		m.addSystemMessage("✓ Plan approved - executing...")
		// TODO: Execute the plan
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

	case PlanRejectedMsg:
		m.addSystemMessage("✗ Plan rejected")
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

	case HandoffStartedMsg:
		status := coordination.HandoffStatus{
			FromAgent: msg.FromAgent,
			ToAgent:   msg.ToAgent,
			Task:      msg.Task,
			Status:    coordination.HandoffExecuting,
			StartTime: time.Now(),
		}
		m.activeHandoffs = append(m.activeHandoffs, status)
		m.addSystemMessage(fmt.Sprintf("⟳ %s → %s: %s", msg.FromAgent, msg.ToAgent, msg.Task))
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

	case HandoffCompletedMsg:
		m.clearActivity()
		// Update handoff status
		for i := range m.activeHandoffs {
			if m.activeHandoffs[i].Status == coordination.HandoffExecuting {
				now := time.Now()
				m.activeHandoffs[i].Status = coordination.HandoffCompleted
				m.activeHandoffs[i].EndTime = &now
				m.activeHandoffs[i].Result = msg.Result
				break
			}
		}

		if msg.Result != nil {
			if msg.Result.Status == coordination.ResultSuccess {
				// Add the delegated agent's response as a chat message
				agentName := msg.Result.AgentName
				if agentName == "" {
					agentName = "Agent"
				}
				resultContent := ""
				if resultStr, ok := msg.Result.Result.(string); ok {
					resultContent = resultStr
				} else if msg.Result.Result != nil {
					resultContent = fmt.Sprintf("%v", msg.Result.Result)
				}
				if resultContent != "" {
					m.messages = append(m.messages, ChatMessage{
						Role:    "assistant",
						Content: resultContent,
						Agent:   agentName,
					})
					// Add to conversation history for context
					m.conversationHistory = append(m.conversationHistory, inference.Message{
						Role:    "assistant",
						Content: fmt.Sprintf("[%s]: %s", agentName, resultContent),
					})
				}
				m.addSystemMessage(fmt.Sprintf("✓ %s completed (%.2fs)", agentName, msg.Result.Duration.Seconds()))
			} else {
				// Handoff failed
				errMsg := msg.Result.Error
				if errMsg == "" {
					errMsg = "Unknown error"
				}
				m.addSystemMessage(fmt.Sprintf("✗ Handoff failed: %s", errMsg))
			}
		} else {
			m.addSystemMessage("✗ Handoff completed with no result")
		}
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

	case ClipboardCopyMsg:
		if msg.Success {
			m.addSystemMessage(fmt.Sprintf("✓ Code block %d copied to clipboard", msg.Index))
		} else {
			errMsg := "unknown error"
			if msg.Error != nil {
				errMsg = msg.Error.Error()
			}
			m.addSystemMessage(fmt.Sprintf("✗ Failed to copy: %s", errMsg))
		}
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

	// Tool execution messages
	case ToolCallDetectedMsg:
		if m.toolExecutor == nil {
			m.addSystemMessage("Tool system not available")
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
			return m, nil
		}

		// Check for approvals needed
		needsApproval := m.toolExecutor.CheckApprovals(msg.Calls, m.autonomyMode == PlanMode)
		if len(needsApproval) > 0 && m.autonomyMode == PlanMode {
			m.pendingApprovals = needsApproval
			return m, func() tea.Msg { return ToolApprovalRequestMsg{Requests: needsApproval} }
		}

		// Execute tools directly
		m.setActivity("tools", fmt.Sprintf("Executing %d tool(s)...", len(msg.Calls)))
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, m.executeTools(msg.Calls)

	case ToolApprovalRequestMsg:
		var sb strings.Builder
		sb.WriteString("🔧 Tool approval required:\n\n")
		for i, req := range msg.Requests {
			sb.WriteString(fmt.Sprintf("%d. %s (Risk: %s)\n", i+1, req.ToolCall.Name, req.Risk))
			if len(req.ToolCall.Parameters) > 0 {
				for k, v := range req.ToolCall.Parameters {
					sb.WriteString(fmt.Sprintf("   %s: %v\n", k, v))
				}
			}
			sb.WriteString("\n")
		}
		sb.WriteString("Ctrl+Y approve all | Ctrl+N deny all")
		m.addSystemMessage(sb.String())
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

	case ToolApproveAllMsg:
		if len(m.pendingApprovals) > 0 {
			// Collect all pending tool calls
			calls := make([]tools.ToolCall, 0, len(m.pendingApprovals))
			for _, req := range m.pendingApprovals {
				calls = append(calls, req.ToolCall)
			}
			m.pendingApprovals = nil
			m.addSystemMessage("✓ Tools approved - executing...")
			m.setActivity("tools", fmt.Sprintf("Executing %d tool(s)...", len(calls)))
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
			return m, m.executeTools(calls)
		}

	case ToolDenyAllMsg:
		if len(m.pendingApprovals) > 0 {
			m.pendingApprovals = nil
			m.addSystemMessage("✗ Tools denied")
			m.toolIterations = 0 // Reset iterations
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
		}

	case ToolExecutionCompleteMsg:
		m.clearActivity()

		// Format results for display
		var sb strings.Builder
		sb.WriteString("🔧 Tool results:\n")
		for _, result := range msg.Results {
			status := "✓"
			if !result.Success {
				status = "✗"
			}
			sb.WriteString(fmt.Sprintf("\n%s %s", status, result.ToolName))
			if result.Error != nil {
				sb.WriteString(fmt.Sprintf(" - Error: %s", result.Error.Message))
			}
		}
		m.addSystemMessage(sb.String())

		// Format results for LLM and continue inference
		formattedResults := m.toolFormatter.FormatResults(msg.Results)
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

		// Continue the conversation with tool results
		return m, m.continueWithToolResults(formattedResults)

	case ToolIterationLimitMsg:
		m.clearActivity()
		m.toolIterations = 0 // Reset for next conversation
		m.addSystemMessage(fmt.Sprintf("⚠ Tool iteration limit reached (%d iterations). Stopping to prevent infinite loop.", msg.Iterations))
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
	}

	// Update viewport
	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	// Update text input
	var tiCmd tea.Cmd
	m.input, tiCmd = m.input.Update(msg)
	cmds = append(cmds, tiCmd)

	return m, tea.Batch(cmds...)
}

// handleKeyMsg processes keyboard input
func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle interrupt during streaming
	if m.streaming {
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.cancelFunc != nil {
				m.cancelFunc()
			}
			m.streaming = false
			return m, func() tea.Msg { return StreamEndMsg{Interrupted: true} }
		case tea.KeyEsc:
			if m.cancelFunc != nil {
				m.cancelFunc()
			}
			m.streaming = false
			return m, func() tea.Msg { return StreamEndMsg{Interrupted: true} }
		}
		return m, nil
	}

	// Handle autocomplete navigation
	if m.showSuggestions {
		switch msg.Type {
		case tea.KeyUp:
			if m.selectedSuggestion > 0 {
				m.selectedSuggestion--
			}
			return m, nil
		case tea.KeyDown:
			if m.selectedSuggestion < len(m.suggestions)-1 {
				m.selectedSuggestion++
			}
			return m, nil
		case tea.KeyTab, tea.KeyEnter:
			if len(m.suggestions) > 0 {
				// Complete the command
				selected := m.suggestions[m.selectedSuggestion]
				m.input.SetValue("/" + selected.Name + " ")
				m.input.CursorEnd()
				m.showSuggestions = false
				m.suggestions = nil
				return m, nil
			}
		case tea.KeyEsc:
			m.showSuggestions = false
			m.suggestions = nil
			return m, nil
		}
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		m.quitting = true
		return m, tea.Quit

	case tea.KeyCtrlA:
		// Toggle autonomy mode
		if m.autonomyMode == AutoMode {
			m.autonomyMode = PlanMode
		} else {
			m.autonomyMode = AutoMode
		}
		return m, func() tea.Msg { return ModeChangedMsg{Mode: m.autonomyMode} }

	case tea.KeyCtrlY:
		// Approve pending plan or tools
		if m.pendingPlan != nil {
			plan := m.pendingPlan
			m.pendingPlan = nil
			return m, func() tea.Msg { return PlanApprovedMsg{Plan: plan} }
		}
		if len(m.pendingApprovals) > 0 {
			return m, func() tea.Msg { return ToolApproveAllMsg{} }
		}
		return m, nil

	case tea.KeyCtrlN:
		// Reject pending plan or tools
		if m.pendingPlan != nil {
			m.pendingPlan = nil
			return m, func() tea.Msg { return PlanRejectedMsg{} }
		}
		if len(m.pendingApprovals) > 0 {
			return m, func() tea.Msg { return ToolDenyAllMsg{} }
		}
		return m, nil

	case tea.KeyCtrlD:
		// Toggle detail level
		if m.planDetailLevel == SummaryDetail {
			m.planDetailLevel = FullDetail
		} else {
			m.planDetailLevel = SummaryDetail
		}
		return m, nil

	case tea.KeyEnter:
		return m.handleSubmit()

	default:
		// Update input and check for autocomplete
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)

		// Check for slash command prefix
		value := m.input.Value()
		if strings.HasPrefix(value, "/") && !strings.Contains(value, " ") {
			prefix := strings.TrimPrefix(value, "/")
			m.suggestions = m.cmdRegistry.FilterCommands(prefix)
			m.showSuggestions = len(m.suggestions) > 0
			m.selectedSuggestion = 0
		} else {
			m.showSuggestions = false
			m.suggestions = nil
		}

		return m, cmd
	}
}

// handleSubmit processes the submitted input
func (m Model) handleSubmit() (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(m.input.Value())
	if value == "" {
		return m, nil
	}

	m.input.SetValue("")
	m.showSuggestions = false
	m.suggestions = nil

	// Check for slash command
	if strings.HasPrefix(value, "/") {
		return m.handleSlashCommand(value)
	}

	// Send message to agent
	return m.sendMessage(value)
}

// handleSlashCommand processes slash commands
func (m Model) handleSlashCommand(input string) (tea.Model, tea.Cmd) {
	parts := strings.SplitN(input[1:], " ", 2)
	cmdName := strings.ToLower(parts[0])
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}

	switch cmdName {
	case "quit", "exit":
		m.quitting = true
		return m, tea.Quit

	case "clear":
		m.messages = make([]ChatMessage, 0)
		m.viewport.SetContent("")
		return m, nil

	case "help":
		m.addSystemMessage(m.renderHelp())
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	case "status":
		return m.handleStatus()

	case "models":
		models := m.registry.GetAvailableModels()
		var sb strings.Builder
		sb.WriteString("Available Models:\n")
		for _, model := range models {
			sb.WriteString(fmt.Sprintf("  %s (%s) - %s\n", model.ID, model.Provider, model.Description))
		}
		m.addSystemMessage(sb.String())
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	case "config":
		cfg := fmt.Sprintf("Configuration:\n  Provider: %s\n  Ollama Host: %s\n  Default Model: %s",
			m.config.Inference.Provider, m.config.Inference.OllamaHost, m.config.Inference.DefaultModel)
		m.addSystemMessage(cfg)
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	case "sntr", "coordination":
		m.currentAgent = inference.AgentSNTR
		m.addSystemMessage("Switched to sntr agent")
		if args != "" {
			return m.sendMessage(args)
		}
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	case "docs":
		m.currentAgent = inference.AgentDocumentation
		m.addSystemMessage("Switched to documentation agent")
		if args != "" {
			return m.sendMessage(args)
		}
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	case "git":
		m.currentAgent = inference.AgentGit
		m.addSystemMessage("Switched to git agent")
		if args != "" {
			return m.sendMessage(args)
		}
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	case "worker":
		m.currentAgent = inference.AgentWorker
		m.addSystemMessage("Switched to worker agent")
		if args != "" {
			return m.sendMessage(args)
		}
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	case "code":
		m.currentAgent = inference.AgentWorkerCode
		m.addSystemMessage("Switched to code worker agent")
		if args != "" {
			return m.sendMessage(args)
		}
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	case "copy":
		if args == "" {
			// Copy the most recent code block
			if len(m.codeBlocks) > 0 {
				return m, m.copyCodeBlock(len(m.codeBlocks))
			}
			m.addSystemMessage("No code blocks to copy")
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
			return m, nil
		}
		// Parse the index
		var index int
		if _, err := fmt.Sscanf(args, "%d", &index); err != nil {
			m.addSystemMessage("Usage: /copy [number] - copy code block by number")
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
			return m, nil
		}
		return m, m.copyCodeBlock(index)

	case "init":
		return m.handleInit()

	case "init-project":
		return m.handleInitProject()

	case "init-global":
		return m.handleInitGlobal()

	case "usage":
		return m.handleUsage()

	case "end":
		return m.handleEndSession()

	case "agents":
		return m.handleAgentsStatus()

	case "route":
		return m.handleRoute(args)

	case "plan":
		return m.handlePlanMode()

	case "checkpoint":
		return m.handleCheckpoint()

	case "skills":
		return m.handleSkillsList()

	default:
		// Check if it's a custom command
		if cmd, ok := m.cmdRegistry.GetCommand(cmdName); ok && cmd.Category == "custom" {
			// Custom commands would need the original REPL's prompt template
			m.addSystemMessage(fmt.Sprintf("Custom command /%s not yet supported in TUI mode", cmdName))
		} else {
			m.addSystemMessage(fmt.Sprintf("Unknown command: /%s. Type /help for available commands.", cmdName))
		}
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil
	}
}

// copyCodeBlock copies a code block to the clipboard
func (m *Model) copyCodeBlock(index int) tea.Cmd {
	return func() tea.Msg {
		if index < 1 || index > len(m.codeBlocks) {
			return ClipboardCopyMsg{
				Success: false,
				Index:   index,
				Error:   fmt.Errorf("code block %d not found (have %d blocks)", index, len(m.codeBlocks)),
			}
		}

		block := m.codeBlocks[index-1]
		if err := CopyToClipboard(block.Content); err != nil {
			return ClipboardCopyMsg{
				Success: false,
				Index:   index,
				Error:   err,
			}
		}

		return ClipboardCopyMsg{
			Success: true,
			Index:   index,
		}
	}
}

// sendMessage sends a message to the current agent
func (m Model) sendMessage(message string) (tea.Model, tea.Cmd) {
	// Add user message
	m.messages = append(m.messages, ChatMessage{
		Role:    "user",
		Content: message,
	})

	// Add to conversation history and reset tool iterations for new conversation
	m.conversationHistory = append(m.conversationHistory, inference.Message{
		Role:    "user",
		Content: message,
	})
	m.toolIterations = 0

	// Set activity status
	agentName := getAgentDisplayName(m.currentAgent)
	m.setActivity("thinking", fmt.Sprintf("%s is thinking...", agentName))

	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()

	// Create context with cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	m.cancelFunc = cancel

	// Get provider and model
	provider, modelID, err := setup.GetProviderForAgent(m.registry, m.currentAgent)
	if err != nil {
		m.clearActivity()
		return m, func() tea.Msg { return StreamErrorMsg{Err: err} }
	}

	// Return commands to get full response and tick for activity updates
	return m, tea.Batch(m.fetchResponse(ctx, provider, modelID, message), DoTick())
}

// ChatResponseMsg contains the full chat response
type ChatResponseMsg struct {
	Content string
	Error   error
}

// fetchResponse gets the full response (non-streaming) for faster display
func (m *Model) fetchResponse(ctx context.Context, provider inference.Provider, modelID, message string) tea.Cmd {
	// Build system prompt before closure to capture current state
	systemPrompt := m.buildDynamicPrompt(m.currentAgent)

	// Capture conversation history for the closure
	// This maintains multi-turn conversation context
	conversationHistory := make([]inference.Message, len(m.conversationHistory))
	copy(conversationHistory, m.conversationHistory)

	return func() tea.Msg {
		req := inference.ChatRequest{
			Model:    modelID,
			Messages: conversationHistory,
			System:   systemPrompt,
		}

		resp, err := provider.Chat(ctx, req)
		if err != nil {
			return ChatResponseMsg{Error: err}
		}
		return ChatResponseMsg{Content: resp.Message.Content}
	}
}

// streamChat creates a command that streams the chat response
func (m *Model) streamChat(ctx context.Context, provider inference.Provider, modelID, message string) tea.Cmd {
	// Build system prompt before creating goroutine to capture current state
	systemPrompt := m.buildDynamicPrompt(m.currentAgent)

	// Capture conversation history for the closure
	// This maintains multi-turn conversation context
	conversationHistory := make([]inference.Message, len(m.conversationHistory))
	copy(conversationHistory, m.conversationHistory)

	// Build request with full conversation history
	req := inference.ChatRequest{
		Model:    modelID,
		Messages: conversationHistory,
		System:   systemPrompt,
	}

	// Create a channel for streaming chunks
	chunkChan := make(chan tea.Msg, 100)

	// Start streaming in goroutine
	go func() {
		defer close(chunkChan)

		// Try streaming first
		stream, err := provider.ChatStream(ctx, req)
		if err != nil {
			// Fall back to non-streaming
			resp, err := provider.Chat(ctx, req)
			if err != nil {
				chunkChan <- StreamErrorMsg{Err: err}
				return
			}
			chunkChan <- StreamChunkMsg{Content: resp.Message.Content, Done: true}
			return
		}
		defer stream.Close()

		// Read chunks and send them through channel
		for {
			select {
			case <-ctx.Done():
				chunkChan <- StreamEndMsg{Interrupted: true}
				return
			default:
				chunk, err := stream.Next()
				if err != nil {
					if err == io.EOF || err.Error() == "EOF" {
						chunkChan <- StreamEndMsg{Interrupted: false}
						return
					}
					chunkChan <- StreamErrorMsg{Err: err}
					return
				}
				chunkChan <- StreamChunkMsg{Content: chunk.Content, Done: chunk.Done}
				if chunk.Done {
					return
				}
			}
		}
	}()

	// Return a command that waits for the first chunk
	return waitForChunk(chunkChan)
}

// waitForChunk creates a command that waits for the next chunk from the channel
func waitForChunk(chunkChan <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-chunkChan
		if !ok {
			return StreamEndMsg{Interrupted: false}
		}

		// If it's a chunk message, we need to chain the next wait
		if chunk, isChunk := msg.(StreamChunkMsg); isChunk && !chunk.Done {
			return streamChunkWithContinuation{
				StreamChunkMsg: chunk,
				chunkChan:      chunkChan,
			}
		}

		return msg
	}
}

// streamChunkWithContinuation wraps a chunk with the channel for continuation
type streamChunkWithContinuation struct {
	StreamChunkMsg
	chunkChan <-chan tea.Msg
}

// addSystemMessage adds a system message to the chat
func (m *Model) addSystemMessage(content string) {
	m.messages = append(m.messages, ChatMessage{
		Role:    "system",
		Content: content,
	})
}

// View implements tea.Model
func (m Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	if !m.ready {
		return "Initializing...\n"
	}

	var b strings.Builder

	// Header
	header := m.renderHeader()
	b.WriteString(header)
	b.WriteString("\n")

	// Chat viewport
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Status bar
	status := m.renderStatusBar()
	b.WriteString(status)
	b.WriteString("\n")

	// Activity status (only show when active)
	if m.activity.Active {
		b.WriteString(m.renderActivityStatus())
		b.WriteString("\n")
	}

	// Input separator (top)
	b.WriteString(m.renderInputSeparator())
	b.WriteString("\n")

	// Input line
	inputLine := m.renderInputLine()
	b.WriteString(inputLine)

	// Autocomplete suggestions
	if m.showSuggestions && len(m.suggestions) > 0 {
		b.WriteString("\n")
		b.WriteString(m.renderSuggestions())
	}

	// Input separator (bottom)
	b.WriteString("\n")
	b.WriteString(m.renderInputSeparator())

	// Help bar
	b.WriteString("\n")
	b.WriteString(m.renderHelpBar())

	return b.String()
}

// renderHeader renders the header bar
func (m *Model) renderHeader() string {
	return GetModernHeader(m.width)
}

// renderStatusBar renders the status bar
func (m *Model) renderStatusBar() string {
	// Autonomy mode indicator
	modeIndicator := "[AUTO]"
	if m.autonomyMode == PlanMode {
		modeIndicator = "[PLAN]"
	}
	mode := m.styles.StatusAgent.Render(modeIndicator)

	agent := m.styles.StatusAgent.Render(getAgentDisplayName(m.currentAgent))
	modelID := m.registry.GetModelForAgent(m.currentAgent)
	model := m.styles.StatusModel.Render(modelID)

	status := mode + " " + agent + " | " + model

	// Add integration status indicators
	if m.services != nil {
		integrationStatus := m.services.RenderStatusBar()
		if integrationStatus != "" {
			status += " | " + m.styles.StatusModel.Render(integrationStatus)
		}
	}

	if m.streaming {
		status += " | " + m.styles.StatusStreaming.Render("streaming...")
	}
	if m.pendingPlan != nil {
		status += " | " + m.styles.StatusStreaming.Render("plan pending")
	}

	return m.styles.StatusBar.Width(m.width).Render(status)
}

// renderActivityStatus renders the activity status line with Nerd Font icons and animated spinner
func (m *Model) renderActivityStatus() string {
	// Get animated spinner frame based on elapsed time
	elapsed := time.Since(m.activity.StartTime)
	spinnerFrame := NerdAnimatedSpinner.Frame(elapsed)

	// Get activity-specific icon
	activityIcon := m.activity.Icon
	if activityIcon == "" {
		activityIcon = GetActivityIcon(m.activity.Type)
	}

	// Calculate duration
	durationStr := fmt.Sprintf("%.1fs", elapsed.Seconds())

	// Build the status line with format: [spinner] [icon] Agent: description (duration)
	var statusParts []string

	// Add spinner
	spinnerStyled := m.styles.ActivityIcon.Render(spinnerFrame)
	statusParts = append(statusParts, spinnerStyled)

	// Add activity icon
	iconStyled := m.styles.ActivityIcon.Render(activityIcon)
	statusParts = append(statusParts, iconStyled)

	// Add agent name if present
	if m.activity.Agent != "" {
		agentIcon := GetAgentIcon(m.activity.Agent)
		agentStyled := m.styles.StatusAgent.Render(agentIcon + " " + m.activity.Agent + ":")
		statusParts = append(statusParts, agentStyled)
	}

	// Add tool info if present
	if m.activity.Tool != "" {
		toolIcon := GetToolIcon(m.activity.Tool)
		toolStyled := m.styles.StatusModel.Render("[" + toolIcon + " " + m.activity.Tool + "]")
		statusParts = append(statusParts, toolStyled)
	}

	// Add description
	textStyled := m.styles.ActivityText.Render(m.activity.Description)
	statusParts = append(statusParts, textStyled)

	// Add duration
	durationStyled := m.styles.ActivityDuration.Render("(" + durationStr + ")")
	statusParts = append(statusParts, durationStyled)

	return m.styles.ActivityBar.Render(strings.Join(statusParts, " "))
}

// setActivity sets the current activity status
func (m *Model) setActivity(activityType, description string) {
	m.activity = ActivityStatus{
		Active:      true,
		Type:        activityType,
		Description: description,
		StartTime:   time.Now(),
		Icon:        GetActivityIcon(activityType),
		Agent:       getAgentDisplayName(m.currentAgent),
	}
}

// setActivityWithTool sets activity with tool context
func (m *Model) setActivityWithTool(activityType, description, toolName string) {
	m.activity = ActivityStatus{
		Active:      true,
		Type:        activityType,
		Description: description,
		StartTime:   time.Now(),
		Icon:        GetToolIcon(toolName),
		Agent:       getAgentDisplayName(m.currentAgent),
		Tool:        toolName,
	}
}

// setActivityWithAgent sets activity with explicit agent context
func (m *Model) setActivityWithAgent(activityType, description, agentName string) {
	m.activity = ActivityStatus{
		Active:      true,
		Type:        activityType,
		Description: description,
		StartTime:   time.Now(),
		Icon:        GetActivityIcon(activityType),
		Agent:       agentName,
	}
}

// clearActivity clears the activity status
func (m *Model) clearActivity() {
	m.activity = ActivityStatus{Active: false}
}

// renderInputSeparator renders a horizontal separator line
func (m *Model) renderInputSeparator() string {
	return m.styles.InputSeparator.Render(strings.Repeat("─", m.width))
}

// renderInputLine renders the input prompt and text
func (m *Model) renderInputLine() string {
	prompt := m.styles.InputPrompt.Render("> ")
	return prompt + m.input.View()
}

// renderSuggestions renders the autocomplete suggestions
func (m *Model) renderSuggestions() string {
	var items []string
	for i, cmd := range m.suggestions {
		name := "/" + cmd.Name
		desc := cmd.Description

		var line string
		if i == m.selectedSuggestion {
			line = m.styles.SuggestionSelected.Render(fmt.Sprintf("%-15s", name)) + " " + m.styles.SuggestionDesc.Render(desc)
		} else {
			line = m.styles.SuggestionItem.Render(fmt.Sprintf("%-15s", name)) + " " + m.styles.SuggestionDesc.Render(desc)
		}
		items = append(items, line)

		// Limit visible suggestions
		if i >= 7 {
			items = append(items, m.styles.SuggestionDesc.Render(fmt.Sprintf("  ... and %d more", len(m.suggestions)-8)))
			break
		}
	}

	content := strings.Join(items, "\n")
	return m.styles.SuggestionBox.Render(content)
}

// renderHelpBar renders the help bar at the bottom
func (m *Model) renderHelpBar() string {
	var help []string

	if m.streaming {
		help = append(help, m.styles.HelpKey.Render("Ctrl+C")+" "+m.styles.HelpDesc.Render("interrupt"))
	} else if m.pendingPlan != nil {
		// Plan pending - show approval options
		help = append(help, m.styles.HelpKey.Render("Ctrl+Y")+" "+m.styles.HelpDesc.Render("approve"))
		help = append(help, m.styles.HelpKey.Render("Ctrl+N")+" "+m.styles.HelpDesc.Render("reject"))
		help = append(help, m.styles.HelpKey.Render("Ctrl+D")+" "+m.styles.HelpDesc.Render("details"))
		help = append(help, m.styles.HelpKey.Render("Ctrl+C")+" "+m.styles.HelpDesc.Render("quit"))
	} else if len(m.pendingApprovals) > 0 {
		// Tools pending approval
		help = append(help, m.styles.HelpKey.Render("Ctrl+Y")+" "+m.styles.HelpDesc.Render("approve tools"))
		help = append(help, m.styles.HelpKey.Render("Ctrl+N")+" "+m.styles.HelpDesc.Render("deny tools"))
		help = append(help, m.styles.HelpKey.Render("Ctrl+C")+" "+m.styles.HelpDesc.Render("quit"))
	} else {
		help = append(help, m.styles.HelpKey.Render("Enter")+" "+m.styles.HelpDesc.Render("send"))
		help = append(help, m.styles.HelpKey.Render("Ctrl+A")+" "+m.styles.HelpDesc.Render("mode"))
		help = append(help, m.styles.HelpKey.Render("Tab")+" "+m.styles.HelpDesc.Render("complete"))
		help = append(help, m.styles.HelpKey.Render("/help")+" "+m.styles.HelpDesc.Render("commands"))
		help = append(help, m.styles.HelpKey.Render("Ctrl+C")+" "+m.styles.HelpDesc.Render("quit"))
	}

	return m.styles.HelpBar.Render(strings.Join(help, "  |  "))
}

// renderMessages renders all chat messages
func (m *Model) renderMessages() string {
	var lines []string

	// Calculate available width for content (leave room for prompt)
	contentWidth := m.width - 15
	if contentWidth < 40 {
		contentWidth = 40
	}

	// Reset code blocks for copy tracking
	m.codeBlocks = make([]*CodeBlock, 0)
	codeBlockIndex := 0

	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			prompt := m.styles.UserPrompt.Render("you> ")
			wrapped := wrapText(msg.Content, contentWidth)
			content := m.styles.UserMessage.Render(wrapped)
			lines = append(lines, prompt+content)
			lines = append(lines, "")

		case "assistant":
			agent := msg.Agent
			if agent == "" {
				agent = getAgentDisplayName(m.currentAgent)
			}
			promptStr := m.styles.UserPrompt.Render(agent + "> ")
			promptAdded := false

			// Parse content for code blocks
			parsed := ParseContent(msg.Content)

			for _, segment := range parsed.Segments {
				if segment.IsCode && segment.CodeBlock != nil {
					// Track code block for /copy functionality
					codeBlockIndex++
					segment.CodeBlock.Index = codeBlockIndex
					m.codeBlocks = append(m.codeBlocks, segment.CodeBlock)

					// Render the code block with our custom styling
					codeBlockRendered := RenderCodeBlock(m.styles, segment.CodeBlock, contentWidth)
					lines = append(lines, codeBlockRendered)
				} else {
					// Render text segments with markdown
					text := strings.TrimSpace(segment.Text)
					if text != "" {
						var rendered string
						if m.mdRenderer != nil {
							mdRendered, err := m.mdRenderer.Render(text)
							if err == nil {
								rendered = strings.TrimSpace(mdRendered)
							} else {
								rendered = wrapText(text, contentWidth)
							}
						} else {
							rendered = wrapText(text, contentWidth)
						}

						// Add agent prompt to first text segment
						renderedLines := strings.Split(rendered, "\n")
						for i, line := range renderedLines {
							if !promptAdded && i == 0 {
								lines = append(lines, promptStr+line)
								promptAdded = true
							} else {
								lines = append(lines, line)
							}
						}
					}
				}
			}

			// If no content was rendered, still show the prompt
			if !promptAdded {
				lines = append(lines, promptStr)
			}
			lines = append(lines, "")

		case "system":
			// Check if this is the startup banner (contains ASCII art)
			if strings.Contains(msg.Content, "░██████╗") {
				// Don't wrap or style the banner - it has its own styling
				lines = append(lines, msg.Content)
			} else {
				wrapped := wrapText(msg.Content, contentWidth)
				content := m.styles.SystemMessage.Render(wrapped)
				lines = append(lines, content)
				lines = append(lines, "")
			}
		}
	}

	return strings.Join(lines, "\n")
}

// wrapText wraps text to fit within the specified width
func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	var result strings.Builder
	var lineLen int

	words := strings.Fields(text)
	for i, word := range words {
		wordLen := len(word)

		if lineLen+wordLen+1 > width && lineLen > 0 {
			result.WriteString("\n")
			lineLen = 0
		}

		if lineLen > 0 {
			result.WriteString(" ")
			lineLen++
		}

		result.WriteString(word)
		lineLen += wordLen

		// Preserve newlines in original text
		if i < len(words)-1 && strings.Contains(text, "\n") {
			// Check if there was a newline after this word in original
			idx := strings.Index(text, word)
			if idx >= 0 {
				afterWord := text[idx+len(word):]
				if len(afterWord) > 0 && afterWord[0] == '\n' {
					result.WriteString("\n")
					lineLen = 0
				}
			}
		}
	}

	return result.String()
}

// renderHelp renders the help text
func (m *Model) renderHelp() string {
	var sb strings.Builder
	sb.WriteString("=== SYNTOR Commands ===\n\n")
	sb.WriteString("Agent Commands (from FalkorDB):\n")
	sb.WriteString("  /agents        - List all agents from FalkorDB graph\n")
	sb.WriteString("  /route <type>  - Query routing for a task type\n")
	sb.WriteString("  /sntr          - Switch to sntr orchestrator\n")
	sb.WriteString("  /docs          - Switch to documentation agent\n")
	sb.WriteString("  /git           - Switch to git agent\n")
	sb.WriteString("  /worker        - Switch to worker agent\n")
	sb.WriteString("  /code          - Switch to code worker agent\n\n")
	sb.WriteString("Session Commands:\n")
	sb.WriteString("  /init          - Initialize session, load context\n")
	sb.WriteString("  /init-project  - Create SYNTOR.md for current project\n")
	sb.WriteString("  /init-global   - Create global CENTAUR.md\n")
	sb.WriteString("  /end           - Wrap up session, save state\n")
	sb.WriteString("  /plan          - Enter plan mode for complex tasks\n")
	sb.WriteString("  /checkpoint    - Create manual checkpoint\n")
	sb.WriteString("  /skills        - List available skills\n\n")
	sb.WriteString("System Commands:\n")
	sb.WriteString("  /help          - Show this help\n")
	sb.WriteString("  /status        - Show full system status\n")
	sb.WriteString("  /usage         - Show token usage and context stats\n")
	sb.WriteString("  /models        - List available models\n")
	sb.WriteString("  /config        - Show configuration\n")
	sb.WriteString("  /clear         - Clear the screen\n")
	sb.WriteString("  /copy [n]      - Copy code block to clipboard\n")
	sb.WriteString("  /quit          - Exit SYNTOR\n")
	return sb.String()
}

// getAgentDisplayName returns the display name for an agent type
func getAgentDisplayName(t inference.AgentType) string {
	switch t {
	case inference.AgentSNTR:
		return "sntr"
	case inference.AgentDocumentation:
		return "docs"
	case inference.AgentGit:
		return "git"
	case inference.AgentWorker:
		return "worker"
	case inference.AgentWorkerCode:
		return "code"
	default:
		return "syntor"
	}
}

// handleInit initializes the session, loading context and skills
func (m Model) handleInit() (tea.Model, tea.Cmd) {
	var sb strings.Builder
	sb.WriteString("=== Session Initialized ===\n\n")

	// Mark session as initialized
	m.sessionInitialized = true
	m.sessionStartTime = time.Now()

	// Record session start in stats
	if m.stats != nil {
		m.stats.RecordSession()
		m.stats.Save()
	}

	// Check global context (CENTAUR.md)
	if m.globalContext != "" {
		sb.WriteString("Global context: loaded from CENTAUR.md\n")
	} else {
		sb.WriteString("Global context: not found\n")
		// Offer to create default
		if !config.GlobalContextExists() {
			sb.WriteString("  → Run /init-global to create default CENTAUR.md\n")
		}
	}

	// Load project context
	if m.projectContext != "" {
		sb.WriteString("Project context: loaded from SYNTOR.md\n")
	} else {
		sb.WriteString("Project context: not found\n")
		// Offer to create
		if !config.ProjectMarkdownExists() {
			sb.WriteString("  → Run /init-project to create SYNTOR.md for this directory\n")
		}
	}

	// Show loaded skills
	if m.skillManager != nil {
		skillNames := m.skillManager.Names()
		if len(skillNames) > 0 {
			sb.WriteString(fmt.Sprintf("Skills loaded: %d (%s)\n", len(skillNames), strings.Join(skillNames, ", ")))
			// Show always-active skills
			alwaysActive := m.skillManager.GetAlwaysActive()
			if len(alwaysActive) > 0 {
				var activeNames []string
				for _, s := range alwaysActive {
					activeNames = append(activeNames, s.Name)
				}
				sb.WriteString(fmt.Sprintf("Always active: %s\n", strings.Join(activeNames, ", ")))
			}
		} else {
			sb.WriteString("Skills loaded: 0\n")
		}
	}

	// Show service connectivity
	if m.services != nil {
		sb.WriteString("\nServices:\n")
		if m.services.HeraldAvailable {
			sb.WriteString("  Herald: connected\n")
		} else {
			sb.WriteString("  Herald: not available\n")
		}
		if m.services.FalkorDBAvailable {
			sb.WriteString("  FalkorDB: connected\n")
		} else {
			sb.WriteString("  FalkorDB: not available\n")
		}
		if m.services.MCPToolCount > 0 {
			sb.WriteString(fmt.Sprintf("  MCP Tools: %d available\n", m.services.MCPToolCount))
		}
	}

	// Show working directory
	sb.WriteString(fmt.Sprintf("\nWorking directory: %s\n", m.workingDir))

	// Show token usage summary
	if m.stats != nil {
		today := m.stats.GetTodayStats()
		if today.Input+today.Output > 0 {
			sb.WriteString(fmt.Sprintf("Today's tokens: %d\n", today.Input+today.Output))
		}
	}

	sb.WriteString("\nReady to assist!")

	m.addSystemMessage(sb.String())
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

// handleInitProject creates a SYNTOR.md file for the current project
func (m Model) handleInitProject() (tea.Model, tea.Cmd) {
	if config.ProjectMarkdownExists() {
		m.addSystemMessage("SYNTOR.md already exists in this project.")
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil
	}

	// Get current directory name as project name
	projectName := filepath.Base(m.workingDir)

	// Create the file
	err := config.CreateProjectMarkdown(projectName, "A project managed with SYNTOR.")
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Failed to create SYNTOR.md: %v", err))
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil
	}

	// Reload project context
	projectContext, _ := config.GetProjectContext()
	m.projectContext = projectContext

	m.addSystemMessage(fmt.Sprintf("Created SYNTOR.md in %s\n\nEdit this file to add project-specific context for the AI.", m.workingDir))
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

// handleInitGlobal creates the global CENTAUR.md file
func (m Model) handleInitGlobal() (tea.Model, tea.Cmd) {
	if config.GlobalContextExists() {
		m.addSystemMessage("CENTAUR.md already exists at " + config.GlobalContextPath())
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil
	}

	// Create the file
	err := config.CreateDefaultGlobalContext()
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Failed to create CENTAUR.md: %v", err))
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil
	}

	// Reload global context
	globalContext, _ := config.GetGlobalContext()
	m.globalContext = globalContext

	m.addSystemMessage(fmt.Sprintf("Created CENTAUR.md at %s\n\nThis provides global system context for all sessions.", config.GlobalContextPath()))
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

// handleUsage displays token usage and context statistics
func (m Model) handleUsage() (tea.Model, tea.Cmd) {
	var sb strings.Builder
	sb.WriteString("=== Session Usage ===\n\n")

	// Session token stats
	sb.WriteString(fmt.Sprintf("Session tokens:\n"))
	sb.WriteString(fmt.Sprintf("  Input:  %d\n", m.sessionInputTokens))
	sb.WriteString(fmt.Sprintf("  Output: %d\n", m.sessionOutputTokens))
	sb.WriteString(fmt.Sprintf("  Total:  %d\n", m.sessionInputTokens+m.sessionOutputTokens))

	// Daily stats from stats.json
	if m.stats != nil {
		today := m.stats.GetTodayStats()
		sb.WriteString(fmt.Sprintf("\nToday's usage:\n"))
		sb.WriteString(fmt.Sprintf("  Sessions:   %d\n", today.Sessions))
		sb.WriteString(fmt.Sprintf("  Tokens:     %d\n", today.Input+today.Output))
		sb.WriteString(fmt.Sprintf("  Tool calls: %d\n", today.Tools))
	}

	// Model and context info
	modelID := m.registry.GetModelForAgent(m.currentAgent)
	sb.WriteString(fmt.Sprintf("\nModel: %s\n", modelID))

	// Estimate context window based on model
	contextWindow := getModelContextWindow(modelID)
	if contextWindow > 0 {
		// Rough estimate: 4 chars per token for conversation
		estimatedTokens := len(strings.Join(func() []string {
			var contents []string
			for _, msg := range m.conversationHistory {
				contents = append(contents, msg.Content)
			}
			return contents
		}(), "")) / 4
		usedPercent := float64(estimatedTokens) / float64(contextWindow) * 100
		sb.WriteString(fmt.Sprintf("Context: ~%.0f%% used (%d / %d tokens)\n", usedPercent, estimatedTokens, contextWindow))

		if usedPercent > 75 {
			sb.WriteString("\n⚠️  Context usage high - consider:\n")
			sb.WriteString("  - /clear to start fresh\n")
			sb.WriteString("  - /checkpoint to save state\n")
		}
	}

	// Conversation stats
	sb.WriteString(fmt.Sprintf("\nConversation history: %d messages\n", len(m.conversationHistory)))

	m.addSystemMessage(sb.String())
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

// getModelContextWindow returns the context window size for a model
func getModelContextWindow(modelID string) int {
	// Common model context windows
	contextWindows := map[string]int{
		"llama3.2:3b":          8192,
		"llama3.2:8b":          8192,
		"llama3.1:8b":          128000,
		"llama3.1:70b":         128000,
		"mistral:7b":           32768,
		"mixtral:8x7b":         32768,
		"qwen2.5-coder:7b":     32768,
		"qwen2.5-coder:14b":    32768,
		"deepseek-coder-v2:16b": 128000,
		"claude-3-opus":        200000,
		"claude-3-sonnet":      200000,
		"claude-3-haiku":       200000,
	}

	if size, ok := contextWindows[modelID]; ok {
		return size
	}

	// Default for unknown models
	return 8192
}

// handleRoute queries FalkorDB to find the best agent for a task type
func (m Model) handleRoute(taskType string) (tea.Model, tea.Cmd) {
	if taskType == "" {
		m.addSystemMessage("Usage: /route <task_type>\n\nExamples:\n  /route code_development\n  /route security_assessment\n  /route budget\n  /route deep_research")
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Routing: %s ===\n\n", taskType))

	if m.services != nil && m.services.FalkorDBAvailable && m.services.FalkorDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := m.services.FalkorDB.RouteTask(ctx, falkordb.RouteQuery{
			TaskType: taskType,
		})
		if err != nil {
			sb.WriteString(fmt.Sprintf("Routing error: %v\n", err))
		} else {
			sb.WriteString(fmt.Sprintf("Agent: %s\n", result.Agent.Name))
			if result.Agent.Role != "" {
				sb.WriteString(fmt.Sprintf("Role: %s\n", result.Agent.Role))
			}
			if result.Agent.Focus != "" {
				sb.WriteString(fmt.Sprintf("Focus: %s\n", result.Agent.Focus))
			}
			if result.Team != nil {
				sb.WriteString(fmt.Sprintf("Team: %s\n", result.Team.Name))
			}
			if len(result.Route.Chain) > 0 {
				sb.WriteString(fmt.Sprintf("Chain: %s\n", strings.Join(result.Route.Chain, " → ")))
			}
			if result.Agent.DefinitionPath != "" {
				sb.WriteString(fmt.Sprintf("Definition: %s\n", result.Agent.DefinitionPath))
			}
		}
	} else {
		sb.WriteString("FalkorDB not connected.\n\n")
		sb.WriteString("Using fallback routing:\n")
		// Use static fallback
		if agent, ok := falkordb.FallbackRoutes[taskType]; ok {
			sb.WriteString(fmt.Sprintf("  %s → %s\n", taskType, agent))
		} else {
			sb.WriteString(fmt.Sprintf("  No route found for: %s\n", taskType))
		}
	}

	m.addSystemMessage(sb.String())
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

// handleStatus shows comprehensive system status
func (m Model) handleStatus() (tea.Model, tea.Cmd) {
	var sb strings.Builder
	sb.WriteString("=== Centaur Status ===\n\n")

	// Agent and model
	modelID := m.registry.GetModelForAgent(m.currentAgent)
	modeName := "Auto"
	if m.autonomyMode == PlanMode {
		modeName = "Plan"
	}
	sb.WriteString(fmt.Sprintf("Agent: %s (%s)\n", getAgentDisplayName(m.currentAgent), modelID))
	sb.WriteString(fmt.Sprintf("Mode: %s\n", modeName))
	sb.WriteString(fmt.Sprintf("Provider: %s\n", m.config.Inference.Provider))

	// Session info
	if m.sessionInitialized {
		duration := time.Since(m.sessionStartTime)
		sb.WriteString(fmt.Sprintf("Session: active (%s)\n", duration.Round(time.Second)))
	} else {
		sb.WriteString("Session: not initialized (run /init)\n")
	}

	// Context hierarchy
	sb.WriteString("\nContext:\n")
	if m.globalContext != "" {
		sb.WriteString(fmt.Sprintf("  Global: %s ✓\n", config.GlobalContextPath()))
	} else {
		sb.WriteString("  Global: ~/.syntor/CENTAUR.md ✗\n")
	}
	if m.projectContext != "" {
		sb.WriteString("  Project: ./SYNTOR.md ✓\n")
	} else {
		sb.WriteString("  Project: ./SYNTOR.md ✗\n")
	}

	// Skills
	if m.skillManager != nil {
		skillCount := m.skillManager.Count()
		if skillCount > 0 {
			alwaysActive := m.skillManager.GetAlwaysActive()
			if len(alwaysActive) > 0 {
				var activeNames []string
				for _, s := range alwaysActive {
					activeNames = append(activeNames, s.Name)
				}
				sb.WriteString(fmt.Sprintf("  Skills: %d loaded (%s)\n", skillCount, strings.Join(activeNames, ", ")))
			} else {
				sb.WriteString(fmt.Sprintf("  Skills: %d loaded (none always-active)\n", skillCount))
			}
		} else {
			sb.WriteString("  Skills: 0 loaded\n")
		}
	}

	// Services
	sb.WriteString("\nServices:\n")
	if m.services != nil {
		if m.services.HeraldAvailable {
			sb.WriteString("  Herald: connected ✓\n")
		} else {
			sb.WriteString("  Herald: disconnected ✗\n")
		}
		if m.services.FalkorDBAvailable {
			sb.WriteString("  FalkorDB: connected ✓\n")
		} else {
			sb.WriteString("  FalkorDB: disconnected ✗\n")
		}
		if m.services.MCPToolCount > 0 {
			sb.WriteString(fmt.Sprintf("  MCP Tools: %d available\n", m.services.MCPToolCount))
		}
	} else {
		sb.WriteString("  No service integrations configured\n")
	}

	// Active handoffs
	if len(m.activeHandoffs) > 0 {
		sb.WriteString("\nActive Handoffs:\n")
		for _, h := range m.activeHandoffs {
			if h.Status == coordination.HandoffExecuting {
				duration := time.Since(h.StartTime)
				sb.WriteString(fmt.Sprintf("  %s → %s (%s)\n", h.FromAgent, h.ToAgent, duration.Round(time.Second)))
			}
		}
	} else {
		sb.WriteString("\nActive Handoffs: none\n")
	}

	// Tool system
	if m.toolRegistry != nil {
		sb.WriteString(fmt.Sprintf("\nTools: available (iterations: %d/%d)\n", m.toolIterations, m.maxToolIterations))
	}

	// Working directory
	sb.WriteString(fmt.Sprintf("\nWorking directory: %s\n", m.workingDir))

	m.addSystemMessage(sb.String())
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

// handleEndSession wraps up the session and saves state
func (m Model) handleEndSession() (tea.Model, tea.Cmd) {
	var sb strings.Builder
	sb.WriteString("=== Session Summary ===\n\n")

	// Calculate session duration
	if !m.sessionStartTime.IsZero() {
		duration := time.Since(m.sessionStartTime)
		sb.WriteString(fmt.Sprintf("Duration: %s\n", duration.Round(time.Second)))
	}

	// Show message count
	userMsgs := 0
	assistantMsgs := 0
	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			userMsgs++
		case "assistant":
			assistantMsgs++
		}
	}
	sb.WriteString(fmt.Sprintf("Messages: %d user, %d assistant\n", userMsgs, assistantMsgs))

	// Show tool usage
	if m.stats != nil {
		today := m.stats.GetTodayStats()
		sb.WriteString(fmt.Sprintf("Tool calls today: %d\n", today.Tools))
	}

	// Save stats
	if m.stats != nil {
		if err := m.stats.Save(); err != nil {
			sb.WriteString(fmt.Sprintf("Warning: failed to save stats: %v\n", err))
		} else {
			sb.WriteString("Stats saved.\n")
		}
	}

	// Create automatic checkpoint
	home, _ := os.UserHomeDir()
	checkpointDir := filepath.Join(home, ".syntor", "checkpoints")
	storage, err := checkpoint.NewFileStorage(checkpointDir)
	if err == nil {
		cp := &checkpoint.Checkpoint{
			ID:        fmt.Sprintf("session-%d", time.Now().Unix()),
			SessionID: "tui-session",
			CreatedAt: time.Now(),
			Type:      checkpoint.TypeManual,
			Metadata: map[string]string{
				"type":           "session_end",
				"working_dir":    m.workingDir,
				"message_count":  fmt.Sprintf("%d", len(m.messages)),
				"current_agent":  string(m.currentAgent),
			},
			Restorable: true,
		}

		ctx := context.Background()
		if err := storage.Save(ctx, cp); err != nil {
			sb.WriteString(fmt.Sprintf("Warning: failed to create checkpoint: %v\n", err))
		} else {
			sb.WriteString(fmt.Sprintf("Checkpoint saved: %s\n", cp.ID))
		}
	} else {
		sb.WriteString(fmt.Sprintf("Warning: failed to initialize checkpoint storage: %v\n", err))
	}

	sb.WriteString("\nSession ended. Use /quit to exit or continue working.")

	m.addSystemMessage(sb.String())
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

// handleAgentsStatus shows the agent status dashboard
func (m Model) handleAgentsStatus() (tea.Model, tea.Cmd) {
	var sb strings.Builder
	sb.WriteString("=== Agent Status Dashboard ===\n\n")

	// Current agent
	sb.WriteString(fmt.Sprintf("Current Agent: %s\n", getAgentDisplayName(m.currentAgent)))
	sb.WriteString(fmt.Sprintf("Model: %s\n", m.registry.GetModelForAgent(m.currentAgent)))
	sb.WriteString(fmt.Sprintf("Autonomy Mode: %s\n\n", map[AutonomyMode]string{AutoMode: "Auto", PlanMode: "Plan"}[m.autonomyMode]))

	// Active handoffs
	if len(m.activeHandoffs) > 0 {
		sb.WriteString("Active Handoffs:\n")
		for _, h := range m.activeHandoffs {
			status := "executing"
			if h.Status == coordination.HandoffCompleted {
				status = "completed"
			}
			sb.WriteString(fmt.Sprintf("  %s -> %s: %s (%s)\n", h.FromAgent, h.ToAgent, h.Task, status))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("Active Handoffs: none\n\n")
	}

	// Available agents from FalkorDB
	sb.WriteString("Available Agents:\n")
	if m.services != nil && m.services.FalkorDBAvailable && m.services.FalkorDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		agents, err := m.services.FalkorDB.ListAgents(ctx, "")
		if err == nil && len(agents) > 0 {
			for _, agent := range agents {
				desc := agent.Description
				if desc == "" {
					desc = agent.Role
				}
				if desc == "" {
					desc = string(agent.Type)
				}
				sb.WriteString(fmt.Sprintf("  %-12s - %s\n", agent.Name, desc))
			}
		} else {
			sb.WriteString("  (FalkorDB query failed, showing local agents)\n")
			sb.WriteString("  sntr         - Primary orchestrator with tools\n")
		}
	} else {
		sb.WriteString("  (FalkorDB not connected - run /agents-sync to update)\n")
		// Show minimal fallback when FalkorDB is unavailable
		sb.WriteString("  sntr         - Primary orchestrator\n")
	}

	// Graph stats if available
	if m.services != nil && m.services.FalkorDBAvailable && m.services.FalkorDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		stats, err := m.services.FalkorDB.GetStats(ctx)
		if err == nil {
			sb.WriteString(fmt.Sprintf("\nGraph Stats:\n"))
			sb.WriteString(fmt.Sprintf("  Agents: %d\n", stats.AgentCount))
			sb.WriteString(fmt.Sprintf("  Teams: %d\n", stats.TeamCount))
			sb.WriteString(fmt.Sprintf("  Relationships: %d\n", stats.RelationshipCount))
		}
	}

	// Integration status
	sb.WriteString("\nIntegrations:\n")
	if m.services != nil {
		if m.services.HeraldAvailable {
			sb.WriteString("  Herald: connected ✓\n")
		} else {
			sb.WriteString("  Herald: disconnected ✗\n")
		}
		if m.services.FalkorDBAvailable {
			sb.WriteString("  FalkorDB: connected ✓\n")
		} else {
			sb.WriteString("  FalkorDB: disconnected ✗\n")
		}
	}

	m.addSystemMessage(sb.String())
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

// handlePlanMode enters or shows plan mode status
func (m Model) handlePlanMode() (tea.Model, tea.Cmd) {
	var sb strings.Builder

	if m.autonomyMode == PlanMode {
		sb.WriteString("Already in Plan mode.\n\n")
		sb.WriteString("Plan mode behavior:\n")
		sb.WriteString("- Agent proposes plans before execution\n")
		sb.WriteString("- Tools require approval before running\n")
		sb.WriteString("- Use Ctrl+Y to approve, Ctrl+N to reject\n")
		sb.WriteString("\nUse Ctrl+A to switch to Auto mode.")
	} else {
		m.autonomyMode = PlanMode
		sb.WriteString("Switched to Plan mode.\n\n")
		sb.WriteString("In Plan mode:\n")
		sb.WriteString("- Agent will propose plans before execution\n")
		sb.WriteString("- Tools will require approval\n")
		sb.WriteString("- Use Ctrl+Y to approve, Ctrl+N to reject")
	}

	m.addSystemMessage(sb.String())
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

// handleCheckpoint creates a manual checkpoint
func (m Model) handleCheckpoint() (tea.Model, tea.Cmd) {
	home, _ := os.UserHomeDir()
	checkpointDir := filepath.Join(home, ".syntor", "checkpoints")
	storage, err := checkpoint.NewFileStorage(checkpointDir)
	if err != nil {
		m.addSystemMessage(fmt.Sprintf("Failed to initialize checkpoint storage: %v", err))
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil
	}

	cp := &checkpoint.Checkpoint{
		ID:        fmt.Sprintf("manual-%d", time.Now().Unix()),
		SessionID: "tui-session",
		CreatedAt: time.Now(),
		Type:      checkpoint.TypeManual,
		Metadata: map[string]string{
			"type":           "manual",
			"working_dir":    m.workingDir,
			"message_count":  fmt.Sprintf("%d", len(m.messages)),
			"current_agent":  string(m.currentAgent),
		},
		Restorable: true,
	}

	ctx := context.Background()
	if err := storage.Save(ctx, cp); err != nil {
		m.addSystemMessage(fmt.Sprintf("Failed to create checkpoint: %v", err))
	} else {
		m.addSystemMessage(fmt.Sprintf("Checkpoint created: %s\nLocation: %s", cp.ID, checkpointDir))
	}

	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

// handleSkillsList lists available skills
func (m Model) handleSkillsList() (tea.Model, tea.Cmd) {
	var sb strings.Builder
	sb.WriteString("=== Available Skills ===\n\n")

	if m.skillManager == nil || m.skillManager.Count() == 0 {
		sb.WriteString("No skills loaded.\n")
		sb.WriteString("\nTo add skills, create SKILL.md files in:\n")
		home, _ := os.UserHomeDir()
		sb.WriteString(fmt.Sprintf("  %s\n", filepath.Join(home, ".syntor", "skills", "<skill-name>", "SKILL.md")))
	} else {
		allSkills := m.skillManager.GetAll()
		for _, skill := range allSkills {
			status := ""
			if skill.AlwaysActive {
				status = " [always active]"
			}
			sb.WriteString(fmt.Sprintf("  %s%s\n", skill.Name, status))
			if skill.Description != "" {
				sb.WriteString(fmt.Sprintf("    %s\n", skill.Description))
			}
			if len(skill.Triggers) > 0 {
				sb.WriteString(fmt.Sprintf("    Triggers: %s\n", strings.Join(skill.Triggers, ", ")))
			}
			sb.WriteString("\n")
		}
	}

	m.addSystemMessage(sb.String())
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

// buildDynamicPrompt builds a system prompt using the manifest-based prompt builder
// Falls back to static prompts if the builder isn't available
// Context Hierarchy: Global (CENTAUR.md) → Project (SYNTOR.md) → Skills
func (m *Model) buildDynamicPrompt(agentType inference.AgentType) string {
	var basePrompt string

	// Try to use the dynamic prompt builder
	if m.promptBuilder != nil && m.manifestStore != nil {
		agentName := agentTypeToManifestName(agentType)
		if _, ok := m.manifestStore.GetManifest(agentName); ok {
			ctx := context.Background()
			systemPrompt, err := m.promptBuilder.Build(ctx, agentName, prompt.BuildOptions{
				IncludeAgents:  true,
				IncludeProject: true,
				PlanMode:       m.autonomyMode == PlanMode,
			})
			if err == nil && systemPrompt != "" {
				basePrompt = systemPrompt
			}
		}
	}

	// Fall back to static prompt if no dynamic prompt
	if basePrompt == "" {
		basePrompt = getSystemPrompt(agentType)
	}

	// Inject global context from CENTAUR.md first (device-level)
	if m.globalContext != "" {
		basePrompt = basePrompt + "\n\n<global-context>\n" + m.globalContext + "\n</global-context>"
	}

	// Append project context from SYNTOR.md (project-level)
	if m.projectContext != "" {
		basePrompt = basePrompt + "\n\n<project-context>\n" + m.projectContext + "\n</project-context>"
	}

	// Inject always-active skills
	if m.skillManager != nil {
		activeSkills := m.skillManager.GetAlwaysActive()
		if len(activeSkills) > 0 {
			skillsContent := skills.InjectAll(activeSkills)
			basePrompt = basePrompt + "\n\n" + skillsContent
		}
	}

	// Add integration context if services are available
	if m.services != nil {
		var integrationInfo []string
		if m.services.HeraldAvailable {
			integrationInfo = append(integrationInfo, "Herald session manager: connected")
		}
		if m.services.FalkorDBAvailable {
			integrationInfo = append(integrationInfo, "FalkorDB agent routing: connected")
		}
		if m.services.MCPToolCount > 0 {
			integrationInfo = append(integrationInfo, fmt.Sprintf("MCP tools available: %d", m.services.MCPToolCount))
		}
		if len(integrationInfo) > 0 {
			basePrompt = basePrompt + "\n\n## Connected Services\n- " + strings.Join(integrationInfo, "\n- ")
		}
	}

	return basePrompt
}

// agentTypeToManifestName converts an AgentType to the manifest name
func agentTypeToManifestName(t inference.AgentType) string {
	switch t {
	case inference.AgentSNTR:
		return "sntr"
	case inference.AgentDocumentation:
		return "documentation"
	case inference.AgentGit:
		return "git"
	case inference.AgentWorker:
		return "worker"
	case inference.AgentWorkerCode:
		return "code"
	default:
		return "worker"
	}
}

// getSystemPrompt returns the system prompt for an agent type
// These are fallback prompts when manifest-based prompts are unavailable
func getSystemPrompt(t inference.AgentType) string {
	switch t {
	case inference.AgentSNTR:
		return `## Identity
You are SNTR (pronounced 'center'), the primary AI orchestration agent for SYNTOR.
You are a capable coding assistant with direct filesystem access and the ability to coordinate multi-agent workflows.

## Your Voice
- **Tone**: Helpful, competent, and direct
- **Style**: Concise but thorough - give enough detail without overwhelming
- **Demeanor**: Professional assistant who takes initiative and follows through

### Phrases to Use
- "Let me check that for you..."
- "I'll execute that now."
- "Here's what I found:"

### Never Say
- "I cannot access your filesystem"
- "As an AI, I don't have access to..."

## Your Responsibilities
- Coordinating multi-agent workflows
- Executing filesystem operations using tools
- Understanding user intent and routing to specialists

## Behavioral Guidelines
- Always use tools when asked about files, directories, or code
- Read files before editing them
- Break complex tasks into smaller, manageable steps
- Provide clear feedback about what actions were taken

## How to Use Tools
You have LOCAL FILESYSTEM ACCESS. Output a JSON code block to call tools:

` + "```json" + `
{
  "tool_calls": [
    {"id": "call_001", "name": "list_directory", "parameters": {"path": "."}}
  ]
}
` + "```" + `

## Your Tools
1. **list_directory** - See what's in a folder
2. **read_file** - Read a file with line numbers
3. **write_file** - Create/overwrite a file
4. **edit_file** - Find and replace in a file
5. **bash** - Run shell commands
6. **glob** - Find files by pattern
7. **grep** - Search in files`
	case inference.AgentDocumentation:
		return `## Identity
You are the Documentation Agent for SYNTOR.

## Your Voice
- **Tone**: Clear, educational, and thorough
- **Style**: Well-structured with examples

## Your Responsibilities
- Help users understand code
- Generate documentation
- Explain concepts clearly`
	case inference.AgentGit:
		return `## Identity
You are the Git Agent for SYNTOR.

## Your Voice
- **Tone**: Precise and careful
- **Style**: Command-focused with explanations

## Your Responsibilities
- Help users with git operations
- Craft meaningful commit messages
- Guide version control best practices`
	case inference.AgentWorker:
		return `## Identity
You are a General Worker Agent for SYNTOR.

## Your Voice
- **Tone**: Helpful and adaptable
- **Style**: Matches the task at hand

## Your Responsibilities
- Handle various tasks as delegated
- Complete work efficiently
- Report results clearly`
	case inference.AgentWorkerCode:
		return `## Identity
You are the Code Worker Agent for SYNTOR.

## Your Voice
- **Tone**: Technical and precise
- **Style**: Clean code with clear explanations

## Your Responsibilities
- Code generation and refactoring
- Programming task completion
- Best practices enforcement`
	default:
		return `## Identity
You are SYNTOR, a helpful AI assistant.

## Your Voice
- **Tone**: Helpful and professional
- **Style**: Clear and concise`
	}
}

// executeTools executes tool calls and returns the results
func (m *Model) executeTools(calls []tools.ToolCall) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		opts := tools.ExecuteOptions{
			WorkingDir: m.workingDir,
			PlanMode:   m.autonomyMode == PlanMode,
		}

		results := m.toolExecutor.ExecuteBatch(ctx, calls, opts)
		return ToolExecutionCompleteMsg{Results: results}
	}
}

// continueWithToolResults continues inference with tool results
func (m *Model) continueWithToolResults(toolResults string) tea.Cmd {
	// Build system prompt before closure to capture current state
	systemPrompt := m.buildDynamicPrompt(m.currentAgent)

	// Add tool results to conversation history
	m.conversationHistory = append(m.conversationHistory, inference.Message{
		Role:    "user",
		Content: toolResults,
	})

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		provider, modelID, err := setup.GetProviderForAgent(m.registry, m.currentAgent)
		if err != nil {
			return ChatResponseMsg{Error: err}
		}

		// Build request with full conversation history
		req := inference.ChatRequest{
			Model:    modelID,
			Messages: m.conversationHistory,
			System:   systemPrompt,
		}

		resp, err := provider.Chat(ctx, req)
		if err != nil {
			return ChatResponseMsg{Error: err}
		}
		return ChatResponseMsg{Content: resp.Message.Content}
	}
}

// executeHandoff performs a real handoff to another agent
func (m *Model) executeHandoff(intent *coordination.HandoffIntent) tea.Cmd {
	return func() tea.Msg {
		// Send handoff started message
		startMsg := HandoffStartedMsg{
			FromAgent: string(m.currentAgent),
			ToAgent:   intent.Target,
			Task:      intent.Task,
		}

		// Check if executor is available
		if m.handoffExecutor == nil {
			return HandoffCompletedMsg{
				Result: &coordination.HandoffResult{
					Status: coordination.ResultError,
					Error:  "Handoff executor not initialized",
				},
			}
		}

		// Execute the handoff with a reasonable timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		// Execute the real inference call to the target agent
		result, err := m.handoffExecutor.Execute(ctx, intent)
		if err != nil {
			return HandoffCompletedMsg{
				Result: &coordination.HandoffResult{
					Status: coordination.ResultError,
					Error:  err.Error(),
				},
			}
		}

		// Return handoff started first, then completed
		// Use tea.Batch to send both messages
		return tea.Batch(
			func() tea.Msg { return startMsg },
			func() tea.Msg {
				return HandoffCompletedMsg{Result: result}
			},
		)()
	}
}

// Run starts the TUI
func Run(cfg *config.SyntorConfig) error {
	model, err := New(cfg)
	if err != nil {
		return err
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
