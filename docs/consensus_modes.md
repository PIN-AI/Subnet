# Consensus Modes Guide

This document explains how to choose and switch between Raft and CometBFT consensus modes.

## Overview

PinAI Subnet supports two consensus mechanisms:

1. **Raft** - Default mode, suitable for development and small-scale deployments
2. **CometBFT** - Tendermint BFT consensus, suitable for production environments

## Consensus Mode Comparison

| Feature | Raft | CometBFT |
|---------|------|----------|
| **Consensus Algorithm** | Raft (leader-based) | Tendermint BFT |
| **Byzantine Fault Tolerance** | ❌ Not supported | ✅ Supported (BFT) |
| **Configuration Complexity** | Simple | More complex |
| **Performance** | High throughput | Stronger consistency guarantees |
| **Network Type** | HTTP endpoints | P2P (libp2p) |
| **Minimum Nodes** | 1 (dev) / 3 (prod) | 3 (required) |
| **Use Cases** | Development, testing, trusted environments | Production, untrusted environments |

## Quick Start

### Method 1: Using Convenience Scripts (Recommended)

```bash
# Start Raft mode
./start-raft.sh

# Start CometBFT mode
./start-cometbft.sh
```

### Method 2: Setting Environment Variables

```bash
# Raft mode (default)
export CONSENSUS_TYPE="raft"
./scripts/start-subnet.sh

# CometBFT mode
export CONSENSUS_TYPE="cometbft"
./scripts/start-subnet.sh
```

## Detailed Configuration

### Raft Mode Configuration

```bash
# Environment variables
export SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000009"
export ROOTLAYER_GRPC="3.17.208.238:9001"
export ROOTLAYER_HTTP="http://3.17.208.238:8081"
export CONSENSUS_TYPE="raft"
export TEST_MODE=true
export START_AGENT=true

# Start
./scripts/start-subnet.sh
```

**Raft Mode Features:**
- Automatic leader election
- HTTP-based communication
- Simple configuration
- Suitable for development environments

**Port Assignment (3 validators):**
- Validator 1: gRPC=9090, Raft=7400
- Validator 2: gRPC=9100, Raft=7401
- Validator 3: gRPC=9110, Raft=7402

### CometBFT Mode Configuration

```bash
# Environment variables
export SUBNET_ID="0x0000000000000000000000000000000000000000000000000000000000000009"
export ROOTLAYER_GRPC="3.17.208.238:9001"
export ROOTLAYER_HTTP="http://3.17.208.238:8081"
export CONSENSUS_TYPE="cometbft"
export TEST_MODE=true
export START_AGENT=true

# Start
./scripts/start-subnet.sh
```

**CometBFT Mode Features:**
- Tendermint BFT consensus
- P2P network discovery
- Byzantine fault tolerance
- Suitable for production environments

**Port Assignment (3 validators):**
- Validator 1: gRPC=9090, CometBFT RPC=26657, P2P=26656
- Validator 2: gRPC=9100, CometBFT RPC=26667, P2P=26666
- Validator 3: gRPC=9110, CometBFT RPC=26677, P2P=26676

## Switching Consensus Modes

### Switching from Raft to CometBFT

1. Stop current services:
```bash
pkill -f 'bin/matcher|bin/validator|bin/registry|bin/simple-agent'
```

2. Clean data:
```bash
rm -rf subnet-logs test-cometbft
```

3. Start CometBFT mode:
```bash
./start-cometbft.sh
```

### Switching from CometBFT to Raft

1. Stop current services:
```bash
pkill -f 'bin/matcher|bin/validator|bin/registry|bin/simple-agent'
```

2. Clean data:
```bash
rm -rf subnet-logs test-cometbft
```

3. Start Raft mode:
```bash
./start-raft.sh
```

**Note**: Switching consensus modes will clear all local state!

## ValidationBundle Workflow

### ValidationBundle in Raft Mode

1. Agent executes task and submits ExecutionReport
2. Validator receives and validates ExecutionReport
3. ExecutionReport is replicated to all validators via Raft
4. Leader creates checkpoint
5. Validators sign checkpoint
6. Gossip protocol collects signatures
7. After reaching threshold, Leader submits ValidationBundle to RootLayer

### ValidationBundle in CometBFT Mode

1. Agent executes task and submits ExecutionReport
2. Validator receives ExecutionReport and submits to CometBFT mempool
3. ExecutionReport enters CometBFT state machine via ABCI
4. CometBFT consensus confirms ExecutionReport
5. Leader creates checkpoint and submits to CometBFT
6. Validators sign checkpoint (ECDSA) during ExtendVote phase
7. CometBFT collects signatures during PrepareProposal phase
8. After reaching threshold, submit ValidationBundle to RootLayer (one per Intent)

**Key Differences**:
- CometBFT uses vote extensions to collect ECDSA signatures
- CometBFT submits one ValidationBundle per Intent (instead of one per checkpoint)
- CometBFT's signature collection is synchronous, integrated into the consensus process

## Debugging and Monitoring

### Viewing Raft Status

```bash
# View leader election
grep -i "leader\|raft" subnet-logs/validator1.log

# Check Raft ports
lsof -i :7400
```

### Viewing CometBFT Status

```bash
# View CometBFT status
curl http://localhost:26657/status | jq

# View consensus logs
grep -i "consensus\|commit" subnet-logs/validator1.log

# View P2P connections
curl http://localhost:26657/net_info | jq
```

### Viewing ValidationBundle Submission

```bash
# View submission status
grep -i "ValidationBundle\|Successfully submitted" subnet-logs/validator1.log

# View errors
grep -i "Failed to submit\|error" subnet-logs/validator1.log
```

## Common Issues

### Q: ValidationBundle submission fails with "assignment not found"

**Cause**: Assignment not yet confirmed on blockchain.

**Solution**:
- This is normal! Both Matcher and Validator have retry mechanisms
- Matcher retries 3 times (15s/30s/45s intervals)
- Will succeed automatically after assignment confirmation

### Q: CometBFT nodes cannot connect

**Cause**: P2P port conflict or firewall blocking.

**Solution**:
```bash
# Check port usage
lsof -i :26656
lsof -i :26657

# Check node ID
curl http://localhost:26657/status | jq '.result.node_info.id'
```

### Q: Raft leader election fails

**Cause**: Validators cannot communicate with each other.

**Solution**:
```bash
# Check Raft ports
lsof -i :7400
lsof -i :7401
lsof -i :7402

# Check validator configuration
grep -i "raft" subnet-logs/validator*.log
```

### Q: How to know which consensus is currently in use?

```bash
# View logs
grep -i "consensus\|raft\|cometbft" subnet-logs/validator1.log | head -5

# Raft shows:
# "Raft consensus enabled"
# "Became Raft leader"

# CometBFT shows:
# "Starting CometBFT node"
# "Starting ABCI application"
```

## Performance Recommendations

### Raft Mode
- Development: 1 validator
- Testing: 3 validators
- Production: 3-5 validators (odd number)

### CometBFT Mode
- Minimum: 3 validators (2f+1, f=1)
- Recommended: 4 validators (tolerates 1 failure)
- Production: 7 validators (tolerates 2 failures)

## Next Steps

- See [Deployment Guide](subnet_deployment_guide.md) for production configuration
- See [E2E Test Guide](e2e_test_guide.md) for testing
- See [Scripts Guide](scripts_guide.md) for all available scripts
