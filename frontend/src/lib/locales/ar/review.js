/**
 * Arabic (ar) review completion.
 *
 * Hand-written translations for keys that previously fell back to English.
 * Merged last so it fills gaps without touching the reviewed base files.
 */
import { createLocale } from '../createLocale.js';
import workspace from './completion/workspace.js';
import admin from './completion/admin.js';
import navigation from './completion/navigation.js';
import channels from './completion/channels.js';
import rest from './completion/rest.js';
import adminOperations from './completion/adminOperations.js';

export default createLocale({
  workspace,
  admin,
  navigation,
  channels,
  rest,
  adminOperations,
});
