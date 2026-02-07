package subagent

import (
	"context"
	"time"
)

// MessageBus defines the interface for inter-agent messaging.
type MessageBus interface {
	// Send delivers a message from one agent to another.
	Send(ctx context.Context, from, to string, msg AgentMessage) error

	// Subscribe returns a channel for receiving messages directed at the given agent.
	Subscribe(agentID string) <-chan AgentMessage

	// Broadcast sends a message to all subscribed agents except the sender.
	Broadcast(from string, msg AgentMessage) error

	// Close shuts down the message bus and closes all channels.
	Close() error
}

// AgentMessage represents a message exchanged between agents.
type AgentMessage struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Type      string    `json:"type"` // "message", "status", "result", "error"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}
