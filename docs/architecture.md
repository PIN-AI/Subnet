# Subnet Architecture Overview

This Go codebase implements the execution layer for a PinAI Subnet. The Matcher assigns intents to agent operators, agents execute work via the standalone SDK, and Validators verify execution results before reporting aggregates back to the RootLayer with **high-performance batch submission support**.

## High-Level Flow

```
RootLayer intents → Matcher (batch pulls) → Agent (SDK) → Validator → RootLayer (batch submission)
                           ↓                                    ↓
                    Assignment batch                  ValidationBundle batch
```

## Core Components

1. **Matcher (`cmd/matcher`, `internal/matcher`)**
   - Pulls intents from the RootLayer client using `CompleteClient` (`internal/rootlayer`)
   - Maintains bidding windows per intent and collects agent bids
   - Selects winning agents using pluggable matching strategies
   - Dispatches execution tasks over gRPC streams (`proto/subnet/matcher.proto`)
   - **Batch Assignment Submission**: Buffers assignments and submits in batches to RootLayer (default: 10 assignments per batch or every 5 seconds)
   - Tracks pending assignments and runner-up fallback logic

2. **Agent (external SDK)**
   - Operators run agents via the published SDK (`subnet-sdk/go`, `subnet-sdk/python`)
   - Agents register with the registry service, subscribe to matcher tasks
   - Execute workloads with custom business logic
   - Submit execution reports to validators via gRPC
   - **SDK Batch Support**: Python and Go SDKs support batch bid and report submission

3. **Validator (`cmd/validator`, `internal/validator`)**
   - Exposes gRPC for execution report submission (`proto/subnet/validator.proto`)
   - Validates report signatures and payloads with custom validation logic
   - Persists state in LevelDB for crash recovery (`internal/storage`)
   - Participates in the threshold consensus FSM (`internal/consensus`)
   - Broadcasts signatures via NATS messaging layer (`internal/messaging`)
   - **Batch ValidationBundle Submission**: Aggregates multiple ValidationBundles and submits in batches to RootLayer
   - Dual submission support: Both blockchain and RootLayer simultaneously
   - Automatic fallback from batch to individual submission on errors

4. **Registry (`cmd/registry`, `internal/registry`)**
   - Tracks agent and validator registrations with health heartbeats
   - Provides discovery endpoints consumed by agents and validators (`/validators`, `/agents`)

5. **Mock RootLayer (`cmd/mock-rootlayer`)**
   - Supplies intents and accepts matcher or validator updates for local testing

## Key Features

### Batch Submission Support

**ValidationBundle Batching (Validator)**:
- Validators collect multiple ValidationBundles and submit them in a single RPC call
- Configurable batch size and flush interval
- Type assertion pattern detects batch API support: `batchClient, supportsBatch := client.(batchSubmitter)`
- Automatic graceful degradation to individual submission if batch API unavailable
- Partial success handling: `partial_ok` flag allows processing remaining bundles even if some fail

**Assignment Batching (Matcher)**:
- Matcher buffers assignments and flushes periodically (every 5s or when buffer reaches 10)
- Background worker handles batch submission without blocking matching operations
- Uses `CompleteClient.PostAssignmentBatch()` for efficient bulk submission

### RootLayer Client Architecture

**CompleteClient** (`internal/rootlayer/complete_client.go`):
- Unified client with full batch submission support
- Implements both `SubmitValidationBundleBatch()` and `PostAssignmentBatch()`
- Supports intent streaming, assignment retrieval, and validation bundle submission
- Used by both Matcher and Validator for high-throughput operations

**GRPCClient** (legacy):
- Basic gRPC client without batch support
- Individual submission only
- Retained for backwards compatibility

## Code Map

- `cmd/` – CLI entry points for matcher, validator, registry, mock rootlayer, and the example simple agent
- `internal/`
  - `matcher/` – bidding windows, matching engine, assignment manager, task streaming, and **batch assignment submission**
  - `validator/` – gRPC server, auth interceptor, execution report validation, and **batch ValidationBundle submission**
  - `consensus/` – threshold FSM, leader rotation, NATS broadcaster wrappers, epoch-based checkpointing
  - `rootlayer/` – HTTP/gRPC clients with **batch submission support** (CompleteClient)
  - `registry/` – in-memory registry service with health tracking
  - `storage/` – LevelDB helpers for validator persistence and crash recovery
  - `grpc/` – shared authentication interceptors and signing helpers
  - `logging/`, `metrics/`, `messaging/`, `types/`, `crypto/` – shared utilities
- `proto/` – generated protobufs for subnet and rootlayer services (`make proto` regenerates)
- `config/` – sample configuration files for matcher and validator nodes
- `scripts/` – deployment, testing, and E2E test scripts
- `docs/` – comprehensive documentation including batch testing guide

## Build & Test

```bash
cd Subnet
make build       # builds matcher, validator, registry, mock rootlayer, simple agent
make test        # runs Go unit tests across modules
make proto       # regenerates protobuf stubs from ../pin_protocol
```

For iterative work you can also run `go test ./...` or build specific binaries from `cmd/<component>`.

## Runtime Expectations

- **NATS Messaging**: Validators rely on NATS for signature gossip (default `nats://127.0.0.1:4222`)
- **Execution Reports**: Accepted over gRPC and revalidated before entering the threshold FSM
- **RootLayer Connectivity**: Matcher and validator require connectivity to RootLayer endpoints (gRPC: `3.17.208.238:9001`, HTTP: `http://3.17.208.238:8081`)
- **Batch Operations**: Both Matcher and Validator use `CompleteClient` for batch submission support
- **Dual Submission**: Validators can submit to both blockchain (Base Sepolia) and RootLayer simultaneously
- **Agent SDK**: Production agents should use `/subnet-sdk` (Go/Python); `cmd/simple-agent` is for demo only

## Batch Submission Flow

### Validator ValidationBundle Batching

```
ExecutionReports → Validator processing → Checkpoint creation (epoch-based)
                                                    ↓
                                          Multiple ValidationBundles
                                                    ↓
                                          Batch buffer (collect)
                                                    ↓
                              Flush trigger (threshold or timeout)
                                                    ↓
                        SubmitValidationBundleBatch() → RootLayer
                                    ↓
                        BatchResponse (success/failed counts)
                                    ↓
                  Process results + retry failed submissions
```

### Matcher Assignment Batching

```
Intent matching → Winner selection → Assignment creation
                                            ↓
                                Assignment batch buffer
                                            ↓
                        Periodic flush (5s or 10 items)
                                            ↓
                        PostAssignmentBatch() → RootLayer
                                            ↓
                                Batch confirmation
```

## Testing

### E2E Testing
Run the comprehensive E2E test suite:
```bash
# Simple E2E test
./run-e2e.sh --no-interactive

# Batch operations E2E test
./run-batch-e2e.sh --intent-count 10 --no-interactive
```

See `docs/batch_test.md` for detailed batch testing documentation.

### Unit Testing
```bash
make test        # Run all tests
go test ./...    # Direct test execution
```

## Configuration

### Matcher Configuration
```yaml
rootlayer:
  grpc_endpoint: "3.17.208.238:9001"
  http_url: "http://3.17.208.238:8081/api/v1"

# Batch assignment submission (automatic)
# Defaults: flush every 5s or when buffer reaches 10 assignments
```

### Validator Configuration
```yaml
rootlayer:
  grpc_endpoint: "3.17.208.238:9001"
  http_url: "http://3.17.208.238:8081/api/v1"

# Batch ValidationBundle submission (automatic)
# Triggered per epoch when checkpoint is created
```

## Supporting Notes

- See `docs/batch_test.md` for comprehensive batch operations testing guide
- `docs/jetstream_evaluation.md` captures NATS JetStream trade-offs for durable messaging
- Proto files under `proto/rootlayer` and `proto/subnet` are authoritative; regenerate with `make proto` when protocol changes
- Both Chinese (`*.zh.md`) and English documentation available in `docs/`
