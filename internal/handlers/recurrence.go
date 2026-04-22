package handlers

import (
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/scheduler"
	"windshift/internal/services"

	"github.com/teambition/rrule-go"
)

// RecurrenceHandler handles recurrence rule API endpoints
type RecurrenceHandler struct {
	db                database.Database
	recurrenceRepo    *repository.RecurrenceRepository
	itemRepo          *repository.ItemRepository
	scheduler         *scheduler.RecurrenceScheduler
	permissionService *services.PermissionService
}

// NewRecurrenceHandler creates a new recurrence handler
func NewRecurrenceHandler(db database.Database, sched *scheduler.RecurrenceScheduler, permissionService *services.PermissionService) *RecurrenceHandler {
	return &RecurrenceHandler{
		db:                db,
		recurrenceRepo:    repository.NewRecurrenceRepository(db),
		itemRepo:          repository.NewItemRepository(db),
		scheduler:         sched,
		permissionService: permissionService,
	}
}

// checkItemEditPermission checks if the current user can edit the given item
func (h *RecurrenceHandler) checkItemEditPermission(w http.ResponseWriter, r *http.Request, itemID int) bool {
	return CheckItemPermission(w, r, h.db, h.permissionService, itemID, models.PermissionItemEdit)
}

// resolveRuleForItem extracts the item ID from the URL, enforces permission, and
// loads the recurrence rule. It writes the appropriate HTTP response on any error
// and returns (nil, false) in that case.
func (h *RecurrenceHandler) resolveRuleForItem(w http.ResponseWriter, r *http.Request, permission string) (*models.RecurrenceRule, bool) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return nil, false
	}
	if !CheckItemPermission(w, r, h.db, h.permissionService, itemID, permission) {
		return nil, false
	}

	rule, err := h.recurrenceRepo.GetByTemplateItemID(itemID)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "recurrence_rule")
		return nil, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	return rule, true
}

// parseRecurrenceDate parses a date string in RFC3339 or YYYY-MM-DD form.
func parseRecurrenceDate(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", s)
}

// GetRecurrence gets the recurrence rule for an item
func (h *RecurrenceHandler) GetRecurrence(w http.ResponseWriter, r *http.Request) {
	rule, ok := h.resolveRuleForItem(w, r, models.PermissionItemView)
	if !ok {
		return
	}
	respondJSONOK(w, rule)
}

// CreateRecurrence creates a recurrence rule for an item
func (h *RecurrenceHandler) CreateRecurrence(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if !h.checkItemEditPermission(w, r, itemID) {
		return
	}

	// Check if rule already exists
	_, err := h.recurrenceRepo.GetByTemplateItemID(itemID)
	if err == nil {
		respondConflict(w, r, "Recurrence rule already exists for this item")
		return
	}
	if err != repository.ErrNotFound {
		respondInternalError(w, r, err)
		return
	}

	// Get the item to verify it exists and get workspace ID
	item, err := h.itemRepo.FindByID(itemID)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "item")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Parse request body
	req, ok := decodeJSON[models.CreateRecurrenceRequest](w, r)
	if !ok {
		return
	}

	// Validate RRULE
	if req.RRule == "" {
		respondValidationError(w, r, "rrule is required")
		return
	}
	if _, err = rrule.StrToROption(req.RRule); err != nil {
		respondValidationError(w, r, "Invalid RRULE format: "+err.Error())
		return
	}

	// Parse dtstart
	if req.DtStart == "" {
		respondValidationError(w, r, "dtstart is required")
		return
	}
	dtstart, err := parseRecurrenceDate(req.DtStart)
	if err != nil {
		respondValidationError(w, r, "Invalid dtstart format (use RFC3339 or YYYY-MM-DD)")
		return
	}

	// Parse optional dtend
	var dtend *time.Time
	if req.DtEnd != nil && *req.DtEnd != "" {
		t, err := parseRecurrenceDate(*req.DtEnd)
		if err != nil {
			respondValidationError(w, r, "Invalid dtend format")
			return
		}
		dtend = &t
	}

	// Get current user
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Build the rule with defaults
	timezone := "UTC"
	if req.Timezone != "" {
		timezone = req.Timezone
	}

	leadTimeDays := 14
	if req.LeadTimeDays != nil {
		leadTimeDays = *req.LeadTimeDays
	}

	copyAssignee := true
	if req.CopyAssignee != nil {
		copyAssignee = *req.CopyAssignee
	}

	copyPriority := true
	if req.CopyPriority != nil {
		copyPriority = *req.CopyPriority
	}

	copyCustomFields := true
	if req.CopyCustomFields != nil {
		copyCustomFields = *req.CopyCustomFields
	}

	copyDescription := true
	if req.CopyDescription != nil {
		copyDescription = *req.CopyDescription
	}

	rule := &models.RecurrenceRule{
		TemplateItemID:   itemID,
		WorkspaceID:      item.WorkspaceID,
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
		CreatedBy:        &currentUser.ID,
	}

	// Create the rule
	ruleID, err := h.recurrenceRepo.Create(rule)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Fetch the created rule with joined fields
	createdRule, err := h.recurrenceRepo.GetByID(ruleID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, createdRule)
}

// UpdateRecurrence updates a recurrence rule
func (h *RecurrenceHandler) UpdateRecurrence(w http.ResponseWriter, r *http.Request) {
	rule, ok := h.resolveRuleForItem(w, r, models.PermissionItemEdit)
	if !ok {
		return
	}

	// Parse request body
	req, ok := decodeJSON[models.UpdateRecurrenceRequest](w, r)
	if !ok {
		return
	}

	// Apply updates
	if req.RRule != nil {
		if _, err := rrule.StrToROption(*req.RRule); err != nil {
			respondValidationError(w, r, "Invalid RRULE format: "+err.Error())
			return
		}
		rule.RRule = *req.RRule
	}

	if req.DtStart != nil {
		dtstart, err := parseRecurrenceDate(*req.DtStart)
		if err != nil {
			respondValidationError(w, r, "Invalid dtstart format")
			return
		}
		rule.DtStart = dtstart
	}

	if req.DtEnd != nil {
		if *req.DtEnd == "" {
			rule.DtEnd = nil
		} else {
			t, err := parseRecurrenceDate(*req.DtEnd)
			if err != nil {
				respondValidationError(w, r, "Invalid dtend format")
				return
			}
			rule.DtEnd = &t
		}
	}

	if req.Timezone != nil {
		rule.Timezone = *req.Timezone
	}
	if req.LeadTimeDays != nil {
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
	if req.StatusOnCreate != nil {
		rule.StatusOnCreate = req.StatusOnCreate
	}
	if req.IsActive != nil {
		rule.IsActive = *req.IsActive
	}

	// Save updates
	if err := h.recurrenceRepo.Update(rule); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Fetch updated rule
	updatedRule, err := h.recurrenceRepo.GetByID(rule.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, updatedRule)
}

// DeleteRecurrence deletes a recurrence rule
func (h *RecurrenceHandler) DeleteRecurrence(w http.ResponseWriter, r *http.Request) {
	rule, ok := h.resolveRuleForItem(w, r, models.PermissionItemEdit)
	if !ok {
		return
	}

	if err := h.recurrenceRepo.Delete(rule.ID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListInstances lists generated instances for a recurrence rule
func (h *RecurrenceHandler) ListInstances(w http.ResponseWriter, r *http.Request) {
	rule, ok := h.resolveRuleForItem(w, r, models.PermissionItemView)
	if !ok {
		return
	}

	// Parse pagination
	limit := 20
	offset := 0
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
			if limit > 100 {
				limit = 100
			}
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Get instances
	instances, err := h.recurrenceRepo.GetInstancesByRuleID(rule.ID, limit, offset)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Get total count
	total, err := h.recurrenceRepo.CountInstancesByRuleID(rule.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	response := map[string]interface{}{
		"instances": instances,
		"pagination": map[string]int{
			"limit":  limit,
			"offset": offset,
			"total":  total,
		},
	}

	respondJSONOK(w, response)
}

// ForceGenerate forces immediate generation for a rule
func (h *RecurrenceHandler) ForceGenerate(w http.ResponseWriter, r *http.Request) {
	rule, ok := h.resolveRuleForItem(w, r, models.PermissionItemEdit)
	if !ok {
		return
	}

	count, err := h.scheduler.ForceGenerate(rule.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"instances_generated": count,
	})
}

// ListByWorkspace lists all recurrence rules for a workspace
func (h *RecurrenceHandler) ListByWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	hasPermission, err := h.permissionService.HasWorkspacePermission(currentUser.ID, workspaceID, models.PermissionItemView)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !hasPermission {
		respondNotFound(w, r, "workspace")
		return
	}

	rules, err := h.recurrenceRepo.ListByWorkspace(workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, rules)
}

// PreviewRRule previews RRULE occurrences
func (h *RecurrenceHandler) PreviewRRule(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[models.RRulePreviewRequest](w, r)
	if !ok {
		return
	}

	if req.RRule == "" {
		respondValidationError(w, r, "rrule is required")
		return
	}

	if req.DtStart == "" {
		respondValidationError(w, r, "dtstart is required")
		return
	}

	// Parse dtstart
	dtstart, err := parseRecurrenceDate(req.DtStart)
	if err != nil {
		respondValidationError(w, r, "Invalid dtstart format")
		return
	}

	// Parse RRULE
	ruleOpt, err := rrule.StrToROption(req.RRule)
	if err != nil {
		respondValidationError(w, r, "Invalid RRULE format: "+err.Error())
		return
	}
	ruleOpt.Dtstart = dtstart

	rule, err := rrule.NewRRule(*ruleOpt)
	if err != nil {
		respondValidationError(w, r, "Failed to create RRULE: "+err.Error())
		return
	}

	// Get preview count
	count := 10
	if req.Count > 0 && req.Count <= 50 {
		count = req.Count
	}

	// Get occurrences
	occurrences := rule.All()
	if len(occurrences) > count {
		occurrences = occurrences[:count]
	}

	// Format for response
	dates := make([]string, len(occurrences))
	for i, t := range occurrences {
		dates[i] = t.Format(time.RFC3339)
	}

	response := map[string]interface{}{
		"rrule":       req.RRule,
		"dtstart":     dtstart.Format(time.RFC3339),
		"occurrences": dates,
	}

	respondJSONOK(w, response)
}
