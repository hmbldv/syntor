package config

import (
	"encoding/json"
	"os"
	"time"

	"github.com/syntor/syntor/pkg/agent"
	"github.com/syntor/syntor/pkg/kafka"
	"github.com/syntor/syntor/pkg/registry"
)

// SystemConfig holds the complete system configuration
type SystemConfig struct {
	System     SystemSettings     `json:"system"`
	Kafka      kafka.BusConfig    `json:"kafka"`
	Registry   registry.RegistryConfig `json:"registry"`
	Monitoring MonitoringConfig   `json:"monitoring"`
	Logging    LoggingConfig      `json:"logging"`
}

// SystemSettings holds general system settings
type SystemSettings struct {
	Environment     string        `json:"environment"` // local, staging, production
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`
	HealthCheckPort int           `json:"health_check_port"`
}

// MonitoringConfig holds monitoring configuration
type MonitoringConfig struct {
	MetricsEnabled bool   `json:"metrics_enabled"`
	MetricsPort    int    `json:"metrics_port"`
	TracingEnabled bool   `json:"tracing_enabled"`
	JaegerEndpoint string `json:"jaeger_endpoint"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level      string `json:"level"` // debug, info, warn, error
	Format     string `json:"format"` // json, text
	OutputPath string `json:"output_path"`
}

// Load loads configuration from a JSON file
func Load(path string) (*SystemConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config SystemConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// LoadAgentConfig loads agent-specific configuration from a JSON file
func LoadAgentConfig(path string) (*agent.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config agent.Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// DefaultSystemConfig returns default system configuration for local development
func DefaultSystemConfig() SystemConfig {
	return SystemConfig{
		System: SystemSettings{
			Environment:     "local",
			ShutdownTimeout: 30 * time.Second,
			HealthCheckPort: 8080,
		},
		Kafka: kafka.BusConfig{
			Brokers:  []string{"localhost:9092"},
			Producer: kafka.DefaultProducerConfig(),
			Consumer: kafka.DefaultConsumerConfig(),
		},
		Registry: registry.DefaultRegistryConfig(),
		Monitoring: MonitoringConfig{
			MetricsEnabled: true,
			MetricsPort:    9090,
			TracingEnabled: true,
			JaegerEndpoint: "http://localhost:14268/api/traces",
		},
		Logging: LoggingConfig{
			Level:      "info",
			Format:     "json",
			OutputPath: "stdout",
		},
	}
}

// GetEnv retrieves environment variable with a default value
func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetEnvInt retrieves environment variable as int with a default value
func GetEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intVal int
		if err := json.Unmarshal([]byte(value), &intVal); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// GetEnvBool retrieves environment variable as bool with a default value
func GetEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}
