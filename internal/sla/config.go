package sla

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"windshift/internal/businesstime"
	"windshift/internal/cql"
	"windshift/internal/itemevents"
	"windshift/internal/models"
)

type statusInfo struct {
	categoryID int
	completed  bool
}

// compiledConfig is the immutable, evaluated form of a workspace's SLA
// configuration for one generation.
type compiledConfig struct {
	workspaceID int
	generation  int

	metrics []*compiledMetric
	// inputAll is true when any metric has a construct the extractor could not
	// understand; the whole workspace then evaluates every fact.
	inputAll bool

	statuses    map[int]statusInfo
	calendars   map[int]*compiledCalendar
	goals       map[int]*compiledGoal
	metricsByID map[int]*compiledMetric

	generator *cql.SQLGenerator

	thresholds []compiledThreshold
}

type compiledCalendar struct {
	raw      businesstime.RawCalendar
	compiled *businesstime.Calendar
}

type compiledMetric struct {
	id            int
	name          string
	displayFormat string

	conditions []*compiledCondition
	goals      []*compiledGoal

	// inputFields holds canonical item change fields referenced by conditions
	// and goals. inputAll means every fact is relevant for this metric.
	inputFields map[string]struct{}
	inputAll    bool

	hasCommentConditions bool
}

type compiledCondition struct {
	phase       string
	typ         string
	statusIDs   map[int]struct{}
	categoryIDs map[int]struct{}
	commentKind string
}

type compiledGoal struct {
	id       int
	position int
	ql       string
	ast      *cql.ASTNode
	targets  []models.SLAGoalTarget
}

// compiledThreshold is an active warning threshold. key encodes the row and
// percent so editing the percent is treated as a new threshold (and can fire
// again for the same cycle), while an unchanged threshold stays once-per-cycle.
type compiledThreshold struct {
	key      string
	percent  int
	metricID *int
}

func (c *compiledConfig) thresholdsFor(metricID int) []compiledThreshold {
	var out []compiledThreshold
	for _, threshold := range c.thresholds {
		if threshold.metricID == nil || *threshold.metricID == metricID {
			out = append(out, threshold)
		}
	}
	return out
}

// goalTarget is the target selected for one item under a metric's goals.
type goalTarget struct {
	goalID  int
	target  *models.SLAGoalTarget
	query   string
	matched bool
}

type configCache struct {
	mu      sync.RWMutex
	entries map[cacheKey]*compiledConfig
}

type cacheKey struct {
	workspaceID int
	generation  int
}

func newConfigCache() *configCache {
	return &configCache{entries: map[cacheKey]*compiledConfig{}}
}

func (c *configCache) get(workspaceID, generation int) (*compiledConfig, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	config, ok := c.entries[cacheKey{workspaceID, generation}]
	return config, ok
}

func (c *configCache) put(config *compiledConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[cacheKey{config.workspaceID, config.generation}] = config
}

func (c *configCache) invalidate(workspaceID int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.entries {
		if key.workspaceID == workspaceID {
			delete(c.entries, key)
		}
	}
}

// loadConfig compiles a workspace's configuration. Calendars referenced by
// targets are loaded eagerly so evaluation never touches the database per fact.
func (e *Engine) loadConfig(ctx context.Context, workspaceID, generation int) (*compiledConfig, error) {
	metrics, err := e.repo.ListMetrics(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	statusCategories, completedCategories, err := e.repo.StatusCategoryInfo(ctx)
	if err != nil {
		return nil, err
	}
	workspaceKey, err := e.repo.WorkspaceKey(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	customFields, err := e.items.GetCQLCustomFieldMapContext(ctx)
	if err != nil {
		return nil, err
	}

	workspaceMap := map[string]int{}
	if workspaceKey != "" {
		workspaceMap[strings.ToLower(workspaceKey)] = workspaceID
	}
	generator := cql.NewSQLGenerator(workspaceMap, customFields, e.db.GetDriverName())
	generator.EnableLegacyCustomFieldNameFallback()

	config := &compiledConfig{
		workspaceID: workspaceID,
		generation:  generation,
		statuses:    map[int]statusInfo{},
		calendars:   map[int]*compiledCalendar{},
		goals:       map[int]*compiledGoal{},
		metricsByID: map[int]*compiledMetric{},
		generator:   generator,
	}
	for statusID, categoryID := range statusCategories {
		config.statuses[statusID] = statusInfo{categoryID: categoryID, completed: completedCategories[categoryID]}
	}

	thresholds, err := e.repo.ListWarningThresholds(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, threshold := range thresholds {
		if !threshold.IsActive {
			continue
		}
		config.thresholds = append(config.thresholds, compiledThreshold{
			key:      fmt.Sprintf("%d:%d", threshold.ID, threshold.Percent),
			percent:  threshold.Percent,
			metricID: threshold.MetricID,
		})
	}

	for i := range metrics {
		metric := &metrics[i]
		if !metric.IsActive {
			continue
		}
		compiled, err := e.compileMetric(ctx, config, metric, customFields)
		if err != nil {
			return nil, fmt.Errorf("compile metric %d: %w", metric.ID, err)
		}
		config.metrics = append(config.metrics, compiled)
		config.metricsByID[compiled.id] = compiled
		if compiled.inputAll {
			config.inputAll = true
		}
	}
	return config, nil
}

func (e *Engine) compileMetric(ctx context.Context, config *compiledConfig, metric *models.SLAMetric, customFields cql.CustomFieldMap) (*compiledMetric, error) {
	compiled := &compiledMetric{
		id:            metric.ID,
		name:          metric.Name,
		displayFormat: metric.DisplayFormat,
		inputFields:   map[string]struct{}{},
	}
	for _, condition := range metric.Conditions {
		parsed, err := compileCondition(condition)
		if err != nil {
			return nil, err
		}
		compiled.conditions = append(compiled.conditions, parsed)
		for _, field := range conditionInputFields(parsed) {
			compiled.inputFields[field] = struct{}{}
		}
		if parsed.typ == conditionCommentByCustomer || parsed.typ == conditionCommentByAgent {
			compiled.hasCommentConditions = true
		}
	}

	for _, goal := range metric.Goals {
		ast, err := parseQLL(goal.QLQuery)
		if err != nil {
			return nil, fmt.Errorf("goal %d: %w", goal.ID, err)
		}
		compiledGoal := &compiledGoal{id: goal.ID, position: goal.Position, ql: goal.QLQuery, ast: ast, targets: goal.Targets}
		compiled.goals = append(compiled.goals, compiledGoal)
		config.goals[goal.ID] = compiledGoal

		fields, all := extractQLInputFields(ast, customFields)
		if all {
			compiled.inputAll = true
		}
		for field := range fields {
			compiled.inputFields[field] = struct{}{}
		}

		for _, target := range goal.Targets {
			if _, ok := config.calendars[target.CalendarID]; ok {
				continue
			}
			calendar, err := e.repo.GetCalendar(ctx, target.CalendarID)
			if err != nil {
				return nil, fmt.Errorf("load target calendar %d: %w", target.CalendarID, err)
			}
			raw, err := businesstime.FromStored(calendar.Timezone, calendar.WeeklyIntervals, calendar.Holidays)
			if err != nil {
				return nil, fmt.Errorf("parse target calendar %d: %w", target.CalendarID, err)
			}
			compiledCal, err := businesstime.Compile(raw)
			if err != nil {
				return nil, fmt.Errorf("compile target calendar %d: %w", target.CalendarID, err)
			}
			config.calendars[target.CalendarID] = &compiledCalendar{raw: raw, compiled: compiledCal}
		}
	}
	return compiled, nil
}

const (
	conditionCreated               = "created"
	conditionStatusEntered         = "status_entered"
	conditionStatusExited          = "status_exited"
	conditionStatusCurrent         = "status_current"
	conditionStatusCategoryEntered = "status_category_entered"
	conditionStatusCategoryExited  = "status_category_exited"
	conditionStatusCategoryCurrent = "status_category_current"
	conditionAssigneeSet           = "assignee_set"
	conditionResolutionSet         = "resolution_set"
	conditionCommentByCustomer     = "comment_by_customer"
	conditionCommentByAgent        = "comment_by_agent"
)

type conditionConfig struct {
	StatusIDs   []int `json:"status_ids"`
	CategoryIDs []int `json:"category_ids"`
}

func compileCondition(condition models.SLACondition) (*compiledCondition, error) {
	compiled := &compiledCondition{phase: condition.Phase, typ: condition.ConditionType, statusIDs: map[int]struct{}{}, categoryIDs: map[int]struct{}{}}
	if len(condition.Config) > 0 {
		var config conditionConfig
		if err := json.Unmarshal(condition.Config, &config); err != nil {
			return nil, fmt.Errorf("condition %d config: %w", condition.ID, err)
		}
		for _, id := range config.StatusIDs {
			compiled.statusIDs[id] = struct{}{}
		}
		for _, id := range config.CategoryIDs {
			compiled.categoryIDs[id] = struct{}{}
		}
	}
	switch condition.ConditionType {
	case conditionStatusCategoryEntered, conditionStatusCategoryExited, conditionStatusCategoryCurrent, conditionResolutionSet:
		// Category conditions carry no status IDs; clear any stray values.
		compiled.statusIDs = map[int]struct{}{}
	case conditionCommentByCustomer:
		compiled.commentKind = "customer"
	case conditionCommentByAgent:
		compiled.commentKind = "agent"
	}
	return compiled, nil
}

// conditionInputFields returns the canonical item fields a condition depends on.
func conditionInputFields(condition *compiledCondition) []string {
	switch condition.typ {
	case conditionCreated:
		return []string{"__created"}
	case conditionStatusEntered, conditionStatusExited, conditionStatusCurrent:
		return []string{"status_id"}
	case conditionStatusCategoryEntered, conditionStatusCategoryExited, conditionStatusCategoryCurrent, conditionResolutionSet:
		return []string{"status_id"}
	case conditionAssigneeSet:
		return []string{"assignee_id"}
	case conditionCommentByCustomer, conditionCommentByAgent:
		return []string{"__comment"}
	}
	return nil
}

// parseQLL parses a Windshift QL expression into an AST.
func parseQLL(query string) (*cql.ASTNode, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, nil
	}
	tokens, err := cql.NewTokenizer(trimmed).Tokenize()
	if err != nil {
		return nil, fmt.Errorf("tokenize QL: %w", err)
	}
	ast, err := cql.NewParser(tokens).Parse()
	if err != nil {
		return nil, fmt.Errorf("parse QL: %w", err)
	}
	return ast, nil
}

// ValidateGoalQuery reports whether an SLA goal QL expression compiles. The
// metric service calls it before persisting a configuration so one malformed
// native goal cannot disable workspace-wide SLA evaluation.
func ValidateGoalQuery(query string) error {
	_, err := parseQLL(query)
	return err
}

// ValidateConditionConfig parses a condition's configuration with the same
// decoder the engine uses at evaluation time. The metric service calls it
// before persisting so one malformed condition shape cannot abort
// workspace-wide configuration compilation later.
func ValidateConditionConfig(condition models.SLACondition) error {
	_, err := compileCondition(condition)
	return err
}

// extractQLInputFields walks a goal AST and returns the canonical item change
// fields the goal can depend on. The second result is true when the extractor
// met a construct it does not understand and the metric must widen to every
// field.
func extractQLInputFields(node *cql.ASTNode, customFields cql.CustomFieldMap) (map[string]struct{}, bool) {
	fields := map[string]struct{}{}
	if node == nil {
		return fields, true
	}
	all := collectASTFields(node, customFields, fields)
	return fields, all
}

func collectASTFields(node *cql.ASTNode, customFields cql.CustomFieldMap, fields map[string]struct{}) bool {
	if node == nil {
		return false
	}
	switch node.Type {
	case cql.NodeBinaryOp:
		leftAll := collectASTFields(node.Left, customFields, fields)
		rightAll := collectASTFields(node.Right, customFields, fields)
		return leftAll || rightAll
	case cql.NodeComparison, cql.NodeInExpression:
		fieldNode := node.Left
		if node.Type == cql.NodeInExpression {
			fieldNode = node.Field
		}
		return collectFieldNode(fieldNode, customFields, fields)
	case cql.NodeNullCheck:
		return collectFieldNode(node.Left, customFields, fields)
	case cql.NodeFunction:
		// now()/today() do not depend on item fields; every other function is
		// treated conservatively.
		switch strings.ToLower(node.Value) {
		case "now", "today", "startofday", "endofday":
			return false
		default:
			return true
		}
	case cql.NodeList:
		if node.Values == nil {
			return false
		}
		return collectASTFields(node.Values, customFields, fields)
	case cql.NodeLiteral, cql.NodeIdentifier:
		return false
	default:
		return true
	}
}

func collectFieldNode(node *cql.ASTNode, customFields cql.CustomFieldMap, fields map[string]struct{}) bool {
	if node == nil {
		return false
	}
	if node.Type == cql.NodeFunction {
		return collectASTFields(node, customFields, fields)
	}
	if node.Type != cql.NodeIdentifier {
		return true
	}
	canonical, ok := canonicalChangeField(node.Value, customFields)
	if !ok {
		return true
	}
	if canonical != "" {
		fields[canonical] = struct{}{}
	}
	return false
}

// canonicalChangeField maps a QL field identifier to the canonical change
// field recorded in itemevents.FieldChange. ok is false when the field is not
// tracked by change facts, which widens the metric to every field.
func canonicalChangeField(field string, customFields cql.CustomFieldMap) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(field))
	if lower == "" {
		return "", true
	}
	if strings.HasPrefix(lower, "cf_") {
		return customFieldChangeField(field[3:], customFields), len(customFields) > 0
	}
	if strings.HasPrefix(lower, "custom.") {
		return customFieldChangeField(field[7:], customFields), len(customFields) > 0
	}
	if strings.HasPrefix(lower, "cfid_") {
		id := strings.TrimPrefix(lower, "cfid_")
		return "cf_" + id, true
	}
	switch lower {
	case "status", "status_id", "statusid":
		return "status_id", true
	case "status_category", "statuscategory", "status_completed", "statuscompleted":
		return "status_id", true
	case "priority", "priority_id", "priorityid":
		return "priority_id", true
	case "assignee", "assignee_id", "assigneeid":
		return "assignee_id", true
	case "reporter", "reporter_id", "reporterid":
		return "reporter_id", true
	case "title":
		return "title", true
	case "description":
		return "description", true
	case "type", "item_type", "item_type_id", "itemtype":
		return "item_type_id", true
	case "project", "project_id", "projectid":
		return "project_id", true
	case "parent", "parent_id", "parentid":
		return "parent_id", true
	case "iteration", "iteration_id", "iterationid", "sprint":
		return "iteration_id", true
	case "request_type", "requesttype", "request_type_id":
		return "request_type_id", true
	case "due_date", "duedate":
		return "due_date", true
	case "start_date", "startdate":
		return "start_date", true
	case "end_date", "enddate":
		return "end_date", true
	case "workspace", "workspace_id", "workspaceid":
		return "workspace_id", true
	case "labels", "components", "milestones", "links", "watchers", "comments":
		// Collection-valued fields are not itemevents change facts yet, so
		// they stay unextractable: metrics referencing them fall back to
		// inputAll and evaluate every recorded fact (WI-1532).
		return "", false
	}
	return "", false
}

func customFieldChangeField(name string, customFields cql.CustomFieldMap) string {
	if info, ok := customFields[strings.ToLower(name)]; ok {
		return fmt.Sprintf("cf_%d", info.ID)
	}
	return ""
}

// relevantForFact reports whether a metric needs evaluation for a fact.
func (m *compiledMetric) relevantForFact(fact itemevents.RecordedFact) bool {
	if m.inputAll {
		return true
	}
	switch fact.Type {
	case itemevents.Created, itemevents.Deleted:
		return true
	case itemevents.CommentCreated:
		return m.hasCommentConditions
	case itemevents.StatusChanged:
		if _, ok := m.inputFields["status_id"]; ok {
			return true
		}
		// A status change also carries any other changed fields.
		return anyChangeRelevant(m.inputFields, fact.Changes)
	case itemevents.Updated:
		return anyChangeRelevant(m.inputFields, fact.Changes)
	case itemevents.Linked, itemevents.Unlinked, itemevents.Merged, itemevents.Split:
		return false
	default:
		return true
	}
}

func anyChangeRelevant(inputFields map[string]struct{}, changes []itemevents.FieldChange) bool {
	for _, change := range changes {
		if _, ok := inputFields[change.Field]; ok {
			return true
		}
	}
	return false
}

// goalInputsChanged reports whether a fact can change the selected goal.
func (m *compiledMetric) goalInputsChanged(fact itemevents.RecordedFact) bool {
	if m.inputAll {
		return true
	}
	for _, change := range fact.Changes {
		if _, ok := m.inputFields[change.Field]; ok {
			return true
		}
	}
	return false
}
