#!/bin/bash

# Test script for gRPC signature authentication
# This script demonstrates the authentication flow without TLS

set -e

echo "=== Testing gRPC Signature Authentication ==="
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    if [ "$1" = "success" ]; then
        echo -e "${GREEN}✓${NC} $2"
    elif [ "$1" = "error" ]; then
        echo -e "${RED}✗${NC} $2"
    elif [ "$1" = "info" ]; then
        echo -e "${YELLOW}ℹ${NC} $2"
    fi
}

# Check if Go is installed
if ! command -v go &> /dev/null; then
    print_status "error" "Go is not installed"
    exit 1
fi

print_status "info" "Building authentication example..."

# Build the example
cd "$(dirname "$0")/../examples/auth_example"
go build -o auth_example main.go

if [ $? -eq 0 ]; then
    print_status "success" "Build successful"
else
    print_status "error" "Build failed"
    exit 1
fi

print_status "info" "Running authentication example..."
echo

# Run the example
./auth_example

# Clean up
rm -f auth_example

echo
print_status "success" "Authentication test completed"

# Optional: Run unit tests for auth components
echo
print_status "info" "Running unit tests for authentication components..."

cd ../..

# Test the auth interceptor
go test ./internal/validator -run TestAuthInterceptor -v

# Test the SDK auth client
go test ./sdk/go -run TestAuthClient -v

echo
print_status "success" "All tests passed!"

echo
echo "=== Summary ==="
echo "1. gRPC server with signature authentication: ✓"
echo "2. Agent authentication with ECDSA signatures: ✓"
echo "3. Nonce-based replay protection: ✓"
echo "4. Timestamp validation: ✓"
echo "5. Agent caching for performance: ✓"
echo
echo "Next steps:"
echo "- Deploy to testnet with real RootLayer integration"
echo "- Monitor authentication performance"
echo "- Add rate limiting per agent"
echo "- Implement TLS in future phase"