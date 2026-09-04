package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/validation"
)

// Errors returned by workspace template operations. Handlers map these to the
// documented HTTP contracts; they wrap with context but must stay identifiable
// via errors.Is.
var (
	// ErrTemplateWorkspaceNotFound maps to 404 TEMPLATE_WORKSPACE_NOT_FOUND:
	// the source is missing or not visible to the caller, so its existence is
	// not disclosed.
	ErrTemplateWorkspaceNotFound = errors.New("template workspace not found or not visible")
	// ErrInvalidWorkspaceTemplate maps to 422 INVALID_WORKSPACE_TEMPLATE: the
	// source is visible but not a usable template (personal, inactive, not
	// marked, or an inconsistent snapshot).
	ErrInvalidWorkspaceTemplate = errors.New("workspace cannot be used as a template")
	// ErrWorkspaceTemplateTooLarge maps to 422 WORKSPACE_TEMPLATE_TOO_LARGE.
	ErrWorkspaceTemplateTooLarge = errors.New("template workspace exceeds the seed item limit")
	// ErrPersonalWorkspaceTemplate rejects marking a personal workspace as a
	// template.
	ErrPersonalWorkspaceTemplate = errors.New("personal workspaces cannot be templates")
)

// MaxTemplateSeedItems bounds how many seed items one clone may copy. The
// first release keeps clones small; the value is overridden from config at
// startup.
var MaxTemplateSeedItems = 1000

// workspaceCloneMaxAttempts bounds whole-transaction retries for PostgreSQL
// serialization aborts and rare rank/item-number unique collisions.
const workspaceCloneMaxAttempts = 3

// WorkspaceSourceAccess applies the authz.CanViewWorkspace (item.view) rule
// inside the clone transaction, so source visibility is decided on the same
// snapshot the clone reads. Implemented by the authorization layer.
type WorkspaceSourceAccess interface {
	CanViewWorkspaceTx(ctx context.Context, tx database.Tx, userID, workspaceID int) (bool, error)
}

// createWorkspaceTx runs one full creation attempt inside a transaction. Any
// error aborts the transaction leaving no destination rows behind.
func (s *WorkspaceService) createWorkspaceTx(ctx context.Context, tx database.Tx, params CreateWorkspaceParams, key string) (*CreateWorkspaceResult, error) {
	result := &CreateWorkspaceResult{}
	clone := workspaceTemplateClone{
		service: s,
		tx:      tx,
		ctx:     ctx,
		params:  params,
		creator: params.CreatorID,
		now:     time.Now().UTC(),
		idMap:   make(map[int]int),
	}

	source, err := clone.authorizeSource()
	if err != nil {
		return nil, err
	}
	if source != nil {
		result.SourceWorkspaceID = source.id
	}

	newID, err := s.repo.CreateTx(tx, &models.Workspace{
		Name:          params.Name,
		Key:           key,
		Description:   params.Description,
		Active:        params.Active == nil || *params.Active,
		TimeProjectID: params.TimeProjectID,
		IsPersonal:    params.IsPersonal,
		OwnerID:       params.OwnerID,
		Icon:          params.Icon,
		Color:         params.Color,
		AvatarURL:     params.AvatarURL,
		DefaultView:   params.DefaultView,
	})
	if err != nil {
		return nil, err
	}
	clone.destinationID = int(newID)

	// CreatorID 0 means "unknown actor" (Jira import jobs); those workspaces
	// are created without an administrator grant, matching the legacy path.
	if params.CreatorID > 0 {
		if err := s.repo.GrantAdministratorRoleTx(tx, newID, params.CreatorID); err != nil {
			return nil, err
		}
	}

	if source != nil {
		configSetAttached, err := clone.attachConfigurationSet(source)
		if err != nil {
			return nil, err
		}
		result.ConfigSetAttached = configSetAttached

		templatesCopied, err := clone.copyItemTemplates(source)
		if err != nil {
			return nil, err
		}
		result.TemplatesCopied = templatesCopied

		itemsCopied, omittedValues, err := clone.copySeedItems(source)
		if err != nil {
			return nil, err
		}
		result.ItemsCopied = itemsCopied
		result.OmittedCustomFieldValues = omittedValues
	}

	workspace, err := clone.hydrateDestination()
	if err != nil {
		return nil, err
	}
	result.Workspace = workspace
	return result, nil
}

type templateSourceWorkspace struct {
	id int
}

type workspaceTemplateClone struct {
	service *WorkspaceService
	tx      database.Tx
	ctx     context.Context
	params  CreateWorkspaceParams
	creator int
	now     time.Time

	sourceID      int
	destinationID int
	idMap         map[int]int
}

// authorizeSource validates the requested template source inside the clone
// transaction. A nil result means blank creation (no template requested).
func (c *workspaceTemplateClone) authorizeSource() (*templateSourceWorkspace, error) {
	if c.params.TemplateWorkspaceID == nil {
		return nil, nil
	}
	if c.params.IsPersonal {
		return nil, fmt.Errorf("%w: personal workspaces cannot be created from a template", ErrInvalidWorkspaceTemplate)
	}
	if c.service.access == nil {
		return nil, errors.New("workspace template access checker is not configured")
	}

	sourceID := *c.params.TemplateWorkspaceID
	if sourceID <= 0 {
		return nil, fmt.Errorf("%w: template workspace id must be positive", ErrInvalidWorkspaceTemplate)
	}

	visible, err := c.service.access.CanViewWorkspaceTx(c.ctx, c.tx, c.creator, sourceID)
	if err != nil {
		return nil, fmt.Errorf("check template source visibility: %w", err)
	}
	if !visible {
		return nil, ErrTemplateWorkspaceNotFound
	}

	eligibility, err := c.service.templates.LoadTemplateEligibilityTx(c.ctx, c.tx, sourceID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrTemplateWorkspaceNotFound
	}
	if err != nil {
		return nil, err
	}
	if eligibility.IsPersonal || !eligibility.Active || !eligibility.IsTemplate {
		return nil, fmt.Errorf("%w: source must be an active, non-personal workspace marked as a template", ErrInvalidWorkspaceTemplate)
	}

	count, err := c.service.templates.CountWorkspaceItemsTx(c.ctx, c.tx, sourceID)
	if err != nil {
		return nil, err
	}
	if count > MaxTemplateSeedItems {
		return nil, fmt.Errorf("%w: %d seed items exceed the limit of %d", ErrWorkspaceTemplateTooLarge, count, MaxTemplateSeedItems)
	}

	c.sourceID = sourceID
	return &templateSourceWorkspace{id: sourceID}, nil
}

// attachConfigurationSet shares the source configuration set with the
// destination by inserting a new assignment row.
func (c *workspaceTemplateClone) attachConfigurationSet(source *templateSourceWorkspace) (bool, error) {
	configSetID, count, err := c.service.templates.GetWorkspaceConfigSetTx(c.ctx, c.tx, source.id)
	if err != nil {
		return false, err
	}
	switch count {
	case 0:
		return false, nil
	case 1:
		if err := c.service.templates.AttachConfigurationSetTx(c.ctx, c.tx, c.destinationID, *configSetID, c.now); err != nil {
			return false, err
		}
		return true, nil
	default:
		return false, fmt.Errorf("%w: source has %d configuration-set assignments, at most one is allowed", ErrInvalidWorkspaceTemplate, count)
	}
}

// copyItemTemplates clones every source work-item template with fresh IDs and
// the clone actor/timestamps, rejecting inconsistent mandatory templates.
func (c *workspaceTemplateClone) copyItemTemplates(source *templateSourceWorkspace) (int, error) {
	templates, err := c.service.templates.ListItemTemplatesTx(c.ctx, c.tx, source.id)
	if err != nil {
		return 0, err
	}
	for _, tmpl := range templates {
		if tmpl.Mode != models.TemplateModeSelectable && tmpl.Mode != models.TemplateModeMandatory {
			return 0, fmt.Errorf("%w: template %q has unknown mode %q", ErrInvalidWorkspaceTemplate, tmpl.Name, tmpl.Mode)
		}
		if tmpl.Mode == models.TemplateModeMandatory && tmpl.IsActive && len(tmpl.ItemTypeIDs) != 1 {
			return 0, fmt.Errorf("%w: active mandatory template %q must target exactly one item type", ErrInvalidWorkspaceTemplate, tmpl.Name)
		}
	}
	for _, tmpl := range templates {
		if _, err := c.service.templates.InsertItemTemplateTx(c.ctx, c.tx, c.destinationID, tmpl, c.creator, c.now); err != nil {
			return 0, err
		}
	}
	return len(templates), nil
}

// copySeedItems clones the source items with fresh IDs, dense destination
// numbers in source item-number order, and fresh fractional indexes in source
// frac-index order. Hierarchy, labels, and internal links are restored from
// the recorded ID map.
func (c *workspaceTemplateClone) copySeedItems(source *templateSourceWorkspace) (itemsCopied, omittedValues int, err error) {
	items, err := c.service.templates.ListSeedItemsTx(c.ctx, c.tx, source.id)
	if err != nil {
		return 0, 0, err
	}
	if len(items) == 0 {
		return 0, 0, nil
	}

	if err := c.validateItemReferences(items); err != nil {
		return 0, 0, err
	}

	defs, err := c.loadCustomFieldDefinitions(items)
	if err != nil {
		return 0, 0, err
	}

	fracKeys, err := c.generateFracIndexes(items)
	if err != nil {
		return 0, 0, err
	}

	omittedValues = 0
	for i, item := range items {
		var statusID *int
		if item.StatusID.Valid {
			value := int(item.StatusID.Int64)
			statusID = &value
		}
		if err := validation.ValidateTaskState(c.tx, c.destinationID, c.creator, item.IsTask, statusID); err != nil {
			return 0, 0, fmt.Errorf("validate seed item %d: %w", item.ID, errors.Join(ErrInvalidWorkspaceTemplate, err))
		}
		customFields, omitted, err := filterTemplateCustomFieldValues(item.CustomFieldValues, defs)
		if err != nil {
			return 0, 0, err
		}
		omittedValues += omitted

		newID, err := c.service.templates.InsertClonedItemTx(c.ctx, c.tx, repository.ClonedItemInsert{
			WorkspaceID:       c.destinationID,
			ItemNumber:        i + 1,
			ItemTypeID:        item.ItemTypeID,
			Title:             item.Title,
			Description:       item.Description,
			IsTask:            item.IsTask,
			StatusID:          item.StatusID,
			PriorityID:        item.PriorityID,
			StartDate:         item.StartDate,
			DueDate:           item.DueDate,
			EndDate:           item.EndDate,
			StoryPoints:       item.StoryPoints,
			EstimateMinutes:   item.EstimateMinutes,
			FracIndex:         fracKeys[item.ID],
			CustomFieldValues: customFields,
			CreatorID:         c.creator,
			CreatedAt:         c.now,
		})
		if err != nil {
			return 0, 0, err
		}
		c.idMap[item.ID] = int(newID)
	}

	if err := c.restoreHierarchy(items); err != nil {
		return 0, 0, err
	}
	if err := c.copyLabels(source); err != nil {
		return 0, 0, err
	}
	if err := c.recordCreatedItems(); err != nil {
		return 0, 0, err
	}
	if err := c.copyItemLinks(source); err != nil {
		return 0, 0, err
	}
	return len(items), omittedValues, nil
}

func (c *workspaceTemplateClone) eventMetadata() itemevents.Metadata {
	metadata := itemevents.System("template")
	if c.creator > 0 {
		metadata = itemevents.User(c.creator, "template")
	}
	metadata.OccurredAt = c.now
	return metadata
}

func (c *workspaceTemplateClone) recordCreatedItems() error {
	ids := make([]int, 0, len(c.idMap))
	for _, id := range c.idMap {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	items, err := repository.NewItemRepository(c.service.db).FindByIDsForUpdateContext(c.ctx, c.tx, ids)
	if err != nil {
		return err
	}
	records := make([]itemevents.CreateRecord, 0, len(items))
	metadata := c.eventMetadata()
	for _, item := range items {
		records = append(records, itemevents.CreateRecord{Item: item, Metadata: metadata})
	}
	_, err = itemevents.NewRecorder(c.service.db).CreatedBatch(c.ctx, c.tx, records)
	return err
}

// validateItemReferences rejects snapshots referencing deleted catalog rows
// before any destination item is written.
func (c *workspaceTemplateClone) validateItemReferences(items []repository.TemplateCloneItem) error {
	itemTypeIDs := collectNotNullInts(items, func(item repository.TemplateCloneItem) sql.NullInt64 { return item.ItemTypeID })
	missing, err := c.service.templates.ReferencedCatalogIDsMissingTx(c.ctx, c.tx, "item_types", itemTypeIDs)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: source references missing item types %v", ErrInvalidWorkspaceTemplate, missing)
	}

	statusIDs := collectNotNullInts(items, func(item repository.TemplateCloneItem) sql.NullInt64 { return item.StatusID })
	missing, err = c.service.templates.ReferencedCatalogIDsMissingTx(c.ctx, c.tx, "statuses", statusIDs)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: source references missing statuses %v", ErrInvalidWorkspaceTemplate, missing)
	}

	priorityIDs := collectNotNullInts(items, func(item repository.TemplateCloneItem) sql.NullInt64 { return item.PriorityID })
	missing, err = c.service.templates.ReferencedCatalogIDsMissingTx(c.ctx, c.tx, "priorities", priorityIDs)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: source references missing priorities %v", ErrInvalidWorkspaceTemplate, missing)
	}
	return nil
}

func collectNotNullInts[T any](items []T, get func(T) sql.NullInt64) []int {
	seen := make(map[int]bool)
	var ids []int
	for _, item := range items {
		value := get(item)
		if value.Valid && !seen[int(value.Int64)] {
			seen[int(value.Int64)] = true
			ids = append(ids, int(value.Int64))
		}
	}
	return ids
}

// loadCustomFieldDefinitions parses every source custom_field_values blob
// (malformed JSON aborts the clone) and loads the referenced definitions.
func (c *workspaceTemplateClone) loadCustomFieldDefinitions(items []repository.TemplateCloneItem) (map[int]repository.TemplateCustomFieldDef, error) {
	fieldIDSeen := make(map[int]bool)
	var fieldIDs []int
	for _, item := range items {
		if !item.CustomFieldValues.Valid || item.CustomFieldValues.String == "" {
			continue
		}
		var values map[string]any
		if err := json.Unmarshal([]byte(item.CustomFieldValues.String), &values); err != nil {
			return nil, fmt.Errorf("%w: item %d has malformed custom_field_values JSON", ErrInvalidWorkspaceTemplate, item.ID)
		}
		for key := range values {
			fieldID, err := strconv.Atoi(key)
			if err != nil {
				continue
			}
			if !fieldIDSeen[fieldID] {
				fieldIDSeen[fieldID] = true
				fieldIDs = append(fieldIDs, fieldID)
			}
		}
	}
	return c.service.templates.LoadCustomFieldDefsTx(c.ctx, c.tx, fieldIDs)
}

// generateFracIndexes allocates fresh globally unique fractional indexes that
// preserve the source relative order.
func (c *workspaceTemplateClone) generateFracIndexes(items []repository.TemplateCloneItem) (map[int]string, error) {
	ordered := make([]repository.TemplateCloneItem, len(items))
	copy(ordered, items)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].FracIndex < ordered[j].FracIndex })

	keys, err := repository.GenerateFracIndexesForBatch(c.tx, len(ordered), c.service.db.GetDriverName())
	if err != nil {
		return nil, err
	}
	byOldID := make(map[int]string, len(ordered))
	for i, item := range ordered {
		byOldID[item.ID] = keys[i]
	}
	return byOldID, nil
}

// restoreHierarchy remaps parent and related-item references and rebuilds the
// materialized hierarchy path. References outside the copied set are cleared.
func (c *workspaceTemplateClone) restoreHierarchy(items []repository.TemplateCloneItem) error {
	byOldID := make(map[int]repository.TemplateCloneItem, len(items))
	for _, item := range items {
		byOldID[item.ID] = item
	}

	paths := make(map[int]string, len(items))
	visiting := make(map[int]bool, len(items))
	var pathFor func(int) (string, error)
	pathFor = func(oldID int) (string, error) {
		if path, ok := paths[oldID]; ok {
			return path, nil
		}
		if visiting[oldID] {
			return "", fmt.Errorf("%w: item hierarchy contains a cycle at item %d", ErrInvalidWorkspaceTemplate, oldID)
		}
		item, ok := byOldID[oldID]
		if !ok {
			return "/", nil
		}

		visiting[oldID] = true
		defer delete(visiting, oldID)

		path := "/"
		if item.ParentID.Valid {
			parentOldID := int(item.ParentID.Int64)
			parentPath, err := pathFor(parentOldID)
			if err != nil {
				return "", err
			}
			if parentNewID, ok := c.idMap[parentOldID]; ok {
				path = parentPath + strconv.Itoa(parentNewID) + "/"
			}
		}
		paths[oldID] = path
		return path, nil
	}

	for _, item := range items {
		newID := c.idMap[item.ID]
		path, err := pathFor(item.ID)
		if err != nil {
			return err
		}

		var parentID *int64
		if item.ParentID.Valid {
			if mapped, ok := c.idMap[int(item.ParentID.Int64)]; ok {
				mapped64 := int64(mapped)
				parentID = &mapped64
			}
		}

		var relatedID *int64
		if item.RelatedWorkItemID.Valid {
			if mapped, ok := c.idMap[int(item.RelatedWorkItemID.Int64)]; ok {
				mapped64 := int64(mapped)
				relatedID = &mapped64
			}
		}

		if parentID == nil && !item.ParentID.Valid && relatedID == nil && !item.RelatedWorkItemID.Valid {
			continue
		}
		if err := c.service.templates.RestoreClonedHierarchyTx(c.ctx, c.tx, int64(newID), path, parentID, relatedID); err != nil {
			return err
		}
	}
	return nil
}

// copyLabels copies global item-label associations for the cloned items.
func (c *workspaceTemplateClone) copyLabels(source *templateSourceWorkspace) error {
	labels, err := c.service.templates.ListItemLabelsTx(c.ctx, c.tx, source.id)
	if err != nil {
		return err
	}
	for _, label := range labels {
		newItemID, ok := c.idMap[label.ItemID]
		if !ok {
			continue
		}
		if err := c.service.templates.InsertItemLabelTx(c.ctx, c.tx, newItemID, label.LabelID, c.now); err != nil {
			return err
		}
	}
	return nil
}

// copyItemLinks copies item-to-item links whose endpoints were both copied,
// remapping both endpoints to destination IDs.
func (c *workspaceTemplateClone) copyItemLinks(source *templateSourceWorkspace) error {
	links, err := c.service.templates.ListItemLinksTx(c.ctx, c.tx, source.id)
	if err != nil {
		return err
	}
	for _, link := range links {
		newSourceID, ok := c.idMap[link.SourceID]
		if !ok {
			continue
		}
		newTargetID, ok := c.idMap[link.TargetID]
		if !ok {
			continue
		}
		remapped := link
		remapped.SourceID = newSourceID
		remapped.TargetID = newTargetID
		linkID, err := c.service.templates.InsertItemLinkTx(c.ctx, c.tx, remapped, c.creator, c.now)
		if err != nil {
			return err
		}
		if linkID == 0 {
			continue
		}
		if _, err := itemevents.NewRecorder(c.service.db).LinkChanged(c.ctx, c.tx, itemevents.Linked, models.ItemLink{
			ID: linkID, LinkTypeID: remapped.LinkTypeID,
			SourceType: "item", SourceID: remapped.SourceID,
			TargetType: "item", TargetID: remapped.TargetID,
		}, c.eventMetadata()); err != nil {
			return err
		}
	}
	return nil
}

func (c *workspaceTemplateClone) hydrateDestination() (*models.Workspace, error) {
	rows := c.tx.QueryRowContext(c.ctx, `
		SELECT id, name, key, description, active, is_template, time_project_id, is_personal, owner_id, icon, color,
		       avatar_url, default_view, internal_comments_enabled, created_at, updated_at
		FROM workspaces WHERE id = ?
	`, c.destinationID)

	var workspace models.Workspace
	var icon, color, defaultView sql.NullString
	err := rows.Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
		&workspace.Active, &workspace.IsTemplate, &workspace.TimeProjectID, &workspace.IsPersonal, &workspace.OwnerID,
		&icon, &color, &workspace.AvatarURL, &defaultView,
		&workspace.InternalCommentsEnabled, &workspace.CreatedAt, &workspace.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("hydrate cloned workspace: %w", err)
	}
	workspace.Icon = icon.String
	workspace.Color = color.String
	workspace.DefaultView = defaultView.String
	return &workspace, nil
}

// filterTemplateCustomFieldValues applies the strict copy policy: keep only
// value types that remain meaningful in the shared configuration set after
// the existing validators' rules (option membership for choices, real booleans
// and numbers, strings for text-like fields). Malformed JSON is an error;
// dropped values are counted for the clone audit.
func filterTemplateCustomFieldValues(raw sql.NullString, defs map[int]repository.TemplateCustomFieldDef) (sql.NullString, int, error) {
	if !raw.Valid || raw.String == "" {
		return sql.NullString{}, 0, nil
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(raw.String), &values); err != nil {
		return sql.NullString{}, 0, fmt.Errorf("malformed custom_field_values JSON: %w", err)
	}

	out := make(map[string]any, len(values))
	omitted := 0
	for key, value := range values {
		fieldID, err := strconv.Atoi(key)
		if err != nil {
			omitted++
			continue
		}
		def, ok := defs[fieldID]
		if !ok {
			omitted++
			continue
		}
		kept, keep := acceptTemplateCustomFieldValue(def, value)
		if !keep {
			omitted++
			continue
		}
		out[key] = kept
	}

	if len(out) == 0 {
		return sql.NullString{}, omitted, nil
	}
	data, err := json.Marshal(out)
	if err != nil {
		return sql.NullString{}, 0, fmt.Errorf("marshal cloned custom_field_values: %w", err)
	}
	return sql.NullString{String: string(data), Valid: true}, omitted, nil
}

// acceptTemplateCustomFieldValue decides whether one custom field value is
// copied and in which normalized form.
func acceptTemplateCustomFieldValue(def repository.TemplateCustomFieldDef, value any) (any, bool) {
	if value == nil {
		return nil, false
	}
	switch def.FieldType {
	case "text", "textarea", "date":
		s, ok := value.(string)
		return s, ok
	case "number":
		n, ok := value.(float64)
		return n, ok
	case models.CustomFieldTypeBoolean, models.CustomFieldTypeCheckbox:
		b, ok := value.(bool)
		return b, ok
	case "select":
		return acceptTemplateOptionValue(def, value)
	case "multiselect":
		items, ok := value.([]any)
		if !ok {
			return nil, false
		}
		kept := make([]int, 0, len(items))
		seen := make(map[int]bool, len(items))
		for _, item := range items {
			id, ok := acceptTemplateOptionValue(def, item)
			if !ok {
				continue
			}
			idInt, _ := id.(int)
			if seen[idInt] {
				continue
			}
			seen[idInt] = true
			kept = append(kept, idInt)
		}
		if len(kept) == 0 {
			return nil, false
		}
		return kept, true
	default:
		// user, multi_user, milestone, iteration, asset, portalcustomer,
		// customerorganisation, linking, and unknown types are dropped.
		return nil, false
	}
}

// acceptTemplateOptionValue validates one select option reference against the
// definition's current option set, mirroring the create-path validator.
func acceptTemplateOptionValue(def repository.TemplateCustomFieldDef, value any) (any, bool) {
	id, ok := coerceTemplateOptionID(value)
	if !ok {
		return nil, false
	}
	options, err := models.ParseSelectOptions(def.Options)
	if err != nil {
		return nil, false
	}
	for _, item := range options.Items {
		if item.ID == id {
			return id, true
		}
	}
	return nil, false
}

func coerceTemplateOptionID(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	case int64:
		return int(x), true
	case string:
		n, err := strconv.Atoi(x)
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

// isWorkspaceCloneRetryable reports whether a failed creation attempt should
// be retried on a fresh transaction: PostgreSQL serialization aborts and the
// rare global rank or workspace item-number unique collisions.
func isWorkspaceCloneRetryable(err error) bool {
	return repository.IsItemCreateRetryable(err)
}

// logWorkspaceCloneResult records clone outcome and volume for operations
// debugging.
func logWorkspaceCloneResult(result *CreateWorkspaceResult, duration time.Duration) {
	slog.Info("workspace created from template",
		slog.String("component", "workspaces"),
		slog.Int("workspace_id", result.Workspace.ID),
		slog.Int("source_workspace_id", result.SourceWorkspaceID),
		slog.Bool("config_set_attached", result.ConfigSetAttached),
		slog.Int("templates_copied", result.TemplatesCopied),
		slog.Int("items_copied", result.ItemsCopied),
		slog.Int("omitted_custom_field_values", result.OmittedCustomFieldValues),
		slog.Duration("duration", duration),
	)
}
