#!/bin/bash
# PinAI Subnet Services Startup Script
# One-click launch for all Subnet services

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Project root
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

print_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

echo ""
echo "🚀 PinAI Subnet Services Launcher"
echo "=================================="
echo ""

# Load environment from .env if exists
if [ -f "$PROJECT_ROOT/.env" ]; then
    print_info "Loading environment from .env..."
    set -a
    source "$PROJECT_ROOT/.env"
    set +a
    print_success "Environment loaded"
else
    print_warning ".env file not found, using system environment variables"
fi

# Check required environment variables
print_info "Checking environment configuration..."
if [ -z "$TEST_PRIVATE_KEY" ]; then
    print_error "TEST_PRIVATE_KEY not set"
    echo ""
    echo "Please set your test private key:"
    echo "  export TEST_PRIVATE_KEY=\"your_test_private_key_here\""
    echo ""
    echo "Or create a .env file (see .env.example)"
    exit 1
fi

# Set default values
SUBNET_ID="${SUBNET_ID:-0x0000000000000000000000000000000000000000000000000000000000000002}"
ROOTLAYER_GRPC="${ROOTLAYER_GRPC:-3.17.208.238:9001}"
ROOTLAYER_HTTP="${ROOTLAYER_HTTP:-http://3.17.208.238:8081}"
ENABLE_CHAIN_SUBMIT="${ENABLE_CHAIN_SUBMIT:-true}"
CHAIN_RPC_URL="${CHAIN_RPC_URL:-https://sepolia.base.org}"
CHAIN_NETWORK="${CHAIN_NETWORK:-base_sepolia}"

# Contract addresses
PIN_BASE_SEPOLIA_INTENT_MANAGER="${PIN_BASE_SEPOLIA_INTENT_MANAGER:-0xD04d23775D3B8e028e6104E31eb0F6c07206EB46}"

print_success "Environment configured"
echo ""
echo "📋 Configuration:"
echo "   Subnet ID: $SUBNET_ID"
echo "   RootLayer gRPC: $ROOTLAYER_GRPC"
echo "   RootLayer HTTP: $ROOTLAYER_HTTP"
echo "   Chain Submit: $ENABLE_CHAIN_SUBMIT"
echo ""

if [ "${SKIP_REGISTRATION_PROMPT:-}" != "1" ]; then
    print_warning "Ensure the target subnet exists on-chain and participants (matcher/validator/agent) have been registered."
    print_warning "Make sure ./scripts/create-subnet.sh and ./scripts/register.sh have been run to finish subnet setup."
    echo ""
    read -r -p "On-chain setup complete? Continue starting services? [y/N]: " CONTINUE_START
    case "$CONTINUE_START" in
        [yY]|[yY][eE][sS])
            ;;
        *)
            print_error "Startup cancelled. Complete subnet creation and participant registration first."
            exit 1
            ;;
    esac
fi

# Clean up old processes
print_info "Cleaning up old processes..."
pkill -f "bin/matcher" 2>/dev/null || true
pkill -f "bin/validator" 2>/dev/null || true
pkill -f "bin/registry" 2>/dev/null || true
pkill -f "bin/test-agent" 2>/dev/null || true
sleep 1
print_success "Cleanup complete"

# Create logs directory
LOGS_DIR="$PROJECT_ROOT/subnet-logs"
mkdir -p "$LOGS_DIR"
print_success "Logs directory: $LOGS_DIR"

# Check NATS
print_info "Checking NATS server..."
if ! lsof -Pi :4222 -sTCP:LISTEN -t >/dev/null 2>&1; then
    print_warning "NATS not running, starting NATS server..."
    nats-server -D > "$LOGS_DIR/nats.log" 2>&1 &
    sleep 2
    if lsof -Pi :4222 -sTCP:LISTEN -t >/dev/null 2>&1; then
        print_success "NATS server started"
    else
        print_error "Failed to start NATS server"
        exit 1
    fi
else
    print_success "NATS server running"
fi

# Check binaries
print_info "Checking binaries..."
MISSING_BINARIES=()
for binary in matcher validator registry; do
    if [ ! -f "$PROJECT_ROOT/bin/$binary" ]; then
        MISSING_BINARIES+=("$binary")
    fi
done

if [ ${#MISSING_BINARIES[@]} -gt 0 ]; then
    print_warning "Missing binaries: ${MISSING_BINARIES[*]}"
    print_info "Building missing binaries..."
    cd "$PROJECT_ROOT"
    make build
    print_success "Build complete"
fi
print_success "All binaries ready"

echo ""
echo "🎬 Starting services..."
echo "=================================="

# Start Registry
print_info "Starting Registry service..."
"$PROJECT_ROOT/bin/registry" \
    -grpc ":8091" \
    -http ":8101" \
    > "$LOGS_DIR/registry.log" 2>&1 &
REGISTRY_PID=$!
sleep 2

if kill -0 $REGISTRY_PID 2>/dev/null; then
    print_success "Registry started (PID: $REGISTRY_PID)"
    echo "   - gRPC: localhost:8091"
    echo "   - HTTP: localhost:8101"
    echo "   - Logs: $LOGS_DIR/registry.log"
else
    print_error "Registry failed to start"
    cat "$LOGS_DIR/registry.log"
    exit 1
fi

# Generate Matcher config
print_info "Generating Matcher configuration..."
MATCHER_CONFIG="$LOGS_DIR/matcher-config.yaml"
cat > "$MATCHER_CONFIG" <<EOF
subnet_id: "$SUBNET_ID"

identity:
  matcher_id: "matcher-main"
  subnet_id: "$SUBNET_ID"

network:
  nats_url: "nats://127.0.0.1:4222"
  matcher_grpc_port: ":8090"

limits:
  bidding_window_sec: 10
  max_bids_per_intent: 100

rootlayer:
  http_url: "$ROOTLAYER_HTTP"
  grpc_endpoint: "$ROOTLAYER_GRPC"
  connect_timeout: 10s
  request_timeout: 30s

agent:
  matcher:
    signer:
      type: "ecdsa"
      private_key: "$TEST_PRIVATE_KEY"

# Private key for matcher
private_key: "$TEST_PRIVATE_KEY"

# On-chain configuration
enable_chain_submit: $ENABLE_CHAIN_SUBMIT
chain_rpc_url: "$CHAIN_RPC_URL"
chain_network: "$CHAIN_NETWORK"
intent_manager_addr: "$PIN_BASE_SEPOLIA_INTENT_MANAGER"
EOF

# Start Matcher
print_info "Starting Matcher service..."
"$PROJECT_ROOT/bin/matcher" \
    --config "$MATCHER_CONFIG" \
    --id "matcher-main" \
    --grpc ":8090" \
    --window 10 \
    > "$LOGS_DIR/matcher.log" 2>&1 &
MATCHER_PID=$!
sleep 2

if kill -0 $MATCHER_PID 2>/dev/null; then
    print_success "Matcher started (PID: $MATCHER_PID)"
    echo "   - gRPC: localhost:8090"
    echo "   - Logs: $LOGS_DIR/matcher.log"
else
    print_error "Matcher failed to start"
    cat "$LOGS_DIR/matcher.log"
    exit 1
fi

# Start Validator (single-node mode for development)
print_info "Starting Validator service (single-node mode)..."
"$PROJECT_ROOT/bin/validator" \
    -id "validator-e2e-1" \
    -grpc 9200 \
    -nats "nats://127.0.0.1:4222" \
    -subnet-id "$SUBNET_ID" \
    -key "${TEST_PRIVATE_KEY#0x}" \
    -rootlayer-endpoint "$ROOTLAYER_GRPC" \
    -registry-grpc "localhost:8091" \
    -registry-http "localhost:8101" \
    -chain-rpc-url "$CHAIN_RPC_URL" \
    -chain-network "$CHAIN_NETWORK" \
    -intent-manager-addr "$PIN_BASE_SEPOLIA_INTENT_MANAGER" \
    -enable-chain-submit \
    -enable-rootlayer \
    -validators 1 \
    -threshold-num 1 \
    -threshold-denom 1 \
    > "$LOGS_DIR/validator.log" 2>&1 &
VALIDATOR_PID=$!
sleep 3

if kill -0 $VALIDATOR_PID 2>/dev/null; then
    print_success "Validator started (PID: $VALIDATOR_PID)"
    echo "   - Node ID: validator-e2e-1"
    echo "   - gRPC: localhost:9200"
    echo "   - Logs: $LOGS_DIR/validator.log"
else
    print_error "Validator failed to start"
    cat "$LOGS_DIR/validator.log"
    exit 1
fi

# Start Test Agent
print_info "Starting Test Agent..."
"$PROJECT_ROOT/bin/test-agent" \
    --agent-id "test-agent-001" \
    --matcher "localhost:8090" \
    --validator "localhost:9200" \
    --subnet-id "$SUBNET_ID" \
    > "$LOGS_DIR/test-agent.log" 2>&1 &
AGENT_PID=$!
sleep 2

if kill -0 $AGENT_PID 2>/dev/null; then
    print_success "Test Agent started (PID: $AGENT_PID)"
    echo "   - Agent ID: test-agent-001"
    echo "   - Logs: $LOGS_DIR/test-agent.log"
else
    print_error "Test Agent failed to start"
    cat "$LOGS_DIR/test-agent.log"
    exit 1
fi

echo ""
echo "=================================="
echo -e "${GREEN}✅ All services started successfully!${NC}"
echo "=================================="
echo ""
echo "📊 Service Status:"
echo "   Registry:  http://localhost:8101"
echo "   Matcher:   localhost:8090 (gRPC)"
echo "   Validator: localhost:9200 (gRPC)"
echo "   Agent:     test-agent-001"
echo ""
echo "📋 Logs:"
echo "   tail -f $LOGS_DIR/registry.log"
echo "   tail -f $LOGS_DIR/matcher.log"
echo "   tail -f $LOGS_DIR/validator.log"
echo "   tail -f $LOGS_DIR/test-agent.log"
echo ""
echo "🛑 To stop all services:"
echo "   pkill -f 'bin/matcher'"
echo "   pkill -f 'bin/validator'"
echo "   pkill -f 'bin/registry'"
echo "   pkill -f 'bin/test-agent'"
echo ""
echo "💡 Next steps:"
echo "   1. Use ./send-intent.sh to submit Intents"
echo "   2. Monitor logs to see the flow"
echo "   3. Check validator consensus in validator.log"
echo ""

# Save PIDs for later reference
echo "$REGISTRY_PID" > "$LOGS_DIR/registry.pid"
echo "$MATCHER_PID" > "$LOGS_DIR/matcher.pid"
echo "$VALIDATOR_PID" > "$LOGS_DIR/validator.pid"

echo "Press Ctrl+C to stop all services..."
echo ""

# Wait for user interrupt
trap 'echo ""; print_info "Stopping services..."; pkill -f "bin/matcher"; pkill -f "bin/validator"; pkill -f "bin/registry"; print_success "All services stopped"; exit 0' INT

# Keep script running
while true; do
    sleep 1
    # Check if all processes are still running
    if ! kill -0 $REGISTRY_PID 2>/dev/null; then
        print_error "Registry stopped unexpectedly"
        break
    fi
    if ! kill -0 $MATCHER_PID 2>/dev/null; then
        print_error "Matcher stopped unexpectedly"
        break
    fi
    if ! kill -0 $VALIDATOR_PID 2>/dev/null; then
        print_error "Validator stopped unexpectedly"
        break
    fi
done

echo ""
print_error "Some services have stopped. Check logs for details."
exit 1
