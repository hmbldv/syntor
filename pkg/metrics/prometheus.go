package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusCollector implements Collector using Prometheus client
type PrometheusCollector struct {
	registry   *prometheus.Registry
	counters   map[string]*prometheus.CounterVec
	gauges     map[string]*prometheus.GaugeVec
	histograms map[string]*prometheus.HistogramVec
	summaries  map[string]*prometheus.SummaryVec
	metrics    map[string]Metric
	mu         sync.RWMutex
}

// NewPrometheusCollector creates a new Prometheus-based metrics collector
func NewPrometheusCollector() *PrometheusCollector {
	registry := prometheus.NewRegistry()

	// Register default Go metrics
	registry.MustRegister(prometheus.NewGoCollector())
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))

	return &PrometheusCollector{
		registry:   registry,
		counters:   make(map[string]*prometheus.CounterVec),
		gauges:     make(map[string]*prometheus.GaugeVec),
		histograms: make(map[string]*prometheus.HistogramVec),
		summaries:  make(map[string]*prometheus.SummaryVec),
		metrics:    make(map[string]Metric),
	}
}

// Register registers a new metric
func (c *PrometheusCollector) Register(metric Metric) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.metrics[metric.Name]; exists {
		return fmt.Errorf("metric %s already registered", metric.Name)
	}

	switch metric.Type {
	case CounterType:
		counter := prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: metric.Name,
				Help: metric.Help,
			},
			metric.Labels,
		)
		if err := c.registry.Register(counter); err != nil {
			return fmt.Errorf("failed to register counter %s: %w", metric.Name, err)
		}
		c.counters[metric.Name] = counter

	case GaugeType:
		gauge := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: metric.Name,
				Help: metric.Help,
			},
			metric.Labels,
		)
		if err := c.registry.Register(gauge); err != nil {
			return fmt.Errorf("failed to register gauge %s: %w", metric.Name, err)
		}
		c.gauges[metric.Name] = gauge

	case HistogramType:
		buckets := metric.Buckets
		if len(buckets) == 0 {
			buckets = prometheus.DefBuckets
		}
		histogram := prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    metric.Name,
				Help:    metric.Help,
				Buckets: buckets,
			},
			metric.Labels,
		)
		if err := c.registry.Register(histogram); err != nil {
			return fmt.Errorf("failed to register histogram %s: %w", metric.Name, err)
		}
		c.histograms[metric.Name] = histogram

	case SummaryType:
		objectives := metric.Objectives
		if len(objectives) == 0 {
			objectives = map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
		}
		summary := prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Name:       metric.Name,
				Help:       metric.Help,
				Objectives: objectives,
			},
			metric.Labels,
		)
		if err := c.registry.Register(summary); err != nil {
			return fmt.Errorf("failed to register summary %s: %w", metric.Name, err)
		}
		c.summaries[metric.Name] = summary

	default:
		return fmt.Errorf("unknown metric type: %s", metric.Type)
	}

	c.metrics[metric.Name] = metric
	return nil
}

// Unregister removes a metric from the collector
func (c *PrometheusCollector) Unregister(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	metric, exists := c.metrics[name]
	if !exists {
		return fmt.Errorf("metric %s not found", name)
	}

	var collector prometheus.Collector
	switch metric.Type {
	case CounterType:
		collector = c.counters[name]
		delete(c.counters, name)
	case GaugeType:
		collector = c.gauges[name]
		delete(c.gauges, name)
	case HistogramType:
		collector = c.histograms[name]
		delete(c.histograms, name)
	case SummaryType:
		collector = c.summaries[name]
		delete(c.summaries, name)
	}

	if collector != nil {
		c.registry.Unregister(collector)
	}

	delete(c.metrics, name)
	return nil
}

// IncrementCounter increments a counter by 1
func (c *PrometheusCollector) IncrementCounter(name string, labels map[string]string) {
	c.mu.RLock()
	counter, exists := c.counters[name]
	c.mu.RUnlock()

	if !exists {
		return
	}

	counter.With(prometheus.Labels(labels)).Inc()
}

// AddCounter adds a value to a counter
func (c *PrometheusCollector) AddCounter(name string, value float64, labels map[string]string) {
	c.mu.RLock()
	counter, exists := c.counters[name]
	c.mu.RUnlock()

	if !exists {
		return
	}

	counter.With(prometheus.Labels(labels)).Add(value)
}

// SetGauge sets the value of a gauge
func (c *PrometheusCollector) SetGauge(name string, value float64, labels map[string]string) {
	c.mu.RLock()
	gauge, exists := c.gauges[name]
	c.mu.RUnlock()

	if !exists {
		return
	}

	gauge.With(prometheus.Labels(labels)).Set(value)
}

// IncrementGauge increments a gauge by 1
func (c *PrometheusCollector) IncrementGauge(name string, labels map[string]string) {
	c.mu.RLock()
	gauge, exists := c.gauges[name]
	c.mu.RUnlock()

	if !exists {
		return
	}

	gauge.With(prometheus.Labels(labels)).Inc()
}

// DecrementGauge decrements a gauge by 1
func (c *PrometheusCollector) DecrementGauge(name string, labels map[string]string) {
	c.mu.RLock()
	gauge, exists := c.gauges[name]
	c.mu.RUnlock()

	if !exists {
		return
	}

	gauge.With(prometheus.Labels(labels)).Dec()
}

// ObserveHistogram records a value in a histogram
func (c *PrometheusCollector) ObserveHistogram(name string, value float64, labels map[string]string) {
	c.mu.RLock()
	histogram, exists := c.histograms[name]
	c.mu.RUnlock()

	if !exists {
		return
	}

	histogram.With(prometheus.Labels(labels)).Observe(value)
}

// ObserveDuration records a duration in a histogram
func (c *PrometheusCollector) ObserveDuration(name string, start time.Time, labels map[string]string) {
	duration := time.Since(start).Seconds()
	c.ObserveHistogram(name, duration, labels)
}

// Handler returns an HTTP handler for Prometheus scraping
func (c *PrometheusCollector) Handler() interface{} {
	return promhttp.HandlerFor(c.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// HTTPHandler returns the handler as http.Handler type
func (c *PrometheusCollector) HTTPHandler() http.Handler {
	return c.Handler().(http.Handler)
}

// GetMetricNames returns a list of registered metric names
func (c *PrometheusCollector) GetMetricNames() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	names := make([]string, 0, len(c.metrics))
	for name := range c.metrics {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// IsRegistered checks if a metric is registered
func (c *PrometheusCollector) IsRegistered(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exists := c.metrics[name]
	return exists
}

// RegisterStandardMetrics registers all standard SYNTOR metrics
func (c *PrometheusCollector) RegisterStandardMetrics() error {
	standardMetrics := []Metric{
		AgentTasksProcessed,
		AgentTaskDuration,
		AgentCurrentLoad,
		AgentActiveTasks,
		MessagesSent,
		MessagesReceived,
		MessageLatency,
		RegisteredAgents,
		RegistryOperations,
		SystemUptime,
		SystemErrors,
	}

	var errors []string
	for _, metric := range standardMetrics {
		if err := c.Register(metric); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to register some metrics: %s", strings.Join(errors, "; "))
	}

	return nil
}

// Global collector instance
var globalCollector Collector

// SetGlobalCollector sets the global metrics collector
func SetGlobalCollector(collector Collector) {
	globalCollector = collector
}

// GetGlobalCollector returns the global metrics collector
func GetGlobalCollector() Collector {
	if globalCollector == nil {
		collector := NewPrometheusCollector()
		collector.RegisterStandardMetrics()
		globalCollector = collector
	}
	return globalCollector
}
