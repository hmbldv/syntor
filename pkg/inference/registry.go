package inference

import (
	"context"
	"fmt"
	"sync"
)

// AgentType represents the type of agent for model assignment
type AgentType string

const (
	AgentSNTR          AgentType = "sntr"          // Primary orchestration agent (renamed from coordination)
	AgentCoordination  AgentType = "sntr"          // Alias for backwards compatibility
	AgentDocumentation AgentType = "documentation"
	AgentGit           AgentType = "git"
	AgentWorker        AgentType = "worker"
	AgentWorkerCode    AgentType = "worker_code"   // Worker for code-specific tasks
)

// AvailableModels defines all models that SYNTOR supports
var AvailableModels = []Model{
	// Local models (Ollama)
	{
		ID:          "llama3.2:3b",
		Name:        "Llama 3.2 3B",
		Provider:    "ollama",
		Description: "Fast, efficient model for simple tasks",
		Parameters:  "3B",
		Context:     8192,
		Capabilities: []string{"chat", "completion"},
		Tags:        []string{"fast", "efficient", "local"},
	},
	{
		ID:          "llama3.2:8b",
		Name:        "Llama 3.2 8B",
		Provider:    "ollama",
		Description: "Balanced performance for general tasks",
		Parameters:  "8B",
		Context:     8192,
		Capabilities: []string{"chat", "completion"},
		Tags:        []string{"balanced", "general", "local"},
	},
	{
		ID:          "mistral:7b",
		Name:        "Mistral 7B",
		Provider:    "ollama",
		Description: "Excellent reasoning and orchestration capabilities",
		Parameters:  "7B",
		Context:     32768,
		Capabilities: []string{"chat", "completion", "reasoning"},
		Tags:        []string{"reasoning", "orchestration", "local"},
	},
	{
		ID:          "qwen2.5-coder:7b",
		Name:        "Qwen 2.5 Coder 7B",
		Provider:    "ollama",
		Description: "Specialized for code generation and understanding",
		Parameters:  "7B",
		Context:     32768,
		Capabilities: []string{"chat", "completion", "code"},
		Tags:        []string{"code", "programming", "local"},
	},
	{
		ID:          "deepseek-coder-v2:16b",
		Name:        "DeepSeek Coder V2 16B",
		Provider:    "ollama",
		Description: "Advanced code analysis and documentation",
		Parameters:  "16B",
		Context:     65536,
		Capabilities: []string{"chat", "completion", "code", "analysis"},
		Tags:        []string{"code", "analysis", "documentation", "local"},
	},
	{
		ID:          "codellama:13b",
		Name:        "Code Llama 13B",
		Provider:    "ollama",
		Description: "Meta's code-specialized model",
		Parameters:  "13B",
		Context:     16384,
		Capabilities: []string{"chat", "completion", "code"},
		Tags:        []string{"code", "meta", "local"},
	},

	// API models (Claude) - for future use
	{
		ID:          "claude-sonnet-4-20250514",
		Name:        "Claude Sonnet 4",
		Provider:    "anthropic",
		Description: "Balanced performance and cost for complex tasks",
		Context:     200000,
		Capabilities: []string{"chat", "completion", "reasoning", "code", "analysis"},
		Tags:        []string{"api", "anthropic", "balanced"},
	},
	{
		ID:          "claude-haiku-4-20250514",
		Name:        "Claude Haiku 4",
		Provider:    "anthropic",
		Description: "Fast and cost-effective for simple tasks",
		Context:     200000,
		Capabilities: []string{"chat", "completion"},
		Tags:        []string{"api", "anthropic", "fast"},
	},
	{
		ID:          "claude-opus-4-20250514",
		Name:        "Claude Opus 4",
		Provider:    "anthropic",
		Description: "Most capable model for complex reasoning",
		Context:     200000,
		Capabilities: []string{"chat", "completion", "reasoning", "code", "analysis"},
		Tags:        []string{"api", "anthropic", "powerful"},
	},

	// API models (DeepSeek) - for future use
	{
		ID:          "deepseek-chat",
		Name:        "DeepSeek Chat",
		Provider:    "deepseek",
		Description: "Cost-effective general purpose model",
		Context:     65536,
		Capabilities: []string{"chat", "completion"},
		Tags:        []string{"api", "deepseek", "cost-effective"},
	},
	{
		ID:          "deepseek-coder",
		Name:        "DeepSeek Coder",
		Provider:    "deepseek",
		Description: "Specialized for code tasks via API",
		Context:     65536,
		Capabilities: []string{"chat", "completion", "code"},
		Tags:        []string{"api", "deepseek", "code"},
	},
}

// DefaultModelAssignments defines the default model for each agent type
var DefaultModelAssignments = map[AgentType]string{
	AgentSNTR:          "mistral:7b",
	AgentDocumentation: "deepseek-coder-v2:16b",
	AgentGit:           "llama3.2:8b",
	AgentWorker:        "llama3.2:3b",
	AgentWorkerCode:    "qwen2.5-coder:7b",
}

// Registry manages inference providers and model assignments
type Registry struct {
	providers        map[string]Provider
	modelAssignments map[AgentType]string
	defaultProvider  string
	defaultModel     string
	mu               sync.RWMutex
}

// NewRegistry creates a new inference registry
func NewRegistry() *Registry {
	return &Registry{
		providers:        make(map[string]Provider),
		modelAssignments: copyAssignments(DefaultModelAssignments),
		defaultProvider:  "ollama",
		defaultModel:     "llama3.2:8b",
	}
}

func copyAssignments(src map[AgentType]string) map[AgentType]string {
	dst := make(map[AgentType]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// RegisterProvider adds a provider to the registry
func (r *Registry) RegisterProvider(provider Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[provider.Name()] = provider
}

// GetProvider returns a provider by name
func (r *Registry) GetProvider(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// GetDefaultProvider returns the default provider
func (r *Registry) GetDefaultProvider() (Provider, bool) {
	return r.GetProvider(r.defaultProvider)
}

// SetDefaultProvider sets the default provider
func (r *Registry) SetDefaultProvider(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.providers[name]; !ok {
		return fmt.Errorf("provider not registered: %s", name)
	}
	r.defaultProvider = name
	return nil
}

// SetDefaultModel sets the default model for all agents
func (r *Registry) SetDefaultModel(modelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultModel = modelID
}

// GetDefaultModel returns the default model
func (r *Registry) GetDefaultModel() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaultModel
}

// SetModelForAgent assigns a model to a specific agent type
func (r *Registry) SetModelForAgent(agent AgentType, modelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelAssignments[agent] = modelID
}

// GetModelForAgent returns the assigned model for an agent type
func (r *Registry) GetModelForAgent(agent AgentType) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if model, ok := r.modelAssignments[agent]; ok {
		return model
	}
	return r.defaultModel
}

// GetAllAssignments returns all model assignments
func (r *Registry) GetAllAssignments() map[AgentType]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return copyAssignments(r.modelAssignments)
}

// ResetAssignments resets model assignments to defaults
func (r *Registry) ResetAssignments() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modelAssignments = copyAssignments(DefaultModelAssignments)
}

// GetAvailableModels returns all available models
func (r *Registry) GetAvailableModels() []Model {
	return AvailableModels
}

// GetModelsByProvider returns models for a specific provider
func (r *Registry) GetModelsByProvider(providerName string) []Model {
	var models []Model
	for _, m := range AvailableModels {
		if m.Provider == providerName {
			models = append(models, m)
		}
	}
	return models
}

// GetLocalModels returns models that run locally (Ollama)
func (r *Registry) GetLocalModels() []Model {
	return r.GetModelsByProvider("ollama")
}

// GetAPIModels returns models that require API access
func (r *Registry) GetAPIModels() []Model {
	var models []Model
	for _, m := range AvailableModels {
		if m.Provider != "ollama" {
			models = append(models, m)
		}
	}
	return models
}

// FindModel finds a model by ID
func (r *Registry) FindModel(modelID string) (Model, bool) {
	for _, m := range AvailableModels {
		if m.ID == modelID {
			return m, true
		}
	}
	return Model{}, false
}

// GetProviderForModel returns the provider name for a model
func (r *Registry) GetProviderForModel(modelID string) (string, bool) {
	model, found := r.FindModel(modelID)
	if !found {
		return "", false
	}
	return model.Provider, true
}

// ListProviders returns all registered provider names
func (r *Registry) ListProviders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// CheckProviderHealth checks if all registered providers are available
func (r *Registry) CheckProviderHealth(ctx context.Context) map[string]bool {
	r.mu.RLock()
	providers := make(map[string]Provider, len(r.providers))
	for k, v := range r.providers {
		providers[k] = v
	}
	r.mu.RUnlock()

	health := make(map[string]bool, len(providers))
	for name, provider := range providers {
		health[name] = provider.IsAvailable(ctx)
	}
	return health
}

// EnsureModelsAvailable checks if assigned models are available and pulls if needed
func (r *Registry) EnsureModelsAvailable(ctx context.Context, progress func(model string, p PullProgress)) error {
	r.mu.RLock()
	assignments := copyAssignments(r.modelAssignments)
	r.mu.RUnlock()

	// Collect unique models
	models := make(map[string]bool)
	for _, modelID := range assignments {
		models[modelID] = true
	}

	// Check and pull each model
	for modelID := range models {
		providerName, found := r.GetProviderForModel(modelID)
		if !found {
			continue
		}

		provider, ok := r.GetProvider(providerName)
		if !ok {
			continue
		}

		// Check if model exists
		hasModel, err := provider.HasModel(ctx, modelID)
		if err != nil {
			return fmt.Errorf("failed to check model %s: %w", modelID, err)
		}

		if !hasModel {
			// Pull the model
			progressFunc := func(p PullProgress) {
				if progress != nil {
					progress(modelID, p)
				}
			}
			if err := provider.PullModel(ctx, modelID, progressFunc); err != nil {
				return fmt.Errorf("failed to pull model %s: %w", modelID, err)
			}
		}
	}

	return nil
}

// GetModelsByProvider is a package-level helper that returns models for a provider
// This is used by provider stubs to return their available models
func GetModelsByProvider(providerName string) []Model {
	var models []Model
	for _, m := range AvailableModels {
		if m.Provider == providerName {
			models = append(models, m)
		}
	}
	return models
}

// GetAssignedModels returns the list of models currently assigned to agents
func (r *Registry) GetAssignedModels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	models := make(map[string]bool)
	for _, modelID := range r.modelAssignments {
		models[modelID] = true
	}

	result := make([]string, 0, len(models))
	for modelID := range models {
		result = append(result, modelID)
	}
	return result
}
