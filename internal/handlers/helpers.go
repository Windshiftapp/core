package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// respondJSON sends a JSON response with the given status code
func respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

// respondJSONOK sends a JSON response with 200 OK
func respondJSONOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

// respondJSONCreated sends a JSON response with 201 Created
func respondJSONCreated(w http.ResponseWriter, data interface{}) {
	respondJSON(w, http.StatusCreated, data)
}

// parseIDParam extracts and parses an integer ID from URL parameters
func parseIDParam(r *http.Request, paramName string) (int, error) {
	return strconv.Atoi(r.PathValue(paramName))
}

// requireIDParam parses ID and writes error response if invalid, returns 0 and false on error
func requireIDParam(w http.ResponseWriter, r *http.Request, paramName string) (int, bool) {
	id, err := parseIDParam(r, paramName)
	if err != nil {
		respondInvalidID(w, r, paramName)
		return 0, false
	}
	return id, true
}

// decodeJSON decodes a JSON request body into a value of type T.
// Returns the decoded value and true on success, or zero value and false on error
// (error response already written).
func decodeJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return v, false
	}
	return v, true
}

// requireWorkspaceIDParam resolves a workspace path parameter that may be a numeric ID or a workspace key.
// Returns the numeric workspace ID and true on success, or 0 and false if resolution fails (error already written).
func requireWorkspaceIDParam(w http.ResponseWriter, r *http.Request, cache *WorkspaceKeyCache, paramName string) (int, bool) {
	raw := r.PathValue(paramName)
	if raw == "" {
		respondBadRequest(w, r, "Workspace ID or key is required")
		return 0, false
	}
	id, ok := cache.Resolve(raw)
	if !ok {
		respondNotFound(w, r, "workspace")
		return 0, false
	}
	return id, true
}

// respondJSONWithWarnings sends a JSON response with warnings if any exist
// If there are warnings, the response is wrapped in {"data": ..., "warnings": [...]}
// If there are no warnings, the response is sent as-is for backward compatibility
func respondJSONWithWarnings(w http.ResponseWriter, statusCode int, data interface{}, warnings []models.APIWarning) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if len(warnings) > 0 {
		response := map[string]interface{}{
			"data":     data,
			"warnings": warnings,
		}
		_ = json.NewEncoder(w).Encode(response)
	} else {
		_ = json.NewEncoder(w).Encode(data)
	}
}

// respondJSONOKWithWarnings sends 200 OK with optional warnings
func respondJSONOKWithWarnings(w http.ResponseWriter, data interface{}, warnings []models.APIWarning) {
	respondJSONWithWarnings(w, http.StatusOK, data, warnings)
}

// respondJSONCreatedWithWarnings sends 201 Created with optional warnings
func respondJSONCreatedWithWarnings(w http.ResponseWriter, data interface{}, warnings []models.APIWarning) {
	respondJSONWithWarnings(w, http.StatusCreated, data, warnings)
}

// createCacheWarning creates a standardized cache invalidation warning
func createCacheWarning(cacheType string, err error, ctx string) models.APIWarning {
	return models.APIWarning{
		Code:    "cache_invalidation_failed",
		Message: fmt.Sprintf("Failed to invalidate %s cache: %v", cacheType, err),
		Context: ctx,
	}
}

// logAudit logs a successful resource action audit event.
func logAudit(db database.Database, r *http.Request, user *models.User, actionType, resourceType string, resourceID *int, resourceName string) {
	_ = logger.LogAudit(db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   actionType,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Success:      true,
	})
}

// logAuditWithDetails logs a successful resource action audit event with extra
// structured details (serialized to JSON in the audit log row).
func logAuditWithDetails(db database.Database, r *http.Request, user *models.User, actionType, resourceType string, resourceID *int, resourceName string, details map[string]interface{}) {
	_ = logger.LogAudit(db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   actionType,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Details:      details,
		Success:      true,
	})
}

// channelResult contains a resolved channel and its parsed config.
// Used by both PortalHandler and FormHandler to avoid duplicating the
// query-channels-by-slug lookup pattern.
type channelResult struct {
	channel models.Channel
	config  models.ChannelConfig
}

// findChannelBySlug queries all channels of the given type, parses each
// channel's JSON config, and returns the first enabled channel whose slug
// (extracted via slugFromConfig) matches the provided slug. This is the
// single implementation behind PortalHandler.findChannelByPortalSlug and
// FormHandler.findChannelByFormSlug.
func findChannelBySlug(ctx context.Context, db database.Database, channelType, slug string, slugFromConfig func(*models.ChannelConfig) string) (*channelResult, error) {
	query := `
		SELECT id, name, type, config, status
		FROM channels
		WHERE type = ?
		ORDER BY created_at DESC
	`

	rows, err := db.QueryContext(ctx, query, channelType)
	if err != nil {
		return nil, fmt.Errorf("failed to query %s channels: %w", channelType, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var ch models.Channel
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Type, &ch.Config, &ch.Status); err != nil {
			continue
		}

		var cfg models.ChannelConfig
		if ch.Config != "" {
			if err := json.Unmarshal([]byte(ch.Config), &cfg); err != nil {
				continue
			}
		}

		if slugFromConfig(&cfg) == slug && ch.Status == "enabled" {
			return &channelResult{channel: ch, config: cfg}, nil
		}
	}

	return nil, fmt.Errorf("%s channel not found", channelType)
}

// removeIDFromPortalSections loads the channel config, removes idToRemove from the portal section
// ID list accessed via getIDs/setIDs, and persists the updated config. Errors are silently ignored
// (best-effort cleanup).
func removeIDFromPortalSections(
	db database.Database,
	channelID int,
	idToRemove int,
	getIDs func(*models.PortalSection) []int,
	setIDs func(*models.PortalSection, []int),
) {
	var configStr string
	err := db.QueryRow("SELECT config FROM channels WHERE id = ?", channelID).Scan(&configStr)
	if err != nil || configStr == "" {
		return
	}
	var config models.ChannelConfig
	if err = json.Unmarshal([]byte(configStr), &config); err != nil {
		return
	}
	modified := false
	for i := range config.PortalSections {
		ids := getIDs(&config.PortalSections[i])
		newIDs := make([]int, 0, len(ids))
		for _, v := range ids {
			if v != idToRemove {
				newIDs = append(newIDs, v)
			} else {
				modified = true
			}
		}
		setIDs(&config.PortalSections[i], newIDs)
	}
	if modified {
		if updatedJSON, err := json.Marshal(config); err == nil {
			_, _ = db.ExecWrite("UPDATE channels SET config = ?, updated_at = ? WHERE id = ?",
				string(updatedJSON), time.Now(), channelID)
		}
	}
}

// visibilityInput holds the decoded visibility update request.
type visibilityInput struct {
	GroupIDs []int `json:"group_ids"`
	OrgIDs   []int `json:"org_ids"`
}

// decodeAndUpdateVisibility verifies the resource exists, decodes the visibility request,
// and updates the visibility columns in the given table. Returns true on success.
func decodeAndUpdateVisibility(w http.ResponseWriter, r *http.Request, db database.Database, table, resourceName string, id int) bool {
	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM "+table+" WHERE id = ?)", id).Scan(&exists)
	if err != nil || !exists {
		respondNotFound(w, r, resourceName)
		return false
	}

	var req visibilityInput
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return false
	}

	now := time.Now()
	_, err = db.ExecWrite(
		"UPDATE "+table+" SET visibility_group_ids = ?, visibility_org_ids = ?, updated_at = ? WHERE id = ?",
		serializeIntArray(req.GroupIDs), serializeIntArray(req.OrgIDs), now, id,
	)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	return true
}

// verifyResourceInWorkspace checks that a row with the given ID exists in the specified table
// and belongs to the given workspace. Returns true if found, or writes a 404 and returns false.
func verifyResourceInWorkspace(db database.Database, w http.ResponseWriter, r *http.Request, table string, resourceID, workspaceID int, resourceLabel string) bool {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE id = ? AND workspace_id = ?", resourceID, workspaceID).Scan(&count)
	if err != nil || count == 0 {
		respondNotFound(w, r, resourceLabel)
		return false
	}
	return true
}

// requireResourceInWorkspace parses workspaceId and the named resource ID from URL params,
// acquires a read DB, and verifies the resource belongs to the workspace. Returns the read DB,
// workspace ID, resource ID, and true on success. On failure the error response is already
// written and ok is false.
func (h *BaseHandler) requireResourceInWorkspace(w http.ResponseWriter, r *http.Request, table, resourceIDParam, resourceLabel string) (db database.Database, workspaceID, resourceID int, ok bool) {
	workspaceID, ok = requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	resourceID, ok = requireIDParam(w, r, resourceIDParam)
	if !ok {
		return
	}
	db, ok = h.requireReadDB(w, r)
	if !ok {
		return
	}
	if !verifyResourceInWorkspace(db, w, r, table, resourceID, workspaceID, resourceLabel) {
		return db, workspaceID, resourceID, false
	}
	return db, workspaceID, resourceID, true
}

// createDefaultAssetStatuses inserts the standard set of asset statuses for a
// newly created asset management set. Both AssetHandler and AssetStatusHandler
// delegate to this shared helper to avoid duplicating the status definitions.
func createDefaultAssetStatuses(db database.Database, setID int) error {
	now := time.Now()
	defaultStatuses := []struct {
		Name         string
		Color        string
		IsDefault    bool
		DisplayOrder int
	}{
		{"Active", "#22c55e", true, 0},
		{"Inactive", "#6b7280", false, 1},
		{"Maintenance", "#f59e0b", false, 2},
		{"Retired", "#ef4444", false, 3},
	}

	for _, s := range defaultStatuses {
		_, err := db.ExecWrite(`
			INSERT INTO asset_statuses (set_id, name, color, is_default, display_order, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, setID, s.Name, s.Color, s.IsDefault, s.DisplayOrder, now, now)
		if err != nil {
			return err
		}
	}

	return nil
}

// PaginationParams holds parsed pagination values.
type PaginationParams struct {
	Page   int
	Limit  int
	Offset int
}

// parseOffsetPagination extracts limit/offset from query params.
func parseOffsetPagination(r *http.Request, defaultLimit, maxLimit int) (limit, offset int) {
	limit = defaultLimit
	offset = 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= maxLimit {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	return limit, offset
}

// addDateRangeFilter appends date_from/date_to query parameter filters to a SQL query.
// Expects dates in "2006-01-02" format. Compares against a Unix timestamp column.
func addDateRangeFilter(r *http.Request, query *string, args *[]interface{}, column string) {
	if dateFrom := r.URL.Query().Get("date_from"); dateFrom != "" {
		t, err := time.Parse("2006-01-02", dateFrom)
		if err == nil {
			*query += " AND " + column + " >= ?"
			*args = append(*args, t.Unix())
		}
	}
	if dateTo := r.URL.Query().Get("date_to"); dateTo != "" {
		t, err := time.Parse("2006-01-02", dateTo)
		if err == nil {
			*query += " AND " + column + " <= ?"
			*args = append(*args, t.Add(24*time.Hour-time.Second).Unix())
		}
	}
}
