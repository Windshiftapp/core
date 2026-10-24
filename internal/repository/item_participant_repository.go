package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
)

// ItemParticipantRepository persists external request participants: portal
// customers attached to a work item alongside its assignee (WI-1136).
// Participants see customer-facing content only; internal collaborators are
// out of scope and never stored here.
type ItemParticipantRepository struct {
	db database.Database
}

// NewItemParticipantRepository creates a participant repository.
func NewItemParticipantRepository(db database.Database) *ItemParticipantRepository {
	return &ItemParticipantRepository{db: db}
}

// ListByItem returns an item's participants oldest-first, joining the customer
// name/email and the adding user's display name for API responses.
func (r *ItemParticipantRepository) ListByItem(itemID int) ([]models.ItemParticipant, error) {
	rows, err := r.db.Query(`
		SELECT p.id, p.item_id, p.portal_customer_id, p.added_by, p.created_at,
		       pc.name, pc.email,
		       COALESCE(NULLIF(TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), u.username, '') AS added_by_name
		FROM item_participants p
		JOIN portal_customers pc ON pc.id = p.portal_customer_id
		LEFT JOIN users u ON u.id = p.added_by
		WHERE p.item_id = ?
		ORDER BY p.created_at ASC, p.id ASC
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("list item participants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	participants := make([]models.ItemParticipant, 0)
	for rows.Next() {
		var p models.ItemParticipant
		var addedBy sql.NullInt64
		if err := rows.Scan(
			&p.ID, &p.ItemID, &p.PortalCustomerID, &addedBy, &p.CreatedAt,
			&p.CustomerName, &p.CustomerEmail, &p.AddedByName,
		); err != nil {
			return nil, fmt.Errorf("scan item participant: %w", err)
		}
		if addedBy.Valid {
			id := int(addedBy.Int64)
			p.AddedBy = &id
		}
		participants = append(participants, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate item participants: %w", err)
	}
	return participants, nil
}

// ListItemIDsForCustomer returns the item ids the customer participates in,
// newest first.
func (r *ItemParticipantRepository) ListItemIDsForCustomer(customerID int) ([]int, error) {
	rows, err := r.db.Query(`
		SELECT item_id FROM item_participants
		WHERE portal_customer_id = ?
		ORDER BY created_at DESC, id DESC
	`, customerID)
	if err != nil {
		return nil, fmt.Errorf("list items for participant: %w", err)
	}
	defer func() { _ = rows.Close() }()

	itemIDs := make([]int, 0)
	for rows.Next() {
		var itemID int
		if err := rows.Scan(&itemID); err != nil {
			return nil, fmt.Errorf("scan participant item: %w", err)
		}
		itemIDs = append(itemIDs, itemID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate participant items: %w", err)
	}
	return itemIDs, nil
}

// CustomerIDsForItem returns the portal customer ids participating in an item.
func (r *ItemParticipantRepository) CustomerIDsForItem(itemID int) ([]int, error) {
	rows, err := r.db.Query(`
		SELECT portal_customer_id FROM item_participants
		WHERE item_id = ?
		ORDER BY created_at ASC, id ASC
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("list item participant customers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	ids := make([]int, 0)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan item participant customer: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate item participant customers: %w", err)
	}
	return ids, nil
}

// EmailsForItem returns the participant email addresses of an item, used for
// the customer-facing outbound fan-out.
func (r *ItemParticipantRepository) EmailsForItem(itemID int) ([]string, error) {
	rows, err := r.db.Query(`
		SELECT pc.email
		FROM item_participants p
		JOIN portal_customers pc ON pc.id = p.portal_customer_id
		WHERE p.item_id = ?
		ORDER BY p.created_at ASC, p.id ASC
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("list item participant emails: %w", err)
	}
	defer func() { _ = rows.Close() }()

	emails := make([]string, 0)
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("scan item participant email: %w", err)
		}
		emails = append(emails, email)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate item participant emails: %w", err)
	}
	return emails, nil
}

// IsParticipant reports whether the customer participates in the item.
func (r *ItemParticipantRepository) IsParticipant(itemID, customerID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM item_participants
			WHERE item_id = ? AND portal_customer_id = ?
		)
	`, itemID, customerID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check item participant: %w", err)
	}
	return exists, nil
}

// IsParticipantEmail reports whether any participant of the item has the given
// email address (case-insensitive, trimmed).
func (r *ItemParticipantRepository) IsParticipantEmail(itemID int, email string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false, nil
	}
	var exists bool
	err := r.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM item_participants p
			JOIN portal_customers pc ON pc.id = p.portal_customer_id
			WHERE p.item_id = ? AND LOWER(pc.email) = ?
		)
	`, itemID, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check item participant email: %w", err)
	}
	return exists, nil
}

// WasParticipantRemoved reports whether the item's history records removing a
// participant with this email. Removal hard-deletes the participant row, so the
// history entry is the durable tombstone that keeps an explicit removal from
// being overridden by earlier thread participation.
func (r *ItemParticipantRepository) WasParticipantRemoved(itemID int, email string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false, nil
	}
	var exists bool
	err := r.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM item_history
			WHERE item_id = ? AND field_name = 'participant_removed' AND LOWER(old_value) = ?
		)
	`, itemID, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check participant removal history: %w", err)
	}
	return exists, nil
}

// Add attaches a customer to an item as a participant. It is idempotent: a
// duplicate add is a no-op and reports created=false.
func (r *ItemParticipantRepository) Add(itemID, customerID int, addedBy *int) (bool, error) {
	res, err := r.db.ExecWrite(`
		INSERT INTO item_participants (item_id, portal_customer_id, added_by, created_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (item_id, portal_customer_id) DO NOTHING
	`, itemID, customerID, addedBy)
	if err != nil {
		return false, fmt.Errorf("add item participant: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("add item participant rows: %w", err)
	}
	return affected > 0, nil
}

// Remove detaches a customer from an item. It reports whether a row was
// actually removed.
func (r *ItemParticipantRepository) Remove(itemID, customerID int) (bool, error) {
	res, err := r.db.ExecWrite(`
		DELETE FROM item_participants
		WHERE item_id = ? AND portal_customer_id = ?
	`, itemID, customerID)
	if err != nil {
		return false, fmt.Errorf("remove item participant: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("remove item participant rows: %w", err)
	}
	return affected > 0, nil
}

// DeleteForItem hard-deletes every participant of an item. Used when the item
// row is being removed inside an existing transaction.
func (r *ItemParticipantRepository) DeleteForItem(tx database.Tx, itemID int) error {
	if _, err := tx.Exec("DELETE FROM item_participants WHERE item_id = ?", itemID); err != nil {
		return fmt.Errorf("delete item participants: %w", err)
	}
	return nil
}
