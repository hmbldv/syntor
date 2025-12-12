package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/syntor/syntor/internal/coordination"
	"github.com/syntor/syntor/pkg/agent"
	"github.com/syntor/syntor/pkg/config"
	"github.com/syntor/syntor/pkg/kafka"
	"github.com/syntor/syntor/pkg/registry"
)

var version = "dev"

func main() {
	fmt.Printf("SYNTOR Coordination Agent v%s\n", version)

	// Load configuration from environment
	agentConfig := agent.Config{
		ID:   config.GetEnv("SYNTOR_AGENT_ID", "coordination-agent-1"),
		Name: config.GetEnv("SYNTOR_AGENT_NAME", "coordination-agent"),
	}

	coordConfig := coordination.Config{
		AgentConfig:         agentConfig,
		HeartbeatInterval:   30 * time.Second,
		HealthCheckInterval: 10 * time.Second,
		TaskTimeoutCheck:    60 * time.Second,
		FailoverThreshold:   90 * time.Second,
		MaxPendingTasks:     1000,
		ScalingThreshold:    0.8,
	}

	// Initialize Kafka client
	kafkaConfig := kafka.BusConfig{
		Brokers:  []string{config.GetEnv("SYNTOR_KAFKA_BROKERS", "localhost:9092")},
		Producer: kafka.DefaultProducerConfig(),
		Consumer: kafka.DefaultConsumerConfig(),
	}
	kafkaConfig.Consumer.GroupID = "syntor-coordination"

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

	// Create and start agent
	coordAgent := coordination.New(coordConfig, reg, kafkaClient)

	fmt.Println("Starting Coordination Agent...")
	if err := coordAgent.Start(ctx); err != nil {
		fmt.Printf("Failed to start agent: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Coordination Agent started (ID: %s)\n", coordAgent.ID())
	fmt.Println("Press Ctrl+C to stop")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	fmt.Println("\nShutting down...")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := coordAgent.Stop(shutdownCtx); err != nil {
		fmt.Printf("Error during shutdown: %v\n", err)
	}

	kafkaClient.Close()
	reg.Close()

	fmt.Println("Coordination Agent stopped")
}
