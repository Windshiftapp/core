<script>
  import { onMount, untrack } from 'svelte';
  import { items as itemsApi } from '../../api/items.js';
  import {
    applyCollectionItem,
    collectionStore,
    scheduleCollectionReconciliation,
  } from '../../stores/collectionContext.js';
  import { useCollectionEventStream } from '../../composables/useCollectionEventStream.svelte.js';
  import { t } from '../../stores/i18n.svelte.js';
  import { errorToast } from '../../stores/toasts.svelte.js';
  import { workspacePermissions } from '../../stores/workspacePermissions.svelte.js';
  import { useGradientStyles, loadWorkspaceGradient } from '../../stores/workspaceGradient.svelte.js';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import FileCheck from '@lucide/svelte/icons/file-check';
  import Minus from '@lucide/svelte/icons/minus';
  import ViewHeader from '../../layout/ViewHeader.svelte';
  import StaticViewBackground from '../../layout/StaticViewBackground.svelte';
  import SubFilterBar from './SubFilterBar.svelte';
  import LinkComponent from '../../components/Link.svelte';
  import TestCaseViewModal from '../../dialogs/TestCaseViewModal.svelte';
  import EmptyState from '../../components/EmptyState.svelte';
  import TableHeaderBar from '../../components/TableHeaderBar.svelte';
  import { moduleSettings } from '../../stores/moduleSettings.js';
  import { collectionEditorOptions } from '../../stores/collectionEditorOptions.svelte.js';
  import { collectionFieldLinks } from '../../stores/collectionFieldLinks.svelte.js';
  import { itemTestCaseLinksStore } from '../../stores/itemTestCaseLinksStore.svelte.js';
  import { workspaceDataStore } from '../../stores/workspaceDataStore.svelte.js';
  import {
    getListColumnLabel,
    listGridMinWidth,
    listGridTemplateColumns,
  } from '../../utils/workItemListColumns.js';
  import ListCellRenderer from './ListCellRenderer.svelte';
  import ColumnSelector from './ColumnSelector.svelte';
  import CollectionHierarchyLoadMore from './CollectionHierarchyLoadMore.svelte';
  import { useListColumnConfig } from './listColumnConfig.svelte.js';
  import {
    findOrphansByMissingParent,
    indexCollectionHierarchy,
    mergeAncestorContext,
  } from './collectionHierarchy.js';

  let { workspaceId, collectionId = null } = $props();

  let workspace = $derived(workspaceDataStore.workspace);
  // Items as loaded from the collection store, plus fetched ancestor context.
  let storeItems = $state([]);
  // Missing parent id -> ancestor chain (root -> parent). Fills gaps left by
  // pagination and the completed filter so hierarchies stay whole.
  let ancestorsByParent = $state({});
  let ancestorsRequestInFlight = false;
  let ancestorsRerunPending = false;
  let itemTypes = $derived(workspaceDataStore.itemTypes);
  let statuses = $derived(workspaceDataStore.statuses);
  let statusCategories = $derived(workspaceDataStore.statusCategories);
  let priorities = $derived(workspaceDataStore.priorities);
  let users = $derived(workspaceDataStore.users);
  let milestones = $derived(workspaceDataStore.milestones);
  let iterations = $derived(workspaceDataStore.iterations);
  let projects = $derived(workspaceDataStore.projects);
  let customFieldDefinitions = $derived(workspaceDataStore.customFieldDefinitions);

  // Shown fields come from the same list-column configuration as the list view.
  const columnConfig = useListColumnConfig(() => ({ workspaceId, collectionId }));
  let listColumns = $derived(columnConfig.columns);
  let loading = $state(true);
  let currentCollectionName = $state('Default');
  let expandedItems = $state(new Set()); // Track which items are expanded
  let allItems = $derived(
    [...mergeAncestorContext(storeItems, ancestorsByParent)].sort(
      (a, b) => a.level - b.level || a.id - b.id
    )
  );
  let hierarchyIndex = $derived(indexCollectionHierarchy(allItems));
  // Filter-scoped item total from the server: everything the tree contains
  // across all pages, independent of expansion state or the loaded page.
  // Drives the header's "N shown" so it cannot be misread as per-page count.
  let matchingItemCount = $derived(collectionStore.itemsPagination?.total_items ?? null);
  
  // Test case toggle state
  let showTestCases = $state(false);
  let loadingTestCases = $state(false);

  // Test case modal state
  let showTestCaseModal = $state(false);
  let selectedTestCaseId = $state(null);
  let selectedTestCaseWorkspaceId = $state(null);

  // Centralized gradient styling
  const styles = useGradientStyles();

  // Scoped SSE invalidations (WI-1624): any frame runs the existing delta fetch,
  // and a reconnect reconciles. Without this the tree kept showing items that
  // were deleted from the item detail modal, unlike the board views.
  useCollectionEventStream(
    () => (collectionId ? { kind: 'collection', id: collectionId } : { kind: 'workspace', id: workspaceId }),
    { onInvalidate: () => scheduleCollectionReconciliation() }
  );

  // On mount, request the same page size used by collection store filter reloads.
  onMount(async () => {
    // Load test case toggle preference from localStorage
    const saved = localStorage.getItem('collectionTree_showTestCases');
    if (saved !== null) {
      showTestCases = saved === 'true';
    }

    if (workspaceId) {
      await loadWorkspaceGradient(workspaceId);
    }
    await loadData();
  });

  // Sync items from central store, then reset expansion to the all-roots
  // default for the new page. Ancestors fetched later expand themselves in
  // syncAncestorContext so their children stay visible.
  // Inline edits patch single rows; those keep the current expansion.
  let loadedItemIdsSignature = null;
  $effect(() => {
    if (!collectionStore.loading && collectionStore.items.length >= 0) {
      currentCollectionName = collectionStore.collectionName;
      const items = [...collectionStore.items].sort((a, b) => a.level - b.level || a.id - b.id);
      storeItems = items;
      const signature = items.map((item) => item.id).join(',');
      if (signature === loadedItemIdsSignature) return;
      loadedItemIdsSignature = signature;
      untrack(() => {
        const rootItems = getRootItems();
        expandedItems = new Set(rootItems.filter(i => hasChildren(i.id)).map(i => i.id));
      });
    }
  });

  // Prime the row-editor cache from the shared workspace store so opening a
  // cell does not repeat those requests (see CollectionList). Global
  // collections skip this: their rows resolve per owning workspace.
  $effect(() => {
    const id = Number(workspaceId);
    if (
      !Number.isInteger(id) ||
      id <= 0 ||
      !workspaceDataStore.initialized ||
      Number(workspaceDataStore.workspaceId) !== id
    ) {
      return;
    }
    untrack(() =>
      collectionEditorOptions.prime(
        id,
        { statuses, users, milestones, iterations, projects },
        workspaceDataStore.planningCompleteness
      )
    );
  });

  // Resolve only the assignees the tree references.
  $effect(() => {
    const ids = allItems.map((item) => item.assignee_id).filter(Boolean);
    if (ids.length > 0) void workspaceDataStore.hydrateUsers(ids);
  });

  // Linking fields live in item_links; hydrate them in one batch when a
  // linking column is shown.
  $effect(() => {
    const linkingFieldIds = new Set(
      customFieldDefinitions
        .filter((field) => field.field_type === 'linking')
        .map((field) => String(field.id)),
    );
    const hasLinkingColumn = listColumns.some(
      (column) =>
        column.field_type === 'custom' && linkingFieldIds.has(String(column.field_identifier)),
    );
    if (hasLinkingColumn) {
      collectionFieldLinks.loadForItems(allItems.map((item) => item.id));
    }
  });

  function handleItemUpdated(data) {
    applyCollectionItem(data.item);
  }

  function handleUpdateError(data) {
    const { error, field } = data;
    console.error(`Failed to update ${field}:`, error);
    errorToast(t('dialogs.alerts.failedToUpdate', { error: `${field}: ${error}` }));
  }

  // Load ancestor chains for items whose parent is not on the loaded page so
  // filtered or paginated children do not render as stray roots.
  $effect(() => {
    const items = storeItems;
    untrack(() => void syncAncestorContext(items));
  });

  async function syncAncestorContext(items) {
    const orphansByParent = findOrphansByMissingParent(items);
    const neededParents = new Set(orphansByParent.keys());

    // Drop cached chains whose gap the store has since filled itself.
    const kept = {};
    let pruned = false;
    for (const [parentId, chain] of Object.entries(ancestorsByParent)) {
      if (neededParents.has(Number(parentId))) {
        kept[parentId] = chain;
      } else {
        pruned = true;
      }
    }
    if (pruned) ancestorsByParent = kept;

    const parentByOrphanId = new Map();
    for (const [parentId, orphans] of orphansByParent) {
      parentByOrphanId.set(orphans[0].id, parentId);
    }
    const orphanIds = [...parentByOrphanId.keys()].filter(
      (orphanId) => !kept[String(parentByOrphanId.get(orphanId))]
    );
    if (orphanIds.length === 0) return;
    if (ancestorsRequestInFlight) {
      // A newer page may need different chains; rerun when the current
      // request settles.
      ancestorsRerunPending = true;
      return;
    }
    ancestorsRequestInFlight = true;
    try {
      const entries = await itemsApi.getManyAncestors(orphanIds);
      for (const { item_id: orphanId, ancestors } of entries) {
        // The gap may have closed while the request was in flight.
        const parentId = parentByOrphanId.get(orphanId);
        if (!ancestors?.length || parentId == null || !neededParents.has(parentId)) continue;
        ancestorsByParent = { ...ancestorsByParent, [String(parentId)]: ancestors };
        // Keep previously visible children visible under their new parents.
        for (const ancestor of ancestors) {
          expandedItems.add(ancestor.id);
        }
      }
      expandedItems = new Set(expandedItems);
    } catch (error) {
      console.error('[CollectionTree] Failed to load ancestor context:', error);
    } finally {
      ancestorsRequestInFlight = false;
    }
    if (ancestorsRerunPending) {
      ancestorsRerunPending = false;
      await syncAncestorContext(storeItems);
    }
  }

  async function loadData() {
    loading = true;
    if (workspaceId) {
      itemTestCaseLinksStore.initialize(workspaceId);
      await workspaceDataStore.initialize(workspaceId);

      if (showTestCases) {
        await loadPendingTestCases();
      }
    } else {
      await workspaceDataStore.initializeGlobal();
    }
    loading = false;
  }

  $effect(() => {
    if (showTestCases && allItems.length > 0) {
      loadPendingTestCases();
    }
  });

  function getItemsByParent(parentId) {
    return hierarchyIndex.childrenByParent.get(parentId) || [];
  }

  function getRootItems() {
    return hierarchyIndex.roots;
  }

  function hasChildren(itemId) {
    return hierarchyIndex.childrenByParent.has(itemId);
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
  function handleTestCaseClick(event, testCaseId, testCaseWorkspaceId = null) {
    event.preventDefault();
    selectedTestCaseId = testCaseId;
    selectedTestCaseWorkspaceId = testCaseWorkspaceId ?? null;
    showTestCaseModal = true;
  }

  function getIndentLevel(level) {
    return `${level * 1.5}rem`;
  }

  function renderTreeItems(parentId = null, level = 0, result = [], rootItems = null) {
    // The root level renders every loaded root; children follow the loaded set.
    const items = parentId === null ? (rootItems || getRootItems()) : getItemsByParent(parentId);

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
      ? renderTreeItems(null, 0, [], getRootItems())
      : []
  );

  // Leading hierarchy track holds the expander (+ test case badge); the depth
  // indent is applied to the title cell so every child row shows its nesting.
  let hierarchyWidth = $derived(1.5 + (showTestCases ? 3 : 0));
  let indentColumnId = $derived(
    listColumns.some((column) => column.field_identifier === 'title')
      ? 'title'
      : listColumns[0]?.field_identifier
  );
  let gridTemplateColumns = $derived(`${hierarchyWidth}rem ${listGridTemplateColumns(listColumns)}`);
  let gridMinWidth = $derived(`calc(${listGridMinWidth(listColumns)} + ${hierarchyWidth + 1}rem)`);
</script>

{#if loading}
  <div class="p-6">
    <div class="animate-pulse">{t('collectionTree.loading')}</div>
  </div>
{:else if workspace || !workspaceId}
  <StaticViewBackground
    backgroundStyle={styles.backgroundStyle}
    contextVars={styles.contextVars}
    testid="tree-view"
  >
    <!-- Content Container -->
      <!-- Header -->
      <div class="mb-6">
        <ViewHeader
          workspaceName={workspace?.name || ''}
          collection={currentCollectionName}
          viewName={t('collectionTree.tree')}
          itemCount={collectionStore.collectionTotal}
          shownCount={collectionStore.loading ? null : matchingItemCount}
        />
      </div>

      <!-- Controls Bar -->
      <div class="flex items-center justify-between mb-6">
        <div class="flex items-center gap-4">
          <SubFilterBar {workspaceId} />
        </div>
        <div class="flex items-center gap-2">
          <ColumnSelector
            columns={listColumns}
            {customFieldDefinitions}
            canConfigure={columnConfig.canConfigure}
            testId="tree-column-selector-trigger"
            onchange={({ columns }) => columnConfig.save(columns)}
          />
        </div>
      </div>

      <!-- Tree View -->
      {#if allItems.length === 0}
        <EmptyState
          title={t('collectionTree.noWorkItemsYet')}
          description={t('collectionTree.createFirstWorkItem')}
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

        </div>

        <!-- Table Container -->
        <div class="rounded-xl border shadow-sm overflow-x-auto" style="{styles.glassStyle(12)}">
          <!-- Table Header -->
          <TableHeaderBar columns={gridTemplateColumns} minWidth={gridMinWidth}>
            <div></div>
            {#each listColumns as column (column.field_identifier)}
              <div>{getListColumnLabel(column, customFieldDefinitions)}</div>
            {/each}
            <div></div>
          </TableHeaderBar>

          <!-- Tree Items -->
          <div style="--divide-color: var(--ds-border);">
            {#each treeData as item, idx (item.isTestCase ? `tc-${item.id}` : `item-${item.id}`)}
            {#if item.isTestCase}
              <!-- Test Case Row -->
              <div
                class="grid gap-4 items-center px-4 py-2.5 transition-colors group bg-ds-success-subtle/30 hover:bg-ds-success-subtle/50"
                style="grid-template-columns: {gridTemplateColumns}; min-width: {gridMinWidth};"
              >
                <!-- Hierarchy Indent + Icon -->
                <div class="flex items-center gap-1">
                  <div class="w-4 h-4 rounded flex items-center justify-center bg-ds-accent-green">
                    <FileCheck class="w-2.5 h-2.5 text-white" />
                  </div>
                </div>

                {#each listColumns as column (column.field_identifier)}
                  <div
                    class="min-w-0 overflow-hidden"
                    style={column.field_identifier === indentColumnId ? `padding-left: ${getIndentLevel(item.level)}` : undefined}
                  >
                    {#if column.field_identifier === 'key'}
                      <LinkComponent
                        href="#view-test-case"
                        onClick={(e) => handleTestCaseClick(e, item.id, item.testCaseData?.workspaceId)}
                        class="text-xs font-mono px-1.5 py-0.5 rounded cursor-pointer transition-colors text-ds-text-accent-green bg-ds-accent-green-subtle hover:opacity-80"
                      >
                        TC-{item.id}
                      </LinkComponent>
                    {:else if column.field_identifier === 'title'}
                      <LinkComponent
                        href="#view-test-case"
                        onClick={(e) => handleTestCaseClick(e, item.id, item.testCaseData?.workspaceId)}
                        class="block text-left w-full text-sm transition-colors truncate cursor-pointer text-ds-text-accent-green hover:opacity-80"
                      >
                        {item.title}
                      </LinkComponent>
                    {:else if column.field_identifier === 'status'}
                      <span class="text-sm text-ds-text-success">{t('testing.testCase')}</span>
                    {/if}
                  </div>
                {/each}
                <div></div>
              </div>
            {:else}
              <!-- Regular Work Item Row -->
              {@const testCaseCount = (showTestCases && itemTestCaseLinksStore.get(item.id)) ? itemTestCaseLinksStore.get(item.id).length : 0}
              <div
                data-testid={`tree-row-${item.id}`}
                class="grid gap-4 items-center px-4 py-3 transition-colors group tree-row"
                style="grid-template-columns: {gridTemplateColumns}; min-width: {gridMinWidth}; border-top: 1px solid var(--ctx-border, var(--ds-border));{idx === 0 ? 'border-top: none;' : ''}"
              >
                <!-- Hierarchy Indent + Expand/Collapse + Test Case Count -->
                <div class="flex items-center gap-1">
                  {#if item.hasChildren}
                    <button
                      data-testid={`tree-expand-${item.id}`}
                      class="w-6 h-6 flex items-center justify-center rounded transition-colors cursor-pointer expand-btn"
                      onclick={(e) => { e.stopPropagation(); toggleExpanded(item.id); }}
                      aria-label={isExpanded(item.id) ? 'Collapse' : 'Expand'}
                    >
                      {#if isExpanded(item.id)}
                        <ChevronDown class="w-4 h-4" style={styles.glassTextStyle} />
                      {:else}
                        <ChevronRight class="w-4 h-4" style={styles.glassTextStyle} />
                      {/if}
                    </button>
                  {:else}
                    <div class="w-6 h-6"></div> <!-- Spacer for alignment -->
                  {/if}

                  {#if showTestCases && testCaseCount > 0}
                    <span class="inline-flex items-center gap-0.5 px-1.5 py-0.5 text-xs font-medium rounded-full" style="background-color: var(--ds-background-accent-green-subtler); color: var(--ds-accent-green);">
                      <FileCheck class="w-3 h-3" />
                      {testCaseCount}
                    </span>
                  {/if}
                </div>

                {#each listColumns as column (column.field_identifier)}
                  <div
                    class="min-w-0 overflow-hidden"
                    data-testid={column.field_identifier === indentColumnId ? `tree-indent-${item.id}` : undefined}
                    style={column.field_identifier === indentColumnId ? `padding-left: ${getIndentLevel(item.level)}` : undefined}
                  >
                    <ListCellRenderer
                      {item}
                      {column}
                      {workspace}
                      canEdit={workspacePermissions.canEdit(item.workspace_id)}
                      {statuses}
                      {statusCategories}
                      {priorities}
                      {milestones}
                      {iterations}
                      {users}
                      {projects}
                      {itemTypes}
                      {customFieldDefinitions}
                      onitemUpdated={handleItemUpdated}
                      onupdateError={handleUpdateError}
                    />
                  </div>
                {/each}
                <div></div>
              </div>
            {/if}
          {/each}
        </div>
      </div>

      {/if}

      <CollectionHierarchyLoadMore testid="tree-load-more" />
  </StaticViewBackground>
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
  workspaceId={selectedTestCaseWorkspaceId ?? workspaceId}
  onclose={() => { showTestCaseModal = false; selectedTestCaseId = null; selectedTestCaseWorkspaceId = null; }}
/>

<style>
  .tree-row:hover {
    background-color: var(--ds-background-neutral-hovered);
  }

  .expand-btn:hover {
    background-color: var(--ds-background-neutral-hovered);
  }

</style>
