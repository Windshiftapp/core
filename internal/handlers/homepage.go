package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// HomepageHandler handles homepage-related HTTP requests
type HomepageHandler struct {
	workspaceRepo      *repository.WorkspaceRepository
	itemRepo           *repository.ItemRepository
	itemCRUD           *services.ItemCRUDService
	planningService    *services.PlanningService
	activityTracker    *services.ActivityTracker
	permService        *services.PermissionService
	preferencesService *services.UserPreferencesService
}

// NewHomepageHandler creates a new homepage handler
func NewHomepageHandler(workspaceRepo *repository.WorkspaceRepository, itemRepo *repository.ItemRepository, itemCRUD *services.ItemCRUDService, planningService *services.PlanningService, activityTracker *services.ActivityTracker, permService *services.PermissionService, preferencesService *services.UserPreferencesService) *HomepageHandler {
	return &HomepageHandler{
		workspaceRepo:      workspaceRepo,
		itemRepo:           itemRepo,
		itemCRUD:           itemCRUD,
		planningService:    planningService,
		activityTracker:    activityTracker,
		permService:        permService,
		preferencesService: preferencesService,
	}
}

// HomepageData represents the comprehensive data for the user's homepage
type HomepageData struct {
	RecentWorkspaces    []WorkspaceActivity        `json:"recent_workspaces"`
	TotalWorkspaceCount int                        `json:"total_workspace_count"`
	TotalItemCount      int                        `json:"total_item_count"`
	RecentlyViewed      []ItemActivity             `json:"recently_viewed"`
	RecentlyEdited      []ItemActivity             `json:"recently_edited"`
	RecentlyCommented   []ItemActivity             `json:"recently_commented"`
	WatchedItems        []ItemActivity             `json:"watched_items"`
	UpcomingMilestones  []MilestoneProgress        `json:"upcoming_milestones"`
	Layout              models.UserDashboardLayout `json:"layout"`
	LayoutRevision      string                     `json:"layout_revision"`
}

// WorkspaceActivity represents a workspace visit with metadata
type WorkspaceActivity struct {
	WorkspaceID   int    `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	WorkspaceKey  string `json:"workspace_key"`
	Icon          string `json:"icon"`
	Color         string `json:"color"`
	AvatarURL     string `json:"avatar_url,omitempty"`
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
	StatusBuiltinKey    string  `json:"status_builtin_key,omitempty"`
	StatusColor         *string `json:"status_color,omitempty"`
	PriorityID          *int    `json:"priority_id,omitempty"`
	PriorityName        *string `json:"priority_name,omitempty"`
	PriorityBuiltinKey  string  `json:"priority_builtin_key,omitempty"`
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
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Get user activity from ActivityTracker
	if h.activityTracker == nil {
		respondInternalError(w, r, fmt.Errorf("activity tracker not available"))
		return
	}

	userActivity, err := h.activityTracker.GetUserActivity(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	accessibleWorkspaceIDs, err := GetAccessibleWorkspaceIDs(user, nil, h.permService)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to resolve accessible workspaces: %w", err))
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
		Layout: models.UserDashboardLayout{
			Sections: []models.UserDashboardSection{},
			Widgets:  []models.UserDashboardWidget{},
		},
	}

	// Dashboard layout is part of the route snapshot so homepage entry does not
	// need a second request. A layout read failure is non-critical: the client
	// can still render its default layout around the rest of the homepage data.
	if h.preferencesService != nil {
		layout, revision, layoutErr := h.preferencesService.GetDashboardLayoutSnapshot(user.ID)
		if layoutErr != nil {
			slog.Warn("error loading dashboard layout", slog.String("component", "homepage"), slog.Any("error", layoutErr))
		} else {
			homepageData.Layout = layout
			homepageData.LayoutRevision = revision
		}
	}

	// Homepage totals cover only the workspaces visible in this request's
	// permission snapshot.
	workspaceCount, err := h.workspaceRepo.CountNonPersonalByIDs(accessibleWorkspaceIDs)
	if err != nil {
		slog.Warn("error getting workspace count", slog.String("component", "homepage"), slog.Any("error", err))
		// Continue even if count fails - not critical
	} else {
		homepageData.TotalWorkspaceCount = workspaceCount
	}

	itemCount, err := h.itemRepo.CountNonPersonalByWorkspaceIDs(accessibleWorkspaceIDs)
	if err != nil {
		slog.Warn("error getting item count", slog.String("component", "homepage"), slog.Any("error", err))
		// Continue even if count fails - not critical
	} else {
		homepageData.TotalItemCount = itemCount
	}

	// Batch load workspace details for recent visits
	if len(userActivity.WorkspaceVisits) > 0 {
		workspaceActivities, err := h.getWorkspaceActivitiesBatch(userActivity.WorkspaceVisits, accessibleWorkspaceIDs)
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

	// Load all feed items through the same workspace-scoped list query used by
	// the item collection surface. Revoked and deleted items are absent from the
	// result map, so every feed below inherits the same visibility contract.
	var authorizedItems []models.Item
	if len(allItemActivities) > 0 {
		itemDetails, items, err := h.getItemActivitiesBatch(r.Context(), allItemActivities, accessibleWorkspaceIDs)
		if err != nil {
			slog.Warn("error batch loading items", slog.String("component", "homepage"), slog.Any("error", err))
		} else {
			authorizedItems = items
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
				item, exists := itemDetails[itemID]
				if !exists {
					continue
				}
				homepageData.WatchedItems = append(homepageData.WatchedItems, item)
			}
		}
	}

	// Load upcoming milestones based on user's recent activity - now uses batch approach
	milestoneIDs := getUpcomingMilestones(authorizedItems, 3)
	if len(milestoneIDs) > 0 {
		milestoneStats, err := h.getMilestoneStatsBatch(milestoneIDs, accessibleWorkspaceIDs)
		if err != nil {
			slog.Warn("error loading milestone stats", slog.String("component", "homepage"), slog.Any("error", err))
		} else {
			homepageData.UpcomingMilestones = milestoneStats
		}
	}

	respondJSONOK(w, homepageData)
}

// getWorkspaceActivitiesBatch loads current metadata only for visible visits.
func (h *HomepageHandler) getWorkspaceActivitiesBatch(visits []services.WorkspaceVisit, accessibleWorkspaceIDs []int) ([]WorkspaceActivity, error) {
	if len(visits) == 0 || len(accessibleWorkspaceIDs) == 0 {
		return []WorkspaceActivity{}, nil
	}

	allowed := make(map[int]struct{}, len(accessibleWorkspaceIDs))
	for _, workspaceID := range accessibleWorkspaceIDs {
		allowed[workspaceID] = struct{}{}
	}

	workspaceIDs := make([]int, 0, len(visits))
	visitMap := make(map[int]services.WorkspaceVisit)
	for _, visit := range visits {
		if _, ok := allowed[visit.WorkspaceID]; !ok {
			continue
		}
		workspaceIDs = append(workspaceIDs, visit.WorkspaceID)
		visitMap[visit.WorkspaceID] = visit
	}
	if len(workspaceIDs) == 0 {
		return []WorkspaceActivity{}, nil
	}

	basics, err := h.workspaceRepo.FindBasicsByIDs(workspaceIDs)
	if err != nil {
		return nil, err
	}

	activities := make([]WorkspaceActivity, 0, len(basics))
	for _, wb := range basics {
		activity := WorkspaceActivity{
			WorkspaceID:   wb.ID,
			WorkspaceName: wb.Name,
			WorkspaceKey:  wb.Key,
			Icon:          wb.Icon,
			Color:         wb.Color,
			AvatarURL:     wb.AvatarURL,
		}
		if visit, ok := visitMap[wb.ID]; ok {
			activity.LastVisited = visit.VisitedAt.Format("2006-01-02T15:04:05Z07:00")
			activity.VisitCount = visit.VisitCount
		}
		activities = append(activities, activity)
	}

	return activities, nil
}

// getItemActivitiesBatch loads item details through the canonical item list.
func (h *HomepageHandler) getItemActivitiesBatch(ctx context.Context, activities map[int]services.ItemActivity, workspaceIDs []int) (map[int]ItemActivity, []models.Item, error) {
	if len(activities) == 0 {
		return map[int]ItemActivity{}, []models.Item{}, nil
	}
	if h.itemCRUD == nil {
		return nil, nil, fmt.Errorf("item query service not available")
	}

	// Build item ID list
	itemIDs := make([]int, 0, len(activities))
	for id := range activities {
		itemIDs = append(itemIDs, id)
	}

	items, err := h.itemCRUD.ListByIDsContext(ctx, itemIDs, workspaceIDs)
	if err != nil {
		return nil, nil, err
	}

	result := make(map[int]ItemActivity, len(items))
	for _, item := range items {
		itemActivity := ItemActivity{
			ItemID:              item.ID,
			WorkspaceID:         item.WorkspaceID,
			WorkspaceItemNumber: item.WorkspaceItemNumber,
			Title:               item.Title,
			Status:              item.StatusName,
			StatusBuiltinKey:    item.StatusBuiltinKey,
			PriorityID:          item.PriorityID,
			PriorityBuiltinKey:  item.PriorityBuiltinKey,
			WorkspaceKey:        item.WorkspaceKey,
		}
		if itemActivity.Status == "" {
			itemActivity.Status = "Unknown"
		}
		itemActivity.StatusColor = optionalString(item.StatusColor)
		itemActivity.PriorityName = optionalString(item.PriorityName)
		itemActivity.PriorityColor = optionalString(item.PriorityColor)

		if activity, ok := activities[itemActivity.ItemID]; ok {
			if !activity.ActivityAt.IsZero() {
				itemActivity.LastActivity = activity.ActivityAt.Format("2006-01-02T15:04:05Z07:00")
			}
			itemActivity.ActivityCount = activity.ActivityCount
		}

		result[itemActivity.ItemID] = itemActivity
	}

	return result, items, nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// getUpcomingMilestones returns the most common milestones attached to the
// authorized feed items, ordered deterministically.
func getUpcomingMilestones(items []models.Item, limit int) []int {
	if len(items) == 0 || limit <= 0 {
		return []int{}
	}

	frequencies := make(map[int]int)
	for _, item := range items {
		for _, milestone := range item.Milestones {
			frequencies[milestone.ID]++
		}
	}
	ids := make([]int, 0, len(frequencies))
	for id := range frequencies {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if frequencies[ids[i]] == frequencies[ids[j]] {
			return ids[i] < ids[j]
		}
		return frequencies[ids[i]] > frequencies[ids[j]]
	})
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids
}

// getMilestoneStatsBatch reuses the workspace-scoped planning progress query.
func (h *HomepageHandler) getMilestoneStatsBatch(milestoneIDs, workspaceIDs []int) ([]MilestoneProgress, error) {
	if h.planningService == nil {
		return nil, fmt.Errorf("planning service not available")
	}

	results := make([]MilestoneProgress, 0, len(milestoneIDs))
	for _, milestoneID := range milestoneIDs {
		report, err := h.planningService.GetMilestoneProgress(milestoneID, workspaceIDs)
		if err != nil {
			return nil, err
		}
		progress := MilestoneProgress{
			MilestoneID:     report.MilestoneID,
			MilestoneName:   report.MilestoneName,
			TargetDate:      report.TargetDate,
			CategoryColor:   report.CategoryColor,
			TotalItems:      report.TotalItems,
			DoneItems:       report.CompletedItems,
			NotDoneItems:    report.TotalItems - report.CompletedItems,
			PercentComplete: report.PercentComplete,
		}
		results = append(results, progress)
	}

	return results, nil
}
