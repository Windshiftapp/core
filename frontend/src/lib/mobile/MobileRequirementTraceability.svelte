<script>
  import { ChevronDown, ChevronRight, Link2, Loader, Plus, Trash2 } from '@lucide/svelte';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { t } from '../stores/i18n.svelte.js';
  import { errorToast } from '../stores/toasts.svelte.js';
  import Lozenge from '../components/Lozenge.svelte';
  import ItemTypeIcon from '../components/ItemTypeIcon.svelte';
  import { buildRequirementKeyByPageId, traceabilityEntityHref } from '../features/requirements/requirementKeyMap.js';
  import { linkDirectionLabel } from '../features/requirements/requirementLinkLabels.js';
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
  } from '../features/requirements/requirementTraceabilityModel.js';
  import MobileTraceabilityAddSheet from './MobileTraceabilityAddSheet.svelte';
  import MobileConfirmSheet from './MobileConfirmSheet.svelte';

  let { workspaceId, pageId, canEdit = false } = $props();

  let loading = $state(true);
  let loadError = $state('');
  let pageLinks = $state([]);
  let linkTypesCache = $state([]);
  let requirementKeyByPageId = $state(new Map());
  let loadRequestSeq = 0;

  let addKind = $state(null);
  let addSheetOpen = $state(false);
  let unlinkTarget = $state(null);
  let unlinkSheetOpen = $state(false);
  let unlinkBusy = $state(false);

  let openSections = $state({
    items: true,
    tests: true,
    specifies: false,
    specifiedBy: false,
    relatedPages: false,
    assets: false,
  });

  const linkTypeIds = $derived(resolveLinkTypeIds(linkTypesCache));
  const itemLinks = $derived(filterItemLinks(pageLinks, pageId));
  const testLinks = $derived(filterTestLinks(pageLinks, pageId));
  const specifiesOutgoingLinks = $derived(
    filterSpecifiesOutgoing(pageLinks, pageId, linkTypeIds.specifiesLinkTypeId),
  );
  const specifiedByLinks = $derived(
    filterSpecifiedBy(pageLinks, pageId, linkTypeIds.specifiesLinkTypeId),
  );
  const genericRelatedPageLinks = $derived(
    filterGenericPageLinks(pageLinks, pageId, linkTypeIds.relatesToLinkTypeId),
  );
  const assetLinks = $derived(filterAssetLinks(pageLinks, pageId));
  const isTestCovered = $derived(testLinks.length > 0);

  const addSheetTitle = $derived.by(() => {
    switch (addKind) {
      case 'item':
        return t('requirements.traceability.implementsAdd');
      case 'test':
        return t('requirements.traceability.addTest');
      case 'specifies':
        return t('requirements.traceability.specifiesAdd');
      case 'genericPage':
        return t('requirements.traceability.addPage');
      case 'asset':
        return t('requirements.traceability.addAsset');
      default:
        return '';
    }
  });

  const addSheetPlaceholder = $derived.by(() => {
    switch (addKind) {
      case 'item':
        return t('requirements.traceability.searchItems');
      case 'test':
        return t('requirements.traceability.searchTests');
      case 'specifies':
      case 'genericPage':
        return t('requirements.traceability.searchPages');
      case 'asset':
        return t('requirements.traceability.searchAssets');
      default:
        return '';
    }
  });

  $effect(() => {
    if (!pageId) return;
    addKind = null;
    addSheetOpen = false;
    unlinkTarget = null;
    unlinkSheetOpen = false;
    void ensureLinkTypesLoaded();
    void loadPageLinks();
  });

  $effect(() => {
    if (!workspaceId) return;
    void loadRequirementKeys();
  });

  async function loadRequirementKeys() {
    requirementKeyByPageId = await buildRequirementKeyByPageId(workspaceId);
  }

  async function ensureLinkTypesLoaded() {
    if (linkTypesCache.length > 0) return;
    try {
      const resp = await api.linkTypes.getAll();
      linkTypesCache = Array.isArray(resp) ? resp : (resp?.data ?? []);
    } catch (err) {
      console.error('failed to load link types', err);
    }
  }

  async function loadPageLinks() {
    if (!pageId) return;
    const requestSeq = ++loadRequestSeq;
    loading = true;
    loadError = '';
    try {
      const resp = await api.links.getForPage(pageId);
      if (requestSeq !== loadRequestSeq) return;
      pageLinks = mergePageLinks(resp);
    } catch (err) {
      if (requestSeq !== loadRequestSeq) return;
      console.error('failed to load page links', err);
      pageLinks = [];
      loadError = err?.message || t('requirements.mobile.traceabilityLoadError');
    } finally {
      if (requestSeq === loadRequestSeq) loading = false;
    }
  }

  function toggleSection(key) {
    openSections = { ...openSections, [key]: !openSections[key] };
  }

  function openAdd(kind) {
    addKind = kind;
    addSheetOpen = true;
  }

  async function mergeCreatedLink(link) {
    if (link && !pageLinks.some((l) => l.id === link.id)) {
      pageLinks = [link, ...pageLinks];
    } else {
      await loadPageLinks();
    }
  }

  async function handleAddPick(option) {
    if (!addKind) return;
    try {
      let payload = null;
      switch (addKind) {
        case 'item':
          if (!linkTypeIds.itemLinkTypeId) return;
          payload = buildItemLinkCreate(pageId, option, linkTypeIds.itemLinkTypeId);
          break;
        case 'test':
          if (!linkTypeIds.testsLinkTypeId) return;
          payload = buildTestLinkCreate(pageId, option, linkTypeIds.testsLinkTypeId);
          break;
        case 'specifies':
          if (!linkTypeIds.specifiesLinkTypeId || option.id === pageId) return;
          payload = buildSpecifiesLinkCreate(pageId, option, linkTypeIds.specifiesLinkTypeId);
          break;
        case 'genericPage':
          if (!linkTypeIds.relatesToLinkTypeId || option.id === pageId) return;
          payload = buildGenericPageLinkCreate(pageId, option, linkTypeIds.relatesToLinkTypeId);
          break;
        case 'asset':
          if (!linkTypeIds.relatesToLinkTypeId) return;
          payload = buildAssetLinkCreate(pageId, option, linkTypeIds.relatesToLinkTypeId);
          break;
        default:
          return;
      }
      const link = await api.links.create(payload);
      await mergeCreatedLink(link);
    } catch (err) {
      const message =
        addKind === 'item'
          ? t('requirements.traceability.linkItemError')
          : addKind === 'test'
            ? t('requirements.traceability.linkTestError')
            : addKind === 'asset'
              ? t('requirements.traceability.linkAssetError')
              : t('requirements.traceability.linkPageError');
      errorToast(err?.message || message);
    }
  }

  async function runAddSearch(q) {
    switch (addKind) {
      case 'item':
        return api.links.search(q, 'item', 10);
      case 'test': {
        const results = await api.tests.testCases.getAll(workspaceId, { q, limit: 10 });
        return Array.isArray(results) ? results : (results?.data ?? []);
      }
      case 'specifies':
      case 'genericPage': {
        const results = await api.pages.searchPages(workspaceId, q, { limit: 10 });
        const rows = Array.isArray(results) ? results : (results?.data ?? []);
        return rows.filter((page) => page?.id !== pageId);
      }
      case 'asset':
        return api.links.search(q, 'asset', 10);
      default:
        return [];
    }
  }

  function addPickLabel(option) {
    if (addKind === 'item') {
      const key = option.workspace_key && option.item_number != null
        ? `${option.workspace_key}-${option.item_number}`
        : '';
      return key ? `${key} · ${option.title}` : option.title;
    }
    if (addKind === 'specifies' || addKind === 'genericPage') {
      const meta = requirementKeyByPageId.get(option.id);
      return meta?.key ? `${meta.key} · ${option.title}` : option.title;
    }
    return option.title ?? String(option.id ?? '');
  }

  function requestUnlink(linkId) {
    unlinkTarget = linkId;
    unlinkSheetOpen = true;
  }

  async function confirmUnlink() {
    if (unlinkTarget == null) return;
    unlinkBusy = true;
    try {
      await api.links.delete(unlinkTarget);
      pageLinks = pageLinks.filter((link) => link.id !== unlinkTarget);
      unlinkTarget = null;
      unlinkSheetOpen = false;
    } catch (err) {
      errorToast(err?.message || t('requirements.traceability.unlinkError'));
    } finally {
      unlinkBusy = false;
    }
  }

  function navigateToEntity(entity) {
    const pageMeta = entity.type === 'page' ? requirementKeyByPageId.get(entity.id) : null;
    const href = traceabilityEntityHref(entity, workspaceId, pageMeta);
    if (href) navigate(href);
  }
</script>

<section class="traceability" data-testid="mobile-requirement-traceability">
  <header class="trace-header">
    <Link2 size={16} aria-hidden="true" />
    <h2 class="section-title">{t('requirements.mobile.traceabilitySummary')}</h2>
  </header>

  <div class="summary" data-testid="mobile-requirement-traceability-summary">
    <span>{t('requirements.traceability.linkedItems')}: {itemLinks.length}</span>
    <span>{t('requirements.traceability.linkedTests')}: {testLinks.length}</span>
    {#if isTestCovered}
      <Lozenge color="green">{t('requirements.traceability.covered')}</Lozenge>
    {:else}
      <Lozenge color="red">{t('requirements.traceability.uncovered')}</Lozenge>
    {/if}
  </div>

  {#if loading}
    <div class="center" data-testid="mobile-requirement-traceability-loading">
      <Loader class="spin" size={20} />
    </div>
  {:else if loadError}
    <p class="error" data-testid="mobile-requirement-traceability-error">{loadError}</p>
  {:else}
    <!-- Items -->
    <div class="section-card" data-testid="mobile-traceability-items">
      <div class="section-head">
        <button class="section-toggle" type="button" onclick={() => toggleSection('items')}>
          {#if openSections.items}<ChevronDown size={16} />{:else}<ChevronRight size={16} />{/if}
          <span>{t('requirements.traceability.implementsTitle')}</span>
          <span class="count">{itemLinks.length}</span>
        </button>
        {#if canEdit && linkTypeIds.itemLinkTypeId}
          <button class="add-btn" type="button" onclick={() => openAdd('item')} aria-label={t('requirements.traceability.implementsAdd')}>
            <Plus size={16} />
          </button>
        {/if}
      </div>
      {#if openSections.items}
        {#if itemLinks.length === 0}
          <p class="empty">{t('requirements.traceability.implementsEmpty')}</p>
        {:else}
          <ul class="link-list">
            {#each itemLinks as link (link.id)}
              {@const entity = linkedEntity(link, pageId)}
              {#if entity}
                <li class="link-row">
                  {#if traceabilityEntityHref(entity, workspaceId, null)}
                    <button class="link-main" type="button" onclick={() => navigateToEntity(entity)}>
                      <ItemTypeIcon icon={entity.itemTypeIcon} color={entity.itemTypeColor} />
                      <span class="link-text">
                        {#if linkDirectionLabel(link, pageId)}
                          <span class="relation">{linkDirectionLabel(link, pageId)}</span>
                        {/if}
                        <span class="key">{entity.workspaceKey || 'WORK'}-{entity.itemNumber ?? entity.id}</span>
                        <span class="title">{entity.title}</span>
                      </span>
                    </button>
                  {:else}
                    <div class="link-main static">
                      <ItemTypeIcon icon={entity.itemTypeIcon} color={entity.itemTypeColor} />
                      <span class="title">{entity.title}</span>
                    </div>
                  {/if}
                  {#if canEdit}
                    <button class="unlink-btn" type="button" onclick={() => requestUnlink(link.id)} aria-label={t('requirements.traceability.unlink')}>
                      <Trash2 size={16} />
                    </button>
                  {/if}
                </li>
              {/if}
            {/each}
          </ul>
        {/if}
      {/if}
    </div>

    <!-- Tests -->
    <div class="section-card" data-testid="mobile-traceability-tests">
      <div class="section-head">
        <button class="section-toggle" type="button" onclick={() => toggleSection('tests')}>
          {#if openSections.tests}<ChevronDown size={16} />{:else}<ChevronRight size={16} />{/if}
          <span>{t('requirements.traceability.linkedTests')}</span>
          <span class="count">{testLinks.length}</span>
        </button>
        {#if canEdit && linkTypeIds.testsLinkTypeId}
          <button class="add-btn" type="button" onclick={() => openAdd('test')} aria-label={t('requirements.traceability.addTest')}>
            <Plus size={16} />
          </button>
        {/if}
      </div>
      {#if openSections.tests}
        {#if testLinks.length === 0}
          <p class="empty">{t('requirements.traceability.noTests')}</p>
        {:else}
          <ul class="link-list">
            {#each testLinks as link (link.id)}
              {@const entity = linkedEntity(link, pageId)}
              {#if entity}
                <li class="link-row">
                  {#if traceabilityEntityHref(entity, workspaceId, null)}
                    <button class="link-main" type="button" onclick={() => navigateToEntity(entity)}>
                      <span class="link-text">
                        <span class="title">{entity.title}</span>
                        <Lozenge color="green">{t('requirements.traceability.covered')}</Lozenge>
                      </span>
                    </button>
                  {:else}
                    <div class="link-main static">
                      <span class="title">{entity.title}</span>
                      <Lozenge color="green">{t('requirements.traceability.covered')}</Lozenge>
                    </div>
                  {/if}
                  {#if canEdit}
                    <button class="unlink-btn" type="button" onclick={() => requestUnlink(link.id)} aria-label={t('requirements.traceability.unlink')}>
                      <Trash2 size={16} />
                    </button>
                  {/if}
                </li>
              {/if}
            {/each}
          </ul>
        {/if}
      {/if}
    </div>

    <!-- Specifies -->
    <div class="section-card" data-testid="mobile-traceability-specifies">
      <div class="section-head">
        <button class="section-toggle" type="button" onclick={() => toggleSection('specifies')}>
          {#if openSections.specifies}<ChevronDown size={16} />{:else}<ChevronRight size={16} />{/if}
          <span>{t('requirements.traceability.specifiesTitle')}</span>
          <span class="count">{specifiesOutgoingLinks.length}</span>
        </button>
        {#if canEdit && linkTypeIds.specifiesLinkTypeId}
          <button class="add-btn" type="button" onclick={() => openAdd('specifies')} aria-label={t('requirements.traceability.specifiesAdd')}>
            <Plus size={16} />
          </button>
        {/if}
      </div>
      {#if openSections.specifies}
        {#if specifiesOutgoingLinks.length === 0}
          <p class="empty">{t('requirements.traceability.specifiesEmpty')}</p>
        {:else}
          <ul class="link-list">
            {#each specifiesOutgoingLinks as link (link.id)}
              {@const entity = linkedEntity(link, pageId)}
              {#if entity}
                {@const reqMeta = requirementKeyByPageId.get(entity.id)}
                <li class="link-row">
                  {#if traceabilityEntityHref(entity, workspaceId, reqMeta)}
                    <button class="link-main" type="button" onclick={() => navigateToEntity(entity)}>
                      <span class="link-text">
                        {#if linkDirectionLabel(link, pageId)}
                          <span class="relation">{linkDirectionLabel(link, pageId)}</span>
                        {/if}
                        {#if reqMeta?.key}<Lozenge color="blue">{reqMeta.key}</Lozenge>{/if}
                        <span class="title">{entity.title}</span>
                      </span>
                    </button>
                  {:else}
                    <div class="link-main static"><span class="title">{entity.title}</span></div>
                  {/if}
                  {#if canEdit}
                    <button class="unlink-btn" type="button" onclick={() => requestUnlink(link.id)} aria-label={t('requirements.traceability.unlink')}>
                      <Trash2 size={16} />
                    </button>
                  {/if}
                </li>
              {/if}
            {/each}
          </ul>
        {/if}
      {/if}
    </div>

    <!-- Specified by -->
    <div class="section-card" data-testid="mobile-traceability-specified-by">
      <div class="section-head">
        <button class="section-toggle" type="button" onclick={() => toggleSection('specifiedBy')}>
          {#if openSections.specifiedBy}<ChevronDown size={16} />{:else}<ChevronRight size={16} />{/if}
          <span>{t('requirements.traceability.specifiedByTitle')}</span>
          <span class="count">{specifiedByLinks.length}</span>
        </button>
      </div>
      {#if openSections.specifiedBy}
        {#if specifiedByLinks.length === 0}
          <p class="empty">{t('requirements.traceability.specifiedByEmpty')}</p>
        {:else}
          <ul class="link-list">
            {#each specifiedByLinks as link (link.id)}
              {@const entity = linkedEntity(link, pageId)}
              {#if entity}
                {@const reqMeta = requirementKeyByPageId.get(entity.id)}
                <li class="link-row">
                  {#if traceabilityEntityHref(entity, workspaceId, reqMeta)}
                    <button class="link-main" type="button" onclick={() => navigateToEntity(entity)}>
                      <span class="link-text">
                        {#if linkDirectionLabel(link, pageId)}
                          <span class="relation">{linkDirectionLabel(link, pageId)}</span>
                        {/if}
                        {#if reqMeta?.key}<Lozenge color="blue">{reqMeta.key}</Lozenge>{/if}
                        <span class="title">{entity.title}</span>
                      </span>
                    </button>
                  {:else}
                    <div class="link-main static"><span class="title">{entity.title}</span></div>
                  {/if}
                  {#if canEdit}
                    <button class="unlink-btn" type="button" onclick={() => requestUnlink(link.id)} aria-label={t('requirements.traceability.unlink')}>
                      <Trash2 size={16} />
                    </button>
                  {/if}
                </li>
              {/if}
            {/each}
          </ul>
        {/if}
      {/if}
    </div>

    <!-- Related pages -->
    <div class="section-card" data-testid="mobile-traceability-related-pages">
      <div class="section-head">
        <button class="section-toggle" type="button" onclick={() => toggleSection('relatedPages')}>
          {#if openSections.relatedPages}<ChevronDown size={16} />{:else}<ChevronRight size={16} />{/if}
          <span>{t('requirements.traceability.relatedGenericTitle')}</span>
          <span class="count">{genericRelatedPageLinks.length}</span>
        </button>
        {#if canEdit && linkTypeIds.relatesToLinkTypeId}
          <button class="add-btn" type="button" onclick={() => openAdd('genericPage')} aria-label={t('requirements.traceability.addPage')}>
            <Plus size={16} />
          </button>
        {/if}
      </div>
      {#if openSections.relatedPages}
        {#if genericRelatedPageLinks.length === 0}
          <p class="empty">{t('requirements.traceability.noPages')}</p>
        {:else}
          <ul class="link-list">
            {#each genericRelatedPageLinks as link (link.id)}
              {@const entity = linkedEntity(link, pageId)}
              {#if entity}
                {@const reqMeta = requirementKeyByPageId.get(entity.id)}
                <li class="link-row">
                  {#if traceabilityEntityHref(entity, workspaceId, reqMeta)}
                    <button class="link-main" type="button" onclick={() => navigateToEntity(entity)}>
                      <span class="link-text">
                        {#if reqMeta?.key}<Lozenge color="blue">{reqMeta.key}</Lozenge>{/if}
                        <span class="title">{entity.title}</span>
                      </span>
                    </button>
                  {:else}
                    <div class="link-main static"><span class="title">{entity.title}</span></div>
                  {/if}
                  {#if canEdit}
                    <button class="unlink-btn" type="button" onclick={() => requestUnlink(link.id)} aria-label={t('requirements.traceability.unlink')}>
                      <Trash2 size={16} />
                    </button>
                  {/if}
                </li>
              {/if}
            {/each}
          </ul>
        {/if}
      {/if}
    </div>

    <!-- Assets -->
    <div class="section-card" data-testid="mobile-traceability-assets">
      <div class="section-head">
        <button class="section-toggle" type="button" onclick={() => toggleSection('assets')}>
          {#if openSections.assets}<ChevronDown size={16} />{:else}<ChevronRight size={16} />{/if}
          <span>{t('requirements.traceability.linkedAssets')}</span>
          <span class="count">{assetLinks.length}</span>
        </button>
        {#if canEdit && linkTypeIds.relatesToLinkTypeId}
          <button class="add-btn" type="button" onclick={() => openAdd('asset')} aria-label={t('requirements.traceability.addAsset')}>
            <Plus size={16} />
          </button>
        {/if}
      </div>
      {#if openSections.assets}
        {#if assetLinks.length === 0}
          <p class="empty">{t('requirements.traceability.noAssets')}</p>
        {:else}
          <ul class="link-list">
            {#each assetLinks as link (link.id)}
              {@const entity = linkedEntity(link, pageId)}
              {#if entity}
                <li class="link-row">
                  {#if traceabilityEntityHref(entity, workspaceId, null)}
                    <button class="link-main" type="button" onclick={() => navigateToEntity(entity)}>
                      <span class="title">{entity.title}</span>
                    </button>
                  {:else}
                    <div class="link-main static">
                      <span class="title">{entity.title}</span>
                    </div>
                  {/if}
                  {#if canEdit}
                    <button class="unlink-btn" type="button" onclick={() => requestUnlink(link.id)} aria-label={t('requirements.traceability.unlink')}>
                      <Trash2 size={16} />
                    </button>
                  {/if}
                </li>
              {/if}
            {/each}
          </ul>
        {/if}
      {/if}
    </div>
  {/if}
</section>

<MobileTraceabilityAddSheet
  bind:isOpen={addSheetOpen}
  title={addSheetTitle}
  searchPlaceholder={addSheetPlaceholder}
  searchFn={runAddSearch}
  getLabel={addPickLabel}
  onPick={handleAddPick}
  dataTestid="mobile-traceability-add-sheet"
/>

<MobileConfirmSheet
  bind:isOpen={unlinkSheetOpen}
  title={t('requirements.traceability.unlink')}
  message={t('requirements.mobile.traceabilityUnlinkConfirm')}
  confirmLabel={t('requirements.traceability.unlink')}
  cancelLabel={t('common.cancel')}
  destructive
  busy={unlinkBusy}
  onconfirm={confirmUnlink}
  onclose={() => { unlinkTarget = null; }}
  dataTestid="mobile-traceability-unlink"
/>

<style>
  .traceability {
    margin-bottom: 1.25rem;
    padding: 0.75rem;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-lg, 8px);
    background: var(--ds-surface-raised);
  }

  .trace-header {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    margin-bottom: 0.5rem;
    color: var(--ds-text);
  }

  .section-title {
    font-size: 0.875rem;
    font-weight: var(--font-semibold, 600);
    margin: 0;
  }

  .summary {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.8125rem;
    color: var(--ds-text-subtle);
    margin-bottom: 0.75rem;
  }

  .center {
    display: flex;
    justify-content: center;
    padding: 1rem 0;
    color: var(--ds-text-subtle);
  }

  :global(.spin) {
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .error {
    margin: 0;
    font-size: 0.875rem;
    color: var(--ds-text-danger);
  }

  .section-card {
    border-top: 1px solid var(--ds-border);
    padding-top: 0.5rem;
    margin-top: 0.5rem;
  }

  .section-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }

  .section-toggle {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    flex: 1;
    min-width: 0;
    border: none;
    background: transparent;
    color: var(--ds-text);
    font-size: 0.8125rem;
    font-weight: var(--font-semibold, 600);
    text-align: left;
    padding: 0.25rem 0;
    cursor: pointer;
  }

  .count {
    color: var(--ds-text-subtle);
    font-weight: var(--font-medium, 500);
  }

  .add-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border: none;
    border-radius: var(--radius-md, 6px);
    background: transparent;
    color: var(--ds-interactive);
    cursor: pointer;
  }

  .add-btn:active {
    background: var(--ds-background-neutral-hovered);
  }

  .empty {
    margin: 0.35rem 0 0;
    font-size: 0.8125rem;
    color: var(--ds-text-subtle);
  }

  .link-list {
    list-style: none;
    margin: 0.35rem 0 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .link-row {
    display: flex;
    align-items: center;
    gap: 0.25rem;
  }

  .link-main {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-md, 6px);
    background: var(--ds-surface);
    padding: 0.5rem 0.65rem;
    text-align: left;
    color: inherit;
    font: inherit;
    cursor: pointer;
  }

  .link-main.static {
    cursor: default;
  }

  .link-main:active:not(.static) {
    background: var(--ds-background-neutral-hovered);
  }

  .link-text {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    min-width: 0;
  }

  .relation {
    font-size: 0.6875rem;
    color: var(--ds-text-subtle);
    text-transform: lowercase;
  }

  .key {
    font-size: 0.75rem;
    color: var(--ds-text-subtle);
    font-family: var(--ds-font-family-mono, monospace);
  }

  .title {
    font-size: 0.875rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .unlink-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border: none;
    border-radius: var(--radius-md, 6px);
    background: transparent;
    color: var(--ds-text-subtle);
    cursor: pointer;
    flex-shrink: 0;
  }

  .unlink-btn:active {
    color: var(--ds-text-danger);
    background: var(--ds-background-danger-subtle);
  }
</style>
