package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
)

// sanitizeSCIMUser scrubs the IdP-supplied fields on a SCIM user payload.
// UserName / email values / ExternalID are identifier-shaped; the name
// components and DisplayName render in member lists, group rosters, and
// audit log details.
func sanitizeSCIMUser(u *models.SCIMUser) {
	sanitize.ApplyAll(
		sanitize.Pair{Target: &u.UserName, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &u.Name.GivenName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &u.Name.FamilyName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &u.DisplayName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &u.ExternalID, Policy: sanitize.ShortIdentifier},
	)
	for i := range u.Emails {
		sanitize.Apply(&u.Emails[i].Value, sanitize.ShortIdentifier)
	}
}

// sanitizeSCIMGroup scrubs the IdP-supplied fields on a SCIM group payload.
// DisplayName renders in group directories + member popovers; ExternalID is
// the IdP's identifier-shaped correlation key.
func sanitizeSCIMGroup(g *models.SCIMGroup) {
	sanitize.ApplyAll(
		sanitize.Pair{Target: &g.DisplayName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &g.ExternalID, Policy: sanitize.ShortIdentifier},
	)
}

type SCIMHandler struct {
	repo              *repository.SCIMRepository
	baseURL           string
	permissionService *services.PermissionService
	cacheInvalidator  *services.AuthorizationCacheInvalidator
	auditor           *logger.Auditor
	// deactivateCascade and activeSystemAdminIDs are dependency-injected
	// closures over services.DeactivateOwnedAgentsAndTokens and
	// services.ActiveSystemAdminIDs; injecting them at construction lets the
	// handler stay free of the database import (same pattern as UserHandler).
	deactivateCascade    func(ownerID int) (services.AgentDeactivationResult, error)
	activeSystemAdminIDs func() ([]int, error)
	notificationService  *services.NotificationService
}

func NewSCIMHandler(
	repo *repository.SCIMRepository,
	baseURL string,
	permissionService *services.PermissionService,
	auditor *logger.Auditor,
	deactivateCascade func(ownerID int) (services.AgentDeactivationResult, error),
	activeSystemAdminIDs func() ([]int, error),
	notificationService *services.NotificationService,
	invalidators ...*services.AuthorizationCacheInvalidator,
) *SCIMHandler {
	cacheInvalidator := services.NewAuthorizationCacheInvalidator(permissionService, nil)
	if len(invalidators) > 0 && invalidators[0] != nil {
		cacheInvalidator = invalidators[0]
	}
	return &SCIMHandler{
		repo:                 repo,
		baseURL:              baseURL,
		permissionService:    permissionService,
		cacheInvalidator:     cacheInvalidator,
		auditor:              auditor,
		deactivateCascade:    deactivateCascade,
		activeSystemAdminIDs: activeSystemAdminIDs,
		notificationService:  notificationService,
	}
}

// SCIM request limits.

const scimMaxBodySize = 1 * 1024 * 1024

// Response helpers.

// limitRequestBody bounds request bodies and reports whether they fit.
func (h *SCIMHandler) limitRequestBody(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, scimMaxBodySize)
}

func (h *SCIMHandler) logSCIMAuditEvent(r *http.Request, actionType, resourceType string, resourceID *int, resourceName string, details map[string]any, success bool, errorMsg string) {
	scimToken := middleware.GetSCIMToken(r)
	tokenPrefix := ""
	if scimToken != nil {
		tokenPrefix = scimToken.TokenPrefix
	}

	if details == nil {
		details = make(map[string]any)
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

	// Audit logging is asynchronous so provisioning is not delayed.
	go h.auditor.LogEvent(event)
}

// attrChange records one PATCH mutation for the audit log.
type attrChange struct {
	Op       string `json:"op"`
	Path     string `json:"path"`
	OldValue any    `json:"old_value,omitempty"`
	NewValue any    `json:"new_value,omitempty"`
}

// logPatchOpError records driver details server-side while keeping the client
// response generic.
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

func respondSCIMJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

// isInvalidFilterErr centralizes SCIM filter parse classification.
func isInvalidFilterErr(err error) bool {
	return errors.Is(err, errInvalidSCIMFilter)
}

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

// Service-provider endpoints.

func (h *SCIMHandler) GetServiceProviderConfig(w http.ResponseWriter, r *http.Request) {
	config := GetServiceProviderConfig(h.baseURL)
	respondSCIMJSON(w, http.StatusOK, config)
}

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
		Resources:    make([]any, len(resourceTypes)),
	}
	for i, rt := range resourceTypes {
		response.Resources[i] = rt
	}

	respondSCIMJSON(w, http.StatusOK, response)
}

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
		Resources:    make([]any, len(schemas)),
	}
	for i, s := range schemas {
		response.Resources[i] = s
	}

	respondSCIMJSON(w, http.StatusOK, response)
}

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

// User endpoints.

func (h *SCIMHandler) listUsersFiltered(filter string, startIndex, count int) (*models.SCIMListResponse, error) {
	filterResult, err := ParseSCIMFilterWithAnd(filter, "User")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errInvalidSCIMFilter, err)
	}

	// The repository scopes the query to the IdP-provisioned surface (no
	// agents, no locally-managed humans) — see SCIMRepository.ListUsersFiltered.
	offset := startIndex - 1
	users, totalResults, err := h.repo.ListUsersFiltered(filterResult.WhereClause, filterResult.Args, count, offset)
	if err != nil {
		return nil, err
	}

	resources := make([]any, 0)
	for i := range users {
		resources = append(resources, h.userToSCIM(&users[i]))
	}

	return &models.SCIMListResponse{
		Schemas:      []string{models.SCIMSchemaListResponse},
		TotalResults: totalResults,
		StartIndex:   startIndex,
		ItemsPerPage: len(resources),
		Resources:    resources,
	}, nil
}

func (h *SCIMHandler) listGroupsFiltered(filter string, startIndex, count int) (*models.SCIMListResponse, error) {
	filterResult, err := ParseSCIMFilterWithAnd(filter, "Group")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errInvalidSCIMFilter, err)
	}

	// The repository scopes the query so SCIM only sees what the IdP
	// provisioned — see SCIMRepository.ListGroupsFiltered.
	offset := startIndex - 1
	groups, totalResults, err := h.repo.ListGroupsFiltered(filterResult.WhereClause, filterResult.Args, count, offset)
	if err != nil {
		return nil, err
	}

	resources := make([]any, 0)
	for i := range groups {
		group := &groups[i]
		members, mErr := h.getGroupMembers(group.ID)
		if mErr != nil {
			return nil, fmt.Errorf("load members for group %d: %w", group.ID, mErr)
		}
		resources = append(resources, h.groupToSCIM(group, members))
	}

	return &models.SCIMListResponse{
		Schemas:      []string{models.SCIMSchemaListResponse},
		TotalResults: totalResults,
		StartIndex:   startIndex,
		ItemsPerPage: len(resources),
		Resources:    resources,
	}, nil
}

func (h *SCIMHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	filter, startIndex, count := scimListPagingFromQuery(r)

	response, err := h.listUsersFiltered(filter, startIndex, count)
	if err != nil {
		respondSCIMListError(w, err)
		return
	}

	respondSCIMJSON(w, http.StatusOK, response)
}

func (h *SCIMHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	h.limitRequestBody(w, r)

	var scimUser models.SCIMUser
	if err := newJSONDecoder(w, r).Decode(&scimUser); err != nil {
		if err.Error() == "http: request body too large" {
			respondSCIMErrorMsg(w, http.StatusRequestEntityTooLarge, "Request body too large", "tooLarge")
			return
		}
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid request body", "invalidValue")
		return
	}
	sanitizeSCIMUser(&scimUser)

	if scimUser.UserName == "" {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "userName is required", "invalidValue")
		return
	}

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

	isActive := true
	if scimUser.Active != nil {
		isActive = *scimUser.Active
	}

	existingUser, err := h.repo.GetUserByEmail(email)
	if err == nil {
		// A tombstoned account stays deleted: re-provisioning the same email
		// must conflict rather than resurrect it (RFC 7644 §3.6). Un-deleting
		// is an operator decision, not an IdP-driven one.
		if h.repo.IsUserSCIMDeleted(existingUser.ID) {
			h.logSCIMAuditEvent(r, logger.ActionSCIMUserCreate, logger.ResourceUser, &existingUser.ID, email,
				map[string]any{
					"username": existingUser.Username,
					"reason":   "email_matches_tombstoned_user",
				}, false, "refused: email belongs to a SCIM-deleted user")
			respondSCIMErrorMsg(w, http.StatusConflict, "User with this email already exists", "uniqueness")
			return
		}

		username := scimUser.UserName
		if username == "" {
			username = existingUser.Username
		}
		err = h.repo.AdoptUser(existingUser.ID, username, scimUser.ExternalID, isActive)
		if err != nil {
			slog.Error("SCIM: failed to adopt existing user", slog.Any("error", err), slog.String("email", email))
			respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to adopt existing user", "")
			return
		}

		user, err := h.repo.GetUserByID(existingUser.ID)
		if err != nil {
			respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to retrieve adopted user", "")
			return
		}

		h.logSCIMAuditEvent(r, logger.ActionSCIMUserCreate, logger.ResourceUser, &existingUser.ID, email,
			map[string]any{
				"username":     username,
				"email":        email,
				"adopted":      true,
				"old_username": existingUser.Username,
			}, true, "")

		respondSCIMJSON(w, http.StatusOK, h.userToSCIM(user))
		return
	}

	if h.repo.UsernameExists(scimUser.UserName) {
		respondSCIMErrorMsg(w, http.StatusConflict, "User with this username already exists", "uniqueness")
		return
	}

	firstName := scimUser.Name.GivenName
	lastName := scimUser.Name.FamilyName
	if firstName == "" && lastName == "" && scimUser.DisplayName != "" {
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

	userID, err := h.repo.CreateUser(email, scimUser.UserName, firstName, lastName, isActive, scimUser.ExternalID)
	if err != nil {
		slog.Error("SCIM: failed to create user", slog.Any("error", err), slog.String("email", email))
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to create user", "")
		return
	}

	user, err := h.repo.GetUserByID(userID)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to retrieve created user", "")
		return
	}

	h.logSCIMAuditEvent(r, logger.ActionSCIMUserCreate, logger.ResourceUser, &userID, email,
		map[string]any{"username": scimUser.UserName, "email": email}, true, "")

	respondSCIMJSON(w, http.StatusCreated, h.userToSCIM(user))
}

func (h *SCIMHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid user ID", "invalidValue")
		return
	}

	user, err := h.repo.GetUserByID(id)
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

func (h *SCIMHandler) ReplaceUser(w http.ResponseWriter, r *http.Request) {
	h.limitRequestBody(w, r)

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid user ID", "invalidValue")
		return
	}

	existingUser, err := h.repo.GetUserByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusNotFound, "User not found", "")
		return
	}

	// SCIM PUT must not reach past IdP-provisioned users. Local users get
	// adopted into SCIM through POST's collision-by-email path, never via
	// PUT. See DeleteUser for the full rationale.
	if !existingUser.SCIMManaged {
		h.logSCIMAuditEvent(r, logger.ActionSCIMUserUpdate, logger.ResourceUser, &id, existingUser.Email,
			map[string]any{
				"username": existingUser.Username,
				"reason":   "target_not_scim_managed",
			}, false, "refused: user is not SCIM-managed")
		respondSCIMErrorMsg(w, http.StatusNotFound, "User not found", "")
		return
	}

	var scimUser models.SCIMUser
	if err = newJSONDecoder(w, r).Decode(&scimUser); err != nil {
		if err.Error() == "http: request body too large" {
			respondSCIMErrorMsg(w, http.StatusRequestEntityTooLarge, "Request body too large", "tooLarge")
			return
		}
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid request body", "invalidValue")
		return
	}
	sanitizeSCIMUser(&scimUser)

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

	firstName := scimUser.Name.GivenName
	lastName := scimUser.Name.FamilyName
	if firstName == "" {
		firstName = existingUser.FirstName
	}
	if lastName == "" {
		lastName = existingUser.LastName
	}

	isActive := existingUser.IsActive
	if scimUser.Active != nil {
		isActive = *scimUser.Active
	}

	err = h.repo.ReplaceUser(id, email, scimUser.UserName, firstName, lastName, isActive, scimUser.ExternalID)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to update user", "")
		return
	}

	user, err := h.repo.GetUserByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to retrieve updated user", "")
		return
	}

	h.logSCIMAuditEvent(r, logger.ActionSCIMUserUpdate, logger.ResourceUser, &id, email,
		map[string]any{
			"username":     scimUser.UserName,
			"email":        email,
			"active":       isActive,
			"old_username": existingUser.Username,
			"old_email":    existingUser.Email,
		}, true, "")

	if existingUser.IsActive && !isActive {
		h.handleSCIMUserDeactivation(r, id, existingUser.Username, "scim_replace", existingUser.SCIMManaged)
	}

	respondSCIMJSON(w, http.StatusOK, h.userToSCIM(user))
}

func (h *SCIMHandler) PatchUser(w http.ResponseWriter, r *http.Request) {
	h.limitRequestBody(w, r)

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid user ID", "invalidValue")
		return
	}

	snapshot, err := h.repo.GetUserByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusNotFound, "User not found", "")
		return
	}

	// SCIM PATCH must not reach past IdP-provisioned users — see DeleteUser
	// for the full rationale.
	if !snapshot.SCIMManaged {
		h.logSCIMAuditEvent(r, logger.ActionSCIMUserUpdate, logger.ResourceUser, &id, snapshot.Email,
			map[string]any{
				"username": snapshot.Username,
				"reason":   "target_not_scim_managed",
			}, false, "refused: user is not SCIM-managed")
		respondSCIMErrorMsg(w, http.StatusNotFound, "User not found", "")
		return
	}

	var patchReq models.SCIMPatchRequest
	if err = newJSONDecoder(w, r).Decode(&patchReq); err != nil {
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

	user, err := h.repo.GetUserByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to retrieve updated user", "")
		return
	}

	h.logSCIMAuditEvent(r, logger.ActionSCIMUserUpdate, logger.ResourceUser, &id, user.Email,
		map[string]any{
			"operation_count": len(patchReq.Operations),
			"changes":         changes,
		}, true, "")

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

func (h *SCIMHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid user ID", "invalidValue")
		return
	}

	user, err := h.repo.GetUserByID(id)
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
			map[string]any{
				"username": user.Username,
				"reason":   "target_not_scim_managed",
			}, false, "refused: user is not SCIM-managed")
		respondSCIMErrorMsg(w, http.StatusNotFound, "User not found", "")
		return
	}

	err = h.repo.TombstoneUser(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to delete user", "")
		return
	}

	h.logSCIMAuditEvent(r, logger.ActionSCIMUserDelete, logger.ResourceUser, &id, user.Email,
		map[string]any{"username": user.Username, "email": user.Email}, true, "")

	h.handleSCIMUserDeactivation(r, id, user.Username, "scim_delete", user.SCIMManaged)

	w.WriteHeader(http.StatusNoContent)
}

// Group endpoints.

func (h *SCIMHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	filter, startIndex, count := scimListPagingFromQuery(r)

	response, err := h.listGroupsFiltered(filter, startIndex, count)
	if err != nil {
		respondSCIMListError(w, err)
		return
	}

	respondSCIMJSON(w, http.StatusOK, response)
}

func (h *SCIMHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	h.limitRequestBody(w, r)

	var scimGroup models.SCIMGroup
	if err := newJSONDecoder(w, r).Decode(&scimGroup); err != nil {
		if err.Error() == "http: request body too large" {
			respondSCIMErrorMsg(w, http.StatusRequestEntityTooLarge, "Request body too large", "tooLarge")
			return
		}
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid request body", "invalidValue")
		return
	}
	sanitizeSCIMGroup(&scimGroup)

	if scimGroup.DisplayName == "" {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "displayName is required", "invalidValue")
		return
	}

	if h.repo.GroupNameExists(scimGroup.DisplayName) {
		respondSCIMErrorMsg(w, http.StatusConflict, "Group with this name already exists", "uniqueness")
		return
	}

	groupIDInt, err := h.repo.CreateGroup(scimGroup.DisplayName, scimGroup.ExternalID)
	if err != nil {
		slog.Error("SCIM: failed to create group", slog.Any("error", err), slog.String("name", scimGroup.DisplayName))
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to create group", "")
		return
	}

	groupRef := &models.TeamGroup{ID: groupIDInt, Name: scimGroup.DisplayName}

	addedMemberIDs := []int{}
	// Audit each member insert individually so partial failures remain visible.
	for _, member := range scimGroup.Members {
		memberID, convErr := strconv.Atoi(member.Value)
		if convErr != nil {
			continue
		}
		// Reject members that aren't SCIM-visible. Without this check, a SCIM
		// client could attach local/admin/service users by guessing their IDs.
		if !h.repo.IsUserSCIMVisible(memberID) {
			h.logGroupMemberChange(r, logger.ActionSCIMGroupAddMember, groupRef, memberID,
				fmt.Errorf("user not SCIM-managed"))
			continue
		}
		execErr := h.repo.AddGroupMember(groupIDInt, memberID)
		h.logGroupMemberChange(r, logger.ActionSCIMGroupAddMember, groupRef, memberID, execErr)
		if execErr == nil {
			addedMemberIDs = append(addedMemberIDs, memberID)
		}
	}

	if err := h.cacheInvalidator.Apply(services.AuthorizationInvalidation{UserIDs: addedMemberIDs}); err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to invalidate authorization cache", "")
		return
	}

	group, err := h.repo.GetGroupByID(groupIDInt)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to retrieve created group", "")
		return
	}

	members, _ := h.getGroupMembers(groupIDInt)

	h.logSCIMAuditEvent(r, logger.ActionSCIMGroupCreate, logger.ResourceGroup, &groupIDInt, scimGroup.DisplayName,
		map[string]any{"member_count": len(scimGroup.Members)}, true, "")

	respondSCIMJSON(w, http.StatusCreated, h.groupToSCIM(group, members))
}

func (h *SCIMHandler) GetGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid group ID", "invalidValue")
		return
	}

	group, err := h.repo.GetGroupByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusNotFound, "Group not found", "")
		return
	}

	// Mirror the list-query scope: SCIM must not acknowledge locally-managed
	// groups. 404 (not 403) keeps row existence opaque.
	if !group.SCIMManaged {
		respondSCIMErrorMsg(w, http.StatusNotFound, "Group not found", "")
		return
	}

	members, _ := h.getGroupMembers(id)
	respondSCIMJSON(w, http.StatusOK, h.groupToSCIM(group, members))
}

func (h *SCIMHandler) ReplaceGroup(w http.ResponseWriter, r *http.Request) {
	h.limitRequestBody(w, r)

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid group ID", "invalidValue")
		return
	}

	existingGroup, err := h.repo.GetGroupByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusNotFound, "Group not found", "")
		return
	}

	// SCIM PUT must not reach past IdP-provisioned groups. A SCIM token must
	// not be able to rename or take over a locally-managed group by guessing
	// its ID. See ReplaceUser for the user-side equivalent of this guard.
	if !existingGroup.SCIMManaged {
		h.logSCIMAuditEvent(r, logger.ActionSCIMGroupUpdate, logger.ResourceGroup, &id, existingGroup.Name,
			map[string]any{
				"reason": "target_not_scim_managed",
			}, false, "refused: group is not SCIM-managed")
		respondSCIMErrorMsg(w, http.StatusNotFound, "Group not found", "")
		return
	}

	var scimGroup models.SCIMGroup
	if err = newJSONDecoder(w, r).Decode(&scimGroup); err != nil {
		if err.Error() == "http: request body too large" {
			respondSCIMErrorMsg(w, http.StatusRequestEntityTooLarge, "Request body too large", "tooLarge")
			return
		}
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid request body", "invalidValue")
		return
	}
	sanitizeSCIMGroup(&scimGroup)

	groupRef := &models.TeamGroup{ID: id, Name: scimGroup.DisplayName}

	// Validate and dedupe the replacement member set up front, before we
	// touch the database. Building the accepted list (and the per-member
	// audit-skip list) outside the transaction keeps the tx narrow and lets
	// us collect non-fatal "user not SCIM-visible" rejections without
	// forcing a rollback for IdP-side data hygiene issues.
	type memberSkip struct {
		id  int
		err error
	}
	var (
		acceptedMembers []int
		skippedMembers  []memberSkip
		seen            = make(map[int]bool)
	)
	for _, member := range scimGroup.Members {
		memberID, convErr := strconv.Atoi(member.Value)
		if convErr != nil {
			continue
		}
		if seen[memberID] {
			continue
		}
		seen[memberID] = true
		// Reject members that aren't SCIM-visible. Without this guard, the
		// ON CONFLICT clause would flip a local membership into scim_managed
		// state, effectively letting a SCIM token adopt local users.
		if !h.repo.IsUserSCIMVisible(memberID) {
			skippedMembers = append(skippedMembers, memberSkip{id: memberID, err: fmt.Errorf("user not SCIM-managed")})
			continue
		}
		acceptedMembers = append(acceptedMembers, memberID)
	}

	// The rename + member-set rewrite runs in a single transaction inside the
	// repository so a failure mid-flight cannot leave the group renamed-but-
	// empty (or with only some of the new members applied). All cache
	// invalidation and audit logging happens after a successful Commit; on
	// rollback the caller observes no externally visible change.
	priorMemberIDs, err := h.repo.ReplaceGroup(r.Context(), id, scimGroup.DisplayName, scimGroup.ExternalID, acceptedMembers)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to update group", "")
		return
	}

	// Past this point the write has committed; cache invalidation and audit
	// logging follow once, with the final state visible to other readers.
	if err := h.cacheInvalidator.Apply(services.AuthorizationInvalidation{
		UserIDs: append(priorMemberIDs, acceptedMembers...),
	}); err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to invalidate authorization cache", "")
		return
	}

	for _, uid := range priorMemberIDs {
		h.logGroupMemberChange(r, logger.ActionSCIMGroupRemoveMember, groupRef, uid, nil)
	}
	for _, memberID := range acceptedMembers {
		h.logGroupMemberChange(r, logger.ActionSCIMGroupAddMember, groupRef, memberID, nil)
	}
	for _, skip := range skippedMembers {
		h.logGroupMemberChange(r, logger.ActionSCIMGroupAddMember, groupRef, skip.id, skip.err)
	}

	group, err := h.repo.GetGroupByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to retrieve updated group", "")
		return
	}

	members, _ := h.getGroupMembers(id)

	// Audit log: SCIM group updated (full replace)
	h.logSCIMAuditEvent(r, logger.ActionSCIMGroupUpdate, logger.ResourceGroup, &id, scimGroup.DisplayName,
		map[string]any{
			"old_name":     existingGroup.Name,
			"new_name":     scimGroup.DisplayName,
			"member_count": len(scimGroup.Members),
		}, true, "")

	respondSCIMJSON(w, http.StatusOK, h.groupToSCIM(group, members))
}

func (h *SCIMHandler) PatchGroup(w http.ResponseWriter, r *http.Request) {
	// Security: Limit request body size to prevent memory exhaustion
	h.limitRequestBody(w, r)

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid group ID", "invalidValue")
		return
	}

	snapshot, err := h.repo.GetGroupByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusNotFound, "Group not found", "")
		return
	}

	// SCIM PATCH must not reach past IdP-provisioned groups — see ReplaceGroup
	// for the rationale.
	if !snapshot.SCIMManaged {
		h.logSCIMAuditEvent(r, logger.ActionSCIMGroupUpdate, logger.ResourceGroup, &id, snapshot.Name,
			map[string]any{
				"reason": "target_not_scim_managed",
			}, false, "refused: group is not SCIM-managed")
		respondSCIMErrorMsg(w, http.StatusNotFound, "Group not found", "")
		return
	}

	var patchReq models.SCIMPatchRequest
	if err = newJSONDecoder(w, r).Decode(&patchReq); err != nil {
		if err.Error() == "http: request body too large" {
			respondSCIMErrorMsg(w, http.StatusRequestEntityTooLarge, "Request body too large", "tooLarge")
			return
		}
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid request body", "invalidValue")
		return
	}

	hasMemberOps := false
	for _, op := range patchReq.Operations {
		if strings.EqualFold(op.Path, "members") || strings.HasPrefix(strings.ToLower(op.Path), "members[") {
			hasMemberOps = true
			break
		}
	}
	var invalidation services.AuthorizationInvalidation
	if hasMemberOps {
		invalidation, err = h.cacheInvalidator.GroupPlan(id)
		if err != nil {
			respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to capture authorization invalidation", "")
			return
		}
	}
	var changes []attrChange
	for _, op := range patchReq.Operations {
		opChanges, opErr := h.applyGroupPatchOp(r, snapshot, op)
		if opErr != nil {
			h.logPatchOpError(r, "group", id, op, opErr)
			if hasMemberOps {
				if err := h.applyGroupInvalidationAfterMutation(id, invalidation); err != nil {
					respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to invalidate authorization cache", "")
					return
				}
			}
			respondSCIMErrorMsg(w, http.StatusBadRequest, "Patch operation failed", "invalidValue")
			return
		}
		changes = append(changes, opChanges...)
	}

	if hasMemberOps {
		if err := h.applyGroupInvalidationAfterMutation(id, invalidation); err != nil {
			respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to invalidate authorization cache", "")
			return
		}
	}

	group, err := h.repo.GetGroupByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to retrieve updated group", "")
		return
	}

	members, _ := h.getGroupMembers(id)

	h.logSCIMAuditEvent(r, logger.ActionSCIMGroupUpdate, logger.ResourceGroup, &id, group.Name,
		map[string]any{
			"operation_count": len(patchReq.Operations),
			"changes":         changes,
		}, true, "")

	respondSCIMJSON(w, http.StatusOK, h.groupToSCIM(group, members))
}

func (h *SCIMHandler) applyGroupInvalidationAfterMutation(id int, invalidation services.AuthorizationInvalidation) error {
	current, err := h.cacheInvalidator.GroupPlan(id)
	if err != nil {
		invalidation.ResetPermissions = true
	} else {
		invalidation.UserIDs = append(invalidation.UserIDs, current.UserIDs...)
	}
	return h.cacheInvalidator.Apply(invalidation)
}

func (h *SCIMHandler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid group ID", "invalidValue")
		return
	}

	group, err := h.repo.GetGroupByID(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusNotFound, "Group not found", "")
		return
	}

	// SCIM must only delete groups it provisioned. Honoring a DELETE for a
	// locally-managed group would let a SCIM token wipe organization data
	// the IdP never owned. See DeleteUser for the user-side equivalent.
	if !group.SCIMManaged {
		h.logSCIMAuditEvent(r, logger.ActionSCIMGroupDelete, logger.ResourceGroup, &id, group.Name,
			map[string]any{
				"reason": "target_not_scim_managed",
			}, false, "refused: group is not SCIM-managed")
		respondSCIMErrorMsg(w, http.StatusNotFound, "Group not found", "")
		return
	}

	invalidation, err := h.cacheInvalidator.GroupDeletePlan(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to capture authorization invalidation", "")
		return
	}

	err = h.repo.DeleteGroup(id)
	if err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to delete group", "")
		return
	}
	if err := h.cacheInvalidator.Apply(invalidation); err != nil {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to invalidate authorization cache", "")
		return
	}

	h.logSCIMAuditEvent(r, logger.ActionSCIMGroupDelete, logger.ResourceGroup, &id, group.Name,
		nil, true, "")

	w.WriteHeader(http.StatusNoContent)
}

// Me endpoint.

// scimListPagingFromQuery reads the shared SCIM list query parameters
// (filter, startIndex, count) with the server-side defaults and caps.
func scimListPagingFromQuery(r *http.Request) (filter string, startIndex, count int) {
	filter = r.URL.Query().Get("filter")
	startIndex = 1
	if val, err := strconv.Atoi(r.URL.Query().Get("startIndex")); err == nil && val > 0 {
		startIndex = val
	}
	count = 100
	if val, err := strconv.Atoi(r.URL.Query().Get("count")); err == nil && val > 0 && val <= 200 {
		count = val
	}
	return filter, startIndex, count
}

// respondSCIMListError maps a filtered-list failure to a SCIM error,
// preserving the invalidFilter classification for syntax errors.
func respondSCIMListError(w http.ResponseWriter, err error) {
	if isInvalidFilterErr(err) {
		respondSCIMErrorMsg(w, http.StatusBadRequest, err.Error(), "invalidFilter")
	} else {
		respondSCIMErrorMsg(w, http.StatusInternalServerError, err.Error(), "")
	}
}

func (h *SCIMHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	respondSCIMErrorMsg(w, http.StatusNotImplemented, "The /Me endpoint is not implemented", "")
}

// Search endpoints.

func (h *SCIMHandler) SearchRequest(w http.ResponseWriter, r *http.Request) {
	searchReq, ok := h.decodeSCIMSearchBody(w, r)
	if !ok {
		return
	}
	startIndex, count := normalizeSCIMPaging(searchReq.StartIndex, searchReq.Count)

	resourceType, remainingFilter := ExtractResourceTypeFilter(searchReq.Filter)

	switch resourceType {
	case "User":
		response, err := h.listUsersFiltered(remainingFilter, startIndex, count)
		if err != nil {
			respondSCIMListError(w, err)
			return
		}
		respondSCIMJSON(w, http.StatusOK, response)

	case "Group":
		response, err := h.listGroupsFiltered(remainingFilter, startIndex, count)
		if err != nil {
			respondSCIMListError(w, err)
			return
		}
		respondSCIMJSON(w, http.StatusOK, response)

	default:
		// No resource type specified — search both and combine.
		userResp, userErr := h.listUsersFiltered(remainingFilter, startIndex, count)
		groupResp, groupErr := h.listGroupsFiltered(remainingFilter, startIndex, count)

		combined := models.SCIMListResponse{
			Schemas:      []string{models.SCIMSchemaListResponse},
			TotalResults: 0,
			StartIndex:   startIndex,
			Resources:    make([]any, 0),
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
			// Preserve invalidFilter classification when both parsers reject
			// the filter for syntax reasons. Returning a generic 500 here
			// made IdP debugging hard: a client with a bad filter looked
			// like a server outage, while resource-specific endpoints gave
			// a clean 400 invalidFilter for the same input.
			if isInvalidFilterErr(userErr) && isInvalidFilterErr(groupErr) {
				respondSCIMErrorMsg(w, http.StatusBadRequest, userErr.Error(), "invalidFilter")
				return
			}
			respondSCIMErrorMsg(w, http.StatusInternalServerError, "Failed to search resources", "")
			return
		}

		respondSCIMJSON(w, http.StatusOK, combined)
	}
}

func (h *SCIMHandler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	searchReq, ok := h.decodeSCIMSearchBody(w, r)
	if !ok {
		return
	}
	startIndex, count := normalizeSCIMPaging(searchReq.StartIndex, searchReq.Count)

	response, err := h.listUsersFiltered(searchReq.Filter, startIndex, count)
	if err != nil {
		respondSCIMListError(w, err)
		return
	}
	respondSCIMJSON(w, http.StatusOK, response)
}

func (h *SCIMHandler) SearchGroups(w http.ResponseWriter, r *http.Request) {
	searchReq, ok := h.decodeSCIMSearchBody(w, r)
	if !ok {
		return
	}
	startIndex, count := normalizeSCIMPaging(searchReq.StartIndex, searchReq.Count)

	response, err := h.listGroupsFiltered(searchReq.Filter, startIndex, count)
	if err != nil {
		respondSCIMListError(w, err)
		return
	}
	respondSCIMJSON(w, http.StatusOK, response)
}

// decodeSCIMSearchBody parses the POST search body, enforcing the request
// size limit and SCIM error shapes.
func (h *SCIMHandler) decodeSCIMSearchBody(w http.ResponseWriter, r *http.Request) (models.SCIMSearchRequest, bool) {
	h.limitRequestBody(w, r)
	var searchReq models.SCIMSearchRequest
	if err := newJSONDecoder(w, r).Decode(&searchReq); err != nil {
		if err.Error() == "http: request body too large" {
			respondSCIMErrorMsg(w, http.StatusRequestEntityTooLarge, "Request body too large", "tooLarge")
		} else {
			respondSCIMErrorMsg(w, http.StatusBadRequest, "Invalid request body", "invalidValue")
		}
		return models.SCIMSearchRequest{}, false
	}
	return searchReq, true
}

// normalizeSCIMPaging clamps SCIM paging to the server defaults and caps.
func normalizeSCIMPaging(startIndex, count int) (normalizedStartIndex, normalizedCount int) {
	if startIndex < 1 {
		startIndex = 1
	}
	if count <= 0 {
		count = 100
	}
	if count > 200 {
		count = 200
	}
	return startIndex, count
}

// Bulk endpoint.

const scimBulkMaxOperations = 100

func (h *SCIMHandler) BulkRequest(w http.ResponseWriter, r *http.Request) {
	h.limitRequestBody(w, r)

	var bulkReq models.SCIMBulkRequest
	if err := newJSONDecoder(w, r).Decode(&bulkReq); err != nil {
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

func (h *SCIMHandler) executeBulkOperation(originalReq *http.Request, op models.SCIMBulkOperation) models.SCIMBulkResponseOperation {
	method := strings.ToUpper(op.Method)

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

	recorder := httptest.NewRecorder()
	handler(recorder, subReq)

	result := models.SCIMBulkResponseOperation{
		Method: method,
		BulkID: op.BulkID,
		Status: strconv.Itoa(recorder.Code),
	}

	if recorder.Header().Get("Location") != "" {
		result.Location = recorder.Header().Get("Location")
	}

	if recorder.Body.Len() > 0 {
		var respBody any
		if err := json.Unmarshal(recorder.Body.Bytes(), &respBody); err == nil {
			if recorder.Code >= 400 {
				result.Response = respBody
			} else if method == "POST" || method == "PUT" || method == "GET" || method == "PATCH" {
				if respMap, ok := respBody.(map[string]any); ok {
					if meta, ok := respMap["meta"].(map[string]any); ok {
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

func (h *SCIMHandler) routeBulkOperation(method, path string) http.HandlerFunc {
	path = strings.TrimPrefix(path, "/scim/v2")

	parts := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 2)
	if len(parts) == 0 {
		return nil
	}

	resource := parts[0]
	hasID := len(parts) > 1 && parts[1] != ""

	if hasID {
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

// Helper methods.

func (h *SCIMHandler) getGroupMembers(groupID int) ([]models.SCIMGroupMember, error) {
	// Only SCIM-managed memberships come back from the repository. A
	// locally-added member of an otherwise SCIM-managed group must stay
	// invisible to the IdP; otherwise it'll record the ID in its shadow and
	// try to remove it on the next sync.
	rows, err := h.repo.GetGroupMembers(groupID)
	if err != nil {
		return nil, err
	}

	var members []models.SCIMGroupMember
	for _, row := range rows {
		displayName := strings.TrimSpace(row.FirstName + " " + row.LastName)
		if displayName == "" {
			displayName = row.Username
		}
		members = append(members, models.SCIMGroupMember{
			Value:   strconv.Itoa(row.UserID),
			Ref:     h.baseURL + "/scim/v2/Users/" + strconv.Itoa(row.UserID),
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

// applyUserPatchOp mutates the snapshot for subsequent operations and returns audit changes.
// Unsupported attributes succeed as audited no-ops rather than failing the complete PATCH.
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
			err := h.repo.SetUserActive(userID, active)
			if err != nil {
				return nil, err
			}
			change := attrChange{Op: opLower, Path: "active", OldValue: snapshot.IsActive, NewValue: active}
			snapshot.IsActive = active
			return []attrChange{change}, nil

		case "username":
			if strVal, ok := op.Value.(string); ok {
				sanitize.Apply(&strVal, sanitize.ShortIdentifier)
				err := h.repo.SetUserUsername(userID, strVal)
				if err != nil {
					return nil, err
				}
				change := attrChange{Op: opLower, Path: "userName", OldValue: snapshot.Username, NewValue: strVal}
				snapshot.Username = strVal
				return []attrChange{change}, nil
			}

		case "name.givenname":
			if strVal, ok := op.Value.(string); ok {
				sanitize.Apply(&strVal, sanitize.PlainTextField)
				err := h.repo.SetUserFirstName(userID, strVal)
				if err != nil {
					return nil, err
				}
				change := attrChange{Op: opLower, Path: "name.givenName", OldValue: snapshot.FirstName, NewValue: strVal}
				snapshot.FirstName = strVal
				return []attrChange{change}, nil
			}

		case "name.familyname":
			if strVal, ok := op.Value.(string); ok {
				sanitize.Apply(&strVal, sanitize.PlainTextField)
				err := h.repo.SetUserLastName(userID, strVal)
				if err != nil {
					return nil, err
				}
				change := attrChange{Op: opLower, Path: "name.familyName", OldValue: snapshot.LastName, NewValue: strVal}
				snapshot.LastName = strVal
				return []attrChange{change}, nil
			}

		case "externalid":
			if strVal, ok := op.Value.(string); ok {
				sanitize.Apply(&strVal, sanitize.ShortIdentifier)
				err := h.repo.SetUserExternalID(userID, strVal)
				if err != nil {
					return nil, err
				}
				change := attrChange{Op: opLower, Path: "externalId", OldValue: snapshot.SCIMExternalID, NewValue: strVal}
				snapshot.SCIMExternalID = strVal
				return []attrChange{change}, nil
			}

		case "":
			// No path - value should be an object with attributes
			if valueMap, ok := op.Value.(map[string]any); ok {
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
			err := h.repo.ClearUserExternalID(userID)
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
				sanitize.Apply(&strVal, sanitize.PlainTextField)
				err := h.repo.UpdateGroupName(groupID, strVal)
				if err != nil {
					return nil, err
				}
				change := attrChange{Op: opLower, Path: "displayName", OldValue: snapshot.Name, NewValue: strVal}
				snapshot.Name = strVal
				return []attrChange{change}, nil
			}

		case "externalid":
			if strVal, ok := op.Value.(string); ok {
				sanitize.Apply(&strVal, sanitize.ShortIdentifier)
				err := h.repo.UpdateGroupExternalID(groupID, strVal)
				if err != nil {
					return nil, err
				}
				change := attrChange{Op: opLower, Path: "externalId", OldValue: snapshot.SCIMExternalID, NewValue: strVal}
				snapshot.SCIMExternalID = strVal
				return []attrChange{change}, nil
			}

		case "members":
			if members, ok := op.Value.([]any); ok {
				for _, m := range members {
					memberMap, ok := m.(map[string]any)
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
					// Reject members that aren't SCIM-visible — same rationale
					// as the CreateGroup/ReplaceGroup member loops.
					if !h.repo.IsUserSCIMVisible(memberID) {
						h.logGroupMemberChange(r, logger.ActionSCIMGroupAddMember, snapshot, memberID,
							fmt.Errorf("user not SCIM-managed"))
						continue
					}
					execErr := h.repo.UpsertGroupMember(groupID, memberID)
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
			members, ok := op.Value.([]any)
			if !ok {
				return nil, nil
			}
			for _, m := range members {
				memberMap, ok := m.(map[string]any)
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
				// The repository scopes the delete to SCIM-managed memberships
				// so a SCIM PATCH can't wipe a locally-added row. Matches the
				// bulk DELETE in ReplaceGroup.
				execErr := h.repo.RemoveGroupMember(groupID, memberID)
				h.logGroupMemberChange(r, logger.ActionSCIMGroupRemoveMember, snapshot, memberID, execErr)
			}
			return nil, nil
		}
	}

	return []attrChange{{Op: opLower, Path: op.Path, NewValue: "<unsupported>"}}, nil
}

// handleSCIMUserDeactivation deactivates owned agents, revokes related tokens,
// and records the offboarding impact. trigger identifies the IdP operation;
// scimManaged flags unexpected deactivation of a locally managed user.
func (h *SCIMHandler) handleSCIMUserDeactivation(r *http.Request, userID int, username, trigger string, scimManaged bool) {
	cascade, err := h.deactivateCascade(userID)
	if err != nil {
		slog.Error("scim: offboarding cascade failed",
			slog.Int("owner_id", userID),
			slog.String("trigger", trigger),
			slog.Any("error", err))
		h.logSCIMAuditEvent(r, logger.ActionSCIMUserAgentImpact, logger.ResourceUser, &userID, username,
			map[string]any{"trigger": trigger}, false, err.Error())
		return
	}
	if len(cascade.AgentIDs) == 0 && len(cascade.RevokedAPITokens) == 0 {
		return
	}

	slog.Warn("scim: offboarding cascaded to agent users and tokens",
		slog.Int("owner_id", userID),
		slog.String("owner_username", username),
		slog.String("trigger", trigger),
		slog.Any("deactivated_agent_ids", cascade.AgentIDs),
		slog.Int("revoked_api_tokens", len(cascade.RevokedAPITokens)))

	h.logSCIMAuditEvent(r, logger.ActionSCIMUserAgentImpact, logger.ResourceUser, &userID, username,
		map[string]any{
			"trigger":               trigger,
			"deactivated_agent_ids": cascade.AgentIDs,
			"revoked_api_tokens":    len(cascade.RevokedAPITokens),
		}, true, "")

	// Per-resource audit rows preserve the offboarding trail.
	for _, aid := range cascade.AgentIDs {
		agentID := aid
		h.logSCIMAuditEvent(r, logger.ActionAgentDeactivate, logger.ResourceUser, &agentID, "",
			map[string]any{
				"reason":   "scim_owner_deactivated",
				"owner_id": userID,
				"trigger":  trigger,
			}, true, "")
	}
	for _, tid := range cascade.RevokedAPITokens {
		tokenID := tid
		h.logSCIMAuditEvent(r, logger.ActionAPITokenAutoRevoke, logger.ResourceAPIToken, &tokenID, "",
			map[string]any{
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
	adminIDs, err := h.activeSystemAdminIDs()
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

	meta, _ := json.Marshal(map[string]any{
		"source":                "scim",
		"trigger":               trigger,
		"owner_id":              ownerID,
		"owner_username":        ownerUsername,
		"owner_scim_managed":    scimManaged,
		"deactivated_agent_ids": cascade.AgentIDs,
		"revoked_api_tokens":    len(cascade.RevokedAPITokens),
	})

	if h.notificationService == nil {
		slog.Warn("scim: notification service unavailable; skipping admin notifications")
		return
	}
	for _, aid := range adminIDs {
		_, err := h.notificationService.CreateNotification(models.Notification{
			UserID:             aid,
			Title:              title,
			Message:            message,
			Type:               "warning",
			Metadata:           string(meta),
			AuthorizationScope: models.NotificationScopeSystem,
			SourceType:         "scim.user_deactivation",
			SourceID:           &ownerID,
		})
		if err != nil {
			slog.Warn("scim: failed to create admin notification",
				slog.Int("admin_id", aid), slog.Any("error", err))
		}
	}
}

// logGroupMemberChange writes a single per-member audit entry. The success flag
// and error message reflect the DB write result, so the audit log can be queried
// for failed SCIM member ops (e.g., FK violations on non-existent user_id).
func (h *SCIMHandler) logGroupMemberChange(r *http.Request, actionType string, group *models.TeamGroup, memberID int, execErr error) {
	details := map[string]any{
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
