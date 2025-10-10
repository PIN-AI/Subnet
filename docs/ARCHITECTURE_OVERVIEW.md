# Subnet Architecture Overview

This Go codebase implements the execution layer for a PinAI Subnet. The Matcher assigns intents to agent operators, agents execute work via the standalone SDK, and Validators verify execution results before reporting aggregates back to the RootLayer.

## High-Level Flow

```
RootLayer intents → Matcher → Agent (SDK) → Validator → RootLayer validation bundle
```

1. **Matcher (`cmd/matcher`, `internal/matcher`)**
   - Pulls intents from the RootLayer client (`internal/rootlayer`)
   - Maintains bidding windows per intent and collects agent bids
   - Selects a winning agent and dispatches execution tasks over gRPC streams (`proto/subnet/matcher.proto`)
   - Tracks pending assignments and runner-up fallback logic

2. **Agent (external SDK)**
   - Operators run agents via the published SDK (`subnet-sdk/go`, `subnet-sdk/python`)
   - Agents register with the registry service, subscribe to matcher tasks, execute workloads, and submit execution reports to validators’ HTTP endpoints (`/api/v1/execution-report`)

3. **Validator (`cmd/validator`, `internal/validator`)**
   - Exposes gRPC for execution report submission (`proto/subnet/validator.proto`)
   - Validates report signatures and payloads, persists state in LevelDB (`internal/storage`)
   - Participates in the threshold consensus FSM (`internal/consensus`) and broadcasts signatures via the messaging layer (`internal/messaging`, NATS)
   - Aggregates signatures into `ValidationBundle` messages defined under `proto/rootlayer`

4. **Registry (`cmd/registry`, `internal/registry`)**
   - Tracks agent and validator registrations with health heartbeats
   - Provides discovery endpoints consumed by agents and validators (`/validators`, `/agents`)

5. **Mock RootLayer (`cmd/mock-rootlayer`)**
   - Supplies intents and accepts matcher or validator updates for local testing

## Code Map

- `cmd/` – CLI entry points for matcher, validator, registry, mock rootlayer, and the example simple agent
- `internal/`
  - `matcher/` – bidding windows, matching engine, assignment manager, and task streaming
  - `validator/` – gRPC server, auth interceptor, execution report validation
  - `consensus/` – threshold FSM, leader rotation, NATS broadcaster wrappers
  - `rootlayer/` – HTTP/gRPC clients for RootLayer coordination
  - `registry/` – in-memory registry service with health tracking
  - `storage/` – LevelDB helpers for validator persistence
  - `grpc/` – shared authentication interceptors and signing helpers
  - `logging/`, `metrics/`, `messaging/`, `types/`, `crypto/` – shared utilities
- `proto/` – generated protobufs for subnet and rootlayer services (`make proto` regenerates)
- `config/` – sample configuration (`config.yaml`) for validator nodes

## Build & Test

```bash
cd Subnet
make build       # builds matcher, validator, registry, mock rootlayer, simple agent
make test        # runs Go unit tests across modules
make proto       # regenerates protobuf stubs from ../pin_protocol
```

For iterative work you can also run `go test ./...` or build specific binaries from `cmd/<component>`.

## Runtime Expectations

- Validators rely on NATS for signature gossip (default `nats://127.0.0.1:4222`).
- Execution reports are accepted over HTTP and revalidated before entering the threshold FSM.
- Matcher and validator both require connectivity to the RootLayer client endpoints.
- Agents should use the SDK repos in `/subnet-sdk` for production integrations; the `cmd/simple-agent` binary is only a local demo.

## Supporting Notes

- `docs/jetstream_evaluation.md` captures the trade-offs around adopting NATS JetStream for durable messaging.
- The proto files under `proto/rootlayer` and `proto/subnet` are the single source of truth for service interfaces; regenerate them whenever protocol definitions change in `../pin_protocol`.
