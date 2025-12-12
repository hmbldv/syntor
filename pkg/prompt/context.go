package prompt

import (
	"context"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/syntor/syntor/pkg/manifest"
)

// ContextGatherer collects context from various sources
type ContextGatherer struct {
	manifestStore *manifest.ManifestStore
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
