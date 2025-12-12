# SYNTOR - Synthetic Orchestrator

A high-performance multi-agent AI system with a Claude Code-like CLI experience. Built with Go, Docker, and Apache Kafka for scalable, event-driven AI operations.

## Quick Start

```bash
# Clone and install
git clone https://github.com/syntor/syntor.git
cd syntor
make quickstart

# Initialize (first time)
syntor init

# Start interactive mode
syntor
```

## Features

- **Interactive CLI** - Claude Code-like interface with slash commands and streaming responses
- **Multi-Agent System** - Specialized agents for coordination, documentation, git, and code tasks
- **Flexible AI Models** - Use Ollama locally or API providers (Anthropic Claude, DeepSeek)
- **Per-Agent Models** - Assign different models to different agents based on task requirements
- **Custom Commands** - Create your own slash commands with markdown templates
- **High Performance** - Go-based with goroutines, connection pooling, and sub-millisecond latency
- **Event-Driven** - Apache Kafka message bus for reliable, scalable messaging
- **Observable** - Prometheus metrics, Grafana dashboards, Jaeger distributed tracing
- **Cloud Ready** - Designed for seamless migration to AWS (ECS/EKS, MSK, ElastiCache)

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                      SYNTOR Multi-Agent System                       │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌─────────────┐     ┌─────────────────────────────────────────┐    │
│  │   syntor    │     │            AI Inference Layer            │    │
│  │    CLI      │────▶│  Ollama │ Anthropic API │ DeepSeek API  │    │
│  └─────────────┘     └─────────────────────────────────────────┘    │
│                                       │                              │
│  ┌────────────────────────────────────┼────────────────────────┐    │
│  │              Service Agents        │      Worker Agents     │    │
│  │  ┌─────────────┐ ┌─────────────┐   │  ┌─────────────┐       │    │
│  │  │Coordination │ │Documentation│   │  │   Worker    │ x N   │    │
│  │  │   Agent     │ │   Agent     │   │  │   Agents    │       │    │
│  │  └──────┬──────┘ └──────┬──────┘   │  └──────┬──────┘       │    │
│  │         │               │          │         │              │    │
│  │  ┌──────┴───────────────┴──────────┴─────────┴──────┐       │    │
│  │  │              Kafka Message Bus                    │       │    │
│  │  └──────────────────────┬────────────────────────────┘       │    │
│  └─────────────────────────┼────────────────────────────────────┘    │
│                            │                                         │
│  ┌─────────────────────────┼─────────────────────────────────────┐  │
│  │   Redis    │  PostgreSQL │  Prometheus  │  Grafana  │  Jaeger │  │
│  └─────────────────────────┴─────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

## CLI Usage

### Interactive Mode

```bash
syntor
```

```
╔══════════════════════════════════════════════════════════════╗
║                    SYNTOR Interactive Mode                    ║
╠══════════════════════════════════════════════════════════════╣
║  Type /help for commands, /quit to exit                      ║
║  Current agent: coordination                                  ║
╚══════════════════════════════════════════════════════════════╝

coordination> analyze the project structure
```

### Slash Commands

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/coordination` | Switch to coordination agent |
| `/docs` | Switch to documentation agent |
| `/git` | Switch to git agent |
| `/worker` | Switch to general worker agent |
| `/code` | Switch to code worker agent |
| `/models` | List available models |
| `/status` | Show current agent and model |
| `/config` | Show configuration |
| `/clear` | Clear the screen |
| `/quit` | Exit interactive mode |

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

# Override model for a single command
syntor docs -m qwen2.5-coder:7b "document this package"
```

### Model Management

```bash
syntor models list              # List all available models
syntor models status            # Show agent model assignments
syntor models pull mistral:7b   # Pull/download a model
syntor models assign docs deepseek-coder-v2:16b  # Assign model to agent
```

### Configuration

```bash
syntor config show              # Show current configuration
syntor config edit              # Open config in your editor
syntor config path              # Show config file locations
syntor config set provider ollama
syntor config set default_model llama3.2:8b
```

### Custom Slash Commands

Create custom commands by adding `.md` files:
- `~/.syntor/commands/` - Global commands
- `.syntor/commands/` - Project-specific commands (override global)

Example (`~/.syntor/commands/review.md`):
```markdown
Review the following code for potential issues.

Check for:
1. Bugs and logic errors
2. Security vulnerabilities
3. Performance concerns

{{args}}
```

Use with `/review <code>` in interactive mode.

## Installation

### Prerequisites

- Go 1.23+
- Docker & Docker Compose
- Make
- NVIDIA GPU (optional, for faster local inference)

### Option 1: Quick Install

```bash
git clone https://github.com/syntor/syntor.git
cd syntor
make quickstart
```

This will:
1. Start Ollama with GPU support
2. Build the syntor CLI
3. Install to `/usr/local/bin`

### Option 2: Manual Install

```bash
# Clone repository
git clone https://github.com/syntor/syntor.git
cd syntor

# Start Ollama
make ollama-up

# Build CLI
make syntor-build

# Install (optional)
sudo cp build/syntor /usr/local/bin/

# Run setup wizard
syntor init

# Pull a model
syntor models pull llama3.2:8b
```

### Option 3: Install Script

```bash
./scripts/install.sh
```

## AI Models

### Default Assignments

| Agent | Model | Purpose |
|-------|-------|---------|
| Coordination | `mistral:7b` | Task orchestration and planning |
| Documentation | `deepseek-coder-v2:16b` | Code analysis and documentation |
| Git | `llama3.2:8b` | Commit messages and git operations |
| Worker | `llama3.2:3b` | General tasks |
| Code Worker | `qwen2.5-coder:7b` | Code-specific tasks |

### Configuration Hierarchy

Models can be configured at multiple levels (lower overrides higher):
1. **Global default** - `~/.syntor/config.yaml`
2. **Project override** - `.syntor/config.yaml`
3. **Per-command** - `syntor docs -m <model> "message"`

### Example Configuration

```yaml
# ~/.syntor/config.yaml
inference:
  provider: ollama
  ollama_host: http://localhost:11434
  default_model: llama3.2:8b
  auto_pull: true
  models:
    coordination: mistral:7b
    documentation: deepseek-coder-v2:16b
    git: llama3.2:8b
    worker: llama3.2:3b
    worker_code: qwen2.5-coder:7b

cli:
  theme: auto
  stream_response: true
```

## Full Infrastructure

For the complete multi-agent system with Kafka, Redis, and monitoring:

```bash
# Start infrastructure
make dev

# Create Kafka topics
make topics-create

# Start all agents
make docker-up
```

### Service URLs

| Service | URL | Credentials |
|---------|-----|-------------|
| Ollama | http://localhost:11434 | - |
| Kafka UI | http://localhost:8090 | - |
| Grafana | http://localhost:3000 | admin / syntor_admin |
| Prometheus | http://localhost:9091 | - |
| Jaeger | http://localhost:16686 | - |

## Project Structure

```
syntor/
├── cmd/
│   ├── syntor/              # Main CLI entry point
│   ├── coordination/        # Coordination agent
│   ├── docservice/          # Documentation agent
│   ├── git/                 # Git operations agent
│   └── worker/              # Worker agent
├── internal/
│   ├── cli/                 # CLI implementation (REPL, commands)
│   ├── coordination/        # Coordination agent logic
│   ├── documentation/       # Documentation agent logic
│   ├── git/                 # Git agent logic
│   └── worker/              # Worker agent logic
├── pkg/
│   ├── inference/           # AI inference layer
│   │   ├── ollama/          # Ollama provider
│   │   ├── anthropic/       # Claude API (stub)
│   │   └── deepseek/        # DeepSeek API (stub)
│   ├── config/              # Configuration management
│   ├── setup/               # Initialization helpers
│   ├── agent/               # Agent interfaces
│   ├── kafka/               # Message bus
│   ├── registry/            # Agent discovery
│   ├── resilience/          # Circuit breakers, retry
│   ├── performance/         # Pooling, batching, scaling
│   ├── logging/             # Structured logging
│   ├── metrics/             # Prometheus metrics
│   └── tracing/             # Distributed tracing
├── configs/
│   ├── commands/            # Example custom commands
│   ├── prometheus/          # Prometheus config
│   └── grafana/             # Dashboards
├── deployments/docker/      # Dockerfiles
├── scripts/                 # Utility scripts
└── test/                    # Integration & property tests
```

## Development

```bash
# Build all agents
make build

# Build syntor CLI
make syntor-build

# Run tests
make test

# Run with coverage
make coverage

# Format and lint
make check

# View all make targets
make help
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SYNTOR_OLLAMA_HOST` | Ollama API endpoint | http://localhost:11434 |
| `SYNTOR_INFERENCE_MODEL` | Default model | llama3.2:8b |
| `SYNTOR_LOG_LEVEL` | Log level | info |
| `SYNTOR_KAFKA_BROKERS` | Kafka brokers | localhost:9092 |
| `SYNTOR_REDIS_ADDRESS` | Redis address | localhost:6379 |

## AWS Migration Path

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
