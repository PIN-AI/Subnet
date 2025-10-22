# Quick Start

This guide will help you quickly set up and run PinAI Subnet services for development and testing.

## Prerequisites

- **Go 1.21+** - [Install Go](https://go.dev/doc/install)
- **Git** - For cloning the repository

**Note**: Validators use **Raft consensus** and **Gossip protocol** for coordination - no external message broker required! You can run a single validator for development or multiple validators for production.

## Setup

### 1. Build Binaries

```bash
# Build all binaries
make build

# This creates:
# - bin/registry
# - bin/matcher
# - bin/validator
# - bin/simple-agent
```

### 2. Configure Environment

```bash
# Copy the example environment file
cp .env.example .env

# Edit .env and set your test private key
# IMPORTANT: Use a test-only key with NO real funds!
vi .env
```

Required environment variables in `.env`:

**Test Environment Example:**

```bash
# Test private key (REQUIRED)
# ⚠️ Use a test-only key with NO real funds!
TEST_PRIVATE_KEY=your_test_private_key_here

# Test Subnet configuration
SUBNET_ID=0x0000000000000000000000000000000000000000000000000000000000000002

# Test RootLayer endpoints
ROOTLAYER_GRPC=3.17.208.238:9001
ROOTLAYER_HTTP=http://3.17.208.238:8081

# Test blockchain settings (Base Sepolia testnet)
ENABLE_CHAIN_SUBMIT=true
CHAIN_RPC_URL=https://sepolia.base.org
CHAIN_NETWORK=base_sepolia

# Test contract addresses (Base Sepolia)
INTENT_MANAGER_ADDR=0xD04d23775D3B8e028e6104E31eb0F6c07206EB46
```

**For Production:**
Replace all values above with your production configuration. Use secure key management (KMS, Vault, etc.) instead of storing private keys in files.

## Start Services

### Option 1: One-Click Startup (Recommended)

```bash
# Start all services with one command
./start-subnet.sh
```

This script will:
- Start Registry service (gRPC: 8091, HTTP: 8101)
- Start Matcher service (gRPC: 8090)
- Start Validator service in **single-node Raft mode** (gRPC: 9200)
- Start a test agent for demonstration
- Generate necessary configuration files
- Save process IDs for easy management

The validator automatically bootstraps as a single-node Raft cluster for development. For production multi-validator setup, see [Deployment Guide](subnet_deployment_guide.md).

Logs are saved to `subnet-logs/` directory.

### Option 2: Manual Startup

```bash
# 1. Start Registry
./bin/registry -grpc ":8091" -http ":8101" > subnet-logs/registry.log 2>&1 &

# 2. Start Matcher (requires config file)
cat > /tmp/matcher-config.yaml <<EOF
subnet_id: "$SUBNET_ID"
identity:
  matcher_id: "matcher-main"
  subnet_id: "$SUBNET_ID"
rootlayer:
  grpc_endpoint: "$ROOTLAYER_GRPC"
  http_endpoint: "$ROOTLAYER_HTTP"
registry:
  grpc_address: "localhost:8091"
  http_address: "http://localhost:8101"
network:
  grpc_port: 8090
enable_chain_submit: true
chain_rpc_url: "$CHAIN_RPC_URL"
chain_network: "$CHAIN_NETWORK"
intent_manager_addr: "$INTENT_MANAGER_ADDR"
private_key: "$TEST_PRIVATE_KEY"
EOF

./bin/matcher --config /tmp/matcher-config.yaml > subnet-logs/matcher.log 2>&1 &

# 3. Start Validator (single-node Raft mode)
./bin/validator \
    -id "validator-main" \
    -grpc 9200 \
    -subnet-id "$SUBNET_ID" \
    -key "$TEST_PRIVATE_KEY" \
    -rootlayer-endpoint "$ROOTLAYER_GRPC" \
    -registry-grpc "localhost:8091" \
    -registry-http "localhost:8101" \
    -chain-rpc-url "$CHAIN_RPC_URL" \
    -chain-network "$CHAIN_NETWORK" \
    -intent-manager-addr "$INTENT_MANAGER_ADDR" \
    -enable-chain-submit \
    -enable-rootlayer \
    -validators 1 \
    -threshold-num 1 \
    -threshold-denom 1 \
    -raft-enable \
    -raft-bootstrap \
    -raft-bind "127.0.0.1:7400" \
    -raft-data-dir "./data/raft" \
    -raft-peers "validator-main:127.0.0.1:7400" \
    -gossip-enable \
    -gossip-bind "127.0.0.1" \
    -gossip-port 7950 \
    > subnet-logs/validator.log 2>&1 &

# For multi-validator setup, see subnet_deployment_guide.md
```

## Verify Services

Check that all services are running:

```bash
# Check processes
ps aux | grep -E 'registry|matcher|validator'

# Check Registry HTTP endpoint
curl http://localhost:8101/health

# Check logs
tail -f subnet-logs/registry.log
tail -f subnet-logs/matcher.log
tail -f subnet-logs/validator.log
```

You should see:
- Registry: "Registry service started on :8091 (gRPC) and :8101 (HTTP)"
- Matcher: "Matcher service started successfully"
- Validator: "Validator started, ID: validator-main"

## Send Test Intents

### Option 1: Interactive Script

```bash
./send-intent.sh
```

This provides an interactive menu:
1. Submit custom Intent
2. Submit E2E test Intent
3. Submit demo Intent
4. View configuration
5. Exit

### Option 2: Run E2E Test

```bash
# Full end-to-end test
./scripts/e2e-test.sh --no-interactive

# Or use the convenience script
./run-e2e.sh --no-interactive
```

The E2E test will:
1. Submit an Intent to RootLayer
2. Matcher fetches and assigns the Intent
3. Test agent executes the task
4. Validator validates the result
5. Validator submits ValidationBundle to RootLayer
6. Verify the receipt

### Option 3: Manual Intent Submission

```bash
# Using the submit-intent-signed tool
SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000002" \
ROOTLAYER_HTTP="http://3.17.208.238:8081/api/v1" \
INTENT_TYPE="test-intent" \
PARAMS_JSON='{"task":"My test task"}' \
AMOUNT_WEI="100000000000000" \
./bin/submit-intent-signed
```

## View Logs

```bash
# Follow all logs
tail -f subnet-logs/*.log

# View specific service logs
tail -f subnet-logs/registry.log
tail -f subnet-logs/matcher.log
tail -f subnet-logs/validator.log

# Search for errors
grep -i error subnet-logs/*.log
```

## Stop Services

### Option 1: Stop Script (Graceful)

```bash
./stop-subnet.sh
```

This will gracefully stop all services with SIGTERM first, then SIGKILL if needed.

### Option 2: Kill Processes

```bash
pkill -f 'bin/registry'
pkill -f 'bin/matcher'
pkill -f 'bin/validator'
```

### Option 3: Stop Individual Services

If using `start-subnet.sh`, it creates PID files:

```bash
# Stop individual services
kill $(cat subnet-logs/registry.pid)
kill $(cat subnet-logs/matcher.pid)
kill $(cat subnet-logs/validator.pid)
```

## Troubleshooting

### Services Won't Start

1. **Check port availability**:
   ```bash
   lsof -i :8090  # Matcher
   lsof -i :8091  # Registry gRPC
   lsof -i :8101  # Registry HTTP
   lsof -i :9200  # Validator gRPC
   lsof -i :7400  # Validator Raft
   lsof -i :7950  # Validator Gossip
   ```

3. **Check environment variables**:
   ```bash
   source .env
   echo $TEST_PRIVATE_KEY
   echo $SUBNET_ID
   ```

4. **Check logs for errors**:
   ```bash
   grep -i error subnet-logs/*.log
   ```

### Intent Submission Fails

1. **Check RootLayer connectivity**:
   ```bash
   curl http://3.17.208.238:8081/health
   nc -zv 3.17.208.238 9001
   ```

2. **Verify private key format**:
   - Should be hex without `0x` prefix in most places
   - Check `.env` file has correct format

3. **Check Matcher logs**:
   ```bash
   tail -f subnet-logs/matcher.log | grep -i intent
   ```

### Validator Not Processing Reports

1. **Check Raft consensus**:
   ```bash
   tail -f subnet-logs/validator.log | grep -i raft
   # Look for "entering leader state" or "Became Raft leader"
   ```

2. **Verify validator registered**:
   ```bash
   curl http://localhost:8101/validators
   ```

3. **Check consensus and gossip state**:
   ```bash
   tail -f subnet-logs/validator.log | grep -E "consensus|gossip|checkpoint"
   ```

## Next Steps

- Read the [Architecture Overview](ARCHITECTURE_OVERVIEW.md) to understand the system design
- See [Production Deployment](production_deployment.md) for production setup
- Explore [Testing Guide](testing_guide.md) for comprehensive testing
- Review [API Documentation](api_reference.md) for integration

## Development Workflow

```bash
# 1. Make code changes
vi internal/matcher/server.go

# 2. Rebuild binaries
make build

# 3. Stop services
./stop-subnet.sh

# 4. Restart services
./start-subnet.sh

# 5. Test changes
./send-intent.sh

# 6. Check logs
tail -f subnet-logs/*.log
```

## Common Development Commands

```bash
# Run tests
make test

# Run with race detector
go test -race ./...

# Generate protobuf code
make proto

# Format code
go fmt ./...
gofmt -w .

# Lint code
golangci-lint run

# Clean build artifacts
make clean
```
