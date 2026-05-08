# syntor

Multi-agent AI orchestration system in Go, built on Kafka, Redis, and a rich terminal UI.

> **Note:** This is the Go predecessor to [agnt](https://github.com/hmbldv/agnt), the author's current Rust-based agent runtime. Syntor demonstrated the multi-agent orchestration pattern at scale; agnt evolved the approach into a more composable, security-forward design.

## Quick Start

```bash
git clone https://github.com/hmbldv/syntor.git
cd syntor
make quickstart
syntor init
syntor
```

## Features

- **Terminal UI** — syntax-highlighted code blocks, markdown rendering, keyboard-driven interface
- **Multi-agent coordination** — specialized agents with YAML manifests and dynamic prompt building
- **Tool execution** — read/write files, execute commands, search code (auto or approval-gated)
- **Auto/Plan modes** — toggle between autonomous execution and plan-approval workflows (`Ctrl+A`)
- **Multiple LLM backends** — Ollama (local), Anthropic Claude, DeepSeek; per-agent model assignment
- **Kafka message bus** — event-driven inter-agent communication
- **Redis registry** — agent state and discovery
- **Observability** — Prometheus metrics, Grafana dashboards, Jaeger distributed tracing
- **Custom agents** — YAML manifests with hot-reload, custom slash commands via Markdown templates

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

## Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.23+ |
| Message bus | Apache Kafka |
| State / registry | Redis |
| Persistence | PostgreSQL |
| Metrics | Prometheus + Grafana |
| Tracing | Jaeger |
| Local inference | Ollama |
| Cloud inference | Anthropic Claude, DeepSeek |
| Terminal UI | Bubbletea |

## CLI Usage

```bash
syntor              # TUI mode (default)
syntor --simple     # Simple REPL mode
```

### Slash Commands

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/sntr` | SNTR orchestrator agent |
| `/docs` | Documentation agent |
| `/git` | Git operations agent |
| `/worker` | General worker agent |
| `/copy [n]` | Copy code block n to clipboard |
| `/models` | List available models |
| `/status` | Current agent and model |
| `/clear` | Clear screen |

### Direct Commands

```bash
syntor chat "explain this code"
syntor sntr "analyze the codebase structure"
syntor docs "generate docs for pkg/inference"
syntor git "create a commit message for staged changes"
syntor worker "summarize this file"
```

### Model Management

```bash
syntor models list
syntor models pull mistral:7b
syntor models assign docs deepseek-coder-v2:16b
```

## Agent Manifests

Agents are defined in YAML and hot-reloaded without restart. Place manifests in:
- `~/.syntor/agents/` — global
- `.syntor/agents/` — project-local (overrides global)

```yaml
apiVersion: syntor.dev/v1
kind: Agent
metadata:
  name: security-reviewer
  description: "Security code review specialist"
spec:
  type: specialist
  model:
    default: deepseek-coder-v2:16b
  prompt:
    system: |
      You are a security specialist. Review code for SQL injection,
      XSS, authentication issues, and sensitive data exposure.
  handoff:
    allowedTargets: [sntr, code]
    protocol: structured
```

## Infrastructure

```bash
make dev              # Start infrastructure (Kafka, Redis, Postgres, Grafana, Jaeger)
make topics-create    # Create Kafka topics
make docker-up        # Start all agents
```

| Service | URL |
|---------|-----|
| Kafka UI | http://localhost:8090 |
| Grafana | http://localhost:3000 |
| Prometheus | http://localhost:9091 |
| Jaeger | http://localhost:16686 |

## Project Structure

```
syntor/
├── cmd/
│   ├── syntor/          # CLI entry point
│   ├── coordination/    # Coordination agent
│   ├── docservice/      # Documentation agent
│   ├── git/             # Git agent
│   └── worker/          # Worker agent
├── internal/            # Private packages
├── pkg/
│   ├── inference/       # LLM providers (Ollama, Anthropic, DeepSeek)
│   ├── tools/           # Tool execution + security
│   ├── manifest/        # YAML manifests with hot-reload
│   ├── kafka/           # Message bus
│   ├── registry/        # Agent discovery
│   ├── metrics/         # Prometheus
│   └── tracing/         # Jaeger
└── configs/
    ├── agents/          # Agent YAML files
    ├── prometheus/
    └── grafana/
```

## Development

```bash
make build             # Build all agents
make syntor-build      # Build CLI only
make test              # All tests
make check             # fmt + vet + lint
make help              # All targets
```

## License

MIT — see [LICENSE](LICENSE).
