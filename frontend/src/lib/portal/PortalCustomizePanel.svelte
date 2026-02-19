<script>
  import { onMount, onDestroy, tick } from 'svelte';
  import { draggable } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import {
    Palette, Navigation, X, TextCursorInput, BookOpen, Check,
    Plus, Trash2, Edit, MoreHorizontal, GripVertical,
    Package, Shield, Table2
  } from 'lucide-svelte';
  import Tooltip from '../components/Tooltip.svelte';
  import Spinner from '../components/Spinner.svelte';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import RequestTypeVisibilityModal from '../dialogs/RequestTypeVisibilityModal.svelte';
  import AssetReportVisibilityModal from '../dialogs/RequestTypeVisibilityModal.svelte';
  import GradientSelector from '../components/GradientSelector.svelte';
  import BackgroundImageSelector from '../components/BackgroundImageSelector.svelte';
  import LogoUploader from '../components/LogoUploader.svelte';
  import Label from '../components/Label.svelte';
  import { portalStore, gradients, iconMap } from '../stores/portal.svelte.js';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';

  let {
    onOpenFieldsModal = () => {},
    onOpenRequestTypeModal = () => {},
    onOpenAssetReportModal = () => {}
  } = $props();

  // Visibility modal state
  let showVisibilityModal = $state(false);
  let selectedRequestTypeForVisibility = $state(null);

  function openVisibilityModal(requestType) {
    selectedRequestTypeForVisibility = requestType;
    showVisibilityModal = true;
  }

  function closeVisibilityModal() {
    showVisibilityModal = false;
    selectedRequestTypeForVisibility = null;
  }

  async function handleVisibilitySaved() {
    await portalStore.loadRequestTypes();
  }

  function hasVisibilityRestrictions(requestType) {
    return (requestType.visibility_group_ids?.length > 0) || (requestType.visibility_org_ids?.length > 0);
  }

  // Asset Report visibility modal state
  let showAssetReportVisibilityModal = $state(false);
  let selectedAssetReportForVisibility = $state(null);

  function openAssetReportVisibilityModal(assetReport) {
    selectedAssetReportForVisibility = assetReport;
    showAssetReportVisibilityModal = true;
  }

  function closeAssetReportVisibilityModal() {
    showAssetReportVisibilityModal = false;
    selectedAssetReportForVisibility = null;
  }

  async function handleAssetReportVisibilitySaved() {
    await portalStore.loadAssetReports();
  }

  function hasAssetReportVisibilityRestrictions(report) {
    return (report.visibility_group_ids?.length > 0) || (report.visibility_org_ids?.length > 0);
  }

  async function deleteAssetReport(id) {
    if (!confirm(t('portal.customize.confirmDeleteAssetReport'))) return;

    try {
      await api.assetReports.delete(id);
      await portalStore.loadAssetReports();
    } catch (err) {
      console.error('Failed to delete asset report:', err);
    }
  }

  let showCustomizePanelHover = $state(false);

  async function deleteRequestType(id) {
    if (!confirm(t('portal.customize.confirmDeleteRequestType'))) return;

    try {
      await api.requestTypes.delete(id);
      await portalStore.loadRequestTypes();
    } catch (err) {
      console.error('Failed to delete request type:', err);
    }
  }

  // Drag-and-drop setup
  let cleanupFunctions = [];
  let lastRequestTypeIds = '';
  let lastAssetReportIds = '';

  function setupDraggables() {
    // Setup request type draggables
    if (portalStore.activeSection === 'request-types') {
      const cards = document.querySelectorAll('[data-request-type-card]');
      cards.forEach(card => {
        const dragHandle = card.querySelector('[data-drag-handle]');
        const requestTypeId = card.dataset.requestTypeId;
        const requestType = portalStore.requestTypes.find(rt => String(rt.id) === String(requestTypeId));

        if (!requestType || !dragHandle) return;

        const cleanup = draggable({
          element: card,
          dragHandle: dragHandle,
          getInitialData: () => ({
            type: 'request-type',
            requestType
          }),
          onDragStart: () => {
            portalStore.draggedRequestType = requestType;
            card.style.opacity = '0.5';
          },
          onDrop: () => {
            portalStore.draggedRequestType = null;
            card.style.opacity = '';
          }
        });
        cleanupFunctions.push(cleanup);
      });
    }

    // Setup asset report draggables
    if (portalStore.activeSection === 'asset-reports') {
      const cards = document.querySelectorAll('[data-asset-report-card]');
      cards.forEach(card => {
        const dragHandle = card.querySelector('[data-drag-handle]');
        const reportId = card.dataset.assetReportId;
        const report = portalStore.assetReports.find(ar => String(ar.id) === String(reportId));

        if (!report || !dragHandle) return;

        const cleanup = draggable({
          element: card,
          dragHandle: dragHandle,
          getInitialData: () => ({
            type: 'asset-report',
            assetReport: report
          }),
          onDragStart: () => {
            portalStore.draggedAssetReport = report;
            card.style.opacity = '0.5';
          },
          onDrop: () => {
            portalStore.draggedAssetReport = null;
            card.style.opacity = '';
          }
        });
        cleanupFunctions.push(cleanup);
      });
    }
  }

  onMount(() => {
    // Setup after DOM is ready
    setTimeout(setupDraggables, 100);
  });

  onDestroy(() => {
    cleanupFunctions.forEach(fn => fn());
    cleanupFunctions = [];
  });

  // Re-setup when request types or asset reports change or section changes
  $effect(() => {
    // Track dependencies
    const currentRequestTypeIds = portalStore.requestTypes.map(rt => rt.id).join(',');
    const currentAssetReportIds = portalStore.assetReports.map(ar => ar.id).join(',');
    const isRequestTypesSection = portalStore.activeSection === 'request-types';
    const isAssetReportsSection = portalStore.activeSection === 'asset-reports';

    const requestTypesChanged = currentRequestTypeIds !== lastRequestTypeIds;
    const assetReportsChanged = currentAssetReportIds !== lastAssetReportIds;

    if (requestTypesChanged || assetReportsChanged || isRequestTypesSection || isAssetReportsSection) {
      lastRequestTypeIds = currentRequestTypeIds;
      lastAssetReportIds = currentAssetReportIds;
      // Cleanup previous
      cleanupFunctions.forEach(fn => fn());
      cleanupFunctions = [];
      // Wait for DOM to update then re-setup
      setTimeout(setupDraggables, 100);
    }
  });
</script>

<!-- Customization Panel Overlay (hide when editing request types so sections are visible) -->
<ModalBackdrop
  show={portalStore.showCustomizePanel && portalStore.activeSection !== 'request-types'}
  opacity={0.3}
  blur={0}
  align="none"
  onclose={() => portalStore.showCustomizePanel = false}
/>

<!-- Customization Panel - Slides from Left -->
<div
  class="fixed top-0 left-0 h-full flex shadow-2xl z-50 transform transition-transform duration-300 ease-in-out"
  style="background-color: var(--ds-surface-card);"
  class:translate-x-0={portalStore.showCustomizePanel}
  class:-translate-x-full={!portalStore.showCustomizePanel}
>
  <!-- Vertical Navigation Sidebar -->
  <div class="w-16 border-r flex flex-col items-center py-4" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <!-- Hero Gradient Section -->
    <Tooltip content={t('portal.customize.heroGradient')} placement="right">
      {#snippet children()}
        <button
          onclick={() => portalStore.activeSection = 'hero-gradient'}
          class="w-10 h-10 rounded flex items-center justify-center cursor-pointer transition-all mb-1"
          style="background-color: {portalStore.activeSection === 'hero-gradient' ? 'var(--ds-background-neutral)' : 'transparent'};"
        >
          <Palette class="w-5 h-5" style="color: {portalStore.activeSection === 'hero-gradient' ? 'var(--ds-interactive, #2563eb)' : 'var(--ds-text-subtle)'};" />
        </button>
      {/snippet}
    </Tooltip>

    <!-- Navigation Section -->
    <Tooltip content={t('portal.customize.navigation')} placement="right">
      {#snippet children()}
        <button
          onclick={() => portalStore.activeSection = 'navigation'}
          class="w-10 h-10 rounded flex items-center justify-center cursor-pointer transition-all mb-1"
          style="background-color: {portalStore.activeSection === 'navigation' ? 'var(--ds-background-neutral)' : 'transparent'};"
        >
          <Navigation class="w-5 h-5" style="color: {portalStore.activeSection === 'navigation' ? 'var(--ds-interactive, #2563eb)' : 'var(--ds-text-subtle)'};" />
        </button>
      {/snippet}
    </Tooltip>


    <!-- Request Types Section -->
    <Tooltip content={t('portal.customize.requestTypes')} placement="right">
      {#snippet children()}
        <button
          onclick={() => portalStore.activeSection = 'request-types'}
          class="w-10 h-10 rounded flex items-center justify-center cursor-pointer transition-all mb-1"
          style="background-color: {portalStore.activeSection === 'request-types' ? 'var(--ds-background-neutral)' : 'transparent'};"
        >
          <TextCursorInput class="w-5 h-5" style="color: {portalStore.activeSection === 'request-types' ? 'var(--ds-interactive, #2563eb)' : 'var(--ds-text-subtle)'};" />
        </button>
      {/snippet}
    </Tooltip>

    <!-- Asset Reports Section (only show if asset sets exist) -->
    {#if portalStore.hasAssetSets}
      <Tooltip content={t('portal.customize.assetReports')} placement="right">
        {#snippet children()}
          <button
            onclick={() => portalStore.activeSection = 'asset-reports'}
            class="w-10 h-10 rounded flex items-center justify-center cursor-pointer transition-all mb-1"
            style="background-color: {portalStore.activeSection === 'asset-reports' ? 'var(--ds-background-neutral)' : 'transparent'};"
          >
            <Table2 class="w-5 h-5" style="color: {portalStore.activeSection === 'asset-reports' ? 'var(--ds-interactive, #2563eb)' : 'var(--ds-text-subtle)'};" />
          </button>
        {/snippet}
      </Tooltip>
    {/if}

    <!-- Knowledge Base Section -->
    <Tooltip content={t('portal.customize.knowledgeBase')} placement="right">
      {#snippet children()}
        <button
          onclick={() => portalStore.activeSection = 'knowledge-base'}
          class="w-10 h-10 rounded flex items-center justify-center cursor-pointer transition-all"
          style="background-color: {portalStore.activeSection === 'knowledge-base' ? 'var(--ds-background-neutral)' : 'transparent'};"
        >
          <BookOpen class="w-5 h-5" style="color: {portalStore.activeSection === 'knowledge-base' ? 'var(--ds-interactive, #2563eb)' : 'var(--ds-text-subtle)'};" />
        </button>
      {/snippet}
    </Tooltip>
  </div>

  <!-- Panel Content -->
  <div class="w-96 flex flex-col overflow-hidden">
    <!-- Panel Header -->
    <div class="border-b px-6 py-4 flex items-center justify-between" style="background-color: var(--ds-surface-card); border-color: var(--ds-border);">
      <div class="flex items-center gap-3">
        {#if portalStore.activeSection === 'hero-gradient'}
          <Palette class="w-5 h-5" style="color: var(--ds-text);" />
          <h2 class="text-lg font-semibold" style="color: var(--ds-text);">{t('portal.customize.heroGradient')}</h2>
        {:else if portalStore.activeSection === 'navigation'}
          <Navigation class="w-5 h-5" style="color: var(--ds-text);" />
          <h2 class="text-lg font-semibold" style="color: var(--ds-text);">{t('portal.customize.navigation')}</h2>
        {:else if portalStore.activeSection === 'request-types'}
          <TextCursorInput class="w-5 h-5" style="color: var(--ds-text);" />
          <h2 class="text-lg font-semibold" style="color: var(--ds-text);">{t('portal.customize.requestTypes')}</h2>
        {:else if portalStore.activeSection === 'asset-reports'}
          <Table2 class="w-5 h-5" style="color: var(--ds-text);" />
          <h2 class="text-lg font-semibold" style="color: var(--ds-text);">{t('portal.customize.assetReports')}</h2>
        {:else if portalStore.activeSection === 'knowledge-base'}
          <BookOpen class="w-5 h-5" style="color: var(--ds-text);" />
          <h2 class="text-lg font-semibold" style="color: var(--ds-text);">{t('portal.customize.knowledgeBase')}</h2>
        {/if}
      </div>
      <button
        onclick={() => portalStore.showCustomizePanel = false}
        class="p-2 rounded transition-all"
        style="background-color: {showCustomizePanelHover ? 'var(--ds-background-neutral)' : 'transparent'};"
        onmouseenter={() => showCustomizePanelHover = true}
        onmouseleave={() => showCustomizePanelHover = false}
      >
        <X class="w-5 h-5" style="color: var(--ds-text-subtle);" />
      </button>
    </div>

    <!-- Panel Content Area -->
    <div class="flex-1 overflow-y-auto p-6">
      {#if portalStore.activeSection === 'hero-gradient'}
        <div class="mb-6">
          <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text);">{t('portal.customize.background')}</h3>
          <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">{t('portal.customize.backgroundDescription')}</p>
        </div>

        <!-- Gradient Grid -->
        <div class="mb-6">
          <Label class="mb-3">{t('portal.customize.gradients')}</Label>
          <GradientSelector
            {gradients}
            selectedIndex={portalStore.selectedGradient}
            hasBackgroundImage={portalStore.hasBackgroundImage}
            onSelect={(index) => portalStore.selectGradient(index)}
            columns={6}
            size={25}
          />
        </div>

        <!-- Background Images -->
        <BackgroundImageSelector
          currentImageUrl={portalStore.backgroundImageUrl}
          selectedCategory={portalStore.selectedBackgroundCategory}
          onSelectImage={(url) => portalStore.selectBackgroundImage(url)}
          onRemoveImage={() => portalStore.removeBackgroundImage()}
          onUploadImage={(files) => portalStore.handleBackgroundUpload(files)}
          uploading={portalStore.uploadingBackground}
          uploadCategory="portal_background"
        />

        <!-- Logo Upload -->
        <div class="border-t pt-6 mt-6" style="border-color: var(--ds-border);">
          <LogoUploader
            currentLogoUrl={portalStore.logoUrl}
            onUpload={(files) => portalStore.handleLogoUpload(files)}
            onRemove={() => portalStore.removeLogo()}
            uploading={portalStore.uploadingLogo}
            label={t('portal.customize.logo')}
            helpText={t('portal.customize.logoHelp')}
          />
        </div>
      {:else if portalStore.activeSection === 'navigation'}
        <div class="text-sm" style="color: var(--ds-text-subtle);">
          {t('portal.customize.navigationComingSoon')}
        </div>
      {:else if portalStore.activeSection === 'request-types'}
        <!-- Request Types Management -->
        <div class="mb-6">
          <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text);">{t('portal.customize.requestTypes')}</h3>
          <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
            {t('portal.customize.requestTypesDescription')}
          </p>
        </div>

        {#if portalStore.loadingRequestTypes}
          <div class="flex items-center justify-center py-8">
            <Spinner />
          </div>
        {:else}
          <!-- Request Types List -->
          <div class="space-y-2 mb-4">
            {#each portalStore.requestTypes as requestType}
              {@const hasNoFields = requestType.field_count === 0}
              <div
                class="p-3 rounded border"
                style="background-color: {hasNoFields ? (portalStore.isDarkMode ? '#422006' : '#fffbeb') : (portalStore.isDarkMode ? '#334155' : '#f9fafb')}; border-color: {hasNoFields ? '#f59e0b' : (portalStore.isDarkMode ? '#475569' : '#e5e7eb')};"
                data-request-type-card
                data-request-type-id={requestType.id}
              >
                <div class="flex items-start gap-3">
                  <!-- Icon Preview -->
                  <div class="flex-shrink-0">
                    <div class="w-8 h-8 rounded flex items-center justify-center" style="background-color: {requestType.color || '#6b7280'};">
                      <svelte:component this={iconMap[requestType.icon] || Package} size={16} color="white" />
                    </div>
                  </div>

                  <!-- Drag Handle -->
                  <div class="cursor-grab active:cursor-grabbing pt-1" style="color: {portalStore.isDarkMode ? '#64748b' : '#9ca3af'};" data-drag-handle>
                    <GripVertical class="w-4 h-4" />
                  </div>

                  <!-- Content -->
                  <div class="flex-1 min-w-0">
                    <div class="font-medium text-sm mb-1" style="color: {portalStore.isDarkMode ? '#e2e8f0' : '#111827'};">
                      {requestType.name}
                    </div>
                    {#if requestType.description}
                      <div class="text-xs mb-2" style="color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};">
                        {requestType.description}
                      </div>
                    {/if}
                    <div>
                      <div class="text-xs" style="color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};">
                        <div class="font-medium" style="color: {portalStore.isDarkMode ? '#e2e8f0' : '#374151'};">{requestType.item_type_name || t('common.unknown')}</div>
                      </div>
                      <button
                        onclick={() => onOpenFieldsModal(requestType)}
                        class="text-xs hover:underline text-right"
                        style="color: {hasNoFields ? '#f59e0b' : (portalStore.isDarkMode ? '#60a5fa' : '#2563eb')};"
                      >
                        {#if hasNoFields}
                          <div class="font-medium">{t('portal.customize.addFields')}</div>
                        {:else}
                          <div>{t('portal.customize.fields')} ({requestType.field_count})</div>
                        {/if}
                      </button>
                    </div>
                  </div>

                  <!-- Visibility Button -->
                  <div class="flex-shrink-0">
                    <Tooltip content={hasVisibilityRestrictions(requestType) ? t('portal.visibility.hasRestrictions') : t('portal.visibility.noRestrictions')} placement="top">
                      {#snippet children()}
                        <button
                          onclick={() => openVisibilityModal(requestType)}
                          class="p-1.5 rounded transition-all hover:bg-black/5"
                          title={t('portal.visibility.configureVisibility')}
                        >
                          <Shield
                            class="w-4 h-4"
                            style="color: {hasVisibilityRestrictions(requestType) ? '#f59e0b' : (portalStore.isDarkMode ? '#94a3b8' : '#6b7280')};"
                          />
                        </button>
                      {/snippet}
                    </Tooltip>
                  </div>

                  <!-- Actions Dropdown -->
                  <div class="flex-shrink-0">
                    <DropdownMenu
                      triggerIcon={MoreHorizontal}
                      triggerClass="p-1.5 rounded hover:bg-black/5 transition-all"
                      triggerStyle="color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};"
                      showChevron={false}
                      iconOnly={true}
                      placement="bottom-end"
                      items={[
                        {
                          title: t('common.edit'),
                          icon: Edit,
                          onClick: () => onOpenRequestTypeModal('edit', requestType)
                        },
                        { type: 'divider' },
                        {
                          title: t('common.delete'),
                          icon: Trash2,
                          color: '#dc2626',
                          onClick: () => deleteRequestType(requestType.id)
                        }
                      ]}
                    />
                  </div>
                </div>
              </div>
            {/each}

            {#if portalStore.requestTypes.length === 0}
              <div class="text-center py-8">
                <p class="text-sm mb-4" style="color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};">
                  {t('portal.customize.noRequestTypes')}
                </p>
              </div>
            {/if}
          </div>

          <!-- Add Request Type Button -->
          <button
            onclick={() => onOpenRequestTypeModal('create')}
            class="w-full flex items-center justify-center gap-2 px-4 py-3 rounded border-2 border-dashed transition-all"
            style="border-color: {portalStore.isDarkMode ? '#475569' : '#d1d5db'}; color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};"
          >
            <Plus class="w-5 h-5" />
            <span class="font-medium">{t('portal.customize.addRequestType')}</span>
          </button>
        {/if}
      {:else if portalStore.activeSection === 'asset-reports'}
        <!-- Asset Reports Management -->
        <div class="mb-6">
          <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text);">{t('portal.customize.assetReports')}</h3>
          <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
            {t('portal.customize.assetReportsDescription')}
          </p>
        </div>

        {#if portalStore.loadingAssetReports}
          <div class="flex items-center justify-center py-8">
            <Spinner />
          </div>
        {:else}
          <!-- Asset Reports List -->
          <div class="space-y-2 mb-4">
            {#each portalStore.assetReports as report}
              <div
                class="p-3 rounded border"
                style="background-color: {portalStore.isDarkMode ? '#334155' : '#f9fafb'}; border-color: {portalStore.isDarkMode ? '#475569' : '#e5e7eb'};"
                data-asset-report-card
                data-asset-report-id={report.id}
              >
                <div class="flex items-start gap-3">
                  <!-- Icon Preview -->
                  <div class="flex-shrink-0">
                    <div class="w-8 h-8 rounded flex items-center justify-center" style="background-color: {report.color || '#6b7280'};">
                      <svelte:component this={iconMap[report.icon] || Table2} size={16} color="white" />
                    </div>
                  </div>

                  <!-- Drag Handle -->
                  <div class="cursor-grab active:cursor-grabbing pt-1" style="color: {portalStore.isDarkMode ? '#64748b' : '#9ca3af'};" data-drag-handle>
                    <GripVertical class="w-4 h-4" />
                  </div>

                  <!-- Content -->
                  <div class="flex-1 min-w-0">
                    <div class="font-medium text-sm mb-1 flex items-center gap-2" style="color: {portalStore.isDarkMode ? '#e2e8f0' : '#111827'};">
                      {report.name}
                      {#if !report.is_active}
                        <span
                          class="px-1.5 py-0.5 text-[10px] font-medium rounded"
                          style="background-color: {portalStore.isDarkMode ? 'rgba(156, 163, 175, 0.2)' : '#f3f4f6'}; color: {portalStore.isDarkMode ? '#9ca3af' : '#6b7280'};"
                        >
                          INACTIVE
                        </span>
                      {/if}
                    </div>
                    {#if report.description}
                      <div class="text-xs mb-1" style="color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};">
                        {report.description}
                      </div>
                    {/if}
                    <div class="text-xs" style="color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};">
                      {report.asset_set_name || t('common.unknown')}
                    </div>
                  </div>

                  <!-- Visibility Button -->
                  <div class="flex-shrink-0">
                    <Tooltip content={hasAssetReportVisibilityRestrictions(report) ? t('portal.visibility.hasRestrictions') : t('portal.visibility.noRestrictions')} placement="top">
                      {#snippet children()}
                        <button
                          onclick={() => openAssetReportVisibilityModal(report)}
                          class="p-1.5 rounded transition-all hover:bg-black/5"
                          title={t('portal.visibility.configureVisibility')}
                        >
                          <Shield
                            class="w-4 h-4"
                            style="color: {hasAssetReportVisibilityRestrictions(report) ? '#f59e0b' : (portalStore.isDarkMode ? '#94a3b8' : '#6b7280')};"
                          />
                        </button>
                      {/snippet}
                    </Tooltip>
                  </div>

                  <!-- Actions Dropdown -->
                  <div class="flex-shrink-0">
                    <DropdownMenu
                      triggerIcon={MoreHorizontal}
                      triggerClass="p-1.5 rounded hover:bg-black/5 transition-all"
                      triggerStyle="color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};"
                      showChevron={false}
                      iconOnly={true}
                      placement="bottom-end"
                      items={[
                        {
                          title: t('common.edit'),
                          icon: Edit,
                          onClick: () => onOpenAssetReportModal('edit', report)
                        },
                        { type: 'divider' },
                        {
                          title: t('common.delete'),
                          icon: Trash2,
                          color: '#dc2626',
                          onClick: () => deleteAssetReport(report.id)
                        }
                      ]}
                    />
                  </div>
                </div>
              </div>
            {/each}

            {#if portalStore.assetReports.length === 0}
              <div class="text-center py-8">
                <p class="text-sm mb-4" style="color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};">
                  {t('portal.customize.noAssetReports')}
                </p>
              </div>
            {/if}
          </div>

          <!-- Add Asset Report Button -->
          <button
            onclick={() => onOpenAssetReportModal('create')}
            class="w-full flex items-center justify-center gap-2 px-4 py-3 rounded border-2 border-dashed transition-all"
            style="border-color: {portalStore.isDarkMode ? '#475569' : '#d1d5db'}; color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};"
          >
            <Plus class="w-5 h-5" />
            <span class="font-medium">{t('portal.customize.addAssetReport')}</span>
          </button>
        {/if}
      {:else if portalStore.activeSection === 'knowledge-base'}
        <!-- Knowledge Base Configuration -->
        <div class="mb-6">
          <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text);">{t('portal.customize.docmostKnowledgeBase')}</h3>
          <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
            {t('portal.customize.docmostDescription')}
          </p>
        </div>

        <div class="space-y-4">
          <div>
            <label class="block text-xs font-medium mb-2" style="color: var(--ds-text);">
              {t('portal.customize.docmostShareLink')}
            </label>
            <input
              type="text"
              value={portalStore.knowledgeBaseShareLink}
              oninput={(e) => portalStore.knowledgeBaseShareLink = e.target.value}
              onblur={() => portalStore.saveKnowledgeBaseConfig()}
              class="w-full px-3 py-2 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 text-sm"
              style="background-color: var(--ds-surface); color: var(--ds-text); border-color: var(--ds-border);"
              placeholder={t('portal.customize.docmostShareLinkPlaceholder')}
            />
            <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
              {t('portal.customize.docmostShareLinkHelp')}
            </p>
          </div>

          {#if portalStore.knowledgeBaseShareLink}
            {@const parsed = portalStore.parseDocmostShareLink(portalStore.knowledgeBaseShareLink)}
            {#if parsed.baseURL && parsed.shareID}
              <div class="p-3 rounded" style="background-color: var(--ds-surface-raised);">
                <div class="text-xs font-medium mb-2" style="color: var(--ds-text);">
                  {t('portal.customize.parsedConfiguration')}
                </div>
                <div class="space-y-1 text-xs" style="color: var(--ds-text-subtle);">
                  <div>
                    <span class="font-medium">{t('portal.customize.baseURL')}</span>
                    <span class="ml-1">{parsed.baseURL}</span>
                  </div>
                  <div>
                    <span class="font-medium">{t('portal.customize.shareID')}</span>
                    <span class="ml-1">{parsed.shareID}</span>
                  </div>
                </div>
                <div class="mt-2 flex items-center gap-1 text-xs" style="color: #10b981;">
                  <Check class="w-3 h-3" />
                  <span>{t('portal.customize.configurationValid')}</span>
                </div>
              </div>
            {:else}
              <div class="p-3 rounded" style="background-color: {portalStore.isDarkMode ? 'rgba(220, 38, 38, 0.1)' : '#fee2e2'};">
                <div class="flex items-center gap-1 text-xs" style="color: #dc2626;">
                  <X class="w-3 h-3" />
                  <span>{t('portal.customize.invalidShareLinkFormat')}</span>
                </div>
                <div class="text-xs mt-1" style="color: #dc2626;">
                  {t('portal.customize.expectedFormat')}
                </div>
              </div>
            {/if}
          {/if}

          <div class="pt-4 border-t" style="border-color: var(--ds-border);">
            <h4 class="text-xs font-medium mb-2" style="color: var(--ds-text);">
              {t('portal.customize.howToGetShareLink')}
            </h4>
            <ol class="text-xs space-y-1 list-decimal list-inside" style="color: var(--ds-text-subtle);">
              <li>{t('portal.customize.docmostStep1')}</li>
              <li>{t('portal.customize.docmostStep2')}</li>
              <li>{t('portal.customize.docmostStep3')}</li>
              <li>{t('portal.customize.docmostStep4')}</li>
              <li>{t('portal.customize.docmostStep5')}</li>
            </ol>
          </div>
        </div>
      {/if}
    </div>
  </div>
</div>

<!-- Request Type Visibility Modal -->
<RequestTypeVisibilityModal
  isOpen={showVisibilityModal}
  requestType={selectedRequestTypeForVisibility}
  isDarkMode={portalStore.isDarkMode}
  onSaved={handleVisibilitySaved}
  onClose={closeVisibilityModal}
/>

<!-- Asset Report Visibility Modal (reuses RequestTypeVisibilityModal since structure is same) -->
<AssetReportVisibilityModal
  isOpen={showAssetReportVisibilityModal}
  requestType={selectedAssetReportForVisibility}
  isDarkMode={portalStore.isDarkMode}
  onSaved={handleAssetReportVisibilitySaved}
  onClose={closeAssetReportVisibilityModal}
/>
