package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/models"

	"golang.org/x/crypto/bcrypt"
)

// AuthMiddleware interface to avoid circular imports
type AuthMiddleware interface {
	MarkSetupCompleted()
}

// SessionCreator interface for session management (allows mocking in tests)
type SessionCreator interface {
	CreateSession(userID int, clientIP, userAgent string, rememberMe bool) (*auth.Session, error)
	SetSessionCookie(w http.ResponseWriter, r *http.Request, token string, rememberMe bool) error
}

type SetupHandler struct {
	DB             database.Database
	SessionManager SessionCreator
	AuthMiddleware AuthMiddleware
}

func NewSetupHandler(db database.Database, sessionManager SessionCreator, authMiddleware AuthMiddleware) *SetupHandler {
	return &SetupHandler{
		DB:             db,
		SessionManager: sessionManager,
		AuthMiddleware: authMiddleware,
	}
}

// GetSetupStatus returns the current setup status
func (h *SetupHandler) GetSetupStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.getSetupStatus()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

// CompleteInitialSetup handles the initial setup process
func (h *SetupHandler) CompleteInitialSetup(w http.ResponseWriter, r *http.Request) {
	var req models.SetupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate the setup request
	if err := h.validateSetupRequest(req); err != nil {
		respondValidationError(w, r, fmt.Sprintf("Invalid setup request: %v", err))
		return
	}

	// Check if setup is already completed
	setupCompleted, err := h.getSettingBool("setup_completed")
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if setupCompleted {
		respondBadRequest(w, r, "Setup has already been completed")
		return
	}

	// Begin transaction for atomic setup
	tx, err := h.DB.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Create admin user
	adminUser := req.AdminUser
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(adminUser.PasswordHash), bcrypt.DefaultCost)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Insert user and get ID using RETURNING clause (supported by both SQLite 3.35+ and PostgreSQL)
	var userID int64
	err = tx.QueryRow(`
		INSERT INTO users (email, username, first_name, last_name, password_hash, is_active)
		VALUES (?, ?, ?, ?, ?, true)
		RETURNING id
	`, adminUser.Email, adminUser.Username, adminUser.FirstName, adminUser.LastName, string(hashedPassword)).Scan(&userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Grant system.admin permission to the first user
	var systemAdminPermissionID int
	err = tx.QueryRow("SELECT id FROM permissions WHERE permission_key = 'system.admin'").Scan(&systemAdminPermissionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	_, err = tx.Exec(`
		INSERT INTO user_global_permissions (user_id, permission_id)
		VALUES (?, ?)
	`, userID, systemAdminPermissionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Update module settings
	moduleSettings := []struct {
		key   string
		value bool
	}{
		{"time_tracking_enabled", true}, // Always enabled
		{"test_management_enabled", req.ModuleSettings.TestManagementEnabled},
	}

	for _, setting := range moduleSettings {
		_, err = tx.Exec(`
			UPDATE system_settings
			SET value = ?, updated_at = CURRENT_TIMESTAMP
			WHERE key = ?
		`, strconv.FormatBool(setting.value), setting.key)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to update module setting %s: %w", setting.key, err))
			return
		}
	}

	// Mark setup as completed
	_, err = tx.Exec(`
		UPDATE system_settings
		SET value = 'true', updated_at = CURRENT_TIMESTAMP
		WHERE key = 'setup_completed'
	`)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	_, err = tx.Exec(`
		UPDATE system_settings
		SET value = 'true', updated_at = CURRENT_TIMESTAMP
		WHERE key = 'admin_user_created'
	`)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// CRITICAL SECURITY: Mark setup as completed in auth middleware
	// This immediately enables authentication for all protected endpoints
	// This is a one-way transition (setup→production) and cannot be reversed without server restart
	h.AuthMiddleware.MarkSetupCompleted()

	// Create session for the newly created admin user (auto-login after setup)
	clientIP := h.getClientIP(r)
	session, err := h.SessionManager.CreateSession(int(userID), clientIP, r.UserAgent(), false)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Set session cookie
	if err = h.SessionManager.SetSessionCookie(w, r, session.Token, false); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the updated setup status
	status, err := h.getSetupStatus()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Initial setup completed successfully",
		"status":  status,
	})
}

// GetModuleSettings returns the current module visibility settings
func (h *SetupHandler) GetModuleSettings(w http.ResponseWriter, r *http.Request) {
	timeTracking, err := h.getSettingBool("time_tracking_enabled")
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	testManagement, err := h.getSettingBool("test_management_enabled")
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	settings := models.ModuleSettings{
		TimeTrackingEnabled:   timeTracking,
		TestManagementEnabled: testManagement,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(settings)
}

// UpdateModuleSettings updates module visibility settings
func (h *SetupHandler) UpdateModuleSettings(w http.ResponseWriter, r *http.Request) {
	// Get current user from context (required by middleware)
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		respondUnauthorized(w, r)
		return
	}

	var settings models.ModuleSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Update settings in database
	moduleSettings := []struct {
		key   string
		value bool
	}{
		{"time_tracking_enabled", true}, // Always enabled
		{"test_management_enabled", settings.TestManagementEnabled},
	}

	tx, err := h.DB.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	for _, setting := range moduleSettings {
		_, err = tx.Exec(`
			UPDATE system_settings
			SET value = ?, updated_at = CURRENT_TIMESTAMP
			WHERE key = ?
		`, strconv.FormatBool(setting.value), setting.key)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to update module setting %s: %w", setting.key, err))
			return
		}
	}

	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event
	_ = logger.LogAudit(h.DB, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    h.getClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionModuleEnable, // Using existing constant
		ResourceType: logger.ResourceModule,     // Using existing constant
		ResourceName: "Module Settings",
		Details: map[string]interface{}{
			"time_tracking_enabled":   settings.TimeTrackingEnabled,
			"test_management_enabled": settings.TestManagementEnabled,
		},
		Success: true,
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "Module settings updated successfully",
		"settings": settings,
	})
}

// Helper functions

func (h *SetupHandler) getSetupStatus() (models.SetupStatus, error) {
	var status models.SetupStatus

	setupCompleted, err := h.getSettingBool("setup_completed")
	if err != nil {
		return status, err
	}

	adminUserCreated, err := h.getSettingBool("admin_user_created")
	if err != nil {
		return status, err
	}

	timeTrackingEnabled, err := h.getSettingBool("time_tracking_enabled")
	if err != nil {
		return status, err
	}

	testManagementEnabled, err := h.getSettingBool("test_management_enabled")
	if err != nil {
		return status, err
	}

	status.SetupCompleted = setupCompleted
	status.AdminUserCreated = adminUserCreated
	status.TimeTrackingEnabled = timeTrackingEnabled
	status.TestManagementEnabled = testManagementEnabled

	return status, nil
}

func (h *SetupHandler) getSettingBool(key string) (bool, error) {
	var value string
	err := h.DB.QueryRow("SELECT value FROM system_settings WHERE key = ?", key).Scan(&value)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(value, "true"), nil
}

func (h *SetupHandler) validateSetupRequest(req models.SetupRequest) error {
	// Validate admin user
	if req.AdminUser.Email == "" {
		return fmt.Errorf("admin email is required")
	}
	if req.AdminUser.Username == "" {
		return fmt.Errorf("admin username is required")
	}
	if req.AdminUser.FirstName == "" {
		return fmt.Errorf("admin first name is required")
	}
	if req.AdminUser.LastName == "" {
		return fmt.Errorf("admin last name is required")
	}
	if req.AdminUser.PasswordHash == "" {
		return fmt.Errorf("admin password is required")
	}

	// Basic email validation
	if !strings.Contains(req.AdminUser.Email, "@") {
		return fmt.Errorf("invalid email format")
	}

	return nil
}

// getClientIP extracts the client IP address from request
func (h *SetupHandler) getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (for proxies)
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Take the first IP if multiple are present
		ips := strings.Split(forwarded, ",")
		return strings.TrimSpace(ips[0])
	}

	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	// Remove port if present
	if colonIndex := strings.LastIndex(ip, ":"); colonIndex != -1 {
		ip = ip[:colonIndex]
	}

	return ip
}
