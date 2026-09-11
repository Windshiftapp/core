<script>
  import { onMount, onDestroy } from 'svelte';
  import { dropTargetForElements } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import { ChevronRight, Plus, Trash2, X, Package } from '@lucide/svelte';
  import {
    portalCatalogStore,
    portalCustomizationStore as portalStore,
  } from '../stores/portal.svelte.js';
  import { iconMap } from '../stores/portalPresentation.js';
  import { t } from '../stores/i18n.svelte.js';
  import { confirm } from '../composables/useConfirm.js';
  import AssetReportTable from './AssetReportTable.svelte';
  import Input from '../components/Input.svelte';

  let {
    onOpenRequestForm = () => {},
    onOpenAssetReportForm = () => {}
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
    const section = portalCatalogStore.portalSections.find(s => s.id === sectionId);
    if (section) {
      editingSectionValue = field === 'title' ? section.title : section.subtitle;
    }
  }

  function saveSection() {
    if (editingSectionId && editingSectionField) {
      portalCatalogStore.updateSection(editingSectionId, editingSectionField, editingSectionValue);
      cancelEditingSection();
    }
  }

  function cancelEditingSection() {
    editingSectionId = null;
    editingSectionField = null;
    editingSectionValue = '';
  }

  function handleAddSection() {
    const newSectionId = portalCatalogStore.addSection();
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
      const zoneElement = /** @type {HTMLElement} */ (zone);
      const sectionId = zoneElement.dataset.sectionId;

      const cleanup = dropTargetForElements({
        element: zoneElement,
        canDrop: ({ source }) => /** @type {any} */ (source.data).type === 'request-type' || /** @type {any} */ (source.data).type === 'asset-report',
        onDragEnter: () => {
          dropZoneStates.set(sectionId, { isOver: true });
          dropZoneStates = new Map(dropZoneStates); // trigger reactivity
        },
        onDragLeave: () => {
          dropZoneStates.set(sectionId, { isOver: false });
          dropZoneStates = new Map(dropZoneStates);
        },
        onDrop: ({ source }) => {
          if (/** @type {any} */ (source.data).type === 'request-type') {
            portalCatalogStore.addRequestTypeToSection(sectionId, /** @type {any} */ (source.data).requestType.id);
          } else if (/** @type {any} */ (source.data).type === 'asset-report') {
            portalCatalogStore.addAssetReportToSection(sectionId, /** @type {any} */ (source.data).assetReport.id);
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
    const currentIds = portalCatalogStore.portalSections.map(s => s.id).join(',');
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

<!-- Inline edit input for the section title/subtitle, sharing the save and
     cancel key handlers. -->
{#snippet sectionEditInput(className, styleAttr, placeholder)}
  <Input
    type="text"
    bind:value={editingSectionValue}
    onkeydown={(e) => {
      if (e.key === 'Enter') saveSection();
      if (e.key === 'Escape') cancelEditingSection();
    }}
    onblur={saveSection}
    class={className}
    style={styleAttr}
    {placeholder}
    autofocus
  />
{/snippet}

<!-- Portal Sections -->
<div class="space-y-10">
  {#each portalCatalogStore.portalSections as section, sectionIndex}
    {@const sectionRequestTypes = portalCatalogStore.getSectionRequestTypes(section, portalStore.isEditing || (portalStore.showCustomizePanel && portalStore.activeSection === 'request-types'))}
    {@const sectionAssetReports = portalCatalogStore.getSectionAssetReports(section, portalStore.isEditing || (portalStore.showCustomizePanel && portalStore.activeSection === 'asset-reports'))}
    {@const formAssetReports = sectionAssetReports.filter((r) => r.run_mode === 'form')}
    {@const directAssetReports = sectionAssetReports.filter((r) => r.run_mode !== 'form')}
    {@const gridItems = [...sectionRequestTypes.map((rt) => ({ kind: 'request', item: rt })), ...formAssetReports.map((ar) => ({ kind: 'asset-form', item: ar }))]}
    {@const isDraggingItem = portalCatalogStore.draggedRequestType || portalCatalogStore.draggedAssetReport}
    {@const isDropTarget = portalStore.isEditing || (portalStore.showCustomizePanel && (portalStore.activeSection === 'request-types' || portalStore.activeSection === 'asset-reports'))}
    <!-- Only show section in public view if it has a title, request types, or asset reports -->
    {#if portalStore.isEditing || section.title || gridItems.length > 0 || directAssetReports.length > 0}
      <div class="relative {portalStore.isEditing ? 'p-6 rounded border-2 border-dashed' : ''}" style="{portalStore.isEditing ? `border-color: ${portalStore.isDarkMode ? '#475569' : '#d1d5db'}; background-color: ${portalStore.isDarkMode ? 'rgba(51, 65, 85, 0.3)' : 'rgba(249, 250, 251, 0.5)'};` : ''}">
        {#if portalStore.isEditing}
          <!-- Edit Mode: Show section controls -->
          <div class="absolute -left-10 top-6 flex flex-col gap-1">
            <button
              onclick={() => portalCatalogStore.moveSectionUp(sectionIndex)}
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
              onclick={() => portalCatalogStore.moveSectionDown(sectionIndex)}
              disabled={sectionIndex === portalCatalogStore.portalSections.length - 1}
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
                if (confirmed) portalCatalogStore.deleteSection(section.id);
              }}
              class="p-1 rounded transition-all hover-danger"
              style="color: var(--ds-text-danger);"
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
              {@render sectionEditInput(
                'text-xl font-medium mb-1 bg-transparent border-b-2 focus:outline-none w-full',
                'border-color: var(--ds-border-focused); color: var(--ds-text);',
                'Section title (click to edit)'
              )}
            {:else if section.title}
              <button
                onclick={() => startEditingSection(section.id, 'title')}
                class="text-xl font-medium mb-1 text-left w-full hover:opacity-70 transition-opacity"
                style="color: var(--ds-text);"
              >
                {section.title}
              </button>
            {:else}
              <button
                onclick={() => startEditingSection(section.id, 'title')}
                class="text-xs mb-2 text-left hover:opacity-70 transition-opacity"
                style="color: var(--ds-text-subtle);"
              >
                + Add title
              </button>
            {/if}
          {:else if section.title}
            <h2 class="text-xl font-medium mb-1" style="color: var(--ds-text);">
              {section.title}
            </h2>
          {/if}

          <!-- Section Subtitle -->
          {#if portalStore.isEditing}
            {#if editingSectionId === section.id && editingSectionField === 'subtitle'}
              {@render sectionEditInput(
                'text-sm mb-4 bg-transparent border-b focus:outline-none w-full',
                'border-color: var(--ds-border-focused); color: var(--ds-text-subtle);',
                'Subtitle (optional, click to edit)'
              )}
            {:else}
              <button
                onclick={() => startEditingSection(section.id, 'subtitle')}
                class="text-sm mb-4 text-left w-full hover:opacity-70 transition-opacity"
                style="color: var(--ds-text-subtle);"
              >
                {section.subtitle || '(Click to add subtitle)'}
              </button>
            {/if}
          {:else if section.subtitle}
            <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">
              {section.subtitle}
            </p>
          {/if}

          <!-- Request Types Grid / Drop Zone -->
          <div
            class="mt-5 {isDropTarget ? 'min-h-32' : ''} rounded transition-all"
            class:border-2={isDraggingItem && isDropTarget}
            class:border-dashed={isDraggingItem && isDropTarget}
            style="{isDraggingItem && isDropTarget ? `border-color: ${dropZoneStates.get(section.id)?.isOver ? 'var(--ds-status-info-solid)' : 'var(--ds-border)'}; background-color: ${dropZoneStates.get(section.id)?.isOver ? 'var(--ds-status-info-bg)' : 'transparent'}; padding: 0.5rem;` : ''}"
            data-section-drop-zone
            data-section-id={section.id}
          >
            {#if gridItems.length > 0}
              <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
                {#each gridItems as entry (entry.kind + ':' + entry.item.id)}
                  {@const Icon = iconMap[entry.item.icon] || Package}
                  {@const isAssetForm = entry.kind === 'asset-form'}
                  <button
                    type="button"
                    data-testid={isAssetForm ? 'portal-asset-form-card' : 'portal-request-type-card'}
                    id={isAssetForm ? `portal-asset-form-${entry.item.id}` : undefined}
                    class="appearance-none font-[inherit] text-[inherit] text-left w-full m-0 rounded-md p-4 border transition-colors cursor-pointer relative group hover:border-[var(--ds-border-focused)]"
                    style="background-color: var(--ds-surface-card); border-color: var(--ds-border);"
                    onclick={() => isAssetForm ? onOpenAssetReportForm(entry.item) : onOpenRequestForm(entry.item)}
                  >
                    {#if portalStore.isEditing || (portalStore.showCustomizePanel && (portalStore.activeSection === 'request-types' || portalStore.activeSection === 'asset-reports'))}
                      <span
                        role="button"
                        tabindex="-1"
                        onclick={(e) => {
                          e.stopPropagation();
                          if (isAssetForm) {
                            portalCatalogStore.removeAssetReportFromSection(section.id, entry.item.id);
                          } else {
                            portalCatalogStore.removeRequestTypeFromSection(section.id, entry.item.id);
                          }
                        }}
                        onkeydown={(e) => {
                          if (e.key === 'Enter' || e.key === ' ') {
                            e.stopPropagation();
                            if (isAssetForm) {
                              portalCatalogStore.removeAssetReportFromSection(section.id, entry.item.id);
                            } else {
                              portalCatalogStore.removeRequestTypeFromSection(section.id, entry.item.id);
                            }
                          }
                        }}
                        class="absolute top-2 right-2 p-1 rounded opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer"
                        style="background-color: var(--ds-danger-subtle); color: var(--ds-text-danger);"
                        title={t('portal.removeFromSection')}
                      >
                        <X class="w-3 h-3" />
                      </span>
                    {/if}
                    <div class="flex items-start gap-3.5 pr-5">
                      <div
                        class="w-9 h-9 rounded-md flex items-center justify-center flex-none"
                        style="color: {entry.item.color || 'var(--ds-text-subtle)'}; background-color: color-mix(in srgb, {entry.item.color || 'var(--ds-text-subtle)'} 12%, transparent);"
                      >
                        <Icon size={18} />
                      </div>
                      <div class="min-w-0 flex-1">
                        <div class="text-sm font-medium leading-5 flex items-center gap-2" style="color: var(--ds-text);">
                          {entry.item.name}
                          {#if !entry.item.is_active}
                            <span
                              class="px-1.5 py-0.5 text-[10px] font-medium rounded"
                              style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
                            >
                              INACTIVE
                            </span>
                          {/if}
                        </div>
                        {#if entry.item.description}
                          <p class="text-sm mt-1.5 line-clamp-2 leading-5" style="color: var(--ds-text-subtle);">
                            {entry.item.description}
                          </p>
                        {/if}
                      </div>
                      <ChevronRight class="w-4 h-4 mt-2 flex-none" style="color: var(--ds-text-subtle);" />
                    </div>
                  </button>
                {/each}
              </div>

              <!-- Drop zone indicator when dragging over section with items -->
              {#if isDraggingItem && dropZoneStates.get(section.id)?.isOver && isDropTarget}
                <div class="mt-4 text-center py-4 border-2 border-dashed rounded" style="border-color: var(--ds-status-info-border); background-color: var(--ds-status-info-bg);">
                  <p class="text-sm font-medium" style="color: var(--ds-status-info-text);">
                    {t('portal.dropHereToAdd')}
                  </p>
                </div>
              {/if}
            {:else if isDropTarget}
              <div
                class="text-center py-8 border-2 border-dashed rounded transition-all"
                style="border-color: {dropZoneStates.get(section.id)?.isOver ? 'var(--ds-status-info-solid)' : 'var(--ds-border)'}; background-color: {dropZoneStates.get(section.id)?.isOver ? 'var(--ds-status-info-bg)' : 'transparent'};"
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

          <!-- Asset Reports (direct mode, full width) -->
          {#if directAssetReports.length > 0}
            <div class="mt-6 space-y-4">
              {#each directAssetReports as report}
                <AssetReportTable
                  {report}
                  slug={portalStore.currentSlug}
                  sectionId={section.id}
                  isEditing={portalStore.isEditing || (portalStore.showCustomizePanel && portalStore.activeSection === 'asset-reports')}
                  onRemove={(reportId) => portalCatalogStore.removeAssetReportFromSection(section.id, reportId)}
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
  {#if portalCatalogStore.portalSections.length === 0 && !portalStore.isEditing}
    <div class="text-center py-16">
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        {t('portal.noContentSections')}
      </p>
    </div>
  {/if}
</div>
