package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/syntor/syntor/pkg/agentdb"
	"github.com/syntor/syntor/pkg/inference"
	"github.com/syntor/syntor/pkg/setup"
)

var (
	agentModel string // Override model for this invocation
)

// coordinationCmd runs the coordination agent
var coordinationCmd = &cobra.Command{
	Use:   "coordination [message]",
	Short: "Run the coordination agent",
	Long: `Send a task to the coordination agent for orchestration.

The coordination agent analyzes tasks and coordinates other agents
to accomplish complex multi-step objectives.

Examples:
  syntor coordination "analyze the codebase structure"
  syntor coordination "create a plan to implement feature X"`,
	Aliases: []string{"coord", "orchestrate"},
	RunE: func(cmd *cobra.Command, args []string) error {
		message := strings.Join(args, " ")
		if message == "" {
			return fmt.Errorf("please provide a message")
		}
		return runAgent(inference.AgentCoordination, message)
	},
}

// docsCmd runs the documentation agent
var docsCmd = &cobra.Command{
	Use:   "docs [message]",
	Short: "Run the documentation agent",
	Long: `Send a task to the documentation agent.

The documentation agent specializes in:
  - Generating documentation from code
  - Analyzing code structure and patterns
  - Creating README files and API docs

Examples:
  syntor docs "generate documentation for pkg/inference"
  syntor docs "explain this codebase"`,
	Aliases: []string{"documentation", "doc"},
	RunE: func(cmd *cobra.Command, args []string) error {
		message := strings.Join(args, " ")
		if message == "" {
			return fmt.Errorf("please provide a message")
		}
		return runAgent(inference.AgentDocumentation, message)
	},
}

// gitAgentCmd runs the git agent
var gitAgentCmd = &cobra.Command{
	Use:   "git [message]",
	Short: "Run the git agent",
	Long: `Send a task to the git agent.

The git agent specializes in:
  - Creating commit messages
  - Analyzing git history
  - Managing branches
  - Code review assistance

Examples:
  syntor git "create a commit message for staged changes"
  syntor git "summarize recent commits"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		message := strings.Join(args, " ")
		if message == "" {
			return fmt.Errorf("please provide a message")
		}
		return runAgent(inference.AgentGit, message)
	},
}

// workerCmd runs a worker agent
var workerCmd = &cobra.Command{
	Use:   "worker [message]",
	Short: "Run a worker agent",
	Long: `Send a task to a worker agent.

Worker agents handle general tasks and code-specific operations.
Use --code flag for code-specific tasks.

Examples:
  syntor worker "summarize this file"
  syntor worker --code "refactor this function"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		message := strings.Join(args, " ")
		if message == "" {
			return fmt.Errorf("please provide a message")
		}

		codeMode, _ := cmd.Flags().GetBool("code")
		agentType := inference.AgentWorker
		if codeMode {
			agentType = inference.AgentWorkerCode
		}

		return runAgent(agentType, message)
	},
}

func init() {
	// Add model override flag to all agent commands
	for _, cmd := range []*cobra.Command{coordinationCmd, docsCmd, gitAgentCmd, workerCmd} {
		cmd.Flags().StringVarP(&agentModel, "model", "m", "", "override model for this request")
	}

	// Add code flag to worker
	workerCmd.Flags().Bool("code", false, "use code-specialized model")
}

// runAgent executes a message with the specified agent type
func runAgent(agentType inference.AgentType, message string) error {
	return runAgentByName(string(agentType), message)
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
		loaderCfg := agentdb.UnifiedLoaderConfig{
			AgentDBConfig:  &agentdb.Config{
				Host:     syntorConfig.Integrations.AgentDB.Host,
				Port:     syntorConfig.Integrations.AgentDB.Port,
				Database: syntorConfig.Integrations.AgentDB.Database,
				Schema:   syntorConfig.Integrations.AgentDB.Schema,
				SSLMode:  syntorConfig.Integrations.AgentDB.SSLMode,
				CacheTTL: syntorConfig.Integrations.AgentDB.CacheTTL,
			},
			PreferDatabase: syntorConfig.Integrations.AgentDB.PreferDatabase,
		}
		if loader, err := agentdb.NewUnifiedLoader(loaderCfg); err == nil {
			agentLoader = loader
			defer loader.Close()

			// Load agent definition
			if loaded, err := loader.LoadAgent(ctx, agentName); err == nil {
				loadedAgent = loaded
			}
		}
	}

	// Determine model - prefer database, fall back to static
	var modelID string
	if loadedAgent != nil && loadedAgent.GetModel() != "" {
		modelID = loadedAgent.GetModel()
	} else {
		// Fall back to static registry
		modelID = registry.GetModelForAgent(inference.AgentType(agentName))
	}

	// Allow model override from flag
	if agentModel != "" {
		modelID = agentModel
	}

	// Get provider for model
	provider, _, err := setup.GetProviderForAgent(registry, inference.AgentType(agentName))
	if err != nil {
		return fmt.Errorf("failed to get provider: %w", err)
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

	// Get system prompt - prefer database, fall back to static
	if loadedAgent != nil && loadedAgent.SystemPrompt != "" {
		req.System = loadedAgent.SystemPrompt
		if verbose {
			fmt.Printf("Using system prompt from database (version %d)\n", loadedAgent.Version)
		}
	} else {
		req.System = getSystemPrompt(inference.AgentType(agentName))
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

// getSystemPrompt returns the fallback system prompt for an agent type
// DEPRECATED: Prefer database-loaded system prompts from agentdb
// This is only used when the database is unavailable or agent not found
func getSystemPrompt(agentType inference.AgentType) string {
	// Check for known legacy agent types
	switch agentType {
	case inference.AgentSNTR: // Also matches AgentCoordination (same value)
		return `You are SNTR, the primary AI orchestration agent for SYNTOR.

## Your Role
You coordinate multi-agent workflows and route tasks to specialized agents.

## Guidelines
- Analyze tasks and break them into steps
- Route to appropriate specialized agents
- Provide clear, actionable responses`

	case inference.AgentDocumentation:
		return `You are a documentation specialist agent.

## Your Role
You create clear, comprehensive documentation from code.

## Guidelines
- Analyze code structure and patterns
- Generate well-structured documentation
- Explain complex concepts simply`

	case inference.AgentGit:
		return `You are a git operations specialist agent.

## Your Role
You handle git operations and create conventional commit messages.

## Guidelines
- Use conventional commit format (feat:, fix:, docs:, etc.)
- Analyze changes before committing
- Follow git best practices`

	case inference.AgentWorker:
		return `You are a general worker agent.

## Your Role
You handle general tasks and assist with various requests.

## Guidelines
- Be concise and helpful
- Ask clarifying questions when needed`

	case inference.AgentWorkerCode:
		return `You are a code specialist worker agent.

## Your Role
You write, review, and refactor code.

## Guidelines
- Write clean, well-structured code
- Follow best practices for the language
- Explain your code clearly`

	default:
		// For dynamic agents not in the legacy constants,
		// return a generic prompt that includes their name
		return fmt.Sprintf(`You are the %s agent in the SYNTOR multi-agent system.

## Guidelines
- Be helpful and professional
- Complete the requested task thoroughly
- Ask clarifying questions if needed`, string(agentType))
	}
}
