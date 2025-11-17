#!/bin/bash

# Participant Registration Script
# This script registers Validator, Matcher, and Agent using the SDK

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Load .env file if it exists
if [ -f "$PROJECT_ROOT/.env" ]; then
    echo "Loading environment from .env file..."
    set -a
    source "$PROJECT_ROOT/.env"
    set +a
    echo "✓ .env file loaded"
fi

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default values
CONFIG_FILE="${PROJECT_ROOT}/config/config.yaml"
NETWORK="${NETWORK:-base_sepolia}"
RPC_URL="${RPC_URL:-https://sepolia.base.org}"
SUBNET_CONTRACT="${SUBNET_CONTRACT}"
PRIVATE_KEY="${PRIVATE_KEY}"
DOMAIN="${DOMAIN:-validator.example.com}"

# Fixed smart contract addresses for Base Sepolia (updated 2025-11-03)
# These are protocol-wide addresses, same for all users
export PIN_BASE_SEPOLIA_INTENT_MANAGER="${PIN_BASE_SEPOLIA_INTENT_MANAGER:-0xB2f092E696B33b7a95e1f961369Bb59611CAd093}"
export PIN_BASE_SEPOLIA_SUBNET_FACTORY="${PIN_BASE_SEPOLIA_SUBNET_FACTORY:-0x2b5D7032297Df52ADEd7020c3B825f048Cd2df3E}"
export PIN_BASE_SEPOLIA_STAKING_MANAGER="${PIN_BASE_SEPOLIA_STAKING_MANAGER:-0x7f887e88014e3AF57526B68b431bA16e6968C015}"
export PIN_BASE_SEPOLIA_CHECKPOINT_MANAGER="${PIN_BASE_SEPOLIA_CHECKPOINT_MANAGER:-0x6A61BA20D910576A6c0B39175A6CF98358bB4008}"

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
        --subnet)
            SUBNET_CONTRACT="$2"
            shift 2
            ;;
        --key)
            PRIVATE_KEY="$2"
            shift 2
            ;;
        --domain)
            DOMAIN="$2"
            shift 2
            ;;
        --check)
            CHECK_ONLY="true"
            shift
            ;;
        --dry-run)
            DRY_RUN="true"
            shift
            ;;
        --skip-validator)
            SKIP_VALIDATOR="true"
            shift
            ;;
        --skip-matcher)
            SKIP_MATCHER="true"
            shift
            ;;
        --skip-agent)
            SKIP_AGENT="true"
            shift
            ;;
        --help)
            cat << EOF
Usage: $0 [OPTIONS]

Register Validator, Matcher, and Agent on the subnet.

Options:
  --config FILE         Path to config file (default: config/config.yaml)
  --network NAME        Network name (default: base_sepolia)
  --rpc URL             RPC URL (overrides config)
  --subnet ADDRESS      Subnet contract address (overrides config)
  --key HEX             Private key hex (overrides config)
  --domain DOMAIN       Participant domain (default: validator.example.com)
  --check               Only check registration status
  --dry-run             Dry run (don't submit transactions)
  --skip-validator      Skip validator registration
  --skip-matcher        Skip matcher registration
  --skip-agent          Skip agent registration
  --help                Show this help message

Environment Variables:
  NETWORK               Network name
  RPC_URL               RPC URL
  SUBNET_CONTRACT       Subnet contract address
  PRIVATE_KEY           Private key hex
  DOMAIN                Participant domain

Examples:
  # Register all participants using config file
  $0

  # Check registration status only
  $0 --check

  # Register with custom parameters
  $0 --rpc https://sepolia.base.org \\
     --subnet 0x123... \\
     --key 0xabc... \\
     --domain my-subnet.com

  # Dry run to see what would happen
  $0 --dry-run

  # Register only validator
  $0 --skip-matcher --skip-agent

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
echo -e "${YELLOW}📦 Building registration script...${NC}"
cd "$PROJECT_ROOT"
go build -o "$PROJECT_ROOT/bin/register-participants" "$SCRIPT_DIR/register-participants.go"

if [ $? -ne 0 ]; then
    echo -e "${RED}❌ Failed to build registration script${NC}"
    exit 1
fi

# Prepare command arguments
ARGS=(
    -config "$CONFIG_FILE"
    -network "$NETWORK"
    -domain "$DOMAIN"
)

if [ -n "$RPC_URL" ]; then
    ARGS+=(-rpc "$RPC_URL")
fi

if [ -n "$SUBNET_CONTRACT" ]; then
    ARGS+=(-subnet "$SUBNET_CONTRACT")
fi

if [ -n "$PRIVATE_KEY" ]; then
    ARGS+=(-key "$PRIVATE_KEY")
fi

if [ "$CHECK_ONLY" = "true" ]; then
    ARGS+=(-check)
fi

if [ "$DRY_RUN" = "true" ]; then
    ARGS+=(-dry-run)
fi

if [ "$SKIP_VALIDATOR" = "true" ]; then
    ARGS+=(-skip-validator)
fi

if [ "$SKIP_MATCHER" = "true" ]; then
    ARGS+=(-skip-matcher)
fi

if [ "$SKIP_AGENT" = "true" ]; then
    ARGS+=(-skip-agent)
fi

# Run the registration script
echo -e "${GREEN}🚀 Running registration script...${NC}"
"$PROJECT_ROOT/bin/register-participants" "${ARGS[@]}"

EXIT_CODE=$?

if [ $EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}✅ Registration completed successfully!${NC}"
else
    echo -e "${RED}❌ Registration failed with exit code $EXIT_CODE${NC}"
    exit $EXIT_CODE
fi
