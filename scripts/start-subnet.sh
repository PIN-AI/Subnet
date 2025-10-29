#!/bin/bash
# PinAI Subnet Unified Startup Script
# Supports both Raft+Gossip and CometBFT consensus modes
# All configuration via environment variables

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_info() { echo -e "${BLUE}ℹ${NC} $1"; }
print_success() { echo -e "${GREEN}✓${NC} $1"; }
print_error() { echo -e "${RED}✗${NC} $1"; }
print_warning() { echo -e "${YELLOW}⚠${NC} $1"; }

echo ""
echo "🚀 PinAI Subnet Launcher"
echo "=================================="
echo ""

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# ============================================
# Auto-load .env file if it exists
# ============================================
if [ -f "$PROJECT_ROOT/.env" ]; then
    print_info "Loading environment from .env file..."
    set -a
    source "$PROJECT_ROOT/.env"
    set +a
    print_success ".env file loaded"
else
    print_warning "No .env file found (this is optional)"
    echo "   You can create one by copying .env.example:"
    echo "   cp .env.example .env"
fi
echo ""

# ============================================
# Environment Variables with Defaults
# ============================================

# Consensus type: "raft" or "cometbft"
CONSENSUS_TYPE="${CONSENSUS_TYPE:-raft}"

# Default Subnet configuration (can be overridden per-validator)
DEFAULT_SUBNET_ID="${SUBNET_ID:-0x0000000000000000000000000000000000000000000000000000000000000002}"
ROOTLAYER_GRPC="${ROOTLAYER_GRPC:-3.17.208.238:9001}"
ROOTLAYER_HTTP="${ROOTLAYER_HTTP:-http://3.17.208.238:8081}"

# Blockchain configuration
ENABLE_CHAIN_SUBMIT="${ENABLE_CHAIN_SUBMIT:-true}"
CHAIN_RPC_URL="${CHAIN_RPC_URL:-https://sepolia.base.org}"
CHAIN_NETWORK="${CHAIN_NETWORK:-base_sepolia}"
INTENT_MANAGER_ADDR="${INTENT_MANAGER_ADDR:-0xD04d23775D3B8e028e6104E31eb0F6c07206EB46}"

# Number of validators to start
NUM_VALIDATORS="${NUM_VALIDATORS:-3}"

# Validator private keys (comma-separated)
# REQUIRED: Must be provided via environment variable
# Example: export VALIDATOR_KEYS="key1,key2,key3"
VALIDATOR_KEYS="${VALIDATOR_KEYS:-}"

# Validator public keys (comma-separated, hex format without 0x prefix)
# REQUIRED for proper validator set configuration
# Example: export VALIDATOR_PUBKEYS="pubkey1,pubkey2,pubkey3"
VALIDATOR_PUBKEYS="${VALIDATOR_PUBKEYS:-}"

# Per-validator Subnet IDs (comma-separated, optional)
# If not set, all validators use DEFAULT_SUBNET_ID
# Example: VALIDATOR_SUBNET_IDS="0x...002,0x...003,0x...002"
VALIDATOR_SUBNET_IDS="${VALIDATOR_SUBNET_IDS:-}"

# Logs directory
LOGS_DIR="${LOGS_DIR:-$PROJECT_ROOT/subnet-logs}"

# Test mode: if true, runs in background; if false, waits for user interrupt
TEST_MODE="${TEST_MODE:-false}"

# Start agent: if true, starts simple-agent; if false, skips agent startup
START_AGENT="${START_AGENT:-true}"

# Matcher configuration (only one matcher, specify which subnet it serves)
MATCHER_SUBNET_ID="${MATCHER_SUBNET_ID:-$DEFAULT_SUBNET_ID}"

# Required: Private key for signing (used by matcher)
if [ -z "$TEST_PRIVATE_KEY" ]; then
    print_error "TEST_PRIVATE_KEY not set"
    echo ""
    echo "Please set your test private key:"
    echo "  export TEST_PRIVATE_KEY=\"your_test_private_key_here\""
    echo ""
    exit 1
fi

# Validate consensus type
if [ "$CONSENSUS_TYPE" != "raft" ] && [ "$CONSENSUS_TYPE" != "cometbft" ]; then
    print_error "Invalid CONSENSUS_TYPE: $CONSENSUS_TYPE"
    echo "Must be 'raft' or 'cometbft'"
    exit 1
fi

print_success "Environment loaded"
echo ""
echo "📋 Configuration:"
echo "   Consensus: $CONSENSUS_TYPE"
echo "   Validators: $NUM_VALIDATORS"
echo "   Default Subnet ID: $DEFAULT_SUBNET_ID"
echo "   Matcher Subnet ID: $MATCHER_SUBNET_ID"
echo "   RootLayer gRPC: $ROOTLAYER_GRPC"
echo "   RootLayer HTTP: $ROOTLAYER_HTTP"
echo "   Chain Submit: $ENABLE_CHAIN_SUBMIT"
echo "   Logs: $LOGS_DIR"
echo ""

# ============================================
# Cleanup
# ============================================

print_info "Cleaning up old processes..."
pkill -f "bin/matcher" 2>/dev/null || true
pkill -f "bin/validator" 2>/dev/null || true
pkill -f "bin/registry" 2>/dev/null || true
pkill -f "bin/test-agent" 2>/dev/null || true
pkill -f "bin/simple-agent" 2>/dev/null || true
sleep 1
print_success "Cleanup complete"

# Clean old data
rm -rf "$LOGS_DIR"
mkdir -p "$LOGS_DIR"
print_success "Logs directory: $LOGS_DIR"

# ============================================
# Build binaries
# ============================================

print_info "Checking binaries..."
cd "$PROJECT_ROOT"
if [ ! -f "bin/validator" ] || [ ! -f "bin/matcher" ] || [ ! -f "bin/registry" ]; then
    print_info "Building binaries..."
    make build
    print_success "Build complete"
fi
print_success "Binaries ready"

# ============================================
# Parse validator keys and subnet IDs
# ============================================

# Check if VALIDATOR_KEYS is provided
if [ -z "$VALIDATOR_KEYS" ]; then
    print_error "VALIDATOR_KEYS not set"
    echo ""
    echo "Please provide validator private keys:"
    echo "  export VALIDATOR_KEYS=\"key1,key2,key3\""
    echo ""
    echo "Number of keys must match NUM_VALIDATORS ($NUM_VALIDATORS)"
    echo ""
    echo "⚠️  SECURITY: Never commit private keys to git!"
    echo "   Use .env file (added to .gitignore) or environment variables"
    exit 1
fi

IFS=',' read -ra KEYS_ARRAY <<< "$VALIDATOR_KEYS"

if [ ${#KEYS_ARRAY[@]} -lt $NUM_VALIDATORS ]; then
    print_error "Not enough validator keys provided"
    echo "Requested: $NUM_VALIDATORS validators"
    echo "Provided: ${#KEYS_ARRAY[@]} keys"
    exit 1
fi

# Parse validator public keys
if [ -z "$VALIDATOR_PUBKEYS" ]; then
    print_error "VALIDATOR_PUBKEYS not set"
    echo ""
    echo "Please provide validator public keys:"
    echo "  export VALIDATOR_PUBKEYS=\"pubkey1,pubkey2,pubkey3\""
    echo ""
    echo "Number of pubkeys must match NUM_VALIDATORS ($NUM_VALIDATORS)"
    exit 1
fi

IFS=',' read -ra PUBKEYS_ARRAY <<< "$VALIDATOR_PUBKEYS"

if [ ${#PUBKEYS_ARRAY[@]} -lt $NUM_VALIDATORS ]; then
    print_error "Not enough validator pubkeys provided"
    echo "Requested: $NUM_VALIDATORS validators"
    echo "Provided: ${#PUBKEYS_ARRAY[@]} pubkeys"
    exit 1
fi

# Parse per-validator subnet IDs
if [ -n "$VALIDATOR_SUBNET_IDS" ]; then
    IFS=',' read -ra SUBNET_IDS_ARRAY <<< "$VALIDATOR_SUBNET_IDS"
    if [ ${#SUBNET_IDS_ARRAY[@]} -lt $NUM_VALIDATORS ]; then
        print_error "Not enough subnet IDs provided"
        echo "Requested: $NUM_VALIDATORS validators"
        echo "Provided: ${#SUBNET_IDS_ARRAY[@]} subnet IDs"
        exit 1
    fi
    print_info "Using per-validator Subnet IDs:"
    for i in $(seq 1 $NUM_VALIDATORS); do
        echo "   Validator $i: ${SUBNET_IDS_ARRAY[$i-1]}"
    done
else
    # All validators use the same subnet ID
    for i in $(seq 1 $NUM_VALIDATORS); do
        SUBNET_IDS_ARRAY[$i-1]="$DEFAULT_SUBNET_ID"
    done
    print_info "All validators using Subnet ID: $DEFAULT_SUBNET_ID"
fi

echo ""
echo "🎬 Starting services..."
echo "=================================="

# ============================================
# Start Registry (optional, based on consensus type)
# ============================================

if [ "$CONSENSUS_TYPE" = "raft" ]; then
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
    else
        print_error "Registry failed to start"
        cat "$LOGS_DIR/registry.log"
        exit 1
    fi
else
    print_info "Skipping Registry (not needed for CometBFT)"
fi

# ============================================
# Start Matcher
# ============================================

print_info "Starting Matcher service (Subnet: $MATCHER_SUBNET_ID)..."

MATCHER_CONFIG="$LOGS_DIR/matcher-config.yaml"
cat > "$MATCHER_CONFIG" <<EOF
identity:
  subnet_id: "$MATCHER_SUBNET_ID"
  matcher_id: "matcher-main"

network:
  grpc_port: 8090
  http_port: 8091

rootlayer:
  grpc_endpoint: "$ROOTLAYER_GRPC"
  http_endpoint: "$ROOTLAYER_HTTP"
  enable: true

matching:
  bid_window_seconds: 10
  selection_delay_seconds: 3

# Private key for signing
signer:
  type: "ecdsa"
  private_key: "$TEST_PRIVATE_KEY"

# On-chain configuration
enable_chain_submit: $ENABLE_CHAIN_SUBMIT
chain_rpc_url: "$CHAIN_RPC_URL"
chain_network: "$CHAIN_NETWORK"
intent_manager_addr: "$INTENT_MANAGER_ADDR"
EOF

"$PROJECT_ROOT/bin/matcher" -config "$MATCHER_CONFIG" \
    > "$LOGS_DIR/matcher.log" 2>&1 &
MATCHER_PID=$!
sleep 3

if kill -0 $MATCHER_PID 2>/dev/null; then
    print_success "Matcher started (PID: $MATCHER_PID)"
    echo "   - Subnet: $MATCHER_SUBNET_ID"
    echo "   - gRPC: localhost:8090"
    echo "   - HTTP: localhost:8091"
else
    print_error "Matcher failed to start"
    cat "$LOGS_DIR/matcher.log"
    exit 1
fi

# ============================================
# Start Validators (Raft or CometBFT)
# ============================================

if [ "$CONSENSUS_TYPE" = "raft" ]; then
    # ========================================
    # RAFT + GOSSIP MODE
    # ========================================

    print_info "Starting $NUM_VALIDATORS validators (Raft+Gossip mode)..."

    # Group validators by Subnet ID for proper Raft cluster configuration
    # For now, we'll start all validators but each can have different subnet_id

    # Build peer list (all validators in same Raft cluster for now)
    PEER_LIST=""
    for i in $(seq 1 $NUM_VALIDATORS); do
        VALIDATOR_ID="validator-$i"
        RAFT_PORT=$((7400 + (i-1) * 10))
        if [ -n "$PEER_LIST" ]; then
            PEER_LIST="$PEER_LIST,"
        fi
        PEER_LIST="${PEER_LIST}${VALIDATOR_ID}:127.0.0.1:${RAFT_PORT}"
    done

    # Build gossip seeds
    GOSSIP_SEEDS=""
    for i in $(seq 1 $NUM_VALIDATORS); do
        GOSSIP_PORT=$((7950 + (i-1) * 10))
        if [ -n "$GOSSIP_SEEDS" ]; then
            GOSSIP_SEEDS="$GOSSIP_SEEDS,"
        fi
        GOSSIP_SEEDS="${GOSSIP_SEEDS}127.0.0.1:${GOSSIP_PORT}"
    done

    # Build validator endpoints for report forwarding
    VALIDATOR_ENDPOINTS=""
    for i in $(seq 1 $NUM_VALIDATORS); do
        GRPC_PORT=$((9090 + (i-1) * 10))
        if [ -n "$VALIDATOR_ENDPOINTS" ]; then
            VALIDATOR_ENDPOINTS="$VALIDATOR_ENDPOINTS,"
        fi
        VALIDATOR_ENDPOINTS="${VALIDATOR_ENDPOINTS}validator-$i:localhost:${GRPC_PORT}"
    done

    # Build validator-pubkeys string (validator-id:pubkey format)
    VALIDATOR_PUBKEYS_PARAM=""
    for i in $(seq 1 $NUM_VALIDATORS); do
        VALIDATOR_ID="validator-$i"
        PUBKEY="${PUBKEYS_ARRAY[$i-1]}"
        if [ -n "$VALIDATOR_PUBKEYS_PARAM" ]; then
            VALIDATOR_PUBKEYS_PARAM="$VALIDATOR_PUBKEYS_PARAM,"
        fi
        VALIDATOR_PUBKEYS_PARAM="${VALIDATOR_PUBKEYS_PARAM}${VALIDATOR_ID}:${PUBKEY}"
    done

    for i in $(seq 1 $NUM_VALIDATORS); do
        VALIDATOR_ID="validator-$i"
        GRPC_PORT=$((9090 + (i-1) * 10))
        RAFT_PORT=$((7400 + (i-1) * 10))
        GOSSIP_PORT=$((7950 + (i-1) * 10))
        VALIDATOR_KEY="${KEYS_ARRAY[$i-1]}"
        VALIDATOR_SUBNET_ID="${SUBNET_IDS_ARRAY[$i-1]}"

        print_info "Starting $VALIDATOR_ID (Subnet: $VALIDATOR_SUBNET_ID)..."

        "$PROJECT_ROOT/bin/validator" \
            -id "$VALIDATOR_ID" \
            -grpc $GRPC_PORT \
            -subnet-id "$VALIDATOR_SUBNET_ID" \
            -key "$VALIDATOR_KEY" \
            -storage "$LOGS_DIR/val${i}-storage" \
            -rootlayer-endpoint "$ROOTLAYER_GRPC" \
            -registry-grpc "" \
            -registry-http "" \
            -chain-rpc-url "$CHAIN_RPC_URL" \
            -chain-network "$CHAIN_NETWORK" \
            -intent-manager-addr "$INTENT_MANAGER_ADDR" \
            -enable-chain-submit \
            -enable-rootlayer \
            -validators $NUM_VALIDATORS \
            -threshold-num 2 \
            -threshold-denom 3 \
            -validator-pubkeys "$VALIDATOR_PUBKEYS_PARAM" \
            -raft-enable \
            -raft-bootstrap \
            -raft-bind "127.0.0.1:$RAFT_PORT" \
            -raft-data-dir "$LOGS_DIR/val${i}-raft" \
            -raft-peers "$PEER_LIST" \
            -gossip-enable \
            -gossip-bind "127.0.0.1" \
            -gossip-port $GOSSIP_PORT \
            -gossip-seeds "$GOSSIP_SEEDS" \
            -validator-endpoints "$VALIDATOR_ENDPOINTS" \
            > "$LOGS_DIR/validator-$i.log" 2>&1 &

        VALIDATOR_PID=$!
        sleep 2

        if kill -0 $VALIDATOR_PID 2>/dev/null; then
            print_success "$VALIDATOR_ID started (PID: $VALIDATOR_PID, Subnet: $VALIDATOR_SUBNET_ID, gRPC: $GRPC_PORT)"
        else
            print_error "$VALIDATOR_ID failed to start"
            cat "$LOGS_DIR/validator-$i.log"
            exit 1
        fi
    done

else
    # ========================================
    # COMETBFT MODE
    # ========================================

    print_info "Starting $NUM_VALIDATORS validators (CometBFT mode)..."

    # Setup CometBFT infrastructure
    COMETBFT_DIR="$LOGS_DIR/cometbft-validators"
    mkdir -p "$COMETBFT_DIR"

    # Generate keys for all validators
    print_info "Generating CometBFT keys..."
    "$PROJECT_ROOT/scripts/generate-cometbft-keys.sh" "$COMETBFT_DIR" $NUM_VALIDATORS

    # Generate genesis file (uses first subnet ID)
    print_info "Generating genesis file (Subnet: ${SUBNET_IDS_ARRAY[0]})..."
    "$PROJECT_ROOT/scripts/generate-cometbft-genesis.sh" "$COMETBFT_DIR"

    # Extract node IDs from generated node_key.json files
    print_info "Extracting node IDs from generated keys..."
    NODE_IDS=()
    for i in $(seq 1 $NUM_VALIDATORS); do
        NODE_KEY_FILE="$COMETBFT_DIR/validator${i}/cometbft/config/node_key.json"
        if [ ! -f "$NODE_KEY_FILE" ]; then
            print_error "Node key not found: $NODE_KEY_FILE"
            exit 1
        fi
        NODE_ID=$("$PROJECT_ROOT/scripts/get-cometbft-node-id.sh" "$NODE_KEY_FILE")
        NODE_IDS+=("$NODE_ID")
        echo "  validator${i}: $NODE_ID"
    done

    for i in $(seq 1 $NUM_VALIDATORS); do
        VALIDATOR_ID="validator_$i"
        GRPC_PORT=$((9090 + (i-1) * 10))
        P2P_PORT=$((26656 + (i-1) * 10))
        RPC_PORT=$((26657 + (i-1) * 10))
        VALIDATOR_KEY="${KEYS_ARRAY[$i-1]}"
        VALIDATOR_SUBNET_ID="${SUBNET_IDS_ARRAY[$i-1]}"

        # Build persistent peers list (all other validators)
        PEERS=""
        for j in $(seq 1 $NUM_VALIDATORS); do
            if [ $j -ne $i ]; then
                PEER_P2P_PORT=$((26656 + (j-1) * 10))
                PEER_NODE_ID="${NODE_IDS[$j-1]}"
                if [ -n "$PEERS" ]; then
                    PEERS="$PEERS,"
                fi
                PEERS="${PEERS}${PEER_NODE_ID}@127.0.0.1:${PEER_P2P_PORT}"
            fi
        done

        # Build validator pubkeys list
        VALIDATOR_PUBKEYS=""
        for j in $(seq 1 $NUM_VALIDATORS); do
            if [ -n "$VALIDATOR_PUBKEYS" ]; then
                VALIDATOR_PUBKEYS="$VALIDATOR_PUBKEYS,"
            fi
            VALIDATOR_PUBKEYS="${VALIDATOR_PUBKEYS}validator_$j:10"
        done

        print_info "Starting $VALIDATOR_ID (Subnet: $VALIDATOR_SUBNET_ID)..."

        "$PROJECT_ROOT/bin/validator" \
            --id="$VALIDATOR_ID" \
            --subnet-id="$VALIDATOR_SUBNET_ID" \
            --key="$VALIDATOR_KEY" \
            --grpc=$GRPC_PORT \
            --storage="$COMETBFT_DIR/validator${i}/storage" \
            --registry-grpc="" \
            --registry-http="" \
            --rootlayer-endpoint="$ROOTLAYER_GRPC" \
            --enable-rootlayer=true \
            --consensus-type=cometbft \
            --cometbft-home="$COMETBFT_DIR/validator${i}/cometbft" \
            --cometbft-moniker="$VALIDATOR_ID" \
            --cometbft-p2p-port=$P2P_PORT \
            --cometbft-rpc-port=$RPC_PORT \
            --cometbft-persistent-peers="$PEERS" \
            --validators=$NUM_VALIDATORS \
            --threshold-num=2 \
            --threshold-denom=3 \
            --validator-pubkeys="$VALIDATOR_PUBKEYS" \
            --enable-chain-submit \
            --chain-rpc-url="$CHAIN_RPC_URL" \
            --chain-network="$CHAIN_NETWORK" \
            --intent-manager-addr="$INTENT_MANAGER_ADDR" \
            > "$LOGS_DIR/validator${i}.log" 2>&1 &

        VALIDATOR_PID=$!
        sleep 2

        if kill -0 $VALIDATOR_PID 2>/dev/null; then
            print_success "$VALIDATOR_ID started (PID: $VALIDATOR_PID, Subnet: $VALIDATOR_SUBNET_ID, gRPC: $GRPC_PORT, P2P: $P2P_PORT)"
        else
            print_error "$VALIDATOR_ID failed to start"
            cat "$LOGS_DIR/validator${i}.log"
            exit 1
        fi
    done
fi

# ============================================
# Start Agent (Optional)
# ============================================

if [ "$START_AGENT" = "true" ]; then
    print_info "Starting simple agent (connects to Matcher Subnet: $MATCHER_SUBNET_ID)..."
    "$PROJECT_ROOT/bin/simple-agent" \
        -matcher="localhost:8090" \
        -validator="localhost:9090" \
        -subnet-id="$MATCHER_SUBNET_ID" \
        -id="test-agent-1" \
        -name="TestAgent1" \
        > "$LOGS_DIR/agent.log" 2>&1 &
    AGENT_PID=$!
    sleep 2

    if kill -0 $AGENT_PID 2>/dev/null; then
        print_success "Agent started (PID: $AGENT_PID)"
        echo "   - Agent ID: test-agent-1"
        echo "   - Subnet: $MATCHER_SUBNET_ID"
        echo "   - Matcher: localhost:8090"
    else
        print_error "Agent failed to start"
        cat "$LOGS_DIR/agent.log"
        exit 1
    fi
else
    print_warning "Agent startup skipped (START_AGENT=false)"
    echo "   You can start agents later by running:"
    echo "   ./bin/simple-agent -matcher=localhost:8090 -subnet-id=<SUBNET_ID> -id=my-agent -name=MyAgent"
    AGENT_PID=""
fi

# ============================================
# Summary
# ============================================

echo ""
echo "=================================="
echo -e "${GREEN}✅ All services started!${NC}"
echo "=================================="
echo ""
echo "📊 Service Status:"
if [ "$CONSENSUS_TYPE" = "raft" ]; then
    echo "   Registry:   gRPC=localhost:8091, HTTP=localhost:8101"
fi
echo "   Matcher:    gRPC=localhost:8090, HTTP=localhost:8091 (Subnet: $MATCHER_SUBNET_ID)"
echo "   Validators: $NUM_VALIDATORS ($CONSENSUS_TYPE)"
for i in $(seq 1 $NUM_VALIDATORS); do
    VALIDATOR_SUBNET_ID="${SUBNET_IDS_ARRAY[$i-1]}"
    if [ "$CONSENSUS_TYPE" = "raft" ]; then
        GRPC_PORT=$((9090 + (i-1) * 10))
        echo "     - validator-$i: Subnet=$VALIDATOR_SUBNET_ID, gRPC=$GRPC_PORT"
    else
        GRPC_PORT=$((9090 + (i-1) * 10))
        RPC_PORT=$((26657 + (i-1) * 10))
        echo "     - validator_$i: Subnet=$VALIDATOR_SUBNET_ID, gRPC=$GRPC_PORT, CometBFT RPC=$RPC_PORT"
    fi
done
if [ "$START_AGENT" = "true" ]; then
    echo "   Agent:      test-agent-1 (Matcher Subnet: $MATCHER_SUBNET_ID)"
else
    echo "   Agent:      (not started - use START_AGENT=true to enable)"
fi
echo ""
echo "📋 Logs:"
echo "   tail -f $LOGS_DIR/matcher.log"
for i in $(seq 1 $NUM_VALIDATORS); do
    if [ "$CONSENSUS_TYPE" = "raft" ]; then
        echo "   tail -f $LOGS_DIR/validator-$i.log"
    else
        echo "   tail -f $LOGS_DIR/validator${i}.log"
    fi
done
if [ "$START_AGENT" = "true" ]; then
    echo "   tail -f $LOGS_DIR/agent.log"
fi
echo ""

if [ "$CONSENSUS_TYPE" = "cometbft" ]; then
    echo "🔍 CometBFT Status:"
    echo "   curl http://localhost:26657/status | jq"
    echo ""
fi

echo "🛑 To stop all services:"
echo "   pkill -f 'bin/matcher|bin/validator|bin/registry|bin/simple-agent'"
echo ""

# ============================================
# Wait or Exit
# ============================================

if [ "$TEST_MODE" = "true" ]; then
    print_info "Running in test mode - services started in background"
    exit 0
else
    echo "Press Ctrl+C to stop all services..."
    trap 'echo ""; print_info "Stopping services..."; pkill -f "bin/matcher|bin/validator|bin/registry|bin/simple-agent"; print_success "All services stopped"; exit 0' INT

    while true; do
        sleep 1
    done
fi
