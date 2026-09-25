package v2

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"windshift/internal/csvimport"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/validation"
)

// ticketImportUploadForm mirrors the asset import's multipart contract: the
// CSV body plus optionality flags. Declared for route metadata only.
type ticketImportUploadForm struct {
	File string `form:"file"`
}

// workspaceTarget resolves the authenticated principal and the workspace_id
// path value shared by the ticket import routes.
func workspaceTarget(r *http.Request) (*models.User, int, error) {
	user, err := principal(r)
	if err != nil {
		return nil, 0, err
	}
	id, err := pathID(r, "workspace_id")
	return user, id, err
}

func registerTicketImportRoutes(builder *routeBuilder, tickets *services.TicketImportService) {
	base := "/workspaces/{workspace_id}/tickets/import"

	builder.RawDocument[ticketImportUploadForm, services.TicketImportStaged](http.MethodPost, base+"/upload", http.StatusCreated, "multipart/form-data", AuthAuthenticated, []string{"items:write"}, func(w http.ResponseWriter, r *http.Request) error {
		user, workspaceID, err := workspaceTarget(r)
		if err != nil {
			return err
		}
		// Authorize before touching the body so denials mask existence.
		if err := tickets.Authorize(r.Context(), user.ID, workspaceID, models.PermissionItemCreate); err != nil {
			return ticketImportError(err)
		}
		const maxBody = (50 << 20) + (1 << 20)
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		// The request body is bounded above; ParseMultipartForm's argument only controls memory use.
		//nolint:gosec // G120 does not recognize the MaxBytesReader assignment.
		if err := r.ParseMultipartForm(maxBody); err != nil {
			return newError(http.StatusBadRequest, "invalid_request", "Invalid multipart upload")
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			return newError(http.StatusBadRequest, "invalid_request", "file is required")
		}
		defer func() { _ = file.Close() }()
		staged, err := tickets.Upload(r.Context(), user.ID, workspaceID, header.Filename, r.FormValue("has_header") != "false", r.FormValue("delimiter"), io.LimitReader(file, maxBody))
		if err != nil {
			return ticketImportError(err)
		}
		return writeDocument(w, http.StatusCreated, staged)
	})
	builder.JSON(http.MethodPost, base+"/start", http.StatusAccepted, false, AuthAuthenticated, []string{"items:write"}, func(r *http.Request, input services.TicketImportStart) (*services.TicketImportJob, error) {
		user, workspaceID, err := workspaceTarget(r)
		if err != nil {
			return nil, err
		}
		job, err := tickets.Start(r.Context(), auditActor(r, user), workspaceID, input)
		return job, ticketImportError(err)
	})
	builder.Read(base+"/jobs/{job_id}", AuthAuthenticated, []string{"items:write"}, func(r *http.Request) (*services.TicketImportJob, error) {
		user, workspaceID, err := workspaceTarget(r)
		if err != nil {
			return nil, err
		}
		job, err := tickets.GetJob(r.Context(), user.ID, workspaceID, r.PathValue("job_id"))
		return job, ticketImportError(err)
	})

	// Export streams the workspace's tickets in the exchange schema.
	builder.RawHandler(http.MethodGet, "/workspaces/{workspace_id}/tickets/export", "text/csv", http.StatusOK, AuthAuthenticated, []string{"items:read"}, func(w http.ResponseWriter, r *http.Request) error {
		user, workspaceID, err := workspaceTarget(r)
		if err != nil {
			return err
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "tickets-export.csv"))
		if err := tickets.Export(r.Context(), w, user.ID, workspaceID); err != nil {
			return ticketImportError(err)
		}
		return nil
	})
}

// ticketImportError maps ticket exchange errors onto transport statuses.
func ticketImportError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, services.ErrItemForbidden):
		return newError(http.StatusNotFound, "not_found", "Workspace not found")
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, csvimport.ErrUploadNotFound):
		return newError(http.StatusNotFound, "not_found", "Import upload or job not found")
	case errors.Is(err, csvimport.ErrConfigConflict):
		return newError(http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, services.ErrTicketImportStorageDisabled):
		return newError(http.StatusServiceUnavailable, "service_unavailable", "Ticket import storage is not configured")
	}
	var validationErr *validation.ValidationError
	if errors.As(err, &validationErr) {
		return newError(http.StatusBadRequest, "validation_failed", err.Error())
	}
	return newError(http.StatusBadRequest, "invalid_request", err.Error())
}
