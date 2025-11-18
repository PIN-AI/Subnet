#!/bin/bash
# Start a local agent that connects to Docker services
# This agent runs on the host machine and connects to containerized services

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

print_info() { echo -e "${BLUE}ℹ${NC} $1"; }
print_success() { echo -e "${GREEN}✓${NC} $1"; }
print_error() { echo -e "${RED}✗${NC} $1"; }
print_warning() { echo -e "${YELLOW}⚠${NC} $1"; }

# Project root (parent directory of scripts/)
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

echo ""
echo "🤖 PinAI Subnet - Local Agent Launcher"
echo "======================================"
echo ""

# Load environment variables
if [ -f ".env" ]; then
    set -a
    source .env
    set +a
    print_success "Environment variables loaded"
else
    print_warning ".env file not found, using default values"
fi

# Default values
MATCHER_ADDR="${MATCHER_ADDR:-localhost:8093}"
VALIDATOR_ADDR="${VALIDATOR_ADDR:-localhost:9090}"
# REGISTRY_ADDR - deprecated, registry service removed
SUBNET_ID="${SUBNET_ID:-0x0000000000000000000000000000000000000000000000000000000000000002}"

# Agent configuration
AGENT_ID="${1:-local-agent-$(date +%s)}"
AGENT_NAME="${2:-LocalAgent}"
AGENT_TYPE="${3:-simple}"  # simple or advanced

echo ""
print_info "Configuration:"
echo "  Matcher:    $MATCHER_ADDR"
echo "  Validator:  $VALIDATOR_ADDR"
echo "  Subnet ID:  $SUBNET_ID"
echo "  Agent ID:   $AGENT_ID"
echo "  Agent Name: $AGENT_NAME"
echo "  Agent Type: $AGENT_TYPE"
echo ""

# Check if services are running
print_info "Checking service connectivity..."

if nc -z localhost 8093 2>/dev/null; then
    print_success "Matcher is reachable"
else
    print_error "Cannot reach Matcher at localhost:8093"
    print_warning "Make sure Docker services are running: cd docker && ./docker-start.sh"
    exit 1
fi

if nc -z localhost 9090 2>/dev/null; then
    print_success "Validator is reachable"
else
    print_warning "Cannot reach Validator at localhost:9090"
fi

# Build if binary doesn't exist
if [ ! -f "bin/simple-agent" ]; then
    print_info "Building agent..."
    make build-simple-agent
fi

echo ""
print_info "Starting $AGENT_TYPE agent..."
echo ""

# Start appropriate agent type
if [ "$AGENT_TYPE" = "advanced" ]; then
    # Advanced agent with actual task execution
    if [ ! -f "examples/advanced-agent/main.go" ]; then
        print_error "Advanced agent not found"
        exit 1
    fi
    
    cd examples/advanced-agent
    exec go run main.go \
        -matcher "$MATCHER_ADDR" \
        -validator "$VALIDATOR_ADDR" \
        -subnet-id "$SUBNET_ID" \
        -agent-id "$AGENT_ID" \
        -agent-name "$AGENT_NAME"
else
    # Simple agent (bidding only)
    exec ./bin/simple-agent \
        -matcher "$MATCHER_ADDR" \
        -validator "$VALIDATOR_ADDR" \
        -subnet-id "$SUBNET_ID" \
        -id "$AGENT_ID" \
        -name "$AGENT_NAME"
fi

