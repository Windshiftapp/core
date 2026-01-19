package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

// ExecutionChain tracks state for cycle detection during action cascades.
// The chain is stored in memory and keyed by ExecutionChainID.
type ExecutionChain struct {
	ExecutedActions map[int]bool // Set of action IDs already executed in this chain
	CreatedAt       time.Time    // For TTL cleanup
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
	db     database.Database
	repo   *repository.ActionRepository
	config ActionServiceConfig

	// Action cache: workspace_id -> enabled actions
	actionCache map[int][]*models.Action
	cacheMu     sync.RWMutex

	// Event processing
	eventChan chan *models.ActionEvent
	stopChan  chan struct{}
	wg        sync.WaitGroup

	// Dependencies for action execution
	notificationService *NotificationService

	// Execution chain cache for cascade loop prevention
	// Maps ExecutionChainID -> *ExecutionChain
	chainCache sync.Map

	// Statistics
	eventsProcessed int64
	actionsExecuted int64
	errors          int64
}

// NewActionService creates a new action service
func NewActionService(db database.Database, config ActionServiceConfig) *ActionService {
	service := &ActionService{
		db:          db,
		repo:        repository.NewActionRepository(db),
		config:      config,
		actionCache: make(map[int][]*models.Action),
		eventChan:   make(chan *models.ActionEvent, config.EventBufferSize),
		stopChan:    make(chan struct{}),
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
	as.wg.Wait()
	slog.Debug("action service stopped", slog.String("component", "actions"))
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
		SELECT DISTINCT workspace_id FROM actions WHERE is_enabled = 1
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

	// Load enabled actions for each workspace
	for _, workspaceID := range workspaceIDs {
		actions, err := as.repo.ListEnabledByWorkspace(workspaceID)
		if err != nil {
			slog.Error("failed to load actions for workspace",
				slog.String("component", "actions"),
				slog.Int("workspace_id", workspaceID),
				slog.Any("error", err),
			)
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

// getChain retrieves an execution chain from cache by its ID.
// Returns nil if the chain doesn't exist.
func (as *ActionService) getChain(chainID string) *ExecutionChain {
	if chainID == "" {
		return nil
	}
	if chain, ok := as.chainCache.Load(chainID); ok {
		return chain.(*ExecutionChain)
	}
	return nil
}

// createChain creates a new execution chain and stores it in the cache.
// Returns the newly created chain.
func (as *ActionService) createChain(chainID string) *ExecutionChain {
	chain := &ExecutionChain{
		ExecutedActions: make(map[int]bool),
		CreatedAt:       time.Now(),
	}
	as.chainCache.Store(chainID, chain)
	return chain
}

// cleanupChains removes stale execution chains older than 5 minutes.
// This is called periodically from the cache refresher.
func (as *ActionService) cleanupChains() {
	threshold := time.Now().Add(-5 * time.Minute)
	cleaned := 0
	as.chainCache.Range(func(key, value interface{}) bool {
		chain := value.(*ExecutionChain)
		if chain.CreatedAt.Before(threshold) {
			as.chainCache.Delete(key)
			cleaned++
		}
		return true
	})
	if cleaned > 0 {
		slog.Debug("cleaned up stale execution chains",
			slog.String("component", "actions"),
			slog.Int("count", cleaned),
		)
	}
}

// MaxCascadeDepth is the maximum depth of nested action triggers (safety limit)
const MaxCascadeDepth = 5

// processEvent processes a single action event
func (as *ActionService) processEvent(event *models.ActionEvent) error {
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
		if chain != nil && chain.ExecutedActions[action.ID] {
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
		// Check item_type_id filter
		if config.ItemTypeID != nil {
			itemTypeID, ok := event.NewValues["item_type_id"].(int)
			if !ok || itemTypeID != *config.ItemTypeID {
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
		// Check link_type_id filter
		if config.LinkTypeID != nil {
			linkTypeID, ok := event.NewValues["link_type_id"].(int)
			if !ok || linkTypeID != *config.LinkTypeID {
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
	chain.ExecutedActions[action.ID] = true

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
		as.repo.UpdateExecutionLog(log)
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

		err := as.executeNode(&node, ctx)
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

	as.repo.UpdateExecutionLog(log)

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
func (as *ActionService) executeNode(node *models.ActionNode, ctx *models.ExecutionContext) error {
	switch node.NodeType {
	case models.ActionNodeSetField:
		return as.executeSetField(node, ctx)
	case models.ActionNodeSetStatus:
		return as.executeSetStatus(node, ctx)
	case models.ActionNodeAddComment:
		return as.executeAddComment(node, ctx)
	case models.ActionNodeNotifyUser:
		return as.executeNotifyUser(node, ctx)
	case models.ActionNodeCondition:
		return as.executeCondition(node, ctx)
	default:
		return fmt.Errorf("unknown node type: %s", node.NodeType)
	}
}

// executeSetField executes a set_field node
func (as *ActionService) executeSetField(node *models.ActionNode, ctx *models.ExecutionContext) error {
	var config models.SetFieldNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse set_field config: %w", err)
	}

	// Substitute variables in value
	value := as.substituteVariables(config.Value, ctx)

	// Get current field value for event emission (best effort)
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

	// Update the item's field
	_, err := as.db.Exec(`
		UPDATE items SET `+config.FieldName+` = ?, updated_at = ? WHERE id = ?
	`, value, time.Now(), ctx.Event.ItemID)
	if err != nil {
		return err
	}

	// Emit chained event for potential cascade actions
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

// executeSetStatus executes a set_status node
func (as *ActionService) executeSetStatus(node *models.ActionNode, ctx *models.ExecutionContext) error {
	var config models.SetStatusNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse set_status config: %w", err)
	}

	// Get current status for event emission
	var oldStatusID int
	err := as.db.QueryRow(`SELECT status_id FROM items WHERE id = ?`, ctx.Event.ItemID).Scan(&oldStatusID)
	if err != nil {
		slog.Warn("failed to get current status for cascade event",
			slog.String("component", "actions"),
			slog.Int("item_id", ctx.Event.ItemID),
			slog.Any("error", err),
		)
	}

	// Update the status
	_, err = as.db.Exec(`
		UPDATE items SET status_id = ?, updated_at = ? WHERE id = ?
	`, config.StatusID, time.Now(), ctx.Event.ItemID)
	if err != nil {
		return err
	}

	// Only emit cascade event if status actually changed
	if oldStatusID != config.StatusID {
		// Emit chained event for potential cascade actions
		as.EmitActionEvent(&models.ActionEvent{
			EventType:         models.ActionTriggerStatusTransition,
			WorkspaceID:       ctx.Event.WorkspaceID,
			ItemID:            ctx.Event.ItemID,
			ActorUserID:       ctx.Event.ActorUserID,
			OldValues:         map[string]interface{}{"status_id": oldStatusID},
			NewValues:         map[string]interface{}{"status_id": config.StatusID},
			TriggeredByAction: true,
			ExecutionChainID:  ctx.ChainID,
			CascadeDepth:      ctx.Event.CascadeDepth + 1,
		})
	}

	return nil
}

// executeAddComment executes an add_comment node
func (as *ActionService) executeAddComment(node *models.ActionNode, ctx *models.ExecutionContext) error {
	var config models.AddCommentNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse add_comment config: %w", err)
	}

	// Substitute variables in content
	content := as.substituteVariables(config.Content, ctx)

	_, err := as.db.Exec(`
		INSERT INTO comments (item_id, user_id, content, is_private, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, ctx.Event.ItemID, ctx.Event.ActorUserID, content, config.IsPrivate, time.Now())

	return err
}

// executeNotifyUser executes a notify_user node
func (as *ActionService) executeNotifyUser(node *models.ActionNode, ctx *models.ExecutionContext) error {
	if as.notificationService == nil {
		slog.Warn("notification service not configured, skipping notify_user",
			slog.String("component", "actions"),
		)
		return nil
	}

	var config models.NotifyUserNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse notify_user config: %w", err)
	}

	// Determine recipient user IDs
	userIDs := []int{}
	for _, recipient := range config.Recipients {
		switch recipient {
		case "assignee":
			if assigneeID, ok := ctx.Variables["new_assignee_id"].(int); ok {
				userIDs = append(userIDs, assigneeID)
			}
		case "creator":
			if creatorID, ok := ctx.Variables["new_creator_id"].(int); ok {
				userIDs = append(userIDs, creatorID)
			}
		default:
			// Try to parse as user ID
			if id, err := strconv.Atoi(recipient); err == nil {
				userIDs = append(userIDs, id)
			}
		}
	}

	// Substitute variables in message
	message := as.substituteVariables(config.Message, ctx)
	title := as.substituteVariables(config.Title, ctx)

	// Create notifications for each user
	// Note: Currently the notification service determines recipients based on rules,
	// so we emit one event per intended recipient to trigger notification dispatch
	for range userIDs {
		as.notificationService.EmitEvent(&NotificationEvent{
			EventType:   "action.notification",
			WorkspaceID: ctx.Event.WorkspaceID,
			ActorUserID: ctx.Event.ActorUserID,
			ItemID:      ctx.Event.ItemID,
			Title:       title,
			TemplateData: map[string]interface{}{
				"message": message,
			},
		})
	}

	return nil
}

// executeCondition executes a condition node
func (as *ActionService) executeCondition(node *models.ActionNode, ctx *models.ExecutionContext) error {
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

	// Store result for edge evaluation
	for i, stepResult := range ctx.StepResults {
		if stepResult.NodeID == node.ID {
			if ctx.StepResults[i].Output == nil {
				ctx.StepResults[i].Output = make(map[string]interface{})
			}
			ctx.StepResults[i].Output["condition_result"] = result
			break
		}
	}

	// Also add to current step result
	if len(ctx.StepResults) > 0 {
		lastIdx := len(ctx.StepResults) - 1
		if ctx.StepResults[lastIdx].NodeID == node.ID {
			if ctx.StepResults[lastIdx].Output == nil {
				ctx.StepResults[lastIdx].Output = make(map[string]interface{})
			}
			ctx.StepResults[lastIdx].Output["condition_result"] = result
		}
	}

	return nil
}

// evaluateCondition evaluates a condition
func (as *ActionService) evaluateCondition(value interface{}, operator string, compareValue string) bool {
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
	// Pattern: {{variable_name}}
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

// GetStats returns service statistics
func (as *ActionService) GetStats() map[string]int64 {
	return map[string]int64{
		"events_processed": atomic.LoadInt64(&as.eventsProcessed),
		"actions_executed": atomic.LoadInt64(&as.actionsExecuted),
		"errors":           atomic.LoadInt64(&as.errors),
	}
}
