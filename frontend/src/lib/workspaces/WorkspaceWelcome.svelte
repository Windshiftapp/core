<script>
  import { onDestroy, untrack } from 'svelte';
  import { get } from 'svelte/store';
  import { useDebouncedAction } from '../composables/useDebouncedAction.svelte.js';
  import { workspaces } from '../api/workspaces.js';
  import { collections } from '../api/collections.js';
  import { workspaceDataStore } from '../stores/workspaceDataStore.svelte.js';
  import { overviewChartData, overviewChartRange } from './overviewChartData.js';
  import {
    useGradientStyles,
    loadWorkspaceGradient,
    hydrateWorkspaceGradientLayout,
    workspaceGradientIndex,
    applyToAllViews as applyToAllViewsStore,
    workspaceBackgroundImageUrl
  } from '../stores/workspaceGradient.svelte.js';

  import Edit3 from '@lucide/svelte/icons/edit-3';
  import Plus from '@lucide/svelte/icons/plus';
  import X from '@lucide/svelte/icons/x';
  import LayoutGrid from '@lucide/svelte/icons/layout-grid';
  import Pencil from '@lucide/svelte/icons/pencil';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import { t } from '../stores/i18n.svelte.js';
  import { formatDateOnly } from '../utils/dateFormatter.js';
  import { createId } from '../utils/uuid.js';
  import { errorToast, surfaceError } from '../stores/toasts.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import { createDashboardWidgetDrag } from '../composables/dashboardWidgetDrag.js';
  import { useDashboardCustomization } from '../composables/useDashboardCustomization.svelte.js';
  import {
    WORKSPACE_WIDGET_GRID_COLUMNS,
    clampWidgetWidth,
    getDefaultWidth,
    getWidgetMetadata,
    getWidgetMaxWidth,
    getWidgetMinWidth
  } from '../services/widgetRegistry.js';
  import {
    captureWorkspaceWidgetWidths,
    normalizeWorkspaceWidgets,
    restoreRejectedWorkspaceWidgetWidths
  } from '../services/workspaceWidgetLayout.js';
  import Button from '../components/Button.svelte';
  import Card from '../components/Card.svelte';
  import StateDisplay from '../components/StateDisplay.svelte';
  import Input from '../components/Input.svelte';
  import ViewHeader from '../layout/ViewHeader.svelte';
  import StaticViewBackground from '../layout/StaticViewBackground.svelte';

  // Widget components
  import WidgetWrapper from '../widgets/WidgetWrapper.svelte';
  import StatsCardWidget from '../widgets/StatsCardWidget.svelte';
  import CompletionChartWidget from '../widgets/CompletionChartWidget.svelte';
  import CreatedChartWidget from '../widgets/CreatedChartWidget.svelte';
  import MilestoneProgressWidget from '../widgets/MilestoneProgressWidget.svelte';
  import RecentItemsWidget from '../widgets/RecentItemsWidget.svelte';
  import MyTasksWidget from '../widgets/MyTasksWidget.svelte';
  import OverdueItemsWidget from '../widgets/OverdueItemsWidget.svelte';
  import UpcomingDeadlinesWidget from '../widgets/UpcomingDeadlinesWidget.svelte';
  import IterationTimelineWidget from '../widgets/IterationTimelineWidget.svelte';
  import TestCoverageWidget from '../widgets/TestCoverageWidget.svelte';
  import ZammadSupportWidget from '../widgets/ZammadSupportWidget.svelte';
  import SavedSearchWidget from '../widgets/dashboard/SavedSearchWidget.svelte';
  import StoryPointsWidget from '../widgets/StoryPointsWidget.svelte';
	import SupportMetricsWidget from '../widgets/SupportMetricsWidget.svelte';

  // Customization sidebar
  import WorkspaceCustomizationSidebar from './WorkspaceCustomizationSidebar.svelte';

  let { workspaceId, collectionId = null } = $props();

  let workspace = $state(null);
  let statusCategories = $state([]);
  let stats = $state({
    totalCollections: 0,
    itemsByStatusCategory: {},
    totalItems: 0
  });
  let completedByWeekData = $state([]);
  let createdLast7DaysData = $state([]);
  let milestones = $state([]);
  let loading = $state(true);
  let currentCollectionName = $state('Default');
  let collectionFilter = $state(null);
  let dataLoadVersion = $state(0);
  let dataController = new AbortController();
  let activeScope = $state(null);
  let loadError = $state(false);
  let chartLoading = $state(false);
  let chartError = $state(false);
  let chartRetry = $state(0);

  // Homepage layout state
  let sections = $state([]);
  let widgets = $state([]);
  let isEditMode = $state(false);
  let isCustomizeMode = $state(false);
  let customizationCategory = $state('built-in');
  let savePending = $state(false);
  let savedWidgetWidths = new Map();

  // Drag state
  let draggedWidget = $state(null);
  let dropZoneStates = $state(new Map()); // Map<sectionId, { isOver: boolean }>

  // Initialize gradient styles from global stores
  const gradientStyles = useGradientStyles();


  // Default section layout
  function getDefaultSections() {
    return [
      {
        id: createId(),
        title: t('workspaceDashboard.overview'),
        subtitle: t('workspaceDashboard.overviewDescription'),
        system_key: 'overview',
        display_order: 0,
        widget_ids: []
      },
      {
        id: createId(),
        title: t('workspaceDashboard.progress'),
        subtitle: t('workspaceDashboard.progressDescription'),
        system_key: 'progress',
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

    const widget1 = { id: createId(), type: 'stats', section_id: section1Id, position: 0, width: 3, config: {} };
    const widget2 = { id: createId(), type: 'completion-chart', section_id: section2Id, position: 0, width: 2, config: {} };
    const widget3 = { id: createId(), type: 'created-chart', section_id: section2Id, position: 1, width: 1, config: {} };
    const widget4 = { id: createId(), type: 'milestone-progress', section_id: section2Id, position: 2, width: 3, config: {} };

    // Update section widget_ids
    sections[0].widget_ids = [widget1.id];
    sections[1].widget_ids = [widget2.id, widget3.id, widget4.id];

    return [widget1, widget2, widget3, widget4];
  }

  let lastLoadKey = null;

  let loadKey = $derived(workspaceId ? `${workspaceId}-${collectionId ?? 'default'}` : null);

  $effect(() => {
    const key = loadKey;
    if (key && key !== lastLoadKey) {
      lastLoadKey = key;
      untrack(() => void loadData());
    }
  });

  // Setup drag and drop when in customize mode
  const dnd = createDashboardWidgetDrag({
    widgetCardSelector: '[data-widget-card]',
    dropZoneSelector: '[data-section-drop-zone]',
    dragType: 'widget-type',
    setDraggedWidget: (value) => {
      draggedWidget = value;
    },
    setDropZoneState: (sectionId, state) => {
      dropZoneStates.set(sectionId, state);
      dropZoneStates = new Map(dropZoneStates);
    },
    resetDropZones: () => {
      dropZoneStates = new Map();
    },
    onDropWidget: (sectionId, widgetType) => {
      addWidgetToSection(sectionId, widgetType);
    },
  });

  let dragSetupKey = $derived(isCustomizeMode ? customizationCategory : null);
  $effect(() => {
    if (dragSetupKey === null) {
      dnd.cleanup();
      return;
    }
    dnd.schedule();
    return () => dnd.cleanup();
  });

  async function loadData() {
    if (!workspaceId) return;
    const currentVersion = ++dataLoadVersion;
    const targetWorkspace = workspaceId;
    const targetCollection = collectionId;
    dataController.abort();
    const controller = new AbortController();
    dataController = controller;
    activeScope = null;
    loadError = false;
    loading = true;
    workspace = null;
    statusCategories = [];
    sections = [];
    widgets = [];
    stats = { totalCollections: 0, itemsByStatusCategory: {}, totalItems: 0 };
    milestones = [];
    try {
      const [snapshot, collection, layout] = await Promise.all([
        workspaceDataStore.getReferenceSnapshot(targetWorkspace),
        targetCollection ? collections.get(targetCollection, { signal: controller.signal }) : null,
        loadWorkspaceGradient(targetWorkspace)
      ]);
      if (controller.signal.aborted || currentVersion !== dataLoadVersion) return;
      if (!snapshot?.workspace || (targetCollection && !collection)) throw new Error('Overview scope unavailable');
      workspace = snapshot.workspace;
      statusCategories = snapshot.status_categories ?? [];
      currentCollectionName = collection?.name || 'Default';
      collectionFilter = collection?.ql_query?.trim() || null;
      if (layout?.sections?.length > 0) {
        sections = [...layout.sections].sort((a, b) => a.display_order - b.display_order);
        widgets = normalizeWorkspaceWidgets(layout.widgets);
      } else {
        sections = getDefaultSections();
        widgets = getDefaultWidgets();
      }
      savedWidgetWidths = captureWorkspaceWidgetWidths(widgets);
      activeScope = { workspaceId: targetWorkspace, collectionId: targetCollection, version: currentVersion };
    } catch (error) {
      if (controller.signal.aborted || currentVersion !== dataLoadVersion) return;
      loadError = true;
      console.error('Failed to load workspace data:', error);
    } finally {
      if (currentVersion === dataLoadVersion) {
        loading = false;
      }
    }
  }

  const needsStats = $derived(widgets.some(widget => widget.type === 'stats' || widget.type === 'milestone-progress'));
  const needsCreated = $derived(widgets.some(widget => widget.type === 'created-chart'));
  const needsCompleted = $derived(widgets.some(widget => widget.type === 'completion-chart'));

  $effect(() => {
    const scope = activeScope;
    if (!scope || !needsStats) return;
    const controller = new AbortController();
    void loadStats(scope.workspaceId, scope.collectionId, scope.version, controller.signal);
    return () => controller.abort();
  });

  async function loadStats(targetWorkspace, targetCollection, version, signal) {
    try {
      const statsData = await workspaces.getStats(targetWorkspace, targetCollection ? { collection_id: targetCollection } : {}, { signal });
      if (signal.aborted || version !== dataLoadVersion) return;
      stats.totalCollections = statsData.total_collections || 0;
      stats.totalItems = statsData.total_items || 0;
      stats.itemsByStatusCategory = statsData.items_by_status_category || {};
      milestones = Array.isArray(statsData.milestone_progress) ? statsData.milestone_progress : [];
    } catch (error) {
      if (signal.aborted || version !== dataLoadVersion) return;
      console.error('Failed to load stats:', error);
    }
  }

  $effect.pre(() => {
    const scope = activeScope;
    if (!scope || (!needsCreated && !needsCompleted)) return;
    void chartRetry;
    const controller = new AbortController();
    void loadChartData(scope.workspaceId, scope.collectionId, needsCreated, needsCompleted, scope.version, controller.signal);
    return () => controller.abort();
  });

  async function loadChartData(targetWorkspace, targetCollection, created, completed, version, signal) {
    chartLoading = true;
    chartError = false;
    completedByWeekData = [];
    createdLast7DaysData = [];
    try {
      const range = overviewChartRange(new Date(), completed);
      const response = await workspaces.getOverviewCharts(targetWorkspace, {
        ...range,
        ...(targetCollection ? { collection_id: targetCollection } : {}),
        series: [created && 'created', completed && 'completed'].filter(Boolean).join(',')
      }, { signal });
      if (signal.aborted || version !== dataLoadVersion) return;
      const chart = overviewChartData(response.buckets, range.to);
      completedByWeekData = chart.completed.map(point => ({ ...point, label: t('workspaceDashboard.weekOf', {
        date: formatDateOnly(point.date, { month: 'short', day: 'numeric' })
      }) }));
      createdLast7DaysData = chart.created;
    } catch (error) {
      if (signal.aborted || version !== dataLoadVersion) return;
      chartError = true;
      console.error('Failed to load chart data:', error);
    } finally {
      if (!signal.aborted && version === dataLoadVersion) chartLoading = false;
    }
  }

  let destroyed = false;
  let queuedLayout = null;

  function captureLayout() {
    return {
      sections: sections.map((s, display_order) => ({ ...s, display_order })),
      widgets: widgets.map((w, position) => ({ ...w, position })),
      gradient: get(workspaceGradientIndex),
      applyToAllViews: get(applyToAllViewsStore),
      backgroundImageUrl: get(workspaceBackgroundImageUrl) || ''
    };
  }

  async function saveHomepageLayout(layout = captureLayout()) {
    if (savePending) {
      queuedLayout = layout;
      return;
    }
    savePending = true;
    let attemptedLayout = null;

    try {
      attemptedLayout = layout;

      await workspaces.updateHomepageLayout(workspaceId, attemptedLayout);
      if (!destroyed) hydrateWorkspaceGradientLayout(workspaceId, attemptedLayout);
      savedWidgetWidths = captureWorkspaceWidgetWidths(attemptedLayout.widgets);
    } catch (error) {
      console.error('Failed to save homepage layout:', error);
      if (attemptedLayout) {
        widgets = restoreRejectedWorkspaceWidgetWidths(
          widgets,
          attemptedLayout.widgets,
          savedWidgetWidths
        );
      }
      errorToast(t('dialogs.alerts.failedToSaveLayout'));
    } finally {
      savePending = false;
      if (queuedLayout) {
        const next = queuedLayout;
        queuedLayout = null;
        void saveHomepageLayout(next);
      }
    }
  }

  const debouncedSave = useDebouncedAction(() => saveHomepageLayout(), 1000);

  // Section editing. The workspace dashboard keeps its own local state and
  // debounced save, so persistence is injected.
  const customization = useDashboardCustomization({
    updateSection: (sectionId, values) => {
      sections = sections.map(s => (s.id === sectionId ? { ...s, ...values } : s));
      debouncedSave();
    },
    removeSection: (sectionId) => {
      sections = sections.filter(s => s.id !== sectionId);
    },
    findSection: (sectionId) => sections.find(s => s.id === sectionId),
    getSectionDisplay: (section) => ({ title: section.title, subtitle: section.subtitle }),
    getSectionSaveValues: (_section, draft) => draft,
  });

  // Mode toggles
  function toggleEditMode() {
    isEditMode = !isEditMode;
    if (!isEditMode) {
      // If exiting edit mode while creating a new section, keep it when the
      // user typed a title and discard it otherwise.
      customization.commitOrDiscardDraft();
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
      id: createId(),
      title: t('dashboard.sections.new'),
      subtitle: '',
      display_order: sections.length,
      widget_ids: []
    };
    sections = [...sections, newSection];
    customization.beginNewSection(newSection);
  }

  async function deleteSection(sectionId) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('dashboard.sections.deleteMessage'),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;

    // Remove widgets in this section
    widgets = widgets.filter(w => w.section_id !== sectionId);

    // Remove section
    sections = sections.filter(s => s.id !== sectionId);

    debouncedSave();
  }

  // Widget management
  function addWidgetToSection(sectionId, widgetType) {
    const newWidget = {
      id: createId(),
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
    const widget = widgets.find(w => w.id === widgetId);
    if (!widget) return null;
    const width = clampWidgetWidth(widget.type, newWidth);
    widgets = widgets.map(w =>
      w.id === widgetId ? { ...w, width } : w
    );
    debouncedSave();
    return width;
  }

  function updateWidgetConfig(widgetId, configChanges) {
    widgets = widgets.map((widget) =>
      widget.id === widgetId
        ? { ...widget, config: { ...(widget.config ?? {}), ...configChanges } }
        : widget
    );
    debouncedSave();
  }

  onDestroy(() => {
    destroyed = true;
    dataLoadVersion += 1;
    dataController.abort();
    dnd.cleanup();
    void Promise.resolve(debouncedSave.runScheduledNow()).catch(error => {
      surfaceError(error, { context: 'saveWorkspaceLayoutOnLeave' });
    });
  });

  function getWidgetTitle(type) {
    const widget = getWidgetMetadata(type);
    return widget?.nameKey ? t(widget.nameKey) : widget?.name || type;
  }

  function getSectionTitle(section) {
    if (section.system_key === 'overview' || section.title === 'Overview') return t('workspaceDashboard.overview');
    if (section.system_key === 'progress' || section.title === 'Progress Tracking') return t('workspaceDashboard.progress');
    return section.title;
  }

  function getSectionSubtitle(section) {
    if (section.system_key === 'overview' || section.subtitle === 'Key metrics and statistics') return t('workspaceDashboard.overviewDescription');
    if (section.system_key === 'progress' || section.subtitle === 'Charts and timelines') return t('workspaceDashboard.progressDescription');
    return section.subtitle;
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

<StaticViewBackground
  backgroundStyle={gradientStyles.backgroundStyle}
  contextVars={gradientStyles.contextVars}
  class="workspace-welcome-wrapper"
  contentClass="workspace-welcome p-6"
  testid="workspace-welcome"
>
    {#if loading}
      <StateDisplay type="loading" message={t('workspaceDashboard.loading')} />
    {:else if loadError}
      <StateDisplay type="error" message={t('workspaceDashboard.loadError')} onRetry={() => void loadData()} dataTestid="workspace-overview-error" />
    {:else}
    <!-- Header -->
    <ViewHeader
      viewName={workspace?.name || t('common.workspace')}
      workspaceName={t('workspaceDashboard.homepage')}
      collection={currentCollectionName !== 'Default' ? currentCollectionName : ''}
      hasGradient={gradientStyles.hasCustomBackground}
      textStyle={gradientStyles.textStyle}
      subtleTextStyle={gradientStyles.subtleTextStyle}
    >
      {#snippet actions()}
        <div class="flex items-center gap-2">
          <!-- Edit button -->
          <Button
            variant={isEditMode ? 'primary' : 'default'}
            icon={isEditMode ? X : Edit3}
            onclick={toggleEditMode}
            dataTestid="workspace-dashboard-edit-toggle"
          >
            {isEditMode ? t('workspaceDashboard.doneEditing') : t('common.edit')}
          </Button>

          <!-- Customize button -->
          <Button
            variant={isCustomizeMode ? 'primary' : 'default'}
            icon={isCustomizeMode ? X : LayoutGrid}
            onclick={toggleCustomizeMode}
            dataTestid="workspace-dashboard-customize"
          >
            {isCustomizeMode ? t('common.done') : t('workspaceDashboard.customize')}
          </Button>
        </div>
      {/snippet}
    </ViewHeader>

    <!-- Edit mode notice -->
    {#if isEditMode}
      <div class="mb-4 p-3 border rounded flex items-center justify-between" style="background-color: var(--ds-status-info-bg); border-color: var(--ds-status-info-border);">
        <div class="flex items-center gap-2 text-sm" style="color: var(--ds-status-info-text);">
          <Edit3 class="h-4 w-4" />
          <span>{t('workspaceDashboard.editModeHint')}</span>
        </div>
        <!-- shortcut-guard-exempt: section creation is available only inside dashboard edit mode -->
        <Button
          variant="primary"
          size="small"
          icon={Plus}
          onclick={addSection}
          dataTestid="workspace-add-section"
        >
          {t('layout.addSection')}
        </Button>
      </div>
    {/if}

    <!-- Sections -->
    <div class="space-y-6">
      {#each sections as section (section.id)}
        {@const sectionWidgets = getWidgetsForSection(section.id)}
        <Card glass padding="spacious" dataTestid="workspace-section">
          <!-- Section header -->
          <div class="flex items-center justify-between mb-4">
            {#if customization.editingSectionId === section.id}
              <!-- Editing mode -->
              <div class="flex-1 flex items-center gap-2">
                <Input
                  type="text"
                  bind:value={customization.editingSectionTitle}
                  class="text-lg font-semibold"
                  placeholder={t('dashboard.sections.titlePlaceholder')}
                  onkeydown={customization.handleSectionEditKeydown}
                  dataTestid="workspace-section-title-input"
                />
                <Input
                  type="text"
                  bind:value={customization.editingSectionSubtitle}
                  placeholder={t('dashboard.sections.subtitlePlaceholder')}
                  onkeydown={customization.handleSectionEditKeydown}
                  size="small"
                />
                <Button
                  variant="primary"
                  size="small"
                  onclick={customization.save}
                >
                  {t('common.save')} <span class="ml-1 opacity-60">⏎</span>
                </Button>
                <Button
                  variant="default"
                  size="small"
                  onclick={customization.cancel}
                >
                  {t('common.cancel')} <span class="ml-1 opacity-60">Esc</span>
                </Button>
              </div>
            {:else}
              <!-- Display mode -->
              <div>
                <h2 class="text-xl font-semibold" style={gradientStyles.glassTextStyle}>{getSectionTitle(section)}</h2>
                {#if section.subtitle}
                  <p class="text-sm mt-1" style={gradientStyles.glassSubtleTextStyle}>{getSectionSubtitle(section)}</p>
                {/if}
              </div>

              {#if isEditMode}
                <div class="flex items-center gap-2">
                  <button
                    class="section-edit p-2 rounded"
                    onclick={() => customization.startEditing(section)}
                    title={t('dashboard.sections.rename')}
                    data-testid="workspace-section-edit"
                  >
                    <Pencil class="h-4 w-4" />
                  </button>
                  <button
                    class="p-2 rounded hover-danger" style="color: var(--ds-text-subtle);"
                    onclick={() => deleteSection(section.id)}
                    title={t('layout.deleteSection')}
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
            style="{draggedWidget && isCustomizeMode
              ? `border-color: ${dropZoneStates.get(section.id)?.isOver ? 'var(--ds-border-focused)' : (gradientStyles.hasCustomBackground ? 'rgba(255, 255, 255, 0.3)' : 'var(--ds-border)')};
                 ${dropZoneStates.get(section.id)?.isOver ? 'box-shadow: 0 0 0 2px var(--ds-border-focused);' : ''}
                 background-color: ${dropZoneStates.get(section.id)?.isOver ? 'var(--ds-surface-hovered)' : 'transparent'};
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
                    widgetType={widget.type}
                    width={widget.width}
                    gridColumns={WORKSPACE_WIDGET_GRID_COLUMNS}
                    resizeMinWidth={getWidgetMinWidth(widget.type)}
                    resizeMaxWidth={getWidgetMaxWidth(widget.type)}
                    resizeDefaultWidth={getDefaultWidth(widget.type)}
                    config={widget.config ?? {}}
                    isEditing={isCustomizeMode}
                    onremove={() => removeWidget(widget.id)}
                    onwidthchange={(newWidth) => updateWidgetWidth(widget.id, newWidth)}
                    onconfigchange={(changes) => updateWidgetConfig(widget.id, changes)}
                  >
                    {#if widget.type === 'stats'}
                      <StatsCardWidget {stats} {statusCategories} />
                    {:else if widget.type === 'completion-chart' || widget.type === 'created-chart'}
                      {#if chartLoading}
                        <StateDisplay type="loading" dataTestid="workspace-chart-loading" />
                      {:else if chartError}
                        <StateDisplay type="error" message={t('workspaceDashboard.chartError')} onRetry={() => chartRetry += 1} dataTestid="workspace-chart-error" />
                      {:else if widget.type === 'completion-chart'}
                        <CompletionChartWidget chartData={completedByWeekData} />
                      {:else}
                        <CreatedChartWidget chartData={createdLast7DaysData} />
                      {/if}
                    {:else if widget.type === 'milestone-progress'}
                      <MilestoneProgressWidget {milestones} />
                    {:else if widget.type === 'story-points-by-assignee'}
                      <StoryPointsWidget {workspaceId} />
                    {:else if widget.type === 'support-metrics'}
                      <SupportMetricsWidget
                        {workspaceId}
                        {collectionId}
                        config={widget.config ?? {}}
                      />
                    {:else if widget.type === 'recent-items'}
                      <RecentItemsWidget {workspaceId} {collectionFilter} />
                    {:else if widget.type === 'my-tasks'}
                      <MyTasksWidget {workspaceId} {collectionFilter} />
                    {:else if widget.type === 'overdue-items'}
                      <OverdueItemsWidget {workspaceId} collectionFilter={collectionFilter} />
                    {:else if widget.type === 'upcoming-deadlines'}
                      <UpcomingDeadlinesWidget {workspaceId} {collectionFilter} />
                    {:else if widget.type === 'iteration-timeline'}
                      <IterationTimelineWidget {workspaceId} />
                    {:else if widget.type === 'test-coverage'}
                      <TestCoverageWidget {workspaceId} collectionId={collectionId} />
                    {:else if widget.type === 'zammad-support-overview'}
                      <ZammadSupportWidget {workspaceId} />
                    {:else if widget.type === 'saved-search'}
                      <SavedSearchWidget
                        {workspaceId}
                        config={widget.config ?? {}}
                        onconfigchange={(changes) => updateWidgetConfig(widget.id, changes)}
                      />
                    {:else}
                      <div class="text-center py-8 text-sm" style="color: var(--ds-text-subtle);">
                        {t('dashboard.widgets.unknown', { type: widget.type })}
                      </div>
                    {/if}
                  </WidgetWrapper>
                {/each}
              </div>
            {:else}
              <div class="text-center py-12" style={gradientStyles.emptyStateStyle}>
                <p class="text-sm">{t('dashboard.sections.noWidgets')}</p>
                <p class="text-xs mt-1">{t('dashboard.sections.addWidgetsHint')}</p>
              </div>
            {/if}
          </div>
        </Card>
      {/each}
    </div>

    {#if sections.length === 0}
      <div class="flex flex-col items-center justify-center py-16" style={gradientStyles.emptyStateStyle}>
        <LayoutGrid class="h-16 w-16 mb-4 opacity-30" />
        <p class="text-lg font-medium">{t('dashboard.sections.none')}</p>
        <p class="text-sm mt-2">{t('workspaceDashboard.noSectionsHint')}</p>
      </div>
    {/if}
    {/if}
</StaticViewBackground>

<style>
  /* Workspace homepage wrapper with gradient background */
  :global(.workspace-welcome-wrapper) {
    width: 100%;
    min-height: 100vh;
    position: relative;
  }

  /* Add subtle pattern overlay for depth */
  :global(.workspace-welcome-wrapper::before) {
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

  /* Ensure content appears above the gradient overlay */
  :global(.workspace-welcome) {
    position: relative;
    z-index: 1;
  }

  .section-drop-zone {
    position: relative;
  }

  .section-edit {
    color: var(--ds-text-subtle);
    transition: color 0.15s;
  }

  .section-edit:hover {
    color: var(--ds-interactive);
  }
</style>
