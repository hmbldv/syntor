package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/syntor/syntor/internal/worker"
	"github.com/syntor/syntor/pkg/config"
	"github.com/syntor/syntor/pkg/kafka"
	"github.com/syntor/syntor/pkg/models"
	"github.com/syntor/syntor/pkg/agent"
	"github.com/syntor/syntor/pkg/registry"
)

var version = "dev"

func main() {
	fmt.Printf("SYNTOR Worker Agent v%s\n", version)

	// Load configuration from environment
	agentConfig := agent.Config{
		ID:   config.GetEnv("SYNTOR_AGENT_ID", fmt.Sprintf("worker-agent-%d", time.Now().UnixNano()%10000)),
		Name: config.GetEnv("SYNTOR_AGENT_NAME", "worker-agent"),
	}

	workerConfig := worker.Config{
		AgentConfig:        agentConfig,
		MaxConcurrentTasks: config.GetEnvInt("SYNTOR_MAX_CONCURRENT_TASKS", 10),
		TaskTimeout:        5 * time.Minute,
		HeartbeatInterval:  30 * time.Second,
		WorkPath:           config.GetEnv("SYNTOR_WORK_PATH", "/app/work"),
	}

	// Initialize Kafka client
	kafkaConfig := kafka.BusConfig{
		Brokers:  []string{config.GetEnv("SYNTOR_KAFKA_BROKERS", "localhost:9092")},
		Producer: kafka.DefaultProducerConfig(),
		Consumer: kafka.DefaultConsumerConfig(),
	}
	kafkaConfig.Consumer.GroupID = "syntor-workers"

	kafkaClient := kafka.NewClient(kafkaConfig)

	// Initialize Redis registry
	regConfig := registry.DefaultRegistryConfig()
	regConfig.Redis.Address = config.GetEnv("SYNTOR_REDIS_ADDRESS", "localhost:6379")

	reg := registry.NewRedisRegistry(regConfig)

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to dependencies
	fmt.Println("Connecting to Kafka...")
	if err := kafkaClient.Connect(ctx); err != nil {
		fmt.Printf("Warning: Failed to connect to Kafka: %v\n", err)
	}

	fmt.Println("Connecting to Redis registry...")
	if err := reg.Connect(ctx); err != nil {
		fmt.Printf("Warning: Failed to connect to Redis: %v\n", err)
	}

	// Create worker agent
	workerAgent := worker.New(workerConfig, reg, kafkaClient)

	// Register default handlers
	worker.RegisterDefaultHandlers(workerAgent)

	fmt.Println("Starting Worker Agent...")
	if err := workerAgent.Start(ctx); err != nil {
		fmt.Printf("Failed to start agent: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Worker Agent started (ID: %s)\n", workerAgent.ID())
	fmt.Printf("Capabilities: %v\n", getCapabilityNames(workerAgent.GetCapabilities()))
	fmt.Println("Press Ctrl+C to stop")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	fmt.Println("\nShutting down...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := workerAgent.Stop(shutdownCtx); err != nil {
		fmt.Printf("Error during shutdown: %v\n", err)
	}

	kafkaClient.Close()
	reg.Close()

	fmt.Println("Worker Agent stopped")
}

func getCapabilityNames(caps []models.Capability) []string {
	names := make([]string, len(caps))
	for i, c := range caps {
		names[i] = c.Name
	}
	return names
}
