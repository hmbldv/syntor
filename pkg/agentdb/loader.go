package agentdb

import (
	"context"
	"fmt"
	"sync"

	"github.com/syntor/syntor/pkg/falkordb"
	"github.com/syntor/syntor/pkg/manifest"
	"github.com/syntor/syntor/pkg/prompt"
)

// Quiet suppresses non-error output when true (e.g., in non-verbose CLI mode).
var Quiet bool

// UnifiedLoader combines FalkorDB routing with PostgreSQL rich definitions
// and falls back to manifest-based definitions when database is unavailable
type UnifiedLoader struct {
	// Database clients
	agentDB   *Client
	falkorDB  *falkordb.Client

	// Fallback manifest store
	manifestStore *manifest.ManifestStore

	// Prompt builder for Self context generation
	promptBuilder *prompt.Builder

	// Configuration
	preferDatabase bool // Prefer database over manifests when both available

	mu sync.RWMutex
}

// UnifiedLoaderConfig configures the unified loader
type UnifiedLoaderConfig struct {
	AgentDBConfig   *Config
	FalkorDBConfig  *falkordb.Config
	ManifestPaths   []string
	PreferDatabase  bool
}

// NewUnifiedLoader creates a new unified loader
func NewUnifiedLoader(cfg UnifiedLoaderConfig) (*UnifiedLoader, error) {
	loader := &UnifiedLoader{
		preferDatabase: cfg.PreferDatabase,
	}

	// Try to connect to AgentDB (PostgreSQL)
	if cfg.AgentDBConfig != nil {
		client, err := NewClient(*cfg.AgentDBConfig)
		if err != nil {
			// Non-fatal, continue without database
			if !Quiet {
				fmt.Printf("AgentDB unavailable: %v\n", err)
			}
		} else {
			loader.agentDB = client
		}
	}

	// Try to connect to FalkorDB for routing
	if cfg.FalkorDBConfig != nil {
		client, err := falkordb.New(*cfg.FalkorDBConfig)
		if err != nil {
			// Non-fatal, continue without FalkorDB
			if !Quiet {
				fmt.Printf("FalkorDB unavailable: %v\n", err)
			}
		} else {
			loader.falkorDB = client
		}
	}

	// Initialize manifest store as fallback
	if len(cfg.ManifestPaths) > 0 {
		store, err := manifest.NewManifestStore(cfg.ManifestPaths)
		if err != nil {
			// Non-fatal, continue without manifests
			if !Quiet {
				fmt.Printf("Manifest store unavailable: %v\n", err)
			}
		} else {
			loader.manifestStore = store
		}
	}

	return loader, nil
}

// Close closes all database connections
func (l *UnifiedLoader) Close() error {
	var errs []error

	if l.agentDB != nil {
		if err := l.agentDB.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if l.falkorDB != nil {
		if err := l.falkorDB.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing connections: %v", errs)
	}
	return nil
}

// RouteAndLoad routes a task type to an agent and loads its full definition
func (l *UnifiedLoader) RouteAndLoad(ctx context.Context, taskType string) (*LoadedAgent, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Step 1: Route task type to agent using FalkorDB
	var agentID string
	var routeInfo *RouteInfo

	if l.falkorDB != nil {
		result, err := l.falkorDB.RouteTask(ctx, falkordb.RouteQuery{TaskType: taskType})
		if err == nil {
			agentID = result.Agent.Name
			teamName := ""
			if result.Team != nil {
				teamName = result.Team.Name
			}
			routeInfo = &RouteInfo{
				Agent: result.Agent.Name,
				Role:  result.Agent.Role,
				Team:  teamName,
				Chain: result.Route.Chain,
			}
		}
	}

	// Fallback: Use static routing if FalkorDB unavailable
	if agentID == "" {
		if fallback, ok := falkordb.FallbackRoutes[taskType]; ok {
			agentID = fallback
			routeInfo = &RouteInfo{Agent: fallback}
		} else {
			agentID = "sntr" // Default to SNTR
			routeInfo = &RouteInfo{Agent: "sntr"}
		}
	}

	// Step 2: Load rich definition from PostgreSQL
	var richDef *RichAgentDefinition
	if l.agentDB != nil && l.preferDatabase {
		def, err := l.agentDB.GetDefinition(ctx, agentID)
		if err == nil {
			richDef = def
		}
	}

	// Step 3: Fall back to manifest if no database definition
	var manifestDef *manifest.AgentManifest
	if richDef == nil && l.manifestStore != nil {
		if m, ok := l.manifestStore.GetManifest(agentID); ok {
			manifestDef = m
		}
	}

	// Build loaded agent
	loaded := &LoadedAgent{
		AgentID:   agentID,
		RouteInfo: routeInfo,
	}

	// Populate from rich definition if available
	if richDef != nil {
		loaded.SystemPrompt = richDef.SystemPrompt
		loaded.Personality = richDef.Personality
		loaded.Behavior = richDef.BehavioralRules
		loaded.Interactions = richDef.InteractionProtocols
		loaded.ModelConfig = richDef.ModelConfig
		loaded.Source = "database"
		loaded.Version = richDef.Version
	} else if manifestDef != nil {
		// Convert manifest to loaded agent
		loaded.SystemPrompt = manifestDef.Spec.Prompt.System
		loaded.Source = "manifest"
		loaded.Version = 1

		// Convert identity/voice/behavior if present
		if manifestDef.Spec.Identity != nil || manifestDef.Spec.Voice != nil {
			loaded.Personality = &Personality{}
			if manifestDef.Spec.Voice != nil {
				loaded.Personality.Tone = manifestDef.Spec.Voice.Tone
				loaded.Personality.Style = manifestDef.Spec.Voice.Style
				loaded.Personality.Demeanor = manifestDef.Spec.Voice.Demeanor
				loaded.Personality.Phrases = manifestDef.Spec.Voice.Phrases
				loaded.Personality.Avoid = manifestDef.Spec.Voice.Avoid
			}
		}

		if manifestDef.Spec.Behavior != nil {
			loaded.Behavior = &BehavioralRules{
				Guidelines:         manifestDef.Spec.Behavior.Guidelines,
				EscalationTriggers: manifestDef.Spec.Behavior.Escalate,
			}
		}
	} else {
		return nil, fmt.Errorf("no definition found for agent: %s", agentID)
	}

	return loaded, nil
}

// LoadAgent loads a specific agent by ID
func (l *UnifiedLoader) LoadAgent(ctx context.Context, agentID string) (*LoadedAgent, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	loaded := &LoadedAgent{
		AgentID: agentID,
	}

	// Try database first if preferred
	if l.agentDB != nil && l.preferDatabase {
		def, err := l.agentDB.GetDefinition(ctx, agentID)
		if err == nil {
			loaded.SystemPrompt = def.SystemPrompt
			loaded.Personality = def.Personality
			loaded.Behavior = def.BehavioralRules
			loaded.Interactions = def.InteractionProtocols
			loaded.ModelConfig = def.ModelConfig
			loaded.Source = "database"
			loaded.Version = def.Version
			return loaded, nil
		}
	}

	// Fall back to manifest
	if l.manifestStore != nil {
		if m, ok := l.manifestStore.GetManifest(agentID); ok {
			loaded.SystemPrompt = m.Spec.Prompt.System
			loaded.Source = "manifest"
			loaded.Version = 1

			if m.Spec.Voice != nil {
				loaded.Personality = &Personality{
					Tone:     m.Spec.Voice.Tone,
					Style:    m.Spec.Voice.Style,
					Demeanor: m.Spec.Voice.Demeanor,
					Phrases:  m.Spec.Voice.Phrases,
					Avoid:    m.Spec.Voice.Avoid,
				}
			}

			if m.Spec.Behavior != nil {
				loaded.Behavior = &BehavioralRules{
					Guidelines:         m.Spec.Behavior.Guidelines,
					EscalationTriggers: m.Spec.Behavior.Escalate,
				}
			}

			return loaded, nil
		}
	}

	return nil, fmt.Errorf("agent not found: %s", agentID)
}

// ListAgents returns all available agents
func (l *UnifiedLoader) ListAgents(ctx context.Context) ([]*AgentSummary, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var summaries []*AgentSummary

	// Get from database if available
	if l.agentDB != nil {
		dbSummaries, err := l.agentDB.ListSummaries(ctx, QueryOptions{})
		if err == nil {
			summaries = append(summaries, dbSummaries...)
		}
	}

	// Add manifest agents not in database
	if l.manifestStore != nil {
		dbAgentIDs := make(map[string]bool)
		for _, s := range summaries {
			dbAgentIDs[s.AgentID] = true
		}

		for _, m := range l.manifestStore.ListManifests() {
			if !dbAgentIDs[m.Metadata.Name] {
				summaries = append(summaries, &AgentSummary{
					AgentID:      m.Metadata.Name,
					Name:         m.Metadata.Name,
					Role:         string(m.Spec.Type),
					Capabilities: m.GetCapabilityNames(),
					Version:      1,
				})
			}
		}
	}

	return summaries, nil
}

// GetSelfContext generates SelfAgentContext for prompt building
func (l *UnifiedLoader) GetSelfContext(ctx context.Context, agentID string) (*prompt.SelfAgentContext, error) {
	loaded, err := l.LoadAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}

	self := &prompt.SelfAgentContext{
		Name: agentID,
	}

	// Populate from loaded agent
	if loaded.Personality != nil {
		self.Tone = loaded.Personality.Tone
		self.Style = loaded.Personality.Style
		self.Demeanor = loaded.Personality.Demeanor
		self.Phrases = loaded.Personality.Phrases
		self.Avoid = loaded.Personality.Avoid
	}

	if loaded.Behavior != nil {
		self.Guidelines = loaded.Behavior.Guidelines
		self.Escalate = loaded.Behavior.EscalationTriggers
	}

	if loaded.Interactions != nil {
		self.Collaborate = make(map[string]string)
		for agent, rule := range loaded.Interactions.Collaborate {
			self.Collaborate[agent] = rule.Notes
		}
	}

	return self, nil
}

// GetModelForAgent returns the model assigned to an agent
// Returns empty string if no specific model is configured
func (l *UnifiedLoader) GetModelForAgent(ctx context.Context, agentID string) (string, error) {
	loaded, err := l.LoadAgent(ctx, agentID)
	if err != nil {
		return "", err
	}
	return loaded.GetModel(), nil
}

// GetAgentConfig returns model and system prompt for an agent
func (l *UnifiedLoader) GetAgentConfig(ctx context.Context, agentID string) (model string, systemPrompt string, err error) {
	loaded, err := l.LoadAgent(ctx, agentID)
	if err != nil {
		return "", "", err
	}
	return loaded.GetModel(), loaded.SystemPrompt, nil
}

// IsAgentDBAvailable returns true if AgentDB is connected
func (l *UnifiedLoader) IsAgentDBAvailable() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.agentDB != nil
}

// IsFalkorDBAvailable returns true if FalkorDB is connected
func (l *UnifiedLoader) IsFalkorDBAvailable() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.falkorDB != nil
}

// LoadedAgent represents an agent loaded from any source
type LoadedAgent struct {
	AgentID      string
	SystemPrompt string
	Personality  *Personality
	Behavior     *BehavioralRules
	Interactions *InteractionProtocols
	ModelConfig  *ModelConfig
	RouteInfo    *RouteInfo
	Source       string // "database" or "manifest"
	Version      int
}

// RouteInfo contains routing information from FalkorDB
type RouteInfo struct {
	Agent string
	Role  string
	Team  string
	Chain []string
}

// GetTone returns the personality tone or default
func (a *LoadedAgent) GetTone() string {
	if a.Personality != nil && a.Personality.Tone != "" {
		return a.Personality.Tone
	}
	return "helpful and professional"
}

// GetStyle returns the personality style or default
func (a *LoadedAgent) GetStyle() string {
	if a.Personality != nil && a.Personality.Style != "" {
		return a.Personality.Style
	}
	return "clear and concise"
}

// GetModel returns the configured model for this agent
func (a *LoadedAgent) GetModel() string {
	if a.ModelConfig != nil && a.ModelConfig.DefaultModel != "" {
		return a.ModelConfig.DefaultModel
	}
	return "" // Caller should use fallback
}

// GetModelFallbacks returns fallback models for this agent
func (a *LoadedAgent) GetModelFallbacks() []string {
	if a.ModelConfig != nil {
		return a.ModelConfig.Fallbacks
	}
	return nil
}

