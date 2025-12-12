#!/bin/bash
# SYNTOR Installation Script
# This script installs syntor and its dependencies

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case $ARCH in
    x86_64)
        ARCH="amd64"
        ;;
    arm64|aarch64)
        ARCH="arm64"
        ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

echo -e "${BLUE}"
echo "╔══════════════════════════════════════════════════════════════╗"
echo "║                  SYNTOR Installation Script                   ║"
echo "╚══════════════════════════════════════════════════════════════╝"
echo -e "${NC}"

echo "Detected OS: $OS"
echo "Detected Architecture: $ARCH"
echo ""

# Check prerequisites
check_prereqs() {
    echo -e "${YELLOW}Checking prerequisites...${NC}"

    # Check for Go
    if ! command -v go &> /dev/null; then
        echo -e "${RED}Go is not installed.${NC}"
        echo "Please install Go from https://go.dev/dl/"
        exit 1
    fi
    echo -e "  ${GREEN}✓${NC} Go $(go version | cut -d' ' -f3)"

    # Check for Docker
    if ! command -v docker &> /dev/null; then
        echo -e "${YELLOW}  ⚠ Docker not found (optional for containerized Ollama)${NC}"
    else
        echo -e "  ${GREEN}✓${NC} Docker $(docker --version | cut -d' ' -f3 | tr -d ',')"
    fi

    # Check for Docker Compose
    if ! command -v docker &> /dev/null || ! docker compose version &> /dev/null; then
        echo -e "${YELLOW}  ⚠ Docker Compose not found (optional for containerized Ollama)${NC}"
    else
        echo -e "  ${GREEN}✓${NC} Docker Compose $(docker compose version | cut -d' ' -f4)"
    fi

    echo ""
}

# Build syntor
build_syntor() {
    echo -e "${YELLOW}Building syntor...${NC}"

    # Get version info
    VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
    GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

    # Build
    CGO_ENABLED=0 go build \
        -ldflags="-s -w -X 'github.com/syntor/syntor/internal/cli.Version=$VERSION' \
                  -X 'github.com/syntor/syntor/internal/cli.GitCommit=$GIT_COMMIT' \
                  -X 'github.com/syntor/syntor/internal/cli.BuildTime=$BUILD_TIME'" \
        -o build/syntor ./cmd/syntor

    echo -e "  ${GREEN}✓${NC} Built syntor $VERSION"
    echo ""
}

# Install syntor
install_syntor() {
    echo -e "${YELLOW}Installing syntor...${NC}"

    INSTALL_DIR="/usr/local/bin"

    if [ -w "$INSTALL_DIR" ]; then
        cp build/syntor "$INSTALL_DIR/syntor"
    else
        echo "  Requires sudo to install to $INSTALL_DIR"
        sudo cp build/syntor "$INSTALL_DIR/syntor"
        sudo chmod +x "$INSTALL_DIR/syntor"
    fi

    echo -e "  ${GREEN}✓${NC} Installed to $INSTALL_DIR/syntor"
    echo ""
}

# Setup Ollama
setup_ollama() {
    echo -e "${YELLOW}Setting up Ollama...${NC}"

    if command -v ollama &> /dev/null; then
        echo -e "  ${GREEN}✓${NC} Ollama is already installed"
        return
    fi

    if command -v docker &> /dev/null; then
        echo "  Starting Ollama via Docker..."
        docker compose up -d ollama 2>/dev/null || true
        sleep 5

        if curl -s http://localhost:11434/api/tags &>/dev/null; then
            echo -e "  ${GREEN}✓${NC} Ollama is running via Docker"
        else
            echo -e "${YELLOW}  ⚠ Could not start Ollama. You may need to start it manually.${NC}"
        fi
    else
        echo -e "${YELLOW}  ⚠ Ollama not found. Install from https://ollama.ai${NC}"
    fi

    echo ""
}

# Create default config
create_config() {
    echo -e "${YELLOW}Creating default configuration...${NC}"

    CONFIG_DIR="$HOME/.syntor"
    CONFIG_FILE="$CONFIG_DIR/config.yaml"
    COMMANDS_DIR="$CONFIG_DIR/commands"

    mkdir -p "$CONFIG_DIR"
    mkdir -p "$COMMANDS_DIR"

    if [ ! -f "$CONFIG_FILE" ]; then
        cat > "$CONFIG_FILE" << 'EOF'
# SYNTOR Configuration
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
  editor: vim
  auto_approve: false
  stream_response: true
EOF
        echo -e "  ${GREEN}✓${NC} Created $CONFIG_FILE"
    else
        echo -e "  ${GREEN}✓${NC} Config already exists at $CONFIG_FILE"
    fi

    echo ""
}

# Print completion message
print_complete() {
    echo -e "${GREEN}"
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║                  Installation Complete!                       ║"
    echo "╚══════════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    echo ""
    echo "Quick start:"
    echo "  syntor init              # Run first-time setup"
    echo "  syntor                   # Start interactive mode"
    echo "  syntor --help            # Show all commands"
    echo ""
    echo "Pull a model:"
    echo "  syntor models pull llama3.2:8b"
    echo ""
    echo "Configuration:"
    echo "  ~/.syntor/config.yaml    # Global config"
    echo "  .syntor/config.yaml      # Project config (overrides global)"
    echo ""
}

# Main
main() {
    check_prereqs
    build_syntor
    install_syntor
    setup_ollama
    create_config
    print_complete
}

main "$@"
