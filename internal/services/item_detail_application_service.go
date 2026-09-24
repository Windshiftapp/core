package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

type ItemDetailScreenReader interface {
	LoadScreen(int) (*models.Screen, error)
}

type ItemDetailRequestFieldReader interface {
	ListVisibleFields(context.Context, int, int) ([]models.RequestTypeField, error)
}

type ItemDetailManualActionReader interface {
	ListManualActions(int, int) ([]*models.Action, error)
}

type ItemDetailScreenContext struct {
	Edit *models.Screen `json:"edit"`
	View *models.Screen `json:"view"`
}

type ItemDetailSectionError struct {
	Section string `json:"section"`
	Code    string `json:"code"`
}

type ItemDetailSummary struct {
	Item                   *models.Item                  `json:"item"`
	Links                  EntityLinks                   `json:"links"`
	LinkTypes              []models.LinkType             `json:"link_types"`
	RequestTypeFields      []models.RequestTypeField     `json:"request_type_fields"`
	Transitions            ItemTransitionSummary         `json:"transitions"`
	Watching               bool                          `json:"watching"`
	MergedIntoItemID       *int                          `json:"merged_into_item_id,omitempty"`
	Children               []models.Item                 `json:"children"`
	StoryPointsRollup      *repository.StoryPointsRollup `json:"story_points_rollup,omitempty"`
	Ancestors              []models.Item                 `json:"ancestors"`
	CurrentItemType        *ItemTypeResult               `json:"current_item_type"`
	CurrentHierarchyLevel  *models.HierarchyLevel        `json:"current_hierarchy_level"`
	AvailableSubIssueTypes []ItemTypeResult              `json:"available_sub_issue_types"`
	Priorities             []models.PriorityDisplay      `json:"priorities"`
	ScreenContext          ItemDetailScreenContext       `json:"screen_context"`
	ManualActions          []*models.Action              `json:"manual_actions"`
	PersonalTaskCount      int                           `json:"personal_task_count"`
	SCMAvailable           bool                          `json:"scm_available"`
	HasAgentRuns           bool                          `json:"has_agent_runs"`
	SectionErrors          []ItemDetailSectionError      `json:"section_errors"`
}

type ItemDetailApplicationService struct {
	db            database.Database
	items         *ItemApplicationService
	links         *ItemLinkService
	permissions   *PermissionService
	screens       ItemDetailScreenReader
	requestFields ItemDetailRequestFieldReader
	manualActions ItemDetailManualActionReader
	rollups       ItemStoryPointRollupReader
}

// ItemStoryPointRollupReader computes the recursive story-point rollup for one
// item's descendants. Detail loads only — never list projections.
type ItemStoryPointRollupReader interface {
	SumDescendantStoryPoints(ctx context.Context, parentID int) (repository.StoryPointsRollup, error)
}

func NewItemDetailApplicationService(db database.Database, items *ItemApplicationService, links *ItemLinkService, permissions *PermissionService) *ItemDetailApplicationService {
	return &ItemDetailApplicationService{db: db, items: items, links: links, permissions: permissions}
}

func (s *ItemDetailApplicationService) WithContextReaders(screens ItemDetailScreenReader, requestFields ItemDetailRequestFieldReader, manualActions ItemDetailManualActionReader) *ItemDetailApplicationService {
	s.screens, s.requestFields, s.manualActions = screens, requestFields, manualActions
	return s
}

// WithStoryPointRollups wires the descendant story-point aggregate (GH #256).
func (s *ItemDetailApplicationService) WithStoryPointRollups(reader ItemStoryPointRollupReader) *ItemDetailApplicationService {
	s.rollups = reader
	return s
}

func (s *ItemDetailApplicationService) Get(ctx context.Context, userID, itemID int, surface string, excludePersonal bool) (ItemDetailSummary, error) {
	item, err := s.items.GetWithOptions(ctx, userID, itemID, ItemReadOptions{TrackView: true, ExcludePersonal: excludePersonal})
	if err != nil {
		return ItemDetailSummary{}, err
	}
	return s.load(ctx, userID, item, surface), nil
}

func (s *ItemDetailApplicationService) GetByKey(ctx context.Context, userID int, workspaceKey string, itemNumber int, surface string, excludePersonal bool) (ItemDetailSummary, error) {
	item, err := s.items.GetByKeyWithOptions(ctx, userID, workspaceKey, itemNumber, ItemReadOptions{TrackView: true, ExcludePersonal: excludePersonal})
	if err != nil {
		return ItemDetailSummary{}, err
	}
	return s.load(ctx, userID, item, surface), nil
}

func (s *ItemDetailApplicationService) load(ctx context.Context, userID int, item *models.Item, surface string) ItemDetailSummary {
	result := ItemDetailSummary{
		Item: item, Links: EntityLinks{Outgoing: []models.ItemLink{}, Incoming: []models.ItemLink{}},
		LinkTypes: []models.LinkType{}, RequestTypeFields: []models.RequestTypeField{},
		Transitions: ItemTransitionSummary{AvailableTransitions: []ItemTransitionOption{}},
		Children:    []models.Item{}, Ancestors: []models.Item{}, AvailableSubIssueTypes: []ItemTypeResult{},
		Priorities: []models.PriorityDisplay{}, ManualActions: []*models.Action{},
		SectionErrors: []ItemDetailSectionError{},
	}
	canView, err := s.permissions.HasWorkspacePermission(userID, item.WorkspaceID, models.PermissionItemView)
	if err != nil || !canView {
		return result
	}
	mobile := strings.EqualFold(surface, "mobile")
	var wait sync.WaitGroup
	var errorMu sync.Mutex
	run := func(section string, load func() error) {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := load(); err != nil {
				errorMu.Lock()
				result.SectionErrors = append(result.SectionErrors, ItemDetailSectionError{Section: section, Code: "unavailable"})
				errorMu.Unlock()
				slog.Warn("item detail section unavailable", "section", section, "item_id", item.ID, "error", err)
			}
		}()
	}
	run("transitions", func() error {
		value, err := s.items.AvailableTransitions(ctx, userID, item.ID)
		if err == nil {
			result.Transitions = value
		}
		return err
	})
	run("watch", func() error {
		value, err := s.items.WatchStatus(ctx, userID, item.ID)
		if err == nil {
			result.Watching = value.Watching
		}
		return err
	})
	run("merge-redirect", func() error {
		var mergedInto *int
		if err := s.db.QueryRowContext(ctx,
			`SELECT merged_into_item_id FROM items WHERE id = ?`, item.ID,
		).Scan(&mergedInto); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		result.MergedIntoItemID = mergedInto
		return nil
	})
	run("children", func() error {
		value, err := s.items.Children(ctx, userID, item.ID)
		if err == nil {
			result.Children = value
		}
		return err
	})
	if s.rollups != nil {
		// Detail loads only — never a list projection. The CTE is a no-op for
		// leaf items (one indexed probe), so it runs unconditionally to avoid
		// racing the parallel children loader.
		run("story_points_rollup", func() error {
			rollup, err := s.rollups.SumDescendantStoryPoints(ctx, item.ID)
			if err == nil && rollup.Contributors > 0 {
				result.StoryPointsRollup = &rollup
			}
			return err
		})
	}
	if item.ParentID != nil {
		run("ancestors", func() error {
			value, err := s.items.Ancestors(ctx, userID, item.ID)
			if err == nil {
				result.Ancestors = value
			}
			return err
		})
	}
	run("type context", func() error {
		types, levels, err := s.loadTypeContext()
		if err != nil {
			return err
		}
		result.applyTypeContext(item, types, levels)
		return nil
	})
	if mobile {
		run("personal tasks", func() error {
			items, err := s.items.PersonalTasks(ctx, userID, item.ID)
			if err == nil {
				result.PersonalTaskCount = len(items)
			}
			return err
		})
		run("panel availability", func() error {
			var err error
			result.SCMAvailable, result.HasAgentRuns, err = repository.NewItemRepository(s.db).GetDetailPanelAvailability(item.WorkspaceID, item.ID)
			return err
		})
	} else {
		run("links", func() error {
			outgoing, incoming, err := s.links.ListLinksForEntityWithChecks(userID, "item", item.ID)
			if err == nil {
				result.Links = EntityLinks{Outgoing: nonNilItemLinks(outgoing), Incoming: nonNilItemLinks(incoming)}
			}
			return err
		})
		run("link types", func() error {
			value, err := repository.NewLinkTypeRepository(s.db).List(false)
			if err == nil && value != nil {
				result.LinkTypes = value
			}
			return err
		})
		if item.RequestTypeID != nil && s.requestFields != nil {
			run("request fields", func() error {
				value, err := s.requestFields.ListVisibleFields(ctx, userID, *item.RequestTypeID)
				if err == nil {
					result.RequestTypeFields = value
				}
				return err
			})
		}
		if s.screens != nil {
			run("configuration", func() error {
				priorities, screens, err := s.loadConfiguration(item)
				if err == nil {
					result.Priorities, result.ScreenContext = priorities, screens
				}
				return err
			})
		}
		if s.manualActions != nil {
			run("manual actions", func() error {
				value, err := s.manualActions.ListManualActions(userID, item.WorkspaceID)
				if err == nil {
					result.ManualActions = value
				}
				return err
			})
		}
	}
	wait.Wait()
	sort.Slice(result.SectionErrors, func(i, j int) bool { return result.SectionErrors[i].Section < result.SectionErrors[j].Section })
	return result
}

func (r *ItemDetailSummary) applyTypeContext(item *models.Item, types []ItemTypeResult, levels []models.HierarchyLevel) {
	if item.ItemTypeID == nil {
		return
	}
	for i := range types {
		if types[i].ID == *item.ItemTypeID {
			selected := types[i]
			r.CurrentItemType = &selected
			break
		}
	}
	if r.CurrentItemType == nil || r.CurrentItemType.HierarchyLevel == models.HierarchyLevelGenericSubtask {
		return
	}
	for i := range levels {
		if levels[i].Level == r.CurrentItemType.HierarchyLevel {
			selected := levels[i]
			r.CurrentHierarchyLevel = &selected
			break
		}
	}
	if r.CurrentHierarchyLevel == nil {
		return
	}
	next := r.CurrentHierarchyLevel.Level + 1
	for _, itemType := range types {
		if itemType.HierarchyLevel == next || itemType.HierarchyLevel == models.HierarchyLevelGenericSubtask {
			r.AvailableSubIssueTypes = append(r.AvailableSubIssueTypes, itemType)
		}
	}
}

func (s *ItemDetailApplicationService) loadTypeContext() ([]ItemTypeResult, []models.HierarchyLevel, error) {
	types, err := NewConfigReadService(s.db).ListItemTypes()
	if err != nil {
		return nil, nil, err
	}
	entities, err := NewEnumService(s.db, NewHierarchyLevelConfig()).GetAll()
	if err != nil {
		return nil, nil, err
	}
	levels := make([]models.HierarchyLevel, 0, len(entities))
	for _, entity := range entities {
		level, ok := entity.(*models.HierarchyLevel)
		if !ok {
			return nil, nil, fmt.Errorf("unexpected hierarchy level type %T", entity)
		}
		levels = append(levels, *level)
	}
	return types, levels, nil
}

func (s *ItemDetailApplicationService) loadConfiguration(item *models.Item) ([]models.PriorityDisplay, ItemDetailScreenContext, error) {
	repo := repository.NewConfigurationSetRepository(s.db)
	resolved, err := repo.ResolveEffective(context.Background(), item.WorkspaceID, item.ItemTypeID)
	if err != nil {
		return nil, ItemDetailScreenContext{}, err
	}
	editID := effectiveScreenIDOrFallback(resolved, false)
	viewID := effectiveScreenIDOrFallback(resolved, true)
	edit, err := s.screens.LoadScreen(editID)
	if err != nil {
		return nil, ItemDetailScreenContext{}, err
	}
	screens := ItemDetailScreenContext{Edit: edit}
	if viewID != editID {
		view, err := s.screens.LoadScreen(viewID)
		if err != nil {
			return nil, ItemDetailScreenContext{}, err
		}
		screens.View = view
	}
	priorities := []models.PriorityDisplay{}
	if resolved != nil && len(resolved.Priorities) > 0 {
		priorities = resolved.Priorities
	}
	return priorities, screens, nil
}

// effectiveScreenIDOrFallback maps the canonical resolution onto the detail
// summary's screen context. A nil resolution (personal workspace or nothing
// configured) degrades to the absolute fallback screen.
func effectiveScreenIDOrFallback(resolved *repository.EffectiveConfig, view bool) int {
	if resolved == nil {
		return repository.FallbackScreenID
	}
	id := resolved.ViewScreenID
	if !view {
		id = resolved.EditScreenID
	}
	if id == nil {
		return repository.FallbackScreenID
	}
	return *id
}

func nonNilItemLinks(value []models.ItemLink) []models.ItemLink {
	if value == nil {
		return []models.ItemLink{}
	}
	return value
}
