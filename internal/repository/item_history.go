package repository

import (
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// HistoryEntry represents a single field change in item history
type HistoryEntry struct {
	ID        int
	ItemID    int
	UserID    int
	FieldName string
	OldValue  string
	NewValue  string
	ChangedAt time.Time
}

// RecordHistory records a history entry for an item change
func (r *ItemRepository) RecordHistory(tx database.Tx, entry HistoryEntry) error {
	_, err := tx.Exec(`
		INSERT INTO item_history (item_id, user_id, field_name, old_value, new_value, changed_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, entry.ItemID, entry.UserID, entry.FieldName, entry.OldValue, entry.NewValue, entry.ChangedAt)
	if err != nil {
		return fmt.Errorf("failed to record history: %w", err)
	}
	return nil
}

// RecordHistoryBatch records multiple history entries in one operation
func (r *ItemRepository) RecordHistoryBatch(tx database.Tx, entries []HistoryEntry) error {
	for _, entry := range entries {
		if err := r.RecordHistory(tx, entry); err != nil {
			return err
		}
	}
	return nil
}

// GetHistory returns the history for an item
func (r *ItemRepository) GetHistory(itemID int, limit int) ([]HistoryEntry, error) {
	query := `
		SELECT id, item_id, user_id, field_name, old_value, new_value, changed_at
		FROM item_history
		WHERE item_id = ?
		ORDER BY changed_at DESC
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := r.db.Query(query, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var entry HistoryEntry
		if err := rows.Scan(&entry.ID, &entry.ItemID, &entry.UserID, &entry.FieldName, &entry.OldValue, &entry.NewValue, &entry.ChangedAt); err != nil {
			return nil, fmt.Errorf("failed to scan history entry: %w", err)
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// GetHistoryWithDetails returns history with user names resolved
func (r *ItemRepository) GetHistoryWithDetails(itemID int, limit int) ([]models.ItemHistory, error) {
	query := `
		SELECT h.id, h.item_id, h.user_id, h.field_name, h.old_value, h.new_value, h.changed_at,
		       u.first_name || ' ' || u.last_name as user_name, u.email as user_email
		FROM item_history h
		LEFT JOIN users u ON h.user_id = u.id
		WHERE h.item_id = ?
		ORDER BY h.changed_at DESC
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := r.db.Query(query, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to get history with details: %w", err)
	}
	defer rows.Close()

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
