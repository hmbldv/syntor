// Package vault provides secrets management via HashiCorp Vault
// This eliminates plain text passwords and API keys from config files
package vault

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// Provider defines the interface for secrets backends
type Provider interface {
	// Get retrieves a secret value by key
	Get(ctx context.Context, key string) (string, error)

	// GetWithDefault retrieves a secret or returns the default
	GetWithDefault(ctx context.Context, key, defaultValue string) string

	// Name returns the provider name for logging
	Name() string

	// Close cleans up any resources
	Close() error
}

// Config holds secrets provider configuration
type Config struct {
	// Provider type: "env", "vault"
	Provider string `yaml:"provider" json:"provider"`

	// Vault configuration (if provider == "vault")
	Vault VaultConfig `yaml:"vault" json:"vault"`

	// Cache TTL for secrets (0 = no cache)
	CacheTTL time.Duration `yaml:"cache_ttl" json:"cache_ttl"`
}

// VaultConfig holds Vault-specific configuration
type VaultConfig struct {
	// Enabled controls whether Vault is used
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Address of Vault server
	Address string `yaml:"address" json:"address"`

	// Auth method: "token", "kubernetes", "approle"
	AuthMethod string `yaml:"auth_method" json:"auth_method"`

	// Token for token auth (can come from VAULT_TOKEN env)
	Token string `yaml:"token" json:"token"`

	// Mount path for KV v2 engine
	MountPath string `yaml:"mount_path" json:"mount_path"`

	// Path prefix for SYNTOR secrets
	PathPrefix string `yaml:"path_prefix" json:"path_prefix"`

	// Kubernetes auth config
	KubernetesRole string `yaml:"kubernetes_role" json:"kubernetes_role"`

	// AppRole auth config
	RoleID   string `yaml:"role_id" json:"role_id"`
	SecretID string `yaml:"secret_id" json:"secret_id"`

	// SecretIDPath is a file containing the secret ID (more secure than inline)
	SecretIDPath string `yaml:"secret_id_path" json:"secret_id_path"`

	// Timeout for Vault operations
	Timeout time.Duration `yaml:"timeout" json:"timeout"`

	// FallbackEnabled allows falling back to env vars if Vault unavailable
	FallbackEnabled bool `yaml:"fallback_enabled" json:"fallback_enabled"`
}

// Manager manages secret providers with caching
type Manager struct {
	provider  Provider
	fallback  Provider // EnvProvider as fallback
	cache     map[string]cacheEntry
	cacheTTL  time.Duration
	mu        sync.RWMutex
	useFallback bool
}

type cacheEntry struct {
	value     string
	expiresAt time.Time
}

// NewManager creates a new secrets manager based on config
func NewManager(cfg Config) (*Manager, error) {
	var provider Provider
	var err error

	// Always create env provider as fallback
	fallback := NewEnvProvider()

	switch cfg.Provider {
	case "env", "":
		provider = fallback
	case "vault":
		provider, err = NewVaultProvider(cfg.Vault)
		if err != nil {
			if cfg.Vault.FallbackEnabled {
				// Log warning but continue with env fallback
				fmt.Fprintf(os.Stderr, "Warning: Vault unavailable, using environment variables: %v\n", err)
				return &Manager{
					provider:    fallback,
					fallback:    fallback,
					cache:       make(map[string]cacheEntry),
					cacheTTL:    cfg.CacheTTL,
					useFallback: true,
				}, nil
			}
			return nil, fmt.Errorf("failed to create vault provider: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown secrets provider: %s", cfg.Provider)
	}

	return &Manager{
		provider: provider,
		fallback: fallback,
		cache:    make(map[string]cacheEntry),
		cacheTTL: cfg.CacheTTL,
	}, nil
}

// Get retrieves a secret, using cache if available
func (m *Manager) Get(ctx context.Context, key string) (string, error) {
	// Check cache first
	if m.cacheTTL > 0 {
		m.mu.RLock()
		if entry, ok := m.cache[key]; ok && time.Now().Before(entry.expiresAt) {
			m.mu.RUnlock()
			return entry.value, nil
		}
		m.mu.RUnlock()
	}

	// Fetch from provider
	value, err := m.provider.Get(ctx, key)
	if err != nil {
		// Try fallback if enabled and primary failed
		if m.fallback != nil && m.provider != m.fallback {
			if fallbackValue, fallbackErr := m.fallback.Get(ctx, key); fallbackErr == nil {
				value = fallbackValue
				err = nil
			}
		}
		if err != nil {
			return "", err
		}
	}

	// Update cache
	if m.cacheTTL > 0 {
		m.mu.Lock()
		m.cache[key] = cacheEntry{
			value:     value,
			expiresAt: time.Now().Add(m.cacheTTL),
		}
		m.mu.Unlock()
	}

	return value, nil
}

// GetWithDefault retrieves a secret or returns the default
func (m *Manager) GetWithDefault(ctx context.Context, key, defaultValue string) string {
	value, err := m.Get(ctx, key)
	if err != nil || value == "" {
		return defaultValue
	}
	return value
}

// MustGet retrieves a secret and panics if not found
func (m *Manager) MustGet(ctx context.Context, key string) string {
	value, err := m.Get(ctx, key)
	if err != nil {
		panic(fmt.Sprintf("required secret %s not found: %v", key, err))
	}
	if value == "" {
		panic(fmt.Sprintf("required secret %s is empty", key))
	}
	return value
}

// Provider returns the underlying provider name
func (m *Manager) ProviderName() string {
	return m.provider.Name()
}

// UsingFallback returns true if currently using fallback provider
func (m *Manager) UsingFallback() bool {
	return m.useFallback
}

// Close cleans up resources
func (m *Manager) Close() error {
	return m.provider.Close()
}

// ClearCache clears the secrets cache
func (m *Manager) ClearCache() {
	m.mu.Lock()
	m.cache = make(map[string]cacheEntry)
	m.mu.Unlock()
}

// Well-known secret keys for SYNTOR
const (
	// Database credentials
	KeyAgentDBPassword  = "agentdb_password"
	KeyAgentDBUser      = "agentdb_user"
	KeyFalkorDBPassword = "falkordb_password"

	// API keys
	KeyAnthropicAPIKey = "anthropic_api_key"
	KeyDeepSeekAPIKey  = "deepseek_api_key"
	KeyClaudeAPIKey    = "claude_api_key"

	// Service credentials
	KeyHeraldAPIKey = "herald_api_key"
)

// DefaultConfig returns a default secrets configuration
func DefaultConfig() Config {
	return Config{
		Provider: "vault",
		CacheTTL: 5 * time.Minute,
		Vault: VaultConfig{
			Enabled:         true,
			Address:         "http://192.168.1.61:8200",
			AuthMethod:      "token",
			MountPath:       "secret",
			PathPrefix:      "syntor",
			Timeout:         10 * time.Second,
			FallbackEnabled: true,
		},
	}
}
