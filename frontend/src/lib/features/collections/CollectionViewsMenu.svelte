<script>
  import { createPopover, melt } from '@melt-ui/svelte';
  import { toStore } from 'svelte/store';
  import ListFilter from '@lucide/svelte/icons/list-filter';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Pencil from '@lucide/svelte/icons/pencil';
  import Check from '@lucide/svelte/icons/check';
  import X from '@lucide/svelte/icons/x';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import { queues } from '../../api/queues.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { successToast, warningToast } from '../../stores/toasts.svelte.js';
  import { collectionStore } from '../../stores/collectionContext.svelte.js';
  import { workspaceDataStore } from '../../stores/workspaceDataStore.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import { authStore } from '../../stores/auth.svelte.js';

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

  let views = $state([]);
  let loading = $state(false);
  let loadError = $state(false);
  let draftName = $state('');
  let error = $state('');
  let editingId = $state(null);
  let editingName = $state('');
  let busy = $state(false);

  const workspaceKey = $derived(workspaceDataStore.workspace?.key ?? null);
  const scopeKey = $derived(`${workspaceKey ?? ''}|${collectionId ?? ''}`);
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
    const key = scopeKey;
    if (!key || (!workspaceKey && !collectionId)) return;
    void loadViews();
  });

  async function loadViews() {
    loading = true;
    loadError = false;
    try {
      const all = (await queues.list(workspaceKey, collectionId)) ?? [];
      views = all.filter((entry) => !entry.builtin);
    } catch (err) {
      console.error('Failed to load collection views:', err);
      loadError = true;
      views = [];
    } finally {
      loading = false;
    }
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

  function applyView(view) {
    onapply?.(parseRows(view.filter_state), view.ql);
    open = false;
  }

  async function createView() {
    const name = draftName.trim();
    if (!name || !activeQl || busy) return;
    busy = true;
    try {
      await queues.create(workspaceKey, collectionId, {
        name,
        ql_query: activeQl,
        filter_state: JSON.stringify(activeRows),
      });
      draftName = '';
      error = '';
      successToast(t('collections.sharedViewSaved'));
      await loadViews();
    } catch (err) {
      console.error('Failed to save view:', err);
      warningToast(t('collections.sharedViewSaveFailed'));
    } finally {
      busy = false;
    }
  }

  async function saveRename(view) {
    const name = editingName.trim();
    if (!name || busy) return;
    busy = true;
    try {
      await queues.update(view.id, {
        name,
        ql_query: view.ql,
        filter_state: view.filter_state ?? null,
      });
      editingId = null;
      editingName = '';
      successToast(t('collections.sharedViewRenamed'));
      await loadViews();
    } catch (err) {
      console.error('Failed to rename view:', err);
      warningToast(t('collections.sharedViewSaveFailed'));
    } finally {
      busy = false;
    }
  }

  async function removeView(view) {
    if (busy) return;
    busy = true;
    try {
      await queues.remove(view.id);
      successToast(t('collections.sharedViewDeleted'));
      await loadViews();
    } catch (err) {
      console.error('Failed to delete view:', err);
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
    } catch (err) {
      console.error('Failed to reorder views:', err);
      warningToast(t('collections.sharedViewReorderFailed'));
    } finally {
      busy = false;
    }
  }
</script>

<div class="flex items-center gap-2" data-testid="collection-shared-views">
  <button
    type="button"
    use:melt={$trigger}
    data-testid="collection-shared-views-menu"
    class="inline-flex items-center gap-1.5 rounded px-3 py-1.5 text-sm font-medium transition-colors"
    style="color: var(--ds-text); background-color: var(--ctx-surface, var(--ds-background-neutral));"
    aria-label={t('collections.sharedViews')}
  >
    <ListFilter class="h-4 w-4" />
    <span>{t('collections.sharedViews')}</span>
  </button>
</div>

{#if $openState}
  <div
    use:melt={$content}
    data-testid="collection-shared-views-panel"
    class="z-[60] w-80 rounded border p-3 shadow-xl"
    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
  >
    {#if canAuthor}
      <div class="mb-3">
        <label
          for="collection-shared-view-name"
          class="mb-1.5 block text-xs font-medium"
          style="color: var(--ds-text-subtle);"
        >
          {t('collections.saveSharedView')}
        </label>
        <div class="flex items-center gap-2">
          <input
            id="collection-shared-view-name"
            data-testid="collection-shared-view-name"
            type="text"
            bind:value={draftName}
            maxlength="120"
            placeholder={t('collections.sharedViewNamePlaceholder')}
            class="min-w-0 flex-1 rounded border px-2 py-1.5 text-sm"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
          />
          <button
            type="button"
            data-testid="collection-shared-view-save"
            class="rounded px-2.5 py-1.5 text-sm font-medium"
            style="background-color: var(--ds-interactive); color: var(--ds-text-on-interactive, #fff);"
            disabled={!activeQl || !draftName.trim() || busy}
            onclick={createView}
          >
            {t('common.save')}
          </button>
        </div>
        {#if !activeQl}
          <p class="mt-1.5 text-xs" style="color: var(--ds-text-subtlest);">
            {t('collections.noActiveFilters')}
          </p>
        {/if}
      </div>
    {:else}
      <p class="mb-3 text-xs" style="color: var(--ds-text-subtlest);">
        {t('collections.viewsReadOnly')}
      </p>
    {/if}

    <div class="border-t pt-2" style="border-color: var(--ds-border);">
      {#if loading}
        <p class="px-1 py-2 text-sm" style="color: var(--ds-text-subtle);">{t('common.loading')}</p>
      {:else if views.length === 0}
        <p class="px-1 py-2 text-sm" style="color: var(--ds-text-subtle);">
          {loadError ? t('common.error') : t('collections.noSharedViews')}
        </p>
      {:else}
        {#each views as view, index (view.id)}
          <div
            class="flex items-center gap-1 rounded px-1 py-1"
            data-testid={`collection-shared-view-row-${view.id}`}
          >
            {#if editingId === view.id}
              <input
                data-testid={`collection-shared-view-rename-input-${view.id}`}
                type="text"
                bind:value={editingName}
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
                data-testid={`collection-shared-view-rename-save-${view.id}`}
                onclick={() => saveRename(view)}
              >
                <Check class="h-3.5 w-3.5" />
              </button>
              <button
                type="button"
                class="rounded p-1"
                style="color: var(--ds-text-subtle);"
                title={t('common.cancel')}
                aria-label={t('common.cancel')}
                onclick={() => { editingId = null; editingName = ''; }}
              >
                <X class="h-3.5 w-3.5" />
              </button>
            {:else}
              <button
                type="button"
                class="min-w-0 flex-1 truncate rounded px-1 py-1 text-left text-sm"
                style="color: var(--ds-text);"
                title={view.name}
                data-testid={`collection-shared-view-apply-${view.id}`}
                onclick={() => applyView(view)}
              >
                {view.name}
              </button>
              {#if canAuthor}
                <button
                  type="button"
                  class="rounded p-1"
                  style="color: var(--ds-text-subtle);"
                  title={t('collections.renameView')}
                  aria-label={t('collections.renameView')}
                  data-testid={`collection-shared-view-rename-${view.id}`}
                  onclick={() => { editingId = view.id; editingName = view.name; }}
                >
                  <Pencil class="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  class="rounded p-1"
                  style="color: var(--ds-text-subtle);"
                  title={t('collections.moveViewUp')}
                  aria-label={t('collections.moveViewUp')}
                  disabled={index === 0}
                  data-testid={`collection-shared-view-up-${view.id}`}
                  onclick={() => moveView(index, -1)}
                >
                  <ArrowUp class="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  class="rounded p-1"
                  style="color: var(--ds-text-subtle);"
                  title={t('collections.moveViewDown')}
                  aria-label={t('collections.moveViewDown')}
                  disabled={index === views.length - 1}
                  data-testid={`collection-shared-view-down-${view.id}`}
                  onclick={() => moveView(index, 1)}
                >
                  <ArrowDown class="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  class="rounded p-1"
                  style="color: var(--ds-text-subtle);"
                  title={t('common.delete')}
                  aria-label={t('common.delete')}
                  data-testid={`collection-shared-view-delete-${view.id}`}
                  onclick={() => removeView(view)}
                >
                  <Trash2 class="h-3.5 w-3.5" />
                </button>
              {/if}
            {/if}
          </div>
        {/each}
      {/if}
    </div>
  </div>
{/if}
