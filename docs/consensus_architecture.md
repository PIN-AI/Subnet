# Consensus Architecture - Pluggable Design

## Overview

The Subnet Protocol supports **pluggable consensus engines**, allowing users to choose between different consensus mechanisms based on their trust model and performance requirements.

## Supported Consensus Types

### 1. Raft+Gossip (Currently Implemented)

**Type:** CFT (Crash Fault Tolerant)

**Components:**
- **Raft**: Provides log replication and leader election
- **Gossip (Memberlist)**: P2P signature propagation
- **StateMachine**: Manual signature collection and threshold checking

**Workflow:**
```
1. Leader proposes Checkpoint → Raft replicates to all nodes
2. Each validator signs the Checkpoint independently
3. Signatures broadcast via Gossip P2P network
4. StateMachine collects signatures until threshold (3/4) reached
5. ValidationBundle submitted to RootLayer
```

**Characteristics:**
- ✅ Fast consensus (100ms-1s)
- ✅ Simple implementation
- ✅ Low message overhead O(n)
- ⚠️  Assumes honest nodes (no Byzantine tolerance)
- ⚠️  Vulnerable to malicious validators

**Use Cases:**
- Trusted validator sets
- Private/permissioned networks
- Development and testing

### 2. CometBFT (Planned - Not Yet Implemented)

**Type:** BFT (Byzantine Fault Tolerant)

**Components:**
- **CometBFT Core**: Complete BFT consensus engine
- **ABCI Application**: Application-specific logic
- **Block Storage**: Permanent signature storage

**Workflow:**
```
1. Proposer proposes block (containing Checkpoint)
2. Validators vote: Prevote → Precommit (with signatures)
3. CometBFT automatically collects votes
4. Block committed when 2/3+ signatures collected
5. Extract signatures from Block.Commit → Submit to RootLayer
```

**Characteristics:**
- ✅ Byzantine fault tolerant (tolerates < 1/3 malicious nodes)
- ✅ Signatures permanently stored in blockchain
- ✅ Automatic signature collection
- ⚠️  Higher latency (3-7 seconds)
- ⚠️  Higher message overhead O(n²)

**Use Cases:**
- Public/permissionless networks
- Untrusted validator sets
- High-security requirements

## Architecture Design

### Directory Structure

```
internal/consensus/
├── engine.go              # ConsensusEngine interface
├── config.go              # Public configuration types
├── errors.go              # Error definitions
├── factory.go             # Factory pattern for creating engines
│
├── raftgossip/            # Raft+Gossip implementation
│   ├── consensus.go       # Main implementation
│   ├── raft_consensus.go  # Raft wrapper
│   ├── raft_fsm.go        # Raft finite state machine
│   ├── raft_log.go        # Raft log entries
│   ├── gossip_manager.go  # Gossip P2P manager
│   └── gossip_delegate.go # Gossip message handling
│
└── cometbft/              # CometBFT implementation (future)
    ├── consensus.go       # Main implementation
    ├── abci.go            # ABCI application
    └── config.go          # CometBFT configuration
```

### ConsensusEngine Interface

```go
type ConsensusEngine interface {
    // Lifecycle
    Start(ctx context.Context) error
    Stop() error
    IsReady() bool

    // Leadership
    IsLeader() bool
    GetLeaderID() string
    GetLeaderAddress() string

    // Checkpoint consensus
    ProposeCheckpoint(header *pb.CheckpointHeader) error
    GetLatestCheckpoint() *pb.CheckpointHeader

    // Signature handling (behavior differs by implementation)
    BroadcastSignature(sig *pb.Signature, epoch uint64, checkpointHash []byte) error
    GetSignatures(epoch uint64) []*pb.Signature
    CheckThreshold(epoch uint64) bool

    // Execution report handling
    ProposeExecutionReport(report *pb.ExecutionReport, reportKey string) error
    GetPendingReports() map[string]*pb.ExecutionReport
    ClearPendingReports(keys []string)

    // State queries
    GetCurrentEpoch() uint64
    GetValidatorSet() []string
}
```

### Configuration

#### Raft+Gossip Configuration

```yaml
consensus:
  type: "raft-gossip"

  raft:
    node_id: "validator-1"
    data_dir: "./data/raft"
    bind_address: "127.0.0.1:7000"
    bootstrap: true
    peers:
      - id: "validator-1"
        address: "127.0.0.1:7000"
      - id: "validator-2"
        address: "127.0.0.1:7001"
    heartbeat_timeout: 1s
    election_timeout: 3s
    commit_timeout: 500ms
    max_pool: 3

  gossip:
    node_name: "validator-1"
    bind_address: "0.0.0.0"
    bind_port: 7946
    seeds:
      - "127.0.0.1:7946"
      - "127.0.0.1:7947"
    gossip_interval: 200ms
    probe_interval: 1s
```

#### CometBFT Configuration (Future)

```yaml
consensus:
  type: "cometbft"

  cometbft:
    home_dir: "./data/cometbft"
    chain_id: "subnet-0x1234"
    moniker: "validator-1"

    p2p_listen_address: "tcp://0.0.0.0:26656"
    rpc_listen_address: "tcp://127.0.0.1:26657"

    seeds:
      - "node1-id@192.168.1.1:26656"
    persistent_peers:
      - "node2-id@192.168.1.2:26656"

    timeout_propose: 3s
    timeout_prevote: 1s
    timeout_precommit: 1s
    timeout_commit: 5s

    mempool_size: 5000
```

## Usage Example

### Creating a Consensus Engine

```go
// Define configuration
cfg := &consensus.ConsensusConfig{
    Type: consensus.ConsensusTypeRaftGossip,
    NodeID: "validator-1",
    ValidatorID: "validator-1",

    Raft: &consensus.RaftConfig{
        NodeID: "validator-1",
        DataDir: "./data/raft",
        BindAddress: "127.0.0.1:7000",
        Bootstrap: true,
        // ... other fields
    },

    Gossip: &consensus.GossipConfig{
        NodeName: "validator-1",
        BindAddress: "0.0.0.0",
        BindPort: 7946,
        // ... other fields
    },
}

// Define event handlers
handlers := consensus.ConsensusHandlers{
    OnCheckpointCommitted: func(header *pb.CheckpointHeader) {
        log.Info("Checkpoint committed", "epoch", header.Epoch)
    },
    OnSignatureReceived: func(sig *pb.Signature) {
        log.Debug("Signature received", "signer", sig.SignerId)
    },
}

// Create consensus engine
engine, err := consensus.NewConsensusEngine(cfg, handlers, logger)
if err != nil {
    return err
}

// Start engine
if err := engine.Start(context.Background()); err != nil {
    return err
}

// Use engine
if engine.IsLeader() {
    engine.ProposeCheckpoint(checkpointHeader)
}

engine.BroadcastSignature(signature, epoch, checkpointHash)

if engine.CheckThreshold(epoch) {
    signatures := engine.GetSignatures(epoch)
    // Submit ValidationBundle to RootLayer
}
```

## Key Differences Between Implementations

| Aspect | Raft+Gossip | CometBFT |
|--------|-------------|----------|
| **Signature Collection** | Manual via Gossip + StateMachine | Automatic in consensus voting |
| **Signature Storage** | In-memory map (ephemeral) | Blockchain storage (permanent) |
| **Threshold** | 3/4 (configurable) | 2/3+ (BFT standard) |
| **BroadcastSignature** | Broadcasts via Gossip P2P | No-op (included in votes) |
| **CheckThreshold** | Manual count check | Always true after commit |
| **GetSignatures** | From StateMachine | From Block.Commit |
| **Leader Election** | Raft leader election | Round-robin proposer |
| **Byzantine Tolerance** | No (CFT only) | Yes (BFT) |

## Migration Path

### Current State
- ✅ Raft+Gossip fully implemented
- ✅ Pluggable interface designed
- ✅ Factory pattern ready

### Adding CometBFT Support

1. **Implement CometBFT Engine** (`internal/consensus/cometbft/`)
   - Create `consensus.go` implementing `ConsensusEngine`
   - Implement ABCI application (`abci.go`)
   - Add configuration (`config.go`)

2. **Update Factory**
   - Add `case ConsensusTypeCometBFT` in `factory.go`
   - Implement `newCometBFTEngine()` function

3. **Update Configuration**
   - Uncomment CometBFT types in `engine.go`
   - Add validation for CometBFT config

4. **Testing**
   - Test both consensus types independently
   - Test switching between types
   - Performance benchmarking

## Best Practices

### When to Use Raft+Gossip
- Private/permissioned networks
- Trusted validator sets
- Low-latency requirements
- Simple deployment

### When to Use CometBFT
- Public/permissionless networks
- Untrusted validators
- Byzantine fault tolerance needed
- Audit trail requirements

## Performance Considerations

### Raft+Gossip
- **Throughput**: High (limited by Raft log replication)
- **Latency**: Low (100ms-1s)
- **Scalability**: Good up to ~10 validators
- **Network**: O(n) message complexity

### CometBFT
- **Throughput**: Medium (limited by consensus rounds)
- **Latency**: Medium (3-7s per block)
- **Scalability**: Good up to ~100 validators
- **Network**: O(n²) message complexity

## Security Considerations

### Raft+Gossip
- ⚠️  **No Byzantine tolerance**: Assumes all validators are honest
- ⚠️  **Signature malleability**: Signatures not permanently stored
- ✅ **Simple attack surface**: Fewer components to secure

### CometBFT
- ✅ **Byzantine fault tolerant**: Tolerates up to 1/3 malicious validators
- ✅ **Permanent audit trail**: All signatures stored on-chain
- ⚠️  **Complex implementation**: More attack vectors to consider

## Future Enhancements

1. **HotStuff Consensus**
   - Even faster BFT consensus
   - Linear message complexity O(n)
   - Suitable for high-performance scenarios

2. **Hybrid Mode**
   - Raft for fast consensus
   - Periodic BFT checkpoints for security
   - Best of both worlds

3. **Dynamic Switching**
   - Runtime switching between consensus types
   - Based on network conditions or security requirements

## References

- [Raft Consensus Algorithm](https://raft.github.io/)
- [HashiCorp Memberlist (SWIM Protocol)](https://github.com/hashicorp/memberlist)
- [CometBFT Documentation](https://docs.cometbft.com/)
- [Tendermint Consensus](https://docs.tendermint.com/v0.34/introduction/what-is-tendermint.html)
- [HotStuff BFT](https://arxiv.org/abs/1803.05069)
