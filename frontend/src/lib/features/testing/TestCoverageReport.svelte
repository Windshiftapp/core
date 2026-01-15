<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import Button from '../../components/Button.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import {
    ShieldCheck,
    ShieldX,
    Settings,
    RefreshCw,
    CheckCircle,
    XCircle,
    ChevronDown,
    Link2,
    Package
  } from 'lucide-svelte';
  import { itemTypeIconMap } from '../../utils/icons.js';

  let {
    workspaceId = null,
    hideTitle = false,
    hideHeader = false  // Hide entire header section (for use with external PageHeader)
  } = $props();

  // Data state
  let loading = $state(true);
  let configLoading = $state(false);
  let summaryData = $state(null);
  let requirementsData = $state(null);
  let collections = $state([]);
  let itemTypes = $state([]);
  let config = $state(null);

  // UI state
  let selectedCollectionId = $state(null); // null means "Default"
  let filterCovered = $state('all'); // 'all', 'true', 'false'
  let currentPage = $state(1);
  let pageSize = $state(15);
  let showConfigModal = $state(false);
  let selectedTypeIds = $state([]);

  // Expose state and handlers for external header controls
  export function getCollections() { return collections; }
  export function getSelectedCollectionId() { return selectedCollectionId; }
  export function setSelectedCollectionId(id) {
    selectedCollectionId = id;
    currentPage = 1;
    loadCoverageData();
  }
  export function getFilterCovered() { return filterCovered; }
  export function setFilterCovered(value) {
    filterCovered = value;
    currentPage = 1;
    loadCoverageData();
  }
  export function triggerOpenConfigModal() { openConfigModal(); }

  // Pie chart configuration
  const radius = 48;
  const circumference = 2 * Math.PI * radius;
  const coveredColor = 'var(--ds-status-success-solid, #10b981)';
  const notCoveredColor = 'var(--ds-status-danger-solid, #ef4444)';

  const workspaceTestBase = $derived.by(() =>
    workspaceId ? `/workspaces/${workspaceId}/items` : '/workspaces'
  );

  // Table columns
  const columns = $derived.by(() => [
    {
      key: 'id',
      label: 'ID',
      width: '120px',
      html: true,
      render: (item) =>
        `<a href="${workspaceTestBase}/${item.item_id}" style="color: var(--ds-text-link);" class="hover:underline font-medium">${item.workspace_key}-${item.workspace_item_number}</a>`
    },
    {
      key: 'title',
      label: 'Title',
      render: (item) => item.title || '—'
    },
    {
      key: 'item_type_name',
      label: 'Type',
      width: '140px',
      html: true,
      render: (item) =>
        `<span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-xs font-medium" style="background-color: ${item.item_type_color}20; color: ${item.item_type_color};">${item.item_type_name}</span>`
    },
    {
      key: 'status_name',
      label: 'Status',
      width: '120px',
      render: (item) => item.status_name || '—'
    },
    {
      key: 'is_covered',
      label: 'Coverage',
      width: '100px',
      html: true,
      render: (item) =>
        item.is_covered
          ? `<span class="inline-flex items-center gap-1 text-xs font-medium" style="color: var(--ds-status-success-solid);"><svg class="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"></path></svg>Covered</span>`
          : `<span class="inline-flex items-center gap-1 text-xs font-medium" style="color: var(--ds-status-danger-solid);"><svg class="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd"></path></svg>Not Covered</span>`
    },
    {
      key: 'linked_test_count',
      label: 'Tests',
      width: '80px',
      align: 'text-center',
      html: true,
      render: (item) =>
        `<span class="inline-flex items-center gap-1" style="color: var(--ds-text-subtle);"><svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1"></path></svg>${item.linked_test_count}</span>`
    }
  ]);

  // Computed pie segments
  const pieSegments = $derived.by(() => {
    if (!summaryData || summaryData.total <= 0) return [];

    const segments = [];
    let offset = 0;

    if (summaryData.covered > 0) {
      const fraction = summaryData.covered / summaryData.total;
      const arcLength = fraction * circumference;
      segments.push({
        key: 'covered',
        color: coveredColor,
        dasharray: `${arcLength} ${circumference}`,
        offset: offset
      });
      offset -= arcLength;
    }

    if (summaryData.not_covered > 0) {
      const fraction = summaryData.not_covered / summaryData.total;
      const arcLength = fraction * circumference;
      segments.push({
        key: 'not-covered',
        color: notCoveredColor,
        dasharray: `${arcLength} ${circumference}`,
        offset: offset
      });
    }

    return segments;
  });

  const coverageRate = $derived(summaryData?.coverage_rate ?? 0);

  onMount(() => {
    loadInitialData();
  });

  async function loadInitialData() {
    try {
      loading = true;
      // Load collections and item types in parallel
      const [collectionsRes, itemTypesRes] = await Promise.all([
        api.collections.getAll(),
        api.itemTypes.getAll()
      ]);
      collections = collectionsRes || [];
      itemTypes = itemTypesRes || [];

      // Load coverage data
      await loadCoverageData();
    } catch (error) {
      console.error('Failed to load initial data:', error);
    } finally {
      loading = false;
    }
  }

  async function loadCoverageData() {
    try {
      const id = selectedCollectionId || 'default';

      // Load config first
      try {
        config = await api.tests.coverage.getConfig(id, workspaceId);
        selectedTypeIds = config?.requirement_item_type_ids || [];
      } catch (e) {
        // No config exists yet
        config = null;
        selectedTypeIds = [];
      }

      // Load summary and requirements
      const [summary, requirements] = await Promise.all([
        api.tests.coverage.getSummary(id, workspaceId),
        api.tests.coverage.getRequirements(id, workspaceId, {
          page: currentPage,
          limit: pageSize,
          covered: filterCovered === 'all' ? undefined : filterCovered
        })
      ]);

      summaryData = summary;
      requirementsData = requirements;
    } catch (error) {
      console.error('Failed to load coverage data:', error);
      summaryData = null;
      requirementsData = null;
    }
  }

  async function handleCollectionChange(event) {
    const value = event.target.value;
    selectedCollectionId = value === '' ? null : parseInt(value, 10);
    currentPage = 1;
    loading = true;
    await loadCoverageData();
    loading = false;
  }

  async function handleFilterChange(event) {
    filterCovered = event.target.value;
    currentPage = 1;
    loading = true;
    await loadCoverageData();
    loading = false;
  }

  async function handlePageChange(page) {
    currentPage = page;
    loading = true;
    await loadCoverageData();
    loading = false;
  }

  function openConfigModal() {
    selectedTypeIds = config?.requirement_item_type_ids || [];
    showConfigModal = true;
  }

  function closeConfigModal() {
    showConfigModal = false;
  }

  function toggleItemType(typeId) {
    if (selectedTypeIds.includes(typeId)) {
      selectedTypeIds = selectedTypeIds.filter((id) => id !== typeId);
    } else {
      selectedTypeIds = [...selectedTypeIds, typeId];
    }
  }

  async function saveConfig() {
    try {
      configLoading = true;
      const id = selectedCollectionId || 'default';
      const configData = { requirement_item_type_ids: selectedTypeIds };

      if (config?.id) {
        // Update existing config
        await api.tests.coverage.updateConfig(
          selectedCollectionId || 'default',
          config.id,
          configData
        );
      } else {
        // Create new config
        await api.tests.coverage.createConfig(id, configData, workspaceId);
      }

      showConfigModal = false;
      loading = true;
      await loadCoverageData();
      loading = false;
    } catch (error) {
      console.error('Failed to save config:', error);
      alert('Failed to save configuration');
    } finally {
      configLoading = false;
    }
  }
</script>

<div class="coverage-report">
  <!-- Header with controls -->
  {#if !hideHeader}
  <div class="report-header">
    {#if !hideTitle}
      <div class="header-left">
        <div class="flex items-center gap-2">
          <ShieldCheck class="w-5 h-5" style="color: var(--ds-text-subtle);" />
          <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
            Requirements Coverage
          </h3>
        </div>
        <p class="text-sm mt-1" style="color: var(--ds-text-subtle);">
          Track which requirements have linked test cases
        </p>
      </div>
    {/if}
    <div class="header-controls">
      <!-- Collection selector -->
      <div class="control-group">
        <label class="control-label" for="collection-select">Collection</label>
        <div class="select-wrapper">
          <select
            id="collection-select"
            class="select-input"
            value={selectedCollectionId ?? ''}
            onchange={handleCollectionChange}
          >
            <option value="">Default</option>
            {#each collections as collection (collection.id)}
              <option value={collection.id}>{collection.name}</option>
            {/each}
          </select>
          <ChevronDown class="select-icon" />
        </div>
      </div>

      <!-- Filter -->
      <div class="control-group">
        <label class="control-label" for="filter-select">Filter</label>
        <div class="select-wrapper">
          <select
            id="filter-select"
            class="select-input"
            value={filterCovered}
            onchange={handleFilterChange}
          >
            <option value="all">All Requirements</option>
            <option value="true">Covered Only</option>
            <option value="false">Not Covered Only</option>
          </select>
          <ChevronDown class="select-icon" />
        </div>
      </div>

      <!-- Configure button -->
      <Button variant="default" onclick={openConfigModal}>
        <Settings class="w-4 h-4" />
        Configure
      </Button>
    </div>
  </div>
  {/if}

  <!-- Content -->
  {#if loading}
    <div class="loading-state">
      <RefreshCw class="w-8 h-8 animate-spin" style="color: var(--ds-text-subtle);" />
      <p style="color: var(--ds-text-subtle);">Loading coverage data...</p>
    </div>
  {:else if !config || selectedTypeIds.length === 0}
    <div class="empty-config-state">
      <ShieldX class="w-12 h-12" style="color: var(--ds-text-subtle); opacity: 0.5;" />
      <p class="empty-title">No requirement types configured</p>
      <p class="empty-copy">
        Select which item types should be tracked as requirements to see coverage data.
      </p>
      <Button variant="primary" onclick={openConfigModal}>
        <Settings class="w-4 h-4" />
        Configure Requirements
      </Button>
    </div>
  {:else if !summaryData || summaryData.total === 0}
    <EmptyState
      icon={ShieldX}
      title="No requirements found"
      description="No items matching the configured requirement types were found."
    />
  {:else}
    <div class="coverage-content">
      <!-- Summary row -->
      <div class="summary-row">
        <!-- Pie chart -->
        <div class="pie-section">
          <svg viewBox="0 0 140 140" role="img" aria-label="Coverage breakdown">
            <circle
              cx="70"
              cy="70"
              r={radius}
              fill="transparent"
              stroke="var(--ds-border)"
              stroke-width="16"
            />
            {#each pieSegments as segment (segment.key)}
              <circle
                cx="70"
                cy="70"
                r={radius}
                fill="transparent"
                stroke={segment.color}
                stroke-width="16"
                stroke-linecap="butt"
                stroke-dasharray={segment.dasharray}
                stroke-dashoffset={segment.offset}
                transform="rotate(-90 70 70)"
              />
            {/each}
            <text class="pie-percent" x="70" y="68">{Math.round(coverageRate)}%</text>
            <text class="pie-label" x="70" y="84">covered</text>
          </svg>
        </div>

        <!-- Stats cards -->
        <div class="stats-cards">
          <div class="stat-card">
            <div class="stat-header">
              <span class="stat-dot total"></span>
              <span class="stat-title">Total Requirements</span>
            </div>
            <div class="stat-value">{summaryData.total}</div>
          </div>
          <div class="stat-card">
            <div class="stat-header">
              <span class="stat-dot covered"></span>
              <span class="stat-title">Covered</span>
            </div>
            <div class="stat-value">{summaryData.covered}</div>
          </div>
          <div class="stat-card">
            <div class="stat-header">
              <span class="stat-dot not-covered"></span>
              <span class="stat-title">Not Covered</span>
            </div>
            <div class="stat-value">{summaryData.not_covered}</div>
          </div>
        </div>
      </div>

      <!-- Requirements table -->
      <div class="table-section">
        <DataTable
          {columns}
          data={requirementsData?.items || []}
          keyField="item_id"
          emptyMessage="No requirements found"
          emptyIcon={ShieldX}
          pagination={true}
          pageSize={pageSize}
          currentPage={currentPage}
          totalItems={requirementsData?.pagination?.total || 0}
          onPageChange={handlePageChange}
        />
      </div>
    </div>
  {/if}
</div>

<!-- Configuration Modal -->
<Modal isOpen={showConfigModal} onclose={closeConfigModal} maxWidth="max-w-xl">
  <div class="p-6">
    <h3 class="text-xl font-semibold mb-2" style="color: var(--ds-text);">
      Configure Requirement Types
    </h3>
    <p class="text-sm mb-6" style="color: var(--ds-text-subtle);">
      Select the item types that should be tracked as requirements for coverage analysis.
    </p>

    <div class="type-selection">
      {#if itemTypes.length === 0}
        <p class="text-sm" style="color: var(--ds-text-subtle);">No item types available.</p>
      {:else}
        <div class="type-grid">
          {#each itemTypes as type (type.id)}
            <button
              class="type-option"
              class:selected={selectedTypeIds.includes(type.id)}
              onclick={() => toggleItemType(type.id)}
            >
              <div class="type-icon" style="background-color: {type.color}20; color: {type.color};">
                {#if type.icon && itemTypeIconMap[type.icon]}
                  <svelte:component this={itemTypeIconMap[type.icon]} size={20} />
                {:else}
                  <svelte:component this={Package} size={20} />
                {/if}
              </div>
              <span class="type-name">{type.name}</span>
              {#if selectedTypeIds.includes(type.id)}
                <CheckCircle class="type-check" />
              {/if}
            </button>
          {/each}
        </div>
      {/if}
    </div>

    <div class="modal-footer">
      <Button variant="default" onclick={closeConfigModal}>Cancel</Button>
      <Button variant="primary" onclick={saveConfig} disabled={configLoading}>
        {configLoading ? 'Saving...' : 'Save Configuration'}
      </Button>
    </div>
  </div>
</Modal>

<style>
  .coverage-report {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
  }

  .report-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 1rem;
    padding: 1rem 1.25rem;
    border-bottom: 1px solid var(--ds-border);
  }

  .header-controls {
    display: flex;
    align-items: flex-end;
    gap: 1rem;
  }

  .control-group {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .control-label {
    font-size: 0.75rem;
    font-weight: 500;
    color: var(--ds-text-subtle);
  }

  .select-wrapper {
    position: relative;
  }

  .select-input {
    appearance: none;
    padding: 0.5rem 2rem 0.5rem 0.75rem;
    font-size: 0.875rem;
    border: 1px solid var(--ds-border);
    border-radius: 0.375rem;
    background-color: var(--ds-background-input);
    color: var(--ds-text);
    cursor: pointer;
    min-width: 140px;
  }

  .select-input:focus {
    outline: none;
    border-color: var(--ds-accent);
  }

  .select-wrapper :global(.select-icon) {
    position: absolute;
    right: 0.5rem;
    top: 50%;
    transform: translateY(-50%);
    width: 1rem;
    height: 1rem;
    color: var(--ds-text-subtle);
    pointer-events: none;
  }

  .loading-state,
  .empty-config-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 4rem 2rem;
    text-align: center;
    gap: 0.75rem;
  }

  .empty-title {
    font-weight: 600;
    color: var(--ds-text);
    margin-top: 0.5rem;
  }

  .empty-copy {
    font-size: 0.875rem;
    color: var(--ds-text-subtle);
    max-width: 300px;
    margin-bottom: 0.5rem;
  }

  .coverage-content {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
  }

  .summary-row {
    display: flex;
    align-items: center;
    gap: 2rem;
    padding: 0 1.25rem;
  }

  .pie-section {
    width: 140px;
    height: 140px;
    flex-shrink: 0;
  }

  .pie-section svg {
    width: 100%;
    height: 100%;
  }

  .pie-section :global(.pie-percent) {
    font-size: 1.5rem;
    font-weight: 700;
    fill: var(--ds-text);
    text-anchor: middle;
    dominant-baseline: central;
  }

  .pie-section :global(.pie-label) {
    font-size: 0.75rem;
    fill: var(--ds-text-subtle);
    text-anchor: middle;
    dominant-baseline: central;
  }

  .stats-cards {
    display: flex;
    gap: 1rem;
    flex: 1;
  }

  .stat-card {
    flex: 1;
    padding: 1rem;
    border-radius: 0.5rem;
    background-color: var(--ds-surface);
    border: 1px solid var(--ds-border);
  }

  .stat-header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.5rem;
  }

  .stat-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
  }

  .stat-dot.total {
    background-color: var(--ds-text-subtle);
  }

  .stat-dot.covered {
    background-color: var(--ds-status-success-solid, #10b981);
  }

  .stat-dot.not-covered {
    background-color: var(--ds-status-danger-solid, #ef4444);
  }

  .stat-title {
    font-size: 0.75rem;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--ds-text-subtle);
  }

  .stat-value {
    font-size: 1.5rem;
    font-weight: 700;
    color: var(--ds-text);
  }

  .table-section {
    padding: 0 1.25rem 1.25rem;
  }

  /* Modal styles */
  .type-selection {
    margin-bottom: 1.5rem;
  }

  .type-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
    gap: 0.75rem;
  }

  .type-option {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.5rem;
    padding: 1rem;
    border: 2px solid var(--ds-border);
    border-radius: 0.5rem;
    background-color: var(--ds-surface);
    cursor: pointer;
    position: relative;
    transition: all 0.15s ease;
  }

  .type-option:hover {
    border-color: var(--ds-border-bold);
  }

  .type-option.selected {
    border-color: var(--ds-accent);
    background-color: var(--ds-accent-subtle, rgba(59, 130, 246, 0.05));
  }

  .type-icon {
    width: 40px;
    height: 40px;
    border-radius: 0.375rem;
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 600;
  }

  .type-name {
    font-size: 0.875rem;
    font-weight: 500;
    color: var(--ds-text);
    text-align: center;
  }

  .type-option :global(.type-check) {
    position: absolute;
    top: 0.5rem;
    right: 0.5rem;
    width: 1rem;
    height: 1rem;
    color: var(--ds-accent);
  }

  .modal-footer {
    display: flex;
    justify-content: flex-end;
    gap: 0.75rem;
    padding-top: 1rem;
    border-top: 1px solid var(--ds-border);
  }
</style>
