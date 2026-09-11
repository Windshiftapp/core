<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { formatRelativeTime } from '../../utils/dateFormatter.js';
  import { errorToast, successToast } from '../../stores/toasts.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { confirm } from '../../composables/useConfirm.js';
  import LazyMilkdownEditor from '../../editors/LazyMilkdownEditor.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import Badge from '../../components/Badge.svelte';
  import { IconX, IconHistory, IconRestore } from '@tabler/icons-svelte-runes';
  import { loadPageHistory, pageRevisionAuthorName } from './pageHistoryData.js';

  /** Persistent page-revision drawer with read-only previews and restore. It
   * toggles via `open` to preserve loaded history and preview state. */
  let {
    workspaceId,
    pageId = null,
    /** Effective level resolved by PagesView via api.pages.getPermissions.
     *  Used to hide the per-row Restore button for view-only callers. */
    canRestore = true,
    open = $bindable(false),
    onRestored = () => {},
  } = $props();

  let history = $state(/** @type {any[]} */ ([]));
  let loading = $state(false);
  let error = $state('');
  let expandedRevisionId = $state(/** @type {number|null} */ (null));
  let pendingRestoreId = $state(/** @type {number|null} */ (null));
  // Load only when opened for a new page.
  let lastLoadedFor = $state(/** @type {{wsId:any,pageId:any}|null} */ (null));
  $effect(() => {
    if (!open || pageId == null) return;
    const key = { wsId: workspaceId, pageId };
    if (lastLoadedFor && lastLoadedFor.wsId === key.wsId && lastLoadedFor.pageId === key.pageId) return;
    lastLoadedFor = key;
    void loadHistory();
  });

  async function loadHistory() {
    loading = true;
    error = '';
    try {
      history = await loadPageHistory(api, workspaceId, pageId, { limit: 50 });
    } catch (e) {
      error = e?.message || t('pages.history.loadError');
      history = [];
    } finally {
      loading = false;
    }
  }

  function changeTypeColor(t) {
    switch (t) {
      case 'create':
        return 'success';
      case 'edit':
        return 'primary';
      case 'move':
        return 'warning';
      case 'archive':
        return 'danger';
      case 'restore':
        return 'success';
      default:
        return 'neutral';
    }
  }

  function toggle(revisionId) {
    expandedRevisionId = expandedRevisionId === revisionId ? null : revisionId;
  }

  async function restore(rev) {
    const ok = await confirm({
      title: t('pages.history.restoreTitle', { rev: rev.revision_number }),
      message: t('pages.history.restoreMessage'),
      confirmText: t('pages.history.restoreConfirm'),
      variant: 'warning',
    });
    if (!ok) return;
    pendingRestoreId = rev.id;
    try {
      await api.pages.restoreRevision(workspaceId, pageId, rev.id);
      successToast(t('pages.history.restoredOK', { rev: rev.revision_number }));
      // The restore endpoint creates a new revision and reloads via
      // the parent (which also re-derives the page body shown in the
      // editor). We then refresh our own list to surface the new
      // 'restore' entry without making the user reopen the drawer.
      onRestored();
      lastLoadedFor = null;
      void loadHistory();
    } catch (e) {
      errorToast(e?.message || t('pages.history.restoreError'));
    } finally {
      pendingRestoreId = null;
    }
  }

  function close() {
    open = false;
    expandedRevisionId = null;
  }
</script>

{#if open}
  <aside class="drawer" aria-label={t('pages.history.title')} data-testid="pages-history-drawer">
    <header class="drawer-header">
      <div class="title">
        <IconHistory size="18" />
        <span>{t('pages.history.title')}</span>
      </div>
      <button type="button" class="icon-btn" onclick={close} aria-label={t('common.close')} data-testid="pages-history-close">
        <IconX size="18" />
      </button>
    </header>

    <div class="drawer-body">
      {#if loading}
        <div class="state"><Spinner /></div>
      {:else if error}
        <div class="state">
          <p class="error">{error}</p>
          <button type="button" onclick={() => { lastLoadedFor = null; void loadHistory(); }}>
            {t('common.retry')}
          </button>
        </div>
      {:else if history.length === 0}
        <EmptyState title={t('pages.history.empty')} />
      {:else}
        <ul class="revisions">
          {#each history as rev (rev.id)}
            <li class="rev" class:expanded={expandedRevisionId === rev.id} data-testid="pages-history-row" data-revision={rev.revision_number}>
              <button
                type="button"
                class="rev-header"
                onclick={() => toggle(rev.id)}
                aria-expanded={expandedRevisionId === rev.id}
              >
                <span class="rev-number">#{rev.revision_number}</span>
                <Badge variant={changeTypeColor(rev.change_type)}>{rev.change_type}</Badge>
                <span class="rev-author">{pageRevisionAuthorName(rev)}</span>
                <span class="rev-time" title={rev.created_at}>{formatRelativeTime(rev.created_at)}</span>
              </button>

              {#if expandedRevisionId === rev.id}
                <div class="rev-body">
                  {#if rev.title}
                    <div class="rev-title">{rev.title}</div>
                  {/if}
                  <div class="rev-preview">
                    <LazyMilkdownEditor content={rev.content || ''} readonly={true} {workspaceId} />
                  </div>
                  {#if canRestore && rev.id !== history[0]?.id}
                    <div class="rev-actions">
                      <button
                        type="button"
                        class="restore-btn"
                        onclick={() => restore(rev)}
                        disabled={pendingRestoreId === rev.id}
                        data-testid="pages-history-restore"
                      >
                        <IconRestore size="14" />
                        {pendingRestoreId === rev.id
                          ? t('pages.history.restoring')
                          : t('pages.history.restoreAction', { rev: rev.revision_number })}
                      </button>
                    </div>
                  {/if}
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </aside>
{/if}

<style>
  .drawer {
    position: fixed;
    top: 0;
    right: 0;
    bottom: 0;
    width: 420px;
    max-width: 100vw;
    background: var(--ds-surface, #fff);
    border-left: 1px solid var(--ds-border, #e5e7eb);
    box-shadow: -8px 0 24px rgba(0, 0, 0, 0.08);
    display: flex;
    flex-direction: column;
    z-index: 30;
    animation: slide-in 180ms ease-out;
  }
  @keyframes slide-in {
    from { transform: translateX(100%); }
    to   { transform: translateX(0); }
  }
  .drawer-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px;
    border-bottom: 1px solid var(--ds-border, #e5e7eb);
  }
  .title {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    font-weight: 600;
  }
  .icon-btn {
    background: transparent;
    border: 0;
    cursor: pointer;
    color: var(--ds-text-subtle, #6b7280);
    padding: 4px;
    border-radius: 4px;
  }
  .icon-btn:hover { background: var(--ds-surface-hovered, #f3f4f6); }
  .drawer-body {
    flex: 1;
    overflow-y: auto;
    padding: 8px;
  }
  .state {
    padding: 32px 16px;
    text-align: center;
  }
  .error { color: var(--ds-text-danger, #b91c1c); margin-bottom: 8px; }
  .revisions {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .rev {
    border: 1px solid transparent;
    border-radius: 6px;
  }
  .rev.expanded {
    border-color: var(--ds-border, #e5e7eb);
    background: var(--ds-surface-subtle, #f9fafb);
  }
  .rev-header {
    width: 100%;
    display: grid;
    grid-template-columns: auto auto 1fr auto;
    gap: 8px;
    align-items: center;
    padding: 8px 10px;
    background: transparent;
    border: 0;
    cursor: pointer;
    text-align: left;
    font: inherit;
    color: inherit;
    border-radius: 6px;
  }
  .rev-header:hover { background: var(--ds-surface-hovered, #f3f4f6); }
  .rev-number {
    font-variant-numeric: tabular-nums;
    font-weight: 600;
    color: var(--ds-text-subtle, #6b7280);
    min-width: 2.5em;
  }
  .rev-author {
    color: var(--ds-text-subtle, #6b7280);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .rev-time {
    color: var(--ds-text-subtle, #6b7280);
    font-size: 0.9em;
  }
  .rev-body {
    padding: 0 12px 12px;
  }
  .rev-title {
    font-weight: 600;
    margin-bottom: 8px;
  }
  .rev-preview {
    border: 1px solid var(--ds-border, #e5e7eb);
    border-radius: 4px;
    padding: 8px;
    max-height: 320px;
    overflow-y: auto;
    background: var(--ds-surface, #fff);
    margin-bottom: 8px;
  }
  .rev-actions {
    display: flex;
    justify-content: flex-end;
  }
  .restore-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 6px 10px;
    border-radius: 4px;
    border: 1px solid var(--ds-border, #d1d5db);
    background: var(--ds-surface, #fff);
    cursor: pointer;
    font: inherit;
  }
  .restore-btn:hover:not(:disabled) { background: var(--ds-surface-hovered, #f3f4f6); }
  .restore-btn:disabled { opacity: 0.6; cursor: progress; }
</style>
