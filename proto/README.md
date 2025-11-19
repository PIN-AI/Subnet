# Subnet Protocol Buffers

This directory contains generated Protocol Buffer code for the Subnet components of the Pin Protocol.

## Generated Code

This directory contains only generated Go bindings (`*.pb.go`). Do not edit these files manually.

## Architecture Separation

The Subnet layer is responsible for:
1. **Execution Verification** - Validators verify Agent execution reports
2. **Checkpoint Consensus** - Creating and signing state checkpoints
3. **Validation Policies** - Enforcing execution validation rules
4. **State Management** - Tracking execution results and validator decisions

The RootLayer (separate proto directory) handles:
- Intent submission and lifecycle
- Individual intent bidding periods
- Matcher-Agent assignment coordination

## Layer Interaction

Subnet and RootLayer interact through well-defined boundaries:
- Agents receive Assignments from RootLayer Matchers
- Agents submit ExecutionReports to Subnet Validators (not Matchers)
- Validators verify results and create checkpoints
- Checkpoints eventually synchronize to RootLayer

## Regenerating Bindings

Use the Makefile target to regenerate the protobuf code:

```bash
make proto
```

This regenerates all `*.pb.go` files in this directory.
