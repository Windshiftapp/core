package handlers

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"

	"golang.org/x/crypto/bcrypt"
)

type UserHandler struct {
	repo              *repository.UserRepository
	auditor           *logger.Auditor
	permissionService *services.PermissionService
	invitationService *services.InvitationService
	userSvc           *services.UserReadService
	offboardUser      func(id int) error
	deactivateCascade func(id int) (services.AgentDeactivationResult, error)
}

// CreateUserRequest represents the request payload for creating a user.
//
// Note: is_active is intentionally not accepted from the client. Newly
// created users are always inserted as inactive and need to be explicitly
// activated (POST /users/{id}/activate) or to complete the invitation flow.
// Accepting it from JSON would let an admin (or anyone who later reuses
// this struct) skip the activation gate.
type CreateUserRequest struct {
	Email     string `json:"email" validate:"required,email,max=255"`
	Username  string `json:"username" validate:"required,min=3,max=32"`
	FirstName string `json:"first_name" validate:"required,max=50"`
	LastName  string `json:"last_name" validate:"required,max=50"`
	AvatarURL string `json:"avatar_url,omitempty"`
	Password  string `json:"password,omitempty"` // Plaintext password, will be hashed
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
) *UserHandler {
	return &UserHandler{
		repo:              repo,
		auditor:           auditor,
		permissionService: permissionService,
		invitationService: invitationService,
		userSvc:           userSvc,
		offboardUser:      offboardUser,
		deactivateCascade: deactivateCascade,
	}
}

func (h *UserHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	isAdmin, _ := h.permissionService.IsSystemAdmin(currentUser.ID)

	// Any authenticated user can list users (needed for issue assignment, mentions, etc.)
	// System admins see all users with full details, regular users see only active users
	// with limited fields.
	if !isAdmin {
		users, err := h.userSvc.ListAll()
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		// Strip sensitive fields for non-admins
		for i := range users {
			users[i].Email = ""
			users[i].Timezone = ""
			users[i].Language = ""
		}
		respondJSONOK(w, users)
		return
	}

	users, err := h.repo.ListAdmin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, users)
}

func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	isOwnProfile := currentUser.ID == id
	isAdmin, _ := h.permissionService.IsSystemAdmin(currentUser.ID)
	hasListPerm, _ := h.permissionService.HasGlobalPermission(currentUser.ID, models.PermissionUserList)

	if !isOwnProfile && !isAdmin && !hasListPerm {
		respondForbidden(w, r)
		return
	}

	user, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "user")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Non-admin users with user.list can only see active users (not their own profile)
	if !isOwnProfile && !isAdmin && !user.IsActive {
		respondNotFound(w, r, "user")
		return
	}

	// Limit returned fields when a non-admin views someone else's profile.
	if !isOwnProfile && !isAdmin {
		user.Email = ""
		user.RequiresPasswordReset = false
		user.Timezone = ""
		user.Language = ""
	}

	respondJSONOK(w, user)
}

func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[CreateUserRequest](w, r)
	if !ok {
		return
	}

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
			map[string]interface{}{
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
			map[string]interface{}{
				"email":      user.Email,
				"username":   user.Username,
				"is_invite":  true,
				"email_sent": emailErr == nil,
			},
		)
	}

	respondJSONCreated(w, map[string]interface{}{
		"user":       user,
		"token":      token,
		"email_sent": emailErr == nil,
	})
}

func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := decodeJSON[UpdateUserRequest](w, r)
	if !ok {
		return
	}

	if err := utils.Validate(req); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	old, err := h.repo.GetUpdateProfileSnapshot(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "user")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if old.SCIMManaged {
		respondForbidden(w, r)
		return
	}

	timezone := req.Timezone
	if timezone == "" && old.Timezone.Valid {
		timezone = old.Timezone.String
	}
	if timezone == "" {
		timezone = "UTC"
	}

	language := req.Language
	if language == "" && old.Language.Valid {
		language = old.Language.String
	}
	if language == "" {
		language = "en"
	}

	if exists, err := h.repo.EmailExists(req.Email, id); err != nil {
		respondInternalError(w, r, err)
		return
	} else if exists {
		respondConflict(w, r, "Email already exists")
		return
	}
	if exists, err := h.repo.UsernameExists(req.Username, id); err != nil {
		respondInternalError(w, r, err)
		return
	} else if exists {
		respondConflict(w, r, "Username already exists")
		return
	}

	if err := h.repo.UpdateProfile(id, repository.UpdateProfileParams{
		Email:     req.Email,
		Username:  req.Username,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		AvatarURL: req.AvatarURL,
		Timezone:  timezone,
		Language:  language,
	}); err != nil {
		if errors.Is(err, repository.ErrDuplicateEntry) {
			respondConflict(w, r, "Email or username already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	user, err := h.repo.GetByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	user.IsActive = old.IsActive // Preserve — Update doesn't toggle activation.

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		changes := make(map[string]interface{})
		if old.Email != req.Email {
			changes["email"] = map[string]string{"old": old.Email, "new": req.Email}
		}
		if old.Username != req.Username {
			changes["username"] = map[string]string{"old": old.Username, "new": req.Username}
		}
		if old.FirstName != req.FirstName {
			changes["first_name"] = map[string]string{"old": old.FirstName, "new": req.FirstName}
		}
		if old.LastName != req.LastName {
			changes["last_name"] = map[string]string{"old": old.LastName, "new": req.LastName}
		}
		oldAvatarURL := ""
		if old.AvatarURL.Valid {
			oldAvatarURL = old.AvatarURL.String
		}
		if oldAvatarURL != req.AvatarURL {
			changes["avatar_url"] = map[string]string{"old": oldAvatarURL, "new": req.AvatarURL}
		}
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

		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionUserUpdate, logger.ResourceUser,
			&user.ID, user.Username, changes,
		)
	}

	respondJSONOK(w, user)
}

// UpdateAvatar updates only the avatar_url field for a user
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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

	changes := make(map[string]interface{})
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

	// Audit before anonymization so we capture the original details.
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionUserDelete, logger.ResourceUser,
			&id, deleted.Username,
			map[string]interface{}{
				"email":      deleted.Email,
				"first_name": deleted.FirstName,
				"last_name":  deleted.LastName,
			},
		)
	}

	if err := h.offboardUser(id); err != nil {
		respondInternalError(w, r, err)
		return
	}

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
	var response map[string]interface{}

	if req.GenerateRandom || req.Password == "" {
		password = generateTempPassword()
		response = map[string]interface{}{
			"temporary_password": password,
			"message":            "Password reset successfully. User must change password on next login.",
		}
	} else {
		password = req.Password
		requiresReset = false
		response = map[string]interface{}{
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

	h.auditor.LogWithDetails(r, currentUser,
		logger.ActionUserPasswordReset, logger.ResourceUser,
		&id, target.Username,
		map[string]interface{}{
			"email":                   target.Email,
			"requires_password_reset": requiresReset,
			"password_type":           map[bool]string{true: "generated", false: "custom"}[req.GenerateRandom || req.Password == ""],
		},
	)

	respondJSONOK(w, response)
}

// GetAssignable returns only active users with limited fields for assignment pickers.
// The workspaceId path parameter is accepted for future workspace-scoped filtering but not yet used.
func (h *UserHandler) GetAssignable(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	users, err := h.userSvc.ListAll()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	for i := range users {
		users[i].Email = ""
		users[i].Timezone = ""
		users[i].Language = ""
	}

	respondJSONOK(w, users)
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

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionUserActivate, logger.ResourceUser,
			&id, target.Username,
			map[string]interface{}{
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

	if err := h.repo.SetActive(id, false); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Propagate to any agents this user owns: flip them inactive and revoke every
	// token held by the owner or their agents in a single transaction.
	cascade, cascadeErr := h.deactivateCascade(id)
	if cascadeErr != nil {
		slog.Warn("deactivation cascade failed",
			slog.String("component", "users"),
			slog.Int("owner_id", id),
			slog.Any("error", cascadeErr))
	}

	// Invalidate permission caches (fans out to owned agents via PermissionService).
	_ = h.permissionService.InvalidateUserCache(id)

	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionUserDeactivate, logger.ResourceUser,
			&id, target.Username,
			map[string]interface{}{
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
				map[string]interface{}{
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
				map[string]interface{}{
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
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
