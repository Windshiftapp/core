package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"windshift/internal/businesstime"
	"windshift/internal/cql"
	"windshift/internal/database"
	"windshift/internal/jira"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sla"
)

// ErrSLAImportInvalid marks an import document that cannot be parsed or lacks
// the identities needed to import it safely.
var ErrSLAImportInvalid = errors.New("invalid SLA import document")

// SLAImportSource is stored on imported calendars and metrics so replace mode
// can reconcile exactly the rows this importer owns.
const SLAImportSource = "jira"

// SLAImport modes.
const (
	SLAImportModeMerge   = "merge"
	SLAImportModeReplace = "replace"
)

// SLAImportDocument is the request-scoped Jira SLA export. It is deliberately
// explicit: Jira does not expose SLA configuration through a stable public API,
// so the capture pipeline (or an administrator) supplies this document.
type SLAImportDocument struct {
	Calendars []SLAImportCalendar `json:"calendars"`
	Metrics   []SLAImportMetric   `json:"metrics"`
	Cycles    []SLAImportCycle    `json:"cycles"`
}

type SLAImportCalendar struct {
	SourceID        string                                  `json:"source_id"`
	SourcePayload   json.RawMessage                         `json:"source_payload,omitempty"`
	Name            string                                  `json:"name"`
	Description     string                                  `json:"description"`
	Timezone        string                                  `json:"timezone"`
	WeeklyIntervals map[string][]businesstime.ClockInterval `json:"weekly_intervals"`
	Holidays        []businesstime.Holiday                  `json:"holidays"`
	IsDefault       bool                                    `json:"is_default"`
}

type SLAImportMetric struct {
	SourceID      string               `json:"source_id"`
	SourcePayload json.RawMessage      `json:"source_payload,omitempty"`
	Name          string               `json:"name"`
	DisplayFormat string               `json:"display_format"`
	IsActive      *bool                `json:"is_active"`
	Conditions    []SLAImportCondition `json:"conditions"`
	Goals         []SLAImportGoal      `json:"goals"`
}

type SLAImportCondition struct {
	SourceID      string          `json:"source_id"`
	SourcePayload json.RawMessage `json:"source_payload,omitempty"`
	Phase         string          `json:"phase"`
	ConditionType string          `json:"condition_type"`
	Config        json.RawMessage `json:"config"`
}

type SLAImportGoal struct {
	SourceID      string            `json:"source_id"`
	SourcePayload json.RawMessage   `json:"source_payload,omitempty"`
	Position      int               `json:"position"`
	JQL           string            `json:"jql"`
	QL            string            `json:"ql,omitempty"`
	Targets       []SLAImportTarget `json:"targets"`
}

type SLAImportTarget struct {
	SourceID         string          `json:"source_id"`
	SourcePayload    json.RawMessage `json:"source_payload,omitempty"`
	Position         int             `json:"position"`
	Priority         string          `json:"priority"`
	IsFallback       bool            `json:"is_fallback"`
	TargetMs         int64           `json:"target_ms"`
	CalendarSourceID string          `json:"calendar_source_id"`
}

type SLAImportCycle struct {
	SourceID            string          `json:"source_id"`
	SourcePayload       json.RawMessage `json:"source_payload,omitempty"`
	RemainingMs         *int64          `json:"remaining_ms,omitempty"`
	RemainingAtPauseMs  *int64          `json:"remaining_at_pause_ms,omitempty"`
	WithinCalendarHours *bool           `json:"within_calendar_hours,omitempty"`
	Breached            *bool           `json:"breached,omitempty"`
	MetricSourceID      string          `json:"metric_source_id"`
	ItemID              int             `json:"item_id"`
	ItemKey             string          `json:"item_key"`
	GoalSourceID        string          `json:"goal_source_id"`
	CalendarSourceID    string          `json:"calendar_source_id"`
	CycleNo             int             `json:"cycle_no"`
	Status              string          `json:"status"`
	StartedAt           time.Time       `json:"started_at"`
	StoppedAt           *time.Time      `json:"stopped_at"`
	ElapsedMs           int64           `json:"elapsed_ms"`
	GoalDurationMs      int64           `json:"goal_duration_ms"`
	BreachedAt          *time.Time      `json:"breached_at"`
	BreachTime          *time.Time      `json:"breach_time"`
	Paused              bool            `json:"paused"`
	PauseStartedAt      *time.Time      `json:"pause_started_at"`
}

// SLAImportCounts summarizes planned or applied writes.
type SLAImportCounts struct {
	Create int `json:"create"`
	Update int `json:"update"`
	Skip   int `json:"skip"`
}

// SLAImportIssue is one needs-attention or warning entry.
type SLAImportIssue struct {
	Entity   string `json:"entity"`
	SourceID string `json:"source_id,omitempty"`
	Message  string `json:"message"`
}

// SLAImportResult is the preview and import response.
type SLAImportResult struct {
	Mode           string           `json:"mode"`
	Calendars      SLAImportCounts  `json:"calendars"`
	Metrics        SLAImportCounts  `json:"metrics"`
	Cycles         SLAImportCounts  `json:"cycles"`
	NeedsAttention []SLAImportIssue `json:"needs_attention"`
	Warnings       []SLAImportIssue `json:"warnings"`
}

// SLAImportService imports Jira SLA configuration and cycle facts. It is
// request-scoped: preview resolves everything without writing, import applies
// the same resolution in one transaction per configuration step.
type SLAImportService struct {
	db     database.Database
	repo   *repository.SLARepository
	engine *sla.Engine
}

// NewSLAImportService constructs the import service.
func NewSLAImportService(db database.Database, engine *sla.Engine) *SLAImportService {
	return &SLAImportService{db: db, repo: repository.NewSLARepository(db), engine: engine}
}

// resolvedMetric carries the persisted shape plus import bookkeeping.
type resolvedMetric struct {
	metric      *models.SLAMetric
	exercisable bool // every goal translated and every target resolved
}

// Preview resolves a document without writing and reports what would change.
func (s *SLAImportService) Preview(ctx context.Context, workspaceID int, document SLAImportDocument) (*SLAImportResult, error) {
	result, _, _, _, err := s.resolve(ctx, workspaceID, document)
	if err != nil {
		return nil, err
	}
	result.Mode = SLAImportModeMerge
	return result, nil
}

// Import applies a document in merge or replace mode.
func (s *SLAImportService) Import(ctx context.Context, workspaceID int, document SLAImportDocument, mode string) (*SLAImportResult, error) {
	if mode != SLAImportModeMerge && mode != SLAImportModeReplace {
		return nil, fmt.Errorf("%w: mode must be merge or replace", ErrSLAImportInvalid)
	}
	result, calendars, metrics, cycles, err := s.resolve(ctx, workspaceID, document)
	if err != nil {
		return nil, err
	}
	result.Mode = mode

	// Persist calendars first: metrics and cycles reference them.
	calendarIDs := map[string]int{}
	err = database.WithTx(s.db, func(tx database.Tx) error {
		for _, calendar := range calendars {
			id, err := s.persistCalendar(ctx, tx, calendar)
			if err != nil {
				return err
			}
			calendarIDs[valueOrEmpty(calendar.SourceID)] = id
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	metricIDs := map[string]int{}
	goalIDsByMetric := map[string]map[string]int{}
	goalQueriesByMetric := map[string]map[string]string{}
	err = database.WithTx(s.db, func(tx database.Tx) error {
		for _, resolved := range metrics {
			id, goalIDs, err := s.persistMetric(ctx, tx, resolved, calendarIDs)
			if err != nil {
				return err
			}
			metricIDs[valueOrEmpty(resolved.metric.SourceID)] = id
			sourceID := valueOrEmpty(resolved.metric.SourceID)
			goalIDsByMetric[sourceID] = goalIDs
			goalQueriesByMetric[sourceID] = make(map[string]string, len(resolved.metric.Goals))
			for _, goal := range resolved.metric.Goals {
				goalQueriesByMetric[sourceID][valueOrEmpty(goal.SourceID)] = goal.QLQuery
			}
			if _, err := s.engine.BumpConfigGeneration(ctx, tx, workspaceID); err != nil {
				return err
			}
			metric := id
			if err := s.repo.UpsertJob(ctx, tx, &models.SLAJob{Kind: models.SLAJobRecalcMetric, MetricID: &metric, DueAt: time.Now().UTC()}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	snapshots := map[string]json.RawMessage{}
	for _, calendar := range calendars {
		snapshot, err := importCalendarSnapshot(calendar)
		if err != nil {
			return nil, err
		}
		snapshots[valueOrEmpty(calendar.SourceID)] = snapshot
	}

	// Resolve cycle inputs before the write transaction: SQLite serializes
	// writers, so repository reads must not run while the transaction holds the
	// write lock.
	type cycleWrite struct {
		cycle      *SLAImportCycle
		metricID   int
		itemID     int
		calendarID int
		goalID     int
		goalQuery  string
		snapshot   json.RawMessage
		existingID int64
	}
	writes := make([]cycleWrite, 0, len(cycles))
	for i := range cycles {
		cycle := &cycles[i]
		metricID := metricIDs[cycle.MetricSourceID]
		if metricID == 0 {
			continue
		}
		itemID, err := s.resolveItemID(ctx, workspaceID, cycle.ItemID, cycle.ItemKey)
		if err != nil {
			continue
		}
		calendarID := calendarIDs[cycle.CalendarSourceID]
		snapshot := snapshots[cycle.CalendarSourceID]
		if cycle.CalendarSourceID != "" && calendarID == 0 {
			calendar, err := s.repo.GetCalendarBySourceID(ctx, workspaceID, cycle.CalendarSourceID)
			if err != nil {
				continue
			}
			calendarID = calendar.ID
			snapshot, err = importCalendarSnapshot(calendar)
			if err != nil {
				return nil, err
			}
		}
		existingID := int64(0)
		if existing, err := s.repo.GetCycleBySourceID(ctx, metricID, cycle.SourceID); err == nil {
			existingID = existing.ID
		} else if !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}
		writes = append(writes, cycleWrite{
			cycle:      cycle,
			metricID:   metricID,
			itemID:     itemID,
			calendarID: calendarID,
			goalID:     goalIDsByMetric[cycle.MetricSourceID][cycle.GoalSourceID],
			goalQuery:  goalQueriesByMetric[cycle.MetricSourceID][cycle.GoalSourceID],
			snapshot:   snapshot,
			existingID: existingID,
		})
	}
	err = database.WithTx(s.db, func(tx database.Tx) error {
		for _, write := range writes {
			if err := s.writeCycle(ctx, tx, write.metricID, write.itemID, write.calendarID, write.goalID, write.goalQuery, write.snapshot, write.existingID, write.cycle); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if mode == SLAImportModeReplace {
		if err := s.reconcileReplace(ctx, workspaceID, document, metricIDs, result); err != nil {
			return nil, err
		}
	}
	if s.engine != nil {
		s.engine.Wake()
	}
	return result, nil
}

// resolve builds the persisted shapes and the preview counts without writing.
func (s *SLAImportService) resolve(ctx context.Context, workspaceID int, document SLAImportDocument) (*SLAImportResult, []*models.WorkingCalendar, []resolvedMetric, []SLAImportCycle, error) {
	if err := validateSLAImportIdentities(document); err != nil {
		return nil, nil, nil, nil, err
	}
	result := &SLAImportResult{Warnings: []SLAImportIssue{}, NeedsAttention: []SLAImportIssue{}}

	calendars := make([]*models.WorkingCalendar, 0, len(document.Calendars))
	calendarBySource := map[string]*models.WorkingCalendar{}
	for i := range document.Calendars {
		source := document.Calendars[i]
		if strings.TrimSpace(source.SourceID) == "" {
			return nil, nil, nil, nil, fmt.Errorf("%w: calendar %d has no source_id", ErrSLAImportInvalid, i)
		}
		calendar := buildImportCalendar(workspaceID, source)
		if existing, err := s.repo.GetCalendarBySourceID(ctx, workspaceID, source.SourceID); err == nil {
			calendar.ID = existing.ID
			result.Calendars.Update++
		} else if errors.Is(err, repository.ErrNotFound) {
			result.Calendars.Create++
		} else {
			return nil, nil, nil, nil, err
		}
		calendars = append(calendars, calendar)
		calendarBySource[source.SourceID] = calendar
	}

	metrics := make([]resolvedMetric, 0, len(document.Metrics))
	metricsBySource := map[string]resolvedMetric{}
	for i := range document.Metrics {
		metric, issues := s.resolveMetric(ctx, workspaceID, document.Metrics[i], calendarBySource)
		result.NeedsAttention = append(result.NeedsAttention, issues...)
		if existing, err := s.repo.GetMetricBySourceID(ctx, workspaceID, document.Metrics[i].SourceID); err == nil {
			metric.metric.ID = existing.ID
			result.Metrics.Update++
		} else if errors.Is(err, repository.ErrNotFound) {
			result.Metrics.Create++
		} else {
			return nil, nil, nil, nil, err
		}
		if !metric.exercisable {
			metric.metric.IsActive = false
		}
		metrics = append(metrics, metric)
		metricsBySource[document.Metrics[i].SourceID] = metric
	}

	for i := range document.Cycles {
		cycle := document.Cycles[i]
		metric, exists := metricsBySource[cycle.MetricSourceID]
		if !exists {
			result.NeedsAttention = append(result.NeedsAttention, SLAImportIssue{Entity: "cycle", SourceID: cycle.SourceID, Message: "metric was not included in the import"})
			result.Cycles.Skip++
			continue
		}
		if _, err := s.resolveItemID(ctx, workspaceID, cycle.ItemID, cycle.ItemKey); err != nil {
			result.NeedsAttention = append(result.NeedsAttention, SLAImportIssue{Entity: "cycle", SourceID: cycle.SourceID, Message: err.Error()})
			result.Cycles.Skip++
			continue
		}
		if cycle.Status != "" && cycle.Status != models.SLACycleOngoing && cycle.Status != models.SLACycleCompleted && cycle.Status != models.SLACycleAbandoned {
			result.NeedsAttention = append(result.NeedsAttention, SLAImportIssue{Entity: "cycle", SourceID: cycle.SourceID, Message: "unsupported cycle status: " + cycle.Status})
			result.Cycles.Skip++
			continue
		}
		if cycle.GoalSourceID != "" {
			goalFound := false
			for _, sourceGoal := range document.Metrics {
				if sourceGoal.SourceID != cycle.MetricSourceID {
					continue
				}
				for _, goal := range sourceGoal.Goals {
					goalFound = goalFound || goal.SourceID == cycle.GoalSourceID
				}
			}
			if !goalFound {
				result.NeedsAttention = append(result.NeedsAttention, SLAImportIssue{Entity: "cycle", SourceID: cycle.SourceID, Message: "goal was not found in the imported metric"})
				result.Cycles.Skip++
				continue
			}
		}
		if cycle.CalendarSourceID != "" {
			if _, ok := calendarBySource[cycle.CalendarSourceID]; !ok {
				if _, err := s.repo.GetCalendarBySourceID(ctx, workspaceID, cycle.CalendarSourceID); err != nil {
					result.NeedsAttention = append(result.NeedsAttention, SLAImportIssue{Entity: "cycle", SourceID: cycle.SourceID, Message: "calendar was not found: " + cycle.CalendarSourceID})
					result.Cycles.Skip++
					continue
				}
			}
		}
		existingID := int64(0)
		if metric.metric.ID != 0 {
			existing, err := s.repo.GetCycleBySourceID(ctx, metric.metric.ID, cycle.SourceID)
			if err == nil {
				existingID = existing.ID
			} else if !errors.Is(err, repository.ErrNotFound) {
				return nil, nil, nil, nil, err
			}
		}
		if existingID == 0 {
			result.Cycles.Create++
		} else {
			result.Cycles.Update++
		}
	}

	return result, calendars, metrics, document.Cycles, nil
}

func (s *SLAImportService) resolveMetric(ctx context.Context, workspaceID int, source SLAImportMetric, calendars map[string]*models.WorkingCalendar) (resolvedMetric, []SLAImportIssue) {
	metric := &models.SLAMetric{
		WorkspaceID:   workspaceID,
		Name:          strings.TrimSpace(source.Name),
		DisplayFormat: source.DisplayFormat,
		IsActive:      source.IsActive == nil || *source.IsActive,
		ImportStatus:  "ready",
		Source:        stringPtr(SLAImportSource),
		SourceID:      stringPtr(source.SourceID),
		SourcePayload: rawPayloadString(source.SourcePayload),
	}
	if metric.DisplayFormat == "" {
		metric.DisplayFormat = "time"
	}
	if metric.DisplayFormat != "time" && metric.DisplayFormat != "due_date" {
		metric.DisplayFormat = "time"
	}
	issues := []SLAImportIssue{}
	exercisable := true

	conditions := make([]models.SLACondition, 0, len(source.Conditions))
	for i, condition := range source.Conditions {
		config := condition.Config
		if len(config) == 0 {
			config = json.RawMessage("{}")
		}
		conditions = append(conditions, models.SLACondition{
			Phase:         condition.Phase,
			Position:      i,
			ConditionType: condition.ConditionType,
			Config:        config,
			SourcePayload: rawPayloadString(condition.SourcePayload),
		})
	}
	metric.Conditions = conditions
	if err := validateSLAConditions(metric.Conditions); err != nil {
		exercisable = false
		issues = append(issues, SLAImportIssue{Entity: "condition", SourceID: source.SourceID, Message: err.Error()})
	}

	// One resolved goal per target. A priority-scoped target becomes its own
	// goal with the priority appended to the base query; the fallback target
	// keeps the base query and is ordered last, so first-match-wins preserves
	// priority precedence.
	resolvedGoals := make([]models.SLAGoal, 0, len(source.Goals))
	for _, goal := range source.Goals {
		ql := strings.TrimSpace(goal.QL)
		status := "ready"
		if ql != "" {
			tokens, err := cql.NewTokenizer(ql).Tokenize()
			if err == nil {
				_, err = cql.NewParser(tokens).Parse()
			}
			if err == nil && s.engine != nil {
				err = s.engine.ValidateGoalQL(ctx, workspaceID, ql)
			}
			if err != nil {
				status = "needs_attention"
				exercisable = false
				issues = append(issues, SLAImportIssue{Entity: "goal", SourceID: goal.SourceID, Message: "invalid Windshift QL: " + err.Error()})
			}
		} else {
			translated, unsupported := jira.TranslateJQLToWindshiftQL(goal.JQL, workspaceID)
			if len(unsupported) > 0 {
				status = "needs_attention"
				exercisable = false
				for _, clause := range unsupported {
					issues = append(issues, SLAImportIssue{Entity: "goal", SourceID: goal.SourceID, Message: "untranslated JQL clause: " + clause})
				}
				ql = translated
			} else {
				ql = translated
			}
		}
		baseQL := strings.TrimSpace(ql)
		if baseQL == "" {
			baseQL = "1 = 0"
		}
		newGoal := func(query string, sourceID *string) models.SLAGoal {
			resolved := models.SLAGoal{
				QLQuery:       query,
				ImportStatus:  status,
				SourceID:      sourceID,
				SourcePayload: rawPayloadString(goal.SourcePayload),
			}
			if status == "needs_attention" {
				resolved.OriginalJQL = stringPtr(goal.JQL)
			}
			return resolved
		}

		var fallback *SLAImportTarget
		for i := range goal.Targets {
			target := goal.Targets[i]
			priorityName := strings.TrimSpace(target.Priority)
			if target.IsFallback || priorityName == "" {
				if fallback == nil {
					fallback = &goal.Targets[i]
				}
				continue
			}
			normalized := importPriorityName(priorityName)
			if _, err := s.repo.PriorityIDByName(ctx, normalized); err != nil {
				exercisable = false
				issues = append(issues, SLAImportIssue{Entity: "target", SourceID: target.SourceID, Message: "priority not found: " + target.Priority})
				continue
			}
			resolvedGoal := newGoal(combineGoalQuery(baseQL, normalized), priorityGoalSourceID(goal.SourceID, normalized))
			resolvedTarget, ok := resolveImportTarget(calendars, target)
			if !ok {
				exercisable = false
				resolvedGoal.ImportStatus = "needs_attention"
				issues = append(issues, SLAImportIssue{Entity: "target", SourceID: target.SourceID, Message: "calendar not found: " + target.CalendarSourceID})
			}
			resolvedGoal.Targets = []models.SLAGoalTarget{resolvedTarget}
			resolvedGoals = append(resolvedGoals, resolvedGoal)
		}
		if fallback != nil {
			resolvedGoal := newGoal(baseQL, stringPtr(goal.SourceID))
			resolvedTarget, ok := resolveImportTarget(calendars, *fallback)
			if !ok {
				exercisable = false
				resolvedGoal.ImportStatus = "needs_attention"
				issues = append(issues, SLAImportIssue{Entity: "target", SourceID: fallback.SourceID, Message: "calendar not found: " + fallback.CalendarSourceID})
			}
			resolvedGoal.Targets = []models.SLAGoalTarget{resolvedTarget}
			resolvedGoals = append(resolvedGoals, resolvedGoal)
		}
	}
	for i := range resolvedGoals {
		resolvedGoals[i].Position = i
	}
	metric.Goals = resolvedGoals
	if !exercisable {
		metric.ImportStatus = "needs_attention"
	}
	return resolvedMetric{metric: metric, exercisable: exercisable}, issues
}

// importPriorityName maps Jira's priority labels onto Windshift priority names.
func importPriorityName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "highest":
		return "Critical"
	case "lowest":
		return "Low"
	default:
		return strings.TrimSpace(name)
	}
}

// combineGoalQuery scopes a base goal query to one priority.
func combineGoalQuery(base, priorityName string) string {
	clause := "priority = '" + strings.ReplaceAll(priorityName, "'", "''") + "'"
	if base == "" {
		return clause
	}
	return "(" + base + ") AND " + clause
}

// priorityGoalSourceID gives a split-off priority goal a stable source identity
// so re-imports match it instead of colliding on the parent goal's source id.
func priorityGoalSourceID(goalSourceID, priorityName string) *string {
	if strings.TrimSpace(goalSourceID) == "" {
		return nil
	}
	return stringPtr(goalSourceID + "#" + priorityName)
}

// resolveImportTarget maps an imported target's calendar source id to a
// persisted calendar id. ok is false when the source calendar is unknown.
func resolveImportTarget(calendars map[string]*models.WorkingCalendar, target SLAImportTarget) (models.SLAGoalTarget, bool) {
	resolved := models.SLAGoalTarget{
		TargetMs:         target.TargetMs,
		SourceID:         stringPtr(target.SourceID),
		SourcePayload:    rawPayloadString(target.SourcePayload),
		CalendarSourceID: target.CalendarSourceID,
	}
	calendar, ok := calendars[target.CalendarSourceID]
	if !ok {
		return resolved, false
	}
	resolved.CalendarID = calendar.ID
	if calendar.ID == 0 {
		// New calendar; resolve after persistence. Keep the source id so
		// import can remap it.
		resolved.CalendarSourceID = target.CalendarSourceID
	}
	return resolved, true
}

func (s *SLAImportService) persistCalendar(ctx context.Context, tx database.Tx, calendar *models.WorkingCalendar) (int, error) {
	if calendar.ID != 0 {
		if err := s.repo.UpdateCalendarTx(ctx, tx, calendar); err != nil {
			return 0, err
		}
		return calendar.ID, nil
	}
	id, err := s.repo.CreateCalendarTx(ctx, tx, calendar)
	if err != nil {
		return 0, err
	}
	calendar.ID = id
	return id, nil
}

func (s *SLAImportService) persistMetric(ctx context.Context, tx database.Tx, resolved resolvedMetric, calendarIDs map[string]int) (metricID int, goalIDs map[string]int, err error) {
	metric := resolved.metric
	// Remap target calendars newly created in this import.
	for i := range metric.Goals {
		for j := range metric.Goals[i].Targets {
			target := &metric.Goals[i].Targets[j]
			if target.CalendarID == 0 && target.CalendarSourceID != "" {
				target.CalendarID = calendarIDs[target.CalendarSourceID]
				target.CalendarSourceID = ""
			}
		}
	}
	if metric.ID != 0 {
		err = s.repo.UpdateMetric(ctx, tx, metric)
		metricID = metric.ID
	} else {
		metricID, err = s.repo.CreateMetric(ctx, tx, metric)
	}
	if err != nil {
		return 0, nil, err
	}
	goalIDs, err = s.repo.GoalIDsBySourceID(ctx, tx, metricID)
	if err != nil {
		return 0, nil, err
	}
	return metricID, goalIDs, nil
}

func (s *SLAImportService) writeCycle(ctx context.Context, tx database.Tx, metricID, itemID, calendarID, goalID int, goalQuery string, snapshot json.RawMessage, existingID int64, source *SLAImportCycle) error {
	status := source.Status
	if status == "" {
		status = models.SLACycleOngoing
	}
	cycleNo := source.CycleNo
	if cycleNo <= 0 {
		cycleNo = 1
	}
	now := time.Now().UTC()
	remaining := source.GoalDurationMs - source.ElapsedMs
	if source.RemainingMs != nil {
		remaining = *source.RemainingMs
	}
	breached := source.BreachedAt != nil
	if source.Breached != nil {
		breached = *source.Breached
	}
	withinCalendarHours := false
	if source.WithinCalendarHours != nil {
		withinCalendarHours = *source.WithinCalendarHours
	}
	cycle := &models.ItemSLACycle{
		ItemID:              itemID,
		MetricID:            metricID,
		CycleNo:             cycleNo,
		Status:              status,
		StartedAt:           source.StartedAt,
		StoppedAt:           source.StoppedAt,
		BreachTime:          source.BreachTime,
		BreachedAt:          source.BreachedAt,
		GoalDurationMs:      source.GoalDurationMs,
		ElapsedMs:           source.ElapsedMs,
		RemainingMs:         remaining,
		RemainingAtPauseMs:  source.RemainingAtPauseMs,
		Paused:              source.Paused,
		WithinCalendarHours: withinCalendarHours,
		PauseStartedAt:      source.PauseStartedAt,
		Breached:            breached,
		LastCalculatedAt:    now,
		GoalQuerySnapshot:   goalQuery,
		SourcePayload:       rawPayloadString(source.SourcePayload),
		Origin:              models.SLAOriginImport,
		SourceID:            stringPtr(source.SourceID),
	}
	if goalID != 0 {
		cycle.GoalID = &goalID
	}
	if calendarID != 0 {
		cycle.CalendarID = &calendarID
	}
	if len(snapshot) > 0 {
		cycle.CalendarSnapshot = snapshot
	} else {
		cycle.CalendarSnapshot = json.RawMessage(`{"timezone":"UTC","weekly_intervals":{}}`)
	}
	if status == models.SLACycleOngoing && !cycle.Paused {
		if calendar, err := businesstime.CompileJSON(string(cycle.CalendarSnapshot)); err == nil {
			remaining := time.Duration(cycle.GoalDurationMs-cycle.ElapsedMs) * time.Millisecond
			if remaining > 0 {
				if deadline, ok := calendar.AddCalendarTime(now, remaining); ok {
					cycle.NextDeadlineAt = &deadline
				}
			}
		}
	}

	if existingID == 0 {
		cycleID, err := s.repo.InsertCycle(ctx, tx, cycle)
		if err != nil {
			return err
		}
		cycle.ID = cycleID
	} else {
		cycle.ID = existingID
		if err := s.repo.UpdateCycle(ctx, tx, cycle); err != nil {
			return err
		}
	}
	if err := s.repo.DeleteJobsForCycle(ctx, tx, cycle.ID); err != nil {
		return err
	}
	if cycle.Status == models.SLACycleOngoing && !cycle.Paused && cycle.NextDeadlineAt != nil {
		deadline := *cycle.NextDeadlineAt
		cycleRef := cycle.ID
		job := &models.SLAJob{Kind: models.SLAJobBreach, CycleID: &cycleRef, DueAt: deadline, DeadlineAt: &deadline}
		if err := s.repo.UpsertJob(ctx, tx, job); err != nil {
			return err
		}
	}
	return nil
}

func (s *SLAImportService) reconcileReplace(ctx context.Context, workspaceID int, document SLAImportDocument, metricIDs map[string]int, result *SLAImportResult) error {
	keepMetrics := map[string]bool{}
	for _, metric := range document.Metrics {
		keepMetrics[metric.SourceID] = true
	}
	existingMetrics, err := s.repo.ListImportedMetricSourceIDs(ctx, workspaceID, SLAImportSource)
	if err != nil {
		return err
	}
	for _, sourceID := range existingMetrics {
		if keepMetrics[sourceID] {
			continue
		}
		stale, err := s.repo.GetMetricBySourceID(ctx, workspaceID, sourceID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				continue
			}
			return err
		}
		if err := database.WithTx(s.db, func(tx database.Tx) error {
			if err := s.repo.DeleteMetric(ctx, tx, stale.ID); err != nil {
				return err
			}
			_, err := s.engine.BumpConfigGeneration(ctx, tx, workspaceID)
			return err
		}); err != nil {
			return err
		}
		result.Warnings = append(result.Warnings, SLAImportIssue{Entity: "metric", SourceID: sourceID, Message: "removed by replace import"})
	}

	keepCalendars := map[string]bool{}
	for _, calendar := range document.Calendars {
		keepCalendars[calendar.SourceID] = true
	}
	existingCalendars, err := s.repo.ListImportedCalendarSourceIDs(ctx, workspaceID, SLAImportSource)
	if err != nil {
		return err
	}
	for _, sourceID := range existingCalendars {
		if keepCalendars[sourceID] {
			continue
		}
		stale, err := s.repo.GetCalendarBySourceID(ctx, workspaceID, sourceID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				continue
			}
			return err
		}
		if err := s.repo.DeleteCalendar(ctx, stale.ID); err != nil {
			result.NeedsAttention = append(result.NeedsAttention, SLAImportIssue{Entity: "calendar", SourceID: sourceID, Message: "cannot remove: " + err.Error()})
			continue
		}
		result.Warnings = append(result.Warnings, SLAImportIssue{Entity: "calendar", SourceID: sourceID, Message: "removed by replace import"})
	}
	_ = metricIDs
	return nil
}

func (s *SLAImportService) resolveItemID(ctx context.Context, workspaceID, itemID int, key string) (int, error) {
	if itemID > 0 {
		belongs, err := s.repo.ItemBelongsToWorkspace(ctx, workspaceID, itemID)
		if err != nil {
			return 0, err
		}
		if !belongs {
			return 0, fmt.Errorf("item_id %d does not belong to the authorized workspace", itemID)
		}
		return itemID, nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return 0, fmt.Errorf("cycle has neither item_id nor item_key")
	}
	workspaceKey, err := s.repo.WorkspaceKey(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	prefix, number, ok := strings.Cut(key, "-")
	if !ok {
		return 0, fmt.Errorf("item_key %q must be %s-<number>", key, workspaceKey)
	}
	if !strings.EqualFold(prefix, workspaceKey) {
		return 0, fmt.Errorf("item_key %q does not belong to workspace %s", key, workspaceKey)
	}
	parsed, err := strconv.Atoi(number)
	if err != nil {
		return 0, fmt.Errorf("item_key %q has a non-numeric number", key)
	}
	id, err := s.repo.ItemIDByWorkspaceNumber(ctx, workspaceID, parsed)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return 0, fmt.Errorf("item %q was not imported", key)
		}
		return 0, err
	}
	return id, nil
}

func buildImportCalendar(workspaceID int, source SLAImportCalendar) *models.WorkingCalendar {
	weekly, _ := json.Marshal(source.WeeklyIntervals)
	holidays, _ := json.Marshal(source.Holidays)
	if len(weekly) == 0 || string(weekly) == "null" {
		weekly = []byte("{}")
	}
	if len(holidays) == 0 || string(holidays) == "null" {
		holidays = []byte("[]")
	}
	timezone := strings.TrimSpace(source.Timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	return &models.WorkingCalendar{
		WorkspaceID:     &workspaceID,
		Name:            strings.TrimSpace(source.Name),
		Description:     source.Description,
		Timezone:        timezone,
		WeeklyIntervals: weekly,
		Holidays:        holidays,
		IsDefault:       source.IsDefault,
		Source:          stringPtr(SLAImportSource),
		SourceID:        stringPtr(source.SourceID),
		SourcePayload:   rawPayloadString(source.SourcePayload),
	}
}

func importCalendarSnapshot(calendar *models.WorkingCalendar) (json.RawMessage, error) {
	raw, err := businesstime.FromStored(calendar.Timezone, calendar.WeeklyIntervals, calendar.Holidays)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func rawPayloadString(value json.RawMessage) *string {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	return stringPtr(string(value))
}

func validateSLAImportIdentities(document SLAImportDocument) error {
	validate := func(entity, sourceID string, seen map[string]struct{}) error {
		sourceID = strings.TrimSpace(sourceID)
		if sourceID == "" {
			return fmt.Errorf("%w: %s has no source_id", ErrSLAImportInvalid, entity)
		}
		if _, exists := seen[sourceID]; exists {
			return fmt.Errorf("%w: duplicate %s source_id %q", ErrSLAImportInvalid, entity, sourceID)
		}
		seen[sourceID] = struct{}{}
		return nil
	}

	calendarIDs := map[string]struct{}{}
	for _, calendar := range document.Calendars {
		if err := validate("calendar", calendar.SourceID, calendarIDs); err != nil {
			return err
		}
	}
	metricIDs := map[string]struct{}{}
	goalIDs := map[string]struct{}{}
	targetIDs := map[string]struct{}{}
	for _, metric := range document.Metrics {
		if err := validate("metric", metric.SourceID, metricIDs); err != nil {
			return err
		}
		for _, goal := range metric.Goals {
			if err := validate("goal", goal.SourceID, goalIDs); err != nil {
				return err
			}
			for _, target := range goal.Targets {
				if err := validate("target", target.SourceID, targetIDs); err != nil {
					return err
				}
			}
		}
	}
	cycleIDs := map[string]struct{}{}
	for _, cycle := range document.Cycles {
		if err := validate("cycle", cycle.SourceID, cycleIDs); err != nil {
			return err
		}
		if strings.TrimSpace(cycle.MetricSourceID) == "" {
			return fmt.Errorf("%w: cycle %q has no metric_source_id", ErrSLAImportInvalid, cycle.SourceID)
		}
	}
	return nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
