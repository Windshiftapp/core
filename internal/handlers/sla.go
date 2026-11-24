package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/services"
	"windshift/internal/sla"
)

// SLAHandler serves SLA configuration and item SLA state.
type SLAHandler struct {
	db                database.Database
	repo              *repository.SLARepository
	teamRepo          *repository.TeamRepository
	engine            *sla.Engine
	calendars         *services.SLACalendarService
	settings          *services.SLASettingsService
	bindings          *services.SLATeamBindingService
	metrics           *services.SLAMetricService
	slaImport         *services.SLAImportService
	permissionService *services.PermissionService
}

// NewSLAHandler constructs an SLAHandler.
func NewSLAHandler(db database.Database, engine *sla.Engine, permissionService *services.PermissionService) *SLAHandler {
	return &SLAHandler{
		db:                db,
		repo:              repository.NewSLARepository(db),
		teamRepo:          repository.NewTeamRepository(db),
		engine:            engine,
		calendars:         services.NewSLACalendarService(db, engine),
		settings:          services.NewSLASettingsService(db, engine),
		bindings:          services.NewSLATeamBindingService(db, permissionService),
		metrics:           services.NewSLAMetricService(db, engine),
		slaImport:         services.NewSLAImportService(db, engine),
		permissionService: permissionService,
	}
}

// ---------------------------------------------------------------------------
// Item SLA state
// ---------------------------------------------------------------------------

// GetItemSLA returns the Jira-shaped SLA state for an item. It never writes.
func (h *SLAHandler) GetItemSLA(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	ctx := r.Context()
	var workspaceID int
	if err := h.db.QueryRowContext(ctx, `SELECT workspace_id FROM items WHERE id = ?`, itemID).Scan(&workspaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(w, r, "item")
			return
		}
		respondError(w, r, slaInternal(err))
		return
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionItemView, h.permissionService) {
		return
	}
	states, err := h.engine.ItemSLA(ctx, itemID, workspaceID)
	if err != nil {
		respondError(w, r, slaInternal(err))
		return
	}
	respondJSONOK(w, states)
}

// maxBatchItemSLAIDs bounds one batch read.
const batchItemSLAIDLimit = 200

// GetWorkspaceItemsSLA returns SLA state for many items of one workspace in a
// single request, so list and board badges amortize the workspace-level
// metric, threshold, and coverage loads instead of paying them per row
// (WI-1591). Ids outside the workspace are ignored.
func (h *SLAHandler) GetWorkspaceItemsSLA(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionItemView, h.permissionService) {
		return
	}
	ids, idErr := parseItemIDs(r)
	if idErr != nil {
		respondError(w, r, idErr)
		return
	}
	// Restrict the batch to items actually in the workspace.
	scoped, err := h.filterWorkspaceItemIDs(r.Context(), workspaceID, ids)
	if err != nil {
		respondError(w, r, slaInternal(err))
		return
	}
	states, err := h.engine.ItemsSLABadges(r.Context(), workspaceID, scoped)
	if err != nil {
		respondError(w, r, slaInternal(err))
		return
	}
	respondJSONOK(w, states)
}

// parseItemIDs reads the comma-separated ids query parameter.
func parseItemIDs(r *http.Request) ([]int, *restapi.APIError) {
	raw := strings.TrimSpace(r.URL.Query().Get("ids"))
	if raw == "" {
		return nil, slaInvalidInput("ids is required")
	}
	seen := make(map[int]bool)
	ids := make([]int, 0)
	for _, part := range strings.Split(raw, ",") {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value <= 0 {
			return nil, slaInvalidInput("ids must be positive integers")
		}
		if !seen[value] {
			seen[value] = true
			ids = append(ids, value)
		}
	}
	if len(ids) > batchItemSLAIDLimit {
		return nil, slaInvalidInput(fmt.Sprintf("ids accepts at most %d values", batchItemSLAIDLimit))
	}
	return ids, nil
}

func slaInvalidInput(message string) *restapi.APIError {
	return restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, message)
}

// filterWorkspaceItemIDs resolves which of the requested ids exist in the
// workspace, preserving request order.
func (h *SLAHandler) filterWorkspaceItemIDs(ctx context.Context, workspaceID int, ids []int) ([]int, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, workspaceID)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := h.db.QueryContext(ctx,
		`SELECT id FROM items WHERE workspace_id = ? AND id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	present := make(map[int]bool, len(ids))
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		present[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if present[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Workspace calendars
// ---------------------------------------------------------------------------

type slaCalendarRequest struct {
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Timezone        string          `json:"timezone"`
	WeeklyIntervals json.RawMessage `json:"weekly_intervals"`
	Holidays        json.RawMessage `json:"holidays"`
	IsDefault       bool            `json:"is_default"`
	ApplyToOngoing  bool            `json:"apply_to_ongoing"`
}

// ListWorkspaceCalendars returns a workspace's owned calendars.
func (h *SLAHandler) ListWorkspaceCalendars(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	calendars, err := h.calendars.ListWorkspace(r.Context(), workspaceID)
	if err != nil {
		respondError(w, r, slaInternal(err))
		return
	}
	respondJSONOK(w, calendars)
}

// ListAvailableCalendars returns workspace calendars plus team calendars
// authorized through an active binding.
func (h *SLAHandler) ListAvailableCalendars(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	calendars, err := h.calendars.ListAvailable(r.Context(), workspaceID)
	if err != nil {
		respondError(w, r, slaInternal(err))
		return
	}
	respondJSONOK(w, calendars)
}

// CreateWorkspaceCalendar creates a workspace-owned calendar.
func (h *SLAHandler) CreateWorkspaceCalendar(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	request, ok := decodeJSON[slaCalendarRequest](w, r)
	if !ok {
		return
	}
	calendar, err := h.calendars.CreateWorkspace(r.Context(), workspaceID, request.input())
	if !h.writeCalendarResult(w, r, err) {
		return
	}
	respondJSONCreated(w, calendar)
}

// UpdateWorkspaceCalendar updates a workspace-owned calendar.
func (h *SLAHandler) UpdateWorkspaceCalendar(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	calendarID, ok := requireIDParam(w, r, "calendarId")
	if !ok {
		return
	}
	request, ok := decodeJSON[slaCalendarRequest](w, r)
	if !ok {
		return
	}
	calendar, err := h.calendars.UpdateWorkspace(r.Context(), workspaceID, calendarID, request.input())
	if !h.writeCalendarResult(w, r, err) {
		return
	}
	respondJSONOK(w, calendar)
}

// DeleteWorkspaceCalendar deletes a workspace-owned calendar.
func (h *SLAHandler) DeleteWorkspaceCalendar(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	calendarID, ok := requireIDParam(w, r, "calendarId")
	if !ok {
		return
	}
	if !h.writeCalendarDelete(w, r, h.calendars.DeleteWorkspace(r.Context(), workspaceID, calendarID)) {
		return
	}
	respondJSONOK(w, map[string]bool{"deleted": true})
}

// ---------------------------------------------------------------------------
// Team calendars
// ---------------------------------------------------------------------------

// ListTeamCalendars returns a team's shared calendars.
func (h *SLAHandler) ListTeamCalendars(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.authorizeTeamAdmin(w, r)
	if !ok {
		return
	}
	calendars, err := h.calendars.ListTeam(r.Context(), teamID)
	if err != nil {
		respondError(w, r, slaInternal(err))
		return
	}
	respondJSONOK(w, calendars)
}

// CreateTeamCalendar creates a team-shared calendar.
func (h *SLAHandler) CreateTeamCalendar(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.authorizeTeamAdmin(w, r)
	if !ok {
		return
	}
	request, ok := decodeJSON[slaCalendarRequest](w, r)
	if !ok {
		return
	}
	calendar, err := h.calendars.CreateTeam(r.Context(), teamID, request.input())
	if !h.writeCalendarResult(w, r, err) {
		return
	}
	respondJSONCreated(w, calendar)
}

// UpdateTeamCalendar updates a team-shared calendar.
func (h *SLAHandler) UpdateTeamCalendar(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.authorizeTeamAdmin(w, r)
	if !ok {
		return
	}
	calendarID, ok := requireIDParam(w, r, "calendarId")
	if !ok {
		return
	}
	request, ok := decodeJSON[slaCalendarRequest](w, r)
	if !ok {
		return
	}
	calendar, err := h.calendars.UpdateTeam(r.Context(), teamID, calendarID, request.input())
	if !h.writeCalendarResult(w, r, err) {
		return
	}
	respondJSONOK(w, calendar)
}

// DeleteTeamCalendar deletes a team-shared calendar.
func (h *SLAHandler) DeleteTeamCalendar(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.authorizeTeamAdmin(w, r)
	if !ok {
		return
	}
	calendarID, ok := requireIDParam(w, r, "calendarId")
	if !ok {
		return
	}
	if !h.writeCalendarDelete(w, r, h.calendars.DeleteTeam(r.Context(), teamID, calendarID)) {
		return
	}
	respondJSONOK(w, map[string]bool{"deleted": true})
}

// GetWorkspaceCalendarImpact previews what editing a workspace-visible
// calendar would recalculate, scoped to that workspace.
func (h *SLAHandler) GetWorkspaceCalendarImpact(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	calendarID, ok := requireIDParam(w, r, "calendarId")
	if !ok {
		return
	}
	impact, err := h.calendars.WorkspaceCalendarImpact(r.Context(), workspaceID, calendarID)
	if !h.writeCalendarResult(w, r, err) {
		return
	}
	respondJSONOK(w, impact)
}

// GetTeamCalendarImpact previews what editing a team calendar would
// recalculate across every bound workspace.
func (h *SLAHandler) GetTeamCalendarImpact(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.authorizeTeamAdmin(w, r)
	if !ok {
		return
	}
	calendarID, ok := requireIDParam(w, r, "calendarId")
	if !ok {
		return
	}
	impact, err := h.calendars.TeamCalendarImpact(r.Context(), teamID, calendarID)
	if !h.writeCalendarResult(w, r, err) {
		return
	}
	respondJSONOK(w, impact)
}

// GetCoveragePreview returns the informative SLA-vs-team-service-hours
// comparison for a candidate calendar. Read-only; it never blocks a save.
func (h *SLAHandler) GetCoveragePreview(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	calendarID, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("calendar_id")))
	if err != nil || calendarID <= 0 {
		respondValidationError(w, r, "calendar_id must be a positive integer")
		return
	}
	coverage, err := h.calendars.PreviewCoverage(r.Context(), workspaceID, calendarID)
	if !h.writeCalendarResult(w, r, err) {
		return
	}
	respondJSONOK(w, coverage)
}

// input maps the transport request onto the shared calendar service input.
func (request slaCalendarRequest) input() services.SLACalendarInput {
	return services.SLACalendarInput{
		Name:            request.Name,
		Description:     request.Description,
		Timezone:        request.Timezone,
		WeeklyIntervals: request.WeeklyIntervals,
		Holidays:        request.Holidays,
		IsDefault:       request.IsDefault,
		ApplyToOngoing:  request.ApplyToOngoing,
	}
}

// writeCalendarResult maps service errors to responses. It returns true when
// the caller should write the success body.
func (h *SLAHandler) writeCalendarResult(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, services.ErrSLACalendarInvalid):
		respondValidationError(w, r, err.Error())
	case errors.Is(err, repository.ErrNotFound):
		respondNotFound(w, r, "calendar")
	case errors.Is(err, repository.ErrSLAInUse):
		respondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeConflict, err.Error()))
	default:
		respondError(w, r, slaInternal(err))
	}
	return false
}

func (h *SLAHandler) writeCalendarDelete(w http.ResponseWriter, r *http.Request, err error) bool {
	return h.writeCalendarResult(w, r, err)
}

// ---------------------------------------------------------------------------
// Team-workspace bindings
// ---------------------------------------------------------------------------

type slaBindingRequest struct {
	TeamID      int `json:"team_id"`
	WorkspaceID int `json:"workspace_id"`
}

// ListWorkspaceTeamBindings returns the teams bound to a workspace.
func (h *SLAHandler) ListWorkspaceTeamBindings(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	bindings, err := h.bindings.ListForWorkspace(r.Context(), workspaceID)
	if err != nil {
		respondError(w, r, slaInternal(err))
		return
	}
	respondJSONOK(w, bindings)
}

// CreateWorkspaceTeamBinding binds a team to a workspace after both sides
// consent.
func (h *SLAHandler) CreateWorkspaceTeamBinding(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	request, ok := decodeJSON[slaBindingRequest](w, r)
	if !ok {
		return
	}
	binding, err := h.bindings.Create(r.Context(), user.ID, request.TeamID, workspaceID)
	if !h.writeBindingResult(w, r, err) {
		return
	}
	respondJSONCreated(w, binding)
}

// DeleteWorkspaceTeamBinding removes a workspace-scoped binding after both
// sides consent.
func (h *SLAHandler) DeleteWorkspaceTeamBinding(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	bindingID, ok := requireIDParam(w, r, "bindingId")
	if !ok {
		return
	}
	if !h.writeBindingResult(w, r, h.bindings.DeleteForWorkspace(r.Context(), user.ID, workspaceID, bindingID)) {
		return
	}
	respondJSONOK(w, map[string]bool{"deleted": true})
}

// ListTeamWorkspaceBindings returns the workspaces bound to a team.
func (h *SLAHandler) ListTeamWorkspaceBindings(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.authorizeTeamAdmin(w, r)
	if !ok {
		return
	}
	bindings, err := h.bindings.ListForTeam(r.Context(), teamID)
	if err != nil {
		respondError(w, r, slaInternal(err))
		return
	}
	respondJSONOK(w, bindings)
}

// CreateTeamWorkspaceBinding binds a workspace to a team after both sides
// consent.
func (h *SLAHandler) CreateTeamWorkspaceBinding(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.authorizeTeamAdmin(w, r)
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	request, ok := decodeJSON[slaBindingRequest](w, r)
	if !ok {
		return
	}
	binding, err := h.bindings.Create(r.Context(), user.ID, teamID, request.WorkspaceID)
	if !h.writeBindingResult(w, r, err) {
		return
	}
	respondJSONCreated(w, binding)
}

// DeleteTeamWorkspaceBinding removes a team-scoped binding after both sides
// consent.
func (h *SLAHandler) DeleteTeamWorkspaceBinding(w http.ResponseWriter, r *http.Request) {
	teamID, ok := h.authorizeTeamAdmin(w, r)
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	bindingID, ok := requireIDParam(w, r, "bindingId")
	if !ok {
		return
	}
	if !h.writeBindingResult(w, r, h.bindings.DeleteForTeam(r.Context(), user.ID, teamID, bindingID)) {
		return
	}
	respondJSONOK(w, map[string]bool{"deleted": true})
}

// writeBindingResult maps binding errors to responses. It returns true when
// the caller should write the success body.
func (h *SLAHandler) writeBindingResult(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, services.ErrSLABindingForbidden):
		respondError(w, r, restapi.NewAPIError(http.StatusForbidden, restapi.ErrCodeForbidden, err.Error()))
	case errors.Is(err, repository.ErrDuplicateEntry):
		respondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeConflict, "team is already bound to this workspace"))
	case errors.Is(err, repository.ErrNotFound):
		respondNotFound(w, r, "binding")
	case errors.Is(err, repository.ErrSLAInUse):
		respondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeConflict, err.Error()))
	default:
		respondError(w, r, slaInternal(err))
	}
	return false
}

// ---------------------------------------------------------------------------
// Warning thresholds
// ---------------------------------------------------------------------------

type slaWarningThresholdRequest struct {
	Label    string `json:"label"`
	Percent  int    `json:"percent"`
	MetricID *int   `json:"metric_id"`
	IsActive *bool  `json:"is_active"`
}

func (request slaWarningThresholdRequest) input() services.SLAWarningThresholdInput {
	isActive := true
	if request.IsActive != nil {
		isActive = *request.IsActive
	}
	return services.SLAWarningThresholdInput{
		Label:    request.Label,
		Percent:  request.Percent,
		MetricID: request.MetricID,
		IsActive: isActive,
	}
}

// ListWarningThresholds returns a workspace's SLA warning thresholds. Item
// viewers may read them: the warning badge on lists and boards is part of the
// item surface, and agents hold item.view without workspace administration
// (WI-1578). Mutations stay admin-gated.
func (h *SLAHandler) ListWarningThresholds(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionItemView, h.permissionService) {
		return
	}
	thresholds, err := h.settings.ListWarningThresholds(r.Context(), workspaceID)
	if err != nil {
		respondError(w, r, slaInternal(err))
		return
	}
	respondJSONOK(w, thresholds)
}

// CreateWarningThreshold creates a warning threshold.
func (h *SLAHandler) CreateWarningThreshold(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	request, ok := decodeJSON[slaWarningThresholdRequest](w, r)
	if !ok {
		return
	}
	threshold, err := h.settings.CreateWarningThreshold(r.Context(), workspaceID, request.input())
	if !h.writeThresholdResult(w, r, err) {
		return
	}
	respondJSONCreated(w, threshold)
}

// UpdateWarningThreshold updates a warning threshold.
func (h *SLAHandler) UpdateWarningThreshold(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	thresholdID, ok := requireIDParam(w, r, "thresholdId")
	if !ok {
		return
	}
	request, ok := decodeJSON[slaWarningThresholdRequest](w, r)
	if !ok {
		return
	}
	threshold, err := h.settings.UpdateWarningThreshold(r.Context(), workspaceID, thresholdID, request.input())
	if !h.writeThresholdResult(w, r, err) {
		return
	}
	respondJSONOK(w, threshold)
}

// DeleteWarningThreshold deletes a warning threshold.
func (h *SLAHandler) DeleteWarningThreshold(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	thresholdID, ok := requireIDParam(w, r, "thresholdId")
	if !ok {
		return
	}
	if !h.writeThresholdResult(w, r, h.settings.DeleteWarningThreshold(r.Context(), workspaceID, thresholdID)) {
		return
	}
	respondJSONOK(w, map[string]bool{"deleted": true})
}

func (h *SLAHandler) writeThresholdResult(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, services.ErrSLAThresholdInvalid):
		respondValidationError(w, r, err.Error())
	case errors.Is(err, repository.ErrNotFound):
		respondNotFound(w, r, "warning threshold")
	default:
		respondError(w, r, slaInternal(err))
	}
	return false
}

// ---------------------------------------------------------------------------
// Metrics
// ---------------------------------------------------------------------------

type slaMetricRequest struct {
	Name          string                `json:"name"`
	DisplayFormat string                `json:"display_format"`
	Position      int                   `json:"position"`
	IsActive      *bool                 `json:"is_active"`
	ImportStatus  string                `json:"import_status"`
	Conditions    []models.SLACondition `json:"conditions"`
	Goals         []models.SLAGoal      `json:"goals"`
}

// input maps the transport request onto the shared metric service input.
func (request slaMetricRequest) input() services.SLAMetricInput {
	return services.SLAMetricInput{
		Name:          request.Name,
		DisplayFormat: request.DisplayFormat,
		Position:      request.Position,
		IsActive:      request.IsActive,
		ImportStatus:  request.ImportStatus,
		Conditions:    request.Conditions,
		Goals:         request.Goals,
	}
}

// ListMetrics returns a workspace's SLA metrics.
func (h *SLAHandler) ListMetrics(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	metrics, err := h.metrics.List(r.Context(), workspaceID)
	if err != nil {
		respondError(w, r, slaInternal(err))
		return
	}
	respondJSONOK(w, metrics)
}

// GetMetric returns one SLA metric.
func (h *SLAHandler) GetMetric(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	metricID, ok := requireIDParam(w, r, "metricId")
	if !ok {
		return
	}
	metric, err := h.metrics.Get(r.Context(), workspaceID, metricID)
	if !h.writeMetricResult(w, r, err) {
		return
	}
	respondJSONOK(w, metric)
}

// CreateMetric creates a metric and schedules recalculation.
func (h *SLAHandler) CreateMetric(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	request, ok := decodeJSON[slaMetricRequest](w, r)
	if !ok {
		return
	}
	metric, err := h.metrics.Create(r.Context(), workspaceID, request.input())
	if !h.writeMetricResult(w, r, err) {
		return
	}
	respondJSONCreated(w, metric)
}

// UpdateMetric updates a metric and schedules recalculation.
func (h *SLAHandler) UpdateMetric(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	metricID, ok := requireIDParam(w, r, "metricId")
	if !ok {
		return
	}
	request, ok := decodeJSON[slaMetricRequest](w, r)
	if !ok {
		return
	}
	metric, err := h.metrics.Update(r.Context(), workspaceID, metricID, request.input())
	if !h.writeMetricResult(w, r, err) {
		return
	}
	respondJSONOK(w, metric)
}

// DeleteMetric deletes a metric; cycles and jobs cascade.
func (h *SLAHandler) DeleteMetric(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	metricID, ok := requireIDParam(w, r, "metricId")
	if !ok {
		return
	}
	if !h.writeMetricResult(w, r, h.metrics.Delete(r.Context(), workspaceID, metricID)) {
		return
	}
	respondJSONOK(w, map[string]bool{"deleted": true})
}

// StartRecalculation enqueues a recalculation for one metric or the whole
// workspace.
func (h *SLAHandler) StartRecalculation(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	var request struct {
		MetricID int `json:"metric_id"`
	}
	request, ok = decodeJSON[struct {
		MetricID int `json:"metric_id"`
	}](w, r)
	if !ok {
		return
	}
	enqueued, err := h.metrics.EnqueueRecalculation(r.Context(), workspaceID, request.MetricID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "metric")
		return
	}
	if err != nil {
		respondError(w, r, slaInternal(err))
		return
	}
	respondJSONOK(w, map[string]any{"enqueued": enqueued})
}

// GetReport returns the completed-cycle compliance report for a workspace.
// It applies the same item-view visibility filter as item listings.
func (h *SLAHandler) GetReport(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionItemView, h.permissionService) {
		return
	}
	from, ok := parseReportTime(w, r, "from")
	if !ok {
		return
	}
	to, ok := parseReportTime(w, r, "to")
	if !ok {
		return
	}
	report, err := h.engine.Report(r.Context(), workspaceID, from, to)
	if err != nil {
		respondError(w, r, slaInternal(err))
		return
	}
	respondJSONOK(w, report)
}

func parseReportTime(w http.ResponseWriter, r *http.Request, name string) (*time.Time, bool) {
	value := strings.TrimSpace(r.URL.Query().Get(name))
	if value == "" {
		return nil, true
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		parsed, err = time.Parse(time.DateOnly, value)
	}
	if err != nil {
		respondValidationError(w, r, name+" must be RFC3339 or YYYY-MM-DD")
		return nil, false
	}
	return &parsed, true
}

// ListRecalculations reports pending recalculation work for a workspace.
func (h *SLAHandler) ListRecalculations(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	jobs, err := h.repo.ListRecalculationJobs(r.Context(), workspaceID)
	if err != nil {
		respondError(w, r, slaInternal(err))
		return
	}
	respondJSONOK(w, jobs)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func slaInternal(err error) *restapi.APIError {
	slog.Error("SLA handler internal error", slog.String("component", "sla"), slog.Any("error", err))
	return restapi.ErrInternalError
}

func (h *SLAHandler) authorizeWorkspaceAdmin(w http.ResponseWriter, r *http.Request) (int, bool) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return 0, false
	}
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return 0, false
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionWorkspaceAdmin, h.permissionService) {
		return 0, false
	}
	return workspaceID, true
}

// authorizeTeamAdmin parses the team ID and permits a global team manager or a
// direct team admin, mirroring TeamHandler.canManageTeam.
func (h *SLAHandler) authorizeTeamAdmin(w http.ResponseWriter, r *http.Request) (int, bool) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return 0, false
	}
	teamID, ok := requireIDParam(w, r, "id")
	if !ok {
		return 0, false
	}
	hasGlobal, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionTeamsManage)
	if err != nil {
		respondInternalError(w, r, err)
		return 0, false
	}
	if hasGlobal {
		return teamID, true
	}
	isAdmin, err := h.teamRepo.IsTeamAdmin(teamID, user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return 0, false
	}
	if !isAdmin {
		respondForbidden(w, r)
		return 0, false
	}
	return teamID, true
}

type slaImportRequest struct {
	Mode     string                     `json:"mode"`
	Document services.SLAImportDocument `json:"document"`
}

// PreviewSLAImport resolves a Jira SLA document without writing and reports the
// planned changes and any clauses that need attention.
func (h *SLAHandler) PreviewSLAImport(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	request, ok := decodeJSON[services.SLAImportDocument](w, r)
	if !ok {
		return
	}
	result, err := h.slaImport.Preview(r.Context(), workspaceID, request)
	if !h.writeImportResult(w, r, result, err) {
		return
	}
}

// ImportSLA applies a Jira SLA document in merge or replace mode.
func (h *SLAHandler) ImportSLA(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := h.authorizeWorkspaceAdmin(w, r)
	if !ok {
		return
	}
	request, ok := decodeJSON[slaImportRequest](w, r)
	if !ok {
		return
	}
	result, err := h.slaImport.Import(r.Context(), workspaceID, request.Document, request.Mode)
	if !h.writeImportResult(w, r, result, err) {
		return
	}
}

// writeImportResult maps import errors and writes the result. It returns true
// when the caller should write the success body.
func (h *SLAHandler) writeImportResult(w http.ResponseWriter, r *http.Request, result *services.SLAImportResult, err error) bool {
	switch {
	case err == nil:
		respondJSONOK(w, result)
		return true
	case errors.Is(err, services.ErrSLAImportInvalid):
		respondValidationError(w, r, err.Error())
	default:
		respondError(w, r, slaInternal(err))
	}
	return false
}

// writeMetricResult maps metric service errors to responses. It returns true
// when the caller should write the success body.
func (h *SLAHandler) writeMetricResult(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, services.ErrSLAMetricInvalid):
		respondValidationError(w, r, err.Error())
	case errors.Is(err, repository.ErrNotFound):
		respondNotFound(w, r, "metric")
	default:
		respondError(w, r, slaInternal(err))
	}
	return false
}
