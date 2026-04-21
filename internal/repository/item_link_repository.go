package repository

import (
	"fmt"
	"strings"

	"windshift/internal/database"
)

// ItemLinkPair represents a directed item-to-item link.
type ItemLinkPair struct {
	SourceID int
	TargetID int
}

// ItemLinkRepository provides data access methods for the item_links table.
type ItemLinkRepository struct {
	db database.Database
}

// NewItemLinkRepository creates a new ItemLinkRepository.
func NewItemLinkRepository(db database.Database) *ItemLinkRepository {
	return &ItemLinkRepository{db: db}
}

// FindItemToItemLinksWithin returns every item-to-item link whose source_id
// and target_id both fall within the provided set of item IDs. Returns an
// empty slice when itemIDs is empty.
func (r *ItemLinkRepository) FindItemToItemLinksWithin(itemIDs []int) ([]ItemLinkPair, error) {
	if len(itemIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(itemIDs))
	for i := range itemIDs {
		placeholders[i] = "?"
	}
	args := make([]interface{}, 0, len(itemIDs)*2)
	for _, id := range itemIDs {
		args = append(args, id)
	}
	for _, id := range itemIDs {
		args = append(args, id)
	}

	query := fmt.Sprintf(`
		SELECT source_id, target_id FROM item_links
		WHERE source_type = 'item' AND target_type = 'item'
		  AND source_id IN (%s) AND target_id IN (%s)`,
		strings.Join(placeholders, ","),
		strings.Join(placeholders, ","))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query item_links: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var pairs []ItemLinkPair
	for rows.Next() {
		var p ItemLinkPair
		if err := rows.Scan(&p.SourceID, &p.TargetID); err != nil {
			return nil, fmt.Errorf("scan item_link row: %w", err)
		}
		pairs = append(pairs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate item_links: %w", err)
	}
	return pairs, nil
}
