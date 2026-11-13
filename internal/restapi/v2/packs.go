package v2

import (
	"errors"
	"io"
	"net/http"
	"strings"

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

	if raw := r.FormValue("workspace_id"); raw != "" {
		return nil, services.PackApplyTarget{}, newError(http.StatusBadRequest, "invalid_request", "workspace_id is not supported: framework packs only apply to new workspaces")
	}
	name := strings.TrimSpace(r.FormValue("workspace_name"))
	if name == "" {
		return nil, services.PackApplyTarget{}, newError(http.StatusBadRequest, "invalid_request", "workspace_name is required")
	}
	return archive, services.PackApplyTarget{WorkspaceName: name}, nil
}

// packBuiltinTarget is the JSON body for applying a built-in pack: the name of
// the new workspace to provision. Framework packs only apply to new
// workspaces, so there is no existing-workspace target.
type packBuiltinTarget struct {
	WorkspaceName string `json:"workspace_name"`
}

func (t packBuiltinTarget) resolve() (services.PackApplyTarget, error) {
	name := strings.TrimSpace(t.WorkspaceName)
	if name == "" {
		return services.PackApplyTarget{}, newError(http.StatusBadRequest, "invalid_request", "workspace_name is required")
	}
	return services.PackApplyTarget{WorkspaceName: name}, nil
}

// listBuiltinPacks lists the packs embedded in the running server. System
// administrators only, matching the apply surface.
func listBuiltinPacks(deps Deps) readOperation[[]services.BuiltinPackSummary] {
	return func(r *http.Request) ([]services.BuiltinPackSummary, error) {
		if _, err := requireSystemAdmin(r, deps); err != nil {
			return nil, err
		}
		packs, err := services.BuiltinPacks()
		if err != nil {
			return nil, internalError(err)
		}
		return packs, nil
	}
}

// applyBuiltinPack applies (or dry-run verifies) one embedded pack. It reuses
// the upload path's PackApplyService, so the report is identical.
func applyBuiltinPack(deps Deps, dryRun bool) jsonOperation[packBuiltinTarget, *services.PackApplyReport] {
	return func(r *http.Request, input packBuiltinTarget) (*services.PackApplyReport, error) {
		actor, err := requireSystemAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		archive, err := services.BuiltinPackArchive(r.PathValue("name"))
		if err != nil {
			if errors.Is(err, services.ErrBuiltinPackNotFound) {
				return nil, newError(http.StatusNotFound, "not_found", "Built-in pack was not found")
			}
			return nil, internalError(err)
		}
		target, err := input.resolve()
		if err != nil {
			return nil, err
		}
		req := services.PackApplyRequest{Archive: archive, Target: target, Actor: auditActor(r, actor)}
		var report *services.PackApplyReport
		if dryRun {
			report, err = deps.PackApply.Verify(r.Context(), req)
		} else {
			report, err = deps.PackApply.Apply(r.Context(), req)
		}
		if err != nil {
			return nil, internalError(err)
		}
		if !dryRun {
			deps.PackApply.AuditApply(auditActor(r, actor), report.WorkspaceID, report.Pack, report.PackVersion, report.Status)
		}
		return report, nil
	}
}

func registerPackRoutes(b *routeBuilder, deps Deps) {
	b.Read("/packs", AuthAuthenticated, []string{"workspaces:read"}, listBuiltinPacks(deps))
	b.JSON(http.MethodPost, "/packs/{name}/apply", http.StatusOK, false, AuthAuthenticated, []string{"workspaces:write"}, applyBuiltinPack(deps, false))
	b.JSON(http.MethodPost, "/packs/{name}/verify", http.StatusOK, false, AuthAuthenticated, []string{"workspaces:read"}, applyBuiltinPack(deps, true))

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
		// The archive is fully received and parsed; bound the database work
		// that follows instead of charging its transfer time to the budget.
		ctx, cancel := startDBBudget(r.Context())
		defer cancel()
		actor := auditActorFromRequest(r)
		req := services.PackApplyRequest{
			Archive: archive,
			Target:  services.PackApplyTarget{WorkspaceName: target.WorkspaceName},
			Actor:   actor,
		}
		var report *services.PackApplyReport
		if dryRun {
			report, err = deps.PackApply.Verify(ctx, req)
		} else {
			report, err = deps.PackApply.Apply(ctx, req)
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
