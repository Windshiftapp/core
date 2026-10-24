package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// SLARepository owns SQL for the SLA tables: working_calendars,
// team_workspace_bindings, sla_workspace_state, sla_metrics,
// sla_metric_conditions, sla_goals, sla_goal_targets, item_sla_cycles,
// item_sla_cycle_thresholds, and sla_jobs.
type SLARepository struct {
	db database.Database
}

// ErrSLAInUse marks a delete that would strand an SLA reference.
var ErrSLAInUse = errors.New("SLA resource is still referenced")

// NewSLARepository constructs an SLARepository.
func NewSLARepository(db database.Database) *SLARepository {
	return &SLARepository{db: db}
}

// ---------------------------------------------------------------------------
// Working calendars
// ---------------------------------------------------------------------------

const calendarColumns = `id, workspace_id, team_id, name, description, timezone,
	weekly_intervals, holidays, is_default, source, source_id, source_payload,
	created_at, updated_at`

func scanCalendar(scanner interface{ Scan(...any) error }) (models.WorkingCalendar, error) {
	var cal models.WorkingCalendar
	var workspaceID, teamID sql.NullInt64
	var description sql.NullString
	var source, sourceID, sourcePayload sql.NullString
	var weekly, holidays []byte
	err := scanner.Scan(
		&cal.ID, &workspaceID, &teamID, &cal.Name, &description, &cal.Timezone,
		&weekly, &holidays, &cal.IsDefault, &source, &sourceID, &sourcePayload,
		&cal.CreatedAt, &cal.UpdatedAt,
	)
	if err != nil {
		return models.WorkingCalendar{}, err
	}
	if workspaceID.Valid {
		value := int(workspaceID.Int64)
		cal.WorkspaceID = &value
	}
	if teamID.Valid {
		value := int(teamID.Int64)
		cal.TeamID = &value
	}
	cal.Description = description.String
	cal.WeeklyIntervals = append(json.RawMessage(nil), weekly...)
	if len(holidays) == 0 {
		holidays = []byte("[]")
	}
	cal.Holidays = append(json.RawMessage(nil), holidays...)
	if source.Valid {
		cal.Source = &source.String
	}
	if sourceID.Valid {
		cal.SourceID = &sourceID.String
	}
	if sourcePayload.Valid {
		cal.SourcePayload = &sourcePayload.String
	}
	return cal, nil
}

// ListWorkspaceCalendars returns a workspace's owned calendars.
func (r *SLARepository) ListWorkspaceCalendars(ctx context.Context, workspaceID int) ([]models.WorkingCalendar, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+calendarColumns+` FROM working_calendars WHERE workspace_id = ? ORDER BY is_default DESC, name`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list workspace calendars: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return collectCalendars(rows)
}

// ListTeamCalendars returns a team's shared calendars.
func (r *SLARepository) ListTeamCalendars(ctx context.Context, teamID int) ([]models.WorkingCalendar, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+calendarColumns+` FROM working_calendars WHERE team_id = ? ORDER BY name`, teamID)
	if err != nil {
		return nil, fmt.Errorf("list team calendars: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return collectCalendars(rows)
}

// ListBindableTeamCalendars returns team calendars authorized for a workspace
// through an active team_workspace_bindings row.
func (r *SLARepository) ListBindableTeamCalendars(ctx context.Context, workspaceID int) ([]models.WorkingCalendar, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+calendarColumns+` FROM working_calendars
		WHERE team_id IN (SELECT team_id FROM team_workspace_bindings WHERE workspace_id = ?)
		ORDER BY name`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list bound team calendars: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return collectCalendars(rows)
}

func collectCalendars(rows *sql.Rows) ([]models.WorkingCalendar, error) {
	var calendars []models.WorkingCalendar
	for rows.Next() {
		calendar, err := scanCalendar(rows)
		if err != nil {
			return nil, fmt.Errorf("scan calendar: %w", err)
		}
		calendars = append(calendars, calendar)
	}
	return calendars, rows.Err()
}

// MetricIDsReferencingCalendar returns the metrics whose goals target the
// calendar. It is the recalculation set for a calendar edit that applies to
// ongoing cycles.

func (r *SLARepository) MetricIDsReferencingCalendarTx(ctx context.Context, tx database.Tx, calendarID int) ([]int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT m.id
		FROM sla_metrics m
		JOIN sla_goals g ON g.metric_id = m.id
		JOIN sla_goal_targets t ON t.goal_id = g.id
		WHERE t.calendar_id = ?`, calendarID)
	if err != nil {
		return nil, fmt.Errorf("list metrics referencing calendar: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan metric referencing calendar: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetCalendar loads one calendar, returning ErrNotFound when absent.
func (r *SLARepository) GetCalendar(ctx context.Context, id int) (*models.WorkingCalendar, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+calendarColumns+` FROM working_calendars WHERE id = ?`, id)
	calendar, err := scanCalendar(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load calendar: %w", err)
	}
	return &calendar, nil
}

// GetCalendarBySourceID resolves a workspace-owned calendar by its import
// source identity.
func (r *SLARepository) GetCalendarBySourceID(ctx context.Context, workspaceID int, sourceID string) (*models.WorkingCalendar, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+calendarColumns+` FROM working_calendars WHERE workspace_id = ? AND source_id = ?`, workspaceID, sourceID)
	calendar, err := scanCalendar(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load calendar by source: %w", err)
	}
	return &calendar, nil
}

// GetMetricBySourceID resolves a metric by its import source identity.
func (r *SLARepository) GetMetricBySourceID(ctx context.Context, workspaceID int, sourceID string) (*models.SLAMetric, error) {
	var id int
	err := r.db.QueryRowContext(ctx, `SELECT id FROM sla_metrics WHERE workspace_id = ? AND source_id = ?`, workspaceID, sourceID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load metric by source: %w", err)
	}
	return r.GetMetric(ctx, id)
}

// GetCycleBySourceID resolves an imported cycle by its Jira identity.
func (r *SLARepository) GetCycleBySourceID(ctx context.Context, metricID int, sourceID string) (*models.ItemSLACycle, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+cycleColumns+` FROM item_sla_cycles WHERE metric_id = ? AND source_id = ?`, metricID, sourceID)
	cycle, err := scanCycle(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load cycle by source: %w", err)
	}
	return &cycle, nil
}

// GetCycleBySourceIDTx resolves an imported cycle inside the caller's
// transaction.
func (r *SLARepository) GetCycleBySourceIDTx(ctx context.Context, tx database.Tx, metricID int, sourceID string) (*models.ItemSLACycle, error) {
	row := tx.QueryRowContext(ctx, `SELECT `+cycleColumns+` FROM item_sla_cycles WHERE metric_id = ? AND source_id = ?`, metricID, sourceID)
	cycle, err := scanCycle(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load cycle by source: %w", err)
	}
	return &cycle, nil
}

// GoalIDsBySourceID maps a metric's goals by source identity inside the
// caller's transaction.
func (r *SLARepository) GoalIDsBySourceID(ctx context.Context, tx database.Tx, metricID int) (map[string]int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, source_id FROM sla_goals WHERE metric_id = ? AND source_id IS NOT NULL`, metricID)
	if err != nil {
		return nil, fmt.Errorf("load goal source ids: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := map[string]int{}
	for rows.Next() {
		var id int
		var sourceID string
		if err := rows.Scan(&id, &sourceID); err != nil {
			return nil, fmt.Errorf("scan goal source id: %w", err)
		}
		ids[sourceID] = id
	}
	return ids, rows.Err()
}

// ItemIDByWorkspaceNumber resolves an item by its workspace-relative number,
// which is how an imported Jira issue key maps to a Windshift item.
func (r *SLARepository) ItemIDByWorkspaceNumber(ctx context.Context, workspaceID, number int) (int, error) {
	var id int
	err := r.db.QueryRowContext(ctx, `SELECT id FROM items WHERE workspace_id = ? AND workspace_item_number = ?`, workspaceID, number).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("resolve item number: %w", err)
	}
	return id, nil
}

// ItemBelongsToWorkspace reports whether an item is owned by the workspace.
func (r *SLARepository) ItemBelongsToWorkspace(ctx context.Context, workspaceID, itemID int) (bool, error) {
	var belongs bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM items WHERE id = ? AND workspace_id = ?)`, itemID, workspaceID).Scan(&belongs)
	if err != nil {
		return false, fmt.Errorf("check item workspace: %w", err)
	}
	return belongs, nil
}

// PriorityIDByName resolves a priority by its display name.
func (r *SLARepository) PriorityIDByName(ctx context.Context, name string) (int, error) {
	var id int
	err := r.db.QueryRowContext(ctx, `SELECT id FROM priorities WHERE name = ? ORDER BY id LIMIT 1`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("resolve priority: %w", err)
	}
	return id, nil
}

// ListImportedMetricSourceIDs returns the source ids of metrics imported from a
// system, used to reconcile replace-mode imports.
func (r *SLARepository) ListImportedMetricSourceIDs(ctx context.Context, workspaceID int, source string) ([]string, error) {
	return r.listImportedSourceIDs(ctx, `SELECT source_id FROM sla_metrics WHERE workspace_id = ? AND source = ? AND source_id IS NOT NULL`, workspaceID, source)
}

// ListImportedCalendarSourceIDs returns workspace calendar source ids for a
// source system.
func (r *SLARepository) ListImportedCalendarSourceIDs(ctx context.Context, workspaceID int, source string) ([]string, error) {
	return r.listImportedSourceIDs(ctx, `SELECT source_id FROM working_calendars WHERE workspace_id = ? AND source = ? AND source_id IS NOT NULL`, workspaceID, source)
}

// ListImportedCycleSourceIDs returns cycle source ids for one imported metric.
func (r *SLARepository) ListImportedCycleSourceIDs(ctx context.Context, metricID int) ([]string, error) {
	return r.listImportedSourceIDs(ctx, `SELECT source_id FROM item_sla_cycles WHERE metric_id = ? AND origin = 'import' AND source_id IS NOT NULL`, metricID)
}

func (r *SLARepository) listImportedSourceIDs(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list imported source ids: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan imported source id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteImportedCyclesNotIn removes imported cycles for a metric whose source
// id is not in keep. Native cycles are never touched. Stale rows are selected
// and deleted by primary key in bounded chunks so a large import cannot build
// an oversized NOT IN list.
func (r *SLARepository) DeleteImportedCyclesNotIn(ctx context.Context, tx database.Tx, metricID int, keep []string) error {
	keepSet := make(map[string]struct{}, len(keep))
	for _, id := range keep {
		keepSet[id] = struct{}{}
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, source_id FROM item_sla_cycles WHERE metric_id = ? AND origin = 'import'`, metricID)
	if err != nil {
		return fmt.Errorf("list imported cycles: %w", err)
	}
	var stale []int64
	for rows.Next() {
		var id int64
		var sourceID sql.NullString
		if err := rows.Scan(&id, &sourceID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan imported cycle: %w", err)
		}
		if sourceID.Valid {
			if _, ok := keepSet[sourceID.String]; ok {
				continue
			}
		}
		stale = append(stale, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	const chunk = 500
	for start := 0; start < len(stale); start += chunk {
		end := start + chunk
		if end > len(stale) {
			end = len(stale)
		}
		batch := stale[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		args := make([]any, len(batch))
		for i, id := range batch {
			args[i] = id
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM item_sla_cycles WHERE id IN (`+placeholders+`)`, args...); err != nil {
			return fmt.Errorf("delete stale imported cycles: %w", err)
		}
	}
	return nil
}

// CreateCalendar inserts a calendar and returns its ID.
func (r *SLARepository) CreateCalendar(ctx context.Context, calendar *models.WorkingCalendar) (int, error) {
	holidays := calendar.Holidays
	if len(holidays) == 0 {
		holidays = json.RawMessage("[]")
	}
	var id int
	err := r.db.QueryRowContext(ctx, `INSERT INTO working_calendars
		(workspace_id, team_id, name, description, timezone, weekly_intervals, holidays, is_default, source, source_id, source_payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		nullableInt(calendar.WorkspaceID), nullableInt(calendar.TeamID), calendar.Name, calendar.Description,
		calendar.Timezone, string(calendar.WeeklyIntervals), string(holidays), calendar.IsDefault,
		calendar.Source, calendar.SourceID, calendar.SourcePayload,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create calendar: %w", err)
	}
	return id, nil
}

// UpdateCalendar writes the mutable calendar fields.
func (r *SLARepository) UpdateCalendar(ctx context.Context, calendar *models.WorkingCalendar) error {
	holidays := calendar.Holidays
	if len(holidays) == 0 {
		holidays = json.RawMessage("[]")
	}
	result, err := r.db.ExecContext(ctx, `UPDATE working_calendars
		SET name = ?, description = ?, timezone = ?, weekly_intervals = ?, holidays = ?, is_default = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		calendar.Name, calendar.Description, calendar.Timezone, string(calendar.WeeklyIntervals), string(holidays), calendar.IsDefault, calendar.ID)
	if err != nil {
		return fmt.Errorf("update calendar: %w", err)
	}
	return requireAffected(result)
}

// CreateCalendarTx inserts a calendar inside the caller's transaction.
func (r *SLARepository) CreateCalendarTx(ctx context.Context, tx database.Tx, calendar *models.WorkingCalendar) (int, error) {
	holidays := calendar.Holidays
	if len(holidays) == 0 {
		holidays = json.RawMessage("[]")
	}
	var id int
	err := tx.QueryRowContext(ctx, `INSERT INTO working_calendars
		(workspace_id, team_id, name, description, timezone, weekly_intervals, holidays, is_default, source, source_id, source_payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		nullableInt(calendar.WorkspaceID), nullableInt(calendar.TeamID), calendar.Name, calendar.Description,
		calendar.Timezone, string(calendar.WeeklyIntervals), string(holidays), calendar.IsDefault,
		calendar.Source, calendar.SourceID, calendar.SourcePayload,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create calendar: %w", err)
	}
	return id, nil
}

// UpdateCalendarTx writes the mutable calendar fields inside a transaction.
func (r *SLARepository) UpdateCalendarTx(ctx context.Context, tx database.Tx, calendar *models.WorkingCalendar) error {
	holidays := calendar.Holidays
	if len(holidays) == 0 {
		holidays = json.RawMessage("[]")
	}
	result, err := tx.ExecContext(ctx, `UPDATE working_calendars
		SET name = ?, description = ?, timezone = ?, weekly_intervals = ?, holidays = ?, is_default = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		calendar.Name, calendar.Description, calendar.Timezone, string(calendar.WeeklyIntervals), string(holidays), calendar.IsDefault, calendar.ID)
	if err != nil {
		return fmt.Errorf("update calendar: %w", err)
	}
	return requireAffected(result)
}

// DeleteCalendar removes a calendar unless a goal target references it.
func (r *SLARepository) DeleteCalendar(ctx context.Context, id int) error {
	var targets int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sla_goal_targets WHERE calendar_id = ?`, id).Scan(&targets); err != nil {
		return fmt.Errorf("count calendar targets: %w", err)
	}
	if targets > 0 {
		return fmt.Errorf("%w: calendar is referenced by %d SLA goal target(s)", ErrSLAInUse, targets)
	}
	result, err := r.db.ExecContext(ctx, `DELETE FROM working_calendars WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete calendar: %w", err)
	}
	return requireAffected(result)
}

// ---------------------------------------------------------------------------
// Team-workspace bindings
// ---------------------------------------------------------------------------

// CreateTeamWorkspaceBinding inserts a team-to-workspace consent row. It
// returns ErrDuplicateEntry when the two sides are already bound.
func (r *SLARepository) CreateTeamWorkspaceBinding(ctx context.Context, binding *models.TeamWorkspaceBinding) (int, error) {
	var id int
	err := r.db.QueryRowContext(ctx, `INSERT INTO team_workspace_bindings (team_id, workspace_id, created_by)
		VALUES (?, ?, ?) RETURNING id`, binding.TeamID, binding.WorkspaceID, nullableInt(binding.CreatedBy)).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, ErrDuplicateEntry
		}
		return 0, fmt.Errorf("create team workspace binding: %w", err)
	}
	return id, nil
}

const teamWorkspaceBindingColumns = `b.id, b.team_id, b.workspace_id, b.created_by, b.created_at, b.updated_at, t.name, w.name`

const teamWorkspaceBindingFrom = `FROM team_workspace_bindings b
		JOIN teams t ON t.id = b.team_id
		JOIN workspaces w ON w.id = b.workspace_id`

// GetTeamWorkspaceBinding returns one binding with team and workspace names.
func (r *SLARepository) GetTeamWorkspaceBinding(ctx context.Context, bindingID int) (*models.TeamWorkspaceBinding, error) {
	binding, err := scanTeamWorkspaceBinding(r.db.QueryRowContext(ctx,
		`SELECT `+teamWorkspaceBindingColumns+` `+teamWorkspaceBindingFrom+` WHERE b.id = ?`, bindingID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get team workspace binding: %w", err)
	}
	return binding, nil
}

// ListTeamWorkspaceBindings returns bindings whose workspace is workspaceID.
func (r *SLARepository) ListTeamWorkspaceBindings(ctx context.Context, workspaceID int) ([]models.TeamWorkspaceBinding, error) {
	return r.listTeamWorkspaceBindings(ctx, "b.workspace_id", workspaceID)
}

// ListTeamWorkspaceBindingsForTeam returns bindings whose team is teamID.
func (r *SLARepository) ListTeamWorkspaceBindingsForTeam(ctx context.Context, teamID int) ([]models.TeamWorkspaceBinding, error) {
	return r.listTeamWorkspaceBindings(ctx, "b.team_id", teamID)
}

func (r *SLARepository) listTeamWorkspaceBindings(ctx context.Context, column string, id int) ([]models.TeamWorkspaceBinding, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+teamWorkspaceBindingColumns+` `+teamWorkspaceBindingFrom+` WHERE `+column+` = ? ORDER BY b.id`, id)
	if err != nil {
		return nil, fmt.Errorf("list team workspace bindings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var bindings []models.TeamWorkspaceBinding
	for rows.Next() {
		binding, err := scanTeamWorkspaceBinding(rows)
		if err != nil {
			return nil, fmt.Errorf("scan binding: %w", err)
		}
		bindings = append(bindings, *binding)
	}
	return bindings, rows.Err()
}

func scanTeamWorkspaceBinding(row rowScanner) (*models.TeamWorkspaceBinding, error) {
	var binding models.TeamWorkspaceBinding
	var createdBy sql.NullInt64
	if err := row.Scan(&binding.ID, &binding.TeamID, &binding.WorkspaceID, &createdBy, &binding.CreatedAt, &binding.UpdatedAt, &binding.TeamName, &binding.WorkspaceName); err != nil {
		return nil, err
	}
	if createdBy.Valid {
		value := int(createdBy.Int64)
		binding.CreatedBy = &value
	}
	return &binding, nil
}

// DeleteTeamWorkspaceBinding removes a binding. It refuses when an SLA goal
// target still references one of the team's calendars, so a promise can never
// depend on an unauthorized calendar.
func (r *SLARepository) DeleteTeamWorkspaceBinding(ctx context.Context, workspaceID, bindingID int) error {
	return database.WithTx(r.db, func(tx database.Tx) error {
		query := `SELECT team_id FROM team_workspace_bindings WHERE id = ? AND workspace_id = ?`
		if database.IsPostgresDriver(r.db.GetDriverName()) {
			query += ` FOR UPDATE`
		}
		var teamID int
		if err := tx.QueryRowContext(ctx, query, bindingID, workspaceID).Scan(&teamID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("load binding: %w", err)
		}
		var targets int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sla_goal_targets t
			JOIN working_calendars c ON c.id = t.calendar_id
			JOIN sla_goals g ON g.id = t.goal_id
			JOIN sla_metrics m ON m.id = g.metric_id
			WHERE c.team_id = ? AND m.workspace_id = ?`, teamID, workspaceID).Scan(&targets); err != nil {
			return fmt.Errorf("count binding targets: %w", err)
		}
		if targets > 0 {
			return fmt.Errorf("%w: %d SLA goal target(s) still reference this team's calendars", ErrSLAInUse, targets)
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM team_workspace_bindings WHERE id = ?`, bindingID)
		if err != nil {
			return fmt.Errorf("delete binding: %w", err)
		}
		return requireAffected(result)
	})
}

// ListBoundTeamIDs returns the teams bound to a workspace.
func (r *SLARepository) ListBoundTeamIDs(ctx context.Context, workspaceID int) ([]int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT team_id FROM team_workspace_bindings WHERE workspace_id = ? ORDER BY team_id`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list bound teams: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan bound team: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *SLARepository) WorkspaceIDsForCalendarTx(ctx context.Context, tx database.Tx, calendarID int) ([]int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT workspace_id FROM working_calendars WHERE id = ? AND workspace_id IS NOT NULL
		UNION
		SELECT b.workspace_id FROM team_workspace_bindings b
		JOIN working_calendars c ON c.team_id = b.team_id
		WHERE c.id = ? ORDER BY workspace_id`, calendarID, calendarID)
	if err != nil {
		return nil, fmt.Errorf("list calendar workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var workspaceIDs []int
	for rows.Next() {
		var workspaceID int
		if err := rows.Scan(&workspaceID); err != nil {
			return nil, fmt.Errorf("scan calendar workspace: %w", err)
		}
		workspaceIDs = append(workspaceIDs, workspaceID)
	}
	return workspaceIDs, rows.Err()
}

// CalendarAccessibleToWorkspace reports whether a workspace may reference a
// calendar: it owns it, or a bound team owns it.
func (r *SLARepository) CalendarAccessibleToWorkspace(ctx context.Context, workspaceID, calendarID int) (bool, error) {
	var accessible bool
	err := r.db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM working_calendars c
		WHERE c.id = ?
		  AND (c.workspace_id = ?
		       OR c.team_id IN (SELECT team_id FROM team_workspace_bindings WHERE workspace_id = ?))
	)`, calendarID, workspaceID, workspaceID).Scan(&accessible)
	if err != nil {
		return false, fmt.Errorf("check calendar access: %w", err)
	}
	return accessible, nil
}

func (r *SLARepository) CalendarAccessibleToWorkspaceTx(ctx context.Context, tx database.Tx, workspaceID, calendarID int) (bool, error) {
	var ownerWorkspaceID, teamID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT workspace_id, team_id FROM working_calendars WHERE id = ?`, calendarID).Scan(&ownerWorkspaceID, &teamID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("load calendar owner: %w", err)
	}
	if ownerWorkspaceID.Valid && int(ownerWorkspaceID.Int64) == workspaceID {
		return true, nil
	}
	if !teamID.Valid {
		return false, nil
	}
	query := `SELECT 1 FROM team_workspace_bindings WHERE team_id = ? AND workspace_id = ?`
	if database.IsPostgresDriver(r.db.GetDriverName()) {
		query += ` FOR KEY SHARE`
	}
	var found int
	err := tx.QueryRowContext(ctx, query, int(teamID.Int64), workspaceID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock calendar binding: %w", err)
	}
	return true, nil
}

// CompletedCycleCoverage is one completed cycle's stored scheduling snapshot
// and window, used to compute a report's coverage block.
type CompletedCycleCoverage struct {
	CalendarSnapshot string
	StartedAt        time.Time
	StoppedAt        *time.Time
	BreachedAt       *time.Time
}

// CompletedCyclesForCoverage returns completed cycles in the window with the
// snapshot needed to compute exact coverage. History is never recomputed
// against an edited calendar.
func (r *SLARepository) CompletedCyclesForCoverage(ctx context.Context, workspaceID int, from, to *time.Time) ([]CompletedCycleCoverage, error) {
	query := `SELECT c.calendar_snapshot, c.started_at, c.stopped_at, c.breached_at
		FROM item_sla_cycles c
		JOIN sla_metrics m ON m.id = c.metric_id
		WHERE m.workspace_id = ? AND c.status = 'completed' AND c.calendar_snapshot IS NOT NULL`
	args := []any{workspaceID}
	if from != nil {
		query += ` AND c.stopped_at >= ?`
		args = append(args, *from)
	}
	if to != nil {
		query += ` AND c.stopped_at < ?`
		args = append(args, *to)
	}
	query += ` ORDER BY c.stopped_at`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list cycles for coverage: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var cycles []CompletedCycleCoverage
	for rows.Next() {
		var cycle CompletedCycleCoverage
		var stoppedAt, breachedAt sql.NullTime
		if err := rows.Scan(&cycle.CalendarSnapshot, &cycle.StartedAt, &stoppedAt, &breachedAt); err != nil {
			return nil, fmt.Errorf("scan cycle for coverage: %w", err)
		}
		if stoppedAt.Valid {
			value := stoppedAt.Time
			cycle.StoppedAt = &value
		}
		if breachedAt.Valid {
			value := breachedAt.Time
			cycle.BreachedAt = &value
		}
		cycles = append(cycles, cycle)
	}
	return cycles, rows.Err()
}

// ---------------------------------------------------------------------------
// Workspace configuration generation
// ---------------------------------------------------------------------------

// TouchWorkspaceState creates or advances the workspace's configuration
// generation in the caller's transaction and returns the new generation.
func (r *SLARepository) TouchWorkspaceState(ctx context.Context, tx database.Tx, workspaceID int) (int, error) {
	var generation int
	err := tx.QueryRowContext(ctx, `INSERT INTO sla_workspace_state (workspace_id, config_generation)
		VALUES (?, 1)
		ON CONFLICT (workspace_id) DO UPDATE SET config_generation = sla_workspace_state.config_generation + 1
		RETURNING config_generation`, workspaceID).Scan(&generation)
	if err != nil {
		return 0, fmt.Errorf("touch workspace SLA state: %w", err)
	}
	return generation, nil
}

// WorkspaceGeneration returns the current configuration generation, or zero
// when the workspace has no active SLA configuration.
func (r *SLARepository) WorkspaceGeneration(ctx context.Context, workspaceID int) (int, error) {
	var generation int
	err := r.db.QueryRowContext(ctx, `SELECT config_generation FROM sla_workspace_state WHERE workspace_id = ?`, workspaceID).Scan(&generation)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read workspace SLA state: %w", err)
	}
	return generation, nil
}

// GenerationsForWorkspaces returns the configuration generation for each
// supplied workspace. Workspaces without a row are omitted.
func (r *SLARepository) GenerationsForWorkspaces(ctx context.Context, workspaceIDs []int) (map[int]int, error) {
	if len(workspaceIDs) == 0 {
		return map[int]int{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(workspaceIDs)), ",")
	args := make([]any, len(workspaceIDs))
	for i, id := range workspaceIDs {
		args[i] = id
	}
	rows, err := r.db.QueryContext(ctx, `SELECT workspace_id, config_generation FROM sla_workspace_state WHERE workspace_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("read workspace SLA generations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[int]int, len(workspaceIDs))
	for rows.Next() {
		var workspaceID, generation int
		if err := rows.Scan(&workspaceID, &generation); err != nil {
			return nil, fmt.Errorf("scan workspace generation: %w", err)
		}
		out[workspaceID] = generation
	}
	return out, rows.Err()
}

// GenerationsForWorkspacesTx is the in-transaction form of
// GenerationsForWorkspaces. Reading the generation under the same transaction
// as the item write prevents a stale compiled configuration from being used.
func (r *SLARepository) GenerationsForWorkspacesTx(ctx context.Context, tx database.Tx, workspaceIDs []int) (map[int]int, error) {
	if len(workspaceIDs) == 0 {
		return map[int]int{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(workspaceIDs)), ",")
	args := make([]any, len(workspaceIDs))
	for i, id := range workspaceIDs {
		args[i] = id
	}
	rows, err := tx.QueryContext(ctx, `SELECT workspace_id, config_generation FROM sla_workspace_state WHERE workspace_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("read workspace SLA generations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[int]int, len(workspaceIDs))
	for rows.Next() {
		var workspaceID, generation int
		if err := rows.Scan(&workspaceID, &generation); err != nil {
			return nil, fmt.Errorf("scan workspace generation: %w", err)
		}
		out[workspaceID] = generation
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Metrics
// ---------------------------------------------------------------------------

// ListMetrics returns a workspace's metrics with conditions, goals, and
// targets hydrated.
func (r *SLARepository) ListMetrics(ctx context.Context, workspaceID int) ([]models.SLAMetric, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, workspace_id, name, display_format, position, is_active,
		import_status, source, source_id, source_payload, created_at, updated_at
		FROM sla_metrics WHERE workspace_id = ? ORDER BY position, id`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list metrics: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var metrics []models.SLAMetric
	for rows.Next() {
		metric, err := scanMetric(rows)
		if err != nil {
			return nil, fmt.Errorf("scan metric: %w", err)
		}
		metrics = append(metrics, metric)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range metrics {
		if err := r.hydrateMetric(ctx, &metrics[i]); err != nil {
			return nil, err
		}
	}
	return metrics, nil
}

// ListMetricSummaries returns a workspace's metric identity fields without
// hydrating conditions, goals, or targets. Badge rendering only needs the
// metric id, name, and display format, so this avoids the per-metric and
// per-goal configuration queries ListMetrics issues.
func (r *SLARepository) ListMetricSummaries(ctx context.Context, workspaceID int) ([]models.SLAMetric, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, workspace_id, name, display_format, position, is_active,
		import_status, source, source_id, source_payload, created_at, updated_at
		FROM sla_metrics WHERE workspace_id = ? ORDER BY position, id`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list metric summaries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var metrics []models.SLAMetric
	for rows.Next() {
		metric, err := scanMetric(rows)
		if err != nil {
			return nil, fmt.Errorf("scan metric summary: %w", err)
		}
		metrics = append(metrics, metric)
	}
	return metrics, rows.Err()
}

// GetMetric loads one metric with its conditions, goals, and targets.
func (r *SLARepository) GetMetric(ctx context.Context, metricID int) (*models.SLAMetric, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, workspace_id, name, display_format, position, is_active,
		import_status, source, source_id, source_payload, created_at, updated_at
		FROM sla_metrics WHERE id = ?`, metricID)
	metric, err := scanMetric(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load metric: %w", err)
	}
	if err := r.hydrateMetric(ctx, &metric); err != nil {
		return nil, err
	}
	return &metric, nil
}

func scanMetric(scanner interface{ Scan(...any) error }) (models.SLAMetric, error) {
	var metric models.SLAMetric
	var source, sourceID, sourcePayload sql.NullString
	err := scanner.Scan(&metric.ID, &metric.WorkspaceID, &metric.Name, &metric.DisplayFormat, &metric.Position,
		&metric.IsActive, &metric.ImportStatus, &source, &sourceID, &sourcePayload, &metric.CreatedAt, &metric.UpdatedAt)
	if err != nil {
		return models.SLAMetric{}, err
	}
	if source.Valid {
		metric.Source = &source.String
	}
	if sourceID.Valid {
		metric.SourceID = &sourceID.String
	}
	if sourcePayload.Valid {
		metric.SourcePayload = &sourcePayload.String
	}
	return metric, nil
}

func (r *SLARepository) hydrateMetric(ctx context.Context, metric *models.SLAMetric) error {
	conditionRows, err := r.db.QueryContext(ctx, `SELECT id, metric_id, phase, position, condition_type, config, source_payload
		FROM sla_metric_conditions WHERE metric_id = ? ORDER BY phase, position, id`, metric.ID)
	if err != nil {
		return fmt.Errorf("load metric conditions: %w", err)
	}
	for conditionRows.Next() {
		var condition models.SLACondition
		var config []byte
		var sourcePayload sql.NullString
		if err := conditionRows.Scan(&condition.ID, &condition.MetricID, &condition.Phase, &condition.Position, &condition.ConditionType, &config, &sourcePayload); err != nil {
			_ = conditionRows.Close()
			return fmt.Errorf("scan metric condition: %w", err)
		}
		condition.Config = append(json.RawMessage(nil), config...)
		if sourcePayload.Valid {
			condition.SourcePayload = &sourcePayload.String
		}
		metric.Conditions = append(metric.Conditions, condition)
	}
	if err := conditionRows.Err(); err != nil {
		_ = conditionRows.Close()
		return fmt.Errorf("iterate metric conditions: %w", err)
	}
	if err := conditionRows.Close(); err != nil {
		return err
	}

	goalRows, err := r.db.QueryContext(ctx, `SELECT id, metric_id, position, ql_query, original_jql, import_status, source_id, source_payload, created_at, updated_at
		FROM sla_goals WHERE metric_id = ? ORDER BY position, id`, metric.ID)
	if err != nil {
		return fmt.Errorf("load metric goals: %w", err)
	}
	for goalRows.Next() {
		var goal models.SLAGoal
		var originalJQL, sourceID, sourcePayload sql.NullString
		if err := goalRows.Scan(&goal.ID, &goal.MetricID, &goal.Position, &goal.QLQuery, &originalJQL, &goal.ImportStatus, &sourceID, &sourcePayload, &goal.CreatedAt, &goal.UpdatedAt); err != nil {
			_ = goalRows.Close()
			return fmt.Errorf("scan metric goal: %w", err)
		}
		if originalJQL.Valid {
			goal.OriginalJQL = &originalJQL.String
		}
		if sourceID.Valid {
			goal.SourceID = &sourceID.String
		}
		if sourcePayload.Valid {
			goal.SourcePayload = &sourcePayload.String
		}
		metric.Goals = append(metric.Goals, goal)
	}
	if err := goalRows.Err(); err != nil {
		_ = goalRows.Close()
		return fmt.Errorf("iterate metric goals: %w", err)
	}
	if err := goalRows.Close(); err != nil {
		return err
	}
	for i := range metric.Goals {
		targets, err := r.listTargets(ctx, metric.Goals[i].ID)
		if err != nil {
			return err
		}
		metric.Goals[i].Targets = targets
	}
	return nil
}

func (r *SLARepository) listTargets(ctx context.Context, goalID int) ([]models.SLAGoalTarget, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, goal_id, position, target_ms, calendar_id, source_id, source_payload
		FROM sla_goal_targets WHERE goal_id = ? ORDER BY position, id`, goalID)
	if err != nil {
		return nil, fmt.Errorf("load goal targets: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var targets []models.SLAGoalTarget
	for rows.Next() {
		var target models.SLAGoalTarget
		var sourceID, sourcePayload sql.NullString
		if err := rows.Scan(&target.ID, &target.GoalID, &target.Position, &target.TargetMs, &target.CalendarID, &sourceID, &sourcePayload); err != nil {
			return nil, fmt.Errorf("scan goal target: %w", err)
		}
		if sourceID.Valid {
			target.SourceID = &sourceID.String
		}
		if sourcePayload.Valid {
			target.SourcePayload = &sourcePayload.String
		}
		targets = append(targets, target)
	}
	return targets, rows.Err()
}

// CreateMetric inserts a metric and its conditions, goals, and targets in the
// caller's transaction.
func (r *SLARepository) CreateMetric(ctx context.Context, tx database.Tx, metric *models.SLAMetric) (int, error) {
	var metricID int
	err := tx.QueryRowContext(ctx, `INSERT INTO sla_metrics
		(workspace_id, name, display_format, position, is_active, import_status, source, source_id, source_payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		metric.WorkspaceID, metric.Name, metric.DisplayFormat, metric.Position, metric.IsActive,
		metric.ImportStatus, metric.Source, metric.SourceID, metric.SourcePayload).Scan(&metricID)
	if err != nil {
		return 0, fmt.Errorf("create metric: %w", err)
	}
	if err := r.replaceMetricChildren(ctx, tx, metricID, metric); err != nil {
		return 0, err
	}
	return metricID, nil
}

// UpdateMetric rewrites a metric and replaces its conditions, goals, and
// targets in the caller's transaction.
func (r *SLARepository) UpdateMetric(ctx context.Context, tx database.Tx, metric *models.SLAMetric) error {
	result, err := tx.ExecContext(ctx, `UPDATE sla_metrics
		SET name = ?, display_format = ?, position = ?, is_active = ?, import_status = ?, source = ?, source_id = ?, source_payload = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND workspace_id = ?`,
		metric.Name, metric.DisplayFormat, metric.Position, metric.IsActive, metric.ImportStatus,
		metric.Source, metric.SourceID, metric.SourcePayload, metric.ID, metric.WorkspaceID)
	if err != nil {
		return fmt.Errorf("update metric: %w", err)
	}
	if err := requireAffected(result); err != nil {
		return err
	}
	return r.replaceMetricChildren(ctx, tx, metric.ID, metric)
}

func (r *SLARepository) replaceMetricChildren(ctx context.Context, tx database.Tx, metricID int, metric *models.SLAMetric) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM sla_metric_conditions WHERE metric_id = ?`, metricID); err != nil {
		return fmt.Errorf("clear metric conditions: %w", err)
	}
	for _, condition := range metric.Conditions {
		config := condition.Config
		if len(config) == 0 {
			config = json.RawMessage("{}")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO sla_metric_conditions (metric_id, phase, position, condition_type, config, source_payload)
			VALUES (?, ?, ?, ?, ?, ?)`,
			metricID, condition.Phase, condition.Position, condition.ConditionType, string(config), condition.SourcePayload); err != nil {
			return fmt.Errorf("insert metric condition: %w", err)
		}
	}
	return r.syncMetricGoals(ctx, tx, metricID, metric.Goals)
}

// syncMetricGoals reconciles a metric's goals in place. Existing rows are
// matched by id, then source id, then position, and updated so cycles that
// reference them keep their goal link; goals omitted from the new
// configuration are deleted. Targets have no downstream references, so they
// are replaced.
func (r *SLARepository) syncMetricGoals(ctx context.Context, tx database.Tx, metricID int, goals []models.SLAGoal) error {
	type storedGoal struct {
		id       int
		position int
		sourceID sql.NullString
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, position, source_id FROM sla_goals WHERE metric_id = ?`, metricID)
	if err != nil {
		return fmt.Errorf("load metric goals: %w", err)
	}
	var stored []storedGoal
	for rows.Next() {
		var goal storedGoal
		if err := rows.Scan(&goal.id, &goal.position, &goal.sourceID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan metric goal: %w", err)
		}
		stored = append(stored, goal)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	byID := make(map[int]int, len(stored))
	bySource := make(map[string]int, len(stored))
	byPosition := make(map[int]int, len(stored))
	for i, goal := range stored {
		byID[goal.id] = i
		if goal.sourceID.Valid {
			bySource[goal.sourceID.String] = i
		}
		byPosition[goal.position] = i
	}

	// Park the existing rows on unique negative positions so reordering cannot
	// trip the (metric_id, position) unique constraint mid-update.
	for _, goal := range stored {
		if _, err := tx.ExecContext(ctx, `UPDATE sla_goals SET position = ? WHERE id = ?`, -goal.id, goal.id); err != nil {
			return fmt.Errorf("park metric goal: %w", err)
		}
	}

	used := make([]bool, len(stored))
	for _, goal := range goals {
		match := -1
		if goal.ID != 0 {
			if index, ok := byID[goal.ID]; ok && !used[index] {
				match = index
			}
		}
		if match < 0 && goal.SourceID != nil {
			if index, ok := bySource[*goal.SourceID]; ok && !used[index] {
				match = index
			}
		}
		if match < 0 {
			if index, ok := byPosition[goal.Position]; ok && !used[index] {
				match = index
			}
		}

		goalID := 0
		if match >= 0 {
			used[match] = true
			goalID = stored[match].id
			if _, err := tx.ExecContext(ctx, `UPDATE sla_goals SET position = ?, ql_query = ?, original_jql = ?, import_status = ?, source_id = ?, source_payload = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
				goal.Position, goal.QLQuery, goal.OriginalJQL, goal.ImportStatus, goal.SourceID, goal.SourcePayload, goalID); err != nil {
				return fmt.Errorf("update goal: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM sla_goal_targets WHERE goal_id = ?`, goalID); err != nil {
				return fmt.Errorf("clear goal targets: %w", err)
			}
		} else {
			if err := tx.QueryRowContext(ctx, `INSERT INTO sla_goals (metric_id, position, ql_query, original_jql, import_status, source_id, source_payload)
				VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
				metricID, goal.Position, goal.QLQuery, goal.OriginalJQL, goal.ImportStatus, goal.SourceID, goal.SourcePayload).Scan(&goalID); err != nil {
				return fmt.Errorf("insert goal: %w", err)
			}
		}
		if err := r.insertGoalTargets(ctx, tx, goalID, goal.Targets); err != nil {
			return err
		}
	}

	for index, goal := range stored {
		if used[index] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM sla_goals WHERE id = ?`, goal.id); err != nil {
			return fmt.Errorf("delete goal: %w", err)
		}
	}
	return nil
}

func (r *SLARepository) insertGoalTargets(ctx context.Context, tx database.Tx, goalID int, targets []models.SLAGoalTarget) error {
	for _, target := range targets {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sla_goal_targets (goal_id, position, target_ms, calendar_id, source_id, source_payload)
			VALUES (?, ?, ?, ?, ?, ?)`,
			goalID, target.Position, target.TargetMs, target.CalendarID, target.SourceID, target.SourcePayload); err != nil {
			return fmt.Errorf("insert goal target: %w", err)
		}
	}
	return nil
}

// DeleteMetric removes a metric; cycles and jobs cascade.
func (r *SLARepository) DeleteMetric(ctx context.Context, tx database.Tx, metricID int) error {
	result, err := tx.ExecContext(ctx, `DELETE FROM sla_metrics WHERE id = ?`, metricID)
	if err != nil {
		return fmt.Errorf("delete metric: %w", err)
	}
	return requireAffected(result)
}

// DeleteWorkspaceMetricsTx removes every SLA metric in a workspace, cascading
// goals, targets, cycles, and jobs. Deleting metrics before a workspace
// removes goal targets before the workspace's calendars cascade, which would
// otherwise trip the calendar RESTRICT depending on cascade order.
func (r *SLARepository) DeleteWorkspaceMetricsTx(ctx context.Context, tx database.Tx, workspaceID int) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM sla_metrics WHERE workspace_id = ?`, workspaceID); err != nil {
		return fmt.Errorf("delete workspace SLA metrics: %w", err)
	}
	return nil
}

// TeamReferencedCalendars counts goal targets that reference the team's
// calendars. A non-zero count means deleting the team would strand an SLA, so
// the delete must be refused with a domain error rather than a raw FK failure.
func (r *SLARepository) TeamReferencedCalendars(ctx context.Context, teamID int) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sla_goal_targets t
		JOIN working_calendars c ON c.id = t.calendar_id
		WHERE c.team_id = ?`, teamID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count team calendar references: %w", err)
	}
	return count, nil
}

// ---------------------------------------------------------------------------
// Cycles
// ---------------------------------------------------------------------------

const cycleColumns = `id, item_id, metric_id, goal_id, calendar_id, cycle_no, status,
	started_at, stopped_at, breach_time, goal_duration_ms, elapsed_ms, remaining_ms,
	paused, within_calendar_hours, breached, pause_started_at, next_deadline_at,
	last_calculated_at, remaining_at_pause_ms, breached_at, origin, abandon_reason,
	calendar_snapshot, goal_query_snapshot, source_id, source_payload, created_at, updated_at`

func scanCycle(scanner interface{ Scan(...any) error }) (models.ItemSLACycle, error) {
	var cycle models.ItemSLACycle
	var goalID, calendarID sql.NullInt64
	var stoppedAt, breachTime, pauseStartedAt, nextDeadlineAt, breachedAt sql.NullTime
	var remainingAtPause sql.NullInt64
	var abandonReason, sourceID, sourcePayload sql.NullString
	var calendarSnapshot []byte
	err := scanner.Scan(
		&cycle.ID, &cycle.ItemID, &cycle.MetricID, &goalID, &calendarID, &cycle.CycleNo, &cycle.Status,
		&cycle.StartedAt, &stoppedAt, &breachTime, &cycle.GoalDurationMs, &cycle.ElapsedMs, &cycle.RemainingMs,
		&cycle.Paused, &cycle.WithinCalendarHours, &cycle.Breached, &pauseStartedAt, &nextDeadlineAt,
		&cycle.LastCalculatedAt, &remainingAtPause, &breachedAt, &cycle.Origin, &abandonReason,
		&calendarSnapshot, &cycle.GoalQuerySnapshot, &sourceID, &sourcePayload, &cycle.CreatedAt, &cycle.UpdatedAt,
	)
	if err != nil {
		return models.ItemSLACycle{}, err
	}
	if goalID.Valid {
		value := int(goalID.Int64)
		cycle.GoalID = &value
	}
	if calendarID.Valid {
		value := int(calendarID.Int64)
		cycle.CalendarID = &value
	}
	if stoppedAt.Valid {
		cycle.StoppedAt = &stoppedAt.Time
	}
	if breachTime.Valid {
		cycle.BreachTime = &breachTime.Time
	}
	if pauseStartedAt.Valid {
		cycle.PauseStartedAt = &pauseStartedAt.Time
	}
	if nextDeadlineAt.Valid {
		cycle.NextDeadlineAt = &nextDeadlineAt.Time
	}
	if breachedAt.Valid {
		cycle.BreachedAt = &breachedAt.Time
	}
	if remainingAtPause.Valid {
		value := remainingAtPause.Int64
		cycle.RemainingAtPauseMs = &value
	}
	if sourceID.Valid {
		cycle.SourceID = &sourceID.String
	}
	if abandonReason.Valid {
		cycle.AbandonReason = &abandonReason.String
	}
	if sourcePayload.Valid {
		cycle.SourcePayload = &sourcePayload.String
	}
	cycle.CalendarSnapshot = append(json.RawMessage(nil), calendarSnapshot...)
	return cycle, nil
}

// LockOngoingCycles locks and returns the ongoing cycles for a set of items
// inside the caller's transaction. SQLite serializes writers so it omits
// FOR UPDATE; PostgreSQL takes a row lock ordered with the item write.
func (r *SLARepository) LockOngoingCycles(ctx context.Context, tx database.Tx, itemIDs []int) ([]models.ItemSLACycle, error) {
	if len(itemIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(itemIDs)), ",")
	args := make([]any, len(itemIDs))
	for i, id := range itemIDs {
		args[i] = id
	}
	query := `SELECT ` + cycleColumns + ` FROM item_sla_cycles WHERE status = 'ongoing' AND item_id IN (` + placeholders + `) ORDER BY item_id, metric_id`
	if r.db.GetDriverName() == "postgres" {
		query += " FOR UPDATE"
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("lock ongoing cycles: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var cycles []models.ItemSLACycle
	for rows.Next() {
		cycle, err := scanCycle(rows)
		if err != nil {
			return nil, fmt.Errorf("scan locked cycle: %w", err)
		}
		cycles = append(cycles, cycle)
	}
	return cycles, rows.Err()
}

// StatusCategoryInfo returns status -> category mapping and the set of
// completed category IDs, used to evaluate status/category SLA conditions.
func (r *SLARepository) StatusCategoryInfo(ctx context.Context) (categoryByStatus map[int]int, categoryHasCompleted map[int]bool, err error) {
	rows, err := r.db.QueryContext(ctx, `SELECT s.id, s.category_id, COALESCE(c.is_completed, false)
		FROM statuses s LEFT JOIN status_categories c ON c.id = s.category_id`)
	if err != nil {
		return nil, nil, fmt.Errorf("load status categories: %w", err)
	}
	defer func() { _ = rows.Close() }()
	categories := map[int]int{}
	completed := map[int]bool{}
	for rows.Next() {
		var statusID, categoryID int
		var isCompleted bool
		if err := rows.Scan(&statusID, &categoryID, &isCompleted); err != nil {
			return nil, nil, fmt.Errorf("scan status category: %w", err)
		}
		categories[statusID] = categoryID
		if isCompleted {
			completed[categoryID] = true
		}
	}
	return categories, completed, rows.Err()
}

// WorkspaceKey returns the key for a workspace, used to resolve workspace QL
// predicates in a compiled goal.
func (r *SLARepository) WorkspaceKey(ctx context.Context, workspaceID int) (string, error) {
	var key string
	err := r.db.QueryRowContext(ctx, `SELECT key FROM workspaces WHERE id = ?`, workspaceID).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("load workspace key: %w", err)
	}
	return key, nil
}

// ListCyclesForItem returns every cycle for an item, newest cycle first.
func (r *SLARepository) ListCyclesForItem(ctx context.Context, itemID int) ([]models.ItemSLACycle, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+cycleColumns+` FROM item_sla_cycles WHERE item_id = ? ORDER BY metric_id, cycle_no`, itemID)
	if err != nil {
		return nil, fmt.Errorf("list item cycles: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var cycles []models.ItemSLACycle
	for rows.Next() {
		cycle, err := scanCycle(rows)
		if err != nil {
			return nil, fmt.Errorf("scan cycle: %w", err)
		}
		cycles = append(cycles, cycle)
	}
	return cycles, rows.Err()
}

// ListCyclesForItems loads the cycles of many items in one query, grouped by
// item id. Items without cycles are absent from the map.
func (r *SLARepository) ListCyclesForItems(ctx context.Context, itemIDs []int) (map[int][]models.ItemSLACycle, error) {
	grouped := make(map[int][]models.ItemSLACycle, len(itemIDs))
	if len(itemIDs) == 0 {
		return grouped, nil
	}
	params := make([]any, 0, len(itemIDs))
	for _, id := range itemIDs {
		params = append(params, id)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+cycleColumns+` FROM item_sla_cycles WHERE item_id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(itemIDs)), ",")+") ORDER BY item_id, metric_id, cycle_no", params...)
	if err != nil {
		return nil, fmt.Errorf("list item cycles: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		cycle, err := scanCycle(rows)
		if err != nil {
			return nil, fmt.Errorf("scan cycle: %w", err)
		}
		grouped[cycle.ItemID] = append(grouped[cycle.ItemID], cycle)
	}
	return grouped, rows.Err()
}

// ListOngoingCyclesForItems loads only the ongoing cycles of many items in one
// query, grouped by item id. Badge reads use this so response size tracks the
// number of active metrics rather than the item's ticket history.
func (r *SLARepository) ListOngoingCyclesForItems(ctx context.Context, itemIDs []int) (map[int][]models.ItemSLACycle, error) {
	grouped := make(map[int][]models.ItemSLACycle, len(itemIDs))
	if len(itemIDs) == 0 {
		return grouped, nil
	}
	params := make([]any, 0, len(itemIDs))
	for _, id := range itemIDs {
		params = append(params, id)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+cycleColumns+` FROM item_sla_cycles WHERE status = 'ongoing' AND item_id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(itemIDs)), ",")+") ORDER BY item_id, metric_id, cycle_no", params...)
	if err != nil {
		return nil, fmt.Errorf("list ongoing item cycles: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		cycle, err := scanCycle(rows)
		if err != nil {
			return nil, fmt.Errorf("scan ongoing cycle: %w", err)
		}
		grouped[cycle.ItemID] = append(grouped[cycle.ItemID], cycle)
	}
	return grouped, rows.Err()
}

// GetCycle loads one cycle.
func (r *SLARepository) GetCycle(ctx context.Context, cycleID int64) (*models.ItemSLACycle, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+cycleColumns+` FROM item_sla_cycles WHERE id = ?`, cycleID)
	cycle, err := scanCycle(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load cycle: %w", err)
	}
	return &cycle, nil
}

func (r *SLARepository) OngoingCycleIDsForCalendarTx(ctx context.Context, tx database.Tx, metricID, calendarID int, afterID int64, limit int) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM item_sla_cycles
		WHERE metric_id = ? AND calendar_id = ? AND status = 'ongoing' AND paused = false AND id > ?
		ORDER BY id LIMIT ?`, metricID, calendarID, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("page ongoing calendar cycles: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan ongoing calendar cycle ID: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *SLARepository) LockItemForCycle(ctx context.Context, tx database.Tx, cycleID int64) error {
	var itemID int
	if err := tx.QueryRowContext(ctx, `SELECT item_id FROM item_sla_cycles WHERE id = ?`, cycleID).Scan(&itemID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("load cycle item: %w", err)
	}
	query := `SELECT id FROM items WHERE id = ?`
	if database.IsPostgresDriver(r.db.GetDriverName()) {
		query += ` FOR UPDATE`
	}
	if err := tx.QueryRowContext(ctx, query, itemID).Scan(&itemID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock cycle item: %w", err)
	}
	return nil
}

// GetCycleForUpdate loads a cycle under a row lock inside the caller's
// transaction, ordering job execution with inline evaluation.
func (r *SLARepository) GetCycleForUpdate(ctx context.Context, tx database.Tx, cycleID int64) (*models.ItemSLACycle, error) {
	query := `SELECT ` + cycleColumns + ` FROM item_sla_cycles WHERE id = ?`
	if r.db.GetDriverName() == "postgres" {
		query += " FOR UPDATE"
	}
	row := tx.QueryRowContext(ctx, query, cycleID)
	cycle, err := scanCycle(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load cycle for update: %w", err)
	}
	return &cycle, nil
}

// NextCycleNo returns the next cycle number for an item and metric.
func (r *SLARepository) NextCycleNo(ctx context.Context, tx database.Tx, itemID, metricID int) (int, error) {
	var next int
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(cycle_no), 0) + 1 FROM item_sla_cycles WHERE item_id = ? AND metric_id = ?`, itemID, metricID).Scan(&next)
	if err != nil {
		return 0, fmt.Errorf("next cycle number: %w", err)
	}
	return next, nil
}

// InsertCycle inserts a cycle in the caller's transaction and returns its ID.
func (r *SLARepository) InsertCycle(ctx context.Context, tx database.Tx, cycle *models.ItemSLACycle) (int64, error) {
	holidays := cycle.CalendarSnapshot
	if len(holidays) == 0 {
		holidays = json.RawMessage("[]")
	}
	var id int64
	err := tx.QueryRowContext(ctx, `INSERT INTO item_sla_cycles
		(item_id, metric_id, goal_id, calendar_id, cycle_no, status, started_at, stopped_at, breach_time,
		 goal_duration_ms, elapsed_ms, remaining_ms, paused, within_calendar_hours, breached,
		 pause_started_at, next_deadline_at, last_calculated_at, remaining_at_pause_ms, breached_at,
		 origin, abandon_reason, calendar_snapshot, goal_query_snapshot, source_id, source_payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		cycle.ItemID, cycle.MetricID, nullableInt(cycle.GoalID), nullableInt(cycle.CalendarID), cycle.CycleNo, cycle.Status,
		cycle.StartedAt, cycle.StoppedAt, cycle.BreachTime, cycle.GoalDurationMs, cycle.ElapsedMs, cycle.RemainingMs,
		cycle.Paused, cycle.WithinCalendarHours, cycle.Breached, cycle.PauseStartedAt, cycle.NextDeadlineAt,
		cycle.LastCalculatedAt, cycle.RemainingAtPauseMs, cycle.BreachedAt, cycle.Origin, cycle.AbandonReason,
		string(holidays), cycle.GoalQuerySnapshot, cycle.SourceID, cycle.SourcePayload).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert cycle: %w", err)
	}
	return id, nil
}

// UpdateCycle persists all mutable cycle fields in the caller's transaction.
func (r *SLARepository) UpdateCycle(ctx context.Context, tx database.Tx, cycle *models.ItemSLACycle) error {
	result, err := tx.ExecContext(ctx, `UPDATE item_sla_cycles SET
		item_id = ?, metric_id = ?, cycle_no = ?, goal_id = ?, calendar_id = ?, status = ?, stopped_at = ?, breach_time = ?, goal_duration_ms = ?,
		elapsed_ms = ?, remaining_ms = ?, paused = ?, within_calendar_hours = ?, breached = ?,
		pause_started_at = ?, next_deadline_at = ?, last_calculated_at = ?, remaining_at_pause_ms = ?,
		breached_at = ?, origin = ?, abandon_reason = ?, calendar_snapshot = ?, goal_query_snapshot = ?, source_id = ?, source_payload = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		cycle.ItemID, cycle.MetricID, cycle.CycleNo, nullableInt(cycle.GoalID), nullableInt(cycle.CalendarID), cycle.Status, cycle.StoppedAt, cycle.BreachTime,
		cycle.GoalDurationMs, cycle.ElapsedMs, cycle.RemainingMs, cycle.Paused, cycle.WithinCalendarHours,
		cycle.Breached, cycle.PauseStartedAt, cycle.NextDeadlineAt, cycle.LastCalculatedAt, cycle.RemainingAtPauseMs,
		cycle.BreachedAt, cycle.Origin, cycle.AbandonReason, string(cycle.CalendarSnapshot), cycle.GoalQuerySnapshot, cycle.SourceID, cycle.SourcePayload, cycle.ID)
	if err != nil {
		return fmt.Errorf("update cycle: %w", err)
	}
	return requireAffected(result)
}

// ---------------------------------------------------------------------------
// Jobs
// ---------------------------------------------------------------------------

const jobColumns = `id, kind, threshold_key, cycle_id, item_id, metric_id, due_at, deadline_at,
	cursor, attempts, lease_owner, last_error, state, created_at`

// jobColumnsPrefixed qualifies jobColumns for the "sla_jobs j" alias. Declared
// explicitly (not derived from jobColumns) so joined queries stay in scanJob's
// column order when the base constant's formatting changes.
const jobColumnsPrefixed = `j.id, j.kind, j.threshold_key, j.cycle_id, j.item_id, j.metric_id, j.due_at,
	j.deadline_at, j.cursor, j.attempts, j.lease_owner, j.last_error, j.state, j.created_at`

func scanJob(scanner interface{ Scan(...any) error }) (models.SLAJob, error) {
	var job models.SLAJob
	var cycleID, itemID, metricID sql.NullInt64
	var deadlineAt sql.NullTime
	var cursor, leaseOwner, lastError sql.NullString
	err := scanner.Scan(&job.ID, &job.Kind, &job.ThresholdKey, &cycleID, &itemID, &metricID, &job.DueAt,
		&deadlineAt, &cursor, &job.Attempts, &leaseOwner, &lastError, &job.State, &job.CreatedAt)
	if err != nil {
		return models.SLAJob{}, err
	}
	if cycleID.Valid {
		value := cycleID.Int64
		job.CycleID = &value
	}
	if itemID.Valid {
		value := int(itemID.Int64)
		job.ItemID = &value
	}
	if metricID.Valid {
		value := int(metricID.Int64)
		job.MetricID = &value
	}
	if deadlineAt.Valid {
		job.DeadlineAt = &deadlineAt.Time
	}
	if cursor.Valid {
		job.Cursor = &cursor.String
	}
	if leaseOwner.Valid {
		job.LeaseOwner = &leaseOwner.String
	}
	if lastError.Valid {
		job.LastError = &lastError.String
	}
	return job, nil
}

// UpsertJob arms a job subject, replacing any pending row for the same
// subject. It never accumulates duplicate rows.
func (r *SLARepository) UpsertJob(ctx context.Context, tx database.Tx, job *models.SLAJob) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO sla_jobs
		(kind, threshold_key, cycle_id, item_id, metric_id, due_at, deadline_at, cursor)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (kind, threshold_key, COALESCE(cycle_id, 0), COALESCE(item_id, 0), COALESCE(metric_id, 0))
		DO UPDATE SET due_at = excluded.due_at, deadline_at = excluded.deadline_at, cursor = excluded.cursor,
			attempts = 0, lease_owner = NULL, last_error = NULL, state = 'pending'`,
		job.Kind, job.ThresholdKey, job.CycleID, job.ItemID, job.MetricID, job.DueAt, job.DeadlineAt, job.Cursor)
	if err != nil {
		return fmt.Errorf("upsert SLA job: %w", err)
	}
	return nil
}

// DeleteJob removes a job by identity. Missing rows are not an error. Used
// by self-re-arming recalculation jobs; claimed deadline jobs must use
// DeleteClaimedJob so a stale runner cannot retire a replacement.
func (r *SLARepository) DeleteJob(ctx context.Context, tx database.Tx, kind, thresholdKey string, cycleID *int64, itemID, metricID *int) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM sla_jobs
		WHERE kind = ? AND threshold_key = ?
		  AND COALESCE(cycle_id, 0) = COALESCE(?, 0)
		  AND COALESCE(item_id, 0) = COALESCE(?, 0)
		  AND COALESCE(metric_id, 0) = COALESCE(?, 0)`,
		kind, thresholdKey, nullableInt64(cycleID), nullableInt(itemID), nullableInt(metricID))
	if err != nil {
		return fmt.Errorf("delete SLA job: %w", err)
	}
	return nil
}

// DeleteClaimedJob removes exactly the row this runner claimed: the delete
// only applies while the runner still owns the lease. An inline re-arm that
// upserted the subject clears lease_owner, so a stale claimed job whose
// deadline was replaced can no longer delete the replacement (WI-1575).
func (r *SLARepository) DeleteClaimedJob(ctx context.Context, tx database.Tx, job models.SLAJob, owner string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM sla_jobs WHERE id = ? AND lease_owner = ?`, job.ID, owner)
	if err != nil {
		return fmt.Errorf("delete claimed SLA job: %w", err)
	}
	return nil
}

// DeleteJobsForCycle removes every pending job tied to a cycle.
func (r *SLARepository) DeleteJobsForCycle(ctx context.Context, tx database.Tx, cycleID int64) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM sla_jobs WHERE cycle_id = ?`, cycleID)
	if err != nil {
		return fmt.Errorf("delete cycle jobs: %w", err)
	}
	return nil
}

// NextDueAt returns the earliest pending due time, or ok = false when no job
// is pending.
func (r *SLARepository) NextDueAt(ctx context.Context) (time.Time, bool, error) {
	// Read the column directly rather than MIN(due_at): SQLite returns
	// timestamps as text for expressions but parses direct column scans.
	var due time.Time
	err := r.db.QueryRowContext(ctx, `SELECT due_at FROM sla_jobs WHERE state = 'pending' ORDER BY due_at LIMIT 1`).Scan(&due)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("read next SLA due time: %w", err)
	}
	return due, true, nil
}

// ClaimDueJobs leases up to limit pending jobs due at or before now. The
// outer WHERE rechecks state and due_at, and PostgreSQL locks the candidate
// rows with SKIP LOCKED, so two replicas running the same claim cannot lease
// the same job.
func (r *SLARepository) ClaimDueJobs(ctx context.Context, now time.Time, lease time.Duration, owner string, limit int) ([]models.SLAJob, error) {
	leaseUntil := now.Add(lease)
	candidates := `SELECT id FROM sla_jobs WHERE state = 'pending' AND due_at <= ? ORDER BY due_at LIMIT ?`
	if database.IsPostgresDriver(r.db.GetDriverName()) {
		candidates += ` FOR UPDATE SKIP LOCKED`
	}
	rows, err := r.db.QueryContext(ctx, `UPDATE sla_jobs
		SET due_at = ?, lease_owner = ?, attempts = attempts + 1
		WHERE state = 'pending' AND due_at <= ? AND id IN (`+candidates+`)
		RETURNING `+jobColumns, leaseUntil, owner, now, now, limit)
	if err != nil {
		return nil, fmt.Errorf("claim SLA jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var jobs []models.SLAJob
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("scan claimed job: %w", err)
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// RescheduleJob sets a new due time and records the error.
func (r *SLARepository) RescheduleJob(ctx context.Context, jobID int64, dueAt time.Time, lastError string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sla_jobs SET due_at = ?, lease_owner = NULL, last_error = ? WHERE id = ?`, dueAt, lastError, jobID)
	if err != nil {
		return fmt.Errorf("reschedule SLA job: %w", err)
	}
	return nil
}

// RescheduleOwnedJob is the lease-fenced error path: an owner whose lease
// expired and whose job was reclaimed or re-armed cannot reschedule the row.
func (r *SLARepository) RescheduleOwnedJob(ctx context.Context, jobID int64, owner, lastError string, dueAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sla_jobs SET due_at = ?, lease_owner = NULL, last_error = ?
		WHERE id = ? AND lease_owner = ? AND state = 'pending'`, dueAt, lastError, jobID, owner)
	if err != nil {
		return fmt.Errorf("reschedule owned SLA job: %w", err)
	}
	return nil
}

// FailJob parks a job in the failed state for diagnostics.
func (r *SLARepository) FailJob(ctx context.Context, jobID int64, lastError string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sla_jobs SET state = 'failed', lease_owner = NULL, last_error = ? WHERE id = ?`, lastError, jobID)
	if err != nil {
		return fmt.Errorf("fail SLA job: %w", err)
	}
	return nil
}

// FailOwnedJob is the lease-fenced failure path for claimed jobs.
func (r *SLARepository) FailOwnedJob(ctx context.Context, jobID int64, owner, lastError string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sla_jobs SET state = 'failed', lease_owner = NULL, last_error = ?
		WHERE id = ? AND lease_owner = ? AND state = 'pending'`, lastError, jobID, owner)
	if err != nil {
		return fmt.Errorf("fail owned SLA job: %w", err)
	}
	return nil
}

// RenewJobsLease extends the lease of jobs this owner still holds and returns
// the ids that were renewed. A job reclaimed by another owner or re-armed by
// an inline evaluation is not renewed; the caller must drop it unrun.
func (r *SLARepository) RenewJobsLease(ctx context.Context, jobIDs []int64, owner string, until time.Time) ([]int64, error) {
	if len(jobIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(jobIDs)), ",")
	args := make([]any, 0, len(jobIDs)+2)
	args = append(args, until, owner)
	for _, id := range jobIDs {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx, `UPDATE sla_jobs SET due_at = ?
		WHERE lease_owner = ? AND state = 'pending' AND id IN (`+placeholders+`) RETURNING id`, args...)
	if err != nil {
		return nil, fmt.Errorf("renew SLA leases: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var renewed []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan renewed SLA lease: %w", err)
		}
		renewed = append(renewed, id)
	}
	return renewed, rows.Err()
}

// ListFailedJobs returns parked jobs for diagnostics.
func (r *SLARepository) ListFailedJobs(ctx context.Context, limit int) ([]models.SLAJob, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+jobColumns+` FROM sla_jobs WHERE state = 'failed' ORDER BY due_at LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list failed SLA jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var jobs []models.SLAJob
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("scan failed job: %w", err)
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// ReplayJob resets a failed job to pending, due now.
func (r *SLARepository) ReplayJob(ctx context.Context, jobID int64, now time.Time) error {
	result, err := r.db.ExecContext(ctx, `UPDATE sla_jobs SET state = 'pending', due_at = ?, attempts = 0, lease_owner = NULL, last_error = NULL WHERE id = ? AND state = 'failed'`, now, jobID)
	if err != nil {
		return fmt.Errorf("replay SLA job: %w", err)
	}
	return requireAffected(result)
}

// CountPendingJobs returns the number of pending jobs, used by metrics.
func (r *SLARepository) CountPendingJobs(ctx context.Context) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sla_jobs WHERE state = 'pending'`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count pending SLA jobs: %w", err)
	}
	return count, nil
}

// CountFailedJobs returns the number of failed jobs, used by metrics.
func (r *SLARepository) CountFailedJobs(ctx context.Context) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sla_jobs WHERE state = 'failed'`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count failed SLA jobs: %w", err)
	}
	return count, nil
}

// HasConfiguration reports whether any workspace has active SLA configuration.
// The due-work loop uses it to decide between blocking on a nudge (zero
// queries) and the periodic safety re-read.
func (r *SLARepository) HasConfiguration(ctx context.Context) (bool, error) {
	var exists bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sla_workspace_state)`).Scan(&exists); err != nil {
		return false, fmt.Errorf("check SLA configuration: %w", err)
	}
	return exists, nil
}

// SLAReport aggregates completed cycles for a workspace. from/to are optional
// half-open bounds on the cycle stop time. Current-state segmentation is
// intentionally over stored columns only.
func (r *SLARepository) SLAReport(ctx context.Context, workspaceID int, from, to *time.Time) (*models.SLAReport, error) {
	report := &models.SLAReport{From: from, To: to}

	summaryQuery := `SELECT m.id, m.name,
		(SELECT COUNT(*) FROM item_sla_cycles c2 WHERE c2.metric_id = m.id AND c2.status = 'ongoing'),
		(SELECT COUNT(*) FROM item_sla_cycles c2 WHERE c2.metric_id = m.id AND c2.status = 'ongoing'
			AND (c2.breached_at IS NOT NULL OR c2.next_deadline_at <= ?)),
		COUNT(c.id),
		COALESCE(SUM(CASE WHEN c.breached_at IS NOT NULL THEN 1 ELSE 0 END), 0),
		AVG(c.elapsed_ms), AVG(c.goal_duration_ms), MAX(c.elapsed_ms)
	FROM sla_metrics m
	LEFT JOIN item_sla_cycles c ON c.metric_id = m.id AND c.status = 'completed' AND c.goal_duration_ms > 0`
	// Argument order follows the SQL text: evaluation time, date bounds, then
	// the workspace predicate.
	summaryArgs := []any{time.Now().UTC()}
	if from != nil {
		summaryQuery += ` AND c.stopped_at >= ?`
		summaryArgs = append(summaryArgs, *from)
	}
	if to != nil {
		summaryQuery += ` AND c.stopped_at < ?`
		summaryArgs = append(summaryArgs, *to)
	}
	summaryQuery += ` WHERE m.workspace_id = ? GROUP BY m.id, m.name ORDER BY m.position, m.id`
	summaryArgs = append(summaryArgs, workspaceID)

	rows, err := r.db.QueryContext(ctx, summaryQuery, summaryArgs...)
	if err != nil {
		return nil, fmt.Errorf("SLA report summary: %w", err)
	}
	for rows.Next() {
		var metric models.SLAReportMetric
		var avgElapsed, avgGoal, maxElapsed sql.NullFloat64
		if err := rows.Scan(&metric.MetricID, &metric.MetricName, &metric.Ongoing, &metric.CurrentlyBreached,
			&metric.Completed, &metric.Breached, &avgElapsed, &avgGoal, &maxElapsed); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan SLA report metric: %w", err)
		}
		metric.AvgElapsedMs = floatToMs(avgElapsed)
		metric.AvgGoalMs = floatToMs(avgGoal)
		metric.MaxElapsedMs = floatToMs(maxElapsed)
		report.Metrics = append(report.Metrics, metric)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	breachQuery := `SELECT c.id, c.item_id, w.key, i.workspace_item_number, i.title, m.name, COALESCE(st.name, ''),
		c.stopped_at, c.breached_at, c.elapsed_ms, c.goal_duration_ms
	FROM item_sla_cycles c
	JOIN items i ON i.id = c.item_id
	JOIN workspaces w ON w.id = i.workspace_id
	JOIN sla_metrics m ON m.id = c.metric_id
	LEFT JOIN statuses st ON st.id = i.status_id
	WHERE m.workspace_id = ? AND c.status = 'completed' AND c.breached_at IS NOT NULL`
	breachArgs := []any{workspaceID}
	if from != nil {
		breachQuery += ` AND c.stopped_at >= ?`
		breachArgs = append(breachArgs, *from)
	}
	if to != nil {
		breachQuery += ` AND c.stopped_at < ?`
		breachArgs = append(breachArgs, *to)
	}
	breachQuery += ` ORDER BY c.stopped_at DESC LIMIT 200`

	breachRows, err := r.db.QueryContext(ctx, breachQuery, breachArgs...)
	if err != nil {
		return nil, fmt.Errorf("SLA report breaches: %w", err)
	}
	defer func() { _ = breachRows.Close() }()
	for breachRows.Next() {
		var breach models.SLAReportBreach
		var workspaceKey string
		var itemNumber int
		var stoppedAt, breachedAt sql.NullTime
		if err := breachRows.Scan(&breach.CycleID, &breach.ItemID, &workspaceKey, &itemNumber, &breach.Title,
			&breach.MetricName, &breach.StatusName, &stoppedAt, &breachedAt, &breach.ElapsedMs, &breach.GoalDurationMs); err != nil {
			return nil, fmt.Errorf("scan SLA report breach: %w", err)
		}
		if stoppedAt.Valid {
			breach.StoppedAt = &stoppedAt.Time
		}
		if breachedAt.Valid {
			breach.BreachedAt = &breachedAt.Time
		}
		breach.ItemKey = fmt.Sprintf("%s-%d", workspaceKey, itemNumber)
		report.BreachedItems = append(report.BreachedItems, breach)
	}
	return report, breachRows.Err()
}

func floatToMs(value sql.NullFloat64) *int64 {
	if !value.Valid {
		return nil
	}
	result := int64(value.Float64)
	return &result
}

// ListRecalculationJobs returns pending recalculation work for a workspace.
func (r *SLARepository) ListRecalculationJobs(ctx context.Context, workspaceID int) ([]models.SLAJob, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+jobColumnsPrefixed+` FROM sla_jobs j
		LEFT JOIN sla_metrics m ON m.id = j.metric_id
		LEFT JOIN items i ON i.id = j.item_id
		WHERE j.kind IN ('recalc_item', 'recalc_metric', 'recalc_calendar')
		  AND (m.workspace_id = ? OR i.workspace_id = ?)
		ORDER BY j.due_at`, workspaceID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list recalculations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var jobs []models.SLAJob
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("scan recalc job: %w", err)
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// ListWarningThresholds returns a workspace's active and inactive warning
// thresholds, scoped to the whole workspace or a single metric.
func (r *SLARepository) ListWarningThresholds(ctx context.Context, workspaceID int) ([]models.SLAWarningThreshold, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT t.id, t.workspace_id, t.metric_id, t.label, t.percent, t.is_active,
		t.created_at, t.updated_at, COALESCE(m.name, '')
		FROM sla_warning_thresholds t
		LEFT JOIN sla_metrics m ON m.id = t.metric_id
		WHERE t.workspace_id = ?
		ORDER BY t.percent, t.id`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list SLA warning thresholds: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var thresholds []models.SLAWarningThreshold
	for rows.Next() {
		threshold, err := scanWarningThreshold(rows)
		if err != nil {
			return nil, fmt.Errorf("scan SLA warning threshold: %w", err)
		}
		thresholds = append(thresholds, threshold)
	}
	return thresholds, rows.Err()
}

// GetWarningThreshold loads one warning threshold.
func (r *SLARepository) GetWarningThreshold(ctx context.Context, id int) (*models.SLAWarningThreshold, error) {
	row := r.db.QueryRowContext(ctx, `SELECT t.id, t.workspace_id, t.metric_id, t.label, t.percent, t.is_active,
		t.created_at, t.updated_at, COALESCE(m.name, '')
		FROM sla_warning_thresholds t
		LEFT JOIN sla_metrics m ON m.id = t.metric_id
		WHERE t.id = ?`, id)
	threshold, err := scanWarningThreshold(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load SLA warning threshold: %w", err)
	}
	return &threshold, nil
}

// CreateWarningThreshold inserts a threshold in the caller's transaction.
func (r *SLARepository) CreateWarningThreshold(ctx context.Context, tx database.Tx, threshold *models.SLAWarningThreshold) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx, `INSERT INTO sla_warning_thresholds (workspace_id, metric_id, label, percent, is_active)
		VALUES (?, ?, ?, ?, ?) RETURNING id`,
		threshold.WorkspaceID, nullableInt(threshold.MetricID), threshold.Label, threshold.Percent, threshold.IsActive).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create SLA warning threshold: %w", err)
	}
	return id, nil
}

// UpdateWarningThreshold rewrites a threshold in the caller's transaction.
func (r *SLARepository) UpdateWarningThreshold(ctx context.Context, tx database.Tx, threshold *models.SLAWarningThreshold) error {
	result, err := tx.ExecContext(ctx, `UPDATE sla_warning_thresholds
		SET metric_id = ?, label = ?, percent = ?, is_active = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND workspace_id = ?`,
		nullableInt(threshold.MetricID), threshold.Label, threshold.Percent, threshold.IsActive, threshold.ID, threshold.WorkspaceID)
	if err != nil {
		return fmt.Errorf("update SLA warning threshold: %w", err)
	}
	return requireAffected(result)
}

// DeleteWarningThreshold removes a threshold in the caller's transaction.
func (r *SLARepository) DeleteWarningThreshold(ctx context.Context, tx database.Tx, id int) error {
	result, err := tx.ExecContext(ctx, `DELETE FROM sla_warning_thresholds WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete SLA warning threshold: %w", err)
	}
	return requireAffected(result)
}

// DeleteJobsForCycleKind removes the pending jobs of one kind for a cycle.
func (r *SLARepository) DeleteJobsForCycleKind(ctx context.Context, tx database.Tx, cycleID int64, kind string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM sla_jobs WHERE cycle_id = ? AND kind = ?`, cycleID, kind)
	if err != nil {
		return fmt.Errorf("delete %s jobs for cycle: %w", kind, err)
	}
	return nil
}

func scanWarningThreshold(row rowScanner) (models.SLAWarningThreshold, error) {
	var threshold models.SLAWarningThreshold
	var metricID sql.NullInt64
	if err := row.Scan(&threshold.ID, &threshold.WorkspaceID, &metricID, &threshold.Label, &threshold.Percent,
		&threshold.IsActive, &threshold.CreatedAt, &threshold.UpdatedAt, &threshold.MetricName); err != nil {
		return models.SLAWarningThreshold{}, err
	}
	if metricID.Valid {
		value := int(metricID.Int64)
		threshold.MetricID = &value
	}
	return threshold, nil
}

// ThresholdFired reports whether a warning threshold already fired for a cycle.
func (r *SLARepository) ThresholdFired(ctx context.Context, tx database.Tx, cycleID int64, thresholdKey string) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM item_sla_cycle_thresholds WHERE cycle_id = ? AND threshold_key = ?`, cycleID, thresholdKey).Scan(&count); err != nil {
		return false, fmt.Errorf("read cycle threshold: %w", err)
	}
	return count > 0, nil
}

// MarkThresholdFired records a warning threshold as fired. A duplicate is a
// no-op, which makes emission exactly-once per cycle and threshold.
func (r *SLARepository) MarkThresholdFired(ctx context.Context, tx database.Tx, cycleID int64, thresholdKey string, firedAt time.Time) (bool, error) {
	result, err := tx.ExecContext(ctx, `INSERT INTO item_sla_cycle_thresholds (cycle_id, threshold_key, fired_at)
		VALUES (?, ?, ?) ON CONFLICT (cycle_id, threshold_key) DO NOTHING`, cycleID, thresholdKey, firedAt)
	if err != nil {
		return false, fmt.Errorf("mark cycle threshold: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read threshold rows affected: %w", err)
	}
	return affected == 1, nil
}

// ---------------------------------------------------------------------------
// Calendar impact preview
// ---------------------------------------------------------------------------

// CalendarImpact returns, for every workspace that can use the calendar, the
// goal targets referencing it and the count of ongoing cycles scheduled
// against it. Workspaces owned by a team calendar's bound teams are included
// even when they currently have no references.
func (r *SLARepository) CalendarImpact(ctx context.Context, calendarID int) ([]models.SLACalendarImpactWorkspace, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT w.id, w.name
		FROM workspaces w
		WHERE w.id IN (
			SELECT wc.workspace_id FROM working_calendars wc WHERE wc.id = ? AND wc.workspace_id IS NOT NULL
			UNION
			SELECT b.workspace_id FROM team_workspace_bindings b
			JOIN working_calendars wc ON wc.team_id = b.team_id
			WHERE wc.id = ?
		)
		ORDER BY w.name, w.id`, calendarID, calendarID)
	if err != nil {
		return nil, fmt.Errorf("load calendar impact workspaces: %w", err)
	}
	index := map[int]int{}
	var workspaces []models.SLACalendarImpactWorkspace
	for rows.Next() {
		var workspace models.SLACalendarImpactWorkspace
		if err := rows.Scan(&workspace.WorkspaceID, &workspace.WorkspaceName); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan impact workspace: %w", err)
		}
		index[workspace.WorkspaceID] = len(workspaces)
		workspaces = append(workspaces, workspace)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	targetRows, err := r.db.QueryContext(ctx, `
		SELECT m.workspace_id, t.id, t.goal_id, g.metric_id, m.name, t.target_ms
		FROM sla_goal_targets t
		JOIN sla_goals g ON g.id = t.goal_id
		JOIN sla_metrics m ON m.id = g.metric_id
		WHERE t.calendar_id = ?
		ORDER BY m.workspace_id, m.position, g.position, t.position, t.id`, calendarID)
	if err != nil {
		return nil, fmt.Errorf("load calendar impact targets: %w", err)
	}
	for targetRows.Next() {
		var workspaceID int
		var target models.SLACalendarImpactTarget
		if err := targetRows.Scan(&workspaceID, &target.TargetID, &target.GoalID, &target.MetricID, &target.MetricName, &target.TargetMs); err != nil {
			_ = targetRows.Close()
			return nil, fmt.Errorf("scan impact target: %w", err)
		}
		if idx, ok := index[workspaceID]; ok {
			workspaces[idx].GoalTargets = append(workspaces[idx].GoalTargets, target)
		}
	}
	if err := targetRows.Err(); err != nil {
		_ = targetRows.Close()
		return nil, err
	}
	if err := targetRows.Close(); err != nil {
		return nil, err
	}

	countRows, err := r.db.QueryContext(ctx, `
		SELECT m.workspace_id, COUNT(*)
		FROM item_sla_cycles c
		JOIN sla_metrics m ON m.id = c.metric_id
		WHERE c.calendar_id = ? AND c.status = 'ongoing'
		GROUP BY m.workspace_id`, calendarID)
	if err != nil {
		return nil, fmt.Errorf("load calendar impact cycles: %w", err)
	}
	defer func() { _ = countRows.Close() }()
	for countRows.Next() {
		var workspaceID, count int
		if err := countRows.Scan(&workspaceID, &count); err != nil {
			return nil, fmt.Errorf("scan impact cycle count: %w", err)
		}
		if idx, ok := index[workspaceID]; ok {
			workspaces[idx].OngoingCycles = count
		}
	}
	if err := countRows.Err(); err != nil {
		return nil, err
	}
	return workspaces, nil
}

// Helpers
// ---------------------------------------------------------------------------

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func requireAffected(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read rows affected: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
