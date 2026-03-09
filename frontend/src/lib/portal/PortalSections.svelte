<script>
  import { onMount, onDestroy } from 'svelte';
  import { dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { Plus, Trash2, X, Package, Table2 } from 'lucide-svelte';
  import { portalStore, iconMap } from '../stores/portal.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import AssetReportTable from './AssetReportTable.svelte';

  let {
    onOpenRequestForm = () => {}
  } = $props();

  // Section editing state (local to this component)
  let editingSectionId = $state(null);
  let editingSectionField = $state(null);
  let editingSectionValue = $state('');

  // Drag-and-drop state (dropZoneStates is local, draggedRequestType comes from store)
  let dropZoneStates = $state(new Map());

  function startEditingSection(sectionId, field) {
    editingSectionId = sectionId;
    editingSectionField = field;
    const section = portalStore.portalSections.find(s => s.id === sectionId);
    if (section) {
      editingSectionValue = field === 'title' ? section.title : section.subtitle;
    }
  }

  function saveSection() {
    if (editingSectionId && editingSectionField) {
      portalStore.updateSection(editingSectionId, editingSectionField, editingSectionValue);
      cancelEditingSection();
    }
  }

  function cancelEditingSection() {
    editingSectionId = null;
    editingSectionField = null;
    editingSectionValue = '';
  }

  function handleAddSection() {
    const newSectionId = portalStore.addSection();
    startEditingSection(newSectionId, 'title');
  }

  // Drag-and-drop setup
  let cleanupFunctions = [];
  let lastSectionIds = '';

  function setupDropZones() {
    // Only setup if in edit mode or customize panel with request-types or asset-reports section is open
    const shouldSetup = portalStore.isEditing ||
      (portalStore.showCustomizePanel && (portalStore.activeSection === 'request-types' || portalStore.activeSection === 'asset-reports'));
    if (!shouldSetup) return;

    const zones = document.querySelectorAll('[data-section-drop-zone]');
    zones.forEach(zone => {
      const sectionId = zone.dataset.sectionId;

      const cleanup = dropTargetForElements({
        element: zone,
        canDrop: ({ source }) => source.data.type === 'request-type' || source.data.type === 'asset-report',
        onDragEnter: () => {
          dropZoneStates.set(sectionId, { isOver: true });
          dropZoneStates = new Map(dropZoneStates); // trigger reactivity
        },
        onDragLeave: () => {
          dropZoneStates.set(sectionId, { isOver: false });
          dropZoneStates = new Map(dropZoneStates);
        },
        onDrop: ({ source }) => {
          if (source.data.type === 'request-type') {
            portalStore.addRequestTypeToSection(sectionId, source.data.requestType.id);
          } else if (source.data.type === 'asset-report') {
            portalStore.addAssetReportToSection(sectionId, source.data.assetReport.id);
          }
          dropZoneStates.set(sectionId, { isOver: false });
          dropZoneStates = new Map(dropZoneStates);
        }
      });
      cleanupFunctions.push(cleanup);
    });
  }

  onMount(() => {
    // Setup after DOM is ready
    setTimeout(setupDropZones, 100);
  });

  onDestroy(() => {
    cleanupFunctions.forEach(fn => fn());
    cleanupFunctions = [];
  });

  // Re-setup when sections change or edit mode changes
  $effect(() => {
    // Track dependencies
    const currentIds = portalStore.portalSections.map(s => s.id).join(',');
    const isActive = portalStore.isEditing ||
      (portalStore.showCustomizePanel && (portalStore.activeSection === 'request-types' || portalStore.activeSection === 'asset-reports'));

    if (currentIds !== lastSectionIds || isActive) {
      lastSectionIds = currentIds;
      // Cleanup previous
      cleanupFunctions.forEach(fn => fn());
      cleanupFunctions = [];
      // Wait for DOM to update then re-setup
      setTimeout(setupDropZones, 100);
    }
  });
</script>

<!-- Portal Sections -->
<div class="space-y-12">
  {#each portalStore.portalSections as section, sectionIndex}
    {@const sectionRequestTypes = portalStore.getSectionRequestTypes(section, portalStore.isEditing || (portalStore.showCustomizePanel && portalStore.activeSection === 'request-types'))}
    {@const sectionAssetReports = portalStore.getSectionAssetReports(section, portalStore.isEditing || (portalStore.showCustomizePanel && portalStore.activeSection === 'asset-reports'))}
    {@const isDraggingItem = portalStore.draggedRequestType || portalStore.draggedAssetReport}
    {@const isDropTarget = portalStore.isEditing || (portalStore.showCustomizePanel && (portalStore.activeSection === 'request-types' || portalStore.activeSection === 'asset-reports'))}
    <!-- Only show section in public view if it has a title, request types, or asset reports -->
    {#if portalStore.isEditing || section.title || sectionRequestTypes.length > 0 || sectionAssetReports.length > 0}
      <div class="relative {portalStore.isEditing ? 'p-6 rounded border-2 border-dashed' : ''}" style="{portalStore.isEditing ? `border-color: ${portalStore.isDarkMode ? '#475569' : '#d1d5db'}; background-color: ${portalStore.isDarkMode ? 'rgba(51, 65, 85, 0.3)' : 'rgba(249, 250, 251, 0.5)'};` : ''}">
        {#if portalStore.isEditing}
          <!-- Edit Mode: Show section controls -->
          <div class="absolute -left-10 top-6 flex flex-col gap-1">
            <button
              onclick={() => portalStore.moveSectionUp(sectionIndex)}
              disabled={sectionIndex === 0}
              class="p-1 rounded transition-all disabled:opacity-30 hover:bg-black/5"
              style="color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};"
              title={t('layout.moveUp')}
            >
              <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z" clip-rule="evenodd" />
              </svg>
            </button>
            <button
              onclick={() => portalStore.moveSectionDown(sectionIndex)}
              disabled={sectionIndex === portalStore.portalSections.length - 1}
              class="p-1 rounded transition-all disabled:opacity-30 hover:bg-black/5"
              style="color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};"
              title={t('layout.moveDown')}
            >
              <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                <path fill-rule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clip-rule="evenodd" />
              </svg>
            </button>
            <button
              onclick={async () => {
                const confirmed = await confirm({
                  title: t('common.delete'),
                  message: 'Are you sure you want to delete this section?',
                  confirmText: t('common.delete'),
                  cancelText: t('common.cancel'),
                  variant: 'danger'
                });
                if (confirmed) portalStore.deleteSection(section.id);
              }}
              class="p-1 rounded transition-all hover:bg-red-50"
              style="color: #dc2626;"
              title={t('layout.deleteSection')}
            >
              <Trash2 class="w-4 h-4" />
            </button>
          </div>
        {/if}

        <div>
          <!-- Section Title -->
          {#if portalStore.isEditing}
            {#if editingSectionId === section.id && editingSectionField === 'title'}
              <!-- svelte-ignore a11y_autofocus -->
              <input
                type="text"
                bind:value={editingSectionValue}
                onkeydown={(e) => {
                  if (e.key === 'Enter') saveSection();
                  if (e.key === 'Escape') cancelEditingSection();
                }}
                onblur={saveSection}
                class="text-2xl font-semibold mb-2 bg-transparent border-b-2 border-blue-500 focus:outline-none w-full"
                style="color: var(--ds-text);"
                placeholder="Section title (click to edit)"
                autofocus
              />
            {:else}
              <button
                onclick={() => startEditingSection(section.id, 'title')}
                class="text-2xl font-semibold mb-2 text-left w-full hover:opacity-70 transition-opacity"
                style="color: var(--ds-text);"
              >
                {section.title || '(Click to add title)'}
              </button>
            {/if}
          {:else if section.title}
            <h2 class="text-2xl font-semibold mb-2" style="color: var(--ds-text);">
              {section.title}
            </h2>
          {/if}

          <!-- Section Subtitle -->
          {#if portalStore.isEditing}
            {#if editingSectionId === section.id && editingSectionField === 'subtitle'}
              <!-- svelte-ignore a11y_autofocus -->
              <input
                type="text"
                bind:value={editingSectionValue}
                onkeydown={(e) => {
                  if (e.key === 'Enter') saveSection();
                  if (e.key === 'Escape') cancelEditingSection();
                }}
                onblur={saveSection}
                class="text-sm mb-6 bg-transparent border-b border-blue-500 focus:outline-none w-full"
                style="color: var(--ds-text-subtle);"
                placeholder="Subtitle (optional, click to edit)"
                autofocus
              />
            {:else}
              <button
                onclick={() => startEditingSection(section.id, 'subtitle')}
                class="text-sm mb-6 text-left w-full hover:opacity-70 transition-opacity"
                style="color: var(--ds-text-subtle);"
              >
                {section.subtitle || '(Click to add subtitle)'}
              </button>
            {/if}
          {:else if section.subtitle}
            <p class="text-sm mb-6" style="color: var(--ds-text-subtle);">
              {section.subtitle}
            </p>
          {/if}

          <!-- Request Types Grid / Drop Zone -->
          <div
            class="mt-6 {isDropTarget ? 'min-h-32' : ''} rounded transition-all"
            class:border-2={isDraggingItem && isDropTarget}
            class:border-dashed={isDraggingItem && isDropTarget}
            style="{isDraggingItem && isDropTarget ? `border-color: ${dropZoneStates.get(section.id)?.isOver ? '#3b82f6' : (portalStore.isDarkMode ? '#475569' : '#d1d5db')}; background-color: ${dropZoneStates.get(section.id)?.isOver ? (portalStore.isDarkMode ? 'rgba(59, 130, 246, 0.1)' : '#dbeafe') : 'transparent'}; padding: 0.5rem;` : ''}"
            data-section-drop-zone
            data-section-id={section.id}
          >
            {#if sectionRequestTypes.length > 0}
              <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
                {#each sectionRequestTypes as requestType}
                  <button
                    type="button"
                    class="appearance-none font-[inherit] text-[inherit] text-left w-full m-0 rounded p-6 border hover:shadow-md transition-shadow cursor-pointer relative group"
                    style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
                    onclick={() => onOpenRequestForm(requestType)}
                  >
                    {#if portalStore.isEditing || (portalStore.showCustomizePanel && portalStore.activeSection === 'request-types')}
                      <span
                        role="button"
                        tabindex="-1"
                        onclick={(e) => { e.stopPropagation(); portalStore.removeRequestTypeFromSection(section.id, requestType.id); }}
                        onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.stopPropagation(); portalStore.removeRequestTypeFromSection(section.id, requestType.id); } }}
                        class="absolute top-2 right-2 p-1 rounded opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer"
                        style="background-color: {portalStore.isDarkMode ? 'rgba(220, 38, 38, 0.1)' : '#fee2e2'}; color: #dc2626;"
                        title={t('portal.removeFromSection')}
                      >
                        <X class="w-3 h-3" />
                      </span>
                    {/if}
                    <div class="w-12 h-12 rounded mb-4 flex items-center justify-center" style="background-color: {requestType.color || '#6b7280'};">
                      <svelte:component this={iconMap[requestType.icon] || Package} size={24} color="white" />
                    </div>
                    <div class="font-medium mb-2 flex items-center gap-2" style="color: var(--ds-text);">
                      {requestType.name}
                      {#if !requestType.is_active}
                        <span
                          class="px-1.5 py-0.5 text-[10px] font-medium rounded"
                          style="background-color: {portalStore.isDarkMode ? 'rgba(156, 163, 175, 0.2)' : '#f3f4f6'}; color: {portalStore.isDarkMode ? '#9ca3af' : '#6b7280'};"
                        >
                          INACTIVE
                        </span>
                      {/if}
                    </div>
                    {#if requestType.description}
                      <p class="text-sm" style="color: var(--ds-text-subtle);">
                        {requestType.description}
                      </p>
                    {/if}
                  </button>
                {/each}
              </div>

              <!-- Drop zone indicator when dragging over section with items -->
              {#if isDraggingItem && dropZoneStates.get(section.id)?.isOver && isDropTarget}
                <div class="mt-4 text-center py-4 border-2 border-dashed rounded" style="border-color: #3b82f6; background-color: {portalStore.isDarkMode ? 'rgba(59, 130, 246, 0.1)' : '#dbeafe'};">
                  <p class="text-sm font-medium" style="color: {portalStore.isDarkMode ? '#60a5fa' : '#2563eb'};">
                    {t('portal.dropHereToAdd')}
                  </p>
                </div>
              {/if}
            {:else if isDropTarget}
              <div
                class="text-center py-8 border-2 border-dashed rounded transition-all"
                style="border-color: {dropZoneStates.get(section.id)?.isOver ? '#3b82f6' : (portalStore.isDarkMode ? '#475569' : '#d1d5db')}; background-color: {dropZoneStates.get(section.id)?.isOver ? (portalStore.isDarkMode ? 'rgba(59, 130, 246, 0.1)' : '#dbeafe') : 'transparent'};"
              >
                <p class="text-sm" style="color: var(--ds-text-subtle);">
                  {dropZoneStates.get(section.id)?.isOver ? t('portal.dropHereToAdd') : t('portal.noRequestTypesInSection')}
                </p>
              </div>
            {:else}
              <!-- Empty state for public view -->
              <div
                class="text-center py-8 border rounded"
                style="border-color: var(--ds-border);"
              >
                <p class="text-sm" style="color: var(--ds-text-subtle);">
                  {t('portal.noRequestTypesInSection')}
                </p>
              </div>
            {/if}
          </div>

          <!-- Asset Reports (full width) -->
          {#if sectionAssetReports.length > 0}
            <div class="mt-6 space-y-4">
              {#each sectionAssetReports as report}
                <AssetReportTable
                  {report}
                  slug={portalStore.currentSlug}
                  sectionId={section.id}
                  isEditing={portalStore.isEditing || (portalStore.showCustomizePanel && portalStore.activeSection === 'asset-reports')}
                  onRemove={(reportId) => portalStore.removeAssetReportFromSection(section.id, reportId)}
                />
              {/each}
            </div>
          {/if}
        </div>
      </div>
    {/if}
  {/each}

  <!-- Add Section Button (Edit Mode) -->
  {#if portalStore.isEditing}
    <button
      onclick={handleAddSection}
      class="w-full flex items-center justify-center gap-2 px-6 py-8 rounded border-2 border-dashed transition-all hover:border-solid"
      style="border-color: {portalStore.isDarkMode ? '#475569' : '#d1d5db'}; color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};"
    >
      <Plus class="w-5 h-5" />
      <span class="font-medium">{t('portal.addSection')}</span>
    </button>
  {/if}

  <!-- Empty State (when no sections and not editing) -->
  {#if portalStore.portalSections.length === 0 && !portalStore.isEditing}
    <div class="text-center py-16">
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        {t('portal.noContentSections')}
      </p>
    </div>
  {/if}
</div>
