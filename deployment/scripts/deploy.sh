#!/bin/bash
# Deploy PinAI Subnet to production
# Run this script from the deployment directory

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
echo "🚀 PinAI Subnet - Production Deployment"
echo "========================================"
echo ""

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOYMENT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$DEPLOYMENT_DIR"

# Check if .env exists
if [ ! -f ".env" ]; then
    print_error ".env file not found"
    echo ""
    echo "Please create .env file:"
    echo "  cp ../.env.example .env"
    echo "  nano .env  # Edit configuration"
    echo ""
    exit 1
fi

print_success ".env file found"
echo ""

# Load and validate .env
set -a
source .env
set +a

# ============================================
# Support both comma-separated and individual variables
# ============================================
if [ -n "$VALIDATOR_KEYS" ]; then
    # Convert comma-separated keys to individual variables
    print_info "Detected comma-separated keys, converting..."
    IFS=',' read -ra KEYS_ARRAY <<< "$VALIDATOR_KEYS"
    
    export VALIDATOR_KEY_1="${KEYS_ARRAY[0]}"
    export VALIDATOR_KEY_2="${KEYS_ARRAY[1]}"
    export VALIDATOR_KEY_3="${KEYS_ARRAY[2]}"
    
    print_success "Key conversion completed"
    echo ""
fi

# Generate public keys if not provided
if [ -n "$VALIDATOR_PUBKEYS" ]; then
    print_info "Using provided public keys"
    IFS=',' read -ra PUBKEYS_ARRAY <<< "$VALIDATOR_PUBKEYS"
    export VALIDATOR_PUBKEY_1="${PUBKEYS_ARRAY[0]}"
    export VALIDATOR_PUBKEY_2="${PUBKEYS_ARRAY[1]}"
    export VALIDATOR_PUBKEY_3="${PUBKEYS_ARRAY[2]}"
else
    print_info "Generating public keys from private keys..."
    DERIVE_PUBKEY="../bin/derive-pubkey"
    if [ -f "$DERIVE_PUBKEY" ]; then
        export VALIDATOR_PUBKEY_1=$(${DERIVE_PUBKEY} ${VALIDATOR_KEY_1} 2>/dev/null || echo "validator-1")
        export VALIDATOR_PUBKEY_2=$(${DERIVE_PUBKEY} ${VALIDATOR_KEY_2} 2>/dev/null || echo "validator-2")
        export VALIDATOR_PUBKEY_3=$(${DERIVE_PUBKEY} ${VALIDATOR_KEY_3} 2>/dev/null || echo "validator-3")
        print_success "Public keys generated"
    else
        print_warning "derive-pubkey not found, using validator IDs as pubkeys"
        export VALIDATOR_PUBKEY_1="validator-1"
        export VALIDATOR_PUBKEY_2="validator-2"
        export VALIDATOR_PUBKEY_3="validator-3"
    fi
fi
echo ""

# Check required variables
print_info "Validating configuration..."

# Build required vars list based on NUM_VALIDATORS
REQUIRED_VARS=("SUBNET_ID" "TEST_PRIVATE_KEY")
for i in $(seq 1 ${NUM_VALIDATORS:-3}); do
    REQUIRED_VARS+=("VALIDATOR_KEY_$i")
    REQUIRED_VARS+=("VALIDATOR_PUBKEY_$i")
done

# INTENT_MANAGER_ADDR is optional for testing
if [ -z "$INTENT_MANAGER_ADDR" ]; then
    print_warning "INTENT_MANAGER_ADDR not set (optional for testing)"
fi

MISSING_VARS=()
for var in "${REQUIRED_VARS[@]}"; do
    if [ -z "${!var}" ]; then
        MISSING_VARS+=("$var")
    fi
done

if [ ${#MISSING_VARS[@]} -gt 0 ]; then
    print_error "Missing required environment variables:"
    for var in "${MISSING_VARS[@]}"; do
        echo "  - $var"
    done
    echo ""
    echo "Two formats supported:"
    echo "  Format 1 (comma-separated):"
    echo "    VALIDATOR_KEYS=key1,key2,key3"
    echo "    VALIDATOR_PUBKEYS=pubkey1,pubkey2,pubkey3  (optional, will be derived)"
    echo ""
    echo "  Format 2 (individual variables):"
    echo "    VALIDATOR_KEY_1=key1"
    echo "    VALIDATOR_KEY_2=key2"
    echo "    VALIDATOR_KEY_3=key3"
    echo "    VALIDATOR_PUBKEY_1=pubkey1"
    echo "    VALIDATOR_PUBKEY_2=pubkey2"
    echo "    VALIDATOR_PUBKEY_3=pubkey3"
    exit 1
fi

print_success "Configuration valid"
echo ""

# Check if Docker image exists
if ! docker images pinai-subnet:latest | grep -q pinai-subnet; then
    print_error "Docker image 'pinai-subnet:latest' not found"
    echo ""
    echo "Please build the image first:"
    echo "  ./scripts/build-images.sh"
    echo ""
    exit 1
fi

print_success "Docker image found"
echo ""

# Create data directories
print_info "Creating data directories..."
mkdir -p data/matcher
for i in $(seq 1 ${NUM_VALIDATORS:-3}); do
    mkdir -p "data/validator-$i"
done
print_success "Data directories ready"
echo ""

# Show configuration summary
NUM_VALS=${NUM_VALIDATORS:-3}
THRESHOLD_NUM=$(( ($NUM_VALS * 2 + 2) / 3 ))  # Ceiling of 2/3
echo "📋 Deployment Configuration:"
echo "   Subnet ID: $SUBNET_ID"
echo "   RootLayer: $ROOTLAYER_GRPC"
echo "   Chain: $CHAIN_NETWORK"
echo "   Validators: $NUM_VALS node(s)"
echo "   Threshold: $THRESHOLD_NUM/$NUM_VALS"
echo ""

# Confirm deployment
read -p "Proceed with deployment? (y/n) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    print_warning "Deployment cancelled"
    exit 0
fi
echo ""

# Detect docker-compose command (support both v1 and v2)
if command -v docker-compose &> /dev/null; then
    DOCKER_COMPOSE="docker-compose"
else
    DOCKER_COMPOSE="docker compose"
fi

# Ensure we're in the deployment directory for relative paths
cd "$DEPLOYMENT_DIR"

# Stop any existing services
print_info "Stopping existing services..."
(cd "$DEPLOYMENT_DIR" && $DOCKER_COMPOSE -f docker/docker-compose.yml down 2>/dev/null) || true
echo ""

# ============================================
# Export dynamic environment variables for docker-compose
# ============================================
print_info "Preparing dynamic configuration..."

export NUM_VALIDATORS=${NUM_VALIDATORS:-3}
export THRESHOLD_NUM=$(( ($NUM_VALIDATORS * 2 + 2) / 3 ))  # Ceiling of 2/3
export THRESHOLD_DENOM=$NUM_VALIDATORS

# Build validator pubkey list (validator-id:pubkey,validator-id:pubkey,...)
PUBKEY_LIST=""
for i in $(seq 1 $NUM_VALIDATORS); do
    PUBKEY_VAR="VALIDATOR_PUBKEY_$i"
    if [ -n "${!PUBKEY_VAR}" ]; then
        if [ -n "$PUBKEY_LIST" ]; then
            PUBKEY_LIST="$PUBKEY_LIST,validator-$i:${!PUBKEY_VAR}"
        else
            PUBKEY_LIST="validator-$i:${!PUBKEY_VAR}"
        fi
    fi
done
export VALIDATOR_PUBKEY_LIST="$PUBKEY_LIST"

# Build raft peers list (validator-id:hostname:port,...)
PEERS_LIST=""
for i in $(seq 1 $NUM_VALIDATORS); do
    if [ -n "$PEERS_LIST" ]; then
        PEERS_LIST="$PEERS_LIST,validator-$i:validator-$i:$((7400 + i - 1))"
    else
        PEERS_LIST="validator-$i:validator-$i:$((7400 + i - 1))"
    fi
done
export RAFT_PEERS="$PEERS_LIST"

# Build gossip seeds list (hostname:port,...)
SEEDS_LIST=""
for i in $(seq 1 $NUM_VALIDATORS); do
    if [ -n "$SEEDS_LIST" ]; then
        SEEDS_LIST="$SEEDS_LIST,validator-$i:$((7950 + i - 1))"
    else
        SEEDS_LIST="validator-$i:$((7950 + i - 1))"
    fi
done
export GOSSIP_SEEDS="$SEEDS_LIST"

# Build validator endpoints list (validator-id:hostname:port,...)
ENDPOINTS_LIST=""
for i in $(seq 1 $NUM_VALIDATORS); do
    if [ -n "$ENDPOINTS_LIST" ]; then
        ENDPOINTS_LIST="$ENDPOINTS_LIST,validator-$i:validator-$i:$((9090 + i - 1))"
    else
        ENDPOINTS_LIST="validator-$i:validator-$i:$((9090 + i - 1))"
    fi
done
export VALIDATOR_ENDPOINTS="$ENDPOINTS_LIST"

print_success "Dynamic configuration ready"
echo "   Validators: $NUM_VALIDATORS"
echo "   Threshold: $THRESHOLD_NUM/$THRESHOLD_DENOM"
echo "   Raft peers: $RAFT_PEERS"
echo ""

# Start services
print_info "Starting services..."

# Build list of services to start based on NUM_VALIDATORS
SERVICES="matcher"
for i in $(seq 1 ${NUM_VALIDATORS:-3}); do
    SERVICES="$SERVICES validator-$i"
done

(cd "$DEPLOYMENT_DIR" && $DOCKER_COMPOSE -f docker/docker-compose.yml up -d $SERVICES)

if [ $? -eq 0 ]; then
    print_success "Services started successfully"
else
    print_error "Failed to start services"
    exit 1
fi

echo ""

# Wait for services
print_info "Waiting for services to initialize..."
sleep 10
echo ""

# Check service status
print_info "Service status:"
(cd "$DEPLOYMENT_DIR" && $DOCKER_COMPOSE -f docker/docker-compose.yml ps)
echo ""

# Health checks
print_info "Performing health checks..."
sleep 5

# Check Matcher
if curl -s http://localhost:8094/health > /dev/null 2>&1; then
    print_success "Matcher: Healthy ✓"
else
    print_warning "Matcher: Not responding (may still be starting)"
fi

# Check Validators (dynamic based on NUM_VALIDATORS)
for i in $(seq 1 $NUM_VALIDATORS); do
    port=$((9089 + i))
    if nc -z localhost "$port" 2>/dev/null; then
        print_success "Validator-$i: Running ✓ (port $port)"
    else
        print_warning "Validator-$i: Port $port not ready (may still be starting)"
    fi
done

echo ""

# Success message
echo "======================================"
echo -e "${GREEN}🎉 Deployment Complete!${NC}"
echo "======================================"
echo ""
echo "🌐 Service Addresses:"
echo "   Matcher:      http://localhost:8094/health"
for i in $(seq 1 $NUM_VALIDATORS); do
    port=$((9089 + i))
    echo "   Validator-$i:  localhost:$port"
done
echo ""
echo "📋 Management Commands:"
echo "   View logs:    $DOCKER_COMPOSE -f docker/docker-compose.yml logs -f"
echo "   Check status: $DOCKER_COMPOSE -f docker/docker-compose.yml ps"
echo "   Stop:         $DOCKER_COMPOSE -f docker/docker-compose.yml down"
echo "   Restart:      $DOCKER_COMPOSE -f docker/docker-compose.yml restart"
echo ""
echo "🔍 Monitor logs:"
echo "   $DOCKER_COMPOSE -f docker/docker-compose.yml logs -f validator-1"
echo ""

