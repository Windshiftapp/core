package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/services"
)

// HubHandler handles HTTP requests for the Portal Hub
type HubHandler struct {
	db                database.Database
	permissionService *services.PermissionService
}

// NewHubHandler creates a new hub handler
func NewHubHandler(db database.Database, permissionService *services.PermissionService) *HubHandler {
	return &HubHandler{
		db:                db,
		permissionService: permissionService,
	}
}

// GetHub returns the hub configuration and all enabled portals
// GET /api/hub
func (h *HubHandler) GetHub(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get optional user context (may be nil for unauthenticated requests)
	user, _ := r.Context().Value(middleware.ContextKeyUser).(*models.User)

	var isAdmin bool
	var userGroupIDs []int
	if user != nil {
		isAdmin, _ = h.permissionService.IsSystemAdmin(user.ID)
		userGroupIDs = h.getUserGroupIDs(ctx, user.ID)
	}

	// Get hub configuration from system_settings
	var configJSON string
	err := h.db.QueryRowContext(ctx, `
		SELECT value FROM system_settings WHERE key = 'portal_hub_config'
	`).Scan(&configJSON)

	var config models.PortalHubConfig
	switch {
	case err == sql.ErrNoRows || configJSON == "":
		// Return default config
		config = models.PortalHubConfig{
			Title:             "Portal Hub",
			Description:       "",
			Gradient:          0,
			Theme:             "light",
			SearchPlaceholder: "Search portals...",
			SearchHint:        "",
			Sections:          []models.HubSection{},
			FooterColumns: []models.FooterColumn{
				{Title: "", Links: []struct {
					Text string `json:"text"`
					URL  string `json:"url"`
				}{}},
				{Title: "", Links: []struct {
					Text string `json:"text"`
					URL  string `json:"url"`
				}{}},
				{Title: "", Links: []struct {
					Text string `json:"text"`
					URL  string `json:"url"`
				}{}},
			},
		}
	case err != nil:
		respondInternalError(w, r, err)
		return
	default:
		if err = json.Unmarshal([]byte(configJSON), &config); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Get all enabled portal channels (filtered by user visibility)
	portals, err := h.getEnabledPortals(ctx, isAdmin, userGroupIDs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	response := models.HubResponse{
		Config:  config,
		Portals: portals,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// UpdateHubConfig updates the hub configuration
// PUT /api/hub/config
func (h *HubHandler) UpdateHubConfig(w http.ResponseWriter, r *http.Request) {
	// Get current user for permission check
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		respondUnauthorized(w, r)
		return
	}

	// Check if user is a system admin
	isSystemAdmin, err := h.permissionService.IsSystemAdmin(user.ID)
	if err != nil || !isSystemAdmin {
		respondAdminRequired(w, r)
		return
	}

	var config models.PortalHubConfig
	if err = json.NewDecoder(r.Body).Decode(&config); err != nil {
		respondBadRequest(w, r, "Invalid JSON")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Convert config to JSON
	configJSON, err := json.Marshal(config)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Upsert the hub configuration
	_, err = h.db.ExecWriteContext(ctx, `
		INSERT INTO system_settings (key, value, value_type, description, category)
		VALUES ('portal_hub_config', ?, 'json', 'Configuration for the Portal Hub central page', 'portal')
		ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = CURRENT_TIMESTAMP
	`, string(configJSON), string(configJSON))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Hub configuration saved successfully",
	})
}

// GetHubInbox returns paginated requests from all portals
// GET /api/hub/inbox
func (h *HubHandler) GetHubInbox(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Parse pagination params
	page := 1
	perPage := 20
	if p := r.URL.Query().Get("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if parsed, err := strconv.Atoi(pp); err == nil && parsed > 0 && parsed <= 100 {
			perPage = parsed
		}
	}

	// Optional filters
	portalID := r.URL.Query().Get("portal_id")
	statusFilter := r.URL.Query().Get("status")

	// Build base query
	baseQuery := `
		FROM items i
		JOIN statuses s ON i.status_id = s.id
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN channels c ON i.channel_id = c.id
		LEFT JOIN portal_customers pc ON i.creator_portal_customer_id = pc.id
		WHERE c.type = 'portal'
	`
	args := []interface{}{}

	// Add filters
	if portalID != "" {
		baseQuery += " AND c.id = ?"
		args = append(args, portalID)
	}
	if statusFilter != "" {
		baseQuery += " AND s.name = ?"
		args = append(args, statusFilter)
	}

	// Get total count
	var total int
	countQuery := "SELECT COUNT(DISTINCT i.id) " + baseQuery
	err := h.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Calculate pagination
	offset := (page - 1) * perPage
	totalPages := (total + perPage - 1) / perPage

	// Get items
	query := `
		SELECT
			i.id, i.title, COALESCE(i.description, ''), i.created_at,
			s.name, COALESCE(s.color, '#6b7280'),
			w.key, i.workspace_item_number,
			COALESCE(c.name, ''), COALESCE(JSON_EXTRACT(c.config, '$.portal_slug'), ''),
			pc.name, pc.email
	` + baseQuery + `
		ORDER BY i.created_at DESC
		LIMIT ? OFFSET ?
	`
	args = append(args, perPage, offset)

	rows, err := h.db.QueryContext(ctx, query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var items []models.HubInboxItem
	for rows.Next() {
		var item models.HubInboxItem
		var submitterName, submitterEmail sql.NullString
		err = rows.Scan(
			&item.ID, &item.Title, &item.Description, &item.CreatedAt,
			&item.StatusName, &item.StatusColor,
			&item.WorkspaceKey, &item.WorkspaceItemNumber,
			&item.PortalName, &item.PortalSlug,
			&submitterName, &submitterEmail,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if submitterName.Valid {
			item.SubmitterName = &submitterName.String
		}
		if submitterEmail.Valid {
			item.SubmitterEmail = &submitterEmail.String
		}
		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	response := models.HubInboxResponse{
		Items:      items,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// GetHubInboxItem returns a specific request detail
// GET /api/hub/inbox/:itemId
func (h *HubHandler) GetHubInboxItem(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.Atoi(r.PathValue("itemId"))
	if err != nil {
		respondInvalidID(w, r, "itemId")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT
			i.id, i.title, COALESCE(i.description, ''), i.created_at,
			s.name, COALESCE(s.color, '#6b7280'),
			w.key, i.workspace_item_number,
			COALESCE(c.name, ''), COALESCE(JSON_EXTRACT(c.config, '$.portal_slug'), ''),
			pc.name, pc.email
		FROM items i
		JOIN statuses s ON i.status_id = s.id
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN channels c ON i.channel_id = c.id
		LEFT JOIN portal_customers pc ON i.creator_portal_customer_id = pc.id
		WHERE i.id = ? AND c.type = 'portal'
	`

	var item models.HubInboxItem
	var submitterName, submitterEmail sql.NullString
	err = h.db.QueryRowContext(ctx, query, itemID).Scan(
		&item.ID, &item.Title, &item.Description, &item.CreatedAt,
		&item.StatusName, &item.StatusColor,
		&item.WorkspaceKey, &item.WorkspaceItemNumber,
		&item.PortalName, &item.PortalSlug,
		&submitterName, &submitterEmail,
	)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "item")
		return
	} else if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if submitterName.Valid {
		item.SubmitterName = &submitterName.String
	}
	if submitterEmail.Valid {
		item.SubmitterEmail = &submitterEmail.String
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(item)
}

// getUserGroupIDs returns the group IDs for a user
func (h *HubHandler) getUserGroupIDs(ctx context.Context, userID int) []int {
	rows, err := h.db.QueryContext(ctx, `SELECT group_id FROM group_members WHERE user_id = ?`, userID)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()

	var groupIDs []int
	for rows.Next() {
		var groupID int
		if err := rows.Scan(&groupID); err == nil {
			groupIDs = append(groupIDs, groupID)
		}
	}
	return groupIDs
}

// getEnabledPortals returns all enabled portal channels with metadata
// isAdmin: if true, shows all request types regardless of visibility
// userGroupIDs: internal user group IDs for visibility filtering
func (h *HubHandler) getEnabledPortals(ctx context.Context, isAdmin bool, userGroupIDs []int) ([]models.HubPortalInfo, error) {
	query := `
		SELECT
			c.id, c.name, c.description, c.status, c.config,
			(SELECT COUNT(*) FROM request_types rt WHERE rt.channel_id = c.id AND rt.is_active = true) as request_type_count
		FROM channels c
		WHERE c.type = 'portal' AND c.status = 'enabled'
		ORDER BY c.name ASC
	`

	rows, err := h.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var portals []models.HubPortalInfo
	var portalIDs []int
	for rows.Next() {
		var portal models.HubPortalInfo
		var description sql.NullString
		var configJSON string

		err = rows.Scan(
			&portal.ID, &portal.Name, &description, &portal.Status, &configJSON,
			&portal.RequestTypeCount,
		)
		if err != nil {
			return nil, err
		}

		if description.Valid {
			portal.Description = description.String
		}

		// Parse config to get slug and gradient
		if configJSON != "" {
			var config struct {
				PortalSlug               string `json:"portal_slug"`
				PortalGradient           int    `json:"portal_gradient"`
				PortalBackgroundImageURL string `json:"portal_background_image_url"`
			}
			if err = json.Unmarshal([]byte(configJSON), &config); err == nil {
				portal.Slug = config.PortalSlug
				portal.Gradient = config.PortalGradient
				portal.BackgroundImageURL = config.PortalBackgroundImageURL
			}
		}

		portals = append(portals, portal)
		portalIDs = append(portalIDs, portal.ID)
	}

	if err = rows.Err(); err != nil { //nolint:gocritic // Using = to avoid shadowing err from outer scope
		return nil, err
	}

	// Fetch request types for all portals (filtered by visibility)
	if len(portalIDs) > 0 {
		requestTypes, err := h.getRequestTypesForPortals(ctx, portalIDs, isAdmin, userGroupIDs)
		if err != nil {
			return nil, err
		}

		// Map request types to their portals
		for i := range portals {
			if rts, ok := requestTypes[portals[i].ID]; ok {
				portals[i].RequestTypes = rts
			}
		}
	}

	return portals, nil
}

// getRequestTypesForPortals fetches request types for multiple portal channel IDs
// Filters by visibility based on user context:
// - isAdmin=true: shows all request types
// - userGroupIDs non-empty: filters by visibility_group_ids
// - both false/empty: only shows request types with no visibility restrictions
func (h *HubHandler) getRequestTypesForPortals(ctx context.Context, portalIDs []int, isAdmin bool, userGroupIDs []int) (map[int][]models.HubPortalRequestType, error) {
	if len(portalIDs) == 0 {
		return nil, nil
	}

	// Build query with IN clause
	placeholders := make([]string, len(portalIDs))
	args := make([]interface{}, len(portalIDs))
	for i, id := range portalIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, channel_id, name, COALESCE(description, ''), COALESCE(icon, ''), COALESCE(color, ''),
		       visibility_group_ids, visibility_org_ids
		FROM request_types
		WHERE channel_id IN (%s) AND is_active = true
		ORDER BY display_order ASC
	`, strings.Join(placeholders, ","))

	rows, err := h.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int][]models.HubPortalRequestType)
	for rows.Next() {
		var rt models.HubPortalRequestType
		var channelID int
		var visGroupIDs, visOrgIDs *string
		err = rows.Scan(&rt.ID, &channelID, &rt.Name, &rt.Description, &rt.Icon, &rt.Color, &visGroupIDs, &visOrgIDs)
		if err != nil {
			return nil, err
		}

		// Create full RequestType to use IsVisibleTo method
		fullRT := models.RequestType{
			VisibilityGroupIDs: deserializeIntArray(visGroupIDs),
			VisibilityOrgIDs:   deserializeIntArray(visOrgIDs),
		}

		// Admin sees all, others filtered by visibility
		if isAdmin || fullRT.IsVisibleTo(userGroupIDs, nil) {
			result[channelID] = append(result[channelID], rt)
		}
	}

	if err = rows.Err(); err != nil { //nolint:gocritic // Using = to avoid shadowing err from outer scope
		return nil, err
	}

	return result, nil
}
