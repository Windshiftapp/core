package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/cql"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// WorkspaceStats represents comprehensive statistics for a workspace
type WorkspaceStats struct {
	TotalCollections       int                       `json:"total_collections"`
	ItemsByStatusCategory  map[string]int            `json:"items_by_status_category"`
	TotalItems             int                       `json:"total_items"`
	AssignmentDistribution []AssignmentStats         `json:"assignment_distribution"`
	ProjectStatistics      []ProjectStats            `json:"project_statistics"`
	PriorityBreakdown      map[string]int            `json:"priority_breakdown"`
	MilestoneProgress      []MilestoneStatusProgress `json:"milestone_progress"`
}

// AssignmentStats represents the distribution of items per assignee
type AssignmentStats struct {
	UserID       *int   `json:"user_id"`
	UserName     string `json:"user_name"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	ItemCount    int    `json:"item_count"`
	IsUnassigned bool   `json:"is_unassigned"`
}

// ProjectStats represents statistics for a specific project
type ProjectStats struct {
	ProjectID         *int    `json:"project_id"`
	ProjectName       string  `json:"project_name"`
	ProjectColor      string  `json:"project_color,omitempty"`
	ItemCount         int     `json:"item_count"`
	CompletedCount    int     `json:"completed_count"`
	CompletionPercent float64 `json:"completion_percent"`
}

// MilestoneStatusBreakdown represents the distribution of items per status category within a milestone
type MilestoneStatusBreakdown struct {
	CategoryName       string `json:"category_name"`
	CategoryBuiltinKey string `json:"category_builtin_key,omitempty"`
	CategoryColor      string `json:"category_color,omitempty"`
	ItemCount          int    `json:"item_count"`
	IsCompleted        bool   `json:"is_completed"`
}

// MilestoneStatusProgress aggregates milestone progress for a workspace
type MilestoneStatusProgress struct {
	MilestoneID     int                        `json:"milestone_id"`
	MilestoneName   string                     `json:"milestone_name"`
	TargetDate      *string                    `json:"target_date,omitempty"`
	Status          string                     `json:"status,omitempty"`
	CategoryColor   string                     `json:"category_color,omitempty"`
	TotalItems      int                        `json:"total_items"`
	CompletedItems  int                        `json:"completed_items"`
	PercentComplete float64                    `json:"percent_complete"`
	StatusBreakdown []MilestoneStatusBreakdown `json:"status_breakdown"`
}

// GetStats handles GET /api/workspaces/{id}/stats - returns comprehensive workspace statistics
func (h *WorkspaceHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	// Get workspace ID from URL
	workspaceID, ok := requireWorkspaceIDParam(w, r, h.keyCache, "id")
	if !ok {
		return
	}

	authUser, authOK := RequireAuth(w, r)
	if !authOK {
		return
	}

	queryParams := r.URL.Query()
	vqlQuery := queryParams.Get("ql")
	if vqlQuery == "" {
		vqlQuery = queryParams.Get("vql")
	}
	if vqlQuery == "" {
		vqlQuery = queryParams.Get("cql")
	}

	// Support filtering via collection_id by reusing its CQL query
	if vqlQuery == "" {
		if collectionParam := queryParams.Get("collection_id"); collectionParam != "" {
			collectionID, err := strconv.Atoi(collectionParam)
			if err != nil {
				respondInvalidID(w, r, "collection_id")
				return
			}

			collectionWorkspaceID, collectionQuery, err := h.repo.GetCollectionQuery(collectionID)
			if errors.Is(err, repository.ErrNotFound) {
				respondNotFound(w, r, "collection")
				return
			}
			if err != nil {
				respondInternalError(w, r, err)
				return
			}

			if collectionWorkspaceID != nil && *collectionWorkspaceID != int64(workspaceID) {
				respondBadRequest(w, r, "Collection does not belong to this workspace")
				return
			}

			if strings.TrimSpace(collectionQuery) != "" {
				vqlQuery = collectionQuery
			}
		}
	}

	var filterSQL string
	var filterArgs []any
	if strings.TrimSpace(vqlQuery) != "" {
		workspaceMap, err := h.buildWorkspaceMap()
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		customFieldMap, cfErr := repository.NewItemRepository(h.db).GetCQLCustomFieldMap()
		if cfErr != nil {
			respondInternalError(w, r, cfErr)
			return
		}
		evaluator := cql.NewEvaluator(workspaceMap, customFieldMap, h.db.GetDriverName())
		resolvedQuery := cql.SubstituteFunctions(vqlQuery, cql.UserContext(authUser.ID))
		filterSQL, filterArgs, err = evaluator.EvaluateToSQL(resolvedQuery)
		if err != nil {
			respondBadRequest(w, r, "VQL query error: "+err.Error())
			return
		}
	}

	stats := WorkspaceStats{
		ItemsByStatusCategory:  make(map[string]int),
		AssignmentDistribution: []AssignmentStats{},
		ProjectStatistics:      []ProjectStats{},
		PriorityBreakdown:      make(map[string]int),
		MilestoneProgress:      []MilestoneStatusProgress{},
	}

	// 1. Get total collections count
	collectionCount, err := h.repo.CountCollections(workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	stats.TotalCollections = collectionCount + 1 // +1 for default collection

	// 2-6. Run all five item aggregations through ItemRepository so the SQL
	// for NULL handling, filter composition, and since-window lives in one
	// place.
	sinceCutoff := time.Now().AddDate(0, 0, -30)
	itemStats, err := repository.NewItemRepository(h.db).ComputeWorkspaceItemStats(workspaceID, filterSQL, filterArgs, sinceCutoff)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	stats.TotalItems = itemStats.TotalItems
	stats.ItemsByStatusCategory = itemStats.ItemsByStatusCategory
	stats.PriorityBreakdown = itemStats.PriorityBreakdown
	stats.AssignmentDistribution = make([]AssignmentStats, 0, len(itemStats.AssignmentDistribution))
	for _, a := range itemStats.AssignmentDistribution {
		stats.AssignmentDistribution = append(stats.AssignmentDistribution, AssignmentStats{
			UserID:       a.UserID,
			UserName:     a.UserName,
			FirstName:    a.FirstName,
			LastName:     a.LastName,
			ItemCount:    a.ItemCount,
			IsUnassigned: a.UserID == nil,
		})
	}
	// Determine which time projects the caller may see by name. nil = full
	// access (no masking); otherwise only the listed project IDs keep names.
	var allowedProjects map[int]struct{}
	if accessible, accErr := services.NewTimePermissionService(h.db, h.permissionService).GetAccessibleProjects(authUser.ID); accErr != nil {
		slog.Warn("failed to load accessible projects for stats masking", slog.Int("user_id", authUser.ID), slog.Any("error", accErr))
		allowedProjects = map[int]struct{}{} // fail closed: mask all names
	} else if accessible != nil {
		allowedProjects = make(map[int]struct{}, len(accessible))
		for _, id := range accessible {
			allowedProjects[id] = struct{}{}
		}
	}

	stats.ProjectStatistics = make([]ProjectStats, 0, len(itemStats.ProjectStatistics))
	for _, p := range itemStats.ProjectStatistics {
		project := ProjectStats{
			ProjectID:      p.ProjectID,
			ProjectName:    p.ProjectName,
			ProjectColor:   p.ProjectColor,
			ItemCount:      p.ItemCount,
			CompletedCount: p.CompletedCount,
		}
		// Strip the name/color of a restricted project the caller can't view,
		// keeping the aggregate counts. allowedProjects == nil means full access.
		if allowedProjects != nil && p.ProjectID != nil {
			if _, ok := allowedProjects[*p.ProjectID]; !ok {
				project.ProjectName = ""
				project.ProjectColor = ""
			}
		}
		if project.ItemCount > 0 {
			project.CompletionPercent = float64(project.CompletedCount) / float64(project.ItemCount) * 100
		}
		stats.ProjectStatistics = append(stats.ProjectStatistics, project)
	}

	// 7. Load milestone progress for active milestones referenced in this workspace
	if repoProgress, mpErr := repository.NewPlanningRepository(h.db).GetWorkspaceMilestoneProgress(workspaceID, filterSQL, filterArgs); mpErr == nil {
		milestoneProgress := make([]MilestoneStatusProgress, len(repoProgress))
		for i, rp := range repoProgress {
			breakdowns := make([]MilestoneStatusBreakdown, len(rp.StatusBreakdown))
			for j, rb := range rp.StatusBreakdown {
				breakdowns[j] = MilestoneStatusBreakdown{
					CategoryName:       rb.CategoryName,
					CategoryBuiltinKey: rb.CategoryBuiltinKey,
					CategoryColor:      rb.CategoryColor,
					ItemCount:          rb.ItemCount,
					IsCompleted:        rb.IsCompleted,
				}
			}
			milestoneProgress[i] = MilestoneStatusProgress{
				MilestoneID:     rp.MilestoneID,
				MilestoneName:   rp.MilestoneName,
				TargetDate:      rp.TargetDate,
				Status:          rp.Status,
				CategoryColor:   rp.CategoryColor,
				TotalItems:      rp.TotalItems,
				CompletedItems:  rp.CompletedItems,
				PercentComplete: rp.PercentComplete,
				StatusBreakdown: breakdowns,
			}
		}
		stats.MilestoneProgress = milestoneProgress
	} else {
		respondInternalError(w, r, mpErr)
		return
	}

	respondJSONOK(w, stats)
}
