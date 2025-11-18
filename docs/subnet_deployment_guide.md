# Subnet Complete Deployment and Custom Development Guide

**📍 You are here:** First-Time Setup → Subnet Deployment Guide (Advanced)

**Prerequisites:**
- Completed [Quick Start](quick_start.md) and chose manual deployment
- Completed [Environment Setup](environment_setup.md)

**Time to complete:** ~30 minutes (deployment + verification)

**What you'll learn:**
- Generate and manage validator keys
- Manually start matcher and validators
- Understand the complete intent execution flow
- Customize matcher strategies and validator logic
- Deploy to production with best practices

**Next steps after this guide:**
- ✅ Verify deployment → [Intent Execution Flow](#intent-execution-flow--observability)
- 🔧 Customize → [Custom Development Guide](#custom-development-guide)
- 🏭 Production → [Production Deployment](#production-deployment)

---

## Table of Contents

1. [Overview](#overview)
2. [Validator Key Management](#validator-key-management)
3. [Quick Deployment](#quick-deployment)
4. [Manual Validator Startup](#manual-validator-startup)
5. [Intent Execution Flow & Observability](#intent-execution-flow--observability)
6. [Custom Development Guide](#custom-development-guide)
   - [Matcher Strategy Customization](#matcher-strategy-customization)
   - [Validator Logic Customization](#validator-logic-customization)
   - [Agent Executor Customization](#agent-executor-customization)
7. [Production Deployment](#production-deployment)
8. [Troubleshooting](#troubleshooting)

---

## Overview

### Subnet Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        RootLayer                            │
│   (Blockchain + Intent Pool + Assignment + Validation)      │
└─────────────────┬───────────────────────────┬───────────────┘
                  │                           │
        Intents   │                           │  ValidationBundles
                  ↓                           ↑
┌─────────────────────────────┐   ┌──────────────────────────┐
│         Matcher             │   │      Validators          │
│  - Pull Intents             │   │  - Verify Reports        │
│  - Manage Bidding Windows   │   │  - Consensus Signatures  │
│  - Matching Algorithm       │   │  - Submit Bundles        │
│  - Assign Tasks to Agents   │   │  - Custom Logic          │
└──────────┬──────────────────┘   └────────┬─────────────────┘
           │ Assignments                   │ ExecutionReports
           ↓                               ↑
    ┌────────────────────────────────────────┐
    │            Agents (SDK)                │
    │  - Subscribe to Intents                │
    │  - Submit Bids                         │
    │  - Execute Tasks (Custom Logic)        │
    │  - Submit ExecutionReports             │
    └────────────────────────────────────────┘
```

### Components Requiring Customization

| Component | Required? | Customization |
|-----------|-----------|---------------|
| **Matcher Strategy** | Optional | How to select winner from agent bids |
| **Validator Logic** | **Required** | How to verify execution correctness |
| **Agent Executor** | **Required** | How to execute specific intent tasks |
| **Intent Type Definition** | **Required** | Define supported intent types and params |

---

## Validator Key Management

Each validator signs execution reports and ValidationBundles with an ECDSA key pair. You must provision keys before launching any node.

### Key formats

| Type | Format | Example |
|------|--------|---------|
| Private key | 64 hex chars (no `0x`) | `f9e2a1b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1` |
| Public key | 130 hex chars (uncompressed, prefix `04`) | `040b7dbae9ecfe88...249db3f90d2` |

### Generate keys

```bash
# Private key
openssl rand -hex 32

# Derive public key using helper binary
go build -o bin/derive-pubkey scripts/derive-pubkey.go
./bin/derive-pubkey <private_key_hex>
```

For multiple validators:

```bash
NUM_VALIDATORS=3
PRIVS=()
PUBS=()
for i in $(seq 1 $NUM_VALIDATORS); do
  PRIV=$(openssl rand -hex 32)
  PUB=$(./bin/derive-pubkey "$PRIV")
  PRIVS+=("$PRIV")
  PUBS+=("$PUB")
done
printf 'VALIDATOR_KEYS="%s"\n' "$(IFS=,; echo "${PRIVS[*]}")"
printf 'VALIDATOR_PUBKEYS="%s"\n' "$(IFS=,; echo "${PUBS[*]}")"
```

Add the generated lists to `.env` (already gitignored):

```bash
VALIDATOR_KEYS="key1,key2,key3"
VALIDATOR_PUBKEYS="pubkey1,pubkey2,pubkey3"
```

> Ordering matters: the first private key corresponds to the first public key and to `validator-1` (or your chosen ID). Every validator must share the same ordered `validator_id:pubkey` map.

---

## Quick Deployment

### Prerequisites

```bash
# Install dependencies
go version        # >= 1.21 required
docker --version  # Optional for Docker-based flows

# Clone repository
git clone https://github.com/PIN-AI/Subnet
cd Subnet

# Build binaries once
make build
```

### Step 1: Prepare environment

Copy the template and fill in secrets:

```bash
cp .env.example .env
```

Populate at least:

```bash
TEST_PRIVATE_KEY=<matcher_signing_key>
VALIDATOR_KEYS="key1,key2,key3"          # From the generator above
VALIDATOR_PUBKEYS="pubkey1,pubkey2,pubkey3"
SUBNET_ID=0x0000...0002                  # From scripts/create-subnet.sh
ROOTLAYER_GRPC=3.17.208.238:9001
ROOTLAYER_HTTP=http://3.17.208.238:8081
NUM_VALIDATORS=3
CONSENSUS_TYPE=raft                      # or cometbft
```

> `.env` is gitignored. For production, source values from a secure secret store instead of plaintext files.

If you have not created a subnet yet, run:

```bash
export RPC_URL="https://sepolia.base.org"
export PRIVATE_KEY="<your-chain-key>"
export PIN_NETWORK="base_sepolia"
./scripts/create-subnet.sh --name "My Test Subnet"
```

Record the returned subnet ID and contract address, then plug them into `.env`.

### Step 2: Launch everything

```bash
./scripts/start-subnet.sh
```

The launcher:

- Builds binaries if needed
- Starts matcher, validators, and optionally the sample agent
- Stores logs in `./subnet-logs/`
- Uses the validator keys and subnet configuration from `.env`

Set `TEST_MODE=true` to keep processes in the foreground for easier debugging.

### Step 3: Submit a test intent

```bash
./bin/submit-intent-signed \
  --subnet-id "$SUBNET_ID" \
  --intent-type "e2e-test" \
  --params '{"task":"ping"}' \
  --amount 100000000000000
```

Tail logs to confirm the flow:

```bash
tail -f subnet-logs/matcher.log \
         subnet-logs/validator-1.log \
         subnet-logs/agent.log
```

You should see bids arriving, a winner selected, the agent executing, and validators processing reports. See [Intent Execution Flow & Observability](#intent-execution-flow--observability) for expected log snippets.

---

## Manual Validator Startup

Use manual startup when you need to tweak ports, run validators on separate machines, or debug consensus behaviour.

### Required ports (Raft + Gossip)

| Purpose    | Flag            | Default        |
|------------|-----------------|----------------|
| gRPC API   | `--grpc`        | `9090`         |
| Metrics    | `--metrics`     | `9095`         |
| Raft bind  | `--raft-bind`   | `127.0.0.1:7000` |
| Gossip     | `--gossip-port` | `7946`         |

Every validator on the same host must use unique values for all of the above (not just `--grpc`).

### Single-node example (Raft)

```bash
./bin/validator \
  --id validator-1 \
  --subnet-id "$SUBNET_ID" \
  --key "$PRIVKEY" \
  --grpc 9090 \
  --metrics 9095 \
  --storage ./data/validator-1 \
  --rootlayer-endpoint 3.17.208.238:9001 \
  --enable-rootlayer \
  --validators 1 \
  --threshold-num 1 \
  --threshold-denom 1 \
  --validator-pubkeys "validator-1:$PUBKEY" \
  --raft-enable \
  --raft-bootstrap \
  --raft-bind 127.0.0.1:7000 \
  --raft-data-dir ./data/validator-1-raft \
  --gossip-enable \
  --gossip-bind 127.0.0.1 \
  --gossip-port 7946
```

### Multi-node cluster (3 validators, Raft)

Construct a shared string:

```bash
VALIDATOR_PUBKEYS="validator-1:$PUB1,validator-2:$PUB2,validator-3:$PUB3"
```

**Validator 1**

```bash
./bin/validator \
  --id validator-1 \
  --key "$PRIV1" \
  --grpc 9090 \
  --metrics 9095 \
  --storage ./data/val1-storage \
  --validators 3 \
  --threshold-num 2 \
  --threshold-denom 3 \
  --validator-pubkeys "$VALIDATOR_PUBKEYS" \
  --raft-enable \
  --raft-bootstrap \
  --raft-bind 127.0.0.1:7000 \
  --raft-peers "validator-1:127.0.0.1:7000,validator-2:127.0.0.1:7010,validator-3:127.0.0.1:7020" \
  --gossip-enable \
  --gossip-port 7946 \
  --gossip-seeds "127.0.0.1:7946,127.0.0.1:7956,127.0.0.1:7966" \
  --validator-endpoints "validator-1:localhost:9090,validator-2:localhost:9100,validator-3:localhost:9110"
```

**Validator 2** (increment every port family):

```bash
./bin/validator \
  --id validator-2 \
  --key "$PRIV2" \
  --grpc 9100 \
  --metrics 9105 \
  --raft-bind 127.0.0.1:7010 \
  --gossip-port 7956 \
  --storage ./data/val2-storage \
  --raft-data-dir ./data/val2-raft \
  --validators 3 \
  --threshold-num 2 \
  --threshold-denom 3 \
  --validator-pubkeys "$VALIDATOR_PUBKEYS" \
  --raft-enable \
  --gossip-enable \
  --validator-endpoints "validator-1:localhost:9090,validator-2:localhost:9100,validator-3:localhost:9110"
```

**Validator 3** – continue with `--grpc 9110`, `--metrics 9115`, `--raft-bind 127.0.0.1:7020`, `--gossip-port 7966`, and its own storage directories.

### CometBFT mode

```bash
mkdir -p ./cometbft-data/validator1
./scripts/generate-cometbft-keys.sh ./cometbft-data 1
./scripts/generate-cometbft-genesis.sh ./cometbft-data

./bin/validator \
  --id validator-1 \
  --consensus-type cometbft \
  --key "$PRIVKEY" \
  --grpc 9090 \
  --storage ./cometbft-data/validator1/storage \
  --cometbft-home ./cometbft-data/validator1/cometbft \
  --cometbft-moniker validator-1 \
  --cometbft-p2p-port 26656 \
  --cometbft-rpc-port 26657 \
  --cometbft-proxy-port 26658 \
  --validator-pubkeys "validator-1:$PUBKEY"
```

Repeat for additional nodes with unique ports and directories. CometBFT handles peer discovery through the seed/persistent peer lists in `config.toml`.

---

## Intent Execution Flow & Observability

```
SubmitIntent → IntentStream → AgentBidding → BidMatching → Assignment
           → Execution → ReportSubmit → Validation → Checkpoint → Final Result
```

| Stage | Where to watch | Sample log |
|-------|----------------|------------|
| Intent ingestion | `subnet-logs/matcher.log` | `Received intent intent_id=... type=...` |
| Bids & winners | `subnet-logs/matcher.log` | `Received bid ...` / `Selected winner ...` |
| Agent execution | `subnet-logs/agent.log` | `Executing task ... result_size=...` |
| Report submit | `subnet-logs/agent.log` / validator logs | `Submitting execution report ...` |
| Validation | `subnet-logs/validator-*.log` | `Processed execution report ... pending_reports=...` |
| Consensus & bundles | `subnet-logs/validator-*.log` | `Creating ValidationBundle ...`, `Submitting ValidationBundle ...` |
| RootLayer | `subnet-logs/rootlayer.log` (mock) | `Finalized bundle ...` |

Typical flow:

1. `submit-intent-signed` pushes an intent to the RootLayer.
2. Matcher streams intents, opens a bidding window, and runs the configured strategy.
3. Winning agent receives an assignment, executes the task, and serializes the result.
4. Agent submits an `ExecutionReport` (with `result_data`) to any validator.
5. Validators replicate reports (Raft or CometBFT), collect signatures, and build ValidationBundles.
6. Bundles are sent to the RootLayer and optionally the IntentManager contract when `--enable-chain-submit` is set.

If you see “transaction success” but no data, check the agent log for execution output and the validator log for `Processed execution report` entries. The validator stores the raw report in its LevelDB under `execution_reports/<report_id>` until the bundle is finalized.

---

## Custom Development Guide

### Matcher Strategy Customization

1. Implement a type that satisfies `matcher.BidMatchingStrategy` (see `internal/matcher/bid_strategy.go`). `SelectWinner` must return a winning `*pb.Bid` plus optional runner-ups; the matcher wraps these into `MatchingResult` later, so you never construct that struct yourself.
2. Register the strategy in `internal/matcher/config.go` or inject it via `matcher.NewServerWithStrategy`. You can use config to choose between built-in strategies (`lowest_price`, weighted scoring) or your custom identifier.
3. Rebuild `cmd/matcher` and restart the service. Keep unit tests close to the strategy package so they can be run with `go test ./...`.

### Validator Logic Customization

1. Extend the execution report verification pipeline (see `internal/validator/report_validator.go`). Most teams add intent-specific checks that deserialize `ExecutionReport.ResultData`, validate business rules, and return errors when mismatches occur.
2. Register custom validators by intent type before starting the node. The simplest approach is to maintain a map of `intent_type -> validation func` and wire it into `validator.NewNode`.
3. When validators reject reports, the matcher automatically reassigns runner-ups, so log detailed reasons to simplify debugging.

### Agent Executor Customization

- Use the official SDKs in the [PIN-AI subnet-sdk repo](https://github.com/PIN-AI/subnet-sdk) (`go/`, `python/`) as a starting point. The demo `cmd/simple-agent` shows how to subscribe, bid, execute, and submit reports.
- Embed metadata (price, region, capability) in `pb.Bid.Metadata` so custom matcher strategies can reason about them.
- Persist execution outputs and metadata long enough to retry submissions in case the validator connection drops.

Once these components are customized, follow the deployment steps above to roll them out.

## Production Deployment

### Security Checklist

- [ ] Use KMS for private key management (don't hardcode)
- [ ] Enable TLS/mTLS for gRPC
- [ ] Configure firewall rules
- [ ] Enable blockchain participant verification
- [ ] Set up log collection (ELK/Prometheus)
- [ ] Configure monitoring and alerts
- [ ] Set up auto-restart (systemd/Kubernetes)

### Docker Deployment Example

**Matcher Dockerfile:**

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o matcher ./cmd/matcher

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/matcher /usr/local/bin/
COPY config/config.yaml /etc/subnet/

CMD ["matcher", "--config", "/etc/subnet/config.yaml"]
```

**Docker Compose:**

```yaml
version: '3.8'

services:
  matcher:
    build:
      context: .
      dockerfile: Dockerfile.matcher
    environment:
      - SUBNET_ID=${SUBNET_ID}
      - PRIVATE_KEY=${MATCHER_KEY}

  validator-1:
    build:
      context: .
      dockerfile: Dockerfile.validator
    environment:
      - SUBNET_ID=${SUBNET_ID}
      - VALIDATOR_ID=validator-1
      - PRIVATE_KEY=${VALIDATOR_1_KEY}

  validator-2:
    build:
      context: .
      dockerfile: Dockerfile.validator
    environment:
      - SUBNET_ID=${SUBNET_ID}
      - VALIDATOR_ID=validator-2
      - PRIVATE_KEY=${VALIDATOR_2_KEY}

  validator-3:
    build:
      context: .
      dockerfile: Dockerfile.validator
    environment:
      - SUBNET_ID=${SUBNET_ID}
      - VALIDATOR_ID=validator-3
      - PRIVATE_KEY=${VALIDATOR_3_KEY}
```

Start:

```bash
docker-compose up -d
```

---

## Troubleshooting

| Issue | Symptoms | Resolution |
|-------|----------|------------|
| `--config` flag error | `flag provided but not defined: -config` | Binaries only accept explicit flags. Use `.env` + `start-subnet.sh` or pass flags manually (see [Manual Validator Startup](#manual-validator-startup)). |
| Port conflicts | Second validator fails even after changing `--grpc` | Update **all** ports (`--metrics`, `--raft-bind`, `--gossip-port`, CometBFT ports). Consider a script that offsets ports per node. |
| Unknown validator/public key | Validator rejects reports or agents | Ensure every node shares the same ordered `validator_id:pubkey` map from [Validator Key Management](#validator-key-management). |
| “Transaction success” but no output | Agent claims success but nothing in validator log | Follow the [Intent Execution Flow & Observability](#intent-execution-flow--observability) checklist and inspect `subnet-logs/agent.log` + `subnet-logs/validator-*.log` for `Processed execution report`. |
| Bundles never finalize | Validators keep collecting reports without submission | Verify quorum (`threshold_num/threshold_denom`), Raft peer connectivity, and Gossip ports. Enable debug logs if signatures stop at < threshold. |
| Validators stuck joining | Repeated Raft bootstrap or “node already exists” | Ensure only the first node uses `--raft-bootstrap`, delete stale `raft-data` dirs when changing IDs, and keep `--validator-pubkeys` identical across nodes. |

Useful commands:

```bash
# Matcher intents/bids/winners
tail -f subnet-logs/matcher.log

# Agent execution + report submission
tail -f subnet-logs/agent.log

# Validator consensus
tail -f subnet-logs/validator-1.log
```

Need deeper insight? Refer to `docs/architecture.md` for component internals or reach out on GitHub Issues.
