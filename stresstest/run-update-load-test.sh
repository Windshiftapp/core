#!/bin/bash

# Windshift Update Load Test Runner
# Tests performance impact of history tracking on UPDATE operations

set -e

echo "🚀 Windshift Update Load Test Runner"
echo "======================================"

# Parse command line arguments
USE_REMOTE=false
REMOTE_URL=""
WORKSPACES=10
ITEMS_PER_WORKSPACE=100
UPDATES_PER_ITEM=5
CONCURRENCY=50

while [[ $# -gt 0 ]]; do
  case $1 in
    --remote)
      USE_REMOTE=true
      REMOTE_URL="$2"
      shift 2
      ;;
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
    --updates)
      UPDATES_PER_ITEM="$2"
      shift 2
      ;;
    -h|--help)
      echo ""
      echo "Usage: $0 [OPTIONS]"
      echo ""
      echo "Options:"
      echo "  --remote URL          Test against remote server instead of local"
      echo "  --workspaces N        Number of workspaces to create (default: 10)"
      echo "  --items N             Items per workspace (default: 100)"
      echo "  --updates N           Updates per item (default: 5)"
      echo "  --concurrency N       Concurrent requests (default: 50)"
      echo "  -h, --help            Show this help message"
      echo ""
      echo "Examples:"
      echo "  $0                                        # Local test with defaults"
      echo "  $0 --workspaces 10 --items 100 --updates 5        # Custom test"
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

# Check if Node.js is installed
if ! command -v node &> /dev/null; then
    echo "❌ Node.js is not installed. Please install Node.js first."
    exit 1
fi

if [ "$USE_REMOTE" = true ]; then
    # Remote testing mode
    echo "🌐 Testing remote server: $REMOTE_URL"
    echo "⚠️  WARNING: This will create $(($WORKSPACES * $ITEMS_PER_WORKSPACE)) items and $(($WORKSPACES * $ITEMS_PER_WORKSPACE * $UPDATES_PER_ITEM)) updates!"
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
    echo "   Workspaces:        $WORKSPACES"
    echo "   Items/Workspace:   $ITEMS_PER_WORKSPACE"
    echo "   Updates/Item:      $UPDATES_PER_ITEM"
    echo "   Total Operations:  $(($WORKSPACES * $ITEMS_PER_WORKSPACE * (1 + $UPDATES_PER_ITEM)))"
    echo "   Concurrency:       $CONCURRENCY"
    echo ""

    # Note: Remote testing requires authentication token to be set
    if [ -z "$BEARER_TOKEN" ]; then
        echo "❌ BEARER_TOKEN environment variable must be set for remote testing"
        echo "   Example: export BEARER_TOKEN=crw_your_token_here"
        exit 1
    fi

    cd "$(dirname "$0")"

    # Run the update load test
    echo "🚀 Starting update load test..."
    export WORKSPACES
    export ITEMS_PER_WORKSPACE
    export UPDATES_PER_ITEM
    export CONCURRENCY

    node update-load-test.js
    TEST_EXIT_CODE=$?

else
    # Local testing mode with isolated database
    echo "🗄️  Testing local server with isolated database"

    # Generate unique database and port for this test run
    TEST_DB="updatetest_$(date +%s)_$$.db"
    TEST_PORT=$(( 8000 + (RANDOM % 1000) ))
    API_BASE="http://localhost:$TEST_PORT"

    echo "   Database:          $TEST_DB"
    echo "   Port:              $TEST_PORT"
    echo "   Workspaces:        $WORKSPACES"
    echo "   Items/Workspace:   $ITEMS_PER_WORKSPACE"
    echo "   Updates/Item:      $UPDATES_PER_ITEM"
    echo "   Total Operations:  $(($WORKSPACES * $ITEMS_PER_WORKSPACE * (1 + $UPDATES_PER_ITEM)))"
    echo "   Concurrency:       $CONCURRENCY"
    echo ""

    # Get the script directory and find project root
    SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
    PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

    # Change to project root to run the server
    cd "$PROJECT_ROOT"

    # Build the latest binary
    echo "🔨 Building latest binary..."
    go build -o windshift
    if [ $? -ne 0 ]; then
        echo "❌ Build failed"
        exit 1
    fi
    echo "✅ Build complete"
    echo ""

    # Start test server with isolated database
    echo "🚀 Starting test server..."
    ./windshift -db "$TEST_DB" -p "$TEST_PORT" > "loadtest_server.log" 2>&1 &
    SERVER_PID=$!

    # Function to show server logs on errors
    show_server_logs() {
        echo ""
        echo "🚨 Server logs (last 30 lines):"
        echo "================================="
        tail -30 "loadtest_server.log" 2>/dev/null || echo "No server logs available"
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
        rm -f "loadtest_server.log"
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
            cat "loadtest_server.log" || true
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
VALUES (1, 1, 'Load Test API Token', '$2a$10$RcUp3viPi3V1AWoJGB5DyOM7Sxzr5fdRQURhm5ZIA0QLXPFmuxLtK', 'crw_test1234...', '["read","write","admin"]');

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

    # Run the update load test
    echo "🚀 Starting update load test..."
    export API_BASE="$API_BASE/api"
    export WORKSPACES
    export ITEMS_PER_WORKSPACE
    export UPDATES_PER_ITEM
    export CONCURRENCY

    node update-load-test.js
    TEST_EXIT_CODE=$?

    # Show server logs if test failed
    if [ $TEST_EXIT_CODE -ne 0 ]; then
        show_server_logs
    fi
fi

echo ""
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "✅ Load test completed successfully!"
else
    echo "❌ Load test failed with errors!"
fi

exit $TEST_EXIT_CODE
