package services

import (
	"database/sql"
	"errors"
	"fmt"

	"windshift/internal/database"
)

// ItemLinkService handles item link creation in the database.
// HTTP concerns (notifications, action events) remain in the handler.
type ItemLinkService struct {
	db database.Database
}

// NewItemLinkService creates a new ItemLinkService.
func NewItemLinkService(db database.Database) *ItemLinkService {
	return &ItemLinkService{db: db}
}

// CreateItemLinkParams contains the parameters for creating an item link.
type CreateItemLinkParams struct {
	LinkTypeID    int
	SourceType    string
	SourceID      int
	TargetType    string
	TargetID      int
	CreatedBy     *int
	CustomFieldID *int
}

// ErrInvalidLinkTypeForEntities is returned when the requested link type
// does not allow the given source/target entity types. Centralizing this
// in the service ensures both the public REST handler and the AI
// AcceptDependencies path get the same gate — without it, AI callers
// could create item↔item Tests links that the REST endpoint rejects.
var ErrInvalidLinkTypeForEntities = errors.New("link type does not allow these entity types")

// CreateLink validates and inserts a new item link.
// Returns the new link ID, or 0 if the link was a duplicate (INSERT OR IGNORE).
func (s *ItemLinkService) CreateLink(params CreateItemLinkParams) (int64, error) {
	// Verify the link type exists and is active
	var active bool
	err := s.db.QueryRow("SELECT active FROM link_types WHERE id = ?", params.LinkTypeID).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("link type %d not found", params.LinkTypeID)
	}
	if err != nil {
		return 0, fmt.Errorf("failed to check link type: %w", err)
	}
	if !active {
		return 0, fmt.Errorf("link type %d is not active", params.LinkTypeID)
	}

	// Link type 1 ("Tests") is hardcoded to only allow item↔test_case
	// pairs. Mirrors the check in handlers/item_links.go so direct
	// callers of the service (notably AcceptDependencies) can't bypass it.
	if params.LinkTypeID == 1 {
		isItemTestCase := (params.SourceType == "item" && params.TargetType == "test_case") ||
			(params.SourceType == "test_case" && params.TargetType == "item")
		if !isItemTestCase {
			return 0, ErrInvalidLinkTypeForEntities
		}
	}

	// Insert with ON CONFLICT DO NOTHING to handle duplicates gracefully
	var linkID int64
	err = s.db.QueryRow(`
		INSERT INTO item_links (link_type_id, source_type, source_id, target_type, target_id, created_by, custom_field_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT DO NOTHING
		RETURNING id
	`, params.LinkTypeID, params.SourceType, params.SourceID, params.TargetType, params.TargetID, params.CreatedBy, params.CustomFieldID).Scan(&linkID)
	if errors.Is(err, sql.ErrNoRows) {
		// Duplicate — ON CONFLICT DO NOTHING returns no row
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to create item link: %w", err)
	}

	return linkID, nil
}
