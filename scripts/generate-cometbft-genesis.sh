#!/bin/bash
# Generate CometBFT genesis.json without requiring cometbft CLI
# Usage: ./generate-cometbft-genesis.sh <validators_dir>

set -e

if [ $# -lt 1 ]; then
    echo "Usage: $0 <validators_dir>"
    exit 1
fi

VALIDATORS_DIR="$1"

echo "Generating CometBFT genesis.json from validators in $VALIDATORS_DIR"

# Find all validator directories
VALIDATOR_DIRS=$(find "$VALIDATORS_DIR" -maxdepth 1 -type d -name "validator*" | sort)

if [ -z "$VALIDATOR_DIRS" ]; then
    echo "Error: No validator directories found in $VALIDATORS_DIR"
    exit 1
fi

# Count validators
NUM_VALIDATORS=$(echo "$VALIDATOR_DIRS" | wc -l | tr -d ' ')
echo "Found $NUM_VALIDATORS validators"

# Build validators array for genesis
VALIDATORS_JSON=""
FIRST=true

for VALIDATOR_DIR in $VALIDATOR_DIRS; do
    PRIV_VALIDATOR_KEY="$VALIDATOR_DIR/cometbft/config/priv_validator_key.json"

    if [ ! -f "$PRIV_VALIDATOR_KEY" ]; then
        echo "Warning: priv_validator_key.json not found in $VALIDATOR_DIR"
        continue
    fi

    # Extract fields using jq for proper parsing
    ADDRESS=$(jq -r '.address' "$PRIV_VALIDATOR_KEY")
    PUBKEY=$(jq -r '.pub_key.value' "$PRIV_VALIDATOR_KEY")

    if [ "$FIRST" = true ]; then
        FIRST=false
    else
        VALIDATORS_JSON="$VALIDATORS_JSON,"
    fi

    VALIDATORS_JSON="$VALIDATORS_JSON
    {
      \"address\": \"$ADDRESS\",
      \"pub_key\": {
        \"type\": \"tendermint/PubKeyEd25519\",
        \"value\": \"$PUBKEY\"
      },
      \"power\": \"10\",
      \"name\": \"$(basename $VALIDATOR_DIR)\"
    }"

    echo "  Added validator: $(basename $VALIDATOR_DIR)"
done

# Generate genesis.json
GENESIS_CONTENT="{
  \"genesis_time\": \"$(date -u +"%Y-%m-%dT%H:%M:%S.000000Z")\",
  \"chain_id\": \"subnet-chain\",
  \"initial_height\": \"0\",
  \"consensus_params\": {
    \"block\": {
      \"max_bytes\": \"22020096\",
      \"max_gas\": \"-1\"
    },
    \"evidence\": {
      \"max_age_num_blocks\": \"100000\",
      \"max_age_duration\": \"172800000000000\",
      \"max_bytes\": \"1048576\"
    },
    \"validator\": {
      \"pub_key_types\": [\"ed25519\"]
    },
    \"version\": {
      \"app\": \"1\"
    }
  },
  \"validators\": [$VALIDATORS_JSON
  ],
  \"app_hash\": \"\",
  \"app_state\": {}
}"

# Write genesis to all validator directories
for VALIDATOR_DIR in $VALIDATOR_DIRS; do
    GENESIS_FILE="$VALIDATOR_DIR/cometbft/config/genesis.json"
    echo "$GENESIS_CONTENT" > "$GENESIS_FILE"
    echo "Wrote genesis to $GENESIS_FILE"
done

echo ""
echo "✓ Generated genesis.json with $NUM_VALIDATORS validators"
echo ""
echo "Chain ID: subnet-chain"
echo "Total voting power: $((NUM_VALIDATORS * 10))"
