#!/bin/bash
set -e

# Usage message
usage() {
    cat << EOF
Usage: $0 <validators_json_file>

Register validators to a subnet using their private keys.

Arguments:
    validators_json_file    Path to JSON file containing validator info

Required environment variables:
    RPC_URL                 RPC endpoint (e.g., https://sepolia.base.org)
    PIN_NETWORK             Network name (e.g., base_sepolia)
    SUBNET_ID               Subnet ID (e.g., 0x00...02)
    SUBNET_CONTRACT         Subnet contract address
    PIN_BASE_SEPOLIA_STAKING_MANAGER  Staking manager contract address

Optional environment variables:
    VALIDATOR_STAKE         Stake amount in ETH (default: 0.001)
    BASE_PORT               Base port for validators (default: 9090)
    DOMAIN_SUFFIX           Domain suffix for validators (default: test.pinai.xyz)

Example:
    export RPC_URL="https://sepolia.base.org"
    export PIN_NETWORK="base_sepolia"
    export SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000009"
    export SUBNET_CONTRACT="0x5697DFA452a8cA1598a9CA736b3E9E75dA1a43A6"
    export PIN_BASE_SEPOLIA_STAKING_MANAGER="0xAc11AE66c7831A70Bea940b0AE16c967f940cB65"
    $0 validators.json

EOF
    exit 1
}

# Check arguments
if [ $# -lt 1 ]; then
    echo "❌ Error: Missing validators JSON file"
    usage
fi

VALIDATORS_FILE="$1"

# Check if file exists
if [ ! -f "$VALIDATORS_FILE" ]; then
    echo "❌ Validators file not found: $VALIDATORS_FILE"
    exit 1
fi

# Check required environment variables
REQUIRED_VARS=(
    "RPC_URL"
    "PIN_NETWORK"
    "SUBNET_ID"
    "SUBNET_CONTRACT"
    "PIN_BASE_SEPOLIA_STAKING_MANAGER"
)

for var in "${REQUIRED_VARS[@]}"; do
    if [ -z "${!var}" ]; then
        echo "❌ Error: Required environment variable $var is not set"
        usage
    fi
done

# Set optional variables with defaults
VALIDATOR_STAKE="${VALIDATOR_STAKE:-0.001}"
BASE_PORT="${BASE_PORT:-9090}"
DOMAIN_SUFFIX="${DOMAIN_SUFFIX:-test.pinai.xyz}"

# Get script directory and project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Check if register-participants binary exists
REGISTER_BIN="$PROJECT_ROOT/bin/register-participants"
if [ ! -f "$REGISTER_BIN" ]; then
    echo "❌ Error: register-participants binary not found at $REGISTER_BIN"
    echo "   Please run: make build"
    exit 1
fi

echo "🚀 Registering validators to Subnet:"
echo "   Network: $PIN_NETWORK"
echo "   RPC: $RPC_URL"
echo "   Subnet ID: $SUBNET_ID"
echo "   Subnet Contract: $SUBNET_CONTRACT"
echo "   Stake: $VALIDATOR_STAKE ETH"
echo ""

# Get validator count
VALIDATOR_COUNT=$(python3 << EOF
import json
import sys
try:
    with open('$VALIDATORS_FILE', 'r') as f:
        validators = json.load(f)
    print(len(validators))
except Exception as e:
    print(f"Error reading validators file: {e}", file=sys.stderr)
    sys.exit(1)
EOF
)

if [ -z "$VALIDATOR_COUNT" ] || [ "$VALIDATOR_COUNT" -eq 0 ]; then
    echo "❌ Error: No validators found in $VALIDATORS_FILE"
    exit 1
fi

echo "📋 Found $VALIDATOR_COUNT validators in file"
echo ""

# Extract and display validator info
echo "Validators to register:"
python3 << EOF
import json

with open('$VALIDATORS_FILE', 'r') as f:
    validators = json.load(f)

for i, v in enumerate(validators, 1):
    print(f"  {i}. {v['address']}")
EOF
echo ""

# Register each validator
for i in $(seq 0 $((VALIDATOR_COUNT - 1))); do
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "📝 Registering validator $((i+1))/$VALIDATOR_COUNT..."

    # Extract validator info using Python
    VALIDATOR_INFO=$(python3 << EOF
import json
import sys
try:
    with open('$VALIDATORS_FILE', 'r') as f:
        validators = json.load(f)
    if $i >= len(validators):
        print(f"Error: Validator index $i out of range", file=sys.stderr)
        sys.exit(1)
    v = validators[$i]
    print(f"{v['address']}|{v['private_key']}")
except Exception as e:
    print(f"Error: {e}", file=sys.stderr)
    sys.exit(1)
EOF
)

    if [ $? -ne 0 ]; then
        echo "❌ Failed to extract validator info for index $i"
        exit 1
    fi

    VALIDATOR_ADDR=$(echo "$VALIDATOR_INFO" | cut -d'|' -f1)
    VALIDATOR_KEY=$(echo "$VALIDATOR_INFO" | cut -d'|' -f2)

    echo "   Address: $VALIDATOR_ADDR"

    # Run registration
    PORT=$((BASE_PORT + i))
    DOMAIN="validator-$((i+1)).$DOMAIN_SUFFIX"

    "$REGISTER_BIN" \
        -key "$VALIDATOR_KEY" \
        -network "$PIN_NETWORK" \
        -rpc "$RPC_URL" \
        -subnet "$SUBNET_CONTRACT" \
        -skip-agent \
        -skip-matcher \
        -validator-stake "$VALIDATOR_STAKE" \
        -validator-port "$PORT" \
        -domain "$DOMAIN"

    if [ $? -eq 0 ]; then
        echo "   ✅ Validator $((i+1)) registered successfully!"
    else
        echo "   ❌ Failed to register validator $((i+1))"
        exit 1
    fi

    echo ""
    sleep 2
done

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ All $VALIDATOR_COUNT validators registered successfully!"
echo ""
