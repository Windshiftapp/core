package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/services"
)

// HomepageHandler handles homepage-related HTTP requests
type HomepageHandler struct {
	db              database.Database
	activityTracker *services.ActivityTracker
}

// NewHomepageHandler creates a new homepage handler
func NewHomepageHandler(db database.Database, activityTracker *services.ActivityTracker) *HomepageHandler {
	return &HomepageHandler{
		db:              db,
		activityTracker: activityTracker,
	}
}

// HomepageData represents the comprehensive data for the user's homepage
type HomepageData struct {
	RecentWorkspaces    []WorkspaceActivity `json:"recent_workspaces"`
	TotalWorkspaceCount int                 `json:"total_workspace_count"`
	TotalItemCount      int                 `json:"total_item_count"`
	RecentlyViewed      []ItemActivity      `json:"recently_viewed"`
	RecentlyEdited      []ItemActivity      `json:"recently_edited"`
	RecentlyCommented   []ItemActivity      `json:"recently_commented"`
	WatchedItems        []ItemActivity      `json:"watched_items"`
	UpcomingMilestones  []MilestoneProgress `json:"upcoming_milestones"`
}

// WorkspaceActivity represents a workspace visit with metadata
type WorkspaceActivity struct {
	WorkspaceID   int    `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	WorkspaceKey  string `json:"workspace_key"`
	Icon          string `json:"icon"`
	Color         string `json:"color"`
	LastVisited   string `json:"last_visited"`
	VisitCount    int    `json:"visit_count"`
}

// ItemActivity represents an item with activity metadata
type ItemActivity struct {
	ItemID              int     `json:"item_id"`
	WorkspaceID         int     `json:"workspace_id"`
	WorkspaceKey        string  `json:"workspace_key"`
	WorkspaceItemNumber int     `json:"workspace_item_number"`
	Title               string  `json:"title"`
	Status              string  `json:"status"`
	PriorityID          *int    `json:"priority_id,omitempty"`
	PriorityName        *string `json:"priority_name,omitempty"`
	PriorityColor       *string `json:"priority_color,omitempty"`
	LastActivity        string  `json:"last_activity"`
	ActivityCount       int     `json:"activity_count"`
}

// MilestoneProgress represents milestone progress statistics
type MilestoneProgress struct {
	MilestoneID     int     `json:"milestone_id"`
	MilestoneName   string  `json:"milestone_name"`
	TargetDate      *string `json:"target_date,omitempty"`
	TotalItems      int     `json:"total_items"`
	DoneItems       int     `json:"done_items"`
	NotDoneItems    int     `json:"not_done_items"`
	PercentComplete float64 `json:"percent_complete"`
	CategoryColor   string  `json:"category_color,omitempty"`
}

// GetHomepage handles GET /api/homepage - returns comprehensive homepage data
func (h *HomepageHandler) GetHomepage(w http.ResponseWriter, r *http.Request) {
	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Get user activity from ActivityTracker
	if h.activityTracker == nil {
		http.Error(w, "Activity tracker not available", http.StatusInternalServerError)
		return
	}

	userActivity, err := h.activityTracker.GetUserActivity(user.ID)
	if err != nil {
		slog.Error("error getting user activity", slog.String("component", "homepage"), slog.Int("user_id", user.ID), slog.Any("error", err))
		http.Error(w, "Failed to load activity data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	homepageData := HomepageData{
		RecentWorkspaces:    []WorkspaceActivity{},
		TotalWorkspaceCount: 0,
		TotalItemCount:      0,
		RecentlyViewed:      []ItemActivity{},
		RecentlyEdited:      []ItemActivity{},
		RecentlyCommented:   []ItemActivity{},
		WatchedItems:        []ItemActivity{},
		UpcomingMilestones:  []MilestoneProgress{},
	}

	// Get total workspace count (excluding personal workspaces for onboarding purposes)
	var workspaceCount int
	err = h.db.QueryRow(`SELECT COUNT(*) FROM workspaces WHERE is_personal = 0 OR is_personal IS NULL`).Scan(&workspaceCount)
	if err != nil {
		slog.Warn("error getting workspace count", slog.String("component", "homepage"), slog.Any("error", err))
		// Continue even if count fails - not critical
	} else {
		homepageData.TotalWorkspaceCount = workspaceCount
	}

	// Get total item count system-wide (for onboarding purposes)
	var itemCount int
	err = h.db.QueryRow(`
		SELECT COUNT(*)
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE (w.is_personal = 0 OR w.is_personal IS NULL)
	`).Scan(&itemCount)
	if err != nil {
		slog.Warn("error getting item count", slog.String("component", "homepage"), slog.Any("error", err))
		// Continue even if count fails - not critical
	} else {
		homepageData.TotalItemCount = itemCount
	}

	// Batch load workspace details for recent visits
	if len(userActivity.WorkspaceVisits) > 0 {
		workspaceActivities, err := h.getWorkspaceActivitiesBatch(userActivity.WorkspaceVisits)
		if err != nil {
			slog.Warn("error loading workspace activities", slog.String("component", "homepage"), slog.Any("error", err))
		} else {
			homepageData.RecentWorkspaces = workspaceActivities
		}
	}

	// Collect all item IDs that need to be loaded
	allItemActivities := make(map[int]services.ItemActivity)

	// Collect viewed items
	if viewedItems, ok := userActivity.ItemActivities[services.ActivityView]; ok {
		for _, activity := range viewedItems {
			allItemActivities[activity.ItemID] = activity
		}
	}

	// Collect edited items
	if editedItems, ok := userActivity.ItemActivities[services.ActivityEdit]; ok {
		for _, activity := range editedItems {
			allItemActivities[activity.ItemID] = activity
		}
	}

	// Collect commented items
	if commentedItems, ok := userActivity.ItemActivities[services.ActivityComment]; ok {
		for _, activity := range commentedItems {
			allItemActivities[activity.ItemID] = activity
		}
	}

	// Collect watched items
	for _, itemID := range userActivity.ItemWatches {
		if _, exists := allItemActivities[itemID]; !exists {
			allItemActivities[itemID] = services.ItemActivity{ItemID: itemID}
		}
	}

	// Batch load all item details
	if len(allItemActivities) > 0 {
		itemDetails, err := h.getItemActivitiesBatch(allItemActivities)
		if err != nil {
			slog.Warn("error batch loading items", slog.String("component", "homepage"), slog.Any("error", err))
		} else {
			// Distribute items to appropriate lists with correct timestamps
			if viewedItems, ok := userActivity.ItemActivities[services.ActivityView]; ok {
				for _, activity := range viewedItems {
					if item, exists := itemDetails[activity.ItemID]; exists {
						itemCopy := item
						itemCopy.LastActivity = activity.ActivityAt.Format(time.RFC3339)
						itemCopy.ActivityCount = activity.ActivityCount
						homepageData.RecentlyViewed = append(homepageData.RecentlyViewed, itemCopy)
					}
				}
			}

			if editedItems, ok := userActivity.ItemActivities[services.ActivityEdit]; ok {
				for _, activity := range editedItems {
					if item, exists := itemDetails[activity.ItemID]; exists {
						itemCopy := item
						itemCopy.LastActivity = activity.ActivityAt.Format(time.RFC3339)
						itemCopy.ActivityCount = activity.ActivityCount
						homepageData.RecentlyEdited = append(homepageData.RecentlyEdited, itemCopy)
					}
				}
			}

			if commentedItems, ok := userActivity.ItemActivities[services.ActivityComment]; ok {
				for _, activity := range commentedItems {
					if item, exists := itemDetails[activity.ItemID]; exists {
						itemCopy := item
						itemCopy.LastActivity = activity.ActivityAt.Format(time.RFC3339)
						itemCopy.ActivityCount = activity.ActivityCount
						homepageData.RecentlyCommented = append(homepageData.RecentlyCommented, itemCopy)
					}
				}
			}

			for _, itemID := range userActivity.ItemWatches {
				if item, exists := itemDetails[itemID]; exists {
					homepageData.WatchedItems = append(homepageData.WatchedItems, item)
				}
			}
		}
	}

	// Load upcoming milestones based on user's recent activity - now uses batch approach
	milestoneIDs := h.getUpcomingMilestonesBatch(allItemActivities)
	if len(milestoneIDs) > 0 {
		milestoneStats, err := h.getMilestoneStatsBatch(milestoneIDs)
		if err != nil {
			slog.Warn("error loading milestone stats", slog.String("component", "homepage"), slog.Any("error", err))
		} else {
			homepageData.UpcomingMilestones = milestoneStats
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(homepageData)
}

// getUserFromContext extracts the user from the request context
func (h *HomepageHandler) getUserFromContext(r *http.Request) *models.User {
	if user := r.Context().Value(middleware.ContextKeyUser); user != nil {
		if u, ok := user.(*models.User); ok {
			return u
		}
	}
	return nil
}

// getWorkspaceActivitiesBatch batch loads workspace details for multiple workspace visits
func (h *HomepageHandler) getWorkspaceActivitiesBatch(visits []services.WorkspaceVisit) ([]WorkspaceActivity, error) {
	if len(visits) == 0 {
		return []WorkspaceActivity{}, nil
	}

	// Build workspace ID list
	workspaceIDs := make([]int, len(visits))
	visitMap := make(map[int]services.WorkspaceVisit)
	for i, visit := range visits {
		workspaceIDs[i] = visit.WorkspaceID
		visitMap[visit.WorkspaceID] = visit
	}

	// Build placeholder string for IN query
	placeholders := make([]string, len(workspaceIDs))
	args := make([]interface{}, len(workspaceIDs))
	for i, id := range workspaceIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `
		SELECT id, name, key, icon, color
		FROM workspaces
		WHERE id IN (` + strings.Join(placeholders, ",") + `)
	`

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []WorkspaceActivity
	for rows.Next() {
		var activity WorkspaceActivity
		if err := rows.Scan(&activity.WorkspaceID, &activity.WorkspaceName, &activity.WorkspaceKey, &activity.Icon, &activity.Color); err != nil {
			continue
		}

		if visit, ok := visitMap[activity.WorkspaceID]; ok {
			activity.LastVisited = visit.VisitedAt.Format("2006-01-02T15:04:05Z07:00")
			activity.VisitCount = visit.VisitCount
		}
		activities = append(activities, activity)
	}

	return activities, rows.Err()
}

// getItemActivitiesBatch batch loads item details for multiple items
func (h *HomepageHandler) getItemActivitiesBatch(activities map[int]services.ItemActivity) (map[int]ItemActivity, error) {
	if len(activities) == 0 {
		return map[int]ItemActivity{}, nil
	}

	// Build item ID list
	itemIDs := make([]int, 0, len(activities))
	for id := range activities {
		itemIDs = append(itemIDs, id)
	}

	// Build placeholder string for IN query
	placeholders := make([]string, len(itemIDs))
	args := make([]interface{}, len(itemIDs))
	for i, id := range itemIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.title,
		       COALESCE(s.name, 'Unknown') as status,
		       i.priority_id, p.name as priority_name, p.color as priority_color,
		       w.key as workspace_key, i.milestone_id
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN priorities p ON i.priority_id = p.id
		WHERE i.id IN (` + strings.Join(placeholders, ",") + `)
	`

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int]ItemActivity)
	for rows.Next() {
		var itemActivity ItemActivity
		var milestoneID sql.NullInt64
		if err := rows.Scan(
			&itemActivity.ItemID,
			&itemActivity.WorkspaceID,
			&itemActivity.WorkspaceItemNumber,
			&itemActivity.Title,
			&itemActivity.Status,
			&itemActivity.PriorityID,
			&itemActivity.PriorityName,
			&itemActivity.PriorityColor,
			&itemActivity.WorkspaceKey,
			&milestoneID,
		); err != nil {
			continue
		}

		if activity, ok := activities[itemActivity.ItemID]; ok {
			if !activity.ActivityAt.IsZero() {
				itemActivity.LastActivity = activity.ActivityAt.Format("2006-01-02T15:04:05Z07:00")
			}
			itemActivity.ActivityCount = activity.ActivityCount
		}

		result[itemActivity.ItemID] = itemActivity
	}

	return result, rows.Err()
}

// getUpcomingMilestonesBatch identifies the top 3 most frequently occurring milestones
// from the loaded item activities (avoids N+1 queries)
func (h *HomepageHandler) getUpcomingMilestonesBatch(itemActivities map[int]services.ItemActivity) []int {
	if len(itemActivities) == 0 {
		return []int{}
	}

	// Build item ID list
	itemIDs := make([]int, 0, len(itemActivities))
	for id := range itemActivities {
		itemIDs = append(itemIDs, id)
	}

	// Build placeholder string for IN query
	placeholders := make([]string, len(itemIDs))
	args := make([]interface{}, len(itemIDs))
	for i, id := range itemIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	// Get all milestone IDs for these items in one query
	query := `
		SELECT milestone_id, COUNT(*) as freq
		FROM items
		WHERE id IN (` + strings.Join(placeholders, ",") + `)
		  AND milestone_id IS NOT NULL
		GROUP BY milestone_id
		ORDER BY freq DESC, milestone_id ASC
		LIMIT 3
	`

	rows, err := h.db.Query(query, args...)
	if err != nil {
		slog.Warn("error loading milestone frequencies", slog.String("component", "homepage"), slog.Any("error", err))
		return []int{}
	}
	defer rows.Close()

	var result []int
	for rows.Next() {
		var milestoneID int
		var freq int
		if err := rows.Scan(&milestoneID, &freq); err != nil {
			continue
		}
		result = append(result, milestoneID)
	}

	return result
}

// getMilestoneStatsBatch batch calculates progress statistics for multiple milestones
func (h *HomepageHandler) getMilestoneStatsBatch(milestoneIDs []int) ([]MilestoneProgress, error) {
	if len(milestoneIDs) == 0 {
		return []MilestoneProgress{}, nil
	}

	// Build placeholder string for IN query
	placeholders := make([]string, len(milestoneIDs))
	args := make([]interface{}, len(milestoneIDs))
	for i, id := range milestoneIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	// Get milestone details and statistics in one query
	query := `
		SELECT
			m.id,
			m.name,
			m.target_date,
			mc.color,
			COUNT(i.id) as total_items,
			SUM(CASE WHEN LOWER(sc.name) = 'done' THEN 1 ELSE 0 END) as done_items
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		LEFT JOIN items i ON i.milestone_id = m.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE m.id IN (` + strings.Join(placeholders, ",") + `)
		GROUP BY m.id, m.name, m.target_date, mc.color
		ORDER BY m.id
	`

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []MilestoneProgress
	for rows.Next() {
		var progress MilestoneProgress
		var targetDate sql.NullString
		var categoryColor sql.NullString
		var doneItems sql.NullInt64

		err := rows.Scan(
			&progress.MilestoneID,
			&progress.MilestoneName,
			&targetDate,
			&categoryColor,
			&progress.TotalItems,
			&doneItems,
		)
		if err != nil {
			continue
		}

		if targetDate.Valid {
			progress.TargetDate = &targetDate.String
		}
		if categoryColor.Valid {
			progress.CategoryColor = categoryColor.String
		}
		if doneItems.Valid {
			progress.DoneItems = int(doneItems.Int64)
		}
		progress.NotDoneItems = progress.TotalItems - progress.DoneItems

		// Calculate percentage
		if progress.TotalItems > 0 {
			progress.PercentComplete = float64(progress.DoneItems) / float64(progress.TotalItems) * 100.0
		}

		results = append(results, progress)
	}

	return results, rows.Err()
}
