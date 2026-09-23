package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/repository"
)

// WorkspaceBundleImportService applies a workspace bundle to a target
// workspace. Identity references (item types, custom fields, link types,
// users) are resolved before any write and refused with a structured report
// when missing; content entities are then created through their regular
// application services, so each entity lands atomically and per-entity
// failures are reported without blocking the rest.

var ErrWorkspaceBundleEmpty = errors.New("workspace bundle import: empty bundle")

// WorkspaceBundleImportOutcome records one entity's import result.
type WorkspaceBundleImportOutcome struct {
	Entity string `json:"entity"` // page | item | item_link
	Ref    string `json:"ref,omitempty"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status"` // imported | failed
	Detail string `json:"detail,omitempty"`
}

// WorkspaceBundleImportResult summarizes one bundle import.
type WorkspaceBundleImportResult struct {
	WorkspaceID       int                            `json:"workspace_id"`
	PagesImported     int                            `json:"pages_imported"`
	ItemsImported     int                            `json:"items_imported"`
	ItemLinksImported int                            `json:"item_links_imported"`
	LabelsCreated     int                            `json:"labels_created"`
	Outcomes          []WorkspaceBundleImportOutcome `json:"outcomes"`
}

// WorkspaceBundleImportService is constructed once per request: the bundle
// and target workspace come in, outcomes come out.
type workspaceBundleImporter struct {
	db            database.Database
	ctx           context.Context
	actor         AuditActor
	workspaceID   int
	bundle        *WorkspaceBundle
	idempotent    bool
	outcomes      []WorkspaceBundleImportOutcome
	pageIDs       map[string]int
	failedPages   map[string]bool
	itemIDs       map[string]int
	failedItems   map[string]bool
	labelsCreated int
}

// WorkspaceBundleImportService owns the workspace bundle import flow.
type WorkspaceBundleImportService struct {
	db            database.Database
	configSetRepo *repository.ConfigurationSetRepository
	itemTypes     *repository.ItemTypeRepository
	labels        *repository.LabelRepository
	pages         *PageApplicationService
	pageLabels    *PageLabelService
	items         *ItemCreationService
	links         *ItemLinkService
	permissions   *PermissionService
}

func NewWorkspaceBundleImportService(
	db database.Database,
	configSetRepo *repository.ConfigurationSetRepository,
	itemTypes *repository.ItemTypeRepository,
	labels *repository.LabelRepository,
	pages *PageApplicationService,
	pageLabels *PageLabelService,
	items *ItemCreationService,
	links *ItemLinkService,
	permissions *PermissionService,
) *WorkspaceBundleImportService {
	return &WorkspaceBundleImportService{
		db: db, configSetRepo: configSetRepo, itemTypes: itemTypes, labels: labels,
		pages: pages, pageLabels: pageLabels, items: items, links: links, permissions: permissions,
	}
}

// AuditImport records a workspace bundle import with its outcome counts.
func (s *WorkspaceBundleImportService) AuditImport(actor AuditActor, workspaceID int, result *WorkspaceBundleImportResult) {
	if result == nil {
		return
	}
	emitServiceAudit(s.db, actor, logger.ActionWorkspaceBundleImport, logger.ResourceWorkspace, &workspaceID, "", map[string]any{
		"pages_imported":      result.PagesImported,
		"items_imported":      result.ItemsImported,
		"item_links_imported": result.ItemLinksImported,
		"labels_created":      result.LabelsCreated,
	})
}

// WorkspaceBundleImportOptions tunes an import run. Idempotent mode skips
// entities that already exist in the target workspace under the same stable
// name (page/item titles), which is what pack applies converge on.
type WorkspaceBundleImportOptions struct {
	Idempotent bool
}

// Import applies the bundle to the target workspace.
func (s *WorkspaceBundleImportService) Import(ctx context.Context, actor AuditActor, workspaceID int, bundle *WorkspaceBundle) (*WorkspaceBundleImportResult, error) {
	return s.ImportWithOptions(ctx, actor, workspaceID, bundle, nil)
}

// ImportWithOptions applies the bundle with optional idempotent resolution.
func (s *WorkspaceBundleImportService) ImportWithOptions(ctx context.Context, actor AuditActor, workspaceID int, bundle *WorkspaceBundle, opts *WorkspaceBundleImportOptions) (*WorkspaceBundleImportResult, error) {
	if bundle == nil || bundle.Payload.Pages == nil && bundle.Payload.Items == nil &&
		bundle.Payload.PageLabels == nil && bundle.Payload.Labels == nil && bundle.Payload.ItemLinks == nil &&
		bundle.ConfigurationSet == nil {
		return nil, ErrWorkspaceBundleEmpty
	}

	// 1. Refuse unresolved identity references before any write. The
	// embedded configuration-set template (when present) provides item
	// types, custom fields, and link types, so those names count as
	// resolvable.
	if err := s.validateReferences(ctx, workspaceID, bundle); err != nil {
		return nil, err
	}

	result := &WorkspaceBundleImportResult{WorkspaceID: workspaceID}
	imp := &workspaceBundleImporter{
		db: s.db, ctx: ctx, actor: actor, workspaceID: workspaceID, bundle: bundle,
		idempotent: opts != nil && opts.Idempotent,
		pageIDs:    map[string]int{}, failedPages: map[string]bool{},
		itemIDs: map[string]int{}, failedItems: map[string]bool{},
	}

	// 2. Embedded configuration-set template: apply through the existing
	// config-set import path and attach it to the target workspace.
	if bundle.ConfigurationSet != nil {
		importer := NewConfigSetImportService(s.db, s.configSetRepo)
		configSetID, _, err := importer.Import(ctx, bundle.ConfigurationSet)
		if err != nil {
			return nil, fmt.Errorf("embedded configuration set: %w", err)
		}
		// Attach: replace any existing workspace assignment (delete + insert,
		// matching SaveWorkspaceAssignments semantics) without a transaction
		// wrapper — the config-set import has already committed.
		if _, err := s.db.ExecContext(ctx, `DELETE FROM workspace_configuration_sets WHERE configuration_set_id = ?`, configSetID); err != nil {
			return nil, fmt.Errorf("attach embedded configuration set: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
			VALUES (?, ?, ?)
		`, workspaceID, configSetID, time.Now()); err != nil {
			return nil, fmt.Errorf("attach embedded configuration set: %w", err)
		}
		if s.permissions != nil {
			_ = s.permissions.OnConfigurationSetChanged(configSetID)
		}
	}

	// 3. Label catalogs (created on demand, dedup by name).
	if err := s.ensurePageLabels(imp); err != nil {
		return nil, err
	}
	if err := s.ensureItemLabels(imp); err != nil {
		return nil, err
	}

	// 4. Pages, then items, then links.
	pageLabelIDs, err := s.pageLabelIDMap(workspaceID)
	if err != nil {
		return nil, err
	}
	s.importPages(imp, pageLabelIDs)
	if err := s.importItems(ctx, imp, actor); err != nil {
		return nil, err
	}
	s.importItemLinks(imp)

	result.Outcomes = imp.outcomes
	result.LabelsCreated = imp.labelsCreated
	for _, o := range imp.outcomes {
		switch o.Entity {
		case "page":
			if o.Status == "imported" {
				result.PagesImported++
			}
		case "item":
			if o.Status == "imported" {
				result.ItemsImported++
			}
		case "item_link":
			if o.Status == "imported" {
				result.ItemLinksImported++
			}
		}
	}
	return result, nil
}

// validateReferences refuses bundles referencing identities the target
// cannot resolve, using the same structured-report contract as the
// config-set importer.
func (s *WorkspaceBundleImportService) validateReferences(ctx context.Context, workspaceID int, bundle *WorkspaceBundle) error {
	providedItemTypes := map[string]bool{}
	providedCustomFields := map[string]bool{}
	providedLinkTypes := map[string]bool{}
	if tpl := bundle.ConfigurationSet; tpl != nil {
		for _, it := range tpl.Payload.ItemTypes {
			providedItemTypes[lowerStr(it.Name)] = true
		}
		for _, cf := range tpl.Payload.CustomFields {
			providedCustomFields[lowerStr(cf.Name)] = true
		}
		for _, lt := range tpl.Payload.LinkTypes {
			providedLinkTypes[lowerStr(lt.Name)] = true
		}
	}

	var missing []UnresolvedRef

	workspaceTypes := map[string]bool{}
	types, err := s.itemTypes.ListForWorkspace(workspaceID)
	if err != nil {
		return err
	}
	for _, it := range types {
		workspaceTypes[lowerStr(it.Name)] = true
	}
	seenTypes := map[string]bool{}
	for _, item := range bundle.Payload.Items {
		name := lowerStr(item.ItemTypeName)
		if seenTypes[name] {
			continue
		}
		seenTypes[name] = true
		if !workspaceTypes[name] && !providedItemTypes[name] {
			missing = append(missing, UnresolvedRef{
				Kind: UnresolvedKindItemType, Name: item.ItemTypeName,
				Path: fmt.Sprintf("items/%s", item.Ref),
			})
		}
	}

	seenFields := map[string]bool{}
	for _, item := range bundle.Payload.Items {
		for fieldName := range item.FieldValues {
			key := lowerStr(fieldName)
			if seenFields[key] {
				continue
			}
			seenFields[key] = true
			if !providedCustomFields[key] {
				if id, _ := s.lookupCustomFieldID(ctx, fieldName); id == 0 {
					missing = append(missing, UnresolvedRef{
						Kind: UnresolvedKindCustomField, Name: fieldName,
						Path: fmt.Sprintf("items/%s", item.Ref),
					})
				}
			}
		}
	}

	seenLinkTypes := map[string]bool{}
	for _, link := range bundle.Payload.ItemLinks {
		name := lowerStr(link.LinkTypeName)
		if seenLinkTypes[name] {
			continue
		}
		seenLinkTypes[name] = true
		if !providedLinkTypes[name] {
			if id, _ := s.lookupLinkTypeID(ctx, link.LinkTypeName); id == 0 {
				missing = append(missing, UnresolvedRef{
					Kind: UnresolvedKindLinkType, Name: link.LinkTypeName,
					Path: fmt.Sprintf("item_links/%s→%s", link.SourceRef, link.TargetRef),
				})
			}
		}
	}

	seenEmails := map[string]bool{}
	for _, item := range bundle.Payload.Items {
		email := item.AssigneeEmail
		if email == "" || seenEmails[strings.ToLower(email)] {
			continue
		}
		seenEmails[strings.ToLower(email)] = true
		if id, _ := s.lookupUserID(ctx, email); id == 0 {
			missing = append(missing, UnresolvedRef{
				Kind: UnresolvedKindUser, Email: email,
				Path: fmt.Sprintf("items/%s", item.Ref),
			})
		}
	}

	if len(missing) > 0 {
		return &ErrUnresolvedReferences{Items: missing}
	}
	return nil
}

func (s *WorkspaceBundleImportService) lookupCustomFieldID(ctx context.Context, name string) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM custom_field_definitions WHERE LOWER(name) = LOWER(?)`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (s *WorkspaceBundleImportService) lookupLinkTypeID(ctx context.Context, name string) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM link_types WHERE LOWER(name) = LOWER(?) AND active = true`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (s *WorkspaceBundleImportService) lookupUserID(ctx context.Context, email string) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE LOWER(email) = LOWER(?)`, email).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// ---- application ------------------------------------------------------------

func (imp *workspaceBundleImporter) outcome(entity, ref, name, status, detail string) {
	imp.outcomes = append(imp.outcomes, WorkspaceBundleImportOutcome{
		Entity: entity, Ref: ref, Name: name, Status: status, Detail: detail,
	})
}

// ensurePageLabels creates page-label catalog entries the bundle references.
func (s *WorkspaceBundleImportService) ensurePageLabels(imp *workspaceBundleImporter) error {
	existing := map[string]bool{}
	catalog, err := s.pageLabels.List(imp.workspaceID)
	if err != nil {
		return err
	}
	for _, l := range catalog {
		existing[strings.ToLower(l.Name)] = true
	}
	for _, want := range imp.bundle.Payload.PageLabels {
		if existing[strings.ToLower(want.Name)] {
			continue
		}
		if _, err := s.pageLabels.Create(imp.workspaceID, want.Name, want.Color, imp.actor); err != nil {
			if errors.Is(err, repository.ErrDuplicateEntry) {
				continue
			}
			return fmt.Errorf("create page label %q: %w", want.Name, err)
		}
		imp.labelsCreated++
	}
	// Item labels also referenced from pages? Page label names come from the
	// page-label catalog; make sure page label names used by pages exist even
	// when absent from the catalog section (bundles may omit the catalog).
	for _, page := range imp.bundle.Payload.Pages {
		for _, name := range page.Labels {
			if existing[strings.ToLower(name)] {
				continue
			}
			if _, err := s.pageLabels.Create(imp.workspaceID, name, "", imp.actor); err != nil {
				if errors.Is(err, repository.ErrDuplicateEntry) {
					continue
				}
				return fmt.Errorf("create page label %q: %w", name, err)
			}
			imp.labelsCreated++
		}
	}
	return nil
}

// ensureItemLabels creates item-label catalog entries (global registry).
func (s *WorkspaceBundleImportService) ensureItemLabels(imp *workspaceBundleImporter) error {
	used := map[string]string{}
	for _, item := range imp.bundle.Payload.Items {
		for _, name := range item.Labels {
			used[strings.ToLower(name)] = name
		}
	}
	for _, want := range imp.bundle.Payload.Labels {
		used[strings.ToLower(want.Name)] = want.Name
	}
	for _, name := range used {
		if id, err := s.labels.FindIDByName(name); err == nil && id > 0 {
			continue
		}
		color := ""
		for _, want := range imp.bundle.Payload.Labels {
			if strings.EqualFold(want.Name, name) && want.Color != "" {
				color = want.Color
				break
			}
		}
		if _, _, err := s.labels.Create(name, color); err != nil {
			return fmt.Errorf("create label %q: %w", name, err)
		}
		imp.labelsCreated++
	}
	return nil
}

func (s *WorkspaceBundleImportService) pageLabelIDMap(workspaceID int) (map[string]int, error) {
	catalog, err := s.pageLabels.List(workspaceID)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, l := range catalog {
		out[strings.ToLower(l.Name)] = l.ID
	}
	return out, nil
}

func (s *WorkspaceBundleImportService) importPages(imp *workspaceBundleImporter, pageLabelIDs map[string]int) {
	for _, page := range imp.bundle.Payload.Pages {
		var parentID *int
		if page.ParentRef != "" {
			if imp.failedPages[page.ParentRef] {
				imp.failedPages[page.Ref] = true
				imp.outcome("page", page.Ref, page.Title, "failed", "parent page failed to import")
				continue
			}
			id, ok := imp.pageIDs[page.ParentRef]
			if !ok {
				imp.failedPages[page.Ref] = true
				imp.outcome("page", page.Ref, page.Title, "failed", fmt.Sprintf("parent ref %q not found in bundle", page.ParentRef))
				continue
			}
			parentID = &id
		}
		if imp.idempotent {
			var existingID int
			err := s.db.QueryRowContext(imp.ctx, `
				SELECT id FROM pages
				WHERE workspace_id = ? AND title = ? AND archived_at IS NULL
				ORDER BY id LIMIT 1
			`, imp.workspaceID, page.Title).Scan(&existingID)
			if err == nil {
				imp.pageIDs[page.Ref] = existingID
				imp.outcome("page", page.Ref, page.Title, "skipped", "page already exists")
				continue
			}
			if !errors.Is(err, sql.ErrNoRows) {
				imp.failedPages[page.Ref] = true
				imp.outcome("page", page.Ref, page.Title, "failed", err.Error())
				continue
			}
		}
		created, err := s.pages.Create(imp.actor, CreatePageInput{
			WorkspaceID: imp.workspaceID,
			ParentID:    parentID,
			Title:       page.Title,
			Content:     page.Content,
		})
		if err != nil {
			imp.failedPages[page.Ref] = true
			imp.outcome("page", page.Ref, page.Title, "failed", err.Error())
			continue
		}
		imp.pageIDs[page.Ref] = created.ID
		if len(page.Labels) > 0 {
			ids := make([]int, 0, len(page.Labels))
			for _, name := range page.Labels {
				if id, ok := pageLabelIDs[strings.ToLower(name)]; ok {
					ids = append(ids, id)
				}
			}
			if len(ids) > 0 {
				if _, err := s.pageLabels.SetForPage(imp.workspaceID, created.ID, ids); err != nil {
					imp.outcome("page", page.Ref, page.Title, "failed", "page imported but labels failed: "+err.Error())
					continue
				}
			}
		}
		imp.outcome("page", page.Ref, page.Title, "imported", "")
	}
}

func (s *WorkspaceBundleImportService) importItems(ctx context.Context, imp *workspaceBundleImporter, actor AuditActor) error {
	workspaceTypes := map[string]int{}
	types, err := s.itemTypes.ListForWorkspace(imp.workspaceID)
	if err != nil {
		return err
	}
	for _, it := range types {
		workspaceTypes[strings.ToLower(it.Name)] = it.ID
	}

	for _, item := range imp.bundle.Payload.Items {
		typeID, ok := workspaceTypes[strings.ToLower(item.ItemTypeName)]
		if !ok {
			imp.failedItems[item.Ref] = true
			imp.outcome("item", item.Ref, item.Title, "failed", fmt.Sprintf("item type %q is not available in the target workspace", item.ItemTypeName))
			continue
		}
		if imp.idempotent {
			var existingID int
			err := s.db.QueryRowContext(imp.ctx, `
				SELECT i.id FROM items i
				JOIN item_types it ON it.id = i.item_type_id
				WHERE i.workspace_id = ? AND i.title = ? AND it.name = ?
				ORDER BY i.id LIMIT 1
			`, imp.workspaceID, item.Title, item.ItemTypeName).Scan(&existingID)
			if err == nil {
				imp.itemIDs[item.Ref] = existingID
				imp.outcome("item", item.Ref, item.Title, "skipped", "item already exists")
				continue
			}
			if !errors.Is(err, sql.ErrNoRows) {
				imp.failedItems[item.Ref] = true
				imp.outcome("item", item.Ref, item.Title, "failed", err.Error())
				continue
			}
		}
		input := ItemCreateInput{
			WorkspaceID: imp.workspaceID,
			Title:       item.Title,
			Description: item.Description,
			ItemTypeID:  &typeID,
		}
		if item.PriorityName != "" {
			var priorityID int
			if err := s.db.QueryRowContext(ctx, `SELECT id FROM priorities WHERE LOWER(name) = LOWER(?)`, item.PriorityName).Scan(&priorityID); err == nil {
				input.PriorityID = &priorityID
			}
		}
		if item.AssigneeEmail != "" {
			var userID int
			if err := s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE LOWER(email) = LOWER(?)`, item.AssigneeEmail).Scan(&userID); err == nil {
				input.AssigneeID = &userID
			}
		}
		if item.ParentRef != "" {
			if imp.failedItems[item.ParentRef] {
				imp.failedItems[item.Ref] = true
				imp.outcome("item", item.Ref, item.Title, "failed", "parent item failed to import")
				continue
			}
			if id, ok := imp.itemIDs[item.ParentRef]; ok {
				input.ParentID = &id
			}
		}
		if len(item.FieldValues) > 0 {
			values := make(map[string]any, len(item.FieldValues))
			for fieldName, value := range item.FieldValues {
				var fieldID int
				if err := s.db.QueryRowContext(ctx, `SELECT id FROM custom_field_definitions WHERE LOWER(name) = LOWER(?)`, fieldName).Scan(&fieldID); err != nil {
					continue
				}
				values[strconv.Itoa(fieldID)] = value
			}
			input.CustomFieldValues = values
		}
		if len(item.Labels) > 0 {
			ids := make([]int, 0, len(item.Labels))
			for _, name := range item.Labels {
				if labelID, err := s.labels.FindIDByName(name); err == nil && labelID > 0 {
					ids = append(ids, labelID)
				}
			}
			input.LabelIDs = ids
		}

		created, err := s.items.Create(actor.UserID, actor.Username, input)
		if err != nil {
			imp.failedItems[item.Ref] = true
			imp.outcome("item", item.Ref, item.Title, "failed", err.Error())
			continue
		}
		imp.itemIDs[item.Ref] = created.Item.ID
		imp.outcome("item", item.Ref, item.Title, "imported", "")
	}
	return nil
}

func (s *WorkspaceBundleImportService) importItemLinks(imp *workspaceBundleImporter) {
	for _, link := range imp.bundle.Payload.ItemLinks {
		sourceID, ok := imp.itemIDs[link.SourceRef]
		if !ok {
			imp.outcome("item_link", link.SourceRef+"→"+link.TargetRef, link.LinkTypeName, "failed", "source item was not imported")
			continue
		}
		targetID, ok := imp.itemIDs[link.TargetRef]
		if !ok {
			imp.outcome("item_link", link.SourceRef+"→"+link.TargetRef, link.LinkTypeName, "failed", "target item was not imported")
			continue
		}
		var linkTypeID int
		if err := s.db.QueryRowContext(imp.ctx, `SELECT id FROM link_types WHERE LOWER(name) = LOWER(?) AND active = true`, link.LinkTypeName).Scan(&linkTypeID); err != nil {
			imp.outcome("item_link", link.SourceRef+"→"+link.TargetRef, link.LinkTypeName, "failed", "link type is not available")
			continue
		}
		createdBy := imp.actor.UserID
		if _, err := s.links.CreateLink(CreateItemLinkParams{
			LinkTypeID: linkTypeID,
			SourceType: "item", SourceID: sourceID,
			TargetType: "item", TargetID: targetID,
			CreatedBy: &createdBy,
		}); err != nil {
			imp.outcome("item_link", link.SourceRef+"→"+link.TargetRef, link.LinkTypeName, "failed", err.Error())
			continue
		}
		imp.outcome("item_link", link.SourceRef+"→"+link.TargetRef, link.LinkTypeName, "imported", "")
	}
}
