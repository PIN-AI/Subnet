# Scripts Guide

This directory contains utility scripts for Subnet development, testing, and deployment.

## Overview

The `scripts/` directory provides essential tools for working with PinAI Subnet:

- **Blockchain Operations**: Create subnets and register participants on-chain
- **Testing & Validation**: End-to-end testing and test agents
- **Utilities**: Key derivation and intent submission tools

## Scripts Reference

### 1. create-subnet.sh

**Purpose**: Creates a new subnet on the blockchain

**Location**: `scripts/create-subnet.sh` + `scripts/create-subnet.go`

**Description**:
This script deploys a new subnet to the blockchain using the intent-protocol-contract-sdk. It handles subnet factory interaction, stake governance configuration, and saves subnet information for later use.

**Usage**:
```bash
# Basic usage with defaults
./scripts/create-subnet.sh

# Custom subnet name
./scripts/create-subnet.sh --name "Production Subnet"

# With manual participant approval
./scripts/create-subnet.sh --auto-approve false

# Full configuration
./scripts/create-subnet.sh \
  --rpc https://sepolia.base.org \
  --key 0xYOUR_PRIVATE_KEY \
  --name "My Test Subnet" \
  --auto-approve true
```

**Options**:
- `--config FILE` - Path to config file (default: config/blockchain.yaml)
- `--network NAME` - Network name (default: base_sepolia)
- `--rpc URL` - RPC URL (overrides config)
- `--key HEX` - Private key hex (overrides config)
- `--name NAME` - Subnet name (default: "My Test Subnet")
- `--auto-approve BOOL` - Auto approve participants (default: true)
- `--help` - Show help message

**Environment Variables**:
- `NETWORK` - Network name
- `RPC_URL` - RPC URL
- `PRIVATE_KEY` - Private key hex
- `SUBNET_NAME` - Subnet name

**Output**:
- Creates subnet on blockchain
- Displays subnet ID and contract address
- Saves configuration for validator/matcher setup

---

### 2. register.sh

**Purpose**: Registers Validator, Matcher, and Agent on a subnet

**Location**: `scripts/register.sh` + `scripts/register-participants.go`

**Description**:
This script handles participant registration on an existing subnet. It supports registering all three participant types (Validator, Matcher, Agent) with configurable stake amounts, endpoints, and metadata. The script can check current registration status, perform dry runs, and selectively register specific participant types.

**Usage**:
```bash
# Register all participants using config file
./scripts/register.sh

# Check registration status only
./scripts/register.sh --check

# Register with custom parameters
./scripts/register.sh \
  --rpc https://sepolia.base.org \
  --subnet 0x123... \
  --key 0xabc... \
  --domain my-subnet.com

# Dry run to see what would happen
./scripts/register.sh --dry-run

# Register only validator
./scripts/register.sh --skip-matcher --skip-agent
```

**Options**:
- `--config FILE` - Path to config file (default: config/blockchain.yaml)
- `--network NAME` - Network name (default: base_sepolia)
- `--rpc URL` - RPC URL (overrides config)
- `--subnet ADDRESS` - Subnet contract address (overrides config)
- `--key HEX` - Private key hex (overrides config)
- `--domain DOMAIN` - Participant domain (default: subnet.example.com)
- `--validator-port PORT` - Validator endpoint port (default: 9090)
- `--matcher-port PORT` - Matcher endpoint port (default: 8090)
- `--agent-port PORT` - Agent endpoint port (default: 7070)
- `--validator-stake AMOUNT` - Validator stake in ETH (default: 0.1)
- `--matcher-stake AMOUNT` - Matcher stake in ETH (default: 0.05)
- `--agent-stake AMOUNT` - Agent stake in ETH (default: 0.05)
- `--check` - Only check registration status
- `--dry-run` - Dry run (don't submit transactions)
- `--skip-validator` - Skip validator registration
- `--skip-matcher` - Skip matcher registration
- `--skip-agent` - Skip agent registration
- `--erc20` - Use ERC20 staking instead of ETH
- `--metadata URI` - Metadata URI (optional)

**Features**:
- Queries subnet info and stake requirements
- Checks current registration status for each participant type
- Validates stake amounts against subnet minimums
- Supports both ETH and ERC20 staking
- Displays transaction hashes and confirmation status
- Auto-approves or requires owner approval based on subnet config

**Example Output**:
```
🚀 Starting participant registration script
   Network: base_sepolia
   Subnet Contract: 0x123...
   Signer Address: 0xabc...
   Balance: 1.234567 ETH

📊 Stake Requirements:
   Min Validator Stake: 0.100000 ETH
   Min Matcher Stake: 0.050000 ETH
   Min Agent Stake: 0.050000 ETH
   Auto Approve: true

🔍 Checking current registration status...
   ❌ Not registered as Validator
   ❌ Not registered as Matcher
   ❌ Not registered as Agent

📝 Registering as Validator...
   Domain: subnet.example.com
   Endpoint: https://subnet.example.com:9090
   Stake: 0.100000 ETH
   📤 Transaction submitted: 0xtx123...
   ✅ Validator registration completed

[... similar for Matcher and Agent ...]

🎉 Registration process completed!
```

---

### 3. derive-pubkey.go

**Purpose**: Derives public key from Ethereum private key

**Location**: `scripts/derive-pubkey.go`

**Description**:
Simple utility to derive the uncompressed public key from an Ethereum private key. Useful for validator configuration and debugging authentication issues.

**Usage**:
```bash
# Compile and run
go run scripts/derive-pubkey.go <private_key_hex>

# Or build first
go build -o bin/derive-pubkey scripts/derive-pubkey.go
./bin/derive-pubkey 0xYOUR_PRIVATE_KEY
```

**Example**:
```bash
$ go run scripts/derive-pubkey.go 0xabc123...
0482ea12c5481d481c7f9d7c1a2047401c6e2f855e4cee4d8df0aa197514f3456528ba6c55092b20b51478fd8cf62cde37f206621b3dd47c2be3d5c35e4889bf94
```

**Output Format**:
Returns the uncompressed public key in hexadecimal format (130 characters, 65 bytes).

---

### 5. submit-intent-signed.go

**Purpose**: Submits signed Intents with dual submission (blockchain + RootLayer HTTP)

**Location**: `scripts/submit-intent-signed.go`

**Description**:
Advanced intent submission tool that implements proper EIP-191 signature creation and dual submission strategy. It first submits the Intent to the blockchain (IntentManager contract), then submits the same Intent to RootLayer HTTP API with signature verification. This ensures both on-chain record and fast off-chain propagation.

**Usage**:
```bash
# Build the tool
go build -o bin/submit-intent-signed scripts/submit-intent-signed.go

# Submit with full configuration via environment variables
export PIN_BASE_SEPOLIA_INTENT_MANAGER="0xB2f092E696B33b7a95e1f961369Bb59611CAd093"
export RPC_URL="https://sepolia.base.org"
export PRIVATE_KEY="0xYOUR_PRIVATE_KEY"
export PIN_NETWORK="base_sepolia"
export ROOTLAYER_HTTP="http://3.17.208.238:8081/api/v1"
export SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000002"
export INTENT_TYPE="my-task"
export PARAMS_JSON='{"task":"Process this data","priority":"high"}'
export AMOUNT_WEI="100000000000000"

./bin/submit-intent-signed
```

**Environment Variables** (all required):
- `PIN_BASE_SEPOLIA_INTENT_MANAGER` - IntentManager contract address
- `RPC_URL` - Blockchain RPC URL
- `PRIVATE_KEY` - Submitter's private key (0x prefix optional)
- `PIN_NETWORK` - Network name (base_sepolia, base, local)
- `ROOTLAYER_HTTP` - RootLayer HTTP API base URL
- `SUBNET_ID` - Target subnet ID (32 bytes hex)
- `INTENT_TYPE` - Intent type identifier
- `PARAMS_JSON` - Intent parameters as JSON string
- `AMOUNT_WEI` - Budget amount in Wei
- `DEADLINE_HOURS` - Deadline offset from now (default 1 hour)

**Features**:
- **EIP-191 Signature**: Creates proper Ethereum signed messages
- **Dual Submission**: Blockchain transaction + RootLayer HTTP API
- **Local Verification**: Verifies signature locally before submission
- **Signature Format**: Uses base64url encoding without padding
- **Proper Hashing**: Keccak256(PARAMS_JSON) for signature
- **Transaction Tracking**: Returns both blockchain TX hash and Intent ID

**Example Output**:
```
🚀 Submitting Intent with dual submission (blockchain + RootLayer)

📋 Intent Details:
   Subnet ID: 0x0000...0002
   Intent Type: e2e-test
   Budget: 100000000000000 wei (0.0001 ETH)
   RootLayer HTTP: http://3.17.208.238:8081/api/v1

🔐 Creating EIP-191 signature...
   Submitter: 0xfc5A111b714547fc2D1D796EAAbb68264ed4A132
   Params Hash: 0x5f3d8aa4...
   Signature: vyEpmvTKK-Bhze...

✓ Local signature verification passed

📤 Step 1: Submitting to blockchain (IntentManager)...
   Contract: 0xB2f092E696B33b7a95e1f961369Bb59611CAd093
   Transaction: 0x7b3f9e1d...
   ⏳ Waiting for confirmation...
   ✅ Blockchain transaction confirmed!

📤 Step 2: Submitting to RootLayer HTTP API...
   Endpoint: http://3.17.208.238:8081/api/v1/intents
   ✅ RootLayer accepted Intent

✅ Dual submission completed successfully!
   Intent ID: intent_abc123...
   Blockchain TX: 0x7b3f9e1d...

Intent will be:
1. Recorded on-chain in IntentManager contract
2. Available to Matchers via RootLayer gRPC
```

**Signature Details**:
- Message format: `0x19Ethereum Signed Message:\n32` + keccak256(PARAMS_JSON)
- Encoding: base64url without padding (RFC 4648 Section 5)
- Recovery: V value adjusted for Ethereum (27/28)

---

### 6. test-agent/

**Purpose**: Test agent implementation for E2E testing

**Location**: `scripts/test-agent/`

**Description**:
Contains a simple test agent (`validator_test_agent.go`) that implements the full agent workflow:
- Registers with Matcher
- Submits bids for Intents
- Receives Assignments
- Executes tasks (simulated)
- Submits ExecutionReports to Validators
- Handles receipts

Use this agent to drive local/manual testing flows (for example, after `start-subnet.sh` brings up matcher and validators).

**Files**:
- `validator_test_agent.go` - Main test agent implementation
- `test-agent` - Compiled binary (build manually as needed)

**Usage**:
```bash
cd scripts/test-agent
go build -o test-agent validator_test_agent.go

./test-agent \
  --agent-id "test-agent-001" \
  --matcher "localhost:8090" \
  --validator "localhost:9200" \
  --subnet-id "0x0000000000000000000000000000000000000000000000000000000000000003"
```

---

### 7. intent-test/

**Purpose**: Legacy intent testing utilities

**Location**: `scripts/intent-test/`

**Description**:
Contains older intent submission and testing utilities. Most functionality has been superseded by `submit-intent-signed.go` and the E2E test script.

**Files**:
- Various test scripts and utilities for intent submission

**Status**: Legacy - prefer `submit-intent-signed.go` together with `start-subnet.sh` for current testing workflows

---

### 9. register-validators.sh

**Purpose**: Batch register multiple validators to a subnet

**Location**: `scripts/register-validators.sh`

**Description**:
Automates the process of registering multiple validators from a JSON file. This script reads validator information (addresses and private keys) and registers each one to the specified subnet contract with stake and endpoint configuration.

**Usage**:
```bash
# Set required environment variables
export RPC_URL="https://sepolia.base.org"
export PIN_NETWORK="base_sepolia"
export SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000003"
export SUBNET_CONTRACT="0x5697DFA452a8cA1598a9CA736b3E9E75dA1a43A6"
export PIN_BASE_SEPOLIA_STAKING_MANAGER="0xAc11AE66c7831A70Bea940b0AE16c967f940cB65"

# Optional configuration
export VALIDATOR_STAKE="0.001"  # Default stake amount in ETH
export BASE_PORT="9090"          # Base port (validator 1 = 9090, validator 2 = 9091, etc.)
export DOMAIN_SUFFIX="test.pinai.xyz"  # Domain suffix for validator endpoints

# Run registration
./scripts/register-validators.sh validators.json
```

**Validators JSON Format**:
```json
[
  {
    "id": "validator-1",
    "address": "0x3d5f11B94f1B83fC3dbB9f37dE33CEb978186FED",
    "private_key": "0xabc123..."
  },
  {
    "id": "validator-2",
    "address": "0x4d3A18617613baC4767accC5350Ff6E57c3a3efc",
    "private_key": "0xdef456..."
  }
]
```

**Required Environment Variables**:
- `RPC_URL` - Blockchain RPC endpoint
- `PIN_NETWORK` - Network name (base_sepolia, base, etc.)
- `SUBNET_ID` - Target subnet ID
- `SUBNET_CONTRACT` - Subnet contract address
- `PIN_BASE_SEPOLIA_STAKING_MANAGER` - StakingManager contract address

**Optional Environment Variables**:
- `VALIDATOR_STAKE` - Stake amount in ETH (default: 0.001)
- `BASE_PORT` - Base port for validator endpoints (default: 9090)
- `DOMAIN_SUFFIX` - Domain suffix for endpoints (default: test.pinai.xyz)

**Features**:
- Automatic validator count detection
- Sequential registration with confirmation
- Error handling for each validator
- Configurable stake and port assignments
- Progress tracking and status display

**Example Output**:
```
🚀 Registering validators to Subnet:
   Network: base_sepolia
   RPC: https://sepolia.base.org
   Subnet ID: 0x0000...0009
   Subnet Contract: 0x5697...
   Stake: 0.001 ETH

📋 Found 3 validators in file

Validators to register:
  1. 0x3d5f11B94f1B83fC3dbB9f37dE33CEb978186FED
  2. 0x4d3A18617613baC4767accC5350Ff6E57c3a3efc
  3. 0x734e8FaeF1E5d07E9357718748aC7cdBcfdE9561

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📝 Registering validator 1/3...
   Address: 0x3d5f11B94f1B83fC3dbB9f37dE33CEb978186FED
   ✅ Validator 1 registered successfully!

[... continues for each validator ...]

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
✅ All 3 validators registered successfully!
```

---

### 10. fund-validators.py

**Purpose**: Fund validator accounts with ETH for gas fees

**Location**: `scripts/fund-validators.py`

**Description**:
Python utility to batch transfer ETH to multiple validator addresses. Useful for setting up test validators on testnets like Base Sepolia.

**Usage**:
```bash
# Set environment variables
export PRIVATE_KEY="0xYOUR_FUNDING_ACCOUNT_PRIVATE_KEY"  # Account with ETH
export RPC_URL="https://sepolia.base.org"  # Optional, defaults to Base Sepolia

# Run funding script
./scripts/fund-validators.py validators.json 0.01

# Or use default amount (0.01 ETH)
./scripts/fund-validators.py validators.json
```

**Arguments**:
1. `validators_json_file` - Path to JSON file with validator addresses (required)
2. `amount_eth` - Amount of ETH to send to each validator (optional, default: 0.01)

**Required Environment Variables**:
- `PRIVATE_KEY` or `TEST_PRIVATE_KEY` - Private key of funding account

**Optional Environment Variables**:
- `RPC_URL` - RPC endpoint (default: https://sepolia.base.org)

**Features**:
- Batch ETH transfers to multiple addresses
- Automatic nonce management
- Transaction confirmation tracking
- Balance verification before and after
- Support for both web3.py v5 and v6

**Example Output**:
```
✅ Connected to https://sepolia.base.org
📊 Main account: 0xfc5A111b714547fc2D1D796EAAbb68264ed4A132
💰 Balance: 0.5 ETH
⛽ Gas price: 0.5 gwei

🚀 Sending 0.01 ETH to each validator...
  ✅ validator-1: 0x3d5f11B94f1B83fC3dbB9f37dE33CEb978186FED
     TX: 0xabc123...
  ✅ validator-2: 0x4d3A18617613baC4767accC5350Ff6E57c3a3efc
     TX: 0xdef456...

⏳ Waiting for confirmations...
  ✅ validator-1: Confirmed (block 12345678)
  ✅ validator-2: Confirmed (block 12345679)

✅ Funding complete!

📊 Final validator balances:
  validator-1: 0.01 ETH
  validator-2: 0.01 ETH
```

**Dependencies**:
- `web3` - Ethereum Python library
- `eth-account` - Ethereum account utilities

**Install**:
```bash
pip install web3 eth-account
```

---

## Common Workflows

### Initial Subnet Setup

1. **Create a Subnet**:
   ```bash
   ./scripts/create-subnet.sh --name "My Subnet" --auto-approve true
   # Note the Subnet ID from output
   ```

2. **Register Participants**:
   ```bash
   export SUBNET_CONTRACT="0xYOUR_SUBNET_ADDRESS"
   ./scripts/register.sh --subnet $SUBNET_CONTRACT
   ```

3. **Verify Registration**:
   ```bash
   ./scripts/register.sh --check
   ```

### Development & Testing

1. **Build All Components**:
   ```bash
   make build
   ```

2. **Run Local Subnet**:
```bash
export SUBNET_ID="0xYOUR_SUBNET_ID"
export ROOTLAYER_GRPC="localhost:9001"
export ROOTLAYER_HTTP="http://localhost:8080"
./scripts/start-subnet.sh
```

3. **Monitor Services**:
```bash
# In separate terminals
tail -f subnet-logs/matcher.log
tail -f subnet-logs/validator-1.log
tail -f subnet-logs/agent.log
```

### Production Deployment

1. **Submit Production Intent**:
   ```bash
   export SUBNET_ID="0xPROD_SUBNET_ID"
   export INTENT_TYPE="production-task"
   export PARAMS_JSON='{"task":"Real workload","priority":"high"}'
   ./bin/submit-intent-signed
   ```

---

## Environment Variables Reference

### Common Variables

- `RPC_URL` - Blockchain RPC endpoint
- `PRIVATE_KEY` - Ethereum private key (with or without 0x prefix)
- `SUBNET_ID` - Target subnet ID (32 bytes, 0x-prefixed)
- `PIN_NETWORK` - Network name (base_sepolia, base, local)

### RootLayer Configuration

- `ROOTLAYER_GRPC` - RootLayer gRPC endpoint (host:port)
- `ROOTLAYER_HTTP` - RootLayer HTTP API base URL

### Blockchain Contracts

- `PIN_BASE_SEPOLIA_INTENT_MANAGER` - IntentManager contract address
- `PIN_BASE_SEPOLIA_SUBNET_FACTORY` - SubnetFactory contract address
- `PIN_BASE_SEPOLIA_STAKING_MANAGER` - StakingManager contract address
- `PIN_BASE_SEPOLIA_CHECKPOINT_MANAGER` - CheckpointManager contract address

### Local Test Configuration

- `ENABLE_CHAIN_SUBMIT` - Enable blockchain submission (true/false)
- `CHAIN_RPC_URL` - Blockchain RPC used by local tests
- `CHAIN_NETWORK` - Network name for local tests
- `MATCHER_PRIVATE_KEY` - Private key for the test matcher (DO NOT use in production!)

---

## Security Notes

### Private Keys

- **Never commit private keys** to version control
- Use environment variables or secure config management
- For testing, use dedicated test accounts with minimal funds
- `MATCHER_PRIVATE_KEY` in local testing flows is for development ONLY

### Configuration Files

- Config files may contain sensitive data (private keys)
- Use restrictive file permissions: `chmod 600 config/*.yaml`
- Validator configuration uses hierarchical system: `subnet.yaml` + `validator-N.yaml`
- Blockchain operations use: `blockchain.yaml`
- Consider using environment variables instead of config files for production

---

## Troubleshooting

### Script Fails to Build

**Problem**: `Failed to build X script`

**Solution**:
```bash
# Ensure Go is installed and in PATH
go version

# Update dependencies
go mod tidy

# Try building manually
go build -o bin/X scripts/X.go
```

### E2E Test Services Won't Start

**Problem**: Ports already in use

**Solution**:
```bash
# Check what's using the ports
lsof -i :8090  # Matcher
lsof -i :9200  # Validator

# Kill old processes
pkill -f "bin/matcher"
pkill -f "bin/validator"

# Or adjust ports/flags in scripts/start-subnet.sh
```

### Intent Submission Fails

**Problem**: Signature verification fails

**Solution**:
```bash
# Verify params JSON is valid
echo $PARAMS_JSON | jq .

# Check signature creation with verbose output
# submit-intent-signed shows signature details

# Verify private key format
# Should be 64 hex characters (with optional 0x prefix)
```

### ValidationBundle Not Submitted

**Problem**: Local run completes but no ValidationBundle

**Solution**:
- Wait longer - ValidationBundle submission happens in next checkpoint after ExecutionReport arrives
- Check validator logs: `grep -i "validation bundle" subnet-logs/validator-*.log`
- Verify RootLayer is accessible
- Check if ENABLE_CHAIN_SUBMIT is set correctly

---

## Additional Resources

- **Subnet Deployment Guide**: `docs/subnet_deployment_guide.md`
- **Registration Guide**: `scripts/REGISTRATION_GUIDE.md`
- **Architecture Overview**: `docs/ARCHITECTURE_OVERVIEW.md`
- **API Documentation**: See proto definitions in `proto/`

---

## Contributing

When adding new scripts:

1. Follow the naming convention: `kebab-case.sh` for shell scripts, `kebab-case.go` for Go
2. Add comprehensive `--help` documentation
3. Support both CLI flags and environment variables
4. Include examples in this guide
5. Add error handling and user-friendly messages
6. Use the color output helpers for consistency
