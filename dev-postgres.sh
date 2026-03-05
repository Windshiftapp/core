#!/usr/bin/env bash
set -euo pipefail

PG_CONTAINER="windshift-dev-pg"
PG_PORT=5434
PG_USER="windshift"
PG_PASS="windshift"
PG_DB="windshift"
PG_CONN="postgresql://${PG_USER}:${PG_PASS}@localhost:${PG_PORT}/${PG_DB}?sslmode=disable"
SSO_SECRET="$(openssl rand -hex 32)"
PIDS=()

cleanup() {
  echo ""
  echo "Shutting down..."
  for pid in "${PIDS[@]}"; do
    kill "$pid" 2>/dev/null || true
  done
  wait "${PIDS[@]}" 2>/dev/null || true
  rm -f .dev-windshift-pg
  echo "Stopped go process."
  echo "Removing postgres container..."
  docker stop "$PG_CONTAINER" 2>/dev/null || true
  docker rm "$PG_CONTAINER" 2>/dev/null || true
  echo "Done."
}

trap cleanup SIGINT SIGTERM

# --- Kill previous dev-pg instance if running ---
if [ -f .dev-windshift-pg ] && pgrep -f '\.dev-windshift-pg' >/dev/null 2>&1; then
  echo "Stopping previous dev-pg instance..."
  pkill -f '\.dev-windshift-pg' 2>/dev/null || true
  sleep 1
fi

# --- Remove existing container to ensure a fresh database ---
docker rm -f "$PG_CONTAINER" 2>/dev/null || true

# --- Start fresh postgres container ---
echo "Starting fresh postgres container '$PG_CONTAINER' on port $PG_PORT..."
docker run -d \
  --name "$PG_CONTAINER" \
  -e POSTGRES_USER="$PG_USER" \
  -e POSTGRES_PASSWORD="$PG_PASS" \
  -e POSTGRES_DB="$PG_DB" \
  -p "${PG_PORT}:5432" \
  postgres:17

echo "Waiting for postgres to be ready..."
until docker exec "$PG_CONTAINER" pg_isready -U "$PG_USER" -d "$PG_DB" >/dev/null 2>&1; do
  sleep 0.5
done
echo "Postgres is ready."

# --- Create data directories ---
mkdir -p data/

# --- Build and run windshift against PostgreSQL ---
echo "Building windshift server..."
go build -o .dev-windshift-pg .

echo "Starting windshift server (PostgreSQL)..."
SSO_SECRET="$SSO_SECRET" \
./.dev-windshift-pg \
  -port 7888 \
  -pg-conn "$PG_CONN" \
  -attachment-path data/ \
  -no-csrf \
  -log-level debug &
PIDS+=($!)

# Wait briefly for the Go server to be alive
sleep 1
if ! kill -0 "${PIDS[0]}" 2>/dev/null; then
  echo "ERROR: windshift server failed to start."
  wait "${PIDS[0]}" 2>/dev/null || true
  cleanup
  exit 1
fi

echo ""
echo "windshift running on :7888 (PostgreSQL @ localhost:$PG_PORT)"
echo "Press Ctrl+C to stop."

wait "${PIDS[@]}" || true
