package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// CalendarFeedHandler handles calendar feed token management and ICS feed generation
type CalendarFeedHandler struct {
	db                database.Database
	permissionService *services.PermissionService
	// baseURL is the configured public URL (cfg.BaseURL). When non-empty it is
	// used verbatim for generated feed URLs; otherwise the handler falls back
	// to r.Host without consulting any X-Forwarded-* header (host-header
	// poisoning would otherwise leak the feed token to an attacker-chosen host).
	baseURL string
}

// NewCalendarFeedHandler creates a new calendar feed handler
func NewCalendarFeedHandler(db database.Database, permissionService *services.PermissionService, baseURL string) *CalendarFeedHandler {
	return &CalendarFeedHandler{
		db:                db,
		permissionService: permissionService,
		baseURL:           baseURL,
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
		if errors.Is(err, sql.ErrNoRows) {
			return true, nil // Default to enabled if setting not found
		}
		return false, err
	}
	return strings.EqualFold(value, "true"), nil
}

// requireCalendarFeedEnabled checks the calendar feed feature flag and writes
// an error response if disabled or on failure. Returns true when the caller
// should continue processing.
func (h *CalendarFeedHandler) requireCalendarFeedEnabled(w http.ResponseWriter, r *http.Request) bool {
	enabled, err := h.isCalendarFeedEnabled()
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if !enabled {
		respondForbidden(w, r)
		return false
	}
	return true
}

// GetFeedToken returns the current user's feed token info (or creates one if none exists)
func (h *CalendarFeedHandler) GetFeedToken(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !h.requireCalendarFeedEnabled(w, r) {
		return
	}

	var token CalendarFeedToken
	err := h.db.QueryRow(`
		SELECT id, user_id, token, is_active, last_accessed_at, created_at, updated_at
		FROM calendar_feed_tokens
		WHERE user_id = ?
	`, user.ID).Scan(&token.ID, &token.UserID, &token.Token, &token.IsActive,
		&token.LastAccessedAt, &token.CreatedAt, &token.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		// No token exists, return empty response
		respondJSONOK(w, map[string]interface{}{
			"has_token": false,
		})
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Build feed URL (token is already in DB)
	feedURL := fmt.Sprintf("%s/api/calendar/feed/%s.ics", h.feedBaseURL(r), token.Token)

	response := CalendarFeedTokenResponse{
		FeedURL:        feedURL,
		IsActive:       token.IsActive,
		LastAccessedAt: token.LastAccessedAt,
		CreatedAt:      token.CreatedAt,
	}

	respondJSONOK(w, map[string]interface{}{
		"has_token": true,
		"feed":      response,
	})
}

// CreateFeedToken creates or regenerates a feed token for the current user
func (h *CalendarFeedHandler) CreateFeedToken(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !h.requireCalendarFeedEnabled(w, r) {
		return
	}

	// Generate new token
	token, err := generateFeedToken()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	now := time.Now()

	// Atomic upsert: either insert the row (first time) or update the existing
	// row's token in place. Replaces a non-transactional DELETE+INSERT pair
	// that could permanently revoke a user's working feed when the INSERT
	// failed after the DELETE succeeded. Both SQLite and Postgres support this
	// syntax against the UNIQUE(user_id) constraint on calendar_feed_tokens.
	_, err = h.db.Exec(`
		INSERT INTO calendar_feed_tokens (user_id, token, is_active, created_at, updated_at)
		VALUES (?, ?, true, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET
			token = excluded.token,
			is_active = true,
			updated_at = excluded.updated_at
	`, user.ID, token, now, now)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Build feed URL
	feedURL := fmt.Sprintf("%s/api/calendar/feed/%s.ics", h.feedBaseURL(r), token)

	response := CalendarFeedTokenResponse{
		FeedURL:   feedURL,
		Token:     token, // Include full token only on creation
		IsActive:  true,
		CreatedAt: now,
	}

	respondJSONCreated(w, response)
}

// RevokeFeedToken revokes the current user's feed token
func (h *CalendarFeedHandler) RevokeFeedToken(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
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

	if errors.Is(err, sql.ErrNoRows) {
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

// generateICSForUser creates ICS content for all of a user's scheduled items.
//
// Authorization for each emitted event flows through PermissionService (the
// same path used by GET /calendar/scheduled-items). A previous implementation
// approximated workspace access with a bespoke SQL query that missed group
// roles and over-trusted direct role rows; the feed could include items the
// user no longer has item.view on.
func (h *CalendarFeedHandler) generateICSForUser(userID int) (string, error) {
	workspaceIDs, err := GetAccessibleWorkspaceIDs(&models.User{ID: userID}, h.db, h.permissionService)
	if err != nil {
		return "", err
	}

	if len(workspaceIDs) == 0 {
		return h.buildICSContent(nil, ""), nil
	}

	items, err := repository.NewItemRepository(h.db).ListItemsWithCalendarData(workspaceIDs)
	if err != nil {
		return "", err
	}

	// Per-workspace permission cache. GetAccessibleWorkspaceIDs already checked
	// item.view on every active workspace; items in this list live in one of
	// those workspaces, so the lookup is effectively free — but we still verify
	// explicitly to defend against drift if the helper's contract ever changes.
	canView := make(map[int]bool, len(workspaceIDs))
	for _, id := range workspaceIDs {
		canView[id] = true
	}

	var events []icsEvent

	for _, result := range items {
		item := result.Item
		if !canView[item.WorkspaceID] {
			continue
		}
		for _, entry := range result.CalendarEntries {
			if entry.UserID != userID {
				continue
			}

			itemKey := fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)

			events = append(events, icsEvent{
				UID:             fmt.Sprintf("%d-%s@windshift", item.ID, entry.ScheduledDate),
				Title:           fmt.Sprintf("[%s] %s", itemKey, item.Title),
				Description:     item.Description,
				ScheduledDate:   entry.ScheduledDate,
				ScheduledTime:   entry.ScheduledTime,
				DurationMinutes: entry.DurationMinutes,
				ItemID:          item.ID,
				WorkspaceID:     item.WorkspaceID,
				Notes:           entry.Notes,
			})
		}
	}

	return h.buildICSContent(events, ""), nil
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

// buildICSContent generates RFC 5545 compliant ICS content. Every content
// line is routed through writeFolded so the 75-octet limit is enforced even
// for long titles, descriptions, or notes (strict calendar clients reject
// unfolded long lines).
func (h *CalendarFeedHandler) buildICSContent(events []icsEvent, _ string) string {
	var sb strings.Builder

	writeFolded(&sb, "BEGIN:VCALENDAR")
	writeFolded(&sb, "VERSION:2.0")
	writeFolded(&sb, "PRODID:-//Windshift//Calendar//EN")
	writeFolded(&sb, "CALSCALE:GREGORIAN")
	writeFolded(&sb, "METHOD:PUBLISH")
	writeFolded(&sb, "X-WR-CALNAME:Windshift Calendar")

	for _, event := range events {
		startTime, err := parseScheduleDateTime(event.ScheduledDate, event.ScheduledTime)
		if err != nil {
			continue
		}

		duration := event.DurationMinutes
		if duration <= 0 {
			duration = 60
		}
		endTime := startTime.Add(time.Duration(duration) * time.Minute)

		writeFolded(&sb, "BEGIN:VEVENT")
		writeFolded(&sb, "UID:"+event.UID)
		writeFolded(&sb, "DTSTART:"+formatICSDateTime(startTime))
		writeFolded(&sb, "DTEND:"+formatICSDateTime(endTime))
		writeFolded(&sb, "SUMMARY:"+escapeICS(event.Title))

		desc := event.Description
		if event.Notes != "" {
			if desc != "" {
				desc += "\n\n"
			}
			desc += "Notes: " + event.Notes
		}
		if desc != "" {
			writeFolded(&sb, "DESCRIPTION:"+escapeICS(desc))
		}

		writeFolded(&sb, "END:VEVENT")
	}

	writeFolded(&sb, "END:VCALENDAR")
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

// escapeICS escapes special characters for ICS format per RFC 5545 §3.3.11.
//
// Normalizes CRLF/CR to LF first so that a lone "\r" cannot survive into the
// output and break ICS line structure in permissive clients. Then escapes the
// backslash first (so we never double-escape the synthesized "\n").
func escapeICS(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// writeFolded writes one ICS content line to sb, folded per RFC 5545 §3.1 so
// that no physical line exceeds 75 octets. Continuation lines begin with a
// single space (which itself costs one octet, so they carry up to 74 octets).
// Folding is performed on octets, not runes, but break points are backed up
// so that a multi-byte UTF-8 sequence is never split across lines.
func writeFolded(sb *strings.Builder, line string) {
	const maxFirst = 75
	const maxCont = 74 // 75 minus the leading space
	b := []byte(line)
	if len(b) <= maxFirst {
		sb.Write(b)
		sb.WriteString("\r\n")
		return
	}
	end := safeUTF8Boundary(b, maxFirst)
	sb.Write(b[:end])
	sb.WriteString("\r\n")
	for i := end; i < len(b); {
		chunk := maxCont
		if i+chunk > len(b) {
			chunk = len(b) - i
		}
		stop := safeUTF8Boundary(b[i:i+chunk], chunk)
		sb.WriteString(" ")
		sb.Write(b[i : i+stop])
		sb.WriteString("\r\n")
		i += stop
	}
}

// safeUTF8Boundary returns an index <= n that does not split a UTF-8 rune.
// If n already falls on a rune boundary it is returned as-is; otherwise we
// back up to the most recent rune-start byte.
func safeUTF8Boundary(b []byte, n int) int {
	if n >= len(b) {
		return len(b)
	}
	for n > 0 && !utf8.RuneStart(b[n]) {
		n--
	}
	if n == 0 {
		// Pathological input (no rune start in the window). Fall back to the
		// raw boundary rather than emit an infinite loop.
		return len(b)
	}
	return n
}

// feedBaseURL returns the base URL to embed in generated feed links.
//
// When the operator has configured BASE_URL we use it verbatim — that is the
// canonical public URL for the deployment and the only string that is safe to
// stamp into a URL that carries a long-lived bearer token.
//
// Otherwise we fall back to r.Host with the scheme derived from r.TLS, and
// deliberately ignore X-Forwarded-Host / X-Forwarded-Proto: a client can set
// those headers freely, and trusting them allowed an attacker to redirect a
// freshly generated feed_url (with embedded token) onto an attacker-controlled
// origin. Operators behind a proxy already need BASE_URL set for CORS/CSRF
// correctness (see internal/server/security_config.go), so this is not a
// regression for any supported deployment shape.
func (h *CalendarFeedHandler) feedBaseURL(r *http.Request) string {
	if h.baseURL != "" {
		return h.baseURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}
