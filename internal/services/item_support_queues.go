package services

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// QueueView is one resolved support queue: a virtual built-in preset or a
// persisted custom queue. ID is 0 for a built-in still using the catalog
// definition. Built-in queues carry Key; custom queues are addressed by ID.
type QueueView struct {
	ID          int     `json:"id"`
	Key         string  `json:"key,omitempty"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	QL          string  `json:"ql"`
	FilterState *string `json:"filter_state,omitempty"`
	Builtin     bool    `json:"builtin"`
	Position    int     `json:"position"`
	Count       int64   `json:"count"`
}

// QueueScope identifies where queues are stored: a collection or the
// workspace default view. CollectionID is nil for the workspace default.
type QueueScope struct {
	WorkspaceID  int
	CollectionID *int
}

// QueueInput is the mutable payload for creating or editing a custom queue.
type QueueInput struct {
	Name        string
	QL          string
	FilterState *string
}

// QueueValidationError reports a client-correctable queue write problem.
type QueueValidationError struct {
	Message string
}

func (e *QueueValidationError) Error() string { return e.Message }

func queueValidation(message string) error { return &QueueValidationError{Message: message} }

// builtinQueuePreset is one virtual built-in queue. The CQL runs through the
// shared item-list pipeline, so queue counts and rows enforce workspace
// visibility identically to the items API. Keys are a stable product contract.
//
// Waiting means the ticket's SLA cycle is paused — the WI-584 pause rules fire
// exactly when support waits on the customer. Workspaces without SLA
// configuration therefore see an empty waiting queue rather than a wrong one.
type builtinQueuePreset struct {
	Key         string
	Name        string
	Description string
	QL          string
}

var builtinQueuePresets = []builtinQueuePreset{
	{
		Key:         "unassigned",
		Name:        "Unassigned",
		Description: "Open tickets with no team and no assignee — the routing fallback",
		QL:          `assignee IS NULL AND team IS NULL AND statusCompleted = false`,
	},
	{
		Key:         "team-owned",
		Name:        "Team-owned",
		Description: "Open tickets a team owns that no agent has picked up",
		QL:          `team IS NOT NULL AND assignee IS NULL AND statusCompleted = false`,
	},
	{
		Key:         "assigned-to-me",
		Name:        "Assigned to me",
		Description: "Open tickets assigned to the current agent",
		QL:          `assignee = currentUser() AND statusCompleted = false`,
	},
	{
		Key:         "waiting",
		Name:        "Waiting",
		Description: "Open tickets whose SLA is paused — support waits on the customer",
		QL:          `slaPaused = true AND statusCompleted = false`,
	},
	{
		Key:         "overdue",
		Name:        "Overdue",
		Description: "Open tickets past their due date",
		QL:          `dueDate < startofday() AND statusCompleted = false`,
	},
	{
		Key:         "recently-updated",
		Name:        "Recently updated",
		Description: "Open tickets with activity in the last 24 hours",
		QL:          `updatedAt > -1d AND statusCompleted = false`,
	},
	{
		Key:         "sla-at-risk",
		Name:        "SLA at risk",
		Description: "Open tickets whose running SLA deadline is within 2 days or already passed",
		QL:          `slaRunning = true AND slaDeadline <= 2d`,
	},
}

// ResolveWorkspaceIDByKey maps a workspace key to its numeric id. It exists so
// the queue routes can keep the workspace-key addressing the queue view has
// always used.
func (s *ItemApplicationService) ResolveWorkspaceIDByKey(key string) (int, error) {
	id, err := repository.NewWorkspaceRepository(s.db).FindIDByKey(strings.TrimSpace(key))
	if err != nil {
		return 0, err
	}
	return id, nil
}

// ListQueues resolves the visible queue catalog for a scope: built-in presets
// that are not dismissed, then custom queues by position, each with a live
// count. collectionID nil selects the workspace default view.
func (s *ItemApplicationService) ListQueues(ctx context.Context, userID, workspaceID int, collectionID *int) ([]QueueView, error) {
	scope, err := s.queueScope(workspaceID, collectionID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeQueueRead(userID, scope); err != nil {
		return nil, err
	}
	return s.resolveQueues(ctx, userID, scope)
}

// CreateQueue adds a custom queue to a scope and returns the new queue.
func (s *ItemApplicationService) CreateQueue(ctx context.Context, actor AuditActor, workspaceID int, collectionID *int, input QueueInput) (*QueueView, error) {
	scope, err := s.queueScope(workspaceID, collectionID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeQueueWrite(actor.UserID, scope); err != nil {
		return nil, err
	}
	name, ql, filterState, err := s.validateQueueInput(ctx, actor.UserID, input)
	if err != nil {
		return nil, err
	}
	repo := repository.NewQueueRepository(s.db)
	position, err := repo.NextPosition(scope.WorkspaceID, scope.CollectionID)
	if err != nil {
		return nil, err
	}
	createdBy := actor.UserID
	created, err := repo.Create(&models.Queue{
		WorkspaceID:  scope.WorkspaceID,
		CollectionID: scope.CollectionID,
		Name:         name,
		QLQuery:      ql,
		FilterState:  filterState,
		Position:     position,
		CreatedBy:    &createdBy,
	})
	if err != nil {
		return nil, err
	}
	return queueViewFromModel(created), nil
}

// UpdateQueue rewrites a custom queue's mutable fields.
func (s *ItemApplicationService) UpdateQueue(ctx context.Context, actor AuditActor, queueID int, input QueueInput) (*QueueView, error) {
	repo := repository.NewQueueRepository(s.db)
	queue, err := repo.GetByID(queueID)
	if err != nil {
		return nil, err
	}
	if queue.BuiltinKey != nil {
		return nil, queueValidation("built-in queues cannot be edited")
	}
	if err := s.authorizeQueueWrite(actor.UserID, QueueScope{WorkspaceID: queue.WorkspaceID, CollectionID: queue.CollectionID}); err != nil {
		return nil, err
	}
	name, ql, filterState, err := s.validateQueueInput(ctx, actor.UserID, input)
	if err != nil {
		return nil, err
	}
	queue.Name = name
	queue.QLQuery = ql
	queue.FilterState = filterState
	if err := repo.Update(queue); err != nil {
		return nil, err
	}
	return queueViewFromModel(queue), nil
}

// DeleteQueue removes a custom queue.
func (s *ItemApplicationService) DeleteQueue(actor AuditActor, queueID int) error {
	repo := repository.NewQueueRepository(s.db)
	queue, err := repo.GetByID(queueID)
	if err != nil {
		return err
	}
	if queue.BuiltinKey != nil {
		return queueValidation("built-in queues cannot be deleted")
	}
	if err := s.authorizeQueueWrite(actor.UserID, QueueScope{WorkspaceID: queue.WorkspaceID, CollectionID: queue.CollectionID}); err != nil {
		return err
	}
	return repo.Delete(queueID)
}

// SetBuiltinQueueHidden hides or restores a built-in preset for a scope. The
// preset itself is virtual; only the dismissal is persisted.
func (s *ItemApplicationService) SetBuiltinQueueHidden(actor AuditActor, workspaceID int, collectionID *int, builtinKey string, hidden bool) error {
	scope, err := s.queueScope(workspaceID, collectionID)
	if err != nil {
		return err
	}
	if err := s.authorizeQueueWrite(actor.UserID, scope); err != nil {
		return err
	}
	preset, ok := findBuiltinQueuePreset(builtinKey)
	if !ok {
		return repository.ErrNotFound
	}
	repo := repository.NewQueueRepository(s.db)
	createdBy := actor.UserID
	err = repo.SetBuiltinHidden(&models.Queue{
		WorkspaceID:  scope.WorkspaceID,
		CollectionID: scope.CollectionID,
		Name:         preset.Name,
		QLQuery:      preset.QL,
		BuiltinKey:   &preset.Key,
		Position:     builtinQueuePosition(preset.Key),
		CreatedBy:    &createdBy,
	}, hidden)
	if errors.Is(err, repository.ErrDuplicateEntry) {
		return nil
	}
	return err
}

// ReorderQueues assigns positions to custom queue ids within the scope.
func (s *ItemApplicationService) ReorderQueues(actor AuditActor, workspaceID int, collectionID *int, ids []int) error {
	scope, err := s.queueScope(workspaceID, collectionID)
	if err != nil {
		return err
	}
	if err := s.authorizeQueueWrite(actor.UserID, scope); err != nil {
		return err
	}
	if len(ids) == 0 {
		return queueValidation("ids is required")
	}
	return repository.NewQueueRepository(s.db).Reorder(scope.WorkspaceID, scope.CollectionID, ids)
}

func (s *ItemApplicationService) resolveQueues(ctx context.Context, userID int, scope QueueScope) ([]QueueView, error) {
	rows, err := repository.NewQueueRepository(s.db).ListByScope(scope.WorkspaceID, scope.CollectionID)
	if err != nil {
		return nil, err
	}

	hidden := make(map[string]bool)
	overrides := make(map[string]models.Queue)
	custom := make([]models.Queue, 0, len(rows))
	for _, row := range rows {
		if row.BuiltinKey != nil {
			if row.IsHidden {
				hidden[*row.BuiltinKey] = true
			} else {
				overrides[*row.BuiltinKey] = row
			}
			continue
		}
		if !row.IsHidden {
			custom = append(custom, row)
		}
	}

	views := make([]QueueView, 0, len(builtinQueuePresets)+len(custom))
	for i, preset := range builtinQueuePresets {
		if hidden[preset.Key] {
			continue
		}
		view := QueueView{
			Key:         preset.Key,
			Name:        preset.Name,
			Description: preset.Description,
			QL:          preset.QL,
			Builtin:     true,
			Position:    i,
		}
		if override, ok := overrides[preset.Key]; ok {
			view.ID = override.ID
			view.Name = override.Name
			view.QL = override.QLQuery
			view.FilterState = override.FilterState
		}
		views = append(views, view)
	}
	for _, row := range custom {
		views = append(views, QueueView{
			ID:          row.ID,
			Name:        row.Name,
			QL:          row.QLQuery,
			FilterState: row.FilterState,
			Position:    row.Position,
		})
	}

	if err := s.fillQueueCounts(ctx, userID, scope, views); err != nil {
		return nil, err
	}
	return views, nil
}

// fillQueueCounts evaluates every visible queue in one batched COUNT
// statement, so adding custom queues does not add a round trip per queue. A
// scope with no single workspace (a collection that spans workspaces) counts
// across every workspace the caller can access.
func (s *ItemApplicationService) fillQueueCounts(ctx context.Context, userID int, scope QueueScope, views []QueueView) error {
	workspaceIDs := []int{scope.WorkspaceID}
	if scope.WorkspaceID <= 0 {
		ids, err := s.perm.AccessibleWorkspaceIDs(userID)
		if err != nil {
			return err
		}
		workspaceIDs = ids
	}
	if len(workspaceIDs) == 0 {
		for i := range views {
			views[i].Count = 0
		}
		return nil
	}

	queries := make([]repository.QueueCountQuery, 0, len(views))
	for i := range views {
		resolved, err := s.crud.resolveItemListQLContext(ctx, views[i].QL, 0, "", userID)
		if err != nil {
			return err
		}
		queries = append(queries, repository.QueueCountQuery{
			Key:     strconv.Itoa(i),
			Filters: repository.ItemFilters{QLQuery: resolved.sql, QLArgs: resolved.args},
		})
	}
	counts, err := s.crud.repo.CountQLQueries(ctx, workspaceIDs, queries)
	if err != nil {
		return err
	}
	for i := range views {
		views[i].Count = counts[strconv.Itoa(i)]
	}
	return nil
}

// queueScope resolves a workspace id plus optional collection id into a
// storage scope. A collection scope is identified by the collection alone;
// its workspace is carried when it has one, and a collection that spans
// workspaces is a valid scope.
func (s *ItemApplicationService) queueScope(workspaceID int, collectionID *int) (QueueScope, error) {
	if collectionID == nil {
		if workspaceID <= 0 {
			return QueueScope{}, queueValidation("workspace is required")
		}
		return QueueScope{WorkspaceID: workspaceID}, nil
	}
	collection, err := repository.NewCollectionRepository(s.db).GetByID(*collectionID)
	if err != nil {
		return QueueScope{}, err
	}
	scope := QueueScope{CollectionID: collectionID}
	if collection.WorkspaceID != nil {
		scope.WorkspaceID = *collection.WorkspaceID
	}
	return scope, nil
}

// authorizeQueueRead mirrors the collection read rule: a private collection is
// creator-only, and a collection bound to a workspace additionally requires
// item.view there. A collection that spans workspaces has no single workspace
// to check.
func (s *ItemApplicationService) authorizeQueueRead(userID int, scope QueueScope) error {
	if scope.CollectionID == nil {
		allowed, err := s.perm.HasWorkspacePermission(userID, scope.WorkspaceID, models.PermissionItemView)
		if err != nil {
			return err
		}
		if !allowed {
			return repository.ErrNotFound
		}
		return nil
	}
	collection, err := repository.NewCollectionRepository(s.db).GetByID(*scope.CollectionID)
	if err != nil {
		return err
	}
	if collection.IsPrivate && (collection.CreatedBy == nil || *collection.CreatedBy != userID) {
		return repository.ErrNotFound
	}
	if collection.WorkspaceID != nil {
		allowed, err := s.perm.HasWorkspacePermission(userID, *collection.WorkspaceID, models.PermissionItemView)
		if err != nil {
			return err
		}
		if !allowed {
			return repository.ErrNotFound
		}
	}
	return nil
}

// authorizeQueueWrite keeps the collection owner as the only author, with the
// workspace item.view check retained when the collection is bound to one. The
// workspace default view remains admin-only.
func (s *ItemApplicationService) authorizeQueueWrite(userID int, scope QueueScope) error {
	if scope.CollectionID == nil {
		allowed, err := s.perm.HasWorkspacePermission(userID, scope.WorkspaceID, models.PermissionWorkspaceAdmin)
		if err != nil {
			return err
		}
		if !allowed {
			return repository.ErrNotFound
		}
		return nil
	}
	collection, err := repository.NewCollectionRepository(s.db).GetByID(*scope.CollectionID)
	if err != nil {
		return err
	}
	if collection.CreatedBy == nil || *collection.CreatedBy != userID {
		return repository.ErrNotFound
	}
	if collection.WorkspaceID != nil {
		allowed, err := s.perm.HasWorkspacePermission(userID, *collection.WorkspaceID, models.PermissionItemView)
		if err != nil {
			return err
		}
		if !allowed {
			return repository.ErrNotFound
		}
	}
	return nil
}

func (s *ItemApplicationService) validateQueueInput(ctx context.Context, userID int, input QueueInput) (name, ql string, filterState *string, err error) {
	name = strings.TrimSpace(input.Name)
	if name == "" {
		return "", "", nil, queueValidation("name is required")
	}
	if len(name) > 120 {
		return "", "", nil, queueValidation("name must be at most 120 characters")
	}
	ql = strings.TrimSpace(input.QL)
	if ql == "" {
		return "", "", nil, queueValidation("ql_query is required")
	}
	if _, qlErr := s.crud.resolveItemListQLContext(ctx, ql, 0, "", userID); qlErr != nil {
		return "", "", nil, queueValidation("ql_query is invalid: " + qlErr.Error())
	}
	filterState = input.FilterState
	if filterState != nil && strings.TrimSpace(*filterState) == "" {
		filterState = nil
	}
	return name, ql, filterState, nil
}

func queueViewFromModel(queue *models.Queue) *QueueView {
	return &QueueView{
		ID:          queue.ID,
		Name:        queue.Name,
		QL:          queue.QLQuery,
		FilterState: queue.FilterState,
		Position:    queue.Position,
	}
}

func findBuiltinQueuePreset(key string) (builtinQueuePreset, bool) {
	for _, preset := range builtinQueuePresets {
		if preset.Key == key {
			return preset, true
		}
	}
	return builtinQueuePreset{}, false
}

func builtinQueuePosition(key string) int {
	for i, preset := range builtinQueuePresets {
		if preset.Key == key {
			return i
		}
	}
	return 0
}
