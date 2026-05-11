<script>
  import { Search, Plus, X } from '@lucide/svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import DynamicFieldFilter from './DynamicFieldFilter.svelte';
  import Button from '../../components/Button.svelte';
  import Panel from '../../components/Panel.svelte';
  import Modal from '../../dialogs/Modal.svelte';

  let {
    workspaces = [],
    allStatuses = [],
    allPriorities = [],
    selectedWorkspaces = [],
    selectedStatuses = [],
    selectedPriorities = [],
    searchQuery = '',
    dynamicFilters = [],
    disabled = false,
    /** 'modal' for compact contexts (sidebar), 'inline' for wide pages. */
    searchInputMode = 'modal',
    onupdateworkspaces = null,
    onupdatestatuses = null,
    onupdatepriorities = null,
    onupdatesearch = null,
    onupdatedynamicfilters = null,
    onexecutesearch = null,
  } = $props();

  let showSearchModal = $state(false);
  let tempSearchQuery = $state('');

  function handleWorkspacesChange(newValue) {
    onupdateworkspaces?.(newValue);
    onexecutesearch?.();
  }

  function handleStatusesChange(newValue) {
    onupdatestatuses?.(newValue);
    onexecutesearch?.();
  }

  function handlePrioritiesChange(newValue) {
    onupdatepriorities?.(newValue);
    onexecutesearch?.();
  }

  function openSearchModal() {
    tempSearchQuery = searchQuery;
    showSearchModal = true;
  }

  function closeSearchModal() {
    showSearchModal = false;
  }

  function applySearch() {
    onupdatesearch?.(tempSearchQuery);
    onexecutesearch?.();
    showSearchModal = false;
  }

  function clearSearch() {
    tempSearchQuery = '';
    onupdatesearch?.('');
    onexecutesearch?.();
    showSearchModal = false;
  }

  function addDynamicFilter() {
    const newFilter = { field: null, operator: '=', value: '', values: [] };
    onupdatedynamicfilters?.([...dynamicFilters, newFilter]);
  }

  function removeDynamicFilter(index) {
    onupdatedynamicfilters?.(dynamicFilters.filter((_, i) => i !== index));
    onexecutesearch?.();
  }

  function handleDynamicFilterChange(index, data) {
    const updated = [...dynamicFilters];
    updated[index] = data;
    onupdatedynamicfilters?.(updated);
    onexecutesearch?.();
  }

  function handleDynamicFilterExecute() {
    onexecutesearch?.();
  }
</script>

{#if disabled}
  <div class="mb-4">
    <Panel padding="compact" rounded="md">
      <div class="text-xs" style="color: var(--ds-text-subtle);">
        <div class="font-medium mb-1" style="color: var(--ds-text);">{t('collections.builderDisabled')}</div>
        <div>{t('collections.builderDisabledDesc')}</div>
      </div>
    </Panel>
  </div>
{/if}
<div
  class:pointer-events-none={disabled}
  class:opacity-50={disabled}
  aria-disabled={disabled ? 'true' : undefined}
>
  <!-- Search input -->
  <div class="mb-4">
    {#if searchInputMode === 'inline'}
      <div class="relative">
        <Search
          class="w-4 h-4 absolute left-2.5 top-1/2 -translate-y-1/2 pointer-events-none"
          style="color: var(--ds-icon-subtle);"
        />
        <input
          type="text"
          value={searchQuery}
          oninput={(e) => onupdatesearch?.(e.currentTarget.value)}
          onkeydown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              onexecutesearch?.();
            }
          }}
          placeholder={t('collections.searchItems')}
          class="w-full pl-9 pr-3 py-1.5 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
          style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
        />
        {#if searchQuery}
          <button
            onclick={() => {
              onupdatesearch?.('');
              onexecutesearch?.();
            }}
            class="absolute right-2 top-1/2 -translate-y-1/2 p-0.5 rounded transition-colors"
            style="color: var(--ds-text-subtle);"
            title={t('collections.clearSearch')}
          >
            <X class="w-3 h-3" />
          </button>
        {/if}
      </div>
    {:else}
      <button
        onclick={openSearchModal}
        class="w-full flex items-center gap-2 px-2.5 py-1.5 text-sm border rounded transition-colors"
        style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text-subtle);"
        onmouseenter={(e) => (e.currentTarget.style.borderColor = 'var(--ds-border-bold)')}
        onmouseleave={(e) => (e.currentTarget.style.borderColor = 'var(--ds-border)')}
      >
        <Search class="w-4 h-4 flex-shrink-0" style="color: var(--ds-icon-subtle);" />
        {#if searchQuery}
          <span class="truncate text-left flex-1" style="color: var(--ds-text);">{searchQuery}</span>
          <button
            onclick={(e) => {
              e.stopPropagation();
              clearSearch();
            }}
            class="p-0.5 rounded transition-colors flex-shrink-0"
            style="color: var(--ds-text-subtle);"
            title={t('collections.clearSearch')}
          >
            <X class="w-3 h-3" />
          </button>
        {:else}
          <span class="text-left flex-1">{t('collections.searchItems')}</span>
        {/if}
      </button>
    {/if}
  </div>

  <!-- Filter pickers -->
  <div class="space-y-4">
    <div>
      <span class="block text-xs font-medium mb-1.5" style="color: var(--ds-text-subtle);">
        {t('collections.workspaces')}
      </span>
      <BasePicker
        items={workspaces}
        value={selectedWorkspaces}
        multiple={true}
        placeholder={t('collections.selectWorkspaces')}
        getValue={(item) => item?.id}
        getLabel={(item) => item?.name ?? ''}
        onChange={handleWorkspacesChange}
      />
    </div>

    <div>
      <span class="block text-xs font-medium mb-1.5" style="color: var(--ds-text-subtle);">
        {t('collections.status')}
      </span>
      <BasePicker
        items={allStatuses}
        value={selectedStatuses}
        multiple={true}
        placeholder={t('collections.selectStatuses')}
        getValue={(item) => item?.id}
        getLabel={(item) => item?.name ?? ''}
        onChange={handleStatusesChange}
      />
    </div>

    <div>
      <span class="block text-xs font-medium mb-1.5" style="color: var(--ds-text-subtle);">
        {t('collections.priority')}
      </span>
      <BasePicker
        items={allPriorities}
        value={selectedPriorities}
        multiple={true}
        placeholder={t('collections.selectPriorities')}
        getValue={(item) => item?.id}
        getLabel={(item) => item?.name ?? ''}
        onChange={handlePrioritiesChange}
      />
    </div>

    {#each dynamicFilters as filter, index (index)}
      <DynamicFieldFilter
        {filter}
        compact={true}
        onchange={(data) => handleDynamicFilterChange(index, data)}
        onremove={() => removeDynamicFilter(index)}
        onexecute={handleDynamicFilterExecute}
      />
    {/each}

    <Button
      variant="ghost"
      size="sm"
      icon={Plus}
      onclick={addDynamicFilter}
      class="w-full justify-start"
    >
      {t('collections.addFieldFilter')}
    </Button>
  </div>
</div>

<Modal bind:isOpen={showSearchModal} maxWidth="max-w-md" onclose={closeSearchModal} onSubmit={applySearch}>
  <div class="p-4">
    <h3 class="text-lg font-semibold mb-4" style="color: var(--ds-text);">
      {t('collections.searchItemsTitle')}
    </h3>
    <input
      type="text"
      bind:value={tempSearchQuery}
      placeholder={t('collections.enterSearchText')}
      class="w-full px-3 py-2 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
      style="background-color: var(--ds-surface); border-color: var(--ds-border); color: var(--ds-text);"
    />
    <div class="flex justify-end gap-2 mt-4">
      <Button variant="ghost" size="sm" onclick={clearSearch}>{t('collections.clear')}</Button>
      <Button variant="ghost" size="sm" onclick={closeSearchModal}>{t('common.cancel')}</Button>
      <Button variant="primary" size="sm" onclick={applySearch}>{t('collections.apply')}</Button>
    </div>
  </div>
</Modal>
