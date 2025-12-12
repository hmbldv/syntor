# SYNTOR - Synthetic Orchestrator

A high-performance multi-agent AI system with a Claude Code-like CLI experience. Built with Go, Docker, and Apache Kafka for scalable, event-driven AI operations.

## Quick Start

```bash
# Build and install
make quickstart

# Initialize (first time)
syntor init

# Start interactive mode
syntor

# Or send a direct message
syntor chat "explain this codebase"
```

## Overview

SYNTOR (Synthetic Orchestrator) is designed with a "local-first, cloud-ready" philosophy - fully functional on a single Ubuntu server while maintaining architectural patterns needed for seamless AWS migration.

### Key Features

- **Interactive CLI**: Claude Code-like interface with slash commands
- **Multi-Agent**: Specialized agents for coordination, documentation, git, and general tasks
- **Flexible Models**: Use Ollama locally or API providers (Anthropic, DeepSeek)
- **High Performance**: Go-based agents with goroutines for efficient concurrency
- **Event-Driven**: Apache Kafka message bus for reliable, scalable messaging
- **Containerized**: Docker containers with multi-stage builds for minimal images
- **Observable**: Prometheus metrics, Grafana dashboards, Jaeger distributed tracing
- **Cloud Ready**: Designed for migration to AWS (ECS/EKS, MSK, ElastiCache, RDS)

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    SYNTOR Multi-Agent System                     │
├─────────────────────────────────────────────────────────────────┤
│  Service Agents          │  Worker Agents                       │
│  ┌──────────────────┐    │  ┌──────────────────┐                │
│  │ Coordination     │    │  │ Worker Agent 1   │                │
│  │ Documentation    │    │  │ Worker Agent 2   │                │
│  │ Git Operations   │    │  │ Worker Agent N   │                │
│  └────────┬─────────┘    │  └────────┬─────────┘                │
│           │              │           │                          │
│           └──────────────┴───────────┘                          │
│                          │                                      │
│                    ┌─────▼─────┐                                │
│                    │   Kafka   │                                │
│                    │ Message   │                                │
│                    │   Bus     │                                │
│                    └─────┬─────┘                                │
│                          │                                      │
│  ┌───────────────────────┼───────────────────────┐              │
│  │        Storage        │      Monitoring       │              │
│  │  Redis │ PostgreSQL   │  Prometheus │ Grafana │              │
│  │        │              │  Jaeger              │              │
│  └───────────────────────┴───────────────────────┘              │
└─────────────────────────────────────────────────────────────────┘
```

## CLI Usage

### Interactive Mode

Start an interactive session:

```bash
syntor
```

Available slash commands in interactive mode:
- `/help` - Show available commands
- `/coordination` - Switch to coordination agent
- `/docs` - Switch to documentation agent
- `/git` - Switch to git agent
- `/worker` - Switch to worker agent
- `/code` - Switch to code worker agent
- `/models` - List available models
- `/status` - Show current agent and model
- `/config` - Show configuration
- `/quit` - Exit

### Direct Commands

```bash
# Chat with default agent
syntor chat "explain this code"

# Use specific agents
syntor coordination "analyze the codebase structure"
syntor docs "generate documentation for pkg/inference"
syntor git "create a commit message for staged changes"
syntor worker "summarize this file"
syntor worker --code "refactor this function"

# Model management
syntor models list              # List all models
syntor models status            # Show agent model assignments
syntor models pull mistral:7b   # Pull a model
syntor models assign docs deepseek-coder-v2:16b

# Configuration
syntor config show              # Show current config
syntor config edit              # Edit in your editor
syntor config set provider ollama
```

### Model Configuration

Models can be configured at multiple levels:
1. **Global default**: `~/.syntor/config.yaml`
2. **Project override**: `.syntor/config.yaml`
3. **Per-command**: `syntor docs -m qwen2.5-coder:7b "message"`

### Custom Slash Commands

Create custom commands by adding `.md` files:
- `~/.syntor/commands/` - Global commands
- `.syntor/commands/` - Project-specific commands

Example (`~/.syntor/commands/commit.md`):
```markdown
Analyze the staged git changes and create a conventional commit message.
{{args}}
```

Use with: `/commit` in interactive mode.

## Agent Types

### Service Agents
- **Coordination Agent**: Task routing, orchestration, and planning
- **Documentation Agent**: Documentation generation and code analysis
- **Git Operations Agent**: Version control and commit management

### Worker Agents
- **Worker Agent**: General task execution
- **Code Worker**: Code-specific tasks (refactoring, review, generation)
- Horizontally scalable
- Capability-based task routing

## Prerequisites

- Go 1.23+
- Docker & Docker Compose
- Make
- NVIDIA GPU (optional, for faster inference)

## Installation

### Option 1: Quick Install (Recommended)

```bash
git clone https://github.com/syntor/syntor.git
cd syntor
make quickstart
```

### Option 2: Manual Install

```bash
# Clone repository
git clone https://github.com/syntor/syntor.git
cd syntor

# Start Ollama
make ollama-up

# Build and install CLI
make syntor-install

# Run first-time setup
syntor init

# Pull a model
syntor models pull llama3.2:8b
```

### Option 3: Using Install Script

```bash
./scripts/install.sh
```

## Full Infrastructure Setup

For the complete multi-agent system with Kafka, Redis, and monitoring:

### 1. Start Infrastructure

```bash
# Start all infrastructure (Kafka, Redis, PostgreSQL, monitoring)
make dev

# Create Kafka topics
make topics-create
```

### 2. Access Services

| Service | URL | Credentials |
|---------|-----|-------------|
| Ollama | http://localhost:11434 | - |
| Kafka UI | http://localhost:8090 | - |
| Grafana | http://localhost:3000 | admin / syntor_admin |
| Prometheus | http://localhost:9091 | - |
| Jaeger | http://localhost:16686 | - |

### 3. Full Docker Deployment

```bash
# Build and start all services including agents
make docker-up
```

## Project Structure

```
syntor/
├── cmd/                    # Agent entry points
│   ├── coordination/       # Coordination agent
│   ├── documentation/      # Documentation agent
│   ├── git/               # Git operations agent
│   └── worker/            # Worker agent
├── pkg/                   # Public packages
│   ├── agent/             # Agent interfaces
│   ├── kafka/             # Message bus
│   ├── registry/          # Agent discovery
│   ├── models/            # Core data types
│   ├── config/            # Configuration
│   ├── logging/           # Structured logging
│   ├── metrics/           # Prometheus metrics
│   └── tracing/           # Distributed tracing
├── internal/              # Private agent implementations
├── configs/               # Configuration files
│   ├── prometheus/        # Prometheus config & rules
│   └── grafana/           # Dashboards & datasources
├── deployments/           # Deployment configurations
│   ├── docker/            # Dockerfiles
│   ├── kubernetes/        # K8s manifests (future)
│   └── terraform/         # AWS IaC (future)
├── scripts/               # Utility scripts
└── test/                  # Integration & property tests
```

## Development

### Building

```bash
# Build all agents
make build

# Build specific agent
make build-agent AGENT=coordination
```

### Testing

```bash
# Run all tests
make test

# Run with coverage
make coverage

# Run property-based tests
make test-property
```

### Code Quality

```bash
# Format, vet, and lint
make check
```

## Configuration

### CLI Configuration

SYNTOR uses YAML configuration files:

```yaml
# ~/.syntor/config.yaml (global)
# .syntor/config.yaml (project-level override)

inference:
  provider: ollama
  ollama_host: http://localhost:11434
  default_model: llama3.2:8b
  models:
    coordination: mistral:7b
    documentation: deepseek-coder-v2:16b
    git: llama3.2:8b
    worker: llama3.2:3b
    worker_code: qwen2.5-coder:7b
  auto_pull: true

cli:
  theme: auto
  stream_response: true
```

### AI Models

Default model assignments:

| Agent | Model | Purpose |
|-------|-------|---------|
| Coordination | `mistral:7b` | Task orchestration and planning |
| Documentation | `deepseek-coder-v2:16b` | Code analysis and docs |
| Git | `llama3.2:8b` | Commit messages and git ops |
| Worker | `llama3.2:3b` | General tasks |
| Code Worker | `qwen2.5-coder:7b` | Code-specific tasks |

Change models with:
```bash
syntor models assign coordination mistral:7b
syntor config set default_model llama3.2:8b
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SYNTOR_ENVIRONMENT` | Environment (local/staging/production) | local |
| `SYNTOR_LOG_LEVEL` | Log level (debug/info/warn/error) | info |
| `SYNTOR_OLLAMA_HOST` | Ollama API endpoint | http://localhost:11434 |
| `SYNTOR_INFERENCE_MODEL` | Default inference model | llama3.2:8b |
| `SYNTOR_KAFKA_BROKERS` | Kafka broker addresses | localhost:9092 |
| `SYNTOR_REDIS_ADDRESS` | Redis address | localhost:6379 |
| `SYNTOR_POSTGRES_HOST` | PostgreSQL host | localhost |
| `SYNTOR_METRICS_PORT` | Prometheus metrics port | 9090 |
| `SYNTOR_HEALTH_PORT` | Health check port | 8080 |

## Kafka Topics

| Topic | Purpose |
|-------|---------|
| `syntor.tasks.assignment` | Task assignment to agents |
| `syntor.tasks.status` | Task status updates |
| `syntor.tasks.complete` | Task completion notifications |
| `syntor.agents.registration` | Agent registration events |
| `syntor.agents.heartbeat` | Agent heartbeat signals |
| `syntor.services.request` | Service request messages |
| `syntor.services.response` | Service response messages |
| `syntor.system.events` | System-wide events |
| `syntor.dlq` | Dead letter queue |

## AWS Migration Path

SYNTOR is designed for seamless migration to AWS:

| Local Service | AWS Service |
|---------------|-------------|
| Docker Compose | Amazon ECS/EKS |
| Apache Kafka | Amazon MSK |
| Redis | Amazon ElastiCache |
| PostgreSQL | Amazon RDS |
| Prometheus/Grafana | Amazon CloudWatch |
| Jaeger | AWS X-Ray |

## License

MIT License - see [LICENSE](LICENSE) for details.
