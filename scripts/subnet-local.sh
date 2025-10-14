#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_ROOT="$ROOT_DIR/local-run"
LOG_DIR="$LOG_ROOT/logs"
PID_FILE="$LOG_ROOT/pids"
SUBNET_ID="0x1111111111111111111111111111111111111111111111111111111111111111"
NATS_URL="nats://127.0.0.1:4222"
MATCHER_PORT=8090
VALIDATOR_PORT=9200
METRICS_PORT=9300
ROOTLAYER_HTTP=":9090"
MATCHER_KEY="eef960cc05e034727123db159f1e54f0733b2f51d4a239978771aff320be5b9a"
MATCHER_PUB="0482ea12c5481d481c7f9d7c1a2047401c6e2f855e4cee4d8df0aa197514f3456528ba6c55092b20b51478fd8cf62cde37f206621b3dd47c2be3d5c35e4889bf94"

ensure_dirs() {
  mkdir -p "$LOG_DIR"
}

check_binary() {
  local path="$1"
  local build_target="$2"
  if [ ! -x "$path" ]; then
    echo "[subnet-local] building $build_target ..."
    (cd "$ROOT_DIR" && make "$build_target")
  fi
}

check_port_free() {
  local port="$1"
  ! lsof -Pi ":$port" -sTCP:LISTEN -t >/dev/null 2>&1
}

start_nats_if_needed() {
  if check_port_free 4222; then
    if command -v nats-server >/dev/null 2>&1; then
      echo "[subnet-local] starting embedded NATS on 4222"
      nats-server --addr 127.0.0.1 --port 4222 >>"$LOG_DIR/nats.log" 2>&1 &
      echo "nats:$!" >>"$PID_FILE"
      sleep 1
    else
      echo "[subnet-local] ERROR: NATS not running and nats-server not found" >&2
      exit 1
    fi
  fi
}

start_component() {
  local name="$1"; shift
  local logfile="$LOG_DIR/$name.log"
  echo "[subnet-local] starting $name"
  "$@" >>"$logfile" 2>&1 &
  local pid=$!
  echo "$name:$pid" >>"$PID_FILE"
}

wait_for_port() {
  local port="$1"
  local name="$2"
  local attempts=0
  until ! check_port_free "$port"; do
    sleep 1
    attempts=$((attempts+1))
    if [ $attempts -gt 25 ]; then
      echo "[subnet-local] ERROR: $name did not open port $port" >&2
      exit 1
    fi
  done
}

start_all() {
  if [ -f "$PID_FILE" ]; then
    echo "[subnet-local] services already running (pid file $PID_FILE). Run 'subnet-local.sh stop' first." >&2
    exit 1
  fi

  ensure_dirs
  : >"$PID_FILE"

  check_binary "$ROOT_DIR/bin/mock-rootlayer" build
  check_binary "$ROOT_DIR/bin/matcher" build
  check_binary "$ROOT_DIR/bin/validator" build
  check_binary "$ROOT_DIR/bin/simple-agent" build

  start_nats_if_needed

  start_component mock-rootlayer "$ROOT_DIR/bin/mock-rootlayer" --http "$ROOTLAYER_HTTP"
  wait_for_port 9090 "mock-rootlayer"

  local matcher_cfg="$LOG_DIR/matcher.yaml"
  cat >"$matcher_cfg" <<EOF
subnet_id: "$SUBNET_ID"
identity:
  matcher_id: "matcher-local"
  subnet_id: "$SUBNET_ID"
network:
  nats_url: "$NATS_URL"
  matcher_grpc_port: ":$MATCHER_PORT"
limits:
  bidding_window_sec: 10
  max_bids_per_intent: 50
rootlayer:
  http_url: "http://127.0.0.1:9090"
  grpc_endpoint: "127.0.0.1:9090"
matcher:
  signer:
    type: "ecdsa"
    private_key: "$MATCHER_KEY"
private_key: "$MATCHER_KEY"
EOF
  start_component matcher "$ROOT_DIR/bin/matcher" --config "$matcher_cfg" --grpc ":$MATCHER_PORT"
  wait_for_port "$MATCHER_PORT" "matcher"

  local validator_storage="$LOG_ROOT/validator-data"
  rm -rf "$validator_storage"
  mkdir -p "$validator_storage"
  local validator_pubkeys="validator-local-1:$MATCHER_PUB"
  start_component validator "$ROOT_DIR/bin/validator" \
    --id "validator-local-1" \
    --key "$MATCHER_KEY" \
    --grpc "$VALIDATOR_PORT" \
    --nats "$NATS_URL" \
    --storage "$validator_storage" \
    --validators 1 \
    --threshold-num 1 \
    --threshold-denom 1 \
    --validator-pubkeys "$validator_pubkeys" \
    --subnet-id "$SUBNET_ID" \
    --metrics "$METRICS_PORT"
  wait_for_port "$VALIDATOR_PORT" "validator"

  start_component agent "$ROOT_DIR/bin/simple-agent" --id agent-local-1 --matcher "127.0.0.1:$MATCHER_PORT"

  echo "[subnet-local] services started. Logs under $LOG_DIR"
}

stop_all() {
  if [ ! -f "$PID_FILE" ]; then
    echo "[subnet-local] nothing to stop"
    return
  fi
  tac "$PID_FILE" | while IFS=':' read -r name pid; do
    if [[ "$name" == "nats" ]]; then
      if ! kill "$pid" 2>/dev/null; then
        pkill -f "nats-server --addr 127.0.0.1 --port 4222" 2>/dev/null || true
      fi
    else
      kill "$pid" 2>/dev/null || true
    fi
  done
  rm -f "$PID_FILE"
  echo "[subnet-local] services stopped"
}

status_all() {
  if [ ! -f "$PID_FILE" ]; then
    echo "[subnet-local] status: stopped"
    return
  fi
  echo "[subnet-local] status:"
  while IFS=':' read -r name pid; do
    if kill -0 "$pid" 2>/dev/null; then
      echo "  $name (pid $pid) running"
    else
      echo "  $name (pid $pid) not running"
    fi
  done <"$PID_FILE"
}

show_logs() {
  local svc="${1:-}"
  if [ -z "$svc" ]; then
    echo "Provide a service name" >&2
    exit 1
  fi
  tail -f "$LOG_DIR/$svc.log"
}

usage() {
  cat <<USAGE
Usage: ./scripts/subnet-local.sh <command>

Commands:
  start         Start mock RootLayer, matcher, validator, and simple agent
  stop          Stop all services started by this helper
  status        Show whether services are running
  logs <svc>    Tail logs for a service (mock-rootlayer|matcher|validator|agent|nats)
USAGE
}

cmd=${1:-}
case "$cmd" in
  start)
    start_all
    ;;
  stop)
    stop_all
    ;;
  status)
    status_all
    ;;
  logs)
    shift
    show_logs "${1:-}"
    ;;
  *)
    usage
    [ -n "$cmd" ] && exit 1
    ;;
esac
