#!/usr/bin/env bash
set -euo pipefail

PG_CONTAINER="windshift-dev-postgres"
LOGBOOK_CONTAINER="windshift-dev-logbook"
LOGBOOK_IMAGE="windshift-logbook-dev"
SSO_SECRET="dev-secret-for-testing"
PIDS=()

cleanup() {
  echo ""
  echo "Shutting down..."
  for pid in "${PIDS[@]}"; do
    kill "$pid" 2>/dev/null || true
  done
  wait "${PIDS[@]}" 2>/dev/null || true
  rm -f .dev-windshift
  echo "Stopped go processes."
  echo "Stopping logbook container..."
  docker stop "$LOGBOOK_CONTAINER" 2>/dev/null || true
  docker rm "$LOGBOOK_CONTAINER" 2>/dev/null || true
  echo "Postgres container '$PG_CONTAINER' is still running. Stop it with: docker stop $PG_CONTAINER"
}

trap cleanup SIGINT SIGTERM

# --- Kill previous dev instance if running ---
if [ -f .dev-windshift ] && pgrep -f '\.dev-windshift' >/dev/null 2>&1; then
  echo "Stopping previous dev instance..."
  pkill -f '\.dev-windshift' 2>/dev/null || true
  docker rm -f "$LOGBOOK_CONTAINER" 2>/dev/null || true
  sleep 1
fi

# Clean up legacy container name (renamed from windshift-dev-pgvector)
if docker ps -a --format '{{.Names}}' | grep -q "^windshift-dev-pgvector$"; then
  echo "Removing legacy container 'windshift-dev-pgvector'..."
  docker stop windshift-dev-pgvector 2>/dev/null || true
  docker rm windshift-dev-pgvector 2>/dev/null || true
fi

# --- Start postgres container (for logbook only) ---
if docker ps --format '{{.Names}}' | grep -q "^${PG_CONTAINER}$"; then
  echo "Postgres container '$PG_CONTAINER' is already running."
elif docker ps -a --format '{{.Names}}' | grep -q "^${PG_CONTAINER}$"; then
  echo "Found stopped container '$PG_CONTAINER', removing to ensure correct config..."
  docker rm "$PG_CONTAINER"
fi

if ! docker ps --format '{{.Names}}' | grep -q "^${PG_CONTAINER}$"; then
  echo "Creating postgres container '$PG_CONTAINER'..."
  docker run -d \
    --name "$PG_CONTAINER" \
    -e POSTGRES_USER=logbook \
    -e POSTGRES_PASSWORD=logbook \
    -e POSTGRES_DB=logbook \
    -p 5433:5432 \
    postgres:18
fi

echo "Waiting for postgres to be ready..."
until docker exec "$PG_CONTAINER" psql -U logbook -d logbook -c 'SELECT 1' >/dev/null 2>&1; do
  sleep 0.5
done
echo "Postgres is ready."

# --- Create data directories ---
mkdir -p data/logbook

# --- Build and run main windshift binary (SQLite) ---
echo "Building windshift server..."
go build -o .dev-windshift .

echo "Starting windshift server (SQLite)..."
SSO_SECRET="$SSO_SECRET" \
LLM_ENDPOINT=http://localhost:1234 \
LOGBOOK_ENDPOINT=http://localhost:8090 \
  ./.dev-windshift \
  -port 7777 \
  -attachment-path data/ \
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

# --- Build and run logbook in Docker (needs poppler-utils, kreuzberg-cli) ---
echo "Building logbook Docker image..."
docker build --pull -f Dockerfile.logbook -t "$LOGBOOK_IMAGE" .

# Remove old logbook container if exists
docker rm -f "$LOGBOOK_CONTAINER" 2>/dev/null || true

echo "Starting logbook container..."
docker run -d \
  --name "$LOGBOOK_CONTAINER" \
  --add-host=host.docker.internal:host-gateway \
  -p 8090:8090 \
  -v "$(pwd)/data/logbook:/data/logbook" \
  -e LOGBOOK_DATABASE_URL="postgresql://logbook:logbook@host.docker.internal:5433/logbook?sslmode=disable" \
  -e LOGBOOK_STORAGE_PATH=/data/logbook \
  -e LOGBOOK_PORT=8090 \
  -e LOGBOOK_ARTICLE_ENDPOINT="http://host.docker.internal:7777/api/internal/llm" \
  -e SSO_SECRET="$SSO_SECRET" \
  -e LOG_LEVEL=debug \
  "$LOGBOOK_IMAGE"

echo "windshift running on :7777 (SQLite), logbook running on :8090 (Docker)"
echo "Press Ctrl+C to stop."

# Stream logbook container logs into the terminal
docker logs -f "$LOGBOOK_CONTAINER" &
PIDS+=($!)

wait "${PIDS[@]}" || true
