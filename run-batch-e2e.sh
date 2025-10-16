#!/bin/bash
# Quick launcher for batch E2E tests

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Check environment
if [ -z "$TEST_PRIVATE_KEY" ]; then
    echo "Error: TEST_PRIVATE_KEY not set"
    echo ""
    echo "Please set it with:"
    echo "  export TEST_PRIVATE_KEY=\"your_test_private_key\""
    echo ""
    echo "Or use .env file:"
    echo "  source .env"
    exit 1
fi

# Load .env if exists
if [ -f "$SCRIPT_DIR/.env" ]; then
    echo "Loading environment from .env..."
    source "$SCRIPT_DIR/.env"
fi

# Set defaults if not set
export SUBNET_ID="${SUBNET_ID:-0x0000000000000000000000000000000000000000000000000000000000000002}"
export ROOTLAYER_GRPC="${ROOTLAYER_GRPC:-3.17.208.238:9001}"
export ROOTLAYER_HTTP="${ROOTLAYER_HTTP:-http://3.17.208.238:8081}"
export CHAIN_RPC_URL="${CHAIN_RPC_URL:-https://sepolia.base.org}"
export CHAIN_NETWORK="${CHAIN_NETWORK:-base_sepolia}"
export INTENT_MANAGER_ADDR="${INTENT_MANAGER_ADDR:-0xD04d23775D3B8e028e6104E31eb0F6c07206EB46}"

echo "🚀 Running Batch E2E Test"
echo "   Subnet ID: $SUBNET_ID"
echo "   RootLayer: $ROOTLAYER_GRPC"
echo ""

# Run batch E2E test
exec "$SCRIPT_DIR/scripts/batch-e2e-test.sh" "$@"
