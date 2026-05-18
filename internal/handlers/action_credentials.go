package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// ActionCredentialsHandler exposes CRUD for action credentials.
//
//   - Global credentials (workspace_id IS NULL) are managed under /admin/...
//     and require system-admin (gated at the route level).
//   - Workspace-scoped credentials are managed under /workspaces/{id}/... and
//     require PermissionActionCredentialManage in that workspace.
//
// The plaintext secret travels only on POST create and POST rotate; every
// response uses the sanitized DTO so ciphertext and plaintext never leave
// the server.
type ActionCredentialsHandler struct {
	db                database.Database
	service           *services.ActionCredentialService
	permissionService *services.PermissionService
	keyCache          *WorkspaceKeyCache
}

// NewActionCredentialsHandler builds the handler. serverSecret is the shared
// SSO_SECRET; the service binds it to the action-credentials HKDF realm.
func NewActionCredentialsHandler(db database.Database, permissionService *services.PermissionService, keyCache *WorkspaceKeyCache, serverSecret string) *ActionCredentialsHandler {
	return &ActionCredentialsHandler{
		db:                db,
		service:           services.NewActionCredentialService(repository.NewActionCredentialRepository(db), serverSecret),
		permissionService: permissionService,
		keyCache:          keyCache,
	}
}

// ListGlobal returns all global (workspace_id IS NULL) credentials.
// System-admin only (enforced at the route layer).
func (h *ActionCredentialsHandler) ListGlobal(w http.ResponseWriter, r *http.Request) {
	creds, err := h.service.ListGlobal()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, sanitizeList(creds))
}

// ListForWorkspace returns credentials usable in this workspace: rows whose
// workspace_id matches PLUS every global credential.
func (h *ActionCredentialsHandler) ListForWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireWorkspaceIDParam(w, r, h.keyCache, "workspaceId")
	if !ok {
		return
	}
	if !h.requireWorkspaceCredentialAccess(w, r, workspaceID) {
		return
	}
	creds, err := h.service.ListForWorkspace(workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, sanitizeList(creds))
}

// CreateGlobal creates a workspace_id IS NULL credential. System-admin only.
func (h *ActionCredentialsHandler) CreateGlobal(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	req, ok := decodeJSON[models.CreateActionCredentialRequest](w, r)
	if !ok {
		return
	}
	// Globals never carry a workspace_id; the path enforces that.
	req.WorkspaceID = nil
	created, err := h.service.Create(req, &currentUser.ID)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}
	h.auditCredential(r, currentUser, logger.ActionActionCredentialCreate, created)
	respondJSONCreated(w, created.Sanitize())
}

// CreateForWorkspace creates a workspace-scoped credential. Requires
// PermissionActionCredentialManage in the workspace.
func (h *ActionCredentialsHandler) CreateForWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireWorkspaceIDParam(w, r, h.keyCache, "workspaceId")
	if !ok {
		return
	}
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !h.requireWorkspaceCredentialManage(w, r, currentUser.ID, workspaceID) {
		return
	}
	req, ok := decodeJSON[models.CreateActionCredentialRequest](w, r)
	if !ok {
		return
	}
	// Path scope wins — clients can't smuggle a global credential through a
	// workspace endpoint.
	req.WorkspaceID = &workspaceID
	created, err := h.service.Create(req, &currentUser.ID)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}
	h.auditCredential(r, currentUser, logger.ActionActionCredentialCreate, created)
	respondJSONCreated(w, created.Sanitize())
}

// UpdateGlobal updates metadata on a global credential. System-admin only.
func (h *ActionCredentialsHandler) UpdateGlobal(w http.ResponseWriter, r *http.Request) {
	credentialID, ok := requireIDParam(w, r, "credentialId")
	if !ok {
		return
	}
	cred, err := h.service.Get(credentialID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "action_credential")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if cred.WorkspaceID != nil {
		respondNotFound(w, r, "action_credential") // global path can't see workspace creds
		return
	}
	h.handleUpdate(w, r, cred)
}

// UpdateForWorkspace updates metadata on a workspace-scoped credential.
func (h *ActionCredentialsHandler) UpdateForWorkspace(w http.ResponseWriter, r *http.Request) {
	cred, ok := h.requireWorkspaceCredential(w, r)
	if !ok {
		return
	}
	h.handleUpdate(w, r, cred)
}

func (h *ActionCredentialsHandler) handleUpdate(w http.ResponseWriter, r *http.Request, cred *models.ActionCredential) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	req, ok := decodeJSON[models.UpdateActionCredentialRequest](w, r)
	if !ok {
		return
	}
	updated, err := h.service.UpdateMetadata(cred.ID, req)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "action_credential")
			return
		}
		respondValidationError(w, r, err.Error())
		return
	}
	h.auditCredential(r, currentUser, logger.ActionActionCredentialUpdate, updated)
	respondJSONOK(w, updated.Sanitize())
}

// RotateGlobal replaces the secret on a global credential. System-admin only.
func (h *ActionCredentialsHandler) RotateGlobal(w http.ResponseWriter, r *http.Request) {
	credentialID, ok := requireIDParam(w, r, "credentialId")
	if !ok {
		return
	}
	cred, err := h.service.Get(credentialID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "action_credential")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if cred.WorkspaceID != nil {
		respondNotFound(w, r, "action_credential")
		return
	}
	h.handleRotate(w, r, cred)
}

// RotateForWorkspace replaces the secret on a workspace credential.
func (h *ActionCredentialsHandler) RotateForWorkspace(w http.ResponseWriter, r *http.Request) {
	cred, ok := h.requireWorkspaceCredential(w, r)
	if !ok {
		return
	}
	h.handleRotate(w, r, cred)
}

func (h *ActionCredentialsHandler) handleRotate(w http.ResponseWriter, r *http.Request, cred *models.ActionCredential) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	req, ok := decodeJSON[models.RotateActionCredentialRequest](w, r)
	if !ok {
		return
	}
	rotated, err := h.service.Rotate(cred.ID, req)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "action_credential")
			return
		}
		respondValidationError(w, r, err.Error())
		return
	}
	h.auditCredential(r, currentUser, logger.ActionActionCredentialRotate, rotated)
	respondJSONOK(w, rotated.Sanitize())
}

// DeleteGlobal deletes a global credential.
func (h *ActionCredentialsHandler) DeleteGlobal(w http.ResponseWriter, r *http.Request) {
	credentialID, ok := requireIDParam(w, r, "credentialId")
	if !ok {
		return
	}
	cred, err := h.service.Get(credentialID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "action_credential")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if cred.WorkspaceID != nil {
		respondNotFound(w, r, "action_credential")
		return
	}
	h.handleDelete(w, r, cred)
}

// DeleteForWorkspace deletes a workspace credential.
func (h *ActionCredentialsHandler) DeleteForWorkspace(w http.ResponseWriter, r *http.Request) {
	cred, ok := h.requireWorkspaceCredential(w, r)
	if !ok {
		return
	}
	h.handleDelete(w, r, cred)
}

func (h *ActionCredentialsHandler) handleDelete(w http.ResponseWriter, r *http.Request, cred *models.ActionCredential) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if err := h.service.Delete(cred.ID); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditCredential(r, currentUser, logger.ActionActionCredentialDelete, cred)
	w.WriteHeader(http.StatusNoContent)
}

// requireWorkspaceCredential parses the workspace + credential IDs, enforces
// PermissionActionCredentialManage, and returns the credential record only
// if it belongs to that workspace. Returns 404 (not 403) when the credential
// either does not exist or does not belong to the workspace, so we don't
// leak existence of credentials in other workspaces. (Same invariant as
// CheckItemPermission in base.go.)
func (h *ActionCredentialsHandler) requireWorkspaceCredential(w http.ResponseWriter, r *http.Request) (*models.ActionCredential, bool) {
	workspaceID, ok := requireWorkspaceIDParam(w, r, h.keyCache, "workspaceId")
	if !ok {
		return nil, false
	}
	credentialID, ok := requireIDParam(w, r, "credentialId")
	if !ok {
		return nil, false
	}
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return nil, false
	}
	if !h.requireWorkspaceCredentialManage(w, r, currentUser.ID, workspaceID) {
		return nil, false
	}
	cred, err := h.service.Get(credentialID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "action_credential")
		return nil, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	// Workspace path may only see credentials scoped to that workspace.
	// Globals are managed via /admin/... — surface as not-found here to
	// avoid leaking the global pool's row IDs.
	if cred.WorkspaceID == nil || *cred.WorkspaceID != workspaceID {
		respondNotFound(w, r, "action_credential")
		return nil, false
	}
	return cred, true
}

// requireWorkspaceCredentialManage gates write operations on workspace creds.
// Returns 404 on missing permission so we don't leak workspace existence to
// users with no view of it. System-admin always passes.
func (h *ActionCredentialsHandler) requireWorkspaceCredentialManage(w http.ResponseWriter, r *http.Request, userID, workspaceID int) bool {
	hasPerm, err := h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionActionCredentialManage)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if !hasPerm {
		respondNotFound(w, r, "action_credential")
		return false
	}
	return true
}

// requireWorkspaceCredentialAccess gates read on workspace creds. Same as
// manage today — workspace admins are the only users who should see the list,
// since the IDs are referenced by capability config and credential selection
// is an admin task. If we later split read vs write, plumb a separate perm.
func (h *ActionCredentialsHandler) requireWorkspaceCredentialAccess(w http.ResponseWriter, r *http.Request, workspaceID int) bool {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return false
	}
	return h.requireWorkspaceCredentialManage(w, r, currentUser.ID, workspaceID)
}

func (h *ActionCredentialsHandler) auditCredential(r *http.Request, user *models.User, action string, cred *models.ActionCredential) {
	if user == nil || cred == nil {
		return
	}
	scope := "global"
	if cred.WorkspaceID != nil {
		scope = "workspace"
	}
	// Details intentionally hold only non-sensitive metadata. The audit
	// pipeline's sanitizeAuditDetails additionally redacts any key that
	// looks like a secret, but we don't put plaintext here either way.
	logAuditWithDetails(h.db, r, user, action, logger.ResourceActionCredential, &cred.ID, cred.Name, map[string]interface{}{
		"credential_type": cred.CredentialType,
		"scope":           scope,
		"workspace_id":    cred.WorkspaceID,
		"is_enabled":      cred.IsEnabled,
		"has_secret":      cred.EncryptedSecret != "",
	})
}

func sanitizeList(creds []*models.ActionCredential) []models.ActionCredentialSanitized {
	out := make([]models.ActionCredentialSanitized, 0, len(creds))
	for _, c := range creds {
		out = append(out, c.Sanitize())
	}
	return out
}
