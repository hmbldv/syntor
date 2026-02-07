package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/syntor/syntor/pkg/kafka"
	"github.com/syntor/syntor/pkg/models"
)

// KafkaMessageBus is a Kafka-backed MessageBus implementation.
// Each session uses a dedicated topic: syntor.agents.<session_id>.
type KafkaMessageBus struct {
	client    *kafka.Client
	sessionID string
	topic     string

	mu       sync.RWMutex
	channels map[string]chan AgentMessage
	closed   bool
}

// NewKafkaMessageBus creates a Kafka-backed message bus for the given session.
func NewKafkaMessageBus(client *kafka.Client, sessionID string) *KafkaMessageBus {
	topic := fmt.Sprintf("syntor.agents.%s", sessionID)

	return &KafkaMessageBus{
		client:    client,
		sessionID: sessionID,
		topic:     topic,
		channels:  make(map[string]chan AgentMessage),
	}
}

// Send publishes a message to Kafka with the recipient as partition key.
func (b *KafkaMessageBus) Send(ctx context.Context, from, to string, msg AgentMessage) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return fmt.Errorf("message bus is closed")
	}
	b.mu.RUnlock()

	if msg.ID == "" {
		msg.ID = uuid.New().String()[:8]
	}
	msg.From = from
	msg.To = to
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	return b.publish(ctx, to, msg)
}

// Subscribe starts a Kafka consumer for the given agent and returns its channel.
func (b *KafkaMessageBus) Subscribe(agentID string) <-chan AgentMessage {
	b.mu.Lock()

	if ch, ok := b.channels[agentID]; ok {
		b.mu.Unlock()
		return ch
	}

	ch := make(chan AgentMessage, localInboxBufferSize)
	b.channels[agentID] = ch
	b.mu.Unlock()

	// Start a Kafka consumer that routes messages into the channel.
	handler := func(_ context.Context, m models.Message) error {
		var agentMsg AgentMessage
		payload, err := json.Marshal(m.Payload)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(payload, &agentMsg); err != nil {
			return err
		}

		// Only deliver if targeted at this agent
		if agentMsg.To != agentID {
			return nil
		}

		select {
		case ch <- agentMsg:
		default:
			// Drop if full
		}
		return nil
	}

	// Subscribe on the session topic with a per-agent group
	ctx := context.Background()
	_ = b.client.Subscribe(ctx, b.topic, handler)

	return ch
}

// Broadcast publishes a message to Kafka without a specific partition key.
func (b *KafkaMessageBus) Broadcast(from string, msg AgentMessage) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return fmt.Errorf("message bus is closed")
	}
	b.mu.RUnlock()

	if msg.ID == "" {
		msg.ID = uuid.New().String()[:8]
	}
	msg.From = from
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	// Publish without partition key so all consumers receive it
	return b.publish(context.Background(), "", msg)
}

// Close shuts down all channels and unsubscribes from Kafka.
func (b *KafkaMessageBus) Close() error {
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

	// Unsubscribe from the session topic
	ctx := context.Background()
	_ = b.client.Unsubscribe(ctx, b.topic)

	return nil
}

// publish serializes an AgentMessage into a models.Message and publishes to Kafka.
func (b *KafkaMessageBus) publish(ctx context.Context, key string, msg AgentMessage) error {
	payload := map[string]interface{}{
		"id":        msg.ID,
		"from":      msg.From,
		"to":        msg.To,
		"type":      msg.Type,
		"content":   msg.Content,
		"timestamp": msg.Timestamp.Format(time.RFC3339Nano),
	}

	modelMsg := models.Message{
		ID:            msg.ID,
		Type:          models.MessageType("agent_message"),
		Source:        msg.From,
		Target:        msg.To,
		Payload:       payload,
		Timestamp:     msg.Timestamp,
		CorrelationID: b.sessionID,
	}

	if key != "" {
		return b.client.PublishWithKey(ctx, b.topic, key, modelMsg)
	}
	return b.client.Publish(ctx, b.topic, modelMsg)
}
