// Package agentdb provides PostgreSQL storage for rich agent definitions
// with full system prompts, personality, and behavioral configuration.
//
// This package works alongside FalkorDB (for routing) to provide:
// - Rich agent definitions with 200+ line system prompts
// - Personality and voice configuration
// - Behavioral rules and collaboration guidelines
// - Version history for agent evolution tracking
package agentdb

import (
	"time"
)

// RichAgentDefinition represents a complete agent definition stored in PostgreSQL
// This extends the manifest-based definitions with full system prompts and versioning
type RichAgentDefinition struct {
	// Primary key
	ID      string `json:"id" db:"id"`
	AgentID string `json:"agent_id" db:"agent_id"` // e.g., "sntr", "paladin"
	Version int    `json:"version" db:"version"`
	Current bool   `json:"is_current" db:"is_current"`

	// Identity
	Name string `json:"name" db:"name"`
	Role string `json:"role" db:"role"`
	Team string `json:"team" db:"team"`

	// Rich content
	SystemPrompt         string               `json:"system_prompt" db:"system_prompt"`
	Personality          *Personality         `json:"personality" db:"personality"`
	Expertise            map[string]string    `json:"expertise" db:"expertise"` // domain -> level
	InteractionProtocols *InteractionProtocols `json:"interaction_protocols" db:"interaction_protocols"`
	DecisionFramework    *DecisionFramework   `json:"decision_framework" db:"decision_framework"`
	BehavioralRules      *BehavioralRules     `json:"behavioral_rules" db:"behavioral_rules"`

	// Operational
	Capabilities []string    `json:"capabilities" db:"capabilities"`
	TaskTypes    []string    `json:"task_types" db:"task_types"`
	ModelConfig  *ModelConfig `json:"model_config" db:"model_config"`

	// Metadata
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// Personality defines the agent's communication style and demeanor
type Personality struct {
	// Voice characteristics
	Tone     string `json:"tone"`     // e.g., "Authoritative but approachable"
	Style    string `json:"style"`    // e.g., "Strategic, clear"
	Demeanor string `json:"demeanor"` // e.g., "Seasoned leader"

	// Communication patterns
	Phrases []string `json:"phrases"` // Characteristic phrases to use
	Avoid   []string `json:"avoid"`   // Things to never say

	// Interaction style
	Formality   string `json:"formality"`   // formal, casual, adaptive
	Verbosity   string `json:"verbosity"`   // concise, detailed, adaptive
	Directness  string `json:"directness"`  // direct, diplomatic, adaptive
}

// InteractionProtocols defines how this agent interacts with other agents
type InteractionProtocols struct {
	// Per-agent interaction rules
	Collaborate map[string]CollaborationRule `json:"collaborate"`

	// General protocols
	HandoffStyle   string `json:"handoff_style"`   // structured, natural
	EscalationPath string `json:"escalation_path"` // e.g., "user -> manager -> executive"
	ReportingStyle string `json:"reporting_style"` // brief, detailed, structured
}

// CollaborationRule defines how to interact with a specific agent
type CollaborationRule struct {
	Role         string   `json:"role"`          // delegate_to, receive_from, peer
	ContextShare []string `json:"context_share"` // What context to share
	ExpectOutput string   `json:"expect_output"` // What output format to expect
	Notes        string   `json:"notes"`         // Additional collaboration notes
}

// DecisionFramework defines how the agent makes decisions
type DecisionFramework struct {
	// Decision process
	Process []string `json:"process"` // Steps in decision-making

	// Priorities
	Priorities []string `json:"priorities"` // Ordered list of priorities

	// Constraints
	MustConsider []string `json:"must_consider"` // Things that must be considered
	NeverDo      []string `json:"never_do"`      // Things that must never be done

	// Risk assessment
	RiskTolerance string `json:"risk_tolerance"` // low, medium, high
	RiskFactors   []string `json:"risk_factors"`  // What to evaluate for risk
}

// BehavioralRules defines do/don't rules for the agent
type BehavioralRules struct {
	// Core rules
	Guidelines []string `json:"guidelines"` // General behavioral rules
	Boundaries []string `json:"boundaries"` // Hard limits

	// Escalation
	EscalationTriggers []string `json:"escalation_triggers"` // When to escalate

	// Context-specific rules
	InPlanMode  []string `json:"in_plan_mode"`  // Rules when in plan mode
	InAutoMode  []string `json:"in_auto_mode"`  // Rules when in auto mode
	WithTools   []string `json:"with_tools"`    // Rules when using tools
	WithSecrets []string `json:"with_secrets"`  // Rules when handling secrets
}

// ModelConfig defines model configuration for the agent
type ModelConfig struct {
	DefaultModel string            `json:"default_model"`
	Fallbacks    []string          `json:"fallbacks"`
	Provider     string            `json:"provider"`
	Temperature  float64           `json:"temperature"`
	MaxTokens    int               `json:"max_tokens"`
	Parameters   map[string]string `json:"parameters"` // Provider-specific params
}

// DefinitionHistory tracks changes to agent definitions
type DefinitionHistory struct {
	ID           string                 `json:"id" db:"id"`
	DefinitionID string                 `json:"definition_id" db:"definition_id"`
	Version      int                    `json:"version" db:"version"`
	ChangedFields map[string]interface{} `json:"changed_fields" db:"changed_fields"`
	ChangedAt    time.Time              `json:"changed_at" db:"changed_at"`
	ChangedBy    string                 `json:"changed_by" db:"changed_by"`
}

// AgentSummary provides a lightweight view for listing/routing
type AgentSummary struct {
	AgentID      string   `json:"agent_id" db:"agent_id"`
	Name         string   `json:"name" db:"name"`
	Role         string   `json:"role" db:"role"`
	Team         string   `json:"team" db:"team"`
	Model        string   `json:"model" db:"model"`
	TaskTypes    []string `json:"task_types" db:"task_types"`
	Capabilities []string `json:"capabilities" db:"capabilities"`
	Version      int      `json:"version" db:"version"`
}

// QueryOptions for listing/filtering agents
type QueryOptions struct {
	Team       string   // Filter by team
	TaskTypes  []string // Filter by supported task types
	Capability string   // Filter by capability
	Current    *bool    // Filter by current version only
	Limit      int      // Max results
	Offset     int      // Pagination offset
}

// NewRichAgentDefinition creates a new agent definition with defaults
func NewRichAgentDefinition(agentID, name string) *RichAgentDefinition {
	now := time.Now()
	return &RichAgentDefinition{
		AgentID:   agentID,
		Name:      name,
		Version:   1,
		Current:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// WithPersonality sets personality configuration
func (d *RichAgentDefinition) WithPersonality(p *Personality) *RichAgentDefinition {
	d.Personality = p
	return d
}

// WithBehavior sets behavioral rules
func (d *RichAgentDefinition) WithBehavior(b *BehavioralRules) *RichAgentDefinition {
	d.BehavioralRules = b
	return d
}

// WithInteractions sets interaction protocols
func (d *RichAgentDefinition) WithInteractions(i *InteractionProtocols) *RichAgentDefinition {
	d.InteractionProtocols = i
	return d
}

// GetTone returns the personality tone or default
func (d *RichAgentDefinition) GetTone() string {
	if d.Personality != nil && d.Personality.Tone != "" {
		return d.Personality.Tone
	}
	return "helpful and professional"
}

// GetStyle returns the personality style or default
func (d *RichAgentDefinition) GetStyle() string {
	if d.Personality != nil && d.Personality.Style != "" {
		return d.Personality.Style
	}
	return "clear and concise"
}

// HasCapability checks if the agent has a specific capability
func (d *RichAgentDefinition) HasCapability(cap string) bool {
	for _, c := range d.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// SupportsTaskType checks if the agent supports a task type
func (d *RichAgentDefinition) SupportsTaskType(taskType string) bool {
	for _, t := range d.TaskTypes {
		if t == taskType {
			return true
		}
	}
	return false
}
