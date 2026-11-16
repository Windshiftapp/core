import { BUCKET } from './buckets.js';

/**
 * @typedef {Object} Command
 * @property {string} id           Stable unique id. Used as the React-style key and dedupe key.
 * @property {string} label        Primary display text.
 * @property {string} [description] Secondary display text shown under the label.
 * @property {string} bucket       One of BUCKET.*. Drives grouping + display order.
 * @property {string[]} [keywords] Explicit search synonyms (e.g. ['kanban','cards'] for Board).
 *                                 Tokens from `label` are derived at score time, not stored here.
 * @property {string} [url]        Navigation target. Either `url` or `execute` must be set.
 * @property {() => void | Promise<void>} [execute] Custom action. Receives no args; close palette afterward.
 * @property {string} [submenu]    Opens a sub-palette instead of navigating; CommandPalette intercepts it (e.g. 'recent').
 * @property {(ctx: CommandContext) => boolean} [isAvailable] Optional gate; defaults to true.
 * @property {string} [source]     Provider name, for debug overlay.
 * @property {string} [searchQuery] Owning normalized server query; local scores order but do not discard its hits.
 * @property {boolean} [_isContextCommand] Legacy field (contextCommands.js); displayed as a chip.
 */

/**
 * @typedef {Object} CommandContext
 * @property {Object} route        Current route ({ view, params, path }).
 * @property {Object|null} user
 * @property {Object} permissions  Permission store snapshot.
 * @property {boolean} isSystemAdmin
 * @property {Object} modules      Module settings.
 * @property {string|number|null} workspaceId
 * @property {Object|null} workspace
 * @property {string|null} collectionId
 * @property {string|number|null} itemId
 * @property {Object|null} item
 * @property {Object|null} activeTimer
 * @property {string} query
 */

/**
 * Lightweight factory. Validates shape in dev; otherwise returns the input
 * with normalized fields.
 *
 * @param {Command} cmd
 * @returns {Command}
 */
export function createCommand(cmd) {
  if (import.meta.env?.DEV) {
    if (!cmd.id) throw new Error('createCommand: id is required');
    if (!cmd.label) throw new Error(`createCommand[${cmd.id}]: label is required`);
    if (!cmd.bucket) throw new Error(`createCommand[${cmd.id}]: bucket is required`);
    if (!Object.values(BUCKET).includes(cmd.bucket)) {
      throw new Error(`createCommand[${cmd.id}]: unknown bucket "${cmd.bucket}"`);
    }
    if (!cmd.url && typeof cmd.execute !== 'function') {
      throw new Error(`createCommand[${cmd.id}]: either url or execute is required`);
    }
  }
  return {
    keywords: [],
    description: '',
    ...cmd,
  };
}

/**
 * Derive a bucket for legacy commands that don't carry one yet. Used during
 * the migration so the Phase 1 UI can group existing commands without
 * touching every call site. Phase 3 removes this once providers set bucket
 * explicitly.
 *
 * @param {Object} legacy
 * @returns {string}
 */
export function deriveLegacyBucket(legacy) {
  if (legacy.bucket) return legacy.bucket;
  if (legacy.type === 'context-action') return BUCKET.ITEM_ACTIONS;
  if (legacy.type === 'workspace-context') return BUCKET.WORKSPACE_NAVIGATION;
  if (legacy.type === 'work-item') return BUCKET.SEARCH_RESULTS;
  if (legacy.type === 'create') return BUCKET.CREATE;
  if (legacy.type === 'time-action') return BUCKET.MODULE_ACTIONS;
  if (legacy.type === 'system-action') return BUCKET.SYSTEM;
  if (legacy.isAdmin) return BUCKET.ADMIN;
  if (legacy.type === 'navigation') return BUCKET.GLOBAL_NAVIGATION;
  return BUCKET.GLOBAL_NAVIGATION;
}
