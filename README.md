# PinAI Subnet Template

**Production-ready Subnet template** for building custom intent execution networks on PinAI protocol. This implementation coordinates matcher, validator, and registry services with built-in batch submission support for high-throughput operations.

## 🚀 What is This?

This is a **template** for creating your own Subnet. Fork this repository to:
- Build specialized intent execution networks (e.g., image processing, data computation, AI inference)
- Customize matching strategies for your use case
- Implement domain-specific validation logic
- Deploy production-grade infrastructure

## ✨ Key Features

- **Dual Consensus Options**: Choose between Raft+Gossip or CometBFT (Tendermint) consensus engines
- **CometBFT Integration**: Production-grade BFT consensus with P2P validator discovery
- **Batch Operations**: High-performance batch submission for ValidationBundles and Assignments
- **Flexible Matching**: Pluggable matching strategies (price-based, reputation-based, geo-location, etc.)
- **Threshold Consensus**: Byzantine fault-tolerant validator consensus with configurable thresholds
- **Dual Submission**: Simultaneous blockchain and RootLayer submission for redundancy
- **Production Ready**: Docker support, comprehensive monitoring, and production deployment guides

## 📚 Documentation

### Getting Started
- **[Quick Start Guide](docs/quick_start.md)** - Get your Subnet running in 5 minutes
- **[CometBFT Quick Start](docs/COMETBFT_QUICKSTART.md)** - Start validators with CometBFT consensus
- **[E2E Test Guide](docs/e2e_test_guide.md)** - End-to-end testing workflow

### Deployment & Customization
- **[Subnet Deployment Guide](docs/subnet_deployment_guide.md)** - Complete deployment and customization tutorial
  - Quick deployment with default configuration
  - Custom matcher strategy development
  - Custom validator verification logic
  - Custom agent executor development
  - Production deployment guide
- **[CometBFT Deployment Guide](docs/cometbft_deployment_guide.md)** - CometBFT consensus deployment
  - Static configuration for testing
  - P2P seeds for production
  - Performance tuning
  - Security hardening

### Architecture & Testing
- **[Architecture Overview](docs/architecture.md)** - Full component walkthrough and system design
- **[Batch Operations Testing](docs/batch_test.md)** - Testing batch submission features (ValidationBundles, Assignments)
- **[Scripts Guide](docs/scripts_guide.md)** - Development and deployment scripts reference

## Layout

- `cmd/matcher` – matcher gRPC server with bidding windows and task streams
- `cmd/validator` – validator node receiving execution reports and broadcasting signatures
- `cmd/registry` – lightweight discovery service for agents and validators
- `cmd/mock-rootlayer` – mock RootLayer for local intent generation
- `cmd/simple-agent` – demo agent built on the Go SDK (production agents should live in `../subnet-sdk`)
- `internal/` – shared packages (matcher, validator, consensus FSM, rootlayer client, storage, grpc interceptors, logging, metrics, messaging, types, crypto)
- `proto/` – generated protobufs for subnet and rootlayer APIs (authoritative definitions in `../pin_protocol/proto`)
- `config/` – sample validator configuration (`config.yaml`)
- `docs/` – curated documentation (`ARCHITECTURE_OVERVIEW.md`, `jetstream_evaluation.md`)

## Build & Test

```bash
cd Subnet
make build       # builds matcher, validator, registry, mock-rootlayer, simple-agent
make test        # go test ./...
make proto       # regenerate Go protobufs from ../pin_protocol
```

You can also build individual binaries:

```bash
go build -o bin/validator ./cmd/validator
go build -o bin/matcher   ./cmd/matcher
go build -o bin/registry  ./cmd/registry
go build -o bin/mock-rootlayer ./cmd/mock-rootlayer
go build -o bin/simple-agent   ./cmd/simple-agent
```

## Running the Services

Typical local loop (requires a running NATS server if you enable validator consensus broadcasting):

```bash
# Terminal 1 – Registry
go run ./cmd/registry --http :8092 --grpc :8091

# Terminal 2 – Matcher
go run ./cmd/matcher --grpc :8090 --bidding-window 10

# Terminal 3 – Validator
go run ./cmd/validator --config config/config.yaml

# Optional – Mock RootLayer for intents
go run ./cmd/mock-rootlayer --http :9090

# Optional – Demo agent (uses subnet-sdk/go internally)
go run ./cmd/simple-agent --matcher localhost:8090 --name demo-agent
```

Production agents should use the separate SDK repositories in `../subnet-sdk` (Go and Python implementations).

### On-Chain Participant Verification

The matcher and validator can optionally verify participants against the Subnet contract. Set the `blockchain` section in `config/config.yaml` or the `CHAIN_*` environment variables (`CHAIN_ENABLED`, `CHAIN_RPC_URL`, `SUBNET_CONTRACT_ADDRESS`, `CHAIN_ENABLE_FALLBACK`, `ALLOW_UNVERIFIED_AGENTS`) to enable it. A helper script `scripts/register_subnet_components.go` registers matchers or validators on-chain, while `scripts/check_registration.go` inspects the current on-chain status.

## Protobuf Regeneration

```bash
make proto
# or
protoc -I ../pin_protocol \
  --go_out=paths=source_relative:. \
  --go-grpc_out=paths=source_relative:. \
  ../pin_protocol/proto/subnet/*.proto
```

Regenerate `proto/rootlayer` and `proto/common` targets as needed; commit generated files alongside protocol changes.

## Security Notes

- Demo keys or mock credentials in this repo are for local testing only.
- Enable TLS/mTLS for gRPC services before exposing them publicly.
- Validators rely on threshold attestation; monitor NATS connectivity and persisted LevelDB state to avoid data loss.
