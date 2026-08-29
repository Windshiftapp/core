/**
 * Arabic (ar) translations - Aggregated module
 * RTL language - Right-to-left text direction
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
import pages from './pages.js';
import teams from './teams.js';
import supplemental from './supplemental.js';
import adminOperations from './adminOperations.js';
import dashboard from './dashboard.js';

export default createLocale({
  common,
  auth,
  workspace,
  admin,
  testing,
  time,
  channels,
  workflows,
  ui,
  navigation,
  misc,
  actions,
  logbook,
  analytics,
  pages,
  teams,
  supplemental,
  adminOperations,
  dashboard,
});
