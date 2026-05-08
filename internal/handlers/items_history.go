package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// GetItemHistory returns the history of changes for a specific item
func (h *ItemHandler) GetItemHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// First, get the item to check workspace ownership and permissions
	workspaceID, err := repository.NewItemRepository(h.db).GetWorkspaceID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check if user has permission to view items in this workspace. Active
	// approvers without workspace item.view are allowed through so they can
	// inspect change history for context before deciding.
	canView, permErr := h.canViewItemAsActor(user.ID, id, workspaceID)
	if permErr != nil {
		respondInternalError(w, r, permErr)
		return
	}
	if !canView {
		respondNotFound(w, r, "Item")
		return
	}

	// Fetch history with user information. Approval-engine events live in
	// approval_decisions and are surfaced here too so the item's history tab
	// is the single chronological feed (approve/reject/comment text included).
	// Synthetic IDs for approval rows are negated to avoid colliding with
	// item_history IDs in the response.
	query := `
		SELECT
			ih.id, ih.item_id, ih.user_id, ih.changed_at, ih.field_name, ih.old_value, ih.new_value,
			COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as user_name,
			COALESCE(u.email, '') as user_email
		FROM item_history ih
		LEFT JOIN users u ON ih.user_id = u.id
		WHERE ih.item_id = ?
		UNION ALL
		SELECT
			-d.id AS id,
			ar.item_id,
			COALESCE(d.actor_user_id, 0) AS user_id,
			d.created_at AS changed_at,
			'approval_' || d.decision AS field_name,
			NULL AS old_value,
			d.comment AS new_value,
			COALESCE(u.first_name || ' ' || u.last_name, u.username, 'System') AS user_name,
			COALESCE(u.email, '') AS user_email
		FROM approval_decisions d
		JOIN approval_requests ar ON ar.id = d.approval_request_id
		LEFT JOIN users u ON u.id = d.actor_user_id
		WHERE ar.item_id = ?
		ORDER BY changed_at DESC
	`

	rows, err := h.db.Query(query, id, id)
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
	// Multi-milestone history rows store a comma-joined ID list in
	// old/new_value (e.g. "1,4,5"). Resolve each ID to a name before
	// the single-int Atoi path below.
	if fieldName == "milestones" {
		parts := strings.Split(value, ",")
		names := make([]string, 0, len(parts))
		for _, p := range parts {
			id, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil {
				continue
			}
			if name := h.idResolver.ResolveMilestoneName(id); name != "" {
				names = append(names, name)
			}
		}
		return strings.Join(names, ", ")
	}

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
	case "milestone_id": // legacy field name kept for old history rows
		return h.idResolver.ResolveMilestoneName(id)
	case "item_type_id":
		return h.idResolver.ResolveItemTypeName(id)
	default:
		return ""
	}
}
