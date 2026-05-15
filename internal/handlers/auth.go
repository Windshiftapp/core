package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"

	"golang.org/x/crypto/bcrypt"
)

// dummyPasswordHash is a pre-computed bcrypt hash used to prevent timing attacks
// when checking passwords for non-existent users. The actual value doesn't matter,
// only that bcrypt.CompareHashAndPassword runs in constant time.
var dummyPasswordHash = []byte("$2a$10$dummyHashForTimingAttackPrevention1234567890")

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	db                       database.Database
	userRepo                 *repository.UserRepository
	credentialRepo           *repository.CredentialRepository
	sessionManager           *auth.SessionManager
	rateLimiter              *middleware.RateLimiter
	permissionService        *services.PermissionService
	emailVerificationService *services.EmailVerificationService
	ipExtractor              *utils.IPExtractor
	authPolicyHandler        *AuthPolicyHandler
	adminRateLimiter         *middleware.AdminFallbackRateLimiter
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
	Success            bool         `json:"success"`
	User               *models.User `json:"user,omitempty"`
	Message            string       `json:"message,omitempty"`
	EnrollmentRequired bool         `json:"enrollment_required,omitempty"`
	SSORequired        bool         `json:"sso_required,omitempty"`
	PolicyMessage      string       `json:"policy_message,omitempty"`
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
// authPolicyHandler and adminRateLimiter can be nil for backwards compatibility
func NewAuthHandler(db database.Database, sessionManager *auth.SessionManager, rateLimiter *middleware.RateLimiter, permissionService *services.PermissionService, emailVerificationService *services.EmailVerificationService, ipExtractor *utils.IPExtractor, authPolicyHandler *AuthPolicyHandler, adminRateLimiter *middleware.AdminFallbackRateLimiter) *AuthHandler {
	return &AuthHandler{
		db:                       db,
		userRepo:                 repository.NewUserRepository(db),
		credentialRepo:           repository.NewCredentialRepository(db),
		sessionManager:           sessionManager,
		rateLimiter:              rateLimiter,
		permissionService:        permissionService,
		emailVerificationService: emailVerificationService,
		ipExtractor:              ipExtractor,
		authPolicyHandler:        authPolicyHandler,
		adminRateLimiter:         adminRateLimiter,
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
	req, ok := decodeJSON[LoginRequest](w, r)
	if !ok {
		return
	}

	// Validate input using validator
	if err := utils.Validate(req); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Get client IP for rate limiting
	ipAddress := h.getClientIP(r)

	// Check if IP is locked out due to failed attempts
	if locked, duration := h.rateLimiter.IsLockedOut(ipAddress); locked {
		respondTooManyRequests(w, r, fmt.Sprintf("Too many failed login attempts. Please try again in %s", middleware.FormatLockoutDuration(duration)))
		return
	}

	// Find user by email or username
	user, err := h.findUserByEmailOrUsername(req.EmailOrUsername)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// Record failed attempt
			h.rateLimiter.RecordFailedLogin(ipAddress)
			// Always perform bcrypt comparison to prevent timing attacks
			_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(req.Password))
			h.logFailedLogin(r, req.EmailOrUsername)
			respondUnauthorized(w, r)
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check if user is active
	if !user.IsActive {
		respondUnauthorized(w, r)
		return
	}

	// Agent users cannot log in interactively; they authenticate via API tokens only.
	if user.IsAgent {
		h.rateLimiter.RecordFailedLogin(ipAddress)
		// Run the same dummy bcrypt the not-found path runs so an attacker
		// can't time-distinguish "agent" from "non-existent user". Without
		// this, agent rejection short-circuits faster than a wrong-password
		// check, leaking which usernames are agents (often higher-value
		// automation accounts).
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(req.Password))
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       user.ID,
			Username:     user.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionLoginFailure,
			ResourceType: logger.ResourceUser,
			ResourceID:   &user.ID,
			ResourceName: user.Username,
			Details: map[string]interface{}{
				"reason":   "agent_user_login_blocked",
				"is_agent": true,
			},
			Success:      false,
			ErrorMessage: "agent users cannot log in interactively",
		})
		respondUnauthorized(w, r)
		return
	}

	// Verify password
	if err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		// Record failed attempt
		h.rateLimiter.RecordFailedLogin(ipAddress)
		h.logFailedLogin(r, req.EmailOrUsername)
		respondUnauthorized(w, r)
		return
	}

	// Clear failed attempts on successful password validation
	h.rateLimiter.RecordSuccessfulLogin(ipAddress)

	// Populate system admin status early (needed for policy checks)
	if err = h.populateIsSystemAdmin(user); err != nil {
		slog.Warn("failed to populate system admin status", slog.String("component", "auth"), slog.Any("error", err))
	}

	// Check auth policy (if handler is available)
	var enrollmentRequired bool
	if h.authPolicyHandler != nil && !h.authPolicyHandler.IsPreviewMode() {
		policy := h.authPolicyHandler.GetCurrentPolicy()

		switch policy {
		case AuthPolicySSOPrimary:
			// SSO required - check if admin fallback is allowed
			if user.IsSystemAdmin && h.adminRateLimiter != nil {
				// Admin using fallback - check rate limits (fallback enabled)
				allowed, _, lockedUntil := h.adminRateLimiter.IsAllowed(r.Context(), user.ID, ipAddress)
				if !allowed {
					var msg string
					if lockedUntil != nil {
						msg = fmt.Sprintf("Admin fallback rate limit exceeded. Try again after %s", lockedUntil.Format(time.RFC3339))
					} else {
						msg = "Admin fallback rate limit exceeded. Try again later."
					}
					respondTooManyRequests(w, r, msg)
					return
				}
				_ = h.adminRateLimiter.RecordAttempt(r.Context(), user.ID, ipAddress)
				_ = h.authPolicyHandler.LogAuditEvent(user.ID, "admin_fallback_used", ipAddress, r.UserAgent(), map[string]interface{}{
					"policy": string(policy),
				})
			} else {
				// Either not admin OR fallback is disabled - must use SSO
				respondJSON(w, http.StatusForbidden, LoginResponse{
					Success:       false,
					SSORequired:   true,
					PolicyMessage: "Password login is disabled. Please use SSO to sign in.",
				})
				return
			}

		case AuthPolicyPasskeyOnly, AuthPolicyPasswordPasskey2FA:
			// Check if user has passkey enrolled
			hasPasskey := h.userHasPasskey(user.ID)

			if user.IsSystemAdmin && h.adminRateLimiter != nil {
				// Admin with fallback enabled - allow password with rate limiting
				allowed, _, lockedUntil := h.adminRateLimiter.IsAllowed(r.Context(), user.ID, ipAddress)
				if !allowed {
					var msg string
					if lockedUntil != nil {
						msg = fmt.Sprintf("Admin fallback rate limit exceeded. Try again after %s", lockedUntil.Format(time.RFC3339))
					} else {
						msg = "Admin fallback rate limit exceeded. Try again later."
					}
					respondTooManyRequests(w, r, msg)
					return
				}
				if !hasPasskey {
					_ = h.adminRateLimiter.RecordAttempt(r.Context(), user.ID, ipAddress)
					_ = h.authPolicyHandler.LogAuditEvent(user.ID, "admin_fallback_used", ipAddress, r.UserAgent(), map[string]interface{}{
						"policy": string(policy),
					})
				}
			} else if !hasPasskey {
				// Non-admin without passkey (or admin with fallback disabled) needs to enroll
				enrollmentRequired = true
				_ = h.authPolicyHandler.LogAuditEvent(user.ID, "enrollment_started", ipAddress, r.UserAgent(), map[string]interface{}{
					"policy": string(policy),
				})
			}
			// If user has passkey and policy is password_passkey_2fa, they still need to verify with passkey
			// But for now, we allow password login and mark enrollment_required for the frontend to handle
		}
	}

	// Create session with enrollment flag if needed
	session, err := h.sessionManager.CreateSession(user.ID, ipAddress, r.UserAgent(), req.RememberMe)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Mark session as requiring enrollment if needed
	if enrollmentRequired {
		if err := h.sessionManager.SetEnrollmentRequired(session.ID, true); err != nil {
			slog.Warn("failed to set enrollment required", slog.String("component", "auth"), slog.Any("error", err))
		}
	}

	// Set session cookie
	if err := h.sessionManager.SetSessionCookie(w, r, session.Token, req.RememberMe); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Update user's full name for response
	user.FullName = fmt.Sprintf("%s %s", user.FirstName, user.LastName)
	user.PasswordHash = "" // Never send password hash

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionLoginSuccess,
		ResourceType: logger.ResourceUser,
		ResourceID:   &user.ID,
		ResourceName: user.Username,
		Success:      true,
	})

	response := LoginResponse{
		Success:            true,
		User:               user,
		Message:            "Login successful",
		EnrollmentRequired: enrollmentRequired,
	}

	if enrollmentRequired {
		response.PolicyMessage = "Please enroll a passkey to complete your account setup."
	}

	respondJSONOK(w, response)
}

// Logout handles user logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Get session token
	token, err := h.sessionManager.GetSessionFromRequest(r)
	if err != nil {
		// Even if no session found, clear cookie and return success
		h.sessionManager.ClearSessionCookie(w, r)
		respondJSONOK(w, map[string]interface{}{
			"success": true,
			"message": "Logout successful",
		})
		return
	}

	// Invalidate session
	if err := h.sessionManager.DeleteSession(token); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Clear session cookie
	h.sessionManager.ClearSessionCookie(w, r)

	session, _ := r.Context().Value(middleware.ContextKeySession).(*auth.Session)
	if session != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       session.UserID,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionLogout,
			ResourceType: logger.ResourceUser,
			Success:      true,
		})
	}

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"message": "Logout successful",
	})
}

// GetCurrentUser returns information about the currently authenticated user
func (h *AuthHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	// Get session token from request (since auth middleware skips /api/auth/ paths)
	token, err := h.sessionManager.GetSessionFromRequest(r)
	if err != nil {
		respondUnauthorized(w, r)
		return
	}

	// Get client IP for validation
	clientIP := h.getClientIP(r)

	// Validate session
	session, err := h.sessionManager.ValidateSession(token, clientIP)
	if err != nil {
		respondUnauthorized(w, r)
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

	respondJSONOK(w, response)
}

// RefreshSession extends the current session
func (h *AuthHandler) RefreshSession(w http.ResponseWriter, r *http.Request) {
	// Get session token
	token, err := h.sessionManager.GetSessionFromRequest(r)
	if err != nil {
		respondUnauthorized(w, r)
		return
	}

	// Parse request body for remember me option (body is optional)
	type refreshSessionReq struct {
		RememberMe bool `json:"remember_me"`
	}
	req, ok := decodeOptionalJSON[refreshSessionReq](w, r)
	if !ok {
		return
	}

	// Refresh session
	if err := h.sessionManager.RefreshSession(token, req.RememberMe); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Update cookie with new expiration
	if err := h.sessionManager.SetSessionCookie(w, r, token, req.RememberMe); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"message": "Session refreshed",
	})
}

// LogoutAll invalidates all sessions for the current user
func (h *AuthHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	// Get session from context
	session, ok := r.Context().Value(middleware.ContextKeySession).(*auth.Session)
	if !ok || session == nil {
		respondUnauthorized(w, r)
		return
	}

	// Invalidate all user sessions
	if err := h.sessionManager.DeleteAllUserSessions(session.UserID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Clear current session cookie
	h.sessionManager.ClearSessionCookie(w, r)

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"message": "All sessions logged out",
	})
}

// logFailedLogin records an audit log entry for a failed login attempt.
//
// The attempted identifier is logged as a truncated SHA-256 hash rather
// than cleartext. This prevents an audit-log reader from harvesting the
// usernames/emails that have been tried (real or guessed), while still
// letting them correlate retries against the same identifier across rows.
// Hash length is short enough (16 hex chars / 64 bits) to avoid bloating
// audit details but long enough to make collisions effectively impossible
// for the volume of failed logins a single deployment sees.
func (h *AuthHandler) logFailedLogin(r *http.Request, emailOrUsername string) {
	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       0,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionLoginFailure,
		ResourceType: logger.ResourceUser,
		ResourceName: hashIdentifier(emailOrUsername),
		Success:      false,
		ErrorMessage: "invalid credentials",
	})
}

// hashIdentifier returns a stable, non-reversible tag for an attempted
// login identifier. Lowercased to make username + email-with-different-
// casing collide so retries cluster correctly.
func hashIdentifier(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.ToLower(s)))
	return "sha256:" + hex.EncodeToString(sum[:8])
}

// findUserByEmailOrUsername finds a user by email or username.
func (h *AuthHandler) findUserByEmailOrUsername(emailOrUsername string) (*models.User, error) {
	return h.userRepo.GetByEmailOrUsernameForAuth(emailOrUsername)
}

// getClientIP extracts the client IP with proxy validation
func (h *AuthHandler) getClientIP(r *http.Request) string {
	return h.ipExtractor.GetClientIP(r)
}

// userHasPasskey checks if a user has an active FIDO/passkey credential
func (h *AuthHandler) userHasPasskey(userID int) bool {
	hasPasskey, err := h.credentialRepo.HasActiveFIDO(userID)
	return err == nil && hasPasskey
}

// ChangePassword allows authenticated users to change their password
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	// Get session from context
	session, ok := r.Context().Value(middleware.ContextKeySession).(*auth.Session)
	if !ok || session == nil {
		respondUnauthorized(w, r)
		return
	}

	req, ok := decodeJSON[ChangePasswordRequest](w, r)
	if !ok {
		return
	}

	// Validate input using validator
	if err := utils.Validate(req); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Get current user data to verify current password
	user, err := h.findUserByEmailOrUsername(session.User.Email)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Verify current password
	if err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		respondUnauthorized(w, r)
		return
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Update password in database
	if err = h.userRepo.SetPassword(session.UserID, string(hashedPassword), false); err != nil {
		respondInternalError(w, r, err)
		return
	}

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       session.UserID,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionPasswordChange,
		ResourceType: logger.ResourceUser,
		Success:      true,
	})

	// Optionally logout all other sessions
	if req.LogoutAll {
		// Delete all sessions for this user
		_ = h.sessionManager.DeleteAllUserSessions(session.UserID)

		// Recreate current session
		newSession, err := h.sessionManager.CreateSession(
			session.UserID,
			h.getClientIP(r),
			r.UserAgent(),
			false, // Don't assume remember me for security
		)
		if err == nil {
			_ = h.sessionManager.SetSessionCookie(w, r, newSession.Token, false)
		}
	}

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"message": "Password changed successfully",
	})
}

// VerifyEmail handles email verification via token
func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		respondBadRequest(w, r, "Verification token is required")
		return
	}

	if h.emailVerificationService == nil {
		respondServiceUnavailable(w, r, "Email verification is not configured")
		return
	}

	user, err := h.emailVerificationService.VerifyEmail(token)
	if err != nil {
		switch err {
		case services.ErrTokenExpired:
			respondGone(w, r, "Verification link has expired. Please request a new one.")
		case services.ErrTokenInvalid:
			respondBadRequest(w, r, "Invalid verification link")
		case services.ErrAlreadyVerified:
			// Not an error - just let them know
			respondJSONOK(w, map[string]interface{}{
				"success": true,
				"message": "Email is already verified",
			})
			return
		default:
			slog.Error("email verification error", slog.String("component", "auth"), slog.Any("error", err))
			respondInternalError(w, r, err)
		}
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"success":  true,
		"message":  "Email verified successfully",
		"user_id":  user.ID,
		"verified": true,
	})
}

// ResendVerification resends the verification email to the current user
func (h *AuthHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	// Get session from context
	session, ok := r.Context().Value(middleware.ContextKeySession).(*auth.Session)
	if !ok || session == nil {
		respondUnauthorized(w, r)
		return
	}

	if h.emailVerificationService == nil {
		respondServiceUnavailable(w, r, "Email verification is not configured")
		return
	}

	err := h.emailVerificationService.ResendVerification(session.UserID)
	if err != nil {
		switch err {
		case services.ErrAlreadyVerified:
			respondJSONOK(w, map[string]interface{}{
				"success": true,
				"message": "Email is already verified",
			})
			return
		case services.ErrUserNotFound:
			// Session exists but user was deleted - return generic success to prevent enumeration
			slog.Warn("resend verification for non-existent user",
				slog.String("component", "auth"),
				slog.Int("session_user_id", session.UserID))
			respondJSONOK(w, map[string]interface{}{
				"success": true,
				"message": "If your account exists, a verification email will be sent",
			})
		case services.ErrSMTPNotConfigured:
			respondServiceUnavailable(w, r, "Email service is not available")
		default:
			slog.Error("failed to resend verification", slog.String("component", "auth"), slog.Any("error", err))
			respondInternalError(w, r, err)
		}
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"message": "Verification email sent",
	})
}

// GetVerificationStatus returns the email verification status for the current user
func (h *AuthHandler) GetVerificationStatus(w http.ResponseWriter, r *http.Request) {
	// Get session from context
	session, ok := r.Context().Value(middleware.ContextKeySession).(*auth.Session)
	if !ok || session == nil {
		respondUnauthorized(w, r)
		return
	}

	if h.emailVerificationService == nil {
		// If email verification service is not configured, assume verified
		respondJSONOK(w, map[string]interface{}{
			"email_verified": true,
			"configured":     false,
		})
		return
	}

	verified, err := h.emailVerificationService.IsEmailVerified(session.UserID)
	if err != nil {
		slog.Error("failed to check verification status", slog.String("component", "auth"), slog.Any("error", err))
		// Return verified=true on error for graceful degradation
		respondJSONOK(w, map[string]interface{}{
			"email_verified": true,
			"configured":     false,
		})
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"email_verified": verified,
		"configured":     true,
	})
}
