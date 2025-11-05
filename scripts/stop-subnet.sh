#!/bin/bash
# PinAI Subnet Services Stop Script
# 优雅停止所有 Subnet 服务

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Project root
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOGS_DIR="$PROJECT_ROOT/subnet-logs"

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

echo ""
echo "🛑 PinAI Subnet Services Stop Script"
echo "====================================="
echo ""

# Function to gracefully stop a process
graceful_stop() {
    local process_name=$1
    local pid_file=$2
    local timeout=${3:-10}  # Default 10 seconds timeout

    if [ ! -f "$pid_file" ]; then
        print_warning "$process_name PID file not found, trying pkill"
        pkill -f "bin/$process_name" 2>/dev/null && print_success "$process_name stopped" || print_warning "$process_name not running"
        return
    fi

    local pid=$(cat "$pid_file")

    if ! kill -0 $pid 2>/dev/null; then
        print_warning "$process_name (PID: $pid) not running"
        rm -f "$pid_file"
        return
    fi

    print_info "Stopping $process_name (PID: $pid)..."

    # Send SIGTERM for graceful shutdown
    kill -TERM $pid 2>/dev/null || true

    # Wait for process to stop
    local count=0
    while kill -0 $pid 2>/dev/null && [ $count -lt $timeout ]; do
        sleep 1
        count=$((count + 1))
        echo -n "."
    done
    echo ""

    # Check if process stopped
    if kill -0 $pid 2>/dev/null; then
        print_warning "$process_name did not stop gracefully, sending SIGKILL"
        kill -9 $pid 2>/dev/null || true
        sleep 1
    fi

    if ! kill -0 $pid 2>/dev/null; then
        print_success "$process_name stopped"
        rm -f "$pid_file"
    else
        print_error "Failed to stop $process_name"
    fi
}

# Stop services in reverse order (opposite of startup)
print_info "Stopping Subnet services..."
echo ""

# Stop Validator first (to avoid orphaned consensus states)
if [ -f "$LOGS_DIR/validator.pid" ]; then
    graceful_stop "validator" "$LOGS_DIR/validator.pid" 15
else
    print_warning "Validator PID file not found"
    pkill -f "bin/validator" 2>/dev/null && print_success "Validator stopped" || print_info "Validator not running"
fi

# Stop Matcher
if [ -f "$LOGS_DIR/matcher.pid" ]; then
    graceful_stop "matcher" "$LOGS_DIR/matcher.pid" 10
else
    print_warning "Matcher PID file not found"
    pkill -f "bin/matcher" 2>/dev/null && print_success "Matcher stopped" || print_info "Matcher not running"
fi

# Stop Registry
if [ -f "$LOGS_DIR/registry.pid" ]; then
    graceful_stop "registry" "$LOGS_DIR/registry.pid" 5
else
    print_warning "Registry PID file not found"
    pkill -f "bin/registry" 2>/dev/null && print_success "Registry stopped" || print_info "Registry not running"
fi

echo ""
print_success "All services stopped"

# Optional: Clean up old logs (commented out by default)
# print_info "Cleaning up old logs..."
# rm -f "$LOGS_DIR"/*.log
# print_success "Logs cleaned"

echo ""
echo "====================================="
echo -e "${GREEN}✅ Shutdown complete${NC}"
echo "====================================="
echo ""
echo "💡 To restart services, run: ./start-subnet.sh"
echo ""
