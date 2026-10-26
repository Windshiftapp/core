<script>
  import { createPopover, melt } from '@melt-ui/svelte';
  import { toStore } from 'svelte/store';
  import Bookmark from '@lucide/svelte/icons/bookmark';
  import Pin from '@lucide/svelte/icons/pin';
  import PinOff from '@lucide/svelte/icons/pin-off';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import { t } from '../../stores/i18n.svelte.js';
  import { successToast } from '../../stores/toasts.svelte.js';
  import { collectionPreferences } from '../../stores/collectionPreferences.svelte.js';
  import {
    addSavedFilter,
    normalizeSavedFilters,
    orderedSavedFilters,
    removeSavedFilter,
    savedFiltersStorageKey,
    togglePinnedSavedFilter,
  } from './savedBoardFilters.js';

  let {
    workspaceId = null,
    collectionId = null,
    activeQl = '',
    activeRows = [],
    onapply = null,
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

  let filters = $state([]);
  let draftName = $state('');
  let error = $state('');

  const storageKey = $derived(savedFiltersStorageKey(workspaceId, collectionId));
  const pinnedFilters = $derived(filters.filter((entry) => entry.pinned));
  const canSave = $derived(Boolean(activeQl) && draftName.trim().length > 0);

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
    draftName = '';
    error = '';
  });

  function persist(next) {
    filters = next;
    collectionPreferences.setSavedFilters(workspaceId, collectionId, next);
  }

  function errorMessage(code) {
    if (code === 'duplicate_name') return t('collections.filterNameTaken');
    if (code === 'filter_required') return t('collections.noActiveFilters');
    return t('collections.filterNameRequired');
  }

  function saveCurrent() {
    const result = addSavedFilter(filters, {
      name: draftName,
      ql: activeQl,
      rows: JSON.parse(JSON.stringify(activeRows)),
    });
    if (result.error) {
      error = errorMessage(result.error);
      return;
    }
    persist(result.filters);
    draftName = '';
    error = '';
    successToast(t('collections.filterSaved'));
  }

  function applyFilter(entry) {
    onapply?.(entry.rows, entry.ql);
    open = false;
  }

  function togglePinned(entry) {
    persist(togglePinnedSavedFilter(filters, entry.id));
  }

  function remove(entry) {
    persist(removeSavedFilter(filters, entry.id));
    successToast(t('collections.filterDeleted'));
  }

  function handleKeydown(event) {
    if (event.key === 'Enter' && canSave) {
      event.preventDefault();
      saveCurrent();
    }
  }
</script>

<div class="flex items-center gap-2" data-testid="board-saved-filters">
  <button
    type="button"
    use:melt={$trigger}
    data-testid="board-saved-filters-menu"
    class="inline-flex items-center gap-1.5 rounded px-3 py-1.5 text-sm font-medium transition-colors"
    style="color: var(--ds-text); background-color: var(--ctx-surface, var(--ds-background-neutral));"
    aria-label={t('collections.savedFilters')}
  >
    <Bookmark class="h-4 w-4" />
    <span>{t('collections.savedFilters')}</span>
  </button>

  {#each pinnedFilters as entry (entry.id)}
    <button
      type="button"
      data-testid={`board-saved-filter-pinned-${entry.id}`}
      class="inline-flex max-w-40 items-center gap-1 truncate rounded-full border px-2.5 py-1 text-xs transition-colors"
      style="color: var(--ds-text); border-color: var(--ctx-border, var(--ds-border)); background-color: var(--ctx-surface-raised, var(--ds-surface-raised));"
      title={entry.name}
      onclick={() => applyFilter(entry)}
    >
      <Pin class="h-3 w-3 flex-shrink-0" />
      <span class="truncate">{entry.name}</span>
    </button>
  {/each}
</div>

{#if $openState}
  <div
    use:melt={$content}
    data-testid="board-saved-filters-panel"
    class="z-[60] w-80 rounded border p-3 shadow-xl"
    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
  >
    <div class="mb-3">
      <label
        for="board-saved-filter-name"
        class="mb-1.5 block text-xs font-medium"
        style="color: var(--ds-text-subtle);"
      >
        {t('collections.saveCurrentFilter')}
      </label>
      <div class="flex items-center gap-2">
        <input
          id="board-saved-filter-name"
          data-testid="board-saved-filter-name"
          type="text"
          bind:value={draftName}
          onkeydown={handleKeydown}
          maxlength="60"
          placeholder={t('collections.filterNamePlaceholder')}
          class="min-w-0 flex-1 rounded border px-2 py-1.5 text-sm"
          style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
        />
        <button
          type="button"
          data-testid="board-saved-filter-save"
          class="rounded px-2.5 py-1.5 text-sm font-medium"
          style="background-color: var(--ds-interactive); color: var(--ds-text-on-interactive, #fff);"
          disabled={!canSave}
          onclick={saveCurrent}
        >
          {t('common.save')}
        </button>
      </div>
      {#if error}
        <p class="mt-1.5 text-xs" style="color: var(--ds-text-danger);" data-testid="board-saved-filter-error">
          {error}
        </p>
      {:else if !activeQl}
        <p class="mt-1.5 text-xs" style="color: var(--ds-text-subtlest);">
          {t('collections.noActiveFilters')}
        </p>
      {/if}
    </div>

    <div class="border-t pt-2" style="border-color: var(--ds-border);">
      {#if filters.length === 0}
        <p class="px-1 py-2 text-sm" style="color: var(--ds-text-subtle);">
          {t('collections.noSavedFilters')}
        </p>
      {:else}
        {#each orderedSavedFilters(filters) as entry (entry.id)}
          <div
            class="group flex items-center gap-1 rounded px-1 py-1"
            data-testid={`board-saved-filter-row-${entry.id}`}
          >
            <button
              type="button"
              class="min-w-0 flex-1 truncate rounded px-1 py-1 text-left text-sm"
              style="color: var(--ds-text);"
              title={entry.name}
              data-testid={`board-saved-filter-apply-${entry.id}`}
              onclick={() => applyFilter(entry)}
            >
              {entry.name}
            </button>
            <button
              type="button"
              class="rounded p-1"
              style="color: var(--ds-text-subtle);"
              title={entry.pinned ? t('collections.unpinFilter') : t('collections.pinFilter')}
              aria-label={entry.pinned ? t('collections.unpinFilter') : t('collections.pinFilter')}
              aria-pressed={entry.pinned}
              data-testid={`board-saved-filter-pin-${entry.id}`}
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
              class="rounded p-1"
              style="color: var(--ds-text-subtle);"
              title={t('collections.deleteFilter')}
              aria-label={t('collections.deleteFilter')}
              data-testid={`board-saved-filter-delete-${entry.id}`}
              onclick={() => remove(entry)}
            >
              <Trash2 class="h-3.5 w-3.5" />
            </button>
          </div>
        {/each}
      {/if}
    </div>
  </div>
{/if}
