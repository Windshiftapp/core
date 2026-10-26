package repository

import (
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// HistoryWriter is the minimal write surface item-history recording needs.
// Both database.Database and database.Tx satisfy it, so history rows can be
// recorded inside or outside a caller-owned transaction.
type HistoryWriter interface {
	ExecWrite(query string, args ...any) (sql.Result, error)
}

// Item-history actor kinds. Empty ActorKind is treated as a user actor so
// existing callers keep writing the same rows as before.
const (
	HistoryActorUser           = "user"
	HistoryActorPortalCustomer = "portal_customer"
	HistoryActorSystem         = "system"
)

// HistoryEntry represents a single field change in item history
//
//nolint:revive // field order mirrors the item_history column order for readability
type HistoryEntry struct {
	ID        int
	ItemID    int
	UserID    int
	FieldName string
	OldValue  string
	// OldValueNull preserves SQL NULL for history events with no prior value.
	OldValueNull bool
	NewValue     string
	ChangedAt    time.Time
	// ActorKind is one of the HistoryActor* constants; empty means user.
	ActorKind string
	// ActorPortalCustomerID carries the acting portal customer for
	// portal_customer entries; user_id stays NULL for those rows.
	ActorPortalCustomerID *int
	// Resolved display fields for read paths.
	UserName            string
	UserEmail           string
	PortalCustomerName  string
	PortalCustomerEmail string
	// Source names the surface that produced the change when an agent acted on
	// a human's behalf (aitools.SourceAIChat, SourceMCP, SourceStandardAgent).
	// Empty for a direct cookie-auth write, which needs no explanation. The
	// column answers "did the AI do this?", so it is deliberately sparse
	// rather than re-encoding provenance every UI click already implies.
	Source string
	// AgentRunID is the agent_runs row that produced this change, when an agent
	// made it. It is what lets the history feed report that turn's model and
	// cost instead of leaving the change unattributable. Always nil for a
	// direct write.
	AgentRunID *int
}

// RecordHistory records a history entry for an item change
func (r *ItemRepository) RecordHistory(w HistoryWriter, entry HistoryEntry) error {
	var oldValue any = entry.OldValue
	if entry.OldValueNull {
		oldValue = nil
	}
	var userID any
	var actorCustomerID any
	actorKind := entry.ActorKind
	if actorKind == "" {
		actorKind = HistoryActorUser
	}
	switch actorKind {
	case HistoryActorUser:
		if entry.UserID > 0 {
			userID = entry.UserID
		}
	case HistoryActorPortalCustomer:
		if entry.ActorPortalCustomerID != nil {
			actorCustomerID = *entry.ActorPortalCustomerID
		}
	case HistoryActorSystem:
		// No actor reference.
	default:
		return fmt.Errorf("unknown item history actor kind %q", actorKind)
	}
	_, err := w.ExecWrite(`
		INSERT INTO item_history (item_id, user_id, actor_kind, actor_portal_customer_id, field_name, old_value, new_value, changed_at, source, agent_run_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, entry.ItemID, userID, actorKind, actorCustomerID, entry.FieldName, oldValue, entry.NewValue, entry.ChangedAt,
		nullStringArg(entry.Source), nullIntArg(entry.AgentRunID))
	if err != nil {
		return fmt.Errorf("failed to record history: %w", err)
	}
	return touchItemHistoryMilestoneActivity(w, entry)
}

// RecordHistoryBatch records multiple history entries in one operation
func (r *ItemRepository) RecordHistoryBatch(w HistoryWriter, entries []HistoryEntry) error {
	for _, entry := range entries {
		if err := r.RecordHistory(w, entry); err != nil {
			return err
		}
	}
	return nil
}

// historySelectColumns is the shared projection for item-history reads:
// actor attribution with users and portal customers resolved to display
// names. All scans must use historyScanTargets so NULL user actors (portal
// customers, system actions) don't break row decoding.
const historySelectColumns = `
	ih.id, ih.item_id, ih.user_id, ih.actor_kind, ih.actor_portal_customer_id,
	ih.changed_at, ih.field_name, ih.old_value, ih.new_value,
	COALESCE(NULLIF(TRIM(u.first_name || ' ' || u.last_name), ''), u.username, '') AS user_name,
	COALESCE(u.email, '') AS user_email,
	COALESCE(NULLIF(TRIM(pc.name), ''), pc.email, '') AS portal_customer_name,
	COALESCE(pc.email, '') AS portal_customer_email
`

const historyFromClause = `
	FROM item_history ih
	LEFT JOIN users u ON ih.user_id = u.id
	LEFT JOIN portal_customers pc ON ih.actor_portal_customer_id = pc.id
`

// scanHistoryEntry scans one row projected by historySelectColumns. UserID is
// 0 and ActorKind carries the actor for non-user rows.
func scanHistoryEntry(scan func(dest ...any) error) (HistoryEntry, error) {
	var entry HistoryEntry
	var userID, portalCustomerID sql.NullInt64
	var actorKind sql.NullString
	var userName, userEmail, portalCustomerName, portalCustomerEmail sql.NullString
	if err := scan(&entry.ID, &entry.ItemID, &userID, &actorKind, &portalCustomerID, &entry.ChangedAt,
		&entry.FieldName, &entry.OldValue, &entry.NewValue, &userName, &userEmail,
		&portalCustomerName, &portalCustomerEmail); err != nil {
		return entry, err
	}
	if userID.Valid {
		entry.UserID = int(userID.Int64)
	}
	entry.ActorKind = actorKind.String
	if portalCustomerID.Valid {
		id := int(portalCustomerID.Int64)
		entry.ActorPortalCustomerID = &id
	}
	entry.UserName = userName.String
	entry.UserEmail = userEmail.String
	entry.PortalCustomerName = portalCustomerName.String
	entry.PortalCustomerEmail = portalCustomerEmail.String
	return entry, nil
}

// GetHistory returns the history for an item
func (r *ItemRepository) GetHistory(itemID, limit int) ([]HistoryEntry, error) {
	query := `
		SELECT ` + historySelectColumns + historyFromClause + `
		WHERE ih.item_id = ?
		ORDER BY ih.changed_at DESC, ih.id DESC
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := r.db.Query(query, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []HistoryEntry
	for rows.Next() {
		entry, err := scanHistoryEntry(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to scan history entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate history entries: %w", err)
	}

	return entries, nil
}

// GetHistoryWithDetails returns history with user names resolved
func (r *ItemRepository) GetHistoryWithDetails(itemID, limit int) ([]models.ItemHistory, error) {
	query := `
		SELECT ` + historySelectColumns + historyFromClause + `
		WHERE ih.item_id = ?
		ORDER BY ih.changed_at DESC, ih.id DESC
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := r.db.Query(query, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to get history with details: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []models.ItemHistory
	for rows.Next() {
		entry, err := scanHistoryEntry(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("failed to scan history entry: %w", err)
		}
		entries = append(entries, models.ItemHistory{
			ID: entry.ID, ItemID: entry.ItemID, UserID: entry.UserID,
			ActorKind: entry.ActorKind, PortalCustomerID: entry.ActorPortalCustomerID,
			PortalCustomerName: entry.PortalCustomerName, PortalCustomerEmail: entry.PortalCustomerEmail,
			ChangedAt: entry.ChangedAt, FieldName: entry.FieldName,
			OldValue: historyStringPtr(entry.OldValue, entry.OldValueNull),
			NewValue: historyStringPtr(entry.NewValue, false),
			UserName: entry.UserName, UserEmail: entry.UserEmail,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate history entries with details: %w", err)
	}

	return entries, nil
}

func historyStringPtr(value string, null bool) *string {
	if null {
		return nil
	}
	return &value
}

// DeleteItemHistory removes all history for an item
func (r *ItemRepository) DeleteItemHistory(tx database.Tx, itemID int) error {
	_, err := tx.Exec("DELETE FROM item_history WHERE item_id = ?", itemID)
	if err != nil {
		return fmt.Errorf("failed to delete item history: %w", err)
	}
	return nil
}
