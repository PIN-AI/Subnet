# Subnet (Go) MVP

Go implementation of a PinAI Subnet that coordinates matcher, validator, and registry services. The matcher distributes intents pulled from the RootLayer, agents execute work using the published SDKs, and validators verify execution reports before assembling threshold validation bundles.

See `docs/ARCHITECTURE_OVERVIEW.md` for a full component walkthrough.

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
