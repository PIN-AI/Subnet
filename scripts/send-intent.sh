#!/bin/bash
# PinAI Subnet Interactive Intent Sender
# 交互式发送 Intent

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Project root (parent directory of scripts/)
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

print_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_header() {
    echo -e "${CYAN}$1${NC}"
}

echo ""
print_header "╔═══════════════════════════════════════╗"
print_header "║  PinAI Subnet Intent Sender          ║"
print_header "║  交互式 Intent 提交工具               ║"
print_header "╚═══════════════════════════════════════╝"
echo ""

# Load environment from .env if exists
if [ -f "$PROJECT_ROOT/.env" ]; then
    print_info "Loading environment from .env..."
    set -a
    source "$PROJECT_ROOT/.env"
    set +a
else
    print_warning ".env file not found"
fi

# Check required environment variables
if [ -z "$TEST_PRIVATE_KEY" ]; then
    print_error "TEST_PRIVATE_KEY not set"
    echo ""
    echo "Please set your test private key:"
    echo "  export TEST_PRIVATE_KEY=\"your_test_private_key_here\""
    echo ""
    echo "Or create a .env file (see .env.example)"
    exit 1
fi

# Set default values
SUBNET_ID="${SUBNET_ID:-0x0000000000000000000000000000000000000000000000000000000000000003}"
ROOTLAYER_HTTP="${ROOTLAYER_HTTP:-http://3.17.208.238:8081}"
INTENT_MANAGER="${PIN_BASE_SEPOLIA_INTENT_MANAGER:-0xB2f092E696B33b7a95e1f961369Bb59611CAd093}"
CHAIN_RPC_URL="${CHAIN_RPC_URL:-https://sepolia.base.org}"
CHAIN_NETWORK="${CHAIN_NETWORK:-base_sepolia}"

# Check if submit-intent-signed binary exists
if [ ! -f "$PROJECT_ROOT/bin/submit-intent-signed" ]; then
    print_warning "submit-intent-signed binary not found"
    print_info "Building binary..."
    cd "$PROJECT_ROOT"
    make build-submit-intent-signed
    print_success "Binary built"
fi

echo ""
print_header "Current Configuration:"
echo "   Subnet ID: $SUBNET_ID"
echo "   RootLayer: $ROOTLAYER_HTTP"
echo "   Network: $CHAIN_NETWORK"
echo "   Intent Manager: $INTENT_MANAGER"
echo ""

# Function to submit intent
submit_intent() {
    local intent_type="$1"
    local params_json="$2"
    local amount_wei="$3"

    print_info "Submitting Intent..."
    echo ""
    echo "📋 Intent Details:"
    echo "   Type: $intent_type"
    echo "   Params: $params_json"
    echo "   Amount: $amount_wei WEI"
    echo ""

    # Run submit-intent-signed
    PIN_BASE_SEPOLIA_INTENT_MANAGER="$INTENT_MANAGER" \
    RPC_URL="$CHAIN_RPC_URL" \
    PRIVATE_KEY="$TEST_PRIVATE_KEY" \
    PIN_NETWORK="$CHAIN_NETWORK" \
    ROOTLAYER_HTTP="$ROOTLAYER_HTTP" \
    SUBNET_ID="$SUBNET_ID" \
    INTENT_TYPE="$intent_type" \
    PARAMS_JSON="$params_json" \
    AMOUNT_WEI="$amount_wei" \
    "$PROJECT_ROOT/bin/submit-intent-signed"

    if [ $? -eq 0 ]; then
        echo ""
        print_success "Intent submitted successfully!"
        echo ""
    else
        echo ""
        print_error "Intent submission failed"
        echo ""
        return 1
    fi
}

# Main loop
while true; do
    print_header "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "Choose an option / 选择操作:"
    echo ""
    echo "  1) Submit custom Intent / 提交自定义 Intent"
    echo "  2) Submit E2E test Intent / 提交 E2E 测试 Intent"
    echo "  3) Submit demo Intent / 提交演示 Intent"
    echo "  4) View configuration / 查看配置"
    echo "  5) Exit / 退出"
    echo ""
    read -p "Enter choice [1-5]: " choice
    echo ""

    case $choice in
        1)
            # Custom Intent
            print_header "Custom Intent Submission / 自定义 Intent 提交"
            echo ""

            read -p "Intent Type (e.g., demo-intent, e2e-test): " intent_type
            if [ -z "$intent_type" ]; then
                print_error "Intent type cannot be empty"
                continue
            fi

            echo ""
            echo "Intent Parameters (JSON format)"
            echo "Example: {\"task\":\"process data\",\"priority\":\"high\"}"
            read -p "Params JSON: " params_json
            if [ -z "$params_json" ]; then
                print_error "Params cannot be empty"
                continue
            fi

            echo ""
            echo "Amount in WEI (1 ETH = 10^18 WEI)"
            echo "Examples:"
            echo "  - 100000000000000 (0.0001 ETH)"
            echo "  - 1000000000000000 (0.001 ETH)"
            echo "  - 10000000000000000 (0.01 ETH)"
            read -p "Amount (WEI): " amount_wei
            if [ -z "$amount_wei" ]; then
                print_error "Amount cannot be empty"
                continue
            fi

            echo ""
            submit_intent "$intent_type" "$params_json" "$amount_wei"
            ;;

        2)
            # E2E Test Intent
            print_header "E2E Test Intent / E2E 测试 Intent"
            echo ""

            intent_type="e2e-test"
            timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
            params_json="{\"task\":\"E2E flow test\",\"timestamp\":\"$timestamp\"}"
            amount_wei="100000000000000"

            echo "Using defaults for E2E test:"
            echo "  Type: $intent_type"
            echo "  Params: $params_json"
            echo "  Amount: $amount_wei WEI (0.0001 ETH)"
            echo ""

            read -p "Proceed? [Y/n]: " confirm
            if [[ "$confirm" =~ ^[Nn] ]]; then
                print_info "Cancelled"
                continue
            fi

            echo ""
            submit_intent "$intent_type" "$params_json" "$amount_wei"
            ;;

        3)
            # Demo Intent
            print_header "Demo Intent / 演示 Intent"
            echo ""

            intent_type="demo-intent"
            params_json="{\"task\":\"demo task\",\"demo\":true}"
            amount_wei="100000000000000"

            echo "Using demo defaults:"
            echo "  Type: $intent_type"
            echo "  Params: $params_json"
            echo "  Amount: $amount_wei WEI (0.0001 ETH)"
            echo ""

            read -p "Proceed? [Y/n]: " confirm
            if [[ "$confirm" =~ ^[Nn] ]]; then
                print_info "Cancelled"
                continue
            fi

            echo ""
            submit_intent "$intent_type" "$params_json" "$amount_wei"
            ;;

        4)
            # View Configuration
            print_header "Current Configuration / 当前配置"
            echo ""
            echo "Environment / 环境:"
            echo "  SUBNET_ID: $SUBNET_ID"
            echo "  ROOTLAYER_HTTP: $ROOTLAYER_HTTP"
            echo "  CHAIN_RPC_URL: $CHAIN_RPC_URL"
            echo "  CHAIN_NETWORK: $CHAIN_NETWORK"
            echo ""
            echo "Contract Addresses / 合约地址:"
            echo "  Intent Manager: $INTENT_MANAGER"
            echo ""
            echo "Binary / 二进制:"
            echo "  submit-intent-signed: $PROJECT_ROOT/bin/submit-intent-signed"
            echo ""

            if [ -f "$PROJECT_ROOT/.env" ]; then
                echo "Config source: .env file"
            else
                echo "Config source: environment variables"
            fi
            echo ""

            read -p "Press Enter to continue..."
            ;;

        5)
            # Exit
            print_info "Exiting..."
            echo ""
            exit 0
            ;;

        *)
            print_error "Invalid choice. Please enter 1-5."
            echo ""
            ;;
    esac
done
