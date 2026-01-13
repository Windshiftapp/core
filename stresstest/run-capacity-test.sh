#!/bin/bash

# Windshift Capacity Test Runner
# Finds maximum concurrent users for SQLite by gradual ramp-up until failure

set -e

echo "🚀 Windshift SQLite Capacity Test"
echo "======================================"

# Configuration
USE_REMOTE=false
REMOTE_URL=""
PRE_POPULATE_ITEMS=5000
WORKSPACES=10
START_CONCURRENCY=10
MAX_CONCURRENCY=500
TEST_DURATION=300  # 5 minutes
RAMP_INTERVAL=30   # Increase every 30 seconds
RAMP_INCREMENT=10  # Add 10 users each time

while [[ $# -gt 0 ]]; do
  case $1 in
    --remote)
      USE_REMOTE=true
      REMOTE_URL="$2"
      shift 2
      ;;
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
    -h|--help)
      echo ""
      echo "Usage: $0 [OPTIONS]"
      echo ""
      echo "Options:"
      echo "  --remote URL          Test against remote server"
      echo "  --items N             Items to pre-populate (default: 5000)"
      echo "  --duration N          Test duration in seconds (default: 300)"
      echo "  --start-users N       Starting concurrent users (default: 10)"
      echo "  --max-users N         Maximum concurrent users (default: 500)"
      echo "  --ramp-interval N     Seconds between ramp-ups (default: 30)"
      echo "  -h, --help            Show this help"
      echo ""
      echo "Examples:"
      echo "  $0                              # Local test with defaults"
      echo "  $0 --items 10000 --max-users 200   # Larger test"
      echo "  $0 --remote http://example.com:8080 # Remote server"
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
    echo "⚠️  WARNING: This will create $PRE_POPULATE_ITEMS items on the remote server!"
    echo ""
    read -p "Continue? (yes/no): " confirm
    if [ "$confirm" != "yes" ]; then
        echo "Aborted."
        exit 0
    fi

    export API_BASE="${REMOTE_URL}/api"

    if [ -z "$BEARER_TOKEN" ]; then
        echo "❌ BEARER_TOKEN environment variable must be set for remote testing"
        echo "   Example: export BEARER_TOKEN=crw_your_token_here"
        exit 1
    fi

    echo ""
    echo "⚙️  Configuration:"
    echo "   Target:            $API_BASE"
    echo "   Pre-populate:      $PRE_POPULATE_ITEMS items"
    echo "   Test Duration:     ${TEST_DURATION}s"
    echo "   Start Users:       $START_CONCURRENCY"
    echo "   Max Users:         $MAX_CONCURRENCY"
    echo "   Ramp Interval:     ${RAMP_INTERVAL}s (+${RAMP_INCREMENT} users)"
    echo ""

    cd "$(dirname "$0")"

    # Run capacity test (pre-population happens inside Node.js script)
    echo "🚀 Starting capacity test..."
    export WORKSPACES
    export PRE_POPULATE_ITEMS
    export START_CONCURRENCY
    export MAX_CONCURRENCY
    export TEST_DURATION
    export RAMP_INTERVAL
    export RAMP_INCREMENT

    node realistic-load-test.js
    TEST_EXIT_CODE=$?

else
    # Local testing mode with isolated database
    echo "🗄️  Testing local server with isolated database"

    # Generate unique database and port for this test run
    TEST_DB="capacity_$(date +%s)_$$.db"
    TEST_PORT=$(( 8000 + (RANDOM % 1000) ))
    API_BASE="http://localhost:$TEST_PORT"

    echo "   Database:          $TEST_DB"
    echo "   Port:              $TEST_PORT"
    echo "   Pre-populate:      $PRE_POPULATE_ITEMS items"
    echo "   Test Duration:     ${TEST_DURATION}s"
    echo "   Start Users:       $START_CONCURRENCY"
    echo "   Max Users:         $MAX_CONCURRENCY"
    echo "   Ramp Interval:     ${RAMP_INTERVAL}s (+${RAMP_INCREMENT} users)"
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
    ./windshift -db "$TEST_DB" -p "$TEST_PORT" > "capacity_server.log" 2>&1 &
    SERVER_PID=$!

    # Function to show server logs on errors
    show_server_logs() {
        echo ""
        echo "🚨 Server logs (last 50 lines):"
        echo "================================="
        tail -50 "capacity_server.log" 2>/dev/null || echo "No server logs available"
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
        rm -f "$TEST_DB" "$TEST_DB-shm" "$TEST_DB-wal"
        rm -f "capacity_server.log"
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
            show_server_logs
            exit 1
        fi
        sleep 1
    done

    # Wait a moment for database migrations to complete
    sleep 2

    # Setup test authentication data
    echo "🔧 Setting up test authentication data..."

    sqlite3 "$TEST_DB" << 'EOF'
-- Create admin user with known password hash for "testpass123"
INSERT OR IGNORE INTO users (id, email, username, first_name, last_name, is_active, password_hash)
VALUES (1, 'admin@test.com', 'admin', 'Test', 'Admin', 1, '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy');

-- Create test bearer token (hash of "crw_test1234567890abcdef1234567890abcdef")
INSERT OR IGNORE INTO api_tokens (id, user_id, name, token_hash, token_prefix, permissions)
VALUES (1, 1, 'Capacity Test API Token', '$2a$10$RcUp3viPi3V1AWoJGB5DyOM7Sxzr5fdRQURhm5ZIA0QLXPFmuxLtK', 'crw_test1234...', '["read","write","admin"]');

-- Grant system.admin permission to the user
INSERT OR IGNORE INTO user_global_permissions (user_id, permission_id)
SELECT 1, id FROM permissions WHERE permission_key = 'system.admin';

-- Mark setup as complete
INSERT OR IGNORE INTO system_settings (key, value) VALUES ('setup_completed', 'true');
EOF

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
const BEARER_TOKEN = 'crw_test1234567890abcdef1234567890abcdef';
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

  for (let w = 0; w < workspaceIds.length; w++) {
    const workspaceId = workspaceIds[w];

    for (let i = 0; i < itemsPerWorkspace; i++) {
      const itemNum = (w * itemsPerWorkspace) + i + 1;

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

        await request(`/items/${item.id}`, {
          method: 'PUT',
          body: JSON.stringify({
            ...item,
            status: statuses[u % statuses.length],
            priority: priorities[Math.floor(Math.random() * priorities.length)],
            description: `${item.description} - Updated ${u + 1}`
          })
        });
      }

      if (itemNum % 100 === 0) {
        process.stdout.write(`\r   Created ${itemNum}/${TOTAL_ITEMS} items with history...`);
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

    # Show server logs if test failed
    if [ $TEST_EXIT_CODE -ne 0 ]; then
        show_server_logs
    fi
fi

echo ""
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "✅ Capacity test completed successfully!"
else
    echo "❌ Capacity test failed with errors!"
fi

exit $TEST_EXIT_CODE
