package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// InferenceConfig holds AI inference configuration
type InferenceConfig struct {
	// Provider settings
	Provider        string `yaml:"provider" json:"provider"`                   // ollama, anthropic, deepseek
	OllamaHost      string `yaml:"ollama_host" json:"ollama_host"`             // Ollama API endpoint
	AnthropicAPIKey string `yaml:"anthropic_api_key" json:"anthropic_api_key"` // Anthropic API key (optional)
	DeepSeekAPIKey  string `yaml:"deepseek_api_key" json:"deepseek_api_key"`   // DeepSeek API key (optional)

	// Default model for all agents
	DefaultModel string `yaml:"default_model" json:"default_model"`

	// Per-agent model assignments
	Models AgentModels `yaml:"models" json:"models"`

	// Model pull behavior
	AutoPull bool `yaml:"auto_pull" json:"auto_pull"` // Automatically pull missing models
}

// AgentModels holds model assignments for each agent type
type AgentModels struct {
	Coordination  string `yaml:"coordination" json:"coordination"`
	Documentation string `yaml:"documentation" json:"documentation"`
	Git           string `yaml:"git" json:"git"`
	Worker        string `yaml:"worker" json:"worker"`
	WorkerCode    string `yaml:"worker_code" json:"worker_code"`
}

// ContextConfig holds context window management settings
type ContextConfig struct {
	MaxTokens      int      `yaml:"max_tokens" json:"max_tokens"`
	CompactAt      float64  `yaml:"compact_at" json:"compact_at"`
	PreserveRecent int      `yaml:"preserve_recent" json:"preserve_recent"`
	PreserveKeys   []string `yaml:"preserve_keys" json:"preserve_keys"`
}

// PermissionsConfig holds permission auto-allow settings
type PermissionsConfig struct {
	DefaultMode string `yaml:"default_mode" json:"default_mode"` // "plan" or "auto"
	ConfigPath  string `yaml:"config_path" json:"config_path"`
}

// SyntorConfig holds the complete SYNTOR configuration (YAML format)
type SyntorConfig struct {
	Inference    InferenceConfig    `yaml:"inference" json:"inference"`
	CLI          CLIConfig          `yaml:"cli" json:"cli"`
	Integrations IntegrationsConfig `yaml:"integrations" json:"integrations"`
	Context      ContextConfig      `yaml:"context" json:"context"`
	Permissions  PermissionsConfig  `yaml:"permissions" json:"permissions"`
}

// CLIConfig holds CLI-specific configuration
type CLIConfig struct {
	Theme          string `yaml:"theme" json:"theme"`                       // dark, light, auto
	Editor         string `yaml:"editor" json:"editor"`                     // preferred editor for editing
	AutoApprove    bool   `yaml:"auto_approve" json:"auto_approve"`         // auto-approve certain actions
	StreamResponse bool   `yaml:"stream_response" json:"stream_response"`   // stream responses in real-time
}

// DefaultInferenceConfig returns default inference configuration
func DefaultInferenceConfig() InferenceConfig {
	return InferenceConfig{
		Provider:     "ollama",
		OllamaHost:   GetEnv("SYNTOR_OLLAMA_HOST", "http://localhost:11434"),
		DefaultModel: "llama3.2:8b",
		Models: AgentModels{
			Coordination:  "mistral:7b",
			Documentation: "deepseek-coder-v2:16b",
			Git:           "llama3.2:8b",
			Worker:        "llama3.2:3b",
			WorkerCode:    "qwen2.5-coder:7b",
		},
		AutoPull: true,
	}
}

// DefaultCLIConfig returns default CLI configuration
func DefaultCLIConfig() CLIConfig {
	return CLIConfig{
		Theme:          "auto",
		Editor:         GetEnv("EDITOR", "vim"),
		AutoApprove:    false,
		StreamResponse: true,
	}
}

// DefaultContextConfig returns default context window configuration
func DefaultContextConfig() ContextConfig {
	return ContextConfig{
		MaxTokens:      120000,
		CompactAt:      0.75,
		PreserveRecent: 10,
		PreserveKeys:   []string{"working_directory", "active_agent"},
	}
}

// DefaultPermissionsConfig returns default permissions configuration
func DefaultPermissionsConfig() PermissionsConfig {
	return PermissionsConfig{
		DefaultMode: "plan",
		ConfigPath:  "",
	}
}

// DefaultSyntorConfig returns default SYNTOR configuration
func DefaultSyntorConfig() SyntorConfig {
	return SyntorConfig{
		Inference:    DefaultInferenceConfig(),
		CLI:          DefaultCLIConfig(),
		Integrations: DefaultIntegrationsConfig(),
		Context:      DefaultContextConfig(),
		Permissions:  DefaultPermissionsConfig(),
	}
}

// ConfigPaths returns the global and project config paths
func ConfigPaths() (globalDir, projectDir string) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	globalDir = filepath.Join(home, ".syntor")
	projectDir = ".syntor"
	return
}

// GlobalConfigPath returns the path to global config file
func GlobalConfigPath() string {
	globalDir, _ := ConfigPaths()
	return filepath.Join(globalDir, "config.yaml")
}

// ProjectConfigPath returns the path to project config file
func ProjectConfigPath() string {
	_, projectDir := ConfigPaths()
	return filepath.Join(projectDir, "config.yaml")
}

// LoadSyntorConfig loads configuration from YAML files
// It merges global config with project-level overrides
func LoadSyntorConfig() (*SyntorConfig, error) {
	config := DefaultSyntorConfig()

	// Load global config first
	globalPath := GlobalConfigPath()
	if err := loadYAMLConfig(globalPath, &config); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	// Override with project config
	projectPath := ProjectConfigPath()
	if err := loadYAMLConfig(projectPath, &config); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	// Override with environment variables
	applyEnvOverrides(&config)

	return &config, nil
}

// loadYAMLConfig loads a YAML config file into the config struct
func loadYAMLConfig(path string, config *SyntorConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return yaml.Unmarshal(data, config)
}

// applyEnvOverrides applies environment variable overrides to config
func applyEnvOverrides(config *SyntorConfig) {
	// Inference overrides
	if v := os.Getenv("SYNTOR_OLLAMA_HOST"); v != "" {
		config.Inference.OllamaHost = v
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		config.Inference.AnthropicAPIKey = v
	}
	if v := os.Getenv("DEEPSEEK_API_KEY"); v != "" {
		config.Inference.DeepSeekAPIKey = v
	}
	if v := os.Getenv("SYNTOR_INFERENCE_MODEL"); v != "" {
		config.Inference.DefaultModel = v
	}
	if v := os.Getenv("SYNTOR_INFERENCE_PROVIDER"); v != "" {
		config.Inference.Provider = v
	}

	// Per-agent model overrides
	if v := os.Getenv("SYNTOR_MODEL_COORDINATION"); v != "" {
		config.Inference.Models.Coordination = v
	}
	if v := os.Getenv("SYNTOR_MODEL_DOCUMENTATION"); v != "" {
		config.Inference.Models.Documentation = v
	}
	if v := os.Getenv("SYNTOR_MODEL_GIT"); v != "" {
		config.Inference.Models.Git = v
	}
	if v := os.Getenv("SYNTOR_MODEL_WORKER"); v != "" {
		config.Inference.Models.Worker = v
	}
	if v := os.Getenv("SYNTOR_MODEL_WORKER_CODE"); v != "" {
		config.Inference.Models.WorkerCode = v
	}

	// Integration overrides
	if v := os.Getenv("SYNTOR_HERALD_URL"); v != "" {
		config.Integrations.Herald.BaseURL = v
	}
	if v := os.Getenv("SYNTOR_FALKORDB_ADDR"); v != "" {
		config.Integrations.FalkorDB.Address = v
	}
}

// ProjectMarkdownPath returns the path to SYNTOR.md in the current project
func ProjectMarkdownPath() string {
	return "SYNTOR.md"
}

// LoadProjectMarkdown loads SYNTOR.md from the current directory or parent directories
func LoadProjectMarkdown() (string, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	// Search current directory and up to 5 parent directories
	dir := cwd
	for i := 0; i < 6; i++ {
		mdPath := filepath.Join(dir, "SYNTOR.md")
		if data, err := os.ReadFile(mdPath); err == nil {
			return string(data), mdPath, nil
		}

		// Move to parent directory
		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached root
		}
		dir = parent
	}

	return "", "", os.ErrNotExist
}

// ProjectMarkdownExists checks if SYNTOR.md exists in the project
func ProjectMarkdownExists() bool {
	_, _, err := LoadProjectMarkdown()
	return err == nil
}

// CreateProjectMarkdown creates a default SYNTOR.md in the current directory
func CreateProjectMarkdown(projectName, projectDesc string) error {
	content := fmt.Sprintf(`# SYNTOR.md

This file provides guidance to SYNTOR when working with code in this repository.

## Project: %s

%s

---

## Codebase Overview

<!-- Describe your project's architecture, key directories, and important files -->

## Development Guidelines

<!-- Add coding conventions, style guides, and best practices -->

## Build & Test Commands

<!-- List common commands for building, testing, and running the project -->

## Important Notes

<!-- Any critical information SYNTOR should know about this codebase -->

## Skills

Active skills from ~/.syntor/skills/ are automatically loaded.
See /skills command for available skills.
`, projectName, projectDesc)

	return os.WriteFile(ProjectMarkdownPath(), []byte(content), 0644)
}

// LoadSettings loads settings.json from the global config directory
func LoadSettings() (map[string]interface{}, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	settingsPath := filepath.Join(home, ".syntor", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]interface{}), nil
		}
		return nil, err
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}

	return settings, nil
}

// SaveSettings saves settings to settings.json in the global config directory
func SaveSettings(settings map[string]interface{}) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	settingsPath := filepath.Join(home, ".syntor", "settings.json")
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(settingsPath, data, 0644)
}

// GetProjectContext returns project-specific context for the AI
func GetProjectContext() (string, error) {
	content, path, err := LoadProjectMarkdown()
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // No SYNTOR.md, not an error
		}
		return "", err
	}

	return fmt.Sprintf("# Project Context (from %s)\n\n%s", path, content), nil
}

// GlobalContextPath returns the path to the global CENTAUR.md
func GlobalContextPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".syntor", "CENTAUR.md")
}

// LoadGlobalContext loads the global CENTAUR.md context
func LoadGlobalContext() (string, error) {
	path := GlobalContextPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // No CENTAUR.md, not an error
		}
		return "", err
	}
	return string(data), nil
}

// GetGlobalContext returns global context for the AI
func GetGlobalContext() (string, error) {
	content, err := LoadGlobalContext()
	if err != nil {
		return "", err
	}
	if content == "" {
		return "", nil
	}
	return content, nil
}

// CreateDefaultGlobalContext creates a default CENTAUR.md file
func CreateDefaultGlobalContext() error {
	path := GlobalContextPath()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	content := `# CENTAUR - Multi-Agent Orchestration System

You are Centaur, an intelligent orchestrator that coordinates specialized agents.

## Core Principles
1. **Route, don't do everything** - Delegate to specialists when appropriate
2. **Show your work** - Make handoffs and agent activity visible
3. **Manage context** - Be aware of token limits and scale appropriately
4. **Iterate and learn** - Build better configurations over time

## Agent Routing Protocol

**IMPORTANT: All agent routing MUST query FalkorDB.**

Before responding to a user query:
1. Assess: Is this a simple question or complex task?
2. Route: Query FalkorDB agents graph for best-suited agent:
   ` + "```" + `cypher
   MATCH (sage:Agent {name: 'Sage'})-[r:ROUTES_TO]->(target:Agent)
   WHERE r.task_type = $task_type
   OPTIONAL MATCH (target)-[:REPORTS_TO*1..3]->(chain:Agent)
   OPTIONAL MATCH (target)-[:MEMBER_OF]->(team:Team)
   RETURN target.name AS agent, target.role, target.focus,
          team.name AS team, collect(DISTINCT chain.name) AS chain,
          target.definition_path, target.operations_dir
   ` + "```" + `
3. Load agent context from definition_path if found
4. Delegate: Hand off to specialist with clear task description
5. Synthesize: Combine results if multi-agent collaboration needed

## Agent Discovery

Use /agents command to list all available agents from FalkorDB.
Agent definitions are stored in the graph - never hardcode agent lists.

## Task Type Keywords

Query FalkorDB for current task type mappings:
` + "```" + `cypher
MATCH (a:Agent)-[r:ROUTES_TO]->(b:Agent)
RETURN DISTINCT r.task_type AS task_type, b.name AS handler
ORDER BY r.task_type
` + "```" + `

## Context Awareness
- Global context: Device-level settings and preferences
- Project context: Current codebase specifics from SYNTOR.md
- Session context: Conversation history and checkpoints
- Skills: Always-active behavioral guidelines
- Agents: Dynamic from FalkorDB graph
`
	return os.WriteFile(path, []byte(content), 0644)
}

// GlobalContextExists checks if CENTAUR.md exists
func GlobalContextExists() bool {
	_, err := os.Stat(GlobalContextPath())
	return err == nil
}

// SaveSyntorConfig saves configuration to the global config file
func SaveSyntorConfig(config *SyntorConfig) error {
	globalDir, _ := ConfigPaths()

	// Ensure directory exists
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	return os.WriteFile(GlobalConfigPath(), data, 0644)
}

// SaveProjectConfig saves configuration to the project config file
func SaveProjectConfig(config *SyntorConfig) error {
	_, projectDir := ConfigPaths()

	// Ensure directory exists
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	return os.WriteFile(ProjectConfigPath(), data, 0644)
}

// GetModelForAgent returns the model to use for a given agent type
func (c *InferenceConfig) GetModelForAgent(agentType string) string {
	switch agentType {
	case "coordination":
		if c.Models.Coordination != "" {
			return c.Models.Coordination
		}
	case "documentation":
		if c.Models.Documentation != "" {
			return c.Models.Documentation
		}
	case "git":
		if c.Models.Git != "" {
			return c.Models.Git
		}
	case "worker":
		if c.Models.Worker != "" {
			return c.Models.Worker
		}
	case "worker_code":
		if c.Models.WorkerCode != "" {
			return c.Models.WorkerCode
		}
	}
	return c.DefaultModel
}

// GetAllAssignedModels returns all unique models assigned to agents
func (c *InferenceConfig) GetAllAssignedModels() []string {
	seen := make(map[string]bool)
	var models []string

	addModel := func(m string) {
		if m != "" && !seen[m] {
			seen[m] = true
			models = append(models, m)
		}
	}

	addModel(c.Models.Coordination)
	addModel(c.Models.Documentation)
	addModel(c.Models.Git)
	addModel(c.Models.Worker)
	addModel(c.Models.WorkerCode)
	addModel(c.DefaultModel)

	return models
}
