#!/bin/bash

# PostgreSQL Mixed Load Test Runner for Windshift
# Runs mixed workload (read/write/update) against PostgreSQL in Docker

set -e

echo "🐘 Windshift PostgreSQL Mixed Load Test Runner"
echo "=============================================="

# Parse command line arguments
DURATION=60
WORKSPACES=10
INITIAL_ITEMS=100
CONCURRENCY=50
CREATE_WEIGHT=40
READ_WEIGHT=40
UPDATE_WEIGHT=20

while [[ $# -gt 0 ]]; do
  case $1 in
    --duration)
      DURATION="$2"
      shift 2
      ;;
    --workspaces)
      WORKSPACES="$2"
      shift 2
      ;;
    --initial-items)
      INITIAL_ITEMS="$2"
      shift 2
      ;;
    --concurrency)
      CONCURRENCY="$2"
      shift 2
      ;;
    --create)
      CREATE_WEIGHT="$2"
      shift 2
      ;;
    --read)
      READ_WEIGHT="$2"
      shift 2
      ;;
    --update)
      UPDATE_WEIGHT="$2"
      shift 2
      ;;
    -h|--help)
      echo ""
      echo "Usage: $0 [OPTIONS]"
      echo ""
      echo "Options:"
      echo "  --duration N          Test duration in seconds (default: 60)"
      echo "  --workspaces N        Number of workspaces (default: 10)"
      echo "  --initial-items N     Items to create initially (default: 100)"
      echo "  --concurrency N       Concurrent operations (default: 50)"
      echo "  --create N            Create operation weight % (default: 40)"
      echo "  --read N              Read operation weight % (default: 40)"
      echo "  --update N            Update operation weight % (default: 20)"
      echo "  -h, --help            Show this help message"
      echo ""
      echo "Examples:"
      echo "  $0                                      # Default test (60s, mixed workload)"
      echo "  $0 --duration 120 --concurrency 100    # 2 min test, high concurrency"
      echo "  $0 --create 20 --read 60 --update 20   # Read-heavy workload"
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
echo "   Duration:          ${DURATION}s"
echo "   Workspaces:        $WORKSPACES"
echo "   Initial Items:     $INITIAL_ITEMS"
echo "   Concurrency:       $CONCURRENCY"
echo "   Operation Mix:     Create ${CREATE_WEIGHT}%, Read ${READ_WEIGHT}%, Update ${UPDATE_WEIGHT}%"
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
./windshift -postgres-connection-string "$POSTGRES_CONN" -p "$TEST_PORT" > "loadtest_postgres_server.log" 2>&1 &
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

# Verify the token was inserted
echo "🔍 Verifying test data..."
TOKEN_COUNT=$(docker exec windshift-postgres-test psql -U windshift_test -d "$TEST_DB" -t -c "SELECT COUNT(*) FROM api_tokens WHERE id = 1;" | tr -d ' ')
if [ "$TOKEN_COUNT" != "1" ]; then
    echo "❌ Bearer token was not inserted correctly (count: $TOKEN_COUNT)"
    echo "   Checking what's in the database:"
    docker exec windshift-postgres-test psql -U windshift_test -d "$TEST_DB" -c "SELECT id, name, token_prefix FROM api_tokens;"
    exit 1
fi

# Set the bearer token (matches the hash we inserted)
BEARER_TOKEN="crw_test1234567890abcdef1234567890abcdef"

echo "✅ Test authentication data setup complete"
echo "   Bearer token verified in database"
echo ""

# Change to stresstest directory
cd "$SCRIPT_DIR"

# Run the mixed load test
echo "🚀 Starting PostgreSQL mixed load test..."
export API_BASE="$API_BASE/api"
export BEARER_TOKEN="$BEARER_TOKEN"
export DURATION
export WORKSPACES
export INITIAL_ITEMS
export CONCURRENCY
export CREATE_WEIGHT
export READ_WEIGHT
export UPDATE_WEIGHT

node mixed-load-test.js
TEST_EXIT_CODE=$?

# Show server logs if test failed
if [ $TEST_EXIT_CODE -ne 0 ]; then
    cd "$PROJECT_ROOT"
    show_server_logs
fi

echo ""
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "✅ PostgreSQL mixed load test completed successfully!"
else
    echo "❌ PostgreSQL mixed load test failed with errors!"
fi

exit $TEST_EXIT_CODE
