package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/syntor/syntor/pkg/config"
	"github.com/syntor/syntor/pkg/inference"
	"github.com/syntor/syntor/pkg/setup"
)

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Manage AI models",
	Long: `Manage AI models used by SYNTOR agents.

Commands:
  list    - List available and installed models
  pull    - Download a model
  status  - Show model status for each agent
  assign  - Assign a model to an agent`,
}

var modelsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available models",
	Long: `List all available models, showing which are installed locally.

The output shows:
  - Model ID and name
  - Provider (ollama, anthropic, deepseek)
  - Parameters/size
  - Installation status`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listModels()
	},
}

var modelsPullCmd = &cobra.Command{
	Use:   "pull <model>",
	Short: "Pull/download a model",
	Long: `Download a model to use with SYNTOR.

Examples:
  syntor models pull mistral:7b
  syntor models pull llama3.2:8b
  syntor models pull qwen2.5-coder:7b`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return pullModel(args[0])
	},
}

var modelsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show model assignments and status",
	Long: `Show which models are assigned to each agent and their availability status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return showModelStatus()
	},
}

var modelsAssignCmd = &cobra.Command{
	Use:   "assign <agent> <model>",
	Short: "Assign a model to an agent",
	Long: `Assign a specific model to an agent type.

Agent types:
  coordination  - Coordination/orchestration agent
  docs          - Documentation agent
  git           - Git operations agent
  worker        - General worker agent
  worker_code   - Code-specific worker agent

Examples:
  syntor models assign coordination mistral:7b
  syntor models assign docs deepseek-coder-v2:16b`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return assignModel(args[0], args[1])
	},
}

func init() {
	modelsCmd.AddCommand(modelsListCmd)
	modelsCmd.AddCommand(modelsPullCmd)
	modelsCmd.AddCommand(modelsStatusCmd)
	modelsCmd.AddCommand(modelsAssignCmd)
}

func listModels() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Initialize registry
	registry, err := setup.InitializeInference(&syntorConfig.Inference)
	if err != nil {
		return fmt.Errorf("failed to initialize inference: %w", err)
	}

	// Get all available models
	allModels := registry.GetAvailableModels()

	// Check which models are installed (for Ollama)
	ollamaProvider, hasOllama := registry.GetProvider("ollama")
	installedModels := make(map[string]bool)
	if hasOllama && ollamaProvider.IsAvailable(ctx) {
		models, err := ollamaProvider.ListModels(ctx)
		if err == nil {
			for _, m := range models {
				installedModels[m.ID] = true
			}
		}
	}

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "MODEL\tPROVIDER\tPARAMETERS\tSTATUS\tDESCRIPTION")
	fmt.Fprintln(w, "-----\t--------\t----------\t------\t-----------")

	for _, m := range allModels {
		status := "available"
		if m.Provider == "ollama" {
			if installedModels[m.ID] {
				status = "installed"
			} else {
				status = "not pulled"
			}
		} else {
			status = "api"
		}

		params := m.Parameters
		if params == "" {
			params = "-"
		}

		desc := m.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			m.ID, m.Provider, params, status, desc)
	}

	w.Flush()
	return nil
}

func pullModel(modelID string) error {
	ctx := context.Background()

	// Initialize registry
	registry, err := setup.InitializeInference(&syntorConfig.Inference)
	if err != nil {
		return fmt.Errorf("failed to initialize inference: %w", err)
	}

	// Find the model
	model, found := registry.FindModel(modelID)
	if !found {
		return fmt.Errorf("model not found: %s", modelID)
	}

	if model.Provider != "ollama" {
		fmt.Printf("Model %s is an API model and doesn't need to be pulled.\n", modelID)
		return nil
	}

	// Get Ollama provider
	provider, ok := registry.GetProvider("ollama")
	if !ok {
		return fmt.Errorf("Ollama provider not available")
	}

	if !provider.IsAvailable(ctx) {
		return fmt.Errorf("Ollama is not running. Start it with: docker compose up -d ollama")
	}

	fmt.Printf("Pulling %s...\n", modelID)

	// Pull with progress
	err = provider.PullModel(ctx, modelID, func(p inference.PullProgress) {
		if p.Status != "" {
			if p.Percent > 0 {
				fmt.Printf("\r%s: %.1f%%", p.Status, p.Percent)
			} else {
				fmt.Printf("\r%s", p.Status)
			}
		}
	})

	fmt.Println() // New line after progress

	if err != nil {
		return fmt.Errorf("failed to pull model: %w", err)
	}

	fmt.Printf("Successfully pulled %s\n", modelID)
	return nil
}

func showModelStatus() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Initialize registry
	registry, err := setup.InitializeInference(&syntorConfig.Inference)
	if err != nil {
		return fmt.Errorf("failed to initialize inference: %w", err)
	}

	// Get all assignments
	assignments := registry.GetAllAssignments()

	// Check Ollama availability
	ollamaProvider, _ := registry.GetProvider("ollama")
	ollamaAvailable := ollamaProvider != nil && ollamaProvider.IsAvailable(ctx)

	// Get installed models
	installedModels := make(map[string]bool)
	if ollamaAvailable {
		models, err := ollamaProvider.ListModels(ctx)
		if err == nil {
			for _, m := range models {
				installedModels[m.ID] = true
			}
		}
	}

	fmt.Println("=== Model Assignments ===")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "AGENT\tMODEL\tPROVIDER\tSTATUS")
	fmt.Fprintln(w, "-----\t-----\t--------\t------")

	agentNames := map[inference.AgentType]string{
		inference.AgentCoordination:  "coordination",
		inference.AgentDocumentation: "docs",
		inference.AgentGit:           "git",
		inference.AgentWorker:        "worker",
		inference.AgentWorkerCode:    "worker_code",
	}

	for agentType, modelID := range assignments {
		providerName, _ := registry.GetProviderForModel(modelID)
		status := "unknown"

		if providerName == "ollama" {
			if !ollamaAvailable {
				status = "ollama offline"
			} else if installedModels[modelID] {
				status = "ready"
			} else {
				status = "not pulled"
			}
		} else {
			// API providers
			status = "api"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			agentNames[agentType], modelID, providerName, status)
	}

	w.Flush()

	fmt.Println()
	fmt.Println("Default model:", registry.GetDefaultModel())
	fmt.Println("Default provider:", syntorConfig.Inference.Provider)

	if !ollamaAvailable {
		fmt.Println()
		fmt.Println("Warning: Ollama is not running. Start it with: docker compose up -d ollama")
	}

	return nil
}

func assignModel(agentType, modelID string) error {
	// Validate agent type
	validAgents := map[string]string{
		"coordination": "Coordination",
		"docs":         "Documentation",
		"git":          "Git",
		"worker":       "Worker",
		"worker_code":  "WorkerCode",
	}

	if _, ok := validAgents[agentType]; !ok {
		return fmt.Errorf("invalid agent type: %s. Valid types: coordination, docs, git, worker, worker_code", agentType)
	}

	// Update config
	switch agentType {
	case "coordination":
		syntorConfig.Inference.Models.Coordination = modelID
	case "docs":
		syntorConfig.Inference.Models.Documentation = modelID
	case "git":
		syntorConfig.Inference.Models.Git = modelID
	case "worker":
		syntorConfig.Inference.Models.Worker = modelID
	case "worker_code":
		syntorConfig.Inference.Models.WorkerCode = modelID
	}

	// Save config
	if err := config.SaveSyntorConfig(syntorConfig); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Assigned %s to %s agent\n", modelID, agentType)
	fmt.Println("Configuration saved to:", config.GlobalConfigPath())

	return nil
}
