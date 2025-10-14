#!/bin/bash

# Subnet Creation Script
# This script creates a new subnet and saves its information

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default values
CONFIG_FILE="${PROJECT_ROOT}/config/config.yaml"
NETWORK="${NETWORK:-base_sepolia}"
RPC_URL="${RPC_URL:-https://sepolia.base.org}"
PRIVATE_KEY="${PRIVATE_KEY}"
SUBNET_NAME="${SUBNET_NAME:-My Test Subnet}"
AUTO_APPROVE="${AUTO_APPROVE:-true}"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --config)
            CONFIG_FILE="$2"
            shift 2
            ;;
        --network)
            NETWORK="$2"
            shift 2
            ;;
        --rpc)
            RPC_URL="$2"
            shift 2
            ;;
        --key)
            PRIVATE_KEY="$2"
            shift 2
            ;;
        --name)
            SUBNET_NAME="$2"
            shift 2
            ;;
        --auto-approve)
            AUTO_APPROVE="$2"
            shift 2
            ;;
        --help)
            cat << EOF
Usage: $0 [OPTIONS]

Create a new subnet on the blockchain.

Options:
  --config FILE         Path to config file (default: config/config.yaml)
  --network NAME        Network name (default: base_sepolia)
  --rpc URL             RPC URL (overrides config)
  --key HEX             Private key hex (overrides config)
  --name NAME           Subnet name (default: "My Test Subnet")
  --auto-approve BOOL   Auto approve participants (default: true)
  --help                Show this help message

Environment Variables:
  NETWORK               Network name
  RPC_URL               RPC URL
  PRIVATE_KEY           Private key hex
  SUBNET_NAME           Subnet name

Examples:
  # Create subnet with default settings
  $0

  # Create subnet with custom name
  $0 --name "Production Subnet"

  # Create subnet with manual approval
  $0 --auto-approve false

EOF
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Build the Go script
echo -e "${YELLOW}📦 Building subnet creation script...${NC}"
cd "$PROJECT_ROOT"
go build -o "$PROJECT_ROOT/bin/create-subnet" "$SCRIPT_DIR/create-subnet.go"

if [ $? -ne 0 ]; then
    echo -e "${RED}❌ Failed to build subnet creation script${NC}"
    exit 1
fi

# Prepare command arguments
ARGS=(
    -config "$CONFIG_FILE"
    -network "$NETWORK"
    -name "$SUBNET_NAME"
)

if [ -n "$RPC_URL" ]; then
    ARGS+=(-rpc "$RPC_URL")
fi

if [ -n "$PRIVATE_KEY" ]; then
    ARGS+=(-key "$PRIVATE_KEY")
fi

if [ -n "$AUTO_APPROVE" ]; then
    ARGS+=(-auto-approve="$AUTO_APPROVE")
fi

# Run the subnet creation script
echo -e "${GREEN}🚀 Running subnet creation script...${NC}"
"$PROJECT_ROOT/bin/create-subnet" "${ARGS[@]}"

EXIT_CODE=$?

if [ $EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}✅ Subnet creation completed successfully!${NC}"
else
    echo -e "${RED}❌ Subnet creation failed with exit code $EXIT_CODE${NC}"
    exit $EXIT_CODE
fi
