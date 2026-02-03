package handlers

import (
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type UserHandler struct {
	db                database.Database
	permissionService *services.PermissionService
}

// CreateUserRequest represents the request payload for creating a user
type CreateUserRequest struct {
	Email     string `json:"email" validate:"required,email,max=255"`
	Username  string `json:"username" validate:"required,min=3,max=32"`
	FirstName string `json:"first_name" validate:"required,max=50"`
	LastName  string `json:"last_name" validate:"required,max=50"`
	IsActive  bool   `json:"is_active"`
	AvatarURL string `json:"avatar_url,omitempty"`
	Password  string `json:"password,omitempty"` // Plaintext password, will be hashed
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

func NewUserHandler(db database.Database, permissionService *services.PermissionService) *UserHandler {
	return &UserHandler{db: db, permissionService: permissionService}
}

func (h *UserHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Authorization check
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	// Check if system admin - gets full access
	isAdmin, _ := h.permissionService.IsSystemAdmin(currentUser.ID)

	// Any authenticated user can list users (needed for issue assignment, mentions, etc.)
	// System admins see all users with full details, regular users see only active users with limited fields
	var query string
	if isAdmin {
		query = `SELECT id, email, username, first_name, last_name, is_active, avatar_url, requires_password_reset, timezone, language, created_at, updated_at FROM users ORDER BY last_name, first_name`
	} else {
		// Limited query for non-admins: active users only, limited fields
		query = `SELECT id, '', username, first_name, last_name, is_active, avatar_url, 0, '', '', created_at, updated_at FROM users WHERE is_active = 1 ORDER BY last_name, first_name`
	}

	rows, err := h.db.Query(query)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		var avatarURL sql.NullString
		var requiresPasswordReset sql.NullBool
		var timezone sql.NullString
		var language sql.NullString
		err := rows.Scan(&user.ID, &user.Email, &user.Username, &user.FirstName, &user.LastName,
			&user.IsActive, &avatarURL, &requiresPasswordReset, &timezone, &language, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		user.AvatarURL = avatarURL.String
		user.RequiresPasswordReset = requiresPasswordReset.Bool
		user.Timezone = "UTC"
		if timezone.Valid {
			user.Timezone = timezone.String
		}
		user.Language = "en"
		if language.Valid {
			user.Language = language.String
		}

		// Set full name for display
		user.FullName = strings.TrimSpace(user.FirstName + " " + user.LastName)

		users = append(users, user)
	}

	if users == nil {
		users = []models.User{}
	}

	respondJSONOK(w, users)
}

func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Authorization check
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	// Determine access level
	isOwnProfile := currentUser.ID == id
	isAdmin, _ := h.permissionService.IsSystemAdmin(currentUser.ID)
	hasListPerm, _ := h.permissionService.HasGlobalPermission(currentUser.ID, models.PermissionUserList)

	// Must have permission: own profile, system admin, or user.list permission
	if !isOwnProfile && !isAdmin && !hasListPerm {
		respondForbidden(w, r)
		return
	}

	var user models.User
	var avatarURL sql.NullString
	var requiresPasswordReset sql.NullBool
	var timezone sql.NullString
	var language sql.NullString
	err := h.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, requires_password_reset, timezone, language, created_at, updated_at
		FROM users WHERE id = ?
	`, id).Scan(&user.ID, &user.Email, &user.Username, &user.FirstName, &user.LastName,
		&user.IsActive, &avatarURL, &requiresPasswordReset, &timezone, &language, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
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

	user.AvatarURL = avatarURL.String
	user.Timezone = "UTC"
	if timezone.Valid {
		user.Timezone = timezone.String
	}
	user.Language = "en"
	if language.Valid {
		user.Language = language.String
	}

	// Set full name for display
	user.FullName = strings.TrimSpace(user.FirstName + " " + user.LastName)

	// For non-admins viewing other users (not own profile), limit the fields returned
	if !isOwnProfile && !isAdmin {
		// Clear sensitive fields for limited access
		user.Email = ""
		user.RequiresPasswordReset = false
		user.Timezone = ""
		user.Language = ""
	}

	respondJSONOK(w, user)
}

func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest

	// Parse request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate input using validator
	if err := utils.Validate(req); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Hash password if provided
	var passwordHash sql.NullString
	if req.Password != "" {
		hashedBytes, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to hash password: %w", err))
			return
		}
		passwordHash = sql.NullString{String: string(hashedBytes), Valid: true}
	}

	now := time.Now()
	var id int64
	err := h.db.QueryRow(`
		INSERT INTO users (email, username, first_name, last_name, is_active, avatar_url, password_hash, requires_password_reset, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, req.Email, req.Username, req.FirstName, req.LastName, req.IsActive,
		nullableString(req.AvatarURL), passwordHash, req.Password == "", now, now).Scan(&id)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: users.email") {
			respondConflict(w, r, "Email already exists")
			return
		}
		if strings.Contains(err.Error(), "UNIQUE constraint failed: users.username") {
			respondConflict(w, r, "Username already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Return the created user
	var user models.User
	var avatarURL sql.NullString
	var requiresPasswordReset sql.NullBool
	err = h.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, requires_password_reset, created_at, updated_at
		FROM users WHERE id = ?
	`, id).Scan(&user.ID, &user.Email, &user.Username, &user.FirstName, &user.LastName,
		&user.IsActive, &avatarURL, &requiresPasswordReset, &user.CreatedAt, &user.UpdatedAt)
	
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	user.AvatarURL = avatarURL.String

	// Set full name for display
	user.FullName = strings.TrimSpace(user.FirstName + " " + user.LastName)

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionUserCreate,
			ResourceType: logger.ResourceUser,
			ResourceID:   &user.ID,
			ResourceName: user.Username,
			Details: map[string]interface{}{
				"email":      user.Email,
				"username":   user.Username,
				"first_name": user.FirstName,
				"last_name":  user.LastName,
					"is_active":  user.IsActive,
			},
			Success: true,
		})
	}

	respondJSONCreated(w, user)
}

func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Parse request
	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate input using validator
	if err := utils.Validate(req); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Get current user data to preserve is_active field and for audit logging
	var oldUser struct {
		Email       string
		Username    string
		FirstName   string
		LastName    string
		Role        string
		IsActive    bool
		AvatarURL   sql.NullString
		Timezone    sql.NullString
		Language    sql.NullString
		SCIMManaged bool
	}
	err := h.db.QueryRow(`
		SELECT email, username, first_name, last_name, is_active, avatar_url, timezone, language,
		       COALESCE(scim_managed, false)
		FROM users WHERE id = ?`, id).Scan(
		&oldUser.Email, &oldUser.Username, &oldUser.FirstName,
		&oldUser.LastName, &oldUser.IsActive, &oldUser.AvatarURL, &oldUser.Timezone, &oldUser.Language,
		&oldUser.SCIMManaged)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "user")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check if user is SCIM-managed
	if oldUser.SCIMManaged {
		respondForbidden(w, r)
		return
	}

	// Use existing values if not provided in request
	timezone := req.Timezone
	if timezone == "" && oldUser.Timezone.Valid {
		timezone = oldUser.Timezone.String
	}
	if timezone == "" {
		timezone = "UTC"
	}

	language := req.Language
	if language == "" && oldUser.Language.Valid {
		language = oldUser.Language.String
	}
	if language == "" {
		language = "en"
	}

	now := time.Now()
	_, err = h.db.ExecWrite(`
		UPDATE users
		SET email = ?, username = ?, first_name = ?, last_name = ?, avatar_url = ?, timezone = ?, language = ?, updated_at = ?
		WHERE id = ?
	`, req.Email, req.Username, req.FirstName, req.LastName,
	   nullableString(req.AvatarURL), timezone, language, now, id)
	
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: users.email") {
			respondConflict(w, r, "Email already exists")
			return
		}
		if strings.Contains(err.Error(), "UNIQUE constraint failed: users.username") {
			respondConflict(w, r, "Username already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Return the updated user
	var user models.User
	var avatarURL sql.NullString
	var tz sql.NullString
	var lang sql.NullString
	err = h.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, timezone, language, created_at, updated_at
		FROM users WHERE id = ?
	`, id).Scan(&user.ID, &user.Email, &user.Username, &user.FirstName, &user.LastName,
		&user.IsActive, &avatarURL, &tz, &lang, &user.CreatedAt, &user.UpdatedAt)

	// Ensure is_active reflects the preserved value, not what was sent in the request
	user.IsActive = oldUser.IsActive

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if avatarURL.Valid {
		user.AvatarURL = avatarURL.String
	}

	if tz.Valid {
		user.Timezone = tz.String
	} else {
		user.Timezone = "UTC"
	}

	if lang.Valid {
		user.Language = lang.String
	} else {
		user.Language = "en"
	}

	// Set full name for display
	user.FullName = strings.TrimSpace(user.FirstName + " " + user.LastName)

	// Log audit event with old and new values
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		changes := make(map[string]interface{})
		if oldUser.Email != req.Email {
			changes["email"] = map[string]string{"old": oldUser.Email, "new": req.Email}
		}
		if oldUser.Username != req.Username {
			changes["username"] = map[string]string{"old": oldUser.Username, "new": req.Username}
		}
		if oldUser.FirstName != req.FirstName {
			changes["first_name"] = map[string]string{"old": oldUser.FirstName, "new": req.FirstName}
		}
		if oldUser.LastName != req.LastName {
			changes["last_name"] = map[string]string{"old": oldUser.LastName, "new": req.LastName}
		}
		oldAvatarURL := ""
		if oldUser.AvatarURL.Valid {
			oldAvatarURL = oldUser.AvatarURL.String
		}
		if oldAvatarURL != req.AvatarURL {
			changes["avatar_url"] = map[string]string{"old": oldAvatarURL, "new": req.AvatarURL}
		}

		// Track timezone changes
		oldTz := "UTC"
		if oldUser.Timezone.Valid {
			oldTz = oldUser.Timezone.String
		}
		if oldTz != timezone {
			changes["timezone"] = map[string]string{"old": oldTz, "new": timezone}
		}

		// Track language changes
		oldLang := "en"
		if oldUser.Language.Valid {
			oldLang = oldUser.Language.String
		}
		if oldLang != language {
			changes["language"] = map[string]string{"old": oldLang, "new": language}
		}

		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionUserUpdate,
			ResourceType: logger.ResourceUser,
			ResourceID:   &user.ID,
			ResourceName: user.Username,
			Details:      changes,
			Success:      true,
		})
	}

	respondJSONOK(w, user)
}

// UpdateAvatar updates only the avatar_url field for a user
func (h *UserHandler) UpdateAvatar(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Authorization check: user can only update their own avatar, or be a system admin
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
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
		AvatarURL *string `json:"avatar_url"` // Use pointer to distinguish between null and empty string
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Update only the avatar_url field
	avatarURL := ""
	if req.AvatarURL != nil {
		avatarURL = *req.AvatarURL
	}

	_, err := h.db.ExecWrite(`UPDATE users SET avatar_url = ?, updated_at = ? WHERE id = ?`,
		avatarURL, time.Now(), id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the updated user
	var user models.User
	err = h.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, created_at, updated_at
		FROM users WHERE id = ?
	`, id).Scan(&user.ID, &user.Email, &user.Username, &user.FirstName, &user.LastName, &user.IsActive, &user.AvatarURL, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "user")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}
	
	// Set full name for display
	user.FullName = strings.TrimSpace(user.FirstName + " " + user.LastName)

	respondJSONOK(w, user)
}

// UpdateRegionalSettings updates only the timezone and language fields for a user
func (h *UserHandler) UpdateRegionalSettings(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Authorization check: user can only update their own regional settings, or be a system admin
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}
	if currentUser.ID != id {
		isAdmin, err := h.permissionService.IsSystemAdmin(currentUser.ID)
		if err != nil || !isAdmin {
			respondForbidden(w, r)
			return
		}
	}

	var req UpdateRegionalSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Get current values for audit logging
	var oldSettings struct {
		Username string
		Timezone sql.NullString
		Language sql.NullString
	}
	err := h.db.QueryRow(`
		SELECT username, timezone, language
		FROM users WHERE id = ?`, id).Scan(
		&oldSettings.Username, &oldSettings.Timezone, &oldSettings.Language)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "user")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Use defaults if not provided
	timezone := req.Timezone
	if timezone == "" {
		timezone = "UTC"
	}

	language := req.Language
	if language == "" {
		language = "en"
	}

	// Update only timezone and language fields
	_, err = h.db.ExecWrite(`
		UPDATE users
		SET timezone = ?, language = ?, updated_at = ?
		WHERE id = ?
	`, timezone, language, time.Now(), id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the updated user
	var user models.User
	var avatarURL sql.NullString
	var requiresPasswordReset sql.NullBool
	var tz sql.NullString
	var lang sql.NullString
	err = h.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, requires_password_reset, timezone, language, created_at, updated_at
		FROM users WHERE id = ?
	`, id).Scan(&user.ID, &user.Email, &user.Username, &user.FirstName, &user.LastName,
		&user.IsActive, &avatarURL, &requiresPasswordReset, &tz, &lang, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if avatarURL.Valid {
		user.AvatarURL = avatarURL.String
	}

	if requiresPasswordReset.Valid {
		user.RequiresPasswordReset = requiresPasswordReset.Bool
	}

	if tz.Valid {
		user.Timezone = tz.String
	} else {
		user.Timezone = "UTC"
	}

	if lang.Valid {
		user.Language = lang.String
	} else {
		user.Language = "en"
	}

	// Set full name for display
	user.FullName = strings.TrimSpace(user.FirstName + " " + user.LastName)

	// Log audit event (currentUser already retrieved for authorization)
	if currentUser != nil {
		changes := make(map[string]interface{})

		// Track timezone changes
		oldTz := "UTC"
		if oldSettings.Timezone.Valid {
			oldTz = oldSettings.Timezone.String
		}
		if oldTz != timezone {
			changes["timezone"] = map[string]string{"old": oldTz, "new": timezone}
		}

		// Track language changes
		oldLang := "en"
		if oldSettings.Language.Valid {
			oldLang = oldSettings.Language.String
		}
		if oldLang != language {
			changes["language"] = map[string]string{"old": oldLang, "new": language}
		}

		if len(changes) > 0 {
			logger.LogAudit(h.db, logger.AuditEvent{
				UserID:       currentUser.ID,
				Username:     currentUser.Username,
				IPAddress:    utils.GetClientIP(r),
				UserAgent:    r.UserAgent(),
				ActionType:   logger.ActionUserUpdate,
				ResourceType: logger.ResourceUser,
				ResourceID:   &user.ID,
				ResourceName: user.Username,
				Details:      changes,
				Success:      true,
			})
		}
	}

	respondJSONOK(w, user)
}

func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get current user for self-deletion check
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil && currentUser.ID == id {
		respondForbidden(w, r)
		return
	}

	// Get user info before deletion for audit logging
	var deletedUser struct {
		Username    string
		Email       string
		FirstName   string
		LastName    string
		SCIMManaged bool
	}
	err := h.db.QueryRow(`
		SELECT username, email, first_name, last_name, COALESCE(scim_managed, false)
		FROM users WHERE id = ?`, id).Scan(
		&deletedUser.Username, &deletedUser.Email,
		&deletedUser.FirstName, &deletedUser.LastName, &deletedUser.SCIMManaged)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "user")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Check if user is SCIM-managed
	if deletedUser.SCIMManaged {
		respondForbidden(w, r)
		return
	}

	if _, execErr := h.db.ExecWrite("DELETE FROM users WHERE id = ?", id); execErr != nil {
		respondInternalError(w, r, execErr)
		return
	}

	// Log audit event (currentUser already fetched above)
	if currentUser != nil {
		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionUserDelete,
			ResourceType: logger.ResourceUser,
			ResourceID:   &id,
			ResourceName: deletedUser.Username,
			Details: map[string]interface{}{
				"email":      deletedUser.Email,
				"first_name": deletedUser.FirstName,
				"last_name":  deletedUser.LastName,
			},
			Success: true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// ResetPasswordRequest represents the request to reset a user's password
type ResetPasswordRequest struct {
	UserID         int    `json:"user_id"`
	Password       string `json:"password,omitempty"`       // Custom password to set
	GenerateRandom bool   `json:"generate_random,omitempty"` // Generate random password
}

// ResetPassword generates a new temporary password and marks user for password reset
func (h *UserHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Parse request body
	var req ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	var password string
	var requiresReset bool = true
	var response map[string]interface{}

	if req.GenerateRandom || req.Password == "" {
		// Generate a secure temporary password (default behavior)
		password = generateTempPassword()
		response = map[string]interface{}{
			"temporary_password": password,
			"message":            "Password reset successfully. User must change password on next login.",
		}
	} else {
		// Use provided custom password
		password = req.Password
		requiresReset = false // Custom passwords don't require mandatory reset
		response = map[string]interface{}{
			"message": "Password set successfully.",
		}
	}

	// Hash the password
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to hash password: %w", err))
		return
	}

	// Get user info for audit logging
	var targetUser struct {
		Username string
		Email    string
	}
	err = h.db.QueryRow(`SELECT username, email FROM users WHERE id = ?`, id).Scan(
		&targetUser.Username, &targetUser.Email)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "user")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Update user with new password hash
	_, err = h.db.ExecWrite(`
		UPDATE users
		SET password_hash = ?, requires_password_reset = ?, updated_at = ?
		WHERE id = ?
	`, string(hashedBytes), requiresReset, time.Now(), id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionUserPasswordReset,
			ResourceType: logger.ResourceUser,
			ResourceID:   &id,
			ResourceName: targetUser.Username,
			Details: map[string]interface{}{
				"email":                    targetUser.Email,
				"requires_password_reset":  requiresReset,
				"password_type":            map[bool]string{true: "generated", false: "custom"}[req.GenerateRandom || req.Password == ""],
			},
			Success: true,
		})
	}

	respondJSONOK(w, response)
}

// ActivateUser activates a user account
func (h *UserHandler) ActivateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user info for audit logging
	var targetUser struct {
		Username string
		Email    string
		IsActive bool
	}
	err := h.db.QueryRow(`SELECT username, email, is_active FROM users WHERE id = ?`, id).Scan(
		&targetUser.Username, &targetUser.Email, &targetUser.IsActive)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "user")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Check if already active
	if targetUser.IsActive {
		respondValidationError(w, r, "User is already active")
		return
	}

	// Activate user
	_, err = h.db.ExecWrite(`
		UPDATE users
		SET is_active = true, updated_at = ?
		WHERE id = ?
	`, time.Now(), id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionUserActivate,
			ResourceType: logger.ResourceUser,
			ResourceID:   &id,
			ResourceName: targetUser.Username,
			Details: map[string]interface{}{
				"email":          targetUser.Email,
				"previous_state": "inactive",
				"new_state":      "active",
			},
			Success: true,
		})
	}

	respondJSONOK(w, map[string]string{"message": "User activated successfully"})
}

// DeactivateUser deactivates a user account
func (h *UserHandler) DeactivateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get current user for self-deactivation check
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil && currentUser.ID == id {
		respondForbidden(w, r)
		return
	}

	// Get user info for audit logging
	var targetUser struct {
		Username string
		Email    string
		IsActive bool
	}
	err := h.db.QueryRow(`SELECT username, email, is_active FROM users WHERE id = ?`, id).Scan(
		&targetUser.Username, &targetUser.Email, &targetUser.IsActive)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "user")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Check if already inactive
	if !targetUser.IsActive {
		respondValidationError(w, r, "User is already inactive")
		return
	}

	// Deactivate user
	_, err = h.db.ExecWrite(`
		UPDATE users
		SET is_active = false, updated_at = ?
		WHERE id = ?
	`, time.Now(), id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event
	if currentUser != nil {
		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionUserDeactivate,
			ResourceType: logger.ResourceUser,
			ResourceID:   &id,
			ResourceName: targetUser.Username,
			Details: map[string]interface{}{
				"email":          targetUser.Email,
				"previous_state": "active",
				"new_state":      "inactive",
			},
			Success: true,
		})
	}

	respondJSONOK(w, map[string]string{"message": "User deactivated successfully"})
}

// generateTempPassword creates a secure temporary password
func generateTempPassword() string {
	// Generate a 12-character password with mix of characters
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	password := make([]byte, 12)

	for i := range password {
		// Use crypto/rand for secure random number generation
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			// Fallback to a simpler approach if crypto/rand fails
			password[i] = charset[i%len(charset)]
		} else {
			password[i] = charset[randomIndex.Int64()]
		}
	}
	return string(password)
}

// Helper function to handle nullable strings for database operations
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}