package hooks

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadConfig loads hooks configuration from a file.
func LoadConfig(path string) (*Config, error) {
	// Expand home directory
	if path != "" && path[0] == '~' {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[1:])
	}

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Return default config if file doesn't exist
		config := DefaultConfig()
		config.ConfigPath = path
		return &config, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	config.ConfigPath = path

	// Apply defaults for missing fields
	if config.GlobalTimeout == 0 {
		config.GlobalTimeout = DefaultConfig().GlobalTimeout
	}

	return &config, nil
}

// SaveConfig saves hooks configuration to a file.
func SaveConfig(config *Config, path string) error {
	if path == "" {
		path = config.ConfigPath
	}
	if path == "" {
		path = DefaultConfig().ConfigPath
	}

	// Expand home directory
	if path[0] == '~' {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[1:])
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

// ExampleConfig returns an example configuration with comments.
func ExampleConfig() string {
	return `# SYNTOR Hooks Configuration
#
# Hooks allow you to intercept and control tool execution.
# Each hook runs at a specific point and can approve, block, or modify operations.

# Global settings
global_timeout: 10s      # Maximum time for a hook to execute
fail_open: false         # If true, tool executes when hook errors. If false, blocked on error.
enable_builtins: true    # Enable built-in security, audit, and confirm hooks

# Custom hooks
hooks:
  # Example: Block rm -rf commands
  - id: block_dangerous_rm
    name: Block Dangerous Remove
    description: Prevents rm -rf on system directories
    type: pre_tool_use
    enabled: true
    order: 1
    tool_match:
      - bash
    handler: shell
    command: |
      if echo "$SYNTOR_TOOL_PARAMS" | grep -q 'rm.*-rf.*/'; then
        echo '{"action": "block", "reason": "Destructive rm command blocked"}'
        exit 1
      fi
      exit 0

  # Example: Require confirmation for file writes
  - id: confirm_writes
    name: Confirm File Writes
    description: Prompts for confirmation before writing files
    type: pre_tool_use
    enabled: true
    order: 10
    tool_match:
      - write_file
      - edit_file
    path_match:
      - "*.go"
      - "*.py"
    handler: builtin
    # Uses the builtin confirm handler

  # Example: Log all bash commands
  - id: audit_bash
    name: Audit Bash Commands
    description: Logs all bash command executions
    type: post_tool_use
    enabled: true
    order: 100
    tool_match:
      - bash
    handler: shell
    command: |
      echo "[$(date -Iseconds)] bash: $SYNTOR_TOOL_PARAMS" >> ~/.syntor/audit.log
      exit 0

  # Example: Use LLM to evaluate security
  - id: llm_security_check
    name: LLM Security Check
    description: Uses AI to evaluate potentially risky operations
    type: pre_tool_use
    enabled: false  # Disabled by default
    order: 5
    tool_match:
      - bash
      - write_file
    handler: prompt
    prompt_template: |
      Evaluate this tool call for security risks:
      Tool: {{tool_name}}
      Parameters: {{tool_params}}
      Working Directory: {{working_dir}}

      Respond with JSON:
      {"decision": "approve|block", "reason": "..."}
`
}
