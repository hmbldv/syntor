package config

import (
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

// SyntorConfig holds the complete SYNTOR configuration (YAML format)
type SyntorConfig struct {
	Inference InferenceConfig `yaml:"inference" json:"inference"`
	CLI       CLIConfig       `yaml:"cli" json:"cli"`
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

// DefaultSyntorConfig returns default SYNTOR configuration
func DefaultSyntorConfig() SyntorConfig {
	return SyntorConfig{
		Inference: DefaultInferenceConfig(),
		CLI:       DefaultCLIConfig(),
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
