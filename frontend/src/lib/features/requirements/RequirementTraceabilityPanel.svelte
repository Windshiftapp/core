<script>
  import { onDestroy, tick, untrack } from 'svelte';
  import { Link2, Plus, Trash2, FileText, CheckSquare, Package } from '@lucide/svelte';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import ItemTypeIcon from '../../components/ItemTypeIcon.svelte';
  import StatusBadge from '../../components/StatusBadge.svelte';
  import DropdownMenu from '../../layout/DropdownMenu.svelte';
  import { buildRequirementKeyByPageId, pageHref } from './requirementKeyMap.js';
  import { linkDirectionLabel } from './requirementLinkLabels.js';
  import {
    mergePageLinks,
    resolveLinkTypeIds,
    linkedEntity,
    filterItemLinks,
    filterTestLinks,
    filterSpecifiesOutgoing,
    filterSpecifiedBy,
    filterGenericPageLinks,
    filterAssetLinks,
    buildItemLinkCreate,
    buildTestLinkCreate,
    buildSpecifiesLinkCreate,
    buildGenericPageLinkCreate,
    buildAssetLinkCreate,
  } from './requirementTraceabilityModel.js';

  let { workspaceId, pageId, canEdit = false } = $props();

  let loading = $state(true);
  let loadError = $state(false);
  let linkTypesError = $state(false);
  let pageLinks = $state([]);
  let linkTypes = $state([]);
  let requirementKeyByPageId = $state(new Map());
  let activeAction = $state(null);
  let searchQuery = $state('');
  let searchResults = $state([]);
  let searching = $state(false);
  let searchError = $state(false);
  let submitting = $state(false);
  let removingLinkIds = $state(new Set());
  let panelElement = $state(null);
  let searchInput = $state(null);
  let resultsElement = $state(null);
  let searchTimer;
  let searchVersion = 0;
  let loadVersion = 0;
  let keyVersion = 0;
  const searchId = $props.id();

  const linkTypeIds = $derived(
    /** @type {Record<string, number | null>} */ (resolveLinkTypeIds(linkTypes))
  );
  const itemLinks = $derived(filterItemLinks(pageLinks, pageId));
  const testLinks = $derived(filterTestLinks(pageLinks, pageId));
  const groups = $derived([
    {
      id: 'items',
      title: t('requirements.traceability.implementsTitle'),
      links: itemLinks,
    },
    {
      id: 'tests',
      title: t('requirements.traceability.linkedTests'),
      links: testLinks,
    },
    {
      id: 'specifies',
      title: t('requirements.traceability.specifiesTitle'),
      links: filterSpecifiesOutgoing(pageLinks, pageId, linkTypeIds.specifiesLinkTypeId),
    },
    {
      id: 'specifiedBy',
      title: t('requirements.traceability.specifiedByTitle'),
      links: filterSpecifiedBy(pageLinks, pageId, linkTypeIds.specifiesLinkTypeId),
    },
    {
      id: 'pages',
      title: t('requirements.traceability.relatedGenericTitle'),
      links: filterGenericPageLinks(pageLinks, pageId, linkTypeIds.relatesToLinkTypeId),
    },
    {
      id: 'assets',
      title: t('requirements.traceability.linkedAssets'),
      links: filterAssetLinks(pageLinks, pageId),
    },
  ].filter((group) => group.links.length > 0));
  const actions = $derived([
    {
      id: 'items',
      label: t('requirements.traceability.implementsAdd'),
      placeholder: t('requirements.traceability.searchItems'),
      error: t('requirements.traceability.linkItemError'),
      linkTypeId: linkTypeIds.itemLinkTypeId,
      entityType: 'item',
      build: buildItemLinkCreate,
    },
    {
      id: 'tests',
      label: t('requirements.traceability.addTest'),
      placeholder: t('requirements.traceability.searchTests'),
      error: t('requirements.traceability.linkTestError'),
      linkTypeId: linkTypeIds.testsLinkTypeId,
      entityType: 'test_case',
      build: buildTestLinkCreate,
    },
    {
      id: 'specifies',
      label: t('requirements.traceability.specifiesAdd'),
      placeholder: t('requirements.traceability.searchPages'),
      error: t('requirements.traceability.linkPageError'),
      linkTypeId: linkTypeIds.specifiesLinkTypeId,
      entityType: 'page',
      build: buildSpecifiesLinkCreate,
    },
    {
      id: 'pages',
      label: t('requirements.traceability.addPage'),
      placeholder: t('requirements.traceability.searchPages'),
      error: t('requirements.traceability.linkPageError'),
      linkTypeId: linkTypeIds.relatesToLinkTypeId,
      entityType: 'page',
      build: buildGenericPageLinkCreate,
    },
    {
      id: 'assets',
      label: t('requirements.traceability.addAsset'),
      placeholder: t('requirements.traceability.searchAssets'),
      error: t('requirements.traceability.linkAssetError'),
      linkTypeId: linkTypeIds.relatesToLinkTypeId,
      entityType: 'asset',
      build: buildAssetLinkCreate,
    },
  ].filter((action) => action.linkTypeId));
  const selectedAction = $derived(actions.find((action) => action.id === activeAction));
  const addMenuItems = $derived(actions.map((action) => ({
    id: action.id,
    title: action.label,
    onClick: () => openSearch(action.id),
  })));

  $effect(() => {
    const id = pageId;
    untrack(() => {
      clearSearch();
      activeAction = null;
      submitting = false;
      removingLinkIds = new Set();
      pageLinks = [];
      if (id) void loadPageLinks(id, true);
    });
  });

  $effect(() => {
    const id = workspaceId;
    void loadRequirementKeys(id);
  });

  $effect(() => {
    void loadLinkTypes();
  });

  onDestroy(() => {
    clearTimeout(searchTimer);
    searchVersion += 1;
    loadVersion += 1;
    keyVersion += 1;
  });

  async function loadPageLinks(id = pageId, initial = false) {
    const version = ++loadVersion;
    if (initial) loading = true;
    loadError = false;
    try {
      const response = await api.links.getForPage(id);
      if (version === loadVersion && id === pageId) pageLinks = mergePageLinks(response);
    } catch {
      if (version === loadVersion && id === pageId) loadError = true;
    } finally {
      if (version === loadVersion && id === pageId) loading = false;
    }
  }

  async function loadLinkTypes() {
    linkTypesError = false;
    try {
      const response = await api.linkTypes.getAll();
      linkTypes = Array.isArray(response) ? response : (response?.data ?? []);
    } catch {
      linkTypesError = true;
    }
  }

  async function loadRequirementKeys(id) {
    const version = ++keyVersion;
    const keys = await buildRequirementKeyByPageId(id);
    if (version === keyVersion) requirementKeyByPageId = keys;
  }

  function clearSearch() {
    clearTimeout(searchTimer);
    searchVersion += 1;
    searchQuery = '';
    searchResults = [];
    searching = false;
    searchError = false;
  }

  async function openSearch(actionId) {
    clearSearch();
    activeAction = actionId;
    await tick();
    searchInput?.focus();
  }

  function closeSearch() {
    if (submitting) return;
    clearSearch();
    activeAction = null;
    panelElement?.querySelector('[data-testid="requirement-add-link"]')?.focus();
  }

  function handleSearchInput(event) {
    searchQuery = event.currentTarget.value;
    clearTimeout(searchTimer);
    const version = ++searchVersion;
    const query = searchQuery.trim();
    searchResults = [];
    searchError = false;
    searching = query.length >= 2;
    if (searching) searchTimer = setTimeout(() => searchTargets(query, version), 250);
  }

  async function searchTargets(query, version = ++searchVersion) {
    const action = selectedAction;
    if (!action) return;
    searching = true;
    searchError = false;
    try {
      const response = action.entityType === 'page'
        ? await api.pages.searchPages(workspaceId, query, { limit: 10 })
        : action.entityType === 'test_case'
          ? await api.tests.testCases.getAll(workspaceId, { all: true, q: query, limit: 10 })
          : await api.links.search(query, action.entityType, 10);
      if (version !== searchVersion) return;
      const rows = Array.isArray(response) ? response : (response?.results ?? response?.data ?? []);
      searchResults = rows.filter((row) => !(action.entityType === 'page' && row.id === pageId));
    } catch {
      if (version === searchVersion) searchError = true;
    } finally {
      if (version === searchVersion) searching = false;
    }
  }

  function handleSearchKeydown(event) {
    if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      closeSearch();
      return;
    }
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp' && event.key !== 'Enter') return;
    const buttons = [...(resultsElement?.querySelectorAll('button:not(:disabled)') || [])];
    if (buttons.length === 0) return;
    const index = buttons.indexOf(event.target);
    if (event.key === 'Enter') {
      if (event.target === searchInput) {
        event.preventDefault();
        buttons[0].click();
      }
      return;
    }
    event.preventDefault();
    if (event.key === 'ArrowUp' && index <= 0) searchInput?.focus();
    else buttons[Math.min(buttons.length - 1, index + (event.key === 'ArrowDown' ? 1 : -1))]?.focus();
  }

  async function linkTarget(target) {
    const action = selectedAction;
    if (!canEdit || !action || submitting || (action.entityType === 'page' && target.id === pageId)) return;
    const id = pageId;
    submitting = true;
    try {
      await api.links.create(action.build(id, target, action.linkTypeId));
      if (id !== pageId) return;
      clearSearch();
      activeAction = null;
      await loadPageLinks(id);
      panelElement?.querySelector('[data-testid="requirement-add-link"]')?.focus();
    } catch (error) {
      errorToast(error?.message || action.error);
    } finally {
      if (id === pageId) submitting = false;
    }
  }

  async function unlink(linkId) {
    if (!canEdit || removingLinkIds.has(linkId)) return;
    const id = pageId;
    removingLinkIds = new Set([...removingLinkIds, linkId]);
    try {
      await api.links.delete(linkId);
      if (id === pageId) pageLinks = pageLinks.filter((link) => link.id !== linkId);
    } catch (error) {
      errorToast(error?.message || t('requirements.traceability.unlinkError'));
    } finally {
      if (id === pageId) removingLinkIds = new Set([...removingLinkIds].filter((value) => value !== linkId));
    }
  }

  function entityHref(entity) {
    const linkedWorkspaceId = entity.workspaceId || workspaceId;
    if (entity.type === 'item') return `/workspaces/${linkedWorkspaceId}/items/${entity.id}`;
    if (entity.type === 'test_case') return `/workspaces/${linkedWorkspaceId}/tests/cases/${entity.id}`;
    if (entity.type === 'asset') return `/assets/${entity.id}`;
    return pageHref(linkedWorkspaceId, entity.id, requirementKeyByPageId.get(entity.id));
  }

  function entityKey(entity) {
    if (entity.type === 'item') return `${entity.workspaceKey || 'WORK'}-${entity.itemNumber ?? entity.id}`;
    if (entity.type === 'test_case') return `TC-${entity.itemNumber ?? entity.id}`;
    return entity.type === 'page' ? requirementKeyByPageId.get(entity.id)?.key : '';
  }
</script>

<section bind:this={panelElement} class="traceability" aria-label={t('requirements.traceability.title')}>
  <header class="traceability__header">
    <div class="traceability__heading">
      <h3><Link2 size={16} aria-hidden="true" />{t('requirements.traceability.title')}</h3>
      {#if !loading && !loadError}
        <span class="traceability__summary">{t('requirements.traceability.summaryStats', { items: itemLinks.length, tests: testLinks.length })}</span>
      {/if}
    </div>
    {#if canEdit && actions.length > 0}
      <DropdownMenu
        triggerText={t('items.addLink')}
        triggerIcon={Plus}
        items={addMenuItems}
        disabled={submitting}
        showChevron={false}
        placement="bottom-end"
        triggerClass="traceability-add"
        triggerTestid="requirement-add-link"
      />
    {/if}
  </header>

  {#if loading}
    <p class="traceability__message" role="status">{t('common.loading')}</p>
  {/if}
  {#if loadError || (canEdit && linkTypesError)}
    <div class="traceability__error" role="alert">
      <span>{t('requirements.mobile.traceabilityLoadError')}</span>
      <Button variant="ghost" size="sm" onclick={() => { if (loadError) void loadPageLinks(); if (linkTypesError) void loadLinkTypes(); }}>{t('common.retry')}</Button>
    </div>
  {/if}

  {#if canEdit && selectedAction}
    <div class="traceability-search">
      <label for={searchId} class="traceability-search__label">{selectedAction.label}</label>
      <div class="traceability-search__controls">
        <Input
          id={searchId}
          bind:inputRef={searchInput}
          value={searchQuery}
          oninput={handleSearchInput}
          onkeydown={handleSearchKeydown}
          placeholder={selectedAction.placeholder}
          disabled={submitting}
          size="small"
          autocomplete="off"
        />
        <Button variant="ghost" size="sm" disabled={submitting} onclick={closeSearch}>{t('common.cancel')}</Button>
      </div>
      {#if searching || submitting}
        <p class="traceability__message" role="status">{t('common.loading')}</p>
      {:else if searchError}
        <div class="traceability__error" role="alert">
          <span>{t('requirements.traceability.searchError')}</span>
          <Button variant="ghost" size="sm" onclick={() => searchTargets(searchQuery.trim())}>{t('common.retry')}</Button>
        </div>
      {:else if searchQuery.trim().length < 2}
        <p class="traceability__message">{t('pickers.startTypingToSearch')}</p>
      {:else if searchResults.length === 0}
        <p class="traceability__message" role="status">{t('pickers.noResultsFor', { query: searchQuery })}</p>
      {:else}
        <ul bind:this={resultsElement} class="traceability-search__results" aria-label={selectedAction.placeholder}>
          {#each searchResults as result (result.id)}
            <li>
              <button type="button" class="traceability-result" onclick={() => linkTarget(result)} onkeydown={handleSearchKeydown}>
                {#if selectedAction.entityType === 'item'}
                  <ItemTypeIcon icon={result.item_type_icon} color={result.item_type_color} />
                {:else if selectedAction.entityType === 'test_case'}
                  <CheckSquare size={16} aria-hidden="true" />
                {:else if selectedAction.entityType === 'asset'}
                  <Package size={16} aria-hidden="true" />
                {:else}
                  <FileText size={16} aria-hidden="true" />
                {/if}
                <span>{result.title}</span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}

  {#if !loading && !loadError && groups.length === 0 && !selectedAction}
    <p class="traceability__message traceability__empty">{t('requirements.traceability.empty')}</p>
  {/if}

  {#each groups as group (group.id)}
    <div class="traceability-group" data-section={group.id}>
      <h4>{group.title}<span>{group.links.length}</span></h4>
      <ul class="traceability-list">
        {#each group.links as link (link.id)}
          {@const entity = linkedEntity(link, pageId)}
          {#if entity}
            {@const key = entityKey(entity)}
            {@const relation = linkDirectionLabel(link, pageId, linkTypes)}
            <li class="traceability-row" data-link-id={link.id}>
              <a class="traceability-row__link" href={entityHref(entity)}>
                {#if entity.type === 'item'}
                  <ItemTypeIcon icon={entity.itemTypeIcon} color={entity.itemTypeColor} />
                {:else if entity.type === 'test_case'}
                  <CheckSquare size={16} aria-hidden="true" />
                {:else if entity.type === 'asset'}
                  <Package size={16} aria-hidden="true" />
                {:else}
                  <FileText size={16} aria-hidden="true" />
                {/if}
                {#if key}<span class="traceability-row__key">{key}</span>{/if}
                <span class="traceability-row__title">{entity.title}</span>
              </a>
              {#if relation || entity.statusName || canEdit}
                <div class="traceability-row__details">
                  {#if relation}<span class="traceability-row__relation">{relation}</span>{/if}
                  {#if entity.statusName}
                    <StatusBadge status={{ label: entity.statusName, categoryColor: entity.statusColor }} uppercase={false} showDot={false} />
                  {/if}
                  {#if canEdit}
                    <button
                      type="button"
                      class="traceability-unlink"
                      disabled={removingLinkIds.has(link.id)}
                      onclick={() => unlink(link.id)}
                      aria-label={`${t('requirements.traceability.unlink')}: ${entity.title}`}
                      title={t('requirements.traceability.unlink')}
                    >
                      <Trash2 size={14} aria-hidden="true" />
                    </button>
                  {/if}
                </div>
              {/if}
            </li>
          {/if}
        {/each}
      </ul>
    </div>
  {/each}
</section>

<style>
  .traceability {
    padding: 1.5rem 0;
    border-top: 1px solid var(--ds-border);
    color: var(--ds-text);
  }
  .traceability__header,
  .traceability__heading,
  .traceability__header h3,
  .traceability__error,
  .traceability-search__controls,
  .traceability-result,
  .traceability-row,
  .traceability-row__link,
  .traceability-row__details {
    display: flex;
    align-items: center;
    gap: 0.625rem;
    min-width: 0;
  }
  .traceability__header {
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 0.5rem 1rem;
  }
  .traceability__heading {
    flex-wrap: wrap;
    gap: 0.375rem 1rem;
  }
  .traceability__header h3 {
    margin: 0;
    font-size: 0.875rem;
    font-weight: 600;
  }
  .traceability__header h3 :global(svg),
  .traceability-row__link > :global(svg),
  .traceability-result > :global(svg) {
    flex-shrink: 0;
    color: var(--ds-text-subtle);
  }
  .traceability__summary {
    color: var(--ds-text-subtle);
    font-size: 0.75rem;
    font-variant-numeric: tabular-nums;
  }
  .traceability :global(.traceability-add) {
    min-height: 2rem;
    padding: 0.25rem 0.5rem;
    color: var(--ds-text-subtle);
    font-size: 0.8125rem;
  }
  .traceability :global(.traceability-add:hover) {
    background: var(--ds-background-neutral-hovered);
    color: var(--ds-text);
  }
  .traceability__message {
    margin: 0.625rem 0 0;
    color: var(--ds-text-subtle);
    font-size: 0.8125rem;
    line-height: 1.6;
  }
  .traceability__empty {
    margin-top: 0.875rem;
  }
  .traceability__error {
    flex-wrap: wrap;
    margin-top: 0.625rem;
    color: var(--ds-text-danger);
    font-size: 0.8125rem;
  }
  .traceability-search {
    margin-top: 1rem;
    padding: 1rem;
    border: 1px solid var(--ds-border);
    border-radius: 0.375rem;
  }
  .traceability-search__label {
    display: block;
    margin-bottom: 0.5rem;
    font-size: 0.8125rem;
    font-weight: 500;
  }
  .traceability-search__controls > :global(input) {
    flex: 1;
    min-width: 0;
  }
  .traceability-search__results,
  .traceability-list {
    list-style: none;
    margin: 0.625rem 0 0;
    padding: 0;
  }
  .traceability-search__results {
    max-height: 16rem;
    overflow-y: auto;
    scrollbar-color: var(--ds-border) transparent;
  }
  .traceability-result {
    width: 100%;
    min-height: 2.5rem;
    padding: 0.625rem 0.5rem;
    border: none;
    border-radius: 0.25rem;
    background: transparent;
    color: var(--ds-text);
    text-align: left;
    font-size: 0.875rem;
    cursor: pointer;
  }
  .traceability-result span {
    overflow-wrap: anywhere;
  }
  .traceability-result:hover,
  .traceability-result:focus-visible {
    background: var(--ds-background-neutral-hovered);
  }
  .traceability-group {
    margin-top: 1.5rem;
  }
  .traceability-group h4 {
    display: flex;
    gap: 0.5rem;
    margin: 0;
    color: var(--ds-text-subtle);
    font-size: 0.75rem;
    font-weight: 500;
  }
  .traceability-group h4 span {
    font-weight: 400;
    font-variant-numeric: tabular-nums;
  }
  .traceability-list {
    display: grid;
    gap: 0.5rem;
  }
  .traceability-row {
    min-height: 3rem;
    padding: 0.5rem 0.75rem;
    border: 1px solid var(--ds-border);
    border-radius: 0.5rem;
    background: var(--ds-surface-raised);
  }
  .traceability-row__link {
    flex: 1;
    gap: 0.625rem;
    min-height: 2rem;
    color: var(--ds-text);
    text-decoration: none;
    text-underline-offset: 0.2em;
  }
  .traceability-row__link:hover .traceability-row__title {
    color: var(--ds-text-link);
    text-decoration: underline;
  }
  .traceability-row__key {
    flex-shrink: 0;
    color: var(--ds-text-subtle);
    font-size: 0.75rem;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }
  .traceability-row__title {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.875rem;
  }
  .traceability-row__details {
    flex-shrink: 0;
    gap: 0.5rem;
  }
  .traceability-row__relation {
    padding: 0.125rem 0.5rem;
    border: 1px solid var(--ds-border);
    border-radius: 999px;
    color: var(--ds-text-subtle);
    font-size: 0.6875rem;
    line-height: 1.4;
  }
  .traceability-unlink {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: 2rem;
    height: 2rem;
    border: none;
    border-radius: 0.25rem;
    background: transparent;
    color: var(--ds-text-subtle);
    cursor: pointer;
  }
  .traceability-unlink:hover {
    color: var(--ds-text-danger);
    background: var(--ds-background-danger-subtle);
  }
  .traceability-unlink:disabled {
    opacity: 0.5;
    cursor: wait;
  }
  .traceability-unlink:focus-visible,
  .traceability-row__link:focus-visible,
  .traceability-result:focus-visible {
    outline: 2px solid var(--ds-border-focused);
    outline-offset: 2px;
  }
  @media (hover: hover) and (pointer: fine) {
    .traceability-unlink {
      opacity: 0;
    }
    .traceability-row:hover .traceability-unlink,
    .traceability-row:focus-within .traceability-unlink {
      opacity: 1;
    }
  }
  @container (max-width: 700px) {
    .traceability-row {
      flex-wrap: wrap;
    }
    .traceability-row__link {
      flex-basis: 100%;
    }
    .traceability-row__title {
      white-space: normal;
      overflow-wrap: anywhere;
    }
    .traceability-row__details {
      flex: 1;
      flex-wrap: wrap;
      padding-left: 1.625rem;
    }
    .traceability-unlink {
      margin-left: auto;
    }
  }
</style>
