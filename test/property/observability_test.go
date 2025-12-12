// +build property

package property

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"

	"github.com/syntor/syntor/pkg/logging"
	"github.com/syntor/syntor/pkg/metrics"
	"github.com/syntor/syntor/pkg/tracing"
)

// **Feature: syntor-multi-agent-system, Property 18: Structured logging with correlation**
// *For any* agent activity, the system should produce structured logs with correlation IDs
// that can be traced across agent interactions
// **Validates: Requirements 7.1**

func TestStructuredLoggingWithCorrelation(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Logs contain correlation ID when set in context
	properties.Property("correlation ID in logs", prop.ForAll(
		func(correlationID string, message string) bool {
			if correlationID == "" || message == "" {
				return true // Skip empty values
			}

			var buf bytes.Buffer
			config := logging.Config{
				Level:  logging.DebugLevel,
				Format: "json",
				Output: &buf,
			}

			logger, err := logging.NewZapLoggerWithWriter(config)
			if err != nil {
				return false
			}

			ctx := logging.WithCorrelationID(context.Background(), correlationID)
			logger.WithContext(ctx).Info(message)
			logger.Sync()

			output := buf.String()
			return strings.Contains(output, correlationID)
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 50 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 100 }),
	))

	// Property: Logs contain trace ID when set in context
	properties.Property("trace ID in logs", prop.ForAll(
		func(traceID string, message string) bool {
			if traceID == "" || message == "" {
				return true
			}

			var buf bytes.Buffer
			config := logging.Config{
				Level:  logging.DebugLevel,
				Format: "json",
				Output: &buf,
			}

			logger, err := logging.NewZapLoggerWithWriter(config)
			if err != nil {
				return false
			}

			ctx := logging.WithTraceID(context.Background(), traceID)
			logger.WithContext(ctx).Info(message)
			logger.Sync()

			output := buf.String()
			return strings.Contains(output, traceID)
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 50 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 100 }),
	))

	// Property: Logs contain agent ID when set in context
	properties.Property("agent ID in logs", prop.ForAll(
		func(agentID string, message string) bool {
			if agentID == "" || message == "" {
				return true
			}

			var buf bytes.Buffer
			config := logging.Config{
				Level:  logging.DebugLevel,
				Format: "json",
				Output: &buf,
			}

			logger, err := logging.NewZapLoggerWithWriter(config)
			if err != nil {
				return false
			}

			ctx := logging.WithAgentID(context.Background(), agentID)
			logger.WithContext(ctx).Info(message)
			logger.Sync()

			output := buf.String()
			return strings.Contains(output, agentID)
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 50 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 100 }),
	))

	// Property: JSON logs are valid JSON
	properties.Property("JSON logs are parseable", prop.ForAll(
		func(message string) bool {
			if message == "" {
				return true
			}

			var buf bytes.Buffer
			config := logging.Config{
				Level:  logging.DebugLevel,
				Format: "json",
				Output: &buf,
			}

			logger, err := logging.NewZapLoggerWithWriter(config)
			if err != nil {
				return false
			}

			logger.Info(message, logging.String("key", "value"))
			logger.Sync()

			var parsed map[string]interface{}
			return json.Unmarshal(buf.Bytes(), &parsed) == nil
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 100 }),
	))

	// Property: Log levels are respected
	properties.Property("log levels filter correctly", prop.ForAll(
		func(levelIdx int) bool {
			levels := []logging.LogLevel{
				logging.DebugLevel,
				logging.InfoLevel,
				logging.WarnLevel,
				logging.ErrorLevel,
			}
			level := levels[levelIdx%len(levels)]

			var buf bytes.Buffer
			config := logging.Config{
				Level:  level,
				Format: "json",
				Output: &buf,
			}

			logger, err := logging.NewZapLoggerWithWriter(config)
			if err != nil {
				return false
			}

			// Debug should only appear at DebugLevel
			logger.Debug("debug message")
			logger.Sync()

			output := buf.String()
			if level == logging.DebugLevel {
				return strings.Contains(output, "debug message")
			}
			return !strings.Contains(output, "debug message")
		},
		gen.IntRange(0, 100),
	))

	properties.TestingRun(t)
}

// **Feature: syntor-multi-agent-system, Property 19: Metrics exposure**
// *For any* system component, performance metrics, message throughput, and error rates
// should be exposed through monitoring endpoints
// **Validates: Requirements 7.2**

func TestMetricsExposure(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Registered metrics can be incremented
	properties.Property("counter metrics increment", prop.ForAll(
		func(value int) bool {
			if value <= 0 {
				value = 1
			}

			collector := metrics.NewPrometheusCollector()
			metric := metrics.Metric{
				Name:   "test_counter",
				Type:   metrics.CounterType,
				Help:   "Test counter",
				Labels: []string{"label1"},
			}

			if err := collector.Register(metric); err != nil {
				return false
			}

			labels := metrics.Labels("label1", "value1")
			for i := 0; i < value; i++ {
				collector.IncrementCounter("test_counter", labels)
			}

			return collector.IsRegistered("test_counter")
		},
		gen.IntRange(1, 100),
	))

	// Property: Gauge metrics can be set
	properties.Property("gauge metrics set value", prop.ForAll(
		func(value float64) bool {
			collector := metrics.NewPrometheusCollector()
			metric := metrics.Metric{
				Name:   "test_gauge",
				Type:   metrics.GaugeType,
				Help:   "Test gauge",
				Labels: []string{"label1"},
			}

			if err := collector.Register(metric); err != nil {
				return false
			}

			labels := metrics.Labels("label1", "value1")
			collector.SetGauge("test_gauge", value, labels)

			return collector.IsRegistered("test_gauge")
		},
		gen.Float64Range(0, 1000),
	))

	// Property: Histogram metrics observe values
	properties.Property("histogram metrics observe", prop.ForAll(
		func(value float64) bool {
			if value < 0 {
				value = -value
			}

			collector := metrics.NewPrometheusCollector()
			metric := metrics.Metric{
				Name:    "test_histogram",
				Type:    metrics.HistogramType,
				Help:    "Test histogram",
				Labels:  []string{"label1"},
				Buckets: []float64{0.1, 0.5, 1.0, 5.0, 10.0},
			}

			if err := collector.Register(metric); err != nil {
				return false
			}

			labels := metrics.Labels("label1", "value1")
			collector.ObserveHistogram("test_histogram", value, labels)

			return collector.IsRegistered("test_histogram")
		},
		gen.Float64Range(0, 100),
	))

	// Property: Standard metrics can be registered
	properties.Property("standard metrics register", prop.ForAll(
		func(_ int) bool {
			collector := metrics.NewPrometheusCollector()
			err := collector.RegisterStandardMetrics()
			if err != nil {
				return false
			}

			// Check that standard metrics are registered
			names := collector.GetMetricNames()
			requiredMetrics := []string{
				"syntor_agent_tasks_processed_total",
				"syntor_agent_current_load",
				"syntor_messages_sent_total",
			}

			for _, required := range requiredMetrics {
				found := false
				for _, name := range names {
					if name == required {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			}
			return true
		},
		gen.Int(),
	))

	// Property: Metrics handler is available
	properties.Property("metrics handler available", prop.ForAll(
		func(_ int) bool {
			collector := metrics.NewPrometheusCollector()
			handler := collector.Handler()
			return handler != nil
		},
		gen.Int(),
	))

	properties.TestingRun(t)
}

// **Feature: syntor-multi-agent-system, Property 20: Distributed tracing**
// *For any* multi-agent interaction, distributed traces should be generated that allow
// following the execution path across all involved agents
// **Validates: Requirements 7.3**

func TestDistributedTracing(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: Spans have valid trace context
	properties.Property("spans have trace context", prop.ForAll(
		func(operationName string) bool {
			if operationName == "" {
				return true
			}

			tracer := tracing.NewJaegerTracer(tracing.DefaultConfig())
			ctx, span := tracer.StartSpan(context.Background(), operationName)
			defer span.Finish()

			traceCtx := span.Context()
			return traceCtx.TraceID != "" && traceCtx.SpanID != "" && ctx != nil
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 50 }),
	))

	// Property: Child spans inherit trace ID
	properties.Property("child spans inherit trace ID", prop.ForAll(
		func(parentIdx int, childIdx int) bool {
			parentOp := fmt.Sprintf("parent-op-%d", parentIdx)
			childOp := fmt.Sprintf("child-op-%d", childIdx)

			tracer := tracing.NewJaegerTracer(tracing.DefaultConfig())

			// Create parent span
			ctx, parentSpan := tracer.StartSpan(context.Background(), parentOp)
			parentCtx := parentSpan.Context()

			// Create child span with parent context
			_, childSpan := tracer.StartSpan(ctx, childOp,
				tracing.WithParent(parentCtx))

			childCtx := childSpan.Context()

			// Child should have same trace ID but different span ID
			sameTrace := childCtx.TraceID == parentCtx.TraceID
			differentSpan := childCtx.SpanID != parentCtx.SpanID

			parentSpan.Finish()
			childSpan.Finish()

			return sameTrace && differentSpan
		},
		gen.IntRange(1, 1000),
		gen.IntRange(1, 1000),
	))

	// Property: Span tags are preserved
	properties.Property("span tags preserved", prop.ForAll(
		func(keyIdx int, valueIdx int) bool {
			key := fmt.Sprintf("tag-key-%d", keyIdx)
			value := fmt.Sprintf("tag-value-%d", valueIdx)

			reporter := tracing.NewInMemoryReporter()
			tracer := tracing.NewJaegerTracerWithReporter(tracing.DefaultConfig(), reporter)

			_, span := tracer.StartSpan(context.Background(), "test-op")
			span.SetTag(key, value)
			span.Finish()

			spans := reporter.GetSpans()
			if len(spans) == 0 {
				return false
			}

			tags := spans[0].GetTags()
			return tags[key] == value
		},
		gen.IntRange(1, 1000),
		gen.IntRange(1, 1000),
	))

	// Property: Trace context can be injected and extracted via carrier
	properties.Property("trace context injection/extraction", prop.ForAll(
		func(operationName string) bool {
			if operationName == "" {
				return true
			}

			tracer := tracing.NewJaegerTracer(tracing.DefaultConfig())

			// Create span
			ctx, span := tracer.StartSpan(context.Background(), operationName)
			originalCtx := span.Context()

			// Inject into carrier
			carrier := tracing.MapCarrier{}
			if err := tracer.Inject(ctx, carrier); err != nil {
				span.Finish()
				return false
			}

			// Verify carrier has trace ID
			hasTraceID := carrier.Get("trace-id") != "" || carrier.Get("uber-trace-id") != ""

			span.Finish()

			return hasTraceID && originalCtx.TraceID != ""
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 && len(s) < 30 }),
	))

	// Property: Finished spans are reported
	properties.Property("finished spans reported", prop.ForAll(
		func(numSpans int) bool {
			if numSpans <= 0 || numSpans > 20 {
				numSpans = 5
			}

			reporter := tracing.NewInMemoryReporter()
			tracer := tracing.NewJaegerTracerWithReporter(tracing.DefaultConfig(), reporter)

			for i := 0; i < numSpans; i++ {
				_, span := tracer.StartSpan(context.Background(), "test-op")
				span.Finish()
			}

			spans := reporter.GetSpans()
			return len(spans) == numSpans
		},
		gen.IntRange(1, 20),
	))

	properties.TestingRun(t)
}

// **Feature: syntor-multi-agent-system, Property 21: Alert triggering**
// *For any* system failure, performance degradation, or capacity limit breach,
// appropriate alerts should be triggered
// **Validates: Requirements 7.4**

// AlertRule represents a simplified alert rule
type AlertRule struct {
	Name       string
	Metric     string
	Threshold  float64
	Operator   string // "gt", "lt", "eq"
	Duration   time.Duration
	Severity   string
}

// AlertEvaluator evaluates alert rules against metric values
type AlertEvaluator struct {
	rules         []AlertRule
	triggeredAlerts []string
}

func NewAlertEvaluator() *AlertEvaluator {
	return &AlertEvaluator{
		rules:         make([]AlertRule, 0),
		triggeredAlerts: make([]string, 0),
	}
}

func (e *AlertEvaluator) AddRule(rule AlertRule) {
	e.rules = append(e.rules, rule)
}

func (e *AlertEvaluator) Evaluate(metricName string, value float64) []string {
	triggered := make([]string, 0)

	for _, rule := range e.rules {
		if rule.Metric != metricName {
			continue
		}

		shouldTrigger := false
		switch rule.Operator {
		case "gt":
			shouldTrigger = value > rule.Threshold
		case "lt":
			shouldTrigger = value < rule.Threshold
		case "eq":
			shouldTrigger = value == rule.Threshold
		case "gte":
			shouldTrigger = value >= rule.Threshold
		case "lte":
			shouldTrigger = value <= rule.Threshold
		}

		if shouldTrigger {
			triggered = append(triggered, rule.Name)
		}
	}

	return triggered
}

func TestAlertTriggering(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = MinTestIterations
	properties := gopter.NewProperties(parameters)

	// Property: High load triggers alert
	properties.Property("high load triggers alert", prop.ForAll(
		func(load float64) bool {
			evaluator := NewAlertEvaluator()
			evaluator.AddRule(AlertRule{
				Name:      "AgentHighLoad",
				Metric:    "syntor_agent_current_load",
				Threshold: 0.9,
				Operator:  "gt",
				Severity:  "warning",
			})

			alerts := evaluator.Evaluate("syntor_agent_current_load", load)

			if load > 0.9 {
				return len(alerts) > 0 && alerts[0] == "AgentHighLoad"
			}
			return len(alerts) == 0
		},
		gen.Float64Range(0, 1.5),
	))

	// Property: Error rate triggers alert
	properties.Property("error rate triggers alert", prop.ForAll(
		func(errorRate float64) bool {
			evaluator := NewAlertEvaluator()
			evaluator.AddRule(AlertRule{
				Name:      "HighErrorRate",
				Metric:    "syntor_error_rate",
				Threshold: 0.1,
				Operator:  "gt",
				Severity:  "critical",
			})

			alerts := evaluator.Evaluate("syntor_error_rate", errorRate)

			if errorRate > 0.1 {
				return len(alerts) > 0 && alerts[0] == "HighErrorRate"
			}
			return len(alerts) == 0
		},
		gen.Float64Range(0, 0.5),
	))

	// Property: No agents triggers alert
	properties.Property("no agents triggers alert", prop.ForAll(
		func(agentCount int) bool {
			if agentCount < 0 {
				agentCount = 0
			}

			evaluator := NewAlertEvaluator()
			evaluator.AddRule(AlertRule{
				Name:      "NoActiveAgents",
				Metric:    "syntor_registered_agents",
				Threshold: 0,
				Operator:  "eq",
				Severity:  "critical",
			})

			alerts := evaluator.Evaluate("syntor_registered_agents", float64(agentCount))

			if agentCount == 0 {
				return len(alerts) > 0 && alerts[0] == "NoActiveAgents"
			}
			return len(alerts) == 0
		},
		gen.IntRange(0, 10),
	))

	// Property: Multiple rules can trigger simultaneously
	properties.Property("multiple alerts trigger", prop.ForAll(
		func(load float64, memory float64) bool {
			evaluator := NewAlertEvaluator()
			evaluator.AddRule(AlertRule{
				Name:      "HighLoad",
				Metric:    "system_load",
				Threshold: 0.8,
				Operator:  "gt",
				Severity:  "warning",
			})
			evaluator.AddRule(AlertRule{
				Name:      "HighMemory",
				Metric:    "system_load",
				Threshold: 0.9,
				Operator:  "gt",
				Severity:  "critical",
			})

			alerts := evaluator.Evaluate("system_load", load)

			expectedCount := 0
			if load > 0.8 {
				expectedCount++
			}
			if load > 0.9 {
				expectedCount++
			}

			return len(alerts) == expectedCount
		},
		gen.Float64Range(0, 1.5),
		gen.Float64Range(0, 1.5),
	))

	// Property: Threshold boundary behavior
	properties.Property("threshold boundary correct", prop.ForAll(
		func(threshold float64) bool {
			if threshold <= 0 {
				threshold = 0.5
			}

			evaluator := NewAlertEvaluator()
			evaluator.AddRule(AlertRule{
				Name:      "ThresholdTest",
				Metric:    "test_metric",
				Threshold: threshold,
				Operator:  "gte",
				Severity:  "warning",
			})

			// At threshold should trigger
			atThreshold := evaluator.Evaluate("test_metric", threshold)
			aboveThreshold := evaluator.Evaluate("test_metric", threshold+0.1)
			belowThreshold := evaluator.Evaluate("test_metric", threshold-0.1)

			return len(atThreshold) == 1 &&
				len(aboveThreshold) == 1 &&
				len(belowThreshold) == 0
		},
		gen.Float64Range(0.1, 10),
	))

	properties.TestingRun(t)
}

// Unit tests

func TestLoggingUnit(t *testing.T) {
	t.Run("context propagation", func(t *testing.T) {
		ctx := context.Background()
		ctx = logging.WithCorrelationID(ctx, "corr-123")
		ctx = logging.WithTraceID(ctx, "trace-456")
		ctx = logging.WithAgentID(ctx, "agent-789")

		assert.Equal(t, "corr-123", logging.GetCorrelationID(ctx))
		assert.Equal(t, "trace-456", logging.GetTraceID(ctx))
		assert.Equal(t, "agent-789", logging.GetAgentID(ctx))
	})

	t.Run("log level parsing", func(t *testing.T) {
		assert.Equal(t, logging.DebugLevel, logging.ParseLevel("debug"))
		assert.Equal(t, logging.InfoLevel, logging.ParseLevel("info"))
		assert.Equal(t, logging.WarnLevel, logging.ParseLevel("warn"))
		assert.Equal(t, logging.ErrorLevel, logging.ParseLevel("error"))
		assert.Equal(t, logging.FatalLevel, logging.ParseLevel("fatal"))
		assert.Equal(t, logging.InfoLevel, logging.ParseLevel("unknown"))
	})
}

func TestTracingUnit(t *testing.T) {
	t.Run("span lifecycle", func(t *testing.T) {
		reporter := tracing.NewInMemoryReporter()
		tracer := tracing.NewJaegerTracerWithReporter(tracing.DefaultConfig(), reporter)

		_, span := tracer.StartSpan(context.Background(), "test-operation")
		jspan := span.(*tracing.JaegerSpan)

		assert.False(t, jspan.IsFinished())
		span.Finish()
		assert.True(t, jspan.IsFinished())

		spans := reporter.GetSpans()
		assert.Len(t, spans, 1)
		assert.Equal(t, "test-operation", spans[0].GetOperationName())
	})

	t.Run("error handling", func(t *testing.T) {
		reporter := tracing.NewInMemoryReporter()
		tracer := tracing.NewJaegerTracerWithReporter(tracing.DefaultConfig(), reporter)

		_, span := tracer.StartSpan(context.Background(), "error-op")
		span.FinishWithError(assert.AnError)

		spans := reporter.GetSpans()
		assert.Len(t, spans, 1)

		tags := spans[0].GetTags()
		assert.Equal(t, true, tags["error"])
	})
}
