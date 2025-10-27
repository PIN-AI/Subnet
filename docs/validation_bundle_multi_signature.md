# ValidationBundle Multi-Signature Architecture

## Overview

ValidationBundle multi-signature is a critical security feature that ensures multiple validators cryptographically sign each ValidationBundle before submission to RootLayer. This provides Byzantine fault tolerance and prevents single validator manipulation of execution results.

## Architecture

### Key Components

1. **Raft Consensus** - Leader election and checkpoint replication
2. **Gossip Protocol** (HashiCorp Memberlist) - Signature propagation across validators
3. **Threshold Signing** - Weighted signature collection (default: 2/3 validators)
4. **Dual Submission** - Simultaneous blockchain and RootLayer HTTP submission

### Workflow

```
1. Agent submits ExecutionReport → Validator
2. Follower forwards report → Leader Validator
3. Leader creates Checkpoint → Raft replication
4. All validators receive Checkpoint via handleCommittedCheckpoint()
5. All validators sign ValidationBundle + gossip signatures
6. All validators collect signatures via gossip
7. Leader waits for threshold (2/3) signatures
8. Leader submits ValidationBundle to RootLayer (with multi-sig)
```

## Implementation Details

### Validator Configuration

**Required CLI Flags**:

```bash
./bin/validator \
  -validator-id "validator-1" \
  -grpc-port 9201 \
  -raft-bind-addr :7001 \
  -raft-data-dir ./data/val1-raft \
  -gossip-bind-addr :6001 \
  -gossip-seeds "127.0.0.1:6002,127.0.0.1:6003" \
  -validator-endpoints "validator-1=127.0.0.1:9201,validator-2=127.0.0.1:9202,validator-3=127.0.0.1:9203" \
  -private-key "0xYOUR_VALIDATOR_PRIVATE_KEY"
```

**Critical Parameters**:

- `-gossip-seeds`: Comma-separated list of other validators' gossip addresses
- `-validator-endpoints`: Mapping of validator IDs to their gRPC endpoints (for forwarding ExecutionReports)
- `-private-key`: Used for signing ValidationBundles (EIP-191 format)

### Code Implementation

**File**: `internal/validator/node.go`

**Key Function**: `handleCommittedCheckpoint()` (lines 601-614)

This function is triggered on ALL validators when a checkpoint is committed via Raft:

```go
// Copy pending reports for ValidationBundle signing (before unlock)
pendingReportsCopy := make(map[string]*pb.ExecutionReport, len(n.pendingReports))
for k, v := range n.pendingReports {
    pendingReportsCopy[k] = proto.Clone(v).(*pb.ExecutionReport)
}

n.mu.Unlock()

// ALL validators sign ValidationBundle for this checkpoint
if len(pendingReportsCopy) > 0 {
    groupedReports := n.groupReportsByIntent(pendingReportsCopy)
    n.logger.Infof("Validator signing ValidationBundle for checkpoint epoch=%d intents=%d",
        headerCopy.Epoch, len(groupedReports))
    n.signAndGossipValidationBundles(headerCopy, groupedReports)
}
```

**Why This Works**:
- `handleCommittedCheckpoint()` is called by Raft's FSM on ALL validators (leader + followers)
- Each validator independently signs the same ValidationBundle data
- Signatures are broadcast via gossip to all peers
- Each validator collects signatures until threshold is reached
- Only the leader submits to RootLayer, but with threshold multi-sig

### Gossip Configuration

**Gossip Seeds**:
Each validator must know at least one other validator's gossip address to form the cluster.

Example for 3 validators:
```bash
# Validator 1 (no seeds needed, others will connect to it)
-gossip-bind-addr :6001

# Validator 2
-gossip-bind-addr :6002 -gossip-seeds "127.0.0.1:6001"

# Validator 3
-gossip-bind-addr :6003 -gossip-seeds "127.0.0.1:6001,127.0.0.1:6002"
```

**Added in**: `cmd/validator/main.go` lines 51, 396-402

### Execution Report Forwarding

**Problem**: Agents may submit ExecutionReports to follower validators, but only the leader can create checkpoints.

**Solution**: `validator-endpoints` mapping allows followers to forward reports to the leader.

**Implementation**: `internal/validator/handlers.go` lines 118-142

```go
func (n *Node) forwardReportToLeader(ctx context.Context, report *pb.ExecutionReport) error {
    leaderID := n.getLeaderIDFromRaft()
    if leaderID == "" {
        return fmt.Errorf("no leader available")
    }

    // Get leader's validator endpoint from mapping
    leaderEndpoint := n.validatorEndpoints[leaderID]
    if leaderEndpoint == "" {
        return fmt.Errorf("leader endpoint not found for %s", leaderID)
    }

    // Forward report to leader's gRPC endpoint
    conn, err := grpc.Dial(leaderEndpoint, ...)
    // ... forward report
}
```

## Security Considerations

### Signature Verification

Each validator verifies:
1. Report signature matches agent's public key
2. Assignment ID exists and matches
3. Parameters hash matches Intent parameters
4. Timestamp is recent (prevents replay attacks)

### Threshold Requirements

- Default: 2/3 validators must sign (Byzantine fault tolerance)
- Configurable via subnet staking weights
- Prevents single validator manipulation

### Private Key Management

**CRITICAL**: Each validator must have a unique private key:
- Used for signing ValidationBundles
- Derivation: `cast wallet new` or similar secure key generation
- Storage: Environment variable or secure config file (never commit to git)
- Format: 64 hex characters with optional `0x` prefix

## Deployment Guide

### Multi-Validator Setup (3 validators)

1. **Generate Validator Keys**:
```bash
# Generate 3 validator private keys securely
cast wallet new  # Repeat 3 times, save private keys
```

2. **Start Validators**:

```bash
# Validator 1 (Leader initially)
./bin/validator \
  -validator-id "validator-1" \
  -grpc-port 9201 \
  -raft-bind-addr :7001 \
  -raft-data-dir ./data/val1-raft \
  -gossip-bind-addr :6001 \
  -validator-endpoints "validator-1=127.0.0.1:9201,validator-2=127.0.0.1:9202,validator-3=127.0.0.1:9203" \
  -private-key "$VALIDATOR1_KEY"

# Validator 2
./bin/validator \
  -validator-id "validator-2" \
  -grpc-port 9202 \
  -raft-bind-addr :7002 \
  -raft-peers "validator-1=127.0.0.1:7001" \
  -raft-data-dir ./data/val2-raft \
  -gossip-bind-addr :6002 \
  -gossip-seeds "127.0.0.1:6001" \
  -validator-endpoints "validator-1=127.0.0.1:9201,validator-2=127.0.0.1:9202,validator-3=127.0.0.1:9203" \
  -private-key "$VALIDATOR2_KEY"

# Validator 3
./bin/validator \
  -validator-id "validator-3" \
  -grpc-port 9203 \
  -raft-bind-addr :7003 \
  -raft-peers "validator-1=127.0.0.1:7001" \
  -raft-data-dir ./data/val3-raft \
  -gossip-bind-addr :6003 \
  -gossip-seeds "127.0.0.1:6001,127.0.0.1:6002" \
  -validator-endpoints "validator-1=127.0.0.1:9201,validator-2=127.0.0.1:9202,validator-3=127.0.0.1:9203" \
  -private-key "$VALIDATOR3_KEY"
```

3. **Verify Cluster Formation**:

```bash
# Check Raft leader election
grep "leadership" subnet-logs/validator-*.log

# Check Gossip cluster
grep "memberlist.*Joined" subnet-logs/validator-*.log

# Verify all validators are signing
grep "Signing ValidationBundle" subnet-logs/validator-*.log
```

## Monitoring and Verification

### Log Patterns to Watch

**Successful Multi-Signature Flow**:

```
# All validators sign
time="..." level=info msg="Signing ValidationBundle intent_id=intent-001 epoch=0"

# Signatures propagate via gossip
time="..." level=info msg="Received ValidationBundle signature via gossip validator=0x3d5f... total_sigs=1"
time="..." level=info msg="Received ValidationBundle signature via gossip validator=0x4d3A... total_sigs=2"

# Threshold reached
time="..." level=info msg="Weighted signature threshold reached via gossip epoch=0 required_weight=2 sigs=2 total_weight=3"

# Leader submits to RootLayer
time="..." level=info msg="Submitting 1 ValidationBundles in batch batch_id=epoch-0-... epoch=0"
```

### Troubleshooting

**Issue**: Only leader is signing ValidationBundles

**Check**:
```bash
grep "Signing ValidationBundle" subnet-logs/validator-*.log | wc -l
# Should see 3x signatures (one per validator) for each ValidationBundle
```

**Fix**: Ensure all validators have gossip seeds configured

---

**Issue**: Signatures not propagating

**Check**:
```bash
grep "Joined cluster" subnet-logs/validator-*.log
# All validators should report joining the cluster
```

**Fix**: Verify gossip bind addresses are reachable and seeds are correct

---

**Issue**: Threshold never reached

**Check**:
```bash
grep "threshold reached" subnet-logs/validator-*.log
```

**Fix**:
- Verify at least 2/3 validators are running
- Check gossip connectivity
- Ensure validators are signing (check "Signing ValidationBundle" logs)

## Protocol Details

### ValidationBundle Signature Format

**Signing Process**:
1. Serialize ValidationBundle data (IntentID, AssignmentID, AgentID, Result, etc.)
2. Compute Keccak256 hash
3. Sign with EIP-191 format: `0x19Ethereum Signed Message:\n32` + hash
4. Encode signature as base64url (no padding)

**Verification**:
- RootLayer recovers signer address from signature
- Checks signer is registered validator for subnet
- Verifies weight threshold is met

### Gossip Message Types

**Defined in**: `proto/subnet/gossip.proto`

1. **ValidationBundleSignature**:
   - `validator_address`: Signer's Ethereum address
   - `intent_id`: Intent being validated
   - `assignment_id`: Assignment ID
   - `signature`: EIP-191 signature bytes
   - `epoch`: Checkpoint epoch number

2. **CheckpointSignature**:
   - Used for Raft checkpoint consensus (separate from ValidationBundle)

## Performance Characteristics

**Tested Configuration**: 3 validators, subnet ID 0x09

- **Signature Propagation**: <1 second
- **Threshold Achievement**: <2 seconds
- **Checkpoint Interval**: ~30 seconds
- **RootLayer Sync Wait**: 15 seconds (configurable)

**Throughput**:
- Successfully processed 40+ ExecutionReports
- Created 13+ checkpoints with multi-sig ValidationBundles
- No signature collection failures

## References

- **Raft Consensus**: Used for checkpoint replication and leader election
- **Gossip Protocol**: HashiCorp Memberlist for signature propagation
- **EIP-191**: Ethereum signed message standard
- **Byzantine Fault Tolerance**: 2/3 threshold prevents single validator manipulation

## Code Files

- `internal/validator/node.go`: Core ValidationBundle signing logic
- `internal/validator/handlers.go`: ExecutionReport forwarding
- `cmd/validator/main.go`: CLI flags and configuration
- `internal/consensus/broadcaster.go`: Gossip integration
- `proto/subnet/gossip.proto`: Gossip message definitions

---

**Last Updated**: 2025-10-26
**Status**: Production-ready
**Tested**: 3-validator setup on subnet ID 0x09
