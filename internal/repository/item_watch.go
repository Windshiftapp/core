package repository

import (
	"fmt"
	"time"

	"windshift/internal/database"
)

// IsWatching checks if a user is watching an item
func (r *ItemRepository) IsWatching(userID, itemID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM item_watches WHERE user_id = ? AND item_id = ?)
	`, userID, itemID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check watch status: %w", err)
	}
	return exists, nil
}

// Watch adds a user watch for an item
func (r *ItemRepository) Watch(userID, itemID int) error {
	_, err := r.db.Exec(`
		INSERT INTO item_watches (user_id, item_id, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT (user_id, item_id) DO NOTHING
	`, userID, itemID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to add watch: %w", err)
	}
	return nil
}

// Unwatch removes a user watch for an item
func (r *ItemRepository) Unwatch(userID, itemID int) error {
	_, err := r.db.Exec(`
		DELETE FROM item_watches WHERE user_id = ? AND item_id = ?
	`, userID, itemID)
	if err != nil {
		return fmt.Errorf("failed to remove watch: %w", err)
	}
	return nil
}

// GetWatchers returns all user IDs watching an item
func (r *ItemRepository) GetWatchers(itemID int) ([]int, error) {
	rows, err := r.db.Query(`
		SELECT user_id FROM item_watches WHERE item_id = ?
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to get watchers: %w", err)
	}
	defer rows.Close()

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

// GetUserWatchedItems returns all item IDs watched by a user
func (r *ItemRepository) GetUserWatchedItems(userID int) ([]int, error) {
	rows, err := r.db.Query(`
		SELECT item_id FROM item_watches WHERE user_id = ?
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get watched items: %w", err)
	}
	defer rows.Close()

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

// DeleteItemWatches removes all watches for an item (used when deleting item)
func (r *ItemRepository) DeleteItemWatches(tx database.Tx, itemID int) error {
	_, err := tx.Exec("DELETE FROM item_watches WHERE item_id = ?", itemID)
	if err != nil {
		return fmt.Errorf("failed to delete item watches: %w", err)
	}
	return nil
}
