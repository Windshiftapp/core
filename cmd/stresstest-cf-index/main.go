// Stress test for the SQLite custom-field-index toggle, with a scaling sweep
// across multiple row counts so that performance can be compared with and
// without the index.
//
// Phases:
//
//	A scaling sweep, no index
//	  Insert rows in batches; at each --checkpoints threshold, pause and run
//	  the JSON-extract query (no index yet) — gives an unindexed scaling curve.
//
//	B enable + restart + descending sweep
//	  RecordIndex, Close, NewSQLiteDB + Initialize (this is the deferred
//	  CREATE INDEX surface), then walk back DOWN through the same checkpoints
//	  by DELETEing rows above each threshold and re-measuring. Same row counts
//	  as Phase A — yields a side-by-side unindexed vs indexed table.
//
//	C disable
//	  DROP INDEX IF EXISTS + DeleteIndexRecord at the smallest checkpoint;
//	  confirms planner falls back to SCAN and tracking row is gone.
//
//	D no-restart round-trip
//	  RecordIndex then immediate disable; confirms no orphan in sqlite_master
//	  or custom_field_indexes when admin toggles without an intervening restart.
//
// Run via core-tests/stresstest/run-index-stress.sh.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

type config struct {
	dbPath      string
	rows        int
	batchSize   int
	keep        bool
	fieldType   string
	checkpoints []int
}

type measurement struct {
	rows      int
	queryMs   float64
	planText  string
	usesIndex bool
}

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "stress test FAILED: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() config {
	var raw string
	cfg := config{}
	flag.StringVar(&cfg.dbPath, "db-path", fmt.Sprintf("/tmp/windshift-cf-stress-%d.db", os.Getpid()), "SQLite DB file path")
	flag.IntVar(&cfg.rows, "rows", 1_000_000, "number of items to insert")
	flag.IntVar(&cfg.batchSize, "batch-size", 5_000, "rows per transaction during bulk insert")
	flag.BoolVar(&cfg.keep, "keep", false, "keep the DB file after success")
	flag.StringVar(&cfg.fieldType, "field-type", "number", "custom field type: number, text, date")
	flag.StringVar(&raw, "checkpoints", "10000,50000,100000,500000,1000000", "comma-separated row counts at which to measure (clamped to --rows)")
	flag.Parse()

	switch cfg.fieldType {
	case "number", "text", "date":
	default:
		fmt.Fprintf(os.Stderr, "invalid --field-type %q (want number|text|date)\n", cfg.fieldType)
		os.Exit(2)
	}

	cps, err := parseCheckpoints(raw, cfg.rows)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --checkpoints %q: %v\n", raw, err)
		os.Exit(2)
	}
	cfg.checkpoints = cps
	return cfg
}

// parseCheckpoints accepts a comma-separated list of positive ints. It filters
// to values <= maxRows, ensures maxRows itself is present (so the curve always
// terminates at the row count actually being inserted), de-duplicates and
// returns ascending.
func parseCheckpoints(raw string, maxRows int) ([]int, error) {
	seen := map[int]bool{}
	var out []int
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("bad checkpoint %q: %w", p, err)
		}
		if n <= 0 {
			return nil, fmt.Errorf("checkpoint must be positive: %d", n)
		}
		if n > maxRows {
			continue
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	if !seen[maxRows] {
		out = append(out, maxRows)
	}
	sort.Ints(out)
	if len(out) == 0 {
		return nil, fmt.Errorf("no usable checkpoints")
	}
	return out, nil
}

func run(cfg config) error {
	fmt.Printf("=== Custom Field Index Stress Test ===\n")
	fmt.Printf("DB path:     %s\n", cfg.dbPath)
	fmt.Printf("Rows:        %d\n", cfg.rows)
	fmt.Printf("Batch size:  %d\n", cfg.batchSize)
	fmt.Printf("Field type:  %s\n", cfg.fieldType)
	fmt.Printf("Checkpoints: %v\n", cfg.checkpoints)
	fmt.Println()

	if _, err := os.Stat(cfg.dbPath); err == nil {
		return fmt.Errorf("db file already exists: %s (delete it or pass a different --db-path)", cfg.dbPath)
	}
	if !cfg.keep {
		defer cleanupDB(cfg.dbPath)
	}

	db, err := openAndInit(cfg.dbPath)
	if err != nil {
		return err
	}

	wsID, err := createWorkspace(db)
	if err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	fmt.Printf("Workspace id: %d\n", wsID)

	cfRepo := repository.NewCustomFieldRepository(db)
	fieldID, err := createCustomField(cfRepo, cfg.fieldType)
	if err != nil {
		return fmt.Errorf("create custom field: %w", err)
	}
	fmt.Printf("Custom field id: %d (%s)\n\n", fieldID, cfg.fieldType)

	indexName := fmt.Sprintf("idx_cf_items_%d", fieldID)
	queryFilter := buildQueryFilter(int(fieldID), cfg.fieldType)

	// Phase A — scaling sweep, no index, during insert
	fmt.Printf("--- Phase A: unindexed scaling sweep ---\n")
	insertStart := time.Now()
	unindexed, err := bulkInsertWithCheckpoints(db, wsID, int(fieldID), cfg, queryFilter, indexName)
	if err != nil {
		return fmt.Errorf("bulk insert: %w", err)
	}
	insertWall := time.Since(insertStart)
	fmt.Printf("Inserted %d items in %s (%.0f rows/sec)\n\n", cfg.rows, insertWall.Round(time.Second), float64(cfg.rows)/insertWall.Seconds())
	for _, m := range unindexed {
		if m.usesIndex {
			return fmt.Errorf("phase A: planner used index %s at %d rows even though it was never recorded", indexName, m.rows)
		}
	}

	// Phase B — enable + restart + descending sweep
	fmt.Printf("--- Phase B: enable, restart, indexed scaling sweep ---\n")
	if err := cfRepo.RecordIndex(int(fieldID), "items", indexName); err != nil {
		return fmt.Errorf("phase B record index: %w", err)
	}
	if err := db.Close(); err != nil {
		return fmt.Errorf("phase B close: %w", err)
	}
	restartStart := time.Now()
	db, err = openAndInit(cfg.dbPath)
	if err != nil {
		return fmt.Errorf("phase B reopen: %w", err)
	}
	restartWall := time.Since(restartStart)
	cfRepo = repository.NewCustomFieldRepository(db)
	exists, err := indexExistsInSqliteMaster(db, indexName)
	if err != nil {
		return fmt.Errorf("phase B check sqlite_master: %w", err)
	}
	if !exists {
		return fmt.Errorf("phase B: deferred CREATE INDEX did not run — index %s not in sqlite_master", indexName)
	}
	fmt.Printf("Deferred CREATE INDEX wall: %s (at %d rows)\n", restartWall.Round(time.Millisecond), cfg.rows)

	indexed, err := descendingIndexedSweep(db, cfg, queryFilter, indexName)
	if err != nil {
		return fmt.Errorf("phase B sweep: %w", err)
	}
	smallestCP := cfg.checkpoints[0]
	for _, m := range indexed {
		if !m.usesIndex {
			fmt.Printf("  NOTE: at %d rows planner chose SCAN over %s (%s)\n", m.rows, indexName, m.planText)
		}
	}

	// Phase C — disable cleanup, at the smallest checkpoint
	fmt.Printf("\n--- Phase C: disable cleanup (at %d rows) ---\n", smallestCP)
	if err := cfRepo.ExecDDL(fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName)); err != nil {
		return fmt.Errorf("phase C drop: %w", err)
	}
	if err := cfRepo.DeleteIndexRecord(int(fieldID), "items"); err != nil {
		return fmt.Errorf("phase C delete record: %w", err)
	}
	exists, err = indexExistsInSqliteMaster(db, indexName)
	if err != nil {
		return fmt.Errorf("phase C check sqlite_master: %w", err)
	}
	if exists {
		return fmt.Errorf("phase C: index %s still present in sqlite_master after DROP", indexName)
	}
	recorded, err := cfRepo.IsIndexRecorded(int(fieldID), "items")
	if err != nil {
		return fmt.Errorf("phase C check tracking row: %w", err)
	}
	if recorded {
		return fmt.Errorf("phase C: tracking row still present after DeleteIndexRecord")
	}
	resC, err := timedQuery(db, queryFilter, indexName)
	if err != nil {
		return fmt.Errorf("phase C query: %w", err)
	}
	if resC.usesIndex {
		return fmt.Errorf("phase C: planner still used dropped index; plan=%s", resC.planText)
	}
	fmt.Printf("Phase C: query=%.2fms plan=%s\n", resC.queryMs, resC.planText)

	// Phase D — toggle on-then-off without restart, still at smallest checkpoint
	fmt.Printf("\n--- Phase D: toggle without restart (at %d rows) ---\n", smallestCP)
	if err := cfRepo.RecordIndex(int(fieldID), "items", indexName); err != nil {
		return fmt.Errorf("phase D record: %w", err)
	}
	if err := cfRepo.ExecDDL(fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName)); err != nil {
		return fmt.Errorf("phase D drop: %w", err)
	}
	if err := cfRepo.DeleteIndexRecord(int(fieldID), "items"); err != nil {
		return fmt.Errorf("phase D delete: %w", err)
	}
	exists, err = indexExistsInSqliteMaster(db, indexName)
	if err != nil {
		return fmt.Errorf("phase D check sqlite_master: %w", err)
	}
	if exists {
		return fmt.Errorf("phase D: index %s appeared in sqlite_master even though no restart occurred", indexName)
	}
	recorded, err = cfRepo.IsIndexRecorded(int(fieldID), "items")
	if err != nil {
		return fmt.Errorf("phase D check tracking row: %w", err)
	}
	if recorded {
		return fmt.Errorf("phase D: tracking row leaked")
	}
	fmt.Printf("Phase D: clean — no index, no tracking row\n")

	if err := db.Close(); err != nil {
		return fmt.Errorf("final close: %w", err)
	}

	printScalingTable(unindexed, indexed)
	printRunInfo(cfg, insertWall, restartWall, resC)
	return nil
}

func openAndInit(path string) (database.Database, error) {
	db, err := database.NewSQLiteDB(path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Initialize(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize db: %w", err)
	}
	return db, nil
}

func cleanupDB(path string) {
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		_ = os.Remove(p)
	}
}

func createWorkspace(db database.Database) (int, error) {
	now := time.Now()
	var id int
	err := db.QueryRow(
		`INSERT INTO workspaces (name, key, active, created_at, updated_at)
		 VALUES (?, ?, 1, ?, ?) RETURNING id`,
		"CF Stress", "CFS", now, now,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func createCustomField(cfRepo *repository.CustomFieldRepository, fieldType string) (int64, error) {
	return cfRepo.Create(&models.CustomFieldDefinition{
		Name:      "stress_cf_" + fieldType,
		FieldType: fieldType,
	}, time.Now())
}

// bulkInsertWithCheckpoints inserts cfg.rows items via repo.Create in batched
// transactions and, each time the running total crosses a checkpoint, pauses
// to run the unindexed query and record a measurement.
func bulkInsertWithCheckpoints(db database.Database, wsID, fieldID int, cfg config, queryFilter, indexName string) ([]measurement, error) {
	repo := repository.NewItemRepository(db)
	fieldKey := strconv.Itoa(fieldID)
	progressInterval := cfg.rows / 20
	if progressInterval < 1000 {
		progressInterval = 1000
	}
	progressStart := time.Now()

	cps := cfg.checkpoints
	cpIdx := 0
	var measurements []measurement

	for batchStart := 0; batchStart < cfg.rows; batchStart += cfg.batchSize {
		batchEnd := batchStart + cfg.batchSize
		if batchEnd > cfg.rows {
			batchEnd = cfg.rows
		}
		tx, err := db.Begin()
		if err != nil {
			return nil, fmt.Errorf("begin batch tx: %w", err)
		}
		for i := batchStart; i < batchEnd; i++ {
			item := &models.Item{
				WorkspaceID:         wsID,
				WorkspaceItemNumber: i + 1,
				Title:               fmt.Sprintf("stress-%d", i+1),
				CustomFieldValues: map[string]interface{}{
					fieldKey: cfValueFor(i, cfg.fieldType),
				},
			}
			if _, err := repo.Create(tx, item); err != nil {
				_ = tx.Rollback()
				return nil, fmt.Errorf("create item %d: %w", i, err)
			}
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit batch ending at %d: %w", batchEnd, err)
		}

		// Progress line
		if batchEnd%progressInterval < cfg.batchSize {
			elapsed := time.Since(progressStart)
			rate := float64(batchEnd) / elapsed.Seconds()
			eta := time.Duration(float64(cfg.rows-batchEnd) / rate * float64(time.Second))
			fmt.Printf("  inserted %d/%d (%.0f rows/sec, eta %s)\n", batchEnd, cfg.rows, rate, eta.Round(time.Second))
		}

		// Drain any checkpoints whose threshold this batch crossed.
		for cpIdx < len(cps) && batchEnd >= cps[cpIdx] {
			m, err := timedQuery(db, queryFilter, indexName)
			if err != nil {
				return nil, fmt.Errorf("checkpoint query at %d: %w", cps[cpIdx], err)
			}
			m.rows = cps[cpIdx]
			fmt.Printf("  [unindexed @ %d rows] %.2fms %s\n", m.rows, m.queryMs, m.planText)
			measurements = append(measurements, m)
			cpIdx++
		}
	}

	if cpIdx != len(cps) {
		return nil, fmt.Errorf("internal: %d checkpoints unrun (last batch ended below them)", len(cps)-cpIdx)
	}
	return measurements, nil
}

// descendingIndexedSweep walks cfg.checkpoints from largest to smallest. At
// each step it deletes any rows above the target threshold (so the next
// measurement runs against exactly cp rows) and then runs the indexed query.
// Output is returned ascending so the scaling table reads naturally.
func descendingIndexedSweep(db database.Database, cfg config, queryFilter, indexName string) ([]measurement, error) {
	cps := cfg.checkpoints
	measurements := make([]measurement, 0, len(cps))
	for i := len(cps) - 1; i >= 0; i-- {
		cp := cps[i]
		if cp < cfg.rows || i < len(cps)-1 {
			// Trim down to cp rows by deleting any id > cp. SQLite uses the
			// PK index, so this is a range scan — proportional to the number
			// of rows removed, not to the total table size.
			delStart := time.Now()
			res, err := db.ExecWrite(`DELETE FROM items WHERE id > ?`, cp)
			if err != nil {
				return nil, fmt.Errorf("delete to %d: %w", cp, err)
			}
			affected, _ := res.RowsAffected()
			if affected > 0 {
				fmt.Printf("  trimmed %d rows to reach %d total in %s\n", affected, cp, time.Since(delStart).Round(time.Millisecond))
			}
		}
		m, err := timedQuery(db, queryFilter, indexName)
		if err != nil {
			return nil, fmt.Errorf("indexed query at %d: %w", cp, err)
		}
		m.rows = cp
		fmt.Printf("  [indexed   @ %d rows] %.2fms %s\n", m.rows, m.queryMs, m.planText)
		measurements = append(measurements, m)
	}
	// Reverse to ascending order.
	for i, j := 0, len(measurements)-1; i < j; i, j = i+1, j-1 {
		measurements[i], measurements[j] = measurements[j], measurements[i]
	}
	return measurements, nil
}

func cfValueFor(i int, fieldType string) interface{} {
	bucket := i % 1000
	switch fieldType {
	case "number":
		return strconv.Itoa(bucket)
	case "text":
		return "v" + strconv.Itoa(bucket)
	case "date":
		return time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, bucket).Format("2006-01-02")
	}
	return strconv.Itoa(bucket)
}

// buildQueryFilter returns the WHERE expression used in both the benchmark
// query and its EXPLAIN QUERY PLAN. Mirrors the JSON-extract style the app
// uses on SQLite (item_repository.go:349).
func buildQueryFilter(fieldID int, fieldType string) string {
	expr := fmt.Sprintf(`NULLIF(custom_field_values,'') ->> '$."%d"'`, fieldID)
	switch fieldType {
	case "number":
		return fmt.Sprintf(`CAST(%s AS NUMERIC) = 42`, expr)
	default:
		return fmt.Sprintf(`%s = 'v42'`, expr)
	}
}

// timedQuery runs the SELECT three times, returns the median runtime, the
// EXPLAIN QUERY PLAN, and whether the plan references the given index name.
func timedQuery(db database.Database, filter, indexName string) (measurement, error) {
	planText, err := explainQueryPlan(db, filter)
	if err != nil {
		return measurement{}, err
	}
	usesIndex := strings.Contains(planText, indexName)

	const runs = 3
	durs := make([]time.Duration, 0, runs)
	for r := 0; r < runs; r++ {
		start := time.Now()
		var count int
		err := db.QueryRow(`SELECT COUNT(*) FROM items WHERE ` + filter).Scan(&count)
		if err != nil {
			return measurement{}, err
		}
		durs = append(durs, time.Since(start))
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	median := durs[runs/2]
	return measurement{
		queryMs:   float64(median.Microseconds()) / 1000.0,
		planText:  planText,
		usesIndex: usesIndex,
	}, nil
}

func explainQueryPlan(db database.Database, filter string) (string, error) {
	rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT COUNT(*) FROM items WHERE ` + filter)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	var parts []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			return "", err
		}
		parts = append(parts, detail)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return strings.Join(parts, "; "), nil
}

func indexExistsInSqliteMaster(db database.Database, name string) (bool, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name = ?`, name).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func printScalingTable(unindexed, indexed []measurement) {
	fmt.Println("\n=== Scaling sweep ===")
	fmt.Printf("%-10s  %-16s  %-16s  %-10s  %s\n", "Rows", "Unindexed (ms)", "Indexed (ms)", "Speedup", "Indexed plan")
	idx := make(map[int]measurement, len(indexed))
	for _, m := range indexed {
		idx[m.rows] = m
	}
	for _, u := range unindexed {
		ix, ok := idx[u.rows]
		if !ok {
			fmt.Printf("%-10d  %-16.2f  %-16s  %-10s  %s\n", u.rows, u.queryMs, "-", "-", "")
			continue
		}
		speedup := "-"
		if ix.queryMs > 0 {
			speedup = fmt.Sprintf("%.0fx", u.queryMs/ix.queryMs)
		}
		fmt.Printf("%-10d  %-16.2f  %-16.2f  %-10s  %s\n", u.rows, u.queryMs, ix.queryMs, speedup, ix.planText)
	}
}

func printRunInfo(cfg config, insertWall, restartWall time.Duration, disabled measurement) {
	fmt.Println("\n=== Phase summary ===")
	fmt.Printf("Inserted %d rows in %s (%.0f rows/sec)\n", cfg.rows, insertWall.Round(time.Second), float64(cfg.rows)/insertWall.Seconds())
	fmt.Printf("Deferred CREATE INDEX wall (at %d rows): %s\n", cfg.rows, restartWall.Round(time.Millisecond))
	fmt.Printf("Phase C disabled query @ %d rows: %.2fms plan=%s\n", cfg.checkpoints[0], disabled.queryMs, disabled.planText)
	fmt.Println("Phase D toggle round-trip without restart: clean")
	fmt.Println("OK")
}
