package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// Errors returned from the programmatic agent-creation entry points. HTTP
// handlers translate these to status codes; other callers (the CLI onboarding
// flow, tests) can branch on them directly.
var (
	ErrAgentsDisabled     = errors.New("user-managed agents are disabled")
	ErrAgentLimitReached  = errors.New("agent limit reached")
	ErrAgentUsernameTaken = errors.New("username already exists")
	ErrAgentEmailTaken    = errors.New("email already exists")
)

// AgentHandler handles profile-scoped CRUD for owned agent users.
// Service users (admin-provisioned, no owner) are NOT managed through this
// surface — they go through the regular admin user-create path.
type AgentHandler struct {
	db                database.Database
	permissionService *services.PermissionService
}

func NewAgentHandler(db database.Database, permissionService *services.PermissionService) *AgentHandler {
	return &AgentHandler{db: db, permissionService: permissionService}
}

// CreateAgentRequest is the payload for POST /api/me/agents.
type CreateAgentRequest struct {
	Username  string `json:"username" validate:"required,min=3,max=32"`
	FirstName string `json:"first_name" validate:"required,max=50"`
	LastName  string `json:"last_name" validate:"required,max=50"`
	Email     string `json:"email,omitempty" validate:"omitempty,email,max=255"`
}

// allowUserManagedAgents reads the admin flag that unlocks self-serve agent
// creation. Admins bypass the flag and can always manage agents.
func (h *AgentHandler) allowUserManagedAgents() bool {
	var value string
	if err := h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'allow_user_managed_agents'").Scan(&value); err == nil {
		return strings.EqualFold(value, "true")
	}
	return false
}

// maxAgentsPerUser reads the configurable cap. Falls back to 5 when the
// setting is missing or malformed.
func (h *AgentHandler) maxAgentsPerUser() int {
	var value string
	if err := h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'max_agents_per_user'").Scan(&value); err == nil {
		if n, perr := strconv.Atoi(value); perr == nil && n >= 0 {
			return n
		}
	}
	return 5
}

// countOwnedAgents returns the number of agents owned by the given user.
func (h *AgentHandler) countOwnedAgents(ownerID int) (int, error) {
	var count int
	err := h.db.QueryRow("SELECT COUNT(*) FROM users WHERE agent_owner_user_id = ?", ownerID).Scan(&count)
	return count, err
}

// CreateOwnedAgent provisions a new agent owned by ownerID after running the
// same policy checks that gate POST /api/me/agents. Returns a typed error when
// a policy or uniqueness check fails so non-HTTP callers (the CLI onboarding
// flow) can branch on the cause. Does NOT write an audit log — the caller is
// expected to attach its own event so the source of the call (profile page vs
// CLI onboarding vs future entry points) remains distinguishable.
func (h *AgentHandler) CreateOwnedAgent(ownerID int, isAdmin bool, req CreateAgentRequest) (*models.User, error) {
	if !isAdmin && !h.allowUserManagedAgents() {
		return nil, ErrAgentsDisabled
	}
	if err := utils.Validate(req); err != nil {
		return nil, fmt.Errorf("invalid agent request: %w", err)
	}
	if !isAdmin {
		maxAgents := h.maxAgentsPerUser()
		count, err := h.countOwnedAgents(ownerID)
		if err != nil {
			return nil, err
		}
		if count >= maxAgents {
			return nil, ErrAgentLimitReached
		}
	}

	// Email is optional for agents. When omitted, synthesize a unique local
	// address so the UNIQUE constraint on users.email still holds and the
	// agent can coexist with real users.
	email := strings.TrimSpace(req.Email)
	if email == "" {
		email = fmt.Sprintf("agent-%s-%d@agents.local", strings.ToLower(req.Username), time.Now().UnixNano())
	}

	var emailExists bool
	_ = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE email = ?)", email).Scan(&emailExists)
	if emailExists {
		return nil, ErrAgentEmailTaken
	}
	var usernameExists bool
	_ = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username = ?)", req.Username).Scan(&usernameExists)
	if usernameExists {
		return nil, ErrAgentUsernameTaken
	}

	now := time.Now()
	var newID int64
	err := h.db.QueryRow(`
		INSERT INTO users (email, username, first_name, last_name, is_active, password_hash, requires_password_reset, is_agent, agent_owner_user_id, email_verified, created_at, updated_at)
		VALUES (?, ?, ?, ?, true, NULL, false, true, ?, true, ?, ?) RETURNING id
	`, email, req.Username, req.FirstName, req.LastName, ownerID, now, now).Scan(&newID)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return nil, ErrAgentUsernameTaken
		}
		return nil, err
	}

	agentID := int(newID)
	return &models.User{
		ID:               agentID,
		Email:            email,
		Username:         req.Username,
		FirstName:        req.FirstName,
		LastName:         req.LastName,
		IsActive:         true,
		IsAgent:          true,
		AgentOwnerUserID: &ownerID,
		FullName:         strings.TrimSpace(req.FirstName + " " + req.LastName),
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

// FindOwnedAgentByUsername looks up an existing owned agent by username. Used
// by the CLI onboarding flow so repeat `ws init` runs on the same machine
// reuse the same agent row (stable identity, revocable per-machine).
func (h *AgentHandler) FindOwnedAgentByUsername(ownerID int, username string) (*models.User, error) {
	var u models.User
	var avatarURL sql.NullString
	err := h.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, created_at, updated_at
		FROM users
		WHERE username = ? AND agent_owner_user_id = ?
	`, username, ownerID).Scan(&u.ID, &u.Email, &u.Username, &u.FirstName, &u.LastName, &u.IsActive, &avatarURL, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.IsAgent = true
	u.AgentOwnerUserID = &ownerID
	if avatarURL.Valid {
		u.AvatarURL = avatarURL.String
	}
	u.FullName = strings.TrimSpace(u.FirstName + " " + u.LastName)
	return &u, nil
}

// Create handles POST /api/me/agents.
func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	isAdmin, _ := h.permissionService.IsSystemAdmin(currentUser.ID)

	req, ok := decodeJSON[CreateAgentRequest](w, r)
	if !ok {
		return
	}

	agent, err := h.CreateOwnedAgent(currentUser.ID, isAdmin, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrAgentsDisabled):
			_ = logger.LogAudit(h.db, logger.AuditEvent{
				UserID:       currentUser.ID,
				Username:     currentUser.Username,
				IPAddress:    utils.GetClientIP(r),
				UserAgent:    r.UserAgent(),
				ActionType:   logger.ActionAgentCreate,
				ResourceType: logger.ResourceUser,
				Details:      map[string]interface{}{"reason": "feature_disabled"},
				Success:      false,
				ErrorMessage: err.Error(),
			})
			respondForbidden(w, r)
		case errors.Is(err, ErrAgentLimitReached):
			_ = logger.LogAudit(h.db, logger.AuditEvent{
				UserID:       currentUser.ID,
				Username:     currentUser.Username,
				IPAddress:    utils.GetClientIP(r),
				UserAgent:    r.UserAgent(),
				ActionType:   logger.ActionAgentCreate,
				ResourceType: logger.ResourceUser,
				Details:      map[string]interface{}{"reason": "max_agents_reached"},
				Success:      false,
				ErrorMessage: err.Error(),
			})
			respondForbidden(w, r)
		case errors.Is(err, ErrAgentUsernameTaken):
			respondConflict(w, r, "Username already exists")
		case errors.Is(err, ErrAgentEmailTaken):
			respondConflict(w, r, "Email already exists")
		default:
			if strings.Contains(err.Error(), "invalid agent request") {
				respondValidationError(w, r, strings.TrimPrefix(err.Error(), "invalid agent request: "))
				return
			}
			respondInternalError(w, r, err)
		}
		return
	}

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       currentUser.ID,
		Username:     currentUser.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionAgentCreate,
		ResourceType: logger.ResourceUser,
		ResourceID:   &agent.ID,
		ResourceName: agent.Username,
		Details: map[string]interface{}{
			"agent_kind":    "owned",
			"owner_user_id": currentUser.ID,
			"email":         agent.Email,
		},
		Success: true,
	})

	respondJSONCreated(w, agent)
}

// List handles GET /api/me/agents.
func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	rows, err := h.db.Query(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, created_at, updated_at
		FROM users
		WHERE agent_owner_user_id = ?
		ORDER BY created_at DESC
	`, currentUser.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	agents := []models.User{}
	for rows.Next() {
		var u models.User
		var avatarURL sql.NullString
		if scanErr := rows.Scan(&u.ID, &u.Email, &u.Username, &u.FirstName, &u.LastName, &u.IsActive, &avatarURL, &u.CreatedAt, &u.UpdatedAt); scanErr != nil {
			slog.Warn("agent list scan failed", slog.Any("error", scanErr))
			continue
		}
		u.IsAgent = true
		ownerID := currentUser.ID
		u.AgentOwnerUserID = &ownerID
		if avatarURL.Valid {
			u.AvatarURL = avatarURL.String
		}
		u.FullName = strings.TrimSpace(u.FirstName + " " + u.LastName)
		agents = append(agents, u)
	}

	respondJSONOK(w, agents)
}

// Delete handles DELETE /api/me/agents/{id}.
func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	agentID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Verify ownership before deletion to avoid disclosing whether the agent exists.
	var ownerID sql.NullInt64
	var username string
	err := h.db.QueryRow("SELECT agent_owner_user_id, username FROM users WHERE id = ?", agentID).Scan(&ownerID, &username)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "agent")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !ownerID.Valid || int(ownerID.Int64) != currentUser.ID {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionAgentDelete,
			ResourceType: logger.ResourceUser,
			ResourceID:   &agentID,
			Details:      map[string]interface{}{"reason": "not_agent_owner"},
			Success:      false,
			ErrorMessage: "caller does not own target agent",
		})
		respondForbidden(w, r)
		return
	}

	if _, err = h.db.ExecWrite("DELETE FROM users WHERE id = ?", agentID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	_ = h.permissionService.InvalidateUserCache(agentID)

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       currentUser.ID,
		Username:     currentUser.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionAgentDelete,
		ResourceType: logger.ResourceUser,
		ResourceID:   &agentID,
		ResourceName: username,
		Details: map[string]interface{}{
			"agent_kind":    "owned",
			"owner_user_id": currentUser.ID,
		},
		Success: true,
	})

	w.WriteHeader(http.StatusNoContent)
}
