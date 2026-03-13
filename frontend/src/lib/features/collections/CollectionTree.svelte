<script>
  import { onMount, untrack } from 'svelte';
  import { api } from '../../api.js';
  import { collectionStore } from '../../stores/collectionContext.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { useGradientStyles, loadWorkspaceGradient } from '../../stores/workspaceGradient.svelte.js';
  import { ChevronRight, ChevronDown, GitBranch, Circle, AlertCircle, Calendar, FileCheck, Minus } from 'lucide-svelte';
  import { itemTypeIconMap } from '../../utils/icons.js';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import ItemKey from '../items/ItemKey.svelte';
  import ColorDot from '../../components/ColorDot.svelte';
  import LinkComponent from '../../components/Link.svelte';
  import TestCaseViewModal from '../../dialogs/TestCaseViewModal.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import TableHeaderBar from '../../components/TableHeaderBar.svelte';
  import { formatDate } from '../../utils/dateFormatter.js';
  import { moduleSettings } from '../../stores/moduleSettings.js';
  import { itemTestCaseLinksStore } from '../../stores/index.js';

  let { workspaceId, collectionId = null } = $props();

  let workspace = $state(null);
  let allItems = $state([]);
  let itemTypes = $state([]);
  let statuses = $state([]);
  let statusCategories = $state([]);
  let priorities = $state([]);
  let loading = $state(true);
  let currentCollectionName = $state('Default');
  let expandedItems = $state(new Set()); // Track which items are expanded
  
  // Pagination state
  let currentPage = $state(1);
  let itemsPerPage = $state(50);

  // Test case toggle state
  let showTestCases = $state(false);
  let loadingTestCases = $state(false);

  // Test case modal state
  let showTestCaseModal = $state(false);
  let selectedTestCaseId = $state(null);

  // Centralized gradient styling
  const styles = useGradientStyles();

  // On mount, request a large page to get full hierarchy
  onMount(async () => {
    // Load test case toggle preference from localStorage
    const saved = localStorage.getItem('collectionTree_showTestCases');
    if (saved !== null) {
      showTestCases = saved === 'true';
    }

    await loadWorkspaceGradient(workspaceId);
    // Request a large page for tree (needs full hierarchy)
    await collectionStore.setItemsPage(1, 500);
    await loadData();
  });

  // Sync items from central store
  $effect(() => {
    if (!collectionStore.loading && collectionStore.items.length >= 0) {
      currentCollectionName = collectionStore.collectionName;
      const sorted = [...collectionStore.items].sort((a, b) => a.level - b.level || a.id - b.id);
      // untrack to avoid tracking reads of allItems/expandedItems via getRootItems/hasChildren
      untrack(() => {
        allItems = sorted;
        const rootItems = getRootItems();
        expandedItems = new Set(rootItems.filter(i => hasChildren(i.id)).map(i => i.id));
      });
    }
  });

  async function loadData() {
    loading = true;
    if (workspaceId) {
      itemTestCaseLinksStore.initialize(workspaceId);

      await Promise.all([
        loadWorkspace(),
        loadItemTypes(),
        loadStatusData(),
        loadPriorities()
      ]);

      if (showTestCases) {
        await loadPendingTestCases();
      }
    }
    loading = false;
  }

  $effect(() => {
    if (showTestCases && allItems.length > 0) {
      loadPendingTestCases();
    }
  });

  async function loadWorkspace() {
    try {
      workspace = await api.workspaces.get(workspaceId);
    } catch (error) {
      console.error('[CollectionTree] Failed to load workspace:', error);
    }
  }

  async function loadItemTypes() {
    try {
      itemTypes = await api.itemTypes.getAll();
    } catch (error) {
      console.error('[CollectionTree] Failed to load item types:', error);
      itemTypes = [];
    }
  }

  async function loadStatusData() {
    try {
      const [statusesData, statusCategoriesData] = await Promise.all([
        api.workspaces.getStatuses(workspaceId),
        api.statusCategories.getAll()
      ]);
      statuses = statusesData || [];
      statusCategories = statusCategoriesData || [];
    } catch (error) {
      console.error('[CollectionTree] Failed to load status data:', error);
      statuses = [];
      statusCategories = [];
    }
  }

  async function loadPriorities() {
    try {
      priorities = await api.priorities.getAll();
    } catch (error) {
      console.error('[CollectionTree] Failed to load priorities:', error);
      priorities = [];
    }
  }

  function getItemsByParent(parentId) {
    return allItems.filter(item => item.parent_id === parentId);
  }

  function getRootItems() {
    return allItems.filter(item => item.parent_id === null);
  }

  function hasChildren(itemId) {
    return allItems.some(item => item.parent_id === itemId);
  }

  function toggleExpanded(itemId) {
    if (expandedItems.has(itemId)) {
      expandedItems.delete(itemId);
    } else {
      expandedItems.add(itemId);
    }
    expandedItems = new Set(expandedItems);
  }

  function isExpanded(itemId) {
    return expandedItems.has(itemId);
  }

  function collapseAll() {
    expandedItems.clear();
    expandedItems = new Set(expandedItems);
  }

  function expandAll() {
    // Expand all items that have children
    allItems.forEach(item => {
      if (hasChildren(item.id)) {
        expandedItems.add(item.id);
      }
    });
    expandedItems = new Set(expandedItems);
  }

  function toggleExpandCollapse() {
    if (expandedItems.size === 0) {
      expandAll();
    } else {
      collapseAll();
    }
  }

  // Load test cases linked to items
  async function loadTestCasesForItems(itemIds) {
    if (!itemIds || itemIds.length === 0) return;
    loadingTestCases = true;
    try {
      await itemTestCaseLinksStore.loadForItems(itemIds);
    } catch (error) {
      console.error('Failed to load test cases:', error);
    } finally {
      loadingTestCases = false;
    }
  }

  // Toggle test case visibility
  async function toggleShowTestCases() {
    showTestCases = !showTestCases;

    // Save preference to localStorage
    localStorage.setItem('collectionTree_showTestCases', showTestCases.toString());

    if (showTestCases) {
      await loadPendingTestCases();
    }
  }

  async function loadPendingTestCases() {
    if (!showTestCases || allItems.length === 0) return;
    if (loadingTestCases) return;
    const visibleItemIds = allItems.map(item => item.id);
    const missingItemIds = visibleItemIds.filter(id => itemTestCaseLinksStore.get(id) === null);
    if (missingItemIds.length === 0) return;
    await loadTestCasesForItems(missingItemIds);
  }

  // Handle test case click to open modal
  function handleTestCaseClick(event, testCaseId) {
    event.preventDefault();
    selectedTestCaseId = testCaseId;
    showTestCaseModal = true;
  }

  function getPaginatedRootItems() {
    const rootItems = getRootItems();
    const startIndex = (currentPage - 1) * itemsPerPage;
    const endIndex = startIndex + itemsPerPage;
    
    return rootItems.slice(startIndex, endIndex);
  }

  function getTotalRootItems() {
    return getRootItems().length;
  }

  function getTotalPages() {
    const total = getTotalRootItems();
    return total === 0 ? 1 : Math.ceil(total / itemsPerPage);
  }

  let paginationInfo = $derived({
    start: (currentPage - 1) * itemsPerPage + 1,
    end: Math.min(currentPage * itemsPerPage, getTotalRootItems()),
    total: getTotalRootItems()
  });

  function goToPage(page) {
    currentPage = page;
  }

  function getIndentLevel(level) {
    return `${level * 24}px`;
  }

  function getItemTypeInfo(item) {
    if (!item.item_type_id || !itemTypes.length) {
      // Fallback to hierarchy level based icons
      const fallbackIndicators = [
        { icon: GitBranch, color: 'text-purple-600', label: 'Epic' },      // Level 0
        { icon: Circle, color: 'text-blue-600', label: 'Feature' },        // Level 1  
        { icon: Circle, color: 'text-green-600', label: 'Story' },         // Level 2
        { icon: Circle, color: 'text-orange-600', label: 'Task' },         // Level 3
        { icon: Circle, color: 'text-gray-600', label: 'Subtask' }         // Level 4+
      ];
      return fallbackIndicators[Math.min(item.level || 0, fallbackIndicators.length - 1)];
    }

    // Find the actual item type
    const itemType = itemTypes.find(type => type.id === item.item_type_id);
    if (itemType) {
      return {
        icon: itemTypeIconMap[itemType.icon] || itemTypeIconMap.FileText,
        color: itemType.color, // Use the actual hex color
        label: itemType.name
      };
    }

    // Fallback if item type not found
    return { icon: Circle, color: 'text-gray-600', label: 'Unknown' };
  }

  function renderTreeItems(parentId = null, level = 0, result = [], rootItems = null) {
    // For root level, use paginated items, otherwise get all children
    const items = parentId === null ? (rootItems || getPaginatedRootItems()) : getItemsByParent(parentId);

    for (const item of items) {
      // Add the current item with its level and children info
      result.push({
        ...item,
        level,
        hasChildren: hasChildren(item.id),
        isTestCase: false
      });

      // If showTestCases is enabled, add test cases after this item
      if (showTestCases && itemTestCaseLinksStore.get(item.id) !== null) {
        const testCases = itemTestCaseLinksStore.get(item.id);
        testCases.forEach(testCase => {
          result.push({
            id: testCase.id,
            title: testCase.title,
            level: level + 1,
            hasChildren: false,
            isTestCase: true,
            testCaseData: testCase
          });
        });
      }

      // Show children if the current item is expanded
      if (isExpanded(item.id)) {
        renderTreeItems(item.id, level + 1, result);
      }
    }

    return result;
  }

  // Rebuild tree when items, expanded state, test cases, or test case toggle changes
  // Svelte 5 automatically tracks dependencies accessed inside renderTreeItems()
  let treeData = $derived(
    allItems.length > 0
      ? renderTreeItems(null, 0, [], getPaginatedRootItems())
      : []
  );
</script>

{#snippet paginationControls()}
  <div class="flex items-center gap-2">
    <button
      class="px-3 py-2 text-sm border rounded disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      style="{styles.glassStyle(12)} {styles.glassTextStyle}"
      onclick={() => goToPage(currentPage - 1)}
      disabled={currentPage === 1}
    >
      {t('common.previous')}
    </button>

    <span class="px-4 py-2 text-sm" style="{styles.textStyle}">
      {t('collectionTree.pageOfTotal', { current: currentPage, total: getTotalPages() })}
    </span>

    <button
      class="px-3 py-2 text-sm border rounded disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
      style="{styles.glassStyle(12)} {styles.glassTextStyle}"
      onclick={() => goToPage(currentPage + 1)}
      disabled={currentPage === getTotalPages()}
    >
      {t('common.next')}
    </button>
  </div>
{/snippet}

{#if loading}
  <div class="p-6">
    <div class="animate-pulse">{t('collectionTree.loading')}</div>
  </div>
{:else if workspace}
  <div class="min-h-screen" style="{styles.backgroundStyle}">
    <!-- Content Container -->
    <div class="p-6">
      <!-- Header -->
      <div class="mb-6">
        <ViewHeader
          workspaceName={workspace.name}
          collection={currentCollectionName}
          viewName={t('collectionTree.tree')}
          itemCount={allItems.length}
          hasGradient={styles.hasCustomBackground}
          textStyle={styles.textStyle}
          subtleTextStyle={styles.subtleTextStyle}
        />
      </div>

      <!-- Tree View -->
      {#if allItems.length === 0}
        <EmptyState
          title={t('collectionTree.noWorkItemsYet')}
          description={t('collectionTree.createFirstWorkItem')}
          hasGradient={styles.hasCustomBackground}
        />
      {:else}
        <!-- Tree Controls -->
        <div class="flex justify-between items-center mb-4">
          <div class="flex items-center gap-2">
            <button
              class="flex items-center gap-2 px-3 py-2 text-sm border rounded-md transition-colors"
              style="{styles.glassStyle(12)} {styles.glassTextStyle}"
              onclick={toggleExpandCollapse}
            >
              {#if expandedItems.size === 0}
                <ChevronDown class="w-4 h-4" />
                {t('collectionTree.expandAll')}
              {:else}
                <Minus class="w-4 h-4" />
                {t('collectionTree.collapseAll')}
              {/if}
            </button>

            <!-- Test Case Toggle Button (only show if module enabled) -->
            {#if $moduleSettings.test_management_enabled}
              <button
                class="flex items-center gap-2 px-3 py-2 text-sm border rounded-md transition-colors"
                style="{styles.glassStyle(12)} {showTestCases ? 'color: var(--ds-accent-green);' : styles.glassTextStyle}"
                onclick={toggleShowTestCases}
                disabled={loadingTestCases}
              >
                {#if loadingTestCases}
                  <div class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin"></div>
                {:else}
                  <FileCheck class="w-4 h-4" />
                {/if}
                {showTestCases ? t('collectionTree.hideTests') : t('collectionTree.showTests')}
              </button>
            {/if}
          </div>

          <!-- Pagination Info and Controls -->
          {#if getTotalPages() > 1}
            <div class="flex items-center gap-4">
              <span class="text-sm" style="{styles.textStyle}">
                {t('collectionTree.showingRootItems', { start: paginationInfo.start, end: paginationInfo.end, total: paginationInfo.total })}
              </span>
              {@render paginationControls()}
            </div>
          {/if}
        </div>

        <!-- Table Container -->
        <div class="rounded-xl border shadow-sm overflow-hidden" style="{styles.glassStyle(12)}">
          <!-- Table Header -->
          <TableHeaderBar columns="3rem 6rem 1fr 6rem 5rem 5rem">
            <div></div>
            <div>{t('collectionTree.issue')}</div>
            <div>{t('common.summary')}</div>
            <div>{t('common.status')}</div>
            <div>{t('common.priority')}</div>
            <div>{t('common.created')}</div>
          </TableHeaderBar>

          <!-- Tree Items -->
          <div style="--divide-color: var(--ds-border);">
            {#each treeData as item, idx (item.isTestCase ? `tc-${item.id}` : `item-${item.id}`)}
            {#if item.isTestCase}
              <!-- Test Case Row -->
              <div
                class="flex items-center gap-4 px-4 py-2.5 transition-colors group bg-green-50/30 {!styles.hasGradient ? 'hover:bg-green-50/50' : ''}"
                style="{styles.hasGradient ? 'hover:background-color: rgba(34, 197, 94, 0.05);' : ''}"
              >
                <!-- Hierarchy Indent + Icon -->
                <div class="flex items-center gap-1" style="margin-left: {getIndentLevel(item.level)}">
                  <div class="w-6 h-6"></div> <!-- Spacer (no expand/collapse) -->

                  <!-- Test Case Icon -->
                  <div class="w-4 h-4 rounded flex items-center justify-center bg-green-600">
                    <FileCheck class="w-2.5 h-2.5 text-white" />
                  </div>
                </div>

                <!-- Test Case ID -->
                <div class="min-w-24">
                  <LinkComponent
                    href="#view-test-case"
                    onClick={(e) => handleTestCaseClick(e, item.id)}
                    class="text-xs font-mono px-1.5 py-0.5 rounded cursor-pointer transition-colors text-green-700 bg-green-100 hover:bg-green-200"
                  >
                    TC-{item.id}
                  </LinkComponent>
                </div>

                <!-- Test Case Title -->
                <div class="flex-1 min-w-0">
                  <LinkComponent
                    href="#view-test-case"
                    onClick={(e) => handleTestCaseClick(e, item.id)}
                    class="text-left w-full text-sm transition-colors truncate cursor-pointer text-green-900 hover:text-green-700"
                  >
                    {item.title}
                  </LinkComponent>
                </div>

                <!-- Empty columns for alignment -->
                <div class="w-24 text-xs text-green-600">{t('testing.testCase')}</div>
                <div class="w-20"></div>
                <div class="w-20"></div>
              </div>
            {:else}
              <!-- Regular Work Item Row -->
              {@const typeInfo = getItemTypeInfo(item)}
              {@const selectedStatus = statuses.find(s => s.id === item.status_id)}
              {@const statusCategory = selectedStatus ? statusCategories.find(sc => sc.id === selectedStatus.category_id) : null}
              {@const selectedPriority = priorities.find(p => p.id === item.priority_id)}
              {@const testCaseCount = (showTestCases && itemTestCaseLinksStore.get(item.id)) ? itemTestCaseLinksStore.get(item.id).length : 0}
              <div
                class="flex items-center gap-4 px-4 py-3 transition-colors group tree-row"
                style="{styles.hasGradient ? '' : 'border-top: 1px solid var(--ds-border);'}{idx === 0 ? 'border-top: none;' : ''}"
              >
                <!-- Hierarchy Indent + Expand/Collapse + Icon -->
                <div class="flex items-center gap-1" style="margin-left: {getIndentLevel(item.level)}">
                  <!-- Expand/Collapse Button -->
                  {#if item.hasChildren}
                    <button
                      class="w-6 h-6 flex items-center justify-center rounded transition-colors cursor-pointer expand-btn"
                      onclick={(e) => { e.stopPropagation(); toggleExpanded(item.id); }}
                      aria-label={isExpanded(item.id) ? 'Collapse' : 'Expand'}
                    >
                      {#if isExpanded(item.id)}
                        <ChevronDown class="w-4 h-4" style="{styles.glassTextStyle}" />
                      {:else}
                        <ChevronRight class="w-4 h-4" style="{styles.glassTextStyle}" />
                      {/if}
                    </button>
                  {:else}
                    <div class="w-6 h-6"></div> <!-- Spacer for alignment -->
                  {/if}

                  <!-- Work Item Type Icon -->
                  <div class="w-4 h-4 rounded flex items-center justify-center" style="background-color: {typeInfo.color.startsWith('#') ? typeInfo.color : 'var(--ds-interactive-subtle)'}">
                    <svelte:component
                      this={typeInfo.icon}
                      class="w-2.5 h-2.5 text-white"
                    />
                  </div>
                </div>

                <!-- Issue Key + Test Case Count Badge -->
                <div class="min-w-24 flex items-center gap-1.5">
                  <ItemKey
                    {item}
                    {workspace}
                    href={collectionId
                      ? `/workspaces/${workspaceId}/collections/${collectionId}/items/${item.id}`
                      : `/workspaces/${workspaceId}/items/${item.id}`}
                    style="{styles.glassSubtleTextStyle}"
                  />

                  <!-- Test Case Count Badge -->
                  {#if showTestCases && testCaseCount > 0}
                    <span class="inline-flex items-center gap-0.5 px-1.5 py-0.5 text-xs font-medium rounded-full" style="background-color: var(--ds-background-accent-green-subtler); color: var(--ds-accent-green);">
                      <FileCheck class="w-3 h-3" />
                      {testCaseCount}
                    </span>
                  {/if}
                </div>

                <!-- Summary -->
                <div class="flex-1 min-w-0">
                  <LinkComponent
                    href={collectionId
                      ? `/workspaces/${workspaceId}/collections/${collectionId}/items/${item.id}`
                      : `/workspaces/${workspaceId}/items/${item.id}`}
                    class="text-left w-full transition-colors truncate cursor-pointer summary-link"
                    style="{styles.glassTextStyle}"
                  >
                    {item.title}
                  </LinkComponent>
                </div>

                <!-- Status -->
                <div class="w-24">
                  <Lozenge
                    text={selectedStatus ? selectedStatus.name : t('collectionTree.noStatus')}
                    customBg={statusCategory?.color || '#6b7280'}
                  />
                </div>

                <!-- Priority -->
                <div class="w-20">
                  {#if selectedPriority}
                    <div class="flex items-center gap-1.5">
                      <ColorDot color={selectedPriority.color} />
                      <span class="text-xs capitalize" style="color: {selectedPriority.color};">
                        {selectedPriority.name}
                      </span>
                    </div>
                  {:else}
                    <span class="text-xs" style="{styles.glassSubtleTextStyle}">-</span>
                  {/if}
                </div>

                <!-- Created Date -->
                <div class="w-20 text-xs" style="{styles.glassSubtleTextStyle}">
                  {formatDate(item.created_at) || '-'}
                </div>
              </div>
            {/if}
          {/each}
        </div>
      </div>

        <!-- Bottom Pagination -->
        {#if getTotalPages() > 1}
          <div class="flex justify-center items-center gap-4 mt-6 pt-4 border-t" style="border-color: var(--ds-border);">
            {@render paginationControls()}
          </div>
        {/if}
      {/if}
    </div>
  </div>
{:else}
  <div class="p-6">
    <div class="text-center" style="color: var(--ds-text-subtle);">
      {t('collectionTree.workspaceNotFound')}
    </div>
  </div>
{/if}

<!-- Test Case View Modal -->
<TestCaseViewModal
  isOpen={showTestCaseModal}
  testCaseId={selectedTestCaseId}
  onclose={() => { showTestCaseModal = false; selectedTestCaseId = null; }}
/>

<style>
  .tree-row:hover {
    background-color: var(--ds-surface-hovered);
  }

  .expand-btn:hover {
    background-color: var(--ds-surface-hovered);
  }

  :global(.summary-link:hover) {
    color: var(--ds-link) !important;
  }
</style>
