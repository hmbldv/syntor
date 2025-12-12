package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"github.com/syntor/syntor/pkg/models"
)

// Client implements the MessageBus interface using Kafka
type Client struct {
	config       BusConfig
	writer       *kafka.Writer
	readers      map[string]*kafka.Reader
	handlers     map[string]MessageHandler
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	connected    bool
	health       models.HealthStatus
}

// NewClient creates a new Kafka client
func NewClient(config BusConfig) *Client {
	return &Client{
		config:   config,
		readers:  make(map[string]*kafka.Reader),
		handlers: make(map[string]MessageHandler),
		health:   models.HealthUnknown,
	}
}

// Connect establishes connection to Kafka brokers
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	c.ctx, c.cancel = context.WithCancel(ctx)

	// Create writer with configuration
	c.writer = &kafka.Writer{
		Addr:         kafka.TCP(c.config.Brokers...),
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    c.config.Producer.BatchSize,
		BatchTimeout: time.Duration(c.config.Producer.LingerMs) * time.Millisecond,
		Async:        false, // Synchronous writes for reliability
		Compression:  compressionCodec(c.config.Producer.CompressionType),
		RequiredAcks: requiredAcks(c.config.Producer.Acks),
	}

	c.connected = true
	c.health = models.HealthHealthy

	return nil
}

// Publish sends a message to a Kafka topic
func (c *Client) Publish(ctx context.Context, topic string, msg models.Message) error {
	return c.PublishWithKey(ctx, topic, msg.ID, msg)
}

// PublishWithKey sends a message to a Kafka topic with a specific key
func (c *Client) PublishWithKey(ctx context.Context, topic string, key string, msg models.Message) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("kafka client not connected")
	}
	c.mu.RUnlock()

	// Validate message
	if err := msg.Validate(); err != nil {
		return fmt.Errorf("invalid message: %w", err)
	}

	// Serialize message
	value, err := msg.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	// Create Kafka message
	kafkaMsg := kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: value,
		Headers: []kafka.Header{
			{Key: "message_type", Value: []byte(msg.Type)},
			{Key: "source", Value: []byte(msg.Source)},
			{Key: "correlation_id", Value: []byte(msg.CorrelationID)},
			{Key: "timestamp", Value: []byte(msg.Timestamp.Format(time.RFC3339Nano))},
		},
	}

	// Add trace headers if present
	if msg.CorrelationID != "" {
		kafkaMsg.Headers = append(kafkaMsg.Headers, kafka.Header{
			Key:   "trace_id",
			Value: []byte(msg.CorrelationID),
		})
	}

	// Write message
	err = c.writer.WriteMessages(ctx, kafkaMsg)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	return nil
}

// Subscribe registers a handler for messages on a topic
func (c *Client) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.readers[topic]; exists {
		return fmt.Errorf("already subscribed to topic: %s", topic)
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        c.config.Brokers,
		Topic:          topic,
		GroupID:        c.config.Consumer.GroupID,
		MinBytes:       1,
		MaxBytes:       10e6, // 10MB
		MaxWait:        500 * time.Millisecond,
		CommitInterval: time.Second,
		StartOffset:    startOffset(c.config.Consumer.AutoOffsetReset),
	})

	c.readers[topic] = reader
	c.handlers[topic] = handler

	// Start consumer goroutine
	c.wg.Add(1)
	go c.consumeMessages(topic, reader, handler)

	return nil
}

// SubscribeToMultiple subscribes to multiple topics with the same handler
func (c *Client) SubscribeToMultiple(ctx context.Context, topics []string, handler MessageHandler) error {
	for _, topic := range topics {
		if err := c.Subscribe(ctx, topic, handler); err != nil {
			return err
		}
	}
	return nil
}

// Unsubscribe removes subscription from a topic
func (c *Client) Unsubscribe(ctx context.Context, topic string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	reader, exists := c.readers[topic]
	if !exists {
		return nil
	}

	if err := reader.Close(); err != nil {
		return fmt.Errorf("failed to close reader for topic %s: %w", topic, err)
	}

	delete(c.readers, topic)
	delete(c.handlers, topic)

	return nil
}

// CreateTopic creates a new Kafka topic
func (c *Client) CreateTopic(ctx context.Context, topic string, config TopicConfig) error {
	conn, err := kafka.DialContext(ctx, "tcp", c.config.Brokers[0])
	if err != nil {
		return fmt.Errorf("failed to connect to kafka: %w", err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("failed to get controller: %w", err)
	}

	controllerConn, err := kafka.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		return fmt.Errorf("failed to connect to controller: %w", err)
	}
	defer controllerConn.Close()

	topicConfig := kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     config.NumPartitions,
		ReplicationFactor: config.ReplicationFactor,
	}

	err = controllerConn.CreateTopics(topicConfig)
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	return nil
}

// DeleteTopic deletes a Kafka topic
func (c *Client) DeleteTopic(ctx context.Context, topic string) error {
	conn, err := kafka.DialContext(ctx, "tcp", c.config.Brokers[0])
	if err != nil {
		return fmt.Errorf("failed to connect to kafka: %w", err)
	}
	defer conn.Close()

	err = conn.DeleteTopics(topic)
	if err != nil {
		return fmt.Errorf("failed to delete topic: %w", err)
	}

	return nil
}

// ListTopics returns all available topics
func (c *Client) ListTopics(ctx context.Context) ([]string, error) {
	conn, err := kafka.DialContext(ctx, "tcp", c.config.Brokers[0])
	if err != nil {
		return nil, fmt.Errorf("failed to connect to kafka: %w", err)
	}
	defer conn.Close()

	partitions, err := conn.ReadPartitions()
	if err != nil {
		return nil, fmt.Errorf("failed to read partitions: %w", err)
	}

	topicSet := make(map[string]struct{})
	for _, p := range partitions {
		topicSet[p.Topic] = struct{}{}
	}

	topics := make([]string, 0, len(topicSet))
	for topic := range topicSet {
		topics = append(topics, topic)
	}

	return topics, nil
}

// Close shuts down the Kafka client
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	// Cancel context to stop consumers
	if c.cancel != nil {
		c.cancel()
	}

	// Wait for consumers to finish
	c.wg.Wait()

	// Close all readers
	for topic, reader := range c.readers {
		if err := reader.Close(); err != nil {
			return fmt.Errorf("failed to close reader for topic %s: %w", topic, err)
		}
	}

	// Close writer
	if c.writer != nil {
		if err := c.writer.Close(); err != nil {
			return fmt.Errorf("failed to close writer: %w", err)
		}
	}

	c.connected = false
	c.health = models.HealthUnknown
	c.readers = make(map[string]*kafka.Reader)
	c.handlers = make(map[string]MessageHandler)

	return nil
}

// Health returns the current health status
func (c *Client) Health() models.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health
}

// consumeMessages reads messages from a topic and invokes the handler
func (c *Client) consumeMessages(topic string, reader *kafka.Reader, handler MessageHandler) {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			// Set read deadline
			ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)

			kafkaMsg, err := reader.FetchMessage(ctx)
			cancel()

			if err != nil {
				if c.ctx.Err() != nil {
					return // Context cancelled, exit gracefully
				}
				continue // Timeout or transient error, retry
			}

			// Deserialize message
			var msg models.Message
			if err := json.Unmarshal(kafkaMsg.Value, &msg); err != nil {
				// Log error and continue
				continue
			}

			// Invoke handler
			handlerCtx := context.Background()
			if err := handler(handlerCtx, msg); err != nil {
				// Handler failed, could implement retry logic here
				continue
			}

			// Commit message
			if err := reader.CommitMessages(c.ctx, kafkaMsg); err != nil {
				// Failed to commit, message will be reprocessed
				continue
			}
		}
	}
}

// Helper functions

func compressionCodec(compression string) kafka.Compression {
	switch compression {
	case "gzip":
		return kafka.Gzip
	case "snappy":
		return kafka.Snappy
	case "lz4":
		return kafka.Lz4
	case "zstd":
		return kafka.Zstd
	default:
		return 0 // No compression
	}
}

func requiredAcks(acks string) kafka.RequiredAcks {
	switch acks {
	case "0":
		return kafka.RequireNone
	case "1":
		return kafka.RequireOne
	case "all", "-1":
		return kafka.RequireAll
	default:
		return kafka.RequireAll
	}
}

func startOffset(offset string) int64 {
	switch offset {
	case "earliest":
		return kafka.FirstOffset
	case "latest":
		return kafka.LastOffset
	default:
		return kafka.FirstOffset
	}
}

// PublishAsync sends a message asynchronously (fire-and-forget)
func (c *Client) PublishAsync(topic string, msg models.Message) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return fmt.Errorf("kafka client not connected")
	}
	c.mu.RUnlock()

	// Generate ID if not set
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		c.Publish(ctx, topic, msg) // Ignore error for async
	}()

	return nil
}

// RequestReply implements request-reply pattern
func (c *Client) RequestReply(ctx context.Context, topic string, msg models.Message, timeout time.Duration) (models.Message, error) {
	// Create reply topic
	replyTopic := fmt.Sprintf("syntor.reply.%s", msg.ID)
	msg.ReplyTo = replyTopic

	// Create temporary subscription for reply
	replyChan := make(chan models.Message, 1)
	errChan := make(chan error, 1)

	handler := func(ctx context.Context, reply models.Message) error {
		replyChan <- reply
		return nil
	}

	if err := c.Subscribe(ctx, replyTopic, handler); err != nil {
		return models.Message{}, fmt.Errorf("failed to subscribe to reply topic: %w", err)
	}
	defer c.Unsubscribe(ctx, replyTopic)

	// Send request
	if err := c.Publish(ctx, topic, msg); err != nil {
		return models.Message{}, fmt.Errorf("failed to publish request: %w", err)
	}

	// Wait for reply
	select {
	case reply := <-replyChan:
		return reply, nil
	case err := <-errChan:
		return models.Message{}, err
	case <-time.After(timeout):
		return models.Message{}, fmt.Errorf("request timeout")
	case <-ctx.Done():
		return models.Message{}, ctx.Err()
	}
}
