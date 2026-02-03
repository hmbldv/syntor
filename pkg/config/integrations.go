package config

import (
	"time"

	"syntor/pkg/checkpoint"
	"syntor/pkg/falkordb"
	"syntor/pkg/herald"
	"syntor/pkg/hooks"
	"syntor/pkg/mcp"
	"syntor/pkg/subagent"
)

// IntegrationsConfig holds configuration for all external integrations.
type IntegrationsConfig struct {
	Herald     HeraldConfig     `yaml:"herald" json:"herald"`
	FalkorDB   FalkorDBConfig   `yaml:"falkordb" json:"falkordb"`
	MCP        MCPConfig        `yaml:"mcp" json:"mcp"`
	Checkpoint CheckpointConfig `yaml:"checkpoint" json:"checkpoint"`
	Hooks      HooksConfig      `yaml:"hooks" json:"hooks"`
	SubAgent   SubAgentConfig   `yaml:"subagent" json:"subagent"`
	Systems    SystemsConfig    `yaml:"systems" json:"systems"`
}

// HeraldConfig holds Herald gateway configuration.
type HeraldConfig struct {
	Enabled          bool          `yaml:"enabled" json:"enabled"`
	BaseURL          string        `yaml:"base_url" json:"base_url"`
	Timeout          time.Duration `yaml:"timeout" json:"timeout"`
	DefaultTrustTier int           `yaml:"default_trust_tier" json:"default_trust_tier"`
}

// ToHeraldConfig converts to herald.Config.
func (c *HeraldConfig) ToHeraldConfig() herald.Config {
	return herald.Config{
		Enabled:          c.Enabled,
		BaseURL:          c.BaseURL,
		Timeout:          c.Timeout,
		DefaultTrustTier: herald.TrustTier(c.DefaultTrustTier),
		RetryAttempts:    3,
		RetryDelay:       500 * time.Millisecond,
	}
}

// FalkorDBConfig holds FalkorDB configuration.
type FalkorDBConfig struct {
	Enabled   bool          `yaml:"enabled" json:"enabled"`
	Address   string        `yaml:"address" json:"address"`
	Password  string        `yaml:"password" json:"password"`
	Database  int           `yaml:"database" json:"database"`
	GraphName string        `yaml:"graph_name" json:"graph_name"`
	Timeout   time.Duration `yaml:"timeout" json:"timeout"`
	CacheTTL  time.Duration `yaml:"cache_ttl" json:"cache_ttl"`
}

// ToFalkorDBConfig converts to falkordb.Config.
func (c *FalkorDBConfig) ToFalkorDBConfig() falkordb.Config {
	return falkordb.Config{
		Enabled:   c.Enabled,
		Address:   c.Address,
		Password:  c.Password,
		Database:  c.Database,
		GraphName: c.GraphName,
		Timeout:   c.Timeout,
		CacheTTL:  c.CacheTTL,
	}
}

// MCPConfig holds MCP server configuration.
type MCPConfig struct {
	AutoConnect    bool          `yaml:"auto_connect" json:"auto_connect"`
	DefaultTimeout time.Duration `yaml:"default_timeout" json:"default_timeout"`
	Servers        []MCPServer   `yaml:"servers" json:"servers"`
}

// MCPServer describes an MCP server.
type MCPServer struct {
	Name    string            `yaml:"name" json:"name"`
	Type    string            `yaml:"type" json:"type"` // stdio, sse, http
	Command string            `yaml:"command" json:"command"`
	Args    []string          `yaml:"args" json:"args"`
	URL     string            `yaml:"url" json:"url"`
	Env     map[string]string `yaml:"env" json:"env"`
}

// ToMCPConfig converts to mcp.Config.
func (c *MCPConfig) ToMCPConfig() mcp.Config {
	var servers []mcp.ServerConfig
	for _, s := range c.Servers {
		servers = append(servers, mcp.ServerConfig{
			Name:    s.Name,
			Type:    s.Type,
			Command: s.Command,
			Args:    s.Args,
			URL:     s.URL,
			Env:     s.Env,
			Timeout: c.DefaultTimeout,
		})
	}
	return mcp.Config{
		Servers:        servers,
		DefaultTimeout: c.DefaultTimeout,
		AutoConnect:    c.AutoConnect,
	}
}

// CheckpointConfig holds checkpoint configuration.
type CheckpointConfig struct {
	Enabled             bool          `yaml:"enabled" json:"enabled"`
	BaseDir             string        `yaml:"base_dir" json:"base_dir"`
	MaxCheckpoints      int           `yaml:"max_checkpoints" json:"max_checkpoints"`
	MaxAge              time.Duration `yaml:"max_age" json:"max_age"`
	MaxTotalSize        int64         `yaml:"max_total_size" json:"max_total_size"`
	AutoBeforeEdit      bool          `yaml:"auto_before_edit" json:"auto_before_edit"`
	AutoBeforeBash      bool          `yaml:"auto_before_bash" json:"auto_before_bash"`
	AutoInterval        time.Duration `yaml:"auto_interval" json:"auto_interval"`
	MinIntervalBetween  time.Duration `yaml:"min_interval_between" json:"min_interval_between"`
}

// ToCheckpointConfigs converts to checkpoint configs.
func (c *CheckpointConfig) ToCheckpointConfigs() (checkpoint.StorageConfig, checkpoint.PolicyConfig) {
	storage := checkpoint.StorageConfig{
		BaseDir:        c.BaseDir,
		MaxCheckpoints: c.MaxCheckpoints,
		MaxAge:         c.MaxAge,
		MaxTotalSize:   c.MaxTotalSize,
	}
	policy := checkpoint.PolicyConfig{
		AutoBeforeEdit:     c.AutoBeforeEdit,
		AutoBeforeBash:     c.AutoBeforeBash,
		AutoInterval:       c.AutoInterval,
		MinIntervalBetween: c.MinIntervalBetween,
	}
	return storage, policy
}

// HooksConfig holds hooks configuration.
type HooksConfig struct {
	Enabled        bool          `yaml:"enabled" json:"enabled"`
	ConfigPath     string        `yaml:"config_path" json:"config_path"`
	GlobalTimeout  time.Duration `yaml:"global_timeout" json:"global_timeout"`
	FailOpen       bool          `yaml:"fail_open" json:"fail_open"`
	EnableBuiltins bool          `yaml:"enable_builtins" json:"enable_builtins"`
}

// ToHooksConfig converts to hooks.Config.
func (c *HooksConfig) ToHooksConfig() hooks.Config {
	return hooks.Config{
		GlobalTimeout:  c.GlobalTimeout,
		FailOpen:       c.FailOpen,
		EnableBuiltins: c.EnableBuiltins,
		ConfigPath:     c.ConfigPath,
	}
}

// SubAgentConfig holds sub-agent configuration.
type SubAgentConfig struct {
	Enabled             bool          `yaml:"enabled" json:"enabled"`
	MaxConcurrent       int           `yaml:"max_concurrent" json:"max_concurrent"`
	DefaultTimeout      time.Duration `yaml:"default_timeout" json:"default_timeout"`
	AutoPromote         bool          `yaml:"auto_promote" json:"auto_promote"`
	PromotionThreshold  int           `yaml:"promotion_threshold" json:"promotion_threshold"`
}

// ToSubAgentConfig converts to subagent.Config.
func (c *SubAgentConfig) ToSubAgentConfig() subagent.Config {
	return subagent.Config{
		MaxConcurrent:  c.MaxConcurrent,
		DefaultTimeout: c.DefaultTimeout,
		AutoPromote:    c.AutoPromote,
		PromotionCriteria: subagent.PromotionCriteria{
			MinSuccessCount:    c.PromotionThreshold,
			NoUserIntervention: true,
			LowRiskOnly:        true,
		},
	}
}

// SystemsConfig holds cross-system configuration.
type SystemsConfig struct {
	Sage  SystemConfig `yaml:"sage" json:"sage"`
	Forge SystemConfig `yaml:"forge" json:"forge"`
	Pali  SystemConfig `yaml:"pali" json:"pali"`
}

// SystemConfig describes a single system.
type SystemConfig struct {
	Host     string `yaml:"host" json:"host"`
	SSHAlias string `yaml:"ssh_alias" json:"ssh_alias"`
	Type     string `yaml:"type" json:"type"` // mac, linux, kali
	Enabled  bool   `yaml:"enabled" json:"enabled"`
}

// DefaultIntegrationsConfig returns default integrations configuration.
func DefaultIntegrationsConfig() IntegrationsConfig {
	return IntegrationsConfig{
		Herald: HeraldConfig{
			Enabled:          true,
			BaseURL:          "http://192.168.1.61:8090",
			Timeout:          30 * time.Second,
			DefaultTrustTier: 1,
		},
		FalkorDB: FalkorDBConfig{
			Enabled:   true,
			Address:   "192.168.1.61:6379",
			GraphName: "agents",
			Timeout:   10 * time.Second,
			CacheTTL:  5 * time.Minute,
		},
		MCP: MCPConfig{
			AutoConnect:    true,
			DefaultTimeout: 30 * time.Second,
			Servers: []MCPServer{
				{
					Name:    "hive-postgres",
					Type:    "stdio",
					Command: "npx",
					Args:    []string{"@anthropic/mcp-postgres"},
				},
				{
					Name:    "hive-falkordb",
					Type:    "stdio",
					Command: "npx",
					Args:    []string{"@anthropic/mcp-falkordb"},
				},
			},
		},
		Checkpoint: CheckpointConfig{
			Enabled:            true,
			BaseDir:            "~/.syntor/checkpoints",
			MaxCheckpoints:     50,
			MaxAge:             7 * 24 * time.Hour,
			MaxTotalSize:       500 * 1024 * 1024,
			AutoBeforeEdit:     true,
			AutoBeforeBash:     true,
			AutoInterval:       5 * time.Minute,
			MinIntervalBetween: 30 * time.Second,
		},
		Hooks: HooksConfig{
			Enabled:        true,
			ConfigPath:     "~/.syntor/hooks.yaml",
			GlobalTimeout:  10 * time.Second,
			FailOpen:       false,
			EnableBuiltins: true,
		},
		SubAgent: SubAgentConfig{
			Enabled:            true,
			MaxConcurrent:      5,
			DefaultTimeout:     10 * time.Minute,
			AutoPromote:        true,
			PromotionThreshold: 5,
		},
		Systems: SystemsConfig{
			Sage: SystemConfig{
				Host:    "localhost",
				Type:    "mac",
				Enabled: true,
			},
			Forge: SystemConfig{
				Host:     "192.168.1.61",
				SSHAlias: "lnx-rig",
				Type:     "linux",
				Enabled:  true,
			},
			Pali: SystemConfig{
				Host:     "10.211.55.9",
				SSHAlias: "pali",
				Type:     "kali",
				Enabled:  true,
			},
		},
	}
}

// ExtendedSyntorConfig is the full configuration including integrations.
type ExtendedSyntorConfig struct {
	SyntorConfig
	Integrations IntegrationsConfig `yaml:"integrations" json:"integrations"`
}

// DefaultExtendedSyntorConfig returns the complete default configuration.
func DefaultExtendedSyntorConfig() ExtendedSyntorConfig {
	return ExtendedSyntorConfig{
		SyntorConfig: DefaultSyntorConfig(),
		Integrations: DefaultIntegrationsConfig(),
	}
}

// ExampleConfigYAML returns an example YAML configuration string.
func ExampleConfigYAML() string {
	return `# SYNTOR Configuration
# ~/.syntor/config.yaml

inference:
  provider: ollama           # LOCAL FIRST - Herald routes to Ollama
  ollama_host: http://192.168.1.61:11434
  default_model: qwen2.5-coder:32b
  auto_pull: true
  models:
    coordination: mistral:7b
    documentation: deepseek-coder-v2:16b
    git: llama3.2:8b
    worker: llama3.2:3b
    worker_code: qwen2.5-coder:7b

cli:
  theme: auto
  stream_response: true
  auto_approve: false

integrations:
  herald:
    enabled: true
    base_url: http://192.168.1.61:8090
    timeout: 30s
    default_trust_tier: 1   # T1 = Read-Only by default

  falkordb:
    enabled: true
    address: 192.168.1.61:6379
    graph_name: agents
    cache_ttl: 5m

  mcp:
    auto_connect: true
    default_timeout: 30s
    servers:
      - name: hive-postgres
        type: stdio
        command: npx
        args: ["@anthropic/mcp-postgres"]
      - name: hive-falkordb
        type: stdio
        command: npx
        args: ["@anthropic/mcp-falkordb"]

  checkpoint:
    enabled: true
    base_dir: ~/.syntor/checkpoints
    max_checkpoints: 50
    max_age: 168h  # 7 days
    auto_before_edit: true
    auto_before_bash: true
    auto_interval: 5m

  hooks:
    enabled: true
    config_path: ~/.syntor/hooks.yaml
    global_timeout: 10s
    fail_open: false
    enable_builtins: true

  subagent:
    enabled: true
    max_concurrent: 5
    default_timeout: 10m
    auto_promote: true
    promotion_threshold: 5

  systems:
    sage:
      host: localhost
      type: mac
      enabled: true
    forge:
      host: 192.168.1.61
      ssh_alias: lnx-rig
      type: linux
      enabled: true
    pali:
      host: 10.211.55.9
      ssh_alias: pali
      type: kali
      enabled: true
`
}
