package scheduler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"

	"github.com/teambition/rrule-go"
)

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
		ticker:          time.NewTicker(5 * time.Minute),
		stopChan:        make(chan struct{}),
		running:         false,
		checkInterval:   5 * time.Minute,
		batchSize:       100,
	}
}

// Start begins the recurrence scheduler
func (rs *RecurrenceScheduler) Start() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.running {
		return
	}

	rs.running = true
	slog.Info("Starting recurrence scheduler (5-minute interval)")

	go rs.schedulerLoop()
}

// Stop stops the recurrence scheduler
func (rs *RecurrenceScheduler) Stop() {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if !rs.running {
		return
	}

	rs.running = false
	rs.ticker.Stop()
	close(rs.stopChan)
	slog.Info("Recurrence scheduler stopped")
}

// schedulerLoop runs the main scheduler loop
func (rs *RecurrenceScheduler) schedulerLoop() {
	// Run immediately on start
	rs.processRecurrenceRules()

	for {
		select {
		case <-rs.ticker.C:
			rs.processRecurrenceRules()
		case <-rs.stopChan:
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

	for _, rule := range rules {
		count, err := rs.generateInstancesForRule(rule)
		if err != nil {
			slog.Error("Error generating instances for rule", "rule_id", rule.ID, "error", err)
			continue
		}
		generatedCount += count
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
	loc, err := time.LoadLocation(rule.Timezone)
	if err != nil || loc == nil {
		slog.Warn("invalid rule timezone, falling back to UTC",
			"rule_id", rule.ID, "tz", rule.Timezone, "error", err)
		loc = time.UTC
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

	// Get occurrences in the window
	occurrences := r.Between(startFrom, generateUntil, true)

	if len(occurrences) == 0 {
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
	for _, occurrence := range occurrences {
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
	}
	_ = rs.recurrenceRepo.UpdateGenerationProgress(rule.ID, progressTo, nextCheck)

	if firstFailure != nil {
		return generatedCount, fmt.Errorf("instance creation failed mid-batch: %w", firstFailure)
	}
	return generatedCount, nil
}

// createInstance creates a single recurring task instance
func (rs *RecurrenceScheduler) createInstance(rule *models.RecurrenceRule, template *models.Item, scheduledDate time.Time) error {
	tx, err := rs.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Get next workspace item number
	var nextNum int
	err = tx.QueryRow(`
		SELECT COALESCE(MAX(workspace_item_number), 0) + 1
		FROM items WHERE workspace_id = ?
	`, template.WorkspaceID).Scan(&nextNum)
	if err != nil {
		return fmt.Errorf("failed to get next item number: %w", err)
	}

	// Get next sequence number for this rule
	seqNum, err := rs.recurrenceRepo.GetNextSequenceNumber(tx, rule.ID)
	if err != nil {
		return fmt.Errorf("failed to get sequence number: %w", err)
	}

	// Build the new item - copy fields based on rule settings
	var description string
	if rule.CopyDescription {
		description = template.Description
	}

	var assigneeID, priorityID *int
	if rule.CopyAssignee {
		assigneeID = template.AssigneeID
	}
	if rule.CopyPriority {
		priorityID = template.PriorityID
	}

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

	// Handle custom field values
	var customFieldValuesJSON *string
	if rule.CopyCustomFields && len(template.CustomFieldValues) > 0 {
		var cfBytes []byte
		cfBytes, err = json.Marshal(template.CustomFieldValues)
		if err == nil {
			cfStr := string(cfBytes)
			customFieldValuesJSON = &cfStr
		}
	}

	// Insert the new item
	var itemID int64
	err = tx.QueryRow(`
		INSERT INTO items (
			workspace_id, workspace_item_number, item_type_id, title, description,
			status_id, priority_id, due_date, is_task, parent_id,
			assignee_id, custom_field_values, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`,
		template.WorkspaceID, nextNum, template.ItemTypeID, template.Title, description,
		statusID, priorityID, scheduledDate, template.IsTask, template.ParentID,
		assigneeID, customFieldValuesJSON, time.Now(), time.Now(),
	).Scan(&itemID)
	if err != nil {
		return fmt.Errorf("failed to create item: %w", err)
	}

	// Create the instance record
	err = rs.recurrenceRepo.CreateInstance(tx, &models.RecurrenceInstance{
		RecurrenceRuleID: rule.ID,
		InstanceItemID:   int(itemID),
		ScheduledDate:    scheduledDate,
		SequenceNumber:   seqNum,
	})
	if err != nil {
		return fmt.Errorf("failed to create instance record: %w", err)
	}

	return tx.Commit()
}

// ForceGenerate triggers immediate generation for a specific rule
func (rs *RecurrenceScheduler) ForceGenerate(ruleID int) (int, error) {
	rule, err := rs.recurrenceRepo.GetByID(ruleID)
	if err != nil {
		return 0, err
	}

	return rs.generateInstancesForRule(rule)
}
