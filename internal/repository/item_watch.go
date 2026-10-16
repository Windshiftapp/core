package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
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

// Watch upserts an active watch for the (user, item) pair, recording the
// reason it was created. If a soft-deleted watch row exists from a previous
// Unwatch, it is reactivated and its reason updated.
func (r *ItemRepository) Watch(userID, itemID int, reason string) error {
	_, err := r.db.ExecWrite(`
		INSERT INTO item_watches (user_id, item_id, is_active, watch_reason, created_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, item_id) DO UPDATE SET
			is_active = ?,
			watch_reason = ?,
			updated_at = CURRENT_TIMESTAMP
	`, userID, itemID, true, reason, true, reason)
	if err != nil {
		return fmt.Errorf("failed to add watch: %w", err)
	}
	return nil
}

// Unwatch soft-deletes the watch by flipping is_active to false, keeping the
// row so re-watching can preserve any watch_reason history.
func (r *ItemRepository) Unwatch(userID, itemID int) error {
	_, err := r.db.ExecWrite(`
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate watchers: %w", err)
	}

	return userIDs, nil
}

// ListWatchers returns the users with an active watch on the item, joined
// with their profile fields, in the order they started watching.
func (r *ItemRepository) ListWatchers(itemID int) ([]models.ItemWatcher, error) {
	rows, err := r.db.Query(`
		SELECT u.id, u.first_name, u.last_name, u.email, u.username, u.avatar_url,
			w.watch_reason, w.updated_at
		FROM item_watches w
		JOIN users u ON u.id = w.user_id
		WHERE w.item_id = ? AND w.is_active = true
		ORDER BY w.updated_at ASC, u.id ASC
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to list watchers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	watchers := []models.ItemWatcher{}
	for rows.Next() {
		var w models.ItemWatcher
		if err := rows.Scan(&w.UserID, &w.FirstName, &w.LastName, &w.Email, &w.Username, &w.AvatarURL,
			&w.WatchReason, &w.WatchedAt); err != nil {
			return nil, fmt.Errorf("failed to scan watcher: %w", err)
		}
		watchers = append(watchers, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate watchers: %w", err)
	}
	return watchers, nil
}

// CountWatchers returns how many users have an active watch on the item.
func (r *ItemRepository) CountWatchers(itemID int) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM item_watches
		WHERE item_id = ? AND is_active = true
	`, itemID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count watchers: %w", err)
	}
	return count, nil
}

// IsActiveUser reports whether the user exists and is not deactivated, so a
// watch is never added on behalf of an account that cannot receive it.
func (r *ItemRepository) IsActiveUser(userID int) (bool, error) {
	var active bool
	err := r.db.QueryRow(`SELECT COALESCE(is_active, true) FROM users WHERE id = ?`, userID).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check user: %w", err)
	}
	return active, nil
}

// GetUserWatchedItems returns item IDs the user has an active watch on,
// newest watch first.
func (r *ItemRepository) GetUserWatchedItems(userID int) ([]int, error) {
	rows, err := r.db.Query(`
		SELECT item_id FROM item_watches
		WHERE user_id = ? AND is_active = true
		ORDER BY created_at DESC
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate watched items: %w", err)
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
