#!/bin/bash
# Batch Intent Submission Tool
# 批量Intent提交工具 - 提交到区块链和RootLayer（双轨提交）
#
# Usage: ./batch-submit-intents.sh [count] [delay_seconds]
# Example: ./batch-submit-intents.sh 5 3

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Parameters
INTENT_COUNT=${1:-5}
DELAY=${2:-3}

echo -e "${CYAN}╔═══════════════════════════════════════╗${NC}"
echo -e "${CYAN}║  Batch Intent Submission              ║${NC}"
echo -e "${CYAN}║  批量Intent提交（双轨提交）            ║${NC}"
echo -e "${CYAN}╚═══════════════════════════════════════╝${NC}"
echo ""

# Load .env if exists
if [ -f "$PROJECT_ROOT/.env" ]; then
    echo -e "${BLUE}ℹ${NC} Loading environment from .env..."
    set -a
    source "$PROJECT_ROOT/.env"
    set +a
fi

# Check required environment
if [ -z "$TEST_PRIVATE_KEY" ]; then
    echo -e "${RED}✗${NC} TEST_PRIVATE_KEY not set"
    echo ""
    echo "Please set your private key:"
    echo "  export TEST_PRIVATE_KEY=\"your_private_key\""
    echo ""
    exit 1
fi

# Check if binary exists
if [ ! -f "$PROJECT_ROOT/bin/submit-intent-signed" ]; then
    echo -e "${BLUE}ℹ${NC} Building submit-intent-signed..."
    cd "$PROJECT_ROOT"
    make build-submit-intent-signed
fi

# Set defaults
SUBNET_ID="${SUBNET_ID:-0x0000000000000000000000000000000000000000000000000000000000000002}"
ROOTLAYER_HTTP="${ROOTLAYER_HTTP:-http://3.17.208.238:8081}"
INTENT_MANAGER="${INTENT_MANAGER_ADDR:-${PIN_BASE_SEPOLIA_INTENT_MANAGER:-0xD04d23775D3B8e028e6104E31eb0F6c07206EB46}}"
CHAIN_RPC_URL="${CHAIN_RPC_URL:-https://sepolia.base.org}"
CHAIN_NETWORK="${CHAIN_NETWORK:-base_sepolia}"
AMOUNT_WEI="${AMOUNT_WEI:-100000000000000}"

echo "📋 Configuration:"
echo "   Intent Count: $INTENT_COUNT"
echo "   Delay: ${DELAY}s between submissions"
echo "   Subnet ID: $SUBNET_ID"
echo "   RootLayer: $ROOTLAYER_HTTP"
echo "   Network: $CHAIN_NETWORK"
echo "   Amount: $AMOUNT_WEI WEI"
echo ""

# Counters
SUCCESS_COUNT=0
FAIL_COUNT=0

echo -e "${CYAN}Starting batch submission...${NC}"
echo ""

# Submit intents in a loop
for i in $(seq 1 $INTENT_COUNT); do
    echo -e "${BLUE}[$i/$INTENT_COUNT]${NC} Submitting Intent #$i..."

    # Prepare params
    TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    PARAMS_JSON="{\"task\":\"batch-intent-$i\",\"index\":$i,\"timestamp\":\"$TIMESTAMP\"}"

    # Submit via submit-intent-signed (dual submission)
    OUTPUT=$(INTENT_MANAGER_ADDR="$INTENT_MANAGER" \
    RPC_URL="$CHAIN_RPC_URL" \
    PRIVATE_KEY="$TEST_PRIVATE_KEY" \
    PIN_NETWORK="$CHAIN_NETWORK" \
    ROOTLAYER_HTTP="$ROOTLAYER_HTTP/api/v1" \
    SUBNET_ID="$SUBNET_ID" \
    INTENT_TYPE="batch-intent" \
    PARAMS_JSON="$PARAMS_JSON" \
    AMOUNT_WEI="$AMOUNT_WEI" \
    "$PROJECT_ROOT/bin/submit-intent-signed" 2>&1)

    EXIT_CODE=$?

    # Extract Intent ID from output if successful
    INTENT_ID=$(echo "$OUTPUT" | grep "Intent ID:" | tail -1 | awk '{print $NF}')

    if [ $EXIT_CODE -eq 0 ]; then
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
        if [ -n "$INTENT_ID" ]; then
            echo -e "  ${GREEN}✓${NC} Intent #$i submitted: $INTENT_ID"
        else
            echo -e "  ${GREEN}✓${NC} Intent #$i submitted successfully"
        fi
    else
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo -e "  ${RED}✗${NC} Intent #$i failed"
        # Show error details if VERBOSE is set
        if [ "$VERBOSE" = "1" ]; then
            echo "$OUTPUT" | head -10
        fi
    fi

    # Wait before next submission (avoid nonce conflicts)
    if [ $i -lt $INTENT_COUNT ]; then
        sleep $DELAY
    fi
done

echo ""
echo -e "${CYAN}═══════════════════════════════════════${NC}"
echo -e "${GREEN}✓${NC} Batch submission completed!"
echo ""
echo "📊 Results:"
echo "   Total: $INTENT_COUNT"
echo "   Success: $SUCCESS_COUNT"
echo "   Failed: $FAIL_COUNT"
echo ""

if [ $FAIL_COUNT -eq 0 ]; then
    echo -e "${GREEN}✓${NC} All intents submitted successfully!"
    exit 0
else
    echo -e "${RED}✗${NC} Some intents failed to submit"
    exit 1
fi
