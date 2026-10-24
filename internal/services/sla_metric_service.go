package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sla"
)

// ErrSLAMetricInvalid marks metric validation failures that the transport
// should surface as a client error rather than an internal fault.
var ErrSLAMetricInvalid = errors.New("invalid SLA metric")

// SLAMetricInput is the transport-neutral metric payload, including the nested
// conditions, goals, and targets.
type SLAMetricInput struct {
	Name          string
	DisplayFormat string
	Position      int
	IsActive      *bool
	ImportStatus  string
	Conditions    []models.SLACondition
	Goals         []models.SLAGoal
}

// SLAMetricService owns SLA metric configuration. Conditions, goals, and
// targets are part of the metric payload, not separate sub-resources: they are
// replaced atomically with the metric, their validation depends on the
// workspace and its team bindings, and every write must bump the workspace
// configuration generation and enqueue recalculation in one transaction.
// See page 70 §9 (WI-1465).
type SLAMetricService struct {
	db     database.Database
	repo   *repository.SLARepository
	engine *sla.Engine
}

// NewSLAMetricService constructs the metric service.
func NewSLAMetricService(db database.Database, engine *sla.Engine) *SLAMetricService {
	return &SLAMetricService{db: db, repo: repository.NewSLARepository(db), engine: engine}
}

// List returns a workspace's metrics.
func (s *SLAMetricService) List(ctx context.Context, workspaceID int) ([]models.SLAMetric, error) {
	return s.repo.ListMetrics(ctx, workspaceID)
}

// Get returns one metric, scoped to its workspace.
func (s *SLAMetricService) Get(ctx context.Context, workspaceID, metricID int) (*models.SLAMetric, error) {
	metric, err := s.repo.GetMetric(ctx, metricID)
	if err != nil {
		return nil, err
	}
	if metric.WorkspaceID != workspaceID {
		return nil, repository.ErrNotFound
	}
	return metric, nil
}

// Create validates and inserts a metric, then schedules recalculation.
func (s *SLAMetricService) Create(ctx context.Context, workspaceID int, input SLAMetricInput) (*models.SLAMetric, error) {
	metric, err := s.build(ctx, workspaceID, input)
	if err != nil {
		return nil, err
	}
	err = database.WithTx(s.db, func(tx database.Tx) error {
		if err := s.validateTargetsTx(ctx, tx, workspaceID, metric.Goals); err != nil {
			return err
		}
		id, err := s.repo.CreateMetric(ctx, tx, metric)
		if err != nil {
			return err
		}
		metric.ID = id
		return s.bumpAndRecalculate(ctx, tx, workspaceID, id)
	})
	if err != nil {
		return nil, err
	}
	s.engine.Wake()
	return metric, nil
}

// Update replaces a metric and schedules recalculation.
func (s *SLAMetricService) Update(ctx context.Context, workspaceID, metricID int, input SLAMetricInput) (*models.SLAMetric, error) {
	if _, err := s.Get(ctx, workspaceID, metricID); err != nil {
		return nil, err
	}
	metric, err := s.build(ctx, workspaceID, input)
	if err != nil {
		return nil, err
	}
	metric.ID = metricID
	err = database.WithTx(s.db, func(tx database.Tx) error {
		if err := s.validateTargetsTx(ctx, tx, workspaceID, metric.Goals); err != nil {
			return err
		}
		if err := s.repo.UpdateMetric(ctx, tx, metric); err != nil {
			return err
		}
		return s.bumpAndRecalculate(ctx, tx, workspaceID, metricID)
	})
	if err != nil {
		return nil, err
	}
	s.engine.Wake()
	return metric, nil
}

// Delete removes a metric; its cycles and jobs cascade.
func (s *SLAMetricService) Delete(ctx context.Context, workspaceID, metricID int) error {
	if _, err := s.Get(ctx, workspaceID, metricID); err != nil {
		return err
	}
	err := database.WithTx(s.db, func(tx database.Tx) error {
		if err := s.repo.DeleteMetric(ctx, tx, metricID); err != nil {
			return err
		}
		_, err := s.engine.BumpConfigGeneration(ctx, tx, workspaceID)
		return err
	})
	if err != nil {
		return err
	}
	s.engine.Wake()
	return nil
}

// EnqueueRecalculation schedules recalculation for one metric or, when
// metricID is zero, the whole workspace. It returns the number of metrics
// enqueued.
func (s *SLAMetricService) EnqueueRecalculation(ctx context.Context, workspaceID, metricID int) (int, error) {
	metricIDs := []int{metricID}
	if metricID != 0 {
		metric, err := s.repo.GetMetric(ctx, metricID)
		if err != nil {
			return 0, err
		}
		if metric.WorkspaceID != workspaceID {
			return 0, repository.ErrNotFound
		}
	}
	if metricID == 0 {
		metrics, err := s.repo.ListMetrics(ctx, workspaceID)
		if err != nil {
			return 0, err
		}
		metricIDs = metricIDs[:0]
		for _, metric := range metrics {
			metricIDs = append(metricIDs, metric.ID)
		}
	}
	err := database.WithTx(s.db, func(tx database.Tx) error {
		for _, id := range metricIDs {
			metric := id
			if err := s.repo.UpsertJob(ctx, tx, &models.SLAJob{Kind: models.SLAJobRecalcMetric, MetricID: &metric, DueAt: time.Now().UTC()}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	s.engine.Wake()
	return len(metricIDs), nil
}

func (s *SLAMetricService) build(ctx context.Context, workspaceID int, input SLAMetricInput) (*models.SLAMetric, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrSLAMetricInvalid)
	}
	displayFormat := input.DisplayFormat
	if displayFormat == "" {
		displayFormat = "time"
	}
	if displayFormat != "time" && displayFormat != "due_date" {
		return nil, fmt.Errorf("%w: display_format must be time or due_date", ErrSLAMetricInvalid)
	}
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}
	importStatus := input.ImportStatus
	if importStatus == "" {
		importStatus = "native"
	}
	metric := &models.SLAMetric{
		WorkspaceID:   workspaceID,
		Name:          name,
		DisplayFormat: displayFormat,
		Position:      input.Position,
		IsActive:      isActive,
		ImportStatus:  importStatus,
		Conditions:    input.Conditions,
		Goals:         input.Goals,
	}
	if err := validateSLAConditions(metric.Conditions); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSLAMetricInvalid, err)
	}
	if len(metric.Goals) == 0 {
		return nil, fmt.Errorf("%w: at least one goal is required", ErrSLAMetricInvalid)
	}
	if err := s.validateTargets(ctx, workspaceID, metric.Goals); err != nil {
		return nil, err
	}
	if err := validateNativeGoalQueries(metric.Goals); err != nil {
		return nil, err
	}
	return metric, nil
}

// validateNativeGoalQueries compiles every native goal's QL before the metric
// is persisted. Imported goals keep their original queries (validated through
// the import path) and are skipped here.
func validateNativeGoalQueries(goals []models.SLAGoal) error {
	for i := range goals {
		status := goals[i].ImportStatus
		if status != "" && !strings.EqualFold(status, "native") {
			continue
		}
		if err := sla.ValidateGoalQuery(goals[i].QLQuery); err != nil {
			return fmt.Errorf("%w: goal %d: %v", ErrSLAMetricInvalid, i, err)
		}
	}
	return nil
}

func validateSLAConditions(conditions []models.SLACondition) error {
	for i := range conditions {
		condition := &conditions[i]
		switch condition.Phase {
		case models.SLAPhaseStart, models.SLAPhasePause, models.SLAPhaseStop:
		default:
			return errors.New("condition phase must be start, pause, or stop")
		}
		switch condition.ConditionType {
		case "created", "status_entered", "status_exited", "status_current", "status_category_entered", "status_category_exited", "status_category_current", "assignee_set", "resolution_set", "comment_by_customer", "comment_by_agent":
		default:
			return fmt.Errorf("unsupported condition_type: %s", condition.ConditionType)
		}
		if err := sla.ValidateConditionConfig(*condition); err != nil {
			return err
		}
	}
	return nil
}

// validateTargets ensures every goal has exactly one usable target whose
// calendar belongs to the workspace or a bound team. A promise may never
// depend on an unauthorized calendar.
func (s *SLAMetricService) validateTargets(ctx context.Context, workspaceID int, goals []models.SLAGoal) error {
	for i := range goals {
		if len(goals[i].Targets) != 1 {
			return fmt.Errorf("%w: each goal needs exactly one target", ErrSLAMetricInvalid)
		}
		target := &goals[i].Targets[0]
		if target.CalendarID == 0 {
			return fmt.Errorf("%w: each goal target needs a calendar_id", ErrSLAMetricInvalid)
		}
		if target.TargetMs <= 0 {
			return fmt.Errorf("%w: each goal target needs a positive target_ms", ErrSLAMetricInvalid)
		}
		accessible, err := s.calendarAccessible(ctx, workspaceID, target.CalendarID)
		if err != nil {
			return err
		}
		if !accessible {
			return fmt.Errorf("%w: goal target references a calendar this workspace cannot use", ErrSLAMetricInvalid)
		}
	}
	return nil
}

func (s *SLAMetricService) calendarAccessible(ctx context.Context, workspaceID, calendarID int) (bool, error) {
	var accessible bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM working_calendars c
		WHERE c.id = ?
		  AND (c.workspace_id = ?
	       OR c.team_id IN (SELECT team_id FROM team_workspace_bindings WHERE workspace_id = ?))
	)`, calendarID, workspaceID, workspaceID).Scan(&accessible)
	if err != nil {
		return false, err
	}
	return accessible, nil
}

func (s *SLAMetricService) validateTargetsTx(ctx context.Context, tx database.Tx, workspaceID int, goals []models.SLAGoal) error {
	for _, goal := range goals {
		for _, target := range goal.Targets {
			accessible, err := s.repo.CalendarAccessibleToWorkspaceTx(ctx, tx, workspaceID, target.CalendarID)
			if err != nil {
				return err
			}
			if !accessible {
				return fmt.Errorf("%w: goal target references a calendar this workspace cannot use", ErrSLAMetricInvalid)
			}
		}
	}
	return nil
}

func (s *SLAMetricService) bumpAndRecalculate(ctx context.Context, tx database.Tx, workspaceID, metricID int) error {
	if _, err := s.engine.BumpConfigGeneration(ctx, tx, workspaceID); err != nil {
		return err
	}
	metric := metricID
	return s.repo.UpsertJob(ctx, tx, &models.SLAJob{Kind: models.SLAJobRecalcMetric, MetricID: &metric, DueAt: time.Now().UTC()})
}
