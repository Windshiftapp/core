<script>
  import { createEventDispatcher } from 'svelte';
  import Modal from './Modal.svelte';
  import ModalHeader from './ModalHeader.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import { FileText } from 'lucide-svelte';
  import { itemTypeIconMap } from '../utils/icons.js';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';

  const dispatch = createEventDispatcher();
  const iconMap = itemTypeIconMap;
  const TEST_LINK_TYPE_ID = 1;

  let {
    isOpen = $bindable(false),
    linkTypes = [],
    currentItemId = null,
    onsubmit = null,
    oncancel = null
  } = $props();

  // Form state
  let formData = $state({
    link_type_id: null,
    target_id: null,
    target_title: '',
    target_type: 'item'
  });

  // Search state
  let searchQuery = $state('');
  let searchResults = $state([]);
  let searching = $state(false);
  let highlightedIndex = $state(-1);
  let searchTimeout = null;
  let inputRef = $state(null);

  // Derived state
  let selectedLinkTypeId = $derived(formData.link_type_id ? Number(formData.link_type_id) : null);
  let isTestLinkTypeSelected = $derived(selectedLinkTypeId === TEST_LINK_TYPE_ID);
  let searchPlaceholder = $derived(isTestLinkTypeSelected ? t('items.searchTestCases') : t('items.searchWorkItems'));
  let searchDisabled = $derived(!formData.link_type_id);
  let canSubmit = $derived(formData.link_type_id && formData.target_id);

  // Reactive search when query changes
  $effect(() => {
    const trimmedQuery = (searchQuery || '').trim();
    const searchType = isTestLinkTypeSelected ? 'test_case' : 'item';

    if (trimmedQuery.length >= 2 && formData.link_type_id) {
      clearTimeout(searchTimeout);
      searchTimeout = setTimeout(async () => {
        try {
          searching = true;
          const results = await api.links.search(trimmedQuery, searchType, 10);
          const items = Array.isArray(results) ? results : [];
          searchResults = searchType === 'item'
            ? items.filter(item => item.id !== currentItemId)
            : items;
          highlightedIndex = searchResults.length > 0 ? 0 : -1;
        } catch (error) {
          console.error('Search failed:', error);
          searchResults = [];
          highlightedIndex = -1;
        } finally {
          searching = false;
        }
      }, 300);
    } else {
      clearTimeout(searchTimeout);
      searchResults = [];
      highlightedIndex = -1;
      searching = false;
    }
  });

  // Reset target when link type changes
  $effect(() => {
    const isTestLink = selectedLinkTypeId === TEST_LINK_TYPE_ID;
    if (!isTestLink && formData.target_type === 'test_case') {
      formData.target_id = null;
      formData.target_title = '';
      formData.target_type = 'item';
      searchQuery = '';
      searchResults = [];
      highlightedIndex = -1;
    }
  });

  function handleKeyDown(e) {
    // Only handle keyboard navigation if we have search results
    if (searchResults.length === 0) return;

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      highlightedIndex = (highlightedIndex + 1) % searchResults.length;
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      highlightedIndex = highlightedIndex <= 0 ? searchResults.length - 1 : highlightedIndex - 1;
    } else if (e.key === 'Enter' && highlightedIndex >= 0) {
      e.preventDefault();
      e.stopPropagation(); // Prevent modal submit
      handleSelectItem(searchResults[highlightedIndex]);
    } else if (e.key === 'Escape') {
      e.stopPropagation(); // Prevent modal close
      searchResults = [];
      highlightedIndex = -1;
    } else if (e.key === 'Tab') {
      searchResults = [];
      highlightedIndex = -1;
    }
  }

  function handleSelectItem(item) {
    formData.target_id = item.id;
    formData.target_title = item.title;
    formData.target_type = item.type || (isTestLinkTypeSelected ? 'test_case' : 'item');
    searchQuery = '';
    searchResults = [];
    highlightedIndex = -1;
  }

  function clearTarget() {
    formData.target_id = null;
    formData.target_title = '';
    formData.target_type = isTestLinkTypeSelected ? 'test_case' : 'item';
    searchQuery = '';
    searchResults = [];
    highlightedIndex = -1;
  }

  function handleSubmit() {
    if (!canSubmit) return;

    if (onsubmit) {
      onsubmit({
        link_type_id: formData.link_type_id,
        target_id: formData.target_id,
        target_type: formData.target_type
      });
    }
    dispatch('submit', {
      link_type_id: formData.link_type_id,
      target_id: formData.target_id,
      target_type: formData.target_type
    });
    handleClose();
  }

  function handleClose() {
    // Reset state
    formData = {
      link_type_id: null,
      target_id: null,
      target_title: '',
      target_type: 'item'
    };
    searchQuery = '';
    searchResults = [];
    highlightedIndex = -1;
    isOpen = false;
    oncancel?.();
    dispatch('cancel');
  }
</script>

<Modal
  bind:isOpen
  maxWidth="max-w-md"
  onclose={handleClose}
  onSubmit={handleSubmit}
  submitDisabled={!canSubmit}
  let:submitHint
>
  <ModalHeader
    title={t('items.addLink')}
    onClose={handleClose}
  />

  <div class="p-6">
    <div class="space-y-4">
      <!-- Link Type Picker -->
      <div class="space-y-1">
        <label for="link-type-picker" class="block text-sm font-medium" style="color: var(--ds-text-subtle);">
          {t('items.linkType')}
        </label>
        <BasePicker
          id="link-type-picker"
          bind:value={formData.link_type_id}
          items={linkTypes}
          placeholder={t('items.chooseRelationshipType')}
          showUnassigned={true}
          unassignedLabel={t('items.chooseRelationshipType')}
          getValue={(item) => item.id}
          getLabel={(item) => item.name}
        />
        {#if isTestLinkTypeSelected}
          <p class="text-xs text-blue-600">{t('items.linkToTestCase')}</p>
        {/if}
      </div>

      <!-- Target Item Search -->
      <div>
        <label for="link-target-search" class="block text-sm font-medium mb-1" style="color: var(--ds-text-subtle);">
          {t('items.targetItem')}
        </label>

        {#if formData.target_id}
          <!-- Selected Item Display -->
          <div class="flex items-center justify-between py-2 px-3 border rounded" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div>
              <div class="text-xs uppercase tracking-wide" style="color: var(--ds-text-subtle);">
                {formData.target_type === 'test_case' ? t('items.testCase') : t('items.workItem')}
              </div>
              <div class="text-sm font-medium" style="color: var(--ds-text);">{formData.target_title}</div>
            </div>
            <button
              type="button"
              class="text-red-600 hover:text-red-800 text-xs cursor-pointer"
              onclick={clearTarget}
            >
              {t('common.clear')}
            </button>
          </div>
        {:else}
          <!-- Search Input -->
          <div class="relative">
            <input
              id="link-target-search"
              bind:this={inputRef}
              type="text"
              bind:value={searchQuery}
              onkeydown={handleKeyDown}
              placeholder={searchPlaceholder}
              class="w-full px-3 py-2 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:bg-gray-100 disabled:text-gray-400"
              style="border-color: var(--ds-border); background-color: var(--ds-surface-raised); color: var(--ds-text);"
              disabled={searchDisabled}
            />

            {#if searchDisabled}
              <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
                {t('items.selectLinkTypeToSearch')}
              </p>
            {/if}

            {#if searching}
              <div class="absolute right-3 top-2.5">
                <div class="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
              </div>
            {/if}

            <!-- Search Results Dropdown -->
            {#if searchResults.length > 0}
              <div
                class="absolute z-50 w-full mt-1 border rounded shadow-lg max-h-48 overflow-y-auto"
                style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);"
              >
                {#each searchResults as result, index}
                  {@const isHighlighted = highlightedIndex === index}
                  {@const itemTypeIcon = result.item_type_icon}
                  {@const itemTypeColor = result.item_type_color}
                  <button
                    type="button"
                    class="w-full text-left px-3 py-2 cursor-pointer border-b last:border-b-0 transition-colors"
                    style="color: var(--ds-text); border-color: var(--ds-border); {isHighlighted ? 'background-color: var(--ds-background-neutral-hovered);' : ''}"
                    onmouseenter={() => highlightedIndex = index}
                    onclick={() => handleSelectItem(result)}
                  >
                    <div class="flex items-center gap-2">
                      <!-- Item type icon -->
                      {#if itemTypeIcon && iconMap[itemTypeIcon]}
                        <div
                          class="w-5 h-5 rounded-full flex items-center justify-center flex-shrink-0"
                          style="background-color: {itemTypeColor || '#6b7280'}20; color: {itemTypeColor || '#6b7280'};"
                        >
                          <svelte:component this={iconMap[itemTypeIcon]} class="w-3 h-3" />
                        </div>
                      {:else}
                        <div
                          class="w-5 h-5 rounded-full flex items-center justify-center flex-shrink-0"
                          style="background-color: #6b728020; color: #6b7280;"
                        >
                          <FileText class="w-3 h-3" />
                        </div>
                      {/if}
                      <div class="flex-1 min-w-0">
                        <div class="font-medium text-sm truncate">{result.title}</div>
                        <div class="text-xs" style="color: var(--ds-text-subtle);">
                          {#if result.type === 'test_case'}
                            {result.description || `Test Case #${result.id}`}
                          {:else}
                            {result.workspace_name || 'Workspace'} · ID {result.id}
                          {/if}
                        </div>
                      </div>
                      <span class="text-[10px] uppercase tracking-wide px-1.5 py-0.5 rounded-full flex-shrink-0 {result.type === 'test_case' ? 'bg-purple-100 text-purple-600' : 'bg-gray-100 text-gray-600'}">
                        {result.type === 'test_case' ? t('items.testCase') : t('items.workItem')}
                      </span>
                    </div>
                  </button>
                {/each}
              </div>
            {/if}
          </div>
        {/if}
      </div>
    </div>
  </div>

  <DialogFooter
    onCancel={handleClose}
    onConfirm={handleSubmit}
    confirmLabel={t('items.addLink')}
    cancelLabel={t('common.cancel')}
    disabled={!canSubmit}
    confirmKeyboardHint={submitHint}
    showKeyboardHint={true}
  />
</Modal>
