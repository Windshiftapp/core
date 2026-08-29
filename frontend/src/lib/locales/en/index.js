/**
 * English (en) translations - Aggregated module
 * Default locale - bundled with the application
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
import teams from './teams.js';
import testing from './testing.js';
import time from './time.js';
import ui from './ui.js';
import workflows from './workflows.js';
import workspace from './workspace.js';
import pages from './pages.js';
import adminOperations from './adminOperations.js';
import agents from './agents.js';

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
  teams,
  pages,
  adminOperations,
  agents,
});
