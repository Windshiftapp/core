package v2

import (
	"io"
	"net/http"

	"windshift/internal/services"
)

// Framework pack endpoints (WI-1336): apply installs a pack archive into a
// workspace (create-or-target), verify validates a pack without applying.
// Both are system-admin operations over the same report the UI renders.

const packUploadMaxBytes = 32 << 20

func parsePackUpload(w http.ResponseWriter, r *http.Request) (*services.PackArchive, services.PackApplyTarget, error) {
	r.Body = http.MaxBytesReader(w, r.Body, packUploadMaxBytes)
	// The request body is bounded above; ParseMultipartForm's argument only
	// controls memory use.
	//nolint:gosec // G120 does not recognize the MaxBytesReader assignment.
	if err := r.ParseMultipartForm(packUploadMaxBytes); err != nil {
		return nil, services.PackApplyTarget{}, newError(http.StatusBadRequest, "invalid_request", "Invalid multipart upload")
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		return nil, services.PackApplyTarget{}, newError(http.StatusBadRequest, "invalid_request", "file is required")
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, services.PackApplyTarget{}, internalError(err)
	}

	archive, err := services.ParsePackArchive(data)
	if err != nil {
		return nil, services.PackApplyTarget{}, newError(http.StatusBadRequest, "invalid_pack", err.Error())
	}

	target := services.PackApplyTarget{}
	if raw := r.FormValue("workspace_id"); raw != "" {
		id := 0
		for _, c := range raw {
			if c < '0' || c > '9' {
				return nil, services.PackApplyTarget{}, newError(http.StatusBadRequest, "invalid_request", "workspace_id must be a positive integer")
			}
			id = id*10 + int(c-'0')
		}
		if id <= 0 {
			return nil, services.PackApplyTarget{}, newError(http.StatusBadRequest, "invalid_request", "workspace_id must be a positive integer")
		}
		target.WorkspaceID = id
	}
	if name := r.FormValue("workspace_name"); name != "" {
		target.WorkspaceName = name
	}
	if target.WorkspaceID == 0 && target.WorkspaceName == "" {
		return nil, services.PackApplyTarget{}, newError(http.StatusBadRequest, "invalid_request", "either workspace_id or workspace_name is required")
	}
	if target.WorkspaceID > 0 && target.WorkspaceName != "" {
		return nil, services.PackApplyTarget{}, newError(http.StatusBadRequest, "invalid_request", "workspace_id and workspace_name are mutually exclusive")
	}
	return archive, target, nil
}

func registerPackRoutes(b *routeBuilder, deps Deps) {
	apply := func(w http.ResponseWriter, r *http.Request, dryRun bool) error {
		if _, err := principal(r); err != nil {
			return err
		}
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return err
		}
		archive, target, err := parsePackUpload(w, r)
		if err != nil {
			return err
		}
		actor := auditActorFromRequest(r)
		req := services.PackApplyRequest{
			Archive: archive,
			Target: services.PackApplyTarget{
				WorkspaceID:   target.WorkspaceID,
				WorkspaceName: target.WorkspaceName,
			},
			Actor: actor,
		}
		var report *services.PackApplyReport
		if dryRun {
			report, err = deps.PackApply.Verify(r.Context(), req)
		} else {
			report, err = deps.PackApply.Apply(r.Context(), req)
		}
		if err != nil {
			return internalError(err)
		}
		if !dryRun {
			deps.PackApply.AuditApply(actor, report.WorkspaceID, report.Pack, report.PackVersion, report.Status)
		}
		return writeDocument(w, http.StatusOK, report)
	}

	b.RawDocument[struct{}, *services.PackApplyReport](http.MethodPost, "/packs/apply", http.StatusOK, "multipart/form-data", AuthAuthenticated, []string{"workspaces:write"}, func(w http.ResponseWriter, r *http.Request) error {
		return apply(w, r, false)
	})
	b.RawDocument[struct{}, *services.PackApplyReport](http.MethodPost, "/packs/verify", http.StatusOK, "multipart/form-data", AuthAuthenticated, []string{"workspaces:read"}, func(w http.ResponseWriter, r *http.Request) error {
		return apply(w, r, true)
	})
}
