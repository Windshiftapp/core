package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"windshift/internal/models"
	"windshift/internal/repository"
)

type createStandardSessionRequest struct {
	AgentProfileID int    `json:"agent_profile_id"`
	Title          string `json:"title,omitempty"`
}

type availableStandardAgent struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Handle    string `json:"handle"`
	AvatarURL string `json:"avatar_url,omitempty"`
	Purpose   string `json:"purpose,omitempty"`
}

func (h *AIHandler) GetGeneralSession(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	session, err := h.conversations.EnsureGeneralSession(r.Context(), user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, session)
}

func (h *AIHandler) ListAgentSessions(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	includeArchived := r.URL.Query().Get("include_archived") == "true"
	sessions, err := h.conversations.ListForParticipant(r.Context(), user.ID, includeArchived)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	workspaceIDs, err := h.permService.AccessibleWorkspaceIDs(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	accessible := make(map[int]struct{}, len(workspaceIDs))
	for _, workspaceID := range workspaceIDs {
		accessible[workspaceID] = struct{}{}
	}
	visible := make([]*models.AgentSession, 0, len(sessions))
	for _, session := range sessions {
		if session.SessionType == models.AgentSessionStandard {
			if session.WorkspaceID == nil {
				continue
			}
			if _, ok := accessible[*session.WorkspaceID]; !ok {
				continue
			}
		}
		visible = append(visible, session)
	}
	respondJSONOK(w, visible)
}

func (h *AIHandler) ListAvailableStandardAgents(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionItemView, h.permService) {
		return
	}
	profiles, err := h.agentBindings.ListForWorkspace(r.Context(), workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	out := make([]availableStandardAgent, 0)
	for _, profile := range profiles {
		if profile.ProfileType != models.AgentProfileStandard || profile.Lifecycle != models.AgentLifecycleReady {
			continue
		}
		out = append(out, availableStandardAgent{
			ID:        profile.ID,
			Name:      profile.DisplayName,
			Handle:    profile.Handle,
			AvatarURL: profile.AvatarURL,
			Purpose:   profile.Purpose,
		})
	}
	respondJSONOK(w, out)
}

func (h *AIHandler) CreateStandardSession(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionItemView, h.permService) {
		return
	}
	req, ok := decodeJSON[createStandardSessionRequest](w, r)
	if !ok {
		return
	}
	if req.AgentProfileID <= 0 {
		respondBadRequest(w, r, "agent_profile_id is required")
		return
	}
	profile, err := h.agentBindings.Get(r.Context(), req.AgentProfileID)
	if err != nil || profile.WorkspaceID != workspaceID {
		respondNotFound(w, r, "agent profile")
		return
	}
	session, err := h.conversations.CreateStandardSession(r.Context(), user.ID, workspaceID, profile, req.Title)
	if err != nil {
		respondConflict(w, r, err.Error())
		return
	}
	respondJSONCreated(w, session)
}

func (h *AIHandler) ListAgentMessages(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	sessionID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	beforeID, _ := strconv.Atoi(r.URL.Query().Get("before_id"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if !h.requireCurrentAgentSessionAccess(w, r, sessionID, user.ID) {
		return
	}
	messages, err := h.conversations.ListMessages(r.Context(), sessionID, beforeID, limit)
	if errors.Is(err, repository.ErrAgentSessionNotFound) {
		respondNotFound(w, r, "agent session")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if messages == nil {
		messages = []models.AgentMessage{}
	}
	respondJSONOK(w, messages)
}

func (h *AIHandler) ArchiveAgentSession(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	sessionID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	if !h.requireCurrentAgentSessionAccess(w, r, sessionID, user.ID) {
		return
	}
	archived, err := h.conversations.ArchiveOwnedStandard(r.Context(), sessionID, user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !archived {
		respondNotFound(w, r, "active owned Standard agent session")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AIHandler) requireCurrentAgentSessionAccess(w http.ResponseWriter, r *http.Request, sessionID, userID int) bool {
	session, err := h.conversations.GetForParticipant(r.Context(), sessionID, userID)
	if errors.Is(err, repository.ErrAgentSessionNotFound) {
		respondNotFound(w, r, "agent session")
		return false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if session.SessionType != models.AgentSessionStandard {
		return true
	}
	if session.WorkspaceID == nil {
		respondNotFound(w, r, "agent session")
		return false
	}
	allowed, err := h.permService.HasWorkspacePermission(userID, *session.WorkspaceID, models.PermissionItemView)
	if err != nil || !allowed {
		respondNotFound(w, r, "agent session")
		return false
	}
	return true
}
