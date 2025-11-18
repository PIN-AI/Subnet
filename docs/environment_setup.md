# Environment Setup Guide

**📍 You are here:** First-Time Setup → Environment Setup

**Prerequisites:** None (this is step 2 of first-time setup)

**Time to complete:** ~10 minutes

**What you'll learn:**
- Install Go 1.24+
- Install Docker & Docker Compose
- Install required tools (openssl, protoc, make)

**Next steps:**
- ✅ After setup → Choose deployment: [Docker](../deployment/README.md) or [Manual](subnet_deployment_guide.md)

---

Complete guide for setting up your development environment for PinAI Subnet.

## Table of Contents

- [System Requirements](#system-requirements)
- [Core Dependencies](#core-dependencies)
- [Optional Tools](#optional-tools)
- [Installation Instructions](#installation-instructions)
- [Project Setup](#project-setup)
- [Configuration](#configuration)
- [Verification](#verification)
- [Troubleshooting](#troubleshooting)

---

## System Requirements

### Supported Operating Systems

- **Linux** (Ubuntu 20.04+, Debian 11+, CentOS 8+)
- **macOS** (11.0 Big Sur or later)
- **Windows** (WSL2 recommended)

### Hardware Requirements

**Minimum**:
- CPU: 2 cores
- RAM: 4 GB
- Disk: 20 GB free space
- Network: Stable internet connection

**Recommended for Production**:
- CPU: 4+ cores
- RAM: 8+ GB
- Disk: 100+ GB SSD
- Network: Low-latency connection (< 100ms to blockchain RPC)

---

## Core Dependencies

### 1. Go Programming Language

**Version Required**: Go 1.24.0 or later

Go is the primary language for Subnet components (Matcher, Validator, Registry).

**Why needed**:
- Compile Subnet binaries
- Run Go-based scripts
- Build custom agents

### 2. Protocol Buffers Compiler (protoc)

**Version Required**: protoc 3.20.0 or later

Used for generating gRPC service definitions.

**Why needed**:
- Generate protocol buffer code
- Compile `.proto` files to Go/Python

### 3. Git

**Version Required**: Git 2.30.0 or later

Version control system.

**Why needed**:
- Clone the repository
- Manage code versions
- Contribute to the project

---

## Optional Tools

### For Testing & Development

1. **Docker & Docker Compose**
   - Containerized testing
   - Version: Docker 20.10+ / Docker Compose 2.0+

2. **curl**
   - HTTP API testing
   - RootLayer interaction
   - Pre-installed on most systems

3. **jq**
   - JSON parsing in scripts
   - Registry CLI formatting
   - Not required but highly recommended

### For Blockchain Interaction

4. **Ethereum Node Access**
   - Public RPC: Infura, Alchemy, QuickNode
   - Or self-hosted node
   - Required for on-chain operations

5. **Ethereum Wallet**
   - Private key with ETH for gas fees
   - Testnet ETH for development
   - Sepolia testnet recommended

### For Python SDK Development

6. **Python 3.9+**
   - For Python SDK development
   - pip package manager
   - virtualenv recommended

---

## Installation Instructions

### macOS

#### Using Homebrew (Recommended)

```bash
# Install Homebrew if not already installed
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Install Go
brew install go

# Verify Go installation
go version  # Should show go1.24.0 or later

# Install Protocol Buffers
brew install protobuf

# Verify protoc installation
protoc --version  # Should show libprotoc 3.20.0 or later

# Install optional tools
brew install curl jq git

# Install Docker Desktop (includes Docker Compose)
# Download from: https://www.docker.com/products/docker-desktop
```

#### Manual Installation

**Go**:
```bash
# Download Go from https://go.dev/dl/
curl -OL https://go.dev/dl/go1.24.0.darwin-arm64.tar.gz  # For Apple Silicon
# or
curl -OL https://go.dev/dl/go1.24.0.darwin-amd64.tar.gz  # For Intel

# Extract and install
sudo tar -C /usr/local -xzf go1.24.0.darwin-*.tar.gz

# Add to PATH (add to ~/.zshrc or ~/.bash_profile)
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin

# Reload shell configuration
source ~/.zshrc  # or source ~/.bash_profile
```

**Protocol Buffers**:
```bash
# Download from https://github.com/protocolbuffers/protobuf/releases
curl -OL https://github.com/protocolbuffers/protobuf/releases/download/v24.4/protoc-24.4-osx-universal_binary.zip

# Extract
unzip protoc-24.4-osx-universal_binary.zip -d $HOME/protoc

# Add to PATH
export PATH=$PATH:$HOME/protoc/bin
```

---

### Linux (Ubuntu/Debian)

```bash
# Update package index
sudo apt update

# Install basic tools
sudo apt install -y curl git build-essential unzip

# Install Go
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz

# Add to PATH (add to ~/.bashrc or ~/.zshrc)
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
echo 'export GOPATH=$HOME/go' >> ~/.bashrc
echo 'export PATH=$PATH:$GOPATH/bin' >> ~/.bashrc
source ~/.bashrc

# Verify Go
go version

# Install Protocol Buffers
sudo apt install -y protobuf-compiler

# Or install latest version manually
PB_REL="https://github.com/protocolbuffers/protobuf/releases"
curl -LO $PB_REL/download/v24.4/protoc-24.4-linux-x86_64.zip
unzip protoc-24.4-linux-x86_64.zip -d $HOME/.local
export PATH="$PATH:$HOME/.local/bin"

# Verify protoc
protoc --version

# Install Docker (optional)
sudo apt install -y docker.io docker-compose
sudo systemctl start docker
sudo systemctl enable docker
sudo usermod -aG docker $USER  # Add user to docker group

# Install jq (optional but recommended)
sudo apt install -y jq
```

---

### Linux (CentOS/RHEL/Fedora)

```bash
# Install basic tools
sudo yum install -y curl git gcc make unzip

# Install Go
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz

# Add to PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
echo 'export GOPATH=$HOME/go' >> ~/.bashrc
echo 'export PATH=$PATH:$GOPATH/bin' >> ~/.bashrc
source ~/.bashrc

# Install Protocol Buffers
sudo yum install -y protobuf-compiler

# Install Docker (Fedora/CentOS 8+)
sudo dnf install -y docker docker-compose
sudo systemctl start docker
sudo systemctl enable docker
sudo usermod -aG docker $USER
```

---

### Windows (WSL2)

**Recommended**: Use Windows Subsystem for Linux 2 (WSL2) for best compatibility.

1. **Install WSL2**:
   ```powershell
   # In PowerShell (Administrator)
   wsl --install
   # Or specify Ubuntu
   wsl --install -d Ubuntu-22.04
   ```

2. **Follow Linux (Ubuntu) instructions** inside WSL2 terminal

3. **Install Docker Desktop for Windows**:
   - Download from https://www.docker.com/products/docker-desktop
   - Enable WSL2 integration in Docker Desktop settings

---

## Project Setup

### 1. Clone the Repository

```bash
# Clone Subnet repository
git clone https://github.com/PIN-AI/Subnet.git
cd Subnet
```

### 2. Install Go Dependencies

```bash
# Download and install Go module dependencies
go mod download

# Verify dependencies
go mod verify
```

### 3. Install Go Protocol Buffer Plugins

```bash
# Install protoc-gen-go (for Protocol Buffers)
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# Install protoc-gen-go-grpc (for gRPC)
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Verify installations
which protoc-gen-go
which protoc-gen-go-grpc
```

### 4. Build All Binaries

```bash
# Build all components using Makefile
make build

# This creates binaries in ./bin/:
# - matcher
# - validator
# - registry
# - mock-rootlayer
# - simple-agent
```

### 5. Verify Build

```bash
# Check built binaries
ls -lh bin/

# Test running a binary
./bin/matcher --help
./bin/validator --help
```

---

## Configuration

### 1. Obtain Testnet ETH

For testing on Base Sepolia:

1. **Get a wallet**:
   - Create with MetaMask or use existing wallet
   - Save your private key securely

2. **Get testnet ETH**:
   - Base Sepolia Faucet: https://www.coinbase.com/faucets/base-ethereum-goerli-faucet
   - Or bridge from Sepolia to Base Sepolia

3. **Save configuration**:
   ```bash
   # Create .env file (DO NOT commit this!)
   cat > .env << 'EOF'
   PRIVATE_KEY=your_private_key_here
   RPC_URL=https://sepolia.base.org
   NETWORK=base_sepolia
   EOF

   # Secure the file
   chmod 600 .env
   ```

### 2. Configure RootLayer Connection

```bash
# Set RootLayer endpoints (example - adjust for your deployment)
export ROOTLAYER_GRPC="3.17.208.238:9001"
export ROOTLAYER_HTTP="http://3.17.208.238:8081/api/v1"

# Or add to your .env file
echo "ROOTLAYER_GRPC=3.17.208.238:9001" >> .env
echo "ROOTLAYER_HTTP=http://3.17.208.238:8081/api/v1" >> .env
```

### 3. Create Configuration File

```bash
# Copy example configuration
cp config/config.example.yaml config/config.yaml

# Edit with your values
nano config/config.yaml
# or
vim config/config.yaml
```

**Minimal config/config.yaml**:
```yaml
subnet_id: "0x0000000000000000000000000000000000000000000000000000000000000003"

identity:
  matcher_id: "matcher-001"
  subnet_id: "0x0000000000000000000000000000000000000000000000000000000000000003"

network:
  matcher_grpc_port: ":8090"

rootlayer:
  http_url: "http://3.17.208.238:8081/api/v1"
  grpc_endpoint: "3.17.208.238:9001"

blockchain:
  rpc_url: "https://sepolia.base.org"
  subnet_contract: "0xYOUR_SUBNET_CONTRACT"

agent:
  matcher:
    signer:
      type: "ecdsa"
      private_key: "YOUR_PRIVATE_KEY"
```

---

## Verification

### Test Your Setup

```bash
# 1. Verify Go works
go version
go env GOPATH

# 2. Verify protoc works
protoc --version

# 3. Verify binaries run
./bin/matcher --help
./bin/validator --help

# 4. Run unit tests
make test
# or
go test ./...

# 5. Build protocol buffers
make proto

# 6. Check Go module status
go mod verify
go mod tidy
```

### Run Quick E2E Test

```bash
# Set required environment variables
export SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000003"
export ROOTLAYER_GRPC="3.17.208.238:9001"
export ROOTLAYER_HTTP="http://3.17.208.238:8081/api/v1"

# Run E2E test (non-interactive mode)
./scripts/start-subnet.sh --no-interactive

# Check for success
echo $?  # Should be 0
```

---

## Troubleshooting

### Go Issues

**Problem**: `go: command not found`

**Solution**:
```bash
# Verify Go is installed
which go

# If not found, check PATH
echo $PATH | grep go

# Add Go to PATH
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin

# Make permanent by adding to ~/.bashrc or ~/.zshrc
```

---

**Problem**: `cannot find package`

**Solution**:
```bash
# Clean module cache
go clean -modcache

# Re-download dependencies
go mod download

# Verify modules
go mod verify

# Update dependencies
go mod tidy
```

---

### Build Issues

**Problem**: `make: command not found`

**Solution**:
```bash
# Install build-essential (Ubuntu/Debian)
sudo apt install build-essential

# Or install make only
sudo apt install make

# macOS
xcode-select --install
```

---

**Problem**: Protocol buffer compilation fails

**Solution**:
```bash
# Check protoc version
protoc --version  # Need 3.20.0+

# Verify protoc plugins are installed
which protoc-gen-go
which protoc-gen-go-grpc

# Reinstall plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Verify PATH includes $GOPATH/bin
echo $PATH | grep $GOPATH/bin

# Add if missing
export PATH=$PATH:$GOPATH/bin
```

---

**Problem**: Binary fails to execute: "permission denied"

**Solution**:
```bash
# Make binaries executable
chmod +x bin/*

# Or rebuild
make clean
make build
```

---

### Network/Blockchain Issues

**Problem**: Cannot connect to RPC endpoint

**Solution**:
```bash
# Test RPC connection
curl -X POST \
  -H "Content-Type: application/json" \
  --data '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' \
  https://sepolia.base.org

# Try alternative RPC
export RPC_URL="https://base-sepolia.g.alchemy.com/v2/YOUR_API_KEY"

# Check if RPC requires API key
# Get free API key from:
# - Alchemy: https://www.alchemy.com/
# - Infura: https://infura.io/
# - QuickNode: https://www.quicknode.com/
```

---

**Problem**: Insufficient funds for gas

**Solution**:
```bash
# Check balance
# Use block explorer: https://sepolia.basescan.org/

# Get testnet ETH
# - Base Sepolia Faucet: https://www.coinbase.com/faucets
# - Or bridge from Sepolia ETH

# Verify wallet address
go run scripts/derive-pubkey.go $PRIVATE_KEY
```

---

### Docker Issues

**Problem**: `permission denied` for Docker commands

**Solution**:
```bash
# Add user to docker group
sudo usermod -aG docker $USER

# Log out and log back in for group to take effect
# Or run
newgrp docker

# Test
docker ps
```

---

**Problem**: Port already in use

**Solution**:
```bash
# Find what's using the port
lsof -i :8090  # Matcher
lsof -i :9200  # Validator
lsof -i :7400  # Validator Raft
lsof -i :7950  # Validator Gossip

# Kill the process
kill -9 <PID>

# Or stop Docker container
docker ps
docker stop <container_name>

# Or change port in configuration
```

---

## Environment Variables Reference

### Required for Development

```bash
# Go
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin

# Protocol Buffers
export PATH=$PATH:$HOME/.local/bin  # If installed locally

# Project-specific
export SUBNET_ID="0x..."
export ROOTLAYER_GRPC="host:port"
export ROOTLAYER_HTTP="http://host:port"
```

### Required for Blockchain Operations

```bash
# Wallet
export PRIVATE_KEY="0x..."
export RPC_URL="https://sepolia.base.org"
export NETWORK="base_sepolia"

# Contracts
export PIN_BASE_SEPOLIA_INTENT_MANAGER="0x..."
export PIN_BASE_SEPOLIA_SUBNET_FACTORY="0x..."
```

### Optional

```bash
# Registry
export REGISTRY_URL="http://localhost:8092"

# Testing
export ENABLE_CHAIN_SUBMIT="true"
export CHAIN_RPC_URL="https://sepolia.base.org"
```

---

## Next Steps

After completing environment setup:

1. **Create a Subnet**:
   ```bash
   ./scripts/create-subnet.sh --help
   ```

2. **Register Participants**:
   ```bash
   ./scripts/register.sh --help
   ```

3. **Run E2E Test**:
   ```bash
   ./scripts/start-subnet.sh --help
   ```

4. **Read Documentation**:
   - [Subnet Deployment Guide](./subnet_deployment_guide.md)
   - [Scripts Guide](./scripts_guide.md)
   - [Architecture Overview](./ARCHITECTURE_OVERVIEW.md)

5. **Join Community**:
   - GitHub: https://github.com/PIN-AI/Subnet
   - Discord: [link]
   - Documentation: [link]

---

## Development Workflow

Typical development workflow after setup:

```bash
# 1. Make code changes
vim internal/matcher/server.go

# 2. Rebuild affected components
make build

# 3. Run tests
go test ./internal/matcher/...

# 4. Test locally
./scripts/start-subnet.sh

# 5. Check for issues
go vet ./...
go fmt ./...

# 6. Commit changes
git add .
git commit -m "feat: improve matcher bidding logic"
```

---

## Performance Tuning

### For Production Deployment

1. **Increase file descriptor limits**:
   ```bash
   ulimit -n 65536
   # Make permanent in /etc/security/limits.conf
   ```

2. **Optimize Go runtime**:
   ```bash
   export GOMAXPROCS=$(nproc)  # Use all CPU cores
   export GOGC=50  # More aggressive GC
   ```

3. **Monitor resources**:
   ```bash
   # Install monitoring tools
   sudo apt install htop iotop nethogs

   # Monitor processes
   htop
   ```

---

## Security Best Practices

1. **Never commit private keys** to version control
2. **Use environment variables** for sensitive data
3. **Set restrictive file permissions**: `chmod 600 config/config.yaml`
4. **Use separate wallets** for testing and production
5. **Keep dependencies updated**: `go get -u ./...`
6. **Use firewall rules** to restrict access
7. **Enable TLS** for production gRPC endpoints

---

## Getting Help

If you encounter issues:

1. Check [Troubleshooting](#troubleshooting) section
2. Review [Scripts Guide](./scripts_guide.md)
3. Search GitHub Issues: https://github.com/PIN-AI/Subnet/issues
4. Ask in Discord: [link]
5. Create a new issue with:
   - Operating system and version
   - Go version (`go version`)
   - Error messages (full output)
   - Steps to reproduce

---

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `make test`
5. Format code: `go fmt ./...`
6. Submit a pull request

See [CONTRIBUTING.md](../CONTRIBUTING.md) for detailed guidelines.
