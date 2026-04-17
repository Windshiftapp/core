package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
	"windshift/internal/webauthn"
)

const (
	// MaxCredentialsPerUser caps how many WebAuthn credentials a single user
	// may register. Prevents unbounded growth of the exclusion list (and
	// the credentials table) from a compromised session.
	maxCredentialsPerUser = 10

	// maxCredentialNameLen bounds user-supplied credential names (UTF-8 runes).
	maxCredentialNameLen = 100
)

type WebAuthnHandler struct {
	db                database.Database
	permissionService *services.PermissionService
	sessionManager    *auth.SessionManager
	config            *webauthn.Config
	sessionStore      *webauthn.SessionStore
	credentialStore   *webauthn.CredentialStore
	ipExtractor       *utils.IPExtractor
}

// NewWebAuthnHandler creates a new WebAuthn handler
func NewWebAuthnHandler(db database.Database, permissionService *services.PermissionService, sessionManager *auth.SessionManager, config *webauthn.Config, ipExtractor *utils.IPExtractor) *WebAuthnHandler {
	return &WebAuthnHandler{
		db:                db,
		permissionService: permissionService,
		sessionManager:    sessionManager,
		config:            config,
		sessionStore:      webauthn.NewSessionStore(db),
		credentialStore:   webauthn.NewCredentialStore(db),
		ipExtractor:       ipExtractor,
	}
}

// FIDORegistrationRequestNew represents the request to start FIDO registration
type FIDORegistrationRequestNew struct {
	CredentialName string `json:"credential_name"`
}

// FIDOCompleteRegistrationRequest represents the completion request
type FIDOCompleteRegistrationRequest struct {
	SessionID      string      `json:"sessionId"`
	CredentialName string      `json:"credentialName"`
	Response       interface{} `json:"response"`
}

// StartFIDORegistrationNew initiates FIDO2/WebAuthn registration with proper verification
func (h *WebAuthnHandler) StartFIDORegistrationNew(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	var err error

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	req, ok := decodeJSON[FIDORegistrationRequestNew](w, r)
	if !ok {
		return
	}

	trimmedName := strings.TrimSpace(req.CredentialName)
	if trimmedName == "" {
		respondValidationError(w, r, "Credential name is required")
		return
	}
	if utf8.RuneCountInString(trimmedName) > maxCredentialNameLen {
		respondValidationError(w, r, "Credential name is too long")
		return
	}

	// Get user information
	var user models.User
	var avatarURL sql.NullString
	err = h.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, avatar_url
		FROM users WHERE id = ?
	`, userID).Scan(&user.ID, &user.Email, &user.Username, &user.FirstName, &user.LastName, &avatarURL)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "user")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Handle NULL avatar_url
	if avatarURL.Valid {
		user.AvatarURL = avatarURL.String
	} else {
		user.AvatarURL = ""
	}

	// Create WebAuthn user wrapper
	webAuthnUser := webauthn.NewUser(&user)

	// Get existing credentials to exclude duplicates
	existingCreds, err := h.credentialStore.GetUserCredentials(userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if len(existingCreds) >= maxCredentialsPerUser {
		respondValidationError(w, r, "Maximum number of WebAuthn credentials reached")
		return
	}
	webAuthnUser.SetCredentials(existingCreds)

	// Begin registration with go-webauthn
	options, sessionData, err := h.config.WebAuthn().BeginRegistration(webAuthnUser)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Save session data
	sessionID, err := h.sessionStore.SaveRegistrationSession(userID, sessionData)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Send response - options already contains the publicKey structure
	// We need to extract just the publicKey content from the CredentialCreation
	response := map[string]interface{}{
		"publicKey": options.Response,
		"sessionId": sessionID,
	}

	respondJSONOK(w, response)
}

// CompleteFIDORegistrationNew completes FIDO2/WebAuthn registration with proper verification
func (h *WebAuthnHandler) CompleteFIDORegistrationNew(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	var err error

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	req, ok := decodeJSON[FIDOCompleteRegistrationRequest](w, r)
	if !ok {
		return
	}

	trimmedName := strings.TrimSpace(req.CredentialName)
	if trimmedName == "" {
		respondValidationError(w, r, "Credential name is required")
		return
	}
	if utf8.RuneCountInString(trimmedName) > maxCredentialNameLen {
		respondValidationError(w, r, "Credential name is too long")
		return
	}
	req.CredentialName = trimmedName

	// Get user information
	var user models.User
	var avatarURL sql.NullString
	err = h.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, avatar_url
		FROM users WHERE id = ?
	`, userID).Scan(&user.ID, &user.Email, &user.Username, &user.FirstName, &user.LastName, &avatarURL)

	if err != nil {
		respondNotFound(w, r, "user")
		return
	}

	// Handle NULL avatar_url
	if avatarURL.Valid {
		user.AvatarURL = avatarURL.String
	} else {
		user.AvatarURL = ""
	}

	// Create WebAuthn user wrapper
	webAuthnUser := webauthn.NewUser(&user)

	// Get session data — bound to both session_type=registration and user_id
	// so a session issued for one user (or for login) cannot be replayed here.
	sessionData, err := h.sessionStore.GetRegistrationSession(req.SessionID, userID)
	if err != nil {
		respondValidationError(w, r, "Invalid or expired session")
		return
	}

	// Recreate request body with just the credential response for go-webauthn
	// The library expects to read the credential directly from r.Body
	credentialJSON, err := json.Marshal(req.Response)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(credentialJSON))

	// Finish registration with go-webauthn (performs all verification)
	credential, err := h.config.WebAuthn().FinishRegistration(webAuthnUser, *sessionData, r)
	if err != nil {
		respondValidationError(w, r, "Registration verification failed: "+err.Error())
		return
	}

	// Check if credential already exists
	exists, err := h.credentialStore.CheckCredentialExists(credential.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if exists {
		respondConflict(w, r, "Credential already registered")
		return
	}

	// Save credential to database
	err = h.credentialStore.SaveCredential(userID, req.CredentialName, credential)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Clear enrollment required flag for this user's sessions
	// This allows the user to continue without being redirected to enrollment again
	if err := h.sessionManager.ClearEnrollmentRequiredByUserID(userID); err != nil {
		slog.Warn("failed to clear enrollment required flag", slog.String("component", "webauthn"), slog.Int("user_id", userID), slog.Any("error", err))
		// Non-fatal, continue
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionWebAuthnRegister,
			ResourceType: logger.ResourceWebAuthn,
			ResourceID:   &userID,
			ResourceName: req.CredentialName,
			Success:      true,
		})
	}

	response := map[string]interface{}{
		"status":  "success",
		"message": "FIDO credential registered successfully",
		"credential": map[string]interface{}{
			"id":              credential.ID,
			"name":            req.CredentialName,
			"attestationType": credential.AttestationType,
			"transport":       credential.Transport,
		},
	}

	respondJSONCreated(w, response)
}

// GetWebAuthnCredentials returns all WebAuthn credentials for a user
func (h *WebAuthnHandler) GetWebAuthnCredentials(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	// Get credentials list (without sensitive data)
	credentials, err := h.credentialStore.GetUserCredentialsList(userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if credentials == nil {
		credentials = []webauthn.WebAuthnCredential{}
	}

	respondJSONOK(w, credentials)
}

// RemoveWebAuthnCredential removes a specific WebAuthn credential
func (h *WebAuthnHandler) RemoveWebAuthnCredential(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	var err error

	credentialID := r.PathValue("credentialId")
	if credentialID == "" {
		respondValidationError(w, r, "Credential ID is required")
		return
	}

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	// Verify the credential belongs to the user
	var ownerID int
	err = h.db.QueryRow(`
		SELECT user_id FROM webauthn_credentials WHERE id = ?
	`, credentialID).Scan(&ownerID)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "credential")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if ownerID != userID {
		respondForbidden(w, r)
		return
	}

	// Delete the credential
	err = h.credentialStore.DeleteCredential(credentialID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionWebAuthnRemove,
			ResourceType: logger.ResourceWebAuthn,
			ResourceID:   &userID,
			ResourceName: credentialID,
			Success:      true,
		})
	}

	response := map[string]string{
		"status":  "success",
		"message": "Credential deleted successfully",
	}

	respondJSONOK(w, response)
}

// FIDOLoginRequestNew represents the request to start FIDO login
type FIDOLoginRequestNew struct {
	EmailOrUsername string `json:"email_or_username"`
}

// FIDOCompleteLoginRequest represents the login completion request
type FIDOCompleteLoginRequest struct {
	SessionID string      `json:"sessionId"`
	Response  interface{} `json:"response"`
}

// StartFIDOLoginNew initiates FIDO authentication with proper verification
func (h *WebAuthnHandler) StartFIDOLoginNew(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[FIDOLoginRequestNew](w, r)
	if !ok {
		return
	}

	if strings.TrimSpace(req.EmailOrUsername) == "" {
		respondValidationError(w, r, "Email or username is required")
		return
	}

	// Find user by email or username
	var user models.User
	var avatarURL sql.NullString
	err := h.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url
		FROM users
		WHERE (email = ? OR username = ?) AND is_active = true
	`, req.EmailOrUsername, req.EmailOrUsername).Scan(
		&user.ID, &user.Email, &user.Username, &user.FirstName,
		&user.LastName, &user.IsActive, &avatarURL,
	)

	if err == sql.ErrNoRows {
		// Don't reveal that user doesn't exist
		respondNotFound(w, r, "credential")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Handle NULL avatar_url
	if avatarURL.Valid {
		user.AvatarURL = avatarURL.String
	} else {
		user.AvatarURL = ""
	}

	if !user.IsActive {
		// Return same response as non-existent user to prevent enumeration
		respondNotFound(w, r, "credential")
		return
	}

	// Create WebAuthn user wrapper
	webAuthnUser := webauthn.NewUser(&user)

	// Get user's credentials
	credentials, err := h.credentialStore.GetUserCredentials(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if len(credentials) == 0 {
		respondNotFound(w, r, "credential")
		return
	}

	webAuthnUser.SetCredentials(credentials)

	// Begin authentication with go-webauthn
	options, sessionData, err := h.config.WebAuthn().BeginLogin(webAuthnUser)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Save session data
	sessionID, err := h.sessionStore.SaveAuthenticationSession(&user.ID, sessionData)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Send response - options already contains the publicKey structure
	// We need to extract just the publicKey content from the CredentialAssertion
	response := map[string]interface{}{
		"publicKey": options.Response,
		"sessionId": sessionID,
	}

	respondJSONOK(w, response)
}

// CompleteFIDOLoginNew completes FIDO authentication with proper verification
func (h *WebAuthnHandler) CompleteFIDOLoginNew(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[FIDOCompleteLoginRequest](w, r)
	if !ok {
		return
	}

	// Get session data
	sessionData, err := h.sessionStore.GetAuthenticationSession(req.SessionID)
	if err != nil {
		respondValidationError(w, r, "Invalid or expired session")
		return
	}

	// Get user ID from session
	userIDBytes := sessionData.UserID
	if len(userIDBytes) == 0 {
		respondValidationError(w, r, "Session missing user ID")
		return
	}

	userID, err := strconv.Atoi(string(userIDBytes))
	if err != nil {
		respondValidationError(w, r, "Invalid user ID in session")
		return
	}

	// Get user information
	var user models.User
	var avatarURL sql.NullString
	err = h.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url
		FROM users WHERE id = ? AND is_active = true
	`, userID).Scan(
		&user.ID, &user.Email, &user.Username, &user.FirstName,
		&user.LastName, &user.IsActive, &avatarURL,
	)

	if err != nil {
		respondUnauthorized(w, r)
		return
	}

	// Handle NULL avatar_url
	if avatarURL.Valid {
		user.AvatarURL = avatarURL.String
	} else {
		user.AvatarURL = ""
	}

	// Create WebAuthn user wrapper
	webAuthnUser := webauthn.NewUser(&user)

	// Get user's credentials
	credentials, err := h.credentialStore.GetUserCredentials(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	webAuthnUser.SetCredentials(credentials)

	// Recreate request body with just the credential response for go-webauthn
	// The library expects to read the credential directly from r.Body
	credentialJSON, err := json.Marshal(req.Response)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(credentialJSON))

	// Finish authentication with go-webauthn (performs all verification)
	credential, err := h.config.WebAuthn().FinishLogin(webAuthnUser, *sessionData, r)
	if err != nil {
		respondUnauthorized(w, r)
		return
	}

	// Persist sign count and clone_warning regardless of the outcome below —
	// admins need the flag visible on the credential list even when we reject.
	if err := h.credentialStore.UpdateCredentialCounter(
		credential.ID,
		credential.Authenticator.SignCount,
		credential.Authenticator.CloneWarning,
	); err != nil {
		slog.Warn("failed to update credential counter", slog.String("component", "webauthn"), slog.Any("error", err))
	}

	// Reject authentication when go-webauthn reports a counter regression:
	// the sign count went backwards, which implies a cloned authenticator
	// (WebAuthn L3 §7.2). We log, audit, and refuse to issue a session.
	if credential.Authenticator.CloneWarning {
		slog.Warn("rejecting webauthn login due to clone warning",
			slog.String("component", "webauthn"),
			slog.Int("user_id", user.ID))
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       user.ID,
			Username:     user.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionLoginFailure,
			ResourceType: logger.ResourceWebAuthn,
			ResourceID:   &user.ID,
			Success:      false,
			ErrorMessage: "authenticator clone warning",
		})
		respondUnauthorized(w, r)
		return
	}

	// Populate system admin status before issuing a session so a transient
	// permission-service failure doesn't silently downgrade an admin.
	if err := h.populateIsSystemAdmin(&user); err != nil {
		slog.Warn("failed to populate system admin status", slog.String("component", "webauthn"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	clientIP := h.ipExtractor.GetClientIP(r)
	session, err := h.sessionManager.CreateSession(user.ID, clientIP, r.UserAgent(), false)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if err := h.sessionManager.SetSessionCookie(w, r, session.Token, false); err != nil {
		if delErr := h.sessionManager.DeleteSession(session.Token); delErr != nil {
			slog.Warn("failed to revoke session after cookie error", slog.Any("error", delErr))
		}
		respondInternalError(w, r, err)
		return
	}

	response := map[string]interface{}{
		"status":  "success",
		"message": "Authentication successful",
		"user":    user,
	}

	respondJSONOK(w, response)
}

// Helper functions

func (h *WebAuthnHandler) populateIsSystemAdmin(user *models.User) error {
	isAdmin, err := h.permissionService.IsSystemAdmin(user.ID)
	if err != nil {
		return err
	}
	user.IsSystemAdmin = isAdmin
	return nil
}
