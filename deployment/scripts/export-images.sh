#!/bin/bash
# Export Docker images for distribution
# Creates a tarball that can be transferred to other servers

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
echo "📦 PinAI Subnet - Export Images for Distribution"
echo "================================================"
echo ""

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOYMENT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$DEPLOYMENT_DIR"

# Check if image exists
if ! docker images pinai-subnet:latest | grep -q pinai-subnet; then
    print_error "Docker image 'pinai-subnet:latest' not found"
    echo ""
    echo "Please build the image first:"
    echo "  ./scripts/build-images.sh [amd64|arm64]"
    echo ""
    exit 1
fi

# Get architecture from the image
IMAGE_ARCH=$(docker inspect pinai-subnet:latest --format '{{.Architecture}}')
if [ -z "$IMAGE_ARCH" ]; then
    print_warning "Could not detect image architecture, using 'unknown'"
    IMAGE_ARCH="unknown"
fi

# Output file with architecture in name
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
OUTPUT_FILE="pinai-subnet-${IMAGE_ARCH}-${TIMESTAMP}.tar.gz"

print_info "Exporting image: pinai-subnet:latest (architecture: $IMAGE_ARCH)"
echo ""

# Save image
print_info "Saving Docker image to tar file..."
docker save pinai-subnet:latest | gzip > "$OUTPUT_FILE"

if [ $? -eq 0 ]; then
    print_success "Image exported successfully"
else
    print_error "Failed to export image"
    exit 1
fi

echo ""

# Show file info
FILE_SIZE=$(ls -lh "$OUTPUT_FILE" | awk '{print $5}')
print_info "Export complete:"
echo "   File: $OUTPUT_FILE"
echo "   Size: $FILE_SIZE"
echo ""

# Create distribution package
print_info "Creating distribution package..."

DIST_DIR="pinai-subnet-dist-${IMAGE_ARCH}-${TIMESTAMP}"
mkdir -p "$DIST_DIR"

# Copy necessary files
cp "$OUTPUT_FILE" "$DIST_DIR/"
cp -r config "$DIST_DIR/"
cp -r scripts "$DIST_DIR/"
mkdir -p "$DIST_DIR/docker"
cp docker/docker-compose.yml "$DIST_DIR/docker/"
cp docker/entrypoint.sh "$DIST_DIR/docker/" 2>/dev/null || true
cp ../.env.example "$DIST_DIR/.env.example"
cp README.md "$DIST_DIR/" 2>/dev/null || true

# Create installation script
cat > "$DIST_DIR/install.sh" << 'EOF'
#!/bin/bash
# PinAI Subnet - Quick Installation Script

set -e

echo "🐳 PinAI Subnet Installation"
echo "============================"
echo ""

# Find tar.gz file
TARBALL=$(ls pinai-subnet-*.tar.gz 2>/dev/null | head -1)

if [ -z "$TARBALL" ]; then
    echo "❌ Error: Image tarball not found"
    exit 1
fi

echo "📦 Loading Docker image..."
docker load < "$TARBALL"

echo "✓ Image loaded successfully"
echo ""
echo "📋 Next steps:"
echo "   1. Configure: cp .env.example .env && nano .env"
echo "   2. Deploy:    ./scripts/deploy.sh"
echo ""
EOF

chmod +x "$DIST_DIR/install.sh"
chmod +x "$DIST_DIR/scripts/"*.sh

# Create tarball
DIST_TARBALL="${DIST_DIR}.tar.gz"
print_info "Creating distribution tarball..."
tar czf "$DIST_TARBALL" "$DIST_DIR"

# Cleanup
rm -rf "$DIST_DIR"
rm "$OUTPUT_FILE"

DIST_SIZE=$(ls -lh "$DIST_TARBALL" | awk '{print $5}')
print_success "Distribution package created: $DIST_TARBALL ($DIST_SIZE)"
echo ""

# Show instructions
echo "======================================"
echo -e "${GREEN}📤 Ready for Distribution${NC}"
echo "======================================"
echo ""
echo "📦 Package: $DIST_TARBALL"
echo "📏 Size: $DIST_SIZE"
echo "🖥️  Architecture: $IMAGE_ARCH"
echo ""
echo "🚀 Deployment on target server ($IMAGE_ARCH):"
echo ""
echo "   # Transfer file"
echo "   scp $DIST_TARBALL user@server:/opt/"
echo ""
echo "   # On target server"
echo "   cd /opt"
echo "   tar xzf $DIST_TARBALL"
echo "   cd ${DIST_DIR}"
echo "   ./install.sh"
echo "   cp .env.example .env"
echo "   nano .env  # Configure"
echo "   ./scripts/deploy.sh"
echo ""

