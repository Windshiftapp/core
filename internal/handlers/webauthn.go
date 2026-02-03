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
	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
	"windshift/internal/webauthn"
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
	userID, err := strconv.Atoi(r.PathValue("userId"))
	if err != nil {
		respondInvalidID(w, r, "userId")
		return
	}

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	var req FIDORegistrationRequestNew
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if strings.TrimSpace(req.CredentialName) == "" {
		respondValidationError(w, r, "Credential name is required")
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// CompleteFIDORegistrationNew completes FIDO2/WebAuthn registration with proper verification
func (h *WebAuthnHandler) CompleteFIDORegistrationNew(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.Atoi(r.PathValue("userId"))
	if err != nil {
		respondInvalidID(w, r, "userId")
		return
	}

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	var req FIDOCompleteRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

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

	// Get session data
	sessionData, err := h.sessionStore.GetRegistrationSession(req.SessionID)
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

	response := map[string]interface{}{
		"status":  "success",
		"message": "FIDO credential registered successfully",
		"credential": map[string]interface{}{
			"id":             credential.ID,
			"name":           req.CredentialName,
			"attestationType": credential.AttestationType,
			"transport":      credential.Transport,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// GetWebAuthnCredentials returns all WebAuthn credentials for a user
func (h *WebAuthnHandler) GetWebAuthnCredentials(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.Atoi(r.PathValue("userId"))
	if err != nil {
		respondInvalidID(w, r, "userId")
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(credentials)
}

// RemoveWebAuthnCredential removes a specific WebAuthn credential
func (h *WebAuthnHandler) RemoveWebAuthnCredential(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.Atoi(r.PathValue("userId"))
	if err != nil {
		respondInvalidID(w, r, "userId")
		return
	}

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

	response := map[string]string{
		"status":  "success",
		"message": "Credential deleted successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
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
	var req FIDOLoginRequestNew
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
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
		respondUnauthorized(w, r)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// CompleteFIDOLoginNew completes FIDO authentication with proper verification
func (h *WebAuthnHandler) CompleteFIDOLoginNew(w http.ResponseWriter, r *http.Request) {
	var req FIDOCompleteLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
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

	// Update credential counter and last used
	err = h.credentialStore.UpdateCredentialCounter(
		credential.ID,
		credential.Authenticator.SignCount,
		credential.Authenticator.CloneWarning,
	)
	if err != nil {
		// Log but don't fail authentication
		// This is a non-critical error
		slog.Warn("failed to update credential counter", slog.String("component", "webauthn"), slog.Any("error", err))
	}

	// Create session
	clientIP := h.ipExtractor.GetClientIP(r)
	session, err := h.sessionManager.CreateSession(user.ID, clientIP, r.UserAgent(), false)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Set session cookie
	if err := h.sessionManager.SetSessionCookie(w, r, session.Token, false); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Populate system admin status (cached for frontend)
	if err := h.populateIsSystemAdmin(&user); err != nil {
		slog.Warn("failed to populate system admin status", slog.String("component", "webauthn"), slog.Any("error", err))
		// Continue anyway - user can still login, just without admin flag
	}

	response := map[string]interface{}{
		"status":  "success",
		"message": "Authentication successful",
		"user":    user,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
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

