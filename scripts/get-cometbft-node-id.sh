#!/bin/bash
# Extract CometBFT node ID from node_key.json
# Node ID = first 20 bytes of SHA256(pubkey_bytes)

if [ $# -lt 1 ]; then
    echo "Usage: $0 <node_key.json>" >&2
    exit 1
fi

NODE_KEY_FILE="$1"

if [ ! -f "$NODE_KEY_FILE" ]; then
    echo "Error: $NODE_KEY_FILE not found" >&2
    exit 1
fi

# Extract the base64 pubkey value from the private key and compute node ID
# CometBFT node ID is the first 20 bytes (40 hex chars) of SHA256(pubkey)
PUBKEY_B64=$(jq -r '.priv_key.value' "$NODE_KEY_FILE")

# Ed25519 private key is 64 bytes: 32 bytes private + 32 bytes public
# We need the last 32 bytes (the public key part)
NODE_ID=$(echo "$PUBKEY_B64" | base64 -d | tail -c 32 | sha256sum | cut -d' ' -f1 | cut -c1-40)

echo "$NODE_ID"
