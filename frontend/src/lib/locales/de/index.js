/**
 * German (de) - Aggregated locale module
 * Combines all split locale modules into a single export
 */

import { createLocale } from '../createLocale.js';
import actions from './actions.js';
import admin from './admin.js';
import analytics from './analytics.js';
import auth from './auth.js';
import channels from './channels.js';
import common from './common.js';
import logbook from './logbook.js';
import misc from './misc.js';
import navigation from './navigation.js';
import testing from './testing.js';
import time from './time.js';
import ui from './ui.js';
import workflows from './workflows.js';
import workspace from './workspace.js';
import teams from './teams.js';
import pages from './pages.js';
import supplemental from './supplemental.js';
import quality from './quality.js';
import review from './review.js';
import adminOperations from './adminOperations.js';
import dashboard from './dashboard.js';

export default createLocale({
  admin,
  auth,
  common,
  channels,
  misc,
  navigation,
  testing,
  time,
  ui,
  workflows,
  workspace,
  actions,
  logbook,
  analytics,
  pages,
  teams,
  supplemental,
  quality,
  review,
  adminOperations,
  dashboard,
});
