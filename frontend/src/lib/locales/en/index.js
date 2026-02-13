/**
 * English (en) translations - Aggregated module
 * Default locale - bundled with the application
 */

import actions from './actions.js';
import admin from './admin.js';
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

export default {
  ...common,
  ...auth,
  ...workspace,
  ...admin,
  ...testing,
  ...time,
  ...channels,
  ...workflows,
  ...ui,
  ...navigation,
  ...misc,
  ...actions,
  ...logbook,
};
