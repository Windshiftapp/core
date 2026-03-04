package ldap

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/sso"
)

// SyncService handles LDAP user synchronization.
type SyncService struct {
	db         database.Database
	encryption *sso.SecretEncryption
}

// NewSyncService creates a new LDAP sync service.
func NewSyncService(db database.Database, encryption *sso.SecretEncryption) *SyncService {
	return &SyncService{db: db, encryption: encryption}
}

// SyncResult contains the results of a sync operation.
type SyncResult struct {
	UsersSynced      int
	UsersCreated     int
	UsersUpdated     int
	UsersDeactivated int
	Errors           []string
}

// SyncUsers performs a full user sync for the given LDAP config.
func (s *SyncService) SyncUsers(config *models.LDAPConfig) (*SyncResult, error) {
	result := &SyncResult{}

	// Create sync status record
	syncStatusID, err := s.createSyncStatus(config.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to create sync status: %w", err)
	}

	// Update status to running
	s.updateSyncStatus(syncStatusID, "running", nil, result)

	// Decrypt bind password
	bindPassword, err := s.encryption.Decrypt(config.BindPasswordEncrypted)
	if err != nil {
		errMsg := "failed to decrypt bind password"
		s.updateSyncStatus(syncStatusID, "failed", &errMsg, result)
		return nil, fmt.Errorf("%s: %w", errMsg, err)
	}

	// Connect to LDAP
	client, err := NewClient(config, bindPassword)
	if err != nil {
		errMsg := fmt.Sprintf("LDAP connection failed: %v", err)
		s.updateSyncStatus(syncStatusID, "failed", &errMsg, result)
		return nil, err
	}
	defer client.Close()

	// Search for users
	ldapUsers, err := client.SearchUsers()
	if err != nil {
		errMsg := fmt.Sprintf("LDAP search failed: %v", err)
		s.updateSyncStatus(syncStatusID, "failed", &errMsg, result)
		return nil, err
	}

	slog.Info("LDAP sync: found users", "count", len(ldapUsers), "config", config.Name)

	// Get existing LDAP user mappings
	existingMappings, err := s.getExistingMappings(config.ID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get existing mappings: %v", err)
		s.updateSyncStatus(syncStatusID, "failed", &errMsg, result)
		return nil, err
	}

	// Track which DNs we've seen (for deactivation)
	seenDNS := make(map[string]bool)

	// Process each LDAP user
	for _, ldapUser := range ldapUsers {
		seenDNS[ldapUser.DN] = true
		result.UsersSynced++

		if mapping, ok := existingMappings[ldapUser.DN]; ok {
			// Existing user - update
			if err := s.updateUser(mapping.UserID, ldapUser); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to update user %s: %v", ldapUser.Email, err))
				continue
			}
			// Update last synced
			_, _ = s.db.Exec("UPDATE ldap_user_mappings SET last_synced_at = CURRENT_TIMESTAMP WHERE id = ?", mapping.ID)
			result.UsersUpdated++
		} else if config.AutoProvisionUsers {
			// New user - create
			if err := s.createUser(config.ID, ldapUser); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to create user %s: %v", ldapUser.Email, err))
				continue
			}
			result.UsersCreated++
		}
	}

	// Deactivate users no longer in LDAP
	if config.AutoDeactivateUsers {
		for dn, mapping := range existingMappings {
			if !seenDNS[dn] {
				if err := s.deactivateUser(mapping.UserID); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("failed to deactivate user %d: %v", mapping.UserID, err))
					continue
				}
				result.UsersDeactivated++
			}
		}
	}

	// Update sync status
	s.updateSyncStatus(syncStatusID, "completed", nil, result)

	// Audit log
	go func() {
		_ = logger.LogAudit(s.db, logger.AuditEvent{
			Username:     "LDAP:sync",
			ActionType:   "ldap.sync",
			ResourceType: "ldap_config",
			ResourceName: config.Name,
			Details: map[string]interface{}{
				"config_id":   config.ID,
				"synced":      result.UsersSynced,
				"created":     result.UsersCreated,
				"updated":     result.UsersUpdated,
				"deactivated": result.UsersDeactivated,
				"errors":      len(result.Errors),
			},
			Success: len(result.Errors) == 0,
		})
	}()

	slog.Info("LDAP sync completed",
		"config", config.Name,
		"synced", result.UsersSynced,
		"created", result.UsersCreated,
		"updated", result.UsersUpdated,
		"deactivated", result.UsersDeactivated,
		"errors", len(result.Errors),
	)

	return result, nil
}

// getExistingMappings returns existing LDAP user mappings keyed by DN.
func (s *SyncService) getExistingMappings(configID int) (map[string]*models.LDAPUserMapping, error) {
	rows, err := s.db.Query(
		"SELECT id, config_id, user_id, ldap_dn, ldap_uid, last_synced_at, created_at FROM ldap_user_mappings WHERE config_id = ?",
		configID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	mappings := make(map[string]*models.LDAPUserMapping)
	for rows.Next() {
		var m models.LDAPUserMapping
		if err := rows.Scan(&m.ID, &m.ConfigID, &m.UserID, &m.LDAPDN, &m.LDAPUID, &m.LastSyncedAt, &m.CreatedAt); err != nil {
			return nil, err
		}
		mappings[m.LDAPDN] = &m
	}
	return mappings, nil
}

// createUser creates a new local user from an LDAP entry.
func (s *SyncService) createUser(configID int, ldapUser LDAPUser) error {
	// Check if user already exists by email
	var existingID int
	err := s.db.QueryRow("SELECT id FROM users WHERE email = ?", strings.ToLower(ldapUser.Email)).Scan(&existingID)
	if err == nil {
		// User exists - just create mapping
		_, err = s.db.Exec(`
			INSERT INTO ldap_user_mappings (config_id, user_id, ldap_dn, ldap_uid)
			VALUES (?, ?, ?, ?)
		`, configID, existingID, ldapUser.DN, ldapUser.UID)
		return err
	}
	if err != sql.ErrNoRows {
		return err
	}

	// Generate username
	username := ldapUser.UID
	if username == "" {
		username = strings.Split(ldapUser.Email, "@")[0]
	}

	// Ensure unique username
	username = s.ensureUniqueUsername(username)

	firstName := ldapUser.FirstName
	lastName := ldapUser.LastName
	if firstName == "" && lastName == "" && ldapUser.DisplayName != "" {
		parts := strings.SplitN(ldapUser.DisplayName, " ", 2)
		firstName = parts[0]
		if len(parts) > 1 {
			lastName = parts[1]
		}
	}

	// Create user (no password - LDAP auth only)
	var userID int64
	err = s.db.QueryRow(`
		INSERT INTO users (email, username, first_name, last_name, is_active, password_hash,
			requires_password_reset, timezone, language, email_verified, created_at, updated_at)
		VALUES (?, ?, ?, ?, true, '', 0, 'UTC', 'en', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
	`, strings.ToLower(ldapUser.Email), username, firstName, lastName).Scan(&userID)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	// Create LDAP mapping
	_, err = s.db.ExecWrite(`
		INSERT INTO ldap_user_mappings (config_id, user_id, ldap_dn, ldap_uid)
		VALUES (?, ?, ?, ?)
	`, configID, int(userID), ldapUser.DN, ldapUser.UID)
	return err
}

// updateUser updates an existing user from LDAP data.
func (s *SyncService) updateUser(userID int, ldapUser LDAPUser) error {
	firstName := ldapUser.FirstName
	lastName := ldapUser.LastName
	if firstName == "" && lastName == "" && ldapUser.DisplayName != "" {
		parts := strings.SplitN(ldapUser.DisplayName, " ", 2)
		firstName = parts[0]
		if len(parts) > 1 {
			lastName = parts[1]
		}
	}

	_, err := s.db.ExecWrite(`
		UPDATE users SET first_name = ?, last_name = ?, email = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, firstName, lastName, strings.ToLower(ldapUser.Email), userID)
	return err
}

// deactivateUser sets a user as inactive.
func (s *SyncService) deactivateUser(userID int) error {
	_, err := s.db.ExecWrite("UPDATE users SET is_active = false, updated_at = CURRENT_TIMESTAMP WHERE id = ?", userID)
	return err
}

// ensureUniqueUsername appends a number if the username already exists.
func (s *SyncService) ensureUniqueUsername(username string) string {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&count)
	if err != nil || count == 0 {
		return username
	}

	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s%d", username, i)
		err = s.db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", candidate).Scan(&count)
		if err != nil || count == 0 {
			return candidate
		}
	}
	return username + fmt.Sprintf("%d", time.Now().Unix())
}

// createSyncStatus creates a new sync status record.
func (s *SyncService) createSyncStatus(configID int) (int, error) {
	var id int64
	err := s.db.QueryRow(`
		INSERT INTO ldap_sync_status (config_id, status, started_at)
		VALUES (?, 'pending', CURRENT_TIMESTAMP) RETURNING id
	`, configID).Scan(&id)
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// updateSyncStatus updates the sync status record.
func (s *SyncService) updateSyncStatus(id int, status string, errorMsg *string, result *SyncResult) {
	var errStr sql.NullString
	if errorMsg != nil {
		errStr = sql.NullString{String: *errorMsg, Valid: true}
	}

	_, err := s.db.ExecWrite(`
		UPDATE ldap_sync_status SET
			status = ?, completed_at = CURRENT_TIMESTAMP,
			users_synced = ?, users_created = ?, users_updated = ?, users_deactivated = ?,
			error_message = ?
		WHERE id = ?
	`, status, result.UsersSynced, result.UsersCreated, result.UsersUpdated, result.UsersDeactivated, errStr, id)
	if err != nil {
		slog.Error("failed to update LDAP sync status", "error", err)
	}
}

// GetConfig retrieves an LDAP config by ID.
func (s *SyncService) GetConfig(id int) (*models.LDAPConfig, error) {
	var config models.LDAPConfig
	var groupBaseDN, groupFilter sql.NullString

	err := s.db.QueryRow(`
		SELECT id, name, enabled, host, port, use_tls, use_ssl, skip_tls_verify,
			bind_dn, bind_password_encrypted, base_dn, user_filter,
			group_base_dn, group_filter,
			attr_username, attr_email, attr_first_name, attr_last_name, attr_display_name, attr_group_member,
			sync_interval_minutes, auto_provision_users, auto_deactivate_users,
			created_at, updated_at
		FROM ldap_configs WHERE id = ?
	`, id).Scan(
		&config.ID, &config.Name, &config.Enabled,
		&config.Host, &config.Port, &config.UseTLS, &config.UseSSL, &config.SkipTLSVerify,
		&config.BindDN, &config.BindPasswordEncrypted, &config.BaseDN, &config.UserFilter,
		&groupBaseDN, &groupFilter,
		&config.AttrUsername, &config.AttrEmail, &config.AttrFirstName, &config.AttrLastName,
		&config.AttrDisplayName, &config.AttrGroupMember,
		&config.SyncIntervalMinutes, &config.AutoProvisionUsers, &config.AutoDeactivateUsers,
		&config.CreatedAt, &config.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("LDAP config not found")
	}
	if err != nil {
		return nil, err
	}

	config.GroupBaseDN = groupBaseDN.String
	config.GroupFilter = groupFilter.String

	return &config, nil
}

// GetLatestSyncStatus returns the most recent sync status for a config.
func (s *SyncService) GetLatestSyncStatus(configID int) (*models.LDAPSyncStatus, error) {
	var status models.LDAPSyncStatus
	var startedAt, completedAt sql.NullTime
	var errorMessage sql.NullString

	err := s.db.QueryRow(`
		SELECT id, config_id, status, started_at, completed_at,
			users_synced, users_created, users_updated, users_deactivated,
			error_message, created_at
		FROM ldap_sync_status
		WHERE config_id = ?
		ORDER BY created_at DESC
		LIMIT 1
	`, configID).Scan(
		&status.ID, &status.ConfigID, &status.Status,
		&startedAt, &completedAt,
		&status.UsersSynced, &status.UsersCreated, &status.UsersUpdated, &status.UsersDeactivated,
		&errorMessage, &status.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if startedAt.Valid {
		status.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		status.CompletedAt = &completedAt.Time
	}
	status.ErrorMessage = errorMessage.String

	return &status, nil
}
