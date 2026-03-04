package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"windshift/internal/models"
	"windshift/internal/utils"
)

// GetItemHistory returns the history of changes for a specific item
func (h *ItemHandler) GetItemHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	// First, get the item to check workspace ownership and permissions
	var workspaceID int
	err := h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", id).Scan(&workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check if user has permission to view items in this workspace
	canView, permErr := h.canViewItem(user.ID, workspaceID)
	if permErr != nil {
		respondInternalError(w, r, permErr)
		return
	}
	if !canView {
		respondNotFound(w, r, "Item")
		return
	}

	// Fetch history with user information
	query := `
		SELECT
			ih.id, ih.item_id, ih.user_id, ih.changed_at, ih.field_name, ih.old_value, ih.new_value,
			COALESCE(u.first_name || ' ' || u.last_name, u.username) as user_name,
			u.email as user_email
		FROM item_history ih
		LEFT JOIN users u ON ih.user_id = u.id
		WHERE ih.item_id = ?
		ORDER BY ih.changed_at DESC
	`

	rows, err := h.db.Query(query, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	history := []models.ItemHistory{}
	for rows.Next() {
		var entry models.ItemHistory
		err = rows.Scan(&entry.ID, &entry.ItemID, &entry.UserID, &entry.ChangedAt, &entry.FieldName, &entry.OldValue, &entry.NewValue, &entry.UserName, &entry.UserEmail)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		history = append(history, entry)
	}

	if err = rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Resolve ID values to human-readable names
	for i := range history {
		h.resolveHistoryValues(&history[i])
	}

	respondJSONOK(w, history)
}

// resolveHistoryValues resolves ID values to human-readable names based on field name
func (h *ItemHandler) resolveHistoryValues(entry *models.ItemHistory) {
	// Resolve old value if present
	if entry.OldValue != nil && *entry.OldValue != "" {
		if resolved := h.resolveValue(entry.FieldName, *entry.OldValue); resolved != "" {
			entry.ResolvedOldValue = &resolved
		}
	}

	// Resolve new value if present
	if entry.NewValue != nil && *entry.NewValue != "" {
		if resolved := h.resolveValue(entry.FieldName, *entry.NewValue); resolved != "" {
			entry.ResolvedNewValue = &resolved
		}
	}
}

// resolveValue resolves a single value based on field name
func (h *ItemHandler) resolveValue(fieldName, value string) string {
	id, err := strconv.Atoi(value)
	if err != nil {
		return ""
	}

	switch fieldName {
	case "assignee_id":
		return h.idResolver.ResolveUserName(id)
	case "priority_id":
		return h.idResolver.ResolvePriorityName(id)
	case "status_id":
		return h.idResolver.ResolveStatusName(id)
	case "parent_id":
		return h.idResolver.ResolveItemKey(id)
	case "project_id":
		return h.idResolver.ResolveProjectName(id)
	case "milestone_id":
		return h.idResolver.ResolveMilestoneName(id)
	case "item_type_id":
		return h.idResolver.ResolveItemTypeName(id)
	default:
		return ""
	}
}
