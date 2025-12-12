package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Initialize inference
	registry, err := setup.InitializeInference(&syntorConfig.Inference)
	if err != nil {
		return fmt.Errorf("failed to initialize inference: %w", err)
	}

	// Get provider and model for this agent
	provider, modelID, err := setup.GetProviderForAgent(registry, agentType)
	if err != nil {
		return fmt.Errorf("failed to get provider: %w", err)
	}

	// Allow model override from flag
	if agentModel != "" {
		modelID = agentModel
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
		fmt.Printf("Using %s with model %s\n", provider.Name(), modelID)
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

	// Add system prompt based on agent type
	req.System = getSystemPrompt(agentType)

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

// getSystemPrompt returns the system prompt for an agent type
func getSystemPrompt(agentType inference.AgentType) string {
	switch agentType {
	case inference.AgentCoordination:
		return `You are SYNTOR's coordination agent. You analyze tasks and create plans
to accomplish complex objectives. You coordinate with other specialized agents
(documentation, git, worker) to complete tasks efficiently.

When given a task:
1. Analyze what needs to be done
2. Break it into steps if needed
3. Identify which agents should handle each step
4. Provide a clear action plan`

	case inference.AgentDocumentation:
		return `You are SYNTOR's documentation agent. You specialize in:
- Analyzing code structure and patterns
- Generating clear documentation
- Creating README files and API documentation
- Explaining complex code in simple terms

Provide thorough, well-structured documentation that helps developers understand the code.`

	case inference.AgentGit:
		return `You are SYNTOR's git agent. You specialize in:
- Creating clear, conventional commit messages
- Analyzing git history and changes
- Managing branches and releases
- Code review and change analysis

Follow conventional commit format (feat:, fix:, docs:, etc.) when creating commit messages.`

	case inference.AgentWorker:
		return `You are SYNTOR's worker agent. You handle general tasks including:
- Answering questions about code
- Summarizing files and content
- General programming assistance

Be concise and helpful in your responses.`

	case inference.AgentWorkerCode:
		return `You are SYNTOR's code worker agent. You specialize in:
- Writing and reviewing code
- Refactoring and optimization
- Bug fixing and debugging
- Code generation and completion

Provide clean, well-structured code with clear explanations.`

	default:
		return "You are a helpful AI assistant."
	}
}
