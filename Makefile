# SYNTOR Multi-Agent System Makefile

.PHONY: all build clean test lint fmt run stop logs help
.PHONY: docker-build docker-up docker-down docker-logs docker-clean
.PHONY: infra-up infra-down agents-up agents-down
.PHONY: dev dev-stop proto generate
.PHONY: syntor syntor-build syntor-install ollama-up ollama-down

# Alias for convenience
syntor: syntor-build

# Variables
BINARY_NAME=syntor
GO=go
DOCKER_COMPOSE=docker compose
GOFLAGS=-ldflags="-s -w"
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Build directories
BUILD_DIR=./build
CMD_DIR=./cmd

# Agent binaries
AGENTS=coordination documentation git worker

# Colors for output
GREEN=\033[0;32m
YELLOW=\033[0;33m
RED=\033[0;31m
NC=\033[0m # No Color

# Default target
all: build

## help: Show this help message
help:
	@echo "SYNTOR Multi-Agent System"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Quick Start:"
	@echo "  quickstart      Full quickstart (Ollama + syntor build + install)"
	@echo ""
	@echo "SYNTOR CLI targets:"
	@echo "  syntor-build    Build the syntor CLI binary"
	@echo "  syntor-install  Install syntor to /usr/local/bin"
	@echo "  syntor-docker   Build syntor Docker image"
	@echo ""
	@echo "Ollama targets:"
	@echo "  ollama-up       Start Ollama service"
	@echo "  ollama-down     Stop Ollama service"
	@echo "  ollama-pull     Pull default AI models"
	@echo "  ollama-models   List installed models"
	@echo ""
	@echo "Build targets:"
	@echo "  build           Build all agent binaries"
	@echo "  build-agent     Build specific agent (AGENT=name)"
	@echo "  clean           Remove build artifacts"
	@echo ""
	@echo "Development targets:"
	@echo "  dev             Start development environment (infra only)"
	@echo "  dev-full        Start full development environment (infra + agents)"
	@echo "  dev-stop        Stop development environment"
	@echo "  run-agent       Run agent locally (AGENT=name)"
	@echo ""
	@echo "Docker targets:"
	@echo "  docker-build    Build all Docker images"
	@echo "  docker-up       Start all services"
	@echo "  docker-down     Stop all services"
	@echo "  docker-logs     Show logs (SERVICE=name for specific)"
	@echo "  docker-clean    Remove all containers and volumes"
	@echo ""
	@echo "Infrastructure targets:"
	@echo "  infra-up        Start infrastructure services only"
	@echo "  infra-down      Stop infrastructure services"
	@echo ""
	@echo "Testing targets:"
	@echo "  test            Run all tests"
	@echo "  test-unit       Run unit tests"
	@echo "  test-integration Run integration tests"
	@echo "  test-property   Run property-based tests"
	@echo "  coverage        Generate test coverage report"
	@echo ""
	@echo "Code quality targets:"
	@echo "  lint            Run linter"
	@echo "  fmt             Format code"
	@echo "  vet             Run go vet"
	@echo "  check           Run all checks (lint, vet, fmt)"
	@echo ""
	@echo "Utility targets:"
	@echo "  deps            Download dependencies"
	@echo "  tidy            Tidy go.mod"
	@echo "  generate        Run go generate"

## build: Build all agent binaries
build:
	@mkdir -p $(BUILD_DIR)
	@echo "$(GREEN)Building all agents...$(NC)"
	@for agent in $(AGENTS); do \
		echo "Building $$agent-agent..."; \
		$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$$agent-agent $(CMD_DIR)/$$agent/*.go 2>/dev/null || \
		echo "$(YELLOW)Skipping $$agent-agent (no source yet)$(NC)"; \
	done
	@echo "$(GREEN)Build complete$(NC)"

## build-agent: Build specific agent (AGENT=name)
build-agent:
ifndef AGENT
	$(error AGENT is not set. Usage: make build-agent AGENT=coordination)
endif
	@echo "$(GREEN)Building $(AGENT)-agent...$(NC)"
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(AGENT)-agent $(CMD_DIR)/$(AGENT)/*.go

## clean: Remove build artifacts
clean:
	@echo "$(GREEN)Cleaning build artifacts...$(NC)"
	@rm -rf $(BUILD_DIR)
	@$(GO) clean -cache -testcache

## deps: Download dependencies
deps:
	@echo "$(GREEN)Downloading dependencies...$(NC)"
	$(GO) mod download

## tidy: Tidy go.mod
tidy:
	@echo "$(GREEN)Tidying go.mod...$(NC)"
	$(GO) mod tidy

## test: Run all tests
test:
	@echo "$(GREEN)Running all tests...$(NC)"
	$(GO) test -v -race ./...

## test-unit: Run unit tests
test-unit:
	@echo "$(GREEN)Running unit tests...$(NC)"
	$(GO) test -v -short ./...

## test-integration: Run integration tests
test-integration:
	@echo "$(GREEN)Running integration tests...$(NC)"
	$(GO) test -v -tags=integration ./test/integration/...

## test-property: Run property-based tests
test-property:
	@echo "$(GREEN)Running property-based tests...$(NC)"
	$(GO) test -v -tags=property ./test/property/...

## coverage: Generate test coverage report
coverage:
	@echo "$(GREEN)Generating coverage report...$(NC)"
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## lint: Run linter
lint:
	@echo "$(GREEN)Running linter...$(NC)"
	@which golangci-lint > /dev/null || (echo "$(RED)golangci-lint not installed$(NC)" && exit 1)
	golangci-lint run ./...

## fmt: Format code
fmt:
	@echo "$(GREEN)Formatting code...$(NC)"
	$(GO) fmt ./...
	@which goimports > /dev/null && goimports -w . || true

## vet: Run go vet
vet:
	@echo "$(GREEN)Running go vet...$(NC)"
	$(GO) vet ./...

## check: Run all checks
check: fmt vet lint

## generate: Run go generate
generate:
	@echo "$(GREEN)Running go generate...$(NC)"
	$(GO) generate ./...

# Docker targets

## docker-build: Build all Docker images
docker-build:
	@echo "$(GREEN)Building Docker images...$(NC)"
	$(DOCKER_COMPOSE) build

## docker-up: Start all services
docker-up:
	@echo "$(GREEN)Starting all services...$(NC)"
	$(DOCKER_COMPOSE) up -d
	@echo ""
	@echo "Services available:"
	@echo "  Kafka UI:    http://localhost:8090"
	@echo "  Grafana:     http://localhost:3000 (admin/syntor_admin)"
	@echo "  Prometheus:  http://localhost:9091"
	@echo "  Jaeger:      http://localhost:16686"

## docker-down: Stop all services
docker-down:
	@echo "$(GREEN)Stopping all services...$(NC)"
	$(DOCKER_COMPOSE) down

## docker-logs: Show logs
docker-logs:
ifdef SERVICE
	$(DOCKER_COMPOSE) logs -f $(SERVICE)
else
	$(DOCKER_COMPOSE) logs -f
endif

## docker-clean: Remove all containers and volumes
docker-clean:
	@echo "$(RED)Removing all containers and volumes...$(NC)"
	$(DOCKER_COMPOSE) down -v --remove-orphans
	docker volume prune -f

## docker-ps: Show running containers
docker-ps:
	$(DOCKER_COMPOSE) ps

# Infrastructure targets

## infra-up: Start infrastructure services only
infra-up:
	@echo "$(GREEN)Starting infrastructure services...$(NC)"
	$(DOCKER_COMPOSE) up -d zookeeper kafka kafka-ui redis postgres prometheus grafana jaeger
	@echo ""
	@echo "Waiting for services to be ready..."
	@sleep 10
	@echo ""
	@echo "Infrastructure services available:"
	@echo "  Kafka:       localhost:9092"
	@echo "  Kafka UI:    http://localhost:8090"
	@echo "  Redis:       localhost:6379"
	@echo "  PostgreSQL:  localhost:5432"
	@echo "  Grafana:     http://localhost:3000"
	@echo "  Prometheus:  http://localhost:9091"
	@echo "  Jaeger:      http://localhost:16686"

## infra-down: Stop infrastructure services
infra-down:
	@echo "$(GREEN)Stopping infrastructure services...$(NC)"
	$(DOCKER_COMPOSE) stop zookeeper kafka kafka-ui redis postgres prometheus grafana jaeger

## infra-status: Check infrastructure health
infra-status:
	@echo "Checking infrastructure health..."
	@echo ""
	@echo "Kafka:"
	@docker exec syntor-kafka kafka-broker-api-versions --bootstrap-server localhost:9092 > /dev/null 2>&1 && echo "  ✓ Kafka is healthy" || echo "  ✗ Kafka is not responding"
	@echo ""
	@echo "Redis:"
	@docker exec syntor-redis redis-cli ping > /dev/null 2>&1 && echo "  ✓ Redis is healthy" || echo "  ✗ Redis is not responding"
	@echo ""
	@echo "PostgreSQL:"
	@docker exec syntor-postgres pg_isready -U syntor > /dev/null 2>&1 && echo "  ✓ PostgreSQL is healthy" || echo "  ✗ PostgreSQL is not responding"

# Development targets

## dev: Start development environment (infrastructure only)
dev: infra-up
	@echo ""
	@echo "$(GREEN)Development environment ready!$(NC)"
	@echo "Run agents locally with: make run-agent AGENT=coordination"

## dev-full: Start full development environment
dev-full: docker-up
	@echo ""
	@echo "$(GREEN)Full development environment ready!$(NC)"

## dev-stop: Stop development environment
dev-stop: docker-down

## run-agent: Run agent locally
run-agent:
ifndef AGENT
	$(error AGENT is not set. Usage: make run-agent AGENT=coordination)
endif
	@echo "$(GREEN)Running $(AGENT)-agent locally...$(NC)"
	SYNTOR_ENVIRONMENT=local \
	SYNTOR_LOG_LEVEL=debug \
	SYNTOR_KAFKA_BROKERS=localhost:9092 \
	SYNTOR_REDIS_ADDRESS=localhost:6379 \
	SYNTOR_POSTGRES_HOST=localhost \
	$(GO) run $(CMD_DIR)/$(AGENT)/*.go

# Kafka topics management

## topics-create: Create Kafka topics
topics-create:
	@echo "$(GREEN)Creating Kafka topics...$(NC)"
	docker exec syntor-kafka kafka-topics --create --if-not-exists --bootstrap-server localhost:9092 --topic syntor.tasks.assignment --partitions 3 --replication-factor 1
	docker exec syntor-kafka kafka-topics --create --if-not-exists --bootstrap-server localhost:9092 --topic syntor.tasks.status --partitions 3 --replication-factor 1
	docker exec syntor-kafka kafka-topics --create --if-not-exists --bootstrap-server localhost:9092 --topic syntor.tasks.complete --partitions 3 --replication-factor 1
	docker exec syntor-kafka kafka-topics --create --if-not-exists --bootstrap-server localhost:9092 --topic syntor.agents.registration --partitions 1 --replication-factor 1
	docker exec syntor-kafka kafka-topics --create --if-not-exists --bootstrap-server localhost:9092 --topic syntor.agents.heartbeat --partitions 1 --replication-factor 1
	docker exec syntor-kafka kafka-topics --create --if-not-exists --bootstrap-server localhost:9092 --topic syntor.services.request --partitions 3 --replication-factor 1
	docker exec syntor-kafka kafka-topics --create --if-not-exists --bootstrap-server localhost:9092 --topic syntor.services.response --partitions 3 --replication-factor 1
	docker exec syntor-kafka kafka-topics --create --if-not-exists --bootstrap-server localhost:9092 --topic syntor.system.events --partitions 1 --replication-factor 1
	docker exec syntor-kafka kafka-topics --create --if-not-exists --bootstrap-server localhost:9092 --topic syntor.dlq --partitions 1 --replication-factor 1
	@echo "$(GREEN)Topics created$(NC)"

## topics-list: List Kafka topics
topics-list:
	docker exec syntor-kafka kafka-topics --list --bootstrap-server localhost:9092

# SYNTOR CLI targets

## syntor-build: Build the syntor CLI
syntor-build:
	@mkdir -p $(BUILD_DIR)
	@echo "$(GREEN)Building syntor CLI...$(NC)"
	$(GO) build $(GOFLAGS) \
		-ldflags="-s -w -X 'github.com/syntor/syntor/internal/cli.Version=$(VERSION)' \
		          -X 'github.com/syntor/syntor/internal/cli.GitCommit=$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)' \
		          -X 'github.com/syntor/syntor/internal/cli.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)'" \
		-o $(BUILD_DIR)/syntor ./cmd/syntor
	@echo "$(GREEN)Binary: $(BUILD_DIR)/syntor$(NC)"

## syntor-install: Install syntor CLI to /usr/local/bin
syntor-install: syntor-build
	@echo "$(GREEN)Installing syntor to /usr/local/bin...$(NC)"
	@sudo cp $(BUILD_DIR)/syntor /usr/local/bin/syntor
	@sudo chmod +x /usr/local/bin/syntor
	@echo "$(GREEN)syntor installed successfully$(NC)"
	@echo "Run 'syntor init' to configure first-time setup"

## syntor-docker: Build syntor Docker image
syntor-docker:
	@echo "$(GREEN)Building syntor Docker image...$(NC)"
	docker build -f deployments/docker/Dockerfile.syntor \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown) \
		--build-arg BUILD_TIME=$(shell date -u +%Y-%m-%dT%H:%M:%SZ) \
		-t syntor:$(VERSION) \
		-t syntor:latest \
		.

# Ollama targets

## ollama-up: Start Ollama service
ollama-up:
	@echo "$(GREEN)Starting Ollama service...$(NC)"
	$(DOCKER_COMPOSE) up -d ollama
	@echo "Waiting for Ollama to be ready..."
	@sleep 5
	@echo "$(GREEN)Ollama available at http://localhost:11434$(NC)"

## ollama-down: Stop Ollama service
ollama-down:
	@echo "$(GREEN)Stopping Ollama service...$(NC)"
	$(DOCKER_COMPOSE) stop ollama

## ollama-pull: Pull default models
ollama-pull:
	@echo "$(GREEN)Pulling default models...$(NC)"
	docker exec syntor-ollama ollama pull llama3.2:8b
	docker exec syntor-ollama ollama pull mistral:7b
	docker exec syntor-ollama ollama pull llama3.2:3b
	@echo "$(GREEN)Models pulled successfully$(NC)"

## ollama-models: List installed models
ollama-models:
	@echo "$(GREEN)Installed models:$(NC)"
	docker exec syntor-ollama ollama list

# Quick start target
## quickstart: Full quickstart (Ollama + syntor)
quickstart: ollama-up syntor-build syntor-install
	@echo ""
	@echo "$(GREEN)SYNTOR Quickstart Complete!$(NC)"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Run 'syntor init' to configure first-time setup"
	@echo "  2. Run 'syntor models pull llama3.2:8b' to pull a model"
	@echo "  3. Run 'syntor' to start interactive mode"
	@echo ""
