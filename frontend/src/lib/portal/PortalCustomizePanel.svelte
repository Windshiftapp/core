<script>
  import { onMount, onDestroy } from 'svelte';
  import StateDisplay from '../components/StateDisplay.svelte';
  import { draggable } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import {
    Palette, Navigation, X, TextCursorInput, BookOpen, Check,
    Plus, Trash2, Edit, MoreHorizontal, GripVertical,
    Package, Shield, Table2, Edit3, Info
  } from '@lucide/svelte';
  import Tooltip from '../components/Tooltip.svelte';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import RequestTypeVisibilityModal from '../dialogs/RequestTypeVisibilityModal.svelte';
  import AssetReportVisibilityModal from '../dialogs/RequestTypeVisibilityModal.svelte';
  import GradientSelector from '../components/GradientSelector.svelte';
  import BackgroundImageSelector from '../components/BackgroundImageSelector.svelte';
  import LogoUploader from '../components/LogoUploader.svelte';
  import Label from '../components/Label.svelte';
  import Input from '../components/Input.svelte';
  import {
    portalCatalogStore,
    portalCustomizationStore as portalStore,
  } from '../stores/portal.svelte.js';
  import { gradients, iconMap } from '../stores/portalPresentation.js';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';
  import { api } from '../api.js';
  import { workspacesStore } from '../stores/workspaces.svelte.js';
  import { portalAuthStore } from '../stores/portalAuth.svelte.js';
  import { authStore } from '../stores';
  import { loadPermissionProfile } from '../stores/permissionProfile.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import DescriptionText from '../components/DescriptionText.svelte';
  import RequestTypeFieldsBuilder from '../dialogs/RequestTypeFieldsBuilder.svelte';
  import PagePicker from '../pickers/PagePicker.svelte';

  let {
    onOpenRequestTypeModal = () => {},
    onOpenAssetReportModal = () => {},
    onOpenAssetReportFieldsModal = () => {}
  } = $props();

  // Inline-expanded fields builder. Replaces the previous "open a modal"
  // flow: when an admin clicks "Add Fields" / "Configure Fields" on a
  // request type card, a sibling fixed panel slides in to the right of
  // this customize sidebar instead of covering the whole viewport.
  let expandedRequestTypeForFields = $state(null);

  // Collapse the inline builder when the customize panel itself closes,
  // otherwise the orphan panel stays floating at the left edge.
  $effect(() => {
    if (!portalStore.showCustomizePanel) {
      expandedRequestTypeForFields = null;
    }
  });

  // Visibility modal state
  let showVisibilityModal = $state(false);
  let selectedRequestTypeForVisibility = $state(null);

  // Knowledge-base workspace-pages wiring state (see the knowledge-base
  // customize section). Workspaces and page titles load lazily when the
  // section opens; the wiring itself lives in the portal store so both
  // persistence paths serialize identically. Start pages are picked through
  // the searchable PagePicker, so no page tree is held here.
  let kbWorkspaceId = $state('');
  let kbScope = $state('entire');
  let kbRootPageId = $state(null);
  let kbWorkspaces = $state([]);
  let kbPageTitles = $state({});
  // Workspace administration rights of the current manager, resolved from
  // their permission profile. Null until loaded; the customize panel lives in
  // the portal shell, which does not populate the main app's permission store.
  let kbAdminWorkspaceIds = $state(null);
  let kbIsSystemAdmin = $state(false);

  let kbAddReady = $derived(Boolean(kbWorkspaceId) && (kbScope !== 'subtree' || kbRootPageId));

  // The picker only offers workspaces connected to this portal channel that
  // the current manager administers — mirroring the backend validation in
  // ChannelConfigUpdateService.validateKnowledgeBasePageSources so every
  // selectable option can actually be saved.
  let kbSelectableWorkspaces = $derived(
    kbAdminWorkspaceIds === null
      ? []
      : kbWorkspaces.filter(
          (workspace) =>
            (portalStore.portalData?.workspace_ids ?? []).includes(workspace.id) &&
            (kbIsSystemAdmin || kbAdminWorkspaceIds.has(workspace.id))
        )
  );

  async function loadKbWorkspaces() {
    try {
      kbWorkspaces = (await workspacesStore.load()) ?? [];
    } catch (err) {
      console.error('Failed to load workspaces for knowledge base wiring:', err);
      kbWorkspaces = [];
    }
  }

  async function loadKbWorkspaceEligibility() {
    if (kbAdminWorkspaceIds !== null) return;
    const userId = $portalAuthStore.user?.id ?? $authStore.currentUser?.id;
    if (!userId) return;
    try {
      const profile = await loadPermissionProfile(userId);
      kbIsSystemAdmin = profile.has_system_admin === true;
      const adminIds = new Set();
      for (const [wsId, keys] of Object.entries(profile.workspace_permissions || {})) {
        if (keys.includes('workspace.admin')) {
          adminIds.add(Number(wsId));
        }
      }
      kbAdminWorkspaceIds = adminIds;
    } catch (err) {
      console.error('Failed to load workspace permissions for knowledge base wiring:', err);
      kbAdminWorkspaceIds = new Set();
    }
  }

  async function loadKbPageTitles() {
    const sources = portalStore.knowledgeBasePageSources || [];
    const workspaceIds = [...new Set(sources.map((s) => s.workspace_id))]
      .filter((id) => kbPageTitles[`ws:${id}`] !== true);
    if (workspaceIds.length === 0) return;
    try {
      const rows = await api.pages.getTitles(workspaceIds);
      const titles = { ...kbPageTitles };
      for (const row of Array.isArray(rows) ? rows : []) {
        titles[row.page_id] = row.title;
      }
      for (const workspaceId of workspaceIds) titles[`ws:${workspaceId}`] = true;
      kbPageTitles = titles;
    } catch (err) {
      console.error('Failed to load page titles for knowledge base wiring:', err);
    }
  }

  function onKbWorkspaceChange() {
    kbRootPageId = null;
    kbScope = 'entire';
  }

  function addKbPageSource() {
    const workspaceId = Number(kbWorkspaceId);
    if (!workspaceId) return;
    const source =
      kbScope === 'subtree' && kbRootPageId
        ? { workspace_id: workspaceId, root_page_id: kbRootPageId }
        : { workspace_id: workspaceId };
    portalStore.addKnowledgeBasePageSource(source);
    kbWorkspaceId = '';
    kbScope = 'entire';
    kbRootPageId = null;
  }

  function kbSourceLabel(source) {
    const workspace = kbWorkspaces.find((ws) => ws.id === source.workspace_id);
    const workspaceName = workspace?.name || `#${source.workspace_id}`;
    if (source.root_page_id == null) {
      return t('portal.customize.workspacePagesEntryEntire', { workspace: workspaceName });
    }
    const pageTitle = kbPageTitles[source.root_page_id] || `#${source.root_page_id}`;
    return t('portal.customize.workspacePagesEntrySubtree', {
      workspace: workspaceName,
      page: pageTitle,
    });
  }

  $effect(() => {
    if (
      portalStore.activeSection === 'knowledge-base' &&
      portalStore.showCustomizePanel
    ) {
      if (kbWorkspaces.length === 0) void loadKbWorkspaces();
      void loadKbWorkspaceEligibility();
      void loadKbPageTitles();
    }
  });

  function openVisibilityModal(requestType) {
    selectedRequestTypeForVisibility = requestType;
    showVisibilityModal = true;
  }

  function closeVisibilityModal() {
    showVisibilityModal = false;
    selectedRequestTypeForVisibility = null;
  }

  async function handleVisibilitySaved() {
    await portalCatalogStore.loadRequestTypes();
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
    await portalCatalogStore.loadAssetReports();
  }

  function hasAssetReportVisibilityRestrictions(report) {
    return (report.visibility_group_ids?.length > 0) || (report.visibility_org_ids?.length > 0);
  }

  async function deleteAssetReport(id) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('portal.customize.confirmDeleteAssetReport'),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;

    try {
      await api.assetReports.delete(portalStore.portalData?.channel_id, id);
      await portalCatalogStore.loadAssetReports();
    } catch (err) {
      console.error('Failed to delete asset report:', err);
    }
  }

  let showCustomizePanelHover = $state(false);

  async function deleteRequestType(id) {
    const confirmed = await confirm({
      title: t('common.delete'),
      message: t('portal.customize.confirmDeleteRequestType'),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'danger'
    });
    if (!confirmed) return;

    try {
      await api.requestTypes.delete(portalStore.portalData?.channel_id, id);
      await portalCatalogStore.loadRequestTypes();
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
      cards.forEach(/** @param {HTMLElement} card */ (card) => {
        const dragHandle = card.querySelector('[data-drag-handle]');
        const requestTypeId = card.dataset.requestTypeId;
        const requestType = portalCatalogStore.requestTypes.find(rt => String(rt.id) === String(requestTypeId));

        if (!requestType || !dragHandle) return;

        const cleanup = draggable({
          element: card,
          dragHandle: dragHandle,
          getInitialData: () => ({
            type: 'request-type',
            requestType
          }),
          onDragStart: () => {
            portalCatalogStore.draggedRequestType = requestType;
            card.style.opacity = '0.5';
          },
          onDrop: () => {
            portalCatalogStore.draggedRequestType = null;
            card.style.opacity = '';
          }
        });
        cleanupFunctions.push(cleanup);
      });
    }

    // Setup asset report draggables
    if (portalStore.activeSection === 'asset-reports') {
      const cards = document.querySelectorAll('[data-asset-report-card]');
      cards.forEach(/** @param {HTMLElement} card */ (card) => {
        const dragHandle = card.querySelector('[data-drag-handle]');
        const reportId = card.dataset.assetReportId;
        const report = portalCatalogStore.assetReports.find(ar => String(ar.id) === String(reportId));

        if (!report || !dragHandle) return;

        const cleanup = draggable({
          element: card,
          dragHandle: dragHandle,
          getInitialData: () => ({
            type: 'asset-report',
            assetReport: report
          }),
          onDragStart: () => {
            portalCatalogStore.draggedAssetReport = report;
            card.style.opacity = '0.5';
          },
          onDrop: () => {
            portalCatalogStore.draggedAssetReport = null;
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
    const currentRequestTypeIds = portalCatalogStore.requestTypes.map(rt => rt.id).join(',');
    const currentAssetReportIds = portalCatalogStore.assetReports.map(ar => ar.id).join(',');
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

<!-- Shared card fragments for the request-type and asset-report lists. -->
{#snippet cardIconBadge(color, Icon)}
  <div class="flex-shrink-0">
    <div class="w-8 h-8 rounded flex items-center justify-center" style="background-color: {color || '#6b7280'};">
      <Icon size={16} color="white" />
    </div>
  </div>
{/snippet}

{#snippet dragHandle()}
  <div class="cursor-grab active:cursor-grabbing pt-1" style="color: {portalStore.isDarkMode ? '#64748b' : '#9ca3af'};" data-drag-handle>
    <GripVertical class="w-4 h-4" />
  </div>
{/snippet}

{#snippet visibilityShield(restricted, onConfigure)}
  <div class="flex-shrink-0">
    <Tooltip content={restricted ? t('portal.visibility.hasRestrictions') : t('portal.visibility.noRestrictions')} placement="top">
      {#snippet children()}
        <button
          onclick={onConfigure}
          class="p-1.5 rounded transition-all hover:bg-black/5"
          title={t('portal.visibility.configureVisibility')}
        >
          <Shield
            class="w-4 h-4"
            style="color: {restricted ? '#f59e0b' : (portalStore.isDarkMode ? '#94a3b8' : '#6b7280')};"
          />
        </button>
      {/snippet}
    </Tooltip>
  </div>
{/snippet}

<ModalBackdrop
  show={portalStore.showCustomizePanel && portalStore.activeSection !== 'request-types'}
  opacity={0.3}
  blur={0}
  align="none"
  onclose={() => portalStore.showCustomizePanel = false}
/>

<!-- Customization Panel - Slides from Left.
     In edit mode, leave room for Portal's edit bar above the fixed panels.
     The full-bleed shadow is only applied when the inline fields builder
     is closed, otherwise it casts a visible seam between the two panels. -->
<div
  class="fixed left-0 flex z-50 transform transition-transform duration-300 ease-in-out"
  style="
    top: {portalStore.isEditing ? '2.5rem' : '0'};
    height: {portalStore.isEditing ? 'calc(100% - 2.5rem)' : '100%'};
    background-color: var(--ds-surface-card);
    box-shadow: {expandedRequestTypeForFields ? 'none' : '0 25px 50px -12px rgba(0, 0, 0, 0.25)'};
  "
  class:translate-x-0={portalStore.showCustomizePanel}
  class:-translate-x-full={!portalStore.showCustomizePanel}
  data-testid="portal-customize-panel"
>
  <!-- Vertical Navigation Sidebar -->
  <div class="w-16 border-r flex flex-col items-center py-4" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
    <!-- Edit Mode Toggle -->
    <Tooltip content="Edit Mode" placement="right">
      {#snippet children()}
        <button
          onclick={() => portalStore.toggleEditing()}
          class="w-10 h-10 rounded flex items-center justify-center cursor-pointer transition-all"
          style="background-color: {portalStore.isEditing ? 'var(--ds-background-neutral)' : 'transparent'};"
          data-testid="portal-edit-mode-toggle"
        >
          <Edit3 class="w-5 h-5" style="color: {portalStore.isEditing ? 'var(--ds-interactive, #2563eb)' : 'var(--ds-text-subtle)'};" />
        </button>
      {/snippet}
    </Tooltip>

    <div class="w-8 border-b pb-2 mb-2" style="border-color: var(--ds-border);"></div>

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
    {#if portalCatalogStore.hasAssetSets}
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
          data-testid="portal-customize-kb-section"
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
      <!-- Edit Mode Info Banner -->
      {#if portalStore.isEditing}
        <div class="flex items-center gap-2 px-3 py-2 mb-4 rounded text-xs" style="background-color: var(--ds-status-info-bg); color: var(--ds-status-info-text);">
          <Info class="w-4 h-4 flex-shrink-0" />
          <span>Edit mode active — make changes directly on the portal</span>
        </div>
      {/if}
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

        {#if portalCatalogStore.loadingRequestTypes}
          <StateDisplay type="loading" />
        {:else}
          <!-- Request Types List -->
          <div class="space-y-2 mb-4">
            {#each portalCatalogStore.requestTypes as requestType}
              {@const hasNoFields = requestType.field_count === 0}
              {@const isExpanded = expandedRequestTypeForFields?.id === requestType.id}
              {@const RequestTypeIcon = iconMap[requestType.icon] || Package}
              <div
                class="p-3 rounded border transition-all"
                style="
                  background-color: {isExpanded
                    ? 'var(--ds-background-selected)'
                    : (hasNoFields ? (portalStore.isDarkMode ? '#422006' : '#fffbeb') : (portalStore.isDarkMode ? '#334155' : '#f9fafb'))};
                  border-color: {isExpanded
                    ? 'var(--ds-interactive)'
                    : (hasNoFields ? '#f59e0b' : (portalStore.isDarkMode ? '#475569' : '#e5e7eb'))};
                "
                data-request-type-card
                data-request-type-id={requestType.id}
              >
                <div class="flex items-start gap-3">
                  <!-- Icon Preview -->
                  {@render cardIconBadge(requestType.color, RequestTypeIcon)}

                  <!-- Drag Handle -->
                  {@render dragHandle()}

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
                        {#if requestType.workspace_name}
                          <div>{requestType.workspace_name}{#if requestType.workspace_key}&nbsp;({requestType.workspace_key}){/if}</div>
                        {/if}
                      </div>
                      <button
                        onclick={() => expandedRequestTypeForFields = isExpanded ? null : requestType}
                        class="text-xs hover:underline text-right"
                        style="color: {hasNoFields ? '#f59e0b' : 'var(--ds-text-link)'};"
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
                  {@render visibilityShield(hasVisibilityRestrictions(requestType), () => openVisibilityModal(requestType))}

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
                          color: 'var(--ds-text-danger)',
                          onClick: () => deleteRequestType(requestType.id)
                        }
                      ]}
                    />
                  </div>
                </div>
              </div>
            {/each}

            {#if portalCatalogStore.requestTypes.length === 0}
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

        {#if portalCatalogStore.loadingAssetReports}
          <StateDisplay type="loading" />
        {:else}
          <!-- Asset Reports List -->
          <div class="space-y-2 mb-4">
            {#each portalCatalogStore.assetReports as report}
              {@const ReportIcon = iconMap[report.icon] || Table2}
              <div
                class="p-3 rounded border"
                style="background-color: {portalStore.isDarkMode ? '#334155' : '#f9fafb'}; border-color: {portalStore.isDarkMode ? '#475569' : '#e5e7eb'};"
                data-asset-report-card
                data-asset-report-id={report.id}
              >
                <div class="flex items-start gap-3">
                  <!-- Icon Preview -->
                  {@render cardIconBadge(report.color, ReportIcon)}

                  <!-- Drag Handle -->
                  {@render dragHandle()}

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
                  {@render visibilityShield(hasAssetReportVisibilityRestrictions(report), () => openAssetReportVisibilityModal(report))}

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
                        ...(report.run_mode === 'form' ? [{
                          title: t('requestTypeFields.configureFields'),
                          icon: Edit3,
                          onClick: () => onOpenAssetReportFieldsModal(report)
                        }] : []),
                        { type: 'divider' },
                        {
                          title: t('common.delete'),
                          icon: Trash2,
                          color: 'var(--ds-text-danger)',
                          onClick: () => deleteAssetReport(report.id)
                        }
                      ]}
                    />
                  </div>
                </div>
              </div>
            {/each}

            {#if portalCatalogStore.assetReports.length === 0}
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
            <label for="docmost-share-link" class="block text-xs font-medium mb-2" style="color: var(--ds-text);">
              {t('portal.customize.docmostShareLink')}
            </label>
            <Input
              id="docmost-share-link"
              type="text"
              value={portalStore.knowledgeBaseShareLink}
              oninput={(e) => portalStore.knowledgeBaseShareLink = /** @type {HTMLInputElement} */ (e.target).value}
              onblur={() => portalStore.saveKnowledgeBaseConfig()}
              placeholder={t('portal.customize.docmostShareLinkPlaceholder')}
              size="small"
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
                <div class="flex items-center gap-1 text-xs" style="color: var(--ds-text-danger);">
                  <X class="w-3 h-3" />
                  <span>{t('portal.customize.invalidShareLinkFormat')}</span>
                </div>
                <DescriptionText as="div" variant="danger">
                  {t('portal.customize.expectedFormat')}
                </DescriptionText>
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

          <!-- Workspace pages wiring -->
          <div class="pt-4 border-t" style="border-color: var(--ds-border);" data-testid="kb-pages-wiring">
            <h4 class="text-xs font-medium mb-2" style="color: var(--ds-text);">
              {t('portal.customize.workspacePagesTitle')}
            </h4>
            <p class="text-xs mb-3" style="color: var(--ds-text-subtle);">
              {t('portal.customize.workspacePagesDescription')}
            </p>

            {#if portalStore.knowledgeBasePageSources.length > 0}
              <!-- Clear marker: this knowledge base exposes workspace pages -->
              <div
                class="p-3 rounded mb-3"
                style="background-color: var(--ds-background-neutral); border-left: 3px solid #10b981;"
                data-testid="kb-pages-wiring-notice"
              >
                <div class="flex items-start gap-2 text-xs font-medium" style="color: var(--ds-text);">
                  <BookOpen class="w-4 h-4 flex-shrink-0" style="color: #10b981;" />
                  <span>{t('portal.customize.workspacePagesNotice', { count: portalStore.knowledgeBasePageSources.length })}</span>
                </div>
              </div>
              <div class="space-y-2 mb-3">
                {#each portalStore.knowledgeBasePageSources as source, index}
                  <div
                    class="flex items-center justify-between p-2 rounded text-xs"
                    style="background-color: var(--ds-surface-raised);"
                    data-testid="kb-page-source-entry"
                  >
                    <span style="color: var(--ds-text);">
                      {kbSourceLabel(source)}
                    </span>
                    <button
                      type="button"
                      class="p-1 rounded hover:opacity-70"
                      aria-label={t('portal.customize.workspacePagesRemove')}
                      data-testid="kb-remove-page-source"
                      onclick={() => portalStore.removeKnowledgeBasePageSource(index)}
                    >
                      <Trash2 class="w-3.5 h-3.5" style="color: var(--ds-text-danger);" />
                    </button>
                  </div>
                {/each}
              </div>
            {:else}
              <p class="text-xs mb-3" style="color: var(--ds-text-subtle);" data-testid="kb-pages-wiring-empty">
                {t('portal.customize.workspacePagesEmpty')}
              </p>
            {/if}

            <div class="space-y-2">
              <div>
                <label for="kb-add-workspace" class="block text-xs font-medium mb-1" style="color: var(--ds-text);">
                  {t('portal.customize.workspacePagesPickWorkspace')}
                </label>
                <select
                  id="kb-add-workspace"
                  class="w-full text-xs rounded p-2"
                  style="background-color: var(--ds-surface-raised); color: var(--ds-text); border-color: var(--ds-border);"
                  bind:value={kbWorkspaceId}
                  onchange={() => onKbWorkspaceChange()}
                  data-testid="kb-add-workspace"
                >
                  <option value="">{t('portal.customize.workspacePagesPickWorkspacePlaceholder')}</option>
                  {#each kbSelectableWorkspaces as workspace}
                    <option value={String(workspace.id)}>{workspace.name}</option>
                  {/each}
                </select>
                {#if kbAdminWorkspaceIds !== null && kbSelectableWorkspaces.length === 0}
                  <p class="text-xs mt-1" style="color: var(--ds-text-subtle);" data-testid="kb-add-workspace-none">
                    {t('portal.customize.workspacePagesNoEligible')}
                  </p>
                {/if}
              </div>
              {#if kbWorkspaceId}
                <div>
                  <label for="kb-add-scope" class="block text-xs font-medium mb-1" style="color: var(--ds-text);">
                    {t('portal.customize.workspacePagesScope')}
                  </label>
                  <select
                    id="kb-add-scope"
                    class="w-full text-xs rounded p-2"
                    style="background-color: var(--ds-surface-raised); color: var(--ds-text); border-color: var(--ds-border);"
                    bind:value={kbScope}
                    data-testid="kb-add-scope"
                  >
                    <option value="entire">{t('portal.customize.workspacePagesEntire')}</option>
                    <option value="subtree">{t('portal.customize.workspacePagesSubtree')}</option>
                  </select>
                </div>
                {#if kbScope === 'subtree'}
                  <div>
                    <label for="kb-add-page" class="block text-xs font-medium mb-1" style="color: var(--ds-text);">
                      {t('portal.customize.workspacePagesPickPage')}
                    </label>
                    <PagePicker
                      id="kb-add-page"
                      workspaceId={Number(kbWorkspaceId)}
                      bind:value={kbRootPageId}
                      placeholder={t('portal.customize.workspacePagesPickPagePlaceholder')}
                      inputTestid="kb-add-page"
                      optionTestid={(opt) => `kb-add-page-option-${opt.item?.id}`}
                    />
                  </div>
                {/if}
                <button
                  type="button"
                  class="w-full p-2 rounded text-xs font-medium"
                  style="background-color: var(--ds-interactive, #2563eb); color: #ffffff;"
                  disabled={!kbAddReady}
                  data-testid="kb-add-page-source"
                  onclick={addKbPageSource}
                >
                  {t('portal.customize.workspacePagesAdd')}
                </button>
              {/if}
            </div>
          </div>
        </div>
      {/if}
    </div>
  </div>
</div>

<!-- Inline Fields Builder — slides in to the right of the customize panel.
     A subtle 1px left border draws the seam between the two panels; the
     full ambient shadow is moved to the right edge only so the seam stays
     clean. -->
{#if portalStore.showCustomizePanel && expandedRequestTypeForFields}
  <div
    class="fixed left-[28rem] w-[30rem] z-40 flex flex-col border-l"
    style="
      top: {portalStore.isEditing ? '2.5rem' : '0'};
      height: {portalStore.isEditing ? 'calc(100% - 2.5rem)' : '100%'};
      background-color: var(--ds-surface-card);
      border-color: var(--ds-border);
      box-shadow: 24px 0 48px -12px rgba(0, 0, 0, 0.25);
    "
    data-testid="portal-fields-builder"
  >
    <RequestTypeFieldsBuilder
      requestTypeId={expandedRequestTypeForFields.id}
      requestTypeName={expandedRequestTypeForFields.name}
      channelId={portalStore.portalData?.channel_id}
      isDarkMode={portalStore.isDarkMode}
      onsaved={() => portalCatalogStore.loadRequestTypes()}
      onclose={() => expandedRequestTypeForFields = null}
    />
  </div>
{/if}

<!-- Request Type Visibility Modal -->
<RequestTypeVisibilityModal
  isOpen={showVisibilityModal}
  requestType={selectedRequestTypeForVisibility}
  channelId={portalStore.portalData?.channel_id}
  onSaved={handleVisibilitySaved}
  onclose={closeVisibilityModal}
/>

<!-- Asset Report Visibility Modal (reuses RequestTypeVisibilityModal since structure is same) -->
<AssetReportVisibilityModal
  isOpen={showAssetReportVisibilityModal}
  requestType={selectedAssetReportForVisibility}
  channelId={portalStore.portalData?.channel_id}
  onSaved={handleAssetReportVisibilitySaved}
  onclose={closeAssetReportVisibilityModal}
/>
