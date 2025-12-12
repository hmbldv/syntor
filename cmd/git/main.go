package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/syntor/syntor/internal/git"
	"github.com/syntor/syntor/pkg/agent"
	"github.com/syntor/syntor/pkg/config"
	"github.com/syntor/syntor/pkg/kafka"
	"github.com/syntor/syntor/pkg/registry"
)

var version = "dev"

func main() {
	fmt.Printf("SYNTOR Git Operations Agent v%s\n", version)

	// Load configuration
	agentConfig := agent.Config{
		ID:   config.GetEnv("SYNTOR_AGENT_ID", "git-agent-1"),
		Name: config.GetEnv("SYNTOR_AGENT_NAME", "git-agent"),
	}

	gitConfig := git.Config{
		AgentConfig:       agentConfig,
		ReposPath:         config.GetEnv("SYNTOR_REPOS_PATH", "/app/repos"),
		HeartbeatInterval: 30 * time.Second,
		SyncInterval:      5 * time.Minute,
		GitAuthor:         config.GetEnv("SYNTOR_GIT_AUTHOR", "SYNTOR Git Agent"),
		GitEmail:          config.GetEnv("SYNTOR_GIT_EMAIL", "syntor@local"),
	}

	// Initialize Kafka
	kafkaConfig := kafka.BusConfig{
		Brokers:  []string{config.GetEnv("SYNTOR_KAFKA_BROKERS", "localhost:9092")},
		Producer: kafka.DefaultProducerConfig(),
		Consumer: kafka.DefaultConsumerConfig(),
	}
	kafkaConfig.Consumer.GroupID = "syntor-git"

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

	gitAgent := git.New(gitConfig, reg, kafkaClient)

	fmt.Println("Starting Git Operations Agent...")
	if err := gitAgent.Start(ctx); err != nil {
		fmt.Printf("Failed to start agent: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Git Operations Agent started (ID: %s)\n", gitAgent.ID())
	fmt.Printf("Repos path: %s\n", gitConfig.ReposPath)
	fmt.Println("Press Ctrl+C to stop")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	gitAgent.Stop(shutdownCtx)
	kafkaClient.Close()
	reg.Close()

	fmt.Println("Git Operations Agent stopped")
}
