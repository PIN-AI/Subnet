# Consensus Data Format Compatibility Rules

## Overview

This document describes critical data format differences between Raft and CometBFT consensus mechanisms, and how to maintain compatibility when processing checkpoint data for RootLayer submission.

**IMPORTANT**: This document must be consulted when modifying any code that processes CheckpointHeader data or constructs ValidationBundle for RootLayer submission.

## Problem Background

### CometBFT Vote Extensions

CometBFT uses **Vote Extensions** to include application-specific data in votes. In our implementation:

1. **Vote Extension Content**: Each vote extension contains the **entire serialized CheckpointHeader** (protobuf format)
2. **Storage in CheckpointHeader**: This serialized data gets stored in `CheckpointHeader.ParentCpHash`
3. **Data Size**: Instead of a 32-byte hash, `ParentCpHash` contains the full protobuf message (~100-500 bytes)

### Raft Consensus

In Raft mode:

1. **Simple Hash**: `CheckpointHeader.ParentCpHash` contains a standard 32-byte SHA256 hash
2. **Direct Usage**: Can be directly hex-encoded and used as `root_hash` in ValidationBundle

## Critical Rule: RootHash Field Handling

### Location
- File: `internal/validator/node.go`
- Function: `buildValidationBundleForIntent`
- Lines: ~2279-2296

### The Problem

When constructing `ValidationBundle.RootHash` for RootLayer submission:

```go
// Line 2287: Convert ParentCpHash to hex string
var rootHashStr string
if len(header.ParentCpHash) == 0 {
    rootHashStr = "0x0000000000000000000000000000000000000000000000000000000000000000"
} else {
    rootHashStr = "0x" + hex.EncodeToString(header.ParentCpHash)  // ❌ FAILS for CometBFT
}

bundle := &rootpb.ValidationBundle{
    // ...
    RootHash:     rootHashStr,  // Line 2296
    // ...
}
```

**Why it fails for CometBFT:**
- `hex.EncodeToString(header.ParentCpHash)` produces a very long hex string (200-1000 chars)
- This creates a protobuf `string` field that is valid UTF-8, but RootLayer may expect a 32-byte hash
- The long hex string may cause gRPC marshalling errors: `"string field contains invalid UTF-8"`

### The Solution

**Use SHA256 hash of ParentCpHash for CometBFT, direct encoding for Raft:**

```go
// Format RootHash with consensus-aware handling
var rootHashStr string
if len(header.ParentCpHash) == 0 {
    // Epoch 0 has no parent - use zero hash
    rootHashStr = "0x0000000000000000000000000000000000000000000000000000000000000000"
} else {
    // CometBFT: ParentCpHash contains serialized CheckpointHeader, hash it first
    // Raft: ParentCpHash is already a 32-byte hash, use directly
    if len(header.ParentCpHash) > 32 {
        // CometBFT mode: hash the serialized checkpoint header
        hashBytes := sha256.Sum256(header.ParentCpHash)
        rootHashStr = "0x" + hex.EncodeToString(hashBytes[:])
    } else {
        // Raft mode: use the hash directly
        rootHashStr = "0x" + hex.EncodeToString(header.ParentCpHash)
    }
}
```

## Detection Logic

**How to detect which consensus is in use:**

1. **By data size**: If `len(header.ParentCpHash) > 32`, it's CometBFT serialized data
2. **By configuration**: Check `n.config.ConsensusType` if available
3. **Safe assumption**: Size-based detection is most reliable

## Files That MUST Follow This Rule

### Primary Files
1. `internal/validator/node.go` - `buildValidationBundleForIntent()` function
2. `internal/consensus/chain_manager.go` - Any code that processes `ParentCpHash`

### Related Files (Read-Only Usage)
- `proto/subnet/checkpoint.proto` - CheckpointHeader definition
- `proto/rootlayer/validation.proto` - ValidationBundle definition

## ASN.1/DER Signature Format (DEPRECATED for ValidationBundle)

### CheckpointSignature (Gossip)
- **File**: `proto/subnet/gossip.proto`
- **Field**: `bytes signature_der = 3;`
- **Format**: DER-encoded ECDSA signature
- **Usage**: Checkpoint consensus signatures (Raft only)

### ValidationBundleSignature (Gossip + RootLayer)
- **File**: `proto/subnet/gossip.proto` and `proto/rootlayer/validation.proto`
- **Field**: `bytes signature = 5;` (gossip) / `bytes signature = 2;` (rootlayer)
- **Format**: EIP-191 signature (65 bytes)
- **Usage**: ValidationBundle signatures for RootLayer submission

**IMPORTANT**:
- CheckpointSignature uses DER format (Raft specific)
- ValidationBundleSignature uses EIP-191 format (both Raft and CometBFT)
- Never mix these two formats!

## Testing Requirements

When modifying checkpoint or ValidationBundle code, test BOTH:

1. **Raft Mode**:
   ```bash
   export CONSENSUS_TYPE=raft
   export NUM_VALIDATORS=1
   ./scripts/start-subnet.sh
   # Submit intent and verify ValidationBundle submission succeeds
   ```

2. **CometBFT Mode**:
   ```bash
   export CONSENSUS_TYPE=cometbft
   export NUM_VALIDATORS=3
   ./scripts/start-subnet.sh
   # Submit intent and verify ValidationBundle submission succeeds
   ```

3. **Verification Points**:
   - Agent receives and executes task
   - Validator creates checkpoint
   - ValidationBundle signatures collected via gossip
   - ValidationBundle submitted to RootLayer successfully (check logs for "ValidationBundle submission complete")
   - No "string field contains invalid UTF-8" errors

## Error Messages to Watch For

### CometBFT Specific Errors
```
Failed to submit ValidationBundle to RootLayer: rpc error: code = Internal desc = grpc: error unmarshalling request: string field contains invalid UTF-8
```

**Cause**: RootHash field contains raw ParentCpHash data instead of hashed value

**Fix**: Apply the SHA256 hashing logic described above

### Raft Specific Errors
None expected if following these rules correctly.

## Change History

| Date | Change | Author |
|------|--------|--------|
| 2025-10-30 | Initial documentation of RootHash handling rule | Claude Code |
| 2025-10-30 | Added ASN.1/DER vs EIP-191 signature format clarification | Claude Code |

## Related Documentation

- `docs/consensus_architecture.md` - Overall consensus design
- `docs/validation_bundle_multi_signature.md` - ValidationBundle signature collection
- `docs/cometbft_workflow.md` - CometBFT integration details (if exists)
