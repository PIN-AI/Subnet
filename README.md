# PinAI Subnet Template

**Production-ready Subnet template** for building custom intent execution networks on PinAI protocol.

## 🌐 What is a Subnet?

A **Subnet** is the execution layer in the PinAI protocol where tasks get done. Think of it as a decentralized task marketplace:

```
User submits Intent (task + budget)
        ↓
   [Matcher] ─── Collects bids from Agents, selects winner
        ↓
    [Agent] ─── Executes the task, submits result
        ↓
  [Validator] ─── Verifies result, reaches consensus, anchors to blockchain
        ↓
User receives verified result
```

**Key Roles:**

| Role | What it does | Analogy |
|------|--------------|---------|
| **Matcher** | Coordinates task assignment, runs auctions | Auction house |
| **Agent** | Executes tasks (AI inference, compute, etc.) | Service provider |
| **Validator** | Verifies results, ensures honesty | Referee |

---

## 🚀 What is This Repo?

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

> 💡 **This is a Template** – Fork this repo and customize everything to fit your use case!

**🤖 For Agent Developers (Service Providers):**

Want to provide services on an existing subnet? You only need to develop an Agent – no infrastructure required.

| Resource | Description |
|----------|-------------|
| [Agent SDK (Go/Python)](https://github.com/PIN-AI/subnet-sdk) | Build your own Agent |
| [Quick Start](https://github.com/PIN-AI/subnet-sdk/blob/main/docs/quick-start.md) | Get your first Agent running in 5 minutes |
| [Tutorial](https://github.com/PIN-AI/subnet-sdk/blob/main/docs/tutorial.md) | Complete development guide |

Your Agent can implement any business logic: AI inference, data processing, blockchain indexing, etc.

**🏗️ For Subnet Operators (Infrastructure):**

Running your own subnet? Customize these components:

- [Matcher Strategy](docs/subnet_deployment_guide.md#matcher-strategy-customization) – Custom bid matching logic
- [Validator Logic](docs/subnet_deployment_guide.md#validator-logic-customization) – Custom validation rules
- [Consensus Modes](docs/consensus_modes.md) – Choose Raft+Gossip or CometBFT

**📖 Understanding the System:**

- [Architecture Overview](docs/architecture.md) – Component design and data flow
- [Consensus Data Format](docs/consensus_data_format_compatibility.md) – Internal data structures

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
```

You can also build individual binaries:

```bash
go build -o bin/validator ./cmd/validator
go build -o bin/matcher   ./cmd/matcher
go build -o bin/mock-rootlayer ./cmd/mock-rootlayer
go build -o bin/simple-agent   ./cmd/simple-agent
```

## Running the Services

```bash
# 1. Configure environment
cp .env.example .env
# Edit .env with your keys (see docs/quick_start.md)

# 2. Start subnet
./scripts/start-subnet.sh
```

📚 **Complete guide**: See [Quick Start](docs/quick_start.md) for key generation, registration, and deployment options.


## Security Notes

- Demo keys or mock credentials in this repo are for local testing only.
- Enable TLS/mTLS for gRPC services before exposing them publicly.
- Validators rely on threshold attestation; monitor Raft consensus health and persisted LevelDB state to avoid data loss.
