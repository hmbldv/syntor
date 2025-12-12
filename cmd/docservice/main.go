package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/syntor/syntor/internal/docservice"
	"github.com/syntor/syntor/pkg/agent"
	"github.com/syntor/syntor/pkg/config"
	"github.com/syntor/syntor/pkg/kafka"
	"github.com/syntor/syntor/pkg/registry"
)

var version = "dev"

func main() {
	fmt.Printf("SYNTOR Documentation Agent v%s\n", version)

	// Load configuration
	agentConfig := agent.Config{
		ID:   config.GetEnv("SYNTOR_AGENT_ID", "documentation-agent-1"),
		Name: config.GetEnv("SYNTOR_AGENT_NAME", "documentation-agent"),
	}

	docConfig := docservice.Config{
		AgentConfig:       agentConfig,
		DocsPath:          config.GetEnv("SYNTOR_DOCS_PATH", "/app/docs"),
		HeartbeatInterval: 30 * time.Second,
		IndexInterval:     5 * time.Minute,
		SupportedFormats:  []string{".md", ".txt", ".html", ".rst"},
	}

	// Initialize Kafka
	kafkaConfig := kafka.BusConfig{
		Brokers:  []string{config.GetEnv("SYNTOR_KAFKA_BROKERS", "localhost:9092")},
		Producer: kafka.DefaultProducerConfig(),
		Consumer: kafka.DefaultConsumerConfig(),
	}
	kafkaConfig.Consumer.GroupID = "syntor-documentation"

	kafkaClient := kafka.NewClient(kafkaConfig)

	// Initialize registry
	regConfig := registry.DefaultRegistryConfig()
	regConfig.Redis.Address = config.GetEnv("SYNTOR_REDIS_ADDRESS", "localhost:6379")
	reg := registry.NewRedisRegistry(regConfig)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("Connecting to Kafka...")
	if err := kafkaClient.Connect(ctx); err != nil {
		fmt.Printf("Warning: Failed to connect to Kafka: %v\n", err)
	}

	fmt.Println("Connecting to Redis...")
	if err := reg.Connect(ctx); err != nil {
		fmt.Printf("Warning: Failed to connect to Redis: %v\n", err)
	}

	docAgent := docservice.New(docConfig, reg, kafkaClient)

	fmt.Println("Starting Documentation Agent...")
	if err := docAgent.Start(ctx); err != nil {
		fmt.Printf("Failed to start agent: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Documentation Agent started (ID: %s)\n", docAgent.ID())
	fmt.Printf("Docs path: %s\n", docConfig.DocsPath)
	fmt.Println("Press Ctrl+C to stop")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	docAgent.Stop(shutdownCtx)
	kafkaClient.Close()
	reg.Close()

	fmt.Println("Documentation Agent stopped")
}
