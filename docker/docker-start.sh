#!/bin/bash
# PinAI Subnet - 3 Nodes Docker Startup Script
# Starts a 3-validator Raft cluster with Docker Compose

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

echo ""
echo "🐳 PinAI Subnet - Docker Quick Start"
echo "===================================="
echo "(Default 3-node Raft cluster)"
echo ""

# Get project root (parent directory)
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

# Check Docker
if ! command -v docker &> /dev/null; then
    print_error "Docker is not installed!"
    exit 1
fi

if command -v docker compose &> /dev/null; then
    DOCKER_COMPOSE="docker compose"
else
    DOCKER_COMPOSE="docker-compose"
fi

print_success "Docker environment check passed"
echo ""

# Check .env file
if [ ! -f ".env" ]; then
    print_error ".env file does not exist"
    echo ""
    echo "Please create configuration file:"
    echo "  cp docs/env_template.txt .env"
    echo "  nano .env"
    echo ""
    print_warning "⚠️  Important: You need to configure different keys for 3 validators"
    echo ""
    echo "Variables to set:"
    echo "  1. TEST_PRIVATE_KEY (Matcher key)"
    echo "  2. VALIDATOR_KEY_1, VALIDATOR_PUBKEY_1 (or VALIDATOR_KEYS, VALIDATOR_PUBKEYS)"
    echo "  3. VALIDATOR_KEY_2, VALIDATOR_PUBKEY_2"
    echo "  4. VALIDATOR_KEY_3, VALIDATOR_PUBKEY_3"
    echo "  5. SUBNET_ID (must be an already created Subnet ID)"
    echo ""
    echo "⚠️  Important: Keys must be already registered on blockchain!"
    echo ""
    echo "Complete registration process:"
    echo "  1. Create Subnet:     ./scripts/create-subnet.sh"
    echo "  2. Generate keys:     openssl rand -hex 32 (generate 3 times)"
    echo "  3. Register Validator: ./scripts/register.sh --subnet <ADDRESS> --key <KEY>"
    echo "  4. Configure .env:    nano .env (fill in registered keys)"
    echo "  5. Start services:    cd docker && ./docker-start.sh"
    echo ""
    exit 1
fi

# Load and validate .env
set -a
source .env
set +a

print_success "Environment variables loaded"
echo ""

# ============================================
# Support both comma-separated and individual variables
# ============================================
if [ -n "$VALIDATOR_KEYS" ]; then
    # Convert comma-separated keys to individual variables
    print_info "Detected comma-separated keys, converting..."
    IFS=',' read -ra KEYS_ARRAY <<< "$VALIDATOR_KEYS"
    IFS=',' read -ra PUBKEYS_ARRAY <<< "$VALIDATOR_PUBKEYS"
    
    export VALIDATOR_KEY_1="${KEYS_ARRAY[0]}"
    export VALIDATOR_KEY_2="${KEYS_ARRAY[1]}"
    export VALIDATOR_KEY_3="${KEYS_ARRAY[2]}"
    export VALIDATOR_PUBKEY_1="${PUBKEYS_ARRAY[0]}"
    export VALIDATOR_PUBKEY_2="${PUBKEYS_ARRAY[1]}"
    export VALIDATOR_PUBKEY_3="${PUBKEYS_ARRAY[2]}"
    
    print_success "Key conversion completed"
fi
echo ""

# Check required variables
MISSING_VARS=()

[ -z "$TEST_PRIVATE_KEY" ] && MISSING_VARS+=("TEST_PRIVATE_KEY")
[ -z "$VALIDATOR_KEY_1" ] && MISSING_VARS+=("VALIDATOR_KEY_1")
[ -z "$VALIDATOR_PUBKEY_1" ] && MISSING_VARS+=("VALIDATOR_PUBKEY_1")
[ -z "$VALIDATOR_KEY_2" ] && MISSING_VARS+=("VALIDATOR_KEY_2")
[ -z "$VALIDATOR_PUBKEY_2" ] && MISSING_VARS+=("VALIDATOR_PUBKEY_2")
[ -z "$VALIDATOR_KEY_3" ] && MISSING_VARS+=("VALIDATOR_KEY_3")
[ -z "$VALIDATOR_PUBKEY_3" ] && MISSING_VARS+=("VALIDATOR_PUBKEY_3")
[ -z "$SUBNET_ID" ] && MISSING_VARS+=("SUBNET_ID")

if [ ${#MISSING_VARS[@]} -gt 0 ]; then
    print_error "Missing required environment variables:"
    for var in "${MISSING_VARS[@]}"; do
        echo "  - $var"
    done
    echo ""
    echo "Two formats supported:"
    echo "  Format 1 (comma-separated):"
    echo "    VALIDATOR_KEYS=key1,key2,key3"
    echo "    VALIDATOR_PUBKEYS=pubkey1,pubkey2,pubkey3"
    echo ""
    echo "  Format 2 (individual variables):"
    echo "    VALIDATOR_KEY_1=key1"
    echo "    VALIDATOR_KEY_2=key2"
    echo "    VALIDATOR_KEY_3=key3"
    echo "    VALIDATOR_PUBKEY_1=pubkey1"
    echo "    VALIDATOR_PUBKEY_2=pubkey2"
    echo "    VALIDATOR_PUBKEY_3=pubkey3"
    echo ""
    exit 1
fi

print_success "All required variables configured"
echo ""

# Show configuration
echo "📋 Current Configuration:"
echo "   Subnet ID: $SUBNET_ID"
echo "   RootLayer gRPC: $ROOTLAYER_GRPC"
echo "   Validators: 3 nodes"
echo "   Threshold: 2/3 (at least 2 nodes must agree)"
echo ""

# Create directories
print_info "Creating data directories..."
mkdir -p data/registry data/matcher data/validator-1 data/validator-2 data/validator-3 subnet-logs
print_success "Data directories created"
echo ""

# Ask about agent
read -p "Start Agent service? (y/n) [default: n] " -n 1 -r
echo
START_AGENT=false
PROFILE_FLAG=""
if [[ $REPLY =~ ^[Yy]$ ]]; then
    START_AGENT=true
    PROFILE_FLAG="--profile with-agent"
    print_info "Will start Agent service"
fi
echo ""

# Build
print_info "Building Docker images..."
if $DOCKER_COMPOSE -f docker/docker-compose.yml build; then
    print_success "Image build completed"
else
    print_error "Image build failed"
    exit 1
fi
echo ""

# Start
print_info "Starting 3-node Raft cluster..."
if $DOCKER_COMPOSE -f docker/docker-compose.yml $PROFILE_FLAG up -d; then
    print_success "Cluster started successfully!"
else
    print_error "Cluster startup failed"
    exit 1
fi
echo ""

# Wait for services
print_info "Waiting for services to be ready..."
sleep 10

# Show status
echo ""
echo "📊 Service Status:"
$DOCKER_COMPOSE -f docker/docker-compose.yml ps
echo ""

# Health checks
print_info "Checking service health..."
sleep 5

# Check Registry
if curl -s http://localhost:8101/health > /dev/null 2>&1; then
    print_success "Registry: Healthy ✓"
else
    print_warning "Registry: Not responding"
fi

# Check Matcher
if curl -s http://localhost:8092/health > /dev/null 2>&1; then
    print_success "Matcher: Healthy ✓"
else
    print_warning "Matcher: Not responding"
fi

# Check Validators
for i in 1 2 3; do
    port=$((9089 + i))
    if nc -z localhost $port 2>/dev/null; then
        print_success "Validator-$i: Running ✓ (port $port)"
    else
        print_warning "Validator-$i: Port $port not open"
    fi
done

echo ""

# Summary
echo "======================================"
echo -e "${GREEN}🎉 3-node Raft cluster deployed!${NC}"
echo "======================================"
echo ""
echo "🌐 Service Addresses:"
echo "   Registry:     http://localhost:8101"
echo "   Matcher:      http://localhost:8092"
echo "   Validator-1:  localhost:9090 (Raft: 7400, Gossip: 7950)"
echo "   Validator-2:  localhost:9091 (Raft: 7401, Gossip: 7951)"
echo "   Validator-3:  localhost:9092 (Raft: 7402, Gossip: 7952)"
echo ""
echo "📋 Common Commands:"
echo "   View all logs:        $DOCKER_COMPOSE -f docker/docker-compose.yml logs -f"
echo "   View specific node:   $DOCKER_COMPOSE -f docker/docker-compose.yml logs -f validator-1"
echo "   View status:          $DOCKER_COMPOSE -f docker/docker-compose.yml ps"
echo "   Stop services:        $DOCKER_COMPOSE -f docker/docker-compose.yml down"
echo "   Restart services:     $DOCKER_COMPOSE -f docker/docker-compose.yml restart"
echo ""
echo "🔍 Verify Raft Consensus:"
echo "   # View Raft status in validator-1 logs"
echo "   $DOCKER_COMPOSE -f docker/docker-compose.yml logs validator-1 | grep -i 'raft\\|leader'"
echo ""
echo "   # View all validator logs"
echo "   $DOCKER_COMPOSE -f docker/docker-compose.yml logs -f validator-1 validator-2 validator-3"
echo ""
echo "📊 Resource Usage:"
echo "   docker stats"
echo ""

# Ask to follow logs
read -p "View logs in real-time? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo ""
    echo "Press Ctrl+C to exit log view (services will continue running)"
    echo ""
    sleep 2
    $DOCKER_COMPOSE -f docker/docker-compose.yml logs -f
fi

echo ""
print_success "3-node Raft cluster is running!"
echo ""
