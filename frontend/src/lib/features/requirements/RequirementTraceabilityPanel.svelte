<script>
  import { onDestroy } from 'svelte';
  import { IconPlus, IconTrash, IconLink } from '@tabler/icons-svelte-runes';
  import { api } from '../../api.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import Button from '../../components/Button.svelte';
  import Input from '../../components/Input.svelte';
  import ItemTypeIcon from '../../components/ItemTypeIcon.svelte';
  import StatusBadge from '../../components/StatusBadge.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import Spinner from '../../components/Spinner.svelte';
  import { buildRequirementKeyByPageId, pageHref } from './requirementKeyMap.js';
  import { linkDirectionLabel } from './requirementLinkLabels.js';
  import {
    mergePageLinks,
    resolveLinkTypeIds,
    linkedEntity as resolveLinkedEntity,
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

  let loading = $state(false);
  let pageLinks = $state([]);
  let linkTypesCache = $state([]);
  let loadRequestSeq = 0;

  let requirementKeyByPageId = $state(new Map());
  let itemMode = $state('list');
  let testMode = $state('list');
  let specifiesPageMode = $state('list');
  let genericPageMode = $state('list');
  let assetMode = $state('list');
  let itemSearchQuery = $state('');
  let testSearchQuery = $state('');
  let itemSearchResults = $state([]);
  let testSearchResults = $state([]);
  let itemSearching = $state(false);
  let testSearching = $state(false);
  let itemSubmitting = $state(false);
  let testSubmitting = $state(false);
  let relatedPageSearchQuery = $state('');
  let assetSearchQuery = $state('');
  let relatedPageSearchResults = $state([]);
  let assetSearchResults = $state([]);
  let relatedPageSearching = $state(false);
  let assetSearching = $state(false);
  let relatedPageSubmitting = $state(false);
  let assetSubmitting = $state(false);
  let itemSearchTimer;
  let testSearchTimer;
  let relatedPageSearchTimer;
  let assetSearchTimer;
  let itemSearchVersion = 0;
  let testSearchVersion = 0;
  let relatedPageSearchVersion = 0;
  let assetSearchVersion = 0;

  const linkTypeIds = $derived(resolveLinkTypeIds(linkTypesCache));
  const testsLinkTypeId = $derived(linkTypeIds.testsLinkTypeId);
  const relatesToLinkTypeId = $derived(linkTypeIds.relatesToLinkTypeId);
  const specifiesLinkTypeId = $derived(linkTypeIds.specifiesLinkTypeId);
  const itemLinkTypeId = $derived(linkTypeIds.itemLinkTypeId);

  const itemLinks = $derived(filterItemLinks(pageLinks, pageId));
  const testLinks = $derived(filterTestLinks(pageLinks, pageId));
  const specifiesOutgoingLinks = $derived(filterSpecifiesOutgoing(pageLinks, pageId, specifiesLinkTypeId));
  const specifiedByLinks = $derived(filterSpecifiedBy(pageLinks, pageId, specifiesLinkTypeId));
  const genericRelatedPageLinks = $derived(filterGenericPageLinks(pageLinks, pageId, relatesToLinkTypeId));
  const assetLinks = $derived(filterAssetLinks(pageLinks, pageId));

  $effect(() => {
    if (!pageId) return;
    void ensureLinkTypesLoaded();
    void loadPageLinks();
  });

  $effect(() => {
    if (!workspaceId) return;
    void loadRequirementKeys();
  });

  onDestroy(() => {
    clearTimeout(itemSearchTimer);
    clearTimeout(testSearchTimer);
    clearTimeout(relatedPageSearchTimer);
    clearTimeout(assetSearchTimer);
    itemSearchVersion += 1;
    testSearchVersion += 1;
    relatedPageSearchVersion += 1;
    assetSearchVersion += 1;
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
    try {
      const resp = await api.links.getForPage(pageId);
      if (requestSeq !== loadRequestSeq) return;
      pageLinks = mergePageLinks(resp);
    } catch (err) {
      if (requestSeq !== loadRequestSeq) return;
      console.error('failed to load page links', err);
      pageLinks = [];
    } finally {
      if (requestSeq === loadRequestSeq) loading = false;
    }
  }

  function linkedEntity(link) {
    return resolveLinkedEntity(link, pageId);
  }

  async function unlink(linkId) {
    try {
      await api.links.delete(linkId);
      pageLinks = pageLinks.filter((link) => link.id !== linkId);
    } catch (err) {
      errorToast(err?.message || t('requirements.traceability.unlinkError'));
    }
  }

  function handleItemSearchInput(event) {
    const q = event.currentTarget.value;
    itemSearchQuery = q;
    clearTimeout(itemSearchTimer);
    const version = ++itemSearchVersion;
    if (q.trim().length < 2) {
      itemSearchResults = [];
      itemSearching = false;
      return;
    }
    itemSearching = true;
    itemSearchTimer = setTimeout(() => searchItems(q.trim(), version), 250);
  }

  async function searchItems(q, version) {
    try {
      const results = await api.links.search(q, 'item', 10);
      if (version !== itemSearchVersion) return;
      itemSearchResults = Array.isArray(results) ? results : [];
    } catch (err) {
      if (version !== itemSearchVersion) return;
      itemSearchResults = [];
    } finally {
      if (version === itemSearchVersion) itemSearching = false;
    }
  }

  async function linkItem(item) {
    if (!itemLinkTypeId || itemSubmitting) return;
    itemSubmitting = true;
    try {
      const link = await api.links.create(buildItemLinkCreate(pageId, item, itemLinkTypeId));
      if (link && !pageLinks.some((l) => l.id === link.id)) {
        pageLinks = [link, ...pageLinks];
      } else {
        await loadPageLinks();
      }
      itemMode = 'list';
      itemSearchQuery = '';
      itemSearchResults = [];
    } catch (err) {
      errorToast(err?.message || t('requirements.traceability.linkItemError'));
    } finally {
      itemSubmitting = false;
    }
  }

  function handleTestSearchInput(event) {
    const q = event.currentTarget.value;
    testSearchQuery = q;
    clearTimeout(testSearchTimer);
    const version = ++testSearchVersion;
    if (q.trim().length < 2) {
      testSearchResults = [];
      testSearching = false;
      return;
    }
    testSearching = true;
    testSearchTimer = setTimeout(() => searchTests(q.trim(), version), 250);
  }

  async function searchTests(q, version) {
    try {
      const results = await api.tests.testCases.getAll(workspaceId, { q, limit: 10 });
      if (version !== testSearchVersion) return;
      testSearchResults = Array.isArray(results) ? results : (results?.data ?? []);
    } catch (err) {
      if (version !== testSearchVersion) return;
      testSearchResults = [];
    } finally {
      if (version === testSearchVersion) testSearching = false;
    }
  }

  async function linkTestCase(testCase) {
    if (!testsLinkTypeId || testSubmitting) return;
    testSubmitting = true;
    try {
      const link = await api.links.create(buildTestLinkCreate(pageId, testCase, testsLinkTypeId));
      if (link && !pageLinks.some((l) => l.id === link.id)) {
        pageLinks = [link, ...pageLinks];
      } else {
        await loadPageLinks();
      }
      testMode = 'list';
      testSearchQuery = '';
      testSearchResults = [];
    } catch (err) {
      errorToast(err?.message || t('requirements.traceability.linkTestError'));
    } finally {
      testSubmitting = false;
    }
  }

  function handleRelatedPageSearchInput(event) {
    const q = event.currentTarget.value;
    relatedPageSearchQuery = q;
    clearTimeout(relatedPageSearchTimer);
    const version = ++relatedPageSearchVersion;
    if (q.trim().length < 2) {
      relatedPageSearchResults = [];
      relatedPageSearching = false;
      return;
    }
    relatedPageSearching = true;
    relatedPageSearchTimer = setTimeout(() => searchRelatedPages(q.trim(), version), 250);
  }

  async function searchRelatedPages(q, version) {
    try {
      const results = await api.pages.searchPages(workspaceId, q, { limit: 10 });
      if (version !== relatedPageSearchVersion) return;
      const rows = Array.isArray(results) ? results : (results?.data ?? []);
      relatedPageSearchResults = rows.filter((page) => page?.id !== pageId);
    } catch (err) {
      if (version !== relatedPageSearchVersion) return;
      relatedPageSearchResults = [];
    } finally {
      if (version === relatedPageSearchVersion) relatedPageSearching = false;
    }
  }

  function clearRelatedPageSearch() {
    relatedPageSearchQuery = '';
    relatedPageSearchResults = [];
    relatedPageSearching = false;
  }

  function openSpecifiesPageAdd() {
    genericPageMode = 'list';
    clearRelatedPageSearch();
    specifiesPageMode = 'add';
  }

  function openGenericPageAdd() {
    specifiesPageMode = 'list';
    clearRelatedPageSearch();
    genericPageMode = 'add';
  }

  async function linkPageToPage(linkTypeId, relatedPage, onLinked) {
    if (!linkTypeId || relatedPageSubmitting || relatedPage.id === pageId) return;
    relatedPageSubmitting = true;
    try {
      const payload =
        linkTypeId === specifiesLinkTypeId
          ? buildSpecifiesLinkCreate(pageId, relatedPage, linkTypeId)
          : buildGenericPageLinkCreate(pageId, relatedPage, linkTypeId);
      const link = await api.links.create(payload);
      if (link && !pageLinks.some((l) => l.id === link.id)) {
        pageLinks = [link, ...pageLinks];
      } else {
        await loadPageLinks();
      }
      onLinked();
      clearRelatedPageSearch();
    } catch (err) {
      errorToast(err?.message || t('requirements.traceability.linkPageError'));
    } finally {
      relatedPageSubmitting = false;
    }
  }

  async function linkSpecifiesPage(relatedPage) {
    await linkPageToPage(specifiesLinkTypeId, relatedPage, () => {
      specifiesPageMode = 'list';
    });
  }

  async function linkGenericRelatedPage(relatedPage) {
    await linkPageToPage(relatesToLinkTypeId, relatedPage, () => {
      genericPageMode = 'list';
    });
  }

  function handleAssetSearchInput(event) {
    const q = event.currentTarget.value;
    assetSearchQuery = q;
    clearTimeout(assetSearchTimer);
    const version = ++assetSearchVersion;
    if (q.trim().length < 2) {
      assetSearchResults = [];
      assetSearching = false;
      return;
    }
    assetSearching = true;
    assetSearchTimer = setTimeout(() => searchAssets(q.trim(), version), 250);
  }

  async function searchAssets(q, version) {
    try {
      const results = await api.links.search(q, 'asset', 10);
      if (version !== assetSearchVersion) return;
      assetSearchResults = Array.isArray(results) ? results : [];
    } catch (err) {
      if (version !== assetSearchVersion) return;
      assetSearchResults = [];
    } finally {
      if (version === assetSearchVersion) assetSearching = false;
    }
  }

  async function linkAsset(asset) {
    if (!relatesToLinkTypeId || assetSubmitting) return;
    assetSubmitting = true;
    try {
      const link = await api.links.create(buildAssetLinkCreate(pageId, asset, relatesToLinkTypeId));
      if (link && !pageLinks.some((l) => l.id === link.id)) {
        pageLinks = [link, ...pageLinks];
      } else {
        await loadPageLinks();
      }
      assetMode = 'list';
      assetSearchQuery = '';
      assetSearchResults = [];
    } catch (err) {
      errorToast(err?.message || t('requirements.traceability.linkAssetError'));
    } finally {
      assetSubmitting = false;
    }
  }

</script>

<section class="traceability-panel" aria-label={t('requirements.traceability.title')}>
  <header class="traceability-panel__header">
    <IconLink size={16} aria-hidden="true" />
    <h3>{t('requirements.traceability.title')}</h3>
  </header>

  {#if loading}
    <div class="traceability-panel__loading"><Spinner /></div>
  {:else}
    <div class="traceability-panel__grid">
      <div class="traceability-section">
        <div class="traceability-section__header">
          <h4>{t('requirements.traceability.implementsTitle')}</h4>
          {#if canEdit && itemLinkTypeId && itemMode === 'list'}
            <Button variant="ghost" size="sm" onclick={() => (itemMode = 'add')}>
              <IconPlus size={14} />
              {t('requirements.traceability.implementsAdd')}
            </Button>
          {/if}
        </div>

        {#if itemMode === 'add'}
          <div class="traceability-search">
            <Input
              type="text"
              value={itemSearchQuery}
              oninput={handleItemSearchInput}
              placeholder={t('requirements.traceability.searchItems')}
              size="small"
            />
            <Button variant="ghost" size="sm" onclick={() => { itemMode = 'list'; itemSearchQuery = ''; itemSearchResults = []; }}>
              {t('common.cancel')}
            </Button>
          </div>
          {#if itemSearching}
            <p class="traceability-empty">{t('common.loading')}</p>
          {:else if itemSearchResults.length === 0}
            <p class="traceability-empty">{t('pickers.noResultsFor', { query: itemSearchQuery || '…' })}</p>
          {:else}
            <ul class="traceability-list">
              {#each itemSearchResults as item (item.id)}
                <li>
                  <button type="button" class="traceability-row" onclick={() => linkItem(item)} disabled={itemSubmitting}>
                    <ItemTypeIcon icon={item.item_type_icon} color={item.item_type_color} />
                    <span class="traceability-row__title">{item.title}</span>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        {:else if itemLinks.length === 0}
          <p class="traceability-empty">{t('requirements.traceability.implementsEmpty')}</p>
        {:else}
          <ul class="traceability-list">
            {#each itemLinks as link (link.id)}
              {@const entity = linkedEntity(link)}
              {@const relationLabel = linkDirectionLabel(link, pageId)}
              {#if entity}
                <li class="traceability-row-li">
                  <a class="traceability-row traceability-row--link" href={`/workspaces/${entity.workspaceId || workspaceId}/items/${entity.id}`}>
                    <ItemTypeIcon icon={entity.itemTypeIcon} color={entity.itemTypeColor} />
                    {#if relationLabel}
                      <span class="traceability-row__relation">{relationLabel}</span>
                    {/if}
                    <span class="traceability-row__key">{entity.workspaceKey || 'WORK'}-{entity.itemNumber ?? entity.id}</span>
                    <span class="traceability-row__title">{entity.title}</span>
                    {#if entity.statusName}
                      <StatusBadge status={{ label: entity.statusName, categoryColor: entity.statusColor }} uppercase={false} showDot={false} />
                    {/if}
                  </a>
                  {#if canEdit}
                    <button type="button" class="traceability-unlink" onclick={() => unlink(link.id)} aria-label={t('requirements.traceability.unlink')}>
                      <IconTrash size={14} />
                    </button>
                  {/if}
                </li>
              {/if}
            {/each}
          </ul>
        {/if}
      </div>

      <div class="traceability-section">
        <div class="traceability-section__header">
          <h4>{t('requirements.traceability.linkedTests')}</h4>
          {#if canEdit && testsLinkTypeId && testMode === 'list'}
            <Button variant="ghost" size="sm" onclick={() => (testMode = 'add')}>
              <IconPlus size={14} />
              {t('requirements.traceability.addTest')}
            </Button>
          {/if}
        </div>

        {#if testMode === 'add'}
          <div class="traceability-search">
            <Input
              type="text"
              value={testSearchQuery}
              oninput={handleTestSearchInput}
              placeholder={t('requirements.traceability.searchTests')}
              size="small"
            />
            <Button variant="ghost" size="sm" onclick={() => { testMode = 'list'; testSearchQuery = ''; testSearchResults = []; }}>
              {t('common.cancel')}
            </Button>
          </div>
          {#if testSearching}
            <p class="traceability-empty">{t('common.loading')}</p>
          {:else if testSearchResults.length === 0}
            <p class="traceability-empty">{t('pickers.noResultsFor', { query: testSearchQuery || '…' })}</p>
          {:else}
            <ul class="traceability-list">
              {#each testSearchResults as testCase (testCase.id)}
                <li>
                  <button type="button" class="traceability-row" onclick={() => linkTestCase(testCase)} disabled={testSubmitting}>
                    <span class="traceability-row__title">{testCase.title}</span>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        {:else if testLinks.length === 0}
          <p class="traceability-empty">
            {t('requirements.traceability.noTests')}
            <Lozenge color="red" class="traceability-uncovered">{t('requirements.traceability.uncovered')}</Lozenge>
          </p>
        {:else}
          <ul class="traceability-list">
            {#each testLinks as link (link.id)}
              {@const entity = linkedEntity(link)}
              {#if entity}
                <li class="traceability-row-li">
                  <a class="traceability-row traceability-row--link" href={`/workspaces/${workspaceId}/tests/${entity.id}`}>
                    <span class="traceability-row__title">{entity.title}</span>
                    <Lozenge color="green">{t('requirements.traceability.covered')}</Lozenge>
                  </a>
                  {#if canEdit}
                    <button type="button" class="traceability-unlink" onclick={() => unlink(link.id)} aria-label={t('requirements.traceability.unlink')}>
                      <IconTrash size={14} />
                    </button>
                  {/if}
                </li>
              {/if}
            {/each}
          </ul>
        {/if}
      </div>

      <div class="traceability-section">
        <div class="traceability-section__header">
          <h4>{t('requirements.traceability.specifiesTitle')}</h4>
          {#if canEdit && specifiesLinkTypeId && specifiesPageMode === 'list'}
            <Button variant="ghost" size="sm" onclick={openSpecifiesPageAdd}>
              <IconPlus size={14} />
              {t('requirements.traceability.specifiesAdd')}
            </Button>
          {/if}
        </div>

        {#if specifiesPageMode === 'add'}
          <div class="traceability-search">
            <Input
              type="text"
              value={relatedPageSearchQuery}
              oninput={handleRelatedPageSearchInput}
              placeholder={t('requirements.traceability.searchPages')}
              size="small"
            />
            <Button variant="ghost" size="sm" onclick={() => { specifiesPageMode = 'list'; clearRelatedPageSearch(); }}>
              {t('common.cancel')}
            </Button>
          </div>
          {#if relatedPageSearching}
            <p class="traceability-empty">{t('common.loading')}</p>
          {:else if relatedPageSearchResults.length === 0}
            <p class="traceability-empty">{t('pickers.noResultsFor', { query: relatedPageSearchQuery || '…' })}</p>
          {:else}
            <ul class="traceability-list">
              {#each relatedPageSearchResults as relatedPage (relatedPage.id)}
                {@const relatedPageReqMeta = requirementKeyByPageId.get(relatedPage.id)}
                <li>
                  <button type="button" class="traceability-row" onclick={() => linkSpecifiesPage(relatedPage)} disabled={relatedPageSubmitting}>
                    {#if relatedPageReqMeta?.key}
                      <span class="traceability-row__key">{relatedPageReqMeta.key}</span>
                    {/if}
                    <span class="traceability-row__title">{relatedPage.title}</span>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        {:else if specifiesOutgoingLinks.length === 0}
          <p class="traceability-empty">{t('requirements.traceability.specifiesEmpty')}</p>
        {:else}
          <ul class="traceability-list">
            {#each specifiesOutgoingLinks as link (link.id)}
              {@const entity = linkedEntity(link)}
              {@const relationLabel = linkDirectionLabel(link, pageId)}
              {#if entity}
                {@const reqMeta = requirementKeyByPageId.get(entity.id)}
                <li class="traceability-row-li">
                  <a class="traceability-row traceability-row--link" href={pageHref(workspaceId, entity.id, reqMeta)}>
                    {#if relationLabel}
                      <span class="traceability-row__relation">{relationLabel}</span>
                    {/if}
                    {#if reqMeta?.key}
                      <Lozenge color="blue">{reqMeta.key}</Lozenge>
                    {/if}
                    <span class="traceability-row__title">{entity.title}</span>
                  </a>
                  {#if canEdit}
                    <button type="button" class="traceability-unlink" onclick={() => unlink(link.id)} aria-label={t('requirements.traceability.unlink')}>
                      <IconTrash size={14} />
                    </button>
                  {/if}
                </li>
              {/if}
            {/each}
          </ul>
        {/if}
      </div>

      <div class="traceability-section">
        <div class="traceability-section__header">
          <h4>{t('requirements.traceability.specifiedByTitle')}</h4>
        </div>
        {#if specifiedByLinks.length === 0}
          <p class="traceability-empty">{t('requirements.traceability.specifiedByEmpty')}</p>
        {:else}
          <ul class="traceability-list">
            {#each specifiedByLinks as link (link.id)}
              {@const entity = linkedEntity(link)}
              {@const relationLabel = linkDirectionLabel(link, pageId)}
              {#if entity}
                {@const reqMeta = requirementKeyByPageId.get(entity.id)}
                <li class="traceability-row-li">
                  <a class="traceability-row traceability-row--link" href={pageHref(workspaceId, entity.id, reqMeta)}>
                    {#if relationLabel}
                      <span class="traceability-row__relation">{relationLabel}</span>
                    {/if}
                    {#if reqMeta?.key}
                      <Lozenge color="blue">{reqMeta.key}</Lozenge>
                    {/if}
                    <span class="traceability-row__title">{entity.title}</span>
                  </a>
                  {#if canEdit}
                    <button type="button" class="traceability-unlink" onclick={() => unlink(link.id)} aria-label={t('requirements.traceability.unlink')}>
                      <IconTrash size={14} />
                    </button>
                  {/if}
                </li>
              {/if}
            {/each}
          </ul>
        {/if}
      </div>

      <div class="traceability-section">
        <div class="traceability-section__header">
          <h4>{t('requirements.traceability.relatedGenericTitle')}</h4>
          {#if canEdit && relatesToLinkTypeId && genericPageMode === 'list'}
            <Button variant="ghost" size="sm" onclick={openGenericPageAdd}>
              <IconPlus size={14} />
              {t('requirements.traceability.addPage')}
            </Button>
          {/if}
        </div>

        {#if genericPageMode === 'add'}
          <div class="traceability-search">
            <Input
              type="text"
              value={relatedPageSearchQuery}
              oninput={handleRelatedPageSearchInput}
              placeholder={t('requirements.traceability.searchPages')}
              size="small"
            />
            <Button variant="ghost" size="sm" onclick={() => { genericPageMode = 'list'; clearRelatedPageSearch(); }}>
              {t('common.cancel')}
            </Button>
          </div>
          {#if relatedPageSearching}
            <p class="traceability-empty">{t('common.loading')}</p>
          {:else if relatedPageSearchResults.length === 0}
            <p class="traceability-empty">{t('pickers.noResultsFor', { query: relatedPageSearchQuery || '…' })}</p>
          {:else}
            <ul class="traceability-list">
              {#each relatedPageSearchResults as relatedPage (relatedPage.id)}
                {@const relatedPageReqMeta = requirementKeyByPageId.get(relatedPage.id)}
                <li>
                  <button type="button" class="traceability-row" onclick={() => linkGenericRelatedPage(relatedPage)} disabled={relatedPageSubmitting}>
                    {#if relatedPageReqMeta?.key}
                      <span class="traceability-row__key">{relatedPageReqMeta.key}</span>
                    {/if}
                    <span class="traceability-row__title">{relatedPage.title}</span>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        {:else if genericRelatedPageLinks.length === 0}
          <p class="traceability-empty">{t('requirements.traceability.noPages')}</p>
        {:else}
          <ul class="traceability-list">
            {#each genericRelatedPageLinks as link (link.id)}
              {@const entity = linkedEntity(link)}
              {#if entity}
                {@const reqMeta = requirementKeyByPageId.get(entity.id)}
                <li class="traceability-row-li">
                  <a class="traceability-row traceability-row--link" href={pageHref(workspaceId, entity.id, reqMeta)}>
                    {#if reqMeta?.key}
                      <Lozenge color="blue">{reqMeta.key}</Lozenge>
                    {/if}
                    <span class="traceability-row__title">{entity.title}</span>
                  </a>
                  {#if canEdit}
                    <button type="button" class="traceability-unlink" onclick={() => unlink(link.id)} aria-label={t('requirements.traceability.unlink')}>
                      <IconTrash size={14} />
                    </button>
                  {/if}
                </li>
              {/if}
            {/each}
          </ul>
        {/if}
      </div>

      <div class="traceability-section">
        <div class="traceability-section__header">
          <h4>{t('requirements.traceability.linkedAssets')}</h4>
          {#if canEdit && relatesToLinkTypeId && assetMode === 'list'}
            <Button variant="ghost" size="sm" onclick={() => (assetMode = 'add')}>
              <IconPlus size={14} />
              {t('requirements.traceability.addAsset')}
            </Button>
          {/if}
        </div>

        {#if assetMode === 'add'}
          <div class="traceability-search">
            <Input
              type="text"
              value={assetSearchQuery}
              oninput={handleAssetSearchInput}
              placeholder={t('requirements.traceability.searchAssets')}
              size="small"
            />
            <Button variant="ghost" size="sm" onclick={() => { assetMode = 'list'; assetSearchQuery = ''; assetSearchResults = []; }}>
              {t('common.cancel')}
            </Button>
          </div>
          {#if assetSearching}
            <p class="traceability-empty">{t('common.loading')}</p>
          {:else if assetSearchResults.length === 0}
            <p class="traceability-empty">{t('pickers.noResultsFor', { query: assetSearchQuery || '…' })}</p>
          {:else}
            <ul class="traceability-list">
              {#each assetSearchResults as asset (asset.id)}
                <li>
                  <button type="button" class="traceability-row" onclick={() => linkAsset(asset)} disabled={assetSubmitting}>
                    <span class="traceability-row__title">{asset.title}</span>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        {:else if assetLinks.length === 0}
          <p class="traceability-empty">{t('requirements.traceability.noAssets')}</p>
        {:else}
          <ul class="traceability-list">
            {#each assetLinks as link (link.id)}
              {@const entity = linkedEntity(link)}
              {#if entity}
                <li class="traceability-row-li">
                  <a class="traceability-row traceability-row--link" href={`/assets/${entity.id}`}>
                    <span class="traceability-row__title">{entity.title}</span>
                  </a>
                  {#if canEdit}
                    <button type="button" class="traceability-unlink" onclick={() => unlink(link.id)} aria-label={t('requirements.traceability.unlink')}>
                      <IconTrash size={14} />
                    </button>
                  {/if}
                </li>
              {/if}
            {/each}
          </ul>
        {/if}
      </div>
    </div>
  {/if}
</section>

<style>
  .traceability-panel {
    border-bottom: 1px solid var(--ds-border);
    padding: 0.75rem 1rem 1rem;
    background: var(--ds-surface-sunken);
  }
  .traceability-panel__header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.75rem;
  }
  .traceability-panel__header h3 {
    margin: 0;
    font-size: 0.875rem;
    font-weight: 600;
  }
  .traceability-panel__loading {
    display: flex;
    justify-content: center;
    padding: 1rem;
  }
  .traceability-panel__grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 1rem;
  }
  @media (max-width: 900px) {
    .traceability-panel__grid {
      grid-template-columns: 1fr;
    }
  }
  .traceability-section__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
    margin-bottom: 0.5rem;
  }
  .traceability-section__header h4 {
    margin: 0;
    font-size: 0.8125rem;
    font-weight: 600;
    color: var(--ds-text-subtle);
    text-transform: uppercase;
    letter-spacing: 0.02em;
  }
  .traceability-search {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    margin-bottom: 0.5rem;
  }
  .traceability-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }
  .traceability-row-li {
    display: flex;
    align-items: center;
    gap: 0.25rem;
  }
  .traceability-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    min-width: 0;
    padding: 0.375rem 0.5rem;
    border: 1px solid transparent;
    border-radius: 0.375rem;
    background: var(--ds-surface-raised);
    color: inherit;
    text-align: left;
    cursor: pointer;
  }
  .traceability-row--link {
    text-decoration: none;
  }
  .traceability-row:hover {
    border-color: var(--ds-border);
    background: var(--ds-background-neutral-hovered);
  }
  .traceability-row__relation {
    flex-shrink: 0;
    font-size: 0.6875rem;
    font-weight: 500;
    color: var(--ds-text-subtle);
    text-transform: lowercase;
  }
  .traceability-row__key {
    flex-shrink: 0;
    font-size: 0.75rem;
    color: var(--ds-text-subtle);
    font-family: var(--ds-font-family-mono, monospace);
  }
  .traceability-row__title {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.875rem;
  }
  .traceability-unlink {
    display: inline-flex;
    padding: 0.25rem;
    border: none;
    background: transparent;
    color: var(--ds-text-subtle);
    cursor: pointer;
    border-radius: 0.25rem;
  }
  .traceability-unlink:hover {
    color: var(--ds-text-danger);
    background: var(--ds-background-danger-subtle);
  }
  .traceability-empty {
    margin: 0;
    font-size: 0.8125rem;
    color: var(--ds-text-subtle);
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
  }
  :global(.traceability-uncovered) {
    flex-shrink: 0;
  }
</style>
