package manifest

import (
	"time"
)

// AgentManifest defines a complete agent configuration
type AgentManifest struct {
	APIVersion string    `yaml:"apiVersion" json:"apiVersion"` // "syntor.dev/v1"
	Kind       string    `yaml:"kind" json:"kind"`             // "Agent"
	Metadata   AgentMeta `yaml:"metadata" json:"metadata"`
	Spec       AgentSpec `yaml:"spec" json:"spec"`
}

// AgentMeta contains agent identification
type AgentMeta struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	CreatedAt   time.Time         `yaml:"createdAt,omitempty" json:"createdAt,omitempty"`
	UpdatedAt   time.Time         `yaml:"updatedAt,omitempty" json:"updatedAt,omitempty"`
}

// AgentSpec defines agent behavior and configuration
type AgentSpec struct {
	Type         AgentType        `yaml:"type" json:"type"` // "coordination", "specialist", "worker"
	Model        ModelSpec        `yaml:"model" json:"model"`
	Capabilities []CapabilitySpec `yaml:"capabilities" json:"capabilities"`
	Prompt       PromptSpec       `yaml:"prompt" json:"prompt"`
	Handoff      HandoffSpec      `yaml:"handoff,omitempty" json:"handoff,omitempty"`
	Constraints  ConstraintSpec   `yaml:"constraints,omitempty" json:"constraints,omitempty"`

	// Agent identity and personality (Phase 2)
	Identity *IdentitySpec `yaml:"identity,omitempty" json:"identity,omitempty"`
	Voice    *VoiceSpec    `yaml:"voice,omitempty" json:"voice,omitempty"`
	Behavior *BehaviorSpec `yaml:"behavior,omitempty" json:"behavior,omitempty"`
}

// IdentitySpec defines the agent's core identity
type IdentitySpec struct {
	Statement      string   `yaml:"statement" json:"statement"`                                 // "You are PALADIN, the CISO..."
	Role           string   `yaml:"role,omitempty" json:"role,omitempty"`                       // "Security Leadership"
	Team           string   `yaml:"team,omitempty" json:"team,omitempty"`                       // "CRBRS"
	ResponsibleFor []string `yaml:"responsibleFor,omitempty" json:"responsibleFor,omitempty"`   // Core responsibilities
	Icon           string   `yaml:"icon,omitempty" json:"icon,omitempty"`                       // Nerd Font icon for display
}

// VoiceSpec defines how the agent communicates
type VoiceSpec struct {
	Tone     string   `yaml:"tone,omitempty" json:"tone,omitempty"`         // "Authoritative but approachable"
	Style    string   `yaml:"style,omitempty" json:"style,omitempty"`       // "Strategic, clear"
	Demeanor string   `yaml:"demeanor,omitempty" json:"demeanor,omitempty"` // "Seasoned leader"
	Phrases  []string `yaml:"phrases,omitempty" json:"phrases,omitempty"`   // Characteristic phrases to use
	Avoid    []string `yaml:"avoid,omitempty" json:"avoid,omitempty"`       // Things to never say
}

// BehaviorSpec defines agent behavioral guidelines
type BehaviorSpec struct {
	Guidelines  []string          `yaml:"guidelines,omitempty" json:"guidelines,omitempty"`   // General behavioral rules
	Escalate    []string          `yaml:"escalate,omitempty" json:"escalate,omitempty"`       // When to escalate to user/other agent
	Collaborate map[string]string `yaml:"collaborate,omitempty" json:"collaborate,omitempty"` // Per-agent interaction rules
}

// AgentType defines the category of agent
type AgentType string

const (
	AgentTypeCoordination AgentType = "coordination"
	AgentTypeSpecialist   AgentType = "specialist"
	AgentTypeWorker       AgentType = "worker"
)

// ModelSpec defines model configuration
type ModelSpec struct {
	Default   string   `yaml:"default" json:"default"`
	Fallbacks []string `yaml:"fallbacks,omitempty" json:"fallbacks,omitempty"`
	Provider  string   `yaml:"provider,omitempty" json:"provider,omitempty"` // Override provider
}

// CapabilitySpec defines a capability
type CapabilitySpec struct {
	Name        string            `yaml:"name" json:"name"`
	Version     string            `yaml:"version,omitempty" json:"version,omitempty"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Parameters  map[string]string `yaml:"parameters,omitempty" json:"parameters,omitempty"`
}

// PromptSpec defines prompt templates
type PromptSpec struct {
	System   string          `yaml:"system" json:"system"`                     // System prompt template
	Context  []ContextSource `yaml:"context,omitempty" json:"context,omitempty"` // Context sources to inject
	Examples []PromptExample `yaml:"examples,omitempty" json:"examples,omitempty"` // Few-shot examples
}

// ContextSource defines dynamic context injection
type ContextSource struct {
	Type   ContextType `yaml:"type" json:"type"`             // "agents", "project", "memory", "values"
	Filter string      `yaml:"filter,omitempty" json:"filter,omitempty"` // Filter expression
}

// ContextType defines the type of context to inject
type ContextType string

const (
	ContextTypeAgents  ContextType = "agents"
	ContextTypeProject ContextType = "project"
	ContextTypeMemory  ContextType = "memory"
	ContextTypeValues  ContextType = "values"
)

// PromptExample for few-shot learning
type PromptExample struct {
	Input  string `yaml:"input" json:"input"`
	Output string `yaml:"output" json:"output"`
}

// HandoffSpec defines how agent can delegate
type HandoffSpec struct {
	AllowedTargets  []string `yaml:"allowedTargets,omitempty" json:"allowedTargets,omitempty"` // "*" for any
	Protocol        string   `yaml:"protocol" json:"protocol"`                                   // "structured", "natural"
	RequireApproval bool     `yaml:"requireApproval" json:"requireApproval"`
}

// ConstraintSpec defines operational constraints
type ConstraintSpec struct {
	MaxConcurrent int      `yaml:"maxConcurrent,omitempty" json:"maxConcurrent,omitempty"`
	Timeout       string   `yaml:"timeout,omitempty" json:"timeout,omitempty"` // Duration string
	RateLimits    []string `yaml:"rateLimits,omitempty" json:"rateLimits,omitempty"`
}

// ProjectContext defines project-specific context
type ProjectContext struct {
	APIVersion string             `yaml:"apiVersion" json:"apiVersion"` // "syntor.dev/v1"
	Kind       string             `yaml:"kind" json:"kind"`             // "ProjectContext"
	Metadata   ProjectMeta        `yaml:"metadata" json:"metadata"`
	Spec       ProjectContextSpec `yaml:"spec" json:"spec"`
}

// ProjectMeta contains project identification
type ProjectMeta struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// ProjectContextSpec defines project context details
type ProjectContextSpec struct {
	Values      []string          `yaml:"values,omitempty" json:"values,omitempty"`           // Core values
	Goals       []string          `yaml:"goals,omitempty" json:"goals,omitempty"`             // Current goals
	Conventions map[string]string `yaml:"conventions,omitempty" json:"conventions,omitempty"` // Coding conventions
	Context     map[string]string `yaml:"context,omitempty" json:"context,omitempty"`         // Additional context
}

// GetCapabilityNames returns a list of capability names
func (m *AgentManifest) GetCapabilityNames() []string {
	names := make([]string, len(m.Spec.Capabilities))
	for i, cap := range m.Spec.Capabilities {
		names[i] = cap.Name
	}
	return names
}

// HasCapability checks if the agent has a specific capability
func (m *AgentManifest) HasCapability(name string) bool {
	for _, cap := range m.Spec.Capabilities {
		if cap.Name == name {
			return true
		}
	}
	return false
}

// CanHandoff checks if the agent can hand off to a target
func (m *AgentManifest) CanHandoff(target string) bool {
	for _, t := range m.Spec.Handoff.AllowedTargets {
		if t == "*" || t == target {
			return true
		}
	}
	return false
}

// GetIdentityStatement returns the identity statement or a default
func (m *AgentManifest) GetIdentityStatement() string {
	if m.Spec.Identity != nil && m.Spec.Identity.Statement != "" {
		return m.Spec.Identity.Statement
	}
	return ""
}

// GetVoiceTone returns the voice tone or empty string
func (m *AgentManifest) GetVoiceTone() string {
	if m.Spec.Voice != nil {
		return m.Spec.Voice.Tone
	}
	return ""
}

// GetVoiceStyle returns the voice style or empty string
func (m *AgentManifest) GetVoiceStyle() string {
	if m.Spec.Voice != nil {
		return m.Spec.Voice.Style
	}
	return ""
}

// GetBehaviorGuidelines returns behavior guidelines or empty slice
func (m *AgentManifest) GetBehaviorGuidelines() []string {
	if m.Spec.Behavior != nil {
		return m.Spec.Behavior.Guidelines
	}
	return nil
}

// GetCollaborationRule returns the collaboration rule for a specific agent
func (m *AgentManifest) GetCollaborationRule(agentName string) string {
	if m.Spec.Behavior != nil && m.Spec.Behavior.Collaborate != nil {
		return m.Spec.Behavior.Collaborate[agentName]
	}
	return ""
}

// HasIdentity returns true if the agent has identity configured
func (m *AgentManifest) HasIdentity() bool {
	return m.Spec.Identity != nil && m.Spec.Identity.Statement != ""
}

// HasVoice returns true if the agent has voice configured
func (m *AgentManifest) HasVoice() bool {
	return m.Spec.Voice != nil && (m.Spec.Voice.Tone != "" || m.Spec.Voice.Style != "")
}
