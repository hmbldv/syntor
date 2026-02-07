package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/syntor/syntor/pkg/vault"
)

// SecretResolver resolves secret references in configuration
type SecretResolver struct {
	manager *vault.Manager
}

// NewSecretResolver creates a resolver from secrets config
func NewSecretResolver(cfg SecretsConfig) (*SecretResolver, error) {
	// Convert config types
	vaultCfg := vault.Config{
		Provider: cfg.Provider,
		CacheTTL: cfg.CacheTTL,
		Vault: vault.VaultConfig{
			Enabled:         cfg.Vault.Enabled,
			Address:         cfg.Vault.Address,
			AuthMethod:      cfg.Vault.AuthMethod,
			MountPath:       cfg.Vault.MountPath,
			PathPrefix:      cfg.Vault.PathPrefix,
			RoleID:          cfg.Vault.RoleID,
			SecretIDPath:    cfg.Vault.SecretIDPath,
			Timeout:         cfg.Vault.Timeout,
			FallbackEnabled: cfg.Vault.FallbackEnabled,
		},
	}

	manager, err := vault.NewManager(vaultCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create secrets manager: %w", err)
	}

	return &SecretResolver{manager: manager}, nil
}

// ResolveConfig resolves all secret references in the configuration
func (r *SecretResolver) ResolveConfig(ctx context.Context, cfg *SyntorConfig) error {
	if r.manager.UsingFallback() && !vault.Quiet {
		slog.Warn("secrets: using environment variable fallback (Vault unavailable)")
	}

	// Resolve AgentDB credentials
	if cfg.Integrations.AgentDB.Enabled {
		if cfg.Integrations.AgentDB.Password == "" {
			password := r.manager.GetWithDefault(ctx, vault.KeyAgentDBPassword, "")
			if password != "" {
				cfg.Integrations.AgentDB.Password = password
				slog.Debug("resolved agentdb password from secrets")
			}
		}
		if cfg.Integrations.AgentDB.User == "" {
			user := r.manager.GetWithDefault(ctx, vault.KeyAgentDBUser, "")
			if user != "" {
				cfg.Integrations.AgentDB.User = user
				slog.Debug("resolved agentdb user from secrets")
			}
		}
	}

	// Resolve FalkorDB password
	if cfg.Integrations.FalkorDB.Enabled {
		if cfg.Integrations.FalkorDB.Password == "" {
			password := r.manager.GetWithDefault(ctx, vault.KeyFalkorDBPassword, "")
			if password != "" {
				cfg.Integrations.FalkorDB.Password = password
				slog.Debug("resolved falkordb password from secrets")
			}
		}
	}

	// Resolve API keys for inference
	if cfg.Inference.AnthropicAPIKey == "" {
		apiKey := r.manager.GetWithDefault(ctx, vault.KeyAnthropicAPIKey, "")
		if apiKey != "" {
			cfg.Inference.AnthropicAPIKey = apiKey
			slog.Debug("resolved anthropic api key from secrets")
		}
	}

	if cfg.Inference.DeepSeekAPIKey == "" {
		apiKey := r.manager.GetWithDefault(ctx, vault.KeyDeepSeekAPIKey, "")
		if apiKey != "" {
			cfg.Inference.DeepSeekAPIKey = apiKey
			slog.Debug("resolved deepseek api key from secrets")
		}
	}

	return nil
}

// SetEnvVars exports secrets to environment variables for external tools
func (r *SecretResolver) SetEnvVars(ctx context.Context) error {
	return vault.SetEnvFromSecrets(r.manager, ctx)
}

// Close cleans up the resolver
func (r *SecretResolver) Close() error {
	return r.manager.Close()
}

// Manager returns the underlying secrets manager
func (r *SecretResolver) Manager() *vault.Manager {
	return r.manager
}

// LoadSyntorConfigWithSecrets loads config and resolves all secrets
func LoadSyntorConfigWithSecrets() (*SyntorConfig, *SecretResolver, error) {
	// Load base config
	cfg, err := LoadSyntorConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Skip secrets resolution if provider is "env" or not configured
	if cfg.Integrations.Secrets.Provider == "" || cfg.Integrations.Secrets.Provider == "env" {
		return cfg, nil, nil
	}

	// Create resolver
	resolver, err := NewSecretResolver(cfg.Integrations.Secrets)
	if err != nil {
		// If fallback is enabled and we got an error, try to continue with env provider
		if cfg.Integrations.Secrets.Vault.FallbackEnabled {
			slog.Warn("secrets: Vault unavailable, using environment variables", "error", err)
			// Create a minimal resolver with env provider
			envCfg := SecretsConfig{Provider: "env", CacheTTL: cfg.Integrations.Secrets.CacheTTL}
			resolver, _ = NewSecretResolver(envCfg)
		} else {
			return nil, nil, fmt.Errorf("failed to create secret resolver: %w", err)
		}
	}

	// Resolve secrets
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := resolver.ResolveConfig(ctx, cfg); err != nil {
		slog.Warn("failed to resolve some secrets", "error", err)
	}

	// Export secrets to env vars for external tools
	if err := resolver.SetEnvVars(ctx); err != nil {
		slog.Warn("failed to export secrets to env vars", "error", err)
	}

	return cfg, resolver, nil
}

// MustLoadSyntorConfigWithSecrets loads config or exits on error
func MustLoadSyntorConfigWithSecrets() (*SyntorConfig, *SecretResolver) {
	cfg, resolver, err := LoadSyntorConfigWithSecrets()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return cfg, resolver
}
