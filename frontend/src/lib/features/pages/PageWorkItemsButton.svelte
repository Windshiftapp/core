<script>
  import { createPopover, melt } from '@melt-ui/svelte';
  import { Link2, Trash2, Plus, X } from '@lucide/svelte';
  import ItemTypeIcon from '../../components/ItemTypeIcon.svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import StatusBadge from '../../components/StatusBadge.svelte';
  import Input from '../../components/Input.svelte';
  import { onDestroy } from 'svelte';

  /**
   * Top-right popover on a page detail showing the work items linked to
   * this page. Read + unlink + add via inline work-item search. The
   * parent owns the pageLinks array and re-fetches on success — the
   * button itself just emits callbacks.
   */

  let {
    workspaceId,
    pageId,
    pageLinks = [],
    loading = false,
    pageLinkTypeId = null,
    onlinkCreated = () => {},
    onlinkRemoved = () => {},
  } = $props();

  const {
    elements: { trigger, content },
    states: { open },
  } = createPopover({
    forceVisible: true,
    positioning: { strategy: 'fixed', placement: 'bottom-end', fitViewport: true },
    portal: 'body',
  });

  let count = $derived(pageLinks.length);
  let mode = $state('list'); // 'list' or 'add'
  let searchQuery = $state('');
  let searchResults = $state([]);
  let searching = $state(false);
  let highlightedIndex = $state(-1);
  let submitting = $state(false);
  let searchTimer;
  let searchVersion = 0;

  $effect(() => {
    if (!$open) {
      resetSearch();
    }
  });

  onDestroy(() => {
    clearTimeout(searchTimer);
    searchVersion += 1;
  });

  function resetSearch(nextMode = 'list') {
    clearTimeout(searchTimer);
    searchVersion += 1;
    mode = nextMode;
    searchQuery = '';
    searchResults = [];
    highlightedIndex = -1;
    searching = false;
  }

  function handleSearchInput(event) {
    const q = event.currentTarget.value;
    searchQuery = q;
    clearTimeout(searchTimer);
    const version = ++searchVersion;

    if (q.trim().length < 2) {
      searchResults = [];
      highlightedIndex = -1;
      searching = false;
      return;
    }

    searching = true;
    searchTimer = setTimeout(() => searchItems(q.trim(), version), 250);
  }

  async function searchItems(q, version) {
    try {
      const results = await api.links.search(q, 'item', 10);
      if (version !== searchVersion) return;
      searchResults = Array.isArray(results) ? results : [];
      highlightedIndex = searchResults.length > 0 ? 0 : -1;
    } catch (err) {
      if (version !== searchVersion) return;
      console.error('work-item search failed', err);
      searchResults = [];
    } finally {
      if (version === searchVersion) searching = false;
    }
  }

  async function linkItem(item) {
    if (!pageLinkTypeId || submitting) return;
    submitting = true;
    try {
      const link = await api.links.create({
        link_type_id: pageLinkTypeId,
        source_type: 'item',
        source_id: item.id,
        target_type: 'page',
        target_id: pageId,
      });
      onlinkCreated(link);
      resetSearch();
    } catch (err) {
      errorToast(err?.message || t('pages.workItemsErrorLink'));
    } finally {
      submitting = false;
    }
  }

  async function unlinkItem(linkId) {
    try {
      await api.links.delete(linkId);
      onlinkRemoved(linkId);
    } catch (err) {
      errorToast(err?.message || t('pages.workItemsErrorUnlink'));
    }
  }

  function handleSearchKeyDown(e) {
    if (e.key === 'Escape') {
      e.preventDefault();
      resetSearch();
      return;
    }
    if (searchResults.length === 0) return;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      highlightedIndex = (highlightedIndex + 1) % searchResults.length;
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      highlightedIndex = highlightedIndex <= 0 ? searchResults.length - 1 : highlightedIndex - 1;
    } else if (e.key === 'Enter' && highlightedIndex >= 0) {
      e.preventDefault();
      e.stopPropagation();
      linkItem(searchResults[highlightedIndex]);
    }
  }
</script>

<button
  use:melt={$trigger}
  type="button"
  class="trigger"
  data-testid="page-work-items-trigger"
  aria-label={t('pages.workItemsAria')}
>
  <Link2 size={14} />
  <span class="trigger-label">{t('pages.workItemsButton')}</span>
  {#if count > 0}
    <span class="badge" data-testid="page-work-items-count">{count}</span>
  {/if}
</button>

{#if $open}
  <div use:melt={$content} class="popover" data-testid="page-work-items-popover">
    <header class="popover-header">
      <span class="title">{t('pages.workItemsTitle')}</span>
      {#if pageLinkTypeId != null && mode === 'list'}
        <button
          type="button"
          class="add-btn"
          onclick={() => resetSearch('add')}
          data-testid="page-work-items-add"
        >
          <Plus size={14} />
          {t('pages.addWorkItem')}
        </button>
      {/if}
      {#if mode === 'add'}
        <button
          type="button"
          class="add-btn"
          onclick={() => resetSearch()}
          data-testid="page-work-items-add-cancel"
        >
          <X size={14} />
          {t('pages.addWorkItemCancel')}
        </button>
      {/if}
    </header>

    {#if mode === 'add'}
      <div class="search-row">
        <Input
          type="text"
          value={searchQuery}
          oninput={handleSearchInput}
          onkeydown={handleSearchKeyDown}
          placeholder={t('pages.addWorkItemSearchPlaceholder')}
          class="search-input"
          dataTestid="page-work-items-add-search"
          size="small"
        />
      </div>
      {#if searching}
        <p class="status">{t('common.loading')}</p>
      {:else if searchResults.length === 0}
        <p class="status">
          {#if searchQuery.trim().length < 2}
            {t('pickers.startTypingToSearch')}
          {:else}
            {t('pickers.noResultsFor', { query: searchQuery })}
          {/if}
        </p>
      {:else}
        <ul class="list" data-testid="page-work-items-search-list">
          {#each searchResults as result, index}
            {@const isHighlighted = highlightedIndex === index}
            <li>
              <button
                type="button"
                class="row"
                class:row--highlighted={isHighlighted}
                onmouseenter={() => (highlightedIndex = index)}
                onclick={() => linkItem(result)}
                data-testid="page-work-items-add-result"
                data-item-id={result.id}
              >
                <ItemTypeIcon icon={result.item_type_icon} color={result.item_type_color} />
                <span class="row-title">{result.title}</span>
                <span class="row-meta">{result.workspace_name || ''}</span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    {:else}
      {#if loading}
        <p class="status">{t('pages.workItemsLoading')}</p>
      {:else if pageLinks.length > 0}
        <ul class="list">
          {#each pageLinks as link}
            {@const isCurrentPage = link.target_type === 'page' && link.target_id === pageId}
            {@const linkedItemId = isCurrentPage ? link.source_id : link.target_id}
            {@const linkedItemTitle = isCurrentPage ? link.source_title : link.target_title}
            {@const linkedItemWorkspaceKey = isCurrentPage ? link.source_workspace_key : link.target_workspace_key}
            {@const linkedItemWorkspaceId = isCurrentPage ? link.source_workspace_id : link.target_workspace_id}
            {@const linkedItemNumber = isCurrentPage ? link.source_item_number : link.target_item_number}
            {@const linkedItemStatus = isCurrentPage ? link.source_status_name : link.target_status_name}
            {@const linkedItemStatusColor = isCurrentPage ? link.source_status_color : link.target_status_color}
            {@const linkedItemIconKey = isCurrentPage ? link.source_item_type_icon : link.target_item_type_icon}
            {@const linkedItemIconColor = isCurrentPage ? link.source_item_type_color : link.target_item_type_color}
            {@const linkedItemKey = `${linkedItemWorkspaceKey || 'WORK'}-${linkedItemNumber ?? linkedItemId}`}
            {@const linkedItemHref = `/workspaces/${linkedItemWorkspaceId || workspaceId}/items/${linkedItemId}`}
            <li
              class="row-li"
              data-testid="page-work-items-row"
              data-link-id={link.id}
              data-item-id={linkedItemId}
            >
              <a class="row row--link" href={linkedItemHref}>
                <ItemTypeIcon icon={linkedItemIconKey} color={linkedItemIconColor} />
                <span class="row-key">{linkedItemKey}</span>
                <span class="row-title">{linkedItemTitle}</span>
                {#if linkedItemStatus}
                  <StatusBadge
                    status={{ label: linkedItemStatus, categoryColor: linkedItemStatusColor }}
                    uppercase={false}
                    showDot={false}
                  />
                {/if}
              </a>
              <button
                type="button"
                class="row-delete"
                aria-label={t('pages.removeWorkItemLink')}
                title={t('pages.removeWorkItemLink')}
                onclick={() => unlinkItem(link.id)}
                data-testid="page-work-items-unlink"
              >
                <Trash2 size={14} />
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    {/if}
  </div>
{/if}

<style>
  .trigger {
    display: inline-flex;
    align-items: center;
    gap: 0.375rem;
    padding: 0.25rem 0.5rem;
    border: 1px solid var(--ds-border);
    background: transparent;
    color: var(--ds-text-subtle);
    border-radius: 0.375rem;
    font-size: 0.75rem;
    cursor: pointer;
    transition: background 120ms, color 120ms;
  }
  .trigger:hover {
    background: var(--ds-surface-hovered);
    color: var(--ds-text);
  }
  .trigger-label {
    font-weight: 500;
  }
  .badge {
    background: var(--ds-background-neutral);
    color: var(--ds-text);
    padding: 0 0.375rem;
    border-radius: 999px;
    font-size: 0.625rem;
    min-width: 1.25rem;
    text-align: center;
    line-height: 1rem;
  }

  .popover {
    z-index: 1000;
    width: 360px;
    max-height: 480px;
    background: var(--ds-surface);
    border: 1px solid var(--ds-border);
    border-radius: 0.5rem;
    box-shadow: 0 8px 24px rgb(0 0 0 / 0.12);
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .popover-header {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.5rem 0.75rem;
    border-bottom: 1px solid var(--ds-border);
    font-size: 0.875rem;
  }
  .title {
    font-weight: 600;
    color: var(--ds-text);
  }
  .add-btn {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.25rem 0.5rem;
    background: transparent;
    border: 1px solid var(--ds-border);
    border-radius: 0.25rem;
    color: var(--ds-text-subtle);
    font-size: 0.75rem;
    cursor: pointer;
  }
  .add-btn:hover {
    color: var(--ds-text);
    background: var(--ds-surface-hovered);
  }

  .search-row {
    flex-shrink: 0;
    padding: 0.5rem;
    border-bottom: 1px solid var(--ds-border);
  }
  .search-input {
    width: 100%;
    padding: 0.375rem 0.5rem;
    border: 1px solid var(--ds-border);
    border-radius: 0.25rem;
    background: var(--ds-surface);
    color: var(--ds-text);
    font-size: 0.875rem;
  }
  .search-input:focus {
    outline: none;
    border-color: var(--ds-accent-blue);
  }

  .status {
    padding: 1rem;
    margin: 0;
    color: var(--ds-text-subtle);
    font-size: 0.8125rem;
    text-align: center;
  }

  .list {
    min-height: 0;
    overscroll-behavior: contain;
    list-style: none;
    padding: 0.25rem;
    margin: 0;
    max-height: 360px;
    overflow-y: auto;
  }
  .row-li {
    display: flex;
    align-items: center;
    gap: 0.25rem;
  }
  .row,
  .row--link {
    flex: 1 1 auto;
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem;
    background: transparent;
    border: none;
    border-radius: 0.25rem;
    color: var(--ds-text);
    text-decoration: none;
    text-align: left;
    font-size: 0.8125rem;
    cursor: pointer;
    min-width: 0;
  }
  .row:hover,
  .row--link:hover,
  .row--highlighted {
    background: var(--ds-surface-hovered);
  }
  .row-key {
    font-family: var(--ds-font-mono, ui-monospace, monospace);
    color: var(--ds-text-subtle);
    font-size: 0.6875rem;
    flex-shrink: 0;
  }
  .row-title {
    flex: 1 1 auto;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .row-meta {
    color: var(--ds-text-subtle);
    font-size: 0.6875rem;
    flex-shrink: 0;
  }
  .row :global(.inline-flex) {
    flex-shrink: 0;
  }
  .row-delete {
    padding: 0.375rem;
    background: transparent;
    border: none;
    color: var(--ds-text-subtle);
    border-radius: 0.25rem;
    cursor: pointer;
  }
  .row-delete:hover {
    color: var(--ds-text-danger);
    background: var(--ds-surface-hovered);
  }
</style>
