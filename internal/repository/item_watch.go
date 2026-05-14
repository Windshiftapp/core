package repository

import (
	"fmt"

	"windshift/internal/database"
)

// IsWatching reports whether the user has an active watch on the item.
// Inactive (soft-deleted) watches are treated as not watching.
func (r *ItemRepository) IsWatching(userID, itemID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM item_watches
			WHERE user_id = ? AND item_id = ? AND is_active = true
		)
	`, userID, itemID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check watch status: %w", err)
	}
	return exists, nil
}

// Watch upserts an active watch for the (user, item) pair. If a soft-deleted
// watch row exists from a previous Unwatch, it is reactivated.
func (r *ItemRepository) Watch(userID, itemID int) error {
	_, err := r.db.Exec(`
		INSERT INTO item_watches (user_id, item_id, is_active, created_at, updated_at)
		VALUES (?, ?, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, item_id) DO UPDATE SET
			is_active = true,
			updated_at = CURRENT_TIMESTAMP
	`, userID, itemID)
	if err != nil {
		return fmt.Errorf("failed to add watch: %w", err)
	}
	return nil
}

// Unwatch soft-deletes the watch by flipping is_active to false, matching the
// semantics ActivityTracker uses; keeps the row so re-watching can preserve
// any watch_reason history.
func (r *ItemRepository) Unwatch(userID, itemID int) error {
	_, err := r.db.Exec(`
		UPDATE item_watches
		SET is_active = false, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = ? AND item_id = ?
	`, userID, itemID)
	if err != nil {
		return fmt.Errorf("failed to remove watch: %w", err)
	}
	return nil
}

// GetWatchers returns user IDs with an active watch on the item.
func (r *ItemRepository) GetWatchers(itemID int) ([]int, error) {
	rows, err := r.db.Query(`
		SELECT user_id FROM item_watches
		WHERE item_id = ? AND is_active = true
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to get watchers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var userIDs []int
	for rows.Next() {
		var userID int
		if err := rows.Scan(&userID); err != nil {
			return nil, fmt.Errorf("failed to scan watcher: %w", err)
		}
		userIDs = append(userIDs, userID)
	}

	return userIDs, nil
}

// GetUserWatchedItems returns item IDs the user has an active watch on.
func (r *ItemRepository) GetUserWatchedItems(userID int) ([]int, error) {
	rows, err := r.db.Query(`
		SELECT item_id FROM item_watches
		WHERE user_id = ? AND is_active = true
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get watched items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var itemIDs []int
	for rows.Next() {
		var itemID int
		if err := rows.Scan(&itemID); err != nil {
			return nil, fmt.Errorf("failed to scan watched item: %w", err)
		}
		itemIDs = append(itemIDs, itemID)
	}

	return itemIDs, nil
}

// DeleteItemWatches hard-deletes all watches for an item. Used during item
// deletion when the parent row is also being removed.
func (r *ItemRepository) DeleteItemWatches(tx database.Tx, itemID int) error {
	_, err := tx.Exec("DELETE FROM item_watches WHERE item_id = ?", itemID)
	if err != nil {
		return fmt.Errorf("failed to delete item watches: %w", err)
	}
	return nil
}
