package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// Errors returned from the programmatic agent-creation entry points. HTTP
// handlers translate these to status codes; other callers (the CLI onboarding
// flow, tests) can branch on them directly.
var (
	ErrAgentsDisabled      = errors.New("user-managed agents are disabled")
	ErrAgentLimitReached   = errors.New("agent limit reached")
	ErrAgentUsernameTaken  = errors.New("username already exists")
	ErrAgentEmailTaken     = errors.New("email already exists")
	ErrAgentInactive       = errors.New("agent is inactive")
	ErrInvalidAgentRequest = errors.New("invalid agent request")
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

// UpdateAgentRequest changes an owned agent's human-readable name. Username is
// intentionally absent: ws init uses it as the stable per-machine identity.
type UpdateAgentRequest struct {
	Name string `json:"name" validate:"required,max=100"`
}

// allowUserManagedAgents reads the admin flag that unlocks self-serve agent
// creation. Admins bypass the flag and can always manage agents.
func (h *AgentHandler) allowUserManagedAgents() bool {
	value, ok, err := repository.NewSystemSettingRepository(h.db).GetValue("allow_user_managed_agents")
	if err == nil && ok {
		return strings.EqualFold(value, "true")
	}
	return false
}

// maxAgentsPerUser reads the configurable cap. Falls back to 5 when the
// setting is missing or malformed.
func (h *AgentHandler) maxAgentsPerUser() int {
	value, ok, err := repository.NewSystemSettingRepository(h.db).GetValue("max_agents_per_user")
	if err == nil && ok {
		if n, perr := strconv.Atoi(value); perr == nil && n >= 0 {
			return n
		}
	}
	return 5
}

// countOwnedAgents returns the number of agents owned by the given user.
func (h *AgentHandler) countOwnedAgents(ownerID int) (int, error) {
	return repository.NewUserRepository(h.db).CountOwnedAgents(ownerID)
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
		return nil, fmt.Errorf("%w: %v", ErrInvalidAgentRequest, err)
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

	userRepo := repository.NewUserRepository(h.db)
	emailExists, err := userRepo.EmailExists(email, 0)
	if err != nil {
		return nil, err
	}
	if emailExists {
		return nil, ErrAgentEmailTaken
	}
	usernameExists, err := userRepo.UsernameExists(req.Username, 0)
	if err != nil {
		return nil, err
	}
	if usernameExists {
		return nil, ErrAgentUsernameTaken
	}

	now := time.Now()
	newID, err := userRepo.Create(repository.CreateUserParams{
		Email:                 email,
		Username:              req.Username,
		FirstName:             req.FirstName,
		LastName:              req.LastName,
		IsActive:              true,
		RequiresPasswordReset: false,
		IsAgent:               true,
		EmailVerified:         true,
		AgentOwnerUserID:      &ownerID,
		AgentProvenance:       "user",
	})
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateEntry) {
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
		AgentProvenance:  "user",
		FullName:         strings.TrimSpace(req.FirstName + " " + req.LastName),
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

// ErrOAuthClientDisabledOrMissing fires when CreateOAuthAgent is called with
// a client_id that doesn't reference an enabled oauth_clients row. This is a
// defense-in-depth check — the OAuth code-exchange path already verifies the
// client at consent time, but the window between consent approve and agent
// provisioning is non-zero.
var ErrOAuthClientDisabledOrMissing = errors.New("oauth client is disabled or does not exist")

// CreateOAuthAgent provisions one OAuth-provenance agent per client and user.
// It bypasses self-service limits but requires an enabled client.
func (h *AgentHandler) CreateOAuthAgent(ownerID, oauthClientID int, req CreateAgentRequest) (*models.User, error) {
	if err := utils.Validate(req); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidAgentRequest, err)
	}

	// Recheck the client to close a disable-after-consent race.
	enabled, err := repository.NewOAuthClientRepository(h.db).EnabledByID(oauthClientID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrOAuthClientDisabledOrMissing
	}
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, ErrOAuthClientDisabledOrMissing
	}

	email := strings.TrimSpace(req.Email)
	if email == "" {
		email = fmt.Sprintf("agent-%s-%d@agents.local", strings.ToLower(req.Username), time.Now().UnixNano())
	}

	userRepo := repository.NewUserRepository(h.db)
	emailExists, err := userRepo.EmailExists(email, 0)
	if err != nil {
		return nil, err
	}
	if emailExists {
		return nil, ErrAgentEmailTaken
	}
	usernameExists, err := userRepo.UsernameExists(req.Username, 0)
	if err != nil {
		return nil, err
	}
	if usernameExists {
		return nil, ErrAgentUsernameTaken
	}

	now := time.Now()
	newID, err := userRepo.Create(repository.CreateUserParams{
		Email:                 email,
		Username:              req.Username,
		FirstName:             req.FirstName,
		LastName:              req.LastName,
		IsActive:              true,
		RequiresPasswordReset: false,
		IsAgent:               true,
		EmailVerified:         true,
		AgentOwnerUserID:      &ownerID,
		AgentProvenance:       "oauth",
		OAuthClientID:         &oauthClientID,
	})
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateEntry) {
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
		AgentProvenance:  "oauth",
		OAuthClientID:    &oauthClientID,
		FullName:         strings.TrimSpace(req.FirstName + " " + req.LastName),
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

// FindOwnedAgentByUsername looks up an existing owned agent by username. Used
// by the CLI onboarding flow so repeat `ws init` runs on the same machine
// reuse the same agent row (stable identity, revocable per-machine).
func (h *AgentHandler) FindOwnedAgentByUsername(ownerID int, username string) (*models.User, error) {
	return repository.NewUserRepository(h.db).FindOwnedAgentByUsername(ownerID, username)
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
	// Agent first/last name render in mentions, item author bylines,
	// and the agent picker. Username is identifier-shaped (mention
	// slug). Email is validated separately as an email address.
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.FirstName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.LastName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.Username, Policy: sanitize.ShortIdentifier},
	)

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
				ResourceType: logger.ResourceAgent,
				Details:      map[string]any{"reason": "feature_disabled"},
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
				ResourceType: logger.ResourceAgent,
				Details:      map[string]any{"reason": "max_agents_reached"},
				Success:      false,
				ErrorMessage: err.Error(),
			})
			respondForbidden(w, r)
		case errors.Is(err, ErrAgentUsernameTaken):
			respondConflict(w, r, "Username already exists")
		case errors.Is(err, ErrAgentEmailTaken):
			respondConflict(w, r, "Email already exists")
		case errors.Is(err, ErrInvalidAgentRequest):
			respondValidationError(w, r, strings.TrimPrefix(err.Error(), ErrInvalidAgentRequest.Error()+": "))
		default:
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
		ResourceType: logger.ResourceAgent,
		ResourceID:   &agent.ID,
		ResourceName: agent.Username,
		Details: map[string]any{
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

	agents, err := repository.NewUserRepository(h.db).ListOwnedAgents(currentUser.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, agents)
}

// Update handles PATCH /api/me/agents/{id}.
func (h *AgentHandler) Update(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	agentID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := decodeJSON[UpdateAgentRequest](w, r)
	if !ok {
		return
	}
	sanitize.ApplyAll(sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField})
	req.Name = strings.TrimSpace(req.Name)
	if err := utils.Validate(req); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	agent, err := repository.NewUserRepository(h.db).UpdateOwnedAgentName(agentID, currentUser.ID, req.Name)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "agent")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       currentUser.ID,
		Username:     currentUser.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionAgentUpdate,
		ResourceType: logger.ResourceAgent,
		ResourceID:   &agentID,
		ResourceName: agent.Username,
		Details: map[string]any{
			"agent_kind":    "owned",
			"owner_user_id": currentUser.ID,
			"name":          req.Name,
		},
		Success: true,
	})

	respondJSONOK(w, agent)
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
	userRepo := repository.NewUserRepository(h.db)
	ownerID, username, err := userRepo.OwnedAgentTarget(agentID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "agent")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if ownerID == nil || *ownerID != currentUser.ID {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionAgentDelete,
			ResourceType: logger.ResourceAgent,
			ResourceID:   &agentID,
			Details:      map[string]any{"reason": "not_agent_owner"},
			Success:      false,
			ErrorMessage: "caller does not own target agent",
		})
		respondForbidden(w, r)
		return
	}

	if err = userRepo.Delete(agentID); err != nil {
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
		ResourceType: logger.ResourceAgent,
		ResourceID:   &agentID,
		ResourceName: username,
		Details: map[string]any{
			"agent_kind":    "owned",
			"owner_user_id": currentUser.ID,
		},
		Success: true,
	})

	w.WriteHeader(http.StatusNoContent)
}
