package handlers

import (
	"windshift/internal/database"
	"windshift/internal/email"
	"windshift/internal/logger"
	"windshift/internal/scheduler"
	"windshift/internal/services"
	"windshift/internal/webhook"
	"context"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"

	"windshift/internal/models"

)

// ChannelHandler handles HTTP requests for channels
type ChannelHandler struct {
	db                database.Database
	permissionService *services.PermissionService
	webhookSender     *webhook.WebhookSender
	emailScheduler    *scheduler.EmailScheduler
	encryption        email.Encryptor
	baseURL           string
}

// NewChannelHandler creates a new channel handler
func NewChannelHandler(db database.Database, permissionService *services.PermissionService, webhookSender *webhook.WebhookSender) *ChannelHandler {
	return &ChannelHandler{
		db:                db,
		permissionService: permissionService,
		webhookSender:     webhookSender,
	}
}

// SetEncryption sets the encryption service for OAuth credential handling
func (h *ChannelHandler) SetEncryption(enc email.Encryptor) {
	h.encryption = enc
}

// SetBaseURL sets the base URL for OAuth callbacks
func (h *ChannelHandler) SetBaseURL(baseURL string) {
	h.baseURL = baseURL
}

// SetEmailScheduler sets the email scheduler (used to avoid circular dependencies)
func (h *ChannelHandler) SetEmailScheduler(es *scheduler.EmailScheduler) {
	h.emailScheduler = es
}

// GetChannels returns all channels (admins) or only managed channels (non-admins)
func (h *ChannelHandler) GetChannels(w http.ResponseWriter, r *http.Request) {
	// Get current user
	user, ok := r.Context().Value("user").(*models.User)
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse category_id filter from query params
	categoryFilter := r.URL.Query().Get("category_id")

	var query string
	var args []interface{}

	// Check if user is a system admin
	isSystemAdmin, err := h.permissionService.IsSystemAdmin(user.ID)
	if err == nil && isSystemAdmin {
		// System admins see all channels
		query = `
			SELECT c.id, c.name, c.type, c.direction, c.description, c.status, c.is_default, c.config,
				   c.plugin_name, c.plugin_webhook_id, c.category_id, c.created_at, c.updated_at, c.last_activity,
				   cc.name, cc.color
			FROM channels c
			LEFT JOIN channel_categories cc ON c.category_id = cc.id
		`

		// Add category filter if specified
		if categoryFilter != "" {
			if categoryFilter == "null" {
				query += " WHERE c.category_id IS NULL"
			} else {
				query += " WHERE c.category_id = ?"
				args = append(args, categoryFilter)
			}
		}

		query += " ORDER BY c.is_default DESC, c.created_at ASC"
	} else {
		// Non-admins only see channels they manage
		query = `
			SELECT DISTINCT c.id, c.name, c.type, c.direction, c.description, c.status,
				   c.is_default, c.config, c.plugin_name, c.plugin_webhook_id, c.category_id,
				   c.created_at, c.updated_at, c.last_activity,
				   cc.name, cc.color
			FROM channels c
			LEFT JOIN channel_categories cc ON c.category_id = cc.id
			INNER JOIN channel_managers cm ON c.id = cm.channel_id
			WHERE ((cm.manager_type = 'user' AND cm.manager_id = ?)
			   OR (cm.manager_type = 'group' AND cm.manager_id IN (
				   SELECT group_id FROM group_members WHERE user_id = ?
			   )))
		`
		args = []interface{}{user.ID, user.ID}

		// Add category filter if specified
		if categoryFilter != "" {
			if categoryFilter == "null" {
				query += " AND c.category_id IS NULL"
			} else {
				query += " AND c.category_id = ?"
				args = append(args, categoryFilter)
			}
		}

		query += " ORDER BY c.is_default DESC, c.created_at ASC"
	}

	rows, err := h.db.QueryContext(ctx, query, args...)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get channels: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var channels []models.Channel
	for rows.Next() {
		var channel models.Channel
		var categoryName, categoryColor sql.NullString
		err := rows.Scan(
			&channel.ID, &channel.Name, &channel.Type, &channel.Direction,
			&channel.Description, &channel.Status, &channel.IsDefault, &channel.Config,
			&channel.PluginName, &channel.PluginWebhookID, &channel.CategoryID,
			&channel.CreatedAt, &channel.UpdatedAt, &channel.LastActivity,
			&categoryName, &categoryColor,
		)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to scan channel: %v", err), http.StatusInternalServerError)
			return
		}
		if categoryName.Valid {
			channel.CategoryName = categoryName.String
		}
		if categoryColor.Valid {
			channel.CategoryColor = categoryColor.String
		}
		// Scrub sensitive data from config
		channel.Config = scrubChannelConfig(channel.Config)
		channels = append(channels, channel)
	}

	if err = rows.Err(); err != nil {
		http.Error(w, fmt.Sprintf("Error reading channels: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(channels)
}

// CreateChannel creates a new channel
func (h *ChannelHandler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var channel models.Channel
	if err := json.NewDecoder(r.Body).Decode(&channel); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if channel.Name == "" || channel.Type == "" || channel.Direction == "" {
		http.Error(w, "Name, type, and direction are required", http.StatusBadRequest)
		return
	}

	// Set default status if not provided
	if channel.Status == "" {
		channel.Status = "disabled"
	}

	now := time.Now()
	channel.CreatedAt = now
	channel.UpdatedAt = now

	var id int64
	err := h.db.QueryRowContext(ctx, `
		INSERT INTO channels (name, type, direction, description, status, is_default, config, category_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		channel.Name, channel.Type, channel.Direction, channel.Description,
		channel.Status, channel.IsDefault, channel.Config, channel.CategoryID, channel.CreatedAt, channel.UpdatedAt,
	).Scan(&id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create channel: %v", err), http.StatusInternalServerError)
		return
	}

	channel.ID = int(id)

	// Scrub sensitive data from config before returning
	channel.Config = scrubChannelConfig(channel.Config)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(channel)
}

// GetChannel returns a specific channel by ID
func (h *ChannelHandler) GetChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT c.id, c.name, c.type, c.direction, c.description, c.status, c.is_default, c.config,
			   c.plugin_name, c.plugin_webhook_id, c.category_id, c.created_at, c.updated_at, c.last_activity,
			   cc.name, cc.color
		FROM channels c
		LEFT JOIN channel_categories cc ON c.category_id = cc.id
		WHERE c.id = ?
	`

	var channel models.Channel
	var categoryName, categoryColor sql.NullString
	err = h.db.QueryRowContext(ctx, query, id).Scan(
		&channel.ID, &channel.Name, &channel.Type, &channel.Direction,
		&channel.Description, &channel.Status, &channel.IsDefault, &channel.Config,
		&channel.PluginName, &channel.PluginWebhookID, &channel.CategoryID,
		&channel.CreatedAt, &channel.UpdatedAt, &channel.LastActivity,
		&categoryName, &categoryColor,
	)
	if categoryName.Valid {
		channel.CategoryName = categoryName.String
	}
	if categoryColor.Valid {
		channel.CategoryColor = categoryColor.String
	}
	if err == sql.ErrNoRows {
		http.Error(w, "Channel not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get channel: %v", err), http.StatusInternalServerError)
		return
	}

	// Scrub sensitive data from config
	channel.Config = scrubChannelConfig(channel.Config)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(channel)
}

// scrubChannelConfig removes sensitive fields from the configuration JSON
func scrubChannelConfig(configJSON string) string {
	if configJSON == "" {
		return ""
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return configJSON // Return as is if invalid JSON
	}

	// Remove sensitive fields
	delete(config, "smtp_password")
	delete(config, "imap_password")
	delete(config, "webhook_secret")

	// Re-marshal
	scrubbed, err := json.Marshal(config)
	if err != nil {
		return configJSON
	}
	return string(scrubbed)
}

// UpdateChannel updates an existing channel
func (h *ChannelHandler) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	var updates models.Channel
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if channel exists and is not a plugin-managed channel
	var exists bool
	var pluginName *string
	checkQuery := "SELECT EXISTS(SELECT 1 FROM channels WHERE id = ?), (SELECT plugin_name FROM channels WHERE id = ?)"
	err = h.db.QueryRowContext(ctx, checkQuery, id, id).Scan(&exists, &pluginName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Channel not found", http.StatusNotFound)
		return
	}
	if pluginName != nil && *pluginName != "" {
		http.Error(w, "Cannot modify plugin-managed channel", http.StatusForbidden)
		return
	}

	updates.UpdatedAt = time.Now()

	query := `
		UPDATE channels
		SET name = ?, type = ?, direction = ?, description = ?, status = ?,
		    is_default = ?, config = ?, category_id = ?, updated_at = ?
		WHERE id = ?
	`

	_, err = h.db.ExecWriteContext(ctx, query,
		updates.Name, updates.Type, updates.Direction, updates.Description,
		updates.Status, updates.IsDefault, updates.Config, updates.CategoryID, updates.UpdatedAt, id,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update channel: %v", err), http.StatusInternalServerError)
		return
	}

	// Return the updated channel
	h.GetChannel(w, r)
}

// DeleteChannel deletes a channel
func (h *ChannelHandler) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if channel exists, is not default, and is not plugin-managed
	var exists bool
	var isDefault bool
	var pluginName *string
	checkQuery := `
		SELECT
			EXISTS(SELECT 1 FROM channels WHERE id = ?),
			COALESCE((SELECT is_default FROM channels WHERE id = ?), false),
			(SELECT plugin_name FROM channels WHERE id = ?)
	`
	err = h.db.QueryRowContext(ctx, checkQuery, id, id, id).Scan(&exists, &isDefault, &pluginName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Channel not found", http.StatusNotFound)
		return
	}
	if isDefault {
		http.Error(w, "Cannot delete default channel", http.StatusBadRequest)
		return
	}
	if pluginName != nil && *pluginName != "" {
		http.Error(w, "Cannot delete plugin-managed channel", http.StatusForbidden)
		return
	}

	query := "DELETE FROM channels WHERE id = ?"
	_, err = h.db.ExecWriteContext(ctx, query, id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete channel: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// TestChannel tests a channel configuration by sending a test email
func (h *ChannelHandler) TestChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	// Parse request body for test email address
	var testRequest struct {
		TestEmail string `json:"test_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&testRequest); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if testRequest.TestEmail == "" {
		http.Error(w, "test_email is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // Longer timeout for network operations
	defer cancel()

	// Get channel configuration
	var channel models.Channel
	query := `
		SELECT id, name, type, direction, description, status, is_default, config,
			   plugin_name, plugin_webhook_id, created_at, updated_at, last_activity
		FROM channels
		WHERE id = ?
	`

	err = h.db.QueryRowContext(ctx, query, id).Scan(
		&channel.ID, &channel.Name, &channel.Type, &channel.Direction,
		&channel.Description, &channel.Status, &channel.IsDefault, &channel.Config,
		&channel.PluginName, &channel.PluginWebhookID,
		&channel.CreatedAt, &channel.UpdatedAt, &channel.LastActivity,
	)
	if err == sql.ErrNoRows {
		http.Error(w, "Channel not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get channel: %v", err), http.StatusInternalServerError)
		return
	}

	// Test the channel based on its type
	result := make(map[string]interface{})
	result["channel_id"] = channel.ID
	result["channel_name"] = channel.Name
	result["test_time"] = time.Now()
	result["test_email"] = testRequest.TestEmail

	switch channel.Type {
	case "smtp":
		success, message := h.testSMTPChannelWithEmail(channel, testRequest.TestEmail)
		result["success"] = success
		result["message"] = message
		if success {
			// Update last activity
			h.updateChannelActivity(ctx, channel.ID)
		}
	default:
		result["success"] = false
		result["message"] = fmt.Sprintf("Testing not implemented for channel type: %s", channel.Type)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// TestChannelConfig tests a channel configuration without saving it
func (h *ChannelHandler) TestChannelConfig(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	var testData struct {
		Config models.ChannelConfig `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&testData); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // Longer timeout for network operations
	defer cancel()

	// Get channel type from database
	var channelType string
	query := "SELECT type FROM channels WHERE id = ?"
	err = h.db.QueryRowContext(ctx, query, id).Scan(&channelType)
	if err == sql.ErrNoRows {
		http.Error(w, "Channel not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get channel: %v", err), http.StatusInternalServerError)
		return
	}

	// Test the configuration based on channel type
	result := make(map[string]interface{})
	result["channel_id"] = id
	result["test_time"] = time.Now()

	switch channelType {
	case "smtp":
		result["success"] = h.testSMTPConfig(testData.Config)
		if result["success"].(bool) {
			result["message"] = "SMTP connection successful"
		} else {
			result["message"] = "SMTP connection failed"
		}
	case "webhook":
		if h.webhookSender != nil {
			success, message := h.webhookSender.SendTestWebhook(ctx, &testData.Config)
			result["success"] = success
			result["message"] = message
		} else {
			result["success"] = false
			result["message"] = "Webhook sender not configured"
		}
	default:
		result["success"] = false
		result["message"] = fmt.Sprintf("Testing not supported for channel type: %s", channelType)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// testSMTPChannelWithEmail tests an SMTP channel by sending a test email
func (h *ChannelHandler) testSMTPChannelWithEmail(channel models.Channel, testEmail string) (bool, string) {
	// Parse SMTP configuration
	var config models.ChannelConfig
	if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
		return false, "Failed to parse SMTP configuration: " + err.Error()
	}

	// Basic validation
	if config.SMTPHost == "" {
		return false, "SMTP host is not configured"
	}
	if config.SMTPPort == 0 {
		return false, "SMTP port is not configured"
	}
	if config.SMTPFromEmail == "" {
		return false, "From email is not configured"
	}

	// Create a test email
	subject := "Windshift SMTP Test Email"
	htmlBody := `<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<title>Windshift SMTP Test</title>
	<style>
		body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 20px; background-color: #f5f5f5; }
		.container { max-width: 600px; margin: 0 auto; background-color: white; border-radius: 8px; padding: 24px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
		.header { text-align: center; color: #2563eb; margin-bottom: 24px; }
		.content { color: #374151; line-height: 1.6; }
		.success { background-color: #dcfce7; border: 1px solid #16a34a; color: #15803d; padding: 12px; border-radius: 6px; margin: 16px 0; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<h1>Windshift SMTP Test</h1>
		</div>
		<div class="content">
			<div class="success">
				<strong>Success!</strong> Your SMTP configuration is working correctly.
			</div>
			<p>This test email was sent from Windshift to verify your SMTP settings.</p>
			<p><strong>Channel:</strong> ` + channel.Name + `</p>
			<p><strong>Test Time:</strong> ` + time.Now().Format("January 2, 2006 at 3:04 PM MST") + `</p>
			<p>If you received this email, your SMTP configuration is ready to send notifications.</p>
		</div>
	</div>
</body>
</html>`

	textBody := `Windshift SMTP Test Email

Success! Your SMTP configuration is working correctly.

This test email was sent from Windshift to verify your SMTP settings.

Channel: ` + channel.Name + `
Test Time: ` + time.Now().Format("January 2, 2006 at 3:04 PM MST") + `

If you received this email, your SMTP configuration is ready to send notifications.`

	// Send the test email using the SMTP sender functionality
	err := h.sendTestEmail(&config, testEmail, subject, htmlBody, textBody)
	if err != nil {
		// Provide more specific error guidance based on common SMTP errors
		errorMsg := err.Error()
		if strings.Contains(errorMsg, "502") {
			return false, fmt.Sprintf("SMTP server error (502): %s. This usually means the server doesn't support the requested command. Try checking your server settings or use a different encryption method.", errorMsg)
		} else if strings.Contains(errorMsg, "530") {
			return false, fmt.Sprintf("Authentication failed (530): %s. Please check your username and password.", errorMsg)
		} else if strings.Contains(errorMsg, "535") {
			return false, fmt.Sprintf("Authentication credentials invalid (535): %s. Please verify your username and password are correct.", errorMsg)
		} else if strings.Contains(errorMsg, "connection refused") || strings.Contains(errorMsg, "no such host") {
			return false, fmt.Sprintf("Connection failed: %s. Please check your SMTP host and port settings.", errorMsg)
		} else {
			return false, "Failed to send test email: " + errorMsg
		}
	}

	return true, "Test email sent successfully to " + testEmail
}

// testSMTPChannel tests an SMTP channel configuration (legacy method for backwards compatibility)
func (h *ChannelHandler) testSMTPChannel(channel models.Channel) bool {
	// Parse SMTP configuration
	var config models.ChannelConfig
	if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
		return false
	}

	// Basic validation
	if config.SMTPHost == "" || config.SMTPPort == 0 {
		return false
	}

	return true // Basic validation passed
}

// testSMTPConfig tests SMTP configuration directly
func (h *ChannelHandler) testSMTPConfig(config models.ChannelConfig) bool {
	// Basic validation
	if config.SMTPHost == "" || config.SMTPPort == 0 {
		return false
	}

	// Test actual SMTP connection
	addr := fmt.Sprintf("%s:%d", config.SMTPHost, config.SMTPPort)

	// Set connection timeout
	timeout := 10 * time.Second

	// Attempt to connect to SMTP server
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		// Log SMTP connection failure
		logger.Get().Debug("SMTP connection failed", "error", err)
		return false
	}
	defer conn.Close()

	// If TLS is enabled, test TLS connection
	// TODO: Add SMTPUseTLS field to models.ChannelConfig
	if false /* config.SMTPUseTLS */ {
		tlsConfig := &tls.Config{
			ServerName:         config.SMTPHost,
			InsecureSkipVerify: false, // Require valid certificate
			MinVersion:         tls.VersionTLS12,
		}

		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			// Log SMTP TLS handshake failure
			logger.Get().Debug("SMTP TLS handshake failed", "error", err)
			return false
		}
		tlsConn.Close()
	}

	return true // Connection test successful
}

// sendTestEmail sends a test email using the existing SMTP functionality
func (h *ChannelHandler) sendTestEmail(config *models.ChannelConfig, toEmail, subject, htmlBody, textBody string) error {
	// Build MIME message
	boundary := "----=_NextPart_" + fmt.Sprintf("%d", time.Now().UnixNano())

	var from string
	if config.SMTPFromName != "" {
		from = fmt.Sprintf("%s <%s>", config.SMTPFromName, config.SMTPFromEmail)
	} else {
		from = config.SMTPFromEmail
	}

	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%s\r\n\r\n",
		from, toEmail, subject, boundary)

	textPart := fmt.Sprintf("--%s\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n\r\n",
		boundary, textBody)

	htmlPart := fmt.Sprintf("--%s\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n\r\n",
		boundary, htmlBody)

	ending := fmt.Sprintf("--%s--\r\n", boundary)
	message := headers + textPart + htmlPart + ending

	// Set up authentication
	var auth smtp.Auth
	if config.SMTPUsername != "" && config.SMTPPassword != "" {
		auth = smtp.PlainAuth("", config.SMTPUsername, config.SMTPPassword, config.SMTPHost)
	}

	// Determine server address
	addr := fmt.Sprintf("%s:%d", config.SMTPHost, config.SMTPPort)

	// Handle different encryption types
	switch strings.ToLower(config.SMTPEncryption) {
	case "tls":
		return h.sendTestEmailWithStartTLS(addr, auth, config.SMTPFromEmail, toEmail, message)
	case "ssl":
		return h.sendTestEmailWithSSL(addr, auth, config.SMTPFromEmail, toEmail, message)
	default: // "none" or empty
		return smtp.SendMail(addr, auth, config.SMTPFromEmail, []string{toEmail}, []byte(message))
	}
}

// sendTestEmailWithStartTLS sends test email using STARTTLS encryption
func (h *ChannelHandler) sendTestEmailWithStartTLS(addr string, auth smtp.Auth, from, to, message string) error {
	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()

	// Start TLS
	tlsConfig := &tls.Config{
		ServerName: strings.Split(addr, ":")[0],
		MinVersion: tls.VersionTLS12,
	}
	
	if err = client.StartTLS(tlsConfig); err != nil {
		return err
	}

	// Authenticate
	if auth != nil {
		if err = client.Auth(auth); err != nil {
			return err
		}
	}

	// Set sender and recipient
	if err = client.Mail(from); err != nil {
		return err
	}

	if err = client.Rcpt(to); err != nil {
		return err
	}

	// Send message
	writer, err := client.Data()
	if err != nil {
		return err
	}
	defer writer.Close()

	_, err = writer.Write([]byte(message))
	return err
}

// sendTestEmailWithSSL sends test email using SSL/TLS encryption
func (h *ChannelHandler) sendTestEmailWithSSL(addr string, auth smtp.Auth, from, to, message string) error {
	// Create TLS connection
	tlsConfig := &tls.Config{
		ServerName: strings.Split(addr, ":")[0],
		MinVersion: tls.VersionTLS12,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Create SMTP client on TLS connection
	client, err := smtp.NewClient(conn, strings.Split(addr, ":")[0])
	if err != nil {
		return err
	}
	defer client.Close()

	// Authenticate
	if auth != nil {
		if err = client.Auth(auth); err != nil {
			return err
		}
	}

	// Set sender and recipient
	if err = client.Mail(from); err != nil {
		return err
	}

	if err = client.Rcpt(to); err != nil {
		return err
	}

	// Send message
	writer, err := client.Data()
	if err != nil {
		return err
	}
	defer writer.Close()

	_, err = writer.Write([]byte(message))
	return err
}

// updateChannelActivity updates the last_activity timestamp for a channel
func (h *ChannelHandler) updateChannelActivity(ctx context.Context, channelID int) {
	query := "UPDATE channels SET last_activity = ? WHERE id = ?"
	h.db.ExecWriteContext(ctx, query, time.Now(), channelID)
}

// UpdateChannelConfig updates only the configuration of a channel
func (h *ChannelHandler) UpdateChannelConfig(w http.ResponseWriter, r *http.Request) {
	// Get current user
	user, ok := r.Context().Value("user").(*models.User)
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	var configUpdate struct {
		Config models.ChannelConfig `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&configUpdate); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get current channel status, name, existing config, and plugin info for audit logging and merging
	var oldStatus string
	var channelName string
	var existingConfigJSON string
	var pluginName *string
	checkQuery := "SELECT status, name, config, plugin_name FROM channels WHERE id = ?"
	err = h.db.QueryRowContext(ctx, checkQuery, id).Scan(&oldStatus, &channelName, &existingConfigJSON, &pluginName)
	if err == sql.ErrNoRows {
		http.Error(w, "Channel not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Prevent modification of plugin-managed channels
	if pluginName != nil && *pluginName != "" {
		http.Error(w, "Cannot modify plugin-managed channel configuration", http.StatusForbidden)
		return
	}

	// Merge existing config with new config to preserve unmodified fields
	var mergedConfig map[string]interface{}

	// Unmarshal existing config into map
	if existingConfigJSON != "" {
		if err := json.Unmarshal([]byte(existingConfigJSON), &mergedConfig); err != nil {
			// If existing config is invalid, start with empty map
			mergedConfig = make(map[string]interface{})
		}
	} else {
		mergedConfig = make(map[string]interface{})
	}

	// Unmarshal incoming config into map
	incomingConfigBytes, err := json.Marshal(configUpdate.Config)
	if err != nil {
		http.Error(w, "Failed to serialize incoming config", http.StatusInternalServerError)
		return
	}

	var incomingConfig map[string]interface{}
	if err := json.Unmarshal(incomingConfigBytes, &incomingConfig); err != nil {
		http.Error(w, "Failed to parse incoming config", http.StatusInternalServerError)
		return
	}

	// Merge: incoming config overwrites existing config for keys that are present
	for key, value := range incomingConfig {
		mergedConfig[key] = value
	}

	// Convert merged config back to JSON
	configJSON, err := json.Marshal(mergedConfig)
	if err != nil {
		http.Error(w, "Failed to serialize merged config", http.StatusInternalServerError)
		return
	}

	// Unmarshal merged config into ChannelConfig struct for validation
	var finalConfig models.ChannelConfig
	if err := json.Unmarshal(configJSON, &finalConfig); err != nil {
		http.Error(w, "Failed to parse merged config", http.StatusInternalServerError)
		return
	}

	// Determine status based solely on whether the feature is enabled
	// Frontend is responsible for validating required fields before submission
	newStatus := "disabled"

	// Check if portal is enabled
	if finalConfig.PortalEnabled {
		newStatus = "enabled"
	}

	// Check if SMTP is configured (has host set)
	if finalConfig.SMTPHost != "" {
		newStatus = "enabled"
	}

	query := `
		UPDATE channels
		SET config = ?, status = ?, updated_at = ?
		WHERE id = ?
	`

	_, err = h.db.ExecWriteContext(ctx, query, string(configJSON), newStatus, time.Now(), id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update channel config: %v", err), http.StatusInternalServerError)
		return
	}

	// Log audit event if status changed (activation or deactivation)
	if oldStatus != newStatus {
		var actionType string
		if newStatus == "enabled" && (oldStatus == "disabled" || oldStatus == "" || oldStatus == "pending" || oldStatus == "configured") {
			actionType = logger.ActionChannelActivate
		} else if newStatus == "disabled" && (oldStatus == "enabled" || oldStatus == "configured") {
			actionType = logger.ActionChannelDeactivate
		}

		if actionType != "" {
			logger.LogAudit(h.db, logger.AuditEvent{
				UserID:       user.ID,
				Username:     user.Username,
				IPAddress:    r.RemoteAddr,
				UserAgent:    r.UserAgent(),
				ActionType:   actionType,
				ResourceType: logger.ResourceChannel,
				ResourceID:   &id,
				ResourceName: channelName,
				Details: map[string]interface{}{
					"old_status": oldStatus,
					"new_status": newStatus,
				},
				Success: true,
			})
		}
	}

	// Return success response
	response := map[string]interface{}{
		"success": true,
		"message": "Channel configuration updated successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetChannelManagers returns all managers for a channel
func (h *ChannelHandler) GetChannelManagers(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Verify channel exists
	var exists bool
	checkQuery := "SELECT EXISTS(SELECT 1 FROM channels WHERE id = ?)"
	err = h.db.QueryRowContext(ctx, checkQuery, channelID).Scan(&exists)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Channel not found", http.StatusNotFound)
		return
	}

	// Get managers with joined information
	query := `
		SELECT
			cm.id, cm.channel_id, cm.manager_type, cm.manager_id,
			cm.added_by, cm.created_at, cm.updated_at,
			CASE
				WHEN cm.manager_type = 'user' THEN (u.first_name || ' ' || u.last_name)
				WHEN cm.manager_type = 'group' THEN g.name
				ELSE NULL
			END as manager_name,
			CASE
				WHEN cm.manager_type = 'user' THEN u.email
				ELSE NULL
			END as manager_email,
			(added_by_user.first_name || ' ' || added_by_user.last_name) as added_by_name
		FROM channel_managers cm
		LEFT JOIN users u ON cm.manager_type = 'user' AND cm.manager_id = u.id
		LEFT JOIN groups g ON cm.manager_type = 'group' AND cm.manager_id = g.id
		LEFT JOIN users added_by_user ON cm.added_by = added_by_user.id
		WHERE cm.channel_id = ?
		ORDER BY cm.created_at ASC
	`

	rows, err := h.db.QueryContext(ctx, query, channelID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get managers: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var managers []models.ChannelManager
	for rows.Next() {
		var manager models.ChannelManager
		var addedBy sql.NullInt64
		var managerName sql.NullString
		var managerEmail sql.NullString
		var addedByName sql.NullString

		err := rows.Scan(
			&manager.ID, &manager.ChannelID, &manager.ManagerType, &manager.ManagerID,
			&addedBy, &manager.CreatedAt, &manager.UpdatedAt,
			&managerName, &managerEmail, &addedByName,
		)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to scan manager: %v", err), http.StatusInternalServerError)
			return
		}

		if addedBy.Valid {
			val := int(addedBy.Int64)
			manager.AddedBy = &val
		}
		if managerName.Valid {
			manager.ManagerName = managerName.String
		}
		if managerEmail.Valid {
			manager.ManagerEmail = managerEmail.String
		}
		if addedByName.Valid {
			manager.AddedByName = addedByName.String
		}

		managers = append(managers, manager)
	}

	if err = rows.Err(); err != nil {
		http.Error(w, fmt.Sprintf("Error reading managers: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(managers)
}

// AddChannelManager adds managers to a channel
func (h *ChannelHandler) AddChannelManager(w http.ResponseWriter, r *http.Request) {
	// Get current user
	user, ok := r.Context().Value("user").(*models.User)
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	channelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	var request models.ChannelManagerRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate request
	if request.ManagerType != "user" && request.ManagerType != "group" {
		http.Error(w, "manager_type must be 'user' or 'group'", http.StatusBadRequest)
		return
	}
	if len(request.ManagerIDs) == 0 {
		http.Error(w, "manager_ids must contain at least one ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get channel name for audit logging
	var channelName string
	nameQuery := "SELECT name FROM channels WHERE id = ?"
	err = h.db.QueryRowContext(ctx, nameQuery, channelID).Scan(&channelName)
	if err == sql.ErrNoRows {
		http.Error(w, "Channel not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Set added_by to current user
	addedBy := user.ID

	// Insert managers
	insertQuery := `
		INSERT OR IGNORE INTO channel_managers (channel_id, manager_type, manager_id, added_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	for _, managerID := range request.ManagerIDs {
		_, err := h.db.ExecWriteContext(ctx, insertQuery,
			channelID, request.ManagerType, managerID, addedBy, now, now,
		)
		if err != nil {
			// Check if it's a foreign key violation (user/group doesn't exist)
			if strings.Contains(err.Error(), "FOREIGN KEY") || strings.Contains(err.Error(), "foreign key") {
				http.Error(w, fmt.Sprintf("Invalid %s ID: %d does not exist", request.ManagerType, managerID), http.StatusBadRequest)
				return
			}
			http.Error(w, fmt.Sprintf("Failed to add manager: %v", err), http.StatusInternalServerError)
			return
		}

		// Get manager name for audit log
		var managerName string
		if request.ManagerType == "user" {
			var firstName, lastName string
			nameQuery := "SELECT first_name, last_name FROM users WHERE id = ?"
			err = h.db.QueryRowContext(ctx, nameQuery, managerID).Scan(&firstName, &lastName)
			if err == nil {
				managerName = fmt.Sprintf("%s %s", firstName, lastName)
			}
		} else if request.ManagerType == "group" {
			nameQuery := "SELECT name FROM groups WHERE id = ?"
			err = h.db.QueryRowContext(ctx, nameQuery, managerID).Scan(&managerName)
		}

		// Log audit event
		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       user.ID,
			Username:     user.Username,
			IPAddress:    r.RemoteAddr,
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionChannelAddManager,
			ResourceType: logger.ResourceChannelManager,
			ResourceID:   &channelID,
			ResourceName: channelName,
			Details: map[string]interface{}{
				"manager_type": request.ManagerType,
				"manager_id":   managerID,
				"manager_name": managerName,
			},
			Success: true,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Added %d manager(s) to channel", len(request.ManagerIDs)),
	})
}

// RemoveChannelManager removes a manager from a channel
func (h *ChannelHandler) RemoveChannelManager(w http.ResponseWriter, r *http.Request) {
	// Get current user
	user, ok := r.Context().Value("user").(*models.User)
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	channelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	managerID, err := strconv.Atoi(r.PathValue("managerId"))
	if err != nil {
		http.Error(w, "Invalid manager ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get channel name for audit logging
	var channelName string
	nameQuery := "SELECT name FROM channels WHERE id = ?"
	err = h.db.QueryRowContext(ctx, nameQuery, channelID).Scan(&channelName)
	if err == sql.ErrNoRows {
		http.Error(w, "Channel not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Get manager info before deleting for audit log
	var managerType string
	var actualManagerID int
	managerInfoQuery := "SELECT manager_type, manager_id FROM channel_managers WHERE id = ? AND channel_id = ?"
	err = h.db.QueryRowContext(ctx, managerInfoQuery, managerID, channelID).Scan(&managerType, &actualManagerID)
	if err == sql.ErrNoRows {
		http.Error(w, "Manager not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get manager info: %v", err), http.StatusInternalServerError)
		return
	}

	// Get manager name for audit log
	var managerName string
	if managerType == "user" {
		var firstName, lastName string
		nameQuery := "SELECT first_name, last_name FROM users WHERE id = ?"
		err = h.db.QueryRowContext(ctx, nameQuery, actualManagerID).Scan(&firstName, &lastName)
		if err == nil {
			managerName = fmt.Sprintf("%s %s", firstName, lastName)
		}
	} else if managerType == "group" {
		nameQuery := "SELECT name FROM groups WHERE id = ?"
		err = h.db.QueryRowContext(ctx, nameQuery, actualManagerID).Scan(&managerName)
	}

	// Delete the manager
	deleteQuery := "DELETE FROM channel_managers WHERE id = ? AND channel_id = ?"
	result, err := h.db.ExecWriteContext(ctx, deleteQuery, managerID, channelID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to remove manager: %v", err), http.StatusInternalServerError)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to verify deletion: %v", err), http.StatusInternalServerError)
		return
	}

	if rowsAffected == 0 {
		http.Error(w, "Manager not found", http.StatusNotFound)
		return
	}

	// Log audit event
	logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    r.RemoteAddr,
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionChannelRemoveManager,
		ResourceType: logger.ResourceChannelManager,
		ResourceID:   &channelID,
		ResourceName: channelName,
		Details: map[string]interface{}{
			"manager_type": managerType,
			"manager_id":   actualManagerID,
			"manager_name": managerName,
		},
		Success: true,
	})

	w.WriteHeader(http.StatusNoContent)
}

// ProcessEmailsNow triggers immediate processing of emails for an inbound email channel.
// This is primarily used for testing to avoid waiting for the scheduler interval.
// POST /api/channels/{id}/process-emails
func (h *ChannelHandler) ProcessEmailsNow(w http.ResponseWriter, r *http.Request) {
	// Get current user
	user, ok := r.Context().Value("user").(*models.User)
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	// Check if user is a system admin
	isSystemAdmin, err := h.permissionService.IsSystemAdmin(user.ID)
	if err != nil || !isSystemAdmin {
		http.Error(w, "Admin access required", http.StatusForbidden)
		return
	}

	// Get channel ID from path
	channelIDStr := r.PathValue("id")
	channelID, err := strconv.Atoi(channelIDStr)
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	// Verify channel exists and is an inbound email channel
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var channelType, direction string
	err = h.db.QueryRowContext(ctx, `
		SELECT type, direction FROM channels WHERE id = ?
	`, channelID).Scan(&channelType, &direction)
	if err == sql.ErrNoRows {
		http.Error(w, "Channel not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to query channel: %v", err), http.StatusInternalServerError)
		return
	}

	if channelType != "email" || direction != "inbound" {
		http.Error(w, "Channel is not an inbound email channel", http.StatusBadRequest)
		return
	}

	// Check if email scheduler is available
	if h.emailScheduler == nil {
		http.Error(w, "Email scheduler not available", http.StatusServiceUnavailable)
		return
	}

	// Trigger processing
	err = h.emailScheduler.ProcessChannelNow(channelID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to process channel: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"channel_id": channelID,
		"message":    "Email processing triggered",
	})
}

// Default OAuth scopes for email providers
var defaultEmailOAuthScopes = map[string][]string{
	"microsoft": {
		"https://outlook.office365.com/IMAP.AccessAsUser.All",
		"https://outlook.office365.com/SMTP.Send",
		"openid",
		"profile",
		"email",
		"offline_access",
	},
	"google": {
		"https://mail.google.com/",
		"openid",
		"email",
		"profile",
	},
}

// StartChannelEmailOAuth initiates OAuth flow using channel's inline credentials
// POST /api/channels/{id}/email-oauth/start
func (h *ChannelHandler) StartChannelEmailOAuth(w http.ResponseWriter, r *http.Request) {
	// Get user ID
	user, ok := r.Context().Value("user").(*models.User)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get channel ID
	channelIDStr := r.PathValue("id")
	channelID, err := strconv.Atoi(channelIDStr)
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get channel config
	var configJSON string
	var channelType string
	err = h.db.QueryRowContext(ctx, `
		SELECT config, type FROM channels WHERE id = ?
	`, channelID).Scan(&configJSON, &channelType)
	if err == sql.ErrNoRows {
		http.Error(w, "Channel not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get channel: %v", err), http.StatusInternalServerError)
		return
	}

	if channelType != "email" {
		http.Error(w, "Channel is not an email channel", http.StatusBadRequest)
		return
	}

	// Parse config
	var config models.ChannelConfig
	if configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			http.Error(w, "Failed to parse channel config", http.StatusInternalServerError)
			return
		}
	}

	// Validate inline OAuth credentials
	if config.EmailOAuthProviderType == "" {
		http.Error(w, "OAuth provider type not configured", http.StatusBadRequest)
		return
	}
	if config.EmailOAuthClientID == "" {
		http.Error(w, "OAuth client ID not configured", http.StatusBadRequest)
		return
	}
	if config.EmailOAuthClientSecret == "" {
		http.Error(w, "OAuth client secret not configured", http.StatusBadRequest)
		return
	}

	// Decrypt client secret
	var clientSecret string
	if h.encryption != nil {
		clientSecret, err = h.encryption.Decrypt(config.EmailOAuthClientSecret)
		if err != nil {
			http.Error(w, "Failed to decrypt client secret", http.StatusInternalServerError)
			return
		}
	} else {
		clientSecret = config.EmailOAuthClientSecret
	}

	// Generate state token
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}
	state := hex.EncodeToString(stateBytes)

	// Store state in database (expires in 5 minutes)
	// Reuse the email_oauth_state table but with provider_id = 0 for inline OAuth
	expiresAt := time.Now().Add(5 * time.Minute)
	_, err = h.db.ExecWriteContext(ctx, `
		INSERT INTO email_oauth_state (provider_id, channel_id, state, user_id, expires_at)
		VALUES (0, ?, ?, ?, ?)
	`, channelID, state, user.ID, expiresAt)
	if err != nil {
		http.Error(w, "Failed to store OAuth state", http.StatusInternalServerError)
		return
	}

	// Build redirect URI
	redirectURI := fmt.Sprintf("%s/api/channels/inline-oauth/callback", h.baseURL)

	// Get OAuth URL based on provider type
	var authURL string
	scopes := defaultEmailOAuthScopes[config.EmailOAuthProviderType]

	switch config.EmailOAuthProviderType {
	case "microsoft":
		tenant := config.EmailOAuthTenantID
		if tenant == "" {
			tenant = "common"
		}
		p := email.NewMicrosoftProvider(config.EmailOAuthClientID, clientSecret, tenant, scopes)
		authURL = p.GetOAuthURL(state, redirectURI)
	case "google":
		p := email.NewGoogleProvider(config.EmailOAuthClientID, clientSecret, scopes)
		authURL = p.GetOAuthURL(state, redirectURI)
	default:
		http.Error(w, "Unsupported OAuth provider type", http.StatusBadRequest)
		return
	}

	slog.Info("starting inline OAuth flow for email channel",
		"channel_id", channelID,
		"provider_type", config.EmailOAuthProviderType,
		"user_id", user.ID,
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"auth_url": authURL,
	})
}

// ChannelEmailOAuthCallback handles the OAuth callback for channel-level OAuth
// GET /api/channels/email-oauth/callback
func (h *ChannelHandler) ChannelEmailOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")

	if errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		slog.Error("OAuth error", "error", errorParam, "description", errorDesc)
		// URL-encode the error parameter to prevent open redirect attacks
		http.Redirect(w, r, "/channels?oauth_error="+url.QueryEscape(errorParam), http.StatusFound)
		return
	}

	if code == "" || state == "" {
		http.Error(w, "Missing code or state parameter", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Validate state and get associated data
	var providerID, channelID, userID int
	err := h.db.QueryRowContext(ctx, `
		SELECT provider_id, channel_id, user_id
		FROM email_oauth_state
		WHERE state = ? AND expires_at > CURRENT_TIMESTAMP
	`, state).Scan(&providerID, &channelID, &userID)
	if err == sql.ErrNoRows {
		http.Error(w, "Invalid or expired state", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "Failed to validate state", http.StatusInternalServerError)
		return
	}

	// Delete used state
	h.db.ExecWriteContext(ctx, `DELETE FROM email_oauth_state WHERE state = ?`, state)

	// Get channel config
	var configJSON string
	err = h.db.QueryRowContext(ctx, `SELECT config FROM channels WHERE id = ?`, channelID).Scan(&configJSON)
	if err != nil {
		slog.Error("failed to get channel config", "error", err, "channel_id", channelID)
		http.Redirect(w, r, "/channels?oauth_error=channel_not_found", http.StatusFound)
		return
	}

	var config models.ChannelConfig
	if configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			http.Redirect(w, r, "/channels?oauth_error=invalid_config", http.StatusFound)
			return
		}
	}

	// Decrypt client secret
	var clientSecret string
	if h.encryption != nil && config.EmailOAuthClientSecret != "" {
		clientSecret, _ = h.encryption.Decrypt(config.EmailOAuthClientSecret)
	}

	// Build redirect URI (must match the one used in StartOAuth)
	redirectURI := fmt.Sprintf("%s/api/channels/inline-oauth/callback", h.baseURL)

	// Exchange code for tokens
	var tokens *email.OAuthTokens
	var userEmail string
	scopes := defaultEmailOAuthScopes[config.EmailOAuthProviderType]

	switch config.EmailOAuthProviderType {
	case "microsoft":
		tenant := config.EmailOAuthTenantID
		if tenant == "" {
			tenant = "common"
		}
		p := email.NewMicrosoftProvider(config.EmailOAuthClientID, clientSecret, tenant, scopes)
		tokens, err = p.ExchangeCode(ctx, code, redirectURI)
		if err != nil {
			slog.Error("failed to exchange code", "error", err)
			http.Redirect(w, r, "/channels?oauth_error=exchange_failed", http.StatusFound)
			return
		}
		userEmail, _ = p.GetUserEmail(ctx, tokens.AccessToken)

	case "google":
		p := email.NewGoogleProvider(config.EmailOAuthClientID, clientSecret, scopes)
		tokens, err = p.ExchangeCode(ctx, code, redirectURI)
		if err != nil {
			slog.Error("failed to exchange code", "error", err)
			http.Redirect(w, r, "/channels?oauth_error=exchange_failed", http.StatusFound)
			return
		}
		userEmail, _ = p.GetUserEmail(ctx, tokens.AccessToken)

	default:
		http.Redirect(w, r, "/channels?oauth_error=unsupported_provider", http.StatusFound)
		return
	}

	// Save tokens to channel config
	credManager := email.NewCredentialManager(h.db, h.encryption)
	err = credManager.SaveOAuthTokens(ctx, channelID, tokens, userEmail)
	if err != nil {
		slog.Error("failed to save tokens", "error", err)
		http.Redirect(w, r, "/channels?oauth_error=save_failed", http.StatusFound)
		return
	}

	slog.Info("OAuth completed for email channel (inline credentials)",
		"channel_id", channelID,
		"email", userEmail,
		"user_id", userID,
	)

	// Redirect back to channel config
	http.Redirect(w, r, fmt.Sprintf("/channels/%d?oauth_success=true", channelID), http.StatusFound)
}