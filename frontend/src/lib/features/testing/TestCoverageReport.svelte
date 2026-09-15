<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import Button from '../../components/Button.svelte';
  import Select from '../../components/Select.svelte';
  import DataTable from '../../components/DataTable.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import Card from '../../components/Card.svelte';
  import StatCard from '../../components/StatCard.svelte';
  import StateDisplay from '../../components/StateDisplay.svelte';
  import FormField from '../../components/FormField.svelte';
  import PieChartSegments from '../../components/PieChartSegments.svelte';
  import Modal from '../../dialogs/Modal.svelte';
  import ModalHeader from '../../dialogs/ModalHeader.svelte';
  import DialogFooter from '../../dialogs/DialogFooter.svelte';
  import SectionHeader from '../../layout/SectionHeader.svelte';
  import { escapeHtml } from '../../utils/sanitize.ts';
  import { buildCoveragePieSegments } from '../../utils/pieChart.js';
  import {
    IconShieldX,
    IconSettings,
    IconCircleCheck,
    IconCircleX,
    IconLink
  } from '@tabler/icons-svelte-runes';
  import ItemTypeIcon from '../../components/ItemTypeIcon.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import { REQUIREMENT_TYPES } from '../requirements/requirementTypes.js';
  import { requirementStatusLozenge } from '../requirements/requirementStatuses.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';

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
  let selectedRequirementTypes = $state([]);

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
  const coveredColor = 'var(--ds-status-success-solid, #10b981)';
  const notCoveredColor = 'var(--ds-status-danger-solid, #ef4444)';

  const isLegacyMode = $derived(
    (config?.requirement_item_type_ids?.length ?? 0) > 0 && (config?.requirement_types?.length ?? 0) === 0
  );
  const isPageMode = $derived((config?.requirement_types?.length ?? 0) > 0);
  const isConfigured = $derived(
    isPageMode || (config?.requirement_item_type_ids?.length ?? 0) > 0
  );

  const workspaceTestBase = $derived.by(() =>
    workspaceId ? `/workspaces/${workspaceId}/items` : '/workspaces'
  );
  const workspaceRequirementsBase = $derived.by(() =>
    workspaceId ? `/workspaces/${workspaceId}/requirements` : '/workspaces'
  );

  // Table columns
  const columns = $derived.by(() => {
    if (isPageMode) {
      return [
        {
          key: 'requirement_key',
          label: t('common.id'),
          width: '140px',
          html: true,
          render: (item) =>
            `<a href="${workspaceRequirementsBase}/${item.requirement_number}" style="color: var(--ds-text-link);" class="hover:underline font-medium">${escapeHtml(item.requirement_key || '')}</a>`
        },
        {
          key: 'title',
          label: t('common.title'),
          render: (item) => item.title || '—'
        },
        {
          key: 'requirement_type',
          label: t('common.type'),
          width: '180px',
          render: (item) => t(`requirements.type.${item.requirement_type}`)
        },
        {
          key: 'status',
          label: t('common.status'),
          width: '120px',
          slot: 'status'
        },
        {
          key: 'is_covered',
          label: t('testing.coverage'),
          width: '100px',
          slot: 'is_covered'
        },
        {
          key: 'linked_test_count',
          label: t('testing.tests'),
          width: '80px',
          align: 'text-center',
          render: (item) => String(item.linked_test_count ?? 0)
        }
      ];
    }
    return [
      {
        key: 'id',
        label: t('common.id'),
        width: '120px',
        html: true,
        render: (item) =>
          `<a href="${workspaceTestBase}/${item.item_id}" style="color: var(--ds-text-link);" class="hover:underline font-medium">${escapeHtml(item.workspace_key)}-${escapeHtml(item.workspace_item_number)}</a>`
      },
      {
        key: 'title',
        label: t('common.title'),
        render: (item) => item.title || '—'
      },
      {
        key: 'item_type_name',
        label: t('common.type'),
        width: '140px',
        html: true,
        render: (item) =>
          `<span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-xs font-medium" style="background-color: ${escapeHtml(item.item_type_color)}20; color: ${escapeHtml(item.item_type_color)};">${escapeHtml(item.item_type_name)}</span>`
      },
      {
        key: 'status_name',
        label: t('common.status'),
        width: '120px',
        render: (item) => item.status_name || '—'
      },
      {
        key: 'is_covered',
        label: t('testing.coverage'),
        width: '100px',
        html: true,
        render: (item) =>
          item.is_covered
            ? `<span class="inline-flex items-center gap-1 text-xs font-medium" style="color: var(--ds-status-success-solid);"><svg class="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"></path></svg>${t('testing.covered')}</span>`
            : `<span class="inline-flex items-center gap-1 text-xs font-medium" style="color: var(--ds-status-danger-solid);"><svg class="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd"></path></svg>${t('testing.notCovered')}</span>`
      },
      {
        key: 'linked_test_count',
        label: t('testing.tests'),
        width: '80px',
        align: 'text-center',
        html: true,
        render: (item) =>
          `<span class="inline-flex items-center gap-1" style="color: var(--ds-text-subtle);"><svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1"></path></svg>${item.linked_test_count}</span>`
      }
    ];
  });

  const tableKeyField = $derived(isPageMode ? 'requirement_number' : 'item_id');

  // Computed pie segments
  const pieSegments = $derived.by(() => {
    if (!summaryData || summaryData.total <= 0) return [];
    return buildCoveragePieSegments(
      summaryData.covered, summaryData.not_covered, summaryData.total,
      coveredColor, notCoveredColor, radius
    );
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
        selectedRequirementTypes = config?.requirement_types || [];
      } catch (e) {
        // No config exists yet
        config = null;
        selectedTypeIds = [];
        selectedRequirementTypes = [];
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
    selectedRequirementTypes = config?.requirement_types || [];
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

  function toggleRequirementType(typeValue) {
    if (selectedRequirementTypes.includes(typeValue)) {
      selectedRequirementTypes = selectedRequirementTypes.filter((value) => value !== typeValue);
    } else {
      selectedRequirementTypes = [...selectedRequirementTypes, typeValue];
    }
  }

  async function saveConfig(requirementTypesOverride = null) {
    try {
      configLoading = true;
      const id = selectedCollectionId || 'default';
      const types = requirementTypesOverride ?? selectedRequirementTypes;
      const configData = isLegacyMode && requirementTypesOverride == null
        ? { requirement_item_type_ids: selectedTypeIds }
        : { requirement_types: types };

      if (config?.id) {
        // Update existing config
        await api.tests.coverage.updateConfig(
          selectedCollectionId || 'default',
          config.id,
          configData,
          workspaceId
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
      errorToast(t('testing.failedToSaveConfig'));
    } finally {
      configLoading = false;
    }
  }

  async function migrateToRegistry() {
    await saveConfig([...REQUIREMENT_TYPES]);
  }
</script>

<div class="coverage-report">
  <!-- Header with controls -->
  {#if !hideHeader}
  <Card variant="flat" padding="spacious">
    {#if !hideTitle}
      <SectionHeader
        title={t('testing.requirementsCoverage')}
        subtitle={t('testing.requirementsCoverageSubtitle')}
      />
    {/if}
    <div class="flex flex-wrap items-end gap-3">
      <!-- Collection selector -->
      <FormField id="collection-select" label={t('collections.collection')} class="mb-0 min-w-48">
        <Select
          id="collection-select"
          options={[{ value: '', label: 'Default' }, ...collections.map(c => ({ value: c.id, label: c.name }))]}
          value={selectedCollectionId ?? ''}
          onchange={(v) => handleCollectionChange({ target: { value: v } })}
        />
      </FormField>

      <!-- Filter -->
      <FormField id="filter-select" label={t('common.filter')} class="mb-0 min-w-48">
        <Select
          id="filter-select"
          options={[
            { value: 'all', label: t('testing.allRequirements') },
            { value: 'true', label: t('testing.coveredOnly') },
            { value: 'false', label: t('testing.notCoveredOnly') },
          ]}
          value={filterCovered}
          onchange={(v) => handleFilterChange({ target: { value: v } })}
        />
      </FormField>

      <!-- Configure button -->
      <Button variant="default" onclick={openConfigModal}>
        <IconSettings class="w-4 h-4" />
        {t('testing.configureRequirements')}
      </Button>
    </div>
  </Card>
  {/if}

  <!-- Content -->
  {#if loading}
    <StateDisplay type="loading" message={t('testing.loadingCoverageData')} size="lg" />
  {:else if !config || !isConfigured}
    <EmptyState
      icon={IconShieldX}
      title={t('testing.noRequirementTypesConfigured')}
      description={t('testing.selectRequirementTypesForCoverage')}
    >
      {#snippet action()}
        <Button variant="primary" onclick={openConfigModal}>
          <IconSettings class="w-4 h-4" />
          {t('testing.configureRequirements')}
        </Button>
      {/snippet}
    </EmptyState>
  {:else if !summaryData || summaryData.total === 0}
    <EmptyState
      icon={IconShieldX}
      title={t('testing.noRequirementsFound')}
      description={t('testing.noItemsMatchingRequirements')}
    />
  {:else}
    {#if isLegacyMode}
      <Card variant="flat" padding="default" class="legacy-banner">
        <div class="legacy-banner__content">
          <div>
            <h3>{t('testing.legacyCoverageConfigTitle')}</h3>
            <p>{t('testing.legacyCoverageConfigDescription')}</p>
          </div>
          <Button variant="secondary" onclick={migrateToRegistry} disabled={configLoading}>
            {t('testing.migrateCoverageToRegistry')}
          </Button>
        </div>
      </Card>
    {/if}
    <div class="coverage-content">
      <!-- Summary row -->
      <div class="grid grid-cols-1 gap-4 lg:grid-cols-[180px_1fr]">
        <!-- Pie chart -->
        <Card variant="outlined" padding="default" class="flex items-center justify-center">
          <div class="pie-section">
            <svg viewBox="0 0 140 140" role="img" aria-label="Coverage breakdown">
              <PieChartSegments segments={pieSegments} {radius} />
              <text class="pie-percent" x="70" y="68">{Math.round(coverageRate)}%</text>
              <text class="pie-label" x="70" y="84">{t('testing.covered').toLowerCase()}</text>
            </svg>
          </div>
        </Card>

        <!-- Stats cards -->
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <StatCard icon={IconLink} label={t('testing.totalRequirements')} value={summaryData.total} color="blue" />
          <StatCard icon={IconCircleCheck} label={t('testing.covered')} value={summaryData.covered} color="green" />
          <StatCard icon={IconCircleX} label={t('testing.notCovered')} value={summaryData.not_covered} color="orange" />
        </div>
      </div>

      <!-- Requirements table -->
      <div class="table-section">
        <DataTable
          {columns}
          data={requirementsData?.data || []}
          keyField={tableKeyField}
          emptyMessage={t('testing.noRequirementsFound')}
          emptyIcon={IconShieldX}
          pagination={true}
          pageSize={pageSize}
          currentPage={currentPage}
          totalItems={requirementsData?.pagination?.total_items || 0}
          onPageChange={handlePageChange}
        >
          {#snippet status(item)}
            {#if item.status}
              <Lozenge color={requirementStatusLozenge(item.status)}>
                {t(`requirements.status.${item.status}`)}
              </Lozenge>
            {:else}
              —
            {/if}
          {/snippet}
          {#snippet is_covered(item)}
            {#if item.is_covered}
              <Lozenge color="green">{t('testing.covered')}</Lozenge>
            {:else}
              <Lozenge color="red">{t('testing.notCovered')}</Lozenge>
            {/if}
          {/snippet}
        </DataTable>
      </div>
    </div>
  {/if}
</div>

<!-- Configuration Modal -->
<Modal
  isOpen={showConfigModal}
  onclose={closeConfigModal}
  onSubmit={saveConfig}
  submitDisabled={configLoading}
  maxWidth="max-w-xl"
>
  <ModalHeader
    title={t('testing.configureRequirementTypes')}
    subtitle={t('testing.selectRequirementTypesForCoverageAnalysis')}
    onClose={closeConfigModal}
  />
  <div class="p-6">
    <div class="type-selection">
      {#if isLegacyMode}
        {#if itemTypes.length === 0}
          <p class="text-sm" style="color: var(--ds-text-subtle);">{t('testing.noItemTypesAvailable')}</p>
        {:else}
          <div class="type-grid">
            {#each itemTypes as type (type.id)}
              <button
                class="type-option"
                class:selected={selectedTypeIds.includes(type.id)}
                onclick={() => toggleItemType(type.id)}
              >
                <ItemTypeIcon itemType={type} />
                <span class="type-name">{type.name}</span>
                {#if selectedTypeIds.includes(type.id)}
                  <IconCircleCheck class="type-check" />
                {/if}
              </button>
            {/each}
          </div>
        {/if}
      {:else}
        <div class="type-grid">
          {#each REQUIREMENT_TYPES as typeValue (typeValue)}
            <button
              class="type-option"
              class:selected={selectedRequirementTypes.includes(typeValue)}
              onclick={() => toggleRequirementType(typeValue)}
            >
              <span class="type-name">{t(`requirements.type.${typeValue}`)}</span>
              {#if selectedRequirementTypes.includes(typeValue)}
                <IconCircleCheck class="type-check" />
              {/if}
            </button>
          {/each}
        </div>
      {/if}
    </div>

  </div>
  <DialogFooter
    cancelLabel={t('common.cancel')}
    confirmLabel={t('testing.saveConfiguration')}
    loadingLabel={t('common.saving')}
    onCancel={closeConfigModal}
    onConfirm={saveConfig}
    loading={configLoading}
    showKeyboardHint={true}
  />
</Modal>

<style>
  .coverage-report {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
  }

  .coverage-content {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
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

  .table-section {
    padding-bottom: 1.25rem;
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

  .legacy-banner__content {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
  }

  .legacy-banner__content h3 {
    margin: 0 0 0.25rem;
    font-size: 0.9375rem;
    font-weight: 600;
  }

  .legacy-banner__content p {
    margin: 0;
    font-size: 0.875rem;
    color: var(--ds-text-subtle);
  }

</style>
