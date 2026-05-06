package handlers

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type TestCoverageHandler struct {
	repo              *repository.TestCoverageRepository
	permissionService *services.PermissionService
}

func NewTestCoverageHandler(repo *repository.TestCoverageRepository, permissionService *services.PermissionService) *TestCoverageHandler {
	return &TestCoverageHandler{
		repo:              repo,
		permissionService: permissionService,
	}
}

// GetConfig returns the test coverage configuration for a collection or workspace
func (h *TestCoverageHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	repo := h.repo

	var (
		config *models.TestCoverageConfiguration
		err    error
	)

	if id == "default" {
		workspaceID, ok := parseWorkspaceIDQuery(w, r)
		if !ok {
			return
		}
		config, err = repo.FindConfigForWorkspace(workspaceID)
	} else {
		collectionID, parseErr := strconv.Atoi(id)
		if parseErr != nil {
			respondInvalidID(w, r, "collectionId")
			return
		}
		config, err = repo.FindConfigForCollection(collectionID)
	}

	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "test_coverage_configuration")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, config)
}

// CreateConfig creates a new test coverage configuration
func (h *TestCoverageHandler) CreateConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	req, ok := decodeJSON[models.TestCoverageConfigRequest](w, r)
	if !ok {
		return
	}

	repo := h.repo

	var (
		config *models.TestCoverageConfiguration
		err    error
	)

	if id == "default" {
		workspaceID, ok := parseWorkspaceIDQuery(w, r)
		if !ok {
			return
		}
		config, err = repo.CreateConfigForWorkspace(workspaceID, req.RequirementItemTypeIDs)
	} else {
		collectionID, parseErr := strconv.Atoi(id)
		if parseErr != nil {
			respondInvalidID(w, r, "collectionId")
			return
		}
		config, err = repo.CreateConfigForCollection(collectionID, req.RequirementItemTypeIDs)
	}

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, config)
}

// UpdateConfig updates the test coverage configuration
func (h *TestCoverageHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	configID, ok := requireIDParam(w, r, "configId")
	if !ok {
		return
	}

	req, ok := decodeJSON[models.TestCoverageConfigRequest](w, r)
	if !ok {
		return
	}

	config, err := h.repo.UpdateConfig(configID, req.RequirementItemTypeIDs)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "test_coverage_configuration")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, config)
}

// DeleteConfig deletes the test coverage configuration
func (h *TestCoverageHandler) DeleteConfig(w http.ResponseWriter, r *http.Request) {
	configID, ok := requireIDParam(w, r, "configId")
	if !ok {
		return
	}

	if err := h.repo.DeleteConfig(configID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetSummary returns the coverage summary (for pie chart)
func (h *TestCoverageHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	repo := h.repo

	typeIDs, workspaceID, err := h.getRequirementTypeIDs(repo, id, r.URL.Query().Get("workspace_id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondJSONOK(w, models.TestCoverageSummary{})
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if len(typeIDs) == 0 {
		respondJSONOK(w, models.TestCoverageSummary{})
		return
	}

	total, covered, err := repo.GetCoverageSummary(workspaceID, typeIDs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, buildCoverageSummary(total, covered))
}

// GetRequirements returns the paginated list of requirements with coverage status
func (h *TestCoverageHandler) GetRequirements(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	repo := h.repo

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 25
	}

	coveredFilter := r.URL.Query().Get("covered")
	itemTypeFilter := r.URL.Query().Get("item_type_id")
	searchFilter := r.URL.Query().Get("search")

	emptyResponse := func() models.TestCoverageListResponse {
		return models.TestCoverageListResponse{
			Items:      []models.RequirementCoverageItem{},
			Pagination: models.PaginationMeta{Page: page, Limit: limit, Total: 0},
			Summary:    models.TestCoverageSummary{},
		}
	}

	typeIDs, workspaceID, err := h.getRequirementTypeIDs(repo, id, r.URL.Query().Get("workspace_id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondJSONOK(w, emptyResponse())
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if len(typeIDs) == 0 {
		respondJSONOK(w, emptyResponse())
		return
	}

	// Apply item_type_id filter if it is one of the configured types.
	if itemTypeFilter != "" {
		if itemTypeID, perr := strconv.Atoi(itemTypeFilter); perr == nil {
			for _, tid := range typeIDs {
				if tid == itemTypeID {
					typeIDs = []int{itemTypeID}
					break
				}
			}
		}
	}

	listParams := repository.RequirementListParams{
		WorkspaceID:   workspaceID,
		TypeIDs:       typeIDs,
		CoveredFilter: coveredFilter,
		Search:        searchFilter,
		Limit:         limit,
		Offset:        (page - 1) * limit,
	}

	total, err := repo.CountRequirements(listParams)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	items, err := repo.ListRequirements(listParams)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	summaryTotal, summaryCovered, sumErr := repo.GetCoverageSummary(workspaceID, typeIDs)
	if sumErr != nil {
		slog.Warn("failed to get test coverage summary counts", slog.Any("error", sumErr))
	}

	respondJSONOK(w, models.TestCoverageListResponse{
		Items: items,
		Pagination: models.PaginationMeta{
			Page:  page,
			Limit: limit,
			Total: total,
		},
		Summary: buildCoverageSummary(summaryTotal, summaryCovered),
	})
}

func (h *TestCoverageHandler) getRequirementTypeIDs(repo *repository.TestCoverageRepository, id, workspaceIDStr string) (typeIDs []int, workspaceID int, err error) {
	if id == "default" {
		if workspaceIDStr == "" {
			return nil, 0, sql.ErrNoRows
		}
		workspaceID, err = strconv.Atoi(workspaceIDStr)
		if err != nil {
			return nil, 0, err
		}
		typeIDs, err = repo.GetRequirementTypeIDsForWorkspace(workspaceID)
		if err != nil {
			return nil, 0, err
		}
		return typeIDs, workspaceID, nil
	}

	collectionID, err := strconv.Atoi(id)
	if err != nil {
		return nil, 0, err
	}
	return repo.GetRequirementTypeIDsForCollection(collectionID)
}

func parseWorkspaceIDQuery(w http.ResponseWriter, r *http.Request) (int, bool) {
	workspaceIDStr := r.URL.Query().Get("workspace_id")
	if workspaceIDStr == "" {
		respondValidationError(w, r, "workspace_id query parameter required for default configuration")
		return 0, false
	}
	workspaceID, err := strconv.Atoi(workspaceIDStr)
	if err != nil {
		respondInvalidID(w, r, "workspace_id")
		return 0, false
	}
	return workspaceID, true
}

func buildCoverageSummary(total, covered int) models.TestCoverageSummary {
	summary := models.TestCoverageSummary{
		Total:      total,
		Covered:    covered,
		NotCovered: total - covered,
	}
	if total > 0 {
		summary.CoverageRate = float64(covered) / float64(total) * 100
	}
	return summary
}
