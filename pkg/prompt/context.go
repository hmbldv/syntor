package prompt

import (
	"context"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/syntor/syntor/pkg/manifest"
	"github.com/syntor/syntor/pkg/tools"
)

// ContextGatherer collects context from various sources
type ContextGatherer struct {
	manifestStore *manifest.ManifestStore
	toolRegistry  *tools.Registry
	projectPath   string
	memoryStore   MemoryStore
}

// MemoryStore interface for retrieving stored context
type MemoryStore interface {
	Get(ctx context.Context, key string) (string, error)
	Query(ctx context.Context, prefix string, limit int) ([]MemoryItem, error)
}

// NewContextGatherer creates a new context gatherer
func NewContextGatherer(store *manifest.ManifestStore, projectPath string) *ContextGatherer {
	if projectPath == "" {
		projectPath, _ = os.Getwd()
	}
	return &ContextGatherer{
		manifestStore: store,
		projectPath:   projectPath,
	}
}

// SetMemoryStore sets the memory store for context retrieval
func (g *ContextGatherer) SetMemoryStore(store MemoryStore) {
	g.memoryStore = store
}

// SetToolRegistry sets the tool registry for tool context gathering
func (g *ContextGatherer) SetToolRegistry(registry *tools.Registry) {
	g.toolRegistry = registry
}

// GatherToolContext generates tool descriptions for the system prompt
// If toolNames is empty, returns descriptions for all registered tools
func (g *ContextGatherer) GatherToolContext(toolNames []string) string {
	if g.toolRegistry == nil {
		return ""
	}

	// If no specific tools requested, use all available
	if len(toolNames) == 0 {
		toolNames = g.toolRegistry.List()
	}

	return g.toolRegistry.GenerateToolPrompt(toolNames)
}

// GatherCompactToolContext generates a compact tool list (just names)
func (g *ContextGatherer) GatherCompactToolContext(toolNames []string) string {
	if g.toolRegistry == nil {
		return ""
	}

	if len(toolNames) == 0 {
		toolNames = g.toolRegistry.List()
	}

	return g.toolRegistry.GenerateCompactToolList(toolNames)
}

// ToolContext represents tool information for prompt templates
type ToolContext struct {
	Name        string
	Description string
	Parameters  []ToolParameterContext
	Category    string
}

// ToolParameterContext represents a tool parameter for templates
type ToolParameterContext struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

// GatherToolContextStructured returns structured tool info for templates
func (g *ContextGatherer) GatherToolContextStructured(toolNames []string) []ToolContext {
	if g.toolRegistry == nil {
		return nil
	}

	if len(toolNames) == 0 {
		toolNames = g.toolRegistry.List()
	}

	result := make([]ToolContext, 0, len(toolNames))
	for _, name := range toolNames {
		def, ok := g.toolRegistry.GetDefinition(name)
		if !ok {
			continue
		}

		tc := ToolContext{
			Name:        def.Name,
			Description: def.Description,
			Category:    string(def.Category),
			Parameters:  make([]ToolParameterContext, len(def.Parameters)),
		}

		for i, p := range def.Parameters {
			tc.Parameters[i] = ToolParameterContext{
				Name:        p.Name,
				Type:        p.Type,
				Description: p.Description,
				Required:    p.Required,
			}
		}

		result = append(result, tc)
	}

	return result
}

// GatherAgentContext collects info about all available agents
func (g *ContextGatherer) GatherAgentContext(ctx context.Context) ([]AgentContext, error) {
	manifests := g.manifestStore.ListManifests()
	agents := make([]AgentContext, 0, len(manifests))

	for _, m := range manifests {
		agent := AgentContext{
			Name:         m.Metadata.Name,
			Type:         string(m.Spec.Type),
			Description:  m.Metadata.Description,
			Capabilities: m.GetCapabilityNames(),
			Status:       "healthy", // Default to healthy for local manifests
			Load:         0,
		}
		agents = append(agents, agent)
	}

	return agents, nil
}

// GatherProjectContext loads project-specific context
func (g *ContextGatherer) GatherProjectContext() (*ProjectContext, error) {
	// Try to find project context file
	paths := []string{
		filepath.Join(g.projectPath, ".syntor", "project.yaml"),
		filepath.Join(g.projectPath, ".syntor", "project.yml"),
		filepath.Join(g.projectPath, "syntor.yaml"),
		filepath.Join(g.projectPath, "syntor.yml"),
	}

	var projectFile string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			projectFile = p
			break
		}
	}

	if projectFile == "" {
		// No project context file found, return nil (not an error)
		return nil, nil
	}

	data, err := os.ReadFile(projectFile)
	if err != nil {
		return nil, err
	}

	var projectManifest manifest.ProjectContext
	if err := yaml.Unmarshal(data, &projectManifest); err != nil {
		return nil, err
	}

	return &ProjectContext{
		Name:        projectManifest.Metadata.Name,
		Description: projectManifest.Metadata.Description,
		Path:        g.projectPath,
		Values:      projectManifest.Spec.Values,
		Goals:       projectManifest.Spec.Goals,
		Conventions: projectManifest.Spec.Conventions,
		Context:     projectManifest.Spec.Context,
	}, nil
}

// GatherMemory retrieves relevant memory/context items
func (g *ContextGatherer) GatherMemory(ctx context.Context, query string) ([]MemoryItem, error) {
	if g.memoryStore == nil {
		return nil, nil
	}

	return g.memoryStore.Query(ctx, query, 10)
}

// GatherFullContext gathers all context types
func (g *ContextGatherer) GatherFullContext(ctx context.Context, planMode bool) (*PromptContext, error) {
	promptCtx := &PromptContext{
		PlanMode: planMode,
	}

	// Gather agent context
	agents, err := g.GatherAgentContext(ctx)
	if err == nil {
		promptCtx.Agents = agents
	}

	// Gather project context
	project, err := g.GatherProjectContext()
	if err == nil && project != nil {
		promptCtx.Project = project
	}

	// Gather memory
	memory, err := g.GatherMemory(ctx, "")
	if err == nil {
		promptCtx.Memory = memory
	}

	return promptCtx, nil
}

// SetProjectPath updates the project path for context gathering
func (g *ContextGatherer) SetProjectPath(path string) {
	g.projectPath = path
}

// GetProjectPath returns the current project path
func (g *ContextGatherer) GetProjectPath() string {
	return g.projectPath
}

// ContextStoreAdapter adapts a context.Store to the MemoryStore interface
// This allows the prompt builder to use the context storage system
type ContextStoreAdapter struct {
	store ContextStore
}

// ContextStore is the interface that context.Store implements
// Defined here to avoid circular imports
type ContextStore interface {
	Get(ctx context.Context, key string) (string, error)
	Query(ctx context.Context, prefix string, limit int) ([]ContextItem, error)
}

// ContextItem matches context.Item structure
type ContextItem struct {
	Key       string
	Value     string
	Source    string
	Timestamp int64
}

// NewContextStoreAdapter creates a new adapter
func NewContextStoreAdapter(store ContextStore) *ContextStoreAdapter {
	return &ContextStoreAdapter{store: store}
}

// Get retrieves a single memory item by key
func (a *ContextStoreAdapter) Get(ctx context.Context, key string) (string, error) {
	return a.store.Get(ctx, key)
}

// Query retrieves memory items matching a prefix
func (a *ContextStoreAdapter) Query(ctx context.Context, prefix string, limit int) ([]MemoryItem, error) {
	items, err := a.store.Query(ctx, prefix, limit)
	if err != nil {
		return nil, err
	}

	result := make([]MemoryItem, len(items))
	for i, item := range items {
		result[i] = MemoryItem{
			Key:       item.Key,
			Value:     item.Value,
			Source:    item.Source,
			Timestamp: item.Timestamp,
		}
	}
	return result, nil
}

// InMemoryStore provides a simple in-memory implementation of MemoryStore
// Useful when Redis is not available
type InMemoryStore struct {
	items map[string]MemoryItem
}

// NewInMemoryStore creates a new in-memory store
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		items: make(map[string]MemoryItem),
	}
}

// Get retrieves a memory item by key
func (s *InMemoryStore) Get(ctx context.Context, key string) (string, error) {
	item, ok := s.items[key]
	if !ok {
		return "", nil
	}
	return item.Value, nil
}

// Query retrieves memory items matching a prefix
func (s *InMemoryStore) Query(ctx context.Context, prefix string, limit int) ([]MemoryItem, error) {
	var result []MemoryItem
	for key, item := range s.items {
		if prefix == "" || len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			result = append(result, item)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

// Set stores a memory item
func (s *InMemoryStore) Set(key string, item MemoryItem) {
	s.items[key] = item
}

// Delete removes a memory item
func (s *InMemoryStore) Delete(key string) {
	delete(s.items, key)
}

// Clear removes all memory items
func (s *InMemoryStore) Clear() {
	s.items = make(map[string]MemoryItem)
}
