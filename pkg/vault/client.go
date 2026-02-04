package vault

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strings"

	vault "github.com/hashicorp/vault/api"
)

// VaultProvider reads secrets from HashiCorp Vault KV v2
type VaultProvider struct {
	client     *vault.Client
	mountPath  string
	pathPrefix string
}

// NewVaultProvider creates a new Vault provider
func NewVaultProvider(cfg VaultConfig) (*VaultProvider, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("vault provider disabled")
	}

	// Create Vault client config
	vaultCfg := vault.DefaultConfig()

	// Address from config or environment
	if cfg.Address != "" {
		vaultCfg.Address = cfg.Address
	} else if addr := os.Getenv("VAULT_ADDR"); addr != "" {
		vaultCfg.Address = addr
	} else {
		return nil, fmt.Errorf("vault address not configured")
	}

	// Set timeout
	if cfg.Timeout > 0 {
		vaultCfg.Timeout = cfg.Timeout
	}

	// Create client
	client, err := vault.NewClient(vaultCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	// Authenticate based on method
	switch cfg.AuthMethod {
	case "token", "":
		if err := authenticateToken(client, cfg); err != nil {
			return nil, err
		}
	case "kubernetes":
		if err := authenticateKubernetes(client, cfg); err != nil {
			return nil, err
		}
	case "approle":
		if err := authenticateAppRole(client, cfg); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown vault auth method: %s", cfg.AuthMethod)
	}

	// Set defaults
	mountPath := cfg.MountPath
	if mountPath == "" {
		mountPath = "secret" // Default KV v2 mount
	}

	pathPrefix := cfg.PathPrefix
	if pathPrefix == "" {
		pathPrefix = "syntor" // Default path prefix
	}

	slog.Info("vault provider initialized",
		"address", vaultCfg.Address,
		"mount", mountPath,
		"prefix", pathPrefix,
		"auth_method", cfg.AuthMethod,
	)

	return &VaultProvider{
		client:     client,
		mountPath:  mountPath,
		pathPrefix: pathPrefix,
	}, nil
}

func authenticateToken(client *vault.Client, cfg VaultConfig) error {
	token := cfg.Token
	if token == "" {
		token = os.Getenv("VAULT_TOKEN")
	}
	if token == "" {
		return fmt.Errorf("vault token not configured (set VAULT_TOKEN or config.vault.token)")
	}
	client.SetToken(token)
	return nil
}

func authenticateKubernetes(client *vault.Client, cfg VaultConfig) error {
	// Read service account token
	jwt, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return fmt.Errorf("failed to read service account token: %w", err)
	}

	role := cfg.KubernetesRole
	if role == "" {
		role = "syntor"
	}

	// Login with Kubernetes auth
	resp, err := client.Logical().Write("auth/kubernetes/login", map[string]interface{}{
		"role": role,
		"jwt":  string(jwt),
	})
	if err != nil {
		return fmt.Errorf("kubernetes auth failed: %w", err)
	}

	client.SetToken(resp.Auth.ClientToken)
	slog.Info("vault kubernetes auth successful", "role", role)
	return nil
}

func authenticateAppRole(client *vault.Client, cfg VaultConfig) error {
	roleID := cfg.RoleID
	if roleID == "" {
		roleID = os.Getenv("VAULT_ROLE_ID")
	}

	secretID := cfg.SecretID
	if secretID == "" {
		secretID = os.Getenv("VAULT_SECRET_ID")
	}

	// Try to read secret ID from file if not inline
	if secretID == "" && cfg.SecretIDPath != "" {
		expandedPath := expandPath(cfg.SecretIDPath)
		data, err := os.ReadFile(expandedPath)
		if err != nil {
			return fmt.Errorf("failed to read secret ID from %s: %w", expandedPath, err)
		}
		secretID = strings.TrimSpace(string(data))
	}

	if roleID == "" || secretID == "" {
		return fmt.Errorf("approle role_id and secret_id required")
	}

	resp, err := client.Logical().Write("auth/approle/login", map[string]interface{}{
		"role_id":   roleID,
		"secret_id": secretID,
	})
	if err != nil {
		return fmt.Errorf("approle auth failed: %w", err)
	}

	client.SetToken(resp.Auth.ClientToken)
	slog.Info("vault approle auth successful")
	return nil
}

// Get retrieves a secret from Vault KV v2
func (p *VaultProvider) Get(ctx context.Context, key string) (string, error) {
	// Build the full path: mount/data/prefix
	secretPath := path.Join(p.mountPath, "data", p.pathPrefix)

	secret, err := p.client.Logical().ReadWithContext(ctx, secretPath)
	if err != nil {
		return "", fmt.Errorf("failed to read secret %s: %w", key, err)
	}

	if secret == nil || secret.Data == nil {
		return "", fmt.Errorf("secret %s not found at path %s", key, secretPath)
	}

	// KV v2 wraps data in a "data" key
	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected secret format for %s", key)
	}

	// Map well-known keys to Vault key names
	vaultKey := p.keyToVaultKey(key)

	// Get the value
	if v, ok := data[vaultKey].(string); ok {
		return v, nil
	}

	// Fallback: try the original key
	if v, ok := data[key].(string); ok {
		return v, nil
	}

	return "", fmt.Errorf("key %s not found in secret at %s", key, secretPath)
}

// GetWithDefault retrieves a secret or returns the default
func (p *VaultProvider) GetWithDefault(ctx context.Context, key, defaultValue string) string {
	value, err := p.Get(ctx, key)
	if err != nil || value == "" {
		slog.Debug("using default for secret", "key", key, "error", err)
		return defaultValue
	}
	return value
}

// Name returns the provider name
func (p *VaultProvider) Name() string {
	return "vault"
}

// Close cleans up resources
func (p *VaultProvider) Close() error {
	// Vault client doesn't need explicit cleanup
	return nil
}

// keyToVaultKey maps well-known secret keys to Vault key names
func (p *VaultProvider) keyToVaultKey(key string) string {
	// Map provider.go keys to Vault secret keys
	switch key {
	case KeyAgentDBPassword:
		return "agentdb_password"
	case KeyAgentDBUser:
		return "agentdb_user"
	case KeyFalkorDBPassword:
		return "falkordb_password"
	case KeyAnthropicAPIKey:
		return "anthropic_api_key"
	case KeyDeepSeekAPIKey:
		return "deepseek_api_key"
	case KeyClaudeAPIKey:
		return "claude_api_key"
	case KeyHeraldAPIKey:
		return "herald_api_key"
	default:
		return key
	}
}

// Put stores a secret in Vault (for setup/testing)
func (p *VaultProvider) Put(ctx context.Context, key, value string) error {
	secretPath := path.Join(p.mountPath, "data", p.pathPrefix)
	vaultKey := p.keyToVaultKey(key)

	// Read existing secrets first to preserve other keys
	secret, _ := p.client.Logical().ReadWithContext(ctx, secretPath)
	data := make(map[string]interface{})
	if secret != nil && secret.Data != nil {
		if existingData, ok := secret.Data["data"].(map[string]interface{}); ok {
			for k, v := range existingData {
				data[k] = v
			}
		}
	}

	// Add/update the new key
	data[vaultKey] = value

	_, err := p.client.Logical().WriteWithContext(ctx, secretPath, map[string]interface{}{
		"data": data,
	})
	if err != nil {
		return fmt.Errorf("failed to write secret %s: %w", key, err)
	}

	return nil
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return strings.Replace(path, "~", home, 1)
	}
	return path
}
