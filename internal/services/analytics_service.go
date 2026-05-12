package services

import (
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"windshift/internal/cql"
	"windshift/internal/database"
)

// AnalyticsService provides analytics computations for collection/workspace data.
type AnalyticsService struct {
	db database.Database
}

// NewAnalyticsService creates a new analytics service.
func NewAnalyticsService(db database.Database) *AnalyticsService {
	return &AnalyticsService{db: db}
}

// GetCollectionWorkspaceID returns the workspace_id stored on the given
// collection. Used by the analytics handler to enforce that a caller can't
// fetch analytics for a collection whose workspace they don't have view on.
// Returns sql.ErrNoRows if the collection does not exist.
func (s *AnalyticsService) GetCollectionWorkspaceID(collectionID int) (int, error) {
	var wsID sql.NullInt64
	if err := s.db.QueryRow(`SELECT workspace_id FROM collections WHERE id = ?`, collectionID).Scan(&wsID); err != nil {
		return 0, err
	}
	if !wsID.Valid {
		return 0, nil
	}
	return int(wsID.Int64), nil
}

// DataQuality describes whether enough data exists for meaningful analytics.
type DataQuality struct {
	Sufficient bool   `json:"sufficient"`
	Reason     string `json:"reason,omitempty"`
}

// --- Dataset ---

// DatasetIteration describes an iteration within the analytics dataset.
type DatasetIteration struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Status    string `json:"status"`
	TypeName  string `json:"type_name,omitempty"`
}

// DatasetSummary is the public summary of the resolved dataset.
type DatasetSummary struct {
	TotalItems     int                `json:"total_items"`
	IterationCount int                `json:"iteration_count"`
	Iterations     []DatasetIteration `json:"iterations"`
	DateFrom       string             `json:"date_from"`
	DateTo         string             `json:"date_to"`
}

// dataset holds the resolved item IDs and iteration IDs for internal use.
type dataset struct {
	Summary      DatasetSummary
	ItemIDs      []int
	IterationIDs []int
	WorkspaceID  int
}

// ResolveDatasetParams defines how to resolve the item set.
type ResolveDatasetParams struct {
	WorkspaceID  int
	CollectionID int    // 0 = use workspace items directly
	QLQuery      string // optional direct QL override
	UserID       int    // Authenticated user ID for currentUser() resolution
	StartDate    time.Time
	EndDate      time.Time
}

// resolveDataset resolves the set of items (via collection CQL or workspace fallback),
// extracts unique iteration IDs, and filters iterations that overlap the date range.
func (s *AnalyticsService) resolveDataset(params ResolveDatasetParams) (*dataset, error) {
	var itemIDs []int
	var effectiveWorkspaceID int

	switch {
	case params.CollectionID > 0:
		var err error
		itemIDs, effectiveWorkspaceID, err = s.resolveCollectionItems(params.CollectionID, params.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve collection: %w", err)
		}
	case params.QLQuery != "":
		var err error
		itemIDs, effectiveWorkspaceID, err = s.resolveQLItems(params.QLQuery, params.WorkspaceID, params.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve QL query: %w", err)
		}
	default:
		var err error
		itemIDs, effectiveWorkspaceID, err = s.resolveWorkspaceItems(params.WorkspaceID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve workspace items: %w", err)
		}
	}

	if len(itemIDs) == 0 {
		return &dataset{
			Summary:      DatasetSummary{TotalItems: 0, IterationCount: 0, Iterations: []DatasetIteration{}},
			ItemIDs:      []int{},
			IterationIDs: []int{},
			WorkspaceID:  effectiveWorkspaceID,
		}, nil
	}

	// Deduplicate iteration IDs from the resolved items.
	iterationIDs, err := s.extractIterationIDs(itemIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to extract iteration IDs: %w", err)
	}

	// Load iteration details and filter by date range overlap.
	iterations, err := s.loadIterations(iterationIDs, params.StartDate, params.EndDate)
	if err != nil {
		return nil, fmt.Errorf("failed to load iterations: %w", err)
	}

	// Compute date span from matching iterations.
	dateFrom := params.StartDate.Format("2006-01-02")
	dateTo := params.EndDate.Format("2006-01-02")
	if len(iterations) > 0 {
		dateFrom = iterations[0].StartDate
		dateTo = iterations[len(iterations)-1].EndDate
	}

	matchedIDs := make([]int, len(iterations))
	for i, iter := range iterations {
		matchedIDs[i] = iter.ID
	}

	return &dataset{
		Summary: DatasetSummary{
			TotalItems:     len(itemIDs),
			IterationCount: len(iterations),
			Iterations:     iterations,
			DateFrom:       dateFrom,
			DateTo:         dateTo,
		},
		ItemIDs:      itemIDs,
		IterationIDs: matchedIDs,
		WorkspaceID:  effectiveWorkspaceID,
	}, nil
}

// resolveCollectionItems resolves a collection's CQL to item IDs.
func (s *AnalyticsService) resolveCollectionItems(collectionID, userID int) (_ []int, _ int, retErr error) {
	var wsID sql.NullInt64
	var qlStr sql.NullString
	err := s.db.QueryRow(`SELECT workspace_id, ql_query FROM collections WHERE id = ?`, collectionID).Scan(&wsID, &qlStr)
	if err != nil {
		return nil, 0, fmt.Errorf("collection not found: %w", err)
	}

	effectiveWorkspaceID := 0
	if wsID.Valid {
		effectiveWorkspaceID = int(wsID.Int64)
	}

	if !qlStr.Valid || strings.TrimSpace(qlStr.String) == "" {
		return []int{}, effectiveWorkspaceID, nil
	}

	ids, err := s.evaluateQLToItemIDs(qlStr.String, effectiveWorkspaceID, userID)
	return ids, effectiveWorkspaceID, err
}

// resolveQLItems resolves a direct QL query to item IDs.
func (s *AnalyticsService) resolveQLItems(qlQuery string, workspaceID, userID int) (_ []int, _ int, retErr error) {
	ids, err := s.evaluateQLToItemIDs(qlQuery, workspaceID, userID)
	return ids, workspaceID, err
}

// resolveWorkspaceItems returns all item IDs for a workspace.
func (s *AnalyticsService) resolveWorkspaceItems(workspaceID int) (itemIDs []int, wsID int, retErr error) {
	rows, err := s.db.Query(`SELECT id FROM items WHERE workspace_id = ?`, workspaceID)
	if err != nil {
		return nil, workspaceID, err
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, workspaceID, rows.Err()
}

// evaluateQLToItemIDs evaluates a CQL query and returns matching item IDs.
func (s *AnalyticsService) evaluateQLToItemIDs(qlQuery string, workspaceID, userID int) ([]int, error) {
	workspaceMap, err := s.buildWorkspaceMap()
	if err != nil {
		return nil, err
	}

	resolvedQuery := cql.SubstituteFunctions(qlQuery, cql.UserContext(userID))
	evaluator := cql.NewEvaluator(workspaceMap, s.db.GetDriverName())
	sqlWhere, sqlArgs, err := evaluator.EvaluateToSQL(resolvedQuery)
	if err != nil {
		return nil, fmt.Errorf("CQL evaluation failed: %w", err)
	}
	if sqlWhere == "" {
		return []int{}, nil
	}

	query := fmt.Sprintf(`SELECT i.id FROM items i WHERE (%s)`, sqlWhere)
	if workspaceID > 0 {
		query += ` AND i.workspace_id = ?`
		sqlArgs = append(sqlArgs, workspaceID)
	}

	rows, err := s.db.Query(query, sqlArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

// buildWorkspaceMap builds the name/key/id map needed by the CQL evaluator.
func (s *AnalyticsService) buildWorkspaceMap() (map[string]int, error) {
	rows, err := s.db.Query("SELECT id, name, key FROM workspaces")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	wsMap := make(map[string]int)
	for rows.Next() {
		var id int
		var name, key string
		if err := rows.Scan(&id, &name, &key); err == nil {
			wsMap[fmt.Sprintf("%d", id)] = id
			wsMap[name] = id
			wsMap[key] = id
		}
	}
	return wsMap, rows.Err()
}

// extractIterationIDs returns deduplicated iteration IDs from the given items.
func (s *AnalyticsService) extractIterationIDs(itemIDs []int) ([]int, error) {
	if len(itemIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(itemIDs))
	args := make([]interface{}, len(itemIDs))
	for i, id := range itemIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(
		`SELECT DISTINCT iteration_id FROM items WHERE id IN (%s) AND iteration_id IS NOT NULL`,
		strings.Join(placeholders, ","),
	)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

// loadIterations loads iteration details for the given IDs, filtered by date range overlap.
// Returns iterations sorted chronologically by start_date.
func (s *AnalyticsService) loadIterations(iterationIDs []int, startDate, endDate time.Time) ([]DatasetIteration, error) {
	if len(iterationIDs) == 0 {
		return []DatasetIteration{}, nil
	}

	placeholders := make([]string, len(iterationIDs))
	args := make([]interface{}, len(iterationIDs), len(iterationIDs)+2)
	for i, id := range iterationIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT i.id, i.name, i.start_date, i.end_date, i.status,
		       COALESCE(it.name, '') as type_name
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		WHERE i.id IN (%s)
		  AND i.start_date <= ? AND i.end_date >= ?
		ORDER BY i.start_date ASC
	`, strings.Join(placeholders, ","))

	args = append(args, endDate.Format("2006-01-02"), startDate.Format("2006-01-02"))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var iterations []DatasetIteration
	for rows.Next() {
		var iter DatasetIteration
		if err := rows.Scan(&iter.ID, &iter.Name, &iter.StartDate, &iter.EndDate, &iter.Status, &iter.TypeName); err == nil {
			iterations = append(iterations, iter)
		}
	}
	return iterations, rows.Err()
}

// --- Aggregated Analytics Result ---

// AnalyticsResult is the aggregated response for the analytics page.
type AnalyticsResult struct {
	Dataset        DatasetSummary  `json:"dataset"`
	Velocity       VelocityResult  `json:"velocity"`
	CumulativeFlow CFDResult       `json:"cumulative_flow"`
	CycleTime      CycleTimeResult `json:"cycle_time"`
	Forecast       ForecastResult  `json:"forecast"`
}

// GetAnalytics computes all analytics panels for a dataset.
func (s *AnalyticsService) GetAnalytics(params ResolveDatasetParams) (*AnalyticsResult, error) {
	ds, err := s.resolveDataset(params)
	if err != nil {
		return nil, err
	}

	velocity := s.computeVelocity(ds)
	cfd := s.computeCumulativeFlow(ds, params.StartDate, params.EndDate)
	cycleTime := s.computeCycleTime(ds, params.StartDate, params.EndDate)
	forecast := s.computeForecast(ds)

	return &AnalyticsResult{
		Dataset:        ds.Summary,
		Velocity:       velocity,
		CumulativeFlow: cfd,
		CycleTime:      cycleTime,
		Forecast:       forecast,
	}, nil
}

// --- Velocity ---

// VelocityIteration holds metrics for one iteration.
type VelocityIteration struct {
	IterationID     int     `json:"iteration_id"`
	Name            string  `json:"name"`
	StartDate       string  `json:"start_date"`
	EndDate         string  `json:"end_date"`
	Status          string  `json:"status"`
	CompletedCount  int     `json:"completed_count"`
	CompletedPoints float64 `json:"completed_points"`
	CompletedHours  float64 `json:"completed_hours"`
	TotalCount      int     `json:"total_count"`
	TotalPoints     float64 `json:"total_points"`
}

// VelocityAverages holds rolling averages (completed iterations only).
type VelocityAverages struct {
	AvgCount  float64 `json:"avg_count"`
	AvgPoints float64 `json:"avg_points"`
	AvgHours  float64 `json:"avg_hours"`
}

// VelocityResult is the velocity section of the analytics response.
type VelocityResult struct {
	Iterations  []VelocityIteration `json:"iterations"`
	Averages    VelocityAverages    `json:"averages"`
	DataQuality DataQuality         `json:"data_quality"`
}

func (s *AnalyticsService) computeVelocity(ds *dataset) VelocityResult {
	if len(ds.IterationIDs) == 0 {
		dq := DataQuality{Sufficient: false, Reason: "no_items"}
		if ds.Summary.TotalItems > 0 {
			dq = DataQuality{Sufficient: false, Reason: "no_iterations"}
		}
		return VelocityResult{DataQuality: dq}
	}

	// Build item ID set for filtering.
	itemSet := make(map[int]bool, len(ds.ItemIDs))
	for _, id := range ds.ItemIDs {
		itemSet[id] = true
	}

	var iterations []VelocityIteration
	for _, iterID := range ds.IterationIDs {
		// Find the iteration summary entry.
		var iterInfo DatasetIteration
		for _, iter := range ds.Summary.Iterations {
			if iter.ID == iterID {
				iterInfo = iter
				break
			}
		}

		vi := VelocityIteration{
			IterationID: iterID,
			Name:        iterInfo.Name,
			StartDate:   iterInfo.StartDate,
			EndDate:     iterInfo.EndDate,
			Status:      iterInfo.Status,
		}

		// Total items and points in this iteration from the dataset.
		var totalCount int
		var totalPoints sql.NullFloat64
		_ = s.db.QueryRow(`
			SELECT COUNT(*), COALESCE(SUM(i.story_points), 0)
			FROM items i
			WHERE i.iteration_id = ?
		`, iterID).Scan(&totalCount, &totalPoints)

		// Filter to dataset items only.
		if totalCount > 0 && len(ds.ItemIDs) > 0 {
			placeholders := make([]string, len(ds.ItemIDs))
			args := make([]interface{}, len(ds.ItemIDs)+1)
			args[0] = iterID
			for j, id := range ds.ItemIDs {
				placeholders[j] = "?"
				args[j+1] = id
			}
			_ = s.db.QueryRow(fmt.Sprintf(
				`SELECT COUNT(*), COALESCE(SUM(i.story_points), 0) FROM items i WHERE i.iteration_id = ? AND i.id IN (%s)`,
				strings.Join(placeholders, ","),
			), args...).Scan(&totalCount, &totalPoints)
		}

		vi.TotalCount = totalCount
		vi.TotalPoints = totalPoints.Float64

		// Completed items and points from dataset.
		var completedCount int
		var completedPoints sql.NullFloat64
		if len(ds.ItemIDs) > 0 {
			placeholders := make([]string, len(ds.ItemIDs))
			args := make([]interface{}, len(ds.ItemIDs)+1)
			args[0] = iterID
			for j, id := range ds.ItemIDs {
				placeholders[j] = "?"
				args[j+1] = id
			}
			_ = s.db.QueryRow(fmt.Sprintf(`
				SELECT COUNT(*), COALESCE(SUM(i.story_points), 0)
				FROM items i
				JOIN statuses st ON i.status_id = st.id
				JOIN status_categories sc ON st.category_id = sc.id
				WHERE i.iteration_id = ? AND i.id IN (%s) AND sc.is_completed = true
			`, strings.Join(placeholders, ",")), args...).Scan(&completedCount, &completedPoints)
		}

		vi.CompletedCount = completedCount
		vi.CompletedPoints = completedPoints.Float64

		// Completed hours from worklogs.
		var completedHours sql.NullFloat64
		if len(ds.ItemIDs) > 0 {
			placeholders := make([]string, len(ds.ItemIDs))
			args := make([]interface{}, len(ds.ItemIDs)+3)
			args[0] = iterID
			args[1] = vi.StartDate
			args[2] = vi.EndDate
			for j, id := range ds.ItemIDs {
				placeholders[j] = "?"
				args[j+3] = id
			}
			_ = s.db.QueryRow(fmt.Sprintf(`
				SELECT COALESCE(SUM(tw.hours), 0)
				FROM time_worklogs tw
				JOIN items i ON tw.item_id = i.id
				WHERE i.iteration_id = ? AND tw.log_date >= ? AND tw.log_date <= ?
				  AND i.id IN (%s)
			`, strings.Join(placeholders, ",")), args...).Scan(&completedHours)
		}

		if completedHours.Valid {
			vi.CompletedHours = completedHours.Float64
		}

		iterations = append(iterations, vi)
	}

	// Compute averages from completed iterations only.
	var avgResult VelocityAverages
	var completedIters []VelocityIteration
	for _, iter := range iterations {
		if iter.Status == "completed" {
			completedIters = append(completedIters, iter)
		}
	}
	if len(completedIters) > 0 {
		var sumCount, sumPoints, sumHours float64
		for _, iter := range completedIters {
			sumCount += float64(iter.CompletedCount)
			sumPoints += iter.CompletedPoints
			sumHours += iter.CompletedHours
		}
		n := float64(len(completedIters))
		avgResult.AvgCount = sumCount / n
		avgResult.AvgPoints = sumPoints / n
		avgResult.AvgHours = sumHours / n
	}

	dq := DataQuality{Sufficient: true}
	switch {
	case ds.Summary.TotalItems == 0:
		dq = DataQuality{Sufficient: false, Reason: "no_items"}
	case len(iterations) == 0:
		dq = DataQuality{Sufficient: false, Reason: "no_iterations"}
	default:
		hasData := false
		for _, iter := range iterations {
			if iter.TotalCount > 0 {
				hasData = true
				break
			}
		}
		if !hasData {
			dq = DataQuality{Sufficient: false, Reason: "no_iteration_items"}
		}
	}

	return VelocityResult{
		Iterations:  iterations,
		Averages:    avgResult,
		DataQuality: dq,
	}
}

// --- Cumulative Flow Diagram ---

// CFDCategory describes a status category.
type CFDCategory struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// CFDDataPoint is one day's snapshot.
type CFDDataPoint struct {
	Date   string         `json:"date"`
	Counts map[string]int `json:"counts"`
}

// CFDResult is the cumulative flow section of the analytics response.
type CFDResult struct {
	Categories  []CFDCategory  `json:"categories"`
	DataPoints  []CFDDataPoint `json:"data_points"`
	DataQuality DataQuality    `json:"data_quality"`
}

func (s *AnalyticsService) computeCumulativeFlow(ds *dataset, startDate, endDate time.Time) CFDResult {
	if len(ds.ItemIDs) == 0 {
		return CFDResult{DataQuality: DataQuality{Sufficient: false, Reason: "no_items"}}
	}

	// Get status categories for the workspace.
	catRows, err := s.db.Query(`
		SELECT DISTINCT sc.name, sc.color
		FROM status_categories sc
		JOIN statuses st ON st.category_id = sc.id
		JOIN workflow_transitions wt ON wt.from_status_id = st.id OR wt.to_status_id = st.id
		JOIN workflows w ON wt.workflow_id = w.id
		WHERE w.id = (
			SELECT COALESCE(
				(SELECT cs.workflow_id FROM workspace_configuration_sets wcs
				 JOIN configuration_sets cs ON wcs.configuration_set_id = cs.id
				 WHERE wcs.workspace_id = ?),
				(SELECT cs.workflow_id FROM configuration_sets cs WHERE cs.is_default = true)
			)
		)
		ORDER BY sc.name
	`, ds.WorkspaceID)
	if err != nil {
		return CFDResult{DataQuality: DataQuality{Sufficient: false, Reason: "no_workflow"}}
	}
	defer catRows.Close()

	var categories []CFDCategory
	for catRows.Next() {
		var c CFDCategory
		if err := catRows.Scan(&c.Name, &c.Color); err == nil {
			categories = append(categories, c)
		}
	}
	if len(categories) == 0 {
		return CFDResult{DataQuality: DataQuality{Sufficient: false, Reason: "no_workflow"}}
	}

	// Build status_id -> category_name map.
	statusCategoryMap := make(map[int]string)
	scRows, err := s.db.Query(`SELECT s.id, sc.name FROM statuses s JOIN status_categories sc ON s.category_id = sc.id`)
	if err != nil {
		return CFDResult{DataQuality: DataQuality{Sufficient: false, Reason: "no_workflow"}}
	}
	defer scRows.Close()
	for scRows.Next() {
		var sid int
		var catName string
		if err := scRows.Scan(&sid, &catName); err == nil {
			statusCategoryMap[sid] = catName
		}
	}

	// Get current state of items in dataset.
	itemPlaceholders := strings.Repeat("?,", len(ds.ItemIDs))
	itemPlaceholders = itemPlaceholders[:len(itemPlaceholders)-1]
	itemArgs := make([]interface{}, len(ds.ItemIDs))
	for i, id := range ds.ItemIDs {
		itemArgs[i] = id
	}

	currentState := make(map[int]int)
	itemRows, err := s.db.Query(
		fmt.Sprintf(`SELECT i.id, i.status_id FROM items i WHERE i.id IN (%s)`, itemPlaceholders),
		itemArgs...,
	)
	if err == nil {
		defer itemRows.Close()
		for itemRows.Next() {
			var itemID int
			var statusID sql.NullInt64
			if itemRows.Scan(&itemID, &statusID) == nil && statusID.Valid {
				currentState[itemID] = int(statusID.Int64)
			}
		}
	}

	// Get status changes for dataset items, ordered descending.
	histArgs := make([]interface{}, len(ds.ItemIDs)+1)
	histArgs[0] = startDate.Format("2006-01-02")
	copy(histArgs[1:], itemArgs)

	histRows, err := s.db.Query(fmt.Sprintf(`
		SELECT ih.item_id, ih.changed_at, ih.old_value, ih.new_value
		FROM item_history ih
		WHERE ih.item_id IN (%s)
		  AND ih.field_name = 'status_id'
		  AND ih.changed_at >= ?
		ORDER BY ih.changed_at DESC
	`, itemPlaceholders), histArgs...)
	if err != nil {
		return CFDResult{Categories: categories, DataQuality: DataQuality{Sufficient: false, Reason: "no_history"}}
	}
	defer histRows.Close()

	type statusChange struct {
		ItemID    int
		ChangedAt time.Time
		OldValue  string
		NewValue  string
	}
	var changes []statusChange
	for histRows.Next() {
		var c statusChange
		var changedAtStr string
		var oldVal, newVal sql.NullString
		if err := histRows.Scan(&c.ItemID, &changedAtStr, &oldVal, &newVal); err != nil {
			continue
		}
		c.ChangedAt, _ = time.Parse("2006-01-02 15:04:05", changedAtStr)
		if c.ChangedAt.IsZero() {
			c.ChangedAt, _ = time.Parse(time.RFC3339, changedAtStr)
		}
		if oldVal.Valid {
			c.OldValue = oldVal.String
		}
		if newVal.Valid {
			c.NewValue = newVal.String
		}
		changes = append(changes, c)
	}

	// Clone current state for simulation.
	simState := make(map[int]int)
	for id, sid := range currentState {
		simState[id] = sid
	}

	today := time.Now().Truncate(24 * time.Hour)
	effectiveEnd := endDate
	if today.Before(endDate) {
		effectiveEnd = today
	}

	type daySnapshot struct {
		date   string
		counts map[string]int
	}
	var snapshots []daySnapshot

	changeIdx := 0
	for d := effectiveEnd; !d.Before(startDate); d = d.AddDate(0, 0, -1) {
		for changeIdx < len(changes) {
			c := changes[changeIdx]
			changeDate := c.ChangedAt.Truncate(24 * time.Hour)
			if changeDate.After(d) {
				var oldStatusID int
				if _, err := fmt.Sscanf(c.OldValue, "%d", &oldStatusID); err == nil {
					simState[c.ItemID] = oldStatusID
				}
				changeIdx++
			} else {
				break
			}
		}

		counts := make(map[string]int)
		for _, cat := range categories {
			counts[cat.Name] = 0
		}
		for _, sid := range simState {
			if catName, ok := statusCategoryMap[sid]; ok {
				counts[catName]++
			}
		}

		snapshots = append(snapshots, daySnapshot{
			date:   d.Format("2006-01-02"),
			counts: counts,
		})
	}

	dataPoints := make([]CFDDataPoint, len(snapshots))
	for i, snap := range snapshots {
		dataPoints[len(snapshots)-1-i] = CFDDataPoint{
			Date:   snap.date,
			Counts: snap.counts,
		}
	}

	dq := DataQuality{Sufficient: true}
	if len(currentState) == 0 {
		dq = DataQuality{Sufficient: false, Reason: "no_items"}
	}

	return CFDResult{
		Categories:  categories,
		DataPoints:  dataPoints,
		DataQuality: dq,
	}
}

// --- Cycle Time ---

// CycleTimeStage holds aggregated time for one status/category.
type CycleTimeStage struct {
	Name        string  `json:"name"`
	AvgHours    float64 `json:"avg_hours"`
	MedianHours float64 `json:"median_hours"`
	P85Hours    float64 `json:"p85_hours"`
}

// CycleTimeSummary holds the total cycle time stats.
type CycleTimeSummary struct {
	AvgHours    float64 `json:"avg_hours"`
	MedianHours float64 `json:"median_hours"`
	P85Hours    float64 `json:"p85_hours"`
}

// CycleTimeResult is the cycle time section of the analytics response.
type CycleTimeResult struct {
	TotalItemsAnalyzed int              `json:"total_items_analyzed"`
	Stages             []CycleTimeStage `json:"stages"`
	TotalCycleTime     CycleTimeSummary `json:"total_cycle_time"`
	DataQuality        DataQuality      `json:"data_quality"`
}

func (s *AnalyticsService) computeCycleTime(ds *dataset, startDate, endDate time.Time) CycleTimeResult {
	if len(ds.ItemIDs) == 0 {
		return CycleTimeResult{DataQuality: DataQuality{Sufficient: false, Reason: "no_items"}}
	}

	// Build status_id -> category_name map.
	statusCategoryMap := make(map[string]string)
	scRows, err := s.db.Query(`SELECT s.id, sc.name FROM statuses s JOIN status_categories sc ON s.category_id = sc.id`)
	if err != nil {
		return CycleTimeResult{DataQuality: DataQuality{Sufficient: false, Reason: "no_workflow"}}
	}
	defer scRows.Close()
	for scRows.Next() {
		var sid int
		var catName string
		if scRows.Scan(&sid, &catName) == nil {
			statusCategoryMap[fmt.Sprintf("%d", sid)] = catName
		}
	}

	// Find dataset items completed in the date range.
	itemPlaceholders := strings.Repeat("?,", len(ds.ItemIDs))
	itemPlaceholders = itemPlaceholders[:len(itemPlaceholders)-1]
	itemArgs := make([]interface{}, len(ds.ItemIDs))
	for i, id := range ds.ItemIDs {
		itemArgs[i] = id
	}

	completedArgs := make([]interface{}, len(ds.ItemIDs)+2)
	completedArgs[0] = startDate.Format("2006-01-02")
	completedArgs[1] = endDate.AddDate(0, 0, 1).Format("2006-01-02")
	copy(completedArgs[2:], itemArgs)

	completedRows, err := s.db.Query(fmt.Sprintf(`
		SELECT DISTINCT ih.item_id
		FROM item_history ih
		JOIN statuses s ON CAST(ih.new_value AS INTEGER) = s.id
		JOIN status_categories sc ON s.category_id = sc.id
		WHERE ih.item_id IN (%s)
		  AND ih.field_name = 'status_id'
		  AND sc.is_completed = true
		  AND ih.changed_at >= ? AND ih.changed_at <= ?
	`, itemPlaceholders), completedArgs...)
	if err != nil {
		return CycleTimeResult{DataQuality: DataQuality{Sufficient: false, Reason: "no_completed_items"}}
	}
	defer completedRows.Close()

	var completedItemIDs []int
	for completedRows.Next() {
		var itemID int
		if completedRows.Scan(&itemID) == nil {
			completedItemIDs = append(completedItemIDs, itemID)
		}
	}

	if len(completedItemIDs) == 0 {
		return CycleTimeResult{DataQuality: DataQuality{Sufficient: false, Reason: "no_completed_items"}}
	}

	type itemCategoryHours map[string]float64
	allItemHours := make([]itemCategoryHours, 0, len(completedItemIDs))
	var totalCycleTimes []float64

	for _, itemID := range completedItemIDs {
		histRows, err := s.db.Query(`
			SELECT changed_at, old_value, new_value
			FROM item_history
			WHERE item_id = ? AND field_name = 'status_id'
			ORDER BY changed_at ASC
		`, itemID)
		if err != nil {
			continue
		}

		type transition struct {
			at       time.Time
			newValue string
		}
		var transitions []transition
		for histRows.Next() {
			var t transition
			var changedAtStr string
			var oldVal, newVal sql.NullString
			if histRows.Scan(&changedAtStr, &oldVal, &newVal) != nil {
				continue
			}
			t.at, _ = time.Parse("2006-01-02 15:04:05", changedAtStr)
			if t.at.IsZero() {
				t.at, _ = time.Parse(time.RFC3339, changedAtStr)
			}
			if newVal.Valid {
				t.newValue = newVal.String
			}
			transitions = append(transitions, t)
		}
		histRows.Close()

		if len(transitions) == 0 {
			continue
		}

		catHours := make(itemCategoryHours)
		var totalHours float64

		for i := 0; i < len(transitions); i++ {
			statusID := transitions[i].newValue
			catName := statusCategoryMap[statusID]
			if catName == "" {
				catName = "Unknown"
			}

			var duration time.Duration
			if i+1 < len(transitions) {
				duration = transitions[i+1].at.Sub(transitions[i].at)
			} else {
				duration = time.Since(transitions[i].at)
			}

			hours := duration.Hours()
			catHours[catName] += hours
			totalHours += hours
		}

		allItemHours = append(allItemHours, catHours)
		totalCycleTimes = append(totalCycleTimes, totalHours)
	}

	categoryHoursMap := make(map[string][]float64)
	for _, ich := range allItemHours {
		for cat, hours := range ich {
			categoryHoursMap[cat] = append(categoryHoursMap[cat], hours)
		}
	}

	var stages []CycleTimeStage
	for cat, hoursList := range categoryHoursMap {
		stages = append(stages, CycleTimeStage{
			Name:        cat,
			AvgHours:    avg(hoursList),
			MedianHours: percentile(hoursList, 50),
			P85Hours:    percentile(hoursList, 85),
		})
	}
	sort.Slice(stages, func(i, j int) bool {
		return stages[i].AvgHours > stages[j].AvgHours
	})

	dq := DataQuality{Sufficient: true}
	if len(allItemHours) < 3 {
		dq = DataQuality{Sufficient: false, Reason: "few_completed_items"}
	}

	return CycleTimeResult{
		TotalItemsAnalyzed: len(allItemHours),
		Stages:             stages,
		TotalCycleTime: CycleTimeSummary{
			AvgHours:    avg(totalCycleTimes),
			MedianHours: percentile(totalCycleTimes, 50),
			P85Hours:    percentile(totalCycleTimes, 85),
		},
		DataQuality: dq,
	}
}

// --- Forecast ---

// ForecastEntry holds one confidence level prediction.
type ForecastEntry struct {
	Confidence          int    `json:"confidence"`
	IterationsRemaining int    `json:"iterations_remaining"`
	EstimatedDate       string `json:"estimated_date"`
}

// ForecastResult is the forecast section of the analytics response.
type ForecastResult struct {
	RemainingItems    int             `json:"remaining_items"`
	RemainingPoints   float64         `json:"remaining_points"`
	ThroughputSamples []int           `json:"throughput_samples"`
	Forecasts         []ForecastEntry `json:"forecasts"`
	Method            string          `json:"method"`
	DataQuality       DataQuality     `json:"data_quality"`
}

func (s *AnalyticsService) computeForecast(ds *dataset) ForecastResult {
	if len(ds.ItemIDs) == 0 {
		return ForecastResult{DataQuality: DataQuality{Sufficient: false, Reason: "no_items"}}
	}

	confidenceLevels := []int{50, 85, 95}

	// Get throughput from completed iterations in the dataset.
	var throughputSamples []int
	if len(ds.IterationIDs) > 0 {
		iterPlaceholders := strings.Repeat("?,", len(ds.IterationIDs))
		iterPlaceholders = iterPlaceholders[:len(iterPlaceholders)-1]
		itemPlaceholders := strings.Repeat("?,", len(ds.ItemIDs))
		itemPlaceholders = itemPlaceholders[:len(itemPlaceholders)-1]

		args := make([]interface{}, len(ds.IterationIDs)+len(ds.ItemIDs))
		for i, id := range ds.IterationIDs {
			args[i] = id
		}
		for i, id := range ds.ItemIDs {
			args[len(ds.IterationIDs)+i] = id
		}

		throughputRows, err := s.db.Query(fmt.Sprintf(`
			SELECT iter.id, COUNT(i.id) as completed_count
			FROM iterations iter
			JOIN items i ON i.iteration_id = iter.id
			JOIN statuses st ON i.status_id = st.id
			JOIN status_categories sc ON st.category_id = sc.id
			WHERE iter.id IN (%s)
			  AND iter.status = 'completed'
			  AND i.id IN (%s)
			  AND sc.is_completed = true
			GROUP BY iter.id
			ORDER BY iter.end_date DESC
			LIMIT 10
		`, iterPlaceholders, itemPlaceholders), args...)
		if err == nil {
			defer throughputRows.Close()
			for throughputRows.Next() {
				var iterID, count int
				if throughputRows.Scan(&iterID, &count) == nil {
					throughputSamples = append(throughputSamples, count)
				}
			}
		}
	}

	// Count remaining items in the dataset.
	var remainingItems int
	var remainingPoints float64
	itemPlaceholders := strings.Repeat("?,", len(ds.ItemIDs))
	itemPlaceholders = itemPlaceholders[:len(itemPlaceholders)-1]
	itemArgs := make([]interface{}, len(ds.ItemIDs))
	for i, id := range ds.ItemIDs {
		itemArgs[i] = id
	}

	_ = s.db.QueryRow(fmt.Sprintf(`
		SELECT COUNT(*), COALESCE(SUM(i.story_points), 0)
		FROM items i
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN status_categories sc ON st.category_id = sc.id
		WHERE i.id IN (%s)
		  AND (sc.is_completed IS NULL OR sc.is_completed = false)
	`, itemPlaceholders), itemArgs...).Scan(&remainingItems, &remainingPoints)

	// If not enough throughput data, use linear projection.
	if len(throughputSamples) < 3 {
		return s.linearForecast(remainingItems, remainingPoints, throughputSamples, confidenceLevels, ds)
	}

	// Get average iteration length.
	var avgIterDays float64
	if len(ds.IterationIDs) > 0 {
		iterPlaceholders := strings.Repeat("?,", len(ds.IterationIDs))
		iterPlaceholders = iterPlaceholders[:len(iterPlaceholders)-1]
		iterArgs := make([]interface{}, len(ds.IterationIDs))
		for i, id := range ds.IterationIDs {
			iterArgs[i] = id
		}
		_ = s.db.QueryRow(fmt.Sprintf(`
			SELECT AVG(JULIANDAY(end_date) - JULIANDAY(start_date))
			FROM iterations
			WHERE id IN (%s)
			  AND status = 'completed'
			  AND start_date IS NOT NULL AND end_date IS NOT NULL
		`, iterPlaceholders), iterArgs...).Scan(&avgIterDays)
	}
	if avgIterDays <= 0 {
		avgIterDays = 14
	}

	// Monte Carlo simulation.
	const simulations = 10000
	rng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // not cryptographic
	iterationResults := make([]int, simulations)

	for i := 0; i < simulations; i++ {
		remaining := remainingItems
		iters := 0
		for remaining > 0 && iters < 100 {
			throughput := throughputSamples[rng.Intn(len(throughputSamples))]
			if throughput <= 0 {
				throughput = 1
			}
			remaining -= throughput
			iters++
		}
		iterationResults[i] = iters
	}

	sort.Ints(iterationResults)

	var forecasts []ForecastEntry
	now := time.Now()
	for _, confidence := range confidenceLevels {
		idx := int(math.Ceil(float64(confidence)/100.0*float64(simulations))) - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= simulations {
			idx = simulations - 1
		}
		itersNeeded := iterationResults[idx]
		estDate := now.AddDate(0, 0, int(float64(itersNeeded)*avgIterDays))
		forecasts = append(forecasts, ForecastEntry{
			Confidence:          confidence,
			IterationsRemaining: itersNeeded,
			EstimatedDate:       estDate.Format("2006-01-02"),
		})
	}

	return ForecastResult{
		RemainingItems:    remainingItems,
		RemainingPoints:   remainingPoints,
		ThroughputSamples: throughputSamples,
		Forecasts:         forecasts,
		Method:            "monte_carlo",
		DataQuality:       DataQuality{Sufficient: true},
	}
}

func (s *AnalyticsService) linearForecast(remainingItems int, remainingPoints float64, samples, confidenceLevels []int, _ *dataset) ForecastResult {
	if len(samples) == 0 {
		return ForecastResult{
			RemainingItems:    remainingItems,
			RemainingPoints:   remainingPoints,
			ThroughputSamples: samples,
			Forecasts:         nil,
			Method:            "linear",
			DataQuality:       DataQuality{Sufficient: false, Reason: "no_iterations"},
		}
	}

	sum := 0
	for _, v := range samples {
		sum += v
	}
	avgThroughput := float64(sum) / float64(len(samples))
	if avgThroughput <= 0 {
		avgThroughput = 1
	}

	itersNeeded := int(math.Ceil(float64(remainingItems) / avgThroughput))
	now := time.Now()

	forecasts := make([]ForecastEntry, 0, len(confidenceLevels))
	for _, confidence := range confidenceLevels {
		multiplier := 1.0 + float64(confidence-50)/100.0
		if multiplier < 1 {
			multiplier = 1
		}
		adjIters := int(math.Ceil(float64(itersNeeded) * multiplier))
		estDate := now.AddDate(0, 0, adjIters*14)
		forecasts = append(forecasts, ForecastEntry{
			Confidence:          confidence,
			IterationsRemaining: adjIters,
			EstimatedDate:       estDate.Format("2006-01-02"),
		})
	}

	return ForecastResult{
		RemainingItems:    remainingItems,
		RemainingPoints:   remainingPoints,
		ThroughputSamples: samples,
		Forecasts:         forecasts,
		Method:            "linear",
		DataQuality:       DataQuality{Sufficient: false, Reason: "few_iterations"},
	}
}

// --- Helpers ---

func avg(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func percentile(vals []float64, pct float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	idx := int(math.Ceil(pct/100.0*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
