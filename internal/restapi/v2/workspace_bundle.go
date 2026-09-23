package v2

import (
	"encoding/json"
	"errors"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/services"
)

// Workspace bundles (WI-1335): export a workspace's distributable content —
// pages, labels, typed seed items, item links, and optionally its embedded
// configuration-set template — and import a bundle into a target workspace.

func registerWorkspaceBundleRoutes(b *routeBuilder, deps Deps) {
	b.RawResponse[*services.WorkspaceBundle](http.MethodGet, "/workspaces/{workspace_id}/export", http.StatusOK, "application/json", AuthAuthenticated, []string{"workspaces:read"}, func(w http.ResponseWriter, r *http.Request) error {
		user, err := principal(r)
		if err != nil {
			return err
		}
		workspaceID, err := pathID(r, "workspace_id")
		if err != nil {
			return err
		}
		// Read access hides the workspace entirely; admin access gates the
		// bulk content extraction (WI-1335 AC4).
		canView, err := deps.PageAccess.HasWorkspacePermissionFor(user.ID, workspaceID, models.PermissionItemView)
		if err != nil {
			return internalError(err)
		}
		if !canView {
			return newError(http.StatusNotFound, "not_found", "Workspace not found")
		}
		isAdmin, err := deps.PageAccess.HasWorkspacePermissionFor(user.ID, workspaceID, models.PermissionWorkspaceAdmin)
		if err != nil {
			return internalError(err)
		}
		if !isAdmin {
			return newError(http.StatusForbidden, "insufficient_permission", "Workspace admin permission is required to export a workspace bundle")
		}
		bundle, err := deps.WorkspaceBundleExport.Export(r.Context(), workspaceID, &services.ConfigSetExportBy{
			Username: user.Username, Instance: requestInstance(r),
		})
		if err != nil {
			if errors.Is(err, services.ErrWorkspaceBundleWorkspaceNotFound) {
				return newError(http.StatusNotFound, "not_found", "Workspace not found")
			}
			return internalError(err)
		}
		deps.WorkspaceBundleExport.AuditExport(auditActorFromRequest(r), workspaceID)
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(bundle)
	})

	b.JSON(http.MethodPost, "/workspaces/{workspace_id}/import", http.StatusOK, false, AuthAuthenticated, []string{"workspaces:write"}, func(r *http.Request, bundle services.WorkspaceBundle) (*services.WorkspaceBundleImportResult, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		workspaceID, err := pathID(r, "workspace_id")
		if err != nil {
			return nil, err
		}
		if err := services.SanitizeWorkspaceBundle(&bundle); err != nil {
			return nil, newError(http.StatusBadRequest, "invalid_request", err.Error())
		}
		// The importer needs create rights over both content kinds the
		// bundle carries; individual entities re-check through their own
		// application services.
		canCreatePages, err := deps.PageAccess.HasWorkspacePermissionFor(user.ID, workspaceID, models.PermissionPageCreate)
		if err != nil {
			return nil, internalError(err)
		}
		canCreateItems, err := deps.PageAccess.HasWorkspacePermissionFor(user.ID, workspaceID, models.PermissionItemCreate)
		if err != nil {
			return nil, internalError(err)
		}
		if !canCreatePages || !canCreateItems {
			return nil, newError(http.StatusForbidden, "insufficient_permission",
				"Importing a workspace bundle requires page and item create rights on the target workspace")
		}
		result, err := deps.WorkspaceBundleImport.Import(r.Context(), auditActorFromRequest(r), workspaceID, &bundle)
		if err != nil {
			var unresolvedErr *services.ErrUnresolvedReferences
			if errors.As(err, &unresolvedErr) {
				apiErr := newError(http.StatusUnprocessableEntity, "unresolved_references",
					"Bundle import requires references that don't exist in the target workspace")
				apiErr.Details = unresolvedErr.Items
				return nil, apiErr
			}
			if errors.Is(err, services.ErrWorkspaceBundleEmpty) {
				return nil, newError(http.StatusBadRequest, "invalid_request", "Bundle is empty")
			}
			return nil, internalError(err)
		}
		deps.WorkspaceBundleImport.AuditImport(auditActorFromRequest(r), workspaceID, result)
		return result, nil
	})
}
