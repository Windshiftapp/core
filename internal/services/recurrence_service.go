package services

import (
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"

	"github.com/teambition/rrule-go"
)

// ErrRecurrenceConflict reports that an item already has a recurrence rule.
var ErrRecurrenceConflict = errors.New("recurrence rule already exists for this item")

// ErrRecurrenceWorkspaceLimit reports that a workspace has reached the hard
// recurrence-rule quota.
var ErrRecurrenceWorkspaceLimit = errors.New("workspace recurrence rule limit reached")

// MaxRecurrenceRulesPerWorkspace is enforced for every recurrence creation
// surface.
const MaxRecurrenceRulesPerWorkspace = 100

// MaxRecurrenceLeadTimeDays bounds how far one scheduler pass may look ahead.
const MaxRecurrenceLeadTimeDays = 365

// RecurrenceWorkspaceLimitMessage is the stable user-facing explanation
// returned by every recurrence creation surface when a workspace is full.
func RecurrenceWorkspaceLimitMessage() string {
	return fmt.Sprintf(
		"This workspace has reached the limit of %d recurrence rules",
		MaxRecurrenceRulesPerWorkspace,
	)
}

// RecurrenceValidationKind lets transports preserve their own error envelopes
// while sharing the recurrence validation rules.
type RecurrenceValidationKind string

const (
	RecurrenceMissingField RecurrenceValidationKind = "missing_field"
	RecurrenceInvalidInput RecurrenceValidationKind = "invalid_input"
)

// RecurrenceValidationError is a user-facing validation failure.
type RecurrenceValidationError struct {
	Kind    RecurrenceValidationKind
	Message string
}

func (e *RecurrenceValidationError) Error() string { return e.Message }

// AsRecurrenceValidationError unwraps recurrence validation failures.
func AsRecurrenceValidationError(err error) (*RecurrenceValidationError, bool) {
	var validationErr *RecurrenceValidationError
	ok := errors.As(err, &validationErr)
	return validationErr, ok
}

// RecurrenceGenerator is the scheduler boundary used for on-demand generation.
type RecurrenceGenerator interface {
	ForceGenerate(ruleID int) (int, error)
}

// RecurrenceService owns transport-neutral recurrence behavior.
type RecurrenceService struct {
	repo      *repository.RecurrenceRepository
	generator RecurrenceGenerator
	auditor   *logger.Auditor
}

// NewRecurrenceService creates a recurrence application service.
func NewRecurrenceService(repo *repository.RecurrenceRepository, generator RecurrenceGenerator, auditors ...*logger.Auditor) *RecurrenceService {
	service := &RecurrenceService{repo: repo, generator: generator}
	if len(auditors) > 0 {
		service.auditor = auditors[0]
	}
	return service
}

// RecurrenceInstances contains one page of generated instances.
type RecurrenceInstances struct {
	Items []*models.RecurrenceInstance
	Total int
}

// RecurrencePreview is the normalized result of previewing an RRULE.
type RecurrencePreview struct {
	RRule       string      `json:"rrule"`
	DtStart     time.Time   `json:"dtstart"`
	Occurrences []time.Time `json:"occurrences"`
}

// RecurrenceUpdate is a presence-aware recurrence rule mutation.
type RecurrenceUpdate struct {
	RRule             *string
	DtStart           *string
	DtEndSet          bool
	DtEnd             *string
	Timezone          *string
	LeadTimeDays      *int
	CopyAssignee      *bool
	CopyPriority      *bool
	CopyCustomFields  *bool
	CopyDescription   *bool
	StatusOnCreateSet bool
	StatusOnCreate    *int
	IsActive          *bool
}

// NewRecurrenceUpdate adapts the legacy update DTO to the shared mutation.
func NewRecurrenceUpdate(req models.UpdateRecurrenceRequest) RecurrenceUpdate {
	return RecurrenceUpdate{
		RRule: req.RRule, DtStart: req.DtStart, DtEndSet: req.DtEnd != nil, DtEnd: req.DtEnd,
		Timezone: req.Timezone, LeadTimeDays: req.LeadTimeDays,
		CopyAssignee: req.CopyAssignee, CopyPriority: req.CopyPriority,
		CopyCustomFields: req.CopyCustomFields, CopyDescription: req.CopyDescription,
		StatusOnCreateSet: req.StatusOnCreate != nil, StatusOnCreate: req.StatusOnCreate,
		IsActive: req.IsActive,
	}
}

// Get returns the rule attached to itemID.
func (s *RecurrenceService) Get(itemID int) (*models.RecurrenceRule, error) {
	return s.repo.GetByTemplateItemID(itemID)
}

// Create validates and persists a recurrence rule for an item.
func (s *RecurrenceService) Create(itemID, workspaceID, userID int, req models.CreateRecurrenceRequest, auditActors ...AuditActor) (*models.RecurrenceRule, error) {
	rule, err := buildRecurrenceRule(itemID, workspaceID, userID, req)
	if err != nil {
		return nil, err
	}
	ruleID, err := s.repo.CreateWithinWorkspaceLimit(rule, MaxRecurrenceRulesPerWorkspace)
	if errors.Is(err, repository.ErrRecurrenceRuleExists) {
		return nil, ErrRecurrenceConflict
	}
	if errors.Is(err, repository.ErrRecurrenceRuleLimitReached) {
		return nil, ErrRecurrenceWorkspaceLimit
	}
	if err != nil {
		return nil, err
	}
	created, err := s.repo.GetByID(ruleID)
	if err != nil {
		return nil, err
	}
	s.emitAudit(optionalAuditActor(auditActors), logger.ActionRecurrenceCreate, created, itemID, nil)
	return created, nil
}

// Update applies a partial update and returns the persisted rule.
func (s *RecurrenceService) Update(itemID int, req models.UpdateRecurrenceRequest, auditActors ...AuditActor) (*models.RecurrenceRule, error) {
	return s.UpdateWithPatch(itemID, NewRecurrenceUpdate(req), auditActors...)
}

// UpdateWithPatch applies a presence-aware partial update.
func (s *RecurrenceService) UpdateWithPatch(itemID int, req RecurrenceUpdate, auditActors ...AuditActor) (*models.RecurrenceRule, error) {
	rule, err := s.repo.GetByTemplateItemID(itemID)
	if err != nil {
		return nil, err
	}
	if err := applyRecurrenceUpdate(rule, req); err != nil {
		return nil, err
	}
	if err := s.repo.Update(rule); err != nil {
		return nil, err
	}
	updated, err := s.repo.GetByID(rule.ID)
	if err != nil {
		return nil, err
	}
	s.emitAudit(optionalAuditActor(auditActors), logger.ActionRecurrenceUpdate, updated, itemID, nil)
	return updated, nil
}

// Delete removes the rule attached to itemID.
func (s *RecurrenceService) Delete(itemID int, auditActors ...AuditActor) error {
	rule, err := s.repo.GetByTemplateItemID(itemID)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(rule.ID); err != nil {
		return err
	}
	s.emitAudit(optionalAuditActor(auditActors), logger.ActionRecurrenceDelete, rule, itemID, nil)
	return nil
}

// ListInstances returns generated instances and their total count.
func (s *RecurrenceService) ListInstances(itemID, limit, offset int) (*RecurrenceInstances, error) {
	rule, err := s.repo.GetByTemplateItemID(itemID)
	if err != nil {
		return nil, err
	}
	instances, err := s.repo.GetInstancesByRuleID(rule.ID, limit, offset)
	if err != nil {
		return nil, err
	}
	total, err := s.repo.CountInstancesByRuleID(rule.ID)
	if err != nil {
		return nil, err
	}
	return &RecurrenceInstances{Items: instances, Total: total}, nil
}

// ForceGenerate triggers immediate generation for the rule attached to itemID.
func (s *RecurrenceService) ForceGenerate(itemID int, auditActors ...AuditActor) (int, error) {
	rule, err := s.repo.GetByTemplateItemID(itemID)
	if err != nil {
		return 0, err
	}
	if s.generator == nil {
		return 0, errors.New("recurrence generator is not configured")
	}
	count, err := s.generator.ForceGenerate(rule.ID)
	if err != nil {
		return 0, err
	}
	s.emitAudit(optionalAuditActor(auditActors), logger.ActionRecurrenceForceGenerate, rule, itemID, map[string]any{"instances_generated": count})
	return count, nil
}

func (s *RecurrenceService) emitAudit(actor *AuditActor, action string, rule *models.RecurrenceRule, itemID int, details map[string]any) {
	if actor == nil || s.auditor == nil || rule == nil {
		return
	}
	if details == nil {
		details = map[string]any{}
	}
	details["item_id"] = itemID
	event := logger.AuditEvent{
		UserID:       actor.UserID,
		Username:     actor.Username,
		IPAddress:    actor.IPAddress,
		UserAgent:    actor.UserAgent,
		ActionType:   action,
		ResourceType: logger.ResourceRecurrenceRule,
		ResourceID:   &rule.ID,
		ResourceName: rule.RRule,
		Details:      mergeAuditDetails(details, *actor),
		Success:      true,
	}
	s.auditor.LogEvent(event)
}

// ListByWorkspace returns every recurrence rule in a workspace.
func (s *RecurrenceService) ListByWorkspace(workspaceID int) ([]*models.RecurrenceRule, error) {
	return s.repo.ListByWorkspace(workspaceID)
}

// Preview validates an RRULE and returns a bounded occurrence list.
func (s *RecurrenceService) Preview(req models.RRulePreviewRequest) (*RecurrencePreview, error) {
	if err := sanitizePreviewRequest(&req); err != nil {
		return nil, err
	}
	if req.RRule == "" {
		return nil, recurrenceMissing("rrule is required")
	}
	if req.DtStart == "" {
		return nil, recurrenceMissing("dtstart is required")
	}

	dtstart, err := parseRecurrenceDate(req.DtStart)
	if err != nil {
		return nil, recurrenceInvalid("Invalid dtstart format (use RFC3339 or YYYY-MM-DD)")
	}
	ruleOpt, err := rrule.StrToROption(req.RRule)
	if err != nil {
		return nil, recurrenceInvalid("Invalid RRULE format: " + err.Error())
	}
	ruleOpt.Dtstart = dtstart
	rule, err := rrule.NewRRule(*ruleOpt)
	if err != nil {
		return nil, recurrenceInvalid("Failed to create RRULE: " + err.Error())
	}

	count := 10
	if req.Count > 0 && req.Count <= 50 {
		count = req.Count
	}
	occurrences := make([]time.Time, 0, count)
	iterator := rule.Iterator()
	for len(occurrences) < count {
		occurrence, ok := iterator()
		if !ok {
			break
		}
		occurrences = append(occurrences, occurrence)
	}
	return &RecurrencePreview{RRule: req.RRule, DtStart: dtstart, Occurrences: occurrences}, nil
}

func buildRecurrenceRule(itemID, workspaceID, userID int, req models.CreateRecurrenceRequest) (*models.RecurrenceRule, error) {
	if err := sanitizeCreateRecurrenceRequest(&req); err != nil {
		return nil, err
	}
	if req.RRule == "" {
		return nil, recurrenceMissing("rrule is required")
	}
	if _, err := rrule.StrToROption(req.RRule); err != nil {
		return nil, recurrenceInvalid("Invalid RRULE format: " + err.Error())
	}
	if req.DtStart == "" {
		return nil, recurrenceMissing("dtstart is required")
	}
	dtstart, err := parseRecurrenceDate(req.DtStart)
	if err != nil {
		return nil, recurrenceInvalid("Invalid dtstart format (use RFC3339 or YYYY-MM-DD)")
	}

	var dtend *time.Time
	if req.DtEnd != nil && *req.DtEnd != "" {
		parsed, err := parseRecurrenceDate(*req.DtEnd)
		if err != nil {
			return nil, recurrenceInvalid("Invalid dtend format")
		}
		dtend = &parsed
	}

	timezone := "UTC"
	if req.Timezone != "" {
		timezone = req.Timezone
	}
	timezone, _, err = ResolveTimezone(timezone)
	if err != nil {
		return nil, recurrenceInvalid(err.Error())
	}
	leadTimeDays := valueOr(req.LeadTimeDays, 14)
	if err := validateRecurrenceLeadTime(leadTimeDays); err != nil {
		return nil, err
	}
	copyAssignee := valueOr(req.CopyAssignee, true)
	copyPriority := valueOr(req.CopyPriority, true)
	copyCustomFields := valueOr(req.CopyCustomFields, true)
	copyDescription := valueOr(req.CopyDescription, true)
	createdBy := userID

	return &models.RecurrenceRule{
		TemplateItemID:   itemID,
		WorkspaceID:      workspaceID,
		RRule:            req.RRule,
		DtStart:          dtstart,
		DtEnd:            dtend,
		Timezone:         timezone,
		LeadTimeDays:     leadTimeDays,
		CopyAssignee:     copyAssignee,
		CopyPriority:     copyPriority,
		CopyCustomFields: copyCustomFields,
		CopyDescription:  copyDescription,
		StatusOnCreate:   req.StatusOnCreate,
		IsActive:         true,
		CreatedBy:        &createdBy,
	}, nil
}

func applyRecurrenceUpdate(rule *models.RecurrenceRule, req RecurrenceUpdate) error {
	if err := sanitizeUpdateRecurrenceRequest(&req); err != nil {
		return err
	}
	if req.RRule != nil {
		if _, err := rrule.StrToROption(*req.RRule); err != nil {
			return recurrenceInvalid("Invalid RRULE format: " + err.Error())
		}
		rule.RRule = *req.RRule
	}
	if req.DtStart != nil {
		dtstart, err := parseRecurrenceDate(*req.DtStart)
		if err != nil {
			return recurrenceInvalid("Invalid dtstart format")
		}
		rule.DtStart = dtstart
	}
	if req.DtEndSet {
		if req.DtEnd == nil || *req.DtEnd == "" {
			rule.DtEnd = nil
		} else {
			dtend, err := parseRecurrenceDate(*req.DtEnd)
			if err != nil {
				return recurrenceInvalid("Invalid dtend format")
			}
			rule.DtEnd = &dtend
		}
	}
	if req.Timezone != nil {
		timezone, _, err := ResolveTimezone(*req.Timezone)
		if err != nil {
			return recurrenceInvalid(err.Error())
		}
		rule.Timezone = timezone
	}
	if req.LeadTimeDays != nil {
		if err := validateRecurrenceLeadTime(*req.LeadTimeDays); err != nil {
			return err
		}
		rule.LeadTimeDays = *req.LeadTimeDays
	}
	if req.CopyAssignee != nil {
		rule.CopyAssignee = *req.CopyAssignee
	}
	if req.CopyPriority != nil {
		rule.CopyPriority = *req.CopyPriority
	}
	if req.CopyCustomFields != nil {
		rule.CopyCustomFields = *req.CopyCustomFields
	}
	if req.CopyDescription != nil {
		rule.CopyDescription = *req.CopyDescription
	}
	if req.StatusOnCreateSet {
		rule.StatusOnCreate = req.StatusOnCreate
	}
	if req.IsActive != nil {
		rule.IsActive = *req.IsActive
	}
	return nil
}

func validateRecurrenceLeadTime(days int) error {
	if days < 0 || days > MaxRecurrenceLeadTimeDays {
		return recurrenceInvalid(fmt.Sprintf("lead_time_days must be between 0 and %d", MaxRecurrenceLeadTimeDays))
	}
	return nil
}

func sanitizeCreateRecurrenceRequest(req *models.CreateRecurrenceRequest) error {
	if err := validateRRuleLength(req.RRule); err != nil {
		return err
	}
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.RRule, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &req.Timezone, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &req.DtStart, Policy: sanitize.ShortIdentifier},
	)
	if req.DtEnd != nil {
		sanitize.Apply(req.DtEnd, sanitize.ShortIdentifier)
	}
	return nil
}

func sanitizeUpdateRecurrenceRequest(req *RecurrenceUpdate) error {
	if req.RRule != nil {
		if err := validateRRuleLength(*req.RRule); err != nil {
			return err
		}
	}
	sanitize.ApplyAll(
		sanitize.Pair{Target: req.RRule, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: req.Timezone, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: req.DtStart, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: req.DtEnd, Policy: sanitize.ShortIdentifier},
	)
	return nil
}

func sanitizePreviewRequest(req *models.RRulePreviewRequest) error {
	if err := validateRRuleLength(req.RRule); err != nil {
		return err
	}
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.RRule, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &req.DtStart, Policy: sanitize.ShortIdentifier},
	)
	return nil
}

func validateRRuleLength(value string) error {
	if utf8.RuneCountInString(value) > sanitize.ShortIdentifierMaxRunes {
		return recurrenceInvalid("rrule exceeds the maximum length")
	}
	return nil
}

func parseRecurrenceDate(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	return time.Parse("2006-01-02", value)
}

func recurrenceMissing(message string) error {
	return &RecurrenceValidationError{Kind: RecurrenceMissingField, Message: message}
}

func recurrenceInvalid(message string) error {
	return &RecurrenceValidationError{Kind: RecurrenceInvalidInput, Message: message}
}

func valueOr[T any](value *T, fallback T) T {
	if value != nil {
		return *value
	}
	return fallback
}
