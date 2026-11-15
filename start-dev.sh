#!/usr/bin/env bash
# Starts the SFU (Rust), backend (Go), and frontend (SvelteKit) together,
# and stops all three on Ctrl+C.
#
# `cargo run`, `go run`, and `npm run dev` all spawn the actual process as a
# child, so killing just the top-level PID can leave an orphan behind.
# `set -m` gives each backgrounded job its own process group, and we kill
# the whole group (negative PID) to make sure the real processes die too.
set -uo pipefail
set -m

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SFU_DIR="$ROOT_DIR/capstone_sfu"
BE_DIR="$ROOT_DIR/capstone_be"
FE_DIR="$ROOT_DIR/capstone_fe"

# Public signaling URL the backend hands to browsers. It goes through the
# nginx reverse proxy (TLS), so it must be wss:// on the proxy's host, not the
# SFU's own :9000. Defaults to this machine's first LAN IP; override with
# SFU_WS_URL=wss://<host>/ws ./start-dev.sh
LAN_IP="$(hostname -I | awk '{print $1}')"
export SFU_WS_URL="${SFU_WS_URL:-wss://${LAN_IP:-localhost}/ws}"

# Origins allowed to call the backend cross-origin. Through nginx the frontend
# is same-origin, so this only matters when hitting Vite (:5173) directly.
# Covers every IPv4 address this machine has on its internal networks.
DEFAULT_ORIGINS="http://localhost:5173,http://127.0.0.1:5173,https://localhost,http://frontend.test,https://frontend.test"
for ip in $(hostname -I); do
    [[ "$ip" == *:* ]] && continue # skip IPv6
    DEFAULT_ORIGINS+=",http://$ip:5173,https://$ip"
done
export CORS_ALLOWED_ORIGINS="${CORS_ALLOWED_ORIGINS:-$DEFAULT_ORIGINS}"

PIDS=()

cleanup() {
    echo
    echo "Shutting down..."
    for pid in "${PIDS[@]}"; do
        kill -TERM -- "-$pid" 2>/dev/null
    done
    for _ in 1 2 3 4 5; do
        alive=0
        for pid in "${PIDS[@]}"; do
            kill -0 "$pid" 2>/dev/null && alive=1
        done
        [ "$alive" -eq 0 ] && break
        sleep 1
    done
    for pid in "${PIDS[@]}"; do
        kill -KILL -- "-$pid" 2>/dev/null
    done
    wait 2>/dev/null
    echo "All processes stopped."
}

trap cleanup INT TERM

echo "Starting SFU (cargo run) in $SFU_DIR ..."
(cd "$SFU_DIR" && exec cargo run -p server) > >(sed -u 's/^/[sfu] /') 2>&1 &
PIDS+=("$!")

echo "Starting backend (go run) in $BE_DIR ..."
(cd "$BE_DIR" && exec go run ./cmd/webserver) > >(sed -u 's/^/[backend] /') 2>&1 &
PIDS+=("$!")

echo "Starting frontend (npm run dev) in $FE_DIR ..."
(cd "$FE_DIR" && exec npm run dev) > >(sed -u 's/^/[frontend] /') 2>&1 &
PIDS+=("$!")

echo "All services started (SFU_WS_URL=$SFU_WS_URL). Press Ctrl+C to stop."
wait
