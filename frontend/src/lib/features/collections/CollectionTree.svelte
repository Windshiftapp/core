<script>
  import { onMount } from 'svelte';
  import { api } from '../../api.js';
  import { navigate } from '../../router.js';
  import { getCollection } from '../collections/collectionService.js';
  import { getStatusColor, getStatusInlineStyle, getTextColorForBackground } from '../../utils/statusColors.js';
  import { workspaceGradientIndex, applyToAllViews, loadWorkspaceGradient, getGradientStyle } from '../../stores/workspaceGradient.js';
  import { gradients } from '../../utils/gradients.js';
  import { ChevronRight, ChevronDown, GitBranch, Circle, AlertCircle, Calendar, FileCheck, Minus } from 'lucide-svelte';
  import { itemTypeIconMap } from '../../utils/icons.js';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import ItemKey from '../items/ItemKey.svelte';
  import ColorDot from '../../components/ColorDot.svelte';
  import LinkComponent from '../../components/Link.svelte';
  import TestCaseViewModal from '../../dialogs/TestCaseViewModal.svelte';
  import Lozenge from '../../components/Lozenge.svelte';
  import { formatDate } from '../../utils/dateFormatter.js';
  import { moduleSettings } from '../../stores/moduleSettings.js';

  let { workspaceId, collectionId = null } = $props();

  let workspace = $state(null);
  let allItems = $state([]);
  let itemTypes = $state([]);
  let statuses = $state([]);
  let statusCategories = $state([]);
  let priorities = $state([]);
  let loading = $state(true);
  let currentCollectionName = $state('Default');
  let currentView = 'tree';
  let expandedItems = $state(new Set()); // Track which items are expanded
  
  // Pagination state
  let currentPage = $state(1);
  let itemsPerPage = $state(50);

  // Test case toggle state
  let showTestCases = $state(false);
  let testCaseLinks = $state(new Map()); // Cache: itemId -> array of test cases
  let loadingTestCases = $state(false);

  // Test case modal state
  let showTestCaseModal = $state(false);
  let selectedTestCaseId = $state(null);

  // Reactive gradient styling
  let gradientStyle = $derived(($applyToAllViews && $workspaceGradientIndex > 0) ? getGradientStyle($workspaceGradientIndex) : null);
  let hasGradient = $derived(gradientStyle !== null);
  let backgroundStyle = $derived(hasGradient ? `background: ${gradientStyle};` : 'background-color: var(--ds-surface);');

  // Text on gradient background (white for visibility)
  let textStyle = $derived(hasGradient ? 'color: white;' : 'color: var(--ds-text);');
  let subtleTextStyle = $derived(hasGradient ? 'color: rgba(255, 255, 255, 0.8);' : 'color: var(--ds-text-subtle);');

  // Glass styling for cards/buttons/tables (theme-aware)
  let glassStyle = $derived(hasGradient
    ? 'background-color: var(--ds-glass-bg); border-color: var(--ds-glass-border); backdrop-filter: blur(12px);'
    : 'background-color: var(--ds-surface-raised); border-color: var(--ds-border);');
  let glassTextStyle = $derived('color: var(--ds-text);');
  let glassSubtleTextStyle = $derived('color: var(--ds-text-subtle);');

  onMount(async () => {
    console.log('[CollectionTree] mount workspaceId=', workspaceId, 'collectionId=', collectionId);
    // Load test case toggle preference from localStorage
    const saved = localStorage.getItem('collectionTree_showTestCases');
    if (saved !== null) {
      showTestCases = saved === 'true';
    }

    await loadWorkspaceGradient(workspaceId);
    await loadData();
  });

  // Watch for changes to workspaceId or collectionId after initial mount
  let lastWorkspaceId = workspaceId;
  let lastCollectionId = collectionId;
  $effect(() => {
    const nextWorkspaceId = workspaceId;
    const nextCollectionId = collectionId;
    if (!nextWorkspaceId) {
      return;
    }
    if (nextWorkspaceId !== lastWorkspaceId || nextCollectionId !== lastCollectionId) {
      lastWorkspaceId = nextWorkspaceId;
      lastCollectionId = nextCollectionId;
      loadData('param-change');
    }
  });

  async function loadData(source = 'effect') {
    console.log(`[CollectionTree] loadData triggered from ${source}`);
    loading = true;
    if (workspaceId) {
      console.log('[CollectionTree] start loading workspace', workspaceId, 'collection', collectionId);
      await Promise.all([
        loadWorkspace(),
        loadAllItems(),
        loadItemTypes(),
        loadStatusData(),
        loadPriorities()
      ]);

      // Expand root items by default for better initial UX
      const rootItems = getRootItems();
      rootItems.forEach(item => {
        if (hasChildren(item.id)) {
          expandedItems.add(item.id);
        }
      });
      expandedItems = new Set(expandedItems);
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
    console.log('[CollectionTree] loading workspace');
    try {
      workspace = await api.workspaces.get(workspaceId);
      console.log('[CollectionTree] workspace loaded', workspace?.id);
    } catch (error) {
      console.error('[CollectionTree] Failed to load workspace:', error);
    }
  }

  async function loadItemTypes() {
    console.log('[CollectionTree] loading item types');
    try {
      itemTypes = await api.itemTypes.getAll();
      console.log('[CollectionTree] item types loaded', itemTypes?.length);
    } catch (error) {
      console.error('[CollectionTree] Failed to load item types:', error);
      itemTypes = [];
    }
  }

  async function loadAllItems() {
    try {
      // Build filters based on collection
      const filters = { workspace_id: workspaceId };
      
      if (collectionId) {
      console.log('[CollectionTree] fetching collection info', collectionId);
      const collection = await getCollection(collectionId);
      if (collection) {
        currentCollectionName = collection.name;
        if (collection.cql_query) {
          filters.vql = collection.cql_query;
        }
        }
      } else {
        currentCollectionName = 'Default';
      }
      
      // Load all items for this workspace
      console.log('[CollectionTree] loading items with filters', filters);
      const response = await api.items.getAll(filters);
      console.log('[CollectionTree] items response received', response);
      
      // Handle different response formats
      let items = [];
      if (Array.isArray(response)) {
        items = response;
      } else if (response && Array.isArray(response.items)) {
        items = response.items;
      } else if (response && response.data && Array.isArray(response.data)) {
        items = response.data;
      } else {
        console.warn('Unexpected response format from api.items.getAll:', response);
        items = [];
      }
      
      allItems = items.sort((a, b) => a.level - b.level || a.id - b.id);
    } catch (error) {
      console.error('[CollectionTree] Failed to load items:', error);
      allItems = [];
    }
  }

  async function loadStatusData() {
    console.log('[CollectionTree] loading statuses');
    try {
      const [statusesData, statusCategoriesData] = await Promise.all([
        api.workspaces.getStatuses(workspaceId),
        api.statusCategories.getAll()
      ]);
      statuses = statusesData || [];
      statusCategories = statusCategoriesData || [];
      console.log('[CollectionTree] status data loaded', statuses.length, statusCategories.length);
    } catch (error) {
      console.error('[CollectionTree] Failed to load status data:', error);
      statuses = [];
      statusCategories = [];
    }
  }

  async function loadPriorities() {
    console.log('[CollectionTree] loading priorities');
    try {
      priorities = await api.priorities.getAll();
      console.log('[CollectionTree] priorities loaded', priorities.length);
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
      // Fetch links for all items in parallel
      const linkPromises = itemIds.map(itemId =>
        api.links.getForItem('items', itemId).catch(err => {
          console.error(`Failed to load links for item ${itemId}:`, err);
          return { outgoing: [], incoming: [] };
        })
      );

      const linkResults = await Promise.all(linkPromises);

      // Process results and extract test cases
      itemIds.forEach((itemId, index) => {
        const links = linkResults[index];
        const allLinks = [...(links.outgoing || []), ...(links.incoming || [])];

        // Filter for "Tests" link type (ID = 1) and extract test cases
        const testCases = allLinks
          .filter(link => link.link_type_id === 1)
          .map(link => {
            // Determine if this item is source or target
            const isSource = link.source_type === 'item' && link.source_id === itemId;
            const testCaseData = isSource ? {
              id: link.target_id,
              title: link.target_title,
              type: link.target_type
            } : {
              id: link.source_id,
              title: link.source_title,
              type: link.source_type
            };

            // Only include if it's actually a test case
            return testCaseData.type === 'test_case' ? testCaseData : null;
          })
          .filter(tc => tc !== null);

        testCaseLinks.set(itemId, testCases);
      });

      // Trigger reactivity
      testCaseLinks = new Map(testCaseLinks);
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
    if (!showTestCases || allItems.length === 0) {
      return;
    }
    if (loadingTestCases) {
      return;
    }
    const visibleItemIds = allItems.map(item => item.id);
    const missingItemIds = visibleItemIds.filter(id => !testCaseLinks.has(id));
    if (missingItemIds.length === 0) {
      return;
    }
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

  function getRootPaginationInfo() {
    const total = getTotalRootItems();
    if (total === 0) {
      return { total: 0, start: 0, end: 0 };
    }
    const start = (currentPage - 1) * itemsPerPage + 1;
    const end = Math.min(currentPage * itemsPerPage, total);
    return { total, start, end };
  }

  function goToPage(page) {
    currentPage = page;
  }

  // Status color function using shared utility and official category colors
  function getStatusColorClasses(status) {
    return getStatusColor(status, statuses, statusCategories);
  }

  function getStatusStyle(status) {
    return getStatusInlineStyle(status, statuses, statusCategories);
  }

  function viewItem(item) {
    const url = collectionId 
      ? `/workspaces/${workspaceId}/collections/${collectionId}/items/${item.id}`
      : `/workspaces/${workspaceId}/items/${item.id}`;
    navigate(url);
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
      if (showTestCases && testCaseLinks.has(item.id)) {
        const testCases = testCaseLinks.get(item.id);
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

{#if loading}
  <div class="p-6">
    <div class="animate-pulse">Loading...</div>
  </div>
{:else if workspace}
  <div class="min-h-screen" style="{backgroundStyle}">
    <!-- Content Container -->
    <div class="p-6">
      <!-- Header -->
      <div class="mb-6">
        <ViewHeader
          workspaceName={workspace.name}
          collection={currentCollectionName}
          viewName="Tree"
          itemCount={allItems.length}
          hasGradient={hasGradient}
          textStyle={textStyle}
          subtleTextStyle={subtleTextStyle}
        />
      </div>

      <!-- Tree View -->
      {#if allItems.length === 0}
        <div class="text-center py-12">
          <GitBranch class="w-12 h-12 mx-auto mb-4" style="{subtleTextStyle}" />
          <h3 class="text-lg font-medium mb-2" style="{textStyle}">No work items yet</h3>
          <p style="{subtleTextStyle}">Create your first work item to see the hierarchy tree.</p>
        </div>
      {:else}
        <!-- Tree Controls -->
        <div class="flex justify-between items-center mb-4">
          <div class="flex items-center gap-2">
            <button
              class="flex items-center gap-2 px-3 py-2 text-sm border rounded-md transition-colors"
              style="{glassStyle} {glassTextStyle}"
              onclick={toggleExpandCollapse}
            >
              {#if expandedItems.size === 0}
                <ChevronDown class="w-4 h-4" />
                Expand All
              {:else}
                <Minus class="w-4 h-4" />
                Collapse All
              {/if}
            </button>

            <!-- Test Case Toggle Button (only show if module enabled) -->
            {#if $moduleSettings.test_management_enabled}
              <button
                class="flex items-center gap-2 px-3 py-2 text-sm border rounded-md transition-colors"
                style="{glassStyle} {showTestCases ? 'color: var(--ds-accent-green);' : glassTextStyle}"
                onclick={toggleShowTestCases}
                disabled={loadingTestCases}
              >
                {#if loadingTestCases}
                  <div class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin"></div>
                {:else}
                  <FileCheck class="w-4 h-4" />
                {/if}
                {showTestCases ? 'Hide' : 'Show'} Tests
              </button>
            {/if}
          </div>

          <!-- Pagination Info and Controls -->
          {#if getTotalPages() > 1}
            <div class="flex items-center gap-4">
              <span class="text-sm" style="{textStyle}">
                Showing {paginationInfo.start}-{paginationInfo.end} of {paginationInfo.total} root items
              </span>

              <div class="flex items-center gap-2">
                <button
                  class="px-3 py-1 text-sm border rounded disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                  style="{glassStyle} {glassTextStyle}"
                  onclick={() => goToPage(currentPage - 1)}
                  disabled={currentPage === 1}
                >
                  Previous
                </button>

                <span class="px-3 py-1 text-sm" style="{textStyle}">
                  Page {currentPage} of {getTotalPages()}
                </span>

                <button
                  class="px-3 py-1 text-sm border rounded disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                  style="{glassStyle} {glassTextStyle}"
                  onclick={() => goToPage(currentPage + 1)}
                  disabled={currentPage === getTotalPages()}
                >
                  Next
                </button>
              </div>
            </div>
          {/if}
        </div>

        <!-- Table Container -->
        <div class="rounded-xl border shadow-sm overflow-hidden" style="{glassStyle}">
          <!-- Table Header -->
          <div class="border-b px-4 py-3" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
            <div class="flex items-center gap-4 text-xs font-semibold uppercase tracking-wider" style="{glassSubtleTextStyle}">
              <div class="w-12"></div> <!-- Expand + Icon space -->
              <div class="min-w-24">Issue</div>
              <div class="flex-1">Summary</div>
              <div class="w-24">Status</div>
              <div class="w-20">Priority</div>
              <div class="w-20">Created</div>
            </div>
          </div>

          <!-- Tree Items -->
          <div style="--divide-color: var(--ds-border);">
            {#each treeData as item, idx (item.isTestCase ? `tc-${item.id}` : `item-${item.id}`)}
            {#if item.isTestCase}
              <!-- Test Case Row -->
              <div
                class="flex items-center gap-4 px-4 py-2.5 transition-colors group bg-green-50/30 {!hasGradient ? 'hover:bg-green-50/50' : ''}"
                style="{hasGradient ? 'hover:background-color: rgba(34, 197, 94, 0.05);' : ''}"
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
                <div class="w-24 text-xs text-green-600">Test Case</div>
                <div class="w-20"></div>
                <div class="w-20"></div>
              </div>
            {:else}
              <!-- Regular Work Item Row -->
              {@const typeInfo = getItemTypeInfo(item)}
              {@const selectedStatus = statuses.find(s => s.id === item.status_id)}
              {@const statusCategory = selectedStatus ? statusCategories.find(sc => sc.id === selectedStatus.category_id) : null}
              {@const selectedPriority = priorities.find(p => p.id === item.priority_id)}
              {@const testCaseCount = showTestCases && testCaseLinks.has(item.id) ? testCaseLinks.get(item.id).length : 0}
              <div
                class="flex items-center gap-4 px-4 py-3 transition-colors group tree-row"
                style="{hasGradient ? '' : 'border-top: 1px solid var(--ds-border);'}{idx === 0 ? 'border-top: none;' : ''}"
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
                        <ChevronDown class="w-4 h-4" style="{glassTextStyle}" />
                      {:else}
                        <ChevronRight class="w-4 h-4" style="{glassTextStyle}" />
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
                    className="text-xs font-mono px-1.5 py-0.5 rounded cursor-pointer transition-colors item-key"
                    style="background-color: var(--ds-interactive-subtle); {glassSubtleTextStyle}"
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
                    class="text-left w-full font-medium transition-colors truncate cursor-pointer summary-link"
                    style="{glassTextStyle}"
                  >
                    {item.title}
                  </LinkComponent>
                </div>

                <!-- Status -->
                <div class="w-24">
                  <Lozenge
                    text={selectedStatus ? selectedStatus.name : 'No status'}
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
                    <span class="text-xs" style="{glassSubtleTextStyle}">-</span>
                  {/if}
                </div>

                <!-- Created Date -->
                <div class="w-20 text-xs" style="{glassSubtleTextStyle}">
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
            <div class="flex items-center gap-2">
              <button
                class="px-3 py-2 text-sm border rounded disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                style="{glassStyle} {glassTextStyle}"
                onclick={() => goToPage(currentPage - 1)}
                disabled={currentPage === 1}
              >
                Previous
              </button>

              <span class="px-4 py-2 text-sm" style="{textStyle}">
                Page {currentPage} of {getTotalPages()}
              </span>

              <button
                class="px-3 py-2 text-sm border rounded disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                style="{glassStyle} {glassTextStyle}"
                onclick={() => goToPage(currentPage + 1)}
                disabled={currentPage === getTotalPages()}
              >
                Next
              </button>
            </div>
          </div>
        {/if}
      {/if}
    </div>
  </div>
{:else}
  <div class="p-6">
    <div class="text-center" style="color: var(--ds-text-subtle);">
      Workspace not found.
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

  :global(.item-key:hover) {
    background-color: var(--ds-surface-hovered) !important;
  }

  :global(.summary-link:hover) {
    color: var(--ds-link) !important;
  }
</style>
