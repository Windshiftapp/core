package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"

	"github.com/google/uuid"
)

// AssetActionService handles asynchronous asset action execution
type AssetActionService struct {
	db     database.Database
	repo   *repository.AssetActionRepository
	config ActionServiceConfig

	// Action cache: set_id -> enabled actions
	actionCache map[int][]*models.AssetAction
	cacheMu     sync.RWMutex

	// Event processing
	eventChan chan *models.AssetActionEvent
	stopChan  chan struct{}
	wg        sync.WaitGroup

	// Dependencies
	notificationService *NotificationService
	eventCoordinator    *EventCoordinator
	chainStore          *ExecutionChainStore

	// Statistics
	eventsProcessed int64
	actionsExecuted int64
	errors          int64
}

// NewAssetActionService creates a new asset action service
func NewAssetActionService(db database.Database, config ActionServiceConfig, chainStore *ExecutionChainStore) *AssetActionService {
	if chainStore == nil {
		chainStore = NewExecutionChainStore()
	}
	service := &AssetActionService{
		db:          db,
		repo:        repository.NewAssetActionRepository(db),
		config:      config,
		actionCache: make(map[int][]*models.AssetAction),
		eventChan:   make(chan *models.AssetActionEvent, config.EventBufferSize),
		stopChan:    make(chan struct{}),
		chainStore:  chainStore,
	}

	// Load initial cache
	if err := service.refreshActionCache(); err != nil {
		slog.Warn("failed to load initial asset action cache", slog.String("component", "asset-actions"), slog.Any("error", err))
	}

	// Start background workers
	service.wg.Add(2)
	go service.eventProcessor()
	go service.cacheRefresher()

	slog.Debug("asset action service initialized", slog.String("component", "asset-actions"))

	return service
}

// SetNotificationService sets the notification service for notify_user actions
func (as *AssetActionService) SetNotificationService(ns *NotificationService) {
	as.notificationService = ns
}

// SetEventCoordinator sets the event coordinator for emitting item events
func (as *AssetActionService) SetEventCoordinator(ec *EventCoordinator) {
	as.eventCoordinator = ec
}

// EmitAssetActionEvent sends an event to be processed asynchronously (non-blocking)
func (as *AssetActionService) EmitAssetActionEvent(event *models.AssetActionEvent) {
	slog.Debug("queuing asset action event",
		slog.String("component", "asset-actions"),
		slog.String("event_type", string(event.EventType)),
		slog.Int("set_id", event.SetID),
		slog.Int("asset_id", event.AssetID),
	)

	select {
	case as.eventChan <- event:
	default:
		slog.Warn("asset action event channel full, dropping event",
			slog.String("component", "asset-actions"),
			slog.String("event_type", string(event.EventType)),
			slog.Int("set_id", event.SetID),
		)
		atomic.AddInt64(&as.errors, 1)
	}
}

// InvalidateSetCache invalidates the cache for a specific asset set
func (as *AssetActionService) InvalidateSetCache(setID int) {
	actions, err := as.repo.ListEnabledBySet(setID)
	if err != nil {
		slog.Error("failed to reload actions for asset set",
			slog.String("component", "asset-actions"),
			slog.Int("set_id", setID),
			slog.Any("error", err),
		)
		return
	}

	as.cacheMu.Lock()
	if len(actions) > 0 {
		as.actionCache[setID] = actions
	} else {
		delete(as.actionCache, setID)
	}
	as.cacheMu.Unlock()
}

// Stop gracefully shuts down the asset action service
func (as *AssetActionService) Stop() {
	close(as.stopChan)

	done := make(chan struct{})
	go func() {
		as.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Debug("asset action service stopped successfully", slog.String("component", "asset-actions"))
	case <-time.After(3 * time.Second):
		slog.Warn("asset action service stop timed out after 3s", slog.String("component", "asset-actions"))
	}
}

func (as *AssetActionService) eventProcessor() {
	defer as.wg.Done()

	for {
		select {
		case event := <-as.eventChan:
			if err := as.processEvent(event); err != nil {
				slog.Error("failed to process asset action event",
					slog.String("component", "asset-actions"),
					slog.String("event_type", string(event.EventType)),
					slog.Any("error", err),
				)
				atomic.AddInt64(&as.errors, 1)
			} else {
				atomic.AddInt64(&as.eventsProcessed, 1)
			}
		case <-as.stopChan:
			slog.Debug("stopping asset action event processor", slog.String("component", "asset-actions"))
			for len(as.eventChan) > 0 {
				event := <-as.eventChan
				if err := as.processEvent(event); err != nil {
					slog.Error("failed to process asset action event during shutdown",
						slog.String("component", "asset-actions"),
						slog.Any("error", err),
					)
				}
			}
			return
		}
	}
}

func (as *AssetActionService) cacheRefresher() {
	defer as.wg.Done()

	ticker := time.NewTicker(as.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := as.refreshActionCache(); err != nil {
				slog.Error("failed to refresh asset action cache", slog.String("component", "asset-actions"), slog.Any("error", err))
			}
			as.chainStore.Cleanup()
		case <-as.stopChan:
			return
		}
	}
}

func (as *AssetActionService) refreshActionCache() error {
	rows, err := as.db.Query(`SELECT DISTINCT set_id FROM asset_actions WHERE is_enabled = true`)
	if err != nil {
		return fmt.Errorf("failed to query sets with asset actions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	newCache := make(map[int][]*models.AssetAction)
	var setIDs []int

	for rows.Next() {
		var setID int
		if err := rows.Scan(&setID); err != nil {
			continue
		}
		setIDs = append(setIDs, setID)
	}

	for _, setID := range setIDs {
		actions, err := as.repo.ListEnabledBySet(setID)
		if err != nil {
			slog.Error("failed to load asset actions for set",
				slog.String("component", "asset-actions"),
				slog.Int("set_id", setID),
				slog.Any("error", err),
			)
			continue
		}
		newCache[setID] = actions
	}

	as.cacheMu.Lock()
	as.actionCache = newCache
	as.cacheMu.Unlock()

	return nil
}

func (as *AssetActionService) processEvent(event *models.AssetActionEvent) error {
	slog.Debug("processing asset action event",
		slog.String("component", "asset-actions"),
		slog.String("event_type", string(event.EventType)),
		slog.Int("set_id", event.SetID),
		slog.Int("asset_id", event.AssetID),
		slog.Bool("triggered_by_action", event.TriggeredByAction),
		slog.Int("cascade_depth", event.CascadeDepth),
	)

	// Check cascade depth limit
	if event.CascadeDepth >= MaxCascadeDepth {
		slog.Warn("asset action execution depth limit reached",
			slog.String("component", "asset-actions"),
			slog.String("chain_id", event.ExecutionChainID),
			slog.Int("depth", event.CascadeDepth),
		)
		return nil
	}

	// Get chain state for cycle detection
	var chain *ExecutionChain
	if event.ExecutionChainID != "" {
		chain = as.chainStore.GetChain(event.ExecutionChainID)
	}

	// Get actions for this set from cache
	as.cacheMu.RLock()
	actions := as.actionCache[event.SetID]
	as.cacheMu.RUnlock()

	if len(actions) == 0 {
		return nil
	}

	for _, action := range actions {
		// Cycle detection
		actionKey := fmt.Sprintf("asset:%d", action.ID)
		if chain != nil && chain.ExecutedActions[actionKey] {
			slog.Debug("skipping asset action - already executed in chain",
				slog.String("component", "asset-actions"),
				slog.Int("action_id", action.ID),
				slog.String("chain_id", event.ExecutionChainID),
			)
			continue
		}

		if as.matchesTrigger(action, event) {
			if err := as.executeAction(action, event, chain); err != nil {
				slog.Error("failed to execute asset action",
					slog.String("component", "asset-actions"),
					slog.Int("action_id", action.ID),
					slog.Any("error", err),
				)
			} else {
				atomic.AddInt64(&as.actionsExecuted, 1)
			}
		}
	}

	return nil
}

func (as *AssetActionService) matchesTrigger(action *models.AssetAction, event *models.AssetActionEvent) bool {
	if action.TriggerType != event.EventType {
		return false
	}

	var config models.AssetTriggerConfig
	if action.TriggerConfig != "" {
		if err := json.Unmarshal([]byte(action.TriggerConfig), &config); err != nil {
			slog.Warn("failed to parse asset trigger config",
				slog.String("component", "asset-actions"),
				slog.Int("action_id", action.ID),
				slog.Any("error", err),
			)
			return false
		}
	}

	// Cascade control
	if event.TriggeredByAction && !config.RespondToCascades {
		return false
	}

	if action.TriggerConfig == "" {
		return true
	}

	switch event.EventType {
	case models.AssetTriggerAssetCreated, models.AssetTriggerAssetUpdated:
		if config.AssetTypeID != nil {
			assetTypeID := utils.InterfaceToIntPtr(event.NewValues["asset_type_id"])
			if assetTypeID == nil {
				// Also check from a direct DB lookup for asset_created events
				var typeID int
				err := as.db.QueryRow(`SELECT asset_type_id FROM assets WHERE id = ?`, event.AssetID).Scan(&typeID)
				if err != nil || typeID != *config.AssetTypeID {
					return false
				}
			} else if *assetTypeID != *config.AssetTypeID {
				return false
			}
		}

	case models.AssetTriggerAssetStatusChanged:
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
	}

	return true
}

func (as *AssetActionService) executeAction(action *models.AssetAction, event *models.AssetActionEvent, chain *ExecutionChain) error {
	startTime := time.Now()

	// Get or create execution chain
	chainID := event.ExecutionChainID
	if chainID == "" {
		chainID = uuid.New().String()
		chain = as.chainStore.CreateChain(chainID)
	} else if chain == nil {
		chain = as.chainStore.CreateChain(chainID)
	}

	// Mark this action as executed
	actionKey := fmt.Sprintf("asset:%d", action.ID)
	chain.ExecutedActions[actionKey] = true

	// Create execution log
	log := &models.AssetActionExecutionLog{
		ActionID:     action.ID,
		AssetID:      &event.AssetID,
		TriggerEvent: string(event.EventType),
		Status:       models.ActionStatusRunning,
		StartedAt:    startTime,
	}
	logID, err := as.repo.CreateExecutionLog(log)
	if err != nil {
		slog.Warn("failed to create asset action execution log",
			slog.String("component", "asset-actions"),
			slog.Int("action_id", action.ID),
			slog.Any("error", err),
		)
	}
	log.ID = logID

	// Build execution context
	ctx := &models.AssetActionExecutionContext{
		Action:      action,
		Event:       event,
		Variables:   make(map[string]interface{}),
		StepResults: []models.StepResult{},
		ChainID:     chainID,
	}

	// Populate initial variables
	ctx.Variables["asset_id"] = event.AssetID
	ctx.Variables["set_id"] = event.SetID
	ctx.Variables["actor_user_id"] = event.ActorUserID

	// Load asset data for variable substitution
	as.loadAssetVariables(ctx)

	for k, v := range event.OldValues {
		ctx.Variables["old_"+k] = v
	}
	for k, v := range event.NewValues {
		ctx.Variables["new_"+k] = v
	}

	// Topological sort
	sortedNodes, err := as.topologicalSort(action.Nodes, action.Edges)
	if err != nil {
		log.Status = models.ActionStatusFailed
		log.ErrorMessage = fmt.Sprintf("failed to sort nodes: %v", err)
		completedAt := time.Now()
		log.CompletedAt = &completedAt
		if logErr := as.repo.UpdateExecutionLog(log); logErr != nil {
			slog.Error("failed to update asset execution log", slog.Any("error", logErr))
		}
		return fmt.Errorf("failed to topologically sort nodes: %w", err)
	}

	// Execute nodes
	executedNodes := make(map[int]bool)
	for _, node := range sortedNodes {
		if node.NodeType == models.AssetNodeTrigger {
			executedNodes[node.ID] = true
			continue
		}

		canExecute := as.canExecuteNode(node.ID, action.Edges, executedNodes, ctx)
		if !canExecute {
			continue
		}

		stepResult := models.StepResult{
			NodeID:    node.ID,
			NodeType:  models.ActionNodeType(node.NodeType),
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
			slog.Warn("asset action node execution failed",
				slog.String("component", "asset-actions"),
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

	for _, result := range ctx.StepResults {
		if result.Status == models.ActionStatusFailed {
			log.Status = models.ActionStatusFailed
			break
		}
	}

	if trace, err := json.Marshal(ctx.StepResults); err == nil {
		log.ExecutionTrace = string(trace)
	}

	if logErr := as.repo.UpdateExecutionLog(log); logErr != nil {
		slog.Error("failed to update asset execution log", slog.Any("error", logErr))
	}

	return nil
}

func (as *AssetActionService) loadAssetVariables(ctx *models.AssetActionExecutionContext) {
	var title, assetTag, description sql.NullString
	var assetTypeID, statusID sql.NullInt64
	var typeName, statusName sql.NullString

	err := as.db.QueryRow(`
		SELECT a.title, a.asset_tag, a.description, a.asset_type_id, a.status_id,
		       COALESCE(t.name, ''), COALESCE(s.name, '')
		FROM assets a
		LEFT JOIN asset_types t ON a.asset_type_id = t.id
		LEFT JOIN asset_statuses s ON a.status_id = s.id
		WHERE a.id = ?
	`, ctx.Event.AssetID).Scan(&title, &assetTag, &description, &assetTypeID, &statusID, &typeName, &statusName)
	if err != nil {
		slog.Debug("failed to load asset for variables",
			slog.String("component", "asset-actions"),
			slog.Int("asset_id", ctx.Event.AssetID),
			slog.Any("error", err),
		)
		return
	}

	if title.Valid {
		ctx.Variables["asset_title"] = title.String
	}
	if assetTag.Valid {
		ctx.Variables["asset_tag"] = assetTag.String
	}
	if description.Valid {
		ctx.Variables["asset_description"] = description.String
	}
	if assetTypeID.Valid {
		ctx.Variables["asset_type_id"] = int(assetTypeID.Int64)
	}
	if statusID.Valid {
		ctx.Variables["asset_status_id"] = int(statusID.Int64)
	}
	if typeName.Valid {
		ctx.Variables["asset_type_name"] = typeName.String
	}
	if statusName.Valid {
		ctx.Variables["asset_status_name"] = statusName.String
	}
}

func (as *AssetActionService) executeNode(node *models.AssetActionNode, ctx *models.AssetActionExecutionContext, stepResult *models.StepResult) error {
	switch node.NodeType {
	case models.AssetNodeCreateItem:
		return as.executeCreateItem(node, ctx, stepResult)
	case models.AssetNodeSetField:
		return as.executeSetField(node, ctx, stepResult)
	case models.AssetNodeSetStatus:
		return as.executeSetStatus(node, ctx, stepResult)
	case models.AssetNodeCondition:
		return as.executeCondition(node, ctx, stepResult)
	case models.AssetNodeNotifyUser:
		return as.executeNotifyUser(node, ctx, stepResult)
	default:
		return fmt.Errorf("unknown asset action node type: %s", node.NodeType)
	}
}

// executeCreateItem creates a work item from an asset action
func (as *AssetActionService) executeCreateItem(node *models.AssetActionNode, ctx *models.AssetActionExecutionContext, stepResult *models.StepResult) error {
	var config models.CreateItemNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse create_item config: %w", err)
	}

	title := as.substituteVariables(config.Title, ctx)
	if title == "" {
		title = "Asset Action: " + fmt.Sprintf("%v", ctx.Variables["asset_title"])
	}
	description := as.substituteVariables(config.Description, ctx)

	creatorID := ctx.Event.ActorUserID
	itemTypeID := config.ItemTypeID

	itemID, err := CreateItem(as.db, ItemCreationParams{
		WorkspaceID: config.WorkspaceID,
		Title:       title,
		Description: description,
		ItemTypeID:  &itemTypeID,
		CreatorID:   &creatorID,
	})
	if err != nil {
		return fmt.Errorf("failed to create item: %w", err)
	}

	stepResult.Output = map[string]interface{}{
		"item_id":      itemID,
		"title":        title,
		"workspace_id": config.WorkspaceID,
	}

	// Emit item created event with cascade context
	if as.eventCoordinator != nil {
		item := &models.Item{
			ID:          int(itemID),
			WorkspaceID: config.WorkspaceID,
			Title:       title,
			ItemTypeID:  &itemTypeID,
			CreatorID:   &creatorID,
		}
		_ = as.db.QueryRow(`SELECT key FROM workspaces WHERE id = ?`, config.WorkspaceID).Scan(&item.WorkspaceKey)

		as.eventCoordinator.EmitItemCreatedWithContext(item, creatorID, ActionContext{
			TriggeredByAction: true,
			ExecutionChainID:  ctx.ChainID,
			CascadeDepth:      ctx.Event.CascadeDepth + 1,
			SourceApplication: "asset",
		})
	}

	return nil
}

// executeSetField updates an asset's custom_field_values
func (as *AssetActionService) executeSetField(node *models.AssetActionNode, ctx *models.AssetActionExecutionContext, stepResult *models.StepResult) error {
	var config models.SetFieldNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse set_field config: %w", err)
	}

	value := as.substituteVariables(config.Value, ctx)

	// Get current custom_field_values
	var customFieldsJSON sql.NullString
	err := as.db.QueryRow(`SELECT custom_field_values FROM assets WHERE id = ?`, ctx.Event.AssetID).Scan(&customFieldsJSON)
	if err != nil {
		return fmt.Errorf("failed to get asset custom_field_values: %w", err)
	}

	var customFields map[string]interface{}
	if customFieldsJSON.Valid && customFieldsJSON.String != "" {
		if err = json.Unmarshal([]byte(customFieldsJSON.String), &customFields); err != nil {
			customFields = make(map[string]interface{})
		}
	} else {
		customFields = make(map[string]interface{})
	}

	oldValue := customFields[config.FieldName]
	customFields[config.FieldName] = value

	updatedJSON, err := json.Marshal(customFields)
	if err != nil {
		return fmt.Errorf("failed to serialize custom_field_values: %w", err)
	}

	_, err = as.db.Exec(`UPDATE assets SET custom_field_values = ?, updated_at = ? WHERE id = ?`,
		string(updatedJSON), time.Now(), ctx.Event.AssetID)
	if err != nil {
		return fmt.Errorf("failed to update asset: %w", err)
	}

	stepResult.Output = map[string]interface{}{
		"field_name": config.FieldName,
		"old_value":  oldValue,
		"new_value":  value,
	}

	// Emit cascaded asset_updated event
	as.EmitAssetActionEvent(&models.AssetActionEvent{
		EventType:         models.AssetTriggerAssetUpdated,
		SetID:             ctx.Event.SetID,
		AssetID:           ctx.Event.AssetID,
		ActorUserID:       ctx.Event.ActorUserID,
		OldValues:         map[string]interface{}{config.FieldName: oldValue},
		NewValues:         map[string]interface{}{config.FieldName: value},
		TriggeredByAction: true,
		ExecutionChainID:  ctx.ChainID,
		CascadeDepth:      ctx.Event.CascadeDepth + 1,
		SourceApplication: "asset",
	})

	return nil
}

// executeSetStatus updates an asset's status_id
func (as *AssetActionService) executeSetStatus(node *models.AssetActionNode, ctx *models.AssetActionExecutionContext, stepResult *models.StepResult) error {
	var config models.SetStatusNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse set_status config: %w", err)
	}

	// Get current status
	var oldStatusID int
	err := as.db.QueryRow(`SELECT COALESCE(status_id, 0) FROM assets WHERE id = ?`, ctx.Event.AssetID).Scan(&oldStatusID)
	if err != nil {
		return fmt.Errorf("failed to get current asset status: %w", err)
	}

	_, err = as.db.Exec(`UPDATE assets SET status_id = ?, updated_at = ? WHERE id = ?`,
		config.StatusID, time.Now(), ctx.Event.AssetID)
	if err != nil {
		return fmt.Errorf("failed to update asset status: %w", err)
	}

	stepResult.Output = map[string]interface{}{
		"old_status_id": oldStatusID,
		"new_status_id": config.StatusID,
	}

	// Emit cascaded status_changed event if changed
	if oldStatusID != config.StatusID {
		as.EmitAssetActionEvent(&models.AssetActionEvent{
			EventType:         models.AssetTriggerAssetStatusChanged,
			SetID:             ctx.Event.SetID,
			AssetID:           ctx.Event.AssetID,
			ActorUserID:       ctx.Event.ActorUserID,
			OldValues:         map[string]interface{}{"status_id": oldStatusID},
			NewValues:         map[string]interface{}{"status_id": config.StatusID},
			TriggeredByAction: true,
			ExecutionChainID:  ctx.ChainID,
			CascadeDepth:      ctx.Event.CascadeDepth + 1,
			SourceApplication: "asset",
		})
	}

	return nil
}

// executeCondition evaluates a condition on an asset field
func (as *AssetActionService) executeCondition(node *models.AssetActionNode, ctx *models.AssetActionExecutionContext, stepResult *models.StepResult) error {
	var config models.ConditionNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse condition config: %w", err)
	}

	fieldValue := ctx.Variables[config.FieldName]
	if fieldValue == nil {
		fieldValue = ctx.Variables["new_"+config.FieldName]
	}

	result := evaluateCondition(fieldValue, config.Operator, config.Value)

	stepResult.Output = map[string]interface{}{
		"condition_result": result,
		"field_name":       config.FieldName,
		"field_value":      fieldValue,
		"operator":         config.Operator,
		"compare_value":    config.Value,
	}

	return nil
}

// executeNotifyUser sends notifications
func (as *AssetActionService) executeNotifyUser(node *models.AssetActionNode, ctx *models.AssetActionExecutionContext, stepResult *models.StepResult) error {
	if as.notificationService == nil {
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

	userIDs := []int{}
	for _, recipient := range config.Recipients {
		if id, err := strconv.Atoi(recipient); err == nil {
			userIDs = append(userIDs, id)
		}
	}

	message := as.substituteVariables(config.Message, ctx)
	title := as.substituteVariables(config.Title, ctx)

	for range userIDs {
		as.notificationService.EmitEvent(&NotificationEvent{
			EventType:   "action.notification",
			ActorUserID: ctx.Event.ActorUserID,
			Title:       title,
			TemplateData: map[string]interface{}{
				"message": message,
			},
		})
	}

	stepResult.Output = map[string]interface{}{
		"recipient_count": len(userIDs),
		"title":           title,
		"message":         message,
	}

	return nil
}

// substituteVariables replaces {{variable}} placeholders with actual values
func (as *AssetActionService) substituteVariables(template string, ctx *models.AssetActionExecutionContext) string {
	re := regexp.MustCompile(`\{\{([^}]+)\}\}`)

	return re.ReplaceAllStringFunc(template, func(match string) string {
		varName := strings.TrimPrefix(strings.TrimSuffix(match, "}}"), "{{")
		varName = strings.TrimSpace(varName)

		parts := strings.Split(varName, ".")
		if len(parts) == 2 {
			switch parts[0] {
			case "asset":
				key := "asset_" + parts[1]
				if val, ok := ctx.Variables[key]; ok {
					return fmt.Sprintf("%v", val)
				}
			case "actor":
				if parts[1] == "id" {
					return strconv.Itoa(ctx.Event.ActorUserID)
				}
			}
		}

		if val, ok := ctx.Variables[varName]; ok {
			return fmt.Sprintf("%v", val)
		}

		return match
	})
}

// Shared topology and execution helpers

func (as *AssetActionService) topologicalSort(nodes []models.AssetActionNode, edges []models.AssetActionEdge) ([]models.AssetActionNode, error) {
	if len(nodes) == 0 {
		return nil, nil
	}

	nodeMap := make(map[int]*models.AssetActionNode)
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

	queue := []int{}
	for nodeID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, nodeID)
		}
	}

	sorted := []models.AssetActionNode{}
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

	if len(sorted) != len(nodes) {
		return nil, fmt.Errorf("cycle detected in asset action flow")
	}

	return sorted, nil
}

func (as *AssetActionService) canExecuteNode(nodeID int, edges []models.AssetActionEdge, executedNodes map[int]bool, ctx *models.AssetActionExecutionContext) bool {
	hasIncomingEdge := false
	for _, edge := range edges {
		if edge.TargetNodeID == nodeID {
			hasIncomingEdge = true
			if !executedNodes[edge.SourceNodeID] {
				return false
			}
			if edge.EdgeType == "true" || edge.EdgeType == "false" {
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
	return hasIncomingEdge || len(edges) == 0
}

// evaluateCondition evaluates a condition (reused from workspace action service)
func evaluateCondition(value interface{}, operator, compareValue string) bool {
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
	case "is_empty":
		return strValue == "" || strValue == "null" || strValue == "<nil>"
	case "is_not_empty":
		return strValue != "" && strValue != "null" && strValue != "<nil>"
	default:
		return false
	}
}

// ExecuteActionManually executes an asset action manually for a given asset.
func (as *AssetActionService) ExecuteActionManually(action *models.AssetAction, assetID, actorUserID int) error {
	event := &models.AssetActionEvent{
		EventType:         models.AssetTriggerManual,
		SetID:             action.SetID,
		AssetID:           assetID,
		ActorUserID:       actorUserID,
		OldValues:         map[string]interface{}{},
		NewValues:         map[string]interface{}{},
		TriggeredByAction: false,
		CascadeDepth:      0,
	}

	if err := as.executeAction(action, event, nil); err != nil {
		atomic.AddInt64(&as.errors, 1)
		return err
	}

	atomic.AddInt64(&as.actionsExecuted, 1)
	return nil
}
