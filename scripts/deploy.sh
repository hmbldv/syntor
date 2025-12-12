#!/bin/bash
set -e

# SYNTOR Deployment Script
# This script handles local deployment of the SYNTOR multi-agent system

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    # Check Docker
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed. Please install Docker first."
        exit 1
    fi

    # Check Docker Compose
    if ! command -v docker-compose &> /dev/null; then
        log_error "Docker Compose is not installed. Please install Docker Compose first."
        exit 1
    fi

    # Check Go
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed. Please install Go 1.21+ first."
        exit 1
    fi

    log_info "All prerequisites met."
}

# Build all services
build_services() {
    log_info "Building services..."

    cd "$PROJECT_ROOT"

    # Build Go binaries
    log_info "Building Go binaries..."
    go build -o bin/coordination ./cmd/coordination
    go build -o bin/docservice ./cmd/docservice
    go build -o bin/git ./cmd/git
    go build -o bin/worker ./cmd/worker
    go build -o bin/syntor-cli ./cmd/syntor-cli

    log_info "Build complete."
}

# Build Docker images
build_images() {
    log_info "Building Docker images..."

    cd "$PROJECT_ROOT"

    docker-compose build

    log_info "Docker images built successfully."
}

# Start infrastructure services
start_infrastructure() {
    log_info "Starting infrastructure services..."

    cd "$PROJECT_ROOT"

    # Start infrastructure only
    docker-compose up -d zookeeper kafka redis postgres prometheus grafana jaeger

    # Wait for services to be ready
    log_info "Waiting for infrastructure services to be ready..."
    sleep 10

    # Health check
    check_infrastructure_health
}

# Check infrastructure health
check_infrastructure_health() {
    log_info "Checking infrastructure health..."

    # Check Kafka
    if docker-compose exec -T kafka kafka-topics --bootstrap-server localhost:9092 --list &> /dev/null; then
        log_info "Kafka is ready."
    else
        log_warn "Kafka may not be fully ready yet."
    fi

    # Check Redis
    if docker-compose exec -T redis redis-cli ping &> /dev/null; then
        log_info "Redis is ready."
    else
        log_warn "Redis may not be fully ready yet."
    fi
}

# Start all agents
start_agents() {
    log_info "Starting agents..."

    cd "$PROJECT_ROOT"

    docker-compose up -d coordination docservice git worker

    log_info "Agents started."
}

# Stop all services
stop_all() {
    log_info "Stopping all services..."

    cd "$PROJECT_ROOT"

    docker-compose down

    log_info "All services stopped."
}

# Show status
show_status() {
    log_info "Service Status:"

    cd "$PROJECT_ROOT"

    docker-compose ps
}

# View logs
view_logs() {
    local service=$1

    cd "$PROJECT_ROOT"

    if [ -z "$service" ]; then
        docker-compose logs -f
    else
        docker-compose logs -f "$service"
    fi
}

# Run tests
run_tests() {
    log_info "Running tests..."

    cd "$PROJECT_ROOT"

    # Run unit tests
    log_info "Running unit tests..."
    go test ./pkg/... -v

    # Run property tests
    log_info "Running property tests..."
    go test ./test/property/... -v

    # Run integration tests
    log_info "Running integration tests..."
    go test ./test/integration/... -v

    log_info "All tests passed."
}

# Full deployment
full_deploy() {
    check_prerequisites
    build_services
    build_images
    start_infrastructure
    start_agents
    show_status
}

# Show help
show_help() {
    echo "SYNTOR Deployment Script"
    echo ""
    echo "Usage: $0 <command>"
    echo ""
    echo "Commands:"
    echo "  deploy      Full deployment (build and start all services)"
    echo "  build       Build all services"
    echo "  build-images Build Docker images"
    echo "  start       Start all services"
    echo "  start-infra Start infrastructure only"
    echo "  start-agents Start agent services"
    echo "  stop        Stop all services"
    echo "  status      Show service status"
    echo "  logs [svc]  View logs (optionally for specific service)"
    echo "  test        Run all tests"
    echo "  help        Show this help message"
}

# Main
case "$1" in
    deploy)
        full_deploy
        ;;
    build)
        build_services
        ;;
    build-images)
        build_images
        ;;
    start)
        start_infrastructure
        start_agents
        ;;
    start-infra)
        start_infrastructure
        ;;
    start-agents)
        start_agents
        ;;
    stop)
        stop_all
        ;;
    status)
        show_status
        ;;
    logs)
        view_logs "$2"
        ;;
    test)
        run_tests
        ;;
    help|*)
        show_help
        ;;
esac
