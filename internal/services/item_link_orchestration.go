package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// Optional interfaces are wired on ItemLinkService via With* methods.

// AssetPermissionChecker is the minimal slice of the AssetHandler the link
// orchestration needs. Held as an interface because the implementation
// lives in the handlers package and a direct import would create a cycle.
// nil ⇒ fail-closed (asset endpoints always 404).
type AssetPermissionChecker interface {
	HasAssetSetPermission(userID, setID int, permissionKey string) (bool, error)
}

// WorkspacePermissionChecker is the effective-permission surface link
// authorization needs. Keeping it narrow makes targeted-scope behavior
// testable without constructing the full permission service.
type WorkspacePermissionChecker interface {
	HasWorkspacePermission(userID, workspaceID int, permission string) (bool, error)
	AccessibleWorkspaceIDs(userID int) ([]int, error)
	AccessibleWorkspaceIDKeys(userID int) ([]repository.IDKey, error)
}

// PagePermissionChecker is satisfied by *PagePermissionService (and by
// tests). nil ⇒ fail-closed (page endpoints always 404).
type PagePermissionChecker interface {
	Can(userID, workspaceID, pageID int, op string) (bool, error)
	ListVisiblePageIDs(userID, workspaceID int, pageIDs []int) (map[int]bool, error)
}

// ItemLinkNotificationEmitter is the slot the orchestration uses to fire
// "item linked" / "item unlinked" notifications. nil ⇒ no notifications
// (the orchestration still succeeds).
type ItemLinkNotificationEmitter interface {
	EmitEvent(event *NotificationEvent)
}

// ItemLinkActionEmitter is the slot the orchestration uses to fire action
// events for automation workflows. nil ⇒ no action events.
type ItemLinkActionEmitter interface {
	EmitActionEvent(event *models.ActionEvent)
}

// Sentinel errors map to HTTP status codes in the handlers.

// EntityNotAccessibleError covers both "no such entity" and "no view/edit
// permission" — they share an HTTP response (404, per the existence-leak
// policy) so the orchestration collapses them into one error type.
type EntityNotAccessibleError struct {
	EntityType string
	EntityID   int
}

func (e *EntityNotAccessibleError) Error() string {
	return fmt.Sprintf("%s %d not found or not accessible", e.EntityType, e.EntityID)
}

func IsEntityNotAccessible(err error) bool {
	var e *EntityNotAccessibleError
	return errors.As(err, &e)
}

var (
	// ErrLinkSelfReference is returned when source and target identify the
	// same entity. HTTP layer maps to 400.
	ErrLinkSelfReference = errors.New("cannot create link to self")

	// ErrLinkInvalidEntityType is returned when source_type or target_type
	// is not one of {item, test_case, asset, page}. HTTP layer maps to 400.
	ErrLinkInvalidEntityType = errors.New("invalid entity type (want item, test_case, asset, or page)")

	// ErrLinkExists is returned when an equivalent link already exists in
	// either direction. HTTP layer maps to 409.
	ErrLinkExists = errors.New("link already exists")

	// ErrLinkNotFound is returned by Delete / Get flows. HTTP layer maps
	// to 404.
	ErrLinkNotFound = errors.New("link not found")

	// ErrLinkCrossWorkspacePage is returned when an item↔page link
	// crosses workspaces. HTTP layer maps to 404 (kept opaque to match
	// per-page ACL denial shape).
	ErrLinkCrossWorkspacePage = errors.New("page link endpoints must share a workspace")
)

// Dependency setters.

func (s *ItemLinkService) WithPermissionService(p WorkspacePermissionChecker) *ItemLinkService {
	s.perm = p
	return s
}

// WithPagePermissionChecker wires the page ACL checker; nil fails closed.
func (s *ItemLinkService) WithPagePermissionChecker(p PagePermissionChecker) *ItemLinkService {
	s.pagePerm = p
	return s
}

// WithAssetPermissionChecker wires the asset-set permission checker.
func (s *ItemLinkService) WithAssetPermissionChecker(c AssetPermissionChecker) *ItemLinkService {
	s.assetPerm = c
	return s
}

// WithNotificationEmitter wires optional item-link notifications.
func (s *ItemLinkService) WithNotificationEmitter(e ItemLinkNotificationEmitter) *ItemLinkService {
	s.notifications = e
	return s
}

// WithActionEmitter wires optional automation events for item-source links.
func (s *ItemLinkService) WithActionEmitter(e ItemLinkActionEmitter) *ItemLinkService {
	s.actions = e
	return s
}

// Public orchestration surface.

// ListLinkTypes returns the active system and custom link types.
func (s *ItemLinkService) ListLinkTypes(includeInactive bool) ([]models.LinkType, error) {
	repo := repository.NewLinkTypeRepository(s.db)
	return repo.List(includeInactive)
}

// ListLinksByCustomField returns links for a primary or mirror custom field.
func (s *ItemLinkService) ListLinksByCustomField(fieldID, itemID int, mirror bool) ([]models.ItemLink, error) {
	if mirror {
		return s.getLinksWhere(
			"il.custom_field_id = ? AND il.target_type = 'item' AND il.target_id = ?",
			fieldID, itemID,
		)
	}
	return s.getLinksWhere(
		"il.custom_field_id = ? AND il.source_type = 'item' AND il.source_id = ?",
		fieldID, itemID,
	)
}

// CreateLinkWithChecks validates, authorizes, and persists a link for both API
// surfaces. Optional item-source notifications fire after persistence.
func (s *ItemLinkService) CreateLinkWithChecks(userID int, params CreateItemLinkParams) (*models.ItemLink, error) {
	if err := s.validateCreateLinkWithChecks(userID, params); err != nil {
		return nil, err
	}

	createdBy := userID
	params.CreatedBy = &createdBy

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// Check both directions for an existing link inside the write transaction.
	exists, err := itemLinkExists(tx, params)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrLinkExists
	}

	id, err := createItemLink(tx, params)
	if err != nil {
		return nil, err
	}
	if id == 0 {
		// CreateLink returns 0 on INSERT OR IGNORE; treat as duplicate.
		return nil, ErrLinkExists
	}
	now := time.Now()
	if err := s.touchLinkedItems(tx, now, params); err != nil {
		return nil, err
	}
	metadata := itemevents.User(userID, "application")
	metadata.OccurredAt = now
	if _, err := itemevents.NewRecorder(s.db).LinkChanged(context.Background(), tx, itemevents.Linked, models.ItemLink{
		ID: int(id), LinkTypeID: params.LinkTypeID,
		SourceType: params.SourceType, SourceID: params.SourceID,
		TargetType: params.TargetType, TargetID: params.TargetID,
	}, metadata); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit item link: %w", err)
	}

	return s.finishCreatedLink(userID, params, id)
}

// ReplaceSingleValueFieldLinkWithChecks atomically replaces a non-multi field link.
func (s *ItemLinkService) ReplaceSingleValueFieldLinkWithChecks(userID int, params CreateItemLinkParams) (*models.ItemLink, error) {
	if params.CustomFieldID == nil {
		return nil, fmt.Errorf("custom_field_id is required for single-value replacement")
	}
	if err := s.validateCreateLinkWithChecks(userID, params); err != nil {
		return nil, err
	}

	createdBy := userID
	params.CreatedBy = &createdBy
	type replacementResult struct {
		link     *models.ItemLink
		affected []CreateItemLinkParams
	}
	result, err := database.WithTxResult(s.db, func(tx database.Tx) (*replacementResult, error) {
		if err := lockLinkSource(tx, s.db.GetDriverName(), params.SourceType, params.SourceID); err != nil {
			return nil, err
		}
		affected := []CreateItemLinkParams{params}
		var previousLinks []models.ItemLink
		rows, err := tx.Query(`
			SELECT id, link_type_id, source_type, source_id, target_type, target_id
			FROM item_links
			WHERE custom_field_id = ? AND source_type = ? AND source_id = ?
		`, *params.CustomFieldID, params.SourceType, params.SourceID)
		if err != nil {
			return nil, fmt.Errorf("failed to load previous field links: %w", err)
		}
		for rows.Next() {
			var previous models.ItemLink
			if err := rows.Scan(&previous.ID, &previous.LinkTypeID, &previous.SourceType, &previous.SourceID, &previous.TargetType, &previous.TargetID); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("failed to scan previous field link: %w", err)
			}
			affected = append(affected, CreateItemLinkParams{
				LinkTypeID: previous.LinkTypeID, SourceType: previous.SourceType, SourceID: previous.SourceID,
				TargetType: previous.TargetType, TargetID: previous.TargetID,
			})
			previousLinks = append(previousLinks, previous)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("failed to close previous field links: %w", err)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("failed to iterate previous field links: %w", err)
		}
		if _, err := tx.ExecWrite(`
			DELETE FROM item_links
			WHERE custom_field_id = ? AND source_type = ? AND source_id = ?
		`, *params.CustomFieldID, params.SourceType, params.SourceID); err != nil {
			return nil, fmt.Errorf("failed to clear previous field link: %w", err)
		}
		metadata := itemevents.User(userID, "application")
		recorder := itemevents.NewRecorder(s.db)
		for _, previous := range previousLinks {
			if _, err := recorder.LinkChanged(context.Background(), tx, itemevents.Unlinked, previous, metadata); err != nil {
				return nil, err
			}
		}

		exists, err := itemLinkExists(tx, params)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, ErrLinkExists
		}

		id, err := createItemLink(tx, params)
		if err != nil {
			return nil, err
		}
		if id == 0 {
			return nil, ErrLinkExists
		}
		created, err := getLinkByIDFrom(tx, int(id))
		if err != nil {
			return nil, fmt.Errorf("failed to load replacement field link: %w", err)
		}
		if created == nil {
			return nil, fmt.Errorf("failed to load replacement field link: %w", ErrLinkNotFound)
		}
		if err := s.touchLinkedItems(tx, time.Now(), affected...); err != nil {
			return nil, err
		}
		if _, err := recorder.LinkChanged(context.Background(), tx, itemevents.Linked, *created, metadata); err != nil {
			return nil, err
		}
		return &replacementResult{link: created, affected: affected}, nil
	})
	if err != nil {
		return nil, err
	}

	if params.SourceType == "item" {
		s.emitLinkedEvents(userID, params, result.link)
	}
	publishItemLinkChanges(result.affected...)
	return result.link, nil
}

func lockLinkSource(tx database.Tx, driver, entityType string, entityID int) error {
	table := ""
	switch entityType {
	case "item":
		table = "items"
	case "test_case":
		table = "test_cases"
	case "asset":
		table = "assets"
	case "page":
		table = "pages"
	default:
		return ErrLinkInvalidEntityType
	}
	//nolint:gosec // table is selected from the hardcoded allowlist above.
	query := "SELECT id FROM " + table + " WHERE id = ?"
	if driver == "postgres" {
		query += " FOR UPDATE"
	}
	var lockedID int
	if err := tx.QueryRow(query, entityID).Scan(&lockedID); err != nil {
		return fmt.Errorf("failed to lock field-link source: %w", err)
	}
	return nil
}

func (s *ItemLinkService) validateCreateLinkWithChecks(userID int, params CreateItemLinkParams) error {
	if !isValidLinkEntityType(params.SourceType) || !isValidLinkEntityType(params.TargetType) {
		return ErrLinkInvalidEntityType
	}
	if params.SourceType == params.TargetType && params.SourceID == params.TargetID {
		return ErrLinkSelfReference
	}

	// Check permissions first so duplicate detection cannot reveal a link.
	if err := s.CheckEntityPermission(userID, params.SourceType, params.SourceID, models.PermissionItemEdit, AssetPermissionKeyEdit); err != nil {
		return err
	}
	if err := s.CheckEntityPermission(userID, params.TargetType, params.TargetID, models.PermissionItemView, AssetPermissionKeyView); err != nil {
		return err
	}

	// Item↔page links must remain in one workspace; deny cross-workspace links opaquely.
	if params.SourceType == "page" || params.TargetType == "page" {
		ok, err := s.linkEndpointsShareWorkspace(params.SourceType, params.SourceID, params.TargetType, params.TargetID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrLinkCrossWorkspacePage
		}
	}
	return nil
}

func itemLinkExists(db itemLinkQuerier, params CreateItemLinkParams) (bool, error) {
	var existingID int
	err := db.QueryRow(`
		SELECT id FROM item_links
		WHERE (source_type = ? AND source_id = ? AND target_type = ? AND target_id = ?)
		   OR (source_type = ? AND source_id = ? AND target_type = ? AND target_id = ?)
	`, params.SourceType, params.SourceID, params.TargetType, params.TargetID,
		params.TargetType, params.TargetID, params.SourceType, params.SourceID).Scan(&existingID)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("failed to probe duplicates: %w", err)
	}
	return false, nil
}

func (s *ItemLinkService) finishCreatedLink(userID int, params CreateItemLinkParams, id int64) (*models.ItemLink, error) {
	link, err := s.getLinkByID(int(id))
	if err != nil {
		return nil, fmt.Errorf("failed to load created link: %w", err)
	}
	if link == nil {
		return nil, fmt.Errorf("failed to load created link: %w", ErrLinkNotFound)
	}
	return s.finishHydratedCreatedLink(userID, params, link), nil
}

func (s *ItemLinkService) finishHydratedCreatedLink(userID int, params CreateItemLinkParams, link *models.ItemLink) *models.ItemLink {
	// Emit optional item-source events without rolling back the link on failure.
	if params.SourceType == "item" {
		s.emitLinkedEvents(userID, params, link)
	}
	// Refresh both item endpoints because the UI renders links bidirectionally.
	publishItemLinkChanges(params)
	return link
}

// publishItemLinkChanges announces link changes to every affected endpoint that is a
// work item (WI-483). The legacy notification path fires only for an item
// source; an item that is merely the target (or whose link source is a page)
// would otherwise never refresh its linked-items section.
func publishItemLinkChanges(links ...CreateItemLinkParams) {
	for _, itemID := range linkedItemIDs(links) {
		PublishItemChange(itemID, ItemChangeLink)
	}
}

// DeleteLinkWithChecks authorizes and deletes a link, then emits an item event.
func (s *ItemLinkService) DeleteLinkWithChecks(userID, linkID int) error {
	link, err := s.getLinkByID(linkID)
	if err != nil {
		return err
	}
	if link == nil {
		return ErrLinkNotFound
	}

	if err := s.CheckEntityPermission(userID, link.SourceType, link.SourceID, models.PermissionItemEdit, AssetPermissionKeyEdit); err != nil {
		return err
	}
	// Require visibility of the other endpoint to prevent hidden-link deletion.
	if err := s.CheckEntityPermission(userID, link.TargetType, link.TargetID, models.PermissionItemView, AssetPermissionKeyView); err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin item link deletion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.Exec("DELETE FROM item_links WHERE id = ?", linkID)
	if err != nil {
		return fmt.Errorf("failed to delete link: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrLinkNotFound
	}
	now := time.Now()
	if err := s.touchLinkedItems(tx, now, CreateItemLinkParams{
		SourceType: link.SourceType,
		SourceID:   link.SourceID,
		TargetType: link.TargetType,
		TargetID:   link.TargetID,
	}); err != nil {
		return err
	}
	metadata := itemevents.User(userID, "application")
	metadata.OccurredAt = now
	if _, err := itemevents.NewRecorder(s.db).LinkChanged(context.Background(), tx, itemevents.Unlinked, *link, metadata); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit item link deletion: %w", err)
	}

	if link.SourceType == "item" {
		s.emitUnlinkedEvents(userID, link)
	}
	// Refresh both item endpoints after removal.
	publishItemLinkChanges(CreateItemLinkParams{
		SourceType: link.SourceType,
		SourceID:   link.SourceID,
		TargetType: link.TargetType,
		TargetID:   link.TargetID,
	})
	return nil
}

func (s *ItemLinkService) touchLinkedItems(tx database.Tx, now time.Time, links ...CreateItemLinkParams) error {
	itemRepo := repository.NewItemRepository(s.db)
	for _, itemID := range linkedItemIDs(links) {
		if err := itemRepo.TouchChanged(tx, itemID, now); err != nil {
			return fmt.Errorf("touch linked item %d: %w", itemID, err)
		}
	}
	return nil
}

func linkedItemIDs(links []CreateItemLinkParams) []int {
	itemIDSet := make(map[int]struct{}, len(links)*2)
	for _, link := range links {
		if link.SourceType == "item" {
			itemIDSet[link.SourceID] = struct{}{}
		}
		if link.TargetType == "item" {
			itemIDSet[link.TargetID] = struct{}{}
		}
	}
	itemIDs := make([]int, 0, len(itemIDSet))
	for itemID := range itemIDSet {
		itemIDs = append(itemIDs, itemID)
	}
	slices.Sort(itemIDs)
	return itemIDs
}

// ListLinksForEntityWithChecks returns visible outgoing and incoming links;
// inaccessible endpoints are dropped. entityType is item, test_case, or page.
func (s *ItemLinkService) ListLinksForEntityWithChecks(userID int, entityType string, entityID int) (outgoing, incoming []models.ItemLink, err error) {
	if entityType != "item" && entityType != "test_case" && entityType != "page" {
		return nil, nil, ErrLinkInvalidEntityType
	}
	if err := s.CheckEntityPermission(userID, entityType, entityID, models.PermissionItemView, AssetPermissionKeyView); err != nil {
		return nil, nil, err
	}

	outgoing, err = s.getLinksWhere("source_type = ? AND source_id = ? AND il.custom_field_id IS NULL", entityType, entityID)
	if err != nil {
		return nil, nil, err
	}
	incoming, err = s.getLinksWhere("target_type = ? AND target_id = ?", entityType, entityID)
	if err != nil {
		return nil, nil, err
	}

	outgoing = s.FilterLinksForUser(userID, outgoing)
	incoming = s.FilterLinksForUser(userID, incoming)
	return outgoing, incoming, nil
}

type EntityLinks struct {
	Outgoing []models.ItemLink `json:"outgoing"`
	Incoming []models.ItemLink `json:"incoming"`
}

// ListLinksForItemsWithChecks batches visible links for many items. Every
// requested ID is present, with empty slices for inaccessible or unlinked items.
func (s *ItemLinkService) ListLinksForItemsWithChecks(userID int, itemIDs []int) (map[int]EntityLinks, error) {
	return s.listLinksForItemsWithChecks(userID, itemIDs, false)
}

// ListLinksForItemsWithCustomFieldsAndChecks also includes custom-field links.
func (s *ItemLinkService) ListLinksForItemsWithCustomFieldsAndChecks(userID int, itemIDs []int) (map[int]EntityLinks, error) {
	return s.listLinksForItemsWithChecks(userID, itemIDs, true)
}

func (s *ItemLinkService) listLinksForItemsWithChecks(userID int, itemIDs []int, includeCustomFields bool) (map[int]EntityLinks, error) {
	startedAt := time.Now()
	result := map[int]EntityLinks{}
	ids := dedupInts(itemIDs)
	if len(ids) == 0 {
		return result, nil
	}
	for _, id := range ids {
		result[id] = EntityLinks{Outgoing: []models.ItemLink{}, Incoming: []models.ItemLink{}}
	}

	ph := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	idArgs := toIfaceSlice(ids)

	outgoingFilter := "source_type = ? AND source_id IN (" + ph + ")"
	if !includeCustomFields {
		outgoingFilter += " AND il.custom_field_id IS NULL"
	}
	outgoing, err := s.getLinksWhere(
		outgoingFilter,
		append([]any{"item"}, idArgs...)...)
	if err != nil {
		return nil, err
	}
	incomingFilter := "target_type = ? AND target_id IN (" + ph + ")"
	if !includeCustomFields {
		incomingFilter += " AND il.custom_field_id IS NULL"
	}
	incoming, err := s.getLinksWhere(
		incomingFilter,
		append([]any{"item"}, idArgs...)...)
	if err != nil {
		return nil, err
	}

	allLinks := make([]models.ItemLink, 0, len(outgoing)+len(incoming))
	allLinks = append(allLinks, outgoing...)
	allLinks = append(allLinks, incoming...)
	access := s.authorizeReferencedLinkScopes(userID, allLinks)
	outgoing = s.filterLinksByAccessWithScopes(outgoing, access.workspaceKeys, access.workspaceIDs, access.testWorkspaceIDs, access.assetSetIDs, access.scopes)
	incoming = s.filterLinksByAccessWithScopes(incoming, access.workspaceKeys, access.workspaceIDs, access.testWorkspaceIDs, access.assetSetIDs, access.scopes)
	outgoing = s.FilterPageLinksByACL(userID, outgoing)
	incoming = s.FilterPageLinksByACL(userID, incoming)
	slog.Debug("batch link authorization",
		"requested_items", len(ids),
		"link_rows", len(allLinks),
		"referenced_workspace_scopes", access.workspaceScopeCount,
		"referenced_test_workspace_scopes", access.testWorkspaceScopeCount,
		"referenced_asset_set_scopes", access.assetSetScopeCount,
		"workspace_permission_checks", access.workspacePermissionChecks,
		"test_permission_checks", access.testPermissionChecks,
		"asset_permission_checks", access.assetPermissionChecks,
		"scope_sql_queries", access.scopeSQLQueries,
		"latency_ms", time.Since(startedAt).Milliseconds())

	for _, l := range outgoing {
		if g, ok := result[l.SourceID]; ok {
			g.Outgoing = append(g.Outgoing, l)
			result[l.SourceID] = g
		}
	}
	for _, l := range incoming {
		if g, ok := result[l.TargetID]; ok {
			g.Incoming = append(g.Incoming, l)
			result[l.TargetID] = g
		}
	}
	return result, nil
}

type referencedLinkAccess struct {
	workspaceKeys             map[string]bool
	workspaceIDs              map[int]bool
	testWorkspaceIDs          map[int]bool
	assetSetIDs               map[int]bool
	scopes                    map[scopeKey]endpointScope
	workspaceScopeCount       int
	testWorkspaceScopeCount   int
	assetSetScopeCount        int
	workspacePermissionChecks int
	testPermissionChecks      int
	assetPermissionChecks     int
	scopeSQLQueries           int
}

// authorizeReferencedLinkScopes checks each referenced scope once and never
// enumerates unrelated tenants or asset sets.
func (s *ItemLinkService) authorizeReferencedLinkScopes(userID int, links []models.ItemLink) referencedLinkAccess {
	access := referencedLinkAccess{
		workspaceKeys:    map[string]bool{},
		workspaceIDs:     map[int]bool{},
		testWorkspaceIDs: map[int]bool{},
		assetSetIDs:      map[int]bool{},
		scopes:           s.resolveEndpointScopes(links),
	}

	workspaceKeysByID := map[int]map[string]struct{}{}
	workspaceScopes := map[int]struct{}{}
	testWorkspaceScopes := map[int]struct{}{}
	assetSetScopes := map[int]struct{}{}
	note := func(entityType string, entityID int, workspaceID *int, workspaceKey string) {
		switch entityType {
		case "item", "page":
			if workspaceID != nil {
				workspaceScopes[*workspaceID] = struct{}{}
				if workspaceKey != "" {
					if workspaceKeysByID[*workspaceID] == nil {
						workspaceKeysByID[*workspaceID] = map[string]struct{}{}
					}
					workspaceKeysByID[*workspaceID][workspaceKey] = struct{}{}
				}
			}
		case "test_case":
			if scope, ok := access.scopes[scopeKey{entityType, entityID}]; ok {
				testWorkspaceScopes[scope.wsID] = struct{}{}
			}
		case "asset":
			if scope, ok := access.scopes[scopeKey{entityType, entityID}]; ok {
				assetSetScopes[scope.setID] = struct{}{}
			}
		}
	}
	for _, link := range links {
		note(link.SourceType, link.SourceID, link.SourceWorkspaceID, link.SourceWorkspaceKey)
		note(link.TargetType, link.TargetID, link.TargetWorkspaceID, link.TargetWorkspaceKey)
	}

	access.workspaceScopeCount = len(workspaceScopes)
	access.testWorkspaceScopeCount = len(testWorkspaceScopes)
	access.assetSetScopeCount = len(assetSetScopes)
	s.applyReferencedScopePermissions(userID, workspaceScopes, testWorkspaceScopes, assetSetScopes, workspaceKeysByID, &access)
	access.scopeSQLQueries = referencedScopeQueryCount(links)
	return access
}

func (s *ItemLinkService) applyReferencedScopePermissions(userID int, workspaceScopes, testWorkspaceScopes, assetSetScopes map[int]struct{}, workspaceKeysByID map[int]map[string]struct{}, access *referencedLinkAccess) {
	for workspaceID := range workspaceScopes {
		access.workspacePermissionChecks++
		if s.perm == nil {
			continue
		}
		allowed, err := s.perm.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
		if err != nil || !allowed {
			continue
		}
		access.workspaceIDs[workspaceID] = true
		for key := range workspaceKeysByID[workspaceID] {
			access.workspaceKeys[key] = true
		}
	}
	for workspaceID := range testWorkspaceScopes {
		access.testPermissionChecks++
		if s.perm == nil {
			continue
		}
		allowed, err := s.perm.HasWorkspacePermission(userID, workspaceID, models.PermissionTestView)
		if err == nil && allowed {
			access.testWorkspaceIDs[workspaceID] = true
		}
	}
	for setID := range assetSetScopes {
		access.assetPermissionChecks++
		if s.assetPerm == nil {
			continue
		}
		allowed, err := s.assetPerm.HasAssetSetPermission(userID, setID, AssetPermissionKeyView)
		if err == nil && allowed {
			access.assetSetIDs[setID] = true
		}
	}
}

func referencedScopeQueryCount(links []models.ItemLink) int {
	types := map[string]bool{}
	for _, link := range links {
		for _, entityType := range []string{link.SourceType, link.TargetType} {
			if entityType == "test_case" || entityType == "asset" {
				types[entityType] = true
			}
		}
	}
	return len(types)
}

// dedupInts returns in with duplicates removed, preserving first-seen order.
func dedupInts(in []int) []int {
	seen := make(map[int]bool, len(in))
	out := make([]int, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// Internal helpers.

func isValidLinkEntityType(s string) bool {
	switch s {
	case "item", "test_case", "asset", "page":
		return true
	}
	return false
}

// pageOpForWorkspacePerm maps workspace permissions to page operations;
// unknown values default to view.
func pageOpForWorkspacePerm(workspacePerm string) string {
	if workspacePerm == models.PermissionItemEdit || workspacePerm == models.PermissionItemDelete {
		return PageOpEdit
	}
	return PageOpView
}

// ResolveEntityScope returns workspace_id or set_id without exposing lookup
// failures through the found=false result.
func (s *ItemLinkService) ResolveEntityScope(entityType string, entityID int) (wsID, setID int, found bool, err error) {
	switch entityType {
	case "item":
		wsID, err := repository.NewItemRepository(s.db).GetWorkspaceID(entityID)
		if errors.Is(err, repository.ErrNotFound) {
			return 0, 0, false, nil
		}
		if err != nil {
			return 0, 0, false, err
		}
		return wsID, 0, true, nil
	case "test_case":
		var v int
		err = s.db.QueryRow("SELECT workspace_id FROM test_cases WHERE id = ?", entityID).Scan(&v)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, 0, false, nil
		}
		if err != nil {
			return 0, 0, false, err
		}
		return v, 0, true, nil
	case "asset":
		var v int
		err = s.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", entityID).Scan(&v)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, 0, false, nil
		}
		if err != nil {
			return 0, 0, false, err
		}
		return 0, v, true, nil
	case "page":
		var v int
		err = s.db.QueryRow("SELECT workspace_id FROM pages WHERE id = ?", entityID).Scan(&v)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, 0, false, nil
		}
		if err != nil {
			return 0, 0, false, err
		}
		return v, 0, true, nil
	default:
		return 0, 0, false, fmt.Errorf("unsupported entity type %q", entityType)
	}
}

// CheckEntityPermission returns an opaque not-accessible error for missing or
// unauthorized endpoints.
func (s *ItemLinkService) CheckEntityPermission(userID int, entityType string, entityID int, workspacePerm, assetPermKey string) error {
	wsID, setID, found, err := s.ResolveEntityScope(entityType, entityID)
	if err != nil {
		return err
	}
	if !found {
		return &EntityNotAccessibleError{EntityType: entityType, EntityID: entityID}
	}

	switch entityType {
	case "item":
		if s.perm == nil {
			return &EntityNotAccessibleError{EntityType: entityType, EntityID: entityID}
		}
		hasPerm, err := s.perm.HasWorkspacePermission(userID, wsID, workspacePerm)
		if err != nil || !hasPerm {
			return &EntityNotAccessibleError{EntityType: entityType, EntityID: entityID}
		}
		return nil
	case "test_case":
		if s.perm == nil {
			return &EntityNotAccessibleError{EntityType: entityType, EntityID: entityID}
		}
		permission := models.PermissionTestView
		if workspacePerm == models.PermissionItemEdit || workspacePerm == models.PermissionItemDelete {
			permission = models.PermissionTestManage
		}
		hasPerm, err := s.perm.HasWorkspacePermission(userID, wsID, permission)
		if err != nil || !hasPerm {
			return &EntityNotAccessibleError{EntityType: entityType, EntityID: entityID}
		}
		return nil
	case "asset":
		if s.assetPerm == nil {
			return &EntityNotAccessibleError{EntityType: entityType, EntityID: entityID}
		}
		hasPerm, err := s.assetPerm.HasAssetSetPermission(userID, setID, assetPermKey)
		if err != nil || !hasPerm {
			return &EntityNotAccessibleError{EntityType: entityType, EntityID: entityID}
		}
		return nil
	case "page":
		if s.pagePerm == nil {
			return &EntityNotAccessibleError{EntityType: entityType, EntityID: entityID}
		}
		op := pageOpForWorkspacePerm(workspacePerm)
		hasPerm, err := s.pagePerm.Can(userID, wsID, entityID, op)
		if err != nil || !hasPerm {
			return &EntityNotAccessibleError{EntityType: entityType, EntityID: entityID}
		}
		return nil
	}
	return ErrLinkInvalidEntityType
}

func (s *ItemLinkService) linkEndpointsShareWorkspace(srcType string, srcID int, tgtType string, tgtID int) (bool, error) {
	srcWs, _, srcFound, err := s.ResolveEntityScope(srcType, srcID)
	if err != nil {
		return false, err
	}
	if !srcFound {
		return false, nil
	}
	tgtWs, _, tgtFound, err := s.ResolveEntityScope(tgtType, tgtID)
	if err != nil {
		return false, err
	}
	if !tgtFound {
		return false, nil
	}
	return srcWs == tgtWs, nil
}

func (s *ItemLinkService) AccessibleWorkspaceIDs(userID int) map[int]bool {
	out := map[int]bool{}
	if s.perm == nil {
		return out
	}
	ids, err := s.perm.AccessibleWorkspaceIDs(userID)
	if err != nil {
		return out
	}
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func (s *ItemLinkService) AccessibleWorkspaceKeys(userID int) map[string]bool {
	out := map[string]bool{}
	if s.perm == nil {
		return out
	}
	pairs, err := s.perm.AccessibleWorkspaceIDKeys(userID)
	if err != nil {
		return out
	}
	for _, p := range pairs {
		out[p.Key] = true
	}
	return out
}

func (s *ItemLinkService) AccessibleAssetSetIDs(userID int) map[int]bool {
	out := map[int]bool{}
	if s.assetPerm == nil {
		return out
	}
	rows, err := s.db.Query("SELECT id FROM asset_management_sets")
	if err != nil {
		return out
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ok, err := s.assetPerm.HasAssetSetPermission(userID, id, AssetPermissionKeyView)
		if err == nil && ok {
			out[id] = true
		}
	}
	if err := rows.Err(); err != nil {
		return map[int]bool{}
	}
	return out
}

func (s *ItemLinkService) EndpointVisible(entityType string, entityID int, workspaceKey string, accessibleKeys map[string]bool, accessibleWs, accessibleSets map[int]bool) bool {
	switch entityType {
	case "item":
		return workspaceKey != "" && accessibleKeys[workspaceKey]
	case "test_case", "page":
		wsID, _, found, err := s.ResolveEntityScope(entityType, entityID)
		if err != nil || !found {
			return false
		}
		return accessibleWs[wsID]
	case "asset":
		_, setID, found, err := s.ResolveEntityScope(entityType, entityID)
		if err != nil || !found {
			return false
		}
		return accessibleSets[setID]
	}
	return false
}

// FilterLinksByAccess resolves non-item scopes in batches, then drops links
// whose endpoints are outside the user's allow-sets.
func (s *ItemLinkService) FilterLinksByAccess(links []models.ItemLink, accessibleKeys map[string]bool, accessibleWs, accessibleSets map[int]bool) []models.ItemLink {
	scopes := s.resolveEndpointScopes(links)
	return s.filterLinksByAccessWithScopes(links, accessibleKeys, accessibleWs, accessibleWs, accessibleSets, scopes)
}

// FilterLinksForUser checks each endpoint in its own permission domain and
// drops the whole link row when either endpoint is inaccessible.
func (s *ItemLinkService) FilterLinksForUser(userID int, links []models.ItemLink) []models.ItemLink {
	access := s.authorizeReferencedLinkScopes(userID, links)
	visible := s.filterLinksByAccessWithScopes(links, access.workspaceKeys, access.workspaceIDs, access.testWorkspaceIDs, access.assetSetIDs, access.scopes)
	return s.FilterPageLinksByACL(userID, visible)
}

func (s *ItemLinkService) filterLinksByAccessWithScopes(links []models.ItemLink, accessibleKeys map[string]bool, accessibleWs, accessibleTestWs, accessibleSets map[int]bool, scopes map[scopeKey]endpointScope) []models.ItemLink {
	out := make([]models.ItemLink, 0, len(links))
	for _, l := range links {
		if !s.endpointVisibleScoped(l.SourceType, l.SourceID, l.SourceWorkspaceKey, accessibleKeys, accessibleWs, accessibleTestWs, accessibleSets, scopes) {
			continue
		}
		if !s.endpointVisibleScoped(l.TargetType, l.TargetID, l.TargetWorkspaceKey, accessibleKeys, accessibleWs, accessibleTestWs, accessibleSets, scopes) {
			continue
		}
		out = append(out, l)
	}
	return out
}

type scopeKey struct {
	typ string
	id  int
}

// endpointScope stores a batch-resolved workspace or asset-set ID.
type endpointScope struct {
	wsID  int
	setID int
}

// resolveEndpointScopes batch-resolves scopes for non-item endpoints.
func (s *ItemLinkService) resolveEndpointScopes(links []models.ItemLink) map[scopeKey]endpointScope {
	testCaseIDs := map[int]bool{}
	pageIDs := map[int]bool{}
	assetIDs := map[int]bool{}
	out := map[scopeKey]endpointScope{}
	note := func(typ string, id int, workspaceID *int) {
		switch typ {
		case "test_case":
			testCaseIDs[id] = true
		case "page":
			if workspaceID != nil {
				out[scopeKey{typ, id}] = endpointScope{wsID: *workspaceID}
			} else {
				pageIDs[id] = true
			}
		case "asset":
			assetIDs[id] = true
		}
	}
	for _, l := range links {
		note(l.SourceType, l.SourceID, l.SourceWorkspaceID)
		note(l.TargetType, l.TargetID, l.TargetWorkspaceID)
	}

	s.fillWorkspaceScopes(out, "test_case", "test_cases", testCaseIDs)
	s.fillWorkspaceScopes(out, "page", "pages", pageIDs)
	s.fillAssetSetScopes(out, assetIDs)
	return out
}

func (s *ItemLinkService) fillWorkspaceScopes(out map[scopeKey]endpointScope, entityType, table string, idset map[int]bool) {
	ids := keysOf(idset)
	if len(ids) == 0 {
		return
	}
	//nolint:gosec // G201: table is a hardcoded constant; ids are bound params.
	q := inClauseQuery("SELECT id, workspace_id FROM "+table+" WHERE id IN (", len(ids))
	rows, err := s.db.Query(q, toIfaceSlice(ids)...)
	if err != nil {
		return
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, wsID int
		if err := rows.Scan(&id, &wsID); err != nil {
			continue
		}
		out[scopeKey{entityType, id}] = endpointScope{wsID: wsID}
	}
	_ = rows.Err()
}

func (s *ItemLinkService) fillAssetSetScopes(out map[scopeKey]endpointScope, idset map[int]bool) {
	ids := keysOf(idset)
	if len(ids) == 0 {
		return
	}
	q := inClauseQuery("SELECT id, set_id FROM assets WHERE id IN (", len(ids))
	rows, err := s.db.Query(q, toIfaceSlice(ids)...)
	if err != nil {
		return
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, setID int
		if err := rows.Scan(&id, &setID); err != nil {
			continue
		}
		out[scopeKey{"asset", id}] = endpointScope{setID: setID}
	}
	_ = rows.Err()
}

func (s *ItemLinkService) endpointVisibleScoped(entityType string, entityID int, workspaceKey string, accessibleKeys map[string]bool, accessibleWs, accessibleTestWs, accessibleSets map[int]bool, scopes map[scopeKey]endpointScope) bool {
	switch entityType {
	case "item":
		return workspaceKey != "" && accessibleKeys[workspaceKey]
	case "test_case":
		sc, ok := scopes[scopeKey{entityType, entityID}]
		if !ok {
			return false
		}
		return accessibleTestWs[sc.wsID]
	case "page":
		sc, ok := scopes[scopeKey{entityType, entityID}]
		if !ok {
			return false
		}
		return accessibleWs[sc.wsID]
	case "asset":
		sc, ok := scopes[scopeKey{"asset", entityID}]
		if !ok {
			return false
		}
		return accessibleSets[sc.setID]
	}
	return false
}

func keysOf(set map[int]bool) []int {
	out := make([]int, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

// FilterPageLinksByACL drops links hidden by page ACLs and fails closed when
// the checker is unavailable.
func (s *ItemLinkService) FilterPageLinksByACL(userID int, links []models.ItemLink) []models.ItemLink {
	if len(links) == 0 {
		return links
	}
	hasPage := false
	for _, l := range links {
		if l.SourceType == "page" || l.TargetType == "page" {
			hasPage = true
			break
		}
	}
	if !hasPage {
		return links
	}
	if s.pagePerm == nil {
		out := links[:0]
		for _, l := range links {
			if l.SourceType == "page" || l.TargetType == "page" {
				continue
			}
			out = append(out, l)
		}
		return out
	}

	// Bucket page IDs by workspace so ListVisiblePageIDs can batch.
	bucket := map[int]map[int]struct{}{}
	addPage := func(wsID *int, pageID int) {
		if wsID == nil {
			return
		}
		ids, ok := bucket[*wsID]
		if !ok {
			ids = map[int]struct{}{}
			bucket[*wsID] = ids
		}
		ids[pageID] = struct{}{}
	}
	for _, l := range links {
		if l.SourceType == "page" {
			addPage(l.SourceWorkspaceID, l.SourceID)
		}
		if l.TargetType == "page" {
			addPage(l.TargetWorkspaceID, l.TargetID)
		}
	}
	visible := map[int]map[int]bool{}
	for wsID, ids := range bucket {
		flat := make([]int, 0, len(ids))
		for id := range ids {
			flat = append(flat, id)
		}
		got, err := s.pagePerm.ListVisiblePageIDs(userID, wsID, flat)
		if err != nil {
			got = map[int]bool{}
		}
		visible[wsID] = got
	}
	pageVisible := func(wsID *int, pageID int) bool {
		if wsID == nil {
			return false
		}
		return visible[*wsID][pageID]
	}
	out := make([]models.ItemLink, 0, len(links))
	for _, l := range links {
		if l.SourceType == "page" && !pageVisible(l.SourceWorkspaceID, l.SourceID) {
			continue
		}
		if l.TargetType == "page" && !pageVisible(l.TargetWorkspaceID, l.TargetID) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// getLinksWhere is the shared joined projection for link list endpoints.
func (s *ItemLinkService) getLinksWhere(whereClause string, args ...any) ([]models.ItemLink, error) {
	return getLinksWhere(s.db, whereClause, args...)
}

func getLinksWhere(db itemLinkQuerier, whereClause string, args ...any) ([]models.ItemLink, error) {
	rows, err := db.Query(itemLinksWhereQuery(whereClause), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanItemLinks(rows)
}

func getLinksWhereContext(ctx context.Context, db database.Database, whereClause string, args ...any) ([]models.ItemLink, error) {
	rows, err := db.QueryContext(ctx, itemLinksWhereQuery(whereClause), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanItemLinks(rows)
}

func itemLinksWhereQuery(whereClause string) string {
	return `
		SELECT il.id, il.link_type_id, il.source_type, il.source_id, il.target_type, il.target_id,
		       il.created_by, il.created_at,
		       lt.name, lt.color, lt.forward_label, lt.reverse_label,
		       COALESCE(si.title, stc.title, sa.title, sp.title, '') as source_title,
		       COALESCE(ti.title, ttc.title, ta.title, tp.title, '') as target_title,
		       COALESCE(u.username, '') as created_by_name,
		       si.status_id as source_status_id,
		       COALESCE(ss.name, '') as source_status_name,
		       COALESCE(ssc.color, '') as source_status_color,
		       si.item_type_id as source_item_type_id,
		       COALESCE(sit.name, '') as source_item_type_name,
		       COALESCE(sit.icon, '') as source_item_type_icon,
		       COALESCE(sit.color, '') as source_item_type_color,
		       COALESCE(sw.key, spw.key, '') as source_workspace_key,
		       COALESCE(si.workspace_id, sp.workspace_id) as source_workspace_id,
		       si.workspace_item_number as source_item_number,
		       ti.status_id as target_status_id,
		       COALESCE(ts.name, '') as target_status_name,
		       COALESCE(tsc.color, '') as target_status_color,
		       ti.item_type_id as target_item_type_id,
		       COALESCE(tit.name, '') as target_item_type_name,
		       COALESCE(tit.icon, '') as target_item_type_icon,
		       COALESCE(tit.color, '') as target_item_type_color,
		       COALESCE(tw.key, tpw.key, '') as target_workspace_key,
		       COALESCE(ti.workspace_id, tp.workspace_id) as target_workspace_id,
		       ti.workspace_item_number as target_item_number,
		       il.custom_field_id,
		       COALESCE(cfd.name, '') as custom_field_name
		FROM item_links il
		JOIN link_types lt ON il.link_type_id = lt.id
		LEFT JOIN items si ON il.source_type = 'item' AND il.source_id = si.id
		LEFT JOIN test_cases stc ON il.source_type = 'test_case' AND il.source_id = stc.id
		LEFT JOIN assets sa ON il.source_type = 'asset' AND il.source_id = sa.id
		LEFT JOIN pages sp ON il.source_type = 'page' AND il.source_id = sp.id
		LEFT JOIN items ti ON il.target_type = 'item' AND il.target_id = ti.id
		LEFT JOIN test_cases ttc ON il.target_type = 'test_case' AND il.target_id = ttc.id
		LEFT JOIN assets ta ON il.target_type = 'asset' AND il.target_id = ta.id
		LEFT JOIN pages tp ON il.target_type = 'page' AND il.target_id = tp.id
		LEFT JOIN users u ON il.created_by = u.id
		LEFT JOIN statuses ss ON si.status_id = ss.id
		LEFT JOIN statuses ts ON ti.status_id = ts.id
		LEFT JOIN status_categories ssc ON ss.category_id = ssc.id
		LEFT JOIN status_categories tsc ON ts.category_id = tsc.id
		LEFT JOIN item_types sit ON si.item_type_id = sit.id
		LEFT JOIN item_types tit ON ti.item_type_id = tit.id
		LEFT JOIN workspaces sw ON si.workspace_id = sw.id
		LEFT JOIN workspaces tw ON ti.workspace_id = tw.id
		LEFT JOIN workspaces spw ON sp.workspace_id = spw.id
		LEFT JOIN workspaces tpw ON tp.workspace_id = tpw.id
		LEFT JOIN custom_field_definitions cfd ON il.custom_field_id = cfd.id
		WHERE ` + whereClause + `
		ORDER BY lt.name, il.created_at DESC
	`
}

func scanItemLinks(rows *sql.Rows) ([]models.ItemLink, error) {
	var links []models.ItemLink
	for rows.Next() {
		var link models.ItemLink
		if err := rows.Scan(
			&link.ID, &link.LinkTypeID, &link.SourceType, &link.SourceID,
			&link.TargetType, &link.TargetID, &link.CreatedBy, &link.CreatedAt,
			&link.LinkTypeName, &link.LinkTypeColor, &link.LinkTypeForwardLabel, &link.LinkTypeReverseLabel,
			&link.SourceTitle, &link.TargetTitle, &link.CreatedByName,
			&link.SourceStatusID, &link.SourceStatusName, &link.SourceStatusColor,
			&link.SourceItemTypeID, &link.SourceItemTypeName, &link.SourceItemTypeIcon, &link.SourceItemTypeColor,
			&link.SourceWorkspaceKey, &link.SourceWorkspaceID, &link.SourceItemNumber,
			&link.TargetStatusID, &link.TargetStatusName, &link.TargetStatusColor,
			&link.TargetItemTypeID, &link.TargetItemTypeName, &link.TargetItemTypeIcon, &link.TargetItemTypeColor,
			&link.TargetWorkspaceKey, &link.TargetWorkspaceID, &link.TargetItemNumber,
			&link.CustomFieldID, &link.CustomFieldName,
		); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return links, nil
}

func (s *ItemLinkService) getLinkByID(id int) (*models.ItemLink, error) {
	return getLinkByIDFrom(s.db, id)
}

func getLinkByIDFrom(db itemLinkQuerier, id int) (*models.ItemLink, error) {
	links, err := getLinksWhere(db, "il.id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return nil, nil
	}
	return &links[0], nil
}

// emitLinkedEvents publishes best-effort events for item-source links.
func (s *ItemLinkService) emitLinkedEvents(actorUserID int, params CreateItemLinkParams, link *models.ItemLink) {
	if s.notifications == nil && s.actions == nil {
		return
	}
	sourceItem, err := repository.NewItemRepository(s.db).FindByID(params.SourceID)
	if err != nil {
		return
	}
	actorName := s.lookupActorUsername(actorUserID)
	if s.notifications != nil {
		referencedWorkspaceID, referencedPermission := s.notificationReferenceAccess(params.TargetType, params.TargetID)
		s.notifications.EmitEvent(&NotificationEvent{
			EventType:                     models.EventItemLinked,
			WorkspaceID:                   sourceItem.WorkspaceID,
			ActorUserID:                   actorUserID,
			ItemID:                        params.SourceID,
			AssigneeID:                    sourceItem.AssigneeID,
			CreatorID:                     sourceItem.CreatorID,
			Title:                         "Item Linked",
			ReferencedWorkspaceID:         referencedWorkspaceID,
			ReferencedWorkspacePermission: referencedPermission,
			ReferencedEntityType:          params.TargetType,
			ReferencedEntityID:            params.TargetID,
			TemplateData: map[string]any{
				"item.title":   sourceItem.Title,
				"item.id":      params.SourceID,
				"target.title": link.TargetTitle,
				"target.id":    params.TargetID,
				"user.name":    actorName,
			},
		})
	}
	if s.actions != nil {
		s.actions.EmitActionEvent(&models.ActionEvent{
			EventType:   models.ActionTriggerItemLinked,
			WorkspaceID: sourceItem.WorkspaceID,
			ItemID:      params.SourceID,
			ActorUserID: actorUserID,
			NewValues: map[string]any{
				"link_type_id": params.LinkTypeID,
				"target_type":  params.TargetType,
				"target_id":    params.TargetID,
			},
		})
	}
}

// emitUnlinkedEvents publishes the best-effort item-source unlink event.
func (s *ItemLinkService) emitUnlinkedEvents(actorUserID int, link *models.ItemLink) {
	if s.notifications == nil {
		return
	}
	sourceItem, err := repository.NewItemRepository(s.db).FindByID(link.SourceID)
	if err != nil {
		return
	}
	referencedWorkspaceID, referencedPermission := s.notificationReferenceAccess(link.TargetType, link.TargetID)
	s.notifications.EmitEvent(&NotificationEvent{
		EventType:                     models.EventItemUnlinked,
		WorkspaceID:                   sourceItem.WorkspaceID,
		ActorUserID:                   actorUserID,
		ItemID:                        link.SourceID,
		AssigneeID:                    sourceItem.AssigneeID,
		CreatorID:                     sourceItem.CreatorID,
		Title:                         "Item Unlinked",
		ReferencedWorkspaceID:         referencedWorkspaceID,
		ReferencedWorkspacePermission: referencedPermission,
		ReferencedEntityType:          link.TargetType,
		ReferencedEntityID:            link.TargetID,
		TemplateData: map[string]any{
			"item.title":   sourceItem.Title,
			"item.id":      link.SourceID,
			"target.title": link.TargetTitle,
			"target.id":    link.TargetID,
			"user.name":    s.lookupActorUsername(actorUserID),
		},
	})
}

func (s *ItemLinkService) notificationReferenceAccess(entityType string, entityID int) (workspaceID int, permission string) {
	workspaceID, _, found, err := s.ResolveEntityScope(entityType, entityID)
	permissionByType := map[string]string{
		"item":      models.PermissionItemView,
		"test_case": models.PermissionTestView,
		"page":      models.PermissionPageView,
		"asset":     AssetPermissionKeyView,
	}
	permission = permissionByType[entityType]
	if permission == "" {
		return 0, "invalid"
	}
	if err != nil || !found {
		// Keep the required permission populated with an invalid scope so the
		// notification service drops every recipient fail-closed.
		return 0, permission
	}
	return workspaceID, permission
}

// lookupActorUsername returns template data without blocking link creation.
func (s *ItemLinkService) lookupActorUsername(userID int) string {
	user, err := repository.NewUserRepository(s.db).GetByID(userID)
	if err != nil || user == nil {
		return ""
	}
	return user.Username
}

// CanonicalEntityType maps user-facing path segments to internal entity types.
func CanonicalEntityType(pathSegment string) (string, bool) {
	switch strings.ToLower(pathSegment) {
	case "items", "item":
		return "item", true
	case "test-cases", "test_cases", "test_case":
		return "test_case", true
	case "pages", "page":
		return "page", true
	case "assets", "asset":
		return "asset", true
	}
	return "", false
}
