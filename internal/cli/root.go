package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/syntor/syntor/internal/cli/tui"
	"github.com/syntor/syntor/pkg/agentdb"
	"github.com/syntor/syntor/pkg/config"
	"github.com/syntor/syntor/pkg/vault"
)

var (
	// Version information (set by build)
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"

	// Global flags
	cfgFile    string
	verbose    bool
	jsonOutput bool
	simpleMode bool

	// Global config
	syntorConfig *config.SyntorConfig
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "syntor",
	Short: "SYNTOR - Multi-Agent AI System",
	Long: `SYNTOR is a multi-agent AI orchestration system that coordinates
specialized agents loaded from the agent database.

Start an interactive session:
  syntor

Run a specific agent:
  syntor run sntr "analyze the codebase"
  syntor run coder "write a function"
  syntor run paladin "review security"

List available agents:
  syntor agents list

Manage models:
  syntor models list
  syntor models status`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Suppress infrastructure warnings in non-verbose mode
		vault.Quiet = !verbose
		agentdb.Quiet = !verbose

		// Load configuration with secrets resolution
		cfg, resolver, err := config.LoadSyntorConfigWithSecrets()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		syntorConfig = cfg
		// Note: resolver is used during loading, secrets are already resolved
		// We could store it globally if needed for runtime secret refresh
		_ = resolver
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
	rootCmd.PersistentFlags().BoolVar(&simpleMode, "simple", false, "use simple REPL mode (no TUI)")

	// Enable --version flag (cobra built-in)
	rootCmd.Version = Version

	// Chat-specific flags
	chatCmd.Flags().StringVarP(&agentModel, "model", "m", "", "override model for this chat message")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(modelsCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(initCmd)

	// Agent commands (dynamic from database)
	rootCmd.AddCommand(agentsCmd)
	rootCmd.AddCommand(runCmd)
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
	// Use simple REPL mode if --simple flag is set
	if simpleMode {
		repl, err := NewREPL(syntorConfig)
		if err != nil {
			return fmt.Errorf("failed to initialize REPL: %w", err)
		}
		return repl.Run()
	}

	// Check for resume session (set by sessions.go or env var)
	resumeID := resumeSession
	if resumeID == "" {
		resumeID = os.Getenv("SYNTOR_RESUME_SESSION")
	}

	// Default: use TUI mode with optional resume
	return tui.Run(syntorConfig, tui.RunOptions{
		ResumeSessionID: resumeID,
	})
}

func sendMessage(message string) error {
	// Use SNTR agent by default, with session/memory/project-instructions support
	return runAgentWithOptions(runOptions{
		agentName:   "sntr",
		message:     message,
		resumeID:    resumeSession,
		forkID:      forkSession,
		sessionName: sessionName,
	})
}

func runSetupWizard() error {
	wizard := NewSetupWizard()
	return wizard.Run()
}
