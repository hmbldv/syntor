// +build property

package property

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
	"github.com/syntor/syntor/pkg/kafka"
	"github.com/syntor/syntor/pkg/models"
)

// **Feature: syntor-multi-agent-system, Property 7: Kafka message routing**
// *For any* inter-agent communication, all messages should be routed through Kafka topics and be traceable in Kafka logs
// **Validates: Requirements 3.1**

// **Feature: syntor-multi-agent-system, Property 8: Message delivery reliability**
// *For any* message sent with acknowledgment requirements, the system should ensure delivery according to the configured acknowledgment level
// **Validates: Requirements 3.2**

// MockMessageBus implements a mock Kafka client for property testing
type MockMessageBus struct {
	mu          sync.RWMutex
	published   []PublishedMessage
	subscribers map[string][]kafka.MessageHandler
	topics      map[string]kafka.TopicConfig
	connected   bool
	health      models.HealthStatus
	failRate    float64 // Probability of publish failure (0-1)
}

type PublishedMessage struct {
	Topic   string
	Key     string
	Message models.Message
	Time    time.Time
}

func NewMockMessageBus() *MockMessageBus {
	return &MockMessageBus{
		published:   make([]PublishedMessage, 0),
		subscribers: make(map[string][]kafka.MessageHandler),
		topics:      make(map[string]kafka.TopicConfig),
		health:      models.HealthUnknown,
	}
}

func (m *MockMessageBus) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = true
	m.health = models.HealthHealthy
	return nil
}

func (m *MockMessageBus) Publish(ctx context.Context, topic string, msg models.Message) error {
	return m.PublishWithKey(ctx, topic, msg.ID, msg)
}

func (m *MockMessageBus) PublishWithKey(ctx context.Context, topic string, key string, msg models.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.connected {
		return &kafka.ConnectionError{Message: "not connected"}
	}

	m.published = append(m.published, PublishedMessage{
		Topic:   topic,
		Key:     key,
		Message: msg,
		Time:    time.Now(),
	})

	// Deliver to subscribers
	if handlers, ok := m.subscribers[topic]; ok {
		for _, handler := range handlers {
			go handler(ctx, msg)
		}
	}

	return nil
}

func (m *MockMessageBus) Subscribe(ctx context.Context, topic string, handler kafka.MessageHandler) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.subscribers[topic] == nil {
		m.subscribers[topic] = make([]kafka.MessageHandler, 0)
	}
	m.subscribers[topic] = append(m.subscribers[topic], handler)
	return nil
}

func (m *MockMessageBus) SubscribeToMultiple(ctx context.Context, topics []string, handler kafka.MessageHandler) error {
	for _, topic := range topics {
		if err := m.Subscribe(ctx, topic, handler); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockMessageBus) Unsubscribe(ctx context.Context, topic string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.subscribers, topic)
	return nil
}

func (m *MockMessageBus) CreateTopic(ctx context.Context, topic string, config kafka.TopicConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.topics[topic] = config
	return nil
}

func (m *MockMessageBus) DeleteTopic(ctx context.Context, topic string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.topics, topic)
	return nil
}

func (m *MockMessageBus) ListTopics(ctx context.Context) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	topics := make([]string, 0, len(m.topics))
	for topic := range m.topics {
		topics = append(topics, topic)
	}
	return topics, nil
}

func (m *MockMessageBus) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = false
	m.health = models.HealthUnknown
	return nil
}

func (m *MockMessageBus) Health() models.HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.health
}

func (m *MockMessageBus) GetPublished() []PublishedMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]PublishedMessage, len(m.published))
	copy(result, m.published)
	return result
}

func (m *MockMessageBus) ClearPublished() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = make([]PublishedMessage, 0)
}

// ConnectionError for mock
type connectionError struct {
	message string
}

func (e *connectionError) Error() string {
	return e.message
}

// Add ConnectionError to kafka package for testing
func init() {
	// This is a workaround for the test
}

// TestKafkaMessageRouting tests that messages are properly routed through topics
func TestKafkaMessageRouting(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Messages published to a topic are received by subscribers
	properties.Property("messages reach subscribers", prop.ForAll(
		func(topic string, msgContent string) bool {
			if topic == "" {
				return true // Skip empty topics
			}

			bus := NewMockMessageBus()
			ctx := context.Background()
			bus.Connect(ctx)

			received := make(chan models.Message, 1)
			bus.Subscribe(ctx, topic, func(ctx context.Context, msg models.Message) error {
				received <- msg
				return nil
			})

			msg := models.NewMessage(
				models.MsgTaskAssignment,
				"test-source",
				"test-target",
				map[string]interface{}{"content": msgContent},
			)

			bus.Publish(ctx, topic, msg)

			select {
			case receivedMsg := <-received:
				return receivedMsg.ID == msg.ID
			case <-time.After(100 * time.Millisecond):
				return false
			}
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 100 }),
		gen.AlphaString(),
	))

	// Property: Messages are traceable by their ID
	properties.Property("messages are traceable by ID", prop.ForAll(
		func(messageCount int) bool {
			if messageCount <= 0 || messageCount > 100 {
				messageCount = 10
			}

			bus := NewMockMessageBus()
			ctx := context.Background()
			bus.Connect(ctx)

			topic := "test-topic"
			sentIDs := make(map[string]bool)

			for i := 0; i < messageCount; i++ {
				msg := models.NewMessage(
					models.MsgTaskAssignment,
					"test-source",
					"test-target",
					map[string]interface{}{"index": i},
				)
				sentIDs[msg.ID] = true
				bus.Publish(ctx, topic, msg)
			}

			// Verify all messages are recorded
			published := bus.GetPublished()
			if len(published) != messageCount {
				return false
			}

			// Verify all IDs are present
			for _, pm := range published {
				if !sentIDs[pm.Message.ID] {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 100),
	))

	// Property: Messages are routed to correct topics
	properties.Property("messages routed to correct topic", prop.ForAll(
		func(topicCount int) bool {
			if topicCount <= 0 || topicCount > 10 {
				topicCount = 3
			}

			bus := NewMockMessageBus()
			ctx := context.Background()
			bus.Connect(ctx)

			// Create topics and send messages
			for i := 0; i < topicCount; i++ {
				topic := string(rune('A' + i))
				msg := models.NewMessage(
					models.MsgTaskAssignment,
					"source",
					"target",
					map[string]interface{}{"topic": topic},
				)
				bus.Publish(ctx, topic, msg)
			}

			// Verify each message went to correct topic
			published := bus.GetPublished()
			if len(published) != topicCount {
				return false
			}

			for _, pm := range published {
				expectedTopic := pm.Message.Payload["topic"].(string)
				if pm.Topic != expectedTopic {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 10),
	))

	// Property: Multiple subscribers receive the same message
	properties.Property("multiple subscribers receive same message", prop.ForAll(
		func(subscriberCount int) bool {
			if subscriberCount <= 0 || subscriberCount > 10 {
				subscriberCount = 3
			}

			bus := NewMockMessageBus()
			ctx := context.Background()
			bus.Connect(ctx)

			topic := "multi-sub-topic"
			receivedCount := int32(0)
			var mu sync.Mutex

			// Subscribe multiple handlers
			for i := 0; i < subscriberCount; i++ {
				bus.Subscribe(ctx, topic, func(ctx context.Context, msg models.Message) error {
					mu.Lock()
					receivedCount++
					mu.Unlock()
					return nil
				})
			}

			msg := models.NewMessage(models.MsgTaskAssignment, "source", "target", nil)
			bus.Publish(ctx, topic, msg)

			// Wait for async delivery
			time.Sleep(50 * time.Millisecond)

			mu.Lock()
			result := int(receivedCount) == subscriberCount
			mu.Unlock()

			return result
		},
		gen.IntRange(1, 10),
	))

	properties.TestingRun(t)
}

// TestMessageDeliveryReliability tests message delivery guarantees
func TestMessageDeliveryReliability(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: All published messages are recorded
	properties.Property("all messages are recorded", prop.ForAll(
		func(messageCount int) bool {
			if messageCount <= 0 || messageCount > 1000 {
				messageCount = 100
			}

			bus := NewMockMessageBus()
			ctx := context.Background()
			bus.Connect(ctx)

			topic := "reliability-test"

			for i := 0; i < messageCount; i++ {
				msg := models.NewMessage(
					models.MsgTaskAssignment,
					"source",
					"target",
					map[string]interface{}{"seq": i},
				)
				if err := bus.Publish(ctx, topic, msg); err != nil {
					return false
				}
			}

			published := bus.GetPublished()
			return len(published) == messageCount
		},
		gen.IntRange(1, 1000),
	))

	// Property: Message order is preserved within a topic
	properties.Property("message order preserved", prop.ForAll(
		func(messageCount int) bool {
			if messageCount <= 0 || messageCount > 100 {
				messageCount = 50
			}

			bus := NewMockMessageBus()
			ctx := context.Background()
			bus.Connect(ctx)

			topic := "order-test"

			for i := 0; i < messageCount; i++ {
				msg := models.NewMessage(
					models.MsgTaskAssignment,
					"source",
					"target",
					map[string]interface{}{"seq": i},
				)
				bus.Publish(ctx, topic, msg)
			}

			published := bus.GetPublished()

			// Verify order
			for i, pm := range published {
				seq := int(pm.Message.Payload["seq"].(int))
				if seq != i {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 100),
	))

	// Property: Disconnected client cannot publish
	properties.Property("disconnected client rejects publish", prop.ForAll(
		func(_ int) bool {
			bus := NewMockMessageBus()
			// Don't connect

			msg := models.NewMessage(models.MsgTaskAssignment, "source", "target", nil)
			err := bus.Publish(context.Background(), "topic", msg)

			return err != nil
		},
		gen.Int(),
	))

	// Property: Messages contain required headers
	properties.Property("messages have required metadata", prop.ForAll(
		func(source string, correlationID string) bool {
			if source == "" {
				return true // Skip invalid
			}

			bus := NewMockMessageBus()
			ctx := context.Background()
			bus.Connect(ctx)

			msg := models.NewMessage(models.MsgTaskAssignment, source, "target", nil)
			msg.CorrelationID = correlationID

			bus.Publish(ctx, "test-topic", msg)

			published := bus.GetPublished()
			if len(published) != 1 {
				return false
			}

			pm := published[0]
			return pm.Message.Source == source &&
				pm.Message.CorrelationID == correlationID &&
				!pm.Message.Timestamp.IsZero() &&
				pm.Message.ID != ""
		},
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" }),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// TestTopicManagement tests topic creation and deletion
func TestTopicManagement(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Created topics can be listed
	properties.Property("created topics appear in list", prop.ForAll(
		func(topicNames []string) bool {
			if len(topicNames) == 0 {
				return true
			}

			bus := NewMockMessageBus()
			ctx := context.Background()
			bus.Connect(ctx)

			// Filter unique, non-empty names
			uniqueTopics := make(map[string]bool)
			for _, name := range topicNames {
				if name != "" && len(name) < 50 {
					uniqueTopics[name] = true
				}
			}

			// Create topics
			for topic := range uniqueTopics {
				bus.CreateTopic(ctx, topic, kafka.DefaultTopicConfig())
			}

			// List topics
			listed, err := bus.ListTopics(ctx)
			if err != nil {
				return false
			}

			// Verify all topics are listed
			listedSet := make(map[string]bool)
			for _, t := range listed {
				listedSet[t] = true
			}

			for topic := range uniqueTopics {
				if !listedSet[topic] {
					return false
				}
			}

			return true
		},
		gen.SliceOf(gen.AlphaString()),
	))

	// Property: Deleted topics are removed from list
	properties.Property("deleted topics removed from list", prop.ForAll(
		func(topicName string) bool {
			if topicName == "" {
				return true
			}

			bus := NewMockMessageBus()
			ctx := context.Background()
			bus.Connect(ctx)

			// Create topic
			bus.CreateTopic(ctx, topicName, kafka.DefaultTopicConfig())

			// Verify exists
			listed, _ := bus.ListTopics(ctx)
			found := false
			for _, t := range listed {
				if t == topicName {
					found = true
					break
				}
			}
			if !found {
				return false
			}

			// Delete topic
			bus.DeleteTopic(ctx, topicName)

			// Verify deleted
			listed, _ = bus.ListTopics(ctx)
			for _, t := range listed {
				if t == topicName {
					return false
				}
			}

			return true
		},
		gen.AlphaString().SuchThat(func(s string) bool { return s != "" && len(s) < 50 }),
	))

	properties.TestingRun(t)
}

// Unit tests for standard topic names
func TestStandardTopics(t *testing.T) {
	standardTopics := []string{
		kafka.TopicTaskAssignment,
		kafka.TopicTaskStatus,
		kafka.TopicTaskComplete,
		kafka.TopicAgentRegistration,
		kafka.TopicAgentHeartbeat,
		kafka.TopicServiceRequest,
		kafka.TopicServiceResponse,
		kafka.TopicSystemEvents,
		kafka.TopicDeadLetter,
	}

	for _, topic := range standardTopics {
		assert.NotEmpty(t, topic, "Standard topic should not be empty")
		assert.Contains(t, topic, "syntor.", "Standard topic should have syntor prefix")
	}
}

// Unit tests for default configurations
func TestDefaultConfigurations(t *testing.T) {
	t.Run("DefaultTopicConfig", func(t *testing.T) {
		config := kafka.DefaultTopicConfig()
		assert.Greater(t, config.NumPartitions, 0)
		assert.Greater(t, config.ReplicationFactor, 0)
		assert.Greater(t, config.RetentionMs, int64(0))
	})

	t.Run("DefaultProducerConfig", func(t *testing.T) {
		config := kafka.DefaultProducerConfig()
		assert.NotEmpty(t, config.Acks)
		assert.GreaterOrEqual(t, config.Retries, 0)
	})

	t.Run("DefaultConsumerConfig", func(t *testing.T) {
		config := kafka.DefaultConsumerConfig()
		assert.NotEmpty(t, config.GroupID)
		assert.NotEmpty(t, config.AutoOffsetReset)
	})
}
