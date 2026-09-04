package services

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"windshift/internal/logger"
	"windshift/internal/repository"
)

var (
	// ErrIterationCompletionRequired prevents generic CRUD paths from skipping
	// the atomic completion workflow that moves incomplete work and records its
	// history.
	ErrIterationCompletionRequired = errors.New("iteration must be completed through the completion endpoint")
	// ErrIterationLifecycleConflict protects terminal iteration states from
	// being reopened through an ordinary metadata update.
	ErrIterationLifecycleConflict = errors.New("iteration status transition is not allowed")
)

// iterationScanner is satisfied by both *sql.Row and *sql.Rows.
type iterationScanner interface {
	Scan(dest ...any) error
}

// parseDate tries date-only format first, then falls back to RFC3339.
func parseDate(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

// scanIterationRow scans a single iteration row (with LEFT JOIN type/workspace
// columns) into an IterationResult. The column order must match the standard
// iteration query.
func scanIterationRow(sc iterationScanner) (IterationResult, error) {
	var iter IterationResult
	var description, typeName, typeColor, workspaceName sql.NullString
	var typeID, workspaceID sql.NullInt64

	err := sc.Scan(&iter.ID, &iter.Name, &description, &iter.StartDate, &iter.EndDate, &iter.Status,
		&typeID, &typeName, &typeColor, &iter.IsGlobal, &workspaceID, &workspaceName,
		&iter.CreatedAt, &iter.UpdatedAt)
	if err != nil {
		return iter, err
	}

	iter.Description = description.String
	if start, parseErr := parseDate(iter.StartDate); parseErr == nil {
		iter.StartDate = start.Format("2006-01-02")
	}
	if end, parseErr := parseDate(iter.EndDate); parseErr == nil {
		iter.EndDate = end.Format("2006-01-02")
	}
	iter.TypeName = typeName.String
	iter.TypeColor = typeColor.String
	iter.WorkspaceName = workspaceName.String
	if typeID.Valid {
		id := int(typeID.Int64)
		iter.TypeID = &id
	}
	if workspaceID.Valid {
		id := int(workspaceID.Int64)
		iter.WorkspaceID = &id
	}

	return iter, nil
}

// scanIterations scans all rows from an iteration query into a slice.
func scanIterations(rows *sql.Rows) ([]IterationResult, error) { //nolint:unparam // error is always nil but kept for consistency with scan pattern
	var iterations []IterationResult
	for rows.Next() {
		iter, err := scanIterationRow(rows)
		if err != nil {
			slog.Error("failed to scan iteration row", slog.Any("error", err))
			continue
		}
		iterations = append(iterations, iter)
	}
	if iterations == nil {
		iterations = []IterationResult{}
	}
	return iterations, nil
}

// IterationResult represents an iteration with type details.
type IterationResult struct {
	ID            int
	Name          string
	Description   string
	StartDate     string
	EndDate       string
	Status        string
	TypeID        *int
	TypeName      string
	TypeColor     string
	IsGlobal      bool
	WorkspaceID   *int
	WorkspaceName string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// IterationListParams contains parameters for listing iterations.
type IterationListParams struct {
	Limit         int
	Offset        int
	WorkspaceID   *int   // Filter by workspace
	WorkspaceIDs  []int  // Caller-visible local workspaces for an unscoped list
	TypeID        *int   // Filter by type
	Status        string // Filter by status
	IncludeGlobal bool   // Include global iterations
	SortBy        string
	SortOrder     string
}

// FindIterationByName returns a local workspace iteration by exact name.
func (s *PlanningService) FindIterationByName(workspaceID int, name string) (*IterationResult, error) {
	var id int
	err := s.db.QueryRow(`
		SELECT id FROM iterations
		WHERE workspace_id = ? AND is_global = false AND name = ?
	`, workspaceID, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find iteration by name: %w", err)
	}
	return s.GetIteration(id)
}

// ListIterations retrieves iterations with pagination and filtering.
func (s *PlanningService) ListIterations(params IterationListParams) ([]IterationResult, int, error) {
	list := newPlanningListQuery(iterationSelectQuery+"\nWHERE 1=1", "SELECT COUNT(*) FROM iterations i WHERE 1=1")
	list.addWorkspaceScope("i.workspace_id", "i.is_global", params.WorkspaceID, params.WorkspaceIDs, params.IncludeGlobal)
	list.addNullableIDFilter("i.type_id", params.TypeID)
	list.addStringFilter("i.status", params.Status)
	query, args := list.paginated(iterationOrderByClause(params.SortBy, params.SortOrder), params.Limit, params.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list iterations: %w", err)
	}
	defer rows.Close()

	iterations, _ := scanIterations(rows)

	var total int
	if err := s.db.QueryRow(list.countQuery, list.countArgs...).Scan(&total); err != nil {
		slog.Warn("failed to get iteration pagination count", slog.Any("error", err))
	}

	return iterations, total, nil
}

func iterationOrderByClause(sortBy, sortOrder string) string {
	direction := "ASC"
	if strings.EqualFold(sortOrder, "desc") {
		direction = "DESC"
	}
	switch sortBy {
	case "start_date", "end_date", "name", "status", "created_at", "updated_at":
		return " ORDER BY i." + sortBy + " " + direction + ", i.name ASC, i.id ASC"
	default:
		return " ORDER BY i.start_date DESC, i.name ASC, i.id ASC"
	}
}

// GetIteration retrieves an iteration by ID.
func (s *PlanningService) GetIteration(id int) (*IterationResult, error) {
	row := s.db.QueryRow(iterationSelectQuery+"\nWHERE i.id = ?", id)

	iter, err := scanIterationRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("iteration not found: %d: %w", id, repository.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get iteration: %w", err)
	}

	return &iter, nil
}

// FindIterationIDByName resolves an iteration by exact (case-insensitive)
// name, preferring the workspace's own iteration over a same-named global one.
// Returns nil when nothing matches.
func (s *PlanningService) FindIterationIDByName(workspaceID int, name string) (*int, error) {
	var id int
	err := s.db.QueryRow(`
		SELECT id FROM iterations
		WHERE LOWER(name) = LOWER(?) AND (workspace_id = ? OR is_global = true)
		ORDER BY CASE WHEN workspace_id = ? THEN 0 ELSE 1 END
		LIMIT 1
	`, name, workspaceID, workspaceID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve iteration by name: %w", err)
	}
	return &id, nil
}

// IsIterationGlobal checks if an iteration is global and returns its workspace_id.
func (s *PlanningService) IsIterationGlobal(id int) (isGlobal bool, workspaceID *int, err error) {
	var wsID sql.NullInt64
	err = s.db.QueryRow("SELECT is_global, workspace_id FROM iterations WHERE id = ?", id).Scan(&isGlobal, &wsID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil, fmt.Errorf("iteration not found: %d: %w", id, repository.ErrNotFound)
	}
	if err != nil {
		return false, nil, fmt.Errorf("failed to check iteration: %w", err)
	}
	if wsID.Valid {
		wid := int(wsID.Int64)
		workspaceID = &wid
	}
	return isGlobal, workspaceID, nil
}

// CreateIterationParams contains parameters for creating an iteration.
type CreateIterationParams struct {
	Name        string
	Description string
	StartDate   string
	EndDate     string
	Status      string
	TypeID      *int
	IsGlobal    bool
	WorkspaceID *int
	AuditActor  *AuditActor
}

// CreateIteration creates a new iteration.
func (s *PlanningService) CreateIteration(params CreateIterationParams) (*IterationResult, error) {
	if params.Status == "" {
		params.Status = "planned"
	}
	if err := s.validateIterationMutation(params); err != nil {
		return nil, err
	}

	var id int64
	err := s.db.QueryRow(`
		INSERT INTO iterations (name, description, start_date, end_date, status, type_id, is_global, workspace_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, params.Name, params.Description, params.StartDate, params.EndDate, params.Status, params.TypeID, params.IsGlobal, params.WorkspaceID).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to create iteration: %w", err)
	}

	created, err := s.GetIteration(int(id))
	if err != nil {
		return nil, err
	}
	if params.AuditActor != nil {
		resourceID := created.ID
		emitServiceAudit(s.db, *params.AuditActor, logger.ActionIterationCreate, logger.ResourceIteration, &resourceID, created.Name, nil)
	}
	return created, nil
}

// UpdateIterationParams contains parameters for updating an iteration.
// WorkspaceID is the scope (nil = global) and is used in the WHERE clause to
// prevent cross-scope updates. is_global / workspace_id cannot be changed via
// this method.
type UpdateIterationParams struct {
	ID          int
	Name        string
	Description string
	StartDate   string
	EndDate     string
	Status      string
	TypeID      *int
	WorkspaceID *int // nil = global iteration
	AuditActor  *AuditActor
}

// UpdateIteration updates an existing iteration within its declared scope.
func (s *PlanningService) UpdateIteration(params UpdateIterationParams) (*IterationResult, error) {
	if err := s.validateIterationMutation(CreateIterationParams{
		Name:        params.Name,
		Description: params.Description,
		StartDate:   params.StartDate,
		EndDate:     params.EndDate,
		Status:      params.Status,
		TypeID:      params.TypeID,
		IsGlobal:    params.WorkspaceID == nil,
		WorkspaceID: params.WorkspaceID,
	}); err != nil {
		return nil, err
	}
	currentStatus, err := s.iterationStatusInScope(params.ID, params.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if err := validateIterationStatusTransition(currentStatus, params.Status); err != nil {
		return nil, err
	}

	var (
		res       sql.Result
		updateErr error
	)
	if params.WorkspaceID == nil {
		res, updateErr = s.db.ExecWrite(`
			UPDATE iterations SET name = ?, description = ?, start_date = ?, end_date = ?,
			       status = ?, type_id = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND is_global = true AND status = ?
		`, params.Name, params.Description, params.StartDate, params.EndDate, params.Status, params.TypeID, params.ID, currentStatus)
	} else {
		res, updateErr = s.db.ExecWrite(`
			UPDATE iterations SET name = ?, description = ?, start_date = ?, end_date = ?,
			       status = ?, type_id = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND workspace_id = ? AND is_global = false AND status = ?
		`, params.Name, params.Description, params.StartDate, params.EndDate, params.Status, params.TypeID, params.ID, *params.WorkspaceID, currentStatus)
	}
	if updateErr != nil {
		return nil, fmt.Errorf("failed to update iteration: %w", updateErr)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to read update result: %w", err)
	}
	if n == 0 {
		// The scoped row existed when its lifecycle was checked. A zero-row
		// conditional update means another writer changed the state meanwhile;
		// never overwrite that transition with stale data.
		return nil, ErrIterationLifecycleConflict
	}
	updated, err := s.GetIteration(params.ID)
	if err != nil {
		return nil, err
	}
	if params.AuditActor != nil {
		resourceID := updated.ID
		emitServiceAudit(s.db, *params.AuditActor, logger.ActionIterationUpdate, logger.ResourceIteration, &resourceID, updated.Name, nil)
	}
	return updated, nil
}

func (s *PlanningService) iterationStatusInScope(id int, workspaceID *int) (string, error) {
	var (
		status string
		err    error
	)
	if workspaceID == nil {
		err = s.db.QueryRow("SELECT status FROM iterations WHERE id = ? AND is_global = true", id).Scan(&status)
	} else {
		err = s.db.QueryRow("SELECT status FROM iterations WHERE id = ? AND workspace_id = ? AND is_global = false", id, *workspaceID).Scan(&status)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("iteration not found: %d: %w", id, repository.ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("failed to load iteration status: %w", err)
	}
	return status, nil
}

func validateIterationStatusTransition(current, next string) error {
	valid := func(status string) bool {
		switch status {
		case "planned", "active", "completed", iterationStatusCancelled:
			return true
		default:
			return false
		}
	}
	if !valid(current) || !valid(next) {
		return fmt.Errorf("%w: %q to %q", ErrIterationLifecycleConflict, current, next)
	}
	if next == "completed" && current != "completed" {
		return ErrIterationCompletionRequired
	}
	if (current == "completed" || current == iterationStatusCancelled) && next != current {
		return fmt.Errorf("%w: terminal iteration cannot be reopened", ErrIterationLifecycleConflict)
	}
	return nil
}

// DeleteIteration deletes an iteration and optionally records the user-driven
// mutation once for any transport that supplied an actor.
func (s *PlanningService) DeleteIteration(id int, auditActors ...AuditActor) error {
	var resourceName string
	if len(auditActors) > 0 {
		existing, err := s.GetIteration(id)
		if err != nil {
			return err
		}
		resourceName = existing.Name
	}
	_, err := s.db.ExecWrite("DELETE FROM iterations WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete iteration: %w", err)
	}
	if actor := optionalAuditActor(auditActors); actor != nil {
		emitServiceAudit(s.db, *actor, logger.ActionIterationDelete, logger.ResourceIteration, &id, resourceName, nil)
	}
	return nil
}

// IterationProgressReport represents the full iteration progress data.
type IterationProgressReport struct {
	IterationID     int                       `json:"iteration_id"`
	IterationName   string                    `json:"iteration_name"`
	Description     string                    `json:"description,omitempty"`
	StartDate       string                    `json:"start_date"`
	EndDate         string                    `json:"end_date"`
	Status          string                    `json:"status"`
	TypeColor       string                    `json:"type_color,omitempty"`
	TotalItems      int                       `json:"total_items"`
	CompletedItems  int                       `json:"completed_items"`
	PercentComplete float64                   `json:"percent_complete"`
	StatusBreakdown []StatusBreakdown         `json:"status_breakdown"`
	ItemsByCategory map[string][]ProgressItem `json:"items_by_category"`
}

// GetIterationProgress retrieves progress report for an iteration.
func (s *PlanningService) GetIterationProgress(iterationID int, workspaceIDs []int) (*IterationProgressReport, error) {
	var report IterationProgressReport
	report.IterationID = iterationID
	// Get iteration details
	var description, typeColor sql.NullString
	err := s.db.QueryRow(`
		SELECT i.name, i.description, i.start_date, i.end_date, i.status, it.color
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		WHERE i.id = ?
	`, iterationID).Scan(&report.IterationName, &description, &report.StartDate, &report.EndDate, &report.Status, &typeColor)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("iteration not found: %d: %w", iterationID, repository.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get iteration: %w", err)
	}

	report.Description = description.String
	report.TypeColor = typeColor.String

	// Get status breakdown and items grouped by status category
	acc, err := s.buildProgressReport(repository.ItemFilters{IterationID: &iterationID}, workspaceIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get iteration progress: %w", err)
	}

	report.TotalItems = acc.TotalItems
	report.CompletedItems = acc.CompletedItems
	report.PercentComplete = acc.PercentComplete
	report.StatusBreakdown = acc.StatusBreakdown
	report.ItemsByCategory = acc.ItemsByCategory

	return &report, nil
}

// BurndownDataPoint represents a single day's burndown data.
type BurndownDataPoint struct {
	Date      string `json:"date"`
	Remaining int    `json:"remaining"`
	Completed int    `json:"completed"`
	Ideal     int    `json:"ideal"`
}

// IterationBurndownData represents the full burndown chart data.
type IterationBurndownData struct {
	IterationID int                 `json:"iteration_id"`
	StartDate   string              `json:"start_date"`
	EndDate     string              `json:"end_date"`
	TotalItems  int                 `json:"total_items"`
	DataPoints  []BurndownDataPoint `json:"data_points"`
}

// GetIterationBurndown reconstructs daily membership and status from history.
// TotalItems is the number of distinct visible items that belonged to the
// iteration on at least one reported day. The ideal line is based on the
// end-of-first-day commitment and is intentionally unaffected by later scope
// additions or removals, making scope change visible instead of rewriting the
// original plan.
func (s *PlanningService) GetIterationBurndown(iterationID int, workspaceIDs []int) (*IterationBurndownData, error) {
	iter, err := s.GetIteration(iterationID)
	if err != nil {
		return nil, err
	}
	startDate, err := parseDate(iter.StartDate)
	if err != nil {
		return nil, fmt.Errorf("invalid start date: %w", err)
	}
	endDate, err := parseDate(iter.EndDate)
	if err != nil {
		return nil, fmt.Errorf("invalid end date: %w", err)
	}

	type historicalItemState struct {
		createdAt   time.Time
		iterationID sql.NullInt64
		statusID    sql.NullInt64
	}
	workspaceClause, workspaceArgs := planningWorkspaceFilter("i.workspace_id", workspaceIDs)
	iterationValue := fmt.Sprintf("%d", iterationID)
	itemArgs := make([]any, 0, 3+len(workspaceArgs))
	itemArgs = append(itemArgs, iterationID, iterationValue, iterationValue)
	itemArgs = append(itemArgs, workspaceArgs...)
	rows, err := s.db.Query(`
		SELECT i.id, i.iteration_id, i.status_id, i.created_at
		FROM items i
		WHERE (
			i.iteration_id = ?
			OR EXISTS (
				SELECT 1 FROM item_history ih
				WHERE ih.item_id = i.id
				  AND ih.field_name = 'iteration_id'
				  AND (ih.old_value = ? OR ih.new_value = ?)
			)
		)`+workspaceClause+`
	`, itemArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get historical iteration items: %w", err)
	}
	defer rows.Close()

	itemStates := make(map[int]*historicalItemState)
	for rows.Next() {
		var (
			itemID       int
			createdValue any
			state        historicalItemState
		)
		if err := rows.Scan(&itemID, &state.iterationID, &state.statusID, &createdValue); err != nil {
			return nil, fmt.Errorf("failed to scan historical iteration item: %w", err)
		}
		createdAt, ok := analyticsDBTime(createdValue)
		if !ok {
			return nil, fmt.Errorf("failed to parse creation time for item %d", itemID)
		}
		state.createdAt = createdAt
		itemStates[itemID] = &state
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate historical iteration items: %w", err)
	}
	if len(itemStates) == 0 {
		return &IterationBurndownData{
			IterationID: iterationID,
			StartDate:   iter.StartDate,
			EndDate:     iter.EndDate,
			DataPoints:  []BurndownDataPoint{},
		}, nil
	}

	type historyChange struct {
		itemID    int
		changedAt time.Time
		fieldName string
		oldValue  sql.NullString
		newValue  sql.NullString
	}
	historyArgs := make([]any, 0, 4+len(workspaceArgs))
	historyArgs = append(historyArgs, startDate, iterationID, iterationValue, iterationValue)
	historyArgs = append(historyArgs, workspaceArgs...)
	historyRows, err := s.db.Query(`
		SELECT ih.item_id, ih.changed_at, ih.field_name, ih.old_value, ih.new_value
		FROM item_history ih
		JOIN items i ON i.id = ih.item_id
		WHERE ih.field_name IN ('iteration_id', 'status_id')
		  AND ih.changed_at >= ?
		  AND (
			i.iteration_id = ?
			OR EXISTS (
				SELECT 1 FROM item_history membership
				WHERE membership.item_id = i.id
				  AND membership.field_name = 'iteration_id'
				  AND (membership.old_value = ? OR membership.new_value = ?)
			)
		  )`+workspaceClause+`
		ORDER BY ih.changed_at ASC, ih.id ASC
	`, historyArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to get iteration membership history: %w", err)
	}
	defer historyRows.Close()

	var changes []historyChange
	for historyRows.Next() {
		var change historyChange
		var changedValue any
		if err := historyRows.Scan(
			&change.itemID, &changedValue, &change.fieldName, &change.oldValue, &change.newValue,
		); err != nil {
			return nil, fmt.Errorf("failed to scan iteration history: %w", err)
		}
		changedAt, ok := analyticsDBTime(changedValue)
		if !ok {
			return nil, fmt.Errorf("failed to parse iteration history time for item %d", change.itemID)
		}
		change.changedAt = changedAt
		changes = append(changes, change)
	}
	if err := historyRows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate iteration history: %w", err)
	}

	parseNullableID := func(value sql.NullString) sql.NullInt64 {
		if !value.Valid || value.String == "" {
			return sql.NullInt64{}
		}
		var id int64
		if _, scanErr := fmt.Sscanf(value.String, "%d", &id); scanErr != nil {
			return sql.NullInt64{}
		}
		return sql.NullInt64{Int64: id, Valid: true}
	}
	applyValue := func(state *historicalItemState, fieldName string, value sql.NullString) {
		switch fieldName {
		case "iteration_id":
			state.iterationID = parseNullableID(value)
		case "status_id":
			state.statusID = parseNullableID(value)
		}
	}

	// Rewind current state to immediately before the iteration start. This
	// includes carry-over changes made after the iteration ended.
	for i := len(changes) - 1; i >= 0; i-- {
		if state := itemStates[changes[i].itemID]; state != nil {
			applyValue(state, changes[i].fieldName, changes[i].oldValue)
		}
	}

	statusCompleted := make(map[int64]bool)
	statusRows, err := s.db.Query(`
		SELECT st.id, COALESCE(sc.is_completed, false)
		FROM statuses st
		LEFT JOIN status_categories sc ON sc.id = st.category_id
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get burndown statuses: %w", err)
	}
	defer statusRows.Close()
	for statusRows.Next() {
		var statusID int64
		var completed bool
		if err := statusRows.Scan(&statusID, &completed); err != nil {
			return nil, fmt.Errorf("failed to scan burndown status: %w", err)
		}
		statusCompleted[statusID] = completed
	}
	if err := statusRows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate burndown statuses: %w", err)
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	effectiveEndDate := endDate
	if today.Before(effectiveEndDate) {
		effectiveEndDate = today
	}
	if effectiveEndDate.Before(startDate) {
		return &IterationBurndownData{
			IterationID: iterationID,
			StartDate:   iter.StartDate,
			EndDate:     iter.EndDate,
			DataPoints:  []BurndownDataPoint{},
		}, nil
	}

	var dataPoints []BurndownDataPoint
	everMembers := make(map[int]struct{})
	changeIndex := 0
	for day := startDate; !day.After(effectiveEndDate); day = day.AddDate(0, 0, 1) {
		dayEnd := day.AddDate(0, 0, 1)
		for changeIndex < len(changes) && changes[changeIndex].changedAt.Before(dayEnd) {
			change := changes[changeIndex]
			if state := itemStates[change.itemID]; state != nil {
				applyValue(state, change.fieldName, change.newValue)
			}
			changeIndex++
		}

		remaining, completed := 0, 0
		for itemID, state := range itemStates {
			if !state.createdAt.Before(dayEnd) ||
				!state.iterationID.Valid ||
				int(state.iterationID.Int64) != iterationID {
				continue
			}
			everMembers[itemID] = struct{}{}
			if state.statusID.Valid && statusCompleted[state.statusID.Int64] {
				completed++
			} else {
				remaining++
			}
		}
		dataPoints = append(dataPoints, BurndownDataPoint{
			Date:      day.Format("2006-01-02"),
			Remaining: remaining,
			Completed: completed,
		})
	}

	committedItems := 0
	if len(dataPoints) > 0 {
		committedItems = dataPoints[0].Remaining + dataPoints[0].Completed
	}
	totalDays := int(endDate.Sub(startDate).Hours()/24) + 1
	for i := range dataPoints {
		ideal := committedItems
		if totalDays > 1 {
			ideal = committedItems - (i * committedItems / (totalDays - 1))
			if ideal < 0 {
				ideal = 0
			}
		}
		dataPoints[i].Ideal = ideal
	}

	return &IterationBurndownData{
		IterationID: iterationID,
		StartDate:   iter.StartDate,
		EndDate:     iter.EndDate,
		TotalItems:  len(everMembers),
		DataPoints:  dataPoints,
	}, nil
}
