package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/scm"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type MilestoneHandler struct {
	db                 database.Database
	permissionService  *services.PermissionService
	planningService    *services.PlanningService
	credentialResolver *scm.CredentialResolver
}

func NewMilestoneHandler(db database.Database, permissionService *services.PermissionService, credentialResolver *scm.CredentialResolver) *MilestoneHandler {
	return &MilestoneHandler{
		db:                 db,
		permissionService:  permissionService,
		planningService:    services.NewPlanningService(db),
		credentialResolver: credentialResolver,
	}
}

func (h *MilestoneHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Parse query parameters
	categoryIDStr := r.URL.Query().Get("category_id")
	status := r.URL.Query().Get("status")
	workspaceIDStr := r.URL.Query().Get("workspace_id")
	includeGlobal := r.URL.Query().Get("include_global") != "false" // Default to true

	// Check workspace permission if workspace_id is specified
	if workspaceIDStr != "" {
		if wsID, err := strconv.Atoi(workspaceIDStr); err == nil {
			if !RequireWorkspacePermission(w, r, user.ID, wsID, models.PermissionItemView, h.permissionService) {
				return
			}
		}
	} else {
		// For global-only milestones, check global milestone permission
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionMilestoneCreate)
		if err != nil || !hasGlobalPerm {
			respondForbidden(w, r)
			return
		}
	}

	// Build service params
	params := services.MilestoneListParams{
		Limit:         1000, // Large limit for backwards compatibility
		Offset:        0,
		IncludeGlobal: includeGlobal,
		Status:        status,
	}

	// Parse workspace ID
	if workspaceIDStr != "" {
		if wsID, err := strconv.Atoi(workspaceIDStr); err == nil {
			params.WorkspaceID = &wsID
		}
	}

	// Parse category ID
	if categoryIDStr != "" {
		if categoryIDStr == "null" || categoryIDStr == "0" {
			zero := 0
			params.CategoryID = &zero
		} else if catID, err := strconv.Atoi(categoryIDStr); err == nil {
			params.CategoryID = &catID
		}
	}

	// Use service to list milestones
	results, _, err := h.planningService.ListMilestones(params)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Convert service results to models for response
	milestones := make([]models.Milestone, 0, len(results))
	for _, r := range results {
		milestone := h.milestoneResultToModel(&r, user.ID)
		milestones = append(milestones, milestone)
	}

	respondJSONOK(w, milestones)
}

func (h *MilestoneHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Use service to get milestone
	result, err := h.planningService.GetMilestone(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondNotFound(w, r, "milestone")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check permission based on whether milestone is global or workspace-scoped
	if result.IsGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionMilestoneCreate)
		if err != nil || !hasGlobalPerm {
			respondForbidden(w, r)
			return
		}
	} else if result.WorkspaceID != nil {
		if !RequireWorkspacePermission(w, r, user.ID, *result.WorkspaceID, models.PermissionItemView, h.permissionService) {
			return
		}
	}

	milestone := h.milestoneResultToModel(result, user.ID)
	respondJSONOK(w, milestone)
}

func (h *MilestoneHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	var milestone models.Milestone
	if err := json.NewDecoder(r.Body).Decode(&milestone); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate required fields
	if strings.TrimSpace(milestone.Name) == "" {
		respondValidationError(w, r, "Milestone name is required")
		return
	}

	// Handle empty target_date (set to nil)
	if milestone.TargetDate != nil && strings.TrimSpace(*milestone.TargetDate) == "" {
		milestone.TargetDate = nil
	}

	// Validate status
	//nolint:misspell // British spelling is intentional for status value
	validStatuses := []string{"planning", "in-progress", "completed", "cancelled"}
	statusValid := false
	for _, validStatus := range validStatuses {
		if milestone.Status == validStatus {
			statusValid = true
			break
		}
	}
	if !statusValid {
		milestone.Status = "planning" // Default status
	}

	// Validate global vs workspace constraints
	if milestone.IsGlobal && milestone.WorkspaceID != nil {
		respondValidationError(w, r, "Global milestones cannot have a workspace_id")
		return
	}
	if !milestone.IsGlobal && milestone.WorkspaceID == nil {
		respondValidationError(w, r, "Local milestones must have a workspace_id")
		return
	}

	// Check permission based on whether milestone is global or workspace-scoped
	if milestone.IsGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionMilestoneCreate)
		if err != nil || !hasGlobalPerm {
			respondForbidden(w, r)
			return
		}
	} else if !RequireWorkspacePermission(w, r, user.ID, *milestone.WorkspaceID, models.PermissionItemEdit, h.permissionService) {
		return
	}

	// Validate category_id if provided (using service)
	if milestone.CategoryID != nil {
		exists, err := h.planningService.CategoryExists(*milestone.CategoryID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !exists {
			respondInvalidID(w, r, "category_id")
			return
		}
	}

	// Validate workspace_id if provided (using service)
	if milestone.WorkspaceID != nil {
		exists, err := h.planningService.WorkspaceExists(*milestone.WorkspaceID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !exists {
			respondInvalidID(w, r, "workspace_id")
			return
		}
	}

	// Sanitize user input to prevent XSS
	milestone.Name = utils.StripHTMLTags(milestone.Name)
	milestone.Description = utils.SanitizeCommentContent(milestone.Description)

	// Get target date as string
	targetDate := ""
	if milestone.TargetDate != nil {
		targetDate = *milestone.TargetDate
	}

	// Use service to create milestone
	result, err := h.planningService.CreateMilestone(services.CreateMilestoneParams{
		Name:        milestone.Name,
		Description: milestone.Description,
		TargetDate:  targetDate,
		Status:      milestone.Status,
		CategoryID:  milestone.CategoryID,
		IsGlobal:    milestone.IsGlobal,
		WorkspaceID: milestone.WorkspaceID,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	createdMilestone := h.milestoneResultToModel(result, user.ID)
	respondJSONCreated(w, createdMilestone)
}

func (h *MilestoneHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var milestone models.Milestone
	if err := json.NewDecoder(r.Body).Decode(&milestone); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate required fields
	if strings.TrimSpace(milestone.Name) == "" {
		respondValidationError(w, r, "Milestone name is required")
		return
	}

	// Handle empty target_date (set to nil)
	if milestone.TargetDate != nil && strings.TrimSpace(*milestone.TargetDate) == "" {
		milestone.TargetDate = nil
	}

	// Validate status
	//nolint:misspell // British spelling is intentional for status value
	validStatuses := []string{"planning", "in-progress", "completed", "cancelled"}
	statusValid := false
	for _, validStatus := range validStatuses {
		if milestone.Status == validStatus {
			statusValid = true
			break
		}
	}
	if !statusValid {
		respondValidationError(w, r, "Invalid status")
		return
	}

	// Validate global vs workspace constraints
	if milestone.IsGlobal && milestone.WorkspaceID != nil {
		respondValidationError(w, r, "Global milestones cannot have a workspace_id")
		return
	}
	if !milestone.IsGlobal && milestone.WorkspaceID == nil {
		respondValidationError(w, r, "Local milestones must have a workspace_id")
		return
	}

	// Check permission based on whether milestone is global or workspace-scoped
	if milestone.IsGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionMilestoneCreate)
		if err != nil || !hasGlobalPerm {
			respondForbidden(w, r)
			return
		}
	} else if !RequireWorkspacePermission(w, r, user.ID, *milestone.WorkspaceID, models.PermissionItemEdit, h.permissionService) {
		return
	}

	// Validate category_id if provided (using service)
	if milestone.CategoryID != nil {
		exists, err := h.planningService.CategoryExists(*milestone.CategoryID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !exists {
			respondInvalidID(w, r, "category_id")
			return
		}
	}

	// Validate workspace_id if provided (using service)
	if milestone.WorkspaceID != nil {
		exists, err := h.planningService.WorkspaceExists(*milestone.WorkspaceID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !exists {
			respondInvalidID(w, r, "workspace_id")
			return
		}
	}

	// Sanitize user input to prevent XSS
	milestone.Name = utils.StripHTMLTags(milestone.Name)
	milestone.Description = utils.SanitizeCommentContent(milestone.Description)

	// Get target date as string
	targetDate := ""
	if milestone.TargetDate != nil {
		targetDate = *milestone.TargetDate
	}

	// Use service to update milestone
	result, err := h.planningService.UpdateMilestone(services.UpdateMilestoneParams{
		ID:          id,
		Name:        milestone.Name,
		Description: milestone.Description,
		TargetDate:  targetDate,
		Status:      milestone.Status,
		CategoryID:  milestone.CategoryID,
		IsGlobal:    milestone.IsGlobal,
		WorkspaceID: milestone.WorkspaceID,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	updatedMilestone := h.milestoneResultToModel(result, user.ID)
	respondJSONOK(w, updatedMilestone)
}

func (h *MilestoneHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// First, fetch the milestone to check its properties for permission validation (using service)
	isGlobal, workspaceID, err := h.planningService.IsMilestoneGlobal(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondNotFound(w, r, "milestone")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check permission based on whether milestone is global or workspace-scoped
	if isGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionMilestoneCreate)
		if err != nil || !hasGlobalPerm {
			respondForbidden(w, r)
			return
		}
	} else if workspaceID != nil {
		if !RequireWorkspacePermission(w, r, user.ID, *workspaceID, models.PermissionItemEdit, h.permissionService) {
			return
		}
	}

	// Use service to delete milestone
	if err := h.planningService.DeleteMilestone(id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *MilestoneHandler) GetTestStatistics(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	milestoneID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// First, fetch the milestone to check its properties for permission validation (using service)
	isGlobal, workspaceID, err := h.planningService.IsMilestoneGlobal(milestoneID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondNotFound(w, r, "milestone")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check permission based on whether milestone is global or workspace-scoped
	if isGlobal {
		var hasGlobalPerm bool
		hasGlobalPerm, err = h.permissionService.HasGlobalPermission(user.ID, models.PermissionMilestoneCreate)
		if err != nil || !hasGlobalPerm {
			respondForbidden(w, r)
			return
		}
	} else if workspaceID != nil {
		if !RequireWorkspacePermission(w, r, user.ID, *workspaceID, models.PermissionItemView, h.permissionService) {
			return
		}
	}

	// Use service to get test statistics
	stats, err := h.planningService.GetMilestoneTestStatistics(milestoneID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, stats)
}

// GetProgress handles GET /api/milestones/{id}/progress - returns milestone progress report
func (h *MilestoneHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	milestoneID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// First check permission for this milestone (using service)
	isGlobal, workspaceID, err := h.planningService.IsMilestoneGlobal(milestoneID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondNotFound(w, r, "milestone")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check permission based on whether milestone is global or workspace-scoped
	if isGlobal {
		var hasGlobalPerm bool
		hasGlobalPerm, err = h.permissionService.HasGlobalPermission(user.ID, models.PermissionMilestoneCreate)
		if err != nil || !hasGlobalPerm {
			respondForbidden(w, r)
			return
		}
	} else if workspaceID != nil {
		if !RequireWorkspacePermission(w, r, user.ID, *workspaceID, models.PermissionItemView, h.permissionService) {
			return
		}
	}

	// Use service to get progress report
	report, err := h.planningService.GetMilestoneProgress(milestoneID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondNotFound(w, r, "milestone")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, report)
}

// milestoneResultToModel converts a MilestoneResult to a models.Milestone, applying SCM field
// visibility rules: scm_connection_id and scm_repository on the release are redacted unless the
// user has workspace access to the connection's workspace.
func (h *MilestoneHandler) milestoneResultToModel(r *services.MilestoneResult, userID int) models.Milestone {
	milestone := models.Milestone{
		ID:            r.ID,
		Name:          r.Name,
		Description:   r.Description,
		Status:        r.Status,
		CategoryID:    r.CategoryID,
		CategoryName:  r.CategoryName,
		CategoryColor: r.CategoryColor,
		IsGlobal:      r.IsGlobal,
		WorkspaceID:   r.WorkspaceID,
		WorkspaceName: r.WorkspaceName,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
	if r.TargetDate != "" {
		milestone.TargetDate = &r.TargetDate
	}

	if r.LatestRelease != nil {
		rel := &models.MilestoneRelease{
			ID:              r.LatestRelease.ID,
			MilestoneID:     r.LatestRelease.MilestoneID,
			TagName:         r.LatestRelease.TagName,
			Name:            r.LatestRelease.Name,
			Body:            r.LatestRelease.Body,
			IsDraft:         r.LatestRelease.IsDraft,
			IsPrerelease:    r.LatestRelease.IsPrerelease,
			TargetCommitish: r.LatestRelease.TargetCommitish,
			SCMReleaseID:    r.LatestRelease.SCMReleaseID,
			SCMReleaseURL:   r.LatestRelease.SCMReleaseURL,
			CreatedBy:       r.LatestRelease.CreatedBy,
			CreatedAt:       r.LatestRelease.CreatedAt,
		}
		// Expose scm_connection_id and scm_repository only if user has access to the connection's workspace
		if r.LatestRelease.SCMConnectionID != nil {
			connectionWorkspaceID, err := h.planningService.GetSCMConnectionWorkspaceID(*r.LatestRelease.SCMConnectionID)
			if err == nil && connectionWorkspaceID > 0 {
				hasPerm, permErr := h.permissionService.HasWorkspacePermission(userID, connectionWorkspaceID, models.PermissionItemEdit)
				if permErr == nil && hasPerm {
					rel.SCMConnectionID = r.LatestRelease.SCMConnectionID
					rel.SCMRepository = r.LatestRelease.SCMRepository
				}
			}
		}
		milestone.LatestRelease = rel
	}

	return milestone
}

// releaseRequest is the request body for the Release endpoint.
type releaseRequest struct {
	ConnectionID    int    `json:"connection_id"`
	Repository      string `json:"repository"`       // "owner/repo"
	TagName         string `json:"tag_name"`
	Name            string `json:"name"`
	Body            string `json:"body"`
	IsDraft         bool   `json:"is_draft"`
	IsPrerelease    bool   `json:"is_prerelease"`
	TargetCommitish string `json:"target_commitish"` // optional branch or SHA
}

// Release handles POST /milestones/{id}/release — creates an SCM release and marks the milestone completed.
func (h *MilestoneHandler) Release(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Load the milestone to determine its scope for permission checking
	isGlobal, workspaceID, err := h.planningService.IsMilestoneGlobal(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondNotFound(w, r, "milestone")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Verify the user can mutate this milestone
	if isGlobal {
		hasGlobalPerm, permErr := h.permissionService.HasGlobalPermission(user.ID, models.PermissionMilestoneCreate)
		if permErr != nil || !hasGlobalPerm {
			respondForbidden(w, r)
			return
		}
	} else if workspaceID != nil {
		if !RequireWorkspacePermission(w, r, user.ID, *workspaceID, models.PermissionItemEdit, h.permissionService) {
			return
		}
	}

	var req releaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.TagName == "" {
		respondValidationError(w, r, "tag_name is required")
		return
	}

	// If a connection_id was provided, verify the user also has access to that connection's workspace
	var scmConnectionID *int
	var scmRepository *string
	var scmReleaseID *string
	var scmReleaseURL *string

	if req.ConnectionID > 0 {
		if req.Repository == "" {
			respondValidationError(w, r, "repository is required when connection_id is provided")
			return
		}

		connectionWorkspaceID, wsErr := h.planningService.GetSCMConnectionWorkspaceID(req.ConnectionID)
		if wsErr != nil || connectionWorkspaceID == 0 {
			respondBadRequest(w, r, "SCM connection not found")
			return
		}
		if !RequireWorkspacePermission(w, r, user.ID, connectionWorkspaceID, models.PermissionItemEdit, h.permissionService) {
			return
		}

		if h.credentialResolver != nil {
			// Load the SCM provider
			provider, provErr := h.credentialResolver.GetProviderForConnection(r.Context(), req.ConnectionID)
			if provErr != nil {
				respondBadRequest(w, r, "Failed to load SCM provider: "+provErr.Error())
				return
			}

			// Ensure the provider supports releases
			releaseProvider, supportsReleases := provider.(scm.ReleaseProvider)
			if !supportsReleases {
				respondBadRequest(w, r, "This SCM provider does not support releases")
				return
			}

			// Parse "owner/repo"
			parts := strings.SplitN(req.Repository, "/", 2)
			if len(parts) != 2 {
				respondValidationError(w, r, "repository must be in 'owner/repo' format")
				return
			}
			owner, repo := parts[0], parts[1]

			// Create the release on the SCM provider
			release, releaseErr := releaseProvider.CreateRelease(r.Context(), owner, repo, scm.CreateReleaseOptions{
				TagName:         req.TagName,
				TargetCommitish: req.TargetCommitish,
				Name:            req.Name,
				Body:            req.Body,
				IsDraft:         req.IsDraft,
				IsPrerelease:    req.IsPrerelease,
			})
			if releaseErr != nil {
				respondBadRequest(w, r, "Failed to create SCM release: "+releaseErr.Error())
				return
			}

			cid := req.ConnectionID
			scmConnectionID = &cid
			repoStr := req.Repository
			scmRepository = &repoStr
			scmReleaseID = &release.ID
			scmReleaseURL = &release.URL
		}
	}

	createdBy := user.ID

	// Persist the release record and mark milestone as completed
	result, err := h.planningService.ReleaseMilestone(services.ReleaseMilestoneParams{
		ID:              id,
		TagName:         req.TagName,
		Name:            req.Name,
		Body:            req.Body,
		IsDraft:         req.IsDraft,
		IsPrerelease:    req.IsPrerelease,
		TargetCommitish: req.TargetCommitish,
		SCMConnectionID: scmConnectionID,
		SCMRepository:   scmRepository,
		SCMReleaseID:    scmReleaseID,
		SCMReleaseURL:   scmReleaseURL,
		CreatedBy:       &createdBy,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	milestone := h.milestoneResultToModel(result, user.ID)
	respondJSONOK(w, milestone)
}
