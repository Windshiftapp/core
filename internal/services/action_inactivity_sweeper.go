package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// ActionInactivitySweeperConfig configures the background ticker that drives
// item_inactive automation triggers (WI-1132).
type ActionInactivitySweeperConfig struct {
	// TickInterval is how often to scan for stale open items. Default 5m —
	// thresholds are expressed in hours, so sub-minute sweeps buy nothing.
	TickInterval time.Duration
	// BatchSize bounds the items one action may fire per tick. Default 200.
	BatchSize int
}

// DefaultActionInactivitySweeperConfig returns sensible defaults.
func DefaultActionInactivitySweeperConfig() ActionInactivitySweeperConfig {
	return ActionInactivitySweeperConfig{
		TickInterval: 5 * time.Minute,
		BatchSize:    200,
	}
}

// ActionInactivitySweeper periodically evaluates every enabled item_inactive
// action and emits a targeted ActionEvent per stale open item. A mark row
// records the activity timestamp each firing covered, so an item re-fires
// only after new activity moves its last-activity timestamp forward.
type ActionInactivitySweeper struct {
	db       database.Database
	actions  *repository.ActionRepository
	marks    *repository.ActionTriggerMarkRepository
	emitter  ActionEventEmitter
	config   ActionInactivitySweeperConfig
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewActionInactivitySweeper constructs the sweeper. Call Start() to begin.
// The emitter is the action service; events flow through the normal durable
// admission and matching path.
func NewActionInactivitySweeper(db database.Database, emitter ActionEventEmitter, config ActionInactivitySweeperConfig) *ActionInactivitySweeper {
	if config.TickInterval == 0 {
		config.TickInterval = 5 * time.Minute
	}
	if config.BatchSize == 0 {
		config.BatchSize = 200
	}
	return &ActionInactivitySweeper{
		db:       db,
		actions:  repository.NewActionRepository(db),
		marks:    repository.NewActionTriggerMarkRepository(db),
		emitter:  emitter,
		config:   config,
		stopChan: make(chan struct{}),
	}
}

// Start launches the background worker. Idempotent.
func (s *ActionInactivitySweeper) Start() {
	s.wg.Add(1)
	go s.run()
	slog.Debug("action inactivity sweeper started",
		slog.String("component", "actions"),
		slog.Duration("tick_interval", s.config.TickInterval),
	)
}

// Stop signals shutdown and waits for the worker to drain.
func (s *ActionInactivitySweeper) Stop() {
	close(s.stopChan)
	s.wg.Wait()
}

func (s *ActionInactivitySweeper) run() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.config.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.sweep(time.Now())
		}
	}
}

// SweepOnce evaluates all item_inactive actions once; exposed for tests.
func (s *ActionInactivitySweeper) SweepOnce(now time.Time) int {
	return s.sweep(now)
}

// sweep evaluates every enabled item_inactive action in the database and
// returns how many events were emitted.
func (s *ActionInactivitySweeper) sweep(now time.Time) int {
	actions, err := s.actions.ListEnabledByTrigger(models.ActionTriggerItemInactive)
	if err != nil {
		slog.Warn("inactivity sweep could not list actions",
			slog.String("component", "actions"), slog.Any("error", err))
		return 0
	}
	emitted := 0
	for _, action := range actions {
		emitted += s.sweepAction(action, now)
	}
	return emitted
}

// sweepAction evaluates one item_inactive action. Items qualify when they
// are open, not merged away, past the configured inactivity threshold, and
// either unmarked or active again since the last firing.
func (s *ActionInactivitySweeper) sweepAction(action *models.Action, now time.Time) int {
	var config models.ActionTriggerConfig
	if action.TriggerConfig != "" {
		if err := json.Unmarshal([]byte(action.TriggerConfig), &config); err != nil {
			slog.Warn("inactivity sweep skipping action with unparsable config",
				slog.String("component", "actions"), slog.Int("action_id", action.ID), slog.Any("error", err))
			return 0
		}
	}
	if config.InactiveHours <= 0 {
		return 0
	}
	// Staleness compares normalized UTC strings so mixed storage formats
	// (driver-bound timestamps vs CURRENT_TIMESTAMP defaults) stay ordered.
	cutoff := normalizeSweepTimestamp(now.Add(-time.Duration(config.InactiveHours) * time.Hour))

	items, err := s.staleOpenItems(action.ID, action.WorkspaceID, config.ItemTypeID, cutoff, s.config.BatchSize)
	if err != nil {
		slog.Warn("inactivity sweep query failed",
			slog.String("component", "actions"), slog.Int("action_id", action.ID), slog.Any("error", err))
		return 0
	}

	emitted := 0
	for _, item := range items {
		lastActivity, err := time.Parse(time.RFC3339, item.LastActivity)
		if err != nil {
			slog.Warn("inactivity sweep unparsable item activity",
				slog.String("component", "actions"), slog.Int("item_id", item.ID), slog.String("value", item.LastActivity))
			continue
		}
		if wrote, err := s.marks.Upsert(action.ID, item.ID, lastActivity); err != nil {
			slog.Warn("inactivity sweep could not record mark",
				slog.String("component", "actions"), slog.Int("action_id", action.ID),
				slog.Int("item_id", item.ID), slog.Any("error", err))
			continue
		} else if !wrote {
			// A concurrent sweep already recorded this activity; only the writer emits.
			continue
		}
		s.emitter.EmitActionEvent(&models.ActionEvent{
			EventType:   models.ActionTriggerItemInactive,
			WorkspaceID: action.WorkspaceID,
			ItemID:      item.ID,
			ItemTypeID:  item.ItemTypeID,
			NewValues:   map[string]any{"action_id": action.ID, "inactive_hours": config.InactiveHours},
		})
		emitted++
	}
	return emitted
}

// normalizeSweepTimestamp renders a cutoff in the fixed-width normalized form
// the sweep query compares against.
func normalizeSweepTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// staleOpenItem is one candidate row. LastActivity is the normalized UTC
// timestamp of the item's most recent field update or comment.
type staleOpenItem struct {
	ID           int
	ItemTypeID   *int
	LastActivity string
}

// staleOpenItems returns open, unmerged items in the workspace whose last
// activity (the later of items.updated_at and the newest comment) precedes
// the normalized cutoff string. Items already marked as fired for actionID are
// excluded before the limit, so a run of marked rows cannot starve later
// eligible items out of the batch.
func (s *ActionInactivitySweeper) staleOpenItems(actionID, workspaceID int, itemTypeID *int, cutoff string, limit int) ([]staleOpenItem, error) {
	// Normalize each timestamp before taking the later one. SQLite can store the
	// edit and comment timestamps in different text shapes (a driver-bound value
	// with a "T" versus a CURRENT_TIMESTAMP default with a space), and comparing
	// the raw text would pick the wrong instant. The normalized UTC strings are
	// fixed-width, so both MAX/GREATEST and the later string comparisons below
	// are chronological.
	var lastActivityExpr string
	var markExpr string
	if database.IsPostgresDriver(s.db.GetDriverName()) {
		editActivity := `to_char(i.updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')`
		commentActivity := `(SELECT MAX(to_char(c.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')) FROM comments c WHERE c.item_id = i.id)`
		lastActivityExpr = fmt.Sprintf(`GREATEST(%s, COALESCE(%s, %s))`, editActivity, commentActivity, editActivity)
		markExpr = `to_char(m.last_activity_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')`
	} else {
		editActivity := `strftime('%Y-%m-%dT%H:%M:%fZ', i.updated_at)`
		commentActivity := `(SELECT MAX(strftime('%Y-%m-%dT%H:%M:%fZ', c.created_at)) FROM comments c WHERE c.item_id = i.id)`
		lastActivityExpr = fmt.Sprintf(`MAX(%s, COALESCE(%s, %s))`, editActivity, commentActivity, editActivity)
		markExpr = `strftime('%Y-%m-%dT%H:%M:%fZ', m.last_activity_at)`
	}

	query := fmt.Sprintf(`
		SELECT i.id, i.item_type_id, %s AS last_activity
		FROM items i
		JOIN statuses st ON i.status_id = st.id
		JOIN status_categories sc ON st.category_id = sc.id
		LEFT JOIN action_trigger_marks m ON m.action_id = ? AND m.item_id = i.id
		WHERE i.workspace_id = ?
		  AND sc.is_completed = false
		  AND i.merged_into_item_id IS NULL
		  AND %s <= ?
		  AND (m.last_activity_at IS NULL OR %s > %s)
	`, lastActivityExpr, lastActivityExpr, lastActivityExpr, markExpr)
	args := []any{actionID, workspaceID, cutoff}
	if itemTypeID != nil {
		query += " AND i.item_type_id = ?"
		args = append(args, *itemTypeID)
	}
	query += " ORDER BY i.id ASC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query stale open items (workspace %d): %w", workspaceID, err)
	}
	defer func() { _ = rows.Close() }()

	items := []staleOpenItem{}
	for rows.Next() {
		var item staleOpenItem
		if err := rows.Scan(&item.ID, &item.ItemTypeID, &item.LastActivity); err != nil {
			return nil, fmt.Errorf("scan stale open item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
