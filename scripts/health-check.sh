#!/bin/bash
set -e

# SYNTOR Health Check Script
# Validates system health across all components

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

HEALTHY=true

check_service() {
    local name=$1
    local url=$2
    local expected_status=${3:-200}

    if curl -s -o /dev/null -w "%{http_code}" "$url" | grep -q "$expected_status"; then
        echo -e "${GREEN}[OK]${NC} $name"
        return 0
    else
        echo -e "${RED}[FAIL]${NC} $name"
        HEALTHY=false
        return 1
    fi
}

check_docker_service() {
    local name=$1

    if docker-compose -f "$PROJECT_ROOT/docker-compose.yml" ps "$name" 2>/dev/null | grep -q "Up"; then
        echo -e "${GREEN}[OK]${NC} Docker: $name"
        return 0
    else
        echo -e "${RED}[FAIL]${NC} Docker: $name"
        HEALTHY=false
        return 1
    fi
}

echo "======================================"
echo "SYNTOR System Health Check"
echo "======================================"
echo ""

# Check Docker services
echo "Docker Services:"
check_docker_service "kafka" || true
check_docker_service "redis" || true
check_docker_service "postgres" || true
check_docker_service "prometheus" || true
check_docker_service "grafana" || true
check_docker_service "jaeger" || true
echo ""

# Check HTTP endpoints
echo "HTTP Endpoints:"
check_service "Prometheus" "http://localhost:9090/-/healthy" 200 || true
check_service "Grafana" "http://localhost:3000/api/health" 200 || true
check_service "Jaeger" "http://localhost:16686/" 200 || true
echo ""

# Check agent endpoints (if running)
echo "Agent Health:"
check_service "Coordination Agent" "http://localhost:8081/health" 200 2>/dev/null || echo -e "${YELLOW}[SKIP]${NC} Coordination Agent (not running)"
check_service "Documentation Agent" "http://localhost:8082/health" 200 2>/dev/null || echo -e "${YELLOW}[SKIP]${NC} Documentation Agent (not running)"
check_service "Git Agent" "http://localhost:8083/health" 200 2>/dev/null || echo -e "${YELLOW}[SKIP]${NC} Git Agent (not running)"
check_service "Worker Agent" "http://localhost:8084/health" 200 2>/dev/null || echo -e "${YELLOW}[SKIP]${NC} Worker Agent (not running)"
echo ""

# Summary
echo "======================================"
if $HEALTHY; then
    echo -e "${GREEN}Overall Status: HEALTHY${NC}"
    exit 0
else
    echo -e "${RED}Overall Status: UNHEALTHY${NC}"
    exit 1
fi
