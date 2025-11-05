#!/bin/bash
# PinAI Subnet - Docker One-Click Startup Script
# The simplest deployment method!

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

print_info() { echo -e "${BLUE}ℹ${NC} $1"; }
print_success() { echo -e "${GREEN}✓${NC} $1"; }
print_error() { echo -e "${RED}✗${NC} $1"; }
print_warning() { echo -e "${YELLOW}⚠${NC} $1"; }

echo ""
echo "🐳 PinAI Subnet - Docker Quick Start"
echo "===================================="
echo ""

# ============================================
# Check Docker
# ============================================

if ! command -v docker &> /dev/null; then
    print_error "Docker is not installed!"
    echo ""
    echo "Please install Docker first:"
    echo "  Ubuntu: curl -fsSL https://get.docker.com | sh"
    echo "  macOS:  brew install --cask docker"
    echo "  Or visit: https://docs.docker.com/get-docker/"
    exit 1
fi

if ! command -v docker compose &> /dev/null && ! command -v docker-compose &> /dev/null; then
    print_error "Docker Compose is not installed!"
    echo ""
    echo "Please install Docker Compose:"
    echo "  Ubuntu: sudo apt install docker-compose-plugin"
    echo "  macOS: Included in Docker Desktop"
    exit 1
fi

print_success "Docker environment check passed"
echo ""

# Use docker compose or docker-compose
if command -v docker compose &> /dev/null; then
    DOCKER_COMPOSE="docker compose"
else
    DOCKER_COMPOSE="docker-compose"
fi

# ============================================
# Check .env file
# ============================================

if [ ! -f ".env" ]; then
    print_warning ".env file does not exist"
    echo ""
    read -p "Create .env template automatically? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        cp docs/env_template.txt .env
        chmod 600 .env
        print_success ".env file created"
        echo ""
        print_warning "⚠️  Please edit .env file first and fill in required configuration:"
        echo "  1. TEST_PRIVATE_KEY"
        echo "  2. VALIDATOR_KEYS"
        echo "  3. VALIDATOR_PUBKEYS"
        echo "  4. SUBNET_ID"
        echo "  5. ROOTLAYER_GRPC"
        echo "  6. ROOTLAYER_HTTP"
        echo ""
        read -p "Edit .env file now? (y/n) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            ${EDITOR:-nano} .env
        else
            print_info "Please edit manually: nano .env"
            exit 0
        fi
    else
        print_error ".env file is required to continue"
        echo "Please run: cp docs/env_template.txt .env"
        exit 1
    fi
fi

# Load and validate .env
set -a
source .env
set +a

# Check required variables
MISSING_VARS=()

[ -z "$TEST_PRIVATE_KEY" ] && MISSING_VARS+=("TEST_PRIVATE_KEY")
[ -z "$VALIDATOR_KEYS" ] && MISSING_VARS+=("VALIDATOR_KEYS")
[ -z "$VALIDATOR_PUBKEYS" ] && MISSING_VARS+=("VALIDATOR_PUBKEYS")
[ -z "$SUBNET_ID" ] && MISSING_VARS+=("SUBNET_ID")
[ -z "$ROOTLAYER_GRPC" ] && MISSING_VARS+=("ROOTLAYER_GRPC")
[ -z "$ROOTLAYER_HTTP" ] && MISSING_VARS+=("ROOTLAYER_HTTP")

if [ ${#MISSING_VARS[@]} -gt 0 ]; then
    print_error "Missing required environment variables:"
    for var in "${MISSING_VARS[@]}"; do
        echo "  - $var"
    done
    echo ""
    echo "Please edit .env file and fill in these values"
    exit 1
fi

print_success "Environment variables configured correctly"
echo ""

# ============================================
# Show configuration
# ============================================

echo "📋 Current Configuration:"
echo "   Subnet ID: $SUBNET_ID"
echo "   RootLayer gRPC: $ROOTLAYER_GRPC"
echo "   RootLayer HTTP: $ROOTLAYER_HTTP"
echo "   Blockchain: ${CHAIN_NETWORK:-base_sepolia}"
echo ""

# ============================================
# Check if already running
# ============================================

if $DOCKER_COMPOSE ps | grep -q "Up"; then
    print_warning "Services are already running"
    echo ""
    read -p "Restart services? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        print_info "Stopping existing services..."
        $DOCKER_COMPOSE down
        print_success "Services stopped"
        echo ""
    else
        print_info "Keeping existing services running"
        $DOCKER_COMPOSE ps
        exit 0
    fi
fi

# ============================================
# Create directories
# ============================================

print_info "Creating data directories..."
mkdir -p data/registry data/matcher data/validator subnet-logs
print_success "Data directories created"
echo ""

# ============================================
# Ask about agent
# ============================================

echo "Start Agent service?"
echo "  Agent is used for testing Intent submission and execution"
read -p "Start Agent? (y/n) [default: n] " -n 1 -r
echo
echo ""

START_AGENT=false
PROFILE_FLAG=""
if [[ $REPLY =~ ^[Yy]$ ]]; then
    START_AGENT=true
    PROFILE_FLAG="--profile with-agent"
    print_info "Will start Agent service"
else
    print_info "Not starting Agent service"
fi
echo ""

# ============================================
# Build and start
# ============================================

print_info "Building Docker images..."
echo "(May take a few minutes on first run)"
echo ""

if $DOCKER_COMPOSE build; then
    print_success "Image build completed"
else
    print_error "Image build failed"
    exit 1
fi
echo ""

print_info "Starting services..."
if $DOCKER_COMPOSE $PROFILE_FLAG up -d; then
    print_success "Services started successfully!"
else
    print_error "Service startup failed"
    echo ""
    echo "View logs:"
    echo "  $DOCKER_COMPOSE logs"
    exit 1
fi
echo ""

# ============================================
# Wait for services
# ============================================

print_info "Waiting for services to be ready..."
sleep 5

# Check service status
echo ""
echo "📊 Service Status:"
$DOCKER_COMPOSE ps
echo ""

# ============================================
# Health checks
# ============================================

print_info "Checking service health..."
sleep 5

HEALTH_OK=true

# Check Registry
if curl -s http://localhost:8101/health > /dev/null 2>&1; then
    print_success "Registry: Healthy ✓"
else
    print_warning "Registry: Not responding"
    HEALTH_OK=false
fi

# Check Matcher
if curl -s http://localhost:8092/health > /dev/null 2>&1; then
    print_success "Matcher: Healthy ✓"
else
    print_warning "Matcher: Not responding"
    HEALTH_OK=false
fi

# Check Validator (just port check)
if nc -z localhost 9090 2>/dev/null; then
    print_success "Validator: Running ✓"
else
    print_warning "Validator: Port not open"
    HEALTH_OK=false
fi

echo ""

if [ "$HEALTH_OK" = false ]; then
    print_warning "Some services may still be starting..."
    echo "Please wait a moment, or check logs:"
    echo "  $DOCKER_COMPOSE logs -f"
fi

# ============================================
# Summary
# ============================================

echo "=================================="
echo -e "${GREEN}🎉 Deployment Complete!${NC}"
echo "=================================="
echo ""
echo "🌐 Service Addresses:"
echo "   Registry HTTP: http://localhost:8101"
echo "   Matcher HTTP:  http://localhost:8092"
echo "   Matcher gRPC:  localhost:8090"
echo "   Validator gRPC: localhost:9090"
echo ""
echo "📋 Common Commands:"
echo "   View logs:      $DOCKER_COMPOSE logs -f"
echo "   View status:    $DOCKER_COMPOSE ps"
echo "   Stop services:  $DOCKER_COMPOSE down"
echo "   Restart:        $DOCKER_COMPOSE restart"
echo "   Enter container: $DOCKER_COMPOSE exec validator sh"
echo ""
echo "🔍 Health Checks:"
echo "   curl http://localhost:8101/health  # Registry"
echo "   curl http://localhost:8092/health  # Matcher"
echo ""
echo "📊 View Resource Usage:"
echo "   docker stats"
echo ""
echo "🛑 Stop All Services:"
echo "   $DOCKER_COMPOSE down"
echo ""
echo "📚 Detailed Documentation:"
echo "   docker/README.md"
echo "   docs/quick_start.md"
echo ""

# ============================================
# Ask to follow logs
# ============================================

read -p "View logs in real-time? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo ""
    echo "Press Ctrl+C to exit log view (services will continue running)"
    echo ""
    sleep 2
    $DOCKER_COMPOSE logs -f
fi

echo ""
print_success "Services are running! Use '$DOCKER_COMPOSE logs -f' to view logs"
echo ""
