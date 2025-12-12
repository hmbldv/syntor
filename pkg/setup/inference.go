package setup

import (
	"context"
	"fmt"

	"github.com/syntor/syntor/pkg/config"
	"github.com/syntor/syntor/pkg/inference"
	"github.com/syntor/syntor/pkg/inference/anthropic"
	"github.com/syntor/syntor/pkg/inference/deepseek"
	"github.com/syntor/syntor/pkg/inference/ollama"
)

// InitializeInference creates and configures an inference registry based on config
func InitializeInference(cfg *config.InferenceConfig) (*inference.Registry, error) {
	registry := inference.NewRegistry()

	// Register Ollama provider (always available for local inference)
	ollamaClient := ollama.NewClient(ollama.ClientConfig{
		BaseURL: cfg.OllamaHost,
	})
	registry.RegisterProvider(ollamaClient)

	// Register Anthropic provider (stub for now)
	anthropicClient := anthropic.NewClient(anthropic.ClientConfig{
		APIKey: cfg.AnthropicAPIKey,
	})
	registry.RegisterProvider(anthropicClient)

	// Register DeepSeek provider (stub for now)
	deepseekClient := deepseek.NewClient(deepseek.ClientConfig{
		APIKey: cfg.DeepSeekAPIKey,
	})
	registry.RegisterProvider(deepseekClient)

	// Set default provider
	if err := registry.SetDefaultProvider(cfg.Provider); err != nil {
		// Fall back to ollama if specified provider isn't available
		if err := registry.SetDefaultProvider("ollama"); err != nil {
			return nil, fmt.Errorf("failed to set default provider: %w", err)
		}
	}

	// Set default model
	registry.SetDefaultModel(cfg.DefaultModel)

	// Apply model assignments from config
	if cfg.Models.Coordination != "" {
		registry.SetModelForAgent(inference.AgentCoordination, cfg.Models.Coordination)
	}
	if cfg.Models.Documentation != "" {
		registry.SetModelForAgent(inference.AgentDocumentation, cfg.Models.Documentation)
	}
	if cfg.Models.Git != "" {
		registry.SetModelForAgent(inference.AgentGit, cfg.Models.Git)
	}
	if cfg.Models.Worker != "" {
		registry.SetModelForAgent(inference.AgentWorker, cfg.Models.Worker)
	}
	if cfg.Models.WorkerCode != "" {
		registry.SetModelForAgent(inference.AgentWorkerCode, cfg.Models.WorkerCode)
	}

	return registry, nil
}

// EnsureModels checks and pulls all assigned models
func EnsureModels(ctx context.Context, registry *inference.Registry, progress func(model string, p inference.PullProgress)) error {
	return registry.EnsureModelsAvailable(ctx, progress)
}

// QuickCheck performs a quick health check on the default provider
func QuickCheck(ctx context.Context, registry *inference.Registry) error {
	provider, ok := registry.GetDefaultProvider()
	if !ok {
		return fmt.Errorf("no default provider configured")
	}

	if !provider.IsAvailable(ctx) {
		return fmt.Errorf("default provider %s is not available", provider.Name())
	}

	return nil
}

// GetProviderForAgent returns the appropriate provider for an agent type
func GetProviderForAgent(registry *inference.Registry, agentType inference.AgentType) (inference.Provider, string, error) {
	modelID := registry.GetModelForAgent(agentType)
	providerName, found := registry.GetProviderForModel(modelID)
	if !found {
		// Fall back to default provider
		provider, ok := registry.GetDefaultProvider()
		if !ok {
			return nil, "", fmt.Errorf("no provider found for model %s", modelID)
		}
		return provider, modelID, nil
	}

	provider, ok := registry.GetProvider(providerName)
	if !ok {
		return nil, "", fmt.Errorf("provider %s not registered", providerName)
	}

	return provider, modelID, nil
}
