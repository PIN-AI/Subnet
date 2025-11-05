# Scripts Directory

This directory contains all operational scripts and tools for the PinAI Subnet project.

---

## Table of Contents

- [Core Scripts](#core-scripts)
  - [Startup & Shutdown](#startup--shutdown)
  - [Root Directory Shortcuts](#root-directory-shortcuts)
- [Testing Scripts](#testing-scripts)
- [Deployment Scripts](#deployment-scripts)
- [Utility Scripts](#utility-scripts)
- [CometBFT Scripts](#cometbft-scripts)
- [Script Relationships](#script-relationships)
- [Environment Variables](#environment-variables)
- [Troubleshooting](#troubleshooting)
- [Development Guidelines](#development-guidelines)

---

## Core Scripts

### Startup & Shutdown

#### `start-subnet.sh`
**Purpose**: Start complete subnet (registry + matcher + validators + agent)

**Features**:
- Supports both Raft and CometBFT consensus modes
- Auto-configures multi-validator nodes
- Optional simple-agent startup
- Test mode support (background execution)

**Usage**:
```bash
# Raft mode (default) - 3 validators
./scripts/start-subnet.sh

# CometBFT mode
export CONSENSUS_TYPE=cometbft
./scripts/start-subnet.sh

# 5 validators
export NUM_VALIDATORS=5
./scripts/start-subnet.sh

# Test mode (background)
export TEST_MODE=true
./scripts/start-subnet.sh
```

**Environment Variables**:
- `CONSENSUS_TYPE`: Consensus type (`raft` or `cometbft`, default: `raft`)
- `NUM_VALIDATORS`: Number of validators (default: 3)
- `SUBNET_ID`: Subnet ID
- `ROOTLAYER_GRPC`: RootLayer gRPC address
- `ROOTLAYER_HTTP`: RootLayer HTTP API address
- `TEST_MODE`: Test mode (true=background, false=foreground)
- `START_AGENT`: Whether to start agent (true/false, default: true)

**Related Files**:
- `stop-subnet.sh` - Stop all services
- `send-intent.sh` - Interactive intent submission

---

## Root Directory Shortcuts

### `send-intent.sh`
**Purpose**: Interactive Intent submission tool

**Location**: `scripts/send-intent.sh`

**Features**:
- User-friendly interactive interface
- Custom Intent submission
- Built-in E2E test Intent templates
- Auto-loads .env configuration

**Usage**:
```bash
# Ensure .env is configured
./scripts/send-intent.sh

# Menu options:
# 1) Submit custom Intent
# 2) Submit E2E test Intent
# 3) Submit demo Intent
# 4) View configuration
# 5) Exit
```

**Advantages over CLI**:
- No need to memorize complex environment variables
- Input validation and error messages
- Interactive parameter input
- Better for manual testing and demos

### `run-e2e.sh`
**Purpose**: Quick E2E test entry point

**Usage**:
```bash
./run-e2e.sh --no-interactive
```

### `stop-subnet.sh`
**Purpose**: Stop all subnet processes

**Location**: `scripts/stop-subnet.sh`

**Usage**:
```bash
./scripts/stop-subnet.sh
```

---

## Testing Scripts

### `e2e-test.sh`
**Purpose**: End-to-end test with real RootLayer

**Features**:
- Complete intent submission and execution flow test
- Validates assignment and validation process
- Supports interactive and non-interactive modes

**Usage**:
```bash
# Interactive mode (prompts for confirmation)
./scripts/e2e-test.sh

# Non-interactive mode (direct execution)
./scripts/e2e-test.sh --no-interactive

# With environment variables
export SUBNET_ID="0x..."
export ROOTLAYER_GRPC="3.17.208.238:9001"
export ROOTLAYER_HTTP="http://3.17.208.238:8081"
./scripts/e2e-test.sh --no-interactive
```

### `simple-batch-test.sh`
**Purpose**: Batch submit multiple intents for stress testing

**Usage**:
```bash
./scripts/simple-batch-test.sh
```

### `e2e-test-multi-validator.sh`
**Purpose**: E2E test in multi-validator environment

**Usage**:
```bash
./scripts/e2e-test-multi-validator.sh
```

### `test-leader-rotation.sh`
**Purpose**: Test Raft leader rotation mechanism

**Usage**:
```bash
./scripts/test-leader-rotation.sh
```

### `run-integration-tests.sh`
**Purpose**: Run integration test suite (used by CI/CD)

**Usage**:
```bash
./scripts/run-integration-tests.sh
```

---

## Deployment Scripts

### `create-subnet.sh` + `create-subnet.go`
**Purpose**: Create a new subnet on the blockchain

**Note**:
- `.sh` is the wrapper script that compiles and runs the `.go` file
- `.go` contains the actual subnet creation logic

**Usage**:
```bash
# Create with default configuration
./scripts/create-subnet.sh

# Custom subnet name
./scripts/create-subnet.sh --name "Production Subnet"

# Manual approval mode
./scripts/create-subnet.sh --auto-approve false

# Specify config file
./scripts/create-subnet.sh --config /path/to/config.yaml

# Using environment variables
export NETWORK="base_sepolia"
export RPC_URL="https://sepolia.base.org"
export PRIVATE_KEY="0x..."
export SUBNET_NAME="My Subnet"
./scripts/create-subnet.sh
```

**Parameters**:
- `--config`: Config file path (default: `config/config.yaml`)
- `--network`: Network name (default: `base_sepolia`)
- `--rpc`: RPC URL (overrides config file)
- `--key`: Private key (overrides config file)
- `--name`: Subnet name (default: "My Test Subnet")
- `--auto-approve`: Auto-approve participants (default: true)

### `register-participants.go`
**Purpose**: Register validators/matchers on a subnet

**Usage**:
```bash
# Compile
go build -o bin/register-participants scripts/register-participants.go

# Register validator
./bin/register-participants --role validator --subnet 0x... --private-key 0x...

# Register matcher
./bin/register-participants --role matcher --subnet 0x... --private-key 0x...
```

### `register.sh`
**Purpose**: General-purpose registration script for validators, matchers, and agents

**Usage**:
```bash
# Register validator
./scripts/register.sh --subnet 0x... --key 0x... --domain validator1.pinai.local

# Register validator only (skip matcher and agent)
./scripts/register.sh --subnet 0x... --key 0x... --skip-matcher --skip-agent

# Using environment variables
export PIN_BASE_SEPOLIA_STAKING_MANAGER="0x7f887e88014e3AF57526B68b431bA16e6968C015"
export PIN_BASE_SEPOLIA_SUBNET_FACTORY="0x2b5D7032297Df52ADEd7020c3B825f048Cd2df3E"
./scripts/register.sh --subnet 0x... --key 0x...
```

### `register-validators.sh`
**Purpose**: Batch register multiple validators

**Usage**:
```bash
./scripts/register-validators.sh
```

---

## Utility Scripts

### `submit-intent-signed.go`
**Purpose**: Submit a signed intent

**Usage**:
```bash
# Compile
go build -o bin/submit-intent-signed scripts/submit-intent-signed.go

# Submit intent
PIN_BASE_SEPOLIA_INTENT_MANAGER="0xB2f092E696B33b7a95e1f961369Bb59611CAd093" \
RPC_URL="https://sepolia.base.org" \
PRIVATE_KEY="0x..." \
PIN_NETWORK="base_sepolia" \
ROOTLAYER_HTTP="http://3.17.208.238:8081/api/v1" \
SUBNET_ID="0x..." \
INTENT_TYPE="e2e-test" \
PARAMS_JSON='{"task":"test"}' \
AMOUNT_WEI="100000000000000" \
./bin/submit-intent-signed
```

### `derive-pubkey.go`
**Purpose**: Derive public key from private key

**Usage**:
```bash
# Compile
go build -o bin/derive-pubkey scripts/derive-pubkey.go

# Derive public key
./bin/derive-pubkey --private-key 0x...
```

### `registry-cli.sh`
**Purpose**: Registry service CLI tool

**Usage**:
```bash
# List all registered validators
./scripts/registry-cli.sh list-validators

# List all agents
./scripts/registry-cli.sh list-agents

# Query validator status
./scripts/registry-cli.sh get-validator validator-1
```

### `fund-validators.py`
**Purpose**: Fund validator accounts (for testing)

**Usage**:
```bash
python3 scripts/fund-validators.py
```

### `sync-proto.sh`
**Purpose**: Sync proto definitions from `../pin_protocol` and regenerate Go code

**Usage**:
```bash
./scripts/sync-proto.sh
```

**Note**: Equivalent to `make proto`

---

## CometBFT Scripts

The project supports both Raft and CometBFT consensus modes. The following scripts are for CometBFT configuration:

### `generate-cometbft-genesis.sh`
**Purpose**: Generate genesis.json for CometBFT

**Usage**:
```bash
./scripts/generate-cometbft-genesis.sh
```

### `generate-cometbft-keys.sh`
**Purpose**: Generate key pairs for CometBFT nodes

**Usage**:
```bash
./scripts/generate-cometbft-keys.sh
```

### `get-cometbft-node-id.sh`
**Purpose**: Get CometBFT node ID

**Usage**:
```bash
./scripts/get-cometbft-node-id.sh /path/to/node_key.json
```

---

## Script Relationships

```
Startup Flow:
  start-subnet.sh
    ├── bin/registry (service discovery)
    ├── bin/matcher (task distribution)
    ├── bin/validator (validators × N)
    └── bin/simple-agent (optional)

Deployment Flow:
  create-subnet.sh → create-subnet.go
    ↓
  register.sh / register-validators.sh → register-participants.go
    ↓
  start-subnet.sh

Testing Flow:
  e2e-test.sh / run-e2e.sh
    ↓
  submit-intent-signed.go
    ↓
  Verify matcher/validator/agent collaboration
```

---

## Environment Variables

All scripts support configuration via environment variables. We recommend using a `.env` file:

```bash
# 1. Copy config template
cp .env.example .env

# 2. Edit .env file and fill in required values

# 3. Load environment variables
set -a && source .env && set +a

# 4. Run scripts
./scripts/start-subnet.sh
```

**Required Variables**:
- `TEST_PRIVATE_KEY`: Test private key
- `VALIDATOR_KEYS`: Validator private keys (comma-separated)
- `SUBNET_ID`: Subnet ID
- `ROOTLAYER_GRPC`: RootLayer gRPC address
- `ROOTLAYER_HTTP`: RootLayer HTTP API address

**Optional Variables**:
- `CONSENSUS_TYPE`: Consensus type (raft/cometbft)
- `NUM_VALIDATORS`: Number of validators
- `TEST_MODE`: Test mode toggle
- `START_AGENT`: Whether to start agent
- `ENABLE_CHAIN_SUBMIT`: Whether to submit to blockchain
- And more...

See `.env.example` for details.

---

## Troubleshooting

### Common Issues

1. **Port in use**
   ```bash
   # Check port usage
   lsof -i :9001  # Registry
   lsof -i :9002  # Matcher
   lsof -i :9003  # Validator

   # Stop all services
   ./scripts/stop-subnet.sh
   ```

2. **Compilation errors**
   ```bash
   # Clean and rebuild
   make clean
   make build
   ```

3. **Environment variables not loaded**
   ```bash
   # Use correct loading method
   set -a && source .env && set +a

   # Verify variables
   echo $SUBNET_ID
   ```

4. **Proto definitions out of sync**
   ```bash
   # Re-sync proto definitions
   make proto
   ```

---

## Development Guidelines

### Adding New Scripts

1. **Naming Convention**:
   - Use lowercase with hyphens: `my-script.sh`
   - Go scripts: `my-script.go`

2. **File Location**:
   - All scripts in `scripts/` directory
   - Compiled binaries in `bin/` directory

3. **Documentation**:
   - Add purpose description in script header
   - Provide usage examples
   - Update this README

4. **Error Handling**:
   - Use `set -e` to exit on errors
   - Provide meaningful error messages
   - Log to `subnet-logs/` directory

5. **Environment Variables**:
   - Prefer `.env` file configuration
   - Provide reasonable defaults
   - Document all variables

---

## Related Documentation

- **Main Documentation**: `../README.md`
- **Architecture**: `../docs/architecture.md`
- **Quick Start**: `../docs/quick_start.md`
- **Environment Setup**: `../docs/environment_setup.md`
- **Deployment Guide**: `../docs/subnet_deployment_guide.md`
- **E2E Test Guide**: `../docs/e2e_test_guide.md`
- **Development Guide**: `../CLAUDE.md`

---

## Version History

- **2025-11-04**: Created scripts/README.md, cleaned up redundant scripts
- **2025-11-03**: Updated contract addresses to latest deployment
- **2025-10-29**: Removed CometBFT standalone startup scripts, unified to start-subnet.sh

---

**Last Updated**: 2025-11-04
