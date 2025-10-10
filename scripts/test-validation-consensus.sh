#!/bin/bash
# Validation Bundle Consensus Test
# Tests 4-node consensus on ValidationBundle submission
#
# This test verifies:
# 1. All 4 validators can reach consensus on checkpoint
# 2. Threshold signatures are collected (3/4 threshold)
# 3. Leader successfully builds and submits ValidationBundle to RootLayer
# 4. All validators see the same finalized checkpoint

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LOG_DIR="$PROJECT_ROOT/validation-consensus-test-logs"
PIDS_FILE="$LOG_DIR/pids.txt"

# Ports
NATS_PORT=4222
VALIDATOR_1_PORT=9200
VALIDATOR_2_PORT=9201
VALIDATOR_3_PORT=9202
VALIDATOR_4_PORT=9203

# RootLayer configuration
ROOTLAYER_GRPC="${ROOTLAYER_GRPC:-3.17.208.238:9001}"
ROOTLAYER_HTTP="${ROOTLAYER_HTTP:-http://3.17.208.238:8081}"

# Test configuration
SUBNET_ID="0x1111111111111111111111111111111111111111111111111111111111111111"
CHECKPOINT_INTERVAL="10s"

# Cleanup flag
CLEANUP_ON_EXIT=1

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

print_info() {
    echo -e "${BLUE}[$(date '+%H:%M:%S')]${NC} ${CYAN}ℹ${NC} $1"
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
        pkill -f "bin/validator" 2>/dev/null || true

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

    if [ ! -f "$PROJECT_ROOT/bin/validator" ]; then
        print_error "validator binary not found. Run 'make build' first."
        exit 1
    fi

    # Check if NATS is running
    if ! check_port $NATS_PORT; then
        print_warning "NATS is not running on port $NATS_PORT"
        print_info "Attempting to start NATS with Docker..."

        if command -v docker &> /dev/null; then
            docker run -d --name nats-validation-test -p 4222:4222 nats:latest &>/dev/null || true
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

# Start Validators with 3/4 threshold
start_validators() {
    print_header "Starting 4 Validators (3/4 threshold)"

    # Test private keys (DO NOT use in production!)
    local keys=("0000000000000000000000000000000000000000000000000000000000000001" \
                "0000000000000000000000000000000000000000000000000000000000000002" \
                "0000000000000000000000000000000000000000000000000000000000000003" \
                "0000000000000000000000000000000000000000000000000000000000000004")

    # Corresponding public keys
    local pubkeys=("0479be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8" \
                   "04c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee51ae168fea63dc339a3c58419466ceaeef7f632653266d0e1236431a950cfe52a" \
                   "04f9308a019258c31049344f85f89d5229b531c845836f99b08601f113bce036f9388f7b0f632de8140fe337e62a37f3566500a99934c2231b6cb9fd7584b8e672" \
                   "04e493dbf1c10d80f3581e4904930b1404cc6c13900ee0758474fa94abe8c4cd1351ed993ea0d455b75642e2098ea51448d967ae33bfbdfe40cfe97bdc47739922")

    # Build validator pubkeys parameter
    local validator_pubkeys=""
    for i in 1 2 3 4; do
        local validator_id="validator-consensus-$i"
        local pubkey="${pubkeys[$((i-1))]}"
        if [ -z "$validator_pubkeys" ]; then
            validator_pubkeys="${validator_id}:${pubkey}"
        else
            validator_pubkeys="${validator_pubkeys},${validator_id}:${pubkey}"
        fi
    done

    for i in 1 2 3 4; do
        local port=$((VALIDATOR_1_PORT + i - 1))
        local validator_id="validator-consensus-$i"
        local storage_path="$LOG_DIR/validator-$i-data"
        local key="${keys[$((i-1))]}"
        local registry_grpc_port=$((8090 + i))
        local registry_http_port=$((8100 + i))

        rm -rf "$storage_path"
        mkdir -p "$storage_path"

        print_step "Starting Validator $i on port $port..."

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

    print_success "All 4 validators started"
    sleep 3
}

# Submit test execution report to trigger consensus
submit_test_report() {
    print_header "Submitting Test Execution Report"

    local intent_id="0x$(openssl rand -hex 32)"
    local assignment_id="test-assignment-$(date +%s)"
    local agent_id="test-agent-001"

    print_info "Intent ID: $intent_id"
    print_info "Assignment ID: $assignment_id"

    # Submit to validator 1 via HTTP
    local report_payload=$(cat <<EOF
{
  "report_id": "test-report-$(date +%s)",
  "assignment_id": "$assignment_id",
  "intent_id": "$intent_id",
  "agent_id": "$agent_id",
  "status": "success",
  "result_data": "$(echo -n "Test execution result" | base64)",
  "timestamp": $(date +%s)
}
EOF
)

    print_step "Submitting execution report to Validator 1..."
    curl -s -X POST "http://localhost:$((8100 + 1))/api/v1/execution-report" \
        -H "Content-Type: application/json" \
        -d "$report_payload" | head -5

    print_success "Execution report submitted"
    sleep 2
}

# Monitor consensus progress
monitor_consensus() {
    print_header "Monitoring Consensus Progress"

    local max_wait=60
    local count=0
    local checkpoint_proposed=0
    local signatures_collected=0
    local checkpoint_finalized=0
    local bundle_submitted=0

    print_step "Monitoring validators for consensus (max ${max_wait}s)..."
    echo ""

    while [ $count -lt $max_wait ]; do
        # Check for checkpoint proposal
        if [ $checkpoint_proposed -eq 0 ]; then
            if grep -q "Proposed checkpoint" "$LOG_DIR/validator-1.log" 2>/dev/null || \
               grep -q "Proposed checkpoint" "$LOG_DIR/validator-2.log" 2>/dev/null || \
               grep -q "Proposed checkpoint" "$LOG_DIR/validator-3.log" 2>/dev/null || \
               grep -q "Proposed checkpoint" "$LOG_DIR/validator-4.log" 2>/dev/null; then
                print_success "✓ Checkpoint proposed by leader"
                checkpoint_proposed=1
            fi
        fi

        # Check for signature collection (look for 3/4 threshold)
        if [ $checkpoint_proposed -eq 1 ] && [ $signatures_collected -eq 0 ]; then
            for v in 1 2 3 4; do
                local sig_count=$(grep -c "Added signature to checkpoint" "$LOG_DIR/validator-$v.log" 2>/dev/null || echo "0")
                if [ $sig_count -ge 3 ]; then
                    print_success "✓ Threshold signatures collected (3/4)"
                    signatures_collected=1
                    break
                fi
            done
        fi

        # Check for checkpoint finalization
        if [ $signatures_collected -eq 1 ] && [ $checkpoint_finalized -eq 0 ]; then
            if grep -q "Finalized checkpoint" "$LOG_DIR/validator-1.log" 2>/dev/null || \
               grep -q "Finalized checkpoint" "$LOG_DIR/validator-2.log" 2>/dev/null || \
               grep -q "Finalized checkpoint" "$LOG_DIR/validator-3.log" 2>/dev/null || \
               grep -q "Finalized checkpoint" "$LOG_DIR/validator-4.log" 2>/dev/null; then
                print_success "✓ Checkpoint finalized"
                checkpoint_finalized=1
            fi
        fi

        # Check for ValidationBundle submission
        if [ $checkpoint_finalized -eq 1 ] && [ $bundle_submitted -eq 0 ]; then
            for v in 1 2 3 4; do
                if grep -q "Successfully submitted validation bundle" "$LOG_DIR/validator-$v.log" 2>/dev/null; then
                    print_success "✓ ValidationBundle submitted to RootLayer by validator-$v"
                    bundle_submitted=1

                    # Show the submission details
                    grep "Successfully submitted validation bundle" "$LOG_DIR/validator-$v.log" | tail -1
                    break
                fi
            done
        fi

        # If all steps completed, break
        if [ $checkpoint_proposed -eq 1 ] && \
           [ $signatures_collected -eq 1 ] && \
           [ $checkpoint_finalized -eq 1 ] && \
           [ $bundle_submitted -eq 1 ]; then
            echo ""
            print_success "✅ Full consensus achieved and ValidationBundle submitted!"
            return 0
        fi

        sleep 1
        count=$((count + 1))
    done

    echo ""
    print_error "Consensus did not complete within ${max_wait} seconds"
    print_info "Current state:"
    echo "  - Checkpoint proposed: $checkpoint_proposed"
    echo "  - Signatures collected: $signatures_collected"
    echo "  - Checkpoint finalized: $checkpoint_finalized"
    echo "  - Bundle submitted: $bundle_submitted"
    return 1
}

# Analyze consensus results
analyze_results() {
    print_header "Consensus Analysis"

    echo -e "\n${CYAN}Signature Collection:${NC}"
    for v in 1 2 3 4; do
        local sig_count=$(grep -c "Added signature to checkpoint" "$LOG_DIR/validator-$v.log" 2>/dev/null || echo "0")
        echo "  Validator $v: $sig_count signatures collected"
    done

    echo -e "\n${CYAN}Checkpoint Status:${NC}"
    for v in 1 2 3 4; do
        local proposed=$(grep -c "Proposed checkpoint" "$LOG_DIR/validator-$v.log" 2>/dev/null || echo "0")
        local finalized=$(grep -c "Finalized checkpoint" "$LOG_DIR/validator-$v.log" 2>/dev/null || echo "0")
        echo "  Validator $v: Proposed=$proposed, Finalized=$finalized"
    done

    echo -e "\n${CYAN}ValidationBundle Submission:${NC}"
    local bundle_found=0
    for v in 1 2 3 4; do
        if grep -q "Successfully submitted validation bundle" "$LOG_DIR/validator-$v.log" 2>/dev/null; then
            echo "  ✓ Validator $v successfully submitted ValidationBundle"
            bundle_found=1
        fi
    done

    if [ $bundle_found -eq 0 ]; then
        print_warning "No ValidationBundle submission detected"
        echo ""
        echo "Checking for submission attempts:"
        for v in 1 2 3 4; do
            if grep -q "Attempting to submit checkpoint to RootLayer" "$LOG_DIR/validator-$v.log" 2>/dev/null; then
                echo "  Validator $v attempted submission - check logs for errors"
            fi
        done
    fi

    echo -e "\n${CYAN}Leadership:${NC}"
    for v in 1 2 3 4; do
        if grep -q "Initial leadership status.*is_leader.*true" "$LOG_DIR/validator-$v.log" 2>/dev/null || \
           grep -q "Became leader" "$LOG_DIR/validator-$v.log" 2>/dev/null; then
            echo "  ✓ Validator $v was leader"
        fi
    done

    echo -e "\n${CYAN}Consensus Verification:${NC}"

    # Check if all validators reached the same epoch
    local epochs=()
    for v in 1 2 3 4; do
        local epoch=$(grep "Finalized checkpoint" "$LOG_DIR/validator-$v.log" 2>/dev/null | tail -1 | grep -o "epoch.*[0-9]" | grep -o "[0-9]*" | tail -1 || echo "N/A")
        epochs+=("$epoch")
        echo "  Validator $v finalized epoch: $epoch"
    done

    # Check if all epochs are the same
    local first_epoch="${epochs[0]}"
    local consensus_match=1
    for epoch in "${epochs[@]}"; do
        if [ "$epoch" != "$first_epoch" ] && [ "$epoch" != "N/A" ] && [ "$first_epoch" != "N/A" ]; then
            consensus_match=0
            break
        fi
    done

    echo ""
    if [ $consensus_match -eq 1 ] && [ "$first_epoch" != "N/A" ]; then
        print_success "✅ All validators reached consensus on epoch $first_epoch"
    else
        print_warning "⚠ Validators may have diverged on epoch"
    fi
}

# Show detailed logs
show_logs() {
    print_header "Recent Logs"

    for v in 1 2 3 4; do
        echo -e "\n${CYAN}=== Validator $v ===${NC}"
        tail -15 "$LOG_DIR/validator-$v.log" 2>/dev/null || echo "No logs available"
    done

    echo -e "\n${CYAN}Log Files:${NC}"
    echo "  Full logs available at: $LOG_DIR/"
    for v in 1 2 3 4; do
        echo "    - $LOG_DIR/validator-$v.log"
    done
}

# Main execution
main() {
    print_header "4-Node Validation Bundle Consensus Test"
    print_info "This test verifies that 4 validators can reach consensus and submit ValidationBundle"

    # Create log directory
    mkdir -p "$LOG_DIR"
    rm -f "$PIDS_FILE"

    # Check prerequisites
    check_prerequisites
    sleep 1

    # Start validators
    start_validators
    sleep 3

    # Submit test report to trigger consensus
    submit_test_report
    sleep 2

    # Monitor consensus process
    if monitor_consensus; then
        analyze_results
        show_logs

        print_header "Test Complete - SUCCESS"
        print_success "4-node consensus successfully achieved!"
        print_info "Press Ctrl+C to stop all validators and exit"

        # Keep running for inspection
        while true; do
            sleep 5
        done
    else
        analyze_results
        show_logs

        print_header "Test Complete - FAILED"
        print_error "Consensus did not complete successfully"
        print_info "Check logs above for details"
        exit 1
    fi
}

# Run main
main "$@"
