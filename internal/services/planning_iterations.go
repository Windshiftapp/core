package services

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

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
	TypeID        *int   // Filter by type
	Status        string // Filter by status
	IncludeGlobal bool   // Include global iterations
}

// ListIterations retrieves iterations with pagination and filtering.
func (s *PlanningService) ListIterations(params IterationListParams) ([]IterationResult, int, error) {
	query := `
		SELECT i.id, i.name, i.description, i.start_date, i.end_date, i.status,
		       i.type_id, it.name as type_name, it.color as type_color,
		       i.is_global, i.workspace_id, w.name as workspace_name,
		       i.created_at, i.updated_at
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		LEFT JOIN workspaces w ON i.workspace_id = w.id
		WHERE 1=1`

	countQuery := "SELECT COUNT(*) FROM iterations i WHERE 1=1"
	var args []interface{}
	var countArgs []interface{}

	// Filter by workspace - show local iterations for this workspace + optionally global iterations
	if params.WorkspaceID != nil {
		if params.IncludeGlobal {
			query += " AND (i.workspace_id = ? OR i.is_global = ?)"
			countQuery += " AND (i.workspace_id = ? OR i.is_global = ?)"
			args = append(args, *params.WorkspaceID, true)
			countArgs = append(countArgs, *params.WorkspaceID, true)
		} else {
			query += " AND i.workspace_id = ?"
			countQuery += " AND i.workspace_id = ?"
			args = append(args, *params.WorkspaceID)
			countArgs = append(countArgs, *params.WorkspaceID)
		}
	} else if params.IncludeGlobal {
		// If no workspace specified but include_global, only show global iterations
		query += " AND i.is_global = ?"
		countQuery += " AND i.is_global = ?"
		args = append(args, true)
		countArgs = append(countArgs, true)
	}

	// Filter by type
	if params.TypeID != nil {
		if *params.TypeID == 0 {
			query += " AND i.type_id IS NULL"
			countQuery += " AND i.type_id IS NULL"
		} else {
			query += " AND i.type_id = ?"
			countQuery += " AND i.type_id = ?"
			args = append(args, *params.TypeID)
			countArgs = append(countArgs, *params.TypeID)
		}
	}

	// Filter by status
	if params.Status != "" {
		query += " AND i.status = ?"
		countQuery += " AND i.status = ?"
		args = append(args, params.Status)
		countArgs = append(countArgs, params.Status)
	}

	query += " ORDER BY i.start_date DESC, i.name"
	query += " LIMIT ? OFFSET ?"
	args = append(args, params.Limit, params.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list iterations: %w", err)
	}
	defer rows.Close()

	var iterations []IterationResult
	for rows.Next() {
		var iter IterationResult
		var description, typeName, typeColor, workspaceName sql.NullString
		var typeID, workspaceID sql.NullInt64
		err := rows.Scan(&iter.ID, &iter.Name, &description, &iter.StartDate, &iter.EndDate, &iter.Status,
			&typeID, &typeName, &typeColor, &iter.IsGlobal, &workspaceID, &workspaceName,
			&iter.CreatedAt, &iter.UpdatedAt)
		if err != nil {
			continue
		}
		iter.Description = description.String
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
		iterations = append(iterations, iter)
	}

	if iterations == nil {
		iterations = []IterationResult{}
	}

	var total int
	if err := s.db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		slog.Warn("failed to get iteration pagination count", slog.Any("error", err))
	}

	return iterations, total, nil
}

// GetIteration retrieves an iteration by ID.
func (s *PlanningService) GetIteration(id int) (*IterationResult, error) {
	var iter IterationResult
	var description, typeName, typeColor, workspaceName sql.NullString
	var typeID, workspaceID sql.NullInt64
	err := s.db.QueryRow(`
		SELECT i.id, i.name, i.description, i.start_date, i.end_date, i.status,
		       i.type_id, it.name as type_name, it.color as type_color,
		       i.is_global, i.workspace_id, w.name as workspace_name,
		       i.created_at, i.updated_at
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		LEFT JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.id = ?
	`, id).Scan(&iter.ID, &iter.Name, &description, &iter.StartDate, &iter.EndDate, &iter.Status,
		&typeID, &typeName, &typeColor, &iter.IsGlobal, &workspaceID, &workspaceName,
		&iter.CreatedAt, &iter.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("iteration not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get iteration: %w", err)
	}

	iter.Description = description.String
	iter.TypeName = typeName.String
	iter.TypeColor = typeColor.String
	iter.WorkspaceName = workspaceName.String
	if typeID.Valid {
		tid := int(typeID.Int64)
		iter.TypeID = &tid
	}
	if workspaceID.Valid {
		wid := int(workspaceID.Int64)
		iter.WorkspaceID = &wid
	}

	return &iter, nil
}

// IsIterationGlobal checks if an iteration is global and returns its workspace_id.
func (s *PlanningService) IsIterationGlobal(id int) (isGlobal bool, workspaceID *int, err error) {
	var wsID sql.NullInt64
	err = s.db.QueryRow("SELECT is_global, workspace_id FROM iterations WHERE id = ?", id).Scan(&isGlobal, &wsID)
	if err == sql.ErrNoRows {
		return false, nil, fmt.Errorf("iteration not found: %d", id)
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
}

// CreateIteration creates a new iteration.
func (s *PlanningService) CreateIteration(params CreateIterationParams) (*IterationResult, error) {
	status := params.Status
	if status == "" {
		status = "planned"
	}

	var id int64
	err := s.db.QueryRow(`
		INSERT INTO iterations (name, description, start_date, end_date, status, type_id, is_global, workspace_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, params.Name, params.Description, params.StartDate, params.EndDate, status, params.TypeID, params.IsGlobal, params.WorkspaceID).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to create iteration: %w", err)
	}

	return s.GetIteration(int(id))
}

// UpdateIterationParams contains parameters for updating an iteration.
type UpdateIterationParams struct {
	ID          int
	Name        string
	Description string
	StartDate   string
	EndDate     string
	Status      string
	TypeID      *int
	IsGlobal    bool
	WorkspaceID *int
}

// UpdateIteration updates an existing iteration.
func (s *PlanningService) UpdateIteration(params UpdateIterationParams) (*IterationResult, error) {
	_, err := s.db.ExecWrite(`
		UPDATE iterations SET name = ?, description = ?, start_date = ?, end_date = ?,
		       status = ?, type_id = ?, is_global = ?, workspace_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, params.Name, params.Description, params.StartDate, params.EndDate, params.Status, params.TypeID, params.IsGlobal, params.WorkspaceID, params.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update iteration: %w", err)
	}

	return s.GetIteration(params.ID)
}

// DeleteIteration deletes an iteration.
func (s *PlanningService) DeleteIteration(id int) error {
	_, err := s.db.ExecWrite("DELETE FROM iterations WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete iteration: %w", err)
	}
	return nil
}

// IterationStatusBreakdown represents item counts by status category for an iteration.
type IterationStatusBreakdown struct {
	CategoryName  string `json:"category_name"`
	CategoryColor string `json:"category_color,omitempty"`
	ItemCount     int    `json:"item_count"`
	IsCompleted   bool   `json:"is_completed"`
}

// IterationProgressItem represents a work item in the iteration progress report.
type IterationProgressItem struct {
	ID             int    `json:"id"`
	Title          string `json:"title"`
	WorkspaceID    int    `json:"workspace_id"`
	WorkspaceKey   string `json:"workspace_key"`
	ItemNumber     int    `json:"item_number"`
	StatusName     string `json:"status_name,omitempty"`
	StatusColor    string `json:"status_color,omitempty"`
	PriorityName   string `json:"priority_name,omitempty"`
	PriorityColor  string `json:"priority_color,omitempty"`
	AssigneeName   string `json:"assignee_name,omitempty"`
	AssigneeAvatar string `json:"assignee_avatar,omitempty"`
}

// IterationProgressReport represents the full iteration progress data.
type IterationProgressReport struct {
	IterationID     int                                `json:"iteration_id"`
	IterationName   string                             `json:"iteration_name"`
	Description     string                             `json:"description,omitempty"`
	StartDate       string                             `json:"start_date"`
	EndDate         string                             `json:"end_date"`
	Status          string                             `json:"status"`
	TypeColor       string                             `json:"type_color,omitempty"`
	TotalItems      int                                `json:"total_items"`
	CompletedItems  int                                `json:"completed_items"`
	PercentComplete float64                            `json:"percent_complete"`
	StatusBreakdown []IterationStatusBreakdown         `json:"status_breakdown"`
	ItemsByCategory map[string][]IterationProgressItem `json:"items_by_category"`
}

// GetIterationProgress retrieves progress report for an iteration.
func (s *PlanningService) GetIterationProgress(iterationID int) (*IterationProgressReport, error) {
	var report IterationProgressReport
	report.IterationID = iterationID
	report.ItemsByCategory = make(map[string][]IterationProgressItem)

	// Get iteration details
	var description, typeColor sql.NullString
	err := s.db.QueryRow(`
		SELECT i.name, i.description, i.start_date, i.end_date, i.status, it.color
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		WHERE i.id = ?
	`, iterationID).Scan(&report.IterationName, &description, &report.StartDate, &report.EndDate, &report.Status, &typeColor)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("iteration not found: %d", iterationID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get iteration: %w", err)
	}

	report.Description = description.String
	report.TypeColor = typeColor.String

	// Get status breakdown and items grouped by status category
	rows, err := s.db.Query(`
		SELECT
			i.id, i.title, i.workspace_id, w.key as workspace_key, i.workspace_item_number,
			COALESCE(sc.name, 'No Status') as category_name,
			COALESCE(sc.color, '#9ca3af') as category_color,
			COALESCE(sc.is_completed, false) as is_completed,
			COALESCE(st.name, '') as status_name,
			COALESCE(sc.color, '') as status_color,
			COALESCE(p.name, '') as priority_name,
			COALESCE(p.color, '') as priority_color,
			COALESCE(u.first_name || ' ' || u.last_name, '') as assignee_name,
			COALESCE(u.avatar_url, '') as assignee_avatar
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN status_categories sc ON st.category_id = sc.id
		LEFT JOIN priorities p ON i.priority_id = p.id
		LEFT JOIN users u ON i.assignee_id = u.id
		WHERE i.iteration_id = ?
		ORDER BY sc.name, i.workspace_item_number
	`, iterationID)

	if err != nil {
		return nil, fmt.Errorf("failed to get iteration items: %w", err)
	}
	defer rows.Close()

	// Track status breakdown counts
	breakdownMap := make(map[string]*IterationStatusBreakdown)

	for rows.Next() {
		var item IterationProgressItem
		var categoryName string
		var categoryColorVal string
		var isCompleted bool
		var statusColor, priorityColor sql.NullString

		err := rows.Scan(
			&item.ID, &item.Title, &item.WorkspaceID, &item.WorkspaceKey, &item.ItemNumber,
			&categoryName, &categoryColorVal, &isCompleted,
			&item.StatusName, &statusColor,
			&item.PriorityName, &priorityColor,
			&item.AssigneeName, &item.AssigneeAvatar,
		)
		if err != nil {
			continue
		}

		item.StatusColor = statusColor.String
		item.PriorityColor = priorityColor.String

		// Update breakdown counts
		if _, exists := breakdownMap[categoryName]; !exists {
			breakdownMap[categoryName] = &IterationStatusBreakdown{
				CategoryName:  categoryName,
				CategoryColor: categoryColorVal,
				IsCompleted:   isCompleted,
				ItemCount:     0,
			}
		}
		breakdownMap[categoryName].ItemCount++

		// Add item to category group
		report.ItemsByCategory[categoryName] = append(report.ItemsByCategory[categoryName], item)

		// Update totals
		report.TotalItems++
		if isCompleted {
			report.CompletedItems++
		}
	}

	// Convert breakdown map to slice
	report.StatusBreakdown = make([]IterationStatusBreakdown, 0, len(breakdownMap))
	for _, breakdown := range breakdownMap {
		report.StatusBreakdown = append(report.StatusBreakdown, *breakdown)
	}

	// Calculate percentage
	if report.TotalItems > 0 {
		report.PercentComplete = float64(report.CompletedItems) / float64(report.TotalItems) * 100.0
	}

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

// GetIterationBurndown calculates burndown data for an iteration by replaying item history.
func (s *PlanningService) GetIterationBurndown(iterationID int) (*IterationBurndownData, error) {
	// Get iteration details
	iter, err := s.GetIteration(iterationID)
	if err != nil {
		return nil, err
	}

	// Parse dates
	startDate, err := parseDate(iter.StartDate)
	if err != nil {
		return nil, fmt.Errorf("invalid start date: %w", err)
	}
	endDate, err := parseDate(iter.EndDate)
	if err != nil {
		return nil, fmt.Errorf("invalid end date: %w", err)
	}

	// Get all items in this iteration with their current status category
	rows, err := s.db.Query(`
		SELECT i.id, COALESCE(sc.is_completed, false) as is_completed
		FROM items i
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN status_categories sc ON st.category_id = sc.id
		WHERE i.iteration_id = ?
	`, iterationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get iteration items: %w", err)
	}
	defer rows.Close()

	// Build map of item IDs to their current completed state
	itemStates := make(map[int]bool) // itemID -> isCompleted
	for rows.Next() {
		var itemID int
		var isCompleted bool
		if err = rows.Scan(&itemID, &isCompleted); err != nil {
			continue
		}
		itemStates[itemID] = isCompleted
	}

	totalItems := len(itemStates)
	if totalItems == 0 {
		// Return empty data if no items
		return &IterationBurndownData{
			IterationID: iterationID,
			StartDate:   iter.StartDate,
			EndDate:     iter.EndDate,
			TotalItems:  0,
			DataPoints:  []BurndownDataPoint{},
		}, nil
	}

	// Get all status changes for items in this iteration within the date range
	// We need to work backwards from current state using history
	historyRows, err := s.db.Query(`
		SELECT ih.item_id, ih.changed_at, ih.old_value, ih.new_value
		FROM item_history ih
		JOIN items i ON ih.item_id = i.id
		WHERE i.iteration_id = ?
		  AND ih.field_name = 'status_id'
		  AND ih.changed_at >= ?
		ORDER BY ih.changed_at DESC
	`, iterationID, startDate.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("failed to get item history: %w", err)
	}
	defer historyRows.Close()

	// Collect all status change events
	type statusChange struct {
		ItemID    int
		ChangedAt time.Time
		OldValue  sql.NullString
		NewValue  sql.NullString
	}
	var changes []statusChange

	for historyRows.Next() {
		var c statusChange
		var changedAtStr string
		if err = historyRows.Scan(&c.ItemID, &changedAtStr, &c.OldValue, &c.NewValue); err != nil {
			continue
		}
		// Parse the datetime
		c.ChangedAt, _ = time.Parse("2006-01-02 15:04:05", changedAtStr)
		if c.ChangedAt.IsZero() {
			c.ChangedAt, _ = time.Parse(time.RFC3339, changedAtStr)
		}
		changes = append(changes, c)
	}

	// Get status_id -> is_completed mapping
	statusCompletedMap := make(map[int]bool)
	statusRows, err := s.db.Query(`
		SELECT s.id, COALESCE(sc.is_completed, false)
		FROM statuses s
		LEFT JOIN status_categories sc ON s.category_id = sc.id
	`)
	if err == nil {
		defer statusRows.Close()
		for statusRows.Next() {
			var statusID int
			var isCompleted bool
			if err := statusRows.Scan(&statusID, &isCompleted); err == nil {
				statusCompletedMap[statusID] = isCompleted
			}
		}
	}

	// Helper to check if a status_id string represents a completed status
	isStatusCompleted := func(statusIDStr string) bool {
		if statusIDStr == "" {
			return false
		}
		var statusID int
		if _, err := fmt.Sscanf(statusIDStr, "%d", &statusID); err != nil {
			return false
		}
		return statusCompletedMap[statusID]
	}

	// Build daily data points
	var dataPoints []BurndownDataPoint
	today := time.Now().Truncate(24 * time.Hour)
	effectiveEndDate := endDate
	if today.Before(endDate) {
		effectiveEndDate = today
	}

	totalDays := int(endDate.Sub(startDate).Hours()/24) + 1

	// Start with current state and work backwards through history to build daily snapshots
	// Clone current state for simulation
	dayStates := make(map[int]bool)
	for id, completed := range itemStates {
		dayStates[id] = completed
	}

	// Build data for each day from end to start
	type dayData struct {
		date      string
		remaining int
		completed int
	}
	var dailyData []dayData

	for d := effectiveEndDate; !d.Before(startDate); d = d.AddDate(0, 0, -1) {
		dateStr := d.Format("2006-01-02")

		// Apply any history changes that happened after this day (reverse them)
		for _, c := range changes {
			changeDate := c.ChangedAt.Truncate(24 * time.Hour)
			if changeDate.Equal(d.AddDate(0, 0, 1)) || changeDate.After(d.AddDate(0, 0, 1)) {
				// This change happened after our current day, so reverse it
				// (set the item to its old state)
				if _, exists := dayStates[c.ItemID]; exists {
					dayStates[c.ItemID] = isStatusCompleted(c.OldValue.String)
				}
			}
		}

		// Filter changes to only those not yet processed
		var remainingChanges []statusChange
		for _, c := range changes {
			changeDate := c.ChangedAt.Truncate(24 * time.Hour)
			if changeDate.Before(d.AddDate(0, 0, 1)) {
				remainingChanges = append(remainingChanges, c)
			}
		}
		changes = remainingChanges

		// Count completed and remaining
		completed := 0
		for _, isCompleted := range dayStates {
			if isCompleted {
				completed++
			}
		}
		remaining := totalItems - completed

		dailyData = append(dailyData, dayData{
			date:      dateStr,
			remaining: remaining,
			completed: completed,
		})
	}

	// Reverse to get chronological order
	for i := len(dailyData) - 1; i >= 0; i-- {
		dd := dailyData[i]
		dayIndex := 0
		d, _ := parseDate(dd.date)
		dayIndex = int(d.Sub(startDate).Hours() / 24)

		// Calculate ideal remaining for this day
		ideal := totalItems
		if totalDays > 1 {
			ideal = totalItems - (dayIndex * totalItems / (totalDays - 1))
			if ideal < 0 {
				ideal = 0
			}
		}

		dataPoints = append(dataPoints, BurndownDataPoint{
			Date:      dd.date,
			Remaining: dd.remaining,
			Completed: dd.completed,
			Ideal:     ideal,
		})
	}

	return &IterationBurndownData{
		IterationID: iterationID,
		StartDate:   iter.StartDate,
		EndDate:     iter.EndDate,
		TotalItems:  totalItems,
		DataPoints:  dataPoints,
	}, nil
}
