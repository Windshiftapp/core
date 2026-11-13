import { get } from 'svelte/store';
import { items } from '../api/items.js';
import { customerOrganisations } from '../api/portal.js';
import { time } from '../api/time.js';
import { currentRoute } from '../router.js';
import { itemModalStore } from './itemModalStore.svelte.js';
import { surfaceError } from './toasts.svelte.js';
import { workspacesStore } from './workspaces.svelte.js';

/**
 * One global time-logging surface.
 *
 * Every entry point (command palette, item detail, /time, mobile, hotkey,
 * timer stop) opens this store and renders through the single `TimeLogHost`
 * mounted in the active authenticated shell. Picker data is loaded lazily and
 * once, then invalidated through `invalidate()`.
 */
class TimeLogStore {
  show = $state(false);
  editingWorklog = $state(null);
  editingDraft = $state(null);
  prefill = $state({});

  projects = $state([]);
  customers = $state([]);
  workItems = $state([]);
  workspaces = $state([]);

  drafts = $state([]);
  draftsLoaded = $state(false);

  // Bumped after any successful create/update/book/discard so views can
  // reload their own worklog lists without a shared cache.
  revision = $state(0);

  #pickerPromise = null;
  // Incremented on identity reset so an in-flight request from a previous
  // account cannot repopulate the picker or draft list.
  #generation = 0;

  get draftCount() {
    return this.drafts.length;
  }

  async #loadPickerData() {
    if (this.#pickerPromise) return this.#pickerPromise;
    const generation = this.#generation;
    const fallback = (promise, label) =>
      promise.catch((err) => {
        surfaceError(err, { context: `loadTimeLogPicker:${label}`, silent: true });
        return [];
      });
    this.#pickerPromise = Promise.all([
      fallback(time.projects.getAll(), 'time projects'),
      fallback(customerOrganisations.getAll(), 'customers'),
      fallback(items.getAll({ limit: 100 }), 'work items'),
      fallback(workspacesStore.load(), 'workspaces'),
    ]).then(([projects, customers, workItems, workspaces]) => {
      if (generation !== this.#generation) return;
      this.projects = projects || [];
      this.customers = customers || [];
      this.workItems = workItems?.data ?? [];
      this.workspaces = workspaces || [];
    });
    return this.#pickerPromise;
  }

  /** Load projects/customers/items/workspaces for the pickers and menu. */
  async ensurePickerData() {
    await this.#loadPickerData();
  }

  /**
   * Open the modal. Explicit prefill wins over resolved context so callers
   * that already know the item (item detail, palette context command) are
   * exact.
   */
  /** @param {any} prefill */
  async open(prefill = {}) {
    const context = /** @type {any} */ ({ ...resolveTimeLogContext(), ...prefill });
    this.editingWorklog = context.editingWorklog ?? null;
    this.editingDraft = context.editingDraft ?? null;
    this.prefill = context;
    this.show = true;
    await this.#loadPickerData();
  }

  /** Open a persisted draft by id (recovery surface). */
  async openDraft(draftId) {
    const draft = await time.worklogDrafts.get(draftId);
    await this.open({ editingDraft: draft });
  }

  close() {
    this.show = false;
    this.editingWorklog = null;
    this.editingDraft = null;
    this.prefill = {};
  }

  invalidate() {
    this.#pickerPromise = null;
  }

  /** Clear identity-scoped state on logout or account change. */
  reset() {
    this.#generation += 1;
    this.#pickerPromise = null;
    this.show = false;
    this.editingWorklog = null;
    this.editingDraft = null;
    this.prefill = {};
    this.projects = [];
    this.customers = [];
    this.workItems = [];
    this.workspaces = [];
    this.drafts = [];
    this.draftsLoaded = false;
  }

  async refreshDrafts() {
    const generation = this.#generation;
    try {
      const drafts = (await time.worklogDrafts.getAll()) || [];
      if (generation !== this.#generation) return;
      this.drafts = drafts;
      this.draftsLoaded = true;
    } catch (err) {
      if (generation !== this.#generation) return;
      surfaceError(err, { context: 'refreshTimeLogDrafts', silent: true });
      this.drafts = [];
      this.draftsLoaded = true;
    }
  }

  async removeDraft(draftId) {
    await time.worklogDrafts.delete(draftId);
    await this.refreshDrafts();
    this.revision += 1;
  }

  markSaved() {
    this.revision += 1;
  }
}

/**
 * Resolve the item/workspace context for a context-free open. Mirrors the
 * surface order in the design: open item modal, then the item-detail page,
 * then the active timer.
 */
export function resolveTimeLogContext() {
  if (itemModalStore.isOpen) {
    return { itemId: itemModalStore.itemId, workspaceId: itemModalStore.workspaceId };
  }
  const route = get(currentRoute);
  if (route?.view === 'item-detail' || route?.view === 'mobile-item-detail') {
    const itemId = Number.parseInt(route.params?.itemId ?? route.params?.id, 10);
    if (Number.isFinite(itemId)) {
      const workspaceId =
        route.view === 'item-detail' && route.params?.itemId
          ? Number.parseInt(route.params?.id, 10)
          : null;
      return { itemId, workspaceId: Number.isFinite(workspaceId) ? workspaceId : null };
    }
  }
  return {};
}

export const timeLogStore = new TimeLogStore();

/**
 * Resolve the workspace a new timer should belong to: the context item's
 * workspace first, then the current workspace route. Returns null when neither
 * is available so callers require an explicit choice instead of inventing one.
 */
export function resolveTimerWorkspaceId() {
  const context = resolveTimeLogContext();
  if (context.workspaceId) return context.workspaceId;
  const route = get(currentRoute);
  const id = Number.parseInt(route?.params?.id, 10);
  return Number.isFinite(id) ? id : null;
}
