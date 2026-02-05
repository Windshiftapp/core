package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// CalendarFeedHandler handles calendar feed token management and ICS feed generation
type CalendarFeedHandler struct {
	db                database.Database
	permissionService *services.PermissionService
}

// NewCalendarFeedHandler creates a new calendar feed handler
func NewCalendarFeedHandler(db database.Database, permissionService *services.PermissionService) *CalendarFeedHandler {
	return &CalendarFeedHandler{
		db:                db,
		permissionService: permissionService,
	}
}

// CalendarFeedToken represents a user's calendar feed token
type CalendarFeedToken struct {
	ID             int        `json:"id"`
	UserID         int        `json:"user_id"`
	Token          string     `json:"token,omitempty"` // Only returned on create
	IsActive       bool       `json:"is_active"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// CalendarFeedTokenResponse is the response when getting/creating a feed token
type CalendarFeedTokenResponse struct {
	FeedURL        string     `json:"feed_url"`
	Token          string     `json:"token,omitempty"` // Only returned on create
	IsActive       bool       `json:"is_active"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

const tokenPrefix = "cft_"
const tokenLength = 32

// generateFeedToken creates a new secure feed token
func generateFeedToken() (string, error) {
	bytes := make([]byte, tokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return tokenPrefix + hex.EncodeToString(bytes), nil
}

// isCalendarFeedEnabled checks if calendar feeds are enabled via system settings
func (h *CalendarFeedHandler) isCalendarFeedEnabled() (bool, error) {
	var value string
	err := h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'calendar_feed_enabled'").Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return true, nil // Default to enabled if setting not found
		}
		return false, err
	}
	return strings.EqualFold(value, "true"), nil
}

// GetFeedToken returns the current user's feed token info (or creates one if none exists)
func (h *CalendarFeedHandler) GetFeedToken(w http.ResponseWriter, r *http.Request) {
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Check if calendar feeds are enabled
	enabled, err := h.isCalendarFeedEnabled()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !enabled {
		respondForbidden(w, r)
		return
	}

	var token CalendarFeedToken
	err = h.db.QueryRow(`
		SELECT id, user_id, token, is_active, last_accessed_at, created_at, updated_at
		FROM calendar_feed_tokens
		WHERE user_id = ?
	`, user.ID).Scan(&token.ID, &token.UserID, &token.Token, &token.IsActive,
		&token.LastAccessedAt, &token.CreatedAt, &token.UpdatedAt)

	if err == sql.ErrNoRows {
		// No token exists, return empty response
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"has_token": false,
		})
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Build feed URL (token is already in DB)
	baseURL := getBaseURL(r)
	feedURL := fmt.Sprintf("%s/api/calendar/feed/%s.ics", baseURL, token.Token)

	response := CalendarFeedTokenResponse{
		FeedURL:        feedURL,
		IsActive:       token.IsActive,
		LastAccessedAt: token.LastAccessedAt,
		CreatedAt:      token.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"has_token": true,
		"feed":      response,
	})
}

// CreateFeedToken creates or regenerates a feed token for the current user
func (h *CalendarFeedHandler) CreateFeedToken(w http.ResponseWriter, r *http.Request) {
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Check if calendar feeds are enabled
	enabled, err := h.isCalendarFeedEnabled()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !enabled {
		respondForbidden(w, r)
		return
	}

	// Generate new token
	token, err := generateFeedToken()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Upsert token (delete existing and create new, or just create)
	now := time.Now()

	// Delete existing token if any
	_, _ = h.db.Exec("DELETE FROM calendar_feed_tokens WHERE user_id = ?", user.ID)

	// Insert new token
	_, err = h.db.Exec(`
		INSERT INTO calendar_feed_tokens (user_id, token, is_active, created_at, updated_at)
		VALUES (?, ?, true, ?, ?)
	`, user.ID, token, now, now)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Build feed URL
	baseURL := getBaseURL(r)
	feedURL := fmt.Sprintf("%s/api/calendar/feed/%s.ics", baseURL, token)

	response := CalendarFeedTokenResponse{
		FeedURL:   feedURL,
		Token:     token, // Include full token only on creation
		IsActive:  true,
		CreatedAt: now,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(response)
}

// RevokeFeedToken revokes the current user's feed token
func (h *CalendarFeedHandler) RevokeFeedToken(w http.ResponseWriter, r *http.Request) {
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	result, err := h.db.Exec("DELETE FROM calendar_feed_tokens WHERE user_id = ?", user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "calendar_feed_token")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ServeICSFeed serves the ICS calendar feed for a given token
// This endpoint does NOT require session auth - uses token auth instead
func (h *CalendarFeedHandler) ServeICSFeed(w http.ResponseWriter, r *http.Request) {
	tokenParam := r.PathValue("token")

	// Remove .ics extension if present
	token := strings.TrimSuffix(tokenParam, ".ics")

	// Validate token format
	if !strings.HasPrefix(token, tokenPrefix) {
		respondBadRequest(w, r, "Invalid token format")
		return
	}

	// Check if calendar feeds are enabled
	enabled, err := h.isCalendarFeedEnabled()
	if err != nil {
		respondServiceUnavailable(w, r, "Service unavailable")
		return
	}
	if !enabled {
		respondForbidden(w, r)
		return
	}

	// Look up token and get user
	var userID int
	var isActive bool
	err = h.db.QueryRow(`
		SELECT user_id, is_active FROM calendar_feed_tokens WHERE token = ?
	`, token).Scan(&userID, &isActive)

	if err == sql.ErrNoRows {
		respondUnauthorized(w, r)
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if !isActive {
		respondUnauthorized(w, r)
		return
	}

	// Update last_accessed_at
	_, _ = h.db.Exec("UPDATE calendar_feed_tokens SET last_accessed_at = ? WHERE token = ?", time.Now(), token)

	// Get user's scheduled items
	icsContent, err := h.generateICSForUser(userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Serve ICS content
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=windshift-calendar.ics")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	_, _ = w.Write([]byte(icsContent))
}

// generateICSForUser creates ICS content for all of a user's scheduled items
func (h *CalendarFeedHandler) generateICSForUser(userID int) (string, error) {
	// Get all workspaces accessible to this user
	workspaceIDs, err := h.getAccessibleWorkspaceIDs(userID)
	if err != nil {
		return "", err
	}

	if len(workspaceIDs) == 0 {
		return h.buildICSContent(nil, ""), nil
	}

	// Build workspace filter
	placeholders := make([]string, len(workspaceIDs))
	args := make([]interface{}, len(workspaceIDs))
	for i, id := range workspaceIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	// Query items with calendar data
	query := fmt.Sprintf(`
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.title, i.description,
		       i.calendar_data, w.name as workspace_name, w.key as workspace_key
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.calendar_data IS NOT NULL AND i.calendar_data != ''
		  AND i.workspace_id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()

	var events []icsEvent

	for rows.Next() {
		var itemID, workspaceID int
		var workspaceItemNumber sql.NullInt64
		var title, workspaceName, workspaceKey string
		var description sql.NullString
		var calendarDataJSON sql.NullString

		err := rows.Scan(&itemID, &workspaceID, &workspaceItemNumber, &title, &description,
			&calendarDataJSON, &workspaceName, &workspaceKey)
		if err != nil {
			continue
		}

		if !calendarDataJSON.Valid || calendarDataJSON.String == "" {
			continue
		}

		var calendarData []models.CalendarScheduleEntry
		if err := json.Unmarshal([]byte(calendarDataJSON.String), &calendarData); err != nil {
			continue
		}

		// Filter entries for this user
		for _, entry := range calendarData {
			if entry.UserID != userID {
				continue
			}

			// Build item key (e.g., "PROJ-123")
			itemKey := workspaceKey
			if workspaceItemNumber.Valid {
				itemKey = fmt.Sprintf("%s-%d", workspaceKey, workspaceItemNumber.Int64)
			}

			events = append(events, icsEvent{
				UID:             fmt.Sprintf("%d-%s@windshift", itemID, entry.ScheduledDate),
				Title:           fmt.Sprintf("[%s] %s", itemKey, title),
				Description:     description.String,
				ScheduledDate:   entry.ScheduledDate,
				ScheduledTime:   entry.ScheduledTime,
				DurationMinutes: entry.DurationMinutes,
				ItemID:          itemID,
				WorkspaceID:     workspaceID,
				Notes:           entry.Notes,
			})
		}
	}

	baseURL := "" // Will be empty for feed, client can't know external URL
	return h.buildICSContent(events, baseURL), nil
}

type icsEvent struct {
	UID             string
	Title           string
	Description     string
	ScheduledDate   string
	ScheduledTime   string
	DurationMinutes int
	ItemID          int
	WorkspaceID     int
	Notes           string
}

// buildICSContent generates RFC 5545 compliant ICS content
func (h *CalendarFeedHandler) buildICSContent(events []icsEvent, _ string) string {
	var sb strings.Builder

	// ICS header
	sb.WriteString("BEGIN:VCALENDAR\r\n")
	sb.WriteString("VERSION:2.0\r\n")
	sb.WriteString("PRODID:-//Windshift//Calendar//EN\r\n")
	sb.WriteString("CALSCALE:GREGORIAN\r\n")
	sb.WriteString("METHOD:PUBLISH\r\n")
	sb.WriteString("X-WR-CALNAME:Windshift Calendar\r\n")

	for _, event := range events {
		// Parse date and time
		startTime, err := parseScheduleDateTime(event.ScheduledDate, event.ScheduledTime)
		if err != nil {
			continue
		}

		duration := event.DurationMinutes
		if duration <= 0 {
			duration = 60 // Default 1 hour
		}
		endTime := startTime.Add(time.Duration(duration) * time.Minute)

		sb.WriteString("BEGIN:VEVENT\r\n")
		sb.WriteString(fmt.Sprintf("UID:%s\r\n", event.UID))
		sb.WriteString(fmt.Sprintf("DTSTART:%s\r\n", formatICSDateTime(startTime)))
		sb.WriteString(fmt.Sprintf("DTEND:%s\r\n", formatICSDateTime(endTime)))
		sb.WriteString(fmt.Sprintf("SUMMARY:%s\r\n", escapeICS(event.Title)))

		// Build description with notes and link
		desc := event.Description
		if event.Notes != "" {
			if desc != "" {
				desc += "\n\n"
			}
			desc += "Notes: " + event.Notes
		}
		if desc != "" {
			sb.WriteString(fmt.Sprintf("DESCRIPTION:%s\r\n", escapeICS(desc)))
		}

		sb.WriteString("END:VEVENT\r\n")
	}

	sb.WriteString("END:VCALENDAR\r\n")
	return sb.String()
}

// parseScheduleDateTime parses date (YYYY-MM-DD) and time (HH:MM) into a time.Time
func parseScheduleDateTime(date, timeStr string) (time.Time, error) {
	if timeStr == "" {
		timeStr = "09:00" // Default to 9 AM
	}
	combined := date + "T" + timeStr + ":00"
	return time.Parse("2006-01-02T15:04:05", combined)
}

// formatICSDateTime formats a time.Time to ICS format (YYYYMMDDTHHMMSS)
func formatICSDateTime(t time.Time) string {
	return t.Format("20060102T150405")
}

// escapeICS escapes special characters for ICS format
func escapeICS(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// getAccessibleWorkspaceIDs returns workspace IDs the user can access
func (h *CalendarFeedHandler) getAccessibleWorkspaceIDs(userID int) ([]int, error) {
	// Get all workspaces where user has view access
	rows, err := h.db.Query(`
		SELECT DISTINCT w.id FROM workspaces w
		WHERE w.is_active = true
		  AND (
		    -- User is creator
		    w.created_by = ?
		    -- Or user has explicit role assignment
		    OR EXISTS (
		      SELECT 1 FROM user_workspace_roles uwr
		      WHERE uwr.workspace_id = w.id AND uwr.user_id = ?
		    )
		    -- Or workspace has "everyone" role
		    OR w.everyone_role_id IS NOT NULL
		  )
	`, userID, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// getBaseURL extracts the base URL from the request
func getBaseURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		// Check X-Forwarded-Proto header for proxy setups
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		} else {
			scheme = "http"
		}
	}

	host := r.Host
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		host = forwardedHost
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}
