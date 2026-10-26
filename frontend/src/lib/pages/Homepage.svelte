<script>
  import { onMount, onDestroy, tick } from 'svelte';
  import { authStore } from '../stores/auth.svelte.js';
  import { homepageStore } from '../stores/homepageStore.svelte.js';
  import { permissionStore, isSystemAdmin } from '../stores/permissions.svelte.js';
  import { workspacesStore } from '../stores/workspaces.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import DashboardOnboarding from './DashboardOnboarding.svelte';
  import Text from '../components/Text.svelte';
  import Button from '../components/Button.svelte';
  import Input from '../components/Input.svelte';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronUp from '@lucide/svelte/icons/chevron-up';
  import Edit3 from '@lucide/svelte/icons/edit-3';
  import LayoutGrid from '@lucide/svelte/icons/layout-grid';
  import Plus from '@lucide/svelte/icons/plus';
  import Pencil from '@lucide/svelte/icons/pencil';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import X from '@lucide/svelte/icons/x';
  import { useEventListener } from 'runed';
  import { createDashboardWidgetDrag } from '../composables/dashboardWidgetDrag.js';
  import { confirm } from '../composables/useConfirm.js';

  import {
    getDashboardSectionDisplay,
    getDashboardSectionSaveValues,
    getDashboardWidgetMetadata,
    getDashboardWidgetTitle,
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
  import CreatedByMeWidget from '../widgets/dashboard/CreatedByMeWidget.svelte';
  import PersonalTasksWidget from '../widgets/dashboard/PersonalTasksWidget.svelte';
  import SupportOverviewWidget from '../widgets/SupportOverviewWidget.svelte';
  import SavedSearchWidget from '../widgets/dashboard/SavedSearchWidget.svelte';
  import CustomWidgetWidget from '../widgets/dashboard/CustomWidgetWidget.svelte';
  import WidgetBuilderModal from '../openui/WidgetBuilderModal.svelte';
  import { aiStore } from '../stores/aiStore.svelte.js';

  let greeting = $derived(homepageStore.greeting);
  let greetingText = $derived(greeting ? t(greeting) : '');
  let userName = $derived(authStore.currentUser?.first_name || '');
  let personalizedGreeting = $derived(
    greetingText
      ? userName
        ? t('dashboard.salutation.withName', { salutation: greetingText, name: userName })
        : t('dashboard.salutation.withoutName', { salutation: greetingText })
      : ''
  );
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
    dnd.cleanup();
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
    const created = homepageStore.addSection(t('dashboard.editor.newSection'), '');
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
    const display = getDashboardSectionDisplay(section, t);
    editingSectionId = section.id;
    editingSectionTitle = display.title;
    editingSectionSubtitle = display.subtitle || '';
    isNewSection = false;
  }

  function saveSection() {
    if (!editingSectionId) return;
    const draft = {
      title: editingSectionTitle,
      subtitle: editingSectionSubtitle,
    };
    const section = sections.find((candidate) => candidate.id === editingSectionId);
    homepageStore.updateSection(
      editingSectionId,
      section ? getDashboardSectionSaveValues(section, draft, t) : draft
    );
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
      message: t('dashboard.editor.deleteSectionConfirm'),
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

  const dnd = createDashboardWidgetDrag({
    widgetCardSelector: '[data-dashboard-widget-card]',
    dropZoneSelector: '[data-dashboard-drop-zone]',
    dragType: 'dashboard-widget-type',
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
      // A custom widget needs a definition first, so the builder decides what
      // to add once it saves.
      if (widgetType === 'custom-widget') {
        // The catalog hides this type when generation is unavailable; a stale
        // drag must not add an empty card.
        if (!aiStore.available || !aiStore.isFeatureEnabled('custom_widget')) return;
        builderSectionId = sectionId;
        builderWidgetId = null;
        builderOpen = true;
        return;
      }
      homepageStore.addWidgetToSection(sectionId, widgetType);
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

  let builderOpen = $state(false);
  let builderWidgetId = $state(null);
  let builderSectionId = $state(null);
  // Bumped after an edit so the card reloads its definition.
  let builderRefresh = $state(0);

  function handleWidgetSaved(widget) {
    if (!widget) return;
    if (builderWidgetId) {
      // Renaming in the builder must move the card header too.
      homepageStore.setCustomWidgetTitle(widget.id, widget.name);
      builderRefresh += 1;
      return;
    }
    if (builderSectionId) {
      homepageStore.addCustomWidgetToSection(builderSectionId, widget.id, widget.name);
    }
  }

  function getWidgetTitle(widget) {
    return getDashboardWidgetTitle(widget, t);
  }

  function openBuilderFor(widget) {
    builderWidgetId = Number(widget?.config?.customWidgetId) || null;
    builderSectionId = null;
    builderOpen = true;
  }

  function widgetMenuItems(widget) {
    if (widget?.type !== 'custom-widget' || !widget.config?.customWidgetId) return [];
    return [
      {
        title: t('dashboard.widgetBuilder.editTitle'),
        testid: 'widget-menu-edit',
        onClick: () => openBuilderFor(widget),
      },
    ];
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
  class="min-h-full w-full"
  style="{homepageStore.backgroundStyle} background-attachment: fixed;"
  data-testid="homepage"
  data-ready={layoutLoaded}
>
  <div class="max-w-7xl mx-auto px-6 pt-8 pb-6">
  <!-- Greeting + action buttons -->
  {#if !isOnboarding}
    <div class="mb-6 flex items-start justify-between gap-4">
      <div>
        <Text as="h1" size="2xl" weight="semibold" style={homepageStore.backgroundTextStyle}>
          {personalizedGreeting}
        </Text>
        <Text as="p" size="sm" variant="subtle" style={homepageStore.backgroundSubtleTextStyle}>{currentDate}</Text>
      </div>
      <div class="flex items-center gap-2 shrink-0">
        <Button
          variant={isEditMode ? 'primary' : 'default'}
          icon={isEditMode ? X : Edit3}
          onclick={toggleEditMode}
          dataTestid="dashboard-edit-toggle"
        >
          {isEditMode ? t('dashboard.editor.doneEditing') : t('common.edit')}
        </Button>
        <Button
          variant={isCustomizeMode ? 'primary' : 'default'}
          icon={isCustomizeMode ? X : LayoutGrid}
          onclick={toggleCustomizeMode}
          dataTestid="dashboard-customize-toggle"
        >
          {isCustomizeMode ? t('common.done') : t('dashboard.editor.customize')}
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
        {userName}
        ondismiss={handleOnboardingDismiss}
        {canCreateWorkspaces}
        {accessibleWorkspaces}
      />
    </div>
  {:else}
    <DashboardOnboarding
      workspaceCount={totalWorkspaceCount}
      itemCount={totalItemCount}
      {userName}
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
          <span>{t('dashboard.editor.editModeDescription')}</span>
        </div>
        <!-- shortcut-guard-exempt: contextual edit-mode action -->
        <Button
          variant="primary"
          size="small"
          icon={Plus}
          onclick={addSection}
          dataTestid="dashboard-add-section"
        >
          {t('dashboard.editor.addSection')}
        </Button>
      </div>
    {/if}

    <!-- Sections + widgets -->
    {#if layoutLoaded}
      <div class="space-y-10">
        {#each sections as section, sectionIndex (section.id)}
          {@const sectionWidgets = getSectionWidgets(section.id)}
          {@const sectionDisplay = getDashboardSectionDisplay(section, t)}
          <section
            id={`dashboard-section-${section.id}`}
            data-testid="dashboard-section"
            aria-label={sectionDisplay.title || t('dashboard.editor.sectionLabel')}
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
                    placeholder={t('dashboard.editor.sectionTitlePlaceholder')}
                    onkeydown={handleSectionEditKeydown}
                    dataTestid="dashboard-section-title-input"
                  />
                  <Input
                    type="text"
                    bind:value={editingSectionSubtitle}
                    placeholder={t('dashboard.editor.sectionSubtitlePlaceholder')}
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
                    style={homepageStore.backgroundTextStyle}
                  >{sectionDisplay.title}</h2>
                  {#if sectionDisplay.subtitle}
                    <p class="text-sm mt-0.5" style={homepageStore.backgroundSubtleTextStyle}>{sectionDisplay.subtitle}</p>
                  {/if}
                </div>
                {#if isEditMode}
                  <div class="flex items-center gap-1">
                    <button
                      type="button"
                      class="p-2 rounded transition-colors hover:bg-[var(--ds-background-neutral-hovered)] disabled:cursor-not-allowed disabled:opacity-30"
                      style={homepageStore.backgroundSubtleTextStyle}
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
                      style={homepageStore.backgroundSubtleTextStyle}
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
                      style={homepageStore.backgroundSubtleTextStyle}
                      onclick={() => startEditingSection(section)}
                      title={t('dashboard.editor.renameSection')}
                      aria-label={t('dashboard.editor.renameSection')}
                    >
                      <Pencil class="h-4 w-4" />
                    </button>
                    <button
                      type="button"
                      class="p-2 rounded transition-colors hover:bg-[var(--ds-background-neutral-hovered)]"
                      style={homepageStore.backgroundSubtleTextStyle}
                      onclick={() => handleDeleteSection(section.id)}
                      title={t('dashboard.editor.deleteSection')}
                      aria-label={t('dashboard.editor.deleteSection')}
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
                   background-color: ${dropZoneStates.get(section.id)?.isOver ? 'var(--ds-surface-hovered)' : 'transparent'};
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
                      title={getWidgetTitle(widget)}
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
                      extraMenuItems={widgetMenuItems(widget)}
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
                      {:else if widget.type === 'created-by-me'}
                        <CreatedByMeWidget config={widget.config ?? {}} />
                      {:else if widget.type === 'personal-tasks'}
                        <PersonalTasksWidget config={widget.config ?? {}} />
                      {:else if widget.type === 'support-overview'}
                        <SupportOverviewWidget
                          config={widget.config ?? {}}
                          onconfigchange={(changes) => updateWidgetConfig(widget.id, changes)}
                        />
                      {:else if widget.type === 'custom-widget'}
                        <CustomWidgetWidget
                          config={widget.config ?? {}}
                          refreshKey={builderRefresh}
                          title={getWidgetTitle(widget)}
                        />
                      {:else if widget.type === 'saved-search'}
                        <SavedSearchWidget
                          config={widget.config ?? {}}
                          onconfigchange={(changes) => updateWidgetConfig(widget.id, changes)}
                        />
                      {:else}
                        <div class="text-center py-8 text-sm" style={homepageStore.backgroundSubtleTextStyle}>
                          {t('dashboard.editor.unknownWidgetType', { type: widget.type })}
                        </div>
                      {/if}
                    </WidgetWrapper>
                  {/each}
                </div>
              {:else}
                <div class="text-center py-8" style={homepageStore.backgroundSubtleTextStyle}>
                  <p class="text-sm">{t('dashboard.editor.noWidgets')}</p>
                  <p class="text-xs mt-1">{t('dashboard.editor.addWidgetsHint')}</p>
                </div>
              {/if}
            </div>
          </section>
        {/each}

        {#if sections.length === 0}
          <div class="flex flex-col items-center justify-center py-16" style={homepageStore.backgroundSubtleTextStyle}>
            <LayoutGrid class="h-16 w-16 mb-4 opacity-30" />
            <p class="text-lg font-medium">{t('dashboard.editor.noSections')}</p>
            <p class="text-sm mt-2">{t('dashboard.editor.addSectionsHint')}</p>
          </div>
        {/if}
      </div>
    {/if}
  {/if}
  </div>
</div>

<WidgetBuilderModal
  bind:isOpen={builderOpen}
  widgetId={builderWidgetId}
  workspaceId={null}
  onclose={() => {
    builderWidgetId = null;
    builderSectionId = null;
  }}
  onsaved={handleWidgetSaved}
/>
