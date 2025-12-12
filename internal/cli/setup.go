package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/syntor/syntor/pkg/config"
	"github.com/syntor/syntor/pkg/inference"
	"github.com/syntor/syntor/pkg/setup"
)

// SetupWizard handles first-time setup
type SetupWizard struct {
	config  *config.SyntorConfig
	scanner *bufio.Scanner
}

// NewSetupWizard creates a new setup wizard
func NewSetupWizard() *SetupWizard {
	return &SetupWizard{
		config:  &config.SyntorConfig{},
		scanner: bufio.NewScanner(os.Stdin),
	}
}

// Run executes the setup wizard
func (w *SetupWizard) Run() error {
	w.printHeader()

	// Check if config already exists
	if w.configExists() {
		if !w.confirmReconfigure() {
			fmt.Println("Setup cancelled. Use 'syntor config edit' to modify settings.")
			return nil
		}
	}

	// Start with defaults
	*w.config = config.DefaultSyntorConfig()

	// Step 1: Check Ollama
	fmt.Println("\n[1/4] Checking Ollama availability...")
	if err := w.checkOllama(); err != nil {
		fmt.Printf("\033[33mWarning: %v\033[0m\n", err)
		fmt.Println("SYNTOR requires Ollama for local AI inference.")
		fmt.Println("Start Ollama with: docker compose up -d ollama")
		fmt.Println()
		if !w.confirm("Continue setup anyway?") {
			return fmt.Errorf("setup cancelled - please start Ollama first")
		}
	} else {
		fmt.Println("\033[32mOllama is running and accessible.\033[0m")
	}

	// Step 2: Configure provider
	fmt.Println("\n[2/4] Configuring inference provider...")
	w.configureProvider()

	// Step 3: Configure models
	fmt.Println("\n[3/4] Configuring AI models...")
	w.configureModels()

	// Step 4: Save configuration
	fmt.Println("\n[4/4] Saving configuration...")
	if err := w.saveConfig(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Pull models if requested
	if w.config.Inference.AutoPull {
		fmt.Println("\n[Optional] Pulling required models...")
		if w.confirm("Pull models now? (recommended for first use)") {
			if err := w.pullModels(); err != nil {
				fmt.Printf("\033[33mWarning: Some models failed to pull: %v\033[0m\n", err)
				fmt.Println("You can pull models later with: syntor models pull <model>")
			}
		}
	}

	w.printComplete()
	return nil
}

func (w *SetupWizard) printHeader() {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║               SYNTOR First-Time Setup Wizard                  ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("This wizard will help you configure SYNTOR for first use.")
	fmt.Println("Press Enter to accept defaults, or type a new value.")
}

func (w *SetupWizard) printComplete() {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    Setup Complete!                            ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Configuration saved to:", config.GlobalConfigPath())
	fmt.Println()
	fmt.Println("Quick start:")
	fmt.Println("  syntor                    - Start interactive mode")
	fmt.Println("  syntor chat \"message\"     - Send a message")
	fmt.Println("  syntor models status      - Check model status")
	fmt.Println("  syntor --help             - Show all commands")
	fmt.Println()
}

func (w *SetupWizard) configExists() bool {
	_, err := os.Stat(config.GlobalConfigPath())
	return err == nil
}

func (w *SetupWizard) confirmReconfigure() bool {
	fmt.Println("Configuration already exists at:", config.GlobalConfigPath())
	return w.confirm("Reconfigure SYNTOR?")
}

func (w *SetupWizard) checkOllama() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	defaultCfg := config.DefaultInferenceConfig()
	registry, err := setup.InitializeInference(&defaultCfg)
	if err != nil {
		return err
	}

	if err := setup.QuickCheck(ctx, registry); err != nil {
		return err
	}

	return nil
}

func (w *SetupWizard) configureProvider() {
	fmt.Println()
	fmt.Println("Available inference providers:")
	fmt.Println("  1. ollama (local, recommended)")
	fmt.Println("  2. anthropic (Claude API, requires API key)")
	fmt.Println("  3. deepseek (DeepSeek API, requires API key)")
	fmt.Println()

	choice := w.prompt("Select provider [1]:", "1")
	switch choice {
	case "2", "anthropic":
		w.config.Inference.Provider = "anthropic"
		apiKey := w.prompt("Enter Anthropic API key:", "")
		w.config.Inference.AnthropicAPIKey = apiKey
	case "3", "deepseek":
		w.config.Inference.Provider = "deepseek"
		apiKey := w.prompt("Enter DeepSeek API key:", "")
		w.config.Inference.DeepSeekAPIKey = apiKey
	default:
		w.config.Inference.Provider = "ollama"
		host := w.prompt("Ollama host [http://localhost:11434]:", "http://localhost:11434")
		w.config.Inference.OllamaHost = host
	}
}

func (w *SetupWizard) configureModels() {
	fmt.Println()
	fmt.Println("Model assignment options:")
	fmt.Println("  1. Use defaults (recommended for most users)")
	fmt.Println("  2. Use a single model for all agents")
	fmt.Println("  3. Configure each agent individually")
	fmt.Println()

	choice := w.prompt("Select option [1]:", "1")

	switch choice {
	case "2":
		w.configureSingleModel()
	case "3":
		w.configureIndividualModels()
	default:
		// Use defaults - already set
		fmt.Println("\nUsing default model assignments:")
		fmt.Printf("  Coordination: %s\n", w.config.Inference.Models.Coordination)
		fmt.Printf("  Documentation: %s\n", w.config.Inference.Models.Documentation)
		fmt.Printf("  Git: %s\n", w.config.Inference.Models.Git)
		fmt.Printf("  Worker: %s\n", w.config.Inference.Models.Worker)
		fmt.Printf("  Worker Code: %s\n", w.config.Inference.Models.WorkerCode)
	}

	autoPull := w.confirm("\nAutomatically pull missing models?")
	w.config.Inference.AutoPull = autoPull
}

func (w *SetupWizard) configureSingleModel() {
	fmt.Println("\nAvailable models for Ollama:")
	models := inference.GetModelsByProvider("ollama")
	for i, m := range models {
		fmt.Printf("  %d. %s - %s\n", i+1, m.ID, m.Description)
	}
	fmt.Println()

	model := w.prompt("Enter model name [llama3.2:8b]:", "llama3.2:8b")
	w.config.Inference.DefaultModel = model
	w.config.Inference.Models.Coordination = model
	w.config.Inference.Models.Documentation = model
	w.config.Inference.Models.Git = model
	w.config.Inference.Models.Worker = model
	w.config.Inference.Models.WorkerCode = model
}

func (w *SetupWizard) configureIndividualModels() {
	fmt.Println("\nConfiguring individual agent models:")
	fmt.Println("(Press Enter to use default)")
	fmt.Println()

	w.config.Inference.Models.Coordination = w.prompt(
		fmt.Sprintf("Coordination agent [%s]:", w.config.Inference.Models.Coordination),
		w.config.Inference.Models.Coordination)

	w.config.Inference.Models.Documentation = w.prompt(
		fmt.Sprintf("Documentation agent [%s]:", w.config.Inference.Models.Documentation),
		w.config.Inference.Models.Documentation)

	w.config.Inference.Models.Git = w.prompt(
		fmt.Sprintf("Git agent [%s]:", w.config.Inference.Models.Git),
		w.config.Inference.Models.Git)

	w.config.Inference.Models.Worker = w.prompt(
		fmt.Sprintf("Worker agent [%s]:", w.config.Inference.Models.Worker),
		w.config.Inference.Models.Worker)

	w.config.Inference.Models.WorkerCode = w.prompt(
		fmt.Sprintf("Code worker agent [%s]:", w.config.Inference.Models.WorkerCode),
		w.config.Inference.Models.WorkerCode)
}

func (w *SetupWizard) saveConfig() error {
	// Ensure config directory exists
	globalDir, _ := config.ConfigPaths()
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		return err
	}

	// Create commands directory
	commandsDir := globalDir + "/commands"
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return err
	}

	return config.SaveSyntorConfig(w.config)
}

func (w *SetupWizard) pullModels() error {
	ctx := context.Background()

	registry, err := setup.InitializeInference(&w.config.Inference)
	if err != nil {
		return err
	}

	// Get unique assigned models
	assignedModels := registry.GetAssignedModels()

	// Check which models need to be pulled
	type modelStatus struct {
		name     string
		exists   bool
		complete bool
	}

	models := make([]modelStatus, 0, len(assignedModels))
	provider, _ := registry.GetDefaultProvider()

	fmt.Println()
	for _, modelID := range assignedModels {
		hasModel, _ := provider.HasModel(ctx, modelID)
		models = append(models, modelStatus{name: modelID, exists: hasModel, complete: hasModel})
		if hasModel {
			fmt.Printf("  \033[32m✓\033[0m %s (already installed)\n", modelID)
		} else {
			fmt.Printf("  \033[33m○\033[0m %s (pending)\n", modelID)
		}
	}

	// Pull missing models
	for i, m := range models {
		if m.exists {
			continue
		}

		// Show in-progress status
		fmt.Printf("  \033[36m⟳\033[0m %s: starting...", m.name)

		err := provider.PullModel(ctx, m.name, func(p inference.PullProgress) {
			if p.Percent > 0 {
				// Create progress bar
				barWidth := 30
				filled := int(p.Percent / 100 * float64(barWidth))
				bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
				fmt.Printf("\r  \033[36m⟳\033[0m %s: [%s] %.1f%%", m.name, bar, p.Percent)
			} else if p.Status != "" {
				fmt.Printf("\r  \033[36m⟳\033[0m %s: %s          ", m.name, p.Status)
			}
		})

		if err != nil {
			fmt.Printf("\r  \033[31m✗\033[0m %s: failed - %v\n", m.name, err)
			continue
		}

		// Mark complete
		models[i].complete = true
		fmt.Printf("\r  \033[32m✓\033[0m %s: complete                                        \n", m.name)
	}

	fmt.Println()
	return nil
}

// prompt asks for user input with a default value
func (w *SetupWizard) prompt(question, defaultVal string) string {
	fmt.Print(question + " ")
	w.scanner.Scan()
	input := strings.TrimSpace(w.scanner.Text())
	if input == "" {
		return defaultVal
	}
	return input
}

// confirm asks a yes/no question
func (w *SetupWizard) confirm(question string) bool {
	fmt.Print(question + " [y/N] ")
	w.scanner.Scan()
	input := strings.ToLower(strings.TrimSpace(w.scanner.Text()))
	return input == "y" || input == "yes"
}

// IsFirstRun checks if this is the first run
func IsFirstRun() bool {
	_, err := os.Stat(config.GlobalConfigPath())
	return os.IsNotExist(err)
}

// RunSetupIfNeeded runs setup wizard if this is first run
func RunSetupIfNeeded() error {
	if !IsFirstRun() {
		return nil
	}

	fmt.Println("Welcome to SYNTOR! Running first-time setup...")
	wizard := NewSetupWizard()
	return wizard.Run()
}
