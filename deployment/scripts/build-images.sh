#!/bin/bash
# Build Docker images from pre-compiled binaries
# Run this script from the deployment directory

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
echo "🐳 PinAI Subnet - Build Production Images"
echo "=========================================="
echo ""

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOYMENT_DIR="$(dirname "$SCRIPT_DIR")"
PROJECT_ROOT="$(dirname "$DEPLOYMENT_DIR")"

cd "$PROJECT_ROOT"

# Check if binaries exist
print_info "Checking for pre-compiled binaries..."

REQUIRED_BINARIES=(
    "bin/validator"
    "bin/matcher"
    "bin/registry"
    "bin/simple-agent"
)

MISSING_BINARIES=()
for binary in "${REQUIRED_BINARIES[@]}"; do
    if [ ! -f "$binary" ]; then
        MISSING_BINARIES+=("$binary")
    fi
done

if [ ${#MISSING_BINARIES[@]} -gt 0 ]; then
    print_error "Missing required binaries:"
    for binary in "${MISSING_BINARIES[@]}"; do
        echo "  - $binary"
    done
    echo ""
    print_info "Please build binaries first:"
    echo "  cd $PROJECT_ROOT"
    echo "  make build"
    exit 1
fi

print_success "All required binaries found"
echo ""

# Show binary info
print_info "Binary information:"
for binary in "${REQUIRED_BINARIES[@]}"; do
    size=$(ls -lh "$binary" | awk '{print $5}')
    echo "  - $binary ($size)"
done
echo ""

# Build Docker image
print_info "Building Docker image: pinai-subnet:latest"
echo ""

cd "$DEPLOYMENT_DIR/docker"

docker build \
    -f Dockerfile \
    -t pinai-subnet:latest \
    "$PROJECT_ROOT"

if [ $? -eq 0 ]; then
    print_success "Docker image built successfully"
else
    print_error "Docker image build failed"
    exit 1
fi

echo ""

# Show image info
print_info "Image information:"
docker images pinai-subnet:latest --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}\t{{.CreatedAt}}"
echo ""

# Tag with timestamp
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
docker tag pinai-subnet:latest pinai-subnet:$TIMESTAMP
print_success "Tagged as pinai-subnet:$TIMESTAMP"
echo ""

print_success "Build complete!"
echo ""
echo "📋 Next steps:"
echo "   1. Configure environment:  cp ../.env.example .env && nano .env"
echo "   2. Deploy services:        ./scripts/deploy.sh"
echo "   3. Export for distribution: ./scripts/export-images.sh"
echo ""

