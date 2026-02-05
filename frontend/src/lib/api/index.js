// Main API barrel export - assembles all domain modules into single api object

import { actions } from './actions.js';
import { securitySettings, setup, system, themes } from './admin.js';
import { ai, llmConnections, llmProviders } from './ai.js';
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
import {
  configurationSets,
  customFields,
  hierarchyLevels,
  itemTypes,
  links,
  linkTypes,
  priorities,
  projectFieldRequirements,
  screens,
} from './configuration.js';
import { del, get, post, put } from './core.js';
import { hub } from './hub.js';
// Domain imports
import { items } from './items.js';
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
  issues,
  jiraImport,
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
import { groups, permissions } from './permissions.js';
import {
  contactRoles,
  customerOrganisations,
  portal,
  portalAuth,
  portalCustomers,
} from './portal.js';
import { itemSCMLinks, scmProviders, userSCM, workspaceSCM } from './scm.js';
import { sso } from './sso.js';
import { tests } from './tests/index.js';
import { time, timer } from './time.js';
import {
  activateUser,
  completeFIDORegistration,
  createApiToken,
  createAppToken,
  createSSHKey,
  createUser,
  deactivateUser,
  deleteUser,
  getApiToken,
  getApiTokens,
  getUser,
  getUserAppTokens,
  getUserCredentials,
  getUsers,
  removeUserCredential,
  resetUserPassword,
  revokeApiToken,
  revokeAppToken,
  startFIDORegistration,
  updateAppToken,
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
  issues,
  customFields,
  projectFieldRequirements,
  workspaces,
  workspaceRoles,
  screens,
  items,
  configurationSets,

  // Users (standalone functions)
  getUsers,
  getUser,
  createUser,
  updateUser,
  updateUserAvatar,
  updateUserRegionalSettings,
  deleteUser,
  resetUserPassword,
  activateUser,
  deactivateUser,

  // Group Management
  groups,

  // User Credentials
  getUserCredentials,
  startFIDORegistration,
  completeFIDORegistration,
  createSSHKey,
  removeUserCredential,

  // App Tokens
  getUserAppTokens,
  createAppToken,
  updateAppToken,
  revokeAppToken,

  // API Tokens
  getApiTokens,
  createApiToken,
  getApiToken,
  revokeApiToken,
  validateApiToken,

  // Status Categories
  statusCategories,

  // Statuses
  statuses,

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

  // Portal Hub API (centralized portal management)
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

  // Security Settings (admin only)
  securitySettings,

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

  // AI features
  ai,

  // LLM connection management (admin)
  llmConnections,

  // LLM provider info (user)
  llmProviders,
};

// Security settings exports
export {
  authPolicy,
  getAuthPolicy,
  getAuthPolicyAffected,
  getAuthPolicyPublicStatus,
  getAuthPolicyStats,
  getSecuritySettings,
  updateAuthPolicy,
  updateSecuritySettings,
} from './admin.js';
// Calendar feed exports
export { createCalendarFeedToken, getCalendarFeedToken, revokeCalendarFeedToken } from './misc.js';
// Export helper functions for backward compatibility
export {
  createNotificationSetting,
  deleteNotificationSetting,
  getAvailableNotificationEvents,
  getNotificationSetting,
  getNotificationSettings,
  updateNotificationSetting,
} from './notifications.js';
