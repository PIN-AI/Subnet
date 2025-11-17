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

**Choose your path:** New users start with First-Time Setup. Developers and operators can jump directly to relevant sections below.

> ℹ️ **Contract Address Update (2025-11-03):** Base Sepolia addresses were refreshed. Use values from `.env.example` or `deployment/env.template` to avoid deprecated contracts.

---

### 🚀 First-Time Setup (Essential, ~20 min)

1. **[Quick Start](docs/quick_start.md)** – Choose deployment method + registration workflow
2. **[Environment Setup](docs/environment_setup.md)** – Install Go, Docker, dependencies
3. **Deploy** (pick one):
   - ⭐ **Recommended**: [Docker Deployment](docker/README.md) – 3-node cluster in 5 minutes
   - 🔧 **Advanced**: [Subnet Deployment Guide](docs/subnet_deployment_guide.md) – Manual setup with full control

> ✅ **After deployment**, continue with "Verify & Monitor" below to understand the execution flow.

---

### 🔍 Verify & Monitor (After Deployment)

- **[Intent Execution Flow](docs/subnet_deployment_guide.md#intent-execution-flow--observability)** – How intents flow through the system
- **[Troubleshooting](docs/subnet_deployment_guide.md#troubleshooting)** – Common issues and solutions
- **[Scripts Guide](docs/scripts_guide.md)** – Helper scripts reference

---

### 🔧 Customize & Develop

**Understanding the System:**
- [Architecture Overview](docs/architecture.md) – Component design and data flow
- [Consensus Modes](docs/consensus_modes.md) – Raft+Gossip vs CometBFT comparison
- [Consensus Data Format](docs/consensus_data_format_compatibility.md) – Internal data structures

**Customization Guides:**
- [Matcher Strategy](docs/subnet_deployment_guide.md#matcher-strategy-customization) – Custom bid matching logic
- [Validator Logic](docs/subnet_deployment_guide.md#validator-logic-customization) – Custom validation rules
- [Agent SDK](https://github.com/PIN-AI/subnet-sdk) – Build agents using the Go/Python SDKs (separate repository)

---

### 🏭 Production & Operations

- [Production Deployment](docs/subnet_deployment_guide.md#production-deployment) – Best practices and checklists
- [Deployment Playbook](deployment/README.md) – Operations runbook
- [Security Notes](#security-notes) – Security checklist (see below)

---

### 📁 Reference & History

- [Analysis Reports](analysis-reports/README.md) – Codebase analysis and exploration summaries

## Layout

- `cmd/matcher` – matcher gRPC server with bidding windows and task streams
- `cmd/validator` – validator node receiving execution reports and broadcasting signatures
- `cmd/registry` – lightweight discovery service for agents and validators
- `cmd/mock-rootlayer` – mock RootLayer for local intent generation
- `cmd/simple-agent` – demo agent built on the Go SDK (production agents should live in the [Agent SDK repo](https://github.com/PIN-AI/subnet-sdk))
- `internal/` – shared packages (matcher, validator, consensus FSM, rootlayer client, storage, grpc interceptors, logging, metrics, messaging, types, crypto)
- `proto/` – generated protobufs for subnet and rootlayer APIs
- `config/` – sample validator configuration (`config.yaml`)
- `docs/` – comprehensive documentation (guides, architecture, troubleshooting)

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

### Quick Start (Recommended)

Use the automated startup script:

```bash
# Copy and configure environment
cp .env.example .env
# Edit .env and fill in VALIDATOR_KEYS and VALIDATOR_PUBKEYS (see the "Validator Keys" section below)

# Start complete subnet (matcher + validators + registry)
./scripts/start-subnet.sh
```

### Manual Service Startup (Advanced)

For development and debugging:

```bash
# Terminal 1 – Registry (Raft mode only)
./bin/registry -grpc :8091 -http :8101

# Terminal 2 – Matcher
./bin/matcher -grpc :8090 -http :8091

# Terminal 3 – Validator
# Note: validator requires many parameters. See docs/subnet_deployment_guide.md for details
./bin/validator \
  -id validator-1 \
  -key <your_private_key_hex> \
  -grpc 9090 \
  -subnet-id 0x0000000000000000000000000000000000000000000000000000000000000003 \
  -validators 1 \
  -threshold-num 1 \
  -threshold-denom 1 \
  -validator-pubkeys "validator-1:<your_public_key_hex>" \
  -rootlayer-endpoint 3.17.208.238:9001 \
  -enable-rootlayer

# Optional – Demo agent (uses subnet-sdk/go internally)
./bin/simple-agent -matcher localhost:8090 -subnet-id 0x... -id my-agent -name MyAgent
```

Refer to `docs/scripts_guide.md` for automation details. Production agents should use the separate SDK repositories at https://github.com/PIN-AI/subnet-sdk (Go and Python implementations).

### Validator Keys

Each validator requires an ECDSA key pair:

```bash
# Generate a 32-byte private key (hex-encoded without 0x)
PRIVKEY=$(openssl rand -hex 32)

# Derive the uncompressed public key (requires bin/derive-pubkey)
PUBKEY=$(./bin/derive-pubkey "$PRIVKEY")

echo "Private:  $PRIVKEY"
echo "Public :  $PUBKEY"
```

Populate `.env` with comma-separated lists following the validator order:

```bash
VALIDATOR_KEYS="key1,key2,key3"
VALIDATOR_PUBKEYS="pubkey1,pubkey2,pubkey3"
```

### On-Chain Participant Verification

The matcher and validator can optionally verify participants against the Subnet contract. Set the `blockchain` section in `config/config.yaml` or the `CHAIN_*` environment variables (`CHAIN_ENABLED`, `CHAIN_RPC_URL`, `SUBNET_CONTRACT_ADDRESS`, `CHAIN_ENABLE_FALLBACK`, `ALLOW_UNVERIFIED_AGENTS`) to enable it. A helper script `scripts/register_subnet_components.go` registers matchers or validators on-chain, while `scripts/check_registration.go` inspects the current on-chain status.

## Protobuf Regeneration

```bash
make proto
```

This regenerates Go protobuf code from the proto definitions. The generated files are already included in the repository.

## Security Notes

- Demo keys or mock credentials in this repo are for local testing only.
- Enable TLS/mTLS for gRPC services before exposing them publicly.
- Validators rely on threshold attestation; monitor Raft consensus health and persisted LevelDB state to avoid data loss.
