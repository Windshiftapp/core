package routes

import "net/http"

// RegisterUserRoutes registers user-related routes (users, groups, permissions, credentials, tokens).
func RegisterUserRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth
	admin := deps.PermissionMiddleware.RequireSystemAdmin()

	// User endpoints
	api.HandleH("GET /users", auth(http.HandlerFunc(deps.Users.User.GetAll)))
	api.HandleH("POST /users", admin(deps.AuthRateLimiter.Limit(http.HandlerFunc(deps.Users.User.Create))))
	api.HandleH("GET /users/{id}", auth(http.HandlerFunc(deps.Users.User.Get)))
	api.HandleH("PUT /users/{id}", admin(http.HandlerFunc(deps.Users.User.Update)))
	api.HandleH("DELETE /users/{id}", admin(http.HandlerFunc(deps.Users.User.Delete)))
	api.HandleH("POST /users/{id}/reset-password", admin(deps.AuthRateLimiter.Limit(http.HandlerFunc(deps.Users.User.ResetPassword))))
	api.HandleH("PUT /users/{id}/avatar", auth(http.HandlerFunc(deps.Users.User.UpdateAvatar)))
	api.HandleH("PUT /users/{id}/regional-settings", auth(http.HandlerFunc(deps.Users.User.UpdateRegionalSettings)))
	api.HandleH("POST /users/{id}/activate", admin(http.HandlerFunc(deps.Users.User.ActivateUser)))
	api.HandleH("POST /users/{id}/deactivate", admin(http.HandlerFunc(deps.Users.User.DeactivateUser)))

	// Group endpoints
	api.HandleH("GET /groups", auth(http.HandlerFunc(deps.Users.Group.GetAll)))
	api.HandleH("POST /groups", admin(http.HandlerFunc(deps.Users.Group.Create)))
	api.HandleH("GET /groups/permissions", admin(http.HandlerFunc(deps.Users.Permission.GetAllGroupPermissions)))
	api.HandleH("GET /groups/{id}", auth(http.HandlerFunc(deps.Users.Group.Get)))
	api.HandleH("PUT /groups/{id}", admin(http.HandlerFunc(deps.Users.Group.Update)))
	api.HandleH("DELETE /groups/{id}", admin(http.HandlerFunc(deps.Users.Group.Delete)))
	api.HandleH("POST /groups/{id}/members", admin(http.HandlerFunc(deps.Users.Group.AddMembers)))
	api.HandleH("DELETE /groups/{id}/members", admin(http.HandlerFunc(deps.Users.Group.RemoveMembers)))
	api.HandleH("GET /users/{userId}/groups", auth(http.HandlerFunc(deps.Users.Group.GetUserMemberships)))

	// Permission endpoints
	api.HandleH("GET /permissions", admin(http.HandlerFunc(deps.Users.Permission.GetAllPermissions)))
	api.HandleH("GET /users/{userId}/permissions", auth(http.HandlerFunc(deps.Users.Permission.GetUserPermissions)))
	api.HandleH("POST /permissions/global/grant", admin(http.HandlerFunc(deps.Users.Permission.GrantGlobalPermission)))
	api.HandleH("DELETE /users/{userId}/permissions/global/{permissionId}", admin(http.HandlerFunc(deps.Users.Permission.RevokeGlobalPermission)))
	api.HandleH("POST /permissions/global/grant-group", admin(http.HandlerFunc(deps.Users.Permission.GrantGlobalPermissionToGroup)))
	api.HandleH("DELETE /groups/{groupId}/permissions/global/{permissionId}", admin(http.HandlerFunc(deps.Users.Permission.RevokeGlobalPermissionFromGroup)))

	// Permission Set routes
	api.HandleH("GET /permission-sets", admin(http.HandlerFunc(deps.Users.PermissionSet.GetAll)))
	api.HandleH("POST /permission-sets", admin(http.HandlerFunc(deps.Users.PermissionSet.Create)))
	api.HandleH("GET /permission-sets/{id}", admin(http.HandlerFunc(deps.Users.PermissionSet.Get)))
	api.HandleH("PUT /permission-sets/{id}", admin(http.HandlerFunc(deps.Users.PermissionSet.Update)))
	api.HandleH("DELETE /permission-sets/{id}", admin(http.HandlerFunc(deps.Users.PermissionSet.Delete)))
	api.HandleH("GET /permission-sets/{id}/assignments", admin(http.HandlerFunc(deps.Users.PermissionSet.GetAssignments)))
	api.HandleH("POST /permission-sets/{id}/assignments", admin(http.HandlerFunc(deps.Users.PermissionSet.CreateAssignment)))
	api.HandleH("DELETE /permission-sets/{id}/assignments/{assignmentId}", admin(http.HandlerFunc(deps.Users.PermissionSet.DeleteAssignment)))

	// Workspace Role routes
	api.HandleH("GET /workspace-roles", auth(http.HandlerFunc(deps.Users.WorkspaceRole.GetAll)))
	api.HandleH("GET /workspace-roles/{id}", auth(http.HandlerFunc(deps.Users.WorkspaceRole.Get)))
	api.HandleH("POST /workspace-roles/assign", admin(http.HandlerFunc(deps.Users.WorkspaceRole.AssignRoleToUser)))
	api.HandleH("DELETE /users/{userId}/workspaces/{workspaceId}/roles/{roleId}", admin(http.HandlerFunc(deps.Users.WorkspaceRole.RevokeRoleFromUser)))
	api.HandleH("GET /users/{userId}/workspaces/{workspaceId}/roles", admin(http.HandlerFunc(deps.Users.WorkspaceRole.GetUserRolesInWorkspace)))
	api.HandleH("GET /workspaces/{workspaceId}/role-assignments", admin(http.HandlerFunc(deps.Users.WorkspaceRole.GetWorkspaceRoleAssignments)))

	// User Credential endpoints
	api.HandleH("GET /users/{userId}/credentials", auth(http.HandlerFunc(deps.Users.Credential.GetUserCredentials)))
	api.HandleH("POST /users/{userId}/credentials/webauthn/register/start", auth(deps.FIDORateLimiter.Limit(http.HandlerFunc(deps.Auth.WebAuthn.StartFIDORegistrationNew))))
	api.HandleH("POST /users/{userId}/credentials/webauthn/register/complete", auth(deps.FIDORateLimiter.Limit(http.HandlerFunc(deps.Auth.WebAuthn.CompleteFIDORegistrationNew))))
	api.HandleH("GET /users/{userId}/credentials/webauthn", auth(http.HandlerFunc(deps.Auth.WebAuthn.GetWebAuthnCredentials)))
	api.HandleH("DELETE /users/{userId}/credentials/webauthn/{credentialId}", auth(http.HandlerFunc(deps.Auth.WebAuthn.RemoveWebAuthnCredential)))
	api.HandleH("POST /users/{userId}/credentials/ssh", auth(http.HandlerFunc(deps.Users.Credential.CreateSSHKey)))
	api.HandleH("DELETE /users/{userId}/credentials/{credentialId}", auth(http.HandlerFunc(deps.Users.Credential.RemoveCredential)))

	// App Token endpoints
	api.HandleH("GET /users/{userId}/tokens", auth(http.HandlerFunc(deps.Users.AppToken.GetUserAppTokens)))
	api.HandleH("POST /users/{userId}/tokens", auth(deps.AuthRateLimiter.Limit(http.HandlerFunc(deps.Users.AppToken.CreateAppToken))))
	api.HandleH("PUT /users/{userId}/tokens/{tokenId}", auth(http.HandlerFunc(deps.Users.AppToken.UpdateAppToken)))
	api.HandleH("DELETE /users/{userId}/tokens/{tokenId}", auth(http.HandlerFunc(deps.Users.AppToken.RevokeAppToken)))

	// API Token endpoints
	api.HandleH("POST /api-tokens", auth(deps.AuthRateLimiter.Limit(http.HandlerFunc(deps.Users.APIToken.CreateToken))))
	api.HandleH("GET /api-tokens", auth(http.HandlerFunc(deps.Users.APIToken.GetUserTokens)))
	api.HandleH("GET /api-tokens/{id}", auth(http.HandlerFunc(deps.Users.APIToken.GetToken)))
	api.HandleH("DELETE /api-tokens/{id}", auth(http.HandlerFunc(deps.Users.APIToken.RevokeToken)))
	api.HandleH("GET /api-tokens/validate", auth(http.HandlerFunc(deps.Users.APIToken.ValidateToken)))
}
