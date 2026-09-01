package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/logger"
	"windshift/internal/models"
	repositorypkg "windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/scm"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type MilestoneHandler struct {
	permissionService      *services.PermissionService
	planningService        *services.PlanningService
	resolveReleaseProvider func(context.Context, int, int) (milestoneReleaseProvider, error)
	auditor                *logger.Auditor
}

type milestoneReleaseProvider interface {
	CreateRelease(context.Context, string, string, scm.CreateReleaseOptions) (*scm.Release, error)
	ListReleases(context.Context, string, string) ([]scm.Release, error)
	ListTags(context.Context, string, string, time.Time) ([]scm.Tag, error)
}

func NewMilestoneHandler(planningService *services.PlanningService, permissionService *services.PermissionService, credentialResolver *scm.CredentialResolver, auditor *logger.Auditor) *MilestoneHandler {
	handler := &MilestoneHandler{
		permissionService: permissionService,
		planningService:   planningService,
		auditor:           auditor,
	}
	if credentialResolver != nil {
		handler.resolveReleaseProvider = func(ctx context.Context, connectionID, userID int) (milestoneReleaseProvider, error) {
			provider, err := credentialResolver.GetProviderForUser(ctx, connectionID, userID)
			if err != nil {
				return nil, err
			}
			releaseProvider, ok := provider.(milestoneReleaseProvider)
			if !ok {
				return nil, errors.New("SCM provider does not support releases")
			}
			return releaseProvider, nil
		}
	}
	return handler
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
	// Check workspace permission if workspace_id is specified.
	// No workspace_id: allow any authenticated user to list global milestones.
	// Write operations (create/update/delete) still require PermissionMilestoneCreate.
	if workspaceIDStr != "" {
		if wsID, err := strconv.Atoi(workspaceIDStr); err == nil {
			if !RequireWorkspacePermission(w, r, user.ID, wsID, models.PermissionItemView, h.permissionService) {
				return
			}
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
		if errors.Is(err, repositorypkg.ErrNotFound) {
			respondNotFound(w, r, "milestone")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check permission based on whether milestone is global or workspace-scoped
	if result.IsGlobal {
		// All authenticated users can view global milestones
	} else if result.WorkspaceID != nil {
		if !RequireWorkspacePermission(w, r, user.ID, *result.WorkspaceID, models.PermissionItemView, h.permissionService) {
			return
		}
	}

	milestone := h.milestoneResultToModel(result, user.ID)

	// Populate all releases for the detail view
	releaseResults, err := h.planningService.ListMilestoneReleases(id)
	if err == nil && len(releaseResults) > 0 {
		releases := make([]models.MilestoneRelease, 0, len(releaseResults))
		for _, rr := range releaseResults {
			rel := models.MilestoneRelease{
				ID:                    rr.ID,
				MilestoneID:           rr.MilestoneID,
				WorkspaceRepositoryID: rr.WorkspaceRepositoryID,
				TagName:               rr.TagName,
				TagURL:                rr.TagURL,
				ReleaseStatus:         rr.ReleaseStatus,
				ReleasedAt:            rr.ReleasedAt,
				Assets:                rr.Assets,
				LastSyncedAt:          rr.LastSyncedAt,
				Name:                  rr.Name,
				Body:                  rr.Body,
				IsDraft:               rr.IsDraft,
				IsPrerelease:          rr.IsPrerelease,
				TargetCommitish:       rr.TargetCommitish,
				SCMReleaseID:          rr.SCMReleaseID,
				SCMReleaseURL:         rr.SCMReleaseURL,
				CreatedBy:             rr.CreatedBy,
				CreatedAt:             rr.CreatedAt,
			}
			// Apply same SCM field visibility rules as milestoneResultToModel
			if rr.SCMConnectionID != nil {
				connectionWorkspaceID, err := h.planningService.GetSCMConnectionWorkspaceID(*rr.SCMConnectionID)
				if err == nil && connectionWorkspaceID > 0 {
					hasPerm, permErr := h.permissionService.HasWorkspacePermission(user.ID, connectionWorkspaceID, models.PermissionItemEdit)
					if permErr == nil && hasPerm {
						rel.SCMConnectionID = rr.SCMConnectionID
						rel.SCMRepository = rr.SCMRepository
					}
				}
			}
			releases = append(releases, rel)
		}
		milestone.Releases = releases
	}

	respondJSONOK(w, milestone)
}

// decodeMilestoneInput decodes a milestone from the request body, validates required fields,
// normalizes target_date, and validates the status. Returns the milestone and ok=true on success.
func decodeMilestoneInput(w http.ResponseWriter, r *http.Request, defaultStatus bool) (models.Milestone, bool) {
	milestone, ok := decodeJSON[models.Milestone](w, r)
	if !ok {
		return milestone, false
	}

	if strings.TrimSpace(milestone.Name) == "" {
		respondValidationError(w, r, "Milestone name is required")
		return milestone, false
	}

	if milestone.TargetDate != nil && strings.TrimSpace(*milestone.TargetDate) == "" {
		milestone.TargetDate = nil
	}

	if !isValidMilestoneStatus(milestone.Status) {
		if defaultStatus {
			milestone.Status = "planning"
		} else {
			respondValidationError(w, r, "Invalid status")
			return milestone, false
		}
	}

	return milestone, true
}

func (h *MilestoneHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	milestone, ok := decodeMilestoneInput(w, r, true)
	if !ok {
		return
	}

	// Validate constraints, permissions, and FK references
	if !h.validateMilestoneConstraints(w, r, &milestone, user.ID) {
		return
	}

	// Sanitize user input to prevent XSS
	milestone.Name = sanitize.PlainTextField.Sanitize(milestone.Name)
	milestone.Description = sanitize.Comment.Sanitize(milestone.Description)

	// Use service to create milestone
	auditActor := services.NewAuditActorFromRequest(r, user, nil, "cookie")
	result, err := h.planningService.CreateMilestone(services.CreateMilestoneParams{
		Name:        milestone.Name,
		Description: milestone.Description,
		TargetDate:  milestone.TargetDate,
		Status:      milestone.Status,
		CategoryID:  milestone.CategoryID,
		IsGlobal:    milestone.IsGlobal,
		WorkspaceID: milestone.WorkspaceID,
		AuditActor:  &auditActor,
	})
	if err != nil {
		if respondPlanningValidationError(w, r, err) {
			return
		}
		respondInternalError(w, r, err)
		return
	}

	createdMilestone := h.milestoneResultToModel(result, user.ID)
	respondJSONCreated(w, createdMilestone)
}

// Update handles both PUT /workspaces/{workspaceId}/milestones/{id} and
// PUT /global/milestones/{id}. The scope is derived from the URL — body-supplied
// workspace_id / is_global are ignored. Permission has already been gated by
// route middleware (workspaceItemEdit / globalMilestoneManage); the SQL UPDATE
// additionally constrains by scope so a milestone in a different workspace can
// never be touched by this request.
func (h *MilestoneHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Scope comes from the URL only.
	var workspaceID *int
	if wsStr := r.PathValue("workspaceId"); wsStr != "" {
		ws, err := strconv.Atoi(wsStr)
		if err != nil {
			respondValidationError(w, r, "Invalid workspaceId")
			return
		}
		workspaceID = &ws
	}

	milestone, ok := decodeMilestoneInput(w, r, false)
	if !ok {
		return
	}

	// Validate category_id if provided
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

	// Sanitize user input to prevent XSS
	milestone.Name = sanitize.PlainTextField.Sanitize(milestone.Name)
	milestone.Description = sanitize.Comment.Sanitize(milestone.Description)

	auditActor := services.NewAuditActorFromRequest(r, user, nil, "cookie")
	result, err := h.planningService.UpdateMilestone(services.UpdateMilestoneParams{
		ID:          id,
		Name:        milestone.Name,
		Description: milestone.Description,
		TargetDate:  milestone.TargetDate,
		Status:      milestone.Status,
		CategoryID:  milestone.CategoryID,
		WorkspaceID: workspaceID,
		AuditActor:  &auditActor,
	})
	if err != nil {
		if respondPlanningValidationError(w, r, err) {
			return
		}
		if errors.Is(err, repositorypkg.ErrNotFound) {
			respondNotFound(w, r, "milestone")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	updatedMilestone := h.milestoneResultToModel(result, user.ID)
	respondJSONOK(w, updatedMilestone)
}

// reorderRequest is the body for the milestone reorder endpoints.
// ordered_ids is the full, in-scope ordering of milestone ids after the
// drag; category_id optionally narrows the scope to a single category.
type reorderRequest struct {
	OrderedIDs []int `json:"ordered_ids"`
	CategoryID *int  `json:"category_id,omitempty"`
}

// Reorder handles POST /workspaces/{workspaceId}/milestones/reorder and
// POST /global/milestones/reorder. The scope is derived from the URL — a
// workspaceId path value scopes the reorder to that workspace's local
// milestones; absence means global. Permission is already gated by route
// middleware (workspaceItemEdit / globalMilestoneManage), and the service
// UPDATE additionally constrains by scope so a milestone in a different
// scope can never be moved.
func (h *MilestoneHandler) Reorder(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[reorderRequest](w, r)
	if !ok {
		return
	}
	if len(req.OrderedIDs) == 0 {
		respondValidationError(w, r, "ordered_ids is required")
		return
	}

	var scope services.MilestoneScope
	if wsStr := r.PathValue("workspaceId"); wsStr != "" {
		ws, err := strconv.Atoi(wsStr)
		if err != nil {
			respondValidationError(w, r, "Invalid workspaceId")
			return
		}
		wid := ws
		scope = services.MilestoneScope{IsGlobal: false, WorkspaceID: &wid, CategoryID: req.CategoryID}
	} else {
		scope = services.MilestoneScope{IsGlobal: true, CategoryID: req.CategoryID}
	}

	auditActor := services.NewAuditActorFromRequest(r, user, nil, "cookie")
	if err := h.planningService.ReorderMilestones(scope, req.OrderedIDs, auditActor); err != nil {
		if errors.Is(err, services.ErrInvalidMilestoneReorder) {
			respondValidationError(w, r, err.Error())
			return
		}
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]any{"ok": true})
}

func (h *MilestoneHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, id, ok := h.resolveMilestone(w, r, models.PermissionItemEdit)
	if !ok {
		return
	}

	// Use service to delete milestone
	auditActor := services.NewAuditActorFromRequest(r, user, nil, "cookie")
	if err := h.planningService.DeleteMilestone(id, auditActor); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *MilestoneHandler) GetTestStatistics(w http.ResponseWriter, r *http.Request) {
	user, milestoneID, ok := h.resolveMilestone(w, r, models.PermissionItemView)
	if !ok {
		return
	}
	workspaceIDs, err := h.permissionService.AccessibleWorkspaceIDs(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Use service to get test statistics
	stats, err := h.planningService.GetMilestoneTestStatistics(milestoneID, workspaceIDs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, stats)
}

// GetTestStatisticsBatch handles GET /api/milestones/test-statistics?ids=1,2.
// The service performs one grouped read and omits local milestones outside the
// caller's accessible workspace set.
func (h *MilestoneHandler) GetTestStatisticsBatch(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	ids := parseIDListParam(r.URL.Query().Get("ids"))
	if len(ids) == 0 {
		respondJSONOK(w, map[int]*services.MilestoneTestStats{})
		return
	}
	if len(ids) > maxBatchItems {
		respondBadRequest(w, r, fmt.Sprintf("too many ids (max %d per request)", maxBatchItems))
		return
	}
	workspaceIDs, err := h.permissionService.AccessibleWorkspaceIDs(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	stats, err := h.planningService.GetMilestoneTestStatisticsBatch(ids, workspaceIDs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, stats)
}

// GetProgress handles GET /api/milestones/{id}/progress - returns milestone progress report
func (h *MilestoneHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	user, milestoneID, ok := h.resolveMilestone(w, r, models.PermissionItemView)
	if !ok {
		return
	}
	workspaceIDs, err := h.permissionService.AccessibleWorkspaceIDs(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Use service to get progress report
	report, err := h.planningService.GetMilestoneProgress(milestoneID, workspaceIDs)
	if err != nil {
		if errors.Is(err, repositorypkg.ErrNotFound) {
			respondNotFound(w, r, "milestone")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, report)
}

// resolveMilestone authenticates the user, parses the milestone ID from the URL,
// and verifies the user has the required permission on the milestone.
// Returns the user, milestone ID, and true on success, or writes an HTTP error
// and returns false on failure.
func (h *MilestoneHandler) resolveMilestone(w http.ResponseWriter, r *http.Request, permission string) (*models.User, int, bool) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return nil, 0, false
	}

	milestoneID, ok := requireIDParam(w, r, "id")
	if !ok {
		return nil, 0, false
	}

	if !h.requireMilestonePermission(w, r, milestoneID, user.ID, permission) {
		return nil, 0, false
	}

	return user, milestoneID, true
}

// requireMilestonePermission fetches the milestone's scope via IsMilestoneGlobal
// and verifies the user has the required permission.  It writes an HTTP error
// response and returns false when the check fails; callers should return
// immediately in that case.  workspacePerm is the permission checked for
// workspace-scoped milestones (e.g. PermissionItemView or PermissionItemEdit);
// global milestones always require PermissionMilestoneCreate.
func (h *MilestoneHandler) requireMilestonePermission(w http.ResponseWriter, r *http.Request, milestoneID, userID int, workspacePerm string) bool {
	isGlobal, workspaceID, err := h.planningService.IsMilestoneGlobal(milestoneID)
	if err != nil {
		if errors.Is(err, repositorypkg.ErrNotFound) {
			respondNotFound(w, r, "milestone")
			return false
		}
		respondInternalError(w, r, err)
		return false
	}

	if isGlobal {
		// If just viewing, allow for all authenticated users
		if workspacePerm == models.PermissionItemView {
			return true
		}

		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(userID, models.PermissionMilestoneCreate)
		if err != nil || !hasGlobalPerm {
			respondForbidden(w, r)
			return false
		}
	} else if workspaceID != nil {
		if !RequireWorkspacePermission(w, r, userID, *workspaceID, workspacePerm, h.permissionService) {
			return false
		}
	}

	return true
}

// validateMilestoneConstraints validates the common constraint and FK checks
// shared by Create and Update: global-vs-workspace constraints, permission
// checks, and category_id / workspace_id existence.  It writes an HTTP error
// response and returns false on failure.
func (h *MilestoneHandler) validateMilestoneConstraints(w http.ResponseWriter, r *http.Request, milestone *models.Milestone, userID int) bool {
	// Validate global vs workspace constraints
	if milestone.IsGlobal && milestone.WorkspaceID != nil {
		respondValidationError(w, r, "Global milestones cannot have a workspace_id")
		return false
	}
	if !milestone.IsGlobal && milestone.WorkspaceID == nil {
		respondValidationError(w, r, "Local milestones must have a workspace_id")
		return false
	}

	// Check permission based on whether milestone is global or workspace-scoped
	if milestone.IsGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(userID, models.PermissionMilestoneCreate)
		if err != nil || !hasGlobalPerm {
			respondForbidden(w, r)
			return false
		}
	} else if !RequireWorkspacePermission(w, r, userID, *milestone.WorkspaceID, models.PermissionItemEdit, h.permissionService) {
		return false
	}

	// Validate category_id if provided (using service)
	if milestone.CategoryID != nil {
		exists, err := h.planningService.CategoryExists(*milestone.CategoryID)
		if err != nil {
			respondInternalError(w, r, err)
			return false
		}
		if !exists {
			respondInvalidID(w, r, "category_id")
			return false
		}
	}

	// Validate workspace_id if provided (using service)
	if milestone.WorkspaceID != nil {
		exists, err := h.planningService.WorkspaceExists(*milestone.WorkspaceID)
		if err != nil {
			respondInternalError(w, r, err)
			return false
		}
		if !exists {
			respondInvalidID(w, r, "workspace_id")
			return false
		}
	}

	return true
}

func isValidMilestoneStatus(status string) bool {
	//nolint:misspell // British spelling is intentional for status value
	for _, s := range []string{"planning", "in-progress", "completed", "cancelled"} {
		if status == s {
			return true
		}
	}
	return false
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
		ExternalKey:   r.ExternalKey,
		Position:      r.Position,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
	if r.TargetDate != "" {
		milestone.TargetDate = &r.TargetDate
	}

	if r.LatestRelease != nil {
		rel := &models.MilestoneRelease{
			ID:                    r.LatestRelease.ID,
			MilestoneID:           r.LatestRelease.MilestoneID,
			WorkspaceRepositoryID: r.LatestRelease.WorkspaceRepositoryID,
			TagName:               r.LatestRelease.TagName,
			TagURL:                r.LatestRelease.TagURL,
			ReleaseStatus:         r.LatestRelease.ReleaseStatus,
			ReleasedAt:            r.LatestRelease.ReleasedAt,
			Assets:                r.LatestRelease.Assets,
			LastSyncedAt:          r.LatestRelease.LastSyncedAt,
			Name:                  r.LatestRelease.Name,
			Body:                  r.LatestRelease.Body,
			IsDraft:               r.LatestRelease.IsDraft,
			IsPrerelease:          r.LatestRelease.IsPrerelease,
			TargetCommitish:       r.LatestRelease.TargetCommitish,
			SCMReleaseID:          r.LatestRelease.SCMReleaseID,
			SCMReleaseURL:         r.LatestRelease.SCMReleaseURL,
			CreatedBy:             r.LatestRelease.CreatedBy,
			CreatedAt:             r.LatestRelease.CreatedAt,
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
	Mode            string `json:"mode"` // create (default) or attach
	ConnectionID    int    `json:"connection_id"`
	RepositoryID    int    `json:"repository_id"`
	Repository      string `json:"repository"` // "owner/repo"
	TagName         string `json:"tag_name"`
	Name            string `json:"name"`
	Body            string `json:"body"`
	IsDraft         bool   `json:"is_draft"`
	IsPrerelease    bool   `json:"is_prerelease"`
	TargetCommitish string `json:"target_commitish"` // optional branch or SHA
}

// Release handles POST /milestones/{id}/release — creates an SCM release and marks the milestone completed.
func (h *MilestoneHandler) Release(w http.ResponseWriter, r *http.Request) {
	user, id, ok := h.resolveMilestone(w, r, models.PermissionItemEdit)
	if !ok {
		return
	}

	req, ok := decodeJSON[releaseRequest](w, r)
	if !ok {
		return
	}

	if req.TagName == "" {
		respondValidationError(w, r, "tag_name is required")
		return
	}
	req.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	if req.Mode == "" {
		req.Mode = "create"
	}
	if req.Mode != "create" && req.Mode != "attach" {
		respondValidationError(w, r, "mode must be 'create' or 'attach'")
		return
	}

	// Resolve every local dependency before persisting the attempt. No provider
	// side effect may occur until the durable idempotency row exists.
	var scmConnectionID *int
	var workspaceRepositoryID *int
	var scmRepository *string
	var releaseProvider milestoneReleaseProvider
	var owner, repository string

	if req.ConnectionID > 0 {
		connectionWorkspaceID, wsErr := h.planningService.GetSCMConnectionWorkspaceID(req.ConnectionID)
		if wsErr != nil || connectionWorkspaceID == 0 {
			respondBadRequest(w, r, "SCM connection not found")
			return
		}
		if !RequireWorkspacePermission(w, r, user.ID, connectionWorkspaceID, models.PermissionItemEdit, h.permissionService) {
			return
		}
		linkedRepository, repoErr := h.planningService.ResolveLinkedSCMRepository(req.ConnectionID, req.RepositoryID, req.Repository)
		if errors.Is(repoErr, services.ErrSCMRepositoryNotLinked) {
			respondValidationError(w, r, "repository must be linked to the selected SCM connection")
			return
		}
		if repoErr != nil {
			respondInternalError(w, r, repoErr)
			return
		}
		repositoryName := linkedRepository.RepositoryName
		repositoryID := linkedRepository.ID
		workspaceRepositoryID = &repositoryID
		var pathOK bool
		owner, repository, pathOK = utils.SplitRepositoryPath(repositoryName)
		if !pathOK {
			respondValidationError(w, r, "repository must be in 'namespace/project' format")
			return
		}
		cid := req.ConnectionID
		scmConnectionID = &cid
		scmRepository = &repositoryName

		if h.resolveReleaseProvider != nil {
			provider, provErr := h.resolveReleaseProvider(r.Context(), req.ConnectionID, user.ID)
			if provErr != nil {
				respondBadRequest(w, r, "Failed to load SCM provider: "+provErr.Error())
				return
			}
			releaseProvider = provider
		}
	}

	createdBy := user.ID
	var existingRelease *scm.Release
	if req.Mode == "attach" {
		if releaseProvider == nil {
			respondValidationError(w, r, "attach mode requires an SCM connection and linked repository")
			return
		}
		releases, listErr := releaseProvider.ListReleases(r.Context(), owner, repository)
		if listErr != nil {
			respondBadRequest(w, r, "Failed to load SCM releases: "+listErr.Error())
			return
		}
		for i := range releases {
			if releases[i].TagName == req.TagName {
				existingRelease = &releases[i]
				break
			}
		}
		if existingRelease == nil {
			tags, listErr := releaseProvider.ListTags(r.Context(), owner, repository, time.Time{})
			if listErr != nil {
				respondBadRequest(w, r, "Failed to load SCM tags: "+listErr.Error())
				return
			}
			for _, tag := range tags {
				if tag.Name == req.TagName {
					existingRelease = &scm.Release{TagName: tag.Name, TagURL: tag.URL, Status: "tag_only"}
					break
				}
			}
		}
		if existingRelease == nil {
			respondValidationError(w, r, "the selected tag or release no longer exists in the SCM repository")
			return
		}
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) > 200 {
		respondValidationError(w, r, "Idempotency-Key must be at most 200 characters")
		return
	}
	if idempotencyKey == "" {
		idempotencyKey = milestoneReleaseFallbackKey(id, user.ID, req, scmRepository)
	}

	params := services.ReleaseMilestoneParams{
		ID:                    id,
		IdempotencyKey:        idempotencyKey,
		TagName:               req.TagName,
		Name:                  req.Name,
		Body:                  req.Body,
		IsDraft:               req.IsDraft,
		IsPrerelease:          req.IsPrerelease,
		TargetCommitish:       req.TargetCommitish,
		SCMConnectionID:       scmConnectionID,
		WorkspaceRepositoryID: workspaceRepositoryID,
		SCMRepository:         scmRepository,
		CreatedBy:             &createdBy,
	}
	attempt, err := h.planningService.BeginMilestoneRelease(r.Context(), params)
	if err != nil {
		if errors.Is(err, services.ErrMilestoneReleaseIdempotencyConflict) {
			respondConflict(w, r, "Idempotency-Key was already used for a different milestone release request")
			return
		}
		if errors.Is(err, services.ErrMilestoneReleaseInProgress) {
			respondConflict(w, r, "This release request is already in progress")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	if attempt.AlreadyCreated {
		result, getErr := h.planningService.GetMilestone(id)
		if getErr != nil {
			respondInternalError(w, r, getErr)
			return
		}
		respondJSONOK(w, h.milestoneResultToModel(result, user.ID))
		return
	}

	if releaseProvider != nil {
		release := existingRelease
		if attempt.NeedsReconcile {
			releases, listErr := releaseProvider.ListReleases(r.Context(), owner, repository)
			if listErr != nil {
				_ = h.planningService.MarkMilestoneReleaseUncertain(r.Context(), attempt.ID, attempt.LeaseToken, listErr.Error())
				respondBadRequest(w, r, "Failed to reconcile SCM release: "+listErr.Error())
				return
			}
			for i := range releases {
				if releases[i].TagName == req.TagName {
					release = &releases[i]
					break
				}
			}
		}
		if release == nil && req.Mode == "create" {
			release, err = releaseProvider.CreateRelease(r.Context(), owner, repository, scm.CreateReleaseOptions{
				TagName:         req.TagName,
				TargetCommitish: req.TargetCommitish,
				Name:            req.Name,
				Body:            req.Body,
				IsDraft:         req.IsDraft,
				IsPrerelease:    req.IsPrerelease,
			})
			if err != nil {
				_ = h.planningService.MarkMilestoneReleaseUncertain(r.Context(), attempt.ID, attempt.LeaseToken, err.Error())
				respondBadRequest(w, r, "Failed to create SCM release: "+err.Error())
				return
			}
		}
		if release.ID != "" {
			params.SCMReleaseID = &release.ID
		}
		if release.URL != "" {
			params.SCMReleaseURL = &release.URL
		}
		params.Name = release.Name
		params.Body = release.Body
		params.IsDraft = release.IsDraft
		params.IsPrerelease = release.IsPrerelease
		if release.TagURL != "" {
			params.TagURL = &release.TagURL
		}
		params.ReleaseStatus = release.Status
		params.Assets = release.Assets
		if release.ReleasedAt != nil {
			releasedAt := release.ReleasedAt.UTC().Format(time.RFC3339)
			params.ReleasedAt = &releasedAt
		}
	}

	result, err := h.planningService.CompleteMilestoneRelease(r.Context(), attempt.ID, attempt.LeaseToken, params)
	if err != nil {
		_ = h.planningService.MarkMilestoneReleaseUncertain(r.Context(), attempt.ID, attempt.LeaseToken, err.Error())
		respondInternalError(w, r, err)
		return
	}

	milestone := h.milestoneResultToModel(result, user.ID)
	h.auditor.Log(r, user, logger.ActionMilestoneRelease, logger.ResourceMilestone, &id, milestone.Name)
	respondJSONOK(w, milestone)
}

func milestoneReleaseFallbackKey(milestoneID, userID int, req releaseRequest, repository *string) string {
	repositoryName := ""
	if repository != nil {
		repositoryName = *repository
	}
	canonical := fmt.Sprintf(
		"%d\n%d\n%s\n%d\n%s\n%s\n%s\n%s\n%t\n%t\n%s",
		milestoneID, userID, req.Mode, req.ConnectionID, repositoryName, req.TagName,
		req.Name, req.Body, req.IsDraft, req.IsPrerelease, req.TargetCommitish,
	)
	sum := sha256.Sum256([]byte(canonical))
	return "auto-" + hex.EncodeToString(sum[:])
}
