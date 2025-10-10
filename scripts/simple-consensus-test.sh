#!/bin/bash
# Simple 4-Node Consensus Test using test-agent
# This test demonstrates the full consensus flow with real ExecutionReports

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

PROJECT_ROOT="/Users/ty/pinai/protocol/Subnet"
LOG_DIR="$PROJECT_ROOT/simple-consensus-logs"

# Cleanup first
pkill -f "bin/validator" 2>/dev/null || true
pkill -f "test-agent" 2>/dev/null || true
rm -rf "$LOG_DIR"
mkdir -p "$LOG_DIR"

echo -e "${CYAN}=== Starting 4 Validators ===${NC}"

# Start 4 validators with correct pubkeys
VALIDATOR_PUBKEYS="validator-e2e-1:0479be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8,validator-e2e-2:04c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee51ae168fea63dc339a3c58419466ceaeef7f632653266d0e1236431a950cfe52a,validator-e2e-3:04f9308a019258c31049344f85f89d5229b531c845836f99b08601f113bce036f9388f7b0f632de8140fe337e62a37f3566500a99934c2231b6cb9fd7584b8e672,validator-e2e-4:04e493dbf1c10d80f3581e4904930b1404cc6c13900ee0758474fa94abe8c4cd1351ed993ea0d455b75642e2098ea51448d967ae33bfbdfe40cfe97bdc47739922"

for i in 1 2 3 4; do
    port=$((9199 + i))
    key="000000000000000000000000000000000000000000000000000000000000000$i"

    echo "Starting validator-e2e-$i on port $port..."

    VALIDATOR_ID="validator-e2e-$i" \
    GRPC_PORT="$port" \
    REGISTRY_GRPC_PORT="$((8090 + i))" \
    REGISTRY_HTTP_PORT="$((8100 + i))" \
    PRIVATE_KEY="$key" \
    VALIDATOR_COUNT="4" \
    THRESHOLD_NUM="3" \
    THRESHOLD_DENOM="4" \
    VALIDATOR_PUBKEYS="$VALIDATOR_PUBKEYS" \
    CHECKPOINT_INTERVAL="5s" \
    SUBNET_ID="0x1111111111111111111111111111111111111111111111111111111111111111" \
    ROOTLAYER_GRPC="3.17.208.238:9001" \
    ENABLE_ROOTLAYER="true" \
    "$PROJECT_ROOT/bin/validator" > "$LOG_DIR/validator-$i.log" 2>&1 &
done

echo "Waiting for validators to start..."
sleep 5

echo -e "\n${CYAN}=== Checking Leadership ===${NC}"
for i in 1 2 3 4; do
    if grep -q "is_leadertrue" "$LOG_DIR/validator-$i.log"; then
        echo -e "${GREEN}✓ Validator $i is the leader${NC}"
        LEADER=$i
    fi
done

if [ -z "$LEADER" ]; then
    echo -e "${YELLOW}⚠ No leader detected yet${NC}"
fi

echo -e "\n${CYAN}=== Starting Test Agent ===${NC}"

# Start test agent to submit execution report
AGENT_ID="test-agent-001" \
MATCHER_ADDR="localhost:8090" \
VALIDATOR_ADDR="localhost:9200" \
"$PROJECT_ROOT/scripts/test-agent/test-agent" \
    --agent-id "test-agent-001" \
    --validator "localhost:9200" \
    > "$LOG_DIR/agent.log" 2>&1 &

AGENT_PID=$!
sleep 3

echo -e "\n${CYAN}=== Submitting Execution Report ===${NC}"

# The test agent should automatically submit a report
# Let's wait and monitor

echo "Monitoring consensus for 45 seconds..."
for t in {1..45}; do
    echo -n "."
    sleep 1

    # Check for checkpoint proposed
    if grep -q "Proposed checkpoint" "$LOG_DIR"/validator-*.log 2>/dev/null; then
        if [ -z "$PROPOSED" ]; then
            echo -e "\n${GREEN}✓ Checkpoint proposed${NC}"
            PROPOSED=1
        fi
    fi

    # Check for threshold reached
    if grep -q "Threshold reached" "$LOG_DIR"/validator-*.log 2>/dev/null; then
        if [ -z "$THRESHOLD" ]; then
            echo -e "\n${GREEN}✓ Threshold signatures collected${NC}"
            THRESHOLD=1
        fi
    fi

    # Check for finalized
    if grep -q "Finalized checkpoint" "$LOG_DIR"/validator-*.log 2>/dev/null; then
        if [ -z "$FINALIZED" ]; then
            echo -e "\n${GREEN}✓ Checkpoint finalized${NC}"
            FINALIZED=1
        fi
    fi

    # Check for ValidationBundle submitted
    if grep -q "Successfully submitted validation bundle" "$LOG_DIR"/validator-*.log 2>/dev/null; then
        if [ -z "$SUBMITTED" ]; then
            echo -e "\n${GREEN}✓ ValidationBundle submitted to RootLayer${NC}"
            SUBMITTED=1
            break
        fi
    fi
done

echo -e "\n\n${CYAN}=== Test Results ===${NC}"

# Count signatures per validator
echo -e "\n${BLUE}Signature Collection:${NC}"
for i in 1 2 3 4; do
    count=$(grep -c "Added signature" "$LOG_DIR/validator-$i.log" 2>/dev/null || echo "0")
    echo "  Validator $i: $count signatures"
done

# Check consensus state
echo -e "\n${BLUE}Consensus State:${NC}"
for i in 1 2 3 4; do
    if grep -q "Finalized checkpoint" "$LOG_DIR/validator-$i.log" 2>/dev/null; then
        epoch=$(grep "Finalized checkpoint" "$LOG_DIR/validator-$i.log" | tail -1 | grep -o "epoch[0-9]*" | grep -o "[0-9]*")
        echo -e "  Validator $i: ${GREEN}Finalized epoch $epoch${NC}"
    else
        echo "  Validator $i: No finalized checkpoint"
    fi
done

# Check ValidationBundle
echo -e "\n${BLUE}ValidationBundle Status:${NC}"
if grep -q "Successfully submitted validation bundle" "$LOG_DIR"/validator-*.log 2>/dev/null; then
    validator=$(grep -l "Successfully submitted validation bundle" "$LOG_DIR"/validator-*.log | head -1 | grep -o "[0-9]")
    intent=$(grep "Successfully submitted validation bundle" "$LOG_DIR/validator-$validator.log" | grep -o "intent_id[^ ]*" | head -1)
    echo -e "  ${GREEN}✓ ValidationBundle submitted by validator-$validator${NC}"
    echo "  Intent: $intent"
else
    echo -e "  ${YELLOW}⚠ No ValidationBundle submission detected${NC}"
fi

echo -e "\n${CYAN}=== Log Files ===${NC}"
echo "Logs available at: $LOG_DIR/"
echo "To view:"
echo "  tail -f $LOG_DIR/validator-1.log"
echo "  tail -f $LOG_DIR/agent.log"

echo -e "\n${CYAN}Press Ctrl+C to stop all services${NC}"
echo "Services will keep running for inspection..."

# Cleanup function
cleanup() {
    echo -e "\n${CYAN}=== Cleaning up ===${NC}"
    pkill -f "bin/validator" 2>/dev/null || true
    pkill -f "test-agent" 2>/dev/null || true
    echo "Done"
}

trap cleanup EXIT INT TERM

# Keep running
while true; do
    sleep 5
done
