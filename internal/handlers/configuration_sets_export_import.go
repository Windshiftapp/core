package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// configSetImportMaxBytes caps the upload size for /configuration-sets/import.
// Sufficient headroom for an ITSM-style bundle (custom fields, screens, a
// non-trivial workflow + condition + approval set) without inviting abuse.
const configSetImportMaxBytes = 5 << 20 // 5 MiB

// Export streams a configuration set as a JSON template suitable for upload
// on another instance via Import. Read-only; permitted to any authenticated
// user (matches the GET /configuration-sets/{id} read auth).
func (h *ConfigurationSetHandler) Export(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Confirm the configuration set exists before building the bundle —
	// gives us a 404 instead of a generic 500 on a missing id.
	cs, err := h.repo.FindByIDBasic(id)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "configuration_set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	exportSvc := services.NewConfigSetExportService(h.db, h.repo)
	tpl, err := exportSvc.Export(r.Context(), id, exportedByFromRequest(r))
	if err != nil {
		if errors.Is(err, services.ErrCannotExportDefault) {
			// Audit the refusal so attempts to extract the default are visible
			// in the security log alongside successful exports.
			currentUser := utils.GetCurrentUser(r)
			if currentUser != nil {
				_ = logger.LogAudit(h.db, logger.AuditEvent{
					UserID:       currentUser.ID,
					Username:     currentUser.Username,
					IPAddress:    utils.GetClientIP(r),
					UserAgent:    r.UserAgent(),
					ActionType:   logger.ActionConfigSetExport,
					ResourceType: logger.ResourceConfigurationSet,
					ResourceID:   &id,
					ResourceName: cs.Name,
					Success:      false,
					ErrorMessage: "default_not_exportable",
				})
			}
			restapi.RespondError(w, r, restapi.NewAPIError(http.StatusForbidden, "default_not_exportable",
				"The default configuration set cannot be exported; clone it first if you need a portable copy."))
			return
		}
		respondInternalError(w, r, fmt.Errorf("export configuration set %d: %w", id, err))
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionConfigSetExport, logger.ResourceConfigurationSet, &id, cs.Name)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", configSetExportFilename(cs.Name)))
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(tpl); err != nil {
		// Headers already sent, so just log.
		respondInternalError(w, r, err)
	}
}

// Import accepts a multipart upload (field "file") containing a JSON template
// produced by Export, applies it inside a transaction, and returns the new
// configuration set record. Admin-only; rate-limited at the route level.
func (h *ConfigurationSetHandler) Import(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, configSetImportMaxBytes)
	if err := r.ParseMultipartForm(configSetImportMaxBytes); err != nil {
		respondBadRequest(w, r, "Failed to parse multipart form (max "+humanBytes(configSetImportMaxBytes)+")")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		respondBadRequest(w, r, "Missing 'file' upload field")
		return
	}
	defer func() { _ = file.Close() }()

	var tpl services.ConfigSetTemplate
	dec := json.NewDecoder(file)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&tpl); err != nil {
		respondBadRequest(w, r, "Invalid template JSON: "+err.Error())
		return
	}

	importSvc := services.NewConfigSetImportService(h.db, h.repo)
	newID, warnings, err := importSvc.Import(r.Context(), &tpl)
	if err != nil {
		var conflictErr *services.ErrDefaultEntityConflict
		if errors.As(err, &conflictErr) {
			apiErr := restapi.NewAPIError(http.StatusConflict, "default_entity_conflict",
				"Import would shadow a default-flagged entity on this instance; rename the bundle or import elsewhere.")
			apiErr.WithDetails(map[string]any{"conflicts": conflictErr.Conflicts})
			restapi.RespondError(w, r, apiErr)
			return
		}
		var unresolvedErr *services.ErrUnresolvedReferences
		if errors.As(err, &unresolvedErr) {
			apiErr := restapi.NewAPIError(http.StatusUnprocessableEntity, "unresolved_references",
				"Import requires identity references that don't exist on this instance")
			apiErr.WithDetails(map[string]any{"unresolved": unresolvedErr.Items})
			restapi.RespondError(w, r, apiErr)
			return
		}
		respondInternalError(w, r, err)
		return
	}

	created, err := h.repo.FindByID(newID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("load created configuration set: %w", err))
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAuditWithDetails(h.db, r, currentUser, logger.ActionConfigSetImport, logger.ResourceConfigurationSet, &newID, created.Name, map[string]interface{}{
			"workflow_id":   created.WorkflowID,
			"warning_count": len(warnings),
		})
	}

	if h.permissionService != nil {
		_ = h.permissionService.OnConfigurationSetChanged(newID)
	}

	if len(warnings) == 0 {
		respondJSONCreated(w, created)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data":     created,
		"warnings": warnings,
	})
}

func exportedByFromRequest(r *http.Request) *services.ConfigSetExportBy {
	user := utils.GetCurrentUser(r)
	if user == nil {
		return nil
	}
	host := r.Host
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return &services.ConfigSetExportBy{
		Username: user.Username,
		Instance: scheme + "://" + host,
	}
}

// configSetExportFilename produces a safe download filename derived from the
// config set's name. Slashes and quotes are stripped to avoid header smuggling.
func configSetExportFilename(name string) string {
	out := make([]byte, 0, len(name)+len(".json"))
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
			out = append(out, c)
		case c == ' ':
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		out = []byte("configuration-set")
	}
	return string(out) + ".json"
}

func humanBytes(n int) string {
	const mib = 1 << 20
	if n >= mib {
		return fmt.Sprintf("%d MiB", n/mib)
	}
	return fmt.Sprintf("%d bytes", n)
}
