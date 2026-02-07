// Package session provides conversation session persistence and resume.
// Sessions are stored as append-only JSONL files in ~/.syntor/sessions/.
package session

import (
	"time"

	"github.com/syntor/syntor/pkg/inference"
)

// Session represents a conversation session with metadata.
type Session struct {
	ID         string            `json:"id"`
	Name       string            `json:"name,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
	WorkingDir string            `json:"working_dir"`
	AgentName  string            `json:"agent_name"`
	TokensUsed int64             `json:"tokens_used"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// StoredMessage wraps an inference.Message with session metadata for JSONL storage.
type StoredMessage struct {
	SessionID string    `json:"session_id"`
	Index     int       `json:"index"`
	Timestamp time.Time `json:"timestamp"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Agent     string    `json:"agent,omitempty"`
}

// ToInferenceMessage converts a StoredMessage to an inference.Message.
func (m *StoredMessage) ToInferenceMessage() inference.Message {
	return inference.Message{
		Role:    m.Role,
		Content: m.Content,
	}
}

// FromInferenceMessage creates a StoredMessage from an inference.Message.
func FromInferenceMessage(sessionID string, index int, msg inference.Message, agent string) StoredMessage {
	return StoredMessage{
		SessionID: sessionID,
		Index:     index,
		Timestamp: time.Now(),
		Role:      msg.Role,
		Content:   msg.Content,
		Agent:     agent,
	}
}

// SessionSummary is a lightweight representation for listing sessions.
type SessionSummary struct {
	ID         string    `json:"id"`
	Name       string    `json:"name,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	WorkingDir string    `json:"working_dir"`
	AgentName  string    `json:"agent_name"`
	Messages   int       `json:"messages"`
	TokensUsed int64     `json:"tokens_used"`
}
