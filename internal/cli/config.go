package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/syntor/syntor/pkg/config"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage SYNTOR configuration",
	Long: `View and edit SYNTOR configuration.

Commands:
  show    - Display current configuration
  edit    - Open configuration in editor
  reset   - Reset to default configuration
  path    - Show configuration file paths`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		return showConfig()
	},
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit configuration in your editor",
	Long: `Open the global configuration file in your default editor.

Set your editor with the EDITOR environment variable or in config.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return editConfig()
	},
}

var configResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset configuration to defaults",
	RunE: func(cmd *cobra.Command, args []string) error {
		return resetConfig()
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show configuration file paths",
	RunE: func(cmd *cobra.Command, args []string) error {
		return showConfigPaths()
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value.

Keys:
  provider         - Default inference provider (ollama, anthropic, deepseek)
  ollama_host      - Ollama API endpoint
  default_model    - Default model for all agents
  auto_pull        - Automatically pull missing models (true/false)
  theme            - CLI theme (dark, light, auto)
  stream_response  - Stream AI responses (true/false)

Examples:
  syntor config set provider ollama
  syntor config set default_model mistral:7b`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setConfigValue(args[0], args[1])
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configEditCmd)
	configCmd.AddCommand(configResetCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configSetCmd)
}

func showConfig() error {
	data, err := yaml.Marshal(syntorConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	fmt.Println("# SYNTOR Configuration")
	fmt.Println("# Location:", config.GlobalConfigPath())
	fmt.Println()
	fmt.Print(string(data))

	return nil
}

func editConfig() error {
	configPath := config.GlobalConfigPath()

	// Ensure config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config
		if err := config.SaveSyntorConfig(syntorConfig); err != nil {
			return fmt.Errorf("failed to create config: %w", err)
		}
	}

	// Get editor
	editor := syntorConfig.CLI.Editor
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vim"
	}

	// Open in editor
	cmd := exec.Command(editor, configPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func resetConfig() error {
	defaultCfg := config.DefaultSyntorConfig()

	if err := config.SaveSyntorConfig(&defaultCfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println("Configuration reset to defaults")
	fmt.Println("Saved to:", config.GlobalConfigPath())

	return nil
}

func showConfigPaths() error {
	globalDir, projectDir := config.ConfigPaths()

	fmt.Println("Configuration Paths:")
	fmt.Println()
	fmt.Println("Global config directory:", globalDir)
	fmt.Println("Global config file:     ", config.GlobalConfigPath())
	fmt.Println()
	fmt.Println("Project config directory:", projectDir)
	fmt.Println("Project config file:     ", config.ProjectConfigPath())
	fmt.Println()
	fmt.Println("The project config (if present) overrides global settings.")

	// Check which files exist
	fmt.Println()
	fmt.Println("Status:")
	if _, err := os.Stat(config.GlobalConfigPath()); err == nil {
		fmt.Println("  Global config: exists")
	} else {
		fmt.Println("  Global config: not found")
	}
	if _, err := os.Stat(config.ProjectConfigPath()); err == nil {
		fmt.Println("  Project config: exists")
	} else {
		fmt.Println("  Project config: not found")
	}

	return nil
}

func setConfigValue(key, value string) error {
	switch key {
	case "provider":
		syntorConfig.Inference.Provider = value
	case "ollama_host":
		syntorConfig.Inference.OllamaHost = value
	case "default_model":
		syntorConfig.Inference.DefaultModel = value
	case "auto_pull":
		syntorConfig.Inference.AutoPull = (value == "true" || value == "1" || value == "yes")
	case "theme":
		syntorConfig.CLI.Theme = value
	case "stream_response":
		syntorConfig.CLI.StreamResponse = (value == "true" || value == "1" || value == "yes")
	case "editor":
		syntorConfig.CLI.Editor = value
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}

	if err := config.SaveSyntorConfig(syntorConfig); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Set %s = %s\n", key, value)
	return nil
}
