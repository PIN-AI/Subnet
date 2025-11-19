# Environment Setup Guide

**📍 You are here:** First-Time Setup → Environment Setup

**Prerequisites:** None (this is step 2 of first-time setup)

**Time to complete:** ~10 minutes

**What you'll learn:**
- Install Go 1.24+
- Install Docker & Docker Compose
- Install required tools (openssl, protoc, make)

**Next steps:**
- ✅ After setup → Continue to [Quick Start Guide](quick_start.md) for deployment

---

Complete guide for setting up your development environment for PinAI Subnet.

## Table of Contents

- [System Requirements](#system-requirements)
- [Core Dependencies](#core-dependencies)
- [Optional Tools](#optional-tools)
- [Installation Instructions](#installation-instructions)
- [Verification](#verification)
- [Troubleshooting](#troubleshooting)
- [Next Steps](#next-steps)

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

Go is the primary language for Subnet components (Matcher, Validator).

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
   - Script formatting and output
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

## Verification

After installation, verify that all tools are correctly installed:

```bash
# 1. Verify Go
go version  # Should show go1.24.0 or later
go env GOPATH  # Should show your GOPATH

# 2. Verify Protocol Buffers
protoc --version  # Should show libprotoc 3.20.0 or later

# 3. Verify Git
git --version  # Should show git 2.30.0 or later

# 4. Verify Docker (if installed)
docker --version
docker compose version

# 5. Verify Go plugins are in PATH
which protoc-gen-go
which protoc-gen-go-grpc

# If not found, install Go protobuf plugins:
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

**Expected output**:
```
go version go1.24.0 darwin/arm64
GOPATH=/Users/username/go
libprotoc 24.4
git version 2.39.0
Docker version 24.0.0
Docker Compose version v2.20.0
/Users/username/go/bin/protoc-gen-go
/Users/username/go/bin/protoc-gen-go-grpc
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

**Problem**: Docker daemon not running

**Solution**:
```bash
# Linux: Start Docker service
sudo systemctl start docker
sudo systemctl enable docker

# macOS/Windows: Start Docker Desktop application

# Verify Docker is running
docker ps
```

---

## Next Steps

✅ **Environment setup complete!**

Now you're ready to deploy your subnet. Choose your deployment path:

### Option 1: Quick Start (Recommended)
Follow the [Quick Start Guide](quick_start.md) to:
- Clone the repository
- Get testnet ETH
- Create your subnet
- Register components
- Start your first validator

**Time to complete**: ~15 minutes

### Option 2: Detailed Manual Deployment
For advanced users who want full control, see the [Subnet Deployment Guide](subnet_deployment_guide.md):
- Manual command-line deployment
- Custom configuration
- Multi-validator setup
- Production deployment

**Time to complete**: ~30-45 minutes

### Option 3: Docker Deployment
For containerized deployment, see the [Docker Deployment Guide](../deployment/README.md):
- Deploy with Docker Compose
- Quick testing and development
- Simplified networking

**Time to complete**: ~10 minutes

---

**Need help?** Check:
- [Quick Start Guide](quick_start.md) - Step-by-step deployment
- [Subnet Deployment Guide](subnet_deployment_guide.md) - Advanced deployment options
- [FAQ](../deployment/FAQ.md) - Common questions
