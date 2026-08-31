// Main API barrel export - assembles all domain modules into single api object

import { actions } from './actions.js';
import {
  agentTemplates,
  oauthClients,
  securitySettings,
  setup,
  shellBootstrap,
  system,
  themes,
} from './admin.js';
import {
  actionCapabilities,
  actionCredentials,
  ai,
  aiFeatures,
  llmConnections,
  llmProviders,
  runnerPools,
  workItemStaleness,
} from './ai.js';
import { analytics } from './analytics.js';
import { approvalSets } from './approvalSets.js';
import { approvals } from './approvals.js';
import {
  assetCategories,
  assetRoles,
  assetSets,
  assetStatuses,
  assets,
  assetTypes,
  itemLinkedAssets,
} from './assets.js';
import { auth } from './auth.js';
import { assetReports, channelCategories, channels, requestTypes } from './channels.js';
import { collectionCategories, collections } from './collections.js';
import { conditionSets } from './conditionSets.js';
import {
  configurationSets,
  customFields,
  hierarchyLevels,
  itemTemplates,
  itemTypes,
  links,
  linkTypes,
  priorities,
  projectFieldRequirements,
  screens,
} from './configuration.js';
import { del, fetchAPI, get, post, put } from './core.js';
import { emailTemplates } from './email-templates.js';
import { forms } from './forms.js';
import { hub } from './hub.js';
import {
  integrationProviders,
  itemIntegrationLinks,
  todoistSync,
  userIntegrations,
} from './integrations.js';
// Domain imports
import { items } from './items.js';
import { leave } from './leave.js';
import { logbook } from './logbook.js';
import { iterations, iterationTypes, milestoneCategories, milestones } from './milestones.js';
import {
  attachmentSettings,
  attachments,
  calendarFeed,
  createComment,
  createDiagram,
  deleteComment,
  deleteDiagram,
  getComments,
  getDiagram,
  getDiagrams,
  homepage,
  jiraImport,
  labels,
  personalLabels,
  projects,
  reviews,
  search,
  updateComment,
  updateDiagram,
} from './misc.js';
import {
  configurationSetNotifications,
  notificationSettings,
  notifications,
} from './notifications.js';
import { oauth } from './oauth.js';
import { objectTranslations } from './objectTranslations.js';
import { onCallSchedules } from './oncall.js';
import { pageLabels, pages } from './pages.js';
import { groups, permissions } from './permissions.js';
import {
  contactRoles,
  customerOrganisations,
  portal,
  portalAuth,
  portalCustomers,
  portalPasskey,
} from './portal.js';
import { recurrence } from './recurrence.js';
import { issueSync, itemSCMLinks, scmProviders, userSCM, workspaceSCM } from './scm.js';
import { sso } from './sso.js';
import { teams } from './teams.js';
import { tests } from './tests/index.js';
import { time, timer } from './time.js';
import { transitions } from './transitions.js';
import {
  activateUser,
  cliAuth,
  completeFIDORegistration,
  createApiToken,
  createMyAgent,
  createSSHKey,
  createUser,
  deactivateUser,
  deleteMyAgent,
  deleteUser,
  getAgentOwner,
  getApiToken,
  getApiTokens,
  getAssignableUsers,
  getMyAgents,
  getScopeCatalog,
  getUser,
  getUserCredentials,
  getUsers,
  inviteUser,
  removeUserCredential,
  resetUserPassword,
  revokeApiToken,
  startFIDORegistration,
  updateMyAgent,
  updateUser,
  updateUserAvatar,
  updateUserRegionalSettings,
  userPreferences,
  validateApiToken,
} from './users.js';
import { statusCategories, statuses, workflows } from './workflows.js';
import { workspaceRoles, workspaces } from './workspaces.js';

// Assemble the api object with the same structure as the original
export const api = {
  // Generic HTTP methods
  get,
  post,
  put,
  delete: del,

  // Domain objects
  projects,
  customFields,
  projectFieldRequirements,
  workspaces,
  workspaceRoles,
  screens,
  items,
  configurationSets,

  // Users (standalone functions)
  getUsers,
  getAssignableUsers,
  getUser,
  getAgentOwner,
  createUser,
  updateUser,
  updateUserAvatar,
  updateUserRegionalSettings,
  deleteUser,
  inviteUser,
  resetUserPassword,
  activateUser,
  deactivateUser,

  // Group Management
  groups,

  // Teams (cross-workspace orgs with on-call)
  teams,

  // On-call schedules (per-team)
  onCallSchedules,

  // User leave periods (per-user, with optional substitute)
  leave,

  // User Credentials
  getUserCredentials,
  startFIDORegistration,
  completeFIDORegistration,
  createSSHKey,
  removeUserCredential,

  // User-Managed Agents
  getMyAgents,
  createMyAgent,
  updateMyAgent,
  deleteMyAgent,

  // CLI onboarding (consent page + capabilities probe)
  cliAuth,

  // API Tokens
  getApiTokens,
  createApiToken,
  getApiToken,
  revokeApiToken,
  validateApiToken,
  getScopeCatalog,

  // Status Categories
  statusCategories,

  // Statuses
  statuses,

  // Condition Sets
  conditionSets,

  // Approval Sets (admin) + Approvals (runtime)
  approvalSets,
  approvals,

  // Per-transition governance (powers override warnings)
  transitions,

  // Recurrence Rules
  recurrence,

  // Workflows
  workflows,

  // Search
  search,

  // Milestone Categories
  milestoneCategories,

  // Channel Categories
  channelCategories,

  // Milestones
  milestones,

  // Iteration Types
  iterationTypes,

  // Iterations
  iterations,

  // Personal Labels
  personalLabels,

  // Workspace Labels
  labels,

  // Attachments
  attachments,

  // Attachment Settings (for admin)
  attachmentSettings,

  // Diagram API functions
  getDiagrams,
  getDiagram,
  createDiagram,
  updateDiagram,
  deleteDiagram,

  // Comment API functions
  getComments,
  createComment,
  updateComment,
  deleteComment,

  // Time tracking API functions
  time,

  // Tests
  tests,

  // Link Types
  linkTypes,

  // Links
  links,

  // Setup
  setup,

  // Active Timer endpoints
  timer,

  // Item Types
  itemTypes,
  itemTemplates,

  // Priorities
  priorities,

  // Hierarchy Levels
  hierarchyLevels,

  // Request Types (channel-scoped)
  requestTypes,

  // Asset Reports (channel-scoped, for portal asset tables)
  assetReports,

  // Collections
  collections,

  // Collection Categories
  collectionCategories,

  // Notifications
  notifications,

  // Channels
  channels,

  // Authentication
  auth,

  // System operations
  system,

  // Homepage
  homepage,

  // Permissions
  permissions,

  // Notification Settings API
  notificationSettings,

  // Instance-wide configurable-object translations
  objectTranslations,

  // Email Templates API (admin-edited transactional emails)
  emailTemplates,

  // Configuration Set Notification assignments
  configurationSetNotifications,

  // Reviews API (daily/weekly review feature)
  reviews,

  // Themes API (application theming)
  themes,

  // User Preferences API
  userPreferences,

  // Portal API (public endpoints, no authentication)
  portal,

  // Portal Auth API (magic link authentication for portal customers)
  portalAuth,

  // Portal Passkey API (WebAuthn registration/login + banner-prompt state)
  portalPasskey,

  // Portal Hub API (centralized portal management)
  forms,
  hub,

  // Portal Customers Management
  portalCustomers,

  // Contact Roles Management
  contactRoles,

  // Customer Organisations
  customerOrganisations,

  // SSO (Single Sign-On) endpoints
  sso,

  // SCM (Source Control Management) providers
  scmProviders,

  // Workspace SCM connections and repositories
  workspaceSCM,

  // Item SCM Links
  itemSCMLinks,

  // User SCM connections (personal OAuth tokens)
  userSCM,

  // Issue Sync (GitHub Issues → Windshift Items)
  issueSync,

  // Integration Providers (Notion, Confluence, etc.)
  integrationProviders,

  // User Integration Connections
  userIntegrations,

  // Todoist personal-task sync
  todoistSync,

  // Item Integration Links
  itemIntegrationLinks,

  // OAuth Clients (admin only) — third-party apps registered against the
  // generic OAuth 2.0 server (/api/oauth/authorize + /api/oauth/token)
  oauthClients,

  // Agent template catalog (admin only) — system-admin overrides for the
  // Agent Studio creation catalog (WI-922)
  agentTemplates,

  // OAuth 2.0 server (consent flow — /authorize/{info,approve,deny})
  oauth,

  // Security Settings (admin only)
  securitySettings,

  // Authenticated application-shell capability discovery
  shellBootstrap,

  // Calendar Feed
  calendarFeed,

  // Asset Management
  assetSets,
  assetRoles,
  assetTypes,
  assetCategories,
  assetStatuses,
  assets,

  // Item linked assets
  itemLinkedAssets,

  // Jira Cloud Import
  jiraImport,

  // Workspace Actions (automation)
  actions,

  // Knowledge Base / Logbook
  logbook,

  // Workspace knowledge pages (wiki)
  pages,
  pageLabels,

  // AI features
  ai,

  // AI features config (admin)
  aiFeatures,

  // Shared work item staleness threshold (admin)
  workItemStaleness,

  // LLM connection management (admin)
  llmConnections,

  // LLM provider info (user)
  llmProviders,

  // Action Capabilities (admin)
  actionCapabilities,

  // Runner pools (admin; runner_pool capability tokens + instances)
  runnerPools,

  // Action Credentials (admin + workspace; encrypted API tokens)
  actionCredentials,

  // Analytics (workspace-level velocity, CFD, cycle time, forecast)
  analytics,
};

// Security settings exports
export { authPolicy, getSecuritySettings, updateSecuritySettings } from './admin.js';
// Coding-agent harness — workspace bindings (WI-88).
export { agentBindings } from './agentBindings.js';
// Coding-agent harness — runs (WI-91).
export { agentRuns } from './agentRuns.js';
// Coding-agent harness — admin security gate (WI-87).
export { agentSecurity } from './agentSecurity.js';
export { agentSkills } from './agentSkills.js';
// Calendar feed exports
export { createCalendarFeedToken, getCalendarFeedToken, revokeCalendarFeedToken } from './misc.js';
// Core utilities
export { fetchAPI };
