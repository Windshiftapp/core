package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/authz"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/middleware"
	"windshift/internal/services"
)

// BaseHandler provides shared dependencies and utilities for REST API handlers.
type BaseHandler struct {
	DB                database.Database
	PermissionService *services.PermissionService
	Perms             *authz.Authz
}

// NewBaseHandler creates a new base handler with shared dependencies.
func NewBaseHandler(db database.Database, permissionService *services.PermissionService) BaseHandler {
	return BaseHandler{
		DB:                db,
		PermissionService: permissionService,
		Perms:             authz.New(db, permissionService),
	}
}

// ParsePagination extracts pagination params from a request.
func (b *BaseHandler) ParsePagination(r *http.Request) restapi.PaginationParams {
	return restapi.ParsePaginationParams(r)
}

// RespondOK writes a 200 OK response.
func (b *BaseHandler) RespondOK(w http.ResponseWriter, data interface{}) {
	restapi.RespondOK(w, data)
}

// RespondCreated writes a 201 Created response.
func (b *BaseHandler) RespondCreated(w http.ResponseWriter, data interface{}) {
	restapi.RespondCreated(w, data)
}

// RespondNoContent writes a 204 No Content response.
func (b *BaseHandler) RespondNoContent(w http.ResponseWriter) {
	restapi.RespondNoContent(w)
}

// RespondPaginated writes a paginated response.
func (b *BaseHandler) RespondPaginated(w http.ResponseWriter, data interface{}, pagination restapi.PaginationParams, total int) {
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

// DecodeBodyOrRespond decodes JSON body or writes 400 on error.
func (b *BaseHandler) DecodeBodyOrRespond(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid request body"))
		return false
	}
	return true
}

// RequireGlobalPermission checks global permission or writes 403.
func (b *BaseHandler) RequireGlobalPermission(w http.ResponseWriter, r *http.Request, userID int, permission, label string) bool {
	hasPermission, err := b.Perms.HasGlobalPermission(userID, permission)
	if err != nil || !hasPermission {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, "FORBIDDEN", label+" permission required"))
		return false
	}
	return true
}

// ValidateRequiredString checks a required string field.
func (b *BaseHandler) ValidateRequiredString(w http.ResponseWriter, r *http.Request, value, fieldName string) bool {
	if strings.TrimSpace(value) == "" {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeMissingField, fieldName+" is required"))
		return false
	}
	return true
}

// ValidateNoFields checks if a dynamic update has no fields set.
func (b *BaseHandler) ValidateNoFields(w http.ResponseWriter, r *http.Request, builder *DynamicUpdateBuilder) bool {
	if builder.IsEmpty() {
		restapi.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "No fields to update"))
		return false
	}
	return true
}

// DynamicUpdateBuilder accumulates SET clauses for UPDATE statements.
// This handler-side version works with any database.Database.
type DynamicUpdateBuilder struct {
	sets []string
	args []interface{}
}

// NewDynamicUpdateBuilder creates a new dynamic update builder.
func NewDynamicUpdateBuilder() *DynamicUpdateBuilder {
	return &DynamicUpdateBuilder{}
}

// AddString adds a string field update if the value is non-nil and non-empty.
func (b *DynamicUpdateBuilder) AddString(field string, value *string) {
	if value != nil && *value != "" {
		b.sets = append(b.sets, field+" = ?")
		b.args = append(b.args, *value)
	}
}

// AddBool adds a boolean field update if the value is non-nil.
func (b *DynamicUpdateBuilder) AddBool(field string, value *bool) {
	if value != nil {
		b.sets = append(b.sets, field+" = ?")
		b.args = append(b.args, *value)
	}
}

// IsEmpty returns true if no fields have been added.
func (b *DynamicUpdateBuilder) IsEmpty() bool {
	return len(b.sets) == 0
}

// Sets returns the SET clauses and args.
func (b *DynamicUpdateBuilder) Sets() (sets []string, args []interface{}) {
	return b.sets, b.args
}

// AddTimestamp adds an updated_at = CURRENT_TIMESTAMP clause.
func (b *DynamicUpdateBuilder) AddTimestamp() {
	b.sets = append(b.sets, "updated_at = CURRENT_TIMESTAMP")
}

// BuildUpdateByID builds an "UPDATE <table> SET ... WHERE id = ?" query
// and returns the query string and args (with id appended).
func (b *DynamicUpdateBuilder) BuildUpdateByID(table string, id int) (query string, args []interface{}) {
	var sets []string
	sets, args = b.Sets()
	args = append(args, id)

	query = "UPDATE " + table + " SET " + strings.Join(sets, ", ") + " WHERE id = ?"
	return query, args
}
