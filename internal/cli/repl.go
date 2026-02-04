package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/syntor/syntor/pkg/config"
	"github.com/syntor/syntor/pkg/inference"
	"github.com/syntor/syntor/pkg/setup"
)

// REPL represents the interactive read-eval-print loop
type REPL struct {
	config       *config.SyntorConfig
	registry     *inference.Registry
	currentAgent inference.AgentType
	history      []string
	slashCmds    map[string]SlashCommand
}

// SlashCommand represents a custom slash command
type SlashCommand struct {
	Name        string
	Description string
	Handler     func(r *REPL, args string) error
}

// NewREPL creates a new REPL instance
func NewREPL(cfg *config.SyntorConfig) (*REPL, error) {
	registry, err := setup.InitializeInference(&cfg.Inference)
	if err != nil {
		return nil, err
	}

	r := &REPL{
		config:       cfg,
		registry:     registry,
		currentAgent: inference.AgentCoordination,
		history:      make([]string, 0),
		slashCmds:    make(map[string]SlashCommand),
	}

	// Register built-in slash commands
	r.registerBuiltinCommands()

	// Load custom slash commands
	r.loadCustomCommands()

	return r, nil
}

// registerBuiltinCommands registers the built-in slash commands
func (r *REPL) registerBuiltinCommands() {
	r.slashCmds["help"] = SlashCommand{
		Name:        "help",
		Description: "Show available commands",
		Handler:     (*REPL).cmdHelp,
	}
	r.slashCmds["quit"] = SlashCommand{
		Name:        "quit",
		Description: "Exit SYNTOR",
		Handler:     (*REPL).cmdQuit,
	}
	r.slashCmds["exit"] = SlashCommand{
		Name:        "exit",
		Description: "Exit SYNTOR",
		Handler:     (*REPL).cmdQuit,
	}
	r.slashCmds["clear"] = SlashCommand{
		Name:        "clear",
		Description: "Clear the screen",
		Handler:     (*REPL).cmdClear,
	}
	r.slashCmds["models"] = SlashCommand{
		Name:        "models",
		Description: "List available models",
		Handler:     (*REPL).cmdModels,
	}
	r.slashCmds["status"] = SlashCommand{
		Name:        "status",
		Description: "Show current agent and model",
		Handler:     (*REPL).cmdStatus,
	}
	r.slashCmds["config"] = SlashCommand{
		Name:        "config",
		Description: "Show configuration",
		Handler:     (*REPL).cmdConfig,
	}

	// Dynamic agent command
	r.slashCmds["agent"] = SlashCommand{
		Name:        "agent",
		Description: "Switch to any agent: /agent <name> [message]",
		Handler:     (*REPL).cmdAgent,
	}
	r.slashCmds["agents"] = SlashCommand{
		Name:        "agents",
		Description: "List all available agents",
		Handler:     (*REPL).cmdAgentsList,
	}
}

// loadCustomCommands loads custom slash commands from config directories
func (r *REPL) loadCustomCommands() {
	globalDir, projectDir := config.ConfigPaths()

	// Load from global commands directory
	r.loadCommandsFromDir(filepath.Join(globalDir, "commands"))

	// Load from project commands directory (overrides global)
	r.loadCommandsFromDir(filepath.Join(projectDir, "commands"))
}

// loadCommandsFromDir loads slash commands from a directory
func (r *REPL) loadCommandsFromDir(dir string) {
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return
	}

	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".md")
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		prompt := strings.TrimSpace(string(content))
		r.slashCmds[name] = SlashCommand{
			Name:        name,
			Description: fmt.Sprintf("Custom command from %s", file),
			Handler: func(prompt string) func(*REPL, string) error {
				return func(r *REPL, args string) error {
					// Replace {{args}} with actual arguments
					fullPrompt := strings.ReplaceAll(prompt, "{{args}}", args)
					return r.sendMessage(fullPrompt)
				}
			}(prompt),
		}
	}
}

// Run starts the interactive REPL
func (r *REPL) Run() error {
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    SYNTOR Interactive Mode                    ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Type /help for commands, /quit to exit                      ║")
	fmt.Println("║  Current agent: coordination                                  ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)

	for {
		// Print prompt
		agentName := getAgentName(r.currentAgent)
		fmt.Printf("\033[36m%s>\033[0m ", agentName)

		// Read input
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Add to history
		r.history = append(r.history, input)

		// Check for slash command
		if strings.HasPrefix(input, "/") {
			if err := r.handleSlashCommand(input); err != nil {
				if err.Error() == "quit" {
					fmt.Println("Goodbye!")
					return nil
				}
				fmt.Printf("\033[31mError: %v\033[0m\n", err)
			}
			continue
		}

		// Send message to current agent
		if err := r.sendMessage(input); err != nil {
			fmt.Printf("\033[31mError: %v\033[0m\n", err)
		}
	}

	return scanner.Err()
}

// handleSlashCommand processes a slash command
func (r *REPL) handleSlashCommand(input string) error {
	// Parse command and args
	parts := strings.SplitN(input[1:], " ", 2)
	cmdName := strings.ToLower(parts[0])
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}

	// Find and execute command
	if cmd, ok := r.slashCmds[cmdName]; ok {
		return cmd.Handler(r, args)
	}

	// Try dynamic agent switch - treat unknown commands as potential agent names
	r.currentAgent = inference.AgentType(cmdName)
	fmt.Printf("Switched to %s agent\n", cmdName)
	if args != "" {
		return r.sendMessage(args)
	}
	return nil
}

// sendMessage sends a message to the current agent
func (r *REPL) sendMessage(message string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Get provider and model
	provider, modelID, err := setup.GetProviderForAgent(r.registry, r.currentAgent)
	if err != nil {
		return err
	}

	// Check availability
	if !provider.IsAvailable(ctx) {
		return fmt.Errorf("provider %s is not available", provider.Name())
	}

	// Build request
	req := inference.ChatRequest{
		Model: modelID,
		Messages: []inference.Message{
			{Role: "user", Content: message},
		},
		System: getGenericSystemPrompt(string(r.currentAgent)),
	}

	// Stream response
	fmt.Println()
	stream, err := provider.ChatStream(ctx, req)
	if err != nil {
		// Fall back to non-streaming
		resp, err := provider.Chat(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println(resp.Message.Content)
		fmt.Println()
		return nil
	}
	defer stream.Close()

	for {
		chunk, err := stream.Next()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return err
		}

		fmt.Print(chunk.Content)

		if chunk.Done {
			break
		}
	}

	fmt.Println()
	fmt.Println()
	return nil
}

// Built-in command handlers

func (r *REPL) cmdHelp(args string) error {
	fmt.Println("\n=== SYNTOR Commands ===")
	fmt.Println()
	fmt.Println("Agent Commands:")
	fmt.Println("  /agent [name]  - List agents or switch to <name>")
	fmt.Println("  /agents        - List all available agents")
	fmt.Println("  /<name>        - Direct switch (e.g. /coder, /paladin)")
	fmt.Println()
	fmt.Println("System Commands:")
	fmt.Println("  /help          - Show this help")
	fmt.Println("  /status        - Show current agent and model")
	fmt.Println("  /models        - List available models")
	fmt.Println("  /config        - Show configuration")
	fmt.Println("  /clear         - Clear the screen")
	fmt.Println("  /quit          - Exit SYNTOR")
	fmt.Println()

	// Show custom commands if any
	customCount := 0
	for name, cmd := range r.slashCmds {
		if strings.HasPrefix(cmd.Description, "Custom command") {
			if customCount == 0 {
				fmt.Println("Custom Commands:")
			}
			fmt.Printf("  /%s  - %s\n", name, cmd.Description)
			customCount++
		}
	}
	if customCount > 0 {
		fmt.Println()
	}

	return nil
}

func (r *REPL) cmdQuit(args string) error {
	return fmt.Errorf("quit")
}

func (r *REPL) cmdClear(args string) error {
	fmt.Print("\033[H\033[2J")
	return nil
}

func (r *REPL) cmdModels(args string) error {
	models := r.registry.GetAvailableModels()
	fmt.Println("\n=== Available Models ===")
	for _, m := range models {
		status := ""
		if m.Provider == "ollama" {
			status = " (local)"
		} else {
			status = " (api)"
		}
		fmt.Printf("  %s%s - %s\n", m.ID, status, m.Description)
	}
	fmt.Println()
	return nil
}

func (r *REPL) cmdStatus(args string) error {
	modelID := r.registry.GetModelForAgent(r.currentAgent)
	fmt.Printf("\nCurrent Agent: %s\n", getAgentName(r.currentAgent))
	fmt.Printf("Current Model: %s\n", modelID)
	fmt.Printf("Provider: %s\n\n", r.config.Inference.Provider)
	return nil
}

func (r *REPL) cmdConfig(args string) error {
	fmt.Println("\n=== Configuration ===")
	fmt.Printf("Provider: %s\n", r.config.Inference.Provider)
	fmt.Printf("Ollama Host: %s\n", r.config.Inference.OllamaHost)
	fmt.Printf("Default Model: %s\n", r.config.Inference.DefaultModel)
	fmt.Printf("Auto Pull: %v\n", r.config.Inference.AutoPull)
	fmt.Printf("Stream Response: %v\n\n", r.config.CLI.StreamResponse)
	return nil
}

func (r *REPL) cmdAgent(args string) error {
	if args == "" {
		return r.cmdAgentsList("")
	}

	// Parse agent name and optional message
	parts := strings.SplitN(args, " ", 2)
	agentName := strings.ToLower(parts[0])
	message := ""
	if len(parts) > 1 {
		message = parts[1]
	}

	// Switch to the agent (AgentType is just a string alias)
	r.currentAgent = inference.AgentType(agentName)
	fmt.Printf("Switched to %s agent\n", agentName)

	if message != "" {
		return r.sendMessage(message)
	}
	return nil
}

func (r *REPL) cmdAgentsList(args string) error {
	fmt.Println("\n=== Available Agents ===")
	fmt.Println("Usage: /agent <name> [message]")
	fmt.Println("   or: /<name> [message]")
	fmt.Println()

	// Load agents from database
	loader, err := getAgentLoader()
	if err != nil {
		fmt.Println("  (Agent database not available)")
		fmt.Println("\n  Common agents:")
		fmt.Println("    sntr      - Primary orchestrator")
		fmt.Println("    coder     - Development partner")
		fmt.Println("    paladin   - Security specialist")
		fmt.Println()
		return nil
	}
	defer loader.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	agents, err := loader.ListAgents(ctx)
	if err != nil {
		fmt.Printf("Error listing agents: %v\n", err)
		return nil
	}

	for _, agent := range agents {
		model := agent.Model
		if model == "" {
			model = "default"
		}
		fmt.Printf("  %-15s %-35s %s\n", agent.Name, agent.Role, model)
	}
	fmt.Printf("\nTotal: %d agents\n\n", len(agents))
	return nil
}

// getAgentName returns a display name for an agent type
func getAgentName(t inference.AgentType) string {
	name := string(t)
	if name == "" {
		return "sntr"
	}
	return name
}
