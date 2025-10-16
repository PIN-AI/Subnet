#!/bin/bash
# Batch Operations E2E Test Script
# Tests: Batch Intent Submission → Batch Bids → Batch ExecutionReports → ValidationBundles
#
# Usage: ./batch-e2e-test.sh [OPTIONS]
#
# Options:
#   --intent-count <n>         Number of intents to submit (default: 5)
#   --batch-size <n>           Batch size for operations (default: 5)
#   --batch-wait <duration>    Batch wait time (default: 10s)
#   --rootlayer-grpc <addr>    RootLayer gRPC endpoint (default: 3.17.208.238:9001)
#   --rootlayer-http <url>     RootLayer HTTP endpoint (default: http://3.17.208.238:8081)
#   --no-interactive           Run test and exit without interactive mode
#
# Environment Variables:
#   ROOTLAYER_GRPC            Alternative to --rootlayer-grpc
#   ROOTLAYER_HTTP            Alternative to --rootlayer-http
#   TEST_PRIVATE_KEY          Private key for testing (REQUIRED)

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LOG_DIR="$PROJECT_ROOT/batch-e2e-logs"
PIDS_FILE="$LOG_DIR/pids.txt"

# Ports
NATS_PORT=4222
MATCHER_PORT=8090
VALIDATOR_1_PORT=9200

# RootLayer configuration
ROOTLAYER_GRPC="${ROOTLAYER_GRPC:-3.17.208.238:9001}"
ROOTLAYER_HTTP="${ROOTLAYER_HTTP:-http://3.17.208.238:8081}"

# Test configuration
SUBNET_ID="${SUBNET_ID:-0x0000000000000000000000000000000000000000000000000000000000000002}"
INTENT_COUNT=5
BATCH_SIZE=5
BATCH_WAIT="10s"
NO_INTERACTIVE=0

# Blockchain configuration
ENABLE_CHAIN_SUBMIT="${ENABLE_CHAIN_SUBMIT:-false}"
CHAIN_RPC_URL="${CHAIN_RPC_URL:-https://sepolia.base.org}"
CHAIN_NETWORK="${CHAIN_NETWORK:-base_sepolia}"
INTENT_MANAGER_ADDR="${INTENT_MANAGER_ADDR:-0xD04d23775D3B8e028e6104E31eb0F6c07206EB46}"

# Private key for testing
TEST_PRIVATE_KEY="${TEST_PRIVATE_KEY:-}"

# Cleanup flag
CLEANUP_ON_EXIT=1

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --intent-count)
            INTENT_COUNT="$2"
            shift 2
            ;;
        --batch-size)
            BATCH_SIZE="$2"
            shift 2
            ;;
        --batch-wait)
            BATCH_WAIT="$2"
            shift 2
            ;;
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
            echo "  --intent-count <n>         Number of intents to submit (default: 5)"
            echo "  --batch-size <n>           Batch size for operations (default: 5)"
            echo "  --batch-wait <duration>    Batch wait time (default: 10s)"
            echo "  --rootlayer-grpc <addr>    RootLayer gRPC endpoint"
            echo "  --rootlayer-http <url>     RootLayer HTTP endpoint"
            echo "  --no-interactive           Run test and exit"
            echo "  --help, -h                 Show this help message"
            echo ""
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
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

print_success() {
    echo -e "${BLUE}[$(date '+%H:%M:%S')]${NC} ${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${BLUE}[$(date '+%H:%M:%S')]${NC} ${RED}✗${NC} $1"
}

print_warning() {
    echo -e "${BLUE}[$(date '+%H:%M:%S')]${NC} ${YELLOW}⚠${NC} $1"
}

print_batch() {
    echo -e "${BLUE}[$(date '+%H:%M:%S')]${NC} ${MAGENTA}📦${NC} $1"
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
        pkill -f "batch-test-agent" 2>/dev/null || true

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

    # Check binaries
    if [ ! -f "$PROJECT_ROOT/bin/matcher" ]; then
        print_error "matcher binary not found. Run 'make build' first."
        exit 1
    fi

    if [ ! -f "$PROJECT_ROOT/bin/validator" ]; then
        print_error "validator binary not found. Run 'make build' first."
        exit 1
    fi

    # Check NATS
    if ! check_port $NATS_PORT; then
        print_warning "NATS is not running on port $NATS_PORT"
        print_info "Attempting to start NATS with Docker..."

        if command -v docker &> /dev/null; then
            docker run -d --name nats-batch-e2e -p 4222:4222 nats:latest &>/dev/null || true
            sleep 2

            if check_port $NATS_PORT; then
                print_success "NATS started successfully"
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

    # Check private key
    if [ -z "$TEST_PRIVATE_KEY" ]; then
        print_error "TEST_PRIVATE_KEY environment variable not set"
        print_info "Set it with: export TEST_PRIVATE_KEY=\"your_test_key\""
        exit 1
    fi

    print_success "All prerequisites checked"
}

# Build batch test binaries
build_batch_binaries() {
    print_header "Building Batch Test Binaries"

    cd "$PROJECT_ROOT"

    # Build batch-submit-intents
    print_step "Building batch-submit-intents..."
    if go build -o bin/batch-submit-intents ./cmd/batch-submit-intents; then
        print_success "batch-submit-intents built successfully"
    else
        print_error "Failed to build batch-submit-intents"
        exit 1
    fi

    # Build batch-test-agent
    print_step "Building batch-test-agent..."
    if go build -o bin/batch-test-agent ./cmd/batch-test-agent; then
        print_success "batch-test-agent built successfully"
    else
        print_error "Failed to build batch-test-agent"
        exit 1
    fi
}

# Start Matcher
start_matcher() {
    print_header "Starting Matcher"

    local matcher_config="$LOG_DIR/matcher-config.yaml"
    cat > "$matcher_config" <<EOF
subnet_id: "$SUBNET_ID"

identity:
  matcher_id: "matcher-batch-e2e"
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

private_key: "$TEST_PRIVATE_KEY"

enable_chain_submit: $ENABLE_CHAIN_SUBMIT
chain_rpc_url: "$CHAIN_RPC_URL"
chain_network: "$CHAIN_NETWORK"
intent_manager_addr: "$INTENT_MANAGER_ADDR"
EOF

    "$PROJECT_ROOT/bin/matcher" \
        --config "$matcher_config" \
        > "$LOG_DIR/matcher.log" 2>&1 &

    echo $! >> "$PIDS_FILE"
    wait_for_port $MATCHER_PORT "Matcher"
}

# Start Validator
start_validators() {
    print_header "Starting Validators"

    local validator_id="validator-batch-e2e-1"
    local storage_path="$LOG_DIR/validator-data"
    rm -rf "$storage_path"
    mkdir -p "$storage_path"

    "$PROJECT_ROOT/bin/validator" \
        --id "$validator_id" \
        --key "$TEST_PRIVATE_KEY" \
        --grpc "$VALIDATOR_1_PORT" \
        --nats "nats://127.0.0.1:$NATS_PORT" \
        --storage "$storage_path" \
        --validators 1 \
        --threshold-num 1 \
        --threshold-denom 1 \
        --subnet-id "$SUBNET_ID" \
        --rootlayer-endpoint "$ROOTLAYER_GRPC" \
        --enable-rootlayer \
        > "$LOG_DIR/validator.log" 2>&1 &

    echo $! >> "$PIDS_FILE"
    wait_for_port $VALIDATOR_1_PORT "Validator"
}

# Submit batch intents
submit_batch_intents() {
    print_header "Submitting Batch Intents"

    print_batch "Submitting $INTENT_COUNT intents to RootLayer..."

    RPC_URL="$CHAIN_RPC_URL" \
    PRIVATE_KEY="$TEST_PRIVATE_KEY" \
    SUBNET_ID="$SUBNET_ID" \
    "$PROJECT_ROOT/bin/batch-submit-intents" \
        --count $INTENT_COUNT \
        --type "batch-test-intent" \
        --params '{"task":"Batch E2E test","test":"batch-operations"}' \
        --network "$CHAIN_NETWORK" \
        2>&1 | tee "$LOG_DIR/batch-submit.log"

    if [ ${PIPESTATUS[0]} -eq 0 ]; then
        print_success "Batch intents submitted successfully"
    else
        print_error "Batch intent submission failed"
        exit 1
    fi
}

# Start batch test agent
start_batch_agent() {
    print_header "Starting Batch Test Agent"

    "$PROJECT_ROOT/bin/batch-test-agent" \
        --matcher "localhost:$MATCHER_PORT" \
        --subnet-id "$SUBNET_ID" \
        --batch true \
        --batch-size $BATCH_SIZE \
        --batch-wait $BATCH_WAIT \
        > "$LOG_DIR/batch-agent.log" 2>&1 &

    echo $! >> "$PIDS_FILE"
    sleep 3
    print_success "Batch test agent started"
}

# Monitor batch operations
monitor_batch_operations() {
    print_header "Monitoring Batch Operations"

    # Step 1: Wait for intents to be pulled
    print_step "Step 1: Waiting for Matcher to pull intents..."
    sleep 5

    local intent_count=$(grep -c "Added intent" "$LOG_DIR/matcher.log" 2>/dev/null || echo "0")
    print_success "Matcher received $intent_count intents"

    # Step 2: Wait for batch bids
    print_step "Step 2: Waiting for batch bid submissions..."
    sleep 15

    if grep -q "Batch bid submission" "$LOG_DIR/batch-agent.log" 2>/dev/null; then
        local batch_count=$(grep -c "Batch bid submission" "$LOG_DIR/batch-agent.log" || echo "0")
        print_batch "Found $batch_count batch bid submissions"
    fi

    # Step 3: Wait for assignments
    print_step "Step 3: Waiting for task assignments..."
    sleep 12

    local assignment_count=$(grep -c "Executing task" "$LOG_DIR/batch-agent.log" 2>/dev/null || echo "0")
    print_success "Agent received $assignment_count task assignments"

    # Step 4: Wait for batch execution reports
    print_step "Step 4: Waiting for batch execution report submissions..."
    sleep 15

    if grep -q "Batch execution report" "$LOG_DIR/batch-agent.log" 2>/dev/null; then
        local batch_report_count=$(grep -c "Batch execution report" "$LOG_DIR/batch-agent.log" || echo "0")
        print_batch "Found $batch_report_count batch execution report submissions"
    fi

    # Step 5: Check validator processing
    print_step "Step 5: Checking validator processing..."
    sleep 5

    local report_count=$(grep -c "Processed execution report" "$LOG_DIR/validator.log" 2>/dev/null || echo "0")
    print_success "Validator processed $report_count execution reports"
}

# Display test results
display_results() {
    print_header "Batch E2E Test Results"

    echo -e "\n${CYAN}Test Configuration:${NC}"
    echo "  Intent Count:   $INTENT_COUNT"
    echo "  Batch Size:     $BATCH_SIZE"
    echo "  Batch Wait:     $BATCH_WAIT"

    echo -e "\n${CYAN}Service Status:${NC}"
    echo "  Matcher:        localhost:$MATCHER_PORT"
    echo "  Validator:      localhost:$VALIDATOR_1_PORT"

    echo -e "\n${CYAN}Statistics:${NC}"
    local intents_received=$(grep -c "Added intent" "$LOG_DIR/matcher.log" 2>/dev/null || echo "0")
    local tasks_executed=$(grep -c "Executing task" "$LOG_DIR/batch-agent.log" 2>/dev/null || echo "0")
    local reports_processed=$(grep -c "Processed execution report" "$LOG_DIR/validator.log" 2>/dev/null || echo "0")
    local batch_bids=$(grep -c "Batch bid submission" "$LOG_DIR/batch-agent.log" 2>/dev/null || echo "0")
    local batch_reports=$(grep -c "Batch execution report" "$LOG_DIR/batch-agent.log" 2>/dev/null || echo "0")

    echo "  Intents Received:        $intents_received"
    echo "  Tasks Executed:          $tasks_executed"
    echo "  Reports Processed:       $reports_processed"
    echo "  Batch Bid Submissions:   $batch_bids"
    echo "  Batch Report Submissions: $batch_reports"

    echo -e "\n${CYAN}Log Files:${NC}"
    echo "  Matcher:        $LOG_DIR/matcher.log"
    echo "  Validator:      $LOG_DIR/validator.log"
    echo "  Batch Agent:    $LOG_DIR/batch-agent.log"
    echo "  Batch Submit:   $LOG_DIR/batch-submit.log"

    # Determine test result
    if [ "$intents_received" -ge "$INTENT_COUNT" ] && [ "$tasks_executed" -ge "$INTENT_COUNT" ]; then
        echo -e "\n${GREEN}✅ Batch E2E Test PASSED${NC}"
        return 0
    else
        echo -e "\n${RED}❌ Batch E2E Test FAILED${NC}"
        return 1
    fi
}

# Main execution
main() {
    print_header "Batch Operations E2E Test"
    print_info "Testing batch bid and execution report submissions"

    # Create log directory
    mkdir -p "$LOG_DIR"
    rm -f "$PIDS_FILE"

    # Check prerequisites
    check_prerequisites

    # Build binaries
    build_batch_binaries

    # Start services
    start_matcher
    sleep 2

    start_validators
    sleep 3

    # Submit batch intents
    submit_batch_intents
    sleep 2

    # Start batch agent
    start_batch_agent
    sleep 2

    # Monitor operations
    monitor_batch_operations

    # Display results
    display_results
    test_result=$?

    # Cleanup or interactive mode
    if [ "$NO_INTERACTIVE" = "1" ]; then
        print_info "Services will stop now."
        cleanup
        exit $test_result
    else
        print_info "Services are running. Press Ctrl+C to stop."
        read -p "Press Enter to stop services..." dummy
        cleanup
        exit $test_result
    fi
}

# Run main
main "$@"
