package tracing

import (
	"context"

	"github.com/syntor/syntor/pkg/models"
)

// Tracer interface for distributed tracing
type Tracer interface {
	// Span management
	StartSpan(ctx context.Context, operationName string, opts ...SpanOption) (context.Context, Span)
	Extract(ctx context.Context, carrier Carrier) (context.Context, error)
	Inject(ctx context.Context, carrier Carrier) error

	// Lifecycle
	Close() error
}

// Span represents a single operation within a trace
type Span interface {
	// Finishing
	Finish()
	FinishWithError(err error)

	// Context
	Context() models.TraceContext

	// Tagging
	SetTag(key string, value interface{}) Span
	SetOperationName(operationName string) Span

	// Logging
	LogFields(fields ...LogField)
	LogError(err error)
}

// SpanOption configures span creation
type SpanOption func(*SpanConfig)

// SpanConfig holds span configuration
type SpanConfig struct {
	Parent      models.TraceContext
	Tags        map[string]interface{}
	StartTime   int64
	SpanKind    SpanKind
}

// SpanKind represents the role of the span
type SpanKind string

const (
	SpanKindServer   SpanKind = "server"
	SpanKindClient   SpanKind = "client"
	SpanKindProducer SpanKind = "producer"
	SpanKindConsumer SpanKind = "consumer"
	SpanKindInternal SpanKind = "internal"
)

// LogField represents a log field for span logging
type LogField struct {
	Key   string
	Value interface{}
}

// Carrier interface for context propagation
type Carrier interface {
	Set(key, value string)
	Get(key string) string
	Keys() []string
}

// MapCarrier implements Carrier using a map
type MapCarrier map[string]string

func (m MapCarrier) Set(key, value string) {
	m[key] = value
}

func (m MapCarrier) Get(key string) string {
	return m[key]
}

func (m MapCarrier) Keys() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Standard span tags
const (
	TagComponent     = "component"
	TagAgentID       = "agent.id"
	TagAgentType     = "agent.type"
	TagTaskID        = "task.id"
	TagTaskType      = "task.type"
	TagMessageID     = "message.id"
	TagMessageType   = "message.type"
	TagTopic         = "kafka.topic"
	TagPartition     = "kafka.partition"
	TagOffset        = "kafka.offset"
	TagError         = "error"
	TagHTTPMethod    = "http.method"
	TagHTTPURL       = "http.url"
	TagHTTPStatus    = "http.status_code"
)

// Standard operation names
const (
	OpMessagePublish  = "message.publish"
	OpMessageConsume  = "message.consume"
	OpTaskExecute     = "task.execute"
	OpTaskRoute       = "task.route"
	OpAgentRegister   = "agent.register"
	OpAgentHeartbeat  = "agent.heartbeat"
	OpRegistryLookup  = "registry.lookup"
)

// WithParent sets the parent trace context
func WithParent(parent models.TraceContext) SpanOption {
	return func(cfg *SpanConfig) {
		cfg.Parent = parent
	}
}

// WithTags sets initial tags for the span
func WithTags(tags map[string]interface{}) SpanOption {
	return func(cfg *SpanConfig) {
		cfg.Tags = tags
	}
}

// WithSpanKind sets the span kind
func WithSpanKind(kind SpanKind) SpanOption {
	return func(cfg *SpanConfig) {
		cfg.SpanKind = kind
	}
}

// Config holds tracer configuration
type Config struct {
	ServiceName    string  `json:"service_name"`
	AgentEndpoint  string  `json:"agent_endpoint"`
	SamplingRate   float64 `json:"sampling_rate"`
	Enabled        bool    `json:"enabled"`
}

// DefaultConfig returns default tracer configuration
func DefaultConfig() Config {
	return Config{
		ServiceName:   "syntor",
		AgentEndpoint: "http://localhost:14268/api/traces",
		SamplingRate:  1.0,
		Enabled:       true,
	}
}
