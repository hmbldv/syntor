package context

import (
	"context"
	"time"
)

// Store defines the interface for context/memory storage
type Store interface {
	// Get retrieves a context value by key
	Get(ctx context.Context, key string) (string, error)

	// Set stores a context value with optional TTL
	Set(ctx context.Context, key, value string, ttl time.Duration) error

	// Delete removes a context value
	Delete(ctx context.Context, key string) error

	// Query searches for context values matching a prefix
	Query(ctx context.Context, prefix string, limit int) ([]Item, error)

	// GetSessionContext retrieves all context for a session
	GetSessionContext(ctx context.Context, sessionID string) (*SessionContext, error)

	// SaveSessionContext stores session context
	SaveSessionContext(ctx context.Context, session *SessionContext) error

	// Close closes the store connection
	Close() error
}

// Item represents a stored context item
type Item struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Source    string    `json:"source"`     // Agent or user that created this
	Timestamp time.Time `json:"timestamp"`
	TTL       int64     `json:"ttl,omitempty"` // Remaining TTL in seconds
}

// SessionContext contains all context for a user session
type SessionContext struct {
	SessionID     string                 `json:"sessionId"`
	UserID        string                 `json:"userId,omitempty"`
	ProjectName   string                 `json:"projectName,omitempty"`
	ProjectPath   string                 `json:"projectPath,omitempty"`
	CoreValues    []string               `json:"coreValues,omitempty"`
	Goals         []string               `json:"goals,omitempty"`
	Conventions   map[string]string      `json:"conventions,omitempty"`
	AgentHistory  []AgentInteraction     `json:"agentHistory,omitempty"`
	ActiveTask    *TaskContext           `json:"activeTask,omitempty"`
	Memory        map[string]string      `json:"memory,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt     time.Time              `json:"createdAt"`
	LastUpdatedAt time.Time              `json:"lastUpdatedAt"`
}

// AgentInteraction records an interaction with an agent
type AgentInteraction struct {
	AgentName string    `json:"agentName"`
	Action    string    `json:"action"`
	Input     string    `json:"input,omitempty"`
	Output    string    `json:"output,omitempty"`
	Duration  int64     `json:"duration"` // Milliseconds
	Timestamp time.Time `json:"timestamp"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

// NewSessionContext creates a new session context with defaults
func NewSessionContext(sessionID string) *SessionContext {
	now := time.Now()
	return &SessionContext{
		SessionID:     sessionID,
		Memory:        make(map[string]string),
		Metadata:      make(map[string]interface{}),
		AgentHistory:  make([]AgentInteraction, 0),
		CreatedAt:     now,
		LastUpdatedAt: now,
	}
}

// AddInteraction adds an agent interaction to the session history
func (s *SessionContext) AddInteraction(interaction AgentInteraction) {
	s.AgentHistory = append(s.AgentHistory, interaction)
	s.LastUpdatedAt = time.Now()

	// Keep only last 50 interactions to prevent unbounded growth
	if len(s.AgentHistory) > 50 {
		s.AgentHistory = s.AgentHistory[len(s.AgentHistory)-50:]
	}
}

// SetMemory stores a key-value pair in session memory
func (s *SessionContext) SetMemory(key, value string) {
	s.Memory[key] = value
	s.LastUpdatedAt = time.Now()
}

// GetMemory retrieves a value from session memory
func (s *SessionContext) GetMemory(key string) (string, bool) {
	val, ok := s.Memory[key]
	return val, ok
}

// GetRecentInteractions returns the n most recent agent interactions
func (s *SessionContext) GetRecentInteractions(n int) []AgentInteraction {
	if n <= 0 || len(s.AgentHistory) == 0 {
		return nil
	}
	if n > len(s.AgentHistory) {
		n = len(s.AgentHistory)
	}
	return s.AgentHistory[len(s.AgentHistory)-n:]
}
