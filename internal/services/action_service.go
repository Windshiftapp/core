// Package services provides business logic and service layer functionality.
package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"windshift/internal/database"
	"windshift/internal/llm"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"

	"github.com/google/uuid"
)

// LLMConnectionResolver resolves an LLM connection ID to a client.
type LLMConnectionResolver interface {
	Resolve(connectionID int) (llm.Client, error)
}

// AssetSetPermissionChecker checks per-asset-set RBAC. Asset sets have their
// own role-based permission model independent of workspace membership, so a
// workspace admin is NOT automatically allowed to write into an asset set.
// Permission keys are the strings used by handlers/asset_handler.go:
// "asset.view", "asset.create", "asset.edit", etc.
type AssetSetPermissionChecker interface {
	HasAssetSetPermission(userID, setID int, permissionKey string) (bool, error)
}

// ActionServiceConfig represents configuration for the action service
type ActionServiceConfig struct {
	RefreshInterval time.Duration // How often to refresh action cache
	EventBufferSize int           // Size of event channel buffer
}

// DefaultActionServiceConfig returns default configuration
func DefaultActionServiceConfig() ActionServiceConfig {
	return ActionServiceConfig{
		RefreshInterval: 5 * time.Minute,
		EventBufferSize: 500,
	}
}

// ActionService handles asynchronous action execution
type ActionService struct {
	db       database.Database
	repo     *repository.ActionRepository
	itemRepo *repository.ItemRepository
	config   ActionServiceConfig

	// Action cache: workspace_id -> enabled actions
	actionCache map[int][]*models.Action
	cacheMu     sync.RWMutex

	// Event processing
	eventChan chan *models.ActionEvent
	stopChan  chan struct{}
	wg        sync.WaitGroup

	// Dependencies for action execution
	notificationService *NotificationService
	commentService      *CommentService
	assetActionService  AssetActionEventEmitter
	eventCoordinator    *EventCoordinator
	teamService         *TeamService

	// AI/container dependencies
	llmConnectionManager LLMConnectionResolver
	containerService     *ContainerService

	// Asset permission checker — consulted before create_asset / update_asset
	// nodes mutate an asset set the action's actor may not control.
	assetPermChecker AssetSetPermissionChecker

	// Shared execution chain store for cross-application cascade loop prevention
	chainStore *ExecutionChainStore

	// Statistics
	eventsProcessed int64
	actionsExecuted int64
	errors          int64
}

// NewActionService creates a new action service
func NewActionService(db database.Database, config ActionServiceConfig, chainStore *ExecutionChainStore) *ActionService {
	if chainStore == nil {
		chainStore = NewExecutionChainStore()
	}
	service := &ActionService{
		db:          db,
		repo:        repository.NewActionRepository(db),
		itemRepo:    repository.NewItemRepository(db),
		config:      config,
		actionCache: make(map[int][]*models.Action),
		eventChan:   make(chan *models.ActionEvent, config.EventBufferSize),
		stopChan:    make(chan struct{}),
		chainStore:  chainStore,
	}

	// Load initial cache
	if err := service.refreshActionCache(); err != nil {
		slog.Warn("failed to load initial action cache", slog.String("component", "actions"), slog.Any("error", err))
	}

	// Start background workers
	service.wg.Add(2)
	go service.eventProcessor()
	go service.cacheRefresher()

	slog.Debug("action service initialized", slog.String("component", "actions"), slog.Duration("refresh_interval", config.RefreshInterval))

	return service
}

// SetNotificationService sets the notification service for notify_user actions
func (as *ActionService) SetNotificationService(ns *NotificationService) {
	as.notificationService = ns
}

// SetCommentService sets the comment service for add_comment actions
func (as *ActionService) SetCommentService(cs *CommentService) {
	as.commentService = cs
}

// SetAssetActionService sets the asset action service for emitting asset events from create_asset/update_asset nodes.
func (as *ActionService) SetAssetActionService(aas AssetActionEventEmitter) {
	as.assetActionService = aas
}

// SetTeamService sets the team service for round-robin assignment actions
func (as *ActionService) SetTeamService(ts *TeamService) {
	as.teamService = ts
}

// SetEventCoordinator sets the event coordinator for emitting item events from create_item-like nodes.
func (as *ActionService) SetEventCoordinator(ec *EventCoordinator) {
	as.eventCoordinator = ec
}

// SetLLMConnectionManager sets the LLM connection manager for AI node types.
func (as *ActionService) SetLLMConnectionManager(m LLMConnectionResolver) {
	as.llmConnectionManager = m
}

// SetContainerService sets the container service for container_run nodes.
func (as *ActionService) SetContainerService(cs *ContainerService) {
	as.containerService = cs
}

// SetAssetPermissionChecker wires the asset-set RBAC check used by
// create_asset / update_asset nodes.
func (as *ActionService) SetAssetPermissionChecker(c AssetSetPermissionChecker) {
	as.assetPermChecker = c
}

// EmitActionEvent sends an event to be processed asynchronously (non-blocking)
func (as *ActionService) EmitActionEvent(event *models.ActionEvent) {
	slog.Debug("queuing action event",
		slog.String("component", "actions"),
		slog.String("event_type", string(event.EventType)),
		slog.Int("workspace_id", event.WorkspaceID),
		slog.Int("item_id", event.ItemID),
	)

	select {
	case as.eventChan <- event:
		// Event queued successfully
	default:
		// Channel full, log warning but don't block
		slog.Warn("action event channel full, dropping event",
			slog.String("component", "actions"),
			slog.String("event_type", string(event.EventType)),
			slog.Int("workspace_id", event.WorkspaceID),
		)
		atomic.AddInt64(&as.errors, 1)
	}
}

// Stop gracefully shuts down the action service
func (as *ActionService) Stop() {
	close(as.stopChan)

	done := make(chan struct{})
	go func() {
		as.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Debug("action service stopped successfully", slog.String("component", "actions"))
	case <-time.After(3 * time.Second):
		slog.Warn("action service stop timed out after 3s", slog.String("component", "actions"))
	}
}

// eventProcessor runs in background and processes events from the channel
func (as *ActionService) eventProcessor() {
	defer as.wg.Done()

	for {
		select {
		case event := <-as.eventChan:
			if err := as.processEvent(event); err != nil {
				slog.Error("failed to process action event",
					slog.String("component", "actions"),
					slog.String("event_type", string(event.EventType)),
					slog.Any("error", err),
				)
				atomic.AddInt64(&as.errors, 1)
			} else {
				atomic.AddInt64(&as.eventsProcessed, 1)
			}
		case <-as.stopChan:
			slog.Debug("stopping action event processor", slog.String("component", "actions"))
			// Drain remaining events
			for len(as.eventChan) > 0 {
				event := <-as.eventChan
				if err := as.processEvent(event); err != nil {
					slog.Error("failed to process action event during shutdown",
						slog.String("component", "actions"),
						slog.String("event_type", string(event.EventType)),
						slog.Any("error", err),
					)
				}
			}
			return
		}
	}
}

// cacheRefresher runs in background and periodically refreshes the action cache
func (as *ActionService) cacheRefresher() {
	defer as.wg.Done()

	ticker := time.NewTicker(as.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := as.refreshActionCache(); err != nil {
				slog.Error("failed to refresh action cache", slog.String("component", "actions"), slog.Any("error", err))
			}
			// Also cleanup stale execution chains
			as.cleanupChains()
		case <-as.stopChan:
			slog.Debug("stopping action cache refresher", slog.String("component", "actions"))
			return
		}
	}
}

// refreshActionCache reloads enabled actions from database
func (as *ActionService) refreshActionCache() error {
	// Get all workspaces with enabled actions
	rows, err := as.db.Query(`
		SELECT DISTINCT workspace_id FROM actions WHERE is_enabled = true
	`)
	if err != nil {
		return fmt.Errorf("failed to query workspaces with actions: %w", err)
	}
	defer rows.Close()

	newCache := make(map[int][]*models.Action)
	workspaceIDs := []int{}

	for rows.Next() {
		var workspaceID int
		if err := rows.Scan(&workspaceID); err != nil {
			continue
		}
		workspaceIDs = append(workspaceIDs, workspaceID)
	}

	// Snapshot the previous cache so we can preserve entries whose refresh
	// query fails this cycle. Dropping a workspace on a transient DB hiccup
	// would silently disable its automations for up to one refresh interval.
	as.cacheMu.RLock()
	prevCache := as.actionCache
	as.cacheMu.RUnlock()

	// Load enabled actions for each workspace
	for _, workspaceID := range workspaceIDs {
		actions, err := as.repo.ListEnabledByWorkspace(workspaceID)
		if err != nil {
			slog.Error("failed to load actions for workspace; keeping previous cache entry",
				slog.String("component", "actions"),
				slog.Int("workspace_id", workspaceID),
				slog.Any("error", err),
			)
			if prev, ok := prevCache[workspaceID]; ok {
				newCache[workspaceID] = prev
			}
			continue
		}
		newCache[workspaceID] = actions
	}

	as.cacheMu.Lock()
	as.actionCache = newCache
	as.cacheMu.Unlock()

	slog.Debug("action cache refreshed",
		slog.String("component", "actions"),
		slog.Int("workspace_count", len(newCache)),
	)

	return nil
}

// InvalidateWorkspaceCache invalidates the cache for a specific workspace
func (as *ActionService) InvalidateWorkspaceCache(workspaceID int) {
	actions, err := as.repo.ListEnabledByWorkspace(workspaceID)
	if err != nil {
		slog.Error("failed to reload actions for workspace",
			slog.String("component", "actions"),
			slog.Int("workspace_id", workspaceID),
			slog.Any("error", err),
		)
		return
	}

	as.cacheMu.Lock()
	if len(actions) > 0 {
		as.actionCache[workspaceID] = actions
	} else {
		delete(as.actionCache, workspaceID)
	}
	as.cacheMu.Unlock()
}

// getChain retrieves an execution chain from the shared store by its ID.
func (as *ActionService) getChain(chainID string) *ExecutionChain {
	return as.chainStore.GetChain(chainID)
}

// createChain creates a new execution chain in the shared store.
func (as *ActionService) createChain(chainID string) *ExecutionChain {
	return as.chainStore.CreateChain(chainID)
}

// cleanupChains delegates to the shared store for cleanup.
func (as *ActionService) cleanupChains() {
	as.chainStore.Cleanup()
}

// MaxCascadeDepth is the maximum depth of nested action triggers (safety limit)
const MaxCascadeDepth = 5

// processEvent processes a single action event
func (as *ActionService) processEvent(event *models.ActionEvent) error { //nolint:unparam // error return kept for API consistency
	slog.Debug("processing action event",
		slog.String("component", "actions"),
		slog.String("event_type", string(event.EventType)),
		slog.Int("workspace_id", event.WorkspaceID),
		slog.Int("item_id", event.ItemID),
		slog.Bool("triggered_by_action", event.TriggeredByAction),
		slog.Int("cascade_depth", event.CascadeDepth),
	)

	// Check cascade depth limit (uses event's immutable depth)
	if event.CascadeDepth >= MaxCascadeDepth {
		slog.Warn("action execution depth limit reached",
			slog.String("component", "actions"),
			slog.String("chain_id", event.ExecutionChainID),
			slog.Int("depth", event.CascadeDepth),
		)
		return nil
	}

	// Get chain state from cache for cycle detection (if cascaded event)
	var chain *ExecutionChain
	if event.ExecutionChainID != "" {
		chain = as.getChain(event.ExecutionChainID)
		if chain == nil {
			slog.Warn("execution chain not found in cache",
				slog.String("component", "actions"),
				slog.String("chain_id", event.ExecutionChainID),
			)
			// Chain expired or missing - treat as new chain (safe default)
		}
	}

	// Get actions for this workspace from cache
	as.cacheMu.RLock()
	actions := as.actionCache[event.WorkspaceID]
	as.cacheMu.RUnlock()

	if len(actions) == 0 {
		slog.Debug("no enabled actions for workspace",
			slog.String("component", "actions"),
			slog.Int("workspace_id", event.WorkspaceID),
		)
		return nil
	}

	// Find matching actions
	for _, action := range actions {
		// Cycle detection: skip if this action already ran in this chain
		actionKey := fmt.Sprintf("workspace:%d", action.ID)
		if chain != nil && chain.HasExecuted(actionKey) {
			slog.Debug("skipping action - already executed in chain",
				slog.String("component", "actions"),
				slog.Int("action_id", action.ID),
				slog.String("action_name", action.Name),
				slog.String("chain_id", event.ExecutionChainID),
			)
			continue
		}

		if as.matchesTrigger(action, event) {
			slog.Debug("action matches trigger, executing",
				slog.String("component", "actions"),
				slog.Int("action_id", action.ID),
				slog.String("action_name", action.Name),
			)

			if err := as.executeAction(action, event, chain); err != nil {
				slog.Error("failed to execute action",
					slog.String("component", "actions"),
					slog.Int("action_id", action.ID),
					slog.Any("error", err),
				)
				// Continue with other actions even if one fails
			} else {
				atomic.AddInt64(&as.actionsExecuted, 1)
			}
		}
	}

	return nil
}

// matchesTrigger checks if an action's trigger matches the event
func (as *ActionService) matchesTrigger(action *models.Action, event *models.ActionEvent) bool {
	// First check if trigger types match
	if action.TriggerType != event.EventType {
		return false
	}

	// Parse trigger config if present
	var config models.ActionTriggerConfig
	if action.TriggerConfig != "" {
		if err := json.Unmarshal([]byte(action.TriggerConfig), &config); err != nil {
			slog.Warn("failed to parse trigger config",
				slog.String("component", "actions"),
				slog.Int("action_id", action.ID),
				slog.Any("error", err),
			)
			return false
		}
	}

	// Check cascade control: if the event was triggered by another action,
	// only process if this action has respond_to_cascades enabled
	if event.TriggeredByAction && !config.RespondToCascades {
		slog.Debug("skipping action - does not respond to cascades",
			slog.String("component", "actions"),
			slog.Int("action_id", action.ID),
			slog.String("action_name", action.Name),
		)
		return false
	}

	// If no trigger config, any event of matching type triggers the action
	if action.TriggerConfig == "" {
		return true
	}

	switch event.EventType {
	case models.ActionTriggerStatusTransition:
		// Check from_status_id and to_status_id conditions
		if config.FromStatusID != nil {
			oldStatusID := utils.InterfaceToIntPtr(event.OldValues["status_id"])
			if oldStatusID == nil || *oldStatusID != *config.FromStatusID {
				return false
			}
		}
		if config.ToStatusID != nil {
			newStatusID := utils.InterfaceToIntPtr(event.NewValues["status_id"])
			if newStatusID == nil || *newStatusID != *config.ToStatusID {
				return false
			}
		}

	case models.ActionTriggerItemCreated, models.ActionTriggerItemUpdated:
		// Check item_type_id filter. Events carry Item.ItemTypeID which is
		// *int, so a raw `.(int)` assertion would always fail — route through
		// InterfaceToIntPtr to unwrap *int / int / int64 / float64 uniformly.
		if config.ItemTypeID != nil {
			itemTypeID := utils.InterfaceToIntPtr(event.NewValues["item_type_id"])
			if itemTypeID == nil || *itemTypeID != *config.ItemTypeID {
				return false
			}
		}
		// For item_updated, check field_name filter
		if event.EventType == models.ActionTriggerItemUpdated && config.FieldName != "" {
			if _, changed := event.NewValues[config.FieldName]; !changed {
				return false
			}
		}

	case models.ActionTriggerItemLinked:
		// Check link_type_id filter — same unwrapping as item_type_id so this
		// keeps working if the event emitter ever starts sending *int.
		if config.LinkTypeID != nil {
			linkTypeID := utils.InterfaceToIntPtr(event.NewValues["link_type_id"])
			if linkTypeID == nil || *linkTypeID != *config.LinkTypeID {
				return false
			}
		}
	}

	return true
}

// executeAction executes an action's flow
func (as *ActionService) executeAction(action *models.Action, event *models.ActionEvent, chain *ExecutionChain) error {
	startTime := time.Now()

	// Get or create execution chain for cascade tracking
	chainID := event.ExecutionChainID
	if chainID == "" {
		// First action in chain - create new chain
		chainID = uuid.New().String()
		chain = as.createChain(chainID)
	} else if chain == nil {
		// Chain ID exists but chain not found (expired) - create new one
		chain = as.createChain(chainID)
	}

	// Mark this action as executed (for cycle detection)
	actionKey := fmt.Sprintf("workspace:%d", action.ID)
	chain.MarkExecuted(actionKey)

	// Create execution log
	log := &models.ActionExecutionLog{
		ActionID:     action.ID,
		ItemID:       &event.ItemID,
		TriggerEvent: string(event.EventType),
		Status:       models.ActionStatusRunning,
		StartedAt:    startTime,
	}
	logID, err := as.repo.CreateExecutionLog(log)
	if err != nil {
		slog.Warn("failed to create execution log",
			slog.String("component", "actions"),
			slog.Int("action_id", action.ID),
			slog.Any("error", err),
		)
	}
	log.ID = logID

	// Build execution context
	ctx := &models.ExecutionContext{
		Action:      action,
		Event:       event,
		Variables:   make(map[string]interface{}),
		StepResults: []models.StepResult{},
		ChainID:     chainID,
	}

	// Populate initial variables from event
	ctx.Variables["item_id"] = event.ItemID
	ctx.Variables["workspace_id"] = event.WorkspaceID
	ctx.Variables["actor_user_id"] = event.ActorUserID
	for k, v := range event.OldValues {
		ctx.Variables["old_"+k] = v
	}
	for k, v := range event.NewValues {
		ctx.Variables["new_"+k] = v
	}

	// Get topologically sorted nodes
	sortedNodes, err := as.topologicalSort(action.Nodes, action.Edges)
	if err != nil {
		log.Status = models.ActionStatusFailed
		log.ErrorMessage = fmt.Sprintf("failed to sort nodes: %v", err)
		completedAt := time.Now()
		log.CompletedAt = &completedAt
		if logErr := as.repo.UpdateExecutionLog(log); logErr != nil {
			slog.Error("failed to update execution log", slog.Any("error", logErr), slog.Int("action_id", action.ID))
		}
		return fmt.Errorf("failed to topologically sort nodes: %w", err)
	}

	// Execute nodes in order
	executedNodes := make(map[int]bool)
	for _, node := range sortedNodes {
		// Skip trigger nodes - they're just entry points
		if node.NodeType == models.ActionNodeTrigger {
			executedNodes[node.ID] = true
			continue
		}

		// Check if all incoming edges allow execution
		canExecute := as.canExecuteNode(node.ID, action.Edges, executedNodes, ctx)
		if !canExecute {
			continue
		}

		stepResult := models.StepResult{
			NodeID:    node.ID,
			NodeType:  node.NodeType,
			Status:    models.ActionStatusRunning,
			StartedAt: time.Now(),
		}

		err := as.executeNode(&node, ctx, &stepResult)
		completedAt := time.Now()
		stepResult.CompletedAt = &completedAt

		if err != nil {
			stepResult.Status = models.ActionStatusFailed
			stepResult.ErrorMessage = err.Error()
			ctx.StepResults = append(ctx.StepResults, stepResult)

			// Log failure but continue - some failures are acceptable
			slog.Warn("node execution failed",
				slog.String("component", "actions"),
				slog.Int("node_id", node.ID),
				slog.String("node_type", string(node.NodeType)),
				slog.Any("error", err),
			)
		} else {
			stepResult.Status = models.ActionStatusCompleted
			ctx.StepResults = append(ctx.StepResults, stepResult)
			executedNodes[node.ID] = true
		}
	}

	// Tear down any containers started during this run — they exist only to
	// service nodes inside this action, and the auto-teardown timeout is a
	// coarse upper bound, not a cleanup signal.
	as.cleanupActionContainers(ctx.StepResults)

	// Update execution log
	completedAt := time.Now()
	log.CompletedAt = &completedAt
	log.Status = models.ActionStatusCompleted

	// Check if any step failed
	for _, result := range ctx.StepResults {
		if result.Status == models.ActionStatusFailed {
			log.Status = models.ActionStatusFailed
			break
		}
	}

	// Serialize execution trace
	if trace, err := json.Marshal(ctx.StepResults); err == nil {
		log.ExecutionTrace = string(trace)
	}

	if logErr := as.repo.UpdateExecutionLog(log); logErr != nil {
		slog.Error("failed to update execution log", slog.Any("error", logErr), slog.Int("action_id", action.ID))
	}

	slog.Debug("action execution completed",
		slog.String("component", "actions"),
		slog.Int("action_id", action.ID),
		slog.String("status", string(log.Status)),
		slog.Duration("duration", time.Since(startTime)),
	)

	return nil
}

// topologicalSort sorts nodes in execution order using Kahn's algorithm
func (as *ActionService) topologicalSort(nodes []models.ActionNode, edges []models.ActionEdge) ([]models.ActionNode, error) {
	if len(nodes) == 0 {
		return nil, nil
	}

	// Build adjacency list and in-degree map
	nodeMap := make(map[int]*models.ActionNode)
	inDegree := make(map[int]int)
	adjacency := make(map[int][]int)

	for i := range nodes {
		nodeMap[nodes[i].ID] = &nodes[i]
		inDegree[nodes[i].ID] = 0
		adjacency[nodes[i].ID] = []int{}
	}

	for _, edge := range edges {
		adjacency[edge.SourceNodeID] = append(adjacency[edge.SourceNodeID], edge.TargetNodeID)
		inDegree[edge.TargetNodeID]++
	}

	// Find nodes with no incoming edges
	queue := []int{}
	for nodeID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, nodeID)
		}
	}

	sorted := []models.ActionNode{}
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]

		if node, ok := nodeMap[nodeID]; ok {
			sorted = append(sorted, *node)
		}

		for _, targetID := range adjacency[nodeID] {
			inDegree[targetID]--
			if inDegree[targetID] == 0 {
				queue = append(queue, targetID)
			}
		}
	}

	// Check for cycles
	if len(sorted) != len(nodes) {
		return nil, fmt.Errorf("cycle detected in action flow")
	}

	return sorted, nil
}

// canExecuteNode checks if a node can be executed based on incoming edges
func (as *ActionService) canExecuteNode(nodeID int, edges []models.ActionEdge, executedNodes map[int]bool, ctx *models.ExecutionContext) bool {
	hasIncomingEdge := false
	for _, edge := range edges {
		if edge.TargetNodeID == nodeID {
			hasIncomingEdge = true

			// Check if source was executed
			if !executedNodes[edge.SourceNodeID] {
				return false
			}

			// For condition edges, check the edge type matches the condition result
			if edge.EdgeType == "true" || edge.EdgeType == "false" {
				// Find the condition result in step results
				for _, result := range ctx.StepResults {
					if result.NodeID == edge.SourceNodeID {
						condResult, ok := result.Output["condition_result"].(bool)
						if !ok {
							return false
						}
						if edge.EdgeType == "true" && !condResult {
							return false
						}
						if edge.EdgeType == "false" && condResult {
							return false
						}
					}
				}
			}
		}
	}

	// If no incoming edges, it's a root node (trigger) - always can execute
	return hasIncomingEdge || len(edges) == 0
}

// executeNode executes a single node
func (as *ActionService) executeNode(node *models.ActionNode, ctx *models.ExecutionContext, stepResult *models.StepResult) error {
	switch node.NodeType {
	case models.ActionNodeSetField:
		return as.executeSetField(node, ctx, stepResult)
	case models.ActionNodeSetStatus:
		return as.executeSetStatus(node, ctx, stepResult)
	case models.ActionNodeAddComment:
		return as.executeAddComment(node, ctx, stepResult)
	case models.ActionNodeNotifyUser:
		return as.executeNotifyUser(node, ctx, stepResult)
	case models.ActionNodeCondition:
		return as.executeCondition(node, ctx, stepResult)
	case models.ActionNodeUpdateAsset:
		return as.executeUpdateAsset(node, ctx, stepResult)
	case models.ActionNodeCreateAsset:
		return as.executeCreateAsset(node, ctx, stepResult)
	case models.ActionNodeRoundRobinAssign:
		return as.executeRoundRobinAssign(node, ctx, stepResult)
	case models.ActionNodeAIExtract:
		return as.executeAIExtract(node, ctx, stepResult)
	case models.ActionNodeAIAgent:
		return as.executeAIAgent(node, ctx, stepResult)
	case models.ActionNodeContainerRun:
		return as.executeContainerRun(node, ctx, stepResult)
	case models.ActionNodeHTTPRequest:
		return as.executeHTTPRequest(node, ctx, stepResult)
	default:
		return fmt.Errorf("unknown node type: %s", node.NodeType)
	}
}

// executeSetField executes a set_field node. It dispatches to either the
// items-table column path or the custom-field path based on config.Target
// (absent/empty == column, for backward compatibility with pre-target configs).
func (as *ActionService) executeSetField(node *models.ActionNode, ctx *models.ExecutionContext, stepResult *models.StepResult) error {
	var config models.SetFieldNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse set_field config: %w", err)
	}

	value := as.substituteVariables(config.Value, ctx)

	if config.Target == "custom_field" {
		return as.executeSetFieldCustom(ctx, stepResult, config, value)
	}
	return as.executeSetFieldColumn(ctx, stepResult, config, value)
}

func (as *ActionService) executeSetFieldColumn(ctx *models.ExecutionContext, stepResult *models.StepResult, config models.SetFieldNodeConfig, value string) error {
	// Reject field names that aren't part of the items-table allowlist —
	// config.FieldName is attacker-controlled (workspace admin writes node config),
	// and it's interpolated into the SELECT below.
	if !repository.IsAllowedItemColumn(config.FieldName) {
		return fmt.Errorf("set_field: field %q is not a writable item column", config.FieldName)
	}

	// Get current field value for event emission (best effort).
	// Safe to concatenate config.FieldName because it was just validated against the allowlist.
	var oldValue interface{}
	row := as.db.QueryRow(`SELECT `+config.FieldName+` FROM items WHERE id = ?`, ctx.Event.ItemID)
	if err := row.Scan(&oldValue); err != nil {
		slog.Debug("failed to get current field value for cascade event",
			slog.String("component", "actions"),
			slog.String("field_name", config.FieldName),
			slog.Int("item_id", ctx.Event.ItemID),
			slog.Any("error", err),
		)
	}

	tx, err := as.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := as.itemRepo.UpdateFields(tx, ctx.Event.ItemID, map[string]interface{}{
		config.FieldName: value,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	stepResult.Output = map[string]interface{}{
		"field_name": config.FieldName,
		"old_value":  oldValue,
		"new_value":  value,
	}

	as.EmitActionEvent(&models.ActionEvent{
		EventType:         models.ActionTriggerItemUpdated,
		WorkspaceID:       ctx.Event.WorkspaceID,
		ItemID:            ctx.Event.ItemID,
		ActorUserID:       ctx.Event.ActorUserID,
		OldValues:         map[string]interface{}{config.FieldName: oldValue},
		NewValues:         map[string]interface{}{config.FieldName: value},
		TriggeredByAction: true,
		ExecutionChainID:  ctx.ChainID,
		CascadeDepth:      ctx.Event.CascadeDepth + 1,
	})

	return nil
}

func (as *ActionService) executeSetFieldCustom(ctx *models.ExecutionContext, stepResult *models.StepResult, config models.SetFieldNodeConfig, value string) error {
	if config.CustomFieldID <= 0 {
		return fmt.Errorf("set_field: custom_field target requires a positive custom_field_id")
	}

	oldValue, err := as.itemRepo.GetItemCustomFieldValue(ctx.Event.ItemID, config.CustomFieldID)
	if err != nil {
		slog.Debug("failed to get current custom field value for cascade event",
			slog.String("component", "actions"),
			slog.Int("custom_field_id", config.CustomFieldID),
			slog.Int("item_id", ctx.Event.ItemID),
			slog.Any("error", err),
		)
	}

	tx, err := as.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := as.itemRepo.SetItemCustomFieldValue(tx, ctx.Event.ItemID, config.CustomFieldID, value); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	key := "custom_field_" + strconv.Itoa(config.CustomFieldID)
	stepResult.Output = map[string]interface{}{
		"field_name":      key,
		"custom_field_id": config.CustomFieldID,
		"old_value":       oldValue,
		"new_value":       value,
	}

	as.EmitActionEvent(&models.ActionEvent{
		EventType:         models.ActionTriggerItemUpdated,
		WorkspaceID:       ctx.Event.WorkspaceID,
		ItemID:            ctx.Event.ItemID,
		ActorUserID:       ctx.Event.ActorUserID,
		OldValues:         map[string]interface{}{key: oldValue},
		NewValues:         map[string]interface{}{key: value},
		TriggeredByAction: true,
		ExecutionChainID:  ctx.ChainID,
		CascadeDepth:      ctx.Event.CascadeDepth + 1,
	})

	return nil
}

// executeSetStatus executes a set_status node. The transition is routed
// through WorkflowService.PerformTransition so workflow validity is enforced
// and history is recorded. Condition-mode / validator-mode rules are NOT
// evaluated for action-triggered transitions (automations run as the system,
// not as the triggering user).
func (as *ActionService) executeSetStatus(node *models.ActionNode, ctx *models.ExecutionContext, stepResult *models.StepResult) error {
	var config models.SetStatusNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse set_status config: %w", err)
	}

	workflowService := NewWorkflowService(as.db)
	result, err := workflowService.PerformTransition(context.Background(), PerformTransitionRequest{
		ItemID:      ctx.Event.ItemID,
		ToStatusID:  config.StatusID,
		ActorUserID: ctx.Event.ActorUserID,
		// Automations skip conditions — empty modes enforces only workflow validity.
		Modes: nil,
	}, as.itemRepo, nil)
	if err != nil {
		if rej := IsTransitionRejection(err); rej != nil {
			slog.Warn("set_status action rejected by workflow",
				slog.String("component", "actions"),
				slog.Int("item_id", ctx.Event.ItemID),
				slog.Int("to_status_id", config.StatusID),
				slog.String("reason", rej.Code),
				slog.String("message", rej.Message),
			)
			return fmt.Errorf("set_status rejected: %s", rej.Message)
		}
		return err
	}

	oldStatusID := 0
	if result.OldStatusID != nil {
		oldStatusID = *result.OldStatusID
	}
	newStatusID := config.StatusID
	if result.NewStatusID != nil {
		newStatusID = *result.NewStatusID
	}

	stepResult.Output = map[string]interface{}{
		"old_status_id":   oldStatusID,
		"new_status_id":   newStatusID,
		"old_status_name": as.getStatusName(oldStatusID),
		"new_status_name": as.getStatusName(newStatusID),
	}

	// Emit cascade event if status actually changed.
	if !result.NoOp {
		as.EmitActionEvent(&models.ActionEvent{
			EventType:         models.ActionTriggerStatusTransition,
			WorkspaceID:       ctx.Event.WorkspaceID,
			ItemID:            ctx.Event.ItemID,
			ActorUserID:       ctx.Event.ActorUserID,
			OldValues:         map[string]interface{}{"status_id": oldStatusID},
			NewValues:         map[string]interface{}{"status_id": newStatusID},
			TriggeredByAction: true,
			ExecutionChainID:  ctx.ChainID,
			CascadeDepth:      ctx.Event.CascadeDepth + 1,
		})
	}

	return nil
}

// executeAddComment executes an add_comment node
func (as *ActionService) executeAddComment(node *models.ActionNode, ctx *models.ExecutionContext, stepResult *models.StepResult) error {
	var config models.AddCommentNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse add_comment config: %w", err)
	}

	// Substitute variables in content
	content := as.substituteVariables(config.Content, ctx)

	var commentID int64

	// Use CommentService if available for unified comment creation with side effects
	if as.commentService != nil {
		result, err := as.commentService.Create(CreateCommentParams{
			ItemID:      ctx.Event.ItemID,
			AuthorID:    ctx.Event.ActorUserID,
			Content:     content,
			IsPrivate:   config.IsPrivate,
			ActorUserID: ctx.Event.ActorUserID,
		})
		if err != nil {
			return fmt.Errorf("failed to create comment via service: %w", err)
		}
		commentID = result.CommentID
	} else {
		// Legacy fallback: direct DB insert without side effects
		slog.Warn("commentService not configured, using legacy comment creation without notifications/mentions/webhooks",
			slog.String("component", "actions"),
			slog.Int("item_id", ctx.Event.ItemID),
		)

		now := time.Now()
		if err := as.db.QueryRow(`
			INSERT INTO comments (item_id, author_id, content, is_private, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?) RETURNING id
		`, ctx.Event.ItemID, ctx.Event.ActorUserID, content, config.IsPrivate, now, now).Scan(&commentID); err != nil {
			return err
		}
	}

	// Populate step result output with change details
	stepResult.Output = map[string]interface{}{
		"content":    content,
		"is_private": config.IsPrivate,
		"comment_id": commentID,
	}

	return nil
}

// executeNotifyUser executes a notify_user node
func (as *ActionService) executeNotifyUser(node *models.ActionNode, ctx *models.ExecutionContext, stepResult *models.StepResult) error {
	if as.notificationService == nil {
		slog.Warn("notification service not configured, skipping notify_user",
			slog.String("component", "actions"),
		)
		// Still populate output to show it was skipped
		stepResult.Output = map[string]interface{}{
			"recipient_count": 0,
			"skipped":         true,
			"reason":          "notification service not configured",
		}
		return nil
	}

	var config models.NotifyUserNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse notify_user config: %w", err)
	}

	// Determine recipient user IDs. The context's variable map may or may
	// not contain assignee/creator — for status_transition / cascade events
	// it often doesn't — so fall back to the item row.
	userIDs := []int{}
	for _, recipient := range config.Recipients {
		switch recipient {
		case "assignee":
			if id := as.lookupItemUserField(ctx, "assignee_id", "new_assignee_id"); id != 0 {
				userIDs = append(userIDs, id)
			}
		case "creator":
			if id := as.lookupItemUserField(ctx, "creator_id", "new_creator_id"); id != 0 {
				userIDs = append(userIDs, id)
			}
		default:
			// Try to parse as explicit user ID
			if id, err := strconv.Atoi(recipient); err == nil && id > 0 {
				userIDs = append(userIDs, id)
			}
		}
	}

	// Substitute variables in message
	message := as.substituteVariables(config.Message, ctx)
	title := as.substituteVariables(config.Title, ctx)

	// Dispatch to resolved recipients directly — rule-based routing can't
	// express "notify exactly these users" so we bypass it.
	err := as.notificationService.NotifyUsers(
		userIDs,
		ctx.Event.WorkspaceID,
		ctx.Event.ItemID,
		ctx.Event.ActorUserID,
		"action",
		title,
		message,
	)
	if err != nil {
		return fmt.Errorf("notify_user failed: %w", err)
	}

	// Populate step result output with notification details
	stepResult.Output = map[string]interface{}{
		"recipient_count": len(userIDs),
		"recipient_ids":   userIDs,
		"title":           title,
		"message":         message,
	}

	return nil
}

// lookupItemUserField resolves a user-id item column (assignee_id / creator_id)
// preferring the execution context's variable map and falling back to a direct
// DB read of the item. Returns 0 when the field is absent or NULL.
func (as *ActionService) lookupItemUserField(ctx *models.ExecutionContext, column, varName string) int {
	if id := utils.InterfaceToIntPtr(ctx.Variables[varName]); id != nil {
		return *id
	}
	if !repository.IsAllowedItemColumn(column) {
		return 0
	}
	var nid sql.NullInt64
	if err := as.db.QueryRow(`SELECT `+column+` FROM items WHERE id = ?`, ctx.Event.ItemID).Scan(&nid); err != nil || !nid.Valid {
		return 0
	}
	return int(nid.Int64)
}

// executeCondition executes a condition node
func (as *ActionService) executeCondition(node *models.ActionNode, ctx *models.ExecutionContext, stepResult *models.StepResult) error {
	var config models.ConditionNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse condition config: %w", err)
	}

	// Get the field value from context
	fieldValue := ctx.Variables[config.FieldName]
	if fieldValue == nil {
		fieldValue = ctx.Variables["new_"+config.FieldName]
	}

	// Evaluate the condition
	result := as.evaluateCondition(fieldValue, config.Operator, config.Value)

	// Populate step result output with condition details
	stepResult.Output = map[string]interface{}{
		"condition_result": result,
		"field_name":       config.FieldName,
		"field_value":      fieldValue,
		"operator":         config.Operator,
		"compare_value":    config.Value,
	}

	return nil
}

// evaluateCondition evaluates a condition
func (as *ActionService) evaluateCondition(value interface{}, operator, compareValue string) bool {
	strValue := fmt.Sprintf("%v", value)

	switch operator {
	case "eq", "==", "equals":
		return strValue == compareValue
	case "ne", "!=", "not_equals":
		return strValue != compareValue
	case "contains":
		return strings.Contains(strValue, compareValue)
	case "not_contains":
		return !strings.Contains(strValue, compareValue)
	case "starts_with":
		return strings.HasPrefix(strValue, compareValue)
	case "ends_with":
		return strings.HasSuffix(strValue, compareValue)
	case "gt", ">":
		if numVal, err := strconv.ParseFloat(strValue, 64); err == nil {
			if numCompare, err := strconv.ParseFloat(compareValue, 64); err == nil {
				return numVal > numCompare
			}
		}
		return strValue > compareValue
	case "lt", "<":
		if numVal, err := strconv.ParseFloat(strValue, 64); err == nil {
			if numCompare, err := strconv.ParseFloat(compareValue, 64); err == nil {
				return numVal < numCompare
			}
		}
		return strValue < compareValue
	case "gte", ">=":
		if numVal, err := strconv.ParseFloat(strValue, 64); err == nil {
			if numCompare, err := strconv.ParseFloat(compareValue, 64); err == nil {
				return numVal >= numCompare
			}
		}
		return strValue >= compareValue
	case "lte", "<=":
		if numVal, err := strconv.ParseFloat(strValue, 64); err == nil {
			if numCompare, err := strconv.ParseFloat(compareValue, 64); err == nil {
				return numVal <= numCompare
			}
		}
		return strValue <= compareValue
	case "is_empty":
		return strValue == "" || strValue == "null" || strValue == "<nil>"
	case "is_not_empty":
		return strValue != "" && strValue != "null" && strValue != "<nil>"
	default:
		return false
	}
}

// substituteVariables replaces {{variable}} placeholders with actual values
func (as *ActionService) substituteVariables(template string, ctx *models.ExecutionContext) string {
	// Matches double-brace variable placeholders like {{variable_name}}
	re := regexp.MustCompile(`\{\{([^}]+)\}\}`)

	return re.ReplaceAllStringFunc(template, func(match string) string {
		// Extract variable name (remove {{ and }})
		varName := strings.TrimPrefix(strings.TrimSuffix(match, "}}"), "{{")
		varName = strings.TrimSpace(varName)

		// Check different variable sources
		parts := strings.Split(varName, ".")
		if len(parts) == 2 {
			switch parts[0] {
			case "item":
				if val, ok := ctx.Variables["new_"+parts[1]]; ok {
					return fmt.Sprintf("%v", val)
				}
			case "trigger":
				if val, ok := ctx.Variables[parts[1]]; ok {
					return fmt.Sprintf("%v", val)
				}
			case "old":
				if val, ok := ctx.Variables["old_"+parts[1]]; ok {
					return fmt.Sprintf("%v", val)
				}
			case "user":
				if ctx.Actor != nil {
					switch parts[1] {
					case "name":
						return ctx.Actor.FirstName + " " + ctx.Actor.LastName
					case "email":
						return ctx.Actor.Email
					case "id":
						return strconv.Itoa(ctx.Actor.ID)
					}
				}
			}
		}

		// Direct variable lookup
		if val, ok := ctx.Variables[varName]; ok {
			return fmt.Sprintf("%v", val)
		}

		// Return original if not found
		return match
	})
}

// cleanupActionContainers stops any containers started by container_run
// nodes during this action's execution. ContainerService keeps an internal
// registry, so a double-stop (auto-teardown racing with this cleanup) is
// harmless — StopContainer becomes a no-op for unknown IDs.
func (as *ActionService) cleanupActionContainers(results []models.StepResult) {
	if as.containerService == nil {
		return
	}
	for _, r := range results {
		if r.NodeType != models.ActionNodeContainerRun {
			continue
		}
		cid, ok := r.Output["container_id"].(string)
		if !ok || cid == "" {
			continue
		}
		if err := as.containerService.StopContainer(cid); err != nil {
			slog.Debug("failed to stop container during action cleanup",
				slog.String("component", "actions"),
				slog.String("container_id", cid),
				slog.Any("error", err),
			)
		}
	}
}

// authorizeAssetMutation enforces asset-set RBAC before a create_asset /
// update_asset node writes. The actor is the user whose action emitted the
// event; an actor of 0 (no user context) is denied because we cannot prove
// authority. If no permission checker is wired, the check is refused closed
// rather than silently skipped.
func (as *ActionService) authorizeAssetMutation(actorUserID, setID int, permissionKey string) error {
	if actorUserID <= 0 {
		return fmt.Errorf("asset mutation requires an identified actor (set %d)", setID)
	}
	if as.assetPermChecker == nil {
		return fmt.Errorf("asset mutation blocked: asset permission checker not configured")
	}
	ok, err := as.assetPermChecker.HasAssetSetPermission(actorUserID, setID, permissionKey)
	if err != nil {
		return fmt.Errorf("failed to check asset set %d permission: %w", setID, err)
	}
	if !ok {
		return fmt.Errorf("user %d not authorized (%s) on asset set %d", actorUserID, permissionKey, setID)
	}
	return nil
}

// getStatusName retrieves a status name by its ID
func (as *ActionService) getStatusName(statusID int) string {
	var name string
	err := as.db.QueryRow(`SELECT name FROM statuses WHERE id = ?`, statusID).Scan(&name)
	if err != nil {
		return fmt.Sprintf("Status #%d", statusID)
	}
	return name
}

// executeUpdateAsset executes an update_asset node
func (as *ActionService) executeUpdateAsset(node *models.ActionNode, ctx *models.ExecutionContext, stepResult *models.StepResult) error {
	var config models.UpdateAssetNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse update_asset config: %w", err)
	}

	// Skip if no field mappings configured
	if len(config.FieldMappings) == 0 {
		stepResult.Output = map[string]interface{}{
			"skipped": true,
			"reason":  "no field mappings configured",
		}
		return nil
	}

	// Get the item's custom_field_values to find the asset reference
	var customFieldValuesJSON sql.NullString
	err := as.db.QueryRow(`SELECT custom_field_values FROM items WHERE id = ?`, ctx.Event.ItemID).Scan(&customFieldValuesJSON)
	if err != nil {
		return fmt.Errorf("failed to get item custom_field_values: %w", err)
	}

	var customFieldValues map[string]interface{}
	if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
		if err = json.Unmarshal([]byte(customFieldValuesJSON.String), &customFieldValues); err != nil {
			return fmt.Errorf("failed to parse item custom_field_values: %w", err)
		}
	}

	// Extract asset ID from source field
	assetFieldValue, exists := customFieldValues[config.SourceFieldID]
	if !exists || assetFieldValue == nil {
		stepResult.Output = map[string]interface{}{
			"skipped": true,
			"reason":  "no asset linked in source field",
		}
		return nil
	}

	// Handle both integer and object formats for asset field value
	var assetID int
	switch v := assetFieldValue.(type) {
	case float64:
		assetID = int(v)
	case int:
		assetID = v
	case map[string]interface{}:
		// Object format: { "id": 123, ... }
		if idVal, ok := v["id"]; ok {
			switch id := idVal.(type) {
			case float64:
				assetID = int(id)
			case int:
				assetID = id
			}
		}
	}

	if assetID == 0 {
		stepResult.Output = map[string]interface{}{
			"skipped": true,
			"reason":  "invalid asset reference format",
		}
		return nil
	}

	// Get the asset and validate it exists with expected type/set
	var asset struct {
		ID                int
		SetID             int
		AssetTypeID       int
		CustomFieldValues sql.NullString
	}
	err = as.db.QueryRow(`
		SELECT id, set_id, asset_type_id, custom_field_values
		FROM assets WHERE id = ?
	`, assetID).Scan(&asset.ID, &asset.SetID, &asset.AssetTypeID, &asset.CustomFieldValues)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("asset not found: %d", assetID)
		}
		return fmt.Errorf("failed to get asset: %w", err)
	}

	// Validate asset type if specified
	if config.AssetTypeID > 0 && asset.AssetTypeID != config.AssetTypeID {
		return fmt.Errorf("asset type mismatch: expected %d, got %d", config.AssetTypeID, asset.AssetTypeID)
	}

	// Validate asset set if specified
	if config.AssetSetID > 0 && asset.SetID != config.AssetSetID {
		return fmt.Errorf("asset set mismatch: expected %d, got %d", config.AssetSetID, asset.SetID)
	}

	// Authorize the actor against the resolved set's RBAC. Without this, a
	// workspace admin's action could mutate assets in a set where the actor
	// has no role at all.
	if err := as.authorizeAssetMutation(ctx.Event.ActorUserID, asset.SetID, "asset.edit"); err != nil {
		return err
	}

	// Parse existing asset custom_field_values
	var assetCustomFields map[string]interface{}
	if asset.CustomFieldValues.Valid && asset.CustomFieldValues.String != "" {
		if err = json.Unmarshal([]byte(asset.CustomFieldValues.String), &assetCustomFields); err != nil {
			assetCustomFields = make(map[string]interface{})
		}
	} else {
		assetCustomFields = make(map[string]interface{})
	}

	// Track old values for logging
	oldValues := make(map[string]interface{})
	newValues := make(map[string]interface{})

	// Apply field mappings
	for _, mapping := range config.FieldMappings {
		var sourceValue interface{}

		switch mapping.SourceType {
		case "variable":
			// Substitute variables in the template
			sourceValue = as.substituteVariables(mapping.SourceValue, ctx)
		case "item_field":
			// Get value from item's custom fields or context
			if val, ok := ctx.Variables["new_"+mapping.SourceValue]; ok {
				sourceValue = val
			} else if val, ok := customFieldValues[mapping.SourceValue]; ok {
				sourceValue = val
			}
		case "literal":
			sourceValue = mapping.SourceValue
		default:
			// Default to variable substitution
			sourceValue = as.substituteVariables(mapping.SourceValue, ctx)
		}

		// Track changes
		oldValues[mapping.TargetFieldID] = assetCustomFields[mapping.TargetFieldID]
		newValues[mapping.TargetFieldID] = sourceValue

		// Update the asset field
		assetCustomFields[mapping.TargetFieldID] = sourceValue
	}

	// Serialize updated custom_field_values
	updatedJSON, err := json.Marshal(assetCustomFields)
	if err != nil {
		return fmt.Errorf("failed to serialize asset custom_field_values: %w", err)
	}

	// Update the asset
	_, err = as.db.Exec(`
		UPDATE assets SET custom_field_values = ?, updated_at = ? WHERE id = ?
	`, string(updatedJSON), time.Now(), assetID)
	if err != nil {
		return fmt.Errorf("failed to update asset: %w", err)
	}

	// Populate step result output
	stepResult.Output = map[string]interface{}{
		"asset_id":      assetID,
		"old_values":    oldValues,
		"new_values":    newValues,
		"mapping_count": len(config.FieldMappings),
	}

	slog.Debug("updated asset via action",
		slog.String("component", "actions"),
		slog.Int("asset_id", assetID),
		slog.Int("item_id", ctx.Event.ItemID),
		slog.Any("mappings", len(config.FieldMappings)),
	)

	// Emit asset action event for cross-application cascade
	if as.assetActionService != nil {
		as.assetActionService.EmitAssetActionEvent(&models.AssetActionEvent{
			EventType:         models.AssetTriggerAssetUpdated,
			SetID:             asset.SetID,
			AssetID:           assetID,
			ActorUserID:       ctx.Event.ActorUserID,
			OldValues:         oldValues,
			NewValues:         newValues,
			TriggeredByAction: true,
			ExecutionChainID:  ctx.ChainID,
			CascadeDepth:      ctx.Event.CascadeDepth + 1,
			SourceApplication: "workspace",
		})
	}

	return nil
}

// executeCreateAsset executes a create_asset node
func (as *ActionService) executeCreateAsset(node *models.ActionNode, ctx *models.ExecutionContext, stepResult *models.StepResult) error {
	var config models.CreateAssetNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse create_asset config: %w", err)
	}

	// Validate required fields
	if config.AssetSetID == 0 {
		return fmt.Errorf("asset_set_id is required")
	}
	if config.AssetTypeID == 0 {
		return fmt.Errorf("asset_type_id is required")
	}

	// Authorize the actor against the target set before we create anything.
	if err := as.authorizeAssetMutation(ctx.Event.ActorUserID, config.AssetSetID, "asset.create"); err != nil {
		return err
	}

	// Substitute variables in title, description, and asset_tag
	title := as.substituteVariables(config.Title, ctx)
	if title == "" {
		return fmt.Errorf("title is required and cannot be empty after substitution")
	}
	description := as.substituteVariables(config.Description, ctx)
	assetTag := as.substituteVariables(config.AssetTag, ctx)

	// Get item's custom field values for field mapping
	var customFieldValuesJSON sql.NullString
	err := as.db.QueryRow(`SELECT custom_field_values FROM items WHERE id = ?`, ctx.Event.ItemID).Scan(&customFieldValuesJSON)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get item custom_field_values: %w", err)
	}

	var itemCustomFields map[string]interface{}
	if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
		if err = json.Unmarshal([]byte(customFieldValuesJSON.String), &itemCustomFields); err != nil {
			itemCustomFields = make(map[string]interface{})
		}
	} else {
		itemCustomFields = make(map[string]interface{})
	}

	// Build custom_field_values from field mappings
	assetCustomFields := make(map[string]interface{})
	for _, mapping := range config.FieldMappings {
		var sourceValue interface{}

		switch mapping.SourceType {
		case "variable":
			sourceValue = as.substituteVariables(mapping.SourceValue, ctx)
		case "item_field":
			if val, ok := ctx.Variables["new_"+mapping.SourceValue]; ok {
				sourceValue = val
			} else if val, ok := itemCustomFields[mapping.SourceValue]; ok {
				sourceValue = val
			}
		case "literal":
			sourceValue = mapping.SourceValue
		default:
			sourceValue = as.substituteVariables(mapping.SourceValue, ctx)
		}

		assetCustomFields[mapping.TargetFieldID] = sourceValue
	}

	// Serialize custom_field_values
	customFieldsJSON, err := json.Marshal(assetCustomFields)
	if err != nil {
		return fmt.Errorf("failed to serialize custom_field_values: %w", err)
	}

	// Determine status_id - use config value or get default from asset set
	var statusID int
	if config.StatusID != nil {
		statusID = *config.StatusID
	} else {
		// Get default status from asset set
		err = as.db.QueryRow(`SELECT default_status_id FROM asset_sets WHERE id = ?`, config.AssetSetID).Scan(&statusID)
		if err != nil {
			slog.Warn("failed to get default status for asset set, using 0",
				slog.String("component", "actions"),
				slog.Int("asset_set_id", config.AssetSetID),
				slog.Any("error", err),
			)
			statusID = 0
		}
	}

	// Insert the new asset
	now := time.Now()
	var assetID int64
	err = as.db.QueryRow(`
		INSERT INTO assets (set_id, asset_type_id, title, description, asset_tag, category_id, status_id, custom_field_values, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, config.AssetSetID, config.AssetTypeID, title, description, assetTag, config.CategoryID, statusID, string(customFieldsJSON), now, now).Scan(&assetID)
	if err != nil {
		return fmt.Errorf("failed to create asset: %w", err)
	}

	// Populate step result output
	stepResult.Output = map[string]interface{}{
		"asset_id":      assetID,
		"title":         title,
		"description":   description,
		"asset_tag":     assetTag,
		"asset_set_id":  config.AssetSetID,
		"asset_type_id": config.AssetTypeID,
		"mapping_count": len(config.FieldMappings),
	}

	slog.Debug("created asset via action",
		slog.String("component", "actions"),
		slog.Int64("asset_id", assetID),
		slog.Int("item_id", ctx.Event.ItemID),
		slog.String("title", title),
	)

	// Emit asset action event for cross-application cascade
	if as.assetActionService != nil {
		as.assetActionService.EmitAssetActionEvent(&models.AssetActionEvent{
			EventType:         models.AssetTriggerAssetCreated,
			SetID:             config.AssetSetID,
			AssetID:           int(assetID),
			ActorUserID:       ctx.Event.ActorUserID,
			TriggeredByAction: true,
			ExecutionChainID:  ctx.ChainID,
			CascadeDepth:      ctx.Event.CascadeDepth + 1,
			SourceApplication: "workspace",
		})
	}

	return nil
}

// executeRoundRobinAssign executes a round_robin_assign node
func (as *ActionService) executeRoundRobinAssign(node *models.ActionNode, ctx *models.ExecutionContext, stepResult *models.StepResult) error {
	if as.teamService == nil {
		return fmt.Errorf("team service not configured")
	}

	var config models.RoundRobinAssignNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse round_robin_assign config: %w", err)
	}

	if config.TeamID == 0 {
		return fmt.Errorf("team_id is required for round_robin_assign")
	}

	// Get current assignee for event emission
	var oldAssigneeID sql.NullInt64
	_ = as.db.QueryRow(`SELECT assignee_id FROM items WHERE id = ?`, ctx.Event.ItemID).Scan(&oldAssigneeID)

	// Get next assignee via round-robin
	assigneeID, err := as.teamService.GetNextRoundRobinAssignee(node.ID, config.TeamID, config.SkipOnLeaveMembers, config.UseLeaveSubstitutes)
	if err != nil {
		return fmt.Errorf("failed to get round-robin assignee: %w", err)
	}

	// Update the item's assignee
	tx, txErr := as.db.BeginTx(context.Background(), nil)
	if txErr != nil {
		return fmt.Errorf("begin tx: %w", txErr)
	}
	defer func() { _ = tx.Rollback() }()
	if txErr = as.itemRepo.UpdateFields(tx, ctx.Event.ItemID, map[string]interface{}{
		"assignee_id": assigneeID,
	}); txErr != nil {
		return fmt.Errorf("failed to update item assignee: %w", txErr)
	}
	if txErr = tx.Commit(); txErr != nil {
		return fmt.Errorf("commit: %w", txErr)
	}

	// Populate step result
	var oldVal interface{}
	if oldAssigneeID.Valid {
		oldVal = int(oldAssigneeID.Int64)
	}
	stepResult.Output = map[string]interface{}{
		"field_name":  "assignee_id",
		"old_value":   oldVal,
		"new_value":   assigneeID,
		"team_id":     config.TeamID,
		"action_node": node.ID,
	}

	// Emit cascade event
	as.EmitActionEvent(&models.ActionEvent{
		EventType:         models.ActionTriggerItemUpdated,
		WorkspaceID:       ctx.Event.WorkspaceID,
		ItemID:            ctx.Event.ItemID,
		ActorUserID:       ctx.Event.ActorUserID,
		OldValues:         map[string]interface{}{"assignee_id": oldVal},
		NewValues:         map[string]interface{}{"assignee_id": assigneeID},
		TriggeredByAction: true,
		ExecutionChainID:  ctx.ChainID,
		CascadeDepth:      ctx.Event.CascadeDepth + 1,
	})

	return nil
}

// GetStats returns service statistics
func (as *ActionService) GetStats() map[string]int64 {
	return map[string]int64{
		"events_processed": atomic.LoadInt64(&as.eventsProcessed),
		"actions_executed": atomic.LoadInt64(&as.actionsExecuted),
		"errors":           atomic.LoadInt64(&as.errors),
	}
}

// ExecuteActionManually executes a specific action for a given item.
// This bypasses the normal trigger matching and directly executes the action.
func (as *ActionService) ExecuteActionManually(action *models.Action, itemID, actorUserID int) error {
	slog.Debug("executing action manually",
		slog.String("component", "actions"),
		slog.Int("action_id", action.ID),
		slog.String("action_name", action.Name),
		slog.Int("item_id", itemID),
		slog.Int("actor_user_id", actorUserID),
	)

	// Create a manual trigger event
	event := &models.ActionEvent{
		EventType:         models.ActionTriggerManual,
		WorkspaceID:       action.WorkspaceID,
		ItemID:            itemID,
		ActorUserID:       actorUserID,
		OldValues:         map[string]interface{}{},
		NewValues:         map[string]interface{}{},
		TriggeredByAction: false,
		CascadeDepth:      0,
	}

	// Execute the action directly (bypassing the event queue and trigger matching)
	if err := as.executeAction(action, event, nil); err != nil {
		slog.Error("failed to execute action manually",
			slog.String("component", "actions"),
			slog.Int("action_id", action.ID),
			slog.Any("error", err),
		)
		atomic.AddInt64(&as.errors, 1)
		return err
	}

	atomic.AddInt64(&as.actionsExecuted, 1)
	return nil
}

// resolveCapability fetches and validates a capability by ID.
func (as *ActionService) resolveCapability(capabilityID int, expectedType models.CapabilityType) (*models.ActionCapability, error) {
	capability, err := as.repo.GetCapabilityByID(capabilityID)
	if err != nil {
		return nil, fmt.Errorf("capability %d not found: %w", capabilityID, err)
	}
	if !capability.IsEnabled {
		return nil, fmt.Errorf("capability %d (%s) is disabled", capabilityID, capability.Name)
	}
	if capability.CapabilityType != expectedType {
		return nil, fmt.Errorf("capability %d is type %s, expected %s", capabilityID, capability.CapabilityType, expectedType)
	}
	return capability, nil
}

// resolveLLMClient resolves a capability ID to an LLM client.
func (as *ActionService) resolveLLMClient(capabilityID int) (llm.Client, error) {
	if as.llmConnectionManager == nil {
		return nil, fmt.Errorf("LLM connection manager not configured")
	}

	capability, err := as.resolveCapability(capabilityID, models.CapabilityLLMConnection)
	if err != nil {
		return nil, err
	}

	var llmConfig models.LLMConnectionCapabilityConfig
	if err := json.Unmarshal([]byte(capability.Config), &llmConfig); err != nil {
		return nil, fmt.Errorf("failed to parse llm_connection config: %w", err)
	}

	client, err := as.llmConnectionManager.Resolve(llmConfig.ConnectionID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve LLM connection %d: %w", llmConfig.ConnectionID, err)
	}
	return client, nil
}

// executeAIExtract executes an ai_extract node — sandboxed LLM analysis with no tools.
func (as *ActionService) executeAIExtract(node *models.ActionNode, ctx *models.ExecutionContext, stepResult *models.StepResult) error {
	var config models.AIExtractNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse ai_extract config: %w", err)
	}

	// Get the untrusted input from execution context
	inputRaw, ok := ctx.Variables[config.InputField]
	if !ok {
		return fmt.Errorf("input field %q not found in execution context", config.InputField)
	}
	input := fmt.Sprintf("%v", inputRaw)

	// Resolve LLM client
	client, err := as.resolveLLMClient(config.CapabilityID)
	if err != nil {
		return err
	}

	// Run sandboxed analysis (no tools, structured output only)
	result, err := llm.RunSandboxedAnalysis[map[string]interface{}](
		context.Background(),
		client,
		llm.SandboxedAnalysisRequest{
			SystemPrompt: config.Prompt,
			Input:        input,
			OutputSchema: json.RawMessage(config.OutputSchema),
		},
	)
	if err != nil {
		return fmt.Errorf("ai_extract failed: %w", err)
	}

	// Store the extracted struct in execution context
	if config.OutputField != "" && result != nil {
		ctx.Variables[config.OutputField] = *result
	}

	stepResult.Output = map[string]interface{}{
		"extracted": result,
	}

	slog.Debug("ai_extract completed",
		slog.String("component", "actions"),
		slog.Int("node_id", node.ID),
		slog.String("output_field", config.OutputField),
	)

	return nil
}

// executeAIAgent executes an ai_agent node — agentic LLM loop with scoped tools.
func (as *ActionService) executeAIAgent(node *models.ActionNode, ctx *models.ExecutionContext, stepResult *models.StepResult) error {
	var config models.AIAgentNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse ai_agent config: %w", err)
	}

	// Resolve LLM client
	client, err := as.resolveLLMClient(config.CapabilityID)
	if err != nil {
		return err
	}

	// Build user message from input fields
	var inputParts []string
	for _, field := range config.InputFields {
		if val, ok := ctx.Variables[field]; ok {
			valJSON, _ := json.Marshal(val)
			inputParts = append(inputParts, fmt.Sprintf("%s: %s", field, string(valJSON)))
		}
	}
	userMessage := strings.Join(inputParts, "\n\n")

	// Substitute variables in system prompt
	systemPrompt := as.substituteVariables(config.Prompt, ctx)

	// Build tool definitions from referenced capabilities
	var tools []llm.ToolDefinition
	toolExecutor := as.buildAgentToolExecutor(ctx, config.Tools)

	for _, toolCapID := range config.Tools {
		toolDefs := as.buildToolDefinitions(toolCapID, ctx)
		tools = append(tools, toolDefs...)
	}

	maxSteps := config.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 10
	}

	// Run agent loop
	agentResult, err := llm.RunAgent(
		context.Background(),
		client,
		llm.AgentConfig{
			SystemPrompt:  systemPrompt,
			Tools:         tools,
			MaxIterations: maxSteps,
			Timeout:       time.Duration(maxSteps*30) * time.Second,
		},
		userMessage,
		toolExecutor,
		nil,
	)
	if err != nil {
		return fmt.Errorf("ai_agent failed: %w", err)
	}

	// Store the result
	if config.OutputField != "" {
		ctx.Variables[config.OutputField] = agentResult.Answer
	}

	stepResult.Output = map[string]interface{}{
		"answer":     agentResult.Answer,
		"iterations": agentResult.Iterations,
		"tool_calls": len(agentResult.ToolCalls),
	}

	slog.Debug("ai_agent completed",
		slog.String("component", "actions"),
		slog.Int("node_id", node.ID),
		slog.Int("iterations", agentResult.Iterations),
		slog.Int("tool_calls", len(agentResult.ToolCalls)),
	)

	return nil
}

// buildToolDefinitions creates tool definitions for a capability ID string.
func (as *ActionService) buildToolDefinitions(capIDStr string, _ *models.ExecutionContext) []llm.ToolDefinition {
	capID, err := strconv.Atoi(capIDStr)
	if err != nil {
		slog.Warn("invalid capability ID in tools list", slog.String("component", "actions"), slog.String("cap_id", capIDStr))
		return nil
	}

	capability, err := as.repo.GetCapabilityByID(capID)
	if err != nil || !capability.IsEnabled {
		return nil
	}

	switch capability.CapabilityType {
	case models.CapabilityHTTPClient:
		return []llm.ToolDefinition{
			{
				Type: "function",
				Function: llm.FunctionDef{
					Name:        fmt.Sprintf("http_request_%d", capID),
					Description: fmt.Sprintf("Make HTTP requests using the %s capability. Allowed URL patterns are configured by the admin.", capability.Name),
					Parameters: json.RawMessage(`{
						"type": "object",
						"properties": {
							"method": {"type": "string", "enum": ["GET", "POST", "PUT", "DELETE", "PATCH"]},
							"url": {"type": "string"},
							"body": {"type": "string"},
							"headers": {"type": "object", "additionalProperties": {"type": "string"}}
						},
						"required": ["method", "url"]
					}`),
				},
			},
		}
	default:
		return nil
	}
}

// buildAgentToolExecutor creates a tool executor function for the agent loop.
func (as *ActionService) buildAgentToolExecutor(_ *models.ExecutionContext, toolCapIDs []string) llm.ToolExecutorFunc {
	return func(execCtx context.Context, name string, arguments string) (string, error) {
		// Parse the capability ID from the tool name (e.g., "http_request_5")
		if strings.HasPrefix(name, "http_request_") {
			capIDStr := strings.TrimPrefix(name, "http_request_")
			capID, err := strconv.Atoi(capIDStr)
			if err != nil {
				return "", fmt.Errorf("invalid tool name: %s", name)
			}

			// Verify the capability is in the allowed list
			allowed := false
			for _, id := range toolCapIDs {
				if id == capIDStr {
					allowed = true
					break
				}
			}
			if !allowed {
				return "", fmt.Errorf("capability %d not in allowed tools", capID)
			}

			return as.executeAgentHTTPRequest(execCtx, capID, arguments)
		}

		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// executeAgentHTTPRequest executes an HTTP request from within an agent tool call.
func (as *ActionService) executeAgentHTTPRequest(ctx context.Context, capID int, arguments string) (string, error) {
	capability, err := as.resolveCapability(capID, models.CapabilityHTTPClient)
	if err != nil {
		return "", err
	}

	var httpConfig models.HTTPClientConfig
	if err := json.Unmarshal([]byte(capability.Config), &httpConfig); err != nil {
		return "", fmt.Errorf("failed to parse http_client config: %w", err)
	}

	var args struct {
		Method  string            `json:"method"`
		URL     string            `json:"url"`
		Body    string            `json:"body"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Validate URL against allowed patterns
	if !isURLAllowed(args.URL, httpConfig.AllowedURLPatterns) {
		return "", fmt.Errorf("URL %q not allowed by capability %d", args.URL, capID)
	}

	return doHTTPRequest(ctx, args.Method, args.URL, args.Body, args.Headers, httpConfig.DefaultHeaders, httpConfig.TimeoutSecs, httpConfig.AllowedURLPatterns)
}

// executeContainerRun executes a container_run node.
func (as *ActionService) executeContainerRun(node *models.ActionNode, ctx *models.ExecutionContext, stepResult *models.StepResult) error {
	if as.containerService == nil {
		return fmt.Errorf("container service not configured")
	}

	var config models.ContainerRunNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse container_run config: %w", err)
	}

	capability, err := as.resolveCapability(config.CapabilityID, models.CapabilityDockerEnvironment)
	if err != nil {
		return err
	}

	var envConfig models.DockerEnvironmentConfig
	if err := json.Unmarshal([]byte(capability.Config), &envConfig); err != nil {
		return fmt.Errorf("failed to parse docker_environment config: %w", err)
	}

	containerInfo, err := as.containerService.StartContainer(context.Background(), envConfig, config.TimeoutSecs)
	if err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Store container info in execution context
	if config.OutputField != "" {
		ctx.Variables[config.OutputField] = map[string]interface{}{
			"container_id": containerInfo.ContainerID,
			"host":         containerInfo.Host,
			"port":         containerInfo.Port,
		}
	}

	stepResult.Output = map[string]interface{}{
		"container_id": containerInfo.ContainerID,
		"host":         containerInfo.Host,
		"port":         containerInfo.Port,
	}

	slog.Debug("container_run started",
		slog.String("component", "actions"),
		slog.Int("node_id", node.ID),
		slog.String("container_id", containerInfo.ContainerID),
		slog.Int("port", containerInfo.Port),
	)

	return nil
}

// executeHTTPRequest executes an http_request node.
func (as *ActionService) executeHTTPRequest(node *models.ActionNode, ctx *models.ExecutionContext, stepResult *models.StepResult) error {
	var config models.HTTPRequestNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse http_request config: %w", err)
	}

	// Substitute variables in URL, body, and headers
	url := as.substituteVariables(config.URLTemplate, ctx)
	body := as.substituteVariables(config.Body, ctx)
	headers := make(map[string]string)
	for k, v := range config.Headers {
		headers[k] = as.substituteVariables(v, ctx)
	}

	// A capability is required — it carries the URL allowlist, default headers,
	// and timeout. Without one, the request would bypass SSRF controls entirely.
	if config.CapabilityID <= 0 {
		return fmt.Errorf("http_request: capability_id is required")
	}

	capability, err := as.resolveCapability(config.CapabilityID, models.CapabilityHTTPClient)
	if err != nil {
		return err
	}

	var httpConfig models.HTTPClientConfig
	if err := json.Unmarshal([]byte(capability.Config), &httpConfig); err != nil {
		return fmt.Errorf("failed to parse http_client config: %w", err)
	}

	if !isURLAllowed(url, httpConfig.AllowedURLPatterns) {
		return fmt.Errorf("URL %q not allowed by capability %d", url, config.CapabilityID)
	}

	result, err := doHTTPRequest(context.Background(), config.Method, url, body, headers, httpConfig.DefaultHeaders, httpConfig.TimeoutSecs, httpConfig.AllowedURLPatterns)
	if err != nil {
		return fmt.Errorf("http_request failed: %w", err)
	}

	// Store response in execution context
	if config.OutputField != "" {
		ctx.Variables[config.OutputField] = result
	}

	stepResult.Output = map[string]interface{}{
		"response_preview": truncateString(result, 500),
	}

	slog.Debug("http_request completed",
		slog.String("component", "actions"),
		slog.Int("node_id", node.ID),
		slog.String("method", config.Method),
		slog.String("url", url),
	)

	return nil
}

// isURLAllowed checks if a URL matches any of the allowed patterns.
// Patterns support wildcards: * matches any sequence of non-/ characters,
// ** matches any sequence including /.
func isURLAllowed(url string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	for _, pattern := range patterns {
		if matchURLPattern(url, pattern) {
			return true
		}
	}
	return false
}

// matchURLPattern matches a URL against a pattern with * and ** wildcards.
func matchURLPattern(url, pattern string) bool {
	// Convert pattern to regex
	regexStr := "^"
	for i := 0; i < len(pattern); i++ {
		switch {
		case i+1 < len(pattern) && pattern[i] == '*' && pattern[i+1] == '*':
			regexStr += ".*"
			i++ // skip second *
		case pattern[i] == '*':
			regexStr += "[^/]*"
		default:
			regexStr += regexp.QuoteMeta(string(pattern[i]))
		}
	}
	regexStr += "$"

	matched, err := regexp.MatchString(regexStr, url)
	if err != nil {
		return false
	}
	return matched
}

// doHTTPRequest performs an HTTP request with the given parameters.
// allowedPatterns is the URL allowlist from the caller's capability — it is
// re-checked on every redirect hop to prevent a compliant initial URL from
// bouncing to an arbitrary target. A scoped http.Client with a dialer that
// rejects loopback / private / link-local addresses also defends against DNS
// rebinding to internal services (169.254.169.254, 127.0.0.1, etc.).
func doHTTPRequest(ctx context.Context, method, url, body string, headers, defaultHeaders map[string]string, timeoutSecs int, allowedPatterns []string) (string, error) {
	if timeoutSecs <= 0 {
		timeoutSecs = 30
	}

	httpCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(httpCtx, method, url, bodyReader)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Apply default headers first, then override with specific headers
	for k, v := range defaultHeaders {
		req.Header.Set(k, v)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := newSSRFSafeClient(time.Duration(timeoutSecs)*time.Second, allowedPatterns)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	result := map[string]interface{}{
		"status_code": resp.StatusCode,
		"body":        string(respBody),
	}
	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

// errDisallowedRedirect is returned when a redirect targets a URL outside the allowlist.
var errDisallowedRedirect = errors.New("redirect URL not in allowlist")

// newSSRFSafeClient returns an http.Client configured for server-side requests
// to admin-allowed URLs: it enforces the allowlist on every redirect and blocks
// dials to loopback/private/link-local addresses so a DNS record that resolves
// to 127.0.0.1 or 169.254.169.254 cannot be used to pivot internally.
func newSSRFSafeClient(timeout time.Duration, allowedPatterns []string) *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		ControlContext: func(_ context.Context, network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("invalid dial address %q: %w", address, err)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("dial host %q did not resolve to an IP", host)
			}
			if isBlockedIP(ip) {
				return fmt.Errorf("dial to %s on %s blocked: non-public address", ip.String(), network)
			}
			return nil
		},
	}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     true,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("stopped after 5 redirects")
			}
			if !isURLAllowed(req.URL.String(), allowedPatterns) {
				return fmt.Errorf("%w: %s", errDisallowedRedirect, req.URL.String())
			}
			return nil
		},
	}
}

// isBlockedIP reports whether an IP is on a network we never want server-side
// automation to reach: loopback, unspecified, link-local (including cloud
// metadata services at 169.254.169.254), RFC1918 private ranges, carrier-grade
// NAT, and IPv6 ULA / link-local.
func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsPrivate() {
		return true
	}
	// Carrier-grade NAT range 100.64.0.0/10 is not caught by IsPrivate.
	if v4 := ip.To4(); v4 != nil {
		_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
		if cgnat.Contains(v4) {
			return true
		}
	}
	return false
}

// truncateString truncates a string to n characters.
func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
