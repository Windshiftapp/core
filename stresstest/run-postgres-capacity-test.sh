#!/bin/bash

# Windshift PostgreSQL Capacity Test Runner
# Finds maximum concurrent users for PostgreSQL by gradual ramp-up until failure

set -e

echo "🐘 Windshift PostgreSQL Capacity Test"
echo "======================================="

# Configuration
PRE_POPULATE_ITEMS=1000  # Default to 1000 for faster test runs
WORKSPACES=10
START_CONCURRENCY=10
MAX_CONCURRENCY=500
TEST_DURATION=300  # 5 minutes
RAMP_INTERVAL=30   # Increase every 30 seconds
RAMP_INCREMENT=10  # Add 10 users each time
ENABLE_DIAGNOSTICS=false

while [[ $# -gt 0 ]]; do
  case $1 in
    --items)
      PRE_POPULATE_ITEMS="$2"
      shift 2
      ;;
    --duration)
      TEST_DURATION="$2"
      shift 2
      ;;
    --start-users)
      START_CONCURRENCY="$2"
      shift 2
      ;;
    --max-users)
      MAX_CONCURRENCY="$2"
      shift 2
      ;;
    --ramp-interval)
      RAMP_INTERVAL="$2"
      shift 2
      ;;
    --diagnostics)
      ENABLE_DIAGNOSTICS=true
      shift
      ;;
    -h|--help)
      echo ""
      echo "Usage: $0 [OPTIONS]"
      echo ""
      echo "Options:"
      echo "  --items N             Items to pre-populate (default: 1000)"
      echo "  --duration N          Test duration in seconds (default: 300)"
      echo "  --start-users N       Starting concurrent users (default: 10)"
      echo "  --max-users N         Maximum concurrent users (default: 500)"
      echo "  --ramp-interval N     Seconds between ramp-ups (default: 30)"
      echo "  --diagnostics         Enable PostgreSQL performance monitoring"
      echo "  -h, --help            Show this help"
      echo ""
      echo "Examples:"
      echo "  $0                              # Default test"
      echo "  $0 --items 1000 --max-users 200 # Smaller test"
      echo "  $0 --diagnostics                # Run with diagnostics"
      echo "  $0 --items 1000 --diagnostics   # Quick test with diagnostics"
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
TEST_DB="windshift_capacity_$(date +%s)_$$"
TEST_PORT=$(( 9000 + (RANDOM % 1000) ))
POSTGRES_PORT=$(( 15432 + (RANDOM % 1000) ))
API_BASE="http://localhost:$TEST_PORT"

echo "🗄️  Testing PostgreSQL with isolated database"
echo "   Database:          $TEST_DB"
echo "   Server Port:       $TEST_PORT"
echo "   PostgreSQL Port:   $POSTGRES_PORT"
echo "   Pre-populate:      $PRE_POPULATE_ITEMS items"
echo "   Test Duration:     ${TEST_DURATION}s"
echo "   Start Users:       $START_CONCURRENCY"
echo "   Max Users:         $MAX_CONCURRENCY"
echo "   Ramp Interval:     ${RAMP_INTERVAL}s (+${RAMP_INCREMENT} users)"
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

    # Stop diagnostics monitoring if running
    if [ -n "$DIAGNOSTICS_PID" ] && kill -0 "$DIAGNOSTICS_PID" 2>/dev/null; then
        echo "   Stopping diagnostics monitor (PID: $DIAGNOSTICS_PID)..."
        kill "$DIAGNOSTICS_PID" 2>/dev/null || true
        wait "$DIAGNOSTICS_PID" 2>/dev/null || true
    fi

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
    rm -f "capacity_postgres_server.log"
    rm -f "$PROJECT_ROOT/tests/test_token.txt"
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
echo ""

# Change to project root to run the server
cd "$PROJECT_ROOT"

# Use existing binary (already built with correct schema order)
if [ ! -f "windshift" ]; then
    echo "❌ windshift binary not found. Please run 'go build -o windshift' first."
    exit 1
fi
echo "✅ Using existing windshift binary"
echo ""

# Function to show server logs on errors
show_server_logs() {
    echo ""
    echo "🚨 Server logs (last 50 lines):"
    echo "================================="
    tail -50 "capacity_postgres_server.log" 2>/dev/null || echo "No server logs available"
    echo "================================="
    echo ""
}

# Start test server with PostgreSQL (uses 50 connections by default)
echo "🚀 Starting test server with PostgreSQL..."
./windshift -postgres-connection-string "$POSTGRES_CONN" -p "$TEST_PORT" > "capacity_postgres_server.log" 2>&1 &
SERVER_PID=$!

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

# Pre-populate database with items and history
echo "📊 Pre-populating database with $PRE_POPULATE_ITEMS items..."
echo "   This may take a few minutes..."
echo ""

# Change to stresstest directory
cd "$SCRIPT_DIR"

# Create pre-population script
cat > /tmp/prepopulate_$$.js << 'PREPOPULATE_SCRIPT'
const API_BASE = process.env.API_BASE;
const BEARER_TOKEN = process.env.BEARER_TOKEN;
const WORKSPACES = parseInt(process.env.WORKSPACES || '10');
const TOTAL_ITEMS = parseInt(process.env.PRE_POPULATE_ITEMS || '5000');

async function request(endpoint, options = {}) {
  const url = `${API_BASE}${endpoint}`;
  const defaultOptions = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${BEARER_TOKEN}`
    },
  };

  const response = await fetch(url, { ...defaultOptions, ...options });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(`HTTP ${response.status}: ${text}`);
  }

  if (response.status === 204) return null;
  return response.json();
}

async function prepopulate() {
  console.log('Creating workspaces...');
  const workspaceIds = [];

  for (let i = 1; i <= WORKSPACES; i++) {
    const randomSuffix = Math.random().toString(36).substring(2, 7).toUpperCase();
    const workspace = await request('/workspaces', {
      method: 'POST',
      body: JSON.stringify({
        name: `Capacity Test Workspace ${i}`,
        key: `CT${String(i).padStart(2, '0')}${randomSuffix}`,
        description: `Pre-populated workspace for capacity testing`
      })
    });
    workspaceIds.push(workspace.id);
    process.stdout.write(`\r   Created ${i}/${WORKSPACES} workspaces...`);
  }

  console.log(`\n✅ Created ${WORKSPACES} workspaces\n`);

  console.log(`Creating ${TOTAL_ITEMS} items with history...`);
  const itemIds = [];
  const itemsPerWorkspace = Math.floor(TOTAL_ITEMS / WORKSPACES);
  let errorCount = 0;

  for (let w = 0; w < workspaceIds.length; w++) {
    const workspaceId = workspaceIds[w];

    for (let i = 0; i < itemsPerWorkspace; i++) {
      const itemNum = (w * itemsPerWorkspace) + i + 1;

      try {
        // Create item
        const item = await request('/items', {
          method: 'POST',
          body: JSON.stringify({
            title: `Pre-populated Item ${itemNum}`,
            description: `Item ${itemNum} for capacity testing`,
            status: 'open',
            priority: 'medium',
            workspace_id: workspaceId
          })
        });

        itemIds.push(item.id);

        // Create 2-3 updates to generate history
        const updates = Math.floor(Math.random() * 2) + 2; // 2 or 3 updates

        for (let u = 0; u < updates; u++) {
          const statuses = ['open', 'in-progress', 'done'];
          const priorities = ['low', 'medium', 'high'];

          try {
            await request(`/items/${item.id}`, {
              method: 'PUT',
              body: JSON.stringify({
                ...item,
                status: statuses[u % statuses.length],
                priority: priorities[Math.floor(Math.random() * priorities.length)],
                description: `${item.description} - Updated ${u + 1}`
              })
            });
          } catch (updateError) {
            errorCount++;
            if (errorCount <= 5) {
              console.error(`\nUpdate error for item ${item.id}: ${updateError.message}`);
            }
          }
        }

        if (itemNum % 100 === 0) {
          process.stdout.write(`\r   Created ${itemNum}/${TOTAL_ITEMS} items with history (errors: ${errorCount})...`);
        }
      } catch (createError) {
        errorCount++;
        if (errorCount <= 5) {
          console.error(`\nCreate error for item ${itemNum}: ${createError.message}`);
        }
        // Continue creating items even if one fails
      }
    }
  }

  console.log(`\n✅ Pre-populated ${itemIds.length} items with history`);

  // Export workspace and item IDs for test
  const fs = require('fs');
  fs.writeFileSync('/tmp/prepopulate_data.json', JSON.stringify({
    workspaceIds,
    itemIds
  }));

  console.log('');
}

prepopulate().catch(error => {
  console.error('Pre-population failed:', error);
  process.exit(1);
});
PREPOPULATE_SCRIPT

# Run pre-population
export API_BASE="$API_BASE/api"
export BEARER_TOKEN
export WORKSPACES
export PRE_POPULATE_ITEMS

node /tmp/prepopulate_$$.js
if [ $? -ne 0 ]; then
    echo "❌ Pre-population failed"
    rm -f /tmp/prepopulate_$$.js
    show_server_logs
    exit 1
fi

# Load pre-populated data
if [ -f /tmp/prepopulate_data.json ]; then
    export PREPOPULATE_DATA="$(cat /tmp/prepopulate_data.json)"
fi

rm -f /tmp/prepopulate_$$.js /tmp/prepopulate_data.json

# Start diagnostics monitoring if enabled
if [ "$ENABLE_DIAGNOSTICS" = "true" ]; then
    echo "📊 Starting PostgreSQL diagnostics monitoring..."
    DIAGNOSTICS_LOG="/tmp/postgres-diagnostics-$(date +%s).log"

    # Create diagnostic collection script
    cat > /tmp/diagnostic_collector_$$.sh <<'DIAGNOSTIC_SCRIPT'
#!/bin/bash
DB_NAME=$1
LOG_FILE=$2
INTERVAL=10  # Collect every 10 seconds

echo "Starting diagnostics collection for database: $DB_NAME" > "$LOG_FILE"
echo "Collection started at: $(date)" >> "$LOG_FILE"
echo "" >> "$LOG_FILE"

while true; do
    echo "========================================" >> "$LOG_FILE"
    echo "Snapshot at: $(date '+%Y-%m-%d %H:%M:%S')" >> "$LOG_FILE"
    echo "========================================" >> "$LOG_FILE"

    # Connection stats
    echo "" >> "$LOG_FILE"
    echo "Connection Status:" >> "$LOG_FILE"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -t -c \
        "SELECT state, COUNT(*) FROM pg_stat_activity GROUP BY state;" \
        2>/dev/null >> "$LOG_FILE"

    # Table stats
    echo "" >> "$LOG_FILE"
    echo "Items Table Stats:" >> "$LOG_FILE"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -t -c \
        "SELECT seq_scan, idx_scan, n_tup_ins, n_tup_upd FROM pg_stat_user_tables WHERE relname = 'items';" \
        2>/dev/null >> "$LOG_FILE"

    # Slowest queries
    echo "" >> "$LOG_FILE"
    echo "Top 5 Slowest Queries:" >> "$LOG_FILE"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -t -c \
        "SELECT calls, ROUND(mean_exec_time::numeric, 2) as mean_ms, LEFT(query, 100)
         FROM pg_stat_statements
         WHERE calls > 5
         ORDER BY mean_exec_time DESC
         LIMIT 5;" \
        2>/dev/null >> "$LOG_FILE" || echo "pg_stat_statements not available" >> "$LOG_FILE"

    echo "" >> "$LOG_FILE"
    sleep $INTERVAL
done
DIAGNOSTIC_SCRIPT

    chmod +x /tmp/diagnostic_collector_$$.sh
    /tmp/diagnostic_collector_$$.sh "$TEST_DB" "$DIAGNOSTICS_LOG" &
    DIAGNOSTICS_PID=$!

    echo "   Diagnostics PID: $DIAGNOSTICS_PID"
    echo "   Log file: $DIAGNOSTICS_LOG"
    echo ""
fi

# Run the capacity test
echo "🔥 Starting capacity test with gradual ramp-up..."
echo ""
export START_CONCURRENCY
export MAX_CONCURRENCY
export TEST_DURATION
export RAMP_INTERVAL
export RAMP_INCREMENT

node realistic-load-test.js
TEST_EXIT_CODE=$?

# Stop diagnostics if running
if [ -n "$DIAGNOSTICS_PID" ] && kill -0 "$DIAGNOSTICS_PID" 2>/dev/null; then
    kill "$DIAGNOSTICS_PID" 2>/dev/null || true
    wait "$DIAGNOSTICS_PID" 2>/dev/null || true

    if [ -f "$DIAGNOSTICS_LOG" ]; then
        echo ""
        echo "📊 Diagnostics Summary"
        echo "===================="
        echo "Full diagnostics saved to: $DIAGNOSTICS_LOG"
        echo ""
        echo "Final Statistics:"
        tail -50 "$DIAGNOSTICS_LOG"
    fi

    rm -f /tmp/diagnostic_collector_$$.sh
fi

# Show server logs if test failed
if [ $TEST_EXIT_CODE -ne 0 ]; then
    show_server_logs
fi

echo ""
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "✅ Capacity test completed successfully!"
else
    echo "❌ Capacity test failed with errors!"
fi

exit $TEST_EXIT_CODE
