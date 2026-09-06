package handlers

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/utils"

	"golang.org/x/crypto/bcrypt"
)

type UserHandler struct {
	repo               *repository.UserRepository
	auditor            *logger.Auditor
	permissionService  *services.PermissionService
	invitationService  *services.InvitationService
	userSvc            *services.UserReadService
	offboardUser       func(id int) error
	deactivateCascade  func(id int) (services.AgentDeactivationResult, error)
	invalidateSessions func(id int)
	workspaceUsers     *services.WorkspaceUserResolver
}

// SetWorkspaceUserResolver wires the shared picker and validation roster.
func (h *UserHandler) SetWorkspaceUserResolver(resolver *services.WorkspaceUserResolver) {
	h.workspaceUsers = resolver
}

func (h *UserHandler) invalidateUserSessions(userID int) {
	if h.invalidateSessions != nil {
		h.invalidateSessions(userID)
	}
}

// CreateUserRequest represents the request payload for creating a user.
//
// is_active is optional and defaults to false when omitted, so admins still
// get the activation gate by default; the create dialog exposes it as an
// explicit opt-in so the resulting state is visible at create time instead
// of surprising the admin after the fact.
type CreateUserRequest struct {
	Email     string `json:"email" validate:"required,email,max=255"`
	Username  string `json:"username" validate:"required,min=3,max=32"`
	FirstName string `json:"first_name" validate:"required,max=50"`
	LastName  string `json:"last_name" validate:"required,max=50"`
	AvatarURL string `json:"avatar_url,omitempty"`
	Password  string `json:"password,omitempty"` // Plaintext password, will be hashed
	IsActive  *bool  `json:"is_active,omitempty"`
	IsAgent   bool   `json:"is_agent,omitempty"` // If true, create as agent user (no password, no interactive login)
}

// UpdateUserRequest represents the request payload for updating a user
type UpdateUserRequest struct {
	Email     string `json:"email" validate:"required,email,max=255"`
	Username  string `json:"username" validate:"required,min=3,max=32"`
	FirstName string `json:"first_name" validate:"required,max=50"`
	LastName  string `json:"last_name" validate:"required,max=50"`
	AvatarURL string `json:"avatar_url,omitempty"`
	Timezone  string `json:"timezone,omitempty"`
	Language  string `json:"language,omitempty"`
}

// UpdateRegionalSettingsRequest represents the request payload for updating regional settings
type UpdateRegionalSettingsRequest struct {
	Timezone string `json:"timezone"`
	Language string `json:"language"`
}

// NewUserHandler creates a UserHandler. offboardUser and deactivateCascade are
// dependency-injected closures over services.OffboardUser and
// services.DeactivateOwnedAgentsAndTokens; injecting them at construction lets
// the handler stay free of the database import.
func NewUserHandler(
	repo *repository.UserRepository,
	auditor *logger.Auditor,
	permissionService *services.PermissionService,
	invitationService *services.InvitationService,
	userSvc *services.UserReadService,
	offboardUser func(id int) error,
	deactivateCascade func(id int) (services.AgentDeactivationResult, error),
	invalidateSessions func(id int),
) *UserHandler {
	return &UserHandler{
		repo:               repo,
		auditor:            auditor,
		permissionService:  permissionService,
		invitationService:  invitationService,
		userSvc:            userSvc,
		offboardUser:       offboardUser,
		deactivateCascade:  deactivateCascade,
		invalidateSessions: invalidateSessions,
	}
}

func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[CreateUserRequest](w, r)
	if !ok {
		return
	}
	// Names render in member lists, mentions, comment author bylines,
	// and audit log entries. Username is identifier-shaped (URL slug
	// for @mentions). AvatarURL is validated separately as a URL.
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.FirstName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.LastName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.Username, Policy: sanitize.ShortIdentifier},
	)

	if err := utils.Validate(req); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Hash password if provided
	var passwordHash *string
	if req.Password != "" {
		hashedBytes, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to hash password: %w", err))
			return
		}
		s := string(hashedBytes)
		passwordHash = &s
	}

	// Agent users never authenticate interactively; clear any submitted password.
	if req.IsAgent {
		passwordHash = nil
	}

	if exists, err := h.repo.EmailExists(req.Email, 0); err != nil {
		respondInternalError(w, r, err)
		return
	} else if exists {
		respondConflict(w, r, "Email already exists")
		return
	}
	if exists, err := h.repo.UsernameExists(req.Username, 0); err != nil {
		respondInternalError(w, r, err)
		return
	} else if exists {
		respondConflict(w, r, "Username already exists")
		return
	}

	id, err := h.repo.Create(repository.CreateUserParams{
		Email:                 req.Email,
		Username:              req.Username,
		FirstName:             req.FirstName,
		LastName:              req.LastName,
		AvatarURL:             req.AvatarURL,
		PasswordHash:          passwordHash,
		RequiresPasswordReset: !req.IsAgent && req.Password == "",
		IsActive:              req.IsActive != nil && *req.IsActive,
		IsAgent:               req.IsAgent,
		EmailVerified:         true,
	})
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateEntry) {
			respondConflict(w, r, "Email or username already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	user, err := h.repo.GetByID(int(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionUserCreate, logger.ResourceUser,
			&user.ID, user.Username,
			map[string]any{
				"email":      user.Email,
				"username":   user.Username,
				"first_name": user.FirstName,
				"last_name":  user.LastName,
				"is_active":  user.IsActive,
				"is_agent":   user.IsAgent,
			},
		)
	}

	respondJSONCreated(w, user)
}

// InviteUser handles inviting a user to the system
func (h *UserHandler) InviteUser(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[CreateUserRequest](w, r)
	if !ok {
		return
	}
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.FirstName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.LastName, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.Username, Policy: sanitize.ShortIdentifier},
	)

	if err := utils.Validate(req); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Invitation flow ignores any submitted password; user sets one when they accept.
	if exists, err := h.repo.EmailExists(req.Email, 0); err != nil {
		respondInternalError(w, r, err)
		return
	} else if exists {
		respondConflict(w, r, "Email already exists")
		return
	}
	if exists, err := h.repo.UsernameExists(req.Username, 0); err != nil {
		respondInternalError(w, r, err)
		return
	} else if exists {
		respondConflict(w, r, "Username already exists")
		return
	}

	id, err := h.repo.Create(repository.CreateUserParams{
		Email:                 req.Email,
		Username:              req.Username,
		FirstName:             req.FirstName,
		LastName:              req.LastName,
		AvatarURL:             req.AvatarURL,
		PasswordHash:          nil,
		RequiresPasswordReset: true,
		IsAgent:               false,
		EmailVerified:         false,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	user, err := h.repo.GetByID(int(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	token, err := h.invitationService.GenerateInvitation(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	emailErr := h.invitationService.SendInvitationEmail(user, token)
	if emailErr != nil {
		slog.Warn("failed to send invitation email", "error", emailErr, "user_id", user.ID)
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionUserCreate, logger.ResourceUser,
			&user.ID, user.Username,
			map[string]any{
				"email":      user.Email,
				"username":   user.Username,
				"is_invite":  true,
				"email_sent": emailErr == nil,
			},
		)
	}

	respondJSONCreated(w, map[string]any{
		"user":       user,
		"token":      token,
		"email_sent": emailErr == nil,
	})
}

func (h *UserHandler) UpdateAvatar(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if currentUser.ID != id {
		isAdmin, err := h.permissionService.IsSystemAdmin(currentUser.ID)
		if err != nil || !isAdmin {
			respondForbidden(w, r)
			return
		}
	}

	var req struct {
		AvatarURL *string `json:"avatar_url"` // pointer to distinguish null vs empty
	}
	if err := newJSONDecoder(w, r).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	avatarURL := ""
	if req.AvatarURL != nil {
		avatarURL = *req.AvatarURL
	}
	if err := h.repo.UpdateAvatar(id, avatarURL); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.invalidateUserSessions(id)

	user, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "user")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, user)
}

// UpdateRegionalSettings updates only the timezone and language fields for a user
func (h *UserHandler) UpdateRegionalSettings(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if currentUser.ID != id {
		isAdmin, err := h.permissionService.IsSystemAdmin(currentUser.ID)
		if err != nil || !isAdmin {
			respondForbidden(w, r)
			return
		}
	}

	req, ok := decodeJSON[UpdateRegionalSettingsRequest](w, r)
	if !ok {
		return
	}
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.Timezone, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &req.Language, Policy: sanitize.ShortIdentifier},
	)

	old, err := h.repo.GetRegionalSnapshot(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "user")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	timezone := req.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	language := req.Language
	if language == "" {
		language = "en"
	}

	if err := h.repo.UpdateRegional(id, timezone, language); err != nil {
		respondInternalError(w, r, err)
		return
	}

	user, err := h.repo.GetByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	changes := make(map[string]any)
	oldTz := "UTC"
	if old.Timezone.Valid {
		oldTz = old.Timezone.String
	}
	if oldTz != timezone {
		changes["timezone"] = map[string]string{"old": oldTz, "new": timezone}
	}
	oldLang := "en"
	if old.Language.Valid {
		oldLang = old.Language.String
	}
	if oldLang != language {
		changes["language"] = map[string]string{"old": oldLang, "new": language}
	}
	if len(changes) > 0 {
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionUserUpdate, logger.ResourceUser,
			&user.ID, user.Username, changes,
		)
	}

	respondJSONOK(w, user)
}

func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil && currentUser.ID == id {
		respondForbidden(w, r)
		return
	}

	deleted, err := h.repo.GetDeleteSnapshot(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "user")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if deleted.SCIMManaged {
		respondForbidden(w, r)
		return
	}

	if err := h.offboardUser(id); err != nil {
		if errors.Is(err, services.ErrUserOffboardingHasProtectedIntegrationLinks) {
			respondConflict(w, r, "Cannot offboard user while their personal workspace contains protected integration links")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Audit after successful offboarding so a failed or blocked operation is
	// never recorded as a successful deletion.
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionUserDelete, logger.ResourceUser,
			&id, deleted.Username,
			map[string]any{
				"email":      deleted.Email,
				"first_name": deleted.FirstName,
				"last_name":  deleted.LastName,
			},
		)
	}
	h.invalidateUserSessions(id)

	w.WriteHeader(http.StatusNoContent)
}

// ResetPasswordRequest represents the request to reset a user's password
type ResetPasswordRequest struct {
	UserID         int    `json:"user_id"`
	Password       string `json:"password,omitempty"`        // Custom password to set
	GenerateRandom bool   `json:"generate_random,omitempty"` // Generate random password
}

// ResetPassword generates a new temporary password and marks user for password reset
func (h *UserHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireSystemAdmin(w, r, currentUser.ID, h.permissionService) {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := decodeJSON[ResetPasswordRequest](w, r)
	if !ok {
		return
	}

	var password string
	requiresReset := true
	var response map[string]any

	if req.GenerateRandom || req.Password == "" {
		password = generateTempPassword()
		response = map[string]any{
			"temporary_password": password,
			"message":            "Password reset successfully. User must change password on next login.",
		}
	} else {
		password = req.Password
		requiresReset = false
		response = map[string]any{
			"message": "Password set successfully.",
		}
	}

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to hash password: %w", err))
		return
	}

	target, err := h.repo.GetPasswordResetTarget(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "user")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if err := h.repo.SetPassword(id, string(hashedBytes), requiresReset); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.invalidateUserSessions(id)

	h.auditor.LogWithDetails(r, currentUser,
		logger.ActionUserPasswordReset, logger.ResourceUser,
		&id, target.Username,
		map[string]any{
			"email":                   target.Email,
			"requires_password_reset": requiresReset,
			"password_type":           map[bool]string{true: "generated", false: "custom"}[req.GenerateRandom || req.Password == ""],
		},
	)

	respondJSONOK(w, response)
}

// GetAgentOwner returns the owner attribution for an agent user. Gated on
// user.list (or system admin) so callers without permission to see other
// users' profiles can't dereference agents to their human owners. 404 is
// returned both when the target isn't an agent and when an unauthorized
// caller asks — keeping the responses indistinguishable prevents probing
// for which user IDs are agents.
func (h *UserHandler) GetAgentOwner(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !canViewAgentOwnerAttribution(h.permissionService, currentUser.ID) {
		respondNotFound(w, r, "agent")
		return
	}

	info, err := h.repo.GetAgentOwner(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "agent")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, info)
}

// canViewAgentOwnerAttribution centralizes the visibility rule used by both
// the individual lookup and enriched activity DTOs. Permission lookup errors
// fail closed.
func canViewAgentOwnerAttribution(permissionService *services.PermissionService, userID int) bool {
	if permissionService == nil {
		return false
	}
	isAdmin, _ := permissionService.IsSystemAdmin(userID)
	if isAdmin {
		return true
	}
	hasListPermission, _ := permissionService.HasGlobalPermission(userID, models.PermissionUserList)
	return hasListPermission
}

// ActivateUser activates a user account
func (h *UserHandler) ActivateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	target, err := h.repo.GetActivationTarget(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "user")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if target.IsActive {
		respondValidationError(w, r, "User is already active")
		return
	}

	if err := h.repo.SetActive(id, true); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.invalidateUserSessions(id)

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionUserActivate, logger.ResourceUser,
			&id, target.Username,
			map[string]any{
				"email":          target.Email,
				"previous_state": "inactive",
				"new_state":      "active",
			},
		)
	}

	respondJSONOK(w, map[string]string{"message": "User activated successfully"})
}

// DeactivateUser deactivates a user account
func (h *UserHandler) DeactivateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil && currentUser.ID == id {
		respondForbidden(w, r)
		return
	}

	target, err := h.repo.GetActivationTarget(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "user")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if !target.IsActive {
		respondValidationError(w, r, "User is already inactive")
		return
	}

	// The shared offboarding service atomically deactivates the user and their
	// agents, revokes their API tokens, and invalidates security caches.
	cascade, cascadeErr := h.deactivateCascade(id)
	if cascadeErr != nil {
		respondInternalError(w, r, cascadeErr)
		return
	}

	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionUserDeactivate, logger.ResourceUser,
			&id, target.Username,
			map[string]any{
				"email":              target.Email,
				"previous_state":     "active",
				"new_state":          "inactive",
				"cascaded_agents":    cascade.AgentIDs,
				"revoked_api_tokens": len(cascade.RevokedAPITokens),
			},
		)

		// Per-agent and per-token audit rows so security can reconstruct what
		// died alongside the deactivation.
		for _, agentID := range cascade.AgentIDs {
			aid := agentID
			h.auditor.LogWithDetails(r, currentUser,
				logger.ActionAgentDeactivate, logger.ResourceUser,
				&aid, "",
				map[string]any{
					"reason":   "owner_deactivated",
					"owner_id": id,
				},
			)
		}
		for _, tid := range cascade.RevokedAPITokens {
			tokenID := tid
			h.auditor.LogWithDetails(r, currentUser,
				logger.ActionAPITokenAutoRevoke, logger.ResourceAPIToken,
				&tokenID, "",
				map[string]any{
					"reason":   "owner_deactivated",
					"owner_id": id,
					"table":    "api_tokens",
				},
			)
		}
	}

	respondJSONOK(w, map[string]string{"message": "User deactivated successfully"})
}

// generateTempPassword creates a secure temporary password
func generateTempPassword() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	password := make([]byte, 12)

	for i := range password {
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			password[i] = charset[i%len(charset)]
		} else {
			password[i] = charset[randomIndex.Int64()]
		}
	}
	return string(password)
}

// nullableString returns the string boxed as an interface for SQL params,
// or nil when empty so the column receives SQL NULL. Kept here because
// notification.go still uses it; will move to a shared helpers file when
// that handler migrates.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
