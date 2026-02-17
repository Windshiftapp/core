<script>
  import { onDestroy } from 'svelte';
  import { get } from 'svelte/store';
  import { api } from '../api.js';
  import {
    useGradientStyles,
    loadWorkspaceGradient,
    workspaceGradientIndex,
    applyToAllViews as applyToAllViewsStore,
    workspaceBackgroundImageUrl
  } from '../stores/workspaceGradient.svelte.js';

  const shortDateFormat = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric' });
  import { getCollection } from '../features/collections/collectionService.js';
  import { Edit3, Plus, X, LayoutGrid, GripVertical, Pencil, Trash2 } from 'lucide-svelte';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { getDefaultWidth } from '../services/widgetRegistry.js';
  import Button from '../components/Button.svelte';
  import Card from '../components/Card.svelte';
  import ViewHeader from '../layout/ViewHeader.svelte';

  // Widget components
  import WidgetWrapper from '../widgets/WidgetWrapper.svelte';
  import StatsCardWidget from '../widgets/StatsCardWidget.svelte';
  import CompletionChartWidget from '../widgets/CompletionChartWidget.svelte';
  import CreatedChartWidget from '../widgets/CreatedChartWidget.svelte';
  import MilestoneProgressWidget from '../widgets/MilestoneProgressWidget.svelte';
  import RecentItemsWidget from '../widgets/RecentItemsWidget.svelte';
  import MyTasksWidget from '../widgets/MyTasksWidget.svelte';
  import OverdueItemsWidget from '../widgets/OverdueItemsWidget.svelte';
  import ItemFilterWidget from '../widgets/ItemFilterWidget.svelte';
  import SavedSearchWidget from '../widgets/SavedSearchWidget.svelte';
  import UpcomingDeadlinesWidget from '../widgets/UpcomingDeadlinesWidget.svelte';
  import SprintTimelineWidget from '../widgets/SprintTimelineWidget.svelte';
  import TestCoverageWidget from '../widgets/TestCoverageWidget.svelte';

  // Customization sidebar
  import WorkspaceCustomizationSidebar from './WorkspaceCustomizationSidebar.svelte';

  export let workspaceId;
  export let collectionId = null;

  let workspace = null;
  let statusCategories = [];
  let stats = {
    totalCollections: 0,
    itemsByStatusCategory: {},
    totalItems: 0
  };
  let completedByWeekData = [];
  let createdLast7DaysData = [];
  let milestones = [];
  let loading = true;
  let currentCollectionName = 'Default';
  let collectionFilter = null;
  let dataLoadVersion = 0;

  // Homepage layout state
  let sections = [];
  let widgets = [];
  let isEditMode = false;
  let isCustomizeMode = false;
  let customizationCategory = 'built-in';
  let setupCleanups = [];
  let savePending = false;

  // Drag state
  let draggedWidget = null;
  let dropZoneStates = new Map(); // Map<sectionId, { isOver: boolean }>

  // Initialize gradient styles from global stores
  const gradientStyles = useGradientStyles();

  // Section editing
  let editingSectionId = null;
  let editingSectionTitle = '';
  let editingSectionSubtitle = '';
  let isNewSection = false; // Track if we're creating a new section

  // Default section layout
  function getDefaultSections() {
    return [
      {
        id: crypto.randomUUID(),
        title: 'Overview',
        subtitle: 'Key metrics and statistics',
        display_order: 0,
        widget_ids: []
      },
      {
        id: crypto.randomUUID(),
        title: 'Progress Tracking',
        subtitle: 'Charts and timelines',
        display_order: 1,
        widget_ids: []
      }
    ];
  }

  // Default widgets for new workspaces
  function getDefaultWidgets() {
    const section1Id = sections[0]?.id;
    const section2Id = sections[1]?.id;

    if (!section1Id || !section2Id) return [];

    const widget1 = { id: crypto.randomUUID(), type: 'stats', section_id: section1Id, position: 0, width: 3, config: {} };
    const widget2 = { id: crypto.randomUUID(), type: 'completion-chart', section_id: section2Id, position: 0, width: 2, config: {} };
    const widget3 = { id: crypto.randomUUID(), type: 'created-chart', section_id: section2Id, position: 1, width: 1, config: {} };
    const widget4 = { id: crypto.randomUUID(), type: 'milestone-progress', section_id: section2Id, position: 2, width: 3, config: {} };

    // Update section widget_ids
    sections[0].widget_ids = [widget1.id];
    sections[1].widget_ids = [widget2.id, widget3.id, widget4.id];

    return [widget1, widget2, widget3, widget4];
  }

  const normalizeStatusName = value => (typeof value === 'string' ? value.toLowerCase().trim() : '');

  function resolveItemStatus(item, statusMapById, statusMapByName) {
    if (!item || !statusMapById || !statusMapByName) return null;
    if (item.status_id && statusMapById.has(item.status_id)) {
      return statusMapById.get(item.status_id);
    }
    const normalizedName = normalizeStatusName(item.status);
    if (!normalizedName) return null;
    return statusMapByName.get(normalizedName) || null;
  }

  function isItemCompleted(item, statusMapById, statusMapByName, completedCategoryIds) {
    const status = resolveItemStatus(item, statusMapById, statusMapByName);
    if (!status || !status.category_id) return false;
    return completedCategoryIds.has(status.category_id);
  }

  let lastLoadKey = null;

  $: if (workspaceId) {
    const currentKey = `${workspaceId}-${collectionId ?? 'default'}`;
    if (currentKey !== lastLoadKey) {
      lastLoadKey = currentKey;
      loadData();
    }
  }

  // Setup drag and drop when in customize mode
  let dragSetupKey = null;
  $: dragSetupKey = isCustomizeMode ? customizationCategory : null;
  $: if (dragSetupKey !== null) {
    setTimeout(() => setupDragAndDrop(), 350);
  } else {
    cleanupDragAndDrop();
  }

  async function loadData() {
    if (!workspaceId) return;
    const currentVersion = ++dataLoadVersion;
    loading = true;
    try {
      const filter = await resolveCollectionContext();
      if (currentVersion !== dataLoadVersion) return;
      collectionFilter = filter;

      await loadWorkspace();
      await loadStatusCategories();
      await Promise.all([
        loadStats(),
        loadChartData(),
        loadHomepageLayout()
      ]);
    } catch (error) {
      console.error('Failed to load workspace data:', error);
    } finally {
      if (currentVersion === dataLoadVersion) {
        loading = false;
      }
    }
  }

  async function resolveCollectionContext() {
    if (!collectionId) {
      currentCollectionName = 'Default';
      return null;
    }

    try {
      const collection = await getCollection(collectionId);
      if (collection) {
        currentCollectionName = collection.name || 'Collection';
        const query = (collection.ql_query || '').trim();
        return query.length > 0 ? query : null;
      }
      currentCollectionName = 'Default';
      return null;
    } catch (error) {
      console.error('Failed to load collection context:', error);
      currentCollectionName = 'Default';
      return null;
    }
  }

  async function loadWorkspace() {
    try {
      workspace = await api.workspaces.get(workspaceId);
    } catch (error) {
      console.error('Failed to load workspace:', error);
    }
  }

  async function loadStatusCategories() {
    try {
      statusCategories = await api.statusCategories.getAll();
    } catch (error) {
      console.error('Failed to load status categories:', error);
    }
  }

  async function loadStats() {
    try {
      const params = {};
      if (collectionId) {
        params.collection_id = collectionId;
      }
      const statsData = await api.workspaces.getStats(workspaceId, params);
      stats.totalCollections = statsData.total_collections || 0;
      stats.totalItems = statsData.total_items || 0;
      stats.itemsByStatusCategory = statsData.items_by_status_category || {};
      milestones = Array.isArray(statsData.milestone_progress) ? statsData.milestone_progress : [];
    } catch (error) {
      console.error('Failed to load stats:', error);
    }
  }

  async function loadChartData() {
    try {
      const filters = { workspace_id: workspaceId, limit: 5000 };
      if (collectionId) {
        filters.collection_id = collectionId;
      }

      const itemsResponse = await api.items.getAll(filters);
      const items = Array.isArray(itemsResponse)
        ? itemsResponse
        : (itemsResponse?.items ?? []);
      const fetchedStatuses = await api.statuses.getAll();
      const statusList = Array.isArray(fetchedStatuses) ? fetchedStatuses : [];
      const localStatusById = new Map(
        statusList
          .filter(status => status?.id)
          .map(status => [status.id, status])
      );
      const localStatusByName = new Map(
        statusList
          .filter(status => status?.name)
          .map(status => [status.name.toLowerCase().trim(), status])
      );
      const localCompletedCategoryIds = new Set(
        statusCategories
          .filter(category => category?.is_completed)
          .map(category => category.id)
      );

      // Completed by week (last 4 weeks aligned to current week)
      const now = new Date();
      const startOfCurrentWeek = new Date(now);
      startOfCurrentWeek.setHours(0, 0, 0, 0);
      startOfCurrentWeek.setDate(startOfCurrentWeek.getDate() - startOfCurrentWeek.getDay());

      completedByWeekData = [];
      const totalWeeks = 4;
      const dayMs = 24 * 60 * 60 * 1000;
      for (let i = 0; i < totalWeeks; i++) {
        const offsetWeeks = totalWeeks - 1 - i;
        const weekStart = new Date(startOfCurrentWeek.getTime() - (offsetWeeks * 7 * dayMs));
        const weekEnd = new Date(weekStart.getTime() + (7 * dayMs));

        const completedCount = items.filter(item => {
          if (!isItemCompleted(item, localStatusById, localStatusByName, localCompletedCategoryIds)) return false;
          const updatedAt = new Date(item.updated_at);
          return updatedAt >= weekStart && updatedAt < weekEnd;
        }).length;

        completedByWeekData.push({
          date: new Date(weekStart),
          count: completedCount,
          label: `Week of ${shortDateFormat.format(weekStart)}`
        });
      }

      // Created last 7 days
      const sevenDaysAgo = new Date(now.getTime() - (7 * 24 * 60 * 60 * 1000));
      createdLast7DaysData = [];
      for (let i = 0; i < 7; i++) {
        const day = new Date(sevenDaysAgo.getTime() + (i * 24 * 60 * 60 * 1000));
        const nextDay = new Date(day.getTime() + (24 * 60 * 60 * 1000));

        const createdCount = items.filter(item => {
          const createdAt = new Date(item.created_at);
          return createdAt >= day && createdAt < nextDay;
        }).length;

        createdLast7DaysData.push({
          date: day,
          count: createdCount
        });
      }
    } catch (error) {
      console.error('Failed to load chart data:', error);
    }
  }

  async function loadHomepageLayout() {
    try {
      // Load gradient/background into global stores (handles gradient, applyToAllViews, backgroundImageUrl)
      await loadWorkspaceGradient(workspaceId);

      const layout = await api.workspaces.getHomepageLayout(workspaceId);
      if (layout && layout.sections && layout.sections.length > 0) {
        sections = layout.sections.sort((a, b) => a.display_order - b.display_order);
        widgets = layout.widgets || [];
      } else {
        // No layout exists, use defaults
        sections = getDefaultSections();
        widgets = getDefaultWidgets();
      }
    } catch (error) {
      console.error('Failed to load homepage layout:', error);
      sections = getDefaultSections();
      widgets = getDefaultWidgets();
    }
  }

  async function saveHomepageLayout() {
    if (savePending) return;
    savePending = true;

    try {
      const layout = {
        sections: sections.map((s, idx) => ({
          ...s,
          display_order: idx
        })),
        widgets: widgets.map((w, idx) => ({
          ...w,
          position: idx
        })),
        gradient: get(workspaceGradientIndex),
        applyToAllViews: get(applyToAllViewsStore),
        backgroundImageUrl: get(workspaceBackgroundImageUrl) || ''
      };

      await api.workspaces.updateHomepageLayout(workspaceId, layout);
    } catch (error) {
      console.error('Failed to save homepage layout:', error);
      alert('Failed to save layout changes');
    } finally {
      savePending = false;
    }
  }

  let saveTimeout;
  function debouncedSave() {
    clearTimeout(saveTimeout);
    saveTimeout = setTimeout(() => saveHomepageLayout(), 1000);
  }

  // Mode toggles
  function toggleEditMode() {
    isEditMode = !isEditMode;
    if (!isEditMode) {
      // If exiting edit mode while creating a new section, remove it
      if (isNewSection && editingSectionId) {
        sections = sections.filter(s => s.id !== editingSectionId);
      }
      editingSectionId = null;
      isNewSection = false;
      debouncedSave();
    }
    if (isEditMode && isCustomizeMode) {
      isCustomizeMode = false;
    }
  }

  function toggleCustomizeMode() {
    isCustomizeMode = !isCustomizeMode;
    if (isCustomizeMode && isEditMode) {
      isEditMode = false;
    }
  }

  // Section management
  function addSection() {
    const newSection = {
      id: crypto.randomUUID(),
      title: 'New Section',
      subtitle: '',
      display_order: sections.length,
      widget_ids: []
    };
    sections = [...sections, newSection];
    editingSectionId = newSection.id;
    editingSectionTitle = newSection.title;
    editingSectionSubtitle = newSection.subtitle;
    isNewSection = true; // Mark as new section
  }

  function startEditingSection(section) {
    editingSectionId = section.id;
    editingSectionTitle = section.title;
    editingSectionSubtitle = section.subtitle || '';
    isNewSection = false; // Editing existing section
  }

  function saveSection() {
    if (!editingSectionId) return;

    sections = sections.map(s =>
      s.id === editingSectionId
        ? { ...s, title: editingSectionTitle, subtitle: editingSectionSubtitle }
        : s
    );
    editingSectionId = null;
    isNewSection = false; // Reset after save
    debouncedSave();
  }

  function cancelEditingSection() {
    // If canceling a new section, remove it from the list
    if (isNewSection && editingSectionId) {
      sections = sections.filter(s => s.id !== editingSectionId);
    }

    editingSectionId = null;
    isNewSection = false;
  }

  function handleSectionEditKeydown(event) {
    if (event.key === 'Enter') {
      event.preventDefault();
      saveSection();
    } else if (event.key === 'Escape') {
      event.preventDefault();
      cancelEditingSection();
    }
  }

  function deleteSection(sectionId) {
    if (!confirm('Delete this section? All widgets in this section will be removed.')) return;

    // Remove widgets in this section
    widgets = widgets.filter(w => w.section_id !== sectionId);

    // Remove section
    sections = sections.filter(s => s.id !== sectionId);

    debouncedSave();
  }

  // Widget management
  function addWidgetToSection(sectionId, widgetType) {
    const newWidget = {
      id: crypto.randomUUID(),
      type: widgetType,
      section_id: sectionId,
      position: widgets.filter(w => w.section_id === sectionId).length,
      width: getDefaultWidth(widgetType),
      config: {}
    };

    widgets = [...widgets, newWidget];

    // Update section's widget_ids
    sections = sections.map(s =>
      s.id === sectionId
        ? { ...s, widget_ids: [...s.widget_ids, newWidget.id] }
        : s
    );

    debouncedSave();
  }

  function removeWidget(widgetId, options = {}) {
    const widget = widgets.find(w => w.id === widgetId);
    if (!widget) return;

    const sectionId = widget.section_id;

    // Remove from widgets
    widgets = widgets.filter(w => w.id !== widgetId);

    // Remove from section's widget_ids
    sections = sections.map(s =>
      s.id === sectionId
        ? { ...s, widget_ids: s.widget_ids.filter(id => id !== widgetId) }
        : s
    );

    if (!options.preventSave) {
      debouncedSave();
    }
  }

  function updateWidgetWidth(widgetId, newWidth) {
    widgets = widgets.map(w =>
      w.id === widgetId ? { ...w, width: newWidth } : w
    );
    debouncedSave();
  }

  // Drag and drop setup
  function setupDragAndDrop() {
    cleanupDragAndDrop();

    // Setup draggable widget cards in sidebar
    const widgetCards = document.querySelectorAll('[data-widget-card]');
    widgetCards.forEach(cardElement => {
      const cleanup = draggable({
        element: cardElement,
        getInitialData: () => ({
          type: 'widget-type',
          widgetType: cardElement.dataset.widgetType
        }),
        onDragStart: () => {
          const currentType = cardElement.dataset.widgetType;
          draggedWidget = currentType ? { type: currentType } : null;
          cardElement.style.opacity = '0.5';
        },
        onDrop: () => {
          draggedWidget = null;
          cardElement.style.opacity = '';
          dropZoneStates = new Map();
        }
      });
      setupCleanups.push(cleanup);
    });

    // Setup drop zones for sections
    const sectionDropZones = document.querySelectorAll('[data-section-drop-zone]');
    sectionDropZones.forEach(element => {
      const sectionId = element.dataset.sectionId;

      dropZoneStates.set(sectionId, { isOver: false });

      const cleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => source.data.type === 'widget-type',
        onDragEnter: () => {
          dropZoneStates.set(sectionId, { isOver: true });
          dropZoneStates = new Map(dropZoneStates);
        },
        onDragLeave: () => {
          dropZoneStates.set(sectionId, { isOver: false });
          dropZoneStates = new Map(dropZoneStates);
        },
        onDrop: ({ source }) => {
          dropZoneStates.set(sectionId, { isOver: false });
          dropZoneStates = new Map(dropZoneStates);

          const data = source.data;
          if (data.type === 'widget-type') {
            addWidgetToSection(sectionId, data.widgetType);
          }
        }
      });

      setupCleanups.push(cleanup);
    });
  }

  function cleanupDragAndDrop() {
    setupCleanups.forEach(cleanup => cleanup());
    setupCleanups = [];
  }

  onDestroy(() => {
    cleanupDragAndDrop();
    clearTimeout(saveTimeout);
  });

  // Get widget title
  function getWidgetTitle(type) {
    const titles = {
      'stats': 'Statistics Overview',
      'completion-chart': 'Items Completed (Last 4 Weeks)',
      'created-chart': 'Items Created (Last 7 Days)',
      'milestone-progress': 'Milestone Progress',
      'recent-items': 'Recent Items',
      'my-tasks': 'My Tasks',
      'overdue-items': 'Overdue Items',
      'item-filter': 'Item Filter',
      'saved-search': 'Saved Search',
      'upcoming-deadlines': 'Upcoming Deadlines',
      'sprint-timeline': 'Sprint Timeline'
    };
    return titles[type] || type;
  }

  // Get widgets for a section
  function getWidgetsForSection(sectionId) {
    return widgets
      .filter(w => w.section_id === sectionId)
      .sort((a, b) => a.position - b.position);
  }
</script>

<!-- Customization Sidebar -->
<WorkspaceCustomizationSidebar
  bind:isOpen={isCustomizeMode}
  bind:activeCategory={customizationCategory}
/>

<div class="workspace-welcome-wrapper" style="{gradientStyles.backgroundStyle}">
  <div class="workspace-welcome p-6">
    {#if loading}
      <div class="flex items-center justify-center h-64">
        <p style={gradientStyles.emptyStateStyle}>Loading workspace data...</p>
      </div>
    {:else}
    <!-- Header -->
    <ViewHeader
      viewName={workspace?.name || 'Workspace'}
      workspaceName="Homepage"
      collection={currentCollectionName !== 'Default' ? currentCollectionName : ''}
      hasGradient={gradientStyles.hasCustomBackground}
      textStyle={gradientStyles.textStyle}
      subtleTextStyle={gradientStyles.subtleTextStyle}
    >
      <svelte:fragment slot="actions">
        <div class="flex items-center gap-2">
          <!-- Edit button -->
          <Button
            variant={isEditMode ? 'primary' : 'default'}
            icon={isEditMode ? X : Edit3}
            onclick={toggleEditMode}
          >
            {isEditMode ? 'Done Editing' : 'Edit'}
          </Button>

          <!-- Customize button -->
          <Button
            variant={isCustomizeMode ? 'primary' : 'default'}
            icon={isCustomizeMode ? X : LayoutGrid}
            onclick={toggleCustomizeMode}
          >
            {isCustomizeMode ? 'Done' : 'Customize'}
          </Button>
        </div>
      </svelte:fragment>
    </ViewHeader>

    <!-- Edit mode notice -->
    {#if isEditMode}
      <div class="mb-4 p-3 bg-blue-50 border border-blue-200 rounded flex items-center justify-between">
        <div class="flex items-center gap-2 text-sm text-blue-800">
          <Edit3 class="h-4 w-4" />
          <span>Edit mode: Add, rename, or delete sections</span>
        </div>
        <Button
          variant="primary"
          size="small"
          icon={Plus}
          onclick={addSection}
        >
          Add Section
        </Button>
      </div>
    {/if}

    <!-- Sections -->
    <div class="space-y-6">
      {#each sections as section (section.id)}
        {@const sectionWidgets = getWidgetsForSection(section.id)}
        <Card glass padding="spacious">
          <!-- Section header -->
          <div class="flex items-center justify-between mb-4">
            {#if editingSectionId === section.id}
              <!-- Editing mode -->
              <div class="flex-1 flex items-center gap-2">
                <input
                  type="text"
                  bind:value={editingSectionTitle}
                  class="px-3 py-2 border border-gray-300 rounded text-lg font-semibold"
                  placeholder="Section title"
                  onkeydown={handleSectionEditKeydown}
                />
                <input
                  type="text"
                  bind:value={editingSectionSubtitle}
                  class="px-3 py-2 border border-gray-300 rounded text-sm"
                  placeholder="Subtitle (optional)"
                  onkeydown={handleSectionEditKeydown}
                />
                <Button
                  variant="primary"
                  size="small"
                  onclick={saveSection}
                >
                  Save <span class="ml-1 opacity-60">⏎</span>
                </Button>
                <Button
                  variant="default"
                  size="small"
                  onclick={cancelEditingSection}
                >
                  Cancel <span class="ml-1 opacity-60">Esc</span>
                </Button>
              </div>
            {:else}
              <!-- Display mode -->
              <div>
                <h2 class="text-xl font-semibold" style={gradientStyles.textStyle}>{section.title}</h2>
                {#if section.subtitle}
                  <p class="text-sm mt-1" style={gradientStyles.subtleTextStyle}>{section.subtitle}</p>
                {/if}
              </div>

              {#if isEditMode}
                <div class="flex items-center gap-2">
                  <button
                    class="p-2 text-gray-600 hover:text-blue-600 hover:bg-blue-50 rounded"
                    onclick={() => startEditingSection(section)}
                    title="Rename section"
                  >
                    <Pencil class="h-4 w-4" />
                  </button>
                  <button
                    class="p-2 text-gray-600 hover:text-red-600 hover:bg-red-50 rounded"
                    onclick={() => deleteSection(section.id)}
                    title="Delete section"
                  >
                    <Trash2 class="h-4 w-4" />
                  </button>
                </div>
              {/if}
            {/if}
          </div>

          <!-- Section drop zone -->
          <div
            class="section-drop-zone min-h-32 rounded transition-all"
            class:border-2={draggedWidget && isCustomizeMode}
            class:border-dashed={draggedWidget && isCustomizeMode}
            class:ring-2={dropZoneStates.get(section.id)?.isOver && draggedWidget && isCustomizeMode}
            class:ring-blue-400={dropZoneStates.get(section.id)?.isOver && draggedWidget && isCustomizeMode}
            style="{draggedWidget && isCustomizeMode
              ? `border-color: ${dropZoneStates.get(section.id)?.isOver ? '#60a5fa' : (gradientStyles.hasCustomBackground ? 'rgba(255, 255, 255, 0.3)' : '#d1d5db')};
                 background-color: ${dropZoneStates.get(section.id)?.isOver ? 'rgba(96, 165, 250, 0.1)' : 'transparent'};
                 padding: 0.5rem;`
              : ''}"
            data-section-drop-zone
            data-section-id={section.id}
          >
            {#if sectionWidgets.length > 0}
              <div class="grid grid-cols-3 gap-4">
                {#each sectionWidgets as widget (widget.id)}
                  <WidgetWrapper
                    title={getWidgetTitle(widget.type)}
                    widgetId={widget.id}
                    bind:width={widget.width}
                    isEditing={isCustomizeMode}
                    onremove={() => removeWidget(widget.id)}
                  >
                    {#if widget.type === 'stats'}
                      <StatsCardWidget {stats} {statusCategories} />
                    {:else if widget.type === 'completion-chart'}
                      <CompletionChartWidget chartData={completedByWeekData} />
                    {:else if widget.type === 'created-chart'}
                      <CreatedChartWidget chartData={createdLast7DaysData} />
                    {:else if widget.type === 'milestone-progress'}
                      <MilestoneProgressWidget {milestones} />
                    {:else if widget.type === 'recent-items'}
                      <RecentItemsWidget {workspaceId} {collectionFilter} />
                    {:else if widget.type === 'my-tasks'}
                      <MyTasksWidget {workspaceId} {collectionFilter} />
                    {:else if widget.type === 'overdue-items'}
                      <OverdueItemsWidget {workspaceId} collectionFilter={collectionFilter} />
                    {:else if widget.type === 'item-filter'}
                      <ItemFilterWidget {workspaceId} config={widget.config} />
                    {:else if widget.type === 'saved-search'}
                      <SavedSearchWidget {workspaceId} config={widget.config} />
                    {:else if widget.type === 'upcoming-deadlines'}
                      <UpcomingDeadlinesWidget {workspaceId} />
                    {:else if widget.type === 'sprint-timeline'}
                      <SprintTimelineWidget {workspaceId} />
                    {:else if widget.type === 'test-coverage'}
                      <TestCoverageWidget {workspaceId} collectionId={collectionId} />
                    {/if}
                  </WidgetWrapper>
                {/each}
              </div>
            {:else}
              <div class="text-center py-12" style={gradientStyles.emptyStateStyle}>
                <p class="text-sm">No widgets in this section yet</p>
                <p class="text-xs mt-1">Click "Customize" to add widgets</p>
              </div>
            {/if}
          </div>
        </Card>
      {/each}
    </div>

    {#if sections.length === 0}
      <div class="flex flex-col items-center justify-center py-16" style={gradientStyles.emptyStateStyle}>
        <LayoutGrid class="h-16 w-16 mb-4 opacity-30" />
        <p class="text-lg font-medium">No sections configured</p>
        <p class="text-sm mt-2">Click "Edit" to add sections to your homepage</p>
      </div>
    {/if}
    {/if}
  </div>
</div>

<style>
  /* Workspace homepage wrapper with gradient background */
  .workspace-welcome-wrapper {
    width: 100%;
    min-height: 100%;
    position: relative;
    background-size: 200% 200%;
    animation: gradient-shift 15s ease infinite;
  }

  /* Add subtle pattern overlay for depth */
  .workspace-welcome-wrapper::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background-image:
      radial-gradient(circle at 20% 50%, rgba(255, 255, 255, 0.1) 0%, transparent 50%),
      radial-gradient(circle at 80% 80%, rgba(255, 255, 255, 0.1) 0%, transparent 50%);
    pointer-events: none;
  }

  /* Floating glow orbs for visual interest */
  .workspace-welcome-wrapper::after {
    content: '';
    position: absolute;
    top: 10%;
    right: 10%;
    width: 300px;
    height: 300px;
    border-radius: 50%;
    background: radial-gradient(circle, rgba(255, 255, 255, 0.15), transparent 70%);
    filter: blur(60px);
    pointer-events: none;
    animation: glow-breathe 6s ease-in-out infinite;
  }

  /* Ensure content appears above the gradient overlay */
  .workspace-welcome {
    position: relative;
    z-index: 1;
    animation: fade-up var(--duration-slow, 300ms) var(--ease-smooth, ease) forwards;
  }

  .section-drop-zone {
    position: relative;
  }

  /* Reduced motion support */
  @media (prefers-reduced-motion: reduce) {
    .workspace-welcome-wrapper {
      animation: none;
    }
    .workspace-welcome-wrapper::after {
      animation: none;
    }
    .workspace-welcome {
      animation: none;
    }
  }
</style>
