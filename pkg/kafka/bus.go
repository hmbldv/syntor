package kafka

import (
	"context"

	"github.com/syntor/syntor/pkg/models"
)

// MessageHandler is a function type for handling incoming messages
type MessageHandler func(ctx context.Context, msg models.Message) error

// MessageBus interface for Kafka abstraction
type MessageBus interface {
	// Publishing
	Publish(ctx context.Context, topic string, msg models.Message) error
	PublishWithKey(ctx context.Context, topic string, key string, msg models.Message) error

	// Subscribing
	Subscribe(ctx context.Context, topic string, handler MessageHandler) error
	SubscribeToMultiple(ctx context.Context, topics []string, handler MessageHandler) error
	Unsubscribe(ctx context.Context, topic string) error

	// Topic management
	CreateTopic(ctx context.Context, topic string, config TopicConfig) error
	DeleteTopic(ctx context.Context, topic string) error
	ListTopics(ctx context.Context) ([]string, error)

	// Lifecycle
	Connect(ctx context.Context) error
	Close() error
	Health() models.HealthStatus
}

// TopicConfig holds configuration for Kafka topic creation
type TopicConfig struct {
	NumPartitions     int               `json:"num_partitions"`
	ReplicationFactor int               `json:"replication_factor"`
	RetentionMs       int64             `json:"retention_ms"`
	CleanupPolicy     string            `json:"cleanup_policy"`
	Configs           map[string]string `json:"configs,omitempty"`
}

// ProducerConfig holds configuration for Kafka producer
type ProducerConfig struct {
	Acks            string `json:"acks"`             // "0", "1", "all"
	Retries         int    `json:"retries"`
	BatchSize       int    `json:"batch_size"`
	LingerMs        int    `json:"linger_ms"`
	CompressionType string `json:"compression_type"` // none, gzip, snappy, lz4, zstd
}

// ConsumerConfig holds configuration for Kafka consumer
type ConsumerConfig struct {
	GroupID          string `json:"group_id"`
	AutoOffsetReset  string `json:"auto_offset_reset"` // earliest, latest
	EnableAutoCommit bool   `json:"enable_auto_commit"`
	MaxPollRecords   int    `json:"max_poll_records"`
	SessionTimeoutMs int    `json:"session_timeout_ms"`
}

// BusConfig holds complete Kafka configuration
type BusConfig struct {
	Brokers  []string       `json:"brokers"`
	Producer ProducerConfig `json:"producer"`
	Consumer ConsumerConfig `json:"consumer"`
	Security SecurityConfig `json:"security,omitempty"`
}

// SecurityConfig holds Kafka security configuration
type SecurityConfig struct {
	Enabled  bool   `json:"enabled"`
	Protocol string `json:"protocol"` // PLAINTEXT, SSL, SASL_PLAINTEXT, SASL_SSL
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// Standard topic names for SYNTOR
const (
	TopicTaskAssignment    = "syntor.tasks.assignment"
	TopicTaskStatus        = "syntor.tasks.status"
	TopicTaskComplete      = "syntor.tasks.complete"
	TopicAgentRegistration = "syntor.agents.registration"
	TopicAgentHeartbeat    = "syntor.agents.heartbeat"
	TopicServiceRequest    = "syntor.services.request"
	TopicServiceResponse   = "syntor.services.response"
	TopicSystemEvents      = "syntor.system.events"
	TopicDeadLetter        = "syntor.dlq"
)

// DefaultTopicConfig returns default topic configuration
func DefaultTopicConfig() TopicConfig {
	return TopicConfig{
		NumPartitions:     3,
		ReplicationFactor: 1, // Use 3 in production
		RetentionMs:       604800000, // 7 days
		CleanupPolicy:     "delete",
	}
}

// DefaultProducerConfig returns default producer configuration
func DefaultProducerConfig() ProducerConfig {
	return ProducerConfig{
		Acks:            "all",
		Retries:         3,
		BatchSize:       16384,
		LingerMs:        1,
		CompressionType: "snappy",
	}
}

// DefaultConsumerConfig returns default consumer configuration
func DefaultConsumerConfig() ConsumerConfig {
	return ConsumerConfig{
		GroupID:          "syntor-default",
		AutoOffsetReset:  "earliest",
		EnableAutoCommit: false,
		MaxPollRecords:   500,
		SessionTimeoutMs: 10000,
	}
}

// ConnectionError represents a Kafka connection error
type ConnectionError struct {
	Message string
}

func (e *ConnectionError) Error() string {
	return "kafka connection error: " + e.Message
}
