package handlers

import (
	"encoding/json"
	"errors"
	"strings"

	"net/http"

	"windshift/internal/customwidgets"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
)

// customWidgetMaxPerUser bounds how many widgets one user can create. The
// dashboard itself caps widgets per layout; this is an abuse bound, not a UX
// limit.
const customWidgetMaxPerUser = 200

// CustomWidgetHandler serves user-authored OpenUI dashboard widgets.
type CustomWidgetHandler struct {
	repo              *repository.CustomWidgetRepository
	permissionService *services.PermissionService
}

func NewCustomWidgetHandler(repo *repository.CustomWidgetRepository, permissionService *services.PermissionService) *CustomWidgetHandler {
	return &CustomWidgetHandler{repo: repo, permissionService: permissionService}
}

type customWidgetRequest struct {
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Program        string          `json:"program"`
	LangVersion    string          `json:"lang_version"`
	LibraryVersion string          `json:"library_version"`
	Queries        json.RawMessage `json:"queries"`
	WorkspaceID    *int            `json:"workspace_id"`
	Visibility     string          `json:"visibility"`
}

// GetCustomWidgets lists the caller's private widgets plus workspace widgets
// in workspaces they can read.
func (h *CustomWidgetHandler) GetCustomWidgets(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	workspaceIDs, err := h.permissionService.AccessibleWorkspaceIDs(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	widgets, err := h.repo.ListForUser(user.ID, workspaceIDs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, widgets)
}

// GetCustomWidget returns one widget. Inaccessible widgets answer 404 rather
// than 403 so the endpoint does not confirm existence.
func (h *CustomWidgetHandler) GetCustomWidget(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	widget, err := h.repo.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "Custom widget")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	allowed, err := canReadCustomWidget(h.permissionService, user.ID, widget)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !allowed {
		respondNotFound(w, r, "Custom widget")
		return
	}
	respondJSONOK(w, widget)
}

// CreateCustomWidget validates and persists a widget owned by the caller.
func (h *CustomWidgetHandler) CreateCustomWidget(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	request, ok := decodeJSON[customWidgetRequest](w, r)
	if !ok {
		return
	}

	widget, err := h.buildWidget(user.ID, request)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	if widget.WorkspaceID != nil {
		allowed, permErr := h.permissionService.HasWorkspacePermission(user.ID, *widget.WorkspaceID, models.PermissionItemEdit)
		if permErr != nil {
			respondInternalError(w, r, permErr)
			return
		}
		if !allowed {
			respondForbidden(w, r)
			return
		}
	}

	count, err := h.repo.CountOwned(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if count >= customWidgetMaxPerUser {
		respondConflict(w, r, "Custom widget limit reached")
		return
	}

	id, err := h.repo.Create(widget)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	created, err := h.repo.Get(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONCreated(w, created)
}

// UpdateCustomWidget replaces the mutable fields of a widget the caller may
// write.
func (h *CustomWidgetHandler) UpdateCustomWidget(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	existing, err := h.repo.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "Custom widget")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	// Read access first: a widget the caller cannot see answers 404, while a
	// readable workspace widget they cannot edit answers 403.
	readable, err := canReadCustomWidget(h.permissionService, user.ID, existing)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !readable {
		respondNotFound(w, r, "Custom widget")
		return
	}
	allowed, err := canWriteCustomWidget(h.permissionService, user.ID, existing)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !allowed {
		respondForbidden(w, r)
		return
	}

	request, ok := decodeJSON[customWidgetRequest](w, r)
	if !ok {
		return
	}
	if request.WorkspaceID != nil && (existing.WorkspaceID == nil || *request.WorkspaceID != *existing.WorkspaceID) {
		respondValidationError(w, r, "workspace_id cannot be changed")
		return
	}

	widget, err := h.buildWidget(user.ID, request)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}
	widget.ID = existing.ID
	widget.WorkspaceID = existing.WorkspaceID
	widget.OwnerUserID = existing.OwnerUserID
	widget.CreatedBy = existing.CreatedBy

	if err := h.repo.Update(widget); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Custom widget")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	updated, err := h.repo.Get(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, updated)
}

// DeleteCustomWidget removes a widget. Layouts keep the stale ID and render an
// unavailable card.
func (h *CustomWidgetHandler) DeleteCustomWidget(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	existing, err := h.repo.Get(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "Custom widget")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	readable, err := canReadCustomWidget(h.permissionService, user.ID, existing)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !readable {
		respondNotFound(w, r, "Custom widget")
		return
	}
	allowed, err := canWriteCustomWidget(h.permissionService, user.ID, existing)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !allowed {
		respondForbidden(w, r)
		return
	}

	if err := h.repo.Delete(id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Custom widget")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ValidateCustomWidget runs the same validation as create without persisting.
func (h *CustomWidgetHandler) ValidateCustomWidget(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireAuth(w, r); !ok {
		return
	}
	request, ok := decodeJSON[customWidgetRequest](w, r)
	if !ok {
		return
	}
	if _, err := h.buildWidget(0, request); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}
	respondJSONOK(w, map[string]bool{"valid": true})
}

// buildWidget validates a request and returns the widget to persist.
func (h *CustomWidgetHandler) buildWidget(ownerUserID int, request customWidgetRequest) (*models.CustomWidget, error) {
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	if len([]rune(name)) > customwidgets.MaxNameRunes {
		return nil, errors.New("name is too long")
	}
	description := strings.TrimSpace(request.Description)
	if len([]rune(description)) > customwidgets.MaxDescriptionRunes {
		return nil, errors.New("description is too long")
	}
	sanitize.Apply(&name, sanitize.PlainTextField)
	sanitize.Apply(&description, sanitize.PlainTextField)

	if err := customwidgets.ValidateProgram(request.Program); err != nil {
		return nil, err
	}
	queries, err := customwidgets.NormalizeQueries(request.Queries)
	if err != nil {
		return nil, err
	}

	visibility := strings.TrimSpace(request.Visibility)
	if request.WorkspaceID == nil {
		visibility = models.CustomWidgetVisibilityPrivate
	} else {
		if visibility == "" {
			visibility = models.CustomWidgetVisibilityWorkspace
		}
		if visibility != models.CustomWidgetVisibilityWorkspace {
			return nil, errors.New("workspace widgets must be workspace-visible")
		}
	}

	widget := &models.CustomWidget{
		WorkspaceID:    request.WorkspaceID,
		Name:           name,
		Description:    description,
		Program:        request.Program,
		LangVersion:    strings.TrimSpace(request.LangVersion),
		LibraryVersion: strings.TrimSpace(request.LibraryVersion),
		Queries:        queries,
		Visibility:     visibility,
		CreatedBy:      ownerUserID,
	}
	if request.WorkspaceID == nil {
		widget.OwnerUserID = &ownerUserID
	}
	return widget, nil
}

// canReadCustomWidget reports whether the viewer may see a widget. Shared with
// the data handler so both agree on visibility.
func canReadCustomWidget(permissionService *services.PermissionService, userID int, widget *models.CustomWidget) (bool, error) {
	if widget.WorkspaceID == nil {
		return widget.OwnerUserID != nil && *widget.OwnerUserID == userID, nil
	}
	return permissionService.HasWorkspacePermission(userID, *widget.WorkspaceID, models.PermissionItemView)
}

func canWriteCustomWidget(permissionService *services.PermissionService, userID int, widget *models.CustomWidget) (bool, error) {
	if widget.WorkspaceID == nil {
		return widget.OwnerUserID != nil && *widget.OwnerUserID == userID, nil
	}
	return permissionService.HasWorkspacePermission(userID, *widget.WorkspaceID, models.PermissionItemEdit)
}
