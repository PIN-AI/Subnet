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

# Check for architecture parameter
TARGET_ARCH="${1:-}"

if [ -n "$TARGET_ARCH" ]; then
    # User specified architecture - validate it
    if [ "$TARGET_ARCH" != "amd64" ] && [ "$TARGET_ARCH" != "arm64" ]; then
        print_error "Invalid architecture: $TARGET_ARCH"
        echo ""
        echo "Usage: $0 [amd64|arm64]"
        echo ""
        echo "  amd64  - Build for x86_64 servers (most AWS EC2, Intel servers)"
        echo "  arm64  - Build for ARM servers (AWS Graviton, Apple Silicon)"
        echo "  (none) - Auto-detect based on current machine"
        exit 1
    fi
    print_info "Using specified architecture: $TARGET_ARCH"
else
    # No parameter - auto-detect architecture
    ARCH=$(uname -m)
    if [ "$ARCH" = "x86_64" ]; then
        TARGET_ARCH="amd64"
    elif [ "$ARCH" = "arm64" ] || [ "$ARCH" = "aarch64" ]; then
        TARGET_ARCH="arm64"
    else
        print_error "Unsupported architecture: $ARCH"
        echo ""
        echo "Please specify target architecture manually:"
        echo "  $0 amd64   # For x86_64 servers"
        echo "  $0 arm64   # For ARM servers"
        exit 1
    fi
    print_info "Auto-detected architecture: $ARCH (Docker: $TARGET_ARCH)"
    print_warning "To build for a different architecture, run: $0 [amd64|arm64]"
fi
echo ""

# Check if binaries exist in architecture-specific directory
LINUX_BIN_DIR="bin/linux-${TARGET_ARCH}"
print_info "Checking for pre-compiled binaries in $LINUX_BIN_DIR/..."

REQUIRED_BINARIES=(
    "${LINUX_BIN_DIR}/validator"
    "${LINUX_BIN_DIR}/matcher"
    "${LINUX_BIN_DIR}/simple-agent"
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
    print_info "Please build Linux binaries first:"
    echo "  cd $PROJECT_ROOT"
    echo "  make build-linux-arm64  # For Apple Silicon (M1/M2/M3)"
    echo "  make build-linux-amd64  # For Intel Mac or x86_64 servers"
    exit 1
fi

# Check if binaries are in correct format (ELF, not Mach-O)
print_info "Checking binary format..."
for binary in "${REQUIRED_BINARIES[@]}"; do
    if file "$binary" | grep -q "Mach-O"; then
        print_error "Binary $binary is in Mach-O format (macOS)"
        echo ""
        echo "Docker requires Linux ELF format binaries."
        echo "Please rebuild using:"
        echo "  cd $PROJECT_ROOT"
        echo "  make build-linux-arm64  # For Apple Silicon"
        echo "  make build-linux-amd64  # For x86_64 servers"
        exit 1
    fi
done

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
print_info "Building Docker image: pinai-subnet:latest (platform: linux/${TARGET_ARCH})"
echo ""

cd "$DEPLOYMENT_DIR/docker"

# Use --platform to ensure correct architecture for cross-platform builds
# This is essential when building on Apple Silicon for x86_64 servers
docker build \
    --platform linux/${TARGET_ARCH} \
    --build-arg TARGETARCH=${TARGET_ARCH} \
    -f Dockerfile \
    -t pinai-subnet:latest \
    -t pinai-subnet:linux-${TARGET_ARCH} \
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

