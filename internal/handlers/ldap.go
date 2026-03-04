package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"windshift/internal/database"
	ldapPkg "windshift/internal/ldap"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/sso"
	"windshift/internal/utils"
)

// LDAPHandler handles LDAP directory management endpoints.
type LDAPHandler struct {
	db          database.Database
	syncService *ldapPkg.SyncService
	encryption  *sso.SecretEncryption
}

// NewLDAPHandler creates a new LDAP handler.
func NewLDAPHandler(db database.Database, syncService *ldapPkg.SyncService, encryption *sso.SecretEncryption) *LDAPHandler {
	return &LDAPHandler{db: db, syncService: syncService, encryption: encryption}
}

// LDAPConfigRequest represents the request body for creating/updating an LDAP config.
type LDAPConfigRequest struct {
	Name                string `json:"name"`
	Enabled             bool   `json:"enabled"`
	Host                string `json:"host"`
	Port                int    `json:"port"`
	UseTLS              bool   `json:"use_tls"`
	UseSSL              bool   `json:"use_ssl"`
	SkipTLSVerify       bool   `json:"skip_tls_verify"`
	BindDN              string `json:"bind_dn"`
	BindPassword        string `json:"bind_password,omitempty"`
	BaseDN              string `json:"base_dn"`
	UserFilter          string `json:"user_filter"`
	GroupBaseDN         string `json:"group_base_dn"`
	GroupFilter         string `json:"group_filter"`
	AttrUsername        string `json:"attr_username"`
	AttrEmail           string `json:"attr_email"`
	AttrFirstName       string `json:"attr_first_name"`
	AttrLastName        string `json:"attr_last_name"`
	AttrDisplayName     string `json:"attr_display_name"`
	AttrGroupMember     string `json:"attr_group_member"`
	SyncIntervalMinutes int    `json:"sync_interval_minutes"`
	AutoProvisionUsers  bool   `json:"auto_provision_users"`
	AutoDeactivateUsers bool   `json:"auto_deactivate_users"`
}

// LDAPConfigResponse represents an LDAP config in API responses (without secrets).
type LDAPConfigResponse struct {
	models.LDAPConfig
	HasBindPassword bool `json:"has_bind_password"`
}

// ListConfigs handles GET /api/admin/ldap/configs
func (h *LDAPHandler) ListConfigs(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT id, name, enabled, host, port, use_tls, use_ssl, skip_tls_verify,
			bind_dn, base_dn, user_filter, group_base_dn, group_filter,
			attr_username, attr_email, attr_first_name, attr_last_name, attr_display_name, attr_group_member,
			sync_interval_minutes, auto_provision_users, auto_deactivate_users,
			created_at, updated_at
		FROM ldap_configs ORDER BY created_at ASC
	`)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	configs := make([]LDAPConfigResponse, 0)
	for rows.Next() {
		var c models.LDAPConfig
		var groupBaseDN, groupFilter sql.NullString

		if err := rows.Scan(
			&c.ID, &c.Name, &c.Enabled, &c.Host, &c.Port, &c.UseTLS, &c.UseSSL, &c.SkipTLSVerify,
			&c.BindDN, &c.BaseDN, &c.UserFilter, &groupBaseDN, &groupFilter,
			&c.AttrUsername, &c.AttrEmail, &c.AttrFirstName, &c.AttrLastName,
			&c.AttrDisplayName, &c.AttrGroupMember,
			&c.SyncIntervalMinutes, &c.AutoProvisionUsers, &c.AutoDeactivateUsers,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			respondInternalError(w, r, err)
			return
		}

		c.GroupBaseDN = groupBaseDN.String
		c.GroupFilter = groupFilter.String

		configs = append(configs, LDAPConfigResponse{LDAPConfig: c, HasBindPassword: true})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(configs)
}

// GetConfig handles GET /api/admin/ldap/configs/{id}
func (h *LDAPHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	config, err := h.syncService.GetConfig(id)
	if err != nil {
		respondNotFound(w, r, "LDAP config")
		return
	}

	resp := LDAPConfigResponse{
		LDAPConfig:      *config,
		HasBindPassword: config.BindPasswordEncrypted != "",
	}
	resp.BindPasswordEncrypted = "" // Never expose

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// CreateConfig handles POST /api/admin/ldap/configs
func (h *LDAPHandler) CreateConfig(w http.ResponseWriter, r *http.Request) {
	var req LDAPConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate
	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}
	if req.Host == "" {
		respondValidationError(w, r, "Host is required")
		return
	}
	if req.BindDN == "" {
		respondValidationError(w, r, "Bind DN is required")
		return
	}
	if req.BindPassword == "" {
		respondValidationError(w, r, "Bind password is required")
		return
	}
	if req.BaseDN == "" {
		respondValidationError(w, r, "Base DN is required")
		return
	}

	// Set defaults
	if req.Port == 0 {
		if req.UseSSL {
			req.Port = 636
		} else {
			req.Port = 389
		}
	}
	if req.UserFilter == "" {
		req.UserFilter = "(objectClass=inetOrgPerson)"
	}
	if req.AttrUsername == "" {
		req.AttrUsername = "uid"
	}
	if req.AttrEmail == "" {
		req.AttrEmail = "mail"
	}
	if req.AttrFirstName == "" {
		req.AttrFirstName = "givenName"
	}
	if req.AttrLastName == "" {
		req.AttrLastName = "sn"
	}
	if req.AttrDisplayName == "" {
		req.AttrDisplayName = "cn"
	}
	if req.AttrGroupMember == "" {
		req.AttrGroupMember = "member"
	}

	// Encrypt bind password
	encryptedPassword, err := h.encryption.Encrypt(req.BindPassword)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	var id int64
	err = h.db.QueryRow(`
		INSERT INTO ldap_configs (
			name, enabled, host, port, use_tls, use_ssl, skip_tls_verify,
			bind_dn, bind_password_encrypted, base_dn, user_filter,
			group_base_dn, group_filter,
			attr_username, attr_email, attr_first_name, attr_last_name, attr_display_name, attr_group_member,
			sync_interval_minutes, auto_provision_users, auto_deactivate_users
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`,
		req.Name, req.Enabled, req.Host, req.Port, req.UseTLS, req.UseSSL, req.SkipTLSVerify,
		req.BindDN, encryptedPassword, req.BaseDN, req.UserFilter,
		nullStr(req.GroupBaseDN), nullStr(req.GroupFilter),
		req.AttrUsername, req.AttrEmail, req.AttrFirstName, req.AttrLastName, req.AttrDisplayName, req.AttrGroupMember,
		req.SyncIntervalMinutes, req.AutoProvisionUsers, req.AutoDeactivateUsers,
	).Scan(&id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		newID := int(id)
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionLDAPConfigCreate,
			ResourceType: logger.ResourceLDAPConfig,
			ResourceID:   &newID,
			ResourceName: req.Name,
			Success:      true,
		})
	}

	config, _ := h.syncService.GetConfig(int(id))
	if config != nil {
		resp := LDAPConfigResponse{LDAPConfig: *config, HasBindPassword: true}
		resp.BindPasswordEncrypted = ""
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	} else {
		w.WriteHeader(http.StatusCreated)
	}
}

// UpdateConfig handles PUT /api/admin/ldap/configs/{id}
func (h *LDAPHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	existing, err := h.syncService.GetConfig(id)
	if err != nil {
		respondNotFound(w, r, "LDAP config")
		return
	}

	var req LDAPConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Update fields
	if req.Name != "" {
		existing.Name = req.Name
	}
	existing.Enabled = req.Enabled
	if req.Host != "" {
		existing.Host = req.Host
	}
	if req.Port > 0 {
		existing.Port = req.Port
	}
	existing.UseTLS = req.UseTLS
	existing.UseSSL = req.UseSSL
	existing.SkipTLSVerify = req.SkipTLSVerify
	if req.BindDN != "" {
		existing.BindDN = req.BindDN
	}
	if req.BaseDN != "" {
		existing.BaseDN = req.BaseDN
	}
	if req.UserFilter != "" {
		existing.UserFilter = req.UserFilter
	}
	if req.GroupBaseDN != "" {
		existing.GroupBaseDN = req.GroupBaseDN
	}
	if req.GroupFilter != "" {
		existing.GroupFilter = req.GroupFilter
	}
	if req.AttrUsername != "" {
		existing.AttrUsername = req.AttrUsername
	}
	if req.AttrEmail != "" {
		existing.AttrEmail = req.AttrEmail
	}
	if req.AttrFirstName != "" {
		existing.AttrFirstName = req.AttrFirstName
	}
	if req.AttrLastName != "" {
		existing.AttrLastName = req.AttrLastName
	}
	if req.AttrDisplayName != "" {
		existing.AttrDisplayName = req.AttrDisplayName
	}
	if req.AttrGroupMember != "" {
		existing.AttrGroupMember = req.AttrGroupMember
	}
	existing.SyncIntervalMinutes = req.SyncIntervalMinutes
	existing.AutoProvisionUsers = req.AutoProvisionUsers
	existing.AutoDeactivateUsers = req.AutoDeactivateUsers

	// Update bind password if provided
	bindPasswordClause := ""
	var args []interface{}
	if req.BindPassword != "" {
		encryptedPassword, encErr := h.encryption.Encrypt(req.BindPassword)
		if encErr != nil {
			respondInternalError(w, r, encErr)
			return
		}
		bindPasswordClause = "bind_password_encrypted = ?, "
		args = append(args, encryptedPassword)
	}

	args = append(args,
		existing.Name, existing.Enabled, existing.Host, existing.Port,
		existing.UseTLS, existing.UseSSL, existing.SkipTLSVerify,
		existing.BindDN, existing.BaseDN, existing.UserFilter,
		nullStr(existing.GroupBaseDN), nullStr(existing.GroupFilter),
		existing.AttrUsername, existing.AttrEmail, existing.AttrFirstName, existing.AttrLastName,
		existing.AttrDisplayName, existing.AttrGroupMember,
		existing.SyncIntervalMinutes, existing.AutoProvisionUsers, existing.AutoDeactivateUsers,
		id,
	)

	_, err = h.db.ExecWrite(`
		UPDATE ldap_configs SET
			`+bindPasswordClause+`
			name = ?, enabled = ?, host = ?, port = ?,
			use_tls = ?, use_ssl = ?, skip_tls_verify = ?,
			bind_dn = ?, base_dn = ?, user_filter = ?,
			group_base_dn = ?, group_filter = ?,
			attr_username = ?, attr_email = ?, attr_first_name = ?, attr_last_name = ?,
			attr_display_name = ?, attr_group_member = ?,
			sync_interval_minutes = ?, auto_provision_users = ?, auto_deactivate_users = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, args...)
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
			ActionType:   logger.ActionLDAPConfigUpdate,
			ResourceType: logger.ResourceLDAPConfig,
			ResourceID:   &id,
			ResourceName: existing.Name,
			Success:      true,
		})
	}

	config, _ := h.syncService.GetConfig(id)
	if config != nil {
		resp := LDAPConfigResponse{LDAPConfig: *config, HasBindPassword: true}
		resp.BindPasswordEncrypted = ""
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// DeleteConfig handles DELETE /api/admin/ldap/configs/{id}
func (h *LDAPHandler) DeleteConfig(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	result, err := h.db.ExecWrite("DELETE FROM ldap_configs WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "LDAP config")
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionLDAPConfigDelete,
			ResourceType: logger.ResourceLDAPConfig,
			ResourceID:   &id,
			ResourceName: "",
			Success:      true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// TestConnection handles POST /api/admin/ldap/configs/{id}/test
func (h *LDAPHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	config, err := h.syncService.GetConfig(id)
	if err != nil {
		respondNotFound(w, r, "LDAP config")
		return
	}

	// Decrypt bind password
	bindPassword, err := h.encryption.Decrypt(config.BindPasswordEncrypted)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Connect and test
	client, err := ldapPkg.NewClient(config, bindPassword)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	defer client.Close()

	if err := client.TestConnection(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Try searching for users to verify filter works
	users, err := client.SearchUsers()
	userCount := 0
	if err == nil {
		userCount = len(users)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"user_count": userCount,
	})
}

// TriggerSync handles POST /api/admin/ldap/configs/{id}/sync
func (h *LDAPHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	config, err := h.syncService.GetConfig(id)
	if err != nil {
		respondNotFound(w, r, "LDAP config")
		return
	}

	// Run sync in background
	go func() {
		result, syncErr := h.syncService.SyncUsers(config)
		if syncErr != nil {
			slog.Error("LDAP sync failed", "config_id", id, "error", syncErr)
		} else {
			slog.Info("LDAP sync completed", "config_id", id,
				"synced", result.UsersSynced, "created", result.UsersCreated)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "sync started",
	})
}

// GetSyncStatus handles GET /api/admin/ldap/configs/{id}/sync-status
func (h *LDAPHandler) GetSyncStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	status, err := h.syncService.GetLatestSyncStatus(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if status == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "never_synced",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

// nullStr converts empty string to sql.NullString.
func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
