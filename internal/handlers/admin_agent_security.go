package handlers

import (
	"net/http"
	"strconv"

	"windshift/internal/logger"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
)

// AgentSecurityHandler hosts the global-admin Security Settings surface
// for the coding-agent harness's acting-identity gate (WI-87 / WI-83 §7).
// All write paths emit audit-log entries via logger.LogAudit; the
// repository is the single source of truth for both the master flag and
// the per-user-and-workspace allowlist.
type AgentSecurityHandler struct {
	repo              *repository.AgentSecurityRepository
	userSvc           *services.UserReadService
	permissionService *services.PermissionService
	auditor           *logger.Auditor
}

// NewAgentSecurityHandler wires the handler to its repo + user-classification
// service + auth + audit dependencies. permissionService.IsSystemAdmin gates
// every endpoint; the auditor records both flag flips and allowlist mutations
// with the actor and a required reason; userSvc.IsCentralizedServiceUser
// keeps non-service users off the allowlist at the boundary.
func NewAgentSecurityHandler(
	repo *repository.AgentSecurityRepository,
	userSvc *services.UserReadService,
	permissionService *services.PermissionService,
	auditor *logger.Auditor,
) *AgentSecurityHandler {
	return &AgentSecurityHandler{
		repo:              repo,
		userSvc:           userSvc,
		permissionService: permissionService,
		auditor:           auditor,
	}
}

// flagResponse is what GET /admin/agent-security/settings returns and what
// PUT echoes back. Kept as its own type so the JSON shape stays stable
// independent of repository internals.
type agentSecurityFlagResponse struct {
	AllowCentralizedServiceUsers bool `json:"allow_centralized_service_users"`
}

type agentSecurityFlagUpdate struct {
	AllowCentralizedServiceUsers bool   `json:"allow_centralized_service_users"`
	Reason                       string `json:"reason"`
}

// GetSettings returns the current state of the master flag.
func (h *AgentSecurityHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireSystemAdmin(w, r, user.ID, h.permissionService) {
		return
	}
	enabled, err := h.repo.GetAllowCentralizedServiceUsers(r.Context())
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSON(w, http.StatusOK, agentSecurityFlagResponse{AllowCentralizedServiceUsers: enabled})
}

// UpdateSettings writes the master flag. A non-empty reason is required so
// the audit trail always carries operator justification — "for testing" is
// fine, "<empty>" is not.
func (h *AgentSecurityHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireSystemAdmin(w, r, user.ID, h.permissionService) {
		return
	}
	var body agentSecurityFlagUpdate
	if err := newJSONDecoder(w, r).Decode(&body); err != nil {
		respondBadRequest(w, r, "invalid request body")
		return
	}
	sanitize.Apply(&body.Reason, sanitize.PlainTextField)
	if body.Reason == "" {
		respondBadRequest(w, r, "reason is required")
		return
	}
	if err := h.repo.SetAllowCentralizedServiceUsers(r.Context(), body.AllowCentralizedServiceUsers); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentSecurityFlagSet, logger.ResourceAgentSecurityFlag, nil, "allow_centralized_service_users", map[string]any{
		"enabled": body.AllowCentralizedServiceUsers,
		"reason":  body.Reason,
	})
	respondJSON(w, http.StatusOK, agentSecurityFlagResponse{AllowCentralizedServiceUsers: body.AllowCentralizedServiceUsers})
}

type allowlistEntryResponse struct {
	UserID          int    `json:"user_id"`
	WorkspaceID     *int   `json:"workspace_id,omitempty"`
	Reason          string `json:"reason"`
	CreatedByUserID *int   `json:"created_by_user_id,omitempty"`
	CreatedAt       string `json:"created_at"`
}

// allowlistCreateRequest is the payload for POST
// /admin/agent-security/allowlist. workspace_ids is the canonical shape:
// an empty/missing array means a single "any workspace" grant, a
// non-empty array creates one grant per id atomically.
type allowlistCreateRequest struct {
	UserID       int    `json:"user_id"`
	WorkspaceIDs []int  `json:"workspace_ids,omitempty"`
	Reason       string `json:"reason"`
}

// ListAllowlist returns every (user, workspace?) grant.
func (h *AgentSecurityHandler) ListAllowlist(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireSystemAdmin(w, r, user.ID, h.permissionService) {
		return
	}
	entries, err := h.repo.ListAllowlist(r.Context())
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	out := make([]allowlistEntryResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, allowlistEntryResponse{
			UserID:          e.UserID,
			WorkspaceID:     e.WorkspaceID,
			Reason:          e.Reason,
			CreatedByUserID: e.CreatedByUserID,
			CreatedAt:       e.CreatedAt,
		})
	}
	respondJSON(w, http.StatusOK, out)
}

// AddAllowlist inserts a new grant. user_id is required; workspace_id is
// optional (omit / null = any workspace). A non-empty reason is required.
func (h *AgentSecurityHandler) AddAllowlist(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireSystemAdmin(w, r, user.ID, h.permissionService) {
		return
	}
	var body allowlistCreateRequest
	if err := newJSONDecoder(w, r).Decode(&body); err != nil {
		respondBadRequest(w, r, "invalid request body")
		return
	}
	sanitize.Apply(&body.Reason, sanitize.PlainTextField)
	if body.UserID <= 0 {
		respondBadRequest(w, r, "user_id is required")
		return
	}
	if body.Reason == "" {
		respondBadRequest(w, r, "reason is required")
		return
	}
	// Only centralized service users (is_agent=true, no owner) belong on
	// the allowlist. Owned agents reach bindings through the WI-87
	// chokepoint without needing a grant, and regular humans must never
	// be impersonated by the harness. Classification lives in the user
	// service so the same rules apply wherever it's checked.
	isService, err := h.userSvc.IsCentralizedServiceUser(r.Context(), body.UserID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !isService {
		respondBadRequest(w, r, "user_id must reference a centralized service user (is_agent=true, no owner)")
		return
	}
	if err := h.repo.AddAllowlistEntries(r.Context(), body.UserID, body.WorkspaceIDs, &user.ID, body.Reason); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentSecurityAllowlistAdd, logger.ResourceAgentSecurityAllowlist, &body.UserID, "", map[string]any{
		"workspace_ids": body.WorkspaceIDs,
		"reason":        body.Reason,
	})

	// Echo the created grants back. An empty WorkspaceIDs slice yields
	// a single any-workspace response row (matching the persisted
	// shape).
	out := make([]allowlistEntryResponse, 0, max(1, len(body.WorkspaceIDs)))
	if len(body.WorkspaceIDs) == 0 {
		out = append(out, allowlistEntryResponse{
			UserID: body.UserID,
			Reason: body.Reason,
		})
	} else {
		for i := range body.WorkspaceIDs {
			ws := body.WorkspaceIDs[i]
			out = append(out, allowlistEntryResponse{
				UserID:      body.UserID,
				WorkspaceID: &ws,
				Reason:      body.Reason,
			})
		}
	}
	respondJSON(w, http.StatusCreated, out)
}

// RemoveAllowlist deletes a grant. user_id comes from the path; workspace_id
// is an optional query param (omit for the any-workspace grant). A reason
// query param is required so the audit trail captures intent.
func (h *AgentSecurityHandler) RemoveAllowlist(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireSystemAdmin(w, r, user.ID, h.permissionService) {
		return
	}
	userIDStr := r.PathValue("user_id")
	userID, err := strconv.Atoi(userIDStr)
	if err != nil || userID <= 0 {
		respondBadRequest(w, r, "user_id path param must be a positive integer")
		return
	}
	var workspaceID *int
	if v := r.URL.Query().Get("workspace_id"); v != "" {
		ws, convErr := strconv.Atoi(v)
		if convErr != nil || ws <= 0 {
			respondBadRequest(w, r, "workspace_id query param must be a positive integer")
			return
		}
		workspaceID = &ws
	}
	reason := r.URL.Query().Get("reason")
	sanitize.Apply(&reason, sanitize.PlainTextField)
	if reason == "" {
		respondBadRequest(w, r, "reason query param is required")
		return
	}
	n, err := h.repo.RemoveAllowlistEntry(r.Context(), userID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if n == 0 {
		respondNotFound(w, r, "allowlist entry")
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentSecurityAllowlistRemove, logger.ResourceAgentSecurityAllowlist, &userID, "", map[string]any{
		"workspace_id": workspaceID,
		"reason":       reason,
	})
	w.WriteHeader(http.StatusNoContent)
}
