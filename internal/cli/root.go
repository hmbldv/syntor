package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/syntor/syntor/pkg/config"
	"github.com/syntor/syntor/pkg/inference"
)

var (
	// Version information (set by build)
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"

	// Global flags
	cfgFile   string
	verbose   bool
	jsonOutput bool

	// Global config
	syntorConfig *config.SyntorConfig
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "syntor",
	Short: "SYNTOR - Multi-Agent AI System",
	Long: `SYNTOR is a multi-agent AI orchestration system that coordinates
specialized agents for documentation, git operations, and general tasks.

Start an interactive session:
  syntor

Run a specific agent:
  syntor coordination "analyze the codebase"
  syntor docs "generate documentation"
  syntor git "create a commit message"

Manage models:
  syntor models list
  syntor models pull mistral:7b
  syntor models status`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Load configuration
		cfg, err := config.LoadSyntorConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		syntorConfig = cfg
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// If no subcommand, start interactive mode
		if len(args) == 0 {
			return runInteractive()
		}
		return cmd.Help()
	},
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.syntor/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output in JSON format")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(modelsCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(initCmd)

	// Agent commands
	rootCmd.AddCommand(coordinationCmd)
	rootCmd.AddCommand(docsCmd)
	rootCmd.AddCommand(gitAgentCmd)
	rootCmd.AddCommand(workerCmd)
}

// versionCmd shows version information
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("SYNTOR %s\n", Version)
		fmt.Printf("Build: %s\n", BuildTime)
		fmt.Printf("Commit: %s\n", GitCommit)
	},
}

// chatCmd starts a chat session with the default agent
var chatCmd = &cobra.Command{
	Use:   "chat [message]",
	Short: "Start a chat session or send a message",
	Long: `Start an interactive chat session with SYNTOR, or send a single message.

Examples:
  syntor chat                    # Start interactive chat
  syntor chat "explain this code"  # Send a single message`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return sendMessage(args[0])
		}
		return runInteractive()
	},
}

// initCmd initializes SYNTOR for first-time use
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize SYNTOR configuration",
	Long: `Run the first-time setup wizard to configure SYNTOR.

This will:
  - Create configuration directory (~/.syntor/)
  - Check Ollama availability
  - Pull required models
  - Configure default settings`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSetupWizard()
	},
}

// runInteractive starts the interactive REPL
func runInteractive() error {
	repl, err := NewREPL(syntorConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize REPL: %w", err)
	}
	return repl.Run()
}

func sendMessage(message string) error {
	// Use coordination agent by default for single messages
	return runAgent(inference.AgentCoordination, message)
}

func runSetupWizard() error {
	wizard := NewSetupWizard()
	return wizard.Run()
}
