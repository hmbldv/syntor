package tracing

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/syntor/syntor/pkg/models"
)

// contextKey type for context values
type contextKey string

const (
	spanContextKey contextKey = "syntor_span"
)

// JaegerTracer implements Tracer interface for Jaeger-compatible tracing
type JaegerTracer struct {
	config      Config
	serviceName string
	spans       map[string]*JaegerSpan
	mu          sync.RWMutex
	reporter    SpanReporter
}

// SpanReporter interface for reporting finished spans
type SpanReporter interface {
	Report(span *JaegerSpan) error
	Close() error
}

// NoopReporter is a reporter that does nothing (for testing)
type NoopReporter struct{}

func (r *NoopReporter) Report(span *JaegerSpan) error { return nil }
func (r *NoopReporter) Close() error                  { return nil }

// InMemoryReporter stores spans in memory (for testing)
type InMemoryReporter struct {
	spans []*JaegerSpan
	mu    sync.Mutex
}

func NewInMemoryReporter() *InMemoryReporter {
	return &InMemoryReporter{
		spans: make([]*JaegerSpan, 0),
	}
}

func (r *InMemoryReporter) Report(span *JaegerSpan) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spans = append(r.spans, span)
	return nil
}

func (r *InMemoryReporter) Close() error { return nil }

func (r *InMemoryReporter) GetSpans() []*JaegerSpan {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]*JaegerSpan, len(r.spans))
	copy(result, r.spans)
	return result
}

func (r *InMemoryReporter) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spans = r.spans[:0]
}

// JaegerSpan implements Span interface
type JaegerSpan struct {
	tracer        *JaegerTracer
	operationName string
	traceID       string
	spanID        string
	parentID      string
	startTime     time.Time
	endTime       time.Time
	tags          map[string]interface{}
	logs          []SpanLog
	finished      bool
	mu            sync.Mutex
}

// SpanLog represents a log entry in a span
type SpanLog struct {
	Timestamp time.Time
	Fields    []LogField
}

// NewJaegerTracer creates a new Jaeger-compatible tracer
func NewJaegerTracer(config Config) *JaegerTracer {
	return &JaegerTracer{
		config:      config,
		serviceName: config.ServiceName,
		spans:       make(map[string]*JaegerSpan),
		reporter:    &NoopReporter{},
	}
}

// NewJaegerTracerWithReporter creates a tracer with a custom reporter
func NewJaegerTracerWithReporter(config Config, reporter SpanReporter) *JaegerTracer {
	return &JaegerTracer{
		config:      config,
		serviceName: config.ServiceName,
		spans:       make(map[string]*JaegerSpan),
		reporter:    reporter,
	}
}

// StartSpan creates a new span
func (t *JaegerTracer) StartSpan(ctx context.Context, operationName string, opts ...SpanOption) (context.Context, Span) {
	cfg := &SpanConfig{
		Tags: make(map[string]interface{}),
	}

	for _, opt := range opts {
		opt(cfg)
	}

	// Generate IDs
	traceID := cfg.Parent.TraceID
	parentID := cfg.Parent.SpanID

	if traceID == "" {
		traceID = generateTraceID()
	}

	spanID := generateSpanID()

	span := &JaegerSpan{
		tracer:        t,
		operationName: operationName,
		traceID:       traceID,
		spanID:        spanID,
		parentID:      parentID,
		startTime:     time.Now(),
		tags:          cfg.Tags,
		logs:          make([]SpanLog, 0),
	}

	// Add span kind tag if specified
	if cfg.SpanKind != "" {
		span.tags["span.kind"] = string(cfg.SpanKind)
	}

	// Add service name tag
	span.tags["service.name"] = t.serviceName

	// Store span
	t.mu.Lock()
	t.spans[spanID] = span
	t.mu.Unlock()

	// Add span to context
	newCtx := context.WithValue(ctx, spanContextKey, span)

	return newCtx, span
}

// Extract extracts trace context from a carrier
func (t *JaegerTracer) Extract(ctx context.Context, carrier Carrier) (context.Context, error) {
	traceID := carrier.Get("uber-trace-id")
	if traceID == "" {
		traceID = carrier.Get("trace-id")
	}

	spanID := carrier.Get("span-id")
	parentID := carrier.Get("parent-id")

	if traceID == "" {
		return ctx, fmt.Errorf("no trace context found in carrier")
	}

	// Create a placeholder span with extracted context
	span := &JaegerSpan{
		tracer:   t,
		traceID:  traceID,
		spanID:   spanID,
		parentID: parentID,
		tags:     make(map[string]interface{}),
		logs:     make([]SpanLog, 0),
	}

	return context.WithValue(ctx, spanContextKey, span), nil
}

// Inject injects trace context into a carrier
func (t *JaegerTracer) Inject(ctx context.Context, carrier Carrier) error {
	span := SpanFromContext(ctx)
	if span == nil {
		return fmt.Errorf("no span in context")
	}

	traceCtx := span.Context()
	carrier.Set("trace-id", traceCtx.TraceID)
	carrier.Set("span-id", traceCtx.SpanID)
	if traceCtx.ParentID != "" {
		carrier.Set("parent-id", traceCtx.ParentID)
	}

	// Also set uber-trace-id for Jaeger compatibility
	carrier.Set("uber-trace-id", fmt.Sprintf("%s:%s:%s:1", traceCtx.TraceID, traceCtx.SpanID, traceCtx.ParentID))

	return nil
}

// Close closes the tracer
func (t *JaegerTracer) Close() error {
	return t.reporter.Close()
}

// SpanFromContext extracts a span from context
func SpanFromContext(ctx context.Context) Span {
	if span, ok := ctx.Value(spanContextKey).(*JaegerSpan); ok {
		return span
	}
	return nil
}

// JaegerSpan methods

// Finish completes the span
func (s *JaegerSpan) Finish() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.finished {
		return
	}

	s.endTime = time.Now()
	s.finished = true

	// Report span
	if s.tracer != nil && s.tracer.reporter != nil {
		s.tracer.reporter.Report(s)
	}

	// Remove from tracer's span map
	if s.tracer != nil {
		s.tracer.mu.Lock()
		delete(s.tracer.spans, s.spanID)
		s.tracer.mu.Unlock()
	}
}

// FinishWithError completes the span with an error
func (s *JaegerSpan) FinishWithError(err error) {
	if err != nil {
		s.SetTag("error", true)
		s.SetTag("error.message", err.Error())
		s.LogError(err)
	}
	s.Finish()
}

// Context returns the trace context
func (s *JaegerSpan) Context() models.TraceContext {
	return models.TraceContext{
		TraceID:  s.traceID,
		SpanID:   s.spanID,
		ParentID: s.parentID,
	}
}

// SetTag sets a tag on the span
func (s *JaegerSpan) SetTag(key string, value interface{}) Span {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tags[key] = value
	return s
}

// SetOperationName changes the operation name
func (s *JaegerSpan) SetOperationName(operationName string) Span {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.operationName = operationName
	return s
}

// LogFields adds log fields to the span
func (s *JaegerSpan) LogFields(fields ...LogField) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, SpanLog{
		Timestamp: time.Now(),
		Fields:    fields,
	})
}

// LogError logs an error to the span
func (s *JaegerSpan) LogError(err error) {
	s.LogFields(
		LogField{Key: "event", Value: "error"},
		LogField{Key: "error.object", Value: err.Error()},
	)
}

// Getter methods for testing

// GetOperationName returns the operation name
func (s *JaegerSpan) GetOperationName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.operationName
}

// GetTags returns the span tags
func (s *JaegerSpan) GetTags() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]interface{})
	for k, v := range s.tags {
		result[k] = v
	}
	return result
}

// GetLogs returns the span logs
func (s *JaegerSpan) GetLogs() []SpanLog {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]SpanLog, len(s.logs))
	copy(result, s.logs)
	return result
}

// GetDuration returns the span duration
func (s *JaegerSpan) GetDuration() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.endTime.IsZero() {
		return time.Since(s.startTime)
	}
	return s.endTime.Sub(s.startTime)
}

// IsFinished returns whether the span is finished
func (s *JaegerSpan) IsFinished() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finished
}

// Helper functions

func generateTraceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateSpanID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Global tracer instance
var globalTracer Tracer

// SetGlobalTracer sets the global tracer instance
func SetGlobalTracer(tracer Tracer) {
	globalTracer = tracer
}

// GetGlobalTracer returns the global tracer instance
func GetGlobalTracer() Tracer {
	if globalTracer == nil {
		globalTracer = NewJaegerTracer(DefaultConfig())
	}
	return globalTracer
}

// Package-level convenience functions

// StartSpan starts a new span using the global tracer
func StartSpan(ctx context.Context, operationName string, opts ...SpanOption) (context.Context, Span) {
	return GetGlobalTracer().StartSpan(ctx, operationName, opts...)
}

// Extract extracts trace context using the global tracer
func Extract(ctx context.Context, carrier Carrier) (context.Context, error) {
	return GetGlobalTracer().Extract(ctx, carrier)
}

// Inject injects trace context using the global tracer
func Inject(ctx context.Context, carrier Carrier) error {
	return GetGlobalTracer().Inject(ctx, carrier)
}
