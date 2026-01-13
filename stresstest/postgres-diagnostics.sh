#!/bin/bash

# Run comprehensive PostgreSQL diagnostics
# Usage: ./postgres-diagnostics.sh <database_name> [output_file]

DB_NAME=$1
OUTPUT_FILE=${2:-"postgres-diagnostics-$(date +%Y%m%d-%H%M%S).txt"}

if [ -z "$DB_NAME" ]; then
    echo "Usage: $0 <database_name> [output_file]"
    echo ""
    echo "Example:"
    echo "  $0 windshift_capacity_12345678_98765"
    echo "  $0 windshift_capacity_12345678_98765 baseline.txt"
    echo ""
    exit 1
fi

echo "📊 Running PostgreSQL Diagnostics"
echo "===================================="
echo "Database:    $DB_NAME"
echo "Output File: $OUTPUT_FILE"
echo ""

{
    echo "======================================================================"
    echo "PostgreSQL Performance Diagnostics Report"
    echo "======================================================================"
    echo "Database:  $DB_NAME"
    echo "Timestamp: $(date '+%Y-%m-%d %H:%M:%S')"
    echo ""

    # ===== CONFIGURATION =====
    echo "======================================================================"
    echo "PostgreSQL Configuration"
    echo "======================================================================"
    echo ""
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -c \
        "SELECT name, setting, unit, context
         FROM pg_settings
         WHERE name IN ('max_connections', 'shared_buffers', 'effective_cache_size',
                        'work_mem', 'maintenance_work_mem', 'random_page_cost',
                        'effective_io_concurrency', 'max_wal_size', 'shared_preload_libraries',
                        'log_min_duration_statement', 'checkpoint_completion_target')
         ORDER BY name;" \
        2>/dev/null

    # ===== CONNECTION STATISTICS =====
    echo ""
    echo "======================================================================"
    echo "Connection Statistics"
    echo "======================================================================"
    echo ""
    echo "Connections by State:"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -c \
        "SELECT state,
                COUNT(*) as connection_count,
                ROUND(COUNT(*) * 100.0 / SUM(COUNT(*)) OVER (), 2) as percentage
         FROM pg_stat_activity
         GROUP BY state
         ORDER BY connection_count DESC;" \
        2>/dev/null

    echo ""
    echo "Total Connections:"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -c \
        "SELECT COUNT(*) as total_connections,
                COUNT(*) FILTER (WHERE state = 'active') as active,
                COUNT(*) FILTER (WHERE state = 'idle') as idle,
                COUNT(*) FILTER (WHERE state = 'idle in transaction') as idle_in_transaction
         FROM pg_stat_activity;" \
        2>/dev/null

    # ===== TABLE STATISTICS =====
    echo ""
    echo "======================================================================"
    echo "Table Access Statistics"
    echo "======================================================================"
    echo ""
    echo "Top 10 Tables by Activity:"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -c \
        "SELECT relname,
                seq_scan as sequential_scans,
                seq_tup_read as seq_tuples,
                idx_scan as index_scans,
                idx_tup_fetch as idx_tuples,
                n_tup_ins as inserts,
                n_tup_upd as updates,
                n_tup_del as deletes,
                n_live_tup as live_tuples,
                n_dead_tup as dead_tuples,
                CASE
                    WHEN n_live_tup + n_dead_tup > 0
                    THEN ROUND(n_dead_tup * 100.0 / (n_live_tup + n_dead_tup), 2)
                    ELSE 0
                END as dead_tuple_pct
         FROM pg_stat_user_tables
         WHERE schemaname = 'public'
         ORDER BY (seq_scan + idx_scan) DESC
         LIMIT 10;" \
        2>/dev/null

    # ===== INDEX USAGE =====
    echo ""
    echo "======================================================================"
    echo "Index Usage Statistics"
    echo "======================================================================"
    echo ""
    echo "Items Table Indexes:"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -c \
        "SELECT indexrelname as index_name,
                idx_scan as scans,
                idx_tup_read as tuples_read,
                idx_tup_fetch as tuples_fetched,
                pg_size_pretty(pg_relation_size(indexrelid)) as size
         FROM pg_stat_user_indexes
         WHERE schemaname = 'public'
           AND relname = 'items'
         ORDER BY idx_scan DESC;" \
        2>/dev/null

    echo ""
    echo "Unused Indexes (0 scans):"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -c \
        "SELECT schemaname,
                relname as table_name,
                indexrelname as index_name,
                pg_size_pretty(pg_relation_size(indexrelid)) as size
         FROM pg_stat_user_indexes
         WHERE idx_scan = 0
           AND indexrelname NOT LIKE '%pkey'
           AND schemaname = 'public'
         ORDER BY pg_relation_size(indexrelid) DESC
         LIMIT 10;" \
        2>/dev/null

    # ===== QUERY PERFORMANCE (pg_stat_statements) =====
    echo ""
    echo "======================================================================"
    echo "Query Performance Statistics (pg_stat_statements)"
    echo "======================================================================"
    echo ""

    # Check if pg_stat_statements is available
    HAS_PG_STAT=$(docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -t -c \
        "SELECT COUNT(*) FROM pg_extension WHERE extname = 'pg_stat_statements';" \
        2>/dev/null | tr -d ' ')

    if [ "$HAS_PG_STAT" -eq "1" ]; then
        echo "Top 10 Queries by Total Execution Time:"
        docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -c \
            "SELECT calls,
                    ROUND(total_exec_time::numeric, 2) as total_time_ms,
                    ROUND(mean_exec_time::numeric, 2) as mean_time_ms,
                    ROUND(max_exec_time::numeric, 2) as max_time_ms,
                    ROUND(stddev_exec_time::numeric, 2) as stddev_ms,
                    LEFT(query, 100) as query_preview
             FROM pg_stat_statements
             WHERE dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
             ORDER BY total_exec_time DESC
             LIMIT 10;" \
            2>/dev/null

        echo ""
        echo "Top 10 Slowest Queries by Mean Execution Time:"
        docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -c \
            "SELECT calls,
                    ROUND(mean_exec_time::numeric, 2) as mean_time_ms,
                    ROUND(max_exec_time::numeric, 2) as max_time_ms,
                    ROUND(total_exec_time::numeric, 2) as total_time_ms,
                    LEFT(query, 100) as query_preview
             FROM pg_stat_statements
             WHERE dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
               AND calls > 10
             ORDER BY mean_exec_time DESC
             LIMIT 10;" \
            2>/dev/null
    else
        echo "⚠️  pg_stat_statements extension not enabled"
        echo "   Enable with: CREATE EXTENSION pg_stat_statements;"
    fi

    # ===== LOCK STATISTICS =====
    echo ""
    echo "======================================================================"
    echo "Lock Statistics"
    echo "======================================================================"
    echo ""
    echo "Current Locks by Type and Mode:"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -c \
        "SELECT locktype,
                mode,
                COUNT(*) as lock_count,
                COUNT(*) FILTER (WHERE NOT granted) as waiting_count
         FROM pg_locks
         GROUP BY locktype, mode
         ORDER BY lock_count DESC
         LIMIT 15;" \
        2>/dev/null

    echo ""
    echo "Blocked Queries:"
    BLOCKED_COUNT=$(docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -t -c \
        "SELECT COUNT(*) FROM pg_locks WHERE NOT granted;" \
        2>/dev/null | tr -d ' ')

    if [ "$BLOCKED_COUNT" -gt 0 ]; then
        echo "⚠️  Found $BLOCKED_COUNT blocked queries"
    else
        echo "✅ No blocked queries"
    fi

    # ===== DATABASE SIZE =====
    echo ""
    echo "======================================================================"
    echo "Database Size"
    echo "======================================================================"
    echo ""
    echo "Total Database Size:"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -c \
        "SELECT pg_size_pretty(pg_database_size(current_database())) as database_size;" \
        2>/dev/null

    echo ""
    echo "Top 10 Tables by Size:"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -c \
        "SELECT relname as table_name,
                pg_size_pretty(pg_total_relation_size(relid)) as total_size,
                pg_size_pretty(pg_relation_size(relid)) as table_size,
                pg_size_pretty(pg_total_relation_size(relid) - pg_relation_size(relid)) as index_size,
                ROUND(100.0 * pg_relation_size(relid) / NULLIF(pg_total_relation_size(relid), 0), 2) as table_pct
         FROM pg_stat_user_tables
         WHERE schemaname = 'public'
         ORDER BY pg_total_relation_size(relid) DESC
         LIMIT 10;" \
        2>/dev/null

    # ===== AUTOVACUUM STATUS =====
    echo ""
    echo "======================================================================"
    echo "Autovacuum Status"
    echo "======================================================================"
    echo ""
    echo "Autovacuum Configuration:"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -c \
        "SELECT name, setting
         FROM pg_settings
         WHERE name LIKE 'autovacuum%'
         ORDER BY name;" \
        2>/dev/null

    echo ""
    echo "Recent Autovacuum Activity:"
    docker exec windshift-postgres-test psql -U windshift_test -d "$DB_NAME" -c \
        "SELECT relname,
                last_vacuum,
                last_autovacuum,
                vacuum_count,
                autovacuum_count,
                n_dead_tup,
                n_live_tup
         FROM pg_stat_user_tables
         WHERE schemaname = 'public'
           AND (last_autovacuum IS NOT NULL OR autovacuum_count > 0)
         ORDER BY last_autovacuum DESC NULLS LAST
         LIMIT 10;" \
        2>/dev/null

    # ===== SUMMARY =====
    echo ""
    echo "======================================================================"
    echo "Diagnostic Summary"
    echo "======================================================================"
    echo ""
    echo "Report generated: $(date '+%Y-%m-%d %H:%M:%S')"
    echo ""

} > "$OUTPUT_FILE"

echo "✅ Diagnostics complete!"
echo ""
echo "Report saved to: $OUTPUT_FILE"
echo ""
echo "To view the report:"
echo "  cat $OUTPUT_FILE"
echo ""
