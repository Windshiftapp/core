package handlers

import (
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
)

// AnalyticsHandler handles analytics endpoints for workspaces.
type AnalyticsHandler struct {
	db                database.Database
	analyticsService  *services.AnalyticsService
	permissionService *services.PermissionService
	keyCache          *WorkspaceKeyCache
}

// NewAnalyticsHandler creates a new analytics handler.
func NewAnalyticsHandler(db database.Database, permissionService *services.PermissionService) *AnalyticsHandler {
	return &AnalyticsHandler{
		db:                db,
		analyticsService:  services.NewAnalyticsService(db),
		permissionService: permissionService,
		keyCache:          NewWorkspaceKeyCache(db),
	}
}

// GetAnalytics handles GET /workspaces/{id}/analytics
// Aggregated endpoint that resolves a dataset (collection or workspace) and computes all panels.
func (h *AnalyticsHandler) GetAnalytics(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireWorkspaceIDParam(w, r, h.keyCache, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !h.hasViewPermission(w, r, user.ID, workspaceID) {
		return
	}

	// Date range (default: last 30 days)
	now := time.Now()
	startDate := now.AddDate(0, 0, -30)
	endDate := now

	if s := r.URL.Query().Get("start_date"); s != "" {
		if parsed, err := time.Parse("2006-01-02", s); err == nil {
			startDate = parsed
		}
	}
	if e := r.URL.Query().Get("end_date"); e != "" {
		if parsed, err := time.Parse("2006-01-02", e); err == nil {
			endDate = parsed
		}
	}

	// Optional collection scope
	var collectionID int
	if cid := r.URL.Query().Get("collection_id"); cid != "" {
		if n, err := strconv.Atoi(cid); err == nil {
			collectionID = n
		}
	}

	// Optional direct QL query
	qlQuery := r.URL.Query().Get("ql")

	result, err := h.analyticsService.GetAnalytics(services.ResolveDatasetParams{
		WorkspaceID:  workspaceID,
		CollectionID: collectionID,
		QLQuery:      qlQuery,
		UserID:       user.ID,
		StartDate:    startDate,
		EndDate:      endDate,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, result)
}

func (h *AnalyticsHandler) hasViewPermission(w http.ResponseWriter, r *http.Request, userID, workspaceID int) bool {
	hasPerm, err := h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
	if err != nil || !hasPerm {
		respondNotFound(w, r, "Workspace")
		return false
	}
	return true
}
