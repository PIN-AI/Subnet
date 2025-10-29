#!/bin/bash
# Generate CometBFT validator keys using Go tool
# Usage: ./generate-cometbft-keys.sh <output_dir> <num_validators>

set -e

if [ $# -lt 2 ]; then
    echo "Usage: $0 <output_dir> <num_validators>"
    exit 1
fi

OUTPUT_DIR="$1"
NUM_VALIDATORS="$2"

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "Generating CometBFT configuration for $NUM_VALIDATORS validators in $OUTPUT_DIR"

# Build key generator if not already built
if [ ! -f "$PROJECT_ROOT/bin/gen-cometbft-keys" ]; then
    echo "Building key generator..."
    go build -o "$PROJECT_ROOT/bin/gen-cometbft-keys" "$PROJECT_ROOT/cmd/gen-cometbft-keys"
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Generate keys for each validator
for i in $(seq 1 $NUM_VALIDATORS); do
    VALIDATOR_HOME="$OUTPUT_DIR/validator${i}/cometbft"
    CONFIG_DIR="$VALIDATOR_HOME/config"
    DATA_DIR="$VALIDATOR_HOME/data"

    echo "Setting up validator$i..."
    mkdir -p "$CONFIG_DIR"
    mkdir -p "$DATA_DIR"

    # Generate keys using our tool
    "$PROJECT_ROOT/bin/gen-cometbft-keys" "$CONFIG_DIR" "$DATA_DIR"

    echo "  ✓ validator$i configured"
done

echo ""
echo "✓ Generated configuration for $NUM_VALIDATORS validators"
echo ""
echo "Keys are properly formatted Ed25519 keys for CometBFT."
