#!/bin/bash
# E2E Test Script - Complete Intent Flow
# Tests: Intent → Matcher → Agent → Validator → Checkpoint → ValidationBundle
#
# Usage: ./e2e-test.sh [OPTIONS]
#
# Options:
#   --rootlayer-grpc <addr>    RootLayer gRPC endpoint (default: 3.17.208.238:9001)
#   --rootlayer-http <url>     RootLayer HTTP endpoint (default: http://3.17.208.238:8080)
#   --no-interactive           Run test and exit without interactive mode
#
# Environment Variables:
#   ROOTLAYER_GRPC            Alternative to --rootlayer-grpc
#   ROOTLAYER_HTTP            Alternative to --rootlayer-http

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LOG_DIR="$PROJECT_ROOT/e2e-test-logs"
PIDS_FILE="$LOG_DIR/pids.txt"

# Ports
NATS_PORT=4222
MATCHER_PORT=8090
VALIDATOR_1_PORT=9200
VALIDATOR_2_PORT=9201
VALIDATOR_3_PORT=9202
VALIDATOR_4_PORT=9203

# RootLayer configuration (can be overridden by env vars or command-line args)
ROOTLAYER_GRPC="${ROOTLAYER_GRPC:-3.17.208.238:9001}"
ROOTLAYER_HTTP="${ROOTLAYER_HTTP:-http://3.17.208.238:8081}"

# Test configuration (can be overridden by env vars or command-line args)
SUBNET_ID="${SUBNET_ID:-0x1111111111111111111111111111111111111111111111111111111111111111}"
TEST_AGENT_ID="e2e-test-agent-001"

# Cleanup flag
CLEANUP_ON_EXIT=1

# Optional: On-chain assignment submission configuration
# Set ENABLE_CHAIN_SUBMIT=true to enable blockchain submission during E2E test
ENABLE_CHAIN_SUBMIT="${ENABLE_CHAIN_SUBMIT:-false}"
CHAIN_RPC_URL="${CHAIN_RPC_URL:-https://sepolia.base.org}"
CHAIN_NETWORK="${CHAIN_NETWORK:-base_sepolia}"
INTENT_MANAGER_ADDR="${INTENT_MANAGER_ADDR:-0xD04d23775D3B8e028e6104E31eb0F6c07206EB46}"
MATCHER_PRIVATE_KEY="${MATCHER_PRIVATE_KEY:-eef960cc05e034727123db159f1e54f0733b2f51d4a239978771aff320be5b9a}"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --rootlayer-grpc)
            ROOTLAYER_GRPC="$2"
            shift 2
            ;;
        --rootlayer-http)
            ROOTLAYER_HTTP="$2"
            shift 2
            ;;
        --no-interactive)
            NO_INTERACTIVE=1
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --rootlayer-grpc <addr>    RootLayer gRPC endpoint (default: 3.17.208.238:9001)"
            echo "  --rootlayer-http <url>     RootLayer HTTP endpoint (default: http://3.17.208.238:8081)"
            echo "  --no-interactive           Run test and exit without interactive mode"
            echo "  --help, -h                 Show this help message"
            echo ""
            echo "Environment Variables:"
            echo "  ROOTLAYER_GRPC            Alternative to --rootlayer-grpc"
            echo "  ROOTLAYER_HTTP            Alternative to --rootlayer-http"
            echo ""
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Print functions
print_header() {
    echo -e "\n${CYAN}================================${NC}"
    echo -e "${CYAN}$1${NC}"
    echo -e "${CYAN}================================${NC}\n"
}

print_step() {
    echo -e "${BLUE}[$(date '+%H:%M:%S')]${NC} ${GREEN}➜${NC} $1"
}

print_info() {
    echo -e "${BLUE}[$(date '+%H:%M:%S')]${NC} ${CYAN}ℹ${NC} $1"
}

print_success() {
    echo -e "${BLUE}[$(date '+%H:%M:%S')]${NC} ${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${BLUE}[$(date '+%H:%M:%S')]${NC} ${RED}✗${NC} $1"
}

print_warning() {
    echo -e "${BLUE}[$(date '+%H:%M:%S')]${NC} ${YELLOW}⚠${NC} $1"
}

# Cleanup function
cleanup() {
    if [ "$CLEANUP_ON_EXIT" -eq 1 ]; then
        print_header "Cleaning up..."

        if [ -f "$PIDS_FILE" ]; then
            print_step "Stopping all services..."
            while read pid; do
                if kill -0 "$pid" 2>/dev/null; then
                    kill "$pid" 2>/dev/null || true
                fi
            done < "$PIDS_FILE"
            rm -f "$PIDS_FILE"
        fi

        # Kill by name as backup
        pkill -f "bin/matcher" 2>/dev/null || true
        pkill -f "bin/validator" 2>/dev/null || true
        pkill -f "test-agent" 2>/dev/null || true

        print_success "Cleanup complete"
    fi
}

trap cleanup EXIT INT TERM

# Check if a port is in use
check_port() {
    local port=$1
    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null 2>&1; then
        return 0
    else
        return 1
    fi
}

# Wait for a port to be ready
wait_for_port() {
    local port=$1
    local service=$2
    local max_wait=30
    local count=0

    print_step "Waiting for $service on port $port..."
    while ! check_port $port; do
        sleep 1
        count=$((count + 1))
        if [ $count -ge $max_wait ]; then
            print_error "$service failed to start on port $port"
            return 1
        fi
    done
    print_success "$service is ready on port $port"
}

# Check prerequisites
check_prerequisites() {
    print_header "Checking Prerequisites"

    # Check if binaries exist
    if [ ! -f "$PROJECT_ROOT/bin/mock-rootlayer" ]; then
        print_error "mock-rootlayer binary not found. Run 'make build' first."
        exit 1
    fi

    if [ ! -f "$PROJECT_ROOT/bin/matcher" ]; then
        print_error "matcher binary not found. Run 'make build' first."
        exit 1
    fi

    if [ ! -f "$PROJECT_ROOT/bin/validator" ]; then
        print_error "validator binary not found. Run 'make build' first."
        exit 1
    fi

    # Check if NATS is running
    if ! check_port $NATS_PORT; then
        print_warning "NATS is not running on port $NATS_PORT"
        print_info "Attempting to start NATS with Docker..."

        if command -v docker &> /dev/null; then
            docker run -d --name nats-e2e-test -p 4222:4222 nats:latest &>/dev/null || true
            sleep 2

            if check_port $NATS_PORT; then
                print_success "NATS started successfully"
                echo "docker-nats" >> "$PIDS_FILE"
            else
                print_error "Failed to start NATS. Please start it manually: nats-server"
                exit 1
            fi
        else
            print_error "Docker not found. Please start NATS manually: nats-server"
            exit 1
        fi
    else
        print_success "NATS is running on port $NATS_PORT"
    fi
}

# Build test agent
build_test_agent() {
    print_header "Building Test Agent"
    cd "$PROJECT_ROOT/scripts/test-agent"
    go build -o test-agent validator_test_agent.go
    print_success "Test agent built successfully"
    cd "$PROJECT_ROOT"
}

# Check RootLayer connectivity
check_rootlayer() {
    print_header "Checking RootLayer Connection"

    mkdir -p "$LOG_DIR"

    print_step "Testing connection to RootLayer at $ROOTLAYER_GRPC..."

    # Extract host and port from ROOTLAYER_GRPC
    local grpc_host=$(echo "$ROOTLAYER_GRPC" | cut -d':' -f1)
    local grpc_port=$(echo "$ROOTLAYER_GRPC" | cut -d':' -f2)

    # Try to connect to RootLayer gRPC
    if nc -z -w 3 "$grpc_host" "$grpc_port" 2>/dev/null; then
        print_success "RootLayer gRPC is reachable at $ROOTLAYER_GRPC"
    else
        print_warning "Cannot reach RootLayer gRPC at $ROOTLAYER_GRPC"
        print_info "Test will continue but may not receive Intents from RootLayer"
    fi
}

# Start Matcher
start_matcher() {
    print_header "Starting Matcher"

    # Create temporary matcher config
    local matcher_config="$LOG_DIR/matcher-config.yaml"
    cat > "$matcher_config" <<EOF
subnet_id: "$SUBNET_ID"

identity:
  matcher_id: "matcher-e2e-test"
  subnet_id: "$SUBNET_ID"

network:
  nats_url: "nats://127.0.0.1:$NATS_PORT"
  matcher_grpc_port: ":$MATCHER_PORT"

limits:
  bidding_window_sec: 10
  max_bids_per_intent: 100

rootlayer:
  http_url: "$ROOTLAYER_HTTP"
  grpc_endpoint: "$ROOTLAYER_GRPC"
  connect_timeout: 10s
  request_timeout: 30s

agent:
  matcher:
    signer:
      type: "ecdsa"
      private_key: "$MATCHER_PRIVATE_KEY"

# Private key for matcher (used for signing and blockchain transactions)
private_key: "$MATCHER_PRIVATE_KEY"

# On-chain configuration (optional)
enable_chain_submit: $ENABLE_CHAIN_SUBMIT
chain_rpc_url: "$CHAIN_RPC_URL"
chain_network: "$CHAIN_NETWORK"
intent_manager_addr: "$INTENT_MANAGER_ADDR"
EOF

    "$PROJECT_ROOT/bin/matcher" \
        --config "$matcher_config" \
        --id "matcher-e2e-test" \
        --grpc ":$MATCHER_PORT" \
        --window 10 \
        > "$LOG_DIR/matcher.log" 2>&1 &

    echo $! >> "$PIDS_FILE"

    wait_for_port $MATCHER_PORT "Matcher"
}

# Start Validators
start_validators() {
    print_header "Starting Validators"

    # Use matcher's private key for validator (DO NOT use in production!)
    local MATCHER_KEY="eef960cc05e034727123db159f1e54f0733b2f51d4a239978771aff320be5b9a"
    local keys=("$MATCHER_KEY")

    # Public key for matcher's private key (derived using secp256k1)
    # Address: 0xfc5A111b714547fc2D1D796EAAbb68264ed4A132
    local pubkeys=("0482ea12c5481d481c7f9d7c1a2047401c6e2f855e4cee4d8df0aa197514f3456528ba6c55092b20b51478fd8cf62cde37f206621b3dd47c2be3d5c35e4889bf94")

    # Build the validator pubkeys parameter (id:pubkey pairs) - SINGLE VALIDATOR
    local validator_id="validator-e2e-1"
    local pubkey="${pubkeys[0]}"
    local validator_pubkeys="${validator_id}:${pubkey}"

    # Create shared validator set config - SINGLE VALIDATOR
    local validator_config="$LOG_DIR/validator-set-config.yaml"
    cat > "$validator_config" <<EOF
subnet_id: "$SUBNET_ID"

validator_set:
  min_validators: 1
  threshold_num: 1
  threshold_denom: 1
  validators:
    - id: "validator-e2e-1"
      endpoint: "127.0.0.1:$VALIDATOR_1_PORT"

nats:
  url: "nats://127.0.0.1:$NATS_PORT"

timeouts:
  checkpoint_interval: 10s
EOF

    # Only start validator 1
    for i in 1; do
        local port=$((VALIDATOR_1_PORT + i - 1))
        local validator_id="validator-e2e-$i"
        local storage_path="$LOG_DIR/validator-$i-data"
        local key="${keys[$((i-1))]}"
        local registry_grpc_port=$((8090 + i))  # 8091, 8092, 8093, 8094
        local registry_http_port=$((8100 + i))  # 8101, 8102, 8103, 8104

        rm -rf "$storage_path"
        mkdir -p "$storage_path"

        # Create individual validator config - SINGLE VALIDATOR
        local val_config="$LOG_DIR/validator-$i-config.yaml"
        cat > "$val_config" <<EOF
subnet_id: "$SUBNET_ID"

identity:
  validator_id: "$validator_id"
  subnet_id: "$SUBNET_ID"

network:
  validator_grpc_port: ":$port"
  nats_url: "nats://127.0.0.1:$NATS_PORT"

storage:
  leveldb_path: "$storage_path"

validator_set:
  min_validators: 1
  threshold_num: 1
  threshold_denom: 1
  validators:
    - id: "validator-e2e-1"
      endpoint: "127.0.0.1:$VALIDATOR_1_PORT"

timeouts:
  checkpoint_interval: 10s
  propose_timeout: 5s
  collect_timeout: 10s
  finalize_timeout: 2s
EOF

        print_step "Starting Validator $i on port $port (registry: $registry_grpc_port/$registry_http_port)..."

        "$PROJECT_ROOT/bin/validator" \
            --id "$validator_id" \
            --key "$key" \
            --grpc "$port" \
            --nats "nats://127.0.0.1:$NATS_PORT" \
            --storage "$storage_path" \
            --validators 1 \
            --threshold-num 1 \
            --threshold-denom 1 \
            --validator-pubkeys "$validator_pubkeys" \
            --registry-grpc ":$registry_grpc_port" \
            --registry-http ":$registry_http_port" \
            --subnet-id "$SUBNET_ID" \
            --rootlayer-endpoint "$ROOTLAYER_GRPC" \
            --enable-rootlayer \
            --enable-chain-submit="$ENABLE_CHAIN_SUBMIT" \
            --chain-rpc-url="$CHAIN_RPC_URL" \
            --chain-network="$CHAIN_NETWORK" \
            --intent-manager-addr="$INTENT_MANAGER_ADDR" \
            > "$LOG_DIR/validator-$i.log" 2>&1 &

        echo $! >> "$PIDS_FILE"

        wait_for_port $port "Validator $i"
    done

    print_success "Validator started"
}

# Start Test Agent
start_test_agent() {
    print_header "Starting Test Agent"

    "$PROJECT_ROOT/scripts/test-agent/test-agent" \
        --agent-id "$TEST_AGENT_ID" \
        --matcher "localhost:$MATCHER_PORT" \
        --validator "localhost:$VALIDATOR_1_PORT" \
        --subnet-id "$SUBNET_ID" \
        > "$LOG_DIR/test-agent.log" 2>&1 &

    echo $! >> "$PIDS_FILE"

    sleep 2
    print_success "Test agent started"
}

# Monitor logs for specific events
monitor_logs() {
    local log_file=$1
    local pattern=$2
    local timeout=${3:-10}
    local count=0

    while [ $count -lt $timeout ]; do
        if grep -q "$pattern" "$log_file" 2>/dev/null; then
            return 0
        fi
        sleep 1
        count=$((count + 1))
    done
    return 1
}

# Submit test Intent to RootLayer using submit-intent-signed (dual submission)
submit_test_intent() {
    print_header "Submitting Test Intent via submit-intent-signed (Dual Submission)"

    # Use submit-intent-signed for proper dual submission (blockchain + RootLayer)
    print_step "Using submit-intent-signed for dual submission..."

    # Run submit-intent-signed with proper env vars
    local output=$(PIN_BASE_SEPOLIA_INTENT_MANAGER="$INTENT_MANAGER_ADDR" \
        RPC_URL="$CHAIN_RPC_URL" \
        PRIVATE_KEY="$MATCHER_PRIVATE_KEY" \
        PIN_NETWORK="$CHAIN_NETWORK" \
        ROOTLAYER_HTTP="$ROOTLAYER_HTTP/api/v1" \
        SUBNET_ID="$SUBNET_ID" \
        INTENT_TYPE="e2e-test" \
        PARAMS_JSON='{"task":"E2E flow test with dual submission","timestamp":"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}' \
        AMOUNT_WEI="100000000000000" \
        "$PROJECT_ROOT/scripts/submit-intent-signed" 2>&1)

    local exit_code=$?

    if [ $exit_code -eq 0 ]; then
        print_success "Intent submitted successfully via dual submission!"

        # Extract Intent ID from output
        local intent_id=$(echo "$output" | grep "Intent ID:" | tail -1 | awk '{print $NF}')
        print_info "Intent ID: $intent_id"

        # Extract blockchain TX hash
        local tx_hash=$(echo "$output" | grep "Blockchain TX:" | tail -1 | awk '{print $NF}')
        if [ -n "$tx_hash" ]; then
            print_info "Blockchain TX: $tx_hash"
        fi

        # Show signature info
        if echo "$output" | grep -q "Local signature verification passed"; then
            print_success "EIP-191 signature verified locally"
        fi
    else
        print_error "Intent submission failed"
        echo "$output" | tail -20
    fi

    sleep 2
}

# Run E2E Test
run_e2e_test() {
    print_header "Running E2E Test Flow"

    # Step 0: Submit test Intent to RootLayer
    submit_test_intent

    # Step 1: Check RootLayer connection and intent pulling
    print_step "Step 1: Waiting for Matcher to pull intents from RootLayer..."
    sleep 5

    print_info "Matcher is connected to RootLayer: $ROOTLAYER_GRPC"
    print_info "Intents will be pulled from real RootLayer"

    # Step 2: Check Matcher received intents
    print_step "Step 2: Verifying Matcher received intents..."
    if monitor_logs "$LOG_DIR/matcher.log" "Added intent" 15; then
        print_success "Matcher received and processed intent"
    else
        print_warning "Matcher may not have received intent yet (check logs)"
    fi

    # Step 3: Check Agent submitted bid
    print_step "Step 3: Checking for agent bids..."
    if monitor_logs "$LOG_DIR/test-agent.log" "Bid accepted" 20; then
        print_success "Agent successfully submitted bid"
    else
        print_warning "Agent bid not confirmed (may still be in bidding window)"
    fi

    # Step 4: Check for Assignment
    print_step "Step 4: Waiting for assignment (bidding window closes)..."
    sleep 12  # Wait for bidding window (10s) + buffer

    if monitor_logs "$LOG_DIR/matcher.log" "Closing bidding window" 5; then
        print_success "Bidding window closed"
    fi

    if monitor_logs "$LOG_DIR/test-agent.log" "Received assignment" 5; then
        print_success "Agent received assignment"
    else
        print_warning "No assignment received (check if there were competing bids)"
    fi

    # Step 5: Check for ExecutionReport
    print_step "Step 5: Checking for ExecutionReport submission..."
    if monitor_logs "$LOG_DIR/test-agent.log" "ExecutionReport submitted" 10; then
        print_success "Agent submitted ExecutionReport to Validator"
    else
        print_warning "ExecutionReport not confirmed yet"
    fi

    # Step 6: Check Validator processed report
    print_step "Step 6: Verifying Validator processed report..."
    if monitor_logs "$LOG_DIR/validator-1.log" "Processed execution report" 5; then
        print_success "Validator processed ExecutionReport and returned Receipt"
    else
        print_warning "Validator processing not confirmed"
    fi

    # Step 7: Wait for ExecutionReport to be broadcast to all validators
    print_step "Step 7: Waiting for ExecutionReport to propagate to all validators..."
    sleep 3  # Brief wait for NATS broadcast

    if grep -q "📡 Broadcasted execution report" "$LOG_DIR/validator-1.log" 2>/dev/null; then
        print_success "ExecutionReport broadcasted to all validators"
    fi

    # Step 8: Wait for first checkpoint to complete (should skip ValidationBundle)
    print_step "Step 8: Waiting for first checkpoint cycle to complete..."
    sleep 12  # Wait for checkpoint interval (10s) + buffer

    # Verify first checkpoint was created
    if monitor_logs "$LOG_DIR/validator-1.log" "Proposed checkpoint" 3; then
        print_success "First checkpoint proposed"
    fi

    # Step 9: Wait for NEXT checkpoint (should include ExecutionReport)
    print_step "Step 9: Waiting for next checkpoint with ExecutionReport (20s+)..."
    print_info "This checkpoint should include the ExecutionReport and submit ValidationBundle"
    sleep 20  # Wait for next checkpoint interval + signature collection

    # Check all validators for ValidationBundle submission
    print_step "Step 9a: Checking for ValidationBundle submission..."
    local bundle_submitted=0

    # Check for successful submission
    for i in 1 2 3 4; do
        if grep -q "✅ Successfully submitted validation bundle to RootLayer" "$LOG_DIR/validator-$i.log" 2>/dev/null; then
            bundle_submitted=1
            print_success "Validator $i successfully submitted ValidationBundle to RootLayer!"
            grep "✅ Successfully submitted" "$LOG_DIR/validator-$i.log" | tail -1

            # Show the bundle details
            print_info "ValidationBundle details:"
            grep -A2 "Building ValidationBundle" "$LOG_DIR/validator-$i.log" | tail -3
            break
        fi
    done

    if [ $bundle_submitted -eq 0 ]; then
        # Check which epochs had reports
        print_info "Checking checkpoint history for ExecutionReports..."

        for i in 1 2 3 4; do
            local epochs_with_reports=$(grep "pending_reports_count=1" "$LOG_DIR/validator-$i.log" 2>/dev/null | wc -l || echo "0")
            if [ "$epochs_with_reports" -gt 0 ]; then
                print_info "Validator $i received ExecutionReport: $(grep 'pending_reports_count=1' "$LOG_DIR/validator-$i.log" | head -1 | grep -o 'epoch=[0-9]*')"
            fi
        done

        # Check if bundle was skipped due to no reports
        local skip_count=$(grep -c "ValidationBundle construction skipped" "$LOG_DIR"/*.log 2>/dev/null || echo "0")
        if [ "$skip_count" -gt 0 ]; then
            print_warning "ValidationBundle skipped $skip_count times (no ExecutionReports at checkpoint time)"
        fi

        # Check for the leader at the time ExecutionReport arrived
        print_info "Checking which epoch had pending reports..."
        local report_epoch=$(grep -h "pending_reports_count=1" "$LOG_DIR"/validator-*.log | head -1 | grep -o 'Building ValidationBundle.*epoch=[0-9]*' | grep -o 'epoch=[0-9]*' | cut -d= -f2)

        if [ -n "$report_epoch" ]; then
            local leader_num=$(( (report_epoch % 4) + 1 ))
            print_info "Epoch $report_epoch should have reports - Leader is validator-$leader_num"
            print_info "Check $LOG_DIR/validator-$leader_num.log for ValidationBundle submission"

            # Show last few ValidationBundle related logs
            print_info "Recent ValidationBundle logs:"
            grep -h "ValidationBundle\|validation bundle" "$LOG_DIR/validator-$leader_num.log" | tail -5
        else
            print_warning "No epoch found with pending_reports_count=1"
            print_info "ExecutionReport may have arrived too late - test needs to wait longer"
        fi
    fi
}

# Display test results
display_results() {
    print_header "Test Results Summary"

    echo -e "\n${CYAN}Service Status:${NC}"
    echo "  RootLayer:      $ROOTLAYER_GRPC (external)"
    echo "  Matcher:        localhost:$MATCHER_PORT"
    echo "  Validator 1:    localhost:$VALIDATOR_1_PORT"

    echo -e "\n${CYAN}Log Files:${NC}"
    echo "  Matcher:        $LOG_DIR/matcher.log"
    echo "  Validator 1:    $LOG_DIR/validator-1.log"
    echo "  Test Agent:     $LOG_DIR/test-agent.log"

    echo -e "\n${CYAN}Quick Stats:${NC}"

    # Check matcher log for intents
    local intent_count=$(grep -c "Added intent" "$LOG_DIR/matcher.log" 2>/dev/null || echo "0")
    echo "  Intents Received: $intent_count"

    # Count reports
    local report_count=$(grep -c "ExecutionReport submitted" "$LOG_DIR/test-agent.log" 2>/dev/null || echo "0")
    echo "  Reports:        $report_count"

    # Count checkpoints
    local checkpoint_count=$(grep -c "Proposed checkpoint" "$LOG_DIR/validator-1.log" 2>/dev/null || echo "0")
    echo "  Checkpoints:    $checkpoint_count"

    echo -e "\n${CYAN}View Logs:${NC}"
    echo "  tail -f $LOG_DIR/matcher.log"
    echo "  tail -f $LOG_DIR/validator-1.log"
    echo "  tail -f $LOG_DIR/test-agent.log"

    echo -e "\n${CYAN}Test Complete!${NC}"
}

# Interactive mode to keep services running
interactive_mode() {
    print_header "Interactive Mode"
    print_info "Services are running. Press Ctrl+C to stop all services and exit."
    print_info "Or type commands:"
    echo ""
    echo "  logs <service>  - Show logs (matcher, validator-1/2/3/4, agent)"
    echo "  stats           - Show quick statistics"
    echo "  quit            - Stop services and exit"
    echo ""

    while true; do
        read -p "> " cmd arg

        case "$cmd" in
            logs)
                case "$arg" in
                    matcher)
                        tail -20 "$LOG_DIR/matcher.log"
                        ;;
                    validator-1|validator-2|validator-3|validator-4)
                        tail -20 "$LOG_DIR/$arg.log"
                        ;;
                    agent)
                        tail -20 "$LOG_DIR/test-agent.log"
                        ;;
                    *)
                        echo "Unknown service. Try: matcher, validator-1/2/3/4, agent"
                        ;;
                esac
                ;;
            stats)
                echo "Quick Statistics:"
                echo "  Intents Received: $(grep -c 'Added intent' $LOG_DIR/matcher.log 2>/dev/null || echo 0)"
                echo "  Reports Submitted: $(grep -c 'ExecutionReport submitted' $LOG_DIR/test-agent.log 2>/dev/null || echo 0)"
                echo "  Checkpoints: $(grep -c 'Proposed checkpoint' $LOG_DIR/validator-1.log 2>/dev/null || echo 0)"
                ;;
            quit|exit)
                break
                ;;
            *)
                echo "Unknown command. Try: logs, status, intents, quit"
                ;;
        esac
    done
}

# Main execution
main() {
    print_header "Subnet E2E Test - Complete Flow"
    print_info "This script will start all services and run the complete E2E test"

    # Create log directory
    mkdir -p "$LOG_DIR"
    rm -f "$PIDS_FILE"

    # Check prerequisites
    check_prerequisites

    # Build test agent
    build_test_agent

    # Check RootLayer connectivity
    check_rootlayer
    sleep 1

    # Start services in order
    start_matcher
    sleep 2

    start_validators
    sleep 3

    start_test_agent
    sleep 2

    # Run E2E test
    run_e2e_test

    # Display results
    display_results

    # Check if no-interactive mode
    if [ "$NO_INTERACTIVE" = "1" ]; then
        print_info "Non-interactive mode. Waiting 30 more seconds for ValidationBundle submission..."
        sleep 30

        # Check again for ValidationBundle submission
        print_step "Final check for ValidationBundle..."
        local bundle_found=0
        for i in 1 2 3 4; do
            if grep -q "✅ Successfully submitted validation bundle to RootLayer" "$LOG_DIR/validator-$i.log" 2>/dev/null; then
                bundle_found=1
                print_success "✅ Validator $i submitted ValidationBundle!"
                grep "Successfully submitted validation bundle" "$LOG_DIR/validator-$i.log" | tail -1
                break
            fi
        done

        if [ $bundle_found -eq 0 ]; then
            print_warning "No ValidationBundle submission found after extended wait"
            print_info "Checking for finalization logs..."
            grep -h "Finalized checkpoint\|finalizeCheckpoint" "$LOG_DIR"/validator-*.log | tail -10
        fi

        print_info "Services will stop now."
        cleanup
        exit 0
    fi

    # Interactive mode
    interactive_mode
}

# Run main
main "$@"
