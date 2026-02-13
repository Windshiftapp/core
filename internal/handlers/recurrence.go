package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/teambition/rrule-go"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/scheduler"
	"windshift/internal/services"
	"windshift/internal/utils"
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

// GetRecurrence gets the recurrence rule for an item
func (h *RecurrenceHandler) GetRecurrence(w http.ResponseWriter, r *http.Request) {
	itemIDStr := r.PathValue("id")
	itemID, err := strconv.Atoi(itemIDStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if !CheckItemPermission(w, r, h.db, h.permissionService, itemID, models.PermissionItemView) {
		return
	}

	rule, err := h.recurrenceRepo.GetByTemplateItemID(itemID)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "recurrence_rule")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rule)
}

// CreateRecurrence creates a recurrence rule for an item
func (h *RecurrenceHandler) CreateRecurrence(w http.ResponseWriter, r *http.Request) {
	itemIDStr := r.PathValue("id")
	itemID, err := strconv.Atoi(itemIDStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if !h.checkItemEditPermission(w, r, itemID) {
		return
	}

	// Check if rule already exists
	_, err = h.recurrenceRepo.GetByTemplateItemID(itemID)
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
	var req models.CreateRecurrenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate RRULE
	if req.RRule == "" {
		respondValidationError(w, r, "rrule is required")
		return
	}
	if _, err := rrule.StrToROption(req.RRule); err != nil {
		respondValidationError(w, r, "Invalid RRULE format: "+err.Error())
		return
	}

	// Parse dtstart
	if req.DtStart == "" {
		respondValidationError(w, r, "dtstart is required")
		return
	}
	dtstart, err := time.Parse(time.RFC3339, req.DtStart)
	if err != nil {
		dtstart, err = time.Parse("2006-01-02", req.DtStart)
		if err != nil {
			respondValidationError(w, r, "Invalid dtstart format (use RFC3339 or YYYY-MM-DD)")
			return
		}
	}

	// Parse optional dtend
	var dtend *time.Time
	if req.DtEnd != nil && *req.DtEnd != "" {
		t, err := time.Parse(time.RFC3339, *req.DtEnd)
		if err != nil {
			t, err = time.Parse("2006-01-02", *req.DtEnd)
			if err != nil {
				respondValidationError(w, r, "Invalid dtend format")
				return
			}
		}
		dtend = &t
	}

	// Get current user
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(createdRule)
}

// UpdateRecurrence updates a recurrence rule
func (h *RecurrenceHandler) UpdateRecurrence(w http.ResponseWriter, r *http.Request) {
	itemIDStr := r.PathValue("id")
	itemID, err := strconv.Atoi(itemIDStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if !h.checkItemEditPermission(w, r, itemID) {
		return
	}

	// Get existing rule
	rule, err := h.recurrenceRepo.GetByTemplateItemID(itemID)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "recurrence_rule")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Parse request body
	var req models.UpdateRecurrenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
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
		dtstart, err := time.Parse(time.RFC3339, *req.DtStart)
		if err != nil {
			dtstart, err = time.Parse("2006-01-02", *req.DtStart)
			if err != nil {
				respondValidationError(w, r, "Invalid dtstart format")
				return
			}
		}
		rule.DtStart = dtstart
	}

	if req.DtEnd != nil {
		if *req.DtEnd == "" {
			rule.DtEnd = nil
		} else {
			t, err := time.Parse(time.RFC3339, *req.DtEnd)
			if err != nil {
				t, err = time.Parse("2006-01-02", *req.DtEnd)
				if err != nil {
					respondValidationError(w, r, "Invalid dtend format")
					return
				}
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedRule)
}

// DeleteRecurrence deletes a recurrence rule
func (h *RecurrenceHandler) DeleteRecurrence(w http.ResponseWriter, r *http.Request) {
	itemIDStr := r.PathValue("id")
	itemID, err := strconv.Atoi(itemIDStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if !h.checkItemEditPermission(w, r, itemID) {
		return
	}

	// Get the rule first
	rule, err := h.recurrenceRepo.GetByTemplateItemID(itemID)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "recurrence_rule")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Delete the rule
	if err := h.recurrenceRepo.Delete(rule.ID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListInstances lists generated instances for a recurrence rule
func (h *RecurrenceHandler) ListInstances(w http.ResponseWriter, r *http.Request) {
	itemIDStr := r.PathValue("id")
	itemID, err := strconv.Atoi(itemIDStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if !CheckItemPermission(w, r, h.db, h.permissionService, itemID, models.PermissionItemView) {
		return
	}

	// Get the rule
	rule, err := h.recurrenceRepo.GetByTemplateItemID(itemID)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "recurrence_rule")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ForceGenerate forces immediate generation for a rule
func (h *RecurrenceHandler) ForceGenerate(w http.ResponseWriter, r *http.Request) {
	itemIDStr := r.PathValue("id")
	itemID, err := strconv.Atoi(itemIDStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if !h.checkItemEditPermission(w, r, itemID) {
		return
	}

	// Get the rule
	rule, err := h.recurrenceRepo.GetByTemplateItemID(itemID)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "recurrence_rule")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Force generation
	count, err := h.scheduler.ForceGenerate(rule.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	response := map[string]interface{}{
		"instances_generated": count,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// PreviewRRule previews RRULE occurrences
func (h *RecurrenceHandler) PreviewRRule(w http.ResponseWriter, r *http.Request) {
	var req models.RRulePreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
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
	dtstart, err := time.Parse(time.RFC3339, req.DtStart)
	if err != nil {
		dtstart, err = time.Parse("2006-01-02", req.DtStart)
		if err != nil {
			respondValidationError(w, r, "Invalid dtstart format")
			return
		}
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
