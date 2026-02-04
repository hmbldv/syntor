package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
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
  status  - Show model status for each agent`,
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
	Long:  `Show which models are assigned to each agent (from database) and their availability status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return showModelStatus()
	},
}

func init() {
	modelsCmd.AddCommand(modelsListCmd)
	modelsCmd.AddCommand(modelsPullCmd)
	modelsCmd.AddCommand(modelsStatusCmd)
}

func listModels() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Initialize registry
	registry, err := setup.InitializeInference(&syntorConfig.Inference)
	if err != nil {
		return fmt.Errorf("failed to initialize inference: %w", err)
	}

	// Get all available models (including from Ollama)
	allModels := registry.GetAvailableModelsWithContext(ctx)

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

	// Find the model - check static list first, then assume Ollama for unknown models
	model, found := registry.FindModel(modelID)
	if !found {
		// Unknown model - assume it's an Ollama model that can be pulled
		model = inference.Model{
			ID:       modelID,
			Name:     modelID,
			Provider: "ollama",
		}
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

	// Get agents from database
	loader, err := getAgentLoader()
	if err != nil {
		return fmt.Errorf("agent database not available: %w", err)
	}
	defer loader.Close()

	agents, err := loader.ListAgents(ctx)
	if err != nil {
		return fmt.Errorf("failed to list agents: %w", err)
	}

	fmt.Println("=== Agent Model Assignments ===")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "AGENT\tROLE\tMODEL\tSTATUS")
	fmt.Fprintln(w, "-----\t----\t-----\t------")

	for _, agent := range agents {
		modelID := agent.Model
		if modelID == "" {
			modelID = registry.GetDefaultModel()
		}

		status := "unknown"
		providerName, _ := registry.GetProviderForModelWithContext(ctx, modelID)

		if providerName == "ollama" {
			if !ollamaAvailable {
				status = "ollama offline"
			} else if installedModels[modelID] {
				status = "ready"
			} else {
				status = "not pulled"
			}
		} else if providerName != "" {
			status = "api"
		}

		role := agent.Role
		if len(role) > 30 {
			role = role[:27] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", agent.Name, role, modelID, status)
	}

	w.Flush()

	fmt.Println()
	fmt.Printf("Total: %d agents\n", len(agents))
	fmt.Println("Default model:", registry.GetDefaultModel())

	if !ollamaAvailable {
		fmt.Println()
		fmt.Println("Warning: Ollama is not running")
	}

	return nil
}
