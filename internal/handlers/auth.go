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
	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"

	"golang.org/x/crypto/bcrypt"
)

// dummyPasswordHash is a valid, pre-computed cost-10 bcrypt hash used to make
// unknown and otherwise non-login-capable accounts perform the same expensive
// password work as an ordinary wrong-password attempt.
var dummyPasswordHash = []byte("$2a$10$MdJYV4jL9BzSg9gDHWv9BOtYUoXp7wV5pc2UffwmgikvnYq4FvzwC")

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	userRepo                 *repository.UserRepository
	credentialRepo           *repository.CredentialRepository
	auditor                  *logger.Auditor
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
	PasskeyRequired    bool         `json:"passkey_required,omitempty"`
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
	ExpiresAt          time.Time `json:"expires_at"`
	IPAddress          string    `json:"ip_address"`
	CreatedAt          time.Time `json:"created_at"`
	EnrollmentRequired bool      `json:"enrollment_required"`
	AuthPendingType    string    `json:"auth_pending_type,omitempty"`
}

// NewAuthHandler creates a new authentication handler
// emailVerificationService can be nil if SMTP is not configured
// authPolicyHandler and adminRateLimiter can be nil for backwards compatibility
func NewAuthHandler(userRepo *repository.UserRepository, credentialRepo *repository.CredentialRepository, auditor *logger.Auditor, sessionManager *auth.SessionManager, rateLimiter *middleware.RateLimiter, permissionService *services.PermissionService, emailVerificationService *services.EmailVerificationService, ipExtractor *utils.IPExtractor, authPolicyHandler *AuthPolicyHandler, adminRateLimiter *middleware.AdminFallbackRateLimiter) *AuthHandler {
	return &AuthHandler{
		userRepo:                 userRepo,
		credentialRepo:           credentialRepo,
		auditor:                  auditor,
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

	// Validate credentials before applying the shared login rate limits.
	if err := utils.Validate(req); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	ipAddress := h.getClientIP(r)

	if locked, duration := h.rateLimiter.IsLockedOut(ipAddress); locked {
		respondTooManyRequests(w, r, fmt.Sprintf("Too many failed login attempts. Please try again in %s", middleware.FormatLockoutDuration(duration)))
		return
	}

	// bcrypt truncates input at 72 bytes, so an over-long password would match a
	// stored 72-byte password on its first 72 bytes. Reject it as invalid
	// credentials, with the same rate-limit and audit accounting as a mismatch,
	// instead of comparing against the stored hash.
	if err := auth.ValidatePasswordLength(req.Password); err != nil {
		h.rejectPasswordLogin(w, r, req.EmailOrUsername, req.Password, "")
		return
	}

	user, err := h.findUserByEmailOrUsername(req.EmailOrUsername)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.rejectPasswordLogin(w, r, req.EmailOrUsername, req.Password, "")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Inactive and agent users are externally indistinguishable from an unknown
	// account. They still incur bcrypt work and failed-attempt accounting.
	if !user.IsActive || user.IsAgent {
		h.rejectPasswordLogin(w, r, req.EmailOrUsername, req.Password, user.PasswordHash)
		return
	}

	if err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		h.rateLimiter.RecordFailedLogin(ipAddress)
		h.logFailedLogin(r, req.EmailOrUsername)
		respondUnauthorized(w, r)
		return
	}

	h.rateLimiter.RecordSuccessfulLogin(ipAddress)

	if err = h.populateIsSystemAdmin(user); err != nil {
		slog.Warn("failed to populate system admin status", slog.String("component", "auth"), slog.Any("error", err))
	}

	var enrollmentRequired, passkeyRequired bool
	if h.authPolicyHandler != nil && !h.authPolicyHandler.IsPreviewMode() {
		policy := h.authPolicyHandler.GetCurrentPolicy()

		switch policy {
		case AuthPolicySSOPrimary:
			// SSO required - check if admin fallback is allowed
			switch {
			case user.IsSystemAdmin && h.adminRateLimiter != nil:
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
				_ = h.authPolicyHandler.LogAuditEvent(user.ID, "admin_fallback_used", ipAddress, r.UserAgent(), map[string]any{
					"policy": string(policy),
				})
			default:
				// Either not admin OR fallback is disabled - must use SSO
				respondJSON(w, http.StatusForbidden, LoginResponse{
					Success:       false,
					SSORequired:   true,
					PolicyMessage: "Password login is disabled. Please use SSO to sign in.",
				})
				return
			}

		case AuthPolicyPasskeyOnly, AuthPolicyPasswordPasskey2FA:
			hasPasskey := h.userHasPasskey(user.ID)

			switch {
			case user.IsSystemAdmin && h.adminRateLimiter != nil:
				// Explicitly enabled administrator fallback preserves emergency
				// password access, but every use is rate-limited and audited.
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
				_ = h.authPolicyHandler.LogAuditEvent(user.ID, "admin_fallback_used", ipAddress, r.UserAgent(), map[string]any{
					"policy": string(policy),
				})
			case !hasPasskey:
				// Password use is limited to a server-restricted enrollment
				// session. Middleware denies every unrelated protected route.
				enrollmentRequired = true
				_ = h.authPolicyHandler.LogAuditEvent(user.ID, "enrollment_started", ipAddress, r.UserAgent(), map[string]any{
					"policy": string(policy),
				})
			case policy == AuthPolicyPasskeyOnly:
				respondJSON(w, http.StatusForbidden, LoginResponse{
					Success:         false,
					PasskeyRequired: true,
					PolicyMessage:   "Password login is disabled. Please use a passkey to sign in.",
				})
				return
			default:
				// The password is valid, but this session remains restricted until
				// a fresh WebAuthn assertion for the same user succeeds.
				passkeyRequired = true
			}
		}
	}

	session, err := h.sessionManager.CreateSession(user.ID, ipAddress, r.UserAgent(), req.RememberMe)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Enrollment and password+passkey sessions are capabilities, not normal
	// authenticated sessions. Mark them restricted before exposing the cookie.
	if enrollmentRequired || passkeyRequired {
		pendingType := auth.AuthPendingEnrollment
		if passkeyRequired {
			pendingType = auth.AuthPendingPasskeyVerification
		}
		if err := h.sessionManager.SetAuthPending(session.ID, pendingType); err != nil {
			_ = h.sessionManager.DeleteSession(session.Token)
			respondInternalError(w, r, err)
			return
		}
		session.EnrollmentRequired = true
		session.AuthPendingType = pendingType
	}

	// Password login always establishes a browser session via cookie. Do not
	// return session tokens in JSON or treat them as REST API bearer tokens; the
	// v1 API accepts only scoped crw_* API tokens.
	if err := h.sessionManager.SetSessionCookie(w, r, session.Token, req.RememberMe); err != nil {
		respondInternalError(w, r, err)
		return
	}

	user.FullName = fmt.Sprintf("%s %s", user.FirstName, user.LastName)
	user.PasswordHash = "" // Never send password hash

	response := LoginResponse{
		Success:            !passkeyRequired,
		User:               user,
		Message:            "Login successful",
		EnrollmentRequired: enrollmentRequired,
		PasskeyRequired:    passkeyRequired,
	}

	switch {
	case enrollmentRequired:
		response.Message = "Passkey enrollment required"
		response.PolicyMessage = "Please enroll a passkey to complete authentication."
	case passkeyRequired:
		response.Message = "Passkey verification required"
		response.PolicyMessage = "Verify with your passkey to complete authentication."
	default:
		h.auditor.Log(r, user, logger.ActionLoginSuccess, logger.ResourceUser, &user.ID, user.Username)
	}

	respondJSONOK(w, response)
}

// Logout handles user logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	token, err := h.sessionManager.GetSessionFromRequest(r)
	if err != nil {
		h.sessionManager.ClearSessionCookie(w, r)
		respondJSONOK(w, map[string]any{
			"success": true,
			"message": "Logout successful",
		})
		return
	}

	if err := h.sessionManager.DeleteSession(token); err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.sessionManager.ClearSessionCookie(w, r)

	session, _ := r.Context().Value(middleware.ContextKeySession).(*auth.Session)
	if session != nil {
		h.auditor.Log(r, &models.User{ID: session.UserID}, logger.ActionLogout, logger.ResourceUser, nil, "")
	}

	respondJSONOK(w, map[string]any{
		"success": true,
		"message": "Logout successful",
	})
}

// GetCurrentUser returns information about the currently authenticated user
func (h *AuthHandler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	token, err := h.sessionManager.GetSessionFromRequest(r)
	if err != nil {
		respondUnauthorized(w, r)
		return
	}

	clientIP := h.getClientIP(r)

	session, err := h.sessionManager.ValidateSessionContext(r.Context(), token, clientIP)
	if err != nil {
		respondUnauthorized(w, r)
		return
	}

	if err := h.populateIsSystemAdmin(session.User); err != nil {
		slog.Warn("failed to populate system admin status", slog.String("component", "auth"), slog.Any("error", err))
		// Continue anyway - user info will be returned, just without admin flag
	}

	sessionInfo := &SessionInfo{
		ExpiresAt:          session.ExpiresAt,
		IPAddress:          session.IPAddress,
		CreatedAt:          session.CreatedAt,
		EnrollmentRequired: session.EnrollmentRequired,
		AuthPendingType:    session.AuthPendingType,
	}

	response := UserResponse{
		User:    session.User,
		Session: sessionInfo,
	}

	respondJSONOK(w, response)
}

// RefreshSession extends the current session
func (h *AuthHandler) RefreshSession(w http.ResponseWriter, r *http.Request) {
	token, err := h.sessionManager.GetSessionFromRequest(r)
	if err != nil {
		respondUnauthorized(w, r)
		return
	}
	if _, err := h.sessionManager.ValidateSessionContext(r.Context(), token, h.getClientIP(r)); err != nil {
		h.sessionManager.ClearSessionCookie(w, r)
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

	if err := h.sessionManager.RefreshSession(token, req.RememberMe); err != nil {
		if errors.Is(err, auth.ErrSessionExpired) || errors.Is(err, auth.ErrInvalidSession) || errors.Is(err, auth.ErrSessionNotFound) {
			h.sessionManager.ClearSessionCookie(w, r)
			respondUnauthorized(w, r)
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if err := h.sessionManager.SetSessionCookie(w, r, token, req.RememberMe); err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, map[string]any{
		"success": true,
		"message": "Session refreshed",
	})
}

// LogoutAll invalidates all sessions for the current user
func (h *AuthHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	session, ok := r.Context().Value(middleware.ContextKeySession).(*auth.Session)
	if !ok || session == nil {
		respondUnauthorized(w, r)
		return
	}

	if err := h.sessionManager.DeleteAllUserSessions(session.UserID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.sessionManager.ClearSessionCookie(w, r)

	respondJSONOK(w, map[string]any{
		"success": true,
		"message": "All sessions logged out",
	})
}

// rejectPasswordLogin applies identical expensive password work, rate-limit
// accounting, audit shape, and public response to unknown, inactive, agent, and
// malformed-password-hash accounts.
func (h *AuthHandler) rejectPasswordLogin(w http.ResponseWriter, r *http.Request, identifier, password, storedHash string) {
	h.rateLimiter.RecordFailedLogin(h.getClientIP(r))

	hash := dummyPasswordHash
	if candidate := []byte(storedHash); len(candidate) > 0 {
		if _, err := bcrypt.Cost(candidate); err == nil {
			hash = candidate
		}
	}
	_ = bcrypt.CompareHashAndPassword(hash, []byte(password))
	h.logFailedLogin(r, identifier)
	respondUnauthorized(w, r)
}

// logFailedLogin records a truncated SHA-256 identifier tag, preserving retry
// correlation without exposing attempted usernames or emails in audit logs.
func (h *AuthHandler) logFailedLogin(r *http.Request, emailOrUsername string) {
	h.auditor.LogFailure(r, nil, logger.ActionLoginFailure, logger.ResourceUser, nil, hashIdentifier(emailOrUsername), "invalid credentials", nil)
}

// hashIdentifier returns a stable case-insensitive, non-reversible audit tag.
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
	session, ok := r.Context().Value(middleware.ContextKeySession).(*auth.Session)
	if !ok || session == nil {
		respondUnauthorized(w, r)
		return
	}

	req, ok := decodeJSON[ChangePasswordRequest](w, r)
	if !ok {
		return
	}

	if err := utils.Validate(req); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	if err := auth.ValidatePasswordLength(req.NewPassword); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	user, err := h.findUserByEmailOrUsername(session.User.Email)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// The current-password comparison truncates at 72 bytes too; treat over-long
	// input as an incorrect password rather than matching a truncated prefix.
	if err := auth.ValidatePasswordLength(req.CurrentPassword); err != nil {
		respondUnauthorized(w, r)
		return
	}

	if err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		respondUnauthorized(w, r)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if err = h.userRepo.SetPassword(session.UserID, string(hashedPassword), false); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.sessionManager.InvalidateUserSessionValidation(session.UserID)

	h.auditor.Log(r, &models.User{ID: session.UserID}, logger.ActionPasswordChange, logger.ResourceUser, nil, "")

	if req.LogoutAll {
		_ = h.sessionManager.DeleteAllUserSessions(session.UserID)

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

	respondJSONOK(w, map[string]any{
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
			respondJSONOK(w, map[string]any{
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
	h.sessionManager.InvalidateUserSessionValidation(user.ID)

	respondJSONOK(w, map[string]any{
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
			respondJSONOK(w, map[string]any{
				"success": true,
				"message": "Email is already verified",
			})
			return
		case services.ErrUserNotFound:
			// Session exists but user was deleted - return generic success to prevent enumeration
			slog.Warn("resend verification for non-existent user",
				slog.String("component", "auth"),
				slog.Int("session_user_id", session.UserID))
			respondJSONOK(w, map[string]any{
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

	respondJSONOK(w, map[string]any{
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
		respondJSONOK(w, map[string]any{
			"email_verified": true,
			"configured":     false,
		})
		return
	}

	verified, err := h.emailVerificationService.IsEmailVerified(session.UserID)
	if err != nil {
		slog.Error("failed to check verification status", slog.String("component", "auth"), slog.Any("error", err))
		// Return verified=true on error for graceful degradation
		respondJSONOK(w, map[string]any{
			"email_verified": true,
			"configured":     false,
		})
		return
	}

	respondJSONOK(w, map[string]any{
		"email_verified": verified,
		"configured":     true,
	})
}
