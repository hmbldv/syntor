package metrics

import (
	"time"
)

// Collector interface for metrics collection
type Collector interface {
	// Counters
	IncrementCounter(name string, labels map[string]string)
	AddCounter(name string, value float64, labels map[string]string)

	// Gauges
	SetGauge(name string, value float64, labels map[string]string)
	IncrementGauge(name string, labels map[string]string)
	DecrementGauge(name string, labels map[string]string)

	// Histograms
	ObserveHistogram(name string, value float64, labels map[string]string)
	ObserveDuration(name string, start time.Time, labels map[string]string)

	// Registry
	Register(metric Metric) error
	Unregister(name string) error

	// HTTP handler for scraping
	Handler() interface{}
}

// Metric represents a metric definition
type Metric struct {
	Name        string
	Type        MetricType
	Help        string
	Labels      []string
	Buckets     []float64 // For histograms
	Objectives  map[float64]float64 // For summaries
}

// MetricType represents the type of metric
type MetricType string

const (
	CounterType   MetricType = "counter"
	GaugeType     MetricType = "gauge"
	HistogramType MetricType = "histogram"
	SummaryType   MetricType = "summary"
)

// Standard SYNTOR metrics
var (
	// Agent metrics
	AgentTasksProcessed = Metric{
		Name:   "syntor_agent_tasks_processed_total",
		Type:   CounterType,
		Help:   "Total number of tasks processed by the agent",
		Labels: []string{"agent_id", "agent_type", "status"},
	}

	AgentTaskDuration = Metric{
		Name:    "syntor_agent_task_duration_seconds",
		Type:    HistogramType,
		Help:    "Duration of task processing in seconds",
		Labels:  []string{"agent_id", "agent_type", "task_type"},
		Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}

	AgentCurrentLoad = Metric{
		Name:   "syntor_agent_current_load",
		Type:   GaugeType,
		Help:   "Current load of the agent (0-1)",
		Labels: []string{"agent_id", "agent_type"},
	}

	AgentActiveTasks = Metric{
		Name:   "syntor_agent_active_tasks",
		Type:   GaugeType,
		Help:   "Number of currently active tasks",
		Labels: []string{"agent_id", "agent_type"},
	}

	// Message bus metrics
	MessagesSent = Metric{
		Name:   "syntor_messages_sent_total",
		Type:   CounterType,
		Help:   "Total number of messages sent",
		Labels: []string{"agent_id", "topic", "message_type"},
	}

	MessagesReceived = Metric{
		Name:   "syntor_messages_received_total",
		Type:   CounterType,
		Help:   "Total number of messages received",
		Labels: []string{"agent_id", "topic", "message_type"},
	}

	MessageLatency = Metric{
		Name:    "syntor_message_latency_seconds",
		Type:    HistogramType,
		Help:    "Message processing latency in seconds",
		Labels:  []string{"agent_id", "topic"},
		Buckets: []float64{.0001, .0005, .001, .005, .01, .05, .1, .5, 1},
	}

	// Registry metrics
	RegisteredAgents = Metric{
		Name:   "syntor_registered_agents",
		Type:   GaugeType,
		Help:   "Number of registered agents",
		Labels: []string{"agent_type", "status"},
	}

	RegistryOperations = Metric{
		Name:   "syntor_registry_operations_total",
		Type:   CounterType,
		Help:   "Total number of registry operations",
		Labels: []string{"operation", "status"},
	}

	// System metrics
	SystemUptime = Metric{
		Name:   "syntor_system_uptime_seconds",
		Type:   GaugeType,
		Help:   "System uptime in seconds",
		Labels: []string{},
	}

	SystemErrors = Metric{
		Name:   "syntor_system_errors_total",
		Type:   CounterType,
		Help:   "Total number of system errors",
		Labels: []string{"component", "error_type"},
	}
)

// Labels creates a labels map from key-value pairs
func Labels(kvs ...string) map[string]string {
	labels := make(map[string]string)
	for i := 0; i < len(kvs)-1; i += 2 {
		labels[kvs[i]] = kvs[i+1]
	}
	return labels
}
