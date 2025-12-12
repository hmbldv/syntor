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

	// Agent switching commands
	r.slashCmds["coordination"] = SlashCommand{
		Name:        "coordination",
		Description: "Switch to coordination agent",
		Handler:     (*REPL).cmdCoordination,
	}
	r.slashCmds["docs"] = SlashCommand{
		Name:        "docs",
		Description: "Switch to documentation agent",
		Handler:     (*REPL).cmdDocs,
	}
	r.slashCmds["git"] = SlashCommand{
		Name:        "git",
		Description: "Switch to git agent",
		Handler:     (*REPL).cmdGit,
	}
	r.slashCmds["worker"] = SlashCommand{
		Name:        "worker",
		Description: "Switch to worker agent",
		Handler:     (*REPL).cmdWorker,
	}
	r.slashCmds["code"] = SlashCommand{
		Name:        "code",
		Description: "Switch to code worker agent",
		Handler:     (*REPL).cmdCode,
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

	return fmt.Errorf("unknown command: /%s. Type /help for available commands", cmdName)
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
		System: getSystemPrompt(r.currentAgent),
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
	fmt.Println("  /coordination  - Switch to coordination agent")
	fmt.Println("  /docs          - Switch to documentation agent")
	fmt.Println("  /git           - Switch to git agent")
	fmt.Println("  /worker        - Switch to general worker agent")
	fmt.Println("  /code          - Switch to code worker agent")
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

func (r *REPL) cmdCoordination(args string) error {
	r.currentAgent = inference.AgentCoordination
	fmt.Println("Switched to coordination agent")
	if args != "" {
		return r.sendMessage(args)
	}
	return nil
}

func (r *REPL) cmdDocs(args string) error {
	r.currentAgent = inference.AgentDocumentation
	fmt.Println("Switched to documentation agent")
	if args != "" {
		return r.sendMessage(args)
	}
	return nil
}

func (r *REPL) cmdGit(args string) error {
	r.currentAgent = inference.AgentGit
	fmt.Println("Switched to git agent")
	if args != "" {
		return r.sendMessage(args)
	}
	return nil
}

func (r *REPL) cmdWorker(args string) error {
	r.currentAgent = inference.AgentWorker
	fmt.Println("Switched to worker agent")
	if args != "" {
		return r.sendMessage(args)
	}
	return nil
}

func (r *REPL) cmdCode(args string) error {
	r.currentAgent = inference.AgentWorkerCode
	fmt.Println("Switched to code worker agent")
	if args != "" {
		return r.sendMessage(args)
	}
	return nil
}

// getAgentName returns a display name for an agent type
func getAgentName(t inference.AgentType) string {
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
