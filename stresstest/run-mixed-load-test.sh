#!/bin/bash

# Windshift Mixed Load Test Runner
# Runs realistic mixed workload (read/write/update) against test or remote server

set -e

echo "🚀 Windshift Mixed Load Test Runner"
echo "================================="

# Parse command line arguments
USE_REMOTE=false
REMOTE_URL=""
DURATION=60
WORKSPACES=10
INITIAL_ITEMS=100
CONCURRENCY=50
CREATE_WEIGHT=40
READ_WEIGHT=40
UPDATE_WEIGHT=20

while [[ $# -gt 0 ]]; do
  case $1 in
    --remote)
      USE_REMOTE=true
      REMOTE_URL="$2"
      shift 2
      ;;
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
      echo "  --remote URL          Test against remote server instead of local"
      echo "  --duration N          Test duration in seconds (default: 60)"
      echo "  --workspaces N        Number of workspaces (default: 10)"
      echo "  --initial-items N     Items to create before test (default: 100)"
      echo "  --concurrency N       Concurrent workers (default: 50)"
      echo "  --create N            % of create operations (default: 40)"
      echo "  --read N              % of read operations (default: 40)"
      echo "  --update N            % of update operations (default: 20)"
      echo "  -h, --help            Show this help message"
      echo ""
      echo "Examples:"
      echo "  $0                                        # Run with defaults"
      echo "  $0 --duration 120 --concurrency 100       # Longer, more concurrent"
      echo "  $0 --create 20 --read 60 --update 20      # Read-heavy workload"
      echo "  $0 --remote http://example.com:8080       # Test remote server"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      echo "Use -h or --help for usage information"
      exit 1
      ;;
  esac
done

# Validate operation mix adds up to 100
TOTAL_WEIGHT=$((CREATE_WEIGHT + READ_WEIGHT + UPDATE_WEIGHT))
if [ $TOTAL_WEIGHT -ne 100 ]; then
    echo "❌ Error: Operation weights must add up to 100% (currently: ${TOTAL_WEIGHT}%)"
    echo "   --create ${CREATE_WEIGHT} + --read ${READ_WEIGHT} + --update ${UPDATE_WEIGHT} = ${TOTAL_WEIGHT}"
    exit 1
fi

# Check if Node.js is installed
if ! command -v node &> /dev/null; then
    echo "❌ Node.js is not installed. Please install Node.js first."
    exit 1
fi

if [ "$USE_REMOTE" = true ]; then
    # Remote testing mode
    echo "🌐 Testing remote server: $REMOTE_URL"
    echo "⚠️  WARNING: This will create data on the remote server!"
    echo ""
    read -p "Continue? (yes/no): " confirm
    if [ "$confirm" != "yes" ]; then
        echo "Aborted."
        exit 0
    fi

    export API_BASE="${REMOTE_URL}/api"

    echo ""
    echo "⚙️  Configuration:"
    echo "   Target:            $API_BASE"
    echo "   Duration:          ${DURATION}s"
    echo "   Workspaces:        $WORKSPACES"
    echo "   Initial Items:     $INITIAL_ITEMS"
    echo "   Concurrency:       $CONCURRENCY"
    echo "   Operation Mix:     Create ${CREATE_WEIGHT}% / Read ${READ_WEIGHT}% / Update ${UPDATE_WEIGHT}%"
    echo ""

    # Note: Remote testing requires authentication token to be set
    if [ -z "$BEARER_TOKEN" ]; then
        echo "❌ BEARER_TOKEN environment variable must be set for remote testing"
        echo "   Example: export BEARER_TOKEN=crw_your_token_here"
        exit 1
    fi

    # Get script directory
    SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
    cd "$SCRIPT_DIR"

    # Run the load test
    echo "🚀 Starting mixed load test..."
    export DURATION
    export WORKSPACES
    export INITIAL_ITEMS
    export CONCURRENCY
    export CREATE_WEIGHT
    export READ_WEIGHT
    export UPDATE_WEIGHT

    node mixed-load-test.js
    TEST_EXIT_CODE=$?

else
    # Local testing mode with isolated database
    echo "🗄️  Testing local server with isolated database"

    # Get script directory and find project root
    SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
    PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

    # Generate unique database and port for this test run
    TEST_DB="mixedload_$(date +%s)_$$.db"
    TEST_PORT=$(( 8000 + (RANDOM % 1000) ))
    API_BASE="http://localhost:$TEST_PORT"

    echo "   Database:          $TEST_DB"
    echo "   Port:              $TEST_PORT"
    echo "   Duration:          ${DURATION}s"
    echo "   Workspaces:        $WORKSPACES"
    echo "   Initial Items:     $INITIAL_ITEMS"
    echo "   Concurrency:       $CONCURRENCY"
    echo "   Operation Mix:     Create ${CREATE_WEIGHT}% / Read ${READ_WEIGHT}% / Update ${UPDATE_WEIGHT}%"
    echo ""

    # Change to project root to run the server
    cd "$PROJECT_ROOT"

    # Start test server with isolated database
    echo "🚀 Starting test server..."
    ./windshift -db "$TEST_DB" -p "$TEST_PORT" > "mixedload_server.log" 2>&1 &
    SERVER_PID=$!

    # Function to show server logs on errors
    show_server_logs() {
        echo ""
        echo "🚨 Server logs (last 30 lines):"
        echo "================================="
        tail -30 "mixedload_server.log" 2>/dev/null || echo "No server logs available"
        echo "================================="
        echo ""
    }

    # Function to cleanup on exit
    cleanup() {
        echo ""
        echo "🧹 Cleaning up..."
        if kill -0 "$SERVER_PID" 2>/dev/null; then
            echo "   Stopping test server (PID: $SERVER_PID)..."
            kill "$SERVER_PID"
            wait "$SERVER_PID" 2>/dev/null || true
        fi
        echo "   Removing test database: $TEST_DB"
        rm -f "$TEST_DB"
        rm -f "mixedload_server.log"
        echo "✅ Cleanup complete"
    }

    # Set trap to cleanup on script exit (success or failure)
    trap cleanup EXIT

    # Wait for server to start
    echo "⏱️  Waiting for test server to start..."
    for i in {1..30}; do
        if curl -s "$API_BASE/api/setup/status" > /dev/null 2>&1; then
            echo "✅ Test server is running"
            break
        fi
        if [ $i -eq 30 ]; then
            echo "❌ Test server failed to start within 30 seconds"
            echo "   Server log:"
            cat "mixedload_server.log" || true
            exit 1
        fi
        sleep 1
    done

    # Wait a moment for database migrations to complete
    sleep 2

    # Setup test authentication data
    echo "🔧 Setting up test authentication data..."

    sqlite3 "$TEST_DB" << 'EOF'
-- Create api_tokens table if it doesn't exist
CREATE TABLE IF NOT EXISTS api_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    permissions TEXT DEFAULT '["read"]',
    expires_at DATETIME NULL,
    last_used_at DATETIME NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Create system_settings table if it doesn't exist
CREATE TABLE IF NOT EXISTS system_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Create admin user with known password hash for "testpass123"
INSERT OR IGNORE INTO users (id, email, username, first_name, last_name, is_active, password_hash)
VALUES (1, 'admin@test.com', 'admin', 'Test', 'Admin', 1, '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy');

-- Create test bearer token (hash of "crw_test1234567890abcdef1234567890abcdef")
INSERT OR IGNORE INTO api_tokens (id, user_id, name, token_hash, token_prefix, permissions)
VALUES (1, 1, 'Mixed Load Test API Token', '$2a$10$RcUp3viPi3V1AWoJGB5DyOM7Sxzr5fdRQURhm5ZIA0QLXPFmuxLtK', 'crw_test1234...', '["read","write","admin"]');

-- Grant system.admin permission to the user
INSERT OR IGNORE INTO user_global_permissions (user_id, permission_id)
SELECT 1, id FROM permissions WHERE permission_key = 'system.admin';

-- Mark setup as complete
INSERT OR IGNORE INTO system_settings (key, value) VALUES ('setup_completed', 'true');
EOF

    echo "✅ Test authentication data setup complete"
    echo ""

    # Change to stresstest directory
    cd "$SCRIPT_DIR"

    # Run the load test
    echo "🚀 Starting mixed load test..."
    export API_BASE="$API_BASE/api"
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
        show_server_logs
    fi
fi

echo ""
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "✅ Mixed load test completed successfully!"
else
    echo "❌ Mixed load test failed with errors!"
fi

exit $TEST_EXIT_CODE
