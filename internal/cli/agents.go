package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/syntor/syntor/pkg/agentdb"
	"github.com/syntor/syntor/pkg/inference"
	"github.com/syntor/syntor/pkg/setup"
)

var (
	agentModel string // Override model for this invocation
)

// agentsCmd is the parent command for agent operations
var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Manage and run agents",
	Long: `Manage and run agents from the agent database.

Commands:
  list    - List all available agents
  run     - Run an agent with a message
  info    - Show details about an agent`,
}

// agentsListCmd lists all agents from the database
var agentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available agents",
	Long:  `List all agents from the agent database with their roles and models.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listAgents()
	},
}

// agentsRunCmd runs a specific agent
var agentsRunCmd = &cobra.Command{
	Use:   "run <agent-name> [message]",
	Short: "Run an agent with a message",
	Long: `Run a specific agent from the database.

Examples:
  syntor agents run sntr "analyze this codebase"
  syntor agents run coder "write a function to parse JSON"
  syntor agents run paladin "review security posture"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		agentName := args[0]
		message := ""
		if len(args) > 1 {
			message = strings.Join(args[1:], " ")
		}
		if message == "" {
			return fmt.Errorf("please provide a message")
		}
		return runAgentByName(agentName, message)
	},
}

// agentsInfoCmd shows details about an agent
var agentsInfoCmd = &cobra.Command{
	Use:   "info <agent-name>",
	Short: "Show details about an agent",
	Long:  `Display detailed information about a specific agent from the database.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return showAgentInfo(args[0])
	},
}

// runCmd is a shortcut to run any agent directly: syntor run <agent> <message>
var runCmd = &cobra.Command{
	Use:   "run <agent-name> [message]",
	Short: "Run an agent (shortcut for 'agents run')",
	Long: `Run any agent from the database by name.

Examples:
  syntor run sntr "coordinate this task"
  syntor run coder "implement this feature"
  syntor run paladin "assess security"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		agentName := args[0]
		message := ""
		if len(args) > 1 {
			message = strings.Join(args[1:], " ")
		}
		if message == "" {
			return fmt.Errorf("please provide a message")
		}
		return runAgentByName(agentName, message)
	},
}

func init() {
	// Build agents command tree
	agentsCmd.AddCommand(agentsListCmd)
	agentsCmd.AddCommand(agentsRunCmd)
	agentsCmd.AddCommand(agentsInfoCmd)

	// Add model override flag
	agentsRunCmd.Flags().StringVarP(&agentModel, "model", "m", "", "override model for this request")
	runCmd.Flags().StringVarP(&agentModel, "model", "m", "", "override model for this request")
}

// listAgents lists all agents from the database
func listAgents() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loader, err := getAgentLoader()
	if err != nil {
		return fmt.Errorf("agent database not available: %w", err)
	}
	defer loader.Close()

	agents, err := loader.ListAgents(ctx)
	if err != nil {
		return fmt.Errorf("failed to list agents: %w", err)
	}

	if len(agents) == 0 {
		fmt.Println("No agents found in database.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "AGENT\tROLE\tMODEL\tSTATUS")
	fmt.Fprintln(w, "-----\t----\t-----\t------")

	for _, agent := range agents {
		model := agent.Model
		if model == "" {
			model = "-"
		}
		status := "ready"
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", agent.Name, agent.Role, model, status)
	}

	w.Flush()
	fmt.Printf("\nTotal: %d agents\n", len(agents))
	return nil
}

// showAgentInfo displays details about a specific agent
func showAgentInfo(agentName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loader, err := getAgentLoader()
	if err != nil {
		return fmt.Errorf("agent database not available: %w", err)
	}
	defer loader.Close()

	agent, err := loader.LoadAgent(ctx, agentName)
	if err != nil {
		return fmt.Errorf("agent not found: %s", agentName)
	}

	fmt.Printf("=== Agent: %s ===\n\n", agent.AgentID)
	fmt.Printf("Source: %s\n", agent.Source)
	fmt.Printf("Version: %d\n", agent.Version)

	if agent.GetModel() != "" {
		fmt.Printf("Model: %s\n", agent.GetModel())
	}

	if agent.Personality != nil {
		fmt.Printf("\nPersonality:\n")
		if agent.Personality.Tone != "" {
			fmt.Printf("  Tone: %s\n", agent.Personality.Tone)
		}
		if agent.Personality.Style != "" {
			fmt.Printf("  Style: %s\n", agent.Personality.Style)
		}
	}

	if agent.SystemPrompt != "" {
		fmt.Printf("\nSystem Prompt (first 500 chars):\n")
		prompt := agent.SystemPrompt
		if len(prompt) > 500 {
			prompt = prompt[:500] + "..."
		}
		fmt.Println(prompt)
	}

	return nil
}

// getAgentLoader creates an agent loader from config
func getAgentLoader() (*agentdb.UnifiedLoader, error) {
	if !syntorConfig.Integrations.AgentDB.Enabled {
		return nil, fmt.Errorf("agent database is not enabled in config")
	}

	loaderCfg := agentdb.UnifiedLoaderConfig{
		AgentDBConfig: &agentdb.Config{
			Host:     syntorConfig.Integrations.AgentDB.Host,
			Port:     syntorConfig.Integrations.AgentDB.Port,
			Database: syntorConfig.Integrations.AgentDB.Database,
			Schema:   syntorConfig.Integrations.AgentDB.Schema,
			User:     syntorConfig.Integrations.AgentDB.User,
			Password: syntorConfig.Integrations.AgentDB.Password,
			SSLMode:  syntorConfig.Integrations.AgentDB.SSLMode,
			CacheTTL: syntorConfig.Integrations.AgentDB.CacheTTL,
		},
		PreferDatabase: syntorConfig.Integrations.AgentDB.PreferDatabase,
	}

	return agentdb.NewUnifiedLoader(loaderCfg)
}

// runAgentByName executes a message with an agent loaded dynamically from databases
func runAgentByName(agentName string, message string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Initialize inference
	registry, err := setup.InitializeInference(&syntorConfig.Inference)
	if err != nil {
		return fmt.Errorf("failed to initialize inference: %w", err)
	}

	// Try to initialize agentdb loader for dynamic resolution
	var agentLoader *agentdb.UnifiedLoader
	var loadedAgent *agentdb.LoadedAgent

	if syntorConfig.Integrations.AgentDB.Enabled {
		if loader, err := getAgentLoader(); err == nil {
			agentLoader = loader
			defer loader.Close()

			// Load agent definition
			if loaded, err := loader.LoadAgent(ctx, agentName); err == nil {
				loadedAgent = loaded
			} else if verbose {
				fmt.Printf("Agent %s not found in database, using fallback\n", agentName)
			}
		}
	}

	// Determine model - prefer database, fall back to default
	var modelID string
	if loadedAgent != nil && loadedAgent.GetModel() != "" {
		modelID = loadedAgent.GetModel()
	} else {
		// Fall back to default model
		modelID = registry.GetDefaultModel()
	}

	// Allow model override from flag
	if agentModel != "" {
		modelID = agentModel
	}

	// Get default provider (Ollama)
	provider, ok := registry.GetDefaultProvider()
	if !ok {
		return fmt.Errorf("no inference provider available")
	}

	// Check provider availability
	if !provider.IsAvailable(ctx) {
		return fmt.Errorf("provider %s is not available. Is Ollama running?", provider.Name())
	}

	// Check if model is available
	hasModel, err := provider.HasModel(ctx, modelID)
	if err != nil {
		return fmt.Errorf("failed to check model: %w", err)
	}

	if !hasModel {
		if syntorConfig.Inference.AutoPull {
			fmt.Printf("Model %s not found, pulling...\n", modelID)
			err := provider.PullModel(ctx, modelID, func(p inference.PullProgress) {
				if p.Percent > 0 {
					fmt.Printf("\rPulling: %.1f%%", p.Percent)
				}
			})
			fmt.Println()
			if err != nil {
				return fmt.Errorf("failed to pull model: %w", err)
			}
		} else {
			return fmt.Errorf("model %s not found. Run: syntor models pull %s", modelID, modelID)
		}
	}

	if verbose {
		fmt.Printf("Using %s with model %s (agent: %s)\n", provider.Name(), modelID, agentName)
	}

	// Build the request
	req := inference.ChatRequest{
		Model: modelID,
		Messages: []inference.Message{
			{
				Role:    "user",
				Content: message,
			},
		},
	}

	// Get system prompt - prefer database, fall back to generic
	if loadedAgent != nil && loadedAgent.SystemPrompt != "" {
		req.System = loadedAgent.SystemPrompt
		if verbose {
			fmt.Printf("Using system prompt from database (version %d)\n", loadedAgent.Version)
		}
	} else {
		req.System = getGenericSystemPrompt(agentName)
	}

	// Use streaming if configured
	if syntorConfig.CLI.StreamResponse {
		return streamChat(ctx, provider, req)
	}

	// Non-streaming request
	resp, err := provider.Chat(ctx, req)
	if err != nil {
		return fmt.Errorf("chat failed: %w", err)
	}

	fmt.Println(resp.Message.Content)

	if verbose {
		fmt.Printf("\n[tokens: %d prompt, %d completion]\n",
			resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
	}

	_ = agentLoader // Silence unused variable warning if loader was created
	return nil
}

// streamChat performs a streaming chat request
func streamChat(ctx context.Context, provider inference.Provider, req inference.ChatRequest) error {
	stream, err := provider.ChatStream(ctx, req)
	if err != nil {
		return fmt.Errorf("stream failed: %w", err)
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
	return nil
}

// getGenericSystemPrompt returns a generic system prompt for unknown agents
// This is only used when the database is unavailable or agent not found
func getGenericSystemPrompt(agentName string) string {
	return fmt.Sprintf(`You are the %s agent in the SYNTOR multi-agent system.

## Guidelines
- Be helpful and professional
- Complete the requested task thoroughly
- Ask clarifying questions if needed`, agentName)
}
