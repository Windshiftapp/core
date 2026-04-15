package logbook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/repository/actionutil"

	"github.com/google/uuid"
)

// generateUUID creates a new UUID string for execution chain tracking.
func generateUUID() string {
	return uuid.New().String()
}

// LogbookActionService handles logbook action execution within the sidecar.
// SQLite-dependent nodes (create_item, create_asset) are delegated to the main
// server via an internal HTTP endpoint.
type LogbookActionService struct {
	db   database.Database // PostgreSQL (logbook tables)
	repo *repository.LogbookActionRepository

	mainServerURL    string
	mainServerSecret string
	baseURL          string
	httpClient       *http.Client

	// Action cache: bucket_id -> enabled actions
	actionCache map[string][]*models.LogbookAction
	cacheMu     sync.RWMutex

	// Event processing
	eventChan chan *models.LogbookActionEvent
	stopChan  chan struct{}
	wg        sync.WaitGroup

	// Statistics
	eventsProcessed int64
	actionsExecuted int64
	errors          int64
}

// NewLogbookActionService creates a new logbook action service for the sidecar.
func NewLogbookActionService(db database.Database, repo *repository.LogbookActionRepository, mainServerURL, mainServerSecret, baseURL string) *LogbookActionService {
	service := &LogbookActionService{
		db:               db,
		repo:             repo,
		mainServerURL:    mainServerURL,
		mainServerSecret: mainServerSecret,
		baseURL:          baseURL,
		actionCache:      make(map[string][]*models.LogbookAction),
		eventChan:        make(chan *models.LogbookActionEvent, 500),
		stopChan:         make(chan struct{}),
	}

	if mainServerURL != "" {
		service.httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	if err := service.refreshActionCache(); err != nil {
		slog.Warn("failed to load initial logbook action cache", slog.String("component", "logbook-actions"), slog.Any("error", err))
	}

	service.wg.Add(2)
	go service.eventProcessor()
	go service.cacheRefresher()

	slog.Info("logbook action service initialized", slog.String("component", "logbook-actions"))

	return service
}

// EmitEvent sends a logbook action event for async processing.
func (s *LogbookActionService) EmitEvent(event *models.LogbookActionEvent) {
	slog.Debug("queuing logbook action event",
		slog.String("component", "logbook-actions"),
		slog.String("event_type", string(event.EventType)),
		slog.String("bucket_id", event.BucketID),
		slog.String("document_id", event.DocumentID),
	)

	select {
	case s.eventChan <- event:
	default:
		slog.Warn("logbook action event channel full, dropping event",
			slog.String("component", "logbook-actions"),
			slog.String("event_type", string(event.EventType)),
			slog.String("bucket_id", event.BucketID),
		)
		atomic.AddInt64(&s.errors, 1)
	}
}

// ExecuteActionDirectly loads an action by ID and executes it with the given event.
// Used for manual trigger from the UI.
func (s *LogbookActionService) ExecuteActionDirectly(actionID int, event *models.LogbookActionEvent) error {
	action, err := s.repo.GetByID(actionID)
	if err != nil {
		return fmt.Errorf("failed to load action %d: %w", actionID, err)
	}

	return s.executeAction(action, event)
}

// Stop gracefully shuts down the service.
func (s *LogbookActionService) Stop() {
	close(s.stopChan)

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Debug("logbook action service stopped", slog.String("component", "logbook-actions"))
	case <-time.After(3 * time.Second):
		slog.Warn("logbook action service stop timed out", slog.String("component", "logbook-actions"))
	}
}

// InvalidateBucketCache invalidates the cache for a specific bucket.
func (s *LogbookActionService) InvalidateBucketCache(bucketID string) {
	actions, err := s.repo.ListEnabledByBucket(bucketID)
	if err != nil {
		slog.Error("failed to reload logbook actions for bucket",
			slog.String("component", "logbook-actions"),
			slog.String("bucket_id", bucketID),
			slog.Any("error", err),
		)
		return
	}

	s.cacheMu.Lock()
	if len(actions) > 0 {
		s.actionCache[bucketID] = actions
	} else {
		delete(s.actionCache, bucketID)
	}
	s.cacheMu.Unlock()
}

func (s *LogbookActionService) eventProcessor() {
	defer s.wg.Done()

	for {
		select {
		case event := <-s.eventChan:
			if err := s.processEvent(event); err != nil {
				slog.Error("failed to process logbook action event",
					slog.String("component", "logbook-actions"),
					slog.String("event_type", string(event.EventType)),
					slog.Any("error", err),
				)
				atomic.AddInt64(&s.errors, 1)
			} else {
				atomic.AddInt64(&s.eventsProcessed, 1)
			}
		case <-s.stopChan:
			slog.Debug("stopping logbook action event processor", slog.String("component", "logbook-actions"))
			for len(s.eventChan) > 0 {
				event := <-s.eventChan
				if err := s.processEvent(event); err != nil {
					slog.Error("failed to process logbook event during shutdown",
						slog.String("component", "logbook-actions"),
						slog.Any("error", err),
					)
				}
			}
			return
		}
	}
}

func (s *LogbookActionService) cacheRefresher() {
	defer s.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.refreshActionCache(); err != nil {
				slog.Error("failed to refresh logbook action cache", slog.String("component", "logbook-actions"), slog.Any("error", err))
			}
		case <-s.stopChan:
			return
		}
	}
}

func (s *LogbookActionService) refreshActionCache() error {
	rows, err := s.db.Query(`
		SELECT DISTINCT bucket_id FROM logbook_actions WHERE is_enabled = true
	`)
	if err != nil {
		return fmt.Errorf("failed to query buckets with actions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	newCache := make(map[string][]*models.LogbookAction)
	var bucketIDs []string

	for rows.Next() {
		var bucketID string
		if err := rows.Scan(&bucketID); err != nil {
			continue
		}
		bucketIDs = append(bucketIDs, bucketID)
	}

	for _, bucketID := range bucketIDs {
		actions, err := s.repo.ListEnabledByBucket(bucketID)
		if err != nil {
			slog.Error("failed to load logbook actions for bucket",
				slog.String("component", "logbook-actions"),
				slog.String("bucket_id", bucketID),
				slog.Any("error", err),
			)
			continue
		}
		newCache[bucketID] = actions
	}

	s.cacheMu.Lock()
	s.actionCache = newCache
	s.cacheMu.Unlock()

	slog.Debug("logbook action cache refreshed",
		slog.String("component", "logbook-actions"),
		slog.Int("bucket_count", len(newCache)),
	)

	return nil
}

func (s *LogbookActionService) processEvent(event *models.LogbookActionEvent) error { //nolint:unparam // error kept for interface consistency
	slog.Debug("processing logbook action event",
		slog.String("component", "logbook-actions"),
		slog.String("event_type", string(event.EventType)),
		slog.String("bucket_id", event.BucketID),
		slog.String("document_id", event.DocumentID),
	)

	s.cacheMu.RLock()
	actions := s.actionCache[event.BucketID]
	s.cacheMu.RUnlock()

	if len(actions) == 0 {
		return nil
	}

	for _, action := range actions {
		if s.matchesTrigger(action, event) {
			slog.Info("logbook action matches trigger, executing",
				slog.String("component", "logbook-actions"),
				slog.Int("action_id", action.ID),
				slog.String("action_name", action.Name),
				slog.String("document_id", event.DocumentID),
				slog.String("event_type", string(event.EventType)),
			)

			if err := s.executeAction(action, event); err != nil {
				slog.Error("failed to execute logbook action",
					slog.String("component", "logbook-actions"),
					slog.Int("action_id", action.ID),
					slog.Any("error", err),
				)
			} else {
				atomic.AddInt64(&s.actionsExecuted, 1)
			}
		}
	}

	return nil
}

func (s *LogbookActionService) matchesTrigger(action *models.LogbookAction, event *models.LogbookActionEvent) bool {
	if action.TriggerType != event.EventType {
		return false
	}

	if action.TriggerConfig == "" {
		return true
	}

	var config models.LogbookTriggerConfig
	if err := json.Unmarshal([]byte(action.TriggerConfig), &config); err != nil {
		slog.Warn("failed to parse logbook trigger config",
			slog.String("component", "logbook-actions"),
			slog.Int("action_id", action.ID),
			slog.Any("error", err),
		)
		return false
	}

	switch event.EventType {
	case models.LogbookTriggerDocumentClassified:
		if len(config.ContentTypes) > 0 {
			matched := false
			for _, ct := range config.ContentTypes {
				if ct == event.ContentType {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}

	case models.LogbookTriggerContentKeyword:
		if len(config.Keywords) > 0 {
			content := strings.ToLower(event.Title + " " + event.RawContent)
			mode := config.KeywordMode
			if mode == "" {
				mode = "any"
			}
			if mode == "any" {
				matched := false
				for _, kw := range config.Keywords {
					if strings.Contains(content, strings.ToLower(kw)) {
						matched = true
						break
					}
				}
				if !matched {
					return false
				}
			} else { // "all"
				for _, kw := range config.Keywords {
					if !strings.Contains(content, strings.ToLower(kw)) {
						return false
					}
				}
			}
		}

	case models.LogbookTriggerMimeType:
		if len(config.MimeTypes) > 0 {
			matched := false
			for _, pattern := range config.MimeTypes {
				if matchMimeType(pattern, event.MimeType) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
	}

	return true
}

// matchMimeType matches a MIME type pattern (supports wildcards like "image/*").
func matchMimeType(pattern, mimeType string) bool {
	if pattern == mimeType {
		return true
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(mimeType, prefix+"/")
	}
	return false
}

func (s *LogbookActionService) executeAction(action *models.LogbookAction, event *models.LogbookActionEvent) error {
	startTime := time.Now()

	// Generate execution chain ID for cross-application loop prevention
	if event.ExecutionChainID == "" {
		event.ExecutionChainID = generateUUID()
	}
	if event.SourceApplication == "" {
		event.SourceApplication = "logbook"
	}

	log := &models.LogbookActionExecutionLog{
		ActionID:     action.ID,
		DocumentID:   &event.DocumentID,
		TriggerEvent: string(event.EventType),
		Status:       models.ActionStatusRunning,
		StartedAt:    startTime,
	}
	logID, err := s.repo.CreateExecutionLog(log)
	if err != nil {
		slog.Warn("failed to create logbook execution log",
			slog.String("component", "logbook-actions"),
			slog.Int("action_id", action.ID),
			slog.Any("error", err),
		)
	}
	log.ID = logID

	// Build execution variables
	vars := map[string]interface{}{
		"doc.id":           event.DocumentID,
		"doc.bucket_id":    event.BucketID,
		"doc.title":        event.Title,
		"doc.content_type": event.ContentType,
		"doc.mime_type":    event.MimeType,
		"doc.source_type":  event.SourceType,
		"doc.author":       event.Author,
		"doc.link":         s.buildDocumentLink(event.DocumentID),
		"actor.id":         event.ActorUserID,
	}

	// Get topologically sorted nodes
	sortedNodes, err := s.topologicalSort(action.Nodes, action.Edges)
	if err != nil {
		log.Status = models.ActionStatusFailed
		log.ErrorMessage = fmt.Sprintf("failed to sort nodes: %v", err)
		completedAt := time.Now()
		log.CompletedAt = &completedAt
		_ = s.repo.UpdateExecutionLog(log)
		return fmt.Errorf("failed to topologically sort nodes: %w", err)
	}

	// Execute nodes in order
	var stepResults []models.StepResult
	executedNodes := make(map[int]bool)
	for _, node := range sortedNodes {
		if node.NodeType == models.LogbookNodeTrigger {
			executedNodes[node.ID] = true
			continue
		}

		canExecute := s.canExecuteNode(node.ID, action.Edges, executedNodes, stepResults)
		if !canExecute {
			continue
		}

		stepResult := models.StepResult{
			NodeID:    node.ID,
			NodeType:  models.ActionNodeType(node.NodeType),
			Status:    models.ActionStatusRunning,
			StartedAt: time.Now(),
		}

		err := s.executeNode(&node, event, vars, &stepResult)
		completedAt := time.Now()
		stepResult.CompletedAt = &completedAt

		if err != nil {
			stepResult.Status = models.ActionStatusFailed
			stepResult.ErrorMessage = err.Error()
			stepResults = append(stepResults, stepResult)
			slog.Warn("logbook action node execution failed",
				slog.String("component", "logbook-actions"),
				slog.Int("node_id", node.ID),
				slog.String("node_type", string(node.NodeType)),
				slog.Any("error", err),
			)
		} else {
			stepResult.Status = models.ActionStatusCompleted
			stepResults = append(stepResults, stepResult)
			executedNodes[node.ID] = true
		}
	}

	// Update execution log
	log.CompletedAt, log.Status, log.ErrorMessage, log.ExecutionTrace = actionutil.FinalizeExecutionLog(stepResults)

	if logErr := s.repo.UpdateExecutionLog(log); logErr != nil {
		slog.Error("failed to update logbook execution log", slog.Any("error", logErr))
	}

	slog.Info("logbook action execution completed",
		slog.String("component", "logbook-actions"),
		slog.Int("action_id", action.ID),
		slog.String("action_name", action.Name),
		slog.String("status", string(log.Status)),
		slog.Duration("duration", time.Since(startTime)),
	)

	return nil
}

func (s *LogbookActionService) topologicalSort(nodes []models.LogbookActionNode, edges []models.LogbookActionEdge) ([]models.LogbookActionNode, error) {
	return actionutil.TopologicalSort(nodes, edges)
}

func (s *LogbookActionService) canExecuteNode(nodeID int, edges []models.LogbookActionEdge, executedNodes map[int]bool, stepResults []models.StepResult) bool {
	return actionutil.CanExecuteNodeTyped(nodeID, edges, executedNodes, stepResults)
}

func (s *LogbookActionService) executeNode(node *models.LogbookActionNode, event *models.LogbookActionEvent, vars map[string]interface{}, stepResult *models.StepResult) error {
	switch node.NodeType {
	case models.LogbookNodeCreateItem:
		return s.executeCreateItem(node, event, vars, stepResult)
	case models.LogbookNodeCreateAsset:
		return s.executeCreateAssetNode(node, event, vars, stepResult)
	case models.LogbookNodeAssociateCustomer:
		return s.executeAssociateCustomer(node, event, stepResult)
	case models.LogbookNodeCondition:
		return s.executeConditionNode(node, event, vars, stepResult)
	default:
		return fmt.Errorf("unknown logbook node type: %s", node.NodeType)
	}
}

// executeCreateItem delegates to the main server via HTTP.
func (s *LogbookActionService) executeCreateItem(node *models.LogbookActionNode, event *models.LogbookActionEvent, vars map[string]interface{}, stepResult *models.StepResult) error {
	return s.executeViaMainServer(node, event, vars, stepResult)
}

// executeCreateAssetNode delegates to the main server via HTTP.
func (s *LogbookActionService) executeCreateAssetNode(node *models.LogbookActionNode, event *models.LogbookActionEvent, vars map[string]interface{}, stepResult *models.StepResult) error {
	return s.executeViaMainServer(node, event, vars, stepResult)
}

// executeViaMainServer sends a node execution request to the main server's internal endpoint.
func (s *LogbookActionService) executeViaMainServer(node *models.LogbookActionNode, event *models.LogbookActionEvent, vars map[string]interface{}, stepResult *models.StepResult) error {
	if s.httpClient == nil || s.mainServerURL == "" {
		return fmt.Errorf("main server not configured for node execution")
	}

	// Substitute variables in node config before sending
	resolvedConfig := s.substituteVariables(node.NodeConfig, vars)

	req := models.NodeExecutionRequest{
		NodeType:          string(node.NodeType),
		NodeConfig:        resolvedConfig,
		Event:             *event,
		TriggeredByAction: event.TriggeredByAction,
		ExecutionChainID:  event.ExecutionChainID,
		CascadeDepth:      event.CascadeDepth,
		SourceApplication: event.SourceApplication,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal node execution request: %w", err)
	}

	url := strings.TrimRight(s.mainServerURL, "/") + "/api/internal/logbook/execute-node"

	slog.Info("executing node via main server",
		slog.String("component", "logbook-actions"),
		slog.String("node_type", string(node.NodeType)),
		slog.String("url", url),
		slog.String("resolved_config", resolvedConfig),
	)

	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create node execution request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.mainServerSecret)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		slog.Error("main server HTTP request failed",
			slog.String("component", "logbook-actions"),
			slog.String("node_type", string(node.NodeType)),
			slog.Any("error", err),
		)
		return fmt.Errorf("node execution request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result models.NodeExecutionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		slog.Error("failed to decode main server response",
			slog.String("component", "logbook-actions"),
			slog.String("node_type", string(node.NodeType)),
			slog.Int("status_code", resp.StatusCode),
			slog.Any("error", err),
		)
		return fmt.Errorf("failed to decode node execution response: %w", err)
	}

	if resp.StatusCode >= 300 || result.Error != "" {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = fmt.Sprintf("main server returned status %d", resp.StatusCode)
		}
		slog.Error("main server node execution failed",
			slog.String("component", "logbook-actions"),
			slog.String("node_type", string(node.NodeType)),
			slog.Int("status_code", resp.StatusCode),
			slog.String("error_message", errMsg),
		)
		return fmt.Errorf("node execution failed: %s", errMsg)
	}

	slog.Info("node execution via main server succeeded",
		slog.String("component", "logbook-actions"),
		slog.String("node_type", string(node.NodeType)),
	)

	stepResult.Output = result.Output
	return nil
}

func (s *LogbookActionService) executeAssociateCustomer(node *models.LogbookActionNode, event *models.LogbookActionEvent, stepResult *models.StepResult) error {
	var config models.AssociateCustomerNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse associate_customer config: %w", err)
	}

	err := s.repo.UpdateDocumentCustomerAssociation(event.DocumentID, config.CustomerOrganisationID, config.PortalCustomerID)
	if err != nil {
		return fmt.Errorf("failed to associate customer: %w", err)
	}

	stepResult.Output = map[string]interface{}{
		"document_id":              event.DocumentID,
		"customer_organisation_id": config.CustomerOrganisationID,
		"portal_customer_id":       config.PortalCustomerID,
	}

	return nil
}

func (s *LogbookActionService) executeConditionNode(node *models.LogbookActionNode, event *models.LogbookActionEvent, vars map[string]interface{}, stepResult *models.StepResult) error {
	var config models.ConditionNodeConfig
	if err := json.Unmarshal([]byte(node.NodeConfig), &config); err != nil {
		return fmt.Errorf("failed to parse condition config: %w", err)
	}

	// Get field value from document metadata
	var fieldValue string
	switch config.FieldName {
	case "content_type":
		fieldValue = event.ContentType
	case "mime_type":
		fieldValue = event.MimeType
	case "title":
		fieldValue = event.Title
	case "source_type":
		fieldValue = event.SourceType
	case "author":
		fieldValue = event.Author
	default:
		if val, ok := vars[config.FieldName]; ok {
			fieldValue = fmt.Sprintf("%v", val)
		}
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

func evaluateCondition(fieldValue, operator, compareValue string) bool {
	switch operator {
	case "eq":
		return fieldValue == compareValue
	case "ne":
		return fieldValue != compareValue
	case "contains":
		return strings.Contains(strings.ToLower(fieldValue), strings.ToLower(compareValue))
	case "not_contains":
		return !strings.Contains(strings.ToLower(fieldValue), strings.ToLower(compareValue))
	case "starts_with":
		return strings.HasPrefix(strings.ToLower(fieldValue), strings.ToLower(compareValue))
	case "ends_with":
		return strings.HasSuffix(strings.ToLower(fieldValue), strings.ToLower(compareValue))
	case "matches":
		re, err := regexp.Compile(compareValue)
		if err != nil {
			return false
		}
		return re.MatchString(fieldValue)
	default:
		return false
	}
}

var logbookVarRegexp = regexp.MustCompile(`\{\{([^}]+)\}\}`)

func (s *LogbookActionService) substituteVariables(template string, vars map[string]interface{}) string {
	return logbookVarRegexp.ReplaceAllStringFunc(template, func(match string) string {
		varName := strings.TrimPrefix(strings.TrimSuffix(match, "}}"), "{{")
		varName = strings.TrimSpace(varName)

		if val, ok := vars[varName]; ok {
			return fmt.Sprintf("%v", val)
		}
		return match
	})
}

func (s *LogbookActionService) buildDocumentLink(documentID string) string {
	base := s.baseURL
	if base == "" {
		return "/logbook/documents/" + documentID
	}
	return strings.TrimRight(base, "/") + "/logbook/documents/" + documentID
}
