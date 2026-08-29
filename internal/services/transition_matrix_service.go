package services

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"windshift/internal/database"
	"windshift/internal/repository"
)

// WorkspaceTransitionMatrix is the database-independent representation used
// by the transition-matrix handler. Options are grouped by item type and then
// by current status; the handler owns the public JSON key format.
type WorkspaceTransitionMatrix struct {
	ByItemType    map[int]map[int][]StatusTransitionOption
	ItemTypeCount int
	StatusCount   int
	WorkflowCount int
	SQLCount      int
	QueryDuration time.Duration
}

// TransitionMatrixStats is a process-local lifetime snapshot. The service
// deliberately has no persistent cache: matrices are configuration data and
// stale workflow edges are more harmful than repeating three bounded reads.
// Concurrent reads for the same workspace are still coalesced.
type TransitionMatrixStats struct {
	PersistentCacheEnabled bool  `json:"persistent_cache_enabled"`
	Requests               int64 `json:"requests"`
	DatabaseLoads          int64 `json:"database_loads"`
	CoalescedResponses     int64 `json:"coalesced_responses"`
	Errors                 int64 `json:"errors"`
	LastSQLCount           int64 `json:"last_sql_count"`
	LastQueryDurationMS    int64 `json:"last_query_duration_ms"`
	LastItemTypeCount      int64 `json:"last_item_type_count"`
	LastStatusCount        int64 `json:"last_status_count"`
	LastWorkflowCount      int64 `json:"last_workflow_count"`
	LastResponseBytes      int64 `json:"last_response_bytes"`
	MaxResponseBytes       int64 `json:"max_response_bytes"`
}

type transitionMatrixCounters struct {
	requests            atomic.Int64
	databaseLoads       atomic.Int64
	coalescedResponses  atomic.Int64
	errors              atomic.Int64
	lastSQLCount        atomic.Int64
	lastQueryDurationMS atomic.Int64
	lastItemTypeCount   atomic.Int64
	lastStatusCount     atomic.Int64
	lastWorkflowCount   atomic.Int64
	lastResponseBytes   atomic.Int64
	maxResponseBytes    atomic.Int64
}

// TransitionMatrixService loads a workspace matrix with a statement count
// independent of item-type/status cardinality.
type TransitionMatrixService struct {
	db       database.Database
	loads    singleflight.Group
	counters transitionMatrixCounters
}

func NewTransitionMatrixService(db database.Database) *TransitionMatrixService {
	return &TransitionMatrixService{db: db}
}

// Load coalesces concurrent cold reads for the same workspace. It does not
// retain results after the load completes, so workflow/status/configuration
// mutations are visible on the next request without invalidation plumbing.
func (s *TransitionMatrixService) Load(ctx context.Context, workspaceID int) (*WorkspaceTransitionMatrix, error) {
	s.counters.requests.Add(1)
	value, err, shared := s.loads.Do(fmt.Sprintf("workspace:%d", workspaceID), func() (any, error) {
		s.counters.databaseLoads.Add(1)
		matrix, loadErr := s.load(ctx, workspaceID)
		if loadErr != nil {
			s.counters.errors.Add(1)
			return nil, loadErr
		}
		s.observeMatrix(matrix)
		return matrix, nil
	})
	if shared {
		s.counters.coalescedResponses.Add(1)
	}
	if err != nil {
		return nil, err
	}
	matrix, ok := value.(*WorkspaceTransitionMatrix)
	if !ok {
		return nil, fmt.Errorf("transition-matrix singleflight returned %T", value)
	}
	return matrix, nil
}

func (s *TransitionMatrixService) load(ctx context.Context, workspaceID int) (*WorkspaceTransitionMatrix, error) {
	matrix := &WorkspaceTransitionMatrix{ByItemType: map[int]map[int][]StatusTransitionOption{}}

	queryStarted := time.Now()
	itemTypeWorkflows, isPersonal, err := s.loadItemTypeWorkflows(ctx, workspaceID)
	matrix.QueryDuration += time.Since(queryStarted)
	matrix.SQLCount++
	if err != nil {
		return nil, err
	}
	if isPersonal || len(itemTypeWorkflows) == 0 {
		return matrix, nil
	}

	workflowIDs := uniqueSortedWorkflowIDs(itemTypeWorkflows)
	queryStarted = time.Now()
	statuses, err := s.loadStatuses(ctx, workflowIDs)
	matrix.QueryDuration += time.Since(queryStarted)
	matrix.SQLCount++
	if err != nil {
		return nil, err
	}
	queryStarted = time.Now()
	edges, anyEdges, err := s.loadEdges(ctx, workflowIDs)
	matrix.QueryDuration += time.Since(queryStarted)
	matrix.SQLCount++
	if err != nil {
		return nil, err
	}

	perWorkflow := make(map[int]map[int][]StatusTransitionOption, len(workflowIDs))
	for _, workflowID := range workflowIDs {
		byStatus := make(map[int][]StatusTransitionOption, len(statuses))
		for _, status := range statuses {
			options := make([]StatusTransitionOption, 0, 1+len(edges[workflowID][status.StatusID])+len(anyEdges[workflowID]))
			options = append(options, status)
			added := map[int]struct{}{status.StatusID: {}}
			for _, edge := range edges[workflowID][status.StatusID] {
				if _, exists := added[edge.StatusID]; exists {
					continue
				}
				options = append(options, edge)
				added[edge.StatusID] = struct{}{}
			}
			// From-all rows apply from every status; direct edges above already
			// claimed their targets, so only uncovered ones are appended.
			for _, edge := range anyEdges[workflowID] {
				if _, exists := added[edge.StatusID]; exists {
					continue
				}
				options = append(options, edge)
				added[edge.StatusID] = struct{}{}
			}
			byStatus[status.StatusID] = options
		}
		perWorkflow[workflowID] = byStatus
	}

	for itemTypeID, workflowID := range itemTypeWorkflows {
		matrix.ByItemType[itemTypeID] = perWorkflow[workflowID]
	}
	matrix.ItemTypeCount = len(itemTypeWorkflows)
	matrix.StatusCount = len(statuses)
	matrix.WorkflowCount = len(workflowIDs)
	return matrix, nil
}

// loadItemTypeWorkflows resolves personal-workspace behavior, applicability,
// item-type overrides, configuration defaults, and the global fallback in one
// set query. A workspace with an explicit item-type catalog emits only those
// types; an unconfigured workspace (or a configuration with no type mappings)
// emits the global catalog, matching WorkspaceService.GetItemTypes.
func (s *TransitionMatrixService) loadItemTypeWorkflows(ctx context.Context, workspaceID int) (resolved map[int]int, isPersonal bool, err error) {
	return repository.NewConfigurationSetRepository(s.db).ListItemTypeWorkflows(ctx, workspaceID)
}

func (s *TransitionMatrixService) loadStatuses(ctx context.Context, workflowIDs []int) ([]StatusTransitionOption, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(workflowIDs)), ",")
	query := fmt.Sprintf(`
		SELECT s.id, COALESCE(s.builtin_key, ''), s.name, sc.color
		FROM statuses s
		LEFT JOIN status_categories sc ON sc.id = s.category_id
		WHERE NOT EXISTS (
			SELECT 1 FROM workflow_transitions wt
			WHERE wt.workflow_id IN (%s)
		)
		OR EXISTS (
			SELECT 1 FROM workflow_transitions wt
			WHERE wt.workflow_id IN (%s)
			  AND (wt.from_status_id = s.id OR wt.to_status_id = s.id)
		)
		ORDER BY s.category_id, s.name, s.id
	`, placeholders, placeholders)
	args := make([]any, 0, len(workflowIDs)*2)
	for range 2 {
		for _, workflowID := range workflowIDs {
			args = append(args, workflowID)
		}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("load transition-matrix statuses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	statuses := []StatusTransitionOption{}
	for rows.Next() {
		var status StatusTransitionOption
		var categoryColor sql.NullString
		if err := rows.Scan(&status.StatusID, &status.BuiltinKey, &status.StatusName, &categoryColor); err != nil {
			return nil, fmt.Errorf("scan transition-matrix status: %w", err)
		}
		if categoryColor.Valid {
			status.CategoryColor = &categoryColor.String
		}
		statuses = append(statuses, status)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transition-matrix statuses: %w", err)
	}
	return statuses, nil
}

func (s *TransitionMatrixService) loadEdges(ctx context.Context, workflowIDs []int) (edgesByStatus map[int]map[int][]StatusTransitionOption, anyEdgesByWorkflow map[int][]StatusTransitionOption, err error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(workflowIDs)), ",")
	query := fmt.Sprintf(`
		SELECT wt.workflow_id, wt.from_status_id, wt.from_all_statuses, wt.id, s.id, COALESCE(s.builtin_key, ''), s.name, sc.color
		FROM workflow_transitions wt
		JOIN statuses s ON s.id = wt.to_status_id
		LEFT JOIN status_categories sc ON sc.id = s.category_id
		WHERE wt.workflow_id IN (%s) AND (wt.from_status_id IS NOT NULL OR wt.from_all_statuses = TRUE)
		ORDER BY wt.workflow_id, wt.from_status_id, wt.display_order, wt.id
	`, placeholders)
	args := make([]any, len(workflowIDs))
	for i, id := range workflowIDs {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("load transition-matrix edges: %w", err)
	}
	defer func() { _ = rows.Close() }()

	edges := make(map[int]map[int][]StatusTransitionOption, len(workflowIDs))
	anyEdges := make(map[int][]StatusTransitionOption, len(workflowIDs))
	for rows.Next() {
		var workflowID int
		var fromStatusID sql.NullInt64
		var fromAll bool
		var option repository.StatusTransitionOption
		var categoryColor sql.NullString
		if err := rows.Scan(
			&workflowID,
			&fromStatusID,
			&fromAll,
			&option.TransitionID,
			&option.StatusID,
			&option.BuiltinKey,
			&option.StatusName,
			&categoryColor,
		); err != nil {
			return nil, nil, fmt.Errorf("scan transition-matrix edge: %w", err)
		}
		if categoryColor.Valid {
			option.CategoryColor = &categoryColor.String
		}
		if edges[workflowID] == nil {
			edges[workflowID] = map[int][]StatusTransitionOption{}
		}
		if fromAll {
			anyEdges[workflowID] = append(anyEdges[workflowID], option)
			continue
		}
		fromID := int(fromStatusID.Int64)
		edges[workflowID][fromID] = append(edges[workflowID][fromID], option)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate transition-matrix edges: %w", err)
	}
	return edges, anyEdges, nil
}

func uniqueSortedWorkflowIDs(itemTypeWorkflows map[int]int) []int {
	seen := make(map[int]struct{}, len(itemTypeWorkflows))
	for _, workflowID := range itemTypeWorkflows {
		seen[workflowID] = struct{}{}
	}
	ids := make([]int, 0, len(seen))
	for workflowID := range seen {
		ids = append(ids, workflowID)
	}
	sort.Ints(ids)
	return ids
}

func (s *TransitionMatrixService) observeMatrix(matrix *WorkspaceTransitionMatrix) {
	s.counters.lastSQLCount.Store(int64(matrix.SQLCount))
	s.counters.lastQueryDurationMS.Store(matrix.QueryDuration.Milliseconds())
	s.counters.lastItemTypeCount.Store(int64(matrix.ItemTypeCount))
	s.counters.lastStatusCount.Store(int64(matrix.StatusCount))
	s.counters.lastWorkflowCount.Store(int64(matrix.WorkflowCount))
}

func (s *TransitionMatrixService) ObserveResponseSize(size int) {
	s.counters.lastResponseBytes.Store(int64(size))
	for {
		current := s.counters.maxResponseBytes.Load()
		if int64(size) <= current || s.counters.maxResponseBytes.CompareAndSwap(current, int64(size)) {
			return
		}
	}
}

func (s *TransitionMatrixService) Stats() TransitionMatrixStats {
	return TransitionMatrixStats{
		PersistentCacheEnabled: false,
		Requests:               s.counters.requests.Load(),
		DatabaseLoads:          s.counters.databaseLoads.Load(),
		CoalescedResponses:     s.counters.coalescedResponses.Load(),
		Errors:                 s.counters.errors.Load(),
		LastSQLCount:           s.counters.lastSQLCount.Load(),
		LastQueryDurationMS:    s.counters.lastQueryDurationMS.Load(),
		LastItemTypeCount:      s.counters.lastItemTypeCount.Load(),
		LastStatusCount:        s.counters.lastStatusCount.Load(),
		LastWorkflowCount:      s.counters.lastWorkflowCount.Load(),
		LastResponseBytes:      s.counters.lastResponseBytes.Load(),
		MaxResponseBytes:       s.counters.maxResponseBytes.Load(),
	}
}
