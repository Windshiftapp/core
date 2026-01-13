#!/bin/bash

# Monitor PostgreSQL performance during stress test
# Usage: ./monitor-postgres.sh <database_name>

DB_NAME=$1

if [ -z "$DB_NAME" ]; then
    echo "Usage: $0 <database_name>"
    echo ""
    echo "Example:"
    echo "  $0 windshift_capacity_12345678_98765"
    echo ""
    exit 1
fi

echo "🔍 Monitoring PostgreSQL Performance"
echo "====================================="
echo "Database: $DB_NAME"
echo "Press Ctrl+C to stop monitoring"
echo ""

while true; do
    clear
    echo "========================================="
    echo "📊 PostgreSQL Performance Monitor"
    echo "Time: $(date '+%Y-%m-%d %H:%M:%S')"
    echo "========================================="

    # Connection counts by state
    echo ""
    echo "🔌 Connection Status:"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -t -c \
        "SELECT state, COUNT(*) as count FROM pg_stat_activity GROUP BY state ORDER BY count DESC;" \
        2>/dev/null | head -10

    # Long-running queries
    echo ""
    echo "⏱️  Long-Running Queries (>1 second):"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -t -c \
        "SELECT pid,
                ROUND(EXTRACT(EPOCH FROM (NOW() - query_start))::numeric, 2) as duration_sec,
                state,
                LEFT(query, 80) as query_preview
         FROM pg_stat_activity
         WHERE state != 'idle'
           AND query NOT LIKE '%pg_stat_activity%'
           AND (NOW() - query_start) > interval '1 second'
         ORDER BY duration_sec DESC
         LIMIT 10;" \
        2>/dev/null || echo "  No long-running queries"

    # Lock statistics
    echo ""
    echo "🔒 Lock Counts by Type:"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -t -c \
        "SELECT locktype, mode, COUNT(*) as lock_count
         FROM pg_locks
         GROUP BY locktype, mode
         ORDER BY lock_count DESC
         LIMIT 10;" \
        2>/dev/null | head -10

    # Blocked queries count
    echo ""
    echo "🚫 Blocked Queries:"
    BLOCKED_COUNT=$(docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -t -c \
        "SELECT COUNT(*) FROM pg_locks WHERE NOT granted;" \
        2>/dev/null | tr -d ' ')

    if [ "$BLOCKED_COUNT" -gt 0 ]; then
        echo "  ⚠️  $BLOCKED_COUNT queries are blocked!"

        # Show what's blocking
        echo ""
        echo "  Blocking Details:"
        docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -t -c \
            "SELECT blocked_locks.pid AS blocked_pid,
                    blocking_locks.pid AS blocking_pid,
                    LEFT(blocked_activity.query, 60) AS blocked_query,
                    LEFT(blocking_activity.query, 60) AS blocking_query
             FROM pg_catalog.pg_locks blocked_locks
             JOIN pg_catalog.pg_stat_activity blocked_activity ON blocked_activity.pid = blocked_locks.pid
             JOIN pg_catalog.pg_locks blocking_locks
                 ON blocking_locks.locktype = blocked_locks.locktype
                 AND blocking_locks.pid != blocked_locks.pid
             JOIN pg_catalog.pg_stat_activity blocking_activity ON blocking_activity.pid = blocking_locks.pid
             WHERE NOT blocked_locks.granted
             LIMIT 5;" \
            2>/dev/null
    else
        echo "  ✅ No blocked queries"
    fi

    # Table statistics (sequential scans vs index scans)
    echo ""
    echo "📊 Items Table Access Patterns:"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -t -c \
        "SELECT
            seq_scan as sequential_scans,
            idx_scan as index_scans,
            n_tup_ins as inserts,
            n_tup_upd as updates,
            n_dead_tup as dead_tuples
         FROM pg_stat_user_tables
         WHERE relname = 'items';" \
        2>/dev/null || echo "  Stats not available yet"

    # Top queries by execution time (if pg_stat_statements is available)
    echo ""
    echo "🐌 Slowest Queries (by mean time):"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -t -c \
        "SELECT
            calls,
            ROUND(mean_exec_time::numeric, 2) as mean_ms,
            ROUND(max_exec_time::numeric, 2) as max_ms,
            LEFT(query, 80) as query_preview
         FROM pg_stat_statements
         WHERE calls > 5
         ORDER BY mean_exec_time DESC
         LIMIT 5;" \
        2>/dev/null || echo "  pg_stat_statements not available (enable it first)"

    sleep 5
done
