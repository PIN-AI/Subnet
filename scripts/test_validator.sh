#!/bin/bash

# Test script for validator node

echo "Building validator..."
cd "$(dirname "$0")/.." || exit 1

# Build validator
go build -o bin/validator ./cmd/validator/

if [ $? -ne 0 ]; then
    echo "Build failed!"
    exit 1
fi

echo "Build successful! Validator binary created at bin/validator"

# Display help
echo ""
echo "To run the validator:"
echo "  ./bin/validator --id validator-1 --key <private-key-hex> --grpc 9090"
echo ""
echo "Options:"
echo "  --id          Validator ID"
echo "  --key         Private key hex (will generate if not provided)"
echo "  --grpc        gRPC server port (default: 9090)"
echo "  --nats        NATS server URL (default: nats://127.0.0.1:4222)"
echo "  --storage     Storage path (default: ./data)"
echo "  --validators  Total validator count (default: 4)"
echo ""
echo "Example:"
echo "  ./bin/validator --id validator-1 --grpc 9090"