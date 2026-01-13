// Barrel export for all stores - allows importing from './stores' instead of individual files

// Auth store
// Access via: authStore.isAuthenticated, authStore.currentUser, authStore.loading, authStore.error
export { authStore } from './auth.svelte.js';

// Permission store
// Access via: permissionStore.hasPermission(id), permissionStore.hasPermissionKey(key),
// permissionStore.isSystemAdmin, permissionStore.canAccessAdmin, permissionStore.canAccessCustomers
// Also exports isSystemAdmin as standalone derived store for backward compatibility
export { permissionStore, isSystemAdmin } from './permissions.svelte.js';

// Workspace stores
// Access currentWorkspace.workspace and workspacesStore properties:
// workspacesStore.allWorkspaces, workspacesStore.regularWorkspaces,
// workspacesStore.personalWorkspace, workspacesStore.loaded, workspacesStore.loading
export { currentWorkspace, workspacesStore } from './workspaces.svelte.js';

// Testing store
// Access via: testingStore.testCases, testingStore.testSets, testingStore.testRuns,
// testingStore.selectedSet, testingStore.selectedRun, testingStore.currentView
export { testingStore } from './testing.svelte.js';

// UI store
// Access via: uiStore.reviewFullscreen
export { uiStore } from './ui.svelte.js';

// Navigation configuration
// Access via: shouldNavigateAfterCreate(viewName), getNavigationConfig()
export { shouldNavigateAfterCreate, getNavigationConfig } from './navigationConfig.svelte.js';

// Search store
// Access via: searchStore.setSearchQuery(), searchStore.toggleWorkspace(), searchStore.executeSearch(), etc.
// Provides centralized state for search/filter functionality with auto-computed QL
export { searchStore } from './searchStore.svelte.js';

// Workspace permissions store
// Access via: workspacePermissions.canView(wsId), workspacePermissions.canEdit(wsId),
// workspacePermissions.canDelete(wsId), workspacePermissions.canViewTests(wsId), etc.
// Provides workspace-scoped permission checking for UI element visibility
export { workspacePermissions } from './workspacePermissions.svelte.js';

// SSO store
// Access via: ssoStore.enabled, ssoStore.providerName, ssoStore.allowPasswordLogin,
// ssoStore.initStatus(), ssoStore.startLogin(), ssoStore.loadProviders()
// Manages SSO status, provider configuration, and external account linking
export { ssoStore } from './sso.svelte.js';
