// Main API barrel export - assembles all domain modules into single api object
import { get, post, put, del } from './core.js';

// Domain imports
import { items } from './items.js';
import { workspaces, workspaceRoles } from './workspaces.js';
import { auth } from './auth.js';
import { time, timer } from './time.js';
import { statusCategories, statuses, workflows } from './workflows.js';
import { assetSets, assetRoles, assetTypes, assetCategories, assetStatuses, assets, itemLinkedAssets } from './assets.js';
import { portal, portalCustomers, contactRoles, customerOrganisations } from './portal.js';
import { scmProviders, workspaceSCM, itemSCMLinks } from './scm.js';
import { channels, channelCategories, requestTypes } from './channels.js';
import { milestoneCategories, milestones, iterationTypes, iterations } from './milestones.js';
import { permissions, groups } from './permissions.js';
import { notifications, notificationSettings, configurationSetNotifications } from './notifications.js';
import {
  configurationSets,
  screens,
  customFields,
  projectFieldRequirements,
  itemTypes,
  priorities,
  hierarchyLevels,
  linkTypes,
  links
} from './configuration.js';
import {
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
  getUserCredentials,
  startFIDORegistration,
  completeFIDORegistration,
  createSSHKey,
  removeUserCredential,
  getUserAppTokens,
  createAppToken,
  updateAppToken,
  revokeAppToken,
  getApiTokens,
  createApiToken,
  getApiToken,
  revokeApiToken,
  validateApiToken,
  userPreferences
} from './users.js';
import { collections, collectionCategories } from './collections.js';
import { sso } from './sso.js';
import { setup, system, themes, securitySettings } from './admin.js';
import {
  projects,
  issues,
  search,
  homepage,
  getDiagrams,
  getDiagram,
  createDiagram,
  updateDiagram,
  deleteDiagram,
  getComments,
  createComment,
  updateComment,
  deleteComment,
  attachments,
  attachmentSettings,
  reviews,
  calendarFeed,
  personalLabels,
  jiraImport
} from './misc.js';
import { tests } from './tests/index.js';

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
};

// Export helper functions for backward compatibility
export {
  getNotificationSettings,
  getNotificationSetting,
  createNotificationSetting,
  updateNotificationSetting,
  deleteNotificationSetting,
  getAvailableNotificationEvents,
} from './notifications.js';

// Security settings exports
export { getSecuritySettings, updateSecuritySettings } from './admin.js';

// Calendar feed exports
export { getCalendarFeedToken, createCalendarFeedToken, revokeCalendarFeedToken } from './misc.js';
