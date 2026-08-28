<script>
  import { onMount, onDestroy, tick } from 'svelte';
  import { authStore, homepageStore, permissionStore, isSystemAdmin, workspacesStore } from '../stores';
  import { t } from '../stores/i18n.svelte.js';
  import DashboardOnboarding from './DashboardOnboarding.svelte';
  import Text from '../components/Text.svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import {
    ChevronDown,
    ChevronUp,
    Edit3,
    LayoutGrid,
    Plus,
    Pencil,
    Trash2,
    X,
  } from '@lucide/svelte';
  import { useEventListener } from 'runed';
  import { confirm } from '../composables/useConfirm.js';
  import { draggable, dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';

  import {
    getDashboardWidgetMetadata,
  } from '../services/dashboardWidgetRegistry.js';

  import WidgetWrapper from '../widgets/WidgetWrapper.svelte';
  import DashboardCustomizationSidebar from '../layout/DashboardCustomizationSidebar.svelte';

  // Dashboard widgets
  import DailyBriefingWidget from '../widgets/dashboard/DailyBriefingWidget.svelte';
  import WhatsNewWidget from '../widgets/dashboard/WhatsNewWidget.svelte';
  import YourActivityWidget from '../widgets/dashboard/YourActivityWidget.svelte';
  import QuickAccessWidget from '../widgets/dashboard/QuickAccessWidget.svelte';
  import UpcomingMilestonesWidget from '../widgets/dashboard/UpcomingMilestonesWidget.svelte';
  import WatchedItemsWidget from '../widgets/dashboard/WatchedItemsWidget.svelte';
  import RecentWorkspacesWidget from '../widgets/dashboard/RecentWorkspacesWidget.svelte';
  import AssignedToMeWidget from '../widgets/dashboard/AssignedToMeWidget.svelte';
  import PersonalTasksWidget from '../widgets/dashboard/PersonalTasksWidget.svelte';
  import SavedSearchWidget from '../widgets/dashboard/SavedSearchWidget.svelte';

  let greeting = $derived(homepageStore.greeting);
  let currentDate = $derived(homepageStore.currentDate);
  let totalWorkspaceCount = $derived(homepageStore.totalWorkspaceCount);
  let totalItemCount = $derived(homepageStore.totalItemCount);
  let isOnboarding = $derived(homepageStore.isOnboarding);

  let sections = $derived(homepageStore.sections);
  let isEditMode = $derived(homepageStore.isEditMode);
  let isCustomizeMode = $derived(homepageStore.isCustomizeMode);
  let layoutLoaded = $derived(homepageStore.layoutLoaded);

  let canCreateWorkspaces = $derived(
    $permissionStore.userPermissionKeys?.has('workspace.create') || $isSystemAdmin
  );
  let accessibleWorkspaces = $derived($workspacesStore.regularWorkspaces || []);

  // Customization sidebar category
  let customizationCategory = $state('activity');

  // Section editing
  let editingSectionId = $state(null);
  let editingSectionTitle = $state('');
  let editingSectionSubtitle = $state('');
  let isNewSection = $state(false);

  // Drag state
  let draggedWidget = $state(null);
  let dropZoneStates = $state(new Map());
  let dragCleanups = [];

  $effect(() => {
    homepageStore.setAccessibleWorkspaces(accessibleWorkspaces, canCreateWorkspaces);
  });

  function handleOnboardingDismiss() {
    homepageStore.dismissOnboarding();
  }

  onMount(async () => {
    const userTimeZone = authStore.currentUser?.timezone || 'UTC';
    await homepageStore.init(userTimeZone);
  });

  onDestroy(() => {
    cleanupDragAndDrop();
  });

  function handleRefresh() {
    homepageStore.refresh();
  }

  useEventListener(() => window, 'refresh-workspaces', handleRefresh);
  useEventListener(() => window, 'refresh-work-items', handleRefresh);

  // --- Layout / mode handlers ---

  function toggleEditMode() {
    // If exiting edit mode while creating a new section, keep or discard it.
    if (homepageStore.isEditMode && isNewSection && editingSectionId) {
      if (editingSectionTitle.trim()) {
        saveSection();
      } else {
        homepageStore.deleteSection(editingSectionId);
      }
    }
    editingSectionId = null;
    isNewSection = false;
    homepageStore.toggleEditMode();
  }

  function toggleCustomizeMode() {
    homepageStore.toggleCustomizeMode();
  }

  async function addSection() {
    const created = homepageStore.addSection(t('dashboard.sections.new'), '');
    editingSectionId = created.id;
    editingSectionTitle = created.title;
    editingSectionSubtitle = created.subtitle;
    isNewSection = true;

    await tick();
    const titleInput = /** @type {HTMLInputElement | null} */ (
      document.getElementById(`dashboard-section-title-${created.id}`)
    );
    titleInput?.focus({ preventScroll: true });
    titleInput?.select();
    document.getElementById(`dashboard-section-${created.id}`)?.scrollIntoView({
      behavior: window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth',
      block: 'center',
    });
  }

  function startEditingSection(section) {
    editingSectionId = section.id;
    editingSectionTitle = section.title;
    editingSectionSubtitle = section.subtitle || '';
    isNewSection = false;
  }

  function saveSection() {
    if (!editingSectionId) return;
    homepageStore.updateSection(editingSectionId, {
      title: editingSectionTitle,
      subtitle: editingSectionSubtitle,
    });
    editingSectionId = null;
    isNewSection = false;
  }

  function cancelEditingSection() {
    if (isNewSection && editingSectionId) {
      homepageStore.deleteSection(editingSectionId);
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

  async function handleDeleteSection(sectionId) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('dashboard.sections.deleteMessage'),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger',
    });
    if (!confirmed) return;
    homepageStore.deleteSection(sectionId);
  }

  function removeWidget(widgetId) {
    homepageStore.removeWidget(widgetId);
  }

  function updateWidgetWidth(widgetId, newWidth) {
    return homepageStore.updateWidgetWidth(widgetId, newWidth);
  }

  function updateWidgetConfig(widgetId, configChanges) {
    homepageStore.updateWidgetConfig(widgetId, configChanges);
  }

  // --- Drag and drop ---

  let dragSetupKey = $derived(isCustomizeMode ? customizationCategory : null);
  $effect(() => {
    if (dragSetupKey === null) {
      cleanupDragAndDrop();
      return;
    }
    const id = setTimeout(() => setupDragAndDrop(), 350);
    return () => clearTimeout(id);
  });

  function setupDragAndDrop() {
    cleanupDragAndDrop();

    const widgetCards = /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll('[data-dashboard-widget-card]'));
    widgetCards.forEach((cardElement) => {
      const cleanup = draggable({
        element: cardElement,
        getInitialData: () => ({
          type: 'dashboard-widget-type',
          widgetType: cardElement.dataset.widgetType,
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
        },
      });
      dragCleanups.push(cleanup);
    });

    const sectionDropZones = /** @type {NodeListOf<HTMLElement>} */ (document.querySelectorAll('[data-dashboard-drop-zone]'));
    sectionDropZones.forEach((element) => {
      const sectionId = element.dataset.sectionId;
      dropZoneStates.set(sectionId, { isOver: false });

      const cleanup = dropTargetForElements({
        element,
        canDrop: ({ source }) => source.data.type === 'dashboard-widget-type',
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
          if (data.type === 'dashboard-widget-type') {
            homepageStore.addWidgetToSection(sectionId, data.widgetType);
          }
        },
      });
      dragCleanups.push(cleanup);
    });
  }

  function cleanupDragAndDrop() {
    dragCleanups.forEach((cleanup) => cleanup());
    dragCleanups = [];
  }

  function getWidgetTitle(type) {
    const widget = getDashboardWidgetMetadata(type);
    return widget?.nameKey ? t(widget.nameKey) : widget?.name || type;
  }

  function getSectionTitle(section) {
    if (section.id === 'default-your-day' || section.title === 'Your Day') return t('dashboard.sections.yourDay');
    if (section.id === 'default-work' || section.title === 'Work') return t('dashboard.sections.work');
    if (section.id === 'default-workspaces' || section.title === 'Workspaces') return t('dashboard.sections.workspaces');
    return section.title;
  }

  function getSectionSubtitle(section) {
    if (section.id === 'default-your-day' || section.subtitle === 'A quick read on what needs your attention') return t('dashboard.sections.yourDayDescription');
    if (section.id === 'default-work' || section.subtitle === 'Your personal list and items assigned to you') return t('dashboard.sections.workDescription');
    if (section.id === 'default-workspaces' || section.subtitle === 'Jump back in') return t('dashboard.sections.workspacesDescription');
    return section.subtitle;
  }

  function getSectionWidgets(sectionId) {
    return homepageStore.getWidgetsForSection(sectionId);
  }
</script>

<DashboardCustomizationSidebar
  bind:isOpen={homepageStore.isCustomizeMode}
  bind:activeCategory={customizationCategory}
/>

<div
  class="max-w-7xl mx-auto px-6 pt-8 pb-6"
  style="background-color: var(--ds-surface);"
  data-testid="homepage"
  data-ready={layoutLoaded}
>
  <!-- Greeting + action buttons -->
  {#if !isOnboarding}
    <div class="mb-6 flex items-start justify-between gap-4">
      <div>
        <Text as="h1" size="2xl" weight="semibold">
          {greeting}, {authStore.currentUser?.first_name || t('dashboard.there')}!
        </Text>
        <Text as="p" size="sm" variant="subtle">{currentDate}</Text>
      </div>
      <div class="flex items-center gap-2 shrink-0">
        <Button
          variant={isEditMode ? 'primary' : 'default'}
          icon={isEditMode ? X : Edit3}
          onclick={toggleEditMode}
          dataTestid="dashboard-edit-toggle"
        >
          {isEditMode ? t('dashboard.doneEditing') : t('common.edit')}
        </Button>
        <Button
          variant={isCustomizeMode ? 'primary' : 'default'}
          icon={isCustomizeMode ? X : LayoutGrid}
          onclick={toggleCustomizeMode}
        >
          {isCustomizeMode ? t('common.done') : t('dashboard.customize')}
        </Button>
      </div>
    </div>
  {/if}

  <!-- Onboarding -->
  {#if isOnboarding}
    <div class="max-w-2xl mx-auto">
      <DashboardOnboarding
        workspaceCount={totalWorkspaceCount}
        itemCount={totalItemCount}
        userName={authStore.currentUser?.first_name || t('dashboard.there')}
        ondismiss={handleOnboardingDismiss}
        {canCreateWorkspaces}
        {accessibleWorkspaces}
      />
    </div>
  {:else}
    <DashboardOnboarding
      workspaceCount={totalWorkspaceCount}
      itemCount={totalItemCount}
      userName={authStore.currentUser?.first_name || t('dashboard.there')}
      ondismiss={handleOnboardingDismiss}
      {canCreateWorkspaces}
      {accessibleWorkspaces}
    />
  {/if}

  {#if !isOnboarding}
    <!-- Edit mode banner -->
    {#if isEditMode}
      <div
        class="mb-4 p-3 border rounded flex items-center justify-between"
        style="background-color: var(--ds-status-info-bg); border-color: var(--ds-status-info-border);"
      >
        <div class="flex items-center gap-2 text-sm" style="color: var(--ds-status-info-text);">
          <Edit3 class="h-4 w-4" />
          <span>{t('dashboard.editModeHint')}</span>
        </div>
        <!-- shortcut-guard-exempt: contextual edit-mode action -->
        <Button
          variant="primary"
          size="small"
          icon={Plus}
          onclick={addSection}
          dataTestid="dashboard-add-section"
        >
          {t('layout.addSection')}
        </Button>
      </div>
    {/if}

    <!-- Sections + widgets -->
    {#if layoutLoaded}
      <div class="space-y-10">
        {#each sections as section, sectionIndex (section.id)}
          {@const sectionWidgets = getSectionWidgets(section.id)}
          <section
            id={`dashboard-section-${section.id}`}
            data-testid="dashboard-section"
            aria-label={getSectionTitle(section) || t('dashboard.sections.section')}
          >
            <!-- Section header -->
            <div class="flex items-center justify-between mb-4">
              {#if editingSectionId === section.id}
                <div class="flex-1 flex items-center gap-2 flex-wrap">
                  <Input
                    id={`dashboard-section-title-${section.id}`}
                    type="text"
                    bind:value={editingSectionTitle}
                    class="text-lg font-semibold"
                    placeholder={t('dashboard.sections.titlePlaceholder')}
                    onkeydown={handleSectionEditKeydown}
                    dataTestid="dashboard-section-title-input"
                  />
                  <Input
                    type="text"
                    bind:value={editingSectionSubtitle}
                    placeholder={t('dashboard.sections.subtitlePlaceholder')}
                    onkeydown={handleSectionEditKeydown}
                    size="small"
                  />
                  <Button variant="primary" size="small" onclick={saveSection}>
                    {t('common.save')} <span class="ml-1 opacity-60">⏎</span>
                  </Button>
                  <Button variant="default" size="small" onclick={cancelEditingSection}>
                    {t('common.cancel')} <span class="ml-1 opacity-60">Esc</span>
                  </Button>
                </div>
              {:else}
                <div>
                  <h2
                    id={`dashboard-section-heading-${section.id}`}
                    class="text-lg font-semibold"
                    style="color: var(--ds-text);"
                  >{getSectionTitle(section)}</h2>
                  {#if section.subtitle}
                    <p class="text-sm mt-0.5" style="color: var(--ds-text-subtle);">{getSectionSubtitle(section)}</p>
                  {/if}
                </div>
                {#if isEditMode}
                  <div class="flex items-center gap-1">
                    <button
                      type="button"
                      class="p-2 rounded transition-colors hover:bg-[var(--ds-background-neutral-hovered)] disabled:cursor-not-allowed disabled:opacity-30"
                      style="color: var(--ds-text-subtle);"
                      onclick={() => homepageStore.moveSection(section.id, -1)}
                      disabled={sectionIndex === 0}
                      title={t('layout.moveUp')}
                      aria-label={t('layout.moveUp')}
                      data-testid="dashboard-section-move-up"
                    >
                      <ChevronUp class="h-4 w-4" />
                    </button>
                    <button
                      type="button"
                      class="p-2 rounded transition-colors hover:bg-[var(--ds-background-neutral-hovered)] disabled:cursor-not-allowed disabled:opacity-30"
                      style="color: var(--ds-text-subtle);"
                      onclick={() => homepageStore.moveSection(section.id, 1)}
                      disabled={sectionIndex === sections.length - 1}
                      title={t('layout.moveDown')}
                      aria-label={t('layout.moveDown')}
                      data-testid="dashboard-section-move-down"
                    >
                      <ChevronDown class="h-4 w-4" />
                    </button>
                    <button
                      type="button"
                      class="p-2 rounded transition-colors hover:bg-[var(--ds-background-neutral-hovered)]"
                      style="color: var(--ds-text-subtle);"
                      onclick={() => startEditingSection(section)}
                      title={t('dashboard.sections.rename')}
                    >
                      <Pencil class="h-4 w-4" />
                    </button>
                    <button
                      type="button"
                      class="p-2 rounded transition-colors hover:bg-[var(--ds-background-neutral-hovered)]"
                      style="color: var(--ds-text-subtle);"
                      onclick={() => handleDeleteSection(section.id)}
                      title={t('layout.deleteSection')}
                    >
                      <Trash2 class="h-4 w-4" />
                    </button>
                  </div>
                {/if}
              {/if}
            </div>

            <!-- Drop zone -->
            <div
              class="section-drop-zone min-h-[6rem] rounded transition-all"
              class:border-2={draggedWidget && isCustomizeMode}
              class:border-dashed={draggedWidget && isCustomizeMode}
              style="{draggedWidget && isCustomizeMode
                ? `border-color: ${dropZoneStates.get(section.id)?.isOver ? 'var(--ds-border-focused)' : 'var(--ds-border)'};
                   ${dropZoneStates.get(section.id)?.isOver ? 'box-shadow: 0 0 0 2px var(--ds-border-focused);' : ''}
                   background-color: ${dropZoneStates.get(section.id)?.isOver ? 'var(--ds-surface-hover)' : 'transparent'};
                   padding: 0.5rem;`
                : ''}"
              data-dashboard-drop-zone
              data-section-id={section.id}
            >
              {#if sectionWidgets.length > 0}
                <div class="grid grid-cols-12 gap-4">
                  {#each sectionWidgets as widget (widget.id)}
                    {@const resizeBounds = homepageStore.getWidgetResizeBounds(widget.id)}
                    <WidgetWrapper
                      title={getWidgetTitle(widget.type)}
                      widgetId={widget.id}
                      widgetType={widget.type}
                      width={widget.width}
                      config={widget.config ?? {}}
                      resizeMinWidth={resizeBounds?.minWidth}
                      resizeMaxWidth={resizeBounds?.maxWidth}
                      isEditing={isCustomizeMode || isEditMode}
                      onremove={() => removeWidget(widget.id)}
                      onwidthchange={(newWidth) => updateWidgetWidth(widget.id, newWidth)}
                      onconfigchange={(changes) => updateWidgetConfig(widget.id, changes)}
                    >
                      {#if widget.type === 'daily-briefing'}
                        <DailyBriefingWidget />
                      {:else if widget.type === 'whats-new'}
                        <WhatsNewWidget />
                      {:else if widget.type === 'your-activity'}
                        <YourActivityWidget />
                      {:else if widget.type === 'quick-access'}
                        <QuickAccessWidget />
                      {:else if widget.type === 'upcoming-milestones'}
                        <UpcomingMilestonesWidget />
                      {:else if widget.type === 'watched-items'}
                        <WatchedItemsWidget />
                      {:else if widget.type === 'recent-workspaces'}
                        <RecentWorkspacesWidget />
                      {:else if widget.type === 'assigned-to-me'}
                        <AssignedToMeWidget config={widget.config ?? {}} />
                      {:else if widget.type === 'personal-tasks'}
                        <PersonalTasksWidget config={widget.config ?? {}} />
                      {:else if widget.type === 'saved-search'}
                        <SavedSearchWidget
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
                <div class="text-center py-8" style="color: var(--ds-text-subtle);">
                  <p class="text-sm">{t('dashboard.sections.noWidgets')}</p>
                  <p class="text-xs mt-1">{t('dashboard.sections.addWidgetsHint')}</p>
                </div>
              {/if}
            </div>
          </section>
        {/each}

        {#if sections.length === 0}
          <div class="flex flex-col items-center justify-center py-16" style="color: var(--ds-text-subtle);">
            <LayoutGrid class="h-16 w-16 mb-4 opacity-30" />
            <p class="text-lg font-medium">{t('dashboard.sections.none')}</p>
            <p class="text-sm mt-2">{t('dashboard.sections.noneHint')}</p>
          </div>
        {/if}
      </div>
    {/if}
  {/if}
</div>
