```
░██████╗██╗░░░██╗███╗░░██╗████████╗░█████╗░██████╗░
██╔════╝╚██╗░██╔╝████╗░██║╚══██╔══╝██╔══██╗██╔══██╗
╚█████╗░░╚████╔╝░██╔██╗██║░░░██║░░░██║░░██║██████╔╝
░╚═══██╗░░╚██╔╝░░██║╚████║░░░██║░░░██║░░██║██╔══██╗
██████╔╝░░░██║░░░██║░╚███║░░░██║░░░╚█████╔╝██║░░██║
╚═════╝░░░░╚═╝░░░╚═╝░░╚══╝░░░╚═╝░░░░╚════╝░╚═╝░░╚═╝
                 Synthetic Orchestrator
```

A high-performance multi-agent AI system with a modern terminal UI. Built with Go, Docker, and Apache Kafka for scalable, event-driven AI operations.

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

- **Modern Terminal UI** - Rich TUI with syntax-highlighted code blocks, markdown rendering, and visual styling
- **Multi-Agent Coordination** - Specialized agents with YAML-based manifests and dynamic prompt building
- **Tool Execution** - SNTR agent can read/write files, execute commands, search code (like Claude Code)
- **Auto/Plan Modes** - Toggle between autonomous execution and plan-approval workflows (Ctrl+A)
- **Code Block Support** - Syntax highlighting with `/copy` command for easy clipboard access
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
│  │  │   SNTR      │ │Documentation│   │  │   Worker    │ x N   │    │
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
syntor              # Launch TUI mode (default)
syntor --simple     # Launch simple REPL mode
```

```
◈ SYNTOR ─────────────────────────────────────── Interactive Mode

░██████╗██╗░░░██╗███╗░░██╗████████╗░█████╗░██████╗░
██╔════╝╚██╗░██╔╝████╗░██║╚══██╔══╝██╔══██╗██╔══██╗
╚█████╗░░╚████╔╝░██╔██╗██║░░░██║░░░██║░░██║██████╔╝
░╚═══██╗░░╚██╔╝░░██║╚████║░░░██║░░░██║░░██║██╔══██╗
██████╔╝░░░██║░░░██║░╚███║░░░██║░░░╚█████╔╝██║░░██║
╚═════╝░░░░╚═╝░░░╚═╝░░╚══╝░░░╚═╝░░░░╚════╝░╚═╝░░╚═╝
                   Synthetic Orchestrator

  Type a message to chat with the AI agent
  Use /help for available commands
  Press Ctrl+A to toggle Auto/Plan mode

────────────────────────────────────────────────────────────────
> your message here
────────────────────────────────────────────────────────────────
 Enter send  |  Ctrl+A mode  |  Tab complete  |  /help commands
```

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Ctrl+A` | Toggle Auto/Plan mode |
| `Ctrl+C` | Interrupt/Quit |
| `Tab` | Autocomplete commands |
| `↑/↓` | Scroll chat history |
| `PgUp/PgDn` | Page through history |

### Slash Commands

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/sntr` | Switch to SNTR agent (primary orchestrator) |
| `/docs` | Switch to documentation agent |
| `/git` | Switch to git agent |
| `/worker` | Switch to general worker agent |
| `/code` | Switch to code worker agent |
| `/copy [n]` | Copy code block n to clipboard |
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
syntor sntr "analyze the codebase structure"
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

## Agent Manifests

Agents are defined via YAML manifests that can be hot-reloaded. Create custom agents in:
- `~/.syntor/agents/` - Global agents
- `.syntor/agents/` - Project-specific agents

### Example Manifest

```yaml
apiVersion: syntor.dev/v1
kind: Agent
metadata:
  name: security-reviewer
  description: "Security code review specialist"
spec:
  type: specialist
  capabilities:
    - name: security_audit
      description: "Reviews code for security vulnerabilities"
  model:
    default: deepseek-coder-v2:16b
  prompt:
    system: |
      You are a security specialist. Review code for:
      - SQL injection vulnerabilities
      - XSS attacks
      - Authentication issues
      - Sensitive data exposure

      {{if .ToolPrompt}}
      ## Available Tools
      {{.ToolPrompt}}
      {{end}}
  handoff:
    allowedTargets:
      - sntr
      - code
    protocol: structured
```

### Manifest Hot-Reload

Changes to manifest files are automatically detected and applied without restarting:

```bash
# Edit an agent manifest
vim ~/.syntor/agents/my-agent.yaml

# Changes take effect immediately
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
| SNTR | `mistral:7b` | Primary orchestrator with tool execution |
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
    sntr: mistral:7b
    documentation: deepseek-coder-v2:16b
    git: llama3.2:8b
    worker: llama3.2:3b
    worker_code: qwen2.5-coder:7b

cli:
  theme: auto
  stream_response: true
```

## Tool System

SNTR, the primary orchestrator agent, has access to a set of tools for interacting with the filesystem and executing commands - similar to Claude Code.

### Available Tools

| Tool | Description |
|------|-------------|
| `read_file` | Read file contents with line numbers |
| `write_file` | Create or overwrite files |
| `edit_file` | Find and replace content in files |
| `bash` | Execute shell commands |
| `glob` | Find files by pattern |
| `grep` | Search file contents with regex |
| `list_directory` | List directory contents |

### Auto Mode vs Plan Mode

SNTR operates in two modes (toggle with `Ctrl+A`):

**Auto Mode (Default)**
- Tools execute automatically without approval
- Best for trusted environments and quick tasks
- Maximum productivity for experienced users

**Plan Mode**
- Tool executions require explicit approval
- SNTR presents a plan before execution
- Use `Ctrl+Y` to approve, `Ctrl+N` to reject
- Safer for learning or when extra caution is needed

### Security

The tool system includes security measures:
- **Path validation** - Prevents traversal attacks and access to sensitive files
- **Command allowlist** - Only safe commands permitted in bash tool
- **Working directory scoping** - File operations constrained to project directory
- **Max iteration limit** - 25 tool iterations per request to prevent infinite loops

### Example Interaction

```
> Read the main.go file and add error handling

[Auto Mode] SNTR is executing tools...

Tool: read_file
File: main.go
Result: [file contents displayed]

Tool: edit_file
File: main.go
Change: Added error handling to main function
Result: ✓ File updated

Done. Added try-catch style error handling with proper error messages.
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
│   ├── cli/                 # CLI implementation
│   │   └── tui/             # Terminal UI (bubbletea)
│   ├── coordination/        # Coordination agent logic
│   ├── documentation/       # Documentation agent logic
│   ├── git/                 # Git agent logic
│   └── worker/              # Worker agent logic
├── pkg/
│   ├── inference/           # AI inference layer
│   │   ├── ollama/          # Ollama provider
│   │   ├── anthropic/       # Claude API (stub)
│   │   └── deepseek/        # DeepSeek API (stub)
│   ├── tools/               # Tool execution system
│   │   ├── implementations/ # Tool implementations (read, write, bash, etc.)
│   │   └── security/        # Security validation
│   ├── manifest/            # YAML agent manifests with hot-reload
│   ├── prompt/              # Dynamic prompt builder
│   ├── coordination/        # Agent coordination protocol
│   ├── context/             # Context propagation
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
│   ├── agents/              # Agent manifest YAML files
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
