package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/syntor/syntor/pkg/config"
	"github.com/syntor/syntor/pkg/coordination"
	"github.com/syntor/syntor/pkg/inference"
	"github.com/syntor/syntor/pkg/manifest"
	"github.com/syntor/syntor/pkg/prompt"
	"github.com/syntor/syntor/pkg/setup"
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
	Type        string // "thinking", "streaming", "tools", "searching", "planning"
	Description string
	StartTime   time.Time
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
	activeHandoffs []coordination.HandoffStatus
	agentTimeline  []coordination.TimelineEvent
	intentParser   *coordination.Parser

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

	// Infrastructure
	config        *config.SyntorConfig
	registry      *inference.Registry
	cancelFunc    context.CancelFunc
	providerReady bool

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

	// Initial messages will be populated on first render when we know the width
	initialMessages := []ChatMessage{}

	m := &Model{
		input:           ti,
		styles:          DefaultStyles(),
		messages:        initialMessages,
		streamBuffer:    &strings.Builder{},
		currentAgent:    inference.AgentCoordination,
		autonomyMode:    PlanMode, // Default to Plan mode (safer)
		planDetailLevel: SummaryDetail,
		activeHandoffs:  make([]coordination.HandoffStatus, 0),
		agentTimeline:   make([]coordination.TimelineEvent, 0),
		intentParser:    coordination.NewParser(),
		manifestStore:   manifestStore,
		promptBuilder:   promptBuilder,
		cmdRegistry:     NewCommandRegistry(),
		mdRenderer:      mdRenderer,
		config:          cfg,
		registry:        registry,
		providerReady:   false,
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

			// Parse response for handoff intents if coordination agent
			if m.currentAgent == "coordination" && m.intentParser != nil {
				if result, err := m.intentParser.ParseResponse(msg.Content); err == nil {
					if result.HasPlan && m.autonomyMode == PlanMode {
						// In Plan mode, queue plan for approval
						m.pendingPlan = result.Plan
						return m, func() tea.Msg { return PlanProposedMsg{Plan: result.Plan} }
					} else if result.HasIntent && m.autonomyMode == AutoMode {
						// In Auto mode, execute intent immediately
						intent := result.GetFirstIntent()
						if intent != nil {
							m.addSystemMessage(fmt.Sprintf("→ Delegating to %s: %s", intent.Target, intent.Task))
						}
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
		statusStr := "✓"
		if msg.Result != nil && msg.Result.Status != coordination.ResultSuccess {
			statusStr = "✗"
		}
		m.addSystemMessage(fmt.Sprintf("%s Handoff completed", statusStr))
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
		// Approve pending plan
		if m.pendingPlan != nil {
			plan := m.pendingPlan
			m.pendingPlan = nil
			return m, func() tea.Msg { return PlanApprovedMsg{Plan: plan} }
		}
		return m, nil

	case tea.KeyCtrlN:
		// Reject pending plan
		if m.pendingPlan != nil {
			m.pendingPlan = nil
			return m, func() tea.Msg { return PlanRejectedMsg{} }
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
		modelID := m.registry.GetModelForAgent(m.currentAgent)
		status := fmt.Sprintf("Agent: %s | Model: %s | Provider: %s",
			getAgentDisplayName(m.currentAgent), modelID, m.config.Inference.Provider)
		m.addSystemMessage(status)
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

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

	case "coordination":
		m.currentAgent = inference.AgentCoordination
		m.addSystemMessage("Switched to coordination agent")
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

	return func() tea.Msg {
		req := inference.ChatRequest{
			Model: modelID,
			Messages: []inference.Message{
				{Role: "user", Content: message},
			},
			System: systemPrompt,
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

	// Build request
	req := inference.ChatRequest{
		Model: modelID,
		Messages: []inference.Message{
			{Role: "user", Content: message},
		},
		System: systemPrompt,
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
	if m.streaming {
		status += " | " + m.styles.StatusStreaming.Render("streaming...")
	}
	if m.pendingPlan != nil {
		status += " | " + m.styles.StatusStreaming.Render("plan pending")
	}

	return m.styles.StatusBar.Width(m.width).Render(status)
}

// renderActivityStatus renders the activity status line
func (m *Model) renderActivityStatus() string {
	// Activity icons based on type
	icons := map[string]string{
		"thinking":  "🤔",
		"streaming": "📝",
		"tools":     "🔧",
		"searching": "🔍",
		"planning":  "📋",
		"agent":     "🤖",
		"loading":   "⏳",
	}

	icon := icons[m.activity.Type]
	if icon == "" {
		icon = "⏳"
	}

	// Calculate duration
	duration := time.Since(m.activity.StartTime)
	durationStr := fmt.Sprintf("%.1fs", duration.Seconds())

	// Build the status line
	iconStyled := m.styles.ActivityIcon.Render(icon)
	textStyled := m.styles.ActivityText.Render(m.activity.Description)
	durationStyled := m.styles.ActivityDuration.Render("(" + durationStr + ")")

	return m.styles.ActivityBar.Render(iconStyled + " " + textStyled + " " + durationStyled)
}

// setActivity sets the current activity status
func (m *Model) setActivity(activityType, description string) {
	m.activity = ActivityStatus{
		Active:      true,
		Type:        activityType,
		Description: description,
		StartTime:   time.Now(),
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
	sb.WriteString("Agent Commands:\n")
	sb.WriteString("  /coordination  - Switch to coordination agent\n")
	sb.WriteString("  /docs          - Switch to documentation agent\n")
	sb.WriteString("  /git           - Switch to git agent\n")
	sb.WriteString("  /worker        - Switch to general worker agent\n")
	sb.WriteString("  /code          - Switch to code worker agent\n\n")
	sb.WriteString("System Commands:\n")
	sb.WriteString("  /help          - Show this help\n")
	sb.WriteString("  /status        - Show current agent and model\n")
	sb.WriteString("  /models        - List available models\n")
	sb.WriteString("  /config        - Show configuration\n")
	sb.WriteString("  /clear         - Clear the screen\n")
	sb.WriteString("  /quit          - Exit SYNTOR\n")
	return sb.String()
}

// getAgentDisplayName returns the display name for an agent type
func getAgentDisplayName(t inference.AgentType) string {
	switch t {
	case inference.AgentCoordination:
		return "coordination"
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

// buildDynamicPrompt builds a system prompt using the manifest-based prompt builder
// Falls back to static prompts if the builder isn't available
func (m *Model) buildDynamicPrompt(agentType inference.AgentType) string {
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
				return systemPrompt
			}
		}
	}

	// Fall back to static prompt
	return getSystemPrompt(agentType)
}

// agentTypeToManifestName converts an AgentType to the manifest name
func agentTypeToManifestName(t inference.AgentType) string {
	switch t {
	case inference.AgentCoordination:
		return "coordination"
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
func getSystemPrompt(t inference.AgentType) string {
	switch t {
	case inference.AgentCoordination:
		return "You are the coordination agent for SYNTOR. Help users plan and orchestrate tasks across different agents."
	case inference.AgentDocumentation:
		return "You are the documentation agent for SYNTOR. Help users understand code, generate documentation, and explain concepts."
	case inference.AgentGit:
		return "You are the git agent for SYNTOR. Help users with git operations, commit messages, and version control tasks."
	case inference.AgentWorker:
		return "You are a general worker agent for SYNTOR. Help users with various tasks and questions."
	case inference.AgentWorkerCode:
		return "You are the code worker agent for SYNTOR. Help users with code generation, refactoring, and programming tasks."
	default:
		return "You are SYNTOR, a helpful AI assistant."
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
