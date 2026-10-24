package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
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
	ViewsApplied      bool                           `json:"views_applied"`
	CalendarsImported int                            `json:"calendars_imported"`
	MetricsImported   int                            `json:"metrics_imported"`
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
	boards        *repository.BoardConfigurationRepository
	pages         *PageApplicationService
	pageLabels    *PageLabelService
	items         *ItemCreationService
	links         *ItemLinkService
	permissions   *PermissionService
	slaCalendars  *SLACalendarService
	slaMetrics    *SLAMetricService
}

func NewWorkspaceBundleImportService(
	db database.Database,
	configSetRepo *repository.ConfigurationSetRepository,
	itemTypes *repository.ItemTypeRepository,
	labels *repository.LabelRepository,
	boards *repository.BoardConfigurationRepository,
	pages *PageApplicationService,
	pageLabels *PageLabelService,
	items *ItemCreationService,
	links *ItemLinkService,
	permissions *PermissionService,
	slaCalendars *SLACalendarService,
	slaMetrics *SLAMetricService,
) *WorkspaceBundleImportService {
	return &WorkspaceBundleImportService{
		db: db, configSetRepo: configSetRepo, itemTypes: itemTypes, labels: labels, boards: boards,
		pages: pages, pageLabels: pageLabels, items: items, links: links, permissions: permissions,
		slaCalendars: slaCalendars, slaMetrics: slaMetrics,
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
		"views_applied":       result.ViewsApplied,
		"calendars_imported":  result.CalendarsImported,
		"metrics_imported":    result.MetricsImported,
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
		bundle.Payload.ViewSettings == nil && bundle.Payload.SLAs == nil &&
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
		// Attach: replace this workspace's assignment (delete + insert,
		// matching SaveWorkspaceAssignments semantics) in one transaction —
		// the config-set import has already committed. Scoping the delete to
		// configuration_set_id would detach the set from every other
		// workspace sharing it.
		if err := database.WithTx(s.db, func(tx database.Tx) error {
			if _, err := tx.ExecContext(ctx, `DELETE FROM workspace_configuration_sets WHERE workspace_id = ?`, workspaceID); err != nil {
				return fmt.Errorf("attach embedded configuration set: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO workspace_configuration_sets (workspace_id, configuration_set_id, created_at)
				VALUES (?, ?, ?)
			`, workspaceID, configSetID, time.Now()); err != nil {
				return fmt.Errorf("attach embedded configuration set: %w", err)
			}
			return nil
		}); err != nil {
			return nil, err
		}
		if s.permissions != nil {
			_ = s.permissions.OnConfigurationSetChanged(configSetID)
		}
	}

	// 2b. Workspace nav preset and SLA seeds. SLA name resolution depends on
	// the configuration set's statuses and priorities, so this runs after the
	// embedded set import above (and after the pack schema stage in apply).
	if bundle.Payload.ViewSettings != nil {
		if err := s.applyViewSettings(workspaceID, bundle.Payload.ViewSettings); err != nil {
			return nil, err
		}
		result.ViewsApplied = true
	}
	if bundle.Payload.SLAs != nil {
		calendarsImported, metricsImported, err := s.applySLAs(ctx, workspaceID, bundle.Payload.SLAs)
		if err != nil {
			return nil, err
		}
		result.CalendarsImported = calendarsImported
		result.MetricsImported = metricsImported
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

// applyViewSettings writes the workspace-scope nav preset, preserving the rest
// of the board configuration when one already exists.
func (s *WorkspaceBundleImportService) applyViewSettings(workspaceID int, preset *WorkspaceBundleViewSettings) error {
	if s.boards == nil {
		return errors.New("workspace bundle import: board configuration repository is not configured")
	}
	if len(preset.EnabledViews) == 0 {
		return errors.New("workspace bundle import: view_settings.enabled_views must contain at least one view")
	}
	seen := make(map[string]bool, len(preset.EnabledViews))
	for _, id := range preset.EnabledViews {
		if !models.IsWorkspaceNavID(id) {
			return fmt.Errorf("workspace bundle import: unknown view %q in view_settings.enabled_views", id)
		}
		if seen[id] {
			return fmt.Errorf("workspace bundle import: duplicate view %q in view_settings.enabled_views", id)
		}
		seen[id] = true
	}
	settings := &models.ViewSettings{EnabledViews: &preset.EnabledViews}
	existing, err := s.boards.GetByWorkspaceID(workspaceID)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		if _, err := s.boards.Create(nil, &workspaceID, &models.BoardConfigurationRequest{ViewSettings: settings}); err != nil {
			return fmt.Errorf("create workspace view settings: %w", err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("load workspace view settings: %w", err)
	}
	merged := mergeViewSettings(existing.ViewSettings, settings)
	if err := s.boards.SetViewSettings(existing.ID, merged); err != nil {
		return fmt.Errorf("update workspace view settings: %w", err)
	}
	return nil
}

// applySLAs seeds the bundle's working calendars and SLA metrics. It is
// idempotent: calendars and metrics already present under the same name are
// reused or skipped, so re-applying a pack converges.
func (s *WorkspaceBundleImportService) applySLAs(ctx context.Context, workspaceID int, slas *WorkspaceBundleSLAs) (calendarsImported, metricsImported int, err error) {
	if s.slaCalendars == nil || s.slaMetrics == nil {
		return 0, 0, errors.New("workspace bundle import: SLA services are not configured")
	}

	existingCalendars, err := s.slaCalendars.ListWorkspace(ctx, workspaceID)
	if err != nil {
		return 0, 0, fmt.Errorf("list workspace calendars: %w", err)
	}
	calendarIDByName := make(map[string]int, len(existingCalendars))
	for _, calendar := range existingCalendars {
		calendarIDByName[lowerStr(calendar.Name)] = calendar.ID
	}
	calendarIDByRef := make(map[string]int, len(slas.Calendars))
	for i := range slas.Calendars {
		cal := &slas.Calendars[i]
		if id, ok := calendarIDByName[lowerStr(cal.Name)]; ok {
			calendarIDByRef[cal.Ref] = id
			continue
		}
		created, err := s.slaCalendars.CreateWorkspace(ctx, workspaceID, SLACalendarInput{
			Name:            cal.Name,
			Description:     cal.Description,
			Timezone:        cal.Timezone,
			WeeklyIntervals: cal.WeeklyIntervals,
			Holidays:        cal.Holidays,
			IsDefault:       cal.IsDefault,
		})
		if err != nil {
			return calendarsImported, metricsImported, fmt.Errorf("create calendar %q: %w", cal.Name, err)
		}
		calendarIDByRef[cal.Ref] = created.ID
		calendarIDByName[lowerStr(cal.Name)] = created.ID
		calendarsImported++
	}

	existingMetrics, err := s.slaMetrics.List(ctx, workspaceID)
	if err != nil {
		return calendarsImported, metricsImported, fmt.Errorf("list workspace SLA metrics: %w", err)
	}
	metricByName := make(map[string]bool, len(existingMetrics))
	for _, metric := range existingMetrics {
		metricByName[lowerStr(metric.Name)] = true
	}
	for i := range slas.Metrics {
		metric := &slas.Metrics[i]
		if metricByName[lowerStr(metric.Name)] {
			continue
		}
		input, err := s.buildSLAMetricInput(ctx, metric, calendarIDByRef)
		if err != nil {
			return calendarsImported, metricsImported, err
		}
		if _, err := s.slaMetrics.Create(ctx, workspaceID, input); err != nil {
			return calendarsImported, metricsImported, fmt.Errorf("create SLA metric %q: %w", metric.Name, err)
		}
		metricByName[lowerStr(metric.Name)] = true
		metricsImported++
	}
	return calendarsImported, metricsImported, nil
}

// buildSLAMetricInput resolves the bundle's name-based metric into the
// transport-neutral service input. Calendar refs must resolve to a bundled
// calendar; priorities resolve against the instance registry.
func (s *WorkspaceBundleImportService) buildSLAMetricInput(ctx context.Context, metric *WorkspaceBundleSLAMetric, calendarIDByRef map[string]int) (SLAMetricInput, error) {
	input := SLAMetricInput{
		Name:          metric.Name,
		DisplayFormat: metric.DisplayFormat,
		IsActive:      metric.IsActive,
	}
	for i := range metric.Conditions {
		condition := &metric.Conditions[i]
		config, err := s.conditionConfig(ctx, condition)
		if err != nil {
			return SLAMetricInput{}, fmt.Errorf("SLA metric %q condition %d: %w", metric.Name, i, err)
		}
		input.Conditions = append(input.Conditions, models.SLACondition{
			Phase:         condition.Phase,
			Position:      i,
			ConditionType: condition.ConditionType,
			Config:        config,
		})
	}
	for i := range metric.Goals {
		goal := &metric.Goals[i]
		baseQL := strings.TrimSpace(goal.QLQuery)
		if baseQL == "" {
			baseQL = "1 = 1"
		}
		newGoal := func(query string) models.SLAGoal {
			return models.SLAGoal{QLQuery: query, ImportStatus: "native"}
		}
		resolveTarget := func(target *WorkspaceBundleSLATarget) (models.SLAGoalTarget, error) {
			calendarID, ok := calendarIDByRef[target.CalendarRef]
			if !ok {
				return models.SLAGoalTarget{}, fmt.Errorf("calendar_ref %q is not a bundled calendar", target.CalendarRef)
			}
			return models.SLAGoalTarget{TargetMs: target.TargetMs, CalendarID: calendarID}, nil
		}

		// One goal per target: priority targets get their own goal with the
		// priority appended to the base query, and the fallback keeps the base
		// query so first-match-wins preserves priority precedence.
		var fallback *WorkspaceBundleSLATarget
		for j := range goal.Targets {
			target := &goal.Targets[j]
			priorityName := strings.TrimSpace(target.PriorityName)
			if target.IsFallback || priorityName == "" {
				if fallback == nil {
					fallback = &goal.Targets[j]
				}
				continue
			}
			if _, err := s.priorityIDByName(ctx, priorityName); err != nil {
				return SLAMetricInput{}, fmt.Errorf("SLA metric %q goal %d target %d: %w", metric.Name, i, j, err)
			}
			resolvedTarget, err := resolveTarget(target)
			if err != nil {
				return SLAMetricInput{}, fmt.Errorf("SLA metric %q goal %d target %d: %w", metric.Name, i, j, err)
			}
			resolved := newGoal(combineGoalQuery(baseQL, priorityName))
			resolved.Targets = []models.SLAGoalTarget{resolvedTarget}
			input.Goals = append(input.Goals, resolved)
		}
		if fallback != nil {
			resolvedTarget, err := resolveTarget(fallback)
			if err != nil {
				return SLAMetricInput{}, fmt.Errorf("SLA metric %q goal %d fallback: %w", metric.Name, i, err)
			}
			resolved := newGoal(baseQL)
			resolved.Targets = []models.SLAGoalTarget{resolvedTarget}
			input.Goals = append(input.Goals, resolved)
		}
	}
	for i := range input.Goals {
		input.Goals[i].Position = i
	}
	return input, nil
}

// conditionConfig encodes the condition's name lists into the engine's
// status_ids/category_ids shape. Conditions with no target get an empty object.
func (s *WorkspaceBundleImportService) conditionConfig(ctx context.Context, condition *WorkspaceBundleSLACondition) (json.RawMessage, error) {
	if len(condition.StatusNames) == 0 && len(condition.CategoryNames) == 0 {
		return json.RawMessage(`{}`), nil
	}
	config := struct {
		StatusIDs   []int `json:"status_ids,omitempty"`
		CategoryIDs []int `json:"category_ids,omitempty"`
	}{}
	for _, name := range condition.StatusNames {
		id, err := s.statusIDByName(ctx, name)
		if err != nil {
			return nil, err
		}
		config.StatusIDs = append(config.StatusIDs, id)
	}
	for _, name := range condition.CategoryNames {
		id, err := s.categoryIDByName(ctx, name)
		if err != nil {
			return nil, err
		}
		config.CategoryIDs = append(config.CategoryIDs, id)
	}
	return json.Marshal(config)
}

func (s *WorkspaceBundleImportService) statusIDByName(ctx context.Context, name string) (int, error) {
	return s.lookupIDByName(ctx, "statuses", "status", name)
}

func (s *WorkspaceBundleImportService) categoryIDByName(ctx context.Context, name string) (int, error) {
	return s.lookupIDByName(ctx, "status_categories", "status category", name)
}

func (s *WorkspaceBundleImportService) priorityIDByName(ctx context.Context, name string) (int, error) {
	return s.lookupIDByName(ctx, "priorities", "priority", name)
}

func (s *WorkspaceBundleImportService) lookupIDByName(ctx context.Context, table, kind, name string) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, "SELECT id FROM "+table+" WHERE LOWER(name) = LOWER(?)", strings.TrimSpace(name)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%s %q was not found", kind, name)
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}

// validateReferences refuses bundles referencing identities the target
// cannot resolve, using the same structured-report contract as the
// config-set importer.
func (s *WorkspaceBundleImportService) validateReferences(ctx context.Context, workspaceID int, bundle *WorkspaceBundle) error {
	providedItemTypes := map[string]bool{}
	providedCustomFields := map[string]bool{}
	providedLinkTypes := map[string]bool{}
	providedPriorities := map[string]bool{}
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
		for _, p := range tpl.Payload.Priorities {
			providedPriorities[lowerStr(p.Name)] = true
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

	seenPriorities := map[string]bool{}
	for _, item := range bundle.Payload.Items {
		name := strings.TrimSpace(item.PriorityName)
		key := lowerStr(name)
		if key == "" || seenPriorities[key] {
			continue
		}
		seenPriorities[key] = true
		if providedPriorities[key] {
			continue
		}
		if id, _ := s.lookupPriorityID(ctx, name); id == 0 {
			missing = append(missing, UnresolvedRef{
				Kind: UnresolvedKindPriority, Name: name,
				Path: fmt.Sprintf("items/%s", item.Ref),
			})
		}
	}

	seenEmails := map[string]bool{}
	for _, item := range bundle.Payload.Items {
		email := item.AssigneeEmail
		if email == "" || seenEmails[strings.ToLower(email)] {
			continue
		}
		seenEmails[strings.ToLower(email)] = true
		if id, _ := s.lookupWorkspaceUserID(ctx, workspaceID, email); id == 0 {
			// Report by path only: echoing the email would let an editor probe
			// which addresses belong to accounts.
			missing = append(missing, UnresolvedRef{
				Kind: UnresolvedKindUser,
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

func (s *WorkspaceBundleImportService) lookupPriorityID(ctx context.Context, name string) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM priorities WHERE LOWER(name) = LOWER(?)`, name).Scan(&id)
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

// lookupWorkspaceUserID resolves an assignee email to a user who can view the
// target workspace. Restricting the lookup to workspace members stops an editor
// from assigning an arbitrary instance user as an imported item's assignee.
func (s *WorkspaceBundleImportService) lookupWorkspaceUserID(ctx context.Context, workspaceID int, email string) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE LOWER(email) = LOWER(?)`, email).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if s.permissions == nil {
		return 0, nil
	}
	actionable, err := NewWorkspaceUserResolver(s.db, s.permissions).CanActInWorkspace(id, workspaceID)
	if err != nil {
		return 0, err
	}
	if !actionable {
		return 0, nil
	}
	return id, nil
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
			priorityID, err := s.lookupPriorityID(ctx, item.PriorityName)
			if err != nil {
				imp.failedItems[item.Ref] = true
				imp.outcome("item", item.Ref, item.Title, "failed", "priority lookup failed: "+err.Error())
				continue
			}
			if priorityID == 0 {
				imp.failedItems[item.Ref] = true
				imp.outcome("item", item.Ref, item.Title, "failed", fmt.Sprintf("priority %q is not available", item.PriorityName))
				continue
			}
			input.PriorityID = &priorityID
		}
		if item.AssigneeEmail != "" {
			if userID, err := s.lookupWorkspaceUserID(ctx, imp.workspaceID, item.AssigneeEmail); err == nil && userID != 0 {
				input.AssigneeID = &userID
			}
		}
		if item.ParentRef != "" {
			if imp.failedItems[item.ParentRef] {
				imp.failedItems[item.Ref] = true
				imp.outcome("item", item.Ref, item.Title, "failed", "parent item failed to import")
				continue
			}
			id, ok := imp.itemIDs[item.ParentRef]
			if !ok {
				// Never silently flatten a subtree: a child whose parent has not
				// been imported yet (out-of-order bundle) is a hard failure.
				imp.failedItems[item.Ref] = true
				imp.outcome("item", item.Ref, item.Title, "failed", fmt.Sprintf("parent ref %q was not imported before this item", item.ParentRef))
				continue
			}
			input.ParentID = &id
		}
		if len(item.FieldValues) > 0 {
			values := make(map[string]any, len(item.FieldValues))
			var fieldProblem string
			for fieldName, value := range item.FieldValues {
				var def customFieldDef
				if err := s.db.QueryRowContext(ctx,
					`SELECT id, name, field_type, COALESCE(options, '') FROM custom_field_definitions WHERE LOWER(name) = LOWER(?)`,
					fieldName,
				).Scan(&def.ID, &def.Name, &def.FieldType, &def.Options); err != nil {
					fieldProblem = fmt.Sprintf("custom field %q is not available", fieldName)
					break
				}
				decoded, err := decodeChoiceFieldValue(def, value)
				if err != nil {
					fieldProblem = err.Error()
					break
				}
				values[strconv.Itoa(def.ID)] = decoded
			}
			if fieldProblem != "" {
				imp.failedItems[item.Ref] = true
				imp.outcome("item", item.Ref, item.Title, "failed", fieldProblem)
				continue
			}
			input.CustomFieldValues = values
		}
		var droppedLabels []string
		if len(item.Labels) > 0 {
			ids := make([]int, 0, len(item.Labels))
			for _, name := range item.Labels {
				labelID, err := s.labels.FindIDByName(name)
				if err != nil || labelID == 0 {
					droppedLabels = append(droppedLabels, name)
					continue
				}
				ids = append(ids, labelID)
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
		detail := ""
		if len(droppedLabels) > 0 {
			detail = "labels not available: " + strings.Join(droppedLabels, ", ")
		}
		imp.outcome("item", item.Ref, item.Title, "imported", detail)
	}
	return nil
}

// decodeChoiceFieldValue turns a select/multiselect option label (or list of
// labels) back into the target instance's option id(s). Non-choice values pass
// through unchanged; a label absent from the field's option set is an error so
// the item is reported instead of storing a broken value.
func decodeChoiceFieldValue(def customFieldDef, value any) (any, error) {
	switch def.FieldType {
	case "select":
		label, ok := value.(string)
		if !ok {
			return value, nil
		}
		id, ok := selectOptionIDByLabel(def.Options, label)
		if !ok {
			return nil, fmt.Errorf("custom field %q: option %q is not in the field's option set", def.Name, label)
		}
		return id, nil
	case "multiselect":
		items, ok := value.([]any)
		if !ok {
			return value, nil
		}
		ids := make([]int, 0, len(items))
		for _, item := range items {
			label, ok := item.(string)
			if !ok {
				return value, nil
			}
			id, ok := selectOptionIDByLabel(def.Options, label)
			if !ok {
				return nil, fmt.Errorf("custom field %q: option %q is not in the field's option set", def.Name, label)
			}
			ids = append(ids, id)
		}
		return ids, nil
	default:
		return value, nil
	}
}

func selectOptionIDByLabel(optionsJSON, label string) (int, bool) {
	opts, err := models.ParseSelectOptions(optionsJSON)
	if err != nil {
		return 0, false
	}
	for _, item := range opts.Items {
		if item.Label == label {
			return item.ID, true
		}
	}
	return 0, false
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
