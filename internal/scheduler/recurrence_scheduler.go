package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/validation"

	"github.com/teambition/rrule-go"
)

// DefaultRecurrenceBatchSize is the maximum number of due rules processed by
// one scheduler pass. Diagnostics use the same value when reporting backlog.
const DefaultRecurrenceBatchSize = 100

// MaxRecurrenceInstancesPerRulePass limits both recurrence expansion work and
// item creation for one rule in one scheduler pass.
const MaxRecurrenceInstancesPerRulePass = 100

// RecurrenceScheduler handles periodic generation of recurring task instances
type RecurrenceScheduler struct {
	db              database.Database
	recurrenceRepo  *repository.RecurrenceRepository
	itemRepo        *repository.ItemRepository
	runRepo         *repository.SchedulerRunRepository
	workflowService *services.WorkflowService
	ticker          *time.Ticker
	stopChan        chan struct{}
	mu              sync.RWMutex
	running         bool

	// Configuration
	checkInterval time.Duration
	batchSize     int
}

// NewRecurrenceScheduler creates a new recurrence scheduler
func NewRecurrenceScheduler(db database.Database, workflowService *services.WorkflowService) *RecurrenceScheduler {
	return &RecurrenceScheduler{
		db:              db,
		recurrenceRepo:  repository.NewRecurrenceRepository(db),
		itemRepo:        repository.NewItemRepository(db),
		runRepo:         repository.NewSchedulerRunRepository(db),
		workflowService: workflowService,
		stopChan:        make(chan struct{}),
		running:         false,
		checkInterval:   5 * time.Minute,
		batchSize:       DefaultRecurrenceBatchSize,
	}
}

// Start begins the recurrence scheduler
func (rs *RecurrenceScheduler) Start() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.running {
		return
	}

	rs.ticker = time.NewTicker(rs.checkInterval)
	rs.stopChan = make(chan struct{})
	rs.running = true
	slog.Info("Starting recurrence scheduler (5-minute interval)")

	go rs.schedulerLoop(rs.ticker, rs.stopChan)
}

// Stop stops the recurrence scheduler
func (rs *RecurrenceScheduler) Stop() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if !rs.running {
		return
	}

	rs.running = false
	if rs.ticker != nil {
		rs.ticker.Stop()
		rs.ticker = nil
	}
	close(rs.stopChan)
	slog.Info("Recurrence scheduler stopped")
}

// schedulerLoop runs the main scheduler loop
func (rs *RecurrenceScheduler) schedulerLoop(ticker *time.Ticker, stopChan <-chan struct{}) {
	// Run immediately on start
	rs.processRecurrenceRules()

	for {
		select {
		case <-ticker.C:
			rs.processRecurrenceRules()
		case <-stopChan:
			return
		}
	}
}

// processRecurrenceRules finds and processes active recurrence rules
func (rs *RecurrenceScheduler) processRecurrenceRules() {
	start := time.Now()
	var generatedCount int
	var runErr error
	defer recordSchedulerRun(rs.runRepo, "recurrence", start, &generatedCount, &runErr)

	slog.Debug("Processing recurrence rules...")

	// Get rules that need processing
	rules, err := rs.recurrenceRepo.GetRulesNeedingGeneration(rs.batchSize)
	if err != nil {
		slog.Error("Error fetching recurrence rules", "error", err)
		runErr = err
		return
	}

	if len(rules) == 0 {
		slog.Debug("No recurrence rules need processing")
		return
	}

	failures := 0
	for _, rule := range rules {
		count, err := rs.generateInstancesForRule(rule)
		if err != nil {
			slog.Error("Error generating instances for rule", "rule_id", rule.ID, "error", err)
			failures++
			continue
		}
		generatedCount += count
	}

	// Surface per-rule failures to scheduler_runs so the admin Diagnostics page
	// doesn't read "100% success rate" while individual rules are stuck failing.
	if failures > 0 {
		runErr = fmt.Errorf("%d of %d recurrence rules failed", failures, len(rules))
	}

	if generatedCount > 0 {
		slog.Info("Recurrence processing complete", "rules_processed", len(rules), "instances_generated", generatedCount)
	}
}

// generateInstancesForRule generates instances for a single recurrence rule
func (rs *RecurrenceScheduler) generateInstancesForRule(rule *models.RecurrenceRule) (int, error) {
	// Parse the RRULE
	ruleOpt, err := rrule.StrToROption(rule.RRule)
	if err != nil {
		return 0, fmt.Errorf("invalid rrule: %w", err)
	}

	// Honor the rule's stored timezone for occurrence expansion. We rebind dtstart
	// to the rule's location *without changing its instant* — this is what makes
	// BYDAY=MO etc. resolve to "Monday in the user's timezone" instead of "Monday in
	// whatever zone time.Time happened to be scanned in (typically UTC)". DST is
	// then handled correctly by rrule-go's tz-aware expansion.
	_, loc := services.ResolveTimezoneOrUTC(rule.Timezone)
	if loc == time.UTC && rule.Timezone != "" && rule.Timezone != "UTC" {
		slog.Warn("invalid rule timezone, falling back to UTC",
			"rule_id", rule.ID, "tz", rule.Timezone)
	}
	ruleOpt.Dtstart = rule.DtStart.In(loc)

	// Create the rrule
	r, err := rrule.NewRRule(*ruleOpt)
	if err != nil {
		return 0, fmt.Errorf("failed to create rrule: %w", err)
	}

	// Determine the generation window
	now := time.Now()
	startFrom := ruleOpt.Dtstart
	if rule.LastGeneratedUntil != nil && !rule.LastGeneratedUntil.IsZero() {
		startFrom = rule.LastGeneratedUntil.In(loc)
	}

	generateUntil := now.AddDate(0, 0, rule.LeadTimeDays)
	if rule.DtEnd != nil && rule.DtEnd.Before(generateUntil) {
		generateUntil = *rule.DtEnd
	}

	// Expand incrementally so high-frequency rules never materialize an
	// unbounded occurrence slice in memory.
	nextOccurrence := r.After(startFrom, true)
	if nextOccurrence.IsZero() || nextOccurrence.After(generateUntil) {
		// No occurrences in window, update next check time
		nextCheck := now.Add(24 * time.Hour)
		_ = rs.recurrenceRepo.UpdateNextCheck(rule.ID, nextCheck)
		return 0, nil
	}

	// Get template item
	templateItem, err := rs.itemRepo.FindByID(rule.TemplateItemID)
	if err != nil {
		return 0, fmt.Errorf("template item not found: %w", err)
	}

	// Get existing instance dates to avoid duplicates
	existingDates, err := rs.recurrenceRepo.GetExistingInstanceDates(rule.ID)
	if err != nil {
		return 0, fmt.Errorf("failed to get existing dates: %w", err)
	}

	// Walk occurrences in chronological order. We track the highest occurrence we've
	// definitively dealt with (created OR known-already-existing), and we BREAK on
	// the first failure rather than continuing — otherwise advancing
	// last_generated_until past a hole would permanently lose that occurrence (the
	// next tick's r.Between(startFrom, ..., true) excludes anything below startFrom,
	// and the dedup map won't contain the never-created date either).
	lastOK := startFrom
	var firstFailure error
	generatedCount := 0
	examinedCount := 0
	for !nextOccurrence.IsZero() && !nextOccurrence.After(generateUntil) && examinedCount < MaxRecurrenceInstancesPerRulePass {
		occurrence := nextOccurrence
		nextOccurrence = r.After(occurrence, false)
		examinedCount++
		dateKey := occurrence.Format("2006-01-02")
		if existingDates[dateKey] {
			if occurrence.After(lastOK) {
				lastOK = occurrence
			}
			continue
		}

		if err := rs.createInstance(rule, templateItem, occurrence); err != nil {
			slog.Error("error creating instance, halting batch to preserve retry semantics",
				"rule_id", rule.ID, "date", dateKey, "error", err)
			firstFailure = err
			break
		}

		if occurrence.After(lastOK) {
			lastOK = occurrence
		}
		generatedCount++
	}

	// On clean run, advance to generateUntil — that's the upper bound we've actually
	// reasoned about. On any failure, only advance to lastOK so the failed
	// occurrence is retried on the next tick. Retry sooner after a failure too.
	progressTo := generateUntil
	nextCheck := now.Add(24 * time.Hour)
	if firstFailure != nil {
		progressTo = lastOK
		nextCheck = now.Add(rs.checkInterval)
	} else if !nextOccurrence.IsZero() && !nextOccurrence.After(generateUntil) {
		progressTo = lastOK
		nextCheck = now.Add(rs.checkInterval)
	}
	_ = rs.recurrenceRepo.UpdateGenerationProgress(rule.ID, progressTo, nextCheck)

	if firstFailure != nil {
		return generatedCount, fmt.Errorf("instance creation failed mid-batch: %w", firstFailure)
	}
	return generatedCount, nil
}

// createInstance creates a single recurring task instance
func (rs *RecurrenceScheduler) createInstance(rule *models.RecurrenceRule, template *models.Item, scheduledDate time.Time) error {
	// Determine status. The user can pin one explicitly; otherwise resolve via the
	// workspace's configuration set → workflow → initial transition (the same path
	// used by handlers/portal.go and handlers/forms.go). The previous hard-coded
	// statusID=1 only worked in workspaces whose first status row happened to have
	// id=1 — anywhere else it produced wrong-status items or tripped the FK.
	var statusID int
	if rule.StatusOnCreate != nil {
		statusID = *rule.StatusOnCreate
	} else {
		resolved, err := rs.workflowService.GetInitialStatusIDCached(template.WorkspaceID, template.ItemTypeID)
		if err != nil {
			return fmt.Errorf("resolve initial status (ws=%d, itemType=%v): %w", template.WorkspaceID, template.ItemTypeID, err)
		}
		if resolved == nil {
			return fmt.Errorf("no initial status configured for workspace=%d itemType=%v — fix workflow before recurrence can generate", template.WorkspaceID, template.ItemTypeID)
		}
		statusID = *resolved
	}

	// Build the new item, copying template fields per the rule's flags. Insert
	// goes through ItemRepository so the items-table write lives in the
	// repository layer (same path scm/issue_sync.go uses), not in the scheduler.
	item := &models.Item{
		WorkspaceID: template.WorkspaceID,
		ItemTypeID:  template.ItemTypeID,
		Title:       template.Title,
		StatusID:    &statusID,
		DueDate:     &scheduledDate,
		IsTask:      template.IsTask,
		ParentID:    template.ParentID,
		CreatorID:   rule.CreatedBy,
	}
	if rule.CopyDescription {
		item.Description = template.Description
	}
	if rule.CopyAssignee {
		item.AssigneeID = template.AssigneeID
	}
	if rule.CopyPriority {
		item.PriorityID = template.PriorityID
	}
	if rule.CopyCustomFields {
		item.CustomFieldValues = template.CustomFieldValues
	}
	actorUserID := 0
	if rule.CreatedBy != nil {
		actorUserID = *rule.CreatedBy
	}
	if err := validation.ValidateTaskState(rs.db, item.WorkspaceID, actorUserID, item.IsTask, item.StatusID); err != nil {
		return fmt.Errorf("validate recurring item: %w", err)
	}

	ctx := context.Background()
	itemID, err := rs.itemRepo.CreateWithRetry(ctx, item, func(tx database.Tx, itemID int) error {
		seqNum, err := rs.recurrenceRepo.GetNextSequenceNumber(tx, rule.ID)
		if err != nil {
			return fmt.Errorf("get next recurrence sequence number: %w", err)
		}
		if err := rs.recurrenceRepo.CreateInstance(tx, &models.RecurrenceInstance{
			RecurrenceRuleID: rule.ID,
			InstanceItemID:   itemID,
			ScheduledDate:    scheduledDate,
			SequenceNumber:   seqNum,
		}); err != nil {
			return fmt.Errorf("create recurrence instance: %w", err)
		}
		created, err := rs.itemRepo.FindByIDForUpdate(tx, itemID)
		if err != nil {
			return fmt.Errorf("load recurring item for event: %w", err)
		}
		metadata := itemevents.System("recurrence")
		if rule.CreatedBy != nil {
			metadata = itemevents.User(*rule.CreatedBy, "recurrence")
		}
		metadata.OccurredAt = time.Now().UTC()
		if _, err := itemevents.NewRecorder(rs.db).Created(ctx, tx, created, nil, metadata); err != nil {
			return fmt.Errorf("record recurring item creation: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("create recurring item: %w", err)
	}

	// Live-update publish (WI-483): the recurrence instance committed. Announce
	// the new item and refresh the template parent's child list if any.
	services.PublishItemChange(itemID, services.ItemChangeCreated)
	if item.ParentID != nil {
		services.PublishItemChange(*item.ParentID, services.ItemChangeUpdated)
	}
	return nil
}

// ForceGenerate triggers immediate generation for a specific rule
func (rs *RecurrenceScheduler) ForceGenerate(ruleID int) (int, error) {
	rule, err := rs.recurrenceRepo.GetByID(ruleID)
	if err != nil {
		return 0, err
	}

	return rs.generateInstancesForRule(rule)
}
