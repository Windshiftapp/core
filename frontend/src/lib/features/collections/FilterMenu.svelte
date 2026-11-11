<script>
  import { untrack } from 'svelte';
  import { createPopover, melt } from '@melt-ui/svelte';
  import { toStore } from 'svelte/store';
  import Filter from '@lucide/svelte/icons/filter';
  import Plus from '@lucide/svelte/icons/plus';
  import Users from '@lucide/svelte/icons/users';
  import Bookmark from '@lucide/svelte/icons/bookmark';
  import Pin from '@lucide/svelte/icons/pin';
  import PinOff from '@lucide/svelte/icons/pin-off';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Pencil from '@lucide/svelte/icons/pencil';
  import Check from '@lucide/svelte/icons/check';
  import X from '@lucide/svelte/icons/x';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import FilterBuilderModal from './FilterBuilderModal.svelte';
  import { isFilterRowActive } from '../shared/filterOperators.js';
  import { queues } from '../../api/queues.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { successToast, warningToast } from '../../stores/toasts.svelte.js';
  import { collectionPreferences } from '../../stores/collectionPreferences.svelte.js';
  import { collectionStore } from '../../stores/collectionContext.svelte.js';
  import { workspaceDataStore } from '../../stores/workspaceDataStore.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import { authStore } from '../../stores/auth.svelte.js';
  import { QLBuilder } from '../../utils/ql.js';
  import {
    addSavedFilter,
    normalizeSavedFilters,
    orderedSavedFilters,
    removeSavedFilter,
    savedFiltersStorageKey,
    togglePinnedSavedFilter,
  } from './savedBoardFilters.js';

  // One entry point for collection filters: team-shared views, the user's own
  // saved filters, and the builder modal that creates either.
  let {
    workspaceId = null,
    collectionId = null,
    activeRows = [],
    applied = false,
    onapply = () => {},
    onclear = () => {},
  } = $props();

  let open = $state(false);
  const controlledOpen = toStore(
    () => open,
    (value) => {
      if (open === value) return;
      open = value;
    }
  );
  const {
    elements: { trigger, content },
    states: { open: openState },
  } = createPopover(
    /** @type {any} */ ({
      forceVisible: true,
      positioning: { strategy: 'fixed', fitViewport: true, placement: 'bottom-start' },
      portal: 'body',
      open: controlledOpen,
    })
  );

  let builderOpen = $state(false);
  let builderSaveMode = $state(false);

  // --- Personal filters ---------------------------------------------------

  let filters = $state([]);
  let editingPersonalId = $state(null);
  let editingPersonalName = $state('');

  const storageKey = $derived(savedFiltersStorageKey(workspaceId, collectionId));

  // The server value wins; the localStorage entry stays readable as a fallback
  // for settings saved before they moved into user preferences.
  $effect(() => {
    const serverFilters = collectionPreferences.getSavedFilters(workspaceId, collectionId);
    if (serverFilters) {
      filters = normalizeSavedFilters(serverFilters);
    } else {
      try {
        filters = normalizeSavedFilters(localStorage.getItem(storageKey));
      } catch {
        filters = [];
      }
    }
  });

  function persist(next) {
    filters = next;
    collectionPreferences.setSavedFilters(workspaceId, collectionId, next);
  }

  function removePersonal(entry) {
    const wasApplied = isApplied(entry);
    persist(removeSavedFilter(filters, entry.id));
    if (wasApplied) onclear();
    successToast(t('collections.filterDeleted'));
  }

  function togglePinned(entry) {
    persist(togglePinnedSavedFilter(filters, entry.id));
  }

  function startRenamePersonal(entry) {
    editingPersonalId = entry.id;
    editingPersonalName = entry.name;
  }

  function commitRenamePersonal(entry) {
    const name = editingPersonalName.trim();
    if (!name) return;
    persist(filters.map((current) => (current.id === entry.id ? { ...current, name } : current)));
    editingPersonalId = null;
    editingPersonalName = '';
  }

  // --- Shared views -------------------------------------------------------

  let views = $state([]);
  let viewsLoading = $state(false);
  let viewsError = $state(false);
  let busy = $state(false);
  let editingViewId = $state(null);
  let editingViewName = $state('');

  const workspaceKey = $derived(workspaceDataStore.workspace?.key ?? null);
  // A collection-scoped list ignores the workspace key, so its request scope is
  // the collection id alone. Keying the effect on the workspace too re-issued
  // the same read when the shell resolved the workspace during bootstrap.
  const requestScope = $derived(
    collectionId
      ? `collection:${collectionId}`
      : workspaceKey
        ? `workspace:${workspaceKey}`
        : null
  );
  // The backend lets a collection's owner manage its views; the workspace
  // default view belongs to workspace admins.
  const canAuthor = $derived(
    collectionId
      ? Boolean(
          collectionStore.boardCollection &&
            authStore.currentUser?.id != null &&
            String(collectionStore.boardCollection.created_by) === String(authStore.currentUser.id)
        )
      : workspacePermissions.canAdminWorkspace(workspaceId)
  );

  $effect(() => {
    const scope = requestScope;
    if (!scope) return;
    // loadViews reads the workspace key and collection id synchronously; keep
    // those reads out of the effect so a bootstrap key change for the same
    // request scope cannot fire a second identical read.
    untrack(() => loadViews());
  });

  async function loadViews() {
    viewsLoading = true;
    viewsError = false;
    try {
      const all = (await queues.list(workspaceKey, collectionId)) ?? [];
      views = all.filter((entry) => !entry.builtin);
    } catch (error) {
      console.error('Failed to load collection views:', error);
      viewsError = true;
      views = [];
    } finally {
      viewsLoading = false;
    }
  }

  async function renameView(view) {
    const name = editingViewName.trim();
    if (!name || busy) return;
    busy = true;
    try {
      await queues.update(view.id, {
        name,
        ql_query: view.ql,
        filter_state: view.filter_state ?? null,
      });
      editingViewId = null;
      editingViewName = '';
      successToast(t('collections.sharedViewRenamed'));
      await loadViews();
    } catch (error) {
      console.error('Failed to rename view:', error);
      warningToast(t('collections.sharedViewSaveFailed'));
    } finally {
      busy = false;
    }
  }

  async function removeView(view) {
    if (busy) return;
    const wasApplied = isApplied({ ql: view.ql });
    busy = true;
    try {
      await queues.remove(view.id);
      if (wasApplied) onclear();
      successToast(t('collections.sharedViewDeleted'));
      await loadViews();
    } catch (error) {
      console.error('Failed to delete view:', error);
      warningToast(t('collections.sharedViewDeleteFailed'));
    } finally {
      busy = false;
    }
  }

  async function moveView(index, delta) {
    const target = index + delta;
    if (target < 0 || target >= views.length || busy) return;
    const next = views.slice();
    [next[index], next[target]] = [next[target], next[index]];
    busy = true;
    try {
      await queues.reorder(workspaceKey, collectionId, next.map((entry) => entry.id));
      views = next;
    } catch (error) {
      console.error('Failed to reorder views:', error);
      warningToast(t('collections.sharedViewReorderFailed'));
    } finally {
      busy = false;
    }
  }

  // --- Applying and saving ------------------------------------------------

  const hasActiveFilter = $derived(applied || activeRows.some(isFilterRowActive));
  // Built from the live rows so the menu can show what is applied and tell
  // which saved entry it came from. Deleting that entry must clear it too.
  const activeQl = $derived(
    QLBuilder.buildQuery({ dynamicFields: activeRows.filter(isFilterRowActive) }) || ''
  );
  const activeRowsList = $derived(activeRows.filter(isFilterRowActive));

  function isApplied(entry) {
    return Boolean(activeQl) && activeQl === entry.ql;
  }

  function describeRow(row) {
    const field = row?.field;
    const label = field?.label || field?.name || field?.id || t('common.filter');
    const operator = row?.operator || '=';
    const value = row?.values?.length ? row.values.join(', ') : row?.value;
    return value == null || value === '' ? `${label} ${operator}` : `${label} ${operator} ${value}`;
  }

  function parseRows(filterState) {
    if (!filterState) return [];
    try {
      const parsed = JSON.parse(filterState);
      return Array.isArray(parsed) ? parsed : [];
    } catch {
      return [];
    }
  }

  function applyRows(rows, ql) {
    onapply(rows, ql ?? QLBuilder.buildQuery({ dynamicFields: rows }) ?? '');
    open = false;
  }

  function applyShared(view) {
    applyRows(parseRows(view.filter_state), view.ql);
  }

  function applyPersonal(entry) {
    applyRows(entry.rows, entry.ql);
  }

  function openBuilder(saveMode) {
    open = false;
    builderSaveMode = saveMode;
    builderOpen = true;
  }

  function errorMessage(code) {
    if (code === 'duplicate_name') return t('collections.filterNameTaken');
    if (code === 'filter_required') return t('collections.noActiveFilters');
    return t('collections.filterNameRequired');
  }

  async function saveFilter({ name, shared, rows }) {
    const ql = QLBuilder.buildQuery({ dynamicFields: rows }) || '';
    if (!ql) return { ok: false, error: t('collections.noActiveFilters') };

    if (shared) {
      if (!canAuthor) return { ok: false, error: t('collections.viewsReadOnly') };
      try {
        await queues.create(workspaceKey, collectionId, {
          name,
          ql_query: ql,
          filter_state: JSON.stringify(rows),
        });
        successToast(t('collections.sharedViewSaved'));
        await loadViews();
      } catch (error) {
        console.error('Failed to save view:', error);
        return { ok: false, error: t('collections.sharedViewSaveFailed') };
      }
    } else {
      const result = addSavedFilter(filters, { name, ql, rows });
      if (result.error) return { ok: false, error: errorMessage(result.error) };
      persist(result.filters);
      successToast(t('collections.filterSaved'));
    }

    onapply(rows, ql);
    return { ok: true };
  }
</script>

<div class="flex items-center gap-2" data-testid="collection-filter-menu">
  <button
    type="button"
    use:melt={$trigger}
    data-testid="dynamic-filter-toggle"
    class="inline-flex items-center gap-1.5 rounded px-3 py-1.5 text-sm font-medium transition-colors"
    style={hasActiveFilter
      ? 'color: var(--ds-text); background-color: var(--ctx-surface-raised, var(--ds-surface-raised));'
      : 'color: var(--ds-text); background-color: var(--ctx-surface, var(--ds-background-neutral));'}
    aria-label={t('collections.filters')}
  >
    <Filter class="h-4 w-4" />
    <span>{t('common.filter')}</span>
    {#if hasActiveFilter}
      <span class="inline-flex items-center justify-center min-w-[1.25rem] h-5 px-1.5 text-xs font-semibold rounded-full" style="background-color: var(--ds-interactive-subtle, var(--ds-background-neutral)); color: var(--ds-text);">
        {activeRows.filter(isFilterRowActive).length || 1}
      </span>
    {/if}
  </button>
</div>

{#if $openState}
  <div
    use:melt={$content}
    data-testid="filter-menu"
    class="z-[60] max-h-[70vh] w-80 overflow-y-auto rounded border p-3 shadow-xl"
    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
  >
    <!-- Active -->
    {#if hasActiveFilter}
      <section class="mb-3" data-testid="filter-menu-active">
        <div class="mb-1.5 flex items-center justify-between gap-2">
          <h4 class="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-ds-text-subtle">
            <Filter class="h-3.5 w-3.5" />
            {t('common.active')}
          </h4>
          <button
            type="button"
            class="rounded px-1.5 py-0.5 text-xs font-medium text-ds-interactive"
            data-testid="filter-menu-clear"
            onclick={() => onclear()}
          >
            {t('common.clear')}
          </button>
        </div>
        {#if activeRowsList.length > 0}
          <div class="flex flex-col gap-1">
            {#each activeRowsList as row, index (index)}
              <div
                class="truncate rounded px-1 py-1 text-sm text-ds-text"
                title={describeRow(row)}
                data-testid={`filter-menu-active-row-${index}`}
              >
                {describeRow(row)}
              </div>
            {/each}
          </div>
        {/if}
      </section>
    {/if}

    <!-- Shared -->
    <section>
      <h4 class="mb-1.5 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide" style="color: var(--ds-text-subtle);">
        <Users class="h-3.5 w-3.5" />
        {t('collections.sharedFilters')}
      </h4>
      {#if viewsLoading}
        <p class="px-1 py-2 text-sm" style="color: var(--ds-text-subtle);">{t('common.loading')}</p>
      {:else if views.length === 0}
        <p class="px-1 py-2 text-sm" style="color: var(--ds-text-subtle);" data-testid="filter-menu-shared-empty">
          {viewsError ? t('common.error') : t('collections.noSharedViews')}
        </p>
      {:else}
        {#each views as view, index (view.id)}
          <div class="group flex items-center gap-1 rounded px-1 py-1" data-testid={`filter-menu-shared-${view.id}`}>
            {#if editingViewId === view.id}
              <input
                data-testid={`filter-menu-shared-rename-input-${view.id}`}
                type="text"
                bind:value={editingViewName}
                maxlength="120"
                class="min-w-0 flex-1 rounded border px-2 py-1 text-sm"
                style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
              />
              <button
                type="button"
                class="rounded p-1"
                style="color: var(--ds-icon-brand);"
                title={t('common.save')}
                aria-label={t('common.save')}
                data-testid={`filter-menu-shared-rename-save-${view.id}`}
                onclick={() => renameView(view)}
              >
                <Check class="h-3.5 w-3.5" />
              </button>
              <button
                type="button"
                class="rounded p-1"
                style="color: var(--ds-text-subtle);"
                title={t('common.cancel')}
                aria-label={t('common.cancel')}
                onclick={() => { editingViewId = null; editingViewName = ''; }}
              >
                <X class="h-3.5 w-3.5" />
              </button>
            {:else}
              <button
                type="button"
                class="min-w-0 flex-1 truncate rounded px-1 py-1 text-left text-sm"
                style="color: var(--ds-text);"
                title={view.name}
                data-testid={`filter-menu-shared-apply-${view.id}`}
                onclick={() => applyShared(view)}
              >
                {view.name}
              </button>
              {#if isApplied({ ql: view.ql })}
                <Check class="h-3.5 w-3.5 flex-shrink-0 text-ds-interactive" data-testid={`filter-menu-shared-applied-${view.id}`} />
              {/if}
              {#if canAuthor}
                <button
                  type="button"
                  class="rounded p-1 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100"
                  style="color: var(--ds-text-subtle);"
                  title={t('collections.renameView')}
                  aria-label={t('collections.renameView')}
                  data-testid={`filter-menu-shared-rename-${view.id}`}
                  onclick={() => { editingViewId = view.id; editingViewName = view.name; }}
                >
                  <Pencil class="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  class="rounded p-1 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100"
                  style="color: var(--ds-text-subtle);"
                  title={t('collections.moveViewUp')}
                  aria-label={t('collections.moveViewUp')}
                  disabled={index === 0}
                  data-testid={`filter-menu-shared-up-${view.id}`}
                  onclick={() => moveView(index, -1)}
                >
                  <ArrowUp class="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  class="rounded p-1 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100"
                  style="color: var(--ds-text-subtle);"
                  title={t('collections.moveViewDown')}
                  aria-label={t('collections.moveViewDown')}
                  disabled={index === views.length - 1}
                  data-testid={`filter-menu-shared-down-${view.id}`}
                  onclick={() => moveView(index, 1)}
                >
                  <ArrowDown class="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  class="rounded p-1 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100"
                  style="color: var(--ds-text-subtle);"
                  title={t('common.delete')}
                  aria-label={t('common.delete')}
                  data-testid={`filter-menu-shared-delete-${view.id}`}
                  onclick={() => removeView(view)}
                >
                  <Trash2 class="h-3.5 w-3.5" />
                </button>
              {/if}
            {/if}
          </div>
        {/each}
      {/if}
    </section>

    <!-- Personal -->
    <section class="mt-3 border-t pt-3" style="border-color: var(--ds-border);">
      <h4 class="mb-1.5 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide" style="color: var(--ds-text-subtle);">
        <Bookmark class="h-3.5 w-3.5" />
        {t('collections.myFilters')}
      </h4>
      {#if filters.length === 0}
        <p class="px-1 py-2 text-sm" style="color: var(--ds-text-subtle);" data-testid="filter-menu-personal-empty">
          {t('collections.noSavedFilters')}
        </p>
      {:else}
        {#each orderedSavedFilters(filters) as entry (entry.id)}
          <div class="group flex items-center gap-1 rounded px-1 py-1" data-testid={`filter-menu-personal-${entry.id}`}>
            {#if editingPersonalId === entry.id}
              <input
                data-testid={`filter-menu-personal-rename-input-${entry.id}`}
                type="text"
                bind:value={editingPersonalName}
                maxlength="60"
                class="min-w-0 flex-1 rounded border px-2 py-1 text-sm"
                style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
              />
              <button
                type="button"
                class="rounded p-1"
                style="color: var(--ds-icon-brand);"
                title={t('common.save')}
                aria-label={t('common.save')}
                data-testid={`filter-menu-personal-rename-save-${entry.id}`}
                onclick={() => commitRenamePersonal(entry)}
              >
                <Check class="h-3.5 w-3.5" />
              </button>
              <button
                type="button"
                class="rounded p-1"
                style="color: var(--ds-text-subtle);"
                title={t('common.cancel')}
                aria-label={t('common.cancel')}
                onclick={() => { editingPersonalId = null; editingPersonalName = ''; }}
              >
                <X class="h-3.5 w-3.5" />
              </button>
            {:else}
              <button
                type="button"
                class="min-w-0 flex-1 truncate rounded px-1 py-1 text-left text-sm"
                style="color: var(--ds-text);"
                title={entry.name}
                data-testid={`filter-menu-personal-apply-${entry.id}`}
                onclick={() => applyPersonal(entry)}
              >
                {entry.name}
              </button>
              {#if isApplied(entry)}
                <Check class="h-3.5 w-3.5 flex-shrink-0 text-ds-interactive" data-testid={`filter-menu-personal-applied-${entry.id}`} />
              {/if}
              <button
                type="button"
                class="rounded p-1 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100"
                style="color: var(--ds-text-subtle);"
                title={entry.pinned ? t('collections.unpinFilter') : t('collections.pinFilter')}
                aria-label={entry.pinned ? t('collections.unpinFilter') : t('collections.pinFilter')}
                aria-pressed={entry.pinned}
                data-testid={`filter-menu-personal-pin-${entry.id}`}
                onclick={() => togglePinned(entry)}
              >
                {#if entry.pinned}
                  <PinOff class="h-3.5 w-3.5" />
                {:else}
                  <Pin class="h-3.5 w-3.5" />
                {/if}
              </button>
              <button
                type="button"
                class="rounded p-1 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100"
                style="color: var(--ds-text-subtle);"
                title={t('collections.renameView')}
                aria-label={t('collections.renameView')}
                data-testid={`filter-menu-personal-rename-${entry.id}`}
                onclick={() => startRenamePersonal(entry)}
              >
                <Pencil class="h-3.5 w-3.5" />
              </button>
              <button
                type="button"
                class="rounded p-1 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100"
                style="color: var(--ds-text-subtle);"
                title={t('collections.deleteFilter')}
                aria-label={t('collections.deleteFilter')}
                data-testid={`filter-menu-personal-delete-${entry.id}`}
                onclick={() => removePersonal(entry)}
              >
                <Trash2 class="h-3.5 w-3.5" />
              </button>
            {/if}
          </div>
        {/each}
      {/if}
    </section>

    <!-- Actions -->
    <div class="mt-3 flex flex-col gap-1 border-t pt-2" style="border-color: var(--ds-border);">
      <button
        type="button"
        class="flex items-center gap-2 rounded px-2 py-1.5 text-left text-sm font-medium"
        style="color: var(--ds-text);"
        data-testid="filter-menu-add"
        onclick={() => openBuilder(false)}
      >
        <Plus class="h-4 w-4" />
        {t('common.addFilter')}
      </button>
      {#if hasActiveFilter}
        <button
          type="button"
          class="flex items-center gap-2 rounded px-2 py-1.5 text-left text-sm font-medium"
          style="color: var(--ds-text);"
          data-testid="filter-menu-save"
          onclick={() => openBuilder(true)}
        >
          <Bookmark class="h-4 w-4" />
          {t('collections.saveFilter')}
        </button>
      {/if}
    </div>
  </div>
{/if}

<FilterBuilderModal
  bind:isOpen={builderOpen}
  initialRows={activeRows}
  startInSaveMode={builderSaveMode}
  canShare={canAuthor}
  onApply={(rows, ql) => applyRows(rows, ql)}
  onSave={saveFilter}
  onClear={onclear}
/>
