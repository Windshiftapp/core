package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/authz"
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/middleware"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// BaseHandler provides shared dependencies and utilities for REST API handlers.
type BaseHandler struct {
	DB                database.Database
	PermissionService *services.PermissionService
	Perms             *authz.Authz
	Auditor           *logger.Auditor
}

// NewBaseHandler creates a new base handler with shared dependencies.
func NewBaseHandler(db database.Database, permissionService *services.PermissionService) BaseHandler {
	return BaseHandler{
		DB:                db,
		PermissionService: permissionService,
		Perms:             authz.New(db, permissionService),
		Auditor:           logger.NewAuditor(db),
	}
}

// AuditActor returns the transport-neutral actor metadata consumed by shared
// application services. Bearer token attribution is preserved centrally.
func (b *BaseHandler) AuditActor(r *http.Request, user *models.User) services.AuditActor {
	return services.NewAuditActorFromRequest(r, user, middleware.GetAPIToken(r.Context()), "bearer")
}

// ParsePagination extracts pagination params from a request.
func (b *BaseHandler) ParsePagination(r *http.Request) restapi.PaginationParams {
	return restapi.ParsePaginationParams(r)
}

// RespondOK writes a 200 OK response.
func (b *BaseHandler) RespondOK(w http.ResponseWriter, data any) {
	restapi.RespondOK(w, data)
}

func (b *BaseHandler) RespondUnauthorized(w http.ResponseWriter, r *http.Request) {
	restapi.RespondError(w, r, restapi.ErrUnauthorized)
}

// RespondCreated writes a 201 Created response.
func (b *BaseHandler) RespondCreated(w http.ResponseWriter, data any) {
	restapi.RespondCreated(w, data)
}

// RespondNoContent writes a 204 No Content response.
func (b *BaseHandler) RespondNoContent(w http.ResponseWriter) {
	restapi.RespondNoContent(w)
}

// RespondPaginated writes a paginated response.
func (b *BaseHandler) RespondPaginated(w http.ResponseWriter, data any, pagination restapi.PaginationParams, total int) {
	restapi.RespondPaginated(w, data, restapi.NewPaginationMeta(pagination, total))
}

// RespondError writes an error response.
func (b *BaseHandler) RespondError(w http.ResponseWriter, r *http.Request, err *restapi.APIError) {
	restapi.RespondError(w, r, err)
}

// RespondInternalError writes a 500 error response.
func (b *BaseHandler) RespondInternalError(w http.ResponseWriter, r *http.Request) {
	restapi.RespondError(w, r, restapi.ErrInternalError)
}

// RespondNotFound writes a 404 error response.
func (b *BaseHandler) RespondNotFound(w http.ResponseWriter, r *http.Request) {
	restapi.RespondError(w, r, restapi.ErrNotFound)
}

// RequireAuth extracts the authenticated user from the request context.
// Returns nil and writes a 401 response if not authenticated.
func (b *BaseHandler) RequireAuth(w http.ResponseWriter, r *http.Request) (*models.User, bool) {
	user := middleware.GetUser(r.Context())
	if user == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return nil, false
	}
	return user, true
}

// ParsePathID parses an integer path parameter from the request.
// Returns 0 and writes a 400 response if the parameter is not a valid integer.
func (b *BaseHandler) ParsePathID(w http.ResponseWriter, r *http.Request, param, label string) (int, bool) {
	id, err := strconv.Atoi(r.PathValue(param))
	if err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid "+label))
		return 0, false
	}
	return id, true
}

// DecodeBodyOrRespond decodes JSON or writes the corresponding client error.
func (b *BaseHandler) DecodeBodyOrRespond(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := restapi.DecodeJSONBody(w, r, v); err != nil {
		if restapi.IsRequestBodyTooLarge(err) {
			restapi.RespondError(w, r, restapi.NewAPIError(http.StatusRequestEntityTooLarge, restapi.ErrCodeRequestTooLarge, "Request body too large"))
			return false
		}
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid request body"))
		return false
	}
	return true
}

func newJSONDecoder(w http.ResponseWriter, r *http.Request) *json.Decoder {
	return restapi.NewJSONDecoder(w, r)
}

// auditDenied records a route-level authorization denial for mutating
// requests. Reads are skipped so high-volume GET paths do not flood the log.
func (b *BaseHandler) auditDenied(r *http.Request, requiredPermission string) {
	if b.Auditor == nil || !utils.IsMutationMethod(r.Method) {
		return
	}
	user := middleware.GetUser(r.Context())
	if user == nil {
		return
	}
	b.Auditor.LogDenied(r, user, logger.ActionAccessDenied, logger.ResourceRoute, nil, r.URL.Path, requiredPermission, nil)
}

// RequireGlobalPermission checks global permission or writes 403.
func (b *BaseHandler) RequireGlobalPermission(w http.ResponseWriter, r *http.Request, userID int, permission, label string) bool {
	hasPermission, err := b.Perms.HasGlobalPermission(userID, permission)
	if err != nil || !hasPermission {
		b.auditDenied(r, permission)
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, "FORBIDDEN", label+" permission required"))
		return false
	}
	return true
}

// RequireWorkspaceViewAccess authenticates the request, parses the workspace
// ID from the {id} path parameter, and verifies the caller can view items in
// that workspace. On failure it writes the appropriate HTTP error and returns
// (0, false). Used by every /workspaces/{id}/<resource> read route.
func (b *BaseHandler) RequireWorkspaceViewAccess(w http.ResponseWriter, r *http.Request) (int, bool) {
	user, ok := b.RequireAuth(w, r)
	if !ok {
		return 0, false
	}
	wsID, ok := b.ParsePathID(w, r, "id", "workspace ID")
	if !ok {
		return 0, false
	}
	canView, _ := b.Perms.CanViewWorkspace(user.ID, wsID)
	if !canView {
		b.auditDenied(r, models.PermissionItemView)
		restapi.RespondError(w, r, restapi.ErrWorkspaceNotFound)
		return 0, false
	}
	return wsID, true
}

// RequireWorkspaceEditAccess is the edit-permission counterpart to
// RequireWorkspaceViewAccess. Used by every /workspaces/{id}/<resource> write
// route.
func (b *BaseHandler) RequireWorkspaceEditAccess(w http.ResponseWriter, r *http.Request) (int, bool) {
	user, ok := b.RequireAuth(w, r)
	if !ok {
		return 0, false
	}
	wsID, ok := b.ParsePathID(w, r, "id", "workspace ID")
	if !ok {
		return 0, false
	}
	canEdit, _ := b.Perms.CanEditWorkspace(user.ID, wsID)
	if !canEdit {
		b.auditDenied(r, models.PermissionItemEdit)
		restapi.RespondError(w, r, restapi.ErrWorkspaceNotFound)
		return 0, false
	}
	return wsID, true
}

// RequireWorkspaceAdminAccess is the workspace-admin counterpart to
// RequireWorkspaceEditAccess, for routes that manage workspace configuration
// (e.g. the work item template catalog) rather than item content. Returns 404
// on failure so workspace existence isn't leaked.
func (b *BaseHandler) RequireWorkspaceAdminAccess(w http.ResponseWriter, r *http.Request) (int, bool) {
	user, ok := b.RequireAuth(w, r)
	if !ok {
		return 0, false
	}
	wsID, ok := b.ParsePathID(w, r, "id", "workspace ID")
	if !ok {
		return 0, false
	}
	canAdmin, _ := b.Perms.CanAdminWorkspace(user.ID, wsID)
	if !canAdmin {
		b.auditDenied(r, models.PermissionWorkspaceAdmin)
		restapi.RespondError(w, r, restapi.ErrWorkspaceNotFound)
		return 0, false
	}
	return wsID, true
}

// maskProjectNames blanks restricted time-project names (direct, time-tracking
// and inherited effective project) on items before they are mapped to response
// DTOs, mirroring the cookie-auth surface. IDs are kept; only names are stripped.
func (b *BaseHandler) maskProjectNames(userID int, items []models.Item) {
	services.NewTimePermissionService(b.DB, b.PermissionService).MaskInaccessibleProjectNames(userID, items)
}

// maskProjectNamesOne applies maskProjectNames to a single item in place.
func (b *BaseHandler) maskProjectNamesOne(userID int, item *models.Item) {
	masked := []models.Item{*item}
	b.maskProjectNames(userID, masked)
	*item = masked[0]
}

// ValidateRequiredString checks a required string field.
func (b *BaseHandler) ValidateRequiredString(w http.ResponseWriter, r *http.Request, value, fieldName string) bool {
	if strings.TrimSpace(value) == "" {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, fieldName+" is required"))
		return false
	}
	return true
}

// ExcludePersonal reports whether the request opts out of personal-workspace
// results via the exclude_personal query parameter. Integration surfaces that
// republish items into shared contexts (e.g. document embeds) set this so the
// caller's own personal items never leak into pages other people read.
func ExcludePersonal(r *http.Request) bool {
	v := r.URL.Query().Get("exclude_personal")
	return v == "true" || v == "1"
}
