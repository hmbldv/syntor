# SYNTOR - Synthetic Orchestrator

A high-performance multi-agent system built with Go, Docker, and Apache Kafka for scalable, event-driven AI operations.

## Overview

SYNTOR (Synthetic Orchestrator) is designed with a "local-first, cloud-ready" philosophy - fully functional on a single Ubuntu server while maintaining architectural patterns needed for seamless AWS migration.

### Key Features

- **High Performance**: Go-based agents with goroutines for efficient concurrency
- **Event-Driven**: Apache Kafka message bus for reliable, scalable messaging
- **Containerized**: Docker containers with multi-stage builds for minimal images
- **Observable**: Prometheus metrics, Grafana dashboards, Jaeger distributed tracing
- **Fault Tolerant**: Circuit breakers, retry logic, and automatic recovery
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

## Agent Types

### Service Agents
- **Coordination Agent**: Task routing, load balancing, agent failover
- **Documentation Agent**: Documentation generation and management
- **Git Operations Agent**: Version control operations

### Worker Agents
- Project-specific task execution
- Horizontally scalable
- Capability-based task routing

## Prerequisites

- Go 1.22+
- Docker & Docker Compose
- Make

## Quick Start

### 1. Clone and Setup

```bash
git clone https://github.com/syntor/syntor.git
cd syntor
```

### 2. Start Infrastructure

```bash
# Start Kafka, Redis, PostgreSQL, and monitoring stack
make dev

# Create Kafka topics
make topics-create
```

### 3. Access Services

| Service | URL | Credentials |
|---------|-----|-------------|
| Kafka UI | http://localhost:8090 | - |
| Grafana | http://localhost:3000 | admin / syntor_admin |
| Prometheus | http://localhost:9091 | - |
| Jaeger | http://localhost:16686 | - |

### 4. Run Agents Locally

```bash
# Run a specific agent
make run-agent AGENT=coordination
```

### 5. Full Docker Deployment

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

Agents are configured via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `SYNTOR_ENVIRONMENT` | Environment (local/staging/production) | local |
| `SYNTOR_LOG_LEVEL` | Log level (debug/info/warn/error) | info |
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
