package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
)

// ItemLinkService owns the link-management business logic shared by every
// caller (cookie-auth handler, v1 bearer handler, AI accept-dependencies
// flow, Jira import). The base file holds only the bare DB insert; the
// full orchestration (permission checks, cross-workspace gating,
// duplicate-detection, list filtering, notification + action emission)
// lives in item_link_orchestration.go and is opt-in via the With* setters
// below.
//
// Callers that only need the DB insert (validation included) can keep
// using `NewItemLinkService(db)` + `CreateLink(...)`. Callers that need
// the full HTTP-grade flow build with `.With...()` and call
// `CreateLinkWithChecks` / `DeleteLinkWithChecks` /
// `ListLinksForEntityWithChecks`.
type ItemLinkService struct {
	db database.Database

	// Optional dependencies — set via the With* methods. Nil-friendly:
	// when missing, callers that need them get a fail-closed permission
	// response. The bare CreateLink path does not touch any of these.
	perm          WorkspacePermissionChecker
	pagePerm      PagePermissionChecker
	assetPerm     AssetPermissionChecker
	notifications ItemLinkNotificationEmitter
	actions       ItemLinkActionEmitter
}

// NewItemLinkService creates a new ItemLinkService with only the database
// wired. Use the With* setters to add the permission / notification
// dependencies the orchestration methods need.
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

type itemLinkQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	QueryRowWrite(query string, args ...any) *sql.Row
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
	return createItemLink(s.db, params)
}

// CreateLinkWithMetadata validates and inserts a link with its canonical fact
// in one transaction. It is the internal path for imports and integrations
// that perform authorization outside this service.
func (s *ItemLinkService) CreateLinkWithMetadata(params CreateItemLinkParams, metadata itemevents.Metadata) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	id, err := createItemLink(tx, params)
	if err != nil || id == 0 {
		return id, err
	}
	link := models.ItemLink{
		ID: int(id), LinkTypeID: params.LinkTypeID,
		SourceType: params.SourceType, SourceID: params.SourceID,
		TargetType: params.TargetType, TargetID: params.TargetID,
		CreatedBy: params.CreatedBy, CustomFieldID: params.CustomFieldID,
	}
	if _, err := itemevents.NewRecorder(s.db).LinkChanged(context.Background(), tx, itemevents.Linked, link, metadata); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func createItemLink(db itemLinkQuerier, params CreateItemLinkParams) (int64, error) {
	// Verify the link type exists and is active, and pull its
	// allowed_entity_types constraint in the same query so we can gate
	// pairs that the link type's schema disallows.
	var (
		active             bool
		allowedEntityTypes sql.NullString
	)
	err := db.QueryRow(
		"SELECT active, allowed_entity_types FROM link_types WHERE id = ?",
		params.LinkTypeID,
	).Scan(&active, &allowedEntityTypes)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("link type %d not found", params.LinkTypeID)
	}
	if err != nil {
		return 0, fmt.Errorf("failed to check link type: %w", err)
	}
	if !active {
		return 0, fmt.Errorf("link type %d is not active", params.LinkTypeID)
	}

	// When the link type declares an allowed_entity_types set, require the
	// unordered {source, target} multiset to be covered by it. e.g.
	// ["item","test_case"] permits (item,test_case) and its mirror but
	// rejects (item,item), because the pair needs one "item" slot and one
	// "test_case" slot. nil/empty allowed_entity_types means no
	// restriction. Mirrors the legacy hardcoded LinkTypeID==1 gate so
	// direct callers of the service (notably AcceptDependencies) can't
	// bypass it.
	if allowedEntityTypes.Valid && allowedEntityTypes.String != "" {
		var allowed []string
		if err := json.Unmarshal([]byte(allowedEntityTypes.String), &allowed); err != nil {
			return 0, fmt.Errorf("invalid allowed_entity_types on link type %d: %w", params.LinkTypeID, err)
		}
		if len(allowed) > 0 {
			budget := make(map[string]int, len(allowed))
			for _, t := range allowed {
				budget[t]++
			}
			need := map[string]int{params.SourceType: 1}
			need[params.TargetType]++
			for t, n := range need {
				if budget[t] < n {
					return 0, ErrInvalidLinkTypeForEntities
				}
			}
		}
	}

	// Insert with ON CONFLICT DO NOTHING to handle duplicates gracefully
	var linkID int64
	err = db.QueryRowWrite(`
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
