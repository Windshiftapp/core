<script>
  import Modal from './Modal.svelte';
  import ModalHeader from './ModalHeader.svelte';
  import DialogFooter from './DialogFooter.svelte';
  import LinkItemSearchResult from './LinkItemSearchResult.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import PagePicker from '../pickers/PagePicker.svelte';
  import Input from '../components/Input.svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';
  import { objectDisplayName } from '../utils/systemLabels.js';
  import { useEventListener } from 'runed';
  import { onDestroy } from 'svelte';

  const TEST_LINK_TYPE_ID = 1;

  let {
    isOpen = $bindable(false),
    linkTypes = [],
    currentItemId = null,
    workspaceId = null,
    preselectLinkTypeId = null,
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
  let inputRef = $state(null);
  let searchTimer;
  let searchVersion = 0;

  // Dropdown coordinates. The results dropdown is rendered as `position:
  // fixed` rather than `absolute` so it escapes the modal's `overflow-hidden`
  // content wrapper (Modal.svelte clips rounded corners; an absolutely-
  // positioned child gets cut off when results overflow the modal). Coords
  // are recomputed whenever the input's bounding rect could change: results
  // arrive, the input mounts, the window resizes/scrolls.
  let dropdownStyle = $state('');
  function recomputeDropdownPosition() {
    if (!inputRef || searchResults.length === 0) {
      dropdownStyle = '';
      return;
    }
    const rect = inputRef.getBoundingClientRect();
    dropdownStyle =
      `position: fixed; top: ${rect.bottom + 4}px; ` +
      `left: ${rect.left}px; width: ${rect.width}px;`;
  }
  $effect(() => {
    // Re-read on results change.
    void searchResults.length;
    recomputeDropdownPosition();
  });
  useEventListener(() => window, 'resize', recomputeDropdownPosition);
  useEventListener(() => window, 'scroll', recomputeDropdownPosition, { capture: true });

  // Derived state
  let selectedLinkTypeId = $derived(formData.link_type_id ? Number(formData.link_type_id) : null);
  let selectedLinkType = $derived(linkTypes.find((lt) => Number(lt.id) === selectedLinkTypeId) ?? null);
  let isTestLinkTypeSelected = $derived(selectedLinkTypeId === TEST_LINK_TYPE_ID);
  // A link type whose allowed_entity_types contains "page" routes the
  // target field to the page picker instead of the inline work-item /
  // test-case search. Detection is data-driven so we don't hardcode an id.
  let isPageLinkTypeSelected = $derived.by(() => {
    const allowed = selectedLinkType?.allowed_entity_types;
    return Array.isArray(allowed) && allowed.includes('page');
  });
  let searchPlaceholder = $derived(isTestLinkTypeSelected ? t('items.searchTestCases') : t('items.searchWorkItems'));
  let searchDisabled = $derived(!formData.link_type_id);
  let canSubmit = $derived(formData.link_type_id && formData.target_id);

  onDestroy(() => {
    clearTimeout(searchTimer);
    searchVersion += 1;
  });

  function clearSearch({ clearQuery = false } = {}) {
    clearTimeout(searchTimer);
    searchVersion += 1;
    if (clearQuery) searchQuery = '';
    searchResults = [];
    highlightedIndex = -1;
    searching = false;
  }

  function handleSearchInput(event) {
    const query = event.currentTarget.value;
    searchQuery = query;
    clearTimeout(searchTimer);
    const version = ++searchVersion;
    const trimmedQuery = query.trim();

    if (isPageLinkTypeSelected || trimmedQuery.length < 2 || !formData.link_type_id) {
      searchResults = [];
      highlightedIndex = -1;
      searching = false;
      return;
    }

    const searchType = isTestLinkTypeSelected ? 'test_case' : 'item';
    searching = true;
    searchTimer = setTimeout(() => searchItems(trimmedQuery, searchType, version), 300);
  }

  async function searchItems(query, searchType, version) {
    try {
      const results = await api.links.search(query, searchType, 10);
      if (version !== searchVersion) return;
      const items = Array.isArray(results) ? results : [];
      searchResults = searchType === 'item'
        ? items.filter(item => item.id !== currentItemId)
        : items;
      highlightedIndex = searchResults.length > 0 ? 0 : -1;
    } catch (error) {
      if (version !== searchVersion) return;
      console.error('Search failed:', error);
      searchResults = [];
      highlightedIndex = -1;
    } finally {
      if (version === searchVersion) searching = false;
    }
  }

  // Reset target when link type changes
  let previousSearchLinkTypeId;
  $effect(() => {
    const currentLinkTypeId = selectedLinkTypeId;
    const isTestLink = selectedLinkTypeId === TEST_LINK_TYPE_ID;
    const isPageLink = isPageLinkTypeSelected;
    if (currentLinkTypeId !== previousSearchLinkTypeId) {
      previousSearchLinkTypeId = currentLinkTypeId;
      clearSearch({ clearQuery: true });
    }
    if (!isTestLink && formData.target_type === 'test_case') {
      formData.target_id = null;
      formData.target_title = '';
      formData.target_type = 'item';
    }
    if (!isPageLink && formData.target_type === 'page') {
      formData.target_id = null;
      formData.target_title = '';
      formData.target_type = 'item';
    }
  });

  // Preselect the link type when the caller passes one (used by the
  // separate "Add" buttons on the item-detail "Linked items" vs "Pages"
  // sections so each opens the modal with the right type already chosen).
  // Only fires while the modal is open and only when no type is set yet,
  // so reopens don't clobber an in-flight pick.
  $effect(() => {
    if (!isOpen) return;
    if (preselectLinkTypeId == null) return;
    if (formData.link_type_id != null) return;
    formData.link_type_id = preselectLinkTypeId;
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
      clearSearch();
    } else if (e.key === 'Tab') {
      clearSearch();
    }
  }

  function handleSelectItem(item) {
    formData.target_id = item.id;
    formData.target_title = item.title;
    formData.target_type = item.type || (isTestLinkTypeSelected ? 'test_case' : 'item');
    clearSearch({ clearQuery: true });
  }

  function handleSelectPage(page) {
    if (!page) return;
    formData.target_id = page.id;
    formData.target_title = page.title;
    formData.target_type = 'page';
    clearSearch({ clearQuery: true });
  }

  function clearTarget() {
    formData.target_id = null;
    formData.target_title = '';
    formData.target_type = isPageLinkTypeSelected
      ? 'page'
      : isTestLinkTypeSelected
        ? 'test_case'
        : 'item';
    clearSearch({ clearQuery: true });
  }

  function handleSubmit() {
    if (!canSubmit) return;

    onsubmit?.({
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
    clearSearch({ clearQuery: true });
    isOpen = false;
    oncancel?.();
  }
</script>

<Modal
  bind:isOpen
  maxWidth="max-w-md"
  zIndexClass="z-[60]"
  onclose={handleClose}
  onSubmit={handleSubmit}
  submitDisabled={!canSubmit}
>
  {#snippet children(submitHint)}
  <div data-testid="link-modal">
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
          getLabel={(item) => objectDisplayName(item)}
          searchFields={['display_name', 'name']}
          optionTestid={(opt) => `link-type-option-${opt.value}`}
        />
        {#if isTestLinkTypeSelected}
          <p class="text-xs" style="color: var(--ds-text-info);">{t('items.linkToTestCase')}</p>
        {/if}
        {#if isPageLinkTypeSelected}
          <p class="text-xs" style="color: var(--ds-text-info);">{t('items.linkToPage')}</p>
        {/if}
      </div>

      <!-- Target Item Search -->
      <div>
        <label for="link-target-search" class="block text-sm font-medium mb-1" style="color: var(--ds-text-subtle);">
          {isPageLinkTypeSelected ? t('items.targetPage') : t('items.targetItem')}
        </label>

        {#if formData.target_id}
          <!-- Selected Item Display -->
          <div class="flex items-center justify-between py-2 px-3 border rounded" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
            <div>
              <div class="text-xs uppercase tracking-wide" style="color: var(--ds-text-subtle);">
                {#if formData.target_type === 'test_case'}
                  {t('items.testCase')}
                {:else if formData.target_type === 'page'}
                  {t('items.page')}
                {:else}
                  {t('items.workItem')}
                {/if}
              </div>
              <div class="text-sm font-medium" style="color: var(--ds-text);">{formData.target_title}</div>
            </div>
            <button
              type="button"
              class="text-xs cursor-pointer hover:underline"
              style="color: var(--ds-text-danger);"
              onclick={clearTarget}
            >
              {t('common.clear')}
            </button>
          </div>
        {:else if isPageLinkTypeSelected}
          <PagePicker
            id="link-target-page-picker"
            workspaceId={workspaceId}
            bind:value={formData.target_id}
            placeholder={t('items.pagePickerPlaceholder')}
            onSelect={handleSelectPage}
          />
        {:else}
          <!-- Search Input -->
          <div class="relative">
            <Input
              id="link-target-search"
              bind:inputRef={inputRef}
              type="text"
              bind:value={searchQuery}
              oninput={handleSearchInput}
              onkeydown={handleKeyDown}
              placeholder={searchPlaceholder}
              disabled={searchDisabled}
              size="small"
            />

            {#if searchDisabled}
              <p class="text-xs mt-2" style="color: var(--ds-text-subtle);">
                {t('items.selectLinkTypeToSearch')}
              </p>
            {/if}

            {#if searching}
              <div class="absolute right-3 top-2.5">
                <div class="w-4 h-4 border-2 rounded-full animate-spin" style="border-color: var(--ds-border-focused); border-top-color: transparent;"></div>
              </div>
            {/if}

            <!-- Search Results Dropdown — fixed-positioned so it escapes
                 Modal.svelte's `overflow-hidden` content wrapper. -->
            {#if searchResults.length > 0}
              <div
                class="z-[70] border rounded max-h-48 overflow-y-auto"
                style="{dropdownStyle} border-color: var(--ds-border-bold); background-color: var(--ds-surface-overlay); box-shadow: var(--ds-shadow-raised);"
              >
                {#each searchResults as result, index}
                  <LinkItemSearchResult
                    {result}
                    highlighted={highlightedIndex === index}
                    onhighlight={() => highlightedIndex = index}
                    onselect={() => handleSelectItem(result)}
                  />
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
  </div>
  {/snippet}
</Modal>
