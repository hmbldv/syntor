package subagent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

const localInboxBufferSize = 100

// LocalMessageBus is an in-memory, channel-based MessageBus implementation.
type LocalMessageBus struct {
	mu       sync.RWMutex
	channels map[string]chan AgentMessage
	closed   bool
}

// NewLocalMessageBus creates a new in-memory message bus.
func NewLocalMessageBus() *LocalMessageBus {
	return &LocalMessageBus{
		channels: make(map[string]chan AgentMessage),
	}
}

// Send delivers a message to a specific agent's inbox.
// Non-blocking: drops the message if the recipient's channel is full.
func (b *LocalMessageBus) Send(_ context.Context, from, to string, msg AgentMessage) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return fmt.Errorf("message bus is closed")
	}

	ch, ok := b.channels[to]
	if !ok {
		return fmt.Errorf("agent not subscribed: %s", to)
	}

	// Stamp metadata
	if msg.ID == "" {
		msg.ID = uuid.New().String()[:8]
	}
	msg.From = from
	msg.To = to
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	select {
	case ch <- msg:
	default:
		// Channel full, drop message to avoid blocking
	}

	return nil
}

// Subscribe creates (or returns) a buffered channel for the given agent.
func (b *LocalMessageBus) Subscribe(agentID string) <-chan AgentMessage {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.channels[agentID]; ok {
		return ch
	}

	ch := make(chan AgentMessage, localInboxBufferSize)
	b.channels[agentID] = ch
	return ch
}

// Broadcast sends a message to every subscribed agent except the sender.
func (b *LocalMessageBus) Broadcast(from string, msg AgentMessage) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return fmt.Errorf("message bus is closed")
	}

	if msg.ID == "" {
		msg.ID = uuid.New().String()[:8]
	}
	msg.From = from
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	for agentID, ch := range b.channels {
		if agentID == from {
			continue
		}

		outMsg := msg
		outMsg.To = agentID

		select {
		case ch <- outMsg:
		default:
			// Channel full, drop to avoid blocking
		}
	}

	return nil
}

// Close shuts down the bus and closes all agent channels.
func (b *LocalMessageBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true
	for id, ch := range b.channels {
		close(ch)
		delete(b.channels, id)
	}

	return nil
}
