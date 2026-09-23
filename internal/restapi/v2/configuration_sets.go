package v2

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type configurationSetDTO = models.ConfigurationSet

type configurationSetListMeta struct {
	Total int `json:"total"`
}

// configurationSetMutationResponse carries the persisted set plus the cache
// warnings the create flow can produce.
type configurationSetMutationResponse struct {
	ConfigurationSet *models.ConfigurationSet `json:"configuration_set"`
	Warnings         []models.APIWarning      `json:"warnings,omitempty"`
}

// requestInstance renders the request origin for export provenance metadata.
func requestInstance(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// configurationSetMutationError maps shared provisioning errors onto v2 error
// semantics. ServiceError values already carry the intended HTTP status.
func configurationSetMutationError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, services.ErrCannotExportDefault) {
		return newError(http.StatusForbidden, "default_not_exportable",
			"The default configuration set cannot be exported or compared against a template; clone it first.")
	}
	var serviceErr *services.ServiceError
	if errors.As(err, &serviceErr) {
		return newError(serviceErr.StatusCode, catalogErrorCode(serviceErr.StatusCode), serviceErr.Message)
	}
	if errors.Is(err, repository.ErrNotFound) {
		return newError(http.StatusNotFound, "not_found", "Configuration set not found")
	}
	return internalError(err)
}

// configurationSetImportError maps template-import failures onto v2 error
// semantics, echoing the same structured reports the browser import surfaces.
func configurationSetImportError(err error) error {
	if err == nil {
		return nil
	}
	var unresolvedErr *services.ErrUnresolvedReferences
	if errors.As(err, &unresolvedErr) {
		e := newError(http.StatusUnprocessableEntity, "unresolved_references",
			"Import requires identity references that don't exist on this instance")
		e.Details = unresolvedErr.Items
		return e
	}
	var defaultConflictErr *services.ErrDefaultEntityConflict
	if errors.As(err, &defaultConflictErr) {
		e := newError(http.StatusConflict, "default_entity_conflict",
			"Import would shadow a default-flagged entity on this instance; rename the bundle or import elsewhere.")
		e.Details = defaultConflictErr.Conflicts
		return e
	}
	var linkTypeConflictErr *services.ErrLinkTypeDefinitionConflict
	if errors.As(err, &linkTypeConflictErr) {
		e := newError(http.StatusConflict, "link_type_definition_conflict",
			"Import contains link types whose names collide with existing link types defined differently on this instance; rename one side or align the definitions.")
		e.Details = linkTypeConflictErr.Conflicts
		return e
	}
	return configurationSetMutationError(err)
}

func registerConfigurationSetRoutes(b *routeBuilder, deps Deps) {
	read := []string{"configuration-sets:read"}
	write := []string{"configuration-sets:write"}

	b.PageMetadata("/configuration-sets", AuthAuthenticated, read, func(r *http.Request) ([]configurationSetDTO, Pagination, int, configurationSetListMeta, error) {
		page, limit := 1, 10
		if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
			page = v
		}
		if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 {
			limit = v
		}
		sets, total, err := deps.ConfigurationSetProvisioning.List(page, limit, r.URL.Query().Get("search"))
		if err != nil {
			return nil, Pagination{}, 0, configurationSetListMeta{}, internalError(err)
		}
		return sets, Pagination{Page: page, PageSize: limit}, total, configurationSetListMeta{Total: total}, nil
	})

	b.Read("/configuration-sets/{configuration_set_id}", AuthAuthenticated, read, func(r *http.Request) (configurationSetDTO, error) {
		id, err := pathID(r, "configuration_set_id")
		if err != nil {
			return configurationSetDTO{}, err
		}
		cs, err := deps.ConfigurationSetProvisioning.Get(id)
		if err != nil {
			return configurationSetDTO{}, configurationSetMutationError(err)
		}
		return *cs, nil
	})

	b.JSON(http.MethodPost, "/configuration-sets", http.StatusCreated, false, AuthAuthenticated, write, func(r *http.Request, input models.ConfigurationSet) (configurationSetMutationResponse, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return configurationSetMutationResponse{}, err
		}
		result, err := deps.ConfigurationSetProvisioning.Create(auditActorFromRequest(r), &input)
		if err != nil {
			return configurationSetMutationResponse{}, configurationSetMutationError(err)
		}
		return configurationSetMutationResponse{ConfigurationSet: result.Set, Warnings: result.Warnings}, nil
	})

	b.Command(http.MethodDelete, "/configuration-sets/{configuration_set_id}", AuthAuthenticated, write, func(r *http.Request) error {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return err
		}
		id, err := pathID(r, "configuration_set_id")
		if err != nil {
			return err
		}
		return configurationSetMutationError(deps.ConfigurationSetProvisioning.Delete(auditActorFromRequest(r), id))
	})

	// Import applies a portable template document — the exact payload Export
	// produces — and creates a fresh configuration set. The request body is
	// the document itself; structured rejection reports (unresolved identity
	// refs, default/name-definition conflicts) ride in error.details.
	b.JSON(http.MethodPost, "/configuration-sets/import", http.StatusCreated, false, AuthAuthenticated, write, func(r *http.Request, tpl services.ConfigSetTemplate) (configurationSetMutationResponse, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return configurationSetMutationResponse{}, err
		}
		result, err := deps.ConfigurationSetProvisioning.ImportTemplate(r.Context(), auditActorFromRequest(r), &tpl)
		if err != nil {
			return configurationSetMutationResponse{}, configurationSetImportError(err)
		}
		return configurationSetMutationResponse{ConfigurationSet: result.Set, Warnings: result.Warnings}, nil
	})

	// Conformance check: does this configuration set still match the
	// canonical template? The template document is the request body — the
	// same payload Import consumes. Read-only; drift rows carry stable IDs
	// used to select rows for repair.
	b.JSON(http.MethodPost, "/configuration-sets/{configuration_set_id}/conformance/check", http.StatusOK, false, AuthAuthenticated, read, func(r *http.Request, tpl services.ConfigSetTemplate) (*services.ConfigSetConformanceReport, error) {
		id, err := pathID(r, "configuration_set_id")
		if err != nil {
			return nil, err
		}
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return nil, err
		}
		if err := services.SanitizeConfigSetTemplate(&tpl); err != nil {
			return nil, newError(http.StatusBadRequest, "invalid_request", err.Error())
		}
		report, err := deps.ConfigSetConformance.Check(r.Context(), id, &tpl)
		if err != nil {
			return nil, configurationSetMutationError(err)
		}
		deps.ConfigSetConformance.AuditCheck(auditActorFromRequest(r), id, report.DriftCount)
		return report, nil
	})

	// Conformance repair restores drifted configuration to the template in
	// one transaction. Drift row IDs select rows; an empty list repairs all
	// repairable rows. Item data is never written.
	b.JSON(http.MethodPost, "/configuration-sets/{configuration_set_id}/conformance/repair", http.StatusOK, false, AuthAuthenticated, write, func(r *http.Request, req services.ConfigSetConformanceRepairRequest) (*services.ConfigSetConformanceRepairResult, error) {
		id, err := pathID(r, "configuration_set_id")
		if err != nil {
			return nil, err
		}
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return nil, err
		}
		if err := services.SanitizeConfigSetTemplate(&req.Template); err != nil {
			return nil, newError(http.StatusBadRequest, "invalid_request", err.Error())
		}
		result, err := deps.ConfigSetConformance.Repair(r.Context(), id, &req.Template, req.IDs)
		if err != nil {
			return nil, configurationSetMutationError(err)
		}
		deps.ConfigSetConformance.AuditRepair(auditActorFromRequest(r), id, result.Repaired, result.Failed, result.Skipped)
		return result, nil
	})

	// Export writes the portable template document directly — no v2 data
	// envelope — matching the browser download contract.
	b.RawResponse[*services.ConfigSetTemplate](http.MethodGet, "/configuration-sets/{configuration_set_id}/export", http.StatusOK, "application/json", AuthAuthenticated, read, func(w http.ResponseWriter, r *http.Request) error {
		id, err := pathID(r, "configuration_set_id")
		if err != nil {
			return err
		}
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return err
		}
		user, err := principal(r)
		if err != nil {
			return err
		}
		tpl, err := deps.ConfigurationSetExport.Export(r.Context(), id, &services.ConfigSetExportBy{Username: user.Username, Instance: requestInstance(r)})
		if err != nil {
			if errors.Is(err, services.ErrCannotExportDefault) {
				deps.ConfigurationSetProvisioning.AuditExport(auditActor(r, user), id, false)
				return newError(http.StatusForbidden, "default_not_exportable",
					"The default configuration set cannot be exported; clone it first if you need a portable copy.")
			}
			return internalError(err)
		}
		deps.ConfigurationSetProvisioning.AuditExport(auditActor(r, user), id, true)
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(tpl)
	})
}
