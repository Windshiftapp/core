<script>
  import { onMount } from 'svelte';
  import { useEventListener } from 'runed';
  import ApiDocsSidebar from '../features/api-docs/ApiDocsSidebar.svelte';
  import SidebarResizeHandle from '../layout/SidebarResizeHandle.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import {
    API_SPEC_VERSIONS,
    filterGroups,
    loadSpec,
    groupOperationsByTag,
  } from '../features/api-docs/openapi-store.svelte.js';

  // ApiOperation pulls in marked + dompurify for description rendering;
  // dynamic-import keeps that weight out of the main bundle so this page
  // only pays for it when actually visited.
  /** @type {import('svelte').Component<any> | null} */
  let ApiOperation = $state(null);

  /** @type {import('../features/api-docs/openapi-store.svelte.js').OpenAPISpec | null} */
  let spec = $state(null);
  /** @type {import('../features/api-docs/openapi-store.svelte.js').OperationGroup[]} */
  let groups = $state([]);
  let loading = $state(true);
  /** @type {string | null} */
  let loadError = $state(null);
  /** @type {string | null} */
  let selectedId = $state(null);
  let selectedVersion = $state(API_SPEC_VERSIONS[0].value);
  let query = $state('');
  let loadSequence = 0;
  /** @type {HTMLElement | null} */
  let mainPanel = $state(null);

  const SIDEBAR_WIDTH_KEY = 'api-docs-sidebar-width';
  const SIDEBAR_MIN_WIDTH = 240;
  const SIDEBAR_MAX_WIDTH = 560;
  const SIDEBAR_DEFAULT_WIDTH = 320;
  let sidebarWidth = $state(SIDEBAR_DEFAULT_WIDTH);
  let sidebarMaxWidth = $state(SIDEBAR_MAX_WIDTH);

  const allOperations = $derived(groups.flatMap((g) => g.operations));
  const navigationOperations = $derived(
    filterGroups(groups, query).flatMap((group) => group.operations)
  );
  const selectedEntry = $derived(
    allOperations.find((e) => e.id === selectedId) || allOperations[0] || null
  );
  const selectedNavigationIndex = $derived(
    selectedEntry
      ? navigationOperations.findIndex((entry) => entry.id === selectedEntry.id)
      : -1
  );
  const previousEntry = $derived(
    selectedNavigationIndex > 0 ? navigationOperations[selectedNavigationIndex - 1] : null
  );
  const nextEntry = $derived(
    selectedNavigationIndex === -1
      ? navigationOperations[0] || null
      : selectedNavigationIndex < navigationOperations.length - 1
        ? navigationOperations[selectedNavigationIndex + 1]
        : null
  );

  function getSidebarMaxWidth() {
    if (typeof window === 'undefined') return SIDEBAR_MAX_WIDTH;
    return Math.max(
      SIDEBAR_MIN_WIDTH,
      Math.min(SIDEBAR_MAX_WIDTH, Math.floor(window.innerWidth * 0.6))
    );
  }

  /** @param {number} width */
  function clampSidebarWidth(width) {
    return Math.min(sidebarMaxWidth, Math.max(SIDEBAR_MIN_WIDTH, width));
  }

  /** @param {number} width @param {boolean} persist */
  function setSidebarWidth(width, persist = true) {
    sidebarWidth = clampSidebarWidth(width);
    if (!persist || typeof localStorage === 'undefined') return;
    try {
      localStorage.setItem(SIDEBAR_WIDTH_KEY, String(sidebarWidth));
    } catch {
      // Storage can be unavailable in private browsing modes.
    }
  }

  function updateSidebarBounds() {
    sidebarMaxWidth = getSidebarMaxWidth();
    sidebarWidth = clampSidebarWidth(sidebarWidth);
  }

  useEventListener(
    () => (typeof window === 'undefined' ? undefined : window),
    'resize',
    updateSidebarBounds
  );

  function versionFromLocation() {
    if (typeof window === 'undefined') return API_SPEC_VERSIONS[0].value;
    const requested = new URLSearchParams(window.location.search).get('version');
    return API_SPEC_VERSIONS.find((entry) => entry.value === requested)?.value
      ?? API_SPEC_VERSIONS[0].value;
  }

  function replaceBrowserLocation() {
    if (typeof window === 'undefined') return;
    const url = new URL(window.location.href);
    if (selectedVersion === API_SPEC_VERSIONS[0].value) url.searchParams.delete('version');
    else url.searchParams.set('version', selectedVersion);
    url.hash = selectedId || '';
    window.history.replaceState(null, '', `${url.pathname}${url.search}${url.hash}`);
  }

  /** @param {string} version @param {boolean} preserveHash */
  async function loadVersion(version, preserveHash = false) {
    const source = API_SPEC_VERSIONS.find((entry) => entry.value === version);
    if (!source) return;
    const sequence = ++loadSequence;
    loading = true;
    loadError = null;
    try {
      const doc = await loadSpec(source.url);
      if (sequence !== loadSequence) return;
      spec = doc;
      groups = groupOperationsByTag(doc);
      selectedId = preserveHash && typeof window !== 'undefined'
        ? window.location.hash.replace(/^#/, '') || null
        : null;
    } catch (err) {
      if (sequence !== loadSequence) return;
      console.error('Failed to load OpenAPI spec', err);
      loadError = err instanceof Error ? err.message : 'Failed to load OpenAPI spec';
    } finally {
      if (sequence === loadSequence) loading = false;
    }
  }

  onMount(() => {
    sidebarMaxWidth = getSidebarMaxWidth();
    try {
      const savedWidth = Number(localStorage.getItem(SIDEBAR_WIDTH_KEY));
      if (Number.isFinite(savedWidth) && savedWidth > 0) {
        sidebarWidth = clampSidebarWidth(savedWidth);
      }
    } catch {
      // Use the default when storage is unavailable.
    }
    selectedVersion = versionFromLocation();
    void loadVersion(selectedVersion, true);
    void import('../features/api-docs/ApiOperation.svelte')
      .then((operationModule) => {
        ApiOperation = operationModule.default;
      })
      .catch((err) => {
        console.error('Failed to load API operation renderer', err);
        loadError = err instanceof Error ? err.message : 'Failed to load API operation renderer';
        loading = false;
      });
  });

  /** @param {string} version */
  function handleVersionChange(version) {
    selectedVersion = version;
    selectedId = null;
    replaceBrowserLocation();
    if (mainPanel) mainPanel.scrollTop = 0;
    void loadVersion(version);
  }

  /** @param {{ id: string }} entry */
  function handleSelect(entry) {
    selectedId = entry.id;
    if (mainPanel) mainPanel.scrollTop = 0;
    // Keep the URL in sync without scrolling the page (we own scroll).
    replaceBrowserLocation();
  }
</script>

<div class="api-docs">
  {#if loading}
    <div class="state">Loading API reference…</div>
  {:else if loadError}
    <div class="state state--error" data-testid="api-docs-error">{loadError}</div>
  {:else if !spec || groups.length === 0}
    <div class="state">No operations are documented in the OpenAPI spec.</div>
  {:else}
    <div
      class="sidebar-pane"
      style:width={`${sidebarWidth}px`}
      data-testid="api-docs-sidebar-pane"
    >
      <ApiDocsSidebar
        {groups}
        bind:query
        selectedId={selectedEntry?.id}
        version={selectedVersion}
        versions={API_SPEC_VERSIONS}
        onselect={handleSelect}
        onversionchange={handleVersionChange}
      />
      <SidebarResizeHandle
        width={sidebarWidth}
        minWidth={SIDEBAR_MIN_WIDTH}
        maxWidth={sidebarMaxWidth}
        defaultWidth={SIDEBAR_DEFAULT_WIDTH}
        label={t('aria.resizeNavigation')}
        title={t('aria.sidebarResizeHint')}
        testId="api-docs-sidebar-resize"
        onresize={(width) => setSidebarWidth(width, false)}
        onresizeend={(width) => setSidebarWidth(width)}
      />
    </div>
    <main class="main" data-testid="api-docs-main" bind:this={mainPanel}>
      {#if selectedEntry}
        <nav class="operation-nav" aria-label="Operation navigation">
          <button
            type="button"
            disabled={!previousEntry}
            aria-label="Previous operation"
            data-testid="api-docs-previous-operation"
            onclick={() => previousEntry && handleSelect(previousEntry)}
          >
            Previous
          </button>
          <span data-testid="api-docs-operation-position">
            {selectedNavigationIndex >= 0
              ? `${selectedNavigationIndex + 1} of ${navigationOperations.length}`
              : `${navigationOperations.length} matching`}
          </span>
          <button
            type="button"
            disabled={!nextEntry}
            aria-label="Next operation"
            data-testid="api-docs-next-operation"
            onclick={() => nextEntry && handleSelect(nextEntry)}
          >
            Next
          </button>
        </nav>
        {#if ApiOperation}
          {@const Operation = ApiOperation}
          {#key selectedEntry.id}
            <Operation {spec} entry={selectedEntry} />
          {/key}
        {/if}
      {/if}
    </main>
  {/if}
</div>

<style>
  .api-docs {
    display: flex;
    height: 100%;
    min-height: 0;
    width: 100%;
    background: var(--ds-surface);
    color: var(--ds-text);
  }
  .main {
    flex: 1 1 auto;
    min-width: 0;
    overflow-y: auto;
    background: var(--ds-surface);
  }
  .sidebar-pane {
    position: relative;
    flex: 0 0 auto;
    min-width: 240px;
    max-width: min(560px, 60vw);
    height: 100%;
  }
  .operation-nav {
    position: sticky;
    z-index: 2;
    top: 0;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 10px;
    min-height: 40px;
    padding: 6px 20px;
    border-bottom: 1px solid var(--ds-border);
    background: color-mix(in srgb, var(--ds-surface) 94%, transparent);
    color: var(--ds-text-subtle);
    font-size: 12px;
    backdrop-filter: blur(8px);
  }
  .operation-nav button {
    border: 1px solid var(--ds-border);
    border-radius: 4px;
    padding: 4px 9px;
    background: var(--ds-surface);
    color: var(--ds-text);
    font-size: 12px;
    cursor: pointer;
  }
  .operation-nav button:hover:not(:disabled) {
    background: var(--ds-surface-hovered);
  }
  .operation-nav button:disabled {
    cursor: default;
    opacity: 0.45;
  }
  .state {
    flex: 1 1 auto;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 48px 24px;
    color: var(--ds-text-subtle);
    font-size: 14px;
  }
  .state--error {
    color: var(--ds-text-danger);
  }
</style>
