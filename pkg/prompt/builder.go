package prompt

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/syntor/syntor/pkg/manifest"
)

// Builder constructs system prompts dynamically from agent manifests
type Builder struct {
	manifestStore *manifest.ManifestStore
	gatherer      *ContextGatherer
	funcMap       template.FuncMap
}

// BuildOptions configures prompt building
type BuildOptions struct {
	IncludeAgents  bool
	IncludeProject bool
	IncludeMemory  bool
	IncludeTools   bool
	ToolNames      []string // Specific tools to include (empty = all)
	PlanMode       bool
	CustomContext  map[string]interface{}
}

// NewBuilder creates a new prompt builder
func NewBuilder(store *manifest.ManifestStore, gatherer *ContextGatherer) *Builder {
	return &Builder{
		manifestStore: store,
		gatherer:      gatherer,
		funcMap:       defaultFuncMap(),
	}
}

// Build generates a complete system prompt for an agent
func (b *Builder) Build(ctx context.Context, agentName string, opts BuildOptions) (string, error) {
	// Get agent manifest
	agentManifest, ok := b.manifestStore.GetManifest(agentName)
	if !ok {
		return "", fmt.Errorf("agent manifest not found: %s", agentName)
	}

	// Build prompt context
	promptCtx := &PromptContext{
		PlanMode: opts.PlanMode,
	}

	// Build Self context from manifest
	promptCtx.Self = b.buildSelfContext(agentManifest)

	// Gather agent context if requested
	if opts.IncludeAgents {
		agents, err := b.gatherer.GatherAgentContext(ctx)
		if err != nil {
			// Non-fatal, continue with empty agent list
			agents = []AgentContext{}
		}
		promptCtx.Agents = agents
	}

	// Gather project context if requested
	if opts.IncludeProject {
		project, err := b.gatherer.GatherProjectContext()
		if err == nil && project != nil {
			promptCtx.Project = project
		}
	}

	// Gather memory if requested
	if opts.IncludeMemory {
		memory, err := b.gatherer.GatherMemory(ctx, "")
		if err == nil {
			promptCtx.Memory = memory
		}
	}

	// Gather tool context if requested
	if opts.IncludeTools {
		promptCtx.Tools = b.gatherer.GatherToolContextStructured(opts.ToolNames)
		promptCtx.ToolPrompt = b.gatherer.GatherToolContext(opts.ToolNames)
	}

	// Add custom context
	if opts.CustomContext != nil {
		promptCtx.Custom = opts.CustomContext
	}

	// Execute template
	return b.executeTemplate(agentManifest.Spec.Prompt.System, promptCtx)
}

// buildSelfContext creates SelfAgentContext from an agent manifest
func (b *Builder) buildSelfContext(m *manifest.AgentManifest) *SelfAgentContext {
	self := &SelfAgentContext{
		Name:         m.Metadata.Name,
		Description:  m.Metadata.Description,
		Type:         string(m.Spec.Type),
		Capabilities: m.GetCapabilityNames(),
	}

	// Populate identity if present
	if m.Spec.Identity != nil {
		self.IdentityStatement = m.Spec.Identity.Statement
		self.Role = m.Spec.Identity.Role
		self.Team = m.Spec.Identity.Team
		self.ResponsibleFor = m.Spec.Identity.ResponsibleFor
		self.Icon = m.Spec.Identity.Icon
	}

	// Populate voice if present
	if m.Spec.Voice != nil {
		self.Tone = m.Spec.Voice.Tone
		self.Style = m.Spec.Voice.Style
		self.Demeanor = m.Spec.Voice.Demeanor
		self.Phrases = m.Spec.Voice.Phrases
		self.Avoid = m.Spec.Voice.Avoid
	}

	// Populate behavior if present
	if m.Spec.Behavior != nil {
		self.Guidelines = m.Spec.Behavior.Guidelines
		self.Escalate = m.Spec.Behavior.Escalate
		self.Collaborate = m.Spec.Behavior.Collaborate
	}

	return self
}

// BuildWithContext builds a prompt with pre-gathered context
func (b *Builder) BuildWithContext(agentName string, promptCtx *PromptContext) (string, error) {
	agentManifest, ok := b.manifestStore.GetManifest(agentName)
	if !ok {
		return "", fmt.Errorf("agent manifest not found: %s", agentName)
	}

	return b.executeTemplate(agentManifest.Spec.Prompt.System, promptCtx)
}

// executeTemplate executes a prompt template with the given context
func (b *Builder) executeTemplate(templateStr string, ctx *PromptContext) (string, error) {
	tmpl, err := template.New("prompt").Funcs(b.funcMap).Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// GetManifestStore returns the underlying manifest store
func (b *Builder) GetManifestStore() *manifest.ManifestStore {
	return b.manifestStore
}

// defaultFuncMap returns the default template functions
func defaultFuncMap() template.FuncMap {
	return template.FuncMap{
		"join": strings.Join,
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": strings.Title,
		"trim":  strings.TrimSpace,
		"contains": strings.Contains,
		"hasPrefix": strings.HasPrefix,
		"hasSuffix": strings.HasSuffix,
		"replace": strings.ReplaceAll,
		"default": func(def, val interface{}) interface{} {
			if val == nil || val == "" {
				return def
			}
			return val
		},
		"list": func(items ...interface{}) []interface{} {
			return items
		},
		"dict": func(pairs ...interface{}) map[string]interface{} {
			m := make(map[string]interface{})
			for i := 0; i < len(pairs)-1; i += 2 {
				key, ok := pairs[i].(string)
				if ok {
					m[key] = pairs[i+1]
				}
			}
			return m
		},
		"indent": func(spaces int, s string) string {
			indent := strings.Repeat(" ", spaces)
			return indent + strings.ReplaceAll(s, "\n", "\n"+indent)
		},
		"printf": fmt.Sprintf,
	}
}

// PromptContext contains all context for prompt generation
type PromptContext struct {
	Self        *SelfAgentContext      // The agent's own identity and voice
	Agents      []AgentContext         // Available agents with status
	Project     *ProjectContext        // Name, values, goals, conventions
	Memory      []MemoryItem           // Relevant context/history
	Tools       []ToolContext          // Available tools (structured)
	ToolPrompt  string                 // Pre-generated tool prompt section
	PlanMode    bool                   // Current autonomy mode
	CurrentTask string                 // Current task being worked on
	Custom      map[string]interface{} // Custom context values
}

// SelfAgentContext contains the agent's own identity for self-reference in prompts
type SelfAgentContext struct {
	// Core identity
	Name              string   // Agent name (e.g., "PALADIN")
	Description       string   // Brief description
	Role              string   // Role (e.g., "Security Leadership")
	Team              string   // Team name (e.g., "CRBRS")
	Type              string   // Agent type (coordination, specialist, worker)
	IdentityStatement string   // Full identity statement

	// Responsibilities
	ResponsibleFor []string // Core responsibilities
	Capabilities   []string // Capability names

	// Voice characteristics
	Tone     string   // Communication tone
	Style    string   // Communication style
	Demeanor string   // Overall demeanor
	Phrases  []string // Characteristic phrases to use
	Avoid    []string // Things to never say

	// Behavioral rules
	Guidelines  []string          // General behavioral rules
	Escalate    []string          // When to escalate
	Collaborate map[string]string // Per-agent interaction rules

	// Display
	Icon string // Nerd Font icon for this agent
}

// AgentContext represents an available agent for prompt context
type AgentContext struct {
	Name         string   // Agent name
	Type         string   // Agent type (coordination, specialist, worker)
	Description  string   // Agent description
	Capabilities []string // List of capability names
	Status       string   // Current status (healthy, degraded, offline)
	Load         float64  // Current load percentage
}

// ProjectContext contains project-specific context
type ProjectContext struct {
	Name        string            // Project name
	Description string            // Project description
	Path        string            // Project path
	Values      []string          // Core values
	Goals       []string          // Current goals
	Conventions map[string]string // Coding/project conventions
	Context     map[string]string // Additional context
}

// MemoryItem represents stored context/memory
type MemoryItem struct {
	Key       string // Memory key
	Value     string // Memory value
	Source    string // Source agent or user
	Timestamp int64  // Unix timestamp
}
