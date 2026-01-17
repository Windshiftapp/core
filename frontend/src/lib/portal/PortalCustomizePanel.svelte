<script>
  import { onMount, onDestroy, tick } from 'svelte';
  import { draggable } from '@atlaskit/pragmatic-drag-and-drop/element/adapter';
  import {
    Palette, Navigation, X, TextCursorInput, BookOpen, Check,
    Plus, Trash2, Edit, Pencil, MoreHorizontal, GripVertical,
    Eye, EyeOff, Package
  } from 'lucide-svelte';
  import Tooltip from '../components/Tooltip.svelte';
  import Spinner from '../components/Spinner.svelte';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import { portalStore, gradients, iconMap } from '../stores/portal.svelte.js';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';

  let {
    onOpenFieldsModal = () => {},
    onOpenRequestTypeModal = () => {}
  } = $props();

  let showCustomizePanelHover = $state(false);

  // Request type renaming state
  let renamingRequestTypeId = $state(null);
  let renamingValue = $state('');

  function startRenaming(requestType) {
    renamingRequestTypeId = requestType.id;
    renamingValue = requestType.name;
  }

  function cancelRenaming() {
    renamingRequestTypeId = null;
    renamingValue = '';
  }

  async function renameRequestType(id) {
    if (!renamingValue.trim()) return;

    try {
      const requestType = portalStore.requestTypes.find(rt => rt.id === id);
      if (!requestType) {
        console.error('Request type not found');
        return;
      }

      await api.requestTypes.update(id, {
        name: renamingValue.trim(),
        item_type_id: requestType.item_type_id
      });
      await portalStore.loadRequestTypes();
      cancelRenaming();
    } catch (err) {
      console.error('Failed to rename request type:', err);
    }
  }

  async function toggleRequestTypeActive(requestType) {
    try {
      await api.requestTypes.update(requestType.id, {
        name: requestType.name,
        item_type_id: requestType.item_type_id,
        is_active: !requestType.is_active
      });
      await portalStore.loadRequestTypes();
    } catch (err) {
      console.error('Failed to toggle request type active status:', err);
    }
  }

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

  function setupDraggables() {
    // Only setup if we're on the request-types section
    if (portalStore.activeSection !== 'request-types') return;

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

  onMount(() => {
    // Setup after DOM is ready
    setTimeout(setupDraggables, 100);
  });

  onDestroy(() => {
    cleanupFunctions.forEach(fn => fn());
    cleanupFunctions = [];
  });

  // Re-setup when request types change or section changes
  $effect(() => {
    // Track dependencies
    const currentIds = portalStore.requestTypes.map(rt => rt.id).join(',');
    const isRequestTypesSection = portalStore.activeSection === 'request-types';

    if (currentIds !== lastRequestTypeIds || isRequestTypesSection) {
      lastRequestTypeIds = currentIds;
      // Cleanup previous
      cleanupFunctions.forEach(fn => fn());
      cleanupFunctions = [];
      // Wait for DOM to update then re-setup
      setTimeout(setupDraggables, 100);
    }
  });
</script>

<!-- Customization Panel Overlay (hide when editing request types so sections are visible) -->
{#if portalStore.showCustomizePanel && portalStore.activeSection !== 'request-types'}
  <div
    class="fixed inset-0 bg-black/30 z-50 transition-opacity"
    onclick={() => portalStore.showCustomizePanel = false}
  ></div>
{/if}

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
          <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text);">{t('portal.customize.gradientStyle')}</h3>
          <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">{t('portal.customize.gradientDescription')}</p>
        </div>

        <!-- Gradient Grid -->
        <div class="grid grid-cols-6 gap-3">
          {#each gradients as gradient, index}
            <button
              onclick={() => portalStore.selectGradient(index)}
              class="group relative w-[25px] h-[25px] rounded overflow-hidden transition-all hover:scale-110"
              class:ring-2={portalStore.selectedGradient === index}
              class:ring-blue-500={portalStore.selectedGradient === index}
              class:ring-offset-2={portalStore.selectedGradient === index}
              title={gradient.name}
            >
              <!-- Gradient Preview -->
              <div
                class="w-full h-full"
                style="background: {gradient.value};"
              ></div>

              <!-- Selected Indicator -->
              {#if portalStore.selectedGradient === index}
                <div class="absolute inset-0 flex items-center justify-center bg-black/20">
                  <div class="w-3 h-3 bg-white rounded-full flex items-center justify-center">
                    <svg class="w-2 h-2 text-blue-600" fill="currentColor" viewBox="0 0 20 20">
                      <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                    </svg>
                  </div>
                </div>
              {/if}
            </button>
          {/each}
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
              <div
                class="p-3 rounded border"
                style="background-color: {portalStore.isDarkMode ? '#334155' : '#f9fafb'}; border-color: {portalStore.isDarkMode ? '#475569' : '#e5e7eb'};"
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
                    {#if renamingRequestTypeId === requestType.id}
                      <!-- Rename Input -->
                      <input
                        type="text"
                        bind:value={renamingValue}
                        onkeydown={(e) => {
                          if (e.key === 'Enter') renameRequestType(requestType.id);
                          if (e.key === 'Escape') cancelRenaming();
                        }}
                        onblur={() => renameRequestType(requestType.id)}
                        class="w-full px-2 py-1 text-sm font-medium rounded border focus:outline-none focus:ring-2 focus:ring-blue-500"
                        style="background-color: {portalStore.isDarkMode ? '#1e293b' : '#ffffff'}; color: {portalStore.isDarkMode ? '#e2e8f0' : '#111827'}; border-color: {portalStore.isDarkMode ? '#475569' : '#d1d5db'};"
                        autofocus
                      />
                    {:else}
                      <div class="font-medium text-sm mb-1 flex items-center gap-2" style="color: {portalStore.isDarkMode ? '#e2e8f0' : '#111827'};">
                        {requestType.name}
                        {#if !requestType.is_active}
                          <span
                            class="px-1.5 py-0.5 text-[10px] font-medium rounded"
                            style="background-color: {portalStore.isDarkMode ? 'rgba(156, 163, 175, 0.2)' : '#f3f4f6'}; color: {portalStore.isDarkMode ? '#9ca3af' : '#6b7280'};"
                          >
                            {t('common.inactive').toUpperCase()}
                          </span>
                        {/if}
                      </div>
                    {/if}
                    {#if requestType.description}
                      <div class="text-xs mb-2" style="color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};">
                        {requestType.description}
                      </div>
                    {/if}
                    <div class="flex items-center gap-2">
                      <div class="text-xs" style="color: {portalStore.isDarkMode ? '#94a3b8' : '#6b7280'};">
                        {t('portal.customize.creates')}: {requestType.item_type_name || t('common.unknown')}
                      </div>
                      <span style="color: {portalStore.isDarkMode ? '#475569' : '#e5e7eb'};">•</span>
                      <button
                        onclick={() => onOpenFieldsModal(requestType)}
                        class="text-xs hover:underline"
                        style="color: {portalStore.isDarkMode ? '#60a5fa' : '#2563eb'};"
                      >
                        {t('portal.customize.fields')}
                      </button>
                    </div>
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
                        {
                          title: t('portal.customize.rename'),
                          icon: Pencil,
                          onClick: () => startRenaming(requestType)
                        },
                        { type: 'divider' },
                        {
                          title: requestType.is_active ? t('portal.customize.markAsInactive') : t('portal.customize.markAsActive'),
                          icon: requestType.is_active ? EyeOff : Eye,
                          onClick: () => toggleRequestTypeActive(requestType)
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
