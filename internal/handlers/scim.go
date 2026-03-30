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
)

// SCIMHandler handles SCIM 2.0 endpoints
type SCIMHandler struct {
	db      database.Database
	baseURL string
}

// NewSCIMHandler creates a new SCIM handler
func NewSCIMHandler(db database.Database, baseURL string) *SCIMHandler {
	return &SCIMHandler{
		db:      db,
		baseURL: baseURL,
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

// logSCIMAuditEvent logs a SCIM provisioning event to the audit log
//
//nolint:unparam // success is always true currently but kept for future error logging
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

	baseQuery := `SELECT id, email, username, first_name, last_name, is_active,
	              COALESCE(scim_external_id, '') as scim_external_id, created_at, updated_at
	              FROM users WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM users WHERE 1=1`

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

	// Check for existing user
	var existingID int
	err := h.db.QueryRow(`SELECT id FROM users WHERE email = ? OR username = ?`, email, scimUser.UserName).Scan(&existingID)
	if err == nil {
		respondSCIMErrorMsg(w, http.StatusConflict, "User with this email or username already exists", "uniqueness")
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
	`, email, scimUser.UserName, firstName, lastName, scimUser.Active, nullIfEmpty(scimUser.ExternalID)).Scan(&userID)
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

	// Update user
	_, err = h.db.Exec(`
		UPDATE users SET email = ?, username = ?, first_name = ?, last_name = ?,
		                 is_active = ?, scim_external_id = ?, scim_managed = true,
		                 updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, email, scimUser.UserName, firstName, lastName, scimUser.Active, nullIfEmpty(scimUser.ExternalID), id)
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
			"active":       scimUser.Active,
			"old_username": existingUser.Username,
			"old_email":    existingUser.Email,
		}, true, "")

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

	// Verify user exists
	_, err = h.getUserByID(id)
	if err != nil {
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

	// Process operations
	for _, op := range patchReq.Operations {
		if err = h.applyUserPatchOp(id, op); err != nil {
			respondSCIMErrorMsg(w, http.StatusBadRequest, "Failed to apply patch: "+err.Error(), "invalidValue")
			return
		}
	}

	// Fetch updated user
	user, err := h.getUserByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to retrieve updated user", "")
		return
	}

	// Audit log: SCIM user patched
	h.logSCIMAuditEvent(r, logger.ActionSCIMUserUpdate, logger.ResourceUser, &id, user.Email,
		map[string]interface{}{"operation_count": len(patchReq.Operations)}, true, "")

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

	// Deactivate rather than delete
	_, err = h.db.Exec(`UPDATE users SET is_active = false, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to delete user", "")
		return
	}

	// Audit log: SCIM user deactivated
	h.logSCIMAuditEvent(r, logger.ActionSCIMUserDelete, logger.ResourceUser, &id, user.Email,
		map[string]interface{}{"username": user.Username, "email": user.Email}, true, "")

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

	// Add members
	for _, member := range scimGroup.Members {
		var memberID int
		memberID, err = strconv.Atoi(member.Value)
		if err != nil {
			continue
		}
		_, _ = h.db.Exec(`
			INSERT INTO group_members (group_id, user_id, scim_managed, added_at)
			VALUES (?, ?, true, CURRENT_TIMESTAMP)
		`, groupID, memberID)
	}

	// Fetch created group
	group, err := h.getGroupByID(int(groupID))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to retrieve created group", "")
		return
	}

	members, _ := h.getGroupMembers(int(groupID))

	// Audit log: SCIM group created
	groupIDInt := int(groupID)
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

	// Replace members - remove SCIM-managed members and add new ones
	_, _ = h.db.Exec(`DELETE FROM group_members WHERE group_id = ? AND scim_managed = true`, id)
	for _, member := range scimGroup.Members {
		var memberID int
		memberID, err = strconv.Atoi(member.Value)
		if err != nil {
			continue
		}
		_, _ = h.db.Exec(`
			INSERT INTO group_members (group_id, user_id, scim_managed, added_at)
			VALUES (?, ?, true, CURRENT_TIMESTAMP)
			ON CONFLICT(group_id, user_id) DO UPDATE SET scim_managed = true
		`, id, memberID)
	}

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

	_, err = h.getGroupByID(id)
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

	for _, op := range patchReq.Operations {
		if err = h.applyGroupPatchOp(id, op); err != nil {
			respondSCIMErrorMsg(w, http.StatusBadRequest, "Failed to apply patch: "+err.Error(), "invalidValue")
			return
		}
	}

	group, err := h.getGroupByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to retrieve updated group", "")
		return
	}

	members, _ := h.getGroupMembers(id)

	// Audit log: SCIM group patched
	h.logSCIMAuditEvent(r, logger.ActionSCIMGroupUpdate, logger.ResourceGroup, &id, group.Name,
		map[string]interface{}{"operation_count": len(patchReq.Operations)}, true, "")

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
		       scim_external_id, COALESCE(scim_managed, false), created_at, updated_at
		FROM users WHERE id = ?
	`, id).Scan(&user.ID, &user.Email, &user.Username, &user.FirstName, &user.LastName,
		&user.IsActive, &scimExternalID, &user.SCIMManaged, &user.CreatedAt, &user.UpdatedAt)
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
		Active: user.IsActive,
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

func (h *SCIMHandler) applyUserPatchOp(userID int, op models.SCIMPatchOp) error {
	opLower := strings.ToLower(op.Op)

	switch opLower {
	case "replace", "add":
		// Handle path-based updates
		path := strings.ToLower(op.Path)

		switch path {
		case "active":
			active, ok := op.Value.(bool)
			if !ok {
				// Try string conversion
				if strVal, ok := op.Value.(string); ok {
					active = strings.EqualFold(strVal, "true")
				}
			}
			_, err := h.db.Exec(`UPDATE users SET is_active = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, active, userID)
			return err

		case "username", "userName":
			if strVal, ok := op.Value.(string); ok {
				_, err := h.db.Exec(`UPDATE users SET username = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, strVal, userID)
				return err
			}

		case "name.givenname", "name.givenName":
			if strVal, ok := op.Value.(string); ok {
				_, err := h.db.Exec(`UPDATE users SET first_name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, strVal, userID)
				return err
			}

		case "name.familyname", "name.familyName":
			if strVal, ok := op.Value.(string); ok {
				_, err := h.db.Exec(`UPDATE users SET last_name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, strVal, userID)
				return err
			}

		case "externalid", "externalId":
			if strVal, ok := op.Value.(string); ok {
				_, err := h.db.Exec(`UPDATE users SET scim_external_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, strVal, userID)
				return err
			}

		case "":
			// No path - value should be an object with attributes
			if valueMap, ok := op.Value.(map[string]interface{}); ok {
				for key, val := range valueMap {
					subOp := models.SCIMPatchOp{Op: op.Op, Path: key, Value: val}
					if err := h.applyUserPatchOp(userID, subOp); err != nil {
						return err
					}
				}
				return nil
			}
		}

	case "remove":
		// For users, remove typically means setting to null/empty
		path := strings.ToLower(op.Path)
		switch path {
		case "externalid", "externalId":
			_, err := h.db.Exec(`UPDATE users SET scim_external_id = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, userID)
			return err
		}
	}

	return nil
}

func (h *SCIMHandler) applyGroupPatchOp(groupID int, op models.SCIMPatchOp) error {
	opLower := strings.ToLower(op.Op)
	path := strings.ToLower(op.Path)

	switch opLower {
	case "replace", "add":
		switch path {
		case "displayname", "displayName":
			if strVal, ok := op.Value.(string); ok {
				_, err := h.db.Exec(`UPDATE groups SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, strVal, groupID)
				return err
			}

		case "externalid", "externalId":
			if strVal, ok := op.Value.(string); ok {
				_, err := h.db.Exec(`UPDATE groups SET scim_external_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, strVal, groupID)
				return err
			}

		case "members":
			// Add members
			if members, ok := op.Value.([]interface{}); ok {
				for _, m := range members {
					if memberMap, ok := m.(map[string]interface{}); ok {
						if valueStr, ok := memberMap["value"].(string); ok {
							memberID, err := strconv.Atoi(valueStr)
							if err != nil {
								continue
							}
							_, _ = h.db.Exec(`
								INSERT INTO group_members (group_id, user_id, scim_managed, added_at)
								VALUES (?, ?, true, CURRENT_TIMESTAMP)
								ON CONFLICT(group_id, user_id) DO UPDATE SET scim_managed = true
							`, groupID, memberID)
						}
					}
				}
			}
			return nil
		}

	case "remove":
		if path == "members" || strings.HasPrefix(path, "members[") {
			// Parse member to remove
			if op.Value != nil {
				if members, ok := op.Value.([]interface{}); ok {
					for _, m := range members {
						if memberMap, ok := m.(map[string]interface{}); ok {
							if valueStr, ok := memberMap["value"].(string); ok {
								memberID, err := strconv.Atoi(valueStr)
								if err != nil {
									continue
								}
								_, _ = h.db.Exec(`DELETE FROM group_members WHERE group_id = ? AND user_id = ?`, groupID, memberID)
							}
						}
					}
				}
			}
			return nil
		}
	}

	return nil
}
