package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/services"
)

// SCIMHandler handles SCIM 2.0 endpoints
type SCIMHandler struct {
	db                database.Database
	baseURL           string
	permissionService *services.PermissionService
}

// NewSCIMHandler creates a new SCIM handler
func NewSCIMHandler(db database.Database, baseURL string, permissionService *services.PermissionService) *SCIMHandler {
	return &SCIMHandler{
		db:                db,
		baseURL:           baseURL,
		permissionService: permissionService,
	}
}

// =============================================================================
// Constants
// =============================================================================

// scimMaxBodySize limits request body size to prevent memory exhaustion (1MB)
const scimMaxBodySize = 1 * 1024 * 1024

// nullIfEmpty converts an empty string to nil (SQL NULL) so that partial unique
// indexes on scim_external_id (WHERE scim_external_id IS NOT NULL) are not
// violated when the field is omitted from the SCIM request.
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// =============================================================================
// Response Helpers
// =============================================================================

// limitRequestBody wraps the request body with a size limiter
// Returns true if the body was limited successfully, false if body is too large
func (h *SCIMHandler) limitRequestBody(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, scimMaxBodySize)
}

// logSCIMAuditEvent logs a SCIM provisioning event to the audit log.
func (h *SCIMHandler) logSCIMAuditEvent(r *http.Request, actionType, resourceType string, resourceID *int, resourceName string, details map[string]interface{}, success bool, errorMsg string) {
	// Get SCIM token from context to identify the requester
	scimToken := middleware.GetSCIMToken(r)
	tokenPrefix := ""
	if scimToken != nil {
		tokenPrefix = scimToken.TokenPrefix
	}

	// Add token prefix to details
	if details == nil {
		details = make(map[string]interface{})
	}
	details["scim_token_prefix"] = tokenPrefix

	event := logger.AuditEvent{
		UserID:       0, // SCIM uses token auth, not user auth
		Username:     "SCIM:" + tokenPrefix,
		IPAddress:    r.RemoteAddr,
		UserAgent:    r.UserAgent(),
		ActionType:   actionType,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Details:      details,
		Success:      success,
		ErrorMessage: errorMsg,
	}

	// Fire and forget - don't block on audit logging
	go func() { _ = logger.LogAudit(h.db, event) }()
}

// attrChange records a single attribute mutation inside a SCIM PATCH request.
// It is embedded under the "changes" key in audit log details so that operators
// can see exactly which attributes an IdP touched and what the old/new values were.
type attrChange struct {
	Op       string      `json:"op"`
	Path     string      `json:"path"`
	OldValue interface{} `json:"old_value,omitempty"`
	NewValue interface{} `json:"new_value,omitempty"`
}

// logPatchOpError records the underlying driver/model error server-side for
// operators, keyed by the SCIM token prefix so it can be correlated to the IdP
// that triggered it. The client response stays generic ("Patch operation
// failed") so driver-level text like FK constraint names doesn't leak out to
// the IdP's logs.
func (h *SCIMHandler) logPatchOpError(r *http.Request, resourceKind string, resourceID int, op models.SCIMPatchOp, opErr error) {
	var tokenPrefix string
	if tok := middleware.GetSCIMToken(r); tok != nil {
		tokenPrefix = tok.TokenPrefix
	}
	slog.Error("scim patch operation failed",
		"resource_kind", resourceKind,
		"resource_id", resourceID,
		"op", op.Op,
		"path", op.Path,
		"token_prefix", tokenPrefix,
		"error", opErr.Error(),
	)
}

// respondSCIMJSON sends a SCIM JSON response
func respondSCIMJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

// respondSCIMErrorMsg sends a SCIM error response
func respondSCIMErrorMsg(w http.ResponseWriter, status int, detail, scimType string) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(status)

	scimError := models.SCIMError{
		Schemas:  []string{models.SCIMSchemaError},
		Detail:   detail,
		Status:   strconv.Itoa(status),
		ScimType: scimType,
	}

	_ = json.NewEncoder(w).Encode(scimError)
}

// =============================================================================
// Service Provider Endpoints
// =============================================================================

// GetServiceProviderConfig returns SCIM capabilities (GET /scim/v2/ServiceProviderConfig)
func (h *SCIMHandler) GetServiceProviderConfig(w http.ResponseWriter, r *http.Request) {
	config := GetServiceProviderConfig(h.baseURL)
	respondSCIMJSON(w, http.StatusOK, config)
}

// GetResourceTypes returns supported resource types (GET /scim/v2/ResourceTypes)
func (h *SCIMHandler) GetResourceTypes(w http.ResponseWriter, r *http.Request) {
	resourceTypes := []models.SCIMResourceType{
		GetUserResourceType(h.baseURL),
		GetGroupResourceType(h.baseURL),
	}

	response := models.SCIMListResponse{
		Schemas:      []string{models.SCIMSchemaListResponse},
		TotalResults: len(resourceTypes),
		StartIndex:   1,
		ItemsPerPage: len(resourceTypes),
		Resources:    make([]interface{}, len(resourceTypes)),
	}
	for i, rt := range resourceTypes {
		response.Resources[i] = rt
	}

	respondSCIMJSON(w, http.StatusOK, response)
}

// GetResourceType returns a single resource type (GET /scim/v2/ResourceTypes/{id})
func (h *SCIMHandler) GetResourceType(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	switch id {
	case "User":
		rt := GetUserResourceType(h.baseURL)
		respondSCIMJSON(w, http.StatusOK, rt)
	case "Group":
		rt := GetGroupResourceType(h.baseURL)
		respondSCIMJSON(w, http.StatusOK, rt)
	default:
		respondSCIMErrorMsg(w, http.StatusNotFound, "ResourceType not found: "+id, "")
	}
}

// GetSchemas returns SCIM schemas (GET /scim/v2/Schemas)
func (h *SCIMHandler) GetSchemas(w http.ResponseWriter, r *http.Request) {
	schemas := []models.SCIMSchema{
		GetUserSchema(h.baseURL),
		GetGroupSchema(h.baseURL),
	}

	response := models.SCIMListResponse{
		Schemas:      []string{models.SCIMSchemaListResponse},
		TotalResults: len(schemas),
		StartIndex:   1,
		ItemsPerPage: len(schemas),
		Resources:    make([]interface{}, len(schemas)),
	}
	for i, s := range schemas {
		response.Resources[i] = s
	}

	respondSCIMJSON(w, http.StatusOK, response)
}

// GetSchema returns a single schema (GET /scim/v2/Schemas/{id})
func (h *SCIMHandler) GetSchema(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	switch id {
	case models.SCIMSchemaUser:
		respondSCIMJSON(w, http.StatusOK, GetUserSchema(h.baseURL))
	case models.SCIMSchemaGroup:
		respondSCIMJSON(w, http.StatusOK, GetGroupSchema(h.baseURL))
	default:
		respondSCIMErrorMsg(w, http.StatusNotFound, "Schema not found: "+id, "")
	}
}

// =============================================================================
// User Endpoints
// =============================================================================

// listUsersFiltered queries users with a SCIM filter and pagination, returning a list response.
func (h *SCIMHandler) listUsersFiltered(filter string, startIndex, count int) (*models.SCIMListResponse, error) {
	filterResult, err := ParseSCIMFilterWithAnd(filter, "User")
	if err != nil {
		return nil, fmt.Errorf("invalid filter: %w", err)
	}

	// SCIM represents the IdP-provisioned surface. Agent users and locally
	// managed humans must stay invisible here: if the IdP ever sees them in a
	// GET /Users sweep it records their IDs in its shadow and then tries to
	// DELETE them on the next sync tick, producing audit noise forever even
	// after the write-side guard refuses every attempt.
	baseQuery := `SELECT id, email, username, first_name, last_name, is_active,
	              COALESCE(scim_external_id, '') as scim_external_id, created_at, updated_at
	              FROM users WHERE is_agent = false AND scim_managed = true`
	countQuery := `SELECT COUNT(*) FROM users WHERE is_agent = false AND scim_managed = true`

	args := []interface{}{}
	if filterResult.WhereClause != "" {
		baseQuery += " AND " + filterResult.WhereClause
		countQuery += " AND " + filterResult.WhereClause
		args = filterResult.Args
	}

	var totalResults int
	if err = h.db.QueryRow(countQuery, args...).Scan(&totalResults); err != nil {
		return nil, fmt.Errorf("failed to count users: %w", err)
	}

	offset := startIndex - 1
	baseQuery += fmt.Sprintf(" ORDER BY id LIMIT %d OFFSET %d", count, offset)

	rows, err := h.db.Query(baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	resources := make([]interface{}, 0)
	for rows.Next() {
		var user models.User
		var scimExternalID string
		err := rows.Scan(&user.ID, &user.Email, &user.Username, &user.FirstName,
			&user.LastName, &user.IsActive, &scimExternalID, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			continue
		}
		user.SCIMExternalID = scimExternalID
		resources = append(resources, h.userToSCIM(&user))
	}

	return &models.SCIMListResponse{
		Schemas:      []string{models.SCIMSchemaListResponse},
		TotalResults: totalResults,
		StartIndex:   startIndex,
		ItemsPerPage: len(resources),
		Resources:    resources,
	}, nil
}

// listGroupsFiltered queries groups with a SCIM filter and pagination, returning a list response.
func (h *SCIMHandler) listGroupsFiltered(filter string, startIndex, count int) (*models.SCIMListResponse, error) {
	filterResult, err := ParseSCIMFilterWithAnd(filter, "Group")
	if err != nil {
		return nil, fmt.Errorf("invalid filter: %w", err)
	}

	baseQuery := `SELECT id, name, description, COALESCE(scim_external_id, '') as scim_external_id,
	              created_at, updated_at FROM groups WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM groups WHERE 1=1`

	args := []interface{}{}
	if filterResult.WhereClause != "" {
		baseQuery += " AND " + filterResult.WhereClause
		countQuery += " AND " + filterResult.WhereClause
		args = filterResult.Args
	}

	var totalResults int
	if err = h.db.QueryRow(countQuery, args...).Scan(&totalResults); err != nil {
		return nil, fmt.Errorf("failed to count groups: %w", err)
	}

	offset := startIndex - 1
	baseQuery += fmt.Sprintf(" ORDER BY id LIMIT %d OFFSET %d", count, offset)

	rows, err := h.db.Query(baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	resources := make([]interface{}, 0)
	for rows.Next() {
		var group models.TeamGroup
		var scimExternalID string
		err := rows.Scan(&group.ID, &group.Name, &group.Description, &scimExternalID,
			&group.CreatedAt, &group.UpdatedAt)
		if err != nil {
			continue
		}
		group.SCIMExternalID = scimExternalID
		members, _ := h.getGroupMembers(group.ID)
		resources = append(resources, h.groupToSCIM(&group, members))
	}

	return &models.SCIMListResponse{
		Schemas:      []string{models.SCIMSchemaListResponse},
		TotalResults: totalResults,
		StartIndex:   startIndex,
		ItemsPerPage: len(resources),
		Resources:    resources,
	}, nil
}

// ListUsers returns users with filtering (GET /scim/v2/Users)
func (h *SCIMHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	startIndexStr := r.URL.Query().Get("startIndex")
	countStr := r.URL.Query().Get("count")

	startIndex := 1
	if startIndexStr != "" {
		if val, err := strconv.Atoi(startIndexStr); err == nil && val > 0 {
			startIndex = val
		}
	}

	count := 100
	if countStr != "" {
		if val, err := strconv.Atoi(countStr); err == nil && val > 0 && val <= 200 {
			count = val
		}
	}

	response, err := h.listUsersFiltered(filter, startIndex, count)
	if err != nil {
		if strings.Contains(err.Error(), "invalid filter") {
			respondSCIMErrorMsg(w, http.StatusBadRequest, err.Error(), "invalidFilter")
		} else {
			respondSCIMErrorMsg(w, http.StatusInternalServerError, err.Error(), "")
		}
		return
	}

	respondSCIMJSON(w, http.StatusOK, response)
}

// CreateUser creates a new SCIM-managed user (POST /scim/v2/Users)
func (h *SCIMHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	// Security: Limit request body size to prevent memory exhaustion
	h.limitRequestBody(w, r)

	var scimUser models.SCIMUser
	if err := json.NewDecoder(r.Body).Decode(&scimUser); err != nil {
		if err.Error() == "http: request body too large" {
			respondSCIMErrorMsg(w, http.StatusRequestEntityTooLarge, "Request body too large", "tooLarge")
			return
		}
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid request body", "invalidValue")
		return
	}

	// Validate required fields
	if scimUser.UserName == "" {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "userName is required", "invalidValue")
		return
	}

	// Extract email from emails array or userName
	email := scimUser.UserName
	if len(scimUser.Emails) > 0 {
		for _, e := range scimUser.Emails {
			if e.Primary || email == scimUser.UserName {
				email = e.Value
				if e.Primary {
					break
				}
			}
		}
	}

	// Resolve active flag: default to true when omitted (SCIM spec)
	isActive := true
	if scimUser.Active != nil {
		isActive = *scimUser.Active
	}

	// Check for existing user by email (adopt into SCIM if matched)
	var existingUser models.User
	err := h.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active,
		       COALESCE(scim_managed, false), created_at, updated_at
		FROM users WHERE email = ?
	`, email).Scan(&existingUser.ID, &existingUser.Email, &existingUser.Username,
		&existingUser.FirstName, &existingUser.LastName, &existingUser.IsActive,
		&existingUser.SCIMManaged, &existingUser.CreatedAt, &existingUser.UpdatedAt)
	if err == nil {
		// Adopt existing user: link to SCIM
		username := scimUser.UserName
		if username == "" {
			username = existingUser.Username
		}
		_, err = h.db.Exec(`
			UPDATE users SET username = ?, scim_managed = true, scim_external_id = ?,
			                 is_active = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, username, nullIfEmpty(scimUser.ExternalID), isActive, existingUser.ID)
		if err != nil {
			slog.Error("SCIM: failed to adopt existing user", slog.Any("error", err), slog.String("email", email))
			respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to adopt existing user", "")
			return
		}

		user, err := h.getUserByID(existingUser.ID)
		if err != nil {
			respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to retrieve adopted user", "")
			return
		}

		h.logSCIMAuditEvent(r, logger.ActionSCIMUserCreate, logger.ResourceUser, &existingUser.ID, email,
			map[string]interface{}{
				"username":     username,
				"email":        email,
				"adopted":      true,
				"old_username": existingUser.Username,
			}, true, "")

		respondSCIMJSON(w, http.StatusOK, h.userToSCIM(user))
		return
	}

	// Check for username collision (email didn't match, but username might)
	var collidingID int
	err = h.db.QueryRow(`SELECT id FROM users WHERE username = ?`, scimUser.UserName).Scan(&collidingID)
	if err == nil {
		respondSCIMErrorMsg(w, http.StatusConflict, "User with this username already exists", "uniqueness")
		return
	}

	// Extract name components
	firstName := scimUser.Name.GivenName
	lastName := scimUser.Name.FamilyName
	if firstName == "" && lastName == "" && scimUser.DisplayName != "" {
		// Try to split displayName
		parts := strings.SplitN(scimUser.DisplayName, " ", 2)
		firstName = parts[0]
		if len(parts) > 1 {
			lastName = parts[1]
		}
	}
	if firstName == "" {
		firstName = scimUser.UserName
	}
	if lastName == "" {
		lastName = ""
	}

	// Insert user
	var userID int64
	err = h.db.QueryRow(`
		INSERT INTO users (email, username, first_name, last_name, is_active,
		                   scim_external_id, scim_managed, email_verified)
		VALUES (?, ?, ?, ?, ?, ?, true, true) RETURNING id
	`, email, scimUser.UserName, firstName, lastName, isActive, nullIfEmpty(scimUser.ExternalID)).Scan(&userID)
	if err != nil {
		slog.Error("SCIM: failed to create user", slog.Any("error", err), slog.String("email", email))
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to create user", "")
		return
	}

	// Fetch created user
	user, err := h.getUserByID(int(userID))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to retrieve created user", "")
		return
	}

	// Audit log: SCIM user created
	userIDInt := int(userID)
	h.logSCIMAuditEvent(r, logger.ActionSCIMUserCreate, logger.ResourceUser, &userIDInt, email,
		map[string]interface{}{"username": scimUser.UserName, "email": email}, true, "")

	respondSCIMJSON(w, http.StatusCreated, h.userToSCIM(user))
}

// GetUser returns a single user (GET /scim/v2/Users/{id})
func (h *SCIMHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid user ID", "invalidValue")
		return
	}

	user, err := h.getUserByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusNotFound, "User not found", "")
		return
	}

	// Mirror the list-query scope: SCIM must not acknowledge agents or
	// locally managed humans. 404 (not 403) keeps row existence opaque.
	if user.IsAgent || !user.SCIMManaged {
		respondSCIMErrorMsg(w, http.StatusNotFound, "User not found", "")
		return
	}

	respondSCIMJSON(w, http.StatusOK, h.userToSCIM(user))
}

// ReplaceUser fully replaces a user (PUT /scim/v2/Users/{id})
func (h *SCIMHandler) ReplaceUser(w http.ResponseWriter, r *http.Request) {
	// Security: Limit request body size to prevent memory exhaustion
	h.limitRequestBody(w, r)

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid user ID", "invalidValue")
		return
	}

	// Verify user exists
	existingUser, err := h.getUserByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusNotFound, "User not found", "")
		return
	}

	// SCIM PUT must not reach past IdP-provisioned users. Local users get
	// adopted into SCIM through POST's collision-by-email path, never via
	// PUT. See DeleteUser for the full rationale.
	if !existingUser.SCIMManaged {
		h.logSCIMAuditEvent(r, logger.ActionSCIMUserUpdate, logger.ResourceUser, &id, existingUser.Email,
			map[string]interface{}{
				"username": existingUser.Username,
				"reason":   "target_not_scim_managed",
			}, false, "refused: user is not SCIM-managed")
		respondSCIMErrorMsg(w, http.StatusNotFound, "User not found", "")
		return
	}

	var scimUser models.SCIMUser
	if err = json.NewDecoder(r.Body).Decode(&scimUser); err != nil {
		if err.Error() == "http: request body too large" {
			respondSCIMErrorMsg(w, http.StatusRequestEntityTooLarge, "Request body too large", "tooLarge")
			return
		}
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid request body", "invalidValue")
		return
	}

	// Extract email
	email := existingUser.Email
	if len(scimUser.Emails) > 0 {
		for _, e := range scimUser.Emails {
			if e.Primary {
				email = e.Value
				break
			}
		}
		if email == existingUser.Email && len(scimUser.Emails) > 0 {
			email = scimUser.Emails[0].Value
		}
	}

	// Extract name
	firstName := scimUser.Name.GivenName
	lastName := scimUser.Name.FamilyName
	if firstName == "" {
		firstName = existingUser.FirstName
	}
	if lastName == "" {
		lastName = existingUser.LastName
	}

	// Resolve active flag: preserve existing when omitted
	isActive := existingUser.IsActive
	if scimUser.Active != nil {
		isActive = *scimUser.Active
	}

	// Update user
	_, err = h.db.Exec(`
		UPDATE users SET email = ?, username = ?, first_name = ?, last_name = ?,
		                 is_active = ?, scim_external_id = ?, scim_managed = true,
		                 updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, email, scimUser.UserName, firstName, lastName, isActive, nullIfEmpty(scimUser.ExternalID), id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to update user", "")
		return
	}

	// Fetch updated user
	user, err := h.getUserByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to retrieve updated user", "")
		return
	}

	// Audit log: SCIM user updated (full replace)
	h.logSCIMAuditEvent(r, logger.ActionSCIMUserUpdate, logger.ResourceUser, &id, email,
		map[string]interface{}{
			"username":     scimUser.UserName,
			"email":        email,
			"active":       isActive,
			"old_username": existingUser.Username,
			"old_email":    existingUser.Email,
		}, true, "")

	// Alert on active→inactive transitions (PUT can deactivate a user via Active: false).
	if existingUser.IsActive && !isActive {
		h.handleSCIMUserDeactivation(r, id, existingUser.Username, "scim_replace", existingUser.SCIMManaged)
	}

	respondSCIMJSON(w, http.StatusOK, h.userToSCIM(user))
}

// PatchUser partially updates a user (PATCH /scim/v2/Users/{id})
func (h *SCIMHandler) PatchUser(w http.ResponseWriter, r *http.Request) {
	// Security: Limit request body size to prevent memory exhaustion
	h.limitRequestBody(w, r)

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid user ID", "invalidValue")
		return
	}

	// Capture a snapshot so applyUserPatchOp can record old/new values per attribute.
	// The snapshot is mutated in place as each op applies, so subsequent ops on the
	// same attribute still see the prior-op value as "old".
	snapshot, err := h.getUserByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusNotFound, "User not found", "")
		return
	}

	// SCIM PATCH must not reach past IdP-provisioned users — see DeleteUser
	// for the full rationale.
	if !snapshot.SCIMManaged {
		h.logSCIMAuditEvent(r, logger.ActionSCIMUserUpdate, logger.ResourceUser, &id, snapshot.Email,
			map[string]interface{}{
				"username": snapshot.Username,
				"reason":   "target_not_scim_managed",
			}, false, "refused: user is not SCIM-managed")
		respondSCIMErrorMsg(w, http.StatusNotFound, "User not found", "")
		return
	}

	var patchReq models.SCIMPatchRequest
	if err = json.NewDecoder(r.Body).Decode(&patchReq); err != nil {
		if err.Error() == "http: request body too large" {
			respondSCIMErrorMsg(w, http.StatusRequestEntityTooLarge, "Request body too large", "tooLarge")
			return
		}
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid request body", "invalidValue")
		return
	}

	var changes []attrChange
	for _, op := range patchReq.Operations {
		opChanges, opErr := h.applyUserPatchOp(snapshot, op)
		if opErr != nil {
			h.logPatchOpError(r, "user", id, op, opErr)
			respondSCIMErrorMsg(w, http.StatusBadRequest, "Patch operation failed", "invalidValue")
			return
		}
		changes = append(changes, opChanges...)
	}

	// Fetch updated user
	user, err := h.getUserByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to retrieve updated user", "")
		return
	}

	// Audit log: SCIM user patched
	h.logSCIMAuditEvent(r, logger.ActionSCIMUserUpdate, logger.ResourceUser, &id, user.Email,
		map[string]interface{}{
			"operation_count": len(patchReq.Operations),
			"changes":         changes,
		}, true, "")

	// Alert on active→inactive transitions applied through this PATCH.
	for _, c := range changes {
		if c.Path != "active" {
			continue
		}
		oldActive, _ := c.OldValue.(bool)
		newActive, _ := c.NewValue.(bool)
		if oldActive && !newActive {
			h.handleSCIMUserDeactivation(r, id, user.Username, "scim_patch", user.SCIMManaged)
			break
		}
	}

	respondSCIMJSON(w, http.StatusOK, h.userToSCIM(user))
}

// DeleteUser deactivates a user (DELETE /scim/v2/Users/{id})
func (h *SCIMHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid user ID", "invalidValue")
		return
	}

	// Get user info for audit logging before deactivation
	user, err := h.getUserByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusNotFound, "User not found", "")
		return
	}

	// SCIM must only operate on users it provisioned. A local user's ID
	// could still collide with a SCIM client's request (misconfig, bad
	// mapping, credential abuse), and silently honoring those deactivates
	// admins and local accounts the IdP never owned.
	if !user.SCIMManaged {
		h.logSCIMAuditEvent(r, logger.ActionSCIMUserDelete, logger.ResourceUser, &id, user.Email,
			map[string]interface{}{
				"username": user.Username,
				"reason":   "target_not_scim_managed",
			}, false, "refused: user is not SCIM-managed")
		respondSCIMErrorMsg(w, http.StatusNotFound, "User not found", "")
		return
	}

	// Deactivate rather than delete
	_, err = h.db.Exec(`UPDATE users SET is_active = false, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to delete user", "")
		return
	}

	// Audit log: SCIM user deactivated
	h.logSCIMAuditEvent(r, logger.ActionSCIMUserDelete, logger.ResourceUser, &id, user.Email,
		map[string]interface{}{"username": user.Username, "email": user.Email}, true, "")

	// Cascade to owned agents + revoke their tokens, and notify admins.
	h.handleSCIMUserDeactivation(r, id, user.Username, "scim_delete", user.SCIMManaged)

	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// Group Endpoints
// =============================================================================

// ListGroups returns groups with filtering (GET /scim/v2/Groups)
func (h *SCIMHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	startIndexStr := r.URL.Query().Get("startIndex")
	countStr := r.URL.Query().Get("count")

	startIndex := 1
	if startIndexStr != "" {
		if val, err := strconv.Atoi(startIndexStr); err == nil && val > 0 {
			startIndex = val
		}
	}

	count := 100
	if countStr != "" {
		if val, err := strconv.Atoi(countStr); err == nil && val > 0 && val <= 200 {
			count = val
		}
	}

	response, err := h.listGroupsFiltered(filter, startIndex, count)
	if err != nil {
		if strings.Contains(err.Error(), "invalid filter") {
			respondSCIMErrorMsg(w, http.StatusBadRequest, err.Error(), "invalidFilter")
		} else {
			respondSCIMErrorMsg(w, http.StatusInternalServerError, err.Error(), "")
		}
		return
	}

	respondSCIMJSON(w, http.StatusOK, response)
}

// CreateGroup creates a new SCIM-managed group (POST /scim/v2/Groups)
func (h *SCIMHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	// Security: Limit request body size to prevent memory exhaustion
	h.limitRequestBody(w, r)

	var scimGroup models.SCIMGroup
	if err := json.NewDecoder(r.Body).Decode(&scimGroup); err != nil {
		if err.Error() == "http: request body too large" {
			respondSCIMErrorMsg(w, http.StatusRequestEntityTooLarge, "Request body too large", "tooLarge")
			return
		}
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid request body", "invalidValue")
		return
	}

	if scimGroup.DisplayName == "" {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "displayName is required", "invalidValue")
		return
	}

	// Check for existing group with same name
	var existingID int
	err := h.db.QueryRow(`SELECT id FROM groups WHERE name = ?`, scimGroup.DisplayName).Scan(&existingID)
	if err == nil {
		respondSCIMErrorMsg(w, http.StatusConflict, "Group with this name already exists", "uniqueness")
		return
	}

	// Insert group
	var groupID int64
	err = h.db.QueryRow(`
		INSERT INTO groups (name, description, scim_external_id, scim_managed, is_active)
		VALUES (?, '', ?, true, true) RETURNING id
	`, scimGroup.DisplayName, nullIfEmpty(scimGroup.ExternalID)).Scan(&groupID)
	if err != nil {
		slog.Error("SCIM: failed to create group", slog.Any("error", err), slog.String("name", scimGroup.DisplayName))
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to create group", "")
		return
	}

	groupIDInt := int(groupID)
	groupRef := &models.TeamGroup{ID: groupIDInt, Name: scimGroup.DisplayName}

	// Add members. Each insert is audited individually so the trail shows exactly
	// which users were provisioned into this group and which (if any) failed.
	for _, member := range scimGroup.Members {
		memberID, convErr := strconv.Atoi(member.Value)
		if convErr != nil {
			continue
		}
		_, execErr := h.db.Exec(`
			INSERT INTO group_members (group_id, user_id, scim_managed, added_at)
			VALUES (?, ?, true, CURRENT_TIMESTAMP)
		`, groupID, memberID)
		h.logGroupMemberChange(r, logger.ActionSCIMGroupAddMember, groupRef, memberID, execErr)
	}

	// Invalidate permission caches for new group members
	if len(scimGroup.Members) > 0 {
		_ = h.permissionService.InvalidateGroupMemberCaches(groupIDInt)
	}

	// Fetch created group
	group, err := h.getGroupByID(groupIDInt)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to retrieve created group", "")
		return
	}

	members, _ := h.getGroupMembers(groupIDInt)

	// Audit log: SCIM group created (aggregate event; per-member events are above)
	h.logSCIMAuditEvent(r, logger.ActionSCIMGroupCreate, logger.ResourceGroup, &groupIDInt, scimGroup.DisplayName,
		map[string]interface{}{"member_count": len(scimGroup.Members)}, true, "")

	respondSCIMJSON(w, http.StatusCreated, h.groupToSCIM(group, members))
}

// GetGroup returns a single group (GET /scim/v2/Groups/{id})
func (h *SCIMHandler) GetGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid group ID", "invalidValue")
		return
	}

	group, err := h.getGroupByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusNotFound, "Group not found", "")
		return
	}

	members, _ := h.getGroupMembers(id)
	respondSCIMJSON(w, http.StatusOK, h.groupToSCIM(group, members))
}

// ReplaceGroup fully replaces a group (PUT /scim/v2/Groups/{id})
func (h *SCIMHandler) ReplaceGroup(w http.ResponseWriter, r *http.Request) {
	// Security: Limit request body size to prevent memory exhaustion
	h.limitRequestBody(w, r)

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid group ID", "invalidValue")
		return
	}

	existingGroup, err := h.getGroupByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusNotFound, "Group not found", "")
		return
	}

	var scimGroup models.SCIMGroup
	if err = json.NewDecoder(r.Body).Decode(&scimGroup); err != nil {
		if err.Error() == "http: request body too large" {
			respondSCIMErrorMsg(w, http.StatusRequestEntityTooLarge, "Request body too large", "tooLarge")
			return
		}
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid request body", "invalidValue")
		return
	}

	// Update group
	_, err = h.db.Exec(`
		UPDATE groups SET name = ?, scim_external_id = ?, scim_managed = true,
		                  updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, scimGroup.DisplayName, nullIfEmpty(scimGroup.ExternalID), id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to update group", "")
		return
	}

	// Invalidate permission caches before replacing members (covers removed members)
	_ = h.permissionService.InvalidateGroupMemberCaches(id)

	groupRef := &models.TeamGroup{ID: id, Name: scimGroup.DisplayName}

	// Capture existing SCIM-managed member IDs so we can emit a remove audit
	// entry per departing user. We do this before the bulk DELETE below.
	var priorMemberIDs []int
	rows, selErr := h.db.Query(`SELECT user_id FROM group_members WHERE group_id = ? AND scim_managed = true`, id)
	if selErr == nil {
		for rows.Next() {
			var uid int
			if scanErr := rows.Scan(&uid); scanErr == nil {
				priorMemberIDs = append(priorMemberIDs, uid)
			}
		}
		_ = rows.Close()
	}

	// Replace members - remove SCIM-managed members and add new ones
	_, delErr := h.db.Exec(`DELETE FROM group_members WHERE group_id = ? AND scim_managed = true`, id)
	for _, uid := range priorMemberIDs {
		h.logGroupMemberChange(r, logger.ActionSCIMGroupRemoveMember, groupRef, uid, delErr)
	}
	for _, member := range scimGroup.Members {
		memberID, convErr := strconv.Atoi(member.Value)
		if convErr != nil {
			continue
		}
		_, execErr := h.db.Exec(`
			INSERT INTO group_members (group_id, user_id, scim_managed, added_at)
			VALUES (?, ?, true, CURRENT_TIMESTAMP)
			ON CONFLICT(group_id, user_id) DO UPDATE SET scim_managed = true
		`, id, memberID)
		h.logGroupMemberChange(r, logger.ActionSCIMGroupAddMember, groupRef, memberID, execErr)
	}

	// Invalidate again for newly added members
	_ = h.permissionService.InvalidateGroupMemberCaches(id)

	group, err := h.getGroupByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to retrieve updated group", "")
		return
	}

	members, _ := h.getGroupMembers(id)

	// Audit log: SCIM group updated (full replace)
	h.logSCIMAuditEvent(r, logger.ActionSCIMGroupUpdate, logger.ResourceGroup, &id, scimGroup.DisplayName,
		map[string]interface{}{
			"old_name":     existingGroup.Name,
			"new_name":     scimGroup.DisplayName,
			"member_count": len(scimGroup.Members),
		}, true, "")

	respondSCIMJSON(w, http.StatusOK, h.groupToSCIM(group, members))
}

// PatchGroup partially updates a group (PATCH /scim/v2/Groups/{id})
func (h *SCIMHandler) PatchGroup(w http.ResponseWriter, r *http.Request) {
	// Security: Limit request body size to prevent memory exhaustion
	h.limitRequestBody(w, r)

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid group ID", "invalidValue")
		return
	}

	snapshot, err := h.getGroupByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusNotFound, "Group not found", "")
		return
	}

	var patchReq models.SCIMPatchRequest
	if err = json.NewDecoder(r.Body).Decode(&patchReq); err != nil {
		if err.Error() == "http: request body too large" {
			respondSCIMErrorMsg(w, http.StatusRequestEntityTooLarge, "Request body too large", "tooLarge")
			return
		}
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid request body", "invalidValue")
		return
	}

	hasMemberOps := false
	var changes []attrChange
	for _, op := range patchReq.Operations {
		if strings.EqualFold(op.Path, "members") || strings.HasPrefix(strings.ToLower(op.Path), "members[") {
			hasMemberOps = true
		}
		opChanges, opErr := h.applyGroupPatchOp(r, snapshot, op)
		if opErr != nil {
			h.logPatchOpError(r, "group", id, op, opErr)
			respondSCIMErrorMsg(w, http.StatusBadRequest, "Patch operation failed", "invalidValue")
			return
		}
		changes = append(changes, opChanges...)
	}

	// Invalidate permission caches if any member operations were applied
	if hasMemberOps {
		_ = h.permissionService.InvalidateGroupMemberCaches(id)
	}

	group, err := h.getGroupByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to retrieve updated group", "")
		return
	}

	members, _ := h.getGroupMembers(id)

	// Audit log: SCIM group patched (aggregate; per-member events emitted inside applyGroupPatchOp)
	h.logSCIMAuditEvent(r, logger.ActionSCIMGroupUpdate, logger.ResourceGroup, &id, group.Name,
		map[string]interface{}{
			"operation_count": len(patchReq.Operations),
			"changes":         changes,
		}, true, "")

	respondSCIMJSON(w, http.StatusOK, h.groupToSCIM(group, members))
}

// DeleteGroup deletes a group (DELETE /scim/v2/Groups/{id})
func (h *SCIMHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid group ID", "invalidValue")
		return
	}

	// Get group info for audit logging before deletion
	group, err := h.getGroupByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusNotFound, "Group not found", "")
		return
	}

	// Invalidate permission caches for group members before deletion
	_ = h.permissionService.InvalidateGroupMemberCaches(id)

	_, err = h.db.Exec(`DELETE FROM groups WHERE id = ?`, id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to delete group", "")
		return
	}

	// Audit log: SCIM group deleted
	h.logSCIMAuditEvent(r, logger.ActionSCIMGroupDelete, logger.ResourceGroup, &id, group.Name,
		nil, true, "")

	w.WriteHeader(http.StatusNoContent)
}

// =============================================================================
// Me Endpoint
// =============================================================================

// GetMe returns 501 Not Implemented (GET /scim/v2/Me)
func (h *SCIMHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	respondSCIMErrorMsg(w, http.StatusNotImplemented, "The /Me endpoint is not implemented", "")
}

// =============================================================================
// Search Endpoint
// =============================================================================

// SearchRequest handles POST /.search (RFC 7644 Section 3.4.3)
func (h *SCIMHandler) SearchRequest(w http.ResponseWriter, r *http.Request) {
	h.limitRequestBody(w, r)

	var searchReq models.SCIMSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&searchReq); err != nil {
		if err.Error() == "http: request body too large" {
			respondSCIMErrorMsg(w, http.StatusRequestEntityTooLarge, "Request body too large", "tooLarge")
			return
		}
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid request body", "invalidValue")
		return
	}

	startIndex := searchReq.StartIndex
	if startIndex < 1 {
		startIndex = 1
	}
	count := searchReq.Count
	if count <= 0 {
		count = 100
	}
	if count > 200 {
		count = 200
	}

	// Extract resource type from filter
	resourceType, remainingFilter := ExtractResourceTypeFilter(searchReq.Filter)

	switch resourceType {
	case "User":
		response, err := h.listUsersFiltered(remainingFilter, startIndex, count)
		if err != nil {
			if strings.Contains(err.Error(), "invalid filter") {
				respondSCIMErrorMsg(w, http.StatusBadRequest, err.Error(), "invalidFilter")
			} else {
				respondSCIMErrorMsg(w, http.StatusInternalServerError, err.Error(), "")
			}
			return
		}
		respondSCIMJSON(w, http.StatusOK, response)

	case "Group":
		response, err := h.listGroupsFiltered(remainingFilter, startIndex, count)
		if err != nil {
			if strings.Contains(err.Error(), "invalid filter") {
				respondSCIMErrorMsg(w, http.StatusBadRequest, err.Error(), "invalidFilter")
			} else {
				respondSCIMErrorMsg(w, http.StatusInternalServerError, err.Error(), "")
			}
			return
		}
		respondSCIMJSON(w, http.StatusOK, response)

	default:
		// No resource type specified — search both and combine
		userResp, userErr := h.listUsersFiltered(remainingFilter, startIndex, count)
		groupResp, groupErr := h.listGroupsFiltered(remainingFilter, startIndex, count)

		combined := models.SCIMListResponse{
			Schemas:      []string{models.SCIMSchemaListResponse},
			TotalResults: 0,
			StartIndex:   startIndex,
			Resources:    make([]interface{}, 0),
		}

		if userErr == nil {
			combined.TotalResults += userResp.TotalResults
			combined.Resources = append(combined.Resources, userResp.Resources...)
		}
		if groupErr == nil {
			combined.TotalResults += groupResp.TotalResults
			combined.Resources = append(combined.Resources, groupResp.Resources...)
		}
		combined.ItemsPerPage = len(combined.Resources)

		if userErr != nil && groupErr != nil {
			respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to search resources", "")
			return
		}

		respondSCIMJSON(w, http.StatusOK, combined)
	}
}

// SearchUsers handles POST /Users/.search — resource-specific search per RFC 7644 §3.4.3
func (h *SCIMHandler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	h.limitRequestBody(w, r)

	var searchReq models.SCIMSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&searchReq); err != nil {
		if err.Error() == "http: request body too large" {
			respondSCIMErrorMsg(w, http.StatusRequestEntityTooLarge, "Request body too large", "tooLarge")
			return
		}
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid request body", "invalidValue")
		return
	}

	startIndex := searchReq.StartIndex
	if startIndex < 1 {
		startIndex = 1
	}
	count := searchReq.Count
	if count <= 0 {
		count = 100
	}
	if count > 200 {
		count = 200
	}

	response, err := h.listUsersFiltered(searchReq.Filter, startIndex, count)
	if err != nil {
		if strings.Contains(err.Error(), "invalid filter") {
			respondSCIMErrorMsg(w, http.StatusBadRequest, err.Error(), "invalidFilter")
		} else {
			respondSCIMErrorMsg(w, http.StatusInternalServerError, err.Error(), "")
		}
		return
	}
	respondSCIMJSON(w, http.StatusOK, response)
}

// SearchGroups handles POST /Groups/.search — resource-specific search per RFC 7644 §3.4.3
func (h *SCIMHandler) SearchGroups(w http.ResponseWriter, r *http.Request) {
	h.limitRequestBody(w, r)

	var searchReq models.SCIMSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&searchReq); err != nil {
		if err.Error() == "http: request body too large" {
			respondSCIMErrorMsg(w, http.StatusRequestEntityTooLarge, "Request body too large", "tooLarge")
			return
		}
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid request body", "invalidValue")
		return
	}

	startIndex := searchReq.StartIndex
	if startIndex < 1 {
		startIndex = 1
	}
	count := searchReq.Count
	if count <= 0 {
		count = 100
	}
	if count > 200 {
		count = 200
	}

	response, err := h.listGroupsFiltered(searchReq.Filter, startIndex, count)
	if err != nil {
		if strings.Contains(err.Error(), "invalid filter") {
			respondSCIMErrorMsg(w, http.StatusBadRequest, err.Error(), "invalidFilter")
		} else {
			respondSCIMErrorMsg(w, http.StatusInternalServerError, err.Error(), "")
		}
		return
	}
	respondSCIMJSON(w, http.StatusOK, response)
}

// =============================================================================
// Bulk Endpoint
// =============================================================================

// scimBulkMaxOperations is the maximum number of operations in a single bulk request
const scimBulkMaxOperations = 100

// BulkRequest handles POST /Bulk (RFC 7644 Section 3.7)
func (h *SCIMHandler) BulkRequest(w http.ResponseWriter, r *http.Request) {
	h.limitRequestBody(w, r)

	var bulkReq models.SCIMBulkRequest
	if err := json.NewDecoder(r.Body).Decode(&bulkReq); err != nil {
		if err.Error() == "http: request body too large" {
			respondSCIMErrorMsg(w, http.StatusRequestEntityTooLarge, "Request body too large", "tooLarge")
			return
		}
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid request body", "invalidValue")
		return
	}

	if len(bulkReq.Operations) > scimBulkMaxOperations {
		respondSCIMErrorMsg(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("Too many operations: %d (max %d)", len(bulkReq.Operations), scimBulkMaxOperations), "tooLarge")
		return
	}

	results := make([]models.SCIMBulkResponseOperation, 0, len(bulkReq.Operations))

	for _, op := range bulkReq.Operations {
		result := h.executeBulkOperation(r, op)
		results = append(results, result)
	}

	respondSCIMJSON(w, http.StatusOK, models.SCIMBulkResponse{
		Schemas:    []string{models.SCIMSchemaBulkResponse},
		Operations: results,
	})
}

// executeBulkOperation dispatches a single bulk operation to the appropriate handler
func (h *SCIMHandler) executeBulkOperation(originalReq *http.Request, op models.SCIMBulkOperation) models.SCIMBulkResponseOperation {
	method := strings.ToUpper(op.Method)

	// Validate method
	switch method {
	case "POST", "PUT", "PATCH", "DELETE", "GET":
		// ok
	default:
		return models.SCIMBulkResponseOperation{
			Method: method,
			BulkID: op.BulkID,
			Status: "400",
			Response: models.NewSCIMError(http.StatusBadRequest,
				"Unsupported method: "+method, "invalidValue"),
		}
	}

	// Build the sub-request
	var body *bytes.Reader
	if op.Data != nil {
		body = bytes.NewReader(op.Data)
	} else {
		body = bytes.NewReader(nil)
	}

	path := op.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	subReq, err := http.NewRequestWithContext(originalReq.Context(), method, path, body)
	if err != nil {
		return models.SCIMBulkResponseOperation{
			Method: method,
			BulkID: op.BulkID,
			Status: "400",
			Response: models.NewSCIMError(http.StatusBadRequest,
				"Failed to build request: "+err.Error(), ""),
		}
	}
	subReq.Header.Set("Content-Type", "application/scim+json")

	// Route to the correct handler
	handler := h.routeBulkOperation(method, path)
	if handler == nil {
		return models.SCIMBulkResponseOperation{
			Method: method,
			BulkID: op.BulkID,
			Status: "400",
			Response: models.NewSCIMError(http.StatusBadRequest,
				"Unknown resource path: "+op.Path, "invalidValue"),
		}
	}

	// Execute using httptest recorder
	recorder := httptest.NewRecorder()
	handler(recorder, subReq)

	result := models.SCIMBulkResponseOperation{
		Method: method,
		BulkID: op.BulkID,
		Status: strconv.Itoa(recorder.Code),
	}

	// Parse response body for location header and error responses
	if recorder.Header().Get("Location") != "" {
		result.Location = recorder.Header().Get("Location")
	}

	// Include response body for errors or resource creation
	if recorder.Body.Len() > 0 {
		var respBody interface{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &respBody); err == nil {
			if recorder.Code >= 400 {
				result.Response = respBody
			} else if method == "POST" || method == "PUT" || method == "GET" || method == "PATCH" {
				// For successful resource operations, extract location from the response
				if respMap, ok := respBody.(map[string]interface{}); ok {
					if meta, ok := respMap["meta"].(map[string]interface{}); ok {
						if loc, ok := meta["location"].(string); ok {
							result.Location = loc
						}
					}
				}
			}
		}
	}

	return result
}

// routeBulkOperation returns the handler function for a given method and path
func (h *SCIMHandler) routeBulkOperation(method, path string) http.HandlerFunc {
	// Normalize path: strip leading /scim/v2 prefix if present
	path = strings.TrimPrefix(path, "/scim/v2")

	// Parse the path to determine resource type and optional ID
	parts := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 2)
	if len(parts) == 0 {
		return nil
	}

	resource := parts[0]
	hasID := len(parts) > 1 && parts[1] != ""

	if hasID {
		// Inject path value for {id} parameter via SetPathValue
		id := parts[1]
		return func(w http.ResponseWriter, r *http.Request) {
			r.SetPathValue("id", id)
			switch resource {
			case "Users":
				switch method {
				case "GET":
					h.GetUser(w, r)
				case "PUT":
					h.ReplaceUser(w, r)
				case "PATCH":
					h.PatchUser(w, r)
				case "DELETE":
					h.DeleteUser(w, r)
				default:
					respondSCIMErrorMsg(w, http.StatusMethodNotAllowed, "Method not allowed", "")
				}
			case "Groups":
				switch method {
				case "GET":
					h.GetGroup(w, r)
				case "PUT":
					h.ReplaceGroup(w, r)
				case "PATCH":
					h.PatchGroup(w, r)
				case "DELETE":
					h.DeleteGroup(w, r)
				default:
					respondSCIMErrorMsg(w, http.StatusMethodNotAllowed, "Method not allowed", "")
				}
			default:
				respondSCIMErrorMsg(w, http.StatusBadRequest, "Unknown resource: "+resource, "invalidValue")
			}
		}
	}

	// Collection-level operations (no ID)
	switch resource {
	case "Users":
		if method == "POST" {
			return h.CreateUser
		}
		if method == "GET" {
			return h.ListUsers
		}
	case "Groups":
		if method == "POST" {
			return h.CreateGroup
		}
		if method == "GET" {
			return h.ListGroups
		}
	}

	return nil
}

// =============================================================================
// Helper Methods
// =============================================================================

func (h *SCIMHandler) getUserByID(id int) (*models.User, error) {
	var user models.User
	var scimExternalID sql.NullString
	err := h.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active,
		       scim_external_id, COALESCE(scim_managed, false), COALESCE(is_agent, false),
		       created_at, updated_at
		FROM users WHERE id = ?
	`, id).Scan(&user.ID, &user.Email, &user.Username, &user.FirstName, &user.LastName,
		&user.IsActive, &scimExternalID, &user.SCIMManaged, &user.IsAgent,
		&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if scimExternalID.Valid {
		user.SCIMExternalID = scimExternalID.String
	}
	return &user, nil
}

func (h *SCIMHandler) getGroupByID(id int) (*models.TeamGroup, error) {
	var group models.TeamGroup
	var scimExternalID sql.NullString
	err := h.db.QueryRow(`
		SELECT id, name, description, scim_external_id, COALESCE(scim_managed, false),
		       created_at, updated_at
		FROM groups WHERE id = ?
	`, id).Scan(&group.ID, &group.Name, &group.Description, &scimExternalID,
		&group.SCIMManaged, &group.CreatedAt, &group.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if scimExternalID.Valid {
		group.SCIMExternalID = scimExternalID.String
	}
	return &group, nil
}

func (h *SCIMHandler) getGroupMembers(groupID int) ([]models.SCIMGroupMember, error) {
	rows, err := h.db.Query(`
		SELECT u.id, u.first_name, u.last_name, u.username
		FROM group_members gm
		JOIN users u ON gm.user_id = u.id
		WHERE gm.group_id = ?
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var members []models.SCIMGroupMember
	for rows.Next() {
		var userID int
		var firstName, lastName, username string
		if err := rows.Scan(&userID, &firstName, &lastName, &username); err != nil {
			continue
		}
		displayName := strings.TrimSpace(firstName + " " + lastName)
		if displayName == "" {
			displayName = username
		}
		members = append(members, models.SCIMGroupMember{
			Value:   strconv.Itoa(userID),
			Ref:     h.baseURL + "/scim/v2/Users/" + strconv.Itoa(userID),
			Display: displayName,
		})
	}
	return members, nil
}

func (h *SCIMHandler) userToSCIM(user *models.User) *models.SCIMUser {
	displayName := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if displayName == "" {
		displayName = user.Username
	}

	return &models.SCIMUser{
		Schemas:    []string{models.SCIMSchemaUser},
		ID:         strconv.Itoa(user.ID),
		ExternalID: user.SCIMExternalID,
		UserName:   user.Username,
		Name: models.SCIMName{
			GivenName:  user.FirstName,
			FamilyName: user.LastName,
			Formatted:  displayName,
		},
		DisplayName: displayName,
		Emails: []models.SCIMEmail{
			{
				Value:   user.Email,
				Type:    "work",
				Primary: true,
			},
		},
		Active: &user.IsActive,
		Meta: &models.SCIMMeta{
			ResourceType: "User",
			Created:      &user.CreatedAt,
			LastModified: &user.UpdatedAt,
			Location:     h.baseURL + "/scim/v2/Users/" + strconv.Itoa(user.ID),
		},
	}
}

func (h *SCIMHandler) groupToSCIM(group *models.TeamGroup, members []models.SCIMGroupMember) *models.SCIMGroup {
	return &models.SCIMGroup{
		Schemas:     []string{models.SCIMSchemaGroup},
		ID:          strconv.Itoa(group.ID),
		ExternalID:  group.SCIMExternalID,
		DisplayName: group.Name,
		Members:     members,
		Meta: &models.SCIMMeta{
			ResourceType: "Group",
			Created:      &group.CreatedAt,
			LastModified: &group.UpdatedAt,
			Location:     h.baseURL + "/scim/v2/Groups/" + strconv.Itoa(group.ID),
		},
	}
}

// applyUserPatchOp applies a single SCIM PATCH operation to the user identified
// by snapshot.ID. It mutates the snapshot in place so subsequent ops see the
// previously-applied values, and returns the set of attribute changes it made
// (for audit logging). Unknown paths emit an "<unsupported>" breadcrumb in the
// returned changes slice instead of a SCIM error, so an IdP pushing an attribute
// we don't support (e.g. phoneNumbers) succeeds as an audited no-op rather than
// failing the whole PATCH — but still leaves a trail operators can grep.
func (h *SCIMHandler) applyUserPatchOp(snapshot *models.User, op models.SCIMPatchOp) ([]attrChange, error) {
	opLower := strings.ToLower(op.Op)
	userID := snapshot.ID

	switch opLower {
	case "replace", "add":
		path := strings.ToLower(op.Path)

		switch path {
		case "active":
			active, ok := op.Value.(bool)
			if !ok {
				if strVal, ok := op.Value.(string); ok {
					active = strings.EqualFold(strVal, "true")
				}
			}
			_, err := h.db.Exec(`UPDATE users SET is_active = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, active, userID)
			if err != nil {
				return nil, err
			}
			change := attrChange{Op: opLower, Path: "active", OldValue: snapshot.IsActive, NewValue: active}
			snapshot.IsActive = active
			return []attrChange{change}, nil

		case "username":
			if strVal, ok := op.Value.(string); ok {
				_, err := h.db.Exec(`UPDATE users SET username = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, strVal, userID)
				if err != nil {
					return nil, err
				}
				change := attrChange{Op: opLower, Path: "userName", OldValue: snapshot.Username, NewValue: strVal}
				snapshot.Username = strVal
				return []attrChange{change}, nil
			}

		case "name.givenname":
			if strVal, ok := op.Value.(string); ok {
				_, err := h.db.Exec(`UPDATE users SET first_name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, strVal, userID)
				if err != nil {
					return nil, err
				}
				change := attrChange{Op: opLower, Path: "name.givenName", OldValue: snapshot.FirstName, NewValue: strVal}
				snapshot.FirstName = strVal
				return []attrChange{change}, nil
			}

		case "name.familyname":
			if strVal, ok := op.Value.(string); ok {
				_, err := h.db.Exec(`UPDATE users SET last_name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, strVal, userID)
				if err != nil {
					return nil, err
				}
				change := attrChange{Op: opLower, Path: "name.familyName", OldValue: snapshot.LastName, NewValue: strVal}
				snapshot.LastName = strVal
				return []attrChange{change}, nil
			}

		case "externalid":
			if strVal, ok := op.Value.(string); ok {
				_, err := h.db.Exec(`UPDATE users SET scim_external_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, strVal, userID)
				if err != nil {
					return nil, err
				}
				change := attrChange{Op: opLower, Path: "externalId", OldValue: snapshot.SCIMExternalID, NewValue: strVal}
				snapshot.SCIMExternalID = strVal
				return []attrChange{change}, nil
			}

		case "":
			// No path - value should be an object with attributes
			if valueMap, ok := op.Value.(map[string]interface{}); ok {
				var changes []attrChange
				for key, val := range valueMap {
					subOp := models.SCIMPatchOp{Op: op.Op, Path: key, Value: val}
					subChanges, err := h.applyUserPatchOp(snapshot, subOp)
					if err != nil {
						return changes, err
					}
					changes = append(changes, subChanges...)
				}
				return changes, nil
			}
		}

	case "remove":
		if strings.EqualFold(op.Path, "externalId") {
			_, err := h.db.Exec(`UPDATE users SET scim_external_id = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, userID)
			if err != nil {
				return nil, err
			}
			change := attrChange{Op: opLower, Path: "externalId", OldValue: snapshot.SCIMExternalID, NewValue: nil}
			snapshot.SCIMExternalID = ""
			return []attrChange{change}, nil
		}
	}

	return []attrChange{{Op: opLower, Path: op.Path, NewValue: "<unsupported>"}}, nil
}

// applyGroupPatchOp applies a single SCIM PATCH operation to the group identified
// by snapshot.ID. It mutates snapshot for attribute changes and emits per-member
// add/remove audit events through the request's SCIM token context. Returns the
// set of attribute changes (member ops are audited individually, not returned here).
// Unknown paths emit an "<unsupported>" breadcrumb rather than a SCIM error — see
// applyUserPatchOp for the rationale.
func (h *SCIMHandler) applyGroupPatchOp(r *http.Request, snapshot *models.TeamGroup, op models.SCIMPatchOp) ([]attrChange, error) {
	opLower := strings.ToLower(op.Op)
	path := strings.ToLower(op.Path)
	groupID := snapshot.ID

	switch opLower {
	case "replace", "add":
		switch path {
		case "displayname":
			if strVal, ok := op.Value.(string); ok {
				_, err := h.db.Exec(`UPDATE groups SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, strVal, groupID)
				if err != nil {
					return nil, err
				}
				change := attrChange{Op: opLower, Path: "displayName", OldValue: snapshot.Name, NewValue: strVal}
				snapshot.Name = strVal
				return []attrChange{change}, nil
			}

		case "externalid":
			if strVal, ok := op.Value.(string); ok {
				_, err := h.db.Exec(`UPDATE groups SET scim_external_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, strVal, groupID)
				if err != nil {
					return nil, err
				}
				change := attrChange{Op: opLower, Path: "externalId", OldValue: snapshot.SCIMExternalID, NewValue: strVal}
				snapshot.SCIMExternalID = strVal
				return []attrChange{change}, nil
			}

		case "members":
			if members, ok := op.Value.([]interface{}); ok {
				for _, m := range members {
					memberMap, ok := m.(map[string]interface{})
					if !ok {
						continue
					}
					valueStr, ok := memberMap["value"].(string)
					if !ok {
						continue
					}
					memberID, err := strconv.Atoi(valueStr)
					if err != nil {
						continue
					}
					_, execErr := h.db.Exec(`
						INSERT INTO group_members (group_id, user_id, scim_managed, added_at)
						VALUES (?, ?, true, CURRENT_TIMESTAMP)
						ON CONFLICT(group_id, user_id) DO UPDATE SET scim_managed = true
					`, groupID, memberID)
					h.logGroupMemberChange(r, logger.ActionSCIMGroupAddMember, snapshot, memberID, execErr)
				}
			}
			return nil, nil
		}

	case "remove":
		if path == "members" || strings.HasPrefix(path, "members[") {
			if op.Value == nil {
				return nil, nil
			}
			members, ok := op.Value.([]interface{})
			if !ok {
				return nil, nil
			}
			for _, m := range members {
				memberMap, ok := m.(map[string]interface{})
				if !ok {
					continue
				}
				valueStr, ok := memberMap["value"].(string)
				if !ok {
					continue
				}
				memberID, err := strconv.Atoi(valueStr)
				if err != nil {
					continue
				}
				_, execErr := h.db.Exec(`DELETE FROM group_members WHERE group_id = ? AND user_id = ?`, groupID, memberID)
				h.logGroupMemberChange(r, logger.ActionSCIMGroupRemoveMember, snapshot, memberID, execErr)
			}
			return nil, nil
		}
	}

	return []attrChange{{Op: opLower, Path: op.Path, NewValue: "<unsupported>"}}, nil
}

// handleSCIMUserDeactivation cascades an owner's SCIM deactivation to any
// agents they own: flips those agents inactive and revokes all api_tokens held
// by the owner or their agents. Emits per-row audit entries mirroring the
// admin deactivation path, plus a baked-in notification to every active system
// admin so integrations can be re-pointed.
//
// trigger identifies which SCIM endpoint caused the cascade (one of
// "scim_delete", "scim_replace", "scim_patch") so operators can correlate the
// audit trail back to the IdP request pattern.
//
// scimManaged tells the admin notification whether the target user was
// actually provisioned via SCIM. A false value flags an anomaly worth
// calling out (a SCIM request deactivated a locally-managed user), which
// the copy surfaces so admins can investigate rather than assume routine
// IdP churn.
//
// Callers guard the active→inactive transition; this function always cascades
// (no-op if nothing is owned/active).
func (h *SCIMHandler) handleSCIMUserDeactivation(r *http.Request, userID int, username, trigger string, scimManaged bool) {
	cascade, err := services.DeactivateOwnedAgentsAndTokens(h.db, userID)
	if err != nil {
		slog.Error("scim: offboarding cascade failed",
			slog.Int("owner_id", userID),
			slog.String("trigger", trigger),
			slog.Any("error", err))
		h.logSCIMAuditEvent(r, logger.ActionSCIMUserAgentImpact, logger.ResourceUser, &userID, username,
			map[string]interface{}{"trigger": trigger}, false, err.Error())
		return
	}

	if len(cascade.AgentIDs) == 0 && len(cascade.RevokedAPITokens) == 0 {
		return
	}

	// Fan cache invalidation out to owner + cascaded agents so revoked agents
	// can't continue serving requests from a stale permission cache entry.
	_ = h.permissionService.InvalidateUserCache(userID)

	slog.Warn("scim: offboarding cascaded to agent users and tokens",
		slog.Int("owner_id", userID),
		slog.String("owner_username", username),
		slog.String("trigger", trigger),
		slog.Any("deactivated_agent_ids", cascade.AgentIDs),
		slog.Int("revoked_api_tokens", len(cascade.RevokedAPITokens)))

	// Aggregate audit row: one per cascade event. Carries the full impact set.
	h.logSCIMAuditEvent(r, logger.ActionSCIMUserAgentImpact, logger.ResourceUser, &userID, username,
		map[string]interface{}{
			"trigger":               trigger,
			"deactivated_agent_ids": cascade.AgentIDs,
			"revoked_api_tokens":    len(cascade.RevokedAPITokens),
		}, true, "")

	// Per-agent and per-token rows so security can reconstruct what died
	// alongside the SCIM offboarding. Mirrors the admin deactivation pattern.
	for _, aid := range cascade.AgentIDs {
		agentID := aid
		h.logSCIMAuditEvent(r, logger.ActionAgentDeactivate, logger.ResourceUser, &agentID, "",
			map[string]interface{}{
				"reason":   "scim_owner_deactivated",
				"owner_id": userID,
				"trigger":  trigger,
			}, true, "")
	}
	for _, tid := range cascade.RevokedAPITokens {
		tokenID := tid
		h.logSCIMAuditEvent(r, logger.ActionAPITokenAutoRevoke, logger.ResourceAPIToken, &tokenID, "",
			map[string]interface{}{
				"reason":   "scim_owner_deactivated",
				"owner_id": userID,
				"trigger":  trigger,
				"table":    "api_tokens",
			}, true, "")
	}

	h.notifyAdminsOfSCIMCascade(userID, username, trigger, scimManaged, cascade)
}

// notifyAdminsOfSCIMCascade inserts a single notification row per active
// system admin summarizing the cascade. Baked-in / hard-coded for now — a
// future config surface can route this through NotificationService with
// per-admin opt-in/out. Failure to write a notification never blocks the
// cascade; it is logged and the caller proceeds.
func (h *SCIMHandler) notifyAdminsOfSCIMCascade(ownerID int, ownerUsername, trigger string, scimManaged bool, cascade services.AgentDeactivationResult) {
	adminIDs, err := services.ActiveSystemAdminIDs(h.db)
	if err != nil {
		slog.Warn("scim: failed to load system admins for cascade notification",
			slog.Int("owner_id", ownerID), slog.Any("error", err))
		return
	}
	if len(adminIDs) == 0 {
		return
	}

	var title, message string
	if scimManaged {
		title = fmt.Sprintf("SCIM offboarding cascaded to %d agent user(s)", len(cascade.AgentIDs))
		message = fmt.Sprintf(
			"%s (user %d) was deactivated via SCIM (%s). "+
				"%d owned agent(s) flipped inactive; %d API token(s) revoked. "+
				"Re-point any integrations that depended on these credentials.",
			ownerUsername, ownerID, trigger,
			len(cascade.AgentIDs), len(cascade.RevokedAPITokens))
	} else {
		// Anomaly: a SCIM request deactivated a user the IdP never
		// provisioned. Phrase the alert so this stands out — admins
		// should verify the SCIM client isn't misconfigured or abused.
		title = fmt.Sprintf("SCIM request deactivated non-SCIM user (%d agent cascades)", len(cascade.AgentIDs))
		message = fmt.Sprintf(
			"%s (user %d) is not SCIM-managed, but a SCIM request (%s) deactivated them. "+
				"%d owned agent(s) flipped inactive; %d API token(s) revoked. "+
				"Verify this was intentional and re-point any integrations that depended on these credentials.",
			ownerUsername, ownerID, trigger,
			len(cascade.AgentIDs), len(cascade.RevokedAPITokens))
	}

	meta, _ := json.Marshal(map[string]interface{}{
		"source":                "scim",
		"trigger":               trigger,
		"owner_id":              ownerID,
		"owner_username":        ownerUsername,
		"owner_scim_managed":    scimManaged,
		"deactivated_agent_ids": cascade.AgentIDs,
		"revoked_api_tokens":    len(cascade.RevokedAPITokens),
	})

	for _, aid := range adminIDs {
		if _, err := h.db.Exec(`
			INSERT INTO notifications (user_id, title, message, type, metadata)
			VALUES (?, ?, ?, 'warning', ?)
		`, aid, title, message, string(meta)); err != nil {
			slog.Warn("scim: failed to insert admin notification",
				slog.Int("admin_id", aid), slog.Any("error", err))
		}
	}
}

// logGroupMemberChange writes a single per-member audit entry. The success flag
// and error message reflect the DB write result, so the audit log can be queried
// for failed SCIM member ops (e.g., FK violations on non-existent user_id).
func (h *SCIMHandler) logGroupMemberChange(r *http.Request, actionType string, group *models.TeamGroup, memberID int, execErr error) {
	details := map[string]interface{}{
		"user_id":    memberID,
		"group_id":   group.ID,
		"group_name": group.Name,
	}
	success := execErr == nil
	errMsg := ""
	if execErr != nil {
		errMsg = execErr.Error()
	}
	h.logSCIMAuditEvent(r, actionType, logger.ResourceGroup, &group.ID, group.Name, details, success, errMsg)
}
