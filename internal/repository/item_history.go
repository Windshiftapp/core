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

// HistoryEntry represents a single field change in item history
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
	// Source names the surface that produced the change when an agent acted on
	// a human's behalf (aitools.SourceAIChat, SourceMCP, SourceStandardAgent).
	// Empty for a direct cookie-auth write, which needs no explanation. The
	// column answers "did the AI do this?", so it is deliberately sparse
	// rather than re-encoding provenance every UI click already implies.
	Source string
}

// RecordHistory records a history entry for an item change
func (r *ItemRepository) RecordHistory(w HistoryWriter, entry HistoryEntry) error {
	var oldValue any = entry.OldValue
	if entry.OldValueNull {
		oldValue = nil
	}
	_, err := w.ExecWrite(`
		INSERT INTO item_history (item_id, user_id, field_name, old_value, new_value, changed_at, source)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, entry.ItemID, entry.UserID, entry.FieldName, oldValue, entry.NewValue, entry.ChangedAt,
		nullStringArg(entry.Source))
	if err != nil {
		return fmt.Errorf("failed to record history: %w", err)
	}
	return nil
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

// GetHistory returns the history for an item
func (r *ItemRepository) GetHistory(itemID, limit int) ([]HistoryEntry, error) {
	query := `
		SELECT id, item_id, user_id, field_name, old_value, new_value, changed_at
		FROM item_history
		WHERE item_id = ?
		ORDER BY changed_at DESC, id DESC
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
		var entry HistoryEntry
		if err := rows.Scan(&entry.ID, &entry.ItemID, &entry.UserID, &entry.FieldName, &entry.OldValue, &entry.NewValue, &entry.ChangedAt); err != nil {
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
		SELECT h.id, h.item_id, h.user_id, h.field_name, h.old_value, h.new_value, h.changed_at,
		       u.first_name || ' ' || u.last_name as user_name, u.email as user_email
		FROM item_history h
		LEFT JOIN users u ON h.user_id = u.id
		WHERE h.item_id = ?
		ORDER BY h.changed_at DESC, h.id DESC
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
		var entry models.ItemHistory
		var userName, userEmail string
		if err := rows.Scan(&entry.ID, &entry.ItemID, &entry.UserID, &entry.FieldName, &entry.OldValue, &entry.NewValue, &entry.ChangedAt, &userName, &userEmail); err != nil {
			return nil, fmt.Errorf("failed to scan history entry: %w", err)
		}
		entry.UserName = userName
		entry.UserEmail = userEmail
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate history entries with details: %w", err)
	}

	return entries, nil
}

// DeleteItemHistory removes all history for an item
func (r *ItemRepository) DeleteItemHistory(tx database.Tx, itemID int) error {
	_, err := tx.Exec("DELETE FROM item_history WHERE item_id = ?", itemID)
	if err != nil {
		return fmt.Errorf("failed to delete item history: %w", err)
	}
	return nil
}
