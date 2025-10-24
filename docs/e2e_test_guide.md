# E2E Test Guide - Complete Intent Flow with Dual Submission

## 🚀 Quick Start

Test the complete Intent flow with blockchain dual submission in one command!

```bash
cd /Users/ty/pinai/protocol/Subnet

# Build all binaries
make build

# Run E2E test with blockchain dual submission
SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000002" \
ROOTLAYER_GRPC="3.17.208.238:9001" \
ROOTLAYER_HTTP="http://3.17.208.238:8081" \
ENABLE_CHAIN_SUBMIT=true \
./scripts/e2e-test.sh --no-interactive
```

---

## 📋 Complete Test Flow

The E2E test covers the entire Intent lifecycle with **dual submission** (blockchain + RootLayer):

```
1. Intent Submission (Dual Submission)
   ├─ Submit to Blockchain (IntentManager contract)
   └─ Submit to RootLayer (for distribution)
   ↓
2. Matcher pulls Intent from RootLayer and creates Bidding Window
   ↓
3. Agent subscribes to Intent stream and submits Bid
   ↓
4. Matcher performs matching and creates Assignment
   ├─ Submit Assignment to Blockchain
   └─ Submit Assignment to RootLayer (fails - known RootLayer API issue)
   ↓
5. Agent receives Assignment and executes task
   ↓
6. Agent submits ExecutionReport to Validator
   ↓
7. Validator validates report and returns Receipt
   ↓
8. Validator broadcasts ExecutionReport to all validators via NATS
   ↓
9. Validator creates Checkpoint with pending reports
   ↓
10. Validators collect signatures (threshold consensus)
   ↓
11. ValidationBundle construction with EIP-191 signatures
   ├─ Uses SDK to compute proper digest (10 fields)
   ├─ Signs with EIP-191 standard (65-byte signature)
   └─ Submits to RootLayer (fails - known RootLayer API issue)
```

---

## ✨ Key Features

### 1. **Dual Submission Pattern**
Both Intent and Assignment are submitted to:
- **Blockchain first** (immutable record on Base Sepolia)
- **RootLayer second** (for distribution to Subnet network)

### 2. **EIP-191 Signature Standard**
All signatures use Ethereum's EIP-191 standard:
- **Intent**: Signed with requester's private key
- **Assignment**: Signed by Matcher
- **ValidationBundle**: Signed by Validator(s)

### 3. **SDK Integration**
Uses `intent-protocol-contract-sdk` for:
- Computing proper message digests
- Generating EIP-191 signatures
- Blockchain transaction submission
- Signature verification

---

## 🎯 Services Auto-Started

| Service | Port | Purpose |
|---------|------|---------|
| **NATS** | 4222 | Message bus (auto-started with Docker if not running) |
| **Matcher** | 8090 | Intent matching service with bidding windows |
| **Validator 1** | 9200 | Validation node (leader for epoch 0) |
| **Test Agent** | - | Demo agent that bids and executes tasks |

---

## 🔑 Environment Variables

### Required for Blockchain Submission

```bash
export SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000002"
export ROOTLAYER_GRPC="3.17.208.238:9001"
export ROOTLAYER_HTTP="http://3.17.208.238:8081"
export ENABLE_CHAIN_SUBMIT=true

# Blockchain configuration (already set in script)
export CHAIN_RPC_URL="https://sepolia.base.org"
export CHAIN_NETWORK="base_sepolia"
export INTENT_MANAGER_ADDR="0xD04d23775D3B8e028e6104E31eb0F6c07206EB46"
export MATCHER_PRIVATE_KEY="xxxxxxxxxxxxxxxxxxxxxxxxx"
```

---

## 📊 Test Results

After running the test, you'll see:

```
================================
Test Results Summary
================================

Service Status:
  RootLayer:      3.17.208.238:9001 (external)
  Matcher:        localhost:8090
  Validator 1:    localhost:9200

Log Files:
  Matcher:        /Users/ty/pinai/protocol/Subnet/e2e-test-logs/matcher.log
  Validator 1:    /Users/ty/pinai/protocol/Subnet/e2e-test-logs/validator-1.log
  Test Agent:     /Users/ty/pinai/protocol/Subnet/e2e-test-logs/test-agent.log

Quick Stats:
  Intents Received: 1
  Reports:        1
  Checkpoints:    1
```

---

## ✅ Success Indicators

### 1. Intent Dual Submission
Check `e2e-test-logs/matcher.log`:
```
✅ Intent submitted successfully via dual submission!
Intent ID: 0x...
EIP-191 signature verified locally
```

### 2. Assignment Blockchain Submission
Check `e2e-test-logs/matcher.log`:
```
⏳ Submitting assignment 0x... to blockchain...
📝 Assignment transaction sent, hash: 0x...
✅ Assignment submitted to blockchain
```

### 3. Validator SDK Initialization
Check `e2e-test-logs/validator-1.log`:
```
SDK client initialized for ValidationBundle signing
rpc_url=https://sepolia.base.org
network=base_sepolia
intent_manager=0xD04d23775D3B8e028e6104E31eb0F6c07206EB46
```

### 4. ValidationBundle EIP-191 Signature
Check `e2e-test-logs/validator-1.log`:
```
✅ Generated EIP-191 ValidationBundle signature
validator=0xfc5A111b714547fc2D1D796EAAbb68264ed4A132
signature_len=65
```

### 5. ValidationBundle Construction
Check `e2e-test-logs/validator-1.log`:
```
✅ ValidationBundle constructed successfully
epoch=0
intent_id=0x...
assignment_id=0x...
signatures=1
```

---

## 🔧 Command-Line Options

```bash
./scripts/e2e-test.sh [OPTIONS]

Options:
  --rootlayer-grpc <addr>    RootLayer gRPC endpoint (default: 3.17.208.238:9001)
  --rootlayer-http <url>     RootLayer HTTP endpoint (default: http://3.17.208.238:8081)
  --no-interactive           Exit after test completes (no interactive mode)
  --help, -h                 Show help message
```

### Environment Variable Configuration

Environment variables override defaults:
```bash
export ROOTLAYER_GRPC="3.17.208.238:9001"
export ROOTLAYER_HTTP="http://3.17.208.238:8081"
export SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000002"
export ENABLE_CHAIN_SUBMIT=true
./scripts/e2e-test.sh
```

Priority: Command-line args > Environment variables > Defaults

---

## 🧪 Manual Intent Submission

To manually submit an Intent with proper dual submission and EIP-191 signature:

```bash
cd /Users/ty/pinai/protocol/Subnet

# Build the submit-intent-signed tool
cd scripts && go build -o submit-intent-signed submit-intent-signed.go

# Submit Intent with dual submission
PIN_BASE_SEPOLIA_INTENT_MANAGER="0xD04d23775D3B8e028e6104E31eb0F6c07206EB46" \
RPC_URL="https://sepolia.base.org" \
PRIVATE_KEY="eef960cc05e034727123db159f1e54f0733b2f51d4a239978771aff320be5b9a" \
PIN_NETWORK="base_sepolia" \
ROOTLAYER_HTTP="http://3.17.208.238:8081/api/v1" \
SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000002" \
INTENT_TYPE="e2e-test" \
PARAMS_JSON='{"task":"Manual test with dual submission"}' \
AMOUNT_WEI="100000000000000" \
./submit-intent-signed
```

This will:
1. Generate a random Intent ID
2. Compute EIP-191 signature using SDK
3. Submit to blockchain (IntentManager contract)
4. Wait for transaction confirmation
5. Submit to RootLayer for distribution

---

## 🐛 Troubleshooting

### Issue 1: SDK Initialization Failed

**Error:**
```
Failed to initialize SDK client: ...
```

**Solution:**
Check environment variables:
```bash
echo $CHAIN_RPC_URL
echo $CHAIN_NETWORK
echo $INTENT_MANAGER_ADDR
```

Ensure they match Base Sepolia testnet configuration.

### Issue 2: Transaction Failed on Blockchain

**Error:**
```
Transaction failed on-chain (status=0)
```

**Solution:**
- Check you have enough ETH on Base Sepolia
- Verify IntentManager contract address is correct
- Check Base Sepolia RPC is responding

### Issue 3: ValidationBundle Submission - "Invalid intent status"

**Error:**
```
Failed to submit validation bundle: Invalid intent status: intent status %s is not PROCESSING
```

**Root Cause:**
After Assignment is submitted to blockchain, RootLayer needs 10-30 seconds to synchronize the Intent status. If ValidationBundle is submitted too early, RootLayer will reject it.

**Solution (Automatic):**
The validator now implements automatic retry logic:
1. **15-second initial delay**: Waits for RootLayer state synchronization after Assignment submission
2. **3 retry attempts**: If submission fails due to Intent status, retries with 10-second intervals
3. **Total wait time**: Up to 45 seconds (15s initial + 3×10s retries)

Check `e2e-test-logs/validator-1.log` for retry logs:
```
⏳ Waiting 15s for RootLayer state synchronization before submitting ValidationBundle
Submitting ValidationBundle to RootLayer attempt=1/3
⚠️ ValidationBundle submission failed due to Intent status not ready, will retry in 10s attempt=1/3
✅ Successfully submitted validation bundle to RootLayer attempt=2
```

### Issue 4: Assignment Submission to RootLayer

**Known Issue:**
```
Failed to submit assignment to RootLayer: invalid agent_id: %v
```

This is a **known RootLayer API issue**. Assignment is successfully submitted to blockchain, but RootLayer HTTP API rejects it due to agent_id format validation. This doesn't affect the E2E flow since validators can proceed with ValidationBundle submission.

### Issue 5: NATS Not Running

**Error:**
```
✗ NATS is not running on port 4222
```

**Solution:**
```bash
# Option 1: Docker (script auto-attempts)
docker run -d --name nats-e2e-test -p 4222:4222 nats:latest

# Option 2: Native install
brew install nats-server
nats-server

# Option 3: Use existing NATS
# Just ensure it's running on port 4222
```

### Issue 6: Port Already in Use

**Error:**
```
Failed to start on port 8090
```

**Solution:**
```bash
# Find and kill processes
lsof -i :8090
kill <PID>

# Or clean up all test services
pkill -f "bin/matcher"
pkill -f "bin/validator"
pkill -f "test-agent"
```

---

## 📝 Viewing Logs

All logs are saved in `e2e-test-logs/`:

```bash
# Real-time Matcher logs
tail -f e2e-test-logs/matcher.log

# Real-time Validator logs
tail -f e2e-test-logs/validator-1.log

# Real-time Agent logs
tail -f e2e-test-logs/test-agent.log

# Search for blockchain transactions
grep "transaction sent" e2e-test-logs/matcher.log

# Search for EIP-191 signatures
grep "EIP-191" e2e-test-logs/validator-1.log

# Search for ValidationBundle
grep "ValidationBundle" e2e-test-logs/validator-1.log
```

---

## 🎯 Success Criteria

A complete successful E2E test should have:

- [x] Intent submitted to blockchain (TX hash logged)
- [x] Intent submitted to RootLayer (HTTP 200 response)
- [x] EIP-191 signature verification passed locally
- [x] Matcher receives Intent from RootLayer
- [x] Test Agent submits bid successfully
- [x] Assignment created and submitted to blockchain
- [x] Agent receives assignment and executes task
- [x] ExecutionReport submitted to Validator
- [x] Validator processes report and broadcasts
- [x] Checkpoint created (epoch 0)
- [x] ValidationBundle constructed with EIP-191 signature (65 bytes)
- [x] No critical errors in logs (RootLayer API errors are expected)

---

## 📚 Related Documentation

- **Architecture Overview**: `docs/architecture.md`
- **Signature Guide**: `../pin_protocol/signature-guide.md` (sections 1-3)
- **SDK Documentation**: `../intent-protocol-contract-sdk/README.md`
- **Coding Assistant Guide**: `CLAUDE.md`

---

## 🔍 Monitoring Blockchain Transactions

### View on Base Sepolia Explorer

After submitting Intent/Assignment, you can view transactions on:

```
https://sepolia.basescan.org/tx/<transaction_hash>
```

Replace `<transaction_hash>` with the hash from logs.

### Check IntentManager Contract

View the IntentManager contract state:

```
https://sepolia.basescan.org/address/0xD04d23775D3B8e028e6104E31eb0F6c07206EB46
```

---

## 💡 Tips

### Speed Up Testing

Reduce bidding window and checkpoint interval:
```bash
# Matcher bidding window: 10s → 5s
# (modify in e2e-test.sh line 272: bidding_window_sec)

# Validator checkpoint interval: 10s → 5s
# (modify in e2e-test.sh line 343: checkpoint_interval)
```

### Test Without Blockchain

To test without blockchain submission (useful for testing consensus only):
```bash
ENABLE_CHAIN_SUBMIT=false ./scripts/e2e-test.sh --no-interactive
```

### Clean Up Test Data

```bash
# Remove all test logs and data
rm -rf e2e-test-logs/

# Kill all test processes
pkill -f "bin/matcher"
pkill -f "bin/validator"
pkill -f "test-agent"
```

---

## 🏗️ Architecture Notes

### Dual Submission Design

The dual submission pattern ensures:
1. **Blockchain**: Immutable record + economic security
2. **RootLayer**: Fast distribution to Subnet network

### EIP-191 Signature Standard

All signatures use Ethereum's EIP-191 standard:
```
signature = sign(keccak256("\x19Ethereum Signed Message:\n32" + digest))
```

This ensures:
- Compatibility with Ethereum tooling
- Standard signature verification
- Protection against replay attacks

### SDK Integration Points

1. **Intent Submission** (`submit-intent-signed.go`)
   - Uses SDK to compute Intent digest
   - Signs with EIP-191
   - Submits to blockchain via SDK

2. **Assignment Submission** (`internal/matcher/assignment_manager.go`)
   - Uses SDK for blockchain submission
   - Falls back to RootLayer-only if blockchain disabled

3. **ValidationBundle** (`internal/validator/node.go`)
   - Uses SDK to compute ValidationBundle digest (10 fields)
   - Signs with EIP-191 standard
   - 65-byte signatures properly formatted

---

**Last Updated**: 2025-10-13
**Version**: v2.0.0 - Dual Submission + EIP-191 Integration
