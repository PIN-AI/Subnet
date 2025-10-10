#!/bin/bash

# Simple script to test leader rotation by running validators for multiple epochs

set -e

echo "================================"
echo "Leader Rotation Test"
echo "================================"
echo ""

# Clean up any old processes
pkill -f "./bin/validator" 2>/dev/null || true
pkill -f "./bin/matcher" 2>/dev/null || true
sleep 1

# Clean logs
rm -rf /tmp/leader-rotation-test
mkdir -p /tmp/leader-rotation-test

echo "Starting 4 validators..."

# Start validators with short checkpoint intervals (10 seconds)
VALIDATOR_PUBKEYS="02d5c4f8c5e8a9f3b2c1d7e6a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7,03e6d5f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6,04f7e6a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7,05a8f7b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7"

# Start validator 1 (will be leader for epoch 0)
VALIDATOR_ID="validator-e2e-1" \
  GRPC_PORT="9200" \
  REGISTRY_GRPC_PORT="8091" \
  REGISTRY_HTTP_PORT="8101" \
  PRIVATE_KEY="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" \
  VALIDATOR_COUNT="4" \
  THRESHOLD_NUM="3" \
  THRESHOLD_DENOM="4" \
  VALIDATOR_PUBKEYS="$VALIDATOR_PUBKEYS" \
  CHECKPOINT_INTERVAL="10s" \
  ./bin/validator > /tmp/leader-rotation-test/validator-1.log 2>&1 &

# Start validator 2 (will be leader for epoch 1)
VALIDATOR_ID="validator-e2e-2" \
  GRPC_PORT="9201" \
  REGISTRY_GRPC_PORT="8092" \
  REGISTRY_HTTP_PORT="8102" \
  PRIVATE_KEY="123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0" \
  VALIDATOR_COUNT="4" \
  THRESHOLD_NUM="3" \
  THRESHOLD_DENOM="4" \
  VALIDATOR_PUBKEYS="$VALIDATOR_PUBKEYS" \
  CHECKPOINT_INTERVAL="10s" \
  ./bin/validator > /tmp/leader-rotation-test/validator-2.log 2>&1 &

# Start validator 3 (will be leader for epoch 2)
VALIDATOR_ID="validator-e2e-3" \
  GRPC_PORT="9202" \
  REGISTRY_GRPC_PORT="8093" \
  REGISTRY_HTTP_PORT="8103" \
  PRIVATE_KEY="23456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef01" \
  VALIDATOR_COUNT="4" \
  THRESHOLD_NUM="3" \
  THRESHOLD_DENOM="4" \
  VALIDATOR_PUBKEYS="$VALIDATOR_PUBKEYS" \
  CHECKPOINT_INTERVAL="10s" \
  ./bin/validator > /tmp/leader-rotation-test/validator-3.log 2>&1 &

# Start validator 4 (will be leader for epoch 3)
VALIDATOR_ID="validator-e2e-4" \
  GRPC_PORT="9203" \
  REGISTRY_GRPC_PORT="8094" \
  REGISTRY_HTTP_PORT="8104" \
  PRIVATE_KEY="3456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef012" \
  VALIDATOR_COUNT="4" \
  THRESHOLD_NUM="3" \
  THRESHOLD_DENOM="4" \
  VALIDATOR_PUBKEYS="$VALIDATOR_PUBKEYS" \
  CHECKPOINT_INTERVAL="10s" \
  ./bin/validator > /tmp/leader-rotation-test/validator-4.log 2>&1 &

echo "Waiting for validators to start..."
sleep 3

echo ""
echo "Validators running. Will monitor for 60 seconds to observe leader rotation..."
echo "Checkpoint interval: 10 seconds"
echo "Expected: Each validator should become leader for 1 epoch in rotation"
echo ""

# Wait for 60 seconds to observe multiple epochs
for i in {1..12}; do
  echo "=== Second $((i*5)) ==="

  # Check which validator is currently proposing checkpoints
  for v in 1 2 3 4; do
    last_log=$(tail -3 /tmp/leader-rotation-test/validator-$v.log 2>/dev/null | grep -E "Proposed checkpoint|Leader rotation|Became leader" | tail -1 || echo "")
    if [ -n "$last_log" ]; then
      echo "Validator $v: $last_log"
    fi
  done

  echo ""
  sleep 5
done

echo ""
echo "================================"
echo "Test Complete - Cleaning Up"
echo "================================"
echo ""

# Kill validators
pkill -f "./bin/validator" 2>/dev/null || true

echo "Final logs:"
echo ""
for v in 1 2 3 4; do
  echo "=== Validator $v ==="
  grep -E "Initial leadership|Proposed checkpoint|Leader rotation|Became leader|No longer leader" /tmp/leader-rotation-test/validator-$v.log 2>/dev/null | tail -10 || echo "No relevant logs"
  echo ""
done

echo "Full logs available at: /tmp/leader-rotation-test/"
