package vault

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// EnvProvider reads secrets from environment variables
// Used as fallback when Vault is unavailable
type EnvProvider struct{}

// NewEnvProvider creates a new environment variable provider
func NewEnvProvider() *EnvProvider {
	return &EnvProvider{}
}

// Get retrieves a secret from environment variables
func (p *EnvProvider) Get(ctx context.Context, key string) (string, error) {
	// Map well-known keys to environment variables
	envKey := p.keyToEnv(key)
	value := os.Getenv(envKey)
	if value == "" {
		return "", fmt.Errorf("environment variable %s not set", envKey)
	}
	return value, nil
}

// GetWithDefault retrieves a secret or returns the default
func (p *EnvProvider) GetWithDefault(ctx context.Context, key, defaultValue string) string {
	value, err := p.Get(ctx, key)
	if err != nil || value == "" {
		return defaultValue
	}
	return value
}

// Name returns the provider name
func (p *EnvProvider) Name() string {
	return "env"
}

// Close cleans up resources (no-op for env provider)
func (p *EnvProvider) Close() error {
	return nil
}

// keyToEnv maps well-known keys to standard environment variable names
func (p *EnvProvider) keyToEnv(key string) string {
	switch key {
	case KeyAgentDBPassword:
		return "SYNTOR_DB_PASSWORD"
	case KeyAgentDBUser:
		return "SYNTOR_DB_USER"
	case KeyFalkorDBPassword:
		return "FALKORDB_PASSWORD"
	case KeyAnthropicAPIKey:
		return "ANTHROPIC_API_KEY"
	case KeyDeepSeekAPIKey:
		return "DEEPSEEK_API_KEY"
	case KeyClaudeAPIKey:
		return "ANTHROPIC_API_KEY" // Claude uses Anthropic key
	case KeyHeraldAPIKey:
		return "HERALD_API_KEY"
	default:
		// Default: uppercase with SYNTOR_ prefix
		return "SYNTOR_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
	}
}

// SetEnvFromSecrets exports well-known secrets to environment variables
// This allows external tools to access secrets without Vault integration
func SetEnvFromSecrets(mgr *Manager, ctx context.Context) error {
	keys := []struct {
		secretKey string
		envKey    string
	}{
		{KeyAnthropicAPIKey, "ANTHROPIC_API_KEY"},
		{KeyDeepSeekAPIKey, "DEEPSEEK_API_KEY"},
		{KeyAgentDBUser, "SYNTOR_DB_USER"},
		{KeyAgentDBPassword, "SYNTOR_DB_PASSWORD"},
		{KeyFalkorDBPassword, "FALKORDB_PASSWORD"},
	}

	for _, k := range keys {
		value := mgr.GetWithDefault(ctx, k.secretKey, "")
		if value != "" {
			os.Setenv(k.envKey, value)
		}
	}

	return nil
}
