package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/llm"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/utils"

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

	respondJSONOK(w, status)
}

// CompleteInitialSetup handles the initial setup process
func (h *SetupHandler) CompleteInitialSetup(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[models.SetupRequest](w, r)
	if !ok {
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

	respondJSONOK(w, map[string]interface{}{
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

	respondJSONOK(w, settings)
}

// UpdateModuleSettings updates module visibility settings
func (h *SetupHandler) UpdateModuleSettings(w http.ResponseWriter, r *http.Request) {
	// Get current user from context (required by middleware)
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	settings, ok := decodeJSON[models.ModuleSettings](w, r)
	if !ok {
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

	respondJSONOK(w, map[string]interface{}{
		"success":  true,
		"message":  "Module settings updated successfully",
		"settings": settings,
	})
}

// GetAIFeaturesConfig returns the per-feature AI configuration plus available connections.
func (h *SetupHandler) GetAIFeaturesConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := llm.LoadAIFeaturesConfig(h.DB)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to load AI features config: %w", err))
		return
	}

	// Also return the available (enabled) connections so the frontend can populate dropdowns.
	type connInfo struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	rows, err := h.DB.Query(`SELECT id, name FROM llm_connections WHERE is_enabled = true ORDER BY is_default DESC, name ASC`)
	var connections []connInfo
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var c connInfo
			if err := rows.Scan(&c.ID, &c.Name); err == nil {
				connections = append(connections, c)
			}
		}
	}
	if connections == nil {
		connections = []connInfo{}
	}

	respondJSONOK(w, map[string]interface{}{
		"config":      cfg,
		"connections": connections,
	})
}

// UpdateAIFeaturesConfig validates and saves the per-feature AI configuration.
func (h *SetupHandler) UpdateAIFeaturesConfig(w http.ResponseWriter, r *http.Request) {
	cfg, ok := decodeJSON[models.AIFeaturesConfig](w, r)
	if !ok {
		return
	}

	validModes := map[models.AIFeatureMode]bool{
		models.AIFeatureModeDefault:  true,
		models.AIFeatureModeSpecific: true,
		models.AIFeatureModeDisabled: true,
	}

	for key, fc := range cfg {
		if !validModes[fc.Mode] {
			respondBadRequest(w, r, fmt.Sprintf("Invalid mode %q for feature %q", fc.Mode, key))
			return
		}
		if fc.Mode == models.AIFeatureModeSpecific && fc.ConnectionID <= 0 {
			respondBadRequest(w, r, fmt.Sprintf("Feature %q requires a connection_id when mode is 'specific'", key))
			return
		}
		if fc.Mode == models.AIFeatureModeSpecific {
			var exists int
			if err := h.DB.QueryRow(`SELECT 1 FROM llm_connections WHERE id = ? AND is_enabled = true`, fc.ConnectionID).Scan(&exists); err != nil {
				respondBadRequest(w, r, fmt.Sprintf("Connection %d does not exist or is disabled (feature %q)", fc.ConnectionID, key))
				return
			}
		}
	}

	// Validate schedule field
	for key, fc := range cfg {
		if key == "daily_briefing" {
			if fc.Schedule != "" && fc.Schedule != "daily" && fc.Schedule != "every_6h" {
				respondBadRequest(w, r, fmt.Sprintf("Invalid schedule %q for feature %q; must be \"daily\" or \"every_6h\"", fc.Schedule, key))
				return
			}
		} else if fc.Schedule != "" {
			respondBadRequest(w, r, fmt.Sprintf("Schedule is not supported for feature %q", key))
			return
		}
	}

	if err := llm.SaveAIFeaturesConfig(h.DB, cfg); err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to save AI features config: %w", err))
		return
	}

	// Log audit event
	if user := utils.GetCurrentUser(r); user != nil {
		_ = logger.LogAudit(h.DB, logger.AuditEvent{
			UserID:       user.ID,
			Username:     user.Username,
			IPAddress:    h.getClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionModuleEnable,
			ResourceType: logger.ResourceModule,
			ResourceName: "AI Features Config",
			Success:      true,
		})
	}

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"config":  cfg,
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
