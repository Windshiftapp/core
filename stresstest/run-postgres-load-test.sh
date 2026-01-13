#!/bin/bash

# PostgreSQL Load Test Runner for Windshift
# Runs load testing against PostgreSQL in Docker

set -e

echo "🐘 Windshift PostgreSQL Load Test Runner"
echo "========================================"

# Parse command line arguments
WORKSPACES=100
ITEMS_PER_WORKSPACE=1000
CONCURRENCY=50

while [[ $# -gt 0 ]]; do
  case $1 in
    --workspaces)
      WORKSPACES="$2"
      shift 2
      ;;
    --items)
      ITEMS_PER_WORKSPACE="$2"
      shift 2
      ;;
    --concurrency)
      CONCURRENCY="$2"
      shift 2
      ;;
    -h|--help)
      echo ""
      echo "Usage: $0 [OPTIONS]"
      echo ""
      echo "Options:"
      echo "  --workspaces N        Number of workspaces to create (default: 100)"
      echo "  --items N             Items per workspace (default: 1000)"
      echo "  --concurrency N       Concurrent requests (default: 50)"
      echo "  -h, --help            Show this help message"
      echo ""
      echo "Examples:"
      echo "  $0                                    # Default test (100k items)"
      echo "  $0 --workspaces 10 --items 100        # Smaller test (1k items)"
      echo "  $0 --workspaces 50 --items 500        # Medium test (25k items)"
      echo "  $0 --concurrency 100                  # Higher concurrency"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      echo "Use -h or --help for usage information"
      exit 1
      ;;
  esac
done

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed. Please install Docker first."
    exit 1
fi

# Check if docker-compose is installed
if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null 2>&1; then
    echo "❌ docker-compose is not installed. Please install docker-compose first."
    exit 1
fi

# Use docker compose (new) or docker-compose (old)
if docker compose version &> /dev/null 2>&1; then
    DOCKER_COMPOSE="docker compose"
else
    DOCKER_COMPOSE="docker-compose"
fi

# Check if Node.js is installed
if ! command -v node &> /dev/null; then
    echo "❌ Node.js is not installed. Please install Node.js first."
    exit 1
fi

# Generate unique database name and port for this test run
TEST_DB="windshift_loadtest_$(date +%s)_$$"
TEST_PORT=$(( 9000 + (RANDOM % 1000) ))
POSTGRES_PORT=$(( 15432 + (RANDOM % 1000) ))
API_BASE="http://localhost:$TEST_PORT"

echo "🗄️  Testing PostgreSQL with isolated database"
echo "   Database:          $TEST_DB"
echo "   Server Port:       $TEST_PORT"
echo "   PostgreSQL Port:   $POSTGRES_PORT"
echo "   Workspaces:        $WORKSPACES"
echo "   Items/Workspace:   $ITEMS_PER_WORKSPACE"
echo "   Total Items:       $(($WORKSPACES * $ITEMS_PER_WORKSPACE))"
echo "   Concurrency:       $CONCURRENCY"
echo ""

# Get the script directory and find project root
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Export port for docker-compose
export POSTGRES_PORT

# Function to cleanup on exit
cleanup() {
    echo ""
    echo "🧹 Cleaning up..."

    # Stop the server if running
    if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
        echo "   Stopping test server (PID: $SERVER_PID)..."
        kill "$SERVER_PID"
        wait "$SERVER_PID" 2>/dev/null || true
    fi

    # Drop the test database
    if [ "$POSTGRES_STARTED" = "true" ]; then
        echo "   Dropping test database: $TEST_DB"
        docker exec windshift-postgres-test psql -U windshift_test -d postgres -c "DROP DATABASE IF EXISTS $TEST_DB;" 2>/dev/null || true
    fi

    # Stop and remove PostgreSQL container
    echo "   Stopping PostgreSQL container..."
    cd "$PROJECT_ROOT/tests"
    $DOCKER_COMPOSE -f docker-compose.postgres.yml down -v 2>/dev/null || true

    # Clean up log files
    cd "$PROJECT_ROOT"
    rm -f "loadtest_postgres_server.log"
    rm -f "$SCRIPT_DIR/test_token.txt"

    echo "✅ Cleanup complete"
}

# Set trap to cleanup on script exit
trap cleanup EXIT

# Start PostgreSQL container
echo "🚀 Starting PostgreSQL 17 container..."
cd "$PROJECT_ROOT/tests"
$DOCKER_COMPOSE -f docker-compose.postgres.yml up -d

# Wait for PostgreSQL to be healthy
echo "⏱️  Waiting for PostgreSQL to be ready..."
for i in {1..30}; do
    if docker exec windshift-postgres-test pg_isready -U windshift_test > /dev/null 2>&1; then
        echo "✅ PostgreSQL is ready"
        POSTGRES_STARTED=true
        break
    fi
    if [ $i -eq 30 ]; then
        echo "❌ PostgreSQL failed to start within 30 seconds"
        docker logs windshift-postgres-test || true
        exit 1
    fi
    sleep 1
done

# Create test database
echo "📊 Creating test database: $TEST_DB"
docker exec windshift-postgres-test psql -U windshift_test -d postgres -c "CREATE DATABASE $TEST_DB;"

# Enable pg_stat_statements for performance monitoring
echo "📈 Enabling pg_stat_statements extension for performance monitoring..."
docker exec windshift-postgres-test psql -U windshift_test -d "$TEST_DB" -c "CREATE EXTENSION IF NOT EXISTS pg_stat_statements;"

# Build connection string
POSTGRES_CONN="postgresql://windshift_test:windshift_test_password@localhost:$POSTGRES_PORT/$TEST_DB?sslmode=disable"

echo "🔗 Connection string: postgresql://windshift_test:***@localhost:$POSTGRES_PORT/$TEST_DB"

# Change to project root to run the server
cd "$PROJECT_ROOT"

# Always rebuild windshift binary to ensure latest code changes
echo "📦 Building windshift binary..."
go build -o windshift

# Start test server with PostgreSQL (uses 50 connections by default)
echo "🚀 Starting windshift server with PostgreSQL..."
SSO_SECRET="loadtest-secret-key-12345" ./windshift -postgres-connection-string "$POSTGRES_CONN" -p "$TEST_PORT" --allowed-hosts "localhost,127.0.0.1" > "loadtest_postgres_server.log" 2>&1 &
SERVER_PID=$!

# Function to show server logs on errors
show_server_logs() {
    echo ""
    echo "🚨 Server logs (last 30 lines):"
    echo "================================="
    tail -30 "loadtest_postgres_server.log" 2>/dev/null || echo "No server logs available"
    echo "================================="
    echo ""
}

# Wait for server to start
echo "⏱️  Waiting for test server to start..."
for i in {1..30}; do
    if curl -s "$API_BASE/api/setup/status" > /dev/null 2>&1; then
        echo "✅ Test server is running"
        break
    fi
    if [ $i -eq 30 ]; then
        echo "❌ Test server failed to start within 30 seconds"
        show_server_logs
        exit 1
    fi
    sleep 1
done

# Wait a moment for database migrations to complete
sleep 2

# Setup test authentication data directly in PostgreSQL (same as SQLite test pattern)
echo "🔧 Setting up test authentication data..."

# Execute each statement separately and show output
echo "   Inserting user..."
docker exec windshift-postgres-test psql -U windshift_test -d "$TEST_DB" -c "INSERT INTO users (id, email, username, first_name, last_name, is_active, password_hash) VALUES (1, 'admin@test.com', 'admin', 'Test', 'Admin', true, '\$2a\$10\$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy') ON CONFLICT (id) DO NOTHING;"
echo "   User insert result: $?"

echo "   Inserting API token..."
docker exec windshift-postgres-test psql -U windshift_test -d "$TEST_DB" -c "INSERT INTO api_tokens (id, user_id, name, token_hash, token_prefix, permissions, is_temporary) VALUES (1, 1, 'Load Test API Token', '\$2a\$10\$RcUp3viPi3V1AWoJGB5DyOM7Sxzr5fdRQURhm5ZIA0QLXPFmuxLtK', 'crw_test1234...', '[\"read\",\"write\",\"admin\"]', false) ON CONFLICT (id) DO NOTHING;"
echo "   API token insert result: $?"

echo "   Inserting global permissions..."
docker exec windshift-postgres-test psql -U windshift_test -d "$TEST_DB" -c "INSERT INTO user_global_permissions (user_id, permission_id) SELECT 1, id FROM permissions WHERE permission_key = 'system.admin' ON CONFLICT DO NOTHING;"
echo "   Global permissions insert result: $?"

echo "   Inserting system settings (setup_completed)..."
docker exec windshift-postgres-test psql -U windshift_test -d "$TEST_DB" -c "INSERT INTO system_settings (key, value, value_type, description, category) VALUES ('setup_completed', 'true', 'boolean', 'Whether initial setup has been completed', 'setup') ON CONFLICT (key) DO UPDATE SET value = 'true';"
echo "   System setting insert result: $?"

echo "   Inserting system settings (admin_user_created)..."
docker exec windshift-postgres-test psql -U windshift_test -d "$TEST_DB" -c "INSERT INTO system_settings (key, value, value_type, description, category) VALUES ('admin_user_created', 'true', 'boolean', 'Whether admin user has been created', 'setup') ON CONFLICT (key) DO UPDATE SET value = 'true';"
echo "   System setting insert result: $?"

# Set the bearer token (matches the hash we inserted)
BEARER_TOKEN="crw_test1234567890abcdef1234567890abcdef"

echo "✅ Test authentication data setup complete"
echo ""

# Change to stresstest directory
cd "$SCRIPT_DIR"

# Run the load test
echo "🚀 Starting PostgreSQL load test..."
export API_BASE="$API_BASE/api"
export BEARER_TOKEN="$BEARER_TOKEN"
export WORKSPACES
export ITEMS_PER_WORKSPACE
export CONCURRENCY

node load-test.js
TEST_EXIT_CODE=$?

# Show server logs if test failed
if [ $TEST_EXIT_CODE -ne 0 ]; then
    cd "$PROJECT_ROOT"
    show_server_logs
fi

echo ""

# Extract and analyze profiling data from server logs
echo ""
echo "📊 Server-Side Profiling Summary"
echo "================================="
cd "$PROJECT_ROOT"

PERF_LINES=$(grep -c '\[PERF\]' "loadtest_postgres_server.log" 2>/dev/null) || PERF_LINES=0
echo "Total profiled requests: $PERF_LINES"

if [ "$PERF_LINES" -gt 0 ]; then
    echo ""
    echo "Timing breakdown (in milliseconds):"
    echo "-----------------------------------"

    # Extract timing data and calculate statistics using awk
    grep '\[PERF\]' "loadtest_postgres_server.log" | head -1000 | awk '
    BEGIN {
        n = 0
    }
    {
        n++
        # Extract values using regex
        for (i=1; i<=NF; i++) {
            if ($i ~ /valid=/) { gsub(/valid=|ms,?/, "", $i); valid[n] = $i + 0 }
            if ($i ~ /frac=/) { gsub(/frac=|ms,?/, "", $i); frac[n] = $i + 0 }
            if ($i ~ /lock=/) { gsub(/lock=|ms,?/, "", $i); lock[n] = $i + 0 }
            if ($i ~ /tx=/) { gsub(/tx=|ms,?/, "", $i); tx[n] = $i + 0 }
            if ($i ~ /query=/) { gsub(/query=|ms,?/, "", $i); query[n] = $i + 0 }
            if ($i ~ /sse=/) { gsub(/sse=|ms,?/, "", $i); sse[n] = $i + 0 }
            if ($i ~ /notify=/) { gsub(/notify=|ms,?/, "", $i); notify[n] = $i + 0 }
            if ($i ~ /gap=/) { gsub(/gap=|ms,?/, "", $i); gap[n] = $i + 0 }
            if ($i ~ /total=/) { gsub(/total=|ms,?/, "", $i); total[n] = $i + 0 }
        }
        sum_valid += valid[n]; sum_frac += frac[n]; sum_lock += lock[n]
        sum_tx += tx[n]; sum_query += query[n]; sum_sse += sse[n]
        sum_notify += notify[n]; sum_gap += gap[n]; sum_total += total[n]
    }
    END {
        if (n > 0) {
            printf "  %-12s avg=%.2fms\n", "valid:", sum_valid/n
            printf "  %-12s avg=%.2fms\n", "frac:", sum_frac/n
            printf "  %-12s avg=%.2fms\n", "lock:", sum_lock/n
            printf "  %-12s avg=%.2fms\n", "tx:", sum_tx/n
            printf "  %-12s avg=%.2fms\n", "query:", sum_query/n
            printf "  %-12s avg=%.2fms\n", "sse:", sum_sse/n
            printf "  %-12s avg=%.2fms\n", "notify:", sum_notify/n
            printf "  %-12s avg=%.2fms\n", "gap:", sum_gap/n
            printf "  %-12s avg=%.2fms\n", "TOTAL:", sum_total/n
            print ""
            print "Legend:"
            print "  valid  = validation (permissions, workspace checks)"
            print "  frac   = fractional index generation"
            print "  lock   = PostgreSQL advisory lock wait"
            print "  tx     = transaction (BEGIN -> INSERT -> COMMIT)"
            print "  query  = SELECT query to return created item"
            print "  sse    = SSE event emission"
            print "  notify = notification event emission"
            print "  gap    = scheduler/unmeasured time (between measurements)"
        }
    }
    '

    echo ""
    echo "Sample profiling lines (first 5):"
    echo "-----------------------------------"
    grep '\[PERF\]' "loadtest_postgres_server.log" | head -5
fi

echo ""
echo "Full server log: $PROJECT_ROOT/loadtest_postgres_server.log"
echo ""

if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "✅ PostgreSQL load test completed successfully!"
else
    echo "❌ PostgreSQL load test failed with errors!"
fi

exit $TEST_EXIT_CODE
