# PinAI Subnet Template

**Production-ready Subnet template** for building custom intent execution networks on PinAI protocol. This implementation coordinates matcher and validator services with built-in batch submission support for high-throughput operations.

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

## 🎯 Built for Flexibility, Designed for Scale

**PinAI Subnet is not just a template – it's a modular framework.** Every layer is designed to be swapped, customized, or replaced while maintaining seamless compatibility with the broader PinAI network.

### Your Network, Your Rules

The only requirement? **Speak the protocol.** As long as your components implement the standardized gRPC interfaces and protobuf message formats, you have unlimited freedom to optimize for your use case:

- **🤖 Agent Logic**: Build specialized execution engines – AI inference, video rendering, blockchain indexing, or anything your agents can compute
- **⚖️ Consensus Mechanism**: Choose Raft for simplicity, CometBFT for Byzantine fault tolerance, or roll your own consensus algorithm
- **💾 Storage Backend**: Pick embedded LevelDB for speed, PostgreSQL for rich queries, or S3 for infinite scale
- **🎯 Matching Strategy**: Optimize for lowest price, best reputation, geographic proximity, or multi-dimensional scoring

### Protocol-Driven Modularity

```
┌─────────────────────────────────────────────────────────┐
│         PinAI Protocol Interface (gRPC + Protobuf)      │
│           ✅ Standardized Message Formats               │
│           ✅ Signature & Verification Specs             │
│           ✅ Checkpoint Anchoring Format                │
└─────────────────────────────────────────────────────────┘
                            ↓
┌─────────────────────────────────────────────────────────┐
│              Your Custom Implementation                 │
│                💡 Performance Optimizations             │
│                💡 Domain-Specific Logic                 │
│                💡 Infrastructure Preferences            │
└─────────────────────────────────────────────────────────┘
```

**The Result?** A subnet that's uniquely yours, yet fully interoperable with the entire PinAI ecosystem.

📚 **Learn More**: [Customization Guide →](docs/subnet_deployment_guide.md#customization)

## 📚 Documentation

**Choose your path:** New users start with First-Time Setup. Developers and operators can jump directly to relevant sections below.

> ℹ️ **Contract Address Update (2025-11-03):** Base Sepolia addresses were refreshed. Use values from `.env.example` to avoid deprecated contracts.

---

### 🚀 First-Time Setup (Essential, ~20 min)

> ⚠️ **PREREQUISITE: Get Testnet ETH First!**
>
> You need **at least 0.05 ETH** on Base Sepolia testnet before proceeding:
> - **Why**: Creating subnet + registering components requires ~0.035 ETH (stake + gas)
> - **Get testnet ETH**: [Coinbase Faucet](https://www.coinbase.com/faucets/base-ethereum-goerli-faucet) or bridge from Sepolia
> - **Verify**: Check your balance at [Base Sepolia Explorer](https://sepolia.basescan.org/)

1. **[Environment Setup](docs/environment_setup.md)** – Install Go, Docker, dependencies
2. **[Quick Start](docs/quick_start.md)** – Choose deployment method + registration workflow
3. **Deploy** (pick one):
   - ⭐ **Recommended**: [Docker Deployment](deployment/README.md) – 3-node cluster in 5 minutes
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
- `cmd/mock-rootlayer` – mock RootLayer for local intent generation
- `cmd/simple-agent` – demo agent built on the Go SDK (production agents should live in the [Agent SDK repo](https://github.com/PIN-AI/subnet-sdk))
- `internal/` – shared packages (matcher, validator, consensus FSM, rootlayer client, storage, grpc interceptors, logging, metrics, messaging, types, crypto)
- `proto/` – generated protobufs for subnet and rootlayer APIs
- `config/` – configuration templates (`subnet.yaml.template`, `validator.yaml.template`, `blockchain.yaml.template`)
- `docs/` – comprehensive documentation (guides, architecture, troubleshooting)

## Build & Test

```bash
cd Subnet
make build       # builds matcher, validator, mock-rootlayer, simple-agent
make test        # go test ./...
make proto       # regenerate Go protobufs from ../pin_protocol
```

## Logs and Debugging

After starting the subnet, logs are written to `./subnet-logs/`:

```
subnet-logs/
├── matcher.log        # Intent ingestion, bid matching
├── validator-1.log    # Consensus, validation, checkpoint
├── validator-2.log
├── validator-3.log
├── agent.log          # Task execution, results
└── rootlayer.log      # RootLayer mock (if used)
```

**View logs:**
```bash
# Watch all logs
tail -f subnet-logs/*.log

# Watch specific service
tail -f subnet-logs/matcher.log
tail -f subnet-logs/validator-1.log

# Search for events
grep "Received intent" subnet-logs/matcher.log
grep "ValidationBundle" subnet-logs/validator-*.log

# Docker logs
docker compose logs -f
docker compose logs -f validator-1
```

📚 **Complete debugging guide**: See [`docs/quick_start.md`](docs/quick_start.md#-log-files-and-debugging) for log patterns, tracing intents, and troubleshooting common issues.

You can also build individual binaries:

```bash
go build -o bin/validator ./cmd/validator
go build -o bin/matcher   ./cmd/matcher
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

# Start complete subnet (matcher + validators + agent)
./scripts/start-subnet.sh
```

### Manual Service Startup (Advanced)

For development and debugging:

```bash
# Terminal 1 – Matcher
./bin/matcher -grpc :8090 -http :8091

# Terminal 2 – Validator
# Option 1: Use hierarchical config files (recommended)
./bin/validator --subnet-config config/subnet.yaml --validator-config config/validator-1.yaml --key <your_private_key_hex>

# Option 2: Use command-line parameters only (advanced)
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

# IMPORTANT for multi-node deployment:
# - Do NOT use config file for multiple validators (port conflicts)
# - Use ./scripts/start-subnet.sh which automatically handles port allocation
# - Or create separate config files (config/validator-1.yaml, validator-2.yaml, etc.)
#
# Note: Command-line parameters override config file values

# Optional – Demo agent (uses subnet-sdk/go internally)
./bin/simple-agent -matcher localhost:8090 -subnet-id 0x... -id my-agent -name MyAgent
```

Refer to `docs/scripts_guide.md` for automation details. Production agents should use the separate SDK repositories at https://github.com/PIN-AI/subnet-sdk (Go and Python implementations).

### Validator Keys

Each validator requires an ECDSA key pair for signing consensus messages:
- **Private key**: 64 hexadecimal characters (kept secret, used for signing)
- **Public key**: 130 hexadecimal characters starting with `04` (shared with other validators)

**Generate keys for 3 validators:**

```bash
# 1. Build the derive-pubkey tool (if not already built)
make build

# 2. Generate private keys
PRIVKEY1=$(openssl rand -hex 32)
PRIVKEY2=$(openssl rand -hex 32)
PRIVKEY3=$(openssl rand -hex 32)

# 3. Derive public keys from private keys
PUBKEY1=$(./bin/derive-pubkey "$PRIVKEY1")
PUBKEY2=$(./bin/derive-pubkey "$PRIVKEY2")
PUBKEY3=$(./bin/derive-pubkey "$PRIVKEY3")

# 4. Display for verification
echo "Validator 1:"
echo "  Private: $PRIVKEY1"
echo "  Public:  $PUBKEY1"
echo ""
echo "Validator 2:"
echo "  Private: $PRIVKEY2"
echo "  Public:  $PUBKEY2"
echo ""
echo "Validator 3:"
echo "  Private: $PRIVKEY3"
echo "  Public:  $PUBKEY3"
```

**Populate `.env` with comma-separated lists:**

```bash
VALIDATOR_KEYS="$PRIVKEY1,$PRIVKEY2,$PRIVKEY3"
VALIDATOR_PUBKEYS="$PUBKEY1,$PUBKEY2,$PUBKEY3"
```

**Key Format Requirements:**
- ✅ Private key: Exactly 64 hex characters (0-9, a-f)
- ✅ Public key: Exactly 130 hex characters starting with `04`
- ✅ No `0x` prefix needed
- ✅ Keys must match the order: first private key → first public key

**Troubleshooting:**
- "derive-pubkey: command not found" → Run `make build` first
- "Error decoding private key" → Check private key is exactly 64 hex characters
- Keys don't work → Ensure VALIDATOR_KEYS and VALIDATOR_PUBKEYS are in the same order

**Security:**
- ⚠️ Use test-only keys with NO real funds
- ⚠️ Never commit `.env` to git (already in `.gitignore`)
- ⚠️ Each validator must have a unique private key

📚 See [`docs/quick_start.md`](docs/quick_start.md) for complete key generation walkthrough.

### On-Chain Participant Verification

The matcher and validator can optionally verify participants against the Subnet contract. Set the `blockchain` section in `config/blockchain.yaml` or the `CHAIN_*` environment variables (`CHAIN_ENABLED`, `CHAIN_RPC_URL`, `SUBNET_CONTRACT_ADDRESS`, `CHAIN_ENABLE_FALLBACK`, `ALLOW_UNVERIFIED_AGENTS`) to enable it. Helper scripts `scripts/create-subnet.sh` and `scripts/register.sh` handle blockchain operations using the SDK.

## Protobuf Regeneration

```bash
make proto
```

This regenerates Go protobuf code from the proto definitions. The generated files are already included in the repository.

## Security Notes

- Demo keys or mock credentials in this repo are for local testing only.
- Enable TLS/mTLS for gRPC services before exposing them publicly.
- Validators rely on threshold attestation; monitor Raft consensus health and persisted LevelDB state to avoid data loss.
