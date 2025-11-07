# E2E Test Guide - Complete Intent Flow

## 🚀 Quick Start

Test the complete Intent flow with blockchain submission using the start-subnet script and send-intent tool.

```bash
cd /Users/ty/pinai/protocol/Subnet

# Build all binaries
make build

# Start Subnet services (registry + matcher + validators + agent)
./scripts/start-subnet.sh

# In another terminal, send a test Intent
./scripts/send-intent.sh
```

---

## 📋 Complete Test Flow

The E2E test covers the entire Intent lifecycle:

```
1. Intent Submission
   ├─ Submit to Blockchain (IntentManager contract)
   └─ Submit to RootLayer (for distribution)
   ↓
2. Matcher pulls Intent from RootLayer and creates Bidding Window
   ↓
3. Agent subscribes to Intent stream and submits Bid
   ↓
4. Matcher performs matching and creates Assignment
   ├─ Submit Assignment to Blockchain
   └─ Submit Assignment to RootLayer
   ↓
5. Agent receives Assignment and executes task
   ↓
6. Agent submits ExecutionReport to Validator
   ↓
7. Validator validates report and returns Receipt
   ↓
8. Validator replicates ExecutionReport via Raft consensus
   ↓
9. Raft leader creates Checkpoint with pending reports
   ↓
10. Validators collect signatures via Gossip protocol (Memberlist)
   ↓
11. ValidationBundle construction with EIP-191 signatures
   ├─ Uses SDK to compute proper digest
   ├─ Signs with EIP-191 standard (65-byte signature)
   └─ Submits to RootLayer and Blockchain
```

---

## ✨ Key Features

### 1. **Raft + Gossip Consensus**
Validators use dual consensus mechanism:
- **Raft**: For leader election and ExecutionReport replication
- **Gossip (Memberlist)**: For signature collection across validators
- **No external dependencies**: No NATS or other message brokers required

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

## 🎯 Services Started

When you run `./scripts/start-subnet.sh`, the following services are started:

| Service | Port | Purpose |
|---------|------|---------|
| **Registry** | 8091 (gRPC), 8092 (HTTP) | Agent/Validator discovery with heartbeats |
| **Matcher** | 8090 | Intent matching service with bidding windows |
| **Validator 1** | 9090 (gRPC), 7400 (Raft) | Validation node with Raft leader |
| **Validator 2** | 9100 (gRPC), 7401 (Raft) | Validation node (Raft follower) |
| **Validator 3** | 9110 (gRPC), 7402 (Raft) | Validation node (Raft follower) |
| **Test Agent** | - | Demo agent that bids and executes tasks |

**Note**: No NATS required! The system uses Raft for consensus and Gossip for signature collection.

---

## 🔑 Environment Variables

### Required Configuration

Create a `.env` file in the project root or set these environment variables:

```bash
# Subnet Configuration
export SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000003"
export ROOTLAYER_GRPC="3.17.208.238:9001"
export ROOTLAYER_HTTP="http://3.17.208.238:8081"

# Blockchain Configuration
export CHAIN_RPC_URL="https://sepolia.base.org"
export CHAIN_NETWORK="base_sepolia"

# Contract Addresses (Base Sepolia)
export PIN_BASE_SEPOLIA_STAKING_MANAGER="0x7f887e88014e3AF57526B68b431bA16e6968C015"
export PIN_BASE_SEPOLIA_SUBNET_FACTORY="0x2b5D7032297Df52ADEd7020c3B825f048Cd2df3E"
export PIN_BASE_SEPOLIA_INTENT_MANAGER="0xB2f092E696B33b7a95e1f961369Bb59611CAd093"

# Private Keys (use test keys only!)
export TEST_PRIVATE_KEY="your_test_private_key_here"
export VALIDATOR_KEYS="key1,key2,key3"  # Comma-separated for 3 validators

# Optional: Consensus Mode (default is raft)
export CONSENSUS_TYPE="raft"  # or "cometbft"
```

**SECURITY NOTE:** Never use production private keys for testing! Use dedicated test keys with no real funds.

---

## 📊 Step-by-Step E2E Testing

### Step 1: Start Subnet Services

```bash
# Load environment
source .env

# Start all services
./scripts/start-subnet.sh
```

This will start:
- Registry service
- Matcher
- 3 Validators (Raft cluster)
- 1 Test Agent

Wait ~10 seconds for all services to initialize and Raft leader election to complete.

### Step 2: Verify Services are Running

```bash
# Check Raft leader
grep -i "became.*leader\|raft" subnet-logs/validator1.log

# Check registry
curl http://localhost:8092/health

# Check validator registration
curl http://localhost:8092/validators
```

### Step 3: Submit Test Intent

```bash
# Interactive mode (recommended for first test)
./scripts/send-intent.sh

# Follow the prompts to enter:
# - Intent type
# - Task description
# - Amount (in Wei)
```

The script will:
1. Load your private key from `.env`
2. Generate a random Intent ID
3. Compute EIP-191 signature using SDK
4. Submit to blockchain (IntentManager contract)
5. Wait for transaction confirmation
6. Submit to RootLayer for distribution

### Step 4: Monitor the Flow

Watch the logs in real-time:

```bash
# Terminal 1: Matcher logs (bidding, matching)
tail -f subnet-logs/matcher.log

# Terminal 2: Validator logs (reports, checkpoints, signatures)
tail -f subnet-logs/validator1.log

# Terminal 3: Agent logs (bids, execution)
tail -f subnet-logs/agent.log
```

---

## ✅ Success Indicators

### 1. Intent Submission Success

Check output from `send-intent.sh`:
```
✅ Intent submitted successfully!
Intent ID: 0x1234...
Transaction Hash: 0xabcd...
EIP-191 signature verified locally
```

### 2. Matcher Receives Intent

Check `subnet-logs/matcher.log`:
```
Received new intent from RootLayer
intent_id=0x1234... subnet_id=0x...0003
Creating bidding window window_duration=10s
```

### 3. Agent Bids

Check `subnet-logs/agent.log`:
```
Received intent notification intent_id=0x1234...
Submitting bid bid_price=0.001 ETH
Bid submitted successfully
```

### 4. Assignment Created

Check `subnet-logs/matcher.log`:
```
Matching completed intent_id=0x1234... winner=agent-001
Assignment created assignment_id=0xabcd...
Assignment submitted to blockchain tx_hash=0x...
```

### 5. Agent Executes Task

Check `subnet-logs/agent.log`:
```
Received assignment assignment_id=0xabcd...
Executing task...
Task completed successfully
Submitting execution report to validator
```

### 6. Validator Processes Report

Check `subnet-logs/validator1.log`:
```
Received execution report assignment_id=0xabcd...
Validation passed
Report replicated via Raft consensus
```

### 7. Checkpoint Creation (Raft Leader)

Check `subnet-logs/validator1.log`:
```
Creating checkpoint epoch=1 pending_reports=1
Checkpoint created checkpoint_id=chk_001
Broadcasting checkpoint for signature collection
```

### 8. Signature Collection (Gossip Protocol)

Check `subnet-logs/validator1.log`, `validator2.log`, `validator3.log`:
```
Received checkpoint signature request checkpoint_id=chk_001
Signing checkpoint with EIP-191
Signature sent via gossip validator_id=validator-1 sig_len=65

# Leader collects signatures
Received checkpoint signature via gossip validator=validator-2
Received checkpoint signature via gossip validator=validator-3
Threshold reached signatures=3/3
```

### 9. ValidationBundle Submission

Check `subnet-logs/validator1.log`:
```
Constructing ValidationBundle intent_id=0x1234...
ValidationBundle signed validators=3 signatures=3
Submitting ValidationBundle to RootLayer
✅ ValidationBundle submitted successfully
```

---

## 🐛 Troubleshooting

### Issue 1: Services Not Starting

**Symptoms:**
- Ports already in use
- Services crash immediately

**Solution:**
```bash
# Stop existing services
./scripts/stop-subnet.sh

# Or manually kill processes
pkill -f 'bin/matcher|bin/validator|bin/registry|bin/simple-agent'

# Check no processes remain
lsof -i :8090,9090,9100,9110,8091

# Restart
./scripts/start-subnet.sh
```

### Issue 2: Raft Leader Election Fails

**Symptoms:**
```
WARN Raft election timeout, starting new election
```

**Solution:**
- Ensure all 3 validators are running
- Check Raft ports are accessible (7400, 7401, 7402)
- Verify validator IDs match in `--validator-pubkeys` parameter

```bash
# Check Raft status
grep -i "raft\|leader" subnet-logs/validator*.log

# Raft should show one leader
grep "Became Raft leader" subnet-logs/validator*.log
```

### Issue 3: No Gossip Signatures Received

**Symptoms:**
```
Checkpoint created but no signatures received
Threshold not reached signatures=1/3
```

**Solution:**
- Check Gossip ports (7946-7948) are open
- Verify all validators have correct `--gossip-peers` configuration
- Check logs for Gossip membership:

```bash
grep -i "gossip\|memberlist" subnet-logs/validator*.log
```

### Issue 4: Intent Not Confirmed on Blockchain

**Symptoms:**
```
Transaction failed on-chain (status=0)
Invalid intent status: intent not found
```

**Solution:**
- Check you have enough ETH on Base Sepolia
- Verify IntentManager contract address is correct
- Wait 10-30 seconds for blockchain confirmation
- Check Base Sepolia RPC is responding:

```bash
curl https://sepolia.base.org \
  -X POST \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
```

### Issue 5: ValidationBundle Submission - "Invalid intent status"

**Error:**
```
Failed to submit validation bundle: Invalid intent status: intent status is not PROCESSING
```

**Root Cause:**
After Assignment is submitted to blockchain, RootLayer needs 15-30 seconds to synchronize the Intent status.

**Solution (Automatic):**
The validator implements automatic retry logic:
1. **15-second initial delay**: Waits for RootLayer state synchronization
2. **5 retry attempts**: If submission fails, retries with 10-second intervals
3. **Total wait time**: Up to 65 seconds (15s initial + 5×10s retries)

Check logs for retry progress:
```bash
grep "ValidationBundle\|retry" subnet-logs/validator1.log
```

### Issue 6: Wrong Consensus Mode

**Symptoms:**
```
CometBFT not configured
P2P port error
```

**Solution:**
Ensure you're using the correct consensus mode:

```bash
# For Raft mode (default)
export CONSENSUS_TYPE="raft"
./scripts/start-subnet.sh

# For CometBFT mode
export CONSENSUS_TYPE="cometbft"
./scripts/start-subnet.sh
```

---

## 📝 Viewing Logs

All logs are saved in `subnet-logs/`:

```bash
# Real-time Matcher logs
tail -f subnet-logs/matcher.log

# Real-time Validator logs (check all 3)
tail -f subnet-logs/validator1.log
tail -f subnet-logs/validator2.log
tail -f subnet-logs/validator3.log

# Real-time Agent logs
tail -f subnet-logs/agent.log

# Search for blockchain transactions
grep "transaction\|tx_hash" subnet-logs/matcher.log

# Search for Raft leader election
grep -i "raft.*leader" subnet-logs/validator*.log

# Search for Gossip signatures
grep -i "gossip.*signature" subnet-logs/validator*.log

# Search for ValidationBundle
grep "ValidationBundle" subnet-logs/validator1.log
```

---

## 🎯 Success Criteria

A complete successful E2E test should have:

- [x] Intent submitted to blockchain (TX hash logged)
- [x] Intent submitted to RootLayer (HTTP 200 response)
- [x] EIP-191 signature verification passed
- [x] Matcher receives Intent from RootLayer
- [x] Test Agent submits bid successfully
- [x] Matcher performs matching and selects winner
- [x] Assignment created and submitted to blockchain
- [x] Agent receives assignment and executes task
- [x] ExecutionReport submitted to Validator
- [x] Validator replicates report via Raft
- [x] Raft leader creates checkpoint
- [x] Signatures collected via Gossip (3/3 validators)
- [x] ValidationBundle constructed with EIP-191 signatures
- [x] ValidationBundle submitted to RootLayer successfully
- [x] No critical errors in logs

---

## 🔍 Monitoring Blockchain Transactions

### View on Base Sepolia Explorer

After submitting Intent/Assignment, view transactions on:

```
https://sepolia.basescan.org/tx/<transaction_hash>
```

Replace `<transaction_hash>` with the hash from logs.

### Check IntentManager Contract

View the IntentManager contract state:

```
https://sepolia.basescan.org/address/0xB2f092E696B33b7a95e1f961369Bb59611CAd093
```

---

## 💡 Advanced Testing

### Test with CometBFT Consensus

Switch to CometBFT for Byzantine fault tolerance:

```bash
export CONSENSUS_TYPE="cometbft"
./scripts/start-subnet.sh

# Check CometBFT status
curl http://localhost:26657/status | jq

# View consensus logs
grep -i "consensus\|commit" subnet-logs/validator1.log
```

See [docs/consensus_modes.md](consensus_modes.md) for detailed configuration.

### Test Leader Rotation

Kill the current Raft leader and watch automatic failover:

```bash
# Find current leader
grep "Became Raft leader" subnet-logs/validator*.log

# Kill the leader process (e.g., validator1)
pkill -f "bin/validator.*9090"

# Watch new leader election (should happen within 5-10 seconds)
tail -f subnet-logs/validator2.log subnet-logs/validator3.log | grep -i "leader"
```

### Stress Test with Multiple Intents

Submit multiple intents in parallel:

```bash
# Submit 10 intents
for i in {1..10}; do
  ./scripts/send-intent.sh --no-interactive &
done

# Monitor checkpoint creation
watch -n 1 'grep "checkpoint" subnet-logs/validator1.log | tail -5'
```

### Test Signature Threshold

Configure different signature thresholds:

```bash
# Require 2/3 signatures (default is 3/3)
export SIGNATURE_THRESHOLD="0.67"  # 67%
./scripts/start-subnet.sh
```

---

## 🧹 Clean Up

Stop all services and clean up test data:

```bash
# Stop all services
./scripts/stop-subnet.sh

# Remove logs
rm -rf subnet-logs/

# Remove Raft data
rm -rf raft-data-*/

# Remove LevelDB state
rm -rf validator-db-*/
```

---

## 📚 Related Documentation

- **Quick Start Guide**: [docs/quick_start.md](quick_start.md)
- **Architecture Overview**: [docs/architecture.md](architecture.md)
- **Consensus Modes**: [docs/consensus_modes.md](consensus_modes.md)
- **Scripts Guide**: [docs/scripts_guide.md](scripts_guide.md)
- **Deployment Guide**: [docs/subnet_deployment_guide.md](subnet_deployment_guide.md)
- **Coding Assistant Guide**: [CLAUDE.md](../CLAUDE.md)

---

## 🏗️ Architecture Notes

### Raft + Gossip Consensus

The system uses a dual consensus mechanism:

1. **Raft Consensus** (Leader Election + Log Replication)
   - Elects a leader among validators
   - Replicates ExecutionReports to all validators
   - Ensures consistency of checkpoint creation
   - Leader creates checkpoints at intervals (default: 10s)

2. **Gossip Protocol** (Signature Collection)
   - Uses HashiCorp Memberlist for P2P communication
   - Leader broadcasts checkpoint for signature requests
   - Each validator signs and sends signature via gossip
   - Leader collects signatures until threshold is reached
   - No external message broker required

### Why No NATS?

Previous versions used NATS JetStream for message broadcasting. The current version uses:
- **Raft** for consensus and replication (built-in to validator)
- **Gossip (Memberlist)** for signature propagation (P2P, no broker)

This eliminates external dependencies and simplifies deployment.

---

**Last Updated**: 2025-11-07
**Version**: v3.0.0 - Raft+Gossip Consensus (No NATS Required)
