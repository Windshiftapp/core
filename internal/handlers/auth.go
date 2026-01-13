package handlers

import (
	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/sso"
	"windshift/internal/utils"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	db                       database.Database
	sessionManager           *auth.SessionManager
	rateLimiter              *middleware.RateLimiter
	permissionService        *services.PermissionService
	emailVerificationService *services.EmailVerificationService
	ipExtractor              *utils.IPExtractor
}

// LoginRequest represents the login request payload
type LoginRequest struct {
	EmailOrUsername string `json:"email_or_username" validate:"required"`
	Password        string `json:"password" validate:"required"`
	RememberMe      bool   `json:"remember_me"`
}

// ChangePasswordRequest represents the change password request payload
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required"`
	LogoutAll       bool   `json:"logout_all"`
}

// LoginResponse represents the login response
type LoginResponse struct {
	Success bool          `json:"success"`
	User    *models.User  `json:"user,omitempty"`
	Message string        `json:"message,omitempty"`
}

// UserResponse represents the current user response
type UserResponse struct {
	User    *models.User `json:"user"`
	Session *SessionInfo `json:"session"`
}

// SessionInfo represents session information
type SessionInfo struct {
	ExpiresAt time.Time `json:"expires_at"`
	IPAddress string    `json:"ip_address"`
	CreatedAt time.Time `json:"created_at"`
}

// NewAuthHandler creates a new authentication handler
// emailVerificationService can be nil if SMTP is not configured
func NewAuthHandler(db database.Database, sessionManager *auth.SessionManager, rateLimiter *middleware.RateLimiter, permissionService *services.PermissionService, emailVerificationService *services.EmailVerificationService, ipExtractor *utils.IPExtractor) *AuthHandler {
	return &AuthHandler{
		db:                       db,
		sessionManager:           sessionManager,
		rateLimiter:              rateLimiter,
		permissionService:        permissionService,
		emailVerificationService: emailVerificationService,
		ipExtractor:              ipExtractor,
	}
}

// populateIsSystemAdmin checks if user has system.admin permission and sets the cached field
// This is called once at login/authentication to avoid repeated DB queries
func (h *AuthHandler) populateIsSystemAdmin(user *models.User) error {
	isAdmin, err := h.permissionService.IsSystemAdmin(user.ID)
	if err != nil {
		return fmt.Errorf("failed to check system admin status: %w", err)
	}
	user.IsSystemAdmin = isAdmin
	return nil
}

// Login handles user authentication
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// Check if password login is allowed (SSO-only mode check)
	providerStore := sso.NewProviderStore(h.db)
	defaultProvider, err := providerStore.GetDefault()
	if err == nil && defaultProvider != nil && defaultProvider.Enabled && !defaultProvider.AllowPasswordLogin {
		http.Error(w, "Password login is disabled. Please use SSO to sign in.", http.StatusForbidden)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate input using validator
	if err := utils.Validate(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get client IP for rate limiting
	ipAddress := h.getClientIP(r)

	// Check if IP is locked out due to failed attempts
	if locked, duration := h.rateLimiter.IsLockedOut(ipAddress); locked {
		http.Error(w, fmt.Sprintf("Too many failed login attempts. Please try again in %s", middleware.FormatLockoutDuration(duration)), http.StatusTooManyRequests)
		return
	}

	// Find user by email or username
	user, err := h.findUserByEmailOrUsername(req.EmailOrUsername)
	if err != nil {
		if err == sql.ErrNoRows {
			// Record failed attempt
			h.rateLimiter.RecordFailedLogin(ipAddress)
			// Don't reveal whether user exists or not
			time.Sleep(100 * time.Millisecond) // Prevent timing attacks
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}
		http.Error(w, "Authentication error", http.StatusInternalServerError)
		return
	}

	// Check if user is active
	if !user.IsActive {
		http.Error(w, "Account is disabled", http.StatusUnauthorized)
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		// Record failed attempt
		h.rateLimiter.RecordFailedLogin(ipAddress)
		// Add delay to prevent timing attacks
		time.Sleep(100 * time.Millisecond)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Clear failed attempts on successful login
	h.rateLimiter.RecordSuccessfulLogin(ipAddress)

	// Create session
	session, err := h.sessionManager.CreateSession(user.ID, ipAddress, r.UserAgent(), req.RememberMe)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	if err := h.sessionManager.SetSessionCookie(w, r, session.Token, req.RememberMe); err != nil {
		http.Error(w, "Failed to set session", http.StatusInternalServerError)
		return
	}

	// Update user's full name for response
	user.FullName = fmt.Sprintf("%s %s", user.FirstName, user.LastName)
	user.PasswordHash = "" // Never send password hash

	// Populate system admin status (cached for frontend)
	if err := h.populateIsSystemAdmin(user); err != nil {
		slog.Warn("failed to populate system admin status", slog.String("component", "auth"), slog.Any("error", err))
		// Continue anyway - user can still login, just without admin flag
	}

	response := LoginResponse{
		Success: true,
		User:    user,
		Message: "Login successful",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Logout handles user logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Get session token
	token, err := h.sessionManager.GetSessionFromRequest(r)
	if err != nil {
		// Even if no session found, clear cookie and return success
		h.sessionManager.ClearSessionCookie(w, r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Logout successful",
		})
		return
	}

	// Invalidate session
	if err := h.sessionManager.DeleteSession(token); err != nil {
		http.Error(w, "Failed to logout", http.StatusInternalServerError)
		return
	}

	// Clear session cookie
	h.sessionManager.ClearSessionCookie(w, r)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Logout successful",
	})
}

// GetCurrentUser returns information about the currently authenticated user
func (h *AuthHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	// Get session token from request (since auth middleware skips /api/auth/ paths)
	token, err := h.sessionManager.GetSessionFromRequest(r)
	if err != nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Get client IP for validation
	clientIP := h.getClientIP(r)

	// Validate session
	session, err := h.sessionManager.ValidateSession(token, clientIP)
	if err != nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Populate system admin status (cached for frontend)
	if err := h.populateIsSystemAdmin(session.User); err != nil {
		slog.Warn("failed to populate system admin status", slog.String("component", "auth"), slog.Any("error", err))
		// Continue anyway - user info will be returned, just without admin flag
	}

	// Prepare response
	sessionInfo := &SessionInfo{
		ExpiresAt: session.ExpiresAt,
		IPAddress: session.IPAddress,
		CreatedAt: session.CreatedAt,
	}

	response := UserResponse{
		User:    session.User,
		Session: sessionInfo,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// RefreshSession extends the current session
func (h *AuthHandler) RefreshSession(w http.ResponseWriter, r *http.Request) {
	// Get session token
	token, err := h.sessionManager.GetSessionFromRequest(r)
	if err != nil {
		http.Error(w, "No session found", http.StatusUnauthorized)
		return
	}

	// Parse request body for remember me option
	var req struct {
		RememberMe bool `json:"remember_me"`
	}
	json.NewDecoder(r.Body).Decode(&req) // Optional, ignore errors

	// Refresh session
	if err := h.sessionManager.RefreshSession(token, req.RememberMe); err != nil {
		http.Error(w, "Failed to refresh session", http.StatusInternalServerError)
		return
	}

	// Update cookie with new expiration
	if err := h.sessionManager.SetSessionCookie(w, r, token, req.RememberMe); err != nil {
		http.Error(w, "Failed to update session cookie", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Session refreshed",
	})
}

// LogoutAll invalidates all sessions for the current user
func (h *AuthHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	// Get session from context
	session, ok := r.Context().Value("session").(*auth.Session)
	if !ok || session == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	// Invalidate all user sessions
	if err := h.sessionManager.DeleteAllUserSessions(session.UserID); err != nil {
		http.Error(w, "Failed to logout all sessions", http.StatusInternalServerError)
		return
	}

	// Clear current session cookie
	h.sessionManager.ClearSessionCookie(w, r)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "All sessions logged out",
	})
}

// findUserByEmailOrUsername finds a user by email or username
func (h *AuthHandler) findUserByEmailOrUsername(emailOrUsername string) (*models.User, error) {
	query := `
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, password_hash, requires_password_reset, created_at, updated_at
		FROM users
		WHERE email = ? OR username = ?
	`

	row := h.db.QueryRow(query, emailOrUsername, emailOrUsername)

	user := &models.User{}
	var avatarURL sql.NullString

	err := row.Scan(
		&user.ID, &user.Email, &user.Username, &user.FirstName, &user.LastName,
		&user.IsActive, &avatarURL, &user.PasswordHash,
		&user.RequiresPasswordReset, &user.CreatedAt, &user.UpdatedAt,
	)
	
	if err != nil {
		return nil, err
	}

	if avatarURL.Valid {
		user.AvatarURL = avatarURL.String
	}

	return user, nil
}

// getClientIP extracts the client IP with proxy validation
func (h *AuthHandler) getClientIP(r *http.Request) string {
	return h.ipExtractor.GetClientIP(r)
}

// ChangePassword allows authenticated users to change their password
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	// Get session from context
	session, ok := r.Context().Value("session").(*auth.Session)
	if !ok || session == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate input using validator
	if err := utils.Validate(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get current user data to verify current password
	user, err := h.findUserByEmailOrUsername(session.User.Email)
	if err != nil {
		http.Error(w, "User not found", http.StatusInternalServerError)
		return
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		http.Error(w, "Current password is incorrect", http.StatusUnauthorized)
		return
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to hash new password", http.StatusInternalServerError)
		return
	}

	// Update password in database
	query := `UPDATE users SET password_hash = ?, requires_password_reset = 0, updated_at = ? WHERE id = ?`
	_, err = h.db.ExecWrite(query, string(hashedPassword), time.Now(), session.UserID)
	if err != nil {
		http.Error(w, "Failed to update password", http.StatusInternalServerError)
		return
	}

	// Optionally logout all other sessions
	if req.LogoutAll {
		// Delete all sessions for this user
		h.sessionManager.DeleteAllUserSessions(session.UserID)
		
		// Recreate current session
		newSession, err := h.sessionManager.CreateSession(
			session.UserID,
			h.getClientIP(r),
			r.UserAgent(),
			false, // Don't assume remember me for security
		)
		if err == nil {
			h.sessionManager.SetSessionCookie(w, r, newSession.Token, false)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Password changed successfully",
	})
}

// VerifyEmail handles email verification via token
func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Verification token is required", http.StatusBadRequest)
		return
	}

	if h.emailVerificationService == nil {
		http.Error(w, "Email verification is not configured", http.StatusServiceUnavailable)
		return
	}

	user, err := h.emailVerificationService.VerifyEmail(token)
	if err != nil {
		switch err {
		case services.ErrTokenExpired:
			http.Error(w, "Verification link has expired. Please request a new one.", http.StatusGone)
		case services.ErrTokenInvalid:
			http.Error(w, "Invalid verification link", http.StatusBadRequest)
		case services.ErrAlreadyVerified:
			// Not an error - just let them know
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"message": "Email is already verified",
			})
			return
		default:
			slog.Error("email verification error", slog.String("component", "auth"), slog.Any("error", err))
			http.Error(w, "Failed to verify email", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "Email verified successfully",
		"user_id":  user.ID,
		"verified": true,
	})
}

// ResendVerification resends the verification email to the current user
func (h *AuthHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	// Get session from context
	session, ok := r.Context().Value("session").(*auth.Session)
	if !ok || session == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	if h.emailVerificationService == nil {
		http.Error(w, "Email verification is not configured", http.StatusServiceUnavailable)
		return
	}

	err := h.emailVerificationService.ResendVerification(session.UserID)
	if err != nil {
		switch err {
		case services.ErrAlreadyVerified:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"message": "Email is already verified",
			})
			return
		case services.ErrUserNotFound:
			http.Error(w, "User not found", http.StatusNotFound)
		case services.ErrSMTPNotConfigured:
			http.Error(w, "Email service is not available", http.StatusServiceUnavailable)
		default:
			slog.Error("failed to resend verification", slog.String("component", "auth"), slog.Any("error", err))
			http.Error(w, "Failed to send verification email", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Verification email sent",
	})
}

// GetVerificationStatus returns the email verification status for the current user
func (h *AuthHandler) GetVerificationStatus(w http.ResponseWriter, r *http.Request) {
	// Get session from context
	session, ok := r.Context().Value("session").(*auth.Session)
	if !ok || session == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	if h.emailVerificationService == nil {
		// If email verification service is not configured, assume verified
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"email_verified": true,
			"configured":     false,
		})
		return
	}

	verified, err := h.emailVerificationService.IsEmailVerified(session.UserID)
	if err != nil {
		slog.Error("failed to check verification status", slog.String("component", "auth"), slog.Any("error", err))
		// Return verified=true on error for graceful degradation
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"email_verified": true,
			"configured":     false,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"email_verified": verified,
		"configured":     true,
	})
}

