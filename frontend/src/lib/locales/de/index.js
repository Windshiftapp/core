/**
 * German (de) - Aggregated locale module
 * Combines all split locale modules into a single export
 */
import admin from './admin.js';
import auth from './auth.js';
import channels from './channels.js';
import common from './common.js';
import misc from './misc.js';
import navigation from './navigation.js';
import testing from './testing.js';
import time from './time.js';
import ui from './ui.js';
import workflows from './workflows.js';
import workspace from './workspace.js';

export default {
  // Admin related (settings, roles, permissions)
  ...admin,

  // Authentication (auth, users, security, portalLogin)
  ...auth,

  // Common (common, toast, errors, validation, placeholders, emptyStates)
  ...common,

  // Channels and notifications (notifications, channels, channel, portal, requestForm, requestTypeFields)
  ...channels,

  // Misc (sprints, iterations, milestones, assets, personal, connections, migration,
  // migrationAssistant, setup, createModal, scm, organization, fields, itemTypes,
  // categories, members, configuration, audit, auditLog, projects)
  ...misc,

  // Navigation (nav, commandPalette, dashboard, search, about, onboarding)
  ...navigation,

  // Testing (testing, testCase)
  ...testing,

  // Time tracking (time, timeProject, timeProjectCategory)
  ...time,

  // UI components (pickers, editors, dialogs, components, aria, layout, widgets, footer)
  ...ui,

  // Workflows and status (statuses, priorities, workflows, screens, screensPage, statusCategory)
  ...workflows,

  // Workspace (workspaces, items, comments, todo, collectionTree, collections, links)
  ...workspace
};
