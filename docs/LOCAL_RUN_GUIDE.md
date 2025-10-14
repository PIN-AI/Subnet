# Local Subnet Quickstart

This guide explains how to start and stop a minimal Subnet stack on a single
machine. It relies on the helper script `scripts/subnet-local.sh`, which wraps
mock RootLayer, matcher, validator, and a demo agent.

## Prerequisites

1. **Go toolchain** – ensure `go version` reports 1.22 or newer.
2. **NATS** – the script will reuse an existing server on `nats://127.0.0.1:4222`.
   If no server is listening it will try to launch `nats-server` locally; install
   it via Homebrew (`brew install nats-server`) or grab the binary from
   https://docs.nats.io. Docker-based NATS is also fine.
3. **Built binaries** – run `make build` at least once so `bin/mock-rootlayer`,
   `bin/matcher`, `bin/validator`, and `bin/simple-agent` exist.

All components use the sample private keys already present in the repository;
replace them before connecting to a shared or production network.

## Start Everything

```bash
cd /Users/ty/pinai/protocol/Subnet
./scripts/subnet-local.sh start
```

What happens:

- Starts NATS if it is not already available on port 4222.
- Launches `mock-rootlayer` on `http://127.0.0.1:9090`.
- Launches a matcher on `localhost:8090` with a 10s bidding window.
- Launches one validator (`validator-local-1`) on `localhost:9200` with a single
  signer and threshold 1/1. On-chain submission and RootLayer gRPC are disabled for
  this preset.
- Launches `simple-agent` connecting to the matcher.
- Creates log files under `local-run/logs/` and records PIDs in `local-run/pids`.

Use `./scripts/subnet-local.sh status` to check which processes are running.

## Stop Everything

```bash
./scripts/subnet-local.sh stop
```

This kills all processes recorded in `local-run/pids` (matcher, validator, mock
rootlayer, agent, and any embedded NATS server started by the script) and then
removes the PID file.

## Tail Logs

```bash
./scripts/subnet-local.sh logs matcher
./scripts/subnet-local.sh logs validator
./scripts/subnet-local.sh logs mock-rootlayer
```

Log files live in `local-run/logs/<service>.log`. The helper command simply runs
`tail -f` on the chosen file.

## Cleaning State

- The validator keeps state in `local-run/validator-data`. Delete it if you want a
  fresh chain after stopping the services.
- `local-run/logs/` contains all service output. Remove the directory to clear old
  logs.

## Adjusting Ports and IDs

The defaults are suitable for a single developer workstation. If ports clash,
edits can be made in `scripts/subnet-local.sh` (search for `MATCHER_PORT`,
`VALIDATOR_PORT`, `ROOTLAYER_HTTP`, etc.). Ensure the log directory is writable.

## Common Issues

- **NATS refuses to start**: run `lsof -Pi :4222` to confirm another instance is
  already bound. Either stop the existing server or edit the script to use a new
  port for both the matcher and validator.
- **Validator exits immediately**: confirm `make build` was run and that the sample
  private key can be read. The script regenerates the validator data directory on
  each start; if you want persistence remove the `rm -rf` line.
- **No intents flowing**: the mock RootLayer currently only exposes HTTP. This
  preset keeps validators off-chain (`--enable-rootlayer` disabled). Integrate the
  contract SDK or point the matcher at a live RootLayer to exercise the full flow.

This helper is intended for local experimentation. For more elaborate multi-node
setups see `docs/LOCAL_RUN_GUIDE.md` together with `docs/E2E_TEST_GUIDE.md` and the
existing `scripts/e2e-test.sh` pipeline.
