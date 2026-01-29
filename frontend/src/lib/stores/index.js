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

// Backlog store
// Access via: backlogStore.count, backlogStore.loading, backlogStore.workspaceId
// Methods: backlogStore.load(wsId), backlogStore.setCount(wsId, count), increment(), decrement()
export { backlogStore } from './backlogStore.svelte.js';

// i18n store
// Access via: i18n.locale, i18n.direction, i18n.isRTL, i18n.loading
// Methods: i18n.init(), i18n.setLocale(code), t(key, params), translateError(error)
// Provides internationalization with support for multiple locales including RTL languages
export { i18n, t, translateError, SUPPORTED_LOCALES } from './i18n.svelte.js';

// Attachment status store
// Access via: attachmentStatus.enabled, attachmentStatus.loaded, attachmentStatus.loading
// Methods: attachmentStatus.load(), attachmentStatus.reload()
// Provides attachment system availability status for UI components
export { attachmentStatus } from './attachmentStatus.svelte.js';

// Work Item Form store
// Access via: workItemFormStore.formData, workItemFormStore.selectedWorkspace,
// workItemFormStore.validate(), workItemFormStore.getFormData(), etc.
// Centralized state management for work item creation form
export { workItemFormStore } from './workItemFormStore.svelte.js';

// Item Detail store
// Access via: itemDetailStore.item, itemDetailStore.workspace, itemDetailStore.editing,
// itemDetailStore.loadItem(wsId, itemId), itemDetailStore.saveField(field, value), etc.
// Centralized state management for item detail view (editing, modals, related data)
export { itemDetailStore } from './itemDetailStore.svelte.js';

// Security store
// Access via: securityStore.credentials, securityStore.apiTokens, securityStore.user,
// securityStore.loadCredentials(), securityStore.createApiToken(), etc.
// Centralized state management for security settings page
export { securityStore } from './securityStore.svelte.js';

// Screen Editor store
// Access via: screenEditorStore.screens, screenEditorStore.screenFields,
// screenEditorStore.loadScreens(), screenEditorStore.saveScreenFields(), etc.
// Centralized state management for screen configuration editor
export { screenEditorStore } from './screenEditorStore.svelte.js';

// Homepage store
// Access via: homepageStore.recentWorkspaces, homepageStore.notifications,
// homepageStore.loadDashboardData(), homepageStore.isOnboarding, etc.
// Centralized state management for homepage/dashboard
export { homepageStore } from './homepageStore.svelte.js';

// Time Entry store
// Access via: timeEntryStore.worklogs, timeEntryStore.filters,
// timeEntryStore.loadWorklogs(), timeEntryStore.saveWorklog(), etc.
// Centralized state management for time tracking entries
export { timeEntryStore } from './timeEntryStore.svelte.js';
