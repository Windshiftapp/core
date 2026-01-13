#!/bin/bash

# Windshift Load Test Runner
# Runs load testing against an isolated test server or remote host

set -e

echo "🚀 Windshift Load Test Runner"
echo "=========================="

# Parse command line arguments
USE_REMOTE=false
REMOTE_URL=""
WORKSPACES=100
ITEMS_PER_WORKSPACE=1000
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
    -h|--help)
      echo ""
      echo "Usage: $0 [OPTIONS]"
      echo ""
      echo "Options:"
      echo "  --remote URL          Test against remote server instead of local"
      echo "  --workspaces N        Number of workspaces to create (default: 100)"
      echo "  --items N             Items per workspace (default: 1000)"
      echo "  --concurrency N       Concurrent requests (default: 50)"
      echo "  -h, --help            Show this help message"
      echo ""
      echo "Examples:"
      echo "  $0                                    # Local test with defaults"
      echo "  $0 --workspaces 10 --items 100        # Smaller local test"
      echo "  $0 --remote http://example.com:8080   # Test remote server"
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
    echo "⚠️  WARNING: This will create $(($WORKSPACES * $ITEMS_PER_WORKSPACE)) items on the remote server!"
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
    echo "   Total Items:       $(($WORKSPACES * $ITEMS_PER_WORKSPACE))"
    echo "   Concurrency:       $CONCURRENCY"
    echo ""

    # Note: Remote testing requires authentication token to be set
    if [ -z "$BEARER_TOKEN" ]; then
        echo "❌ BEARER_TOKEN environment variable must be set for remote testing"
        echo "   Example: export BEARER_TOKEN=crw_your_token_here"
        exit 1
    fi

    cd "$(dirname "$0")"

    # Run the load test
    echo "🚀 Starting load test..."
    export WORKSPACES
    export ITEMS_PER_WORKSPACE
    export CONCURRENCY

    node load-test.js
    TEST_EXIT_CODE=$?

else
    # Local testing mode with isolated database
    echo "🗄️  Testing local server with isolated database"

    # Generate unique database and port for this test run
    TEST_DB="loadtest_$(date +%s)_$$.db"
    TEST_PORT=$(( 8000 + (RANDOM % 1000) ))
    API_BASE="http://localhost:$TEST_PORT"

    echo "   Database:          $TEST_DB"
    echo "   Port:              $TEST_PORT"
    echo "   Workspaces:        $WORKSPACES"
    echo "   Items/Workspace:   $ITEMS_PER_WORKSPACE"
    echo "   Total Items:       $(($WORKSPACES * $ITEMS_PER_WORKSPACE))"
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
    SSO_SECRET="loadtest-secret-key-12345" ./windshift -db "$TEST_DB" -p "$TEST_PORT" --allowed-hosts "localhost,127.0.0.1" > "loadtest_server.log" 2>&1 &
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

    # Run the load test
    echo "🚀 Starting load test..."
    export API_BASE="$API_BASE/api"
    export WORKSPACES
    export ITEMS_PER_WORKSPACE
    export CONCURRENCY

    node load-test.js
    TEST_EXIT_CODE=$?

    # Show server logs if test failed
    if [ $TEST_EXIT_CODE -ne 0 ]; then
        show_server_logs
    fi

    # Extract and analyze profiling data from server logs
    echo ""
    echo "📊 Server-Side Profiling Summary"
    echo "================================="
    cd "$PROJECT_ROOT"

    PERF_LINES=$(grep -c '\[PERF\]' "loadtest_server.log" 2>/dev/null) || PERF_LINES=0
    echo "Total profiled requests: $PERF_LINES"

    if [ "$PERF_LINES" -gt 0 ]; then
        echo ""
        echo "Timing breakdown (in milliseconds):"
        echo "-----------------------------------"

        # Extract timing data and calculate statistics using awk
        grep '\[PERF\]' "loadtest_server.log" | head -1000 | awk '
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
                print "  lock   = advisory lock wait (0 for SQLite)"
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
        grep '\[PERF\]' "loadtest_server.log" | head -5
    fi
fi

echo ""
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "✅ Load test completed successfully!"
else
    echo "❌ Load test failed with errors!"
fi

exit $TEST_EXIT_CODE
