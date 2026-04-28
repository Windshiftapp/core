#!/usr/bin/env bash
set -euo pipefail

SSO_SECRET="dev-secret-for-testing"
DB_PATH="windshift.db"
ATTACHMENT_PATH="data/"
PIDS=()
FRESH=0

for arg in "$@"; do
  case "$arg" in
    --fresh)
      FRESH=1
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      echo "Usage: $0 [--fresh]" >&2
      exit 1
      ;;
  esac
done

cleanup() {
  echo ""
  echo "Shutting down..."
  for pid in "${PIDS[@]}"; do
    kill "$pid" 2>/dev/null || true
  done
  wait "${PIDS[@]}" 2>/dev/null || true
  rm -f .dev-windshift
  echo "Stopped go processes."
}

trap cleanup SIGINT SIGTERM

# --- Kill previous dev instance if running ---
if [ -f .dev-windshift ] && pgrep -f '\.dev-windshift' >/dev/null 2>&1; then
  echo "Stopping previous dev instance..."
  pkill -f '\.dev-windshift' 2>/dev/null || true
  sleep 1
fi

# --- Optionally wipe the SQLite DB and attachments ---
if [ "$FRESH" = "1" ]; then
  echo "Wiping fresh: removing $DB_PATH and $ATTACHMENT_PATH..."
  rm -f "$DB_PATH" "$DB_PATH-shm" "$DB_PATH-wal"
  rm -rf "$ATTACHMENT_PATH"
fi

mkdir -p "$ATTACHMENT_PATH"

# --- Build and run main windshift binary (SQLite) ---
echo "Building windshift server..."
go build -o .dev-windshift .

echo "Starting windshift server (SQLite)..."
SSO_SECRET="$SSO_SECRET" \
LLM_ENDPOINT=http://localhost:1234 \
  ./.dev-windshift \
  -port 7777 \
  -db "$DB_PATH" \
  -attachment-path "$ATTACHMENT_PATH" \
  -no-csrf \
  -log-level debug &
PIDS+=($!)

# Wait briefly for the Go server to be alive
sleep 1
if ! kill -0 "${PIDS[0]}" 2>/dev/null; then
  echo "ERROR: windshift server failed to start."
  wait "${PIDS[0]}" 2>/dev/null || true
  exit 1
fi

echo "windshift running on :7777 (SQLite)"
echo "Press Ctrl+C to stop."

wait "${PIDS[@]}" || true
