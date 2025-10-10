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

# Test configuration
SUBNET_ID="0x1111111111111111111111111111111111111111111111111111111111111111"
TEST_AGENT_ID="e2e-test-agent-001"

# Cleanup flag
CLEANUP_ON_EXIT=1

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
      private_key: "EXAMPLE_PRIVATE_KEY_DO_NOT_USE_1234567890ABCDEF1234567890ABCDEF"
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

    # Test private keys and public keys (DO NOT use in production!)
    local keys=("0000000000000000000000000000000000000000000000000000000000000001" \
                "0000000000000000000000000000000000000000000000000000000000000002" \
                "0000000000000000000000000000000000000000000000000000000000000003" \
                "0000000000000000000000000000000000000000000000000000000000000004")

    # Corresponding public keys (derived from private keys using secp256k1)
    local pubkeys=("0479be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8" \
                   "04c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee51ae168fea63dc339a3c58419466ceaeef7f632653266d0e1236431a950cfe52a" \
                   "04f9308a019258c31049344f85f89d5229b531c845836f99b08601f113bce036f9388f7b0f632de8140fe337e62a37f3566500a99934c2231b6cb9fd7584b8e672" \
                   "04e493dbf1c10d80f3581e4904930b1404cc6c13900ee0758474fa94abe8c4cd1351ed993ea0d455b75642e2098ea51448d967ae33bfbdfe40cfe97bdc47739922")

    # Build the validator pubkeys parameter (id:pubkey pairs)
    local validator_pubkeys=""
    for i in 1 2 3 4; do
        local validator_id="validator-e2e-$i"
        local pubkey="${pubkeys[$((i-1))]}"
        if [ -z "$validator_pubkeys" ]; then
            validator_pubkeys="${validator_id}:${pubkey}"
        else
            validator_pubkeys="${validator_pubkeys},${validator_id}:${pubkey}"
        fi
    done

    # Create shared validator set config
    local validator_config="$LOG_DIR/validator-set-config.yaml"
    cat > "$validator_config" <<EOF
subnet_id: "$SUBNET_ID"

validator_set:
  min_validators: 4
  threshold_num: 3
  threshold_denom: 4
  validators:
    - id: "validator-e2e-1"
      endpoint: "127.0.0.1:$VALIDATOR_1_PORT"
    - id: "validator-e2e-2"
      endpoint: "127.0.0.1:$VALIDATOR_2_PORT"
    - id: "validator-e2e-3"
      endpoint: "127.0.0.1:$VALIDATOR_3_PORT"
    - id: "validator-e2e-4"
      endpoint: "127.0.0.1:$VALIDATOR_4_PORT"

nats:
  url: "nats://127.0.0.1:$NATS_PORT"

timeouts:
  checkpoint_interval: 10s
EOF

    for i in 1 2 3 4; do
        local port=$((VALIDATOR_1_PORT + i - 1))
        local validator_id="validator-e2e-$i"
        local storage_path="$LOG_DIR/validator-$i-data"
        local key="${keys[$((i-1))]}"
        local registry_grpc_port=$((8090 + i))  # 8091, 8092, 8093, 8094
        local registry_http_port=$((8100 + i))  # 8101, 8102, 8103, 8104

        rm -rf "$storage_path"
        mkdir -p "$storage_path"

        # Create individual validator config
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
  min_validators: 4
  threshold_num: 3
  threshold_denom: 4
  validators:
    - id: "validator-e2e-1"
      endpoint: "127.0.0.1:$VALIDATOR_1_PORT"
    - id: "validator-e2e-2"
      endpoint: "127.0.0.1:$VALIDATOR_2_PORT"
    - id: "validator-e2e-3"
      endpoint: "127.0.0.1:$VALIDATOR_3_PORT"
    - id: "validator-e2e-4"
      endpoint: "127.0.0.1:$VALIDATOR_4_PORT"

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
            --validators 4 \
            --threshold-num 3 \
            --threshold-denom 4 \
            --validator-pubkeys "$validator_pubkeys" \
            --registry-grpc ":$registry_grpc_port" \
            --registry-http ":$registry_http_port" \
            --subnet-id "$SUBNET_ID" \
            --rootlayer-endpoint "$ROOTLAYER_GRPC" \
            --enable-rootlayer \
            > "$LOG_DIR/validator-$i.log" 2>&1 &

        echo $! >> "$PIDS_FILE"

        wait_for_port $port "Validator $i"
    done

    print_success "All validators started"
}

# Start Test Agent
start_test_agent() {
    print_header "Starting Test Agent"

    "$PROJECT_ROOT/scripts/test-agent/test-agent" \
        --agent-id "$TEST_AGENT_ID" \
        --matcher "localhost:$MATCHER_PORT" \
        --validator "localhost:$VALIDATOR_1_PORT" \
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

# Submit test Intent to RootLayer
submit_test_intent() {
    print_header "Submitting Test Intent to RootLayer"

    # Generate intent ID (32-byte hex string = 64 hex chars)
    # Use timestamp + random bytes to ensure uniqueness
    local timestamp=$(date +%s)
    local random=$(openssl rand -hex 28)  # 28 bytes = 56 hex chars
    local intent_id="0x$(printf '%08x' $timestamp)${random}"

    # Current timestamp + 1 hour deadline
    local now=$(date +%s)
    local deadline=$((now + 3600))

    # Encode intent data as base64 (required by RootLayer API)
    local intent_raw=$(echo -n '{"task":"E2E flow test","test_id":"e2e-'$(date +%s)'","timestamp":"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}' | base64)
    local metadata=$(echo -n "E2E test metadata" | base64)

    # Test requester address (DO NOT use in production!)
    local requester="0x9290085Cd66bD1A3C7D277EF7DBcbD2e98457b6f"

    # Create payload matching RootLayer API spec
    local intent_payload=$(cat <<EOF
{
  "intentId": "$intent_id",
  "subnetId": "$SUBNET_ID",
  "requester": "$requester",
  "settleChain": "base_sepolia",
  "intentType": "e2e-test",
  "params": {
    "intentRaw": "$intent_raw",
    "metadata": "$metadata"
  },
  "tipsToken": "0x0000000000000000000000000000000000000000",
  "tips": "100",
  "deadline": "$deadline",
  "signature": "0x00",
  "budgetToken": "0x0000000000000000000000000000000000000000",
  "budget": "1000"
}
EOF
)

    print_step "Submitting Intent to $ROOTLAYER_HTTP/api/v1/intents/submit..."
    print_info "Intent ID: $intent_id"

    # Try HTTP submission
    local response=$(curl -s -w "\n%{http_code}" -X POST "$ROOTLAYER_HTTP/api/v1/intents/submit" \
        -H "Content-Type: application/json" \
        -d "$intent_payload" \
        --max-time 10 2>&1)

    local http_code=$(echo "$response" | tail -1)
    local body=$(echo "$response" | sed '$d')

    if [ "$http_code" = "200" ] || [ "$http_code" = "201" ]; then
        print_success "Intent submitted successfully (HTTP $http_code)"
        echo "$body" | head -5
    elif [ "$http_code" = "400" ]; then
        print_error "Intent submission failed with validation error (HTTP 400)"
        echo "$body" | head -10
    elif [ "$http_code" = "000" ]; then
        print_warning "Intent submission timed out (connection issue)"
        print_info "This may be due to network or authentication requirements"
        print_info "Matcher will pull Intents via gRPC polling instead"
    else
        print_warning "Intent submission may have failed (HTTP $http_code)"
        echo "$body" | head -5
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

    # Step 7: Check for Checkpoint creation
    print_step "Step 7: Waiting for Checkpoint creation..."
    sleep 10  # Wait for checkpoint interval

    if monitor_logs "$LOG_DIR/validator-1.log" "Proposed checkpoint" 5; then
        print_success "Checkpoint proposed"
    else
        print_warning "Checkpoint not created yet (timing dependent)"
    fi

    # Step 8: Check for signature collection
    print_step "Step 8: Checking signature collection..."
    if monitor_logs "$LOG_DIR/validator-1.log" "signature" 5; then
        print_success "Signature collection in progress"
    else
        print_warning "Signature collection status unknown"
    fi

    # Step 9: Wait for second checkpoint with real ExecutionReport
    print_step "Step 9: Waiting for second checkpoint with ExecutionReport (15s)..."
    sleep 15  # Wait for new leader to trigger checkpoint with pending reports

    # Check all validators for ValidationBundle submission
    print_step "Step 9a: Checking for ValidationBundle submission..."
    local bundle_submitted=0
    for i in 1 2 3 4; do
        if grep -q "Successfully submitted validation bundle" "$LOG_DIR/validator-$i.log" 2>/dev/null; then
            bundle_submitted=1
            print_success "Validator $i successfully submitted ValidationBundle to RootLayer"
            grep "Successfully submitted" "$LOG_DIR/validator-$i.log" | tail -1
            break
        fi
    done

    if [ $bundle_submitted -eq 0 ]; then
        # Check if bundle was skipped due to no reports (expected for first checkpoint)
        if grep -q "Failed to build validation bundle" "$LOG_DIR/validator-1.log" 2>/dev/null; then
            print_info "First checkpoint correctly skipped ValidationBundle (no ExecutionReports)"
        fi

        # Check if second checkpoint was created
        local checkpoint_count=$(grep -c "Finalized checkpoint" "$LOG_DIR/validator-2.log" 2>/dev/null || echo "0")
        if [ "$checkpoint_count" -gt 0 ]; then
            print_warning "Second checkpoint created but ValidationBundle submission not detected"
            print_info "Check validator-2.log (new leader) for ValidationBundle logs"
        else
            print_warning "Second checkpoint not yet created - test may need more time"
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
    echo "  Validator 2:    localhost:$VALIDATOR_2_PORT"
    echo "  Validator 3:    localhost:$VALIDATOR_3_PORT"
    echo "  Validator 4:    localhost:$VALIDATOR_4_PORT"

    echo -e "\n${CYAN}Log Files:${NC}"
    echo "  Matcher:        $LOG_DIR/matcher.log"
    echo "  Validator 1:    $LOG_DIR/validator-1.log"
    echo "  Validator 2:    $LOG_DIR/validator-2.log"
    echo "  Validator 3:    $LOG_DIR/validator-3.log"
    echo "  Validator 4:    $LOG_DIR/validator-4.log"
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
        print_info "Non-interactive mode. Services will stop now."
        cleanup
        exit 0
    fi

    # Interactive mode
    interactive_mode
}

# Run main
main "$@"
