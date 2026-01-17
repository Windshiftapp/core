<script>
  import { FileText, Link2, Trash2 } from 'lucide-svelte';
  import { itemTypeIconMap } from '../../utils/icons.js';
  import Button from '../../components/Button.svelte';
  import Tooltip from '../../components/Tooltip.svelte';
  import LinkComponent from '../../components/Link.svelte';
  import BasePicker from '../../pickers/BasePicker.svelte';
  import { createEventDispatcher } from 'svelte';
  import { t } from '../../stores/i18n.svelte.js';

  const dispatch = createEventDispatcher();

  export let item;
  export let workspace;
  export let workspaceId;
  export let itemId;
  export let itemLinks = [];
  export let loadingLinks = false;
  export let availableSubIssueTypes = [];
  export let childItems = [];
  export let loadingChildItems = false;
  export let showAddLinkForm = false;
  export let addLinkData = { link_type_id: null, target_id: null, target_title: '', target_type: 'item' };
  export let linkTypes = [];
  export let searchResults = [];
  export let searchQuery = '';
  export let searching = false;
  export let itemTypes = [];
  export let isModal = false;

  const TEST_LINK_TYPE_ID = 1;
  $: selectedLinkTypeId = addLinkData?.link_type_id ? Number(addLinkData.link_type_id) : null;
  $: isTestLinkTypeSelected = selectedLinkTypeId === TEST_LINK_TYPE_ID;
  $: searchPlaceholder = isTestLinkTypeSelected ? t('items.searchTestCases') : t('items.searchWorkItems');
  $: searchDisabled = !addLinkData?.link_type_id;

  // Use centralized icon map for item types
  $: currentItemId = parseInt(itemId);
  const iconMap = itemTypeIconMap;
  
  function getLinkLabel(link) {
    const isCurrentSource = currentItemId === link.source_id;
    if (link.link_type_id === TEST_LINK_TYPE_ID && isCurrentSource && link.source_type === 'item' && link.target_type === 'test_case') {
      return link.link_type_reverse_label;
    }
    return isCurrentSource ? link.link_type_forward_label : link.link_type_reverse_label;
  }

  function handleLinkClick(event, linkedItemType, linkedItemId, linkedItemWorkspaceId, linkedItemHref) {
    if (linkedItemType === 'test_case') {
      event.preventDefault();
      dispatch('view-test-case', { testCaseId: linkedItemId });
      return;
    }

    if (isModal) {
      event.preventDefault();
      const targetWorkspaceId = linkedItemWorkspaceId || workspaceId;
      const destination = linkedItemHref || `/workspaces/${targetWorkspaceId}/items/${linkedItemId}`;
      dispatch('navigate', { path: destination });
    }
  }

  function startCreateSubIssue() {
    dispatch('create-sub-issue');
  }

  
  function removeLink(linkId) {
    dispatch('remove-link', { linkId });
  }
  
  function selectItem(selectedItem) {
    dispatch('select-item', { selectedItem });
  }
  
  function addLink() {
    dispatch('add-link');
  }
  
  function cancelAddLink() {
    showAddLinkForm = false;
    addLinkData = { link_type_id: null, target_id: null, target_title: '', target_type: 'item' };
    searchQuery = '';
    searchResults = [];
  }
  
  // Debug data
  $: {
    if (childItems && childItems.length > 0) {
    }
  }
</script>

<!-- Links Section -->
{#if itemLinks.length > 0 || showAddLinkForm}
  <div class="mt-6">
    <div class="pt-2">
      <!-- Header with icon and label -->
      <div class="flex items-center gap-2 mb-4">
        <Link2 class="w-4 h-4" style="color: var(--ds-text-subtle);" />
        <h3 class="text-sm font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle); font-size: 11px;">{t('items.linkedItems')}</h3>
      </div>

      {#if loadingLinks}
        <div class="text-center py-4">
          <div class="text-sm text-gray-500">{t('items.loadingLinks')}</div>
        </div>
      {:else}
      <div class="space-y-2">
        {#each itemLinks as link}
          {@const isCurrentSource = link.source_id === currentItemId}
          {@const linkedItemType = isCurrentSource ? link.target_type : link.source_type}
          {@const linkedItemId = isCurrentSource ? link.target_id : link.source_id}
          {@const linkedItemWorkspaceId = isCurrentSource ? link.target_workspace_id : link.source_workspace_id}
          {@const linkedItemKeyPrefix = linkedItemType === 'test_case'
            ? 'TC'
            : (isCurrentSource
              ? (link.target_workspace_key || workspace?.key || 'WORK')
              : (link.source_workspace_key || workspace?.key || 'WORK'))}
          {@const linkedItemKey = `${linkedItemKeyPrefix}-${linkedItemId}`}
          {@const linkedItemTitle = isCurrentSource ? link.target_title : link.source_title}
          {@const linkedItemHref = linkedItemType === 'test_case'
            ? '#view-test-case'
            : `/workspaces/${linkedItemWorkspaceId || workspaceId}/items/${linkedItemId}`}
          {@const isLinkedTestCase = linkedItemType === 'test_case'}
          {@const linkedItemTypeIcon = isCurrentSource ? link.target_item_type_icon : link.source_item_type_icon}
          {@const linkedItemTypeColor = isCurrentSource ? link.target_item_type_color : link.source_item_type_color}
          {@const linkedItemStatusName = isCurrentSource ? link.target_status_name : link.source_status_name}
          <!-- Item row with card styling and hover-reveal delete -->
          <div
            class="group flex items-center justify-between px-4 py-3 rounded-lg border transition-colors"
            style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
          >
            <div class="flex items-center gap-3 flex-1 min-w-0">
              <!-- Item type icon -->
              {#if linkedItemTypeIcon && iconMap[linkedItemTypeIcon]}
                <div
                  class="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0"
                  style="background-color: {linkedItemTypeColor || '#6b7280'}20; color: {linkedItemTypeColor || '#6b7280'};"
                >
                  <svelte:component this={iconMap[linkedItemTypeIcon]} class="w-3.5 h-3.5" />
                </div>
              {:else}
                <div
                  class="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0"
                  style="background-color: #6b728020; color: #6b7280;"
                >
                  <FileText class="w-3.5 h-3.5" />
                </div>
              {/if}
              <!-- Item key -->
              <LinkComponent
                href={linkedItemHref}
                class="text-xs font-mono whitespace-nowrap transition-colors cursor-pointer"
                style="color: var(--ds-text-subtle);"
                onClick={(event) => handleLinkClick(event, linkedItemType, linkedItemId, linkedItemWorkspaceId, linkedItemHref)}
              >
                {linkedItemKey}
              </LinkComponent>
              <!-- Item title -->
              <LinkComponent
                href={linkedItemHref}
                class="text-sm hover:text-blue-600 cursor-pointer truncate"
                onClick={(event) => handleLinkClick(event, linkedItemType, linkedItemId, linkedItemWorkspaceId, linkedItemHref)}
                style="color: var(--ds-text);"
              >
                {linkedItemTitle}
              </LinkComponent>
            </div>
            <!-- Right side: status badge + delete button -->
            <div class="flex items-center gap-2 flex-shrink-0">
              {#if linkedItemStatusName}
                <span class="text-xs px-2 py-0.5 rounded-full" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                  {linkedItemStatusName}
                </span>
              {/if}
              <button
                class="p-1 rounded hidden group-hover:flex cursor-pointer delete-button"
                style="color: var(--ds-text-subtle);"
                onmouseenter={(e) => e.currentTarget.style.color = '#dc2626'}
                onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-text-subtle)'}
                onclick={() => removeLink(link.id)}
                title={t('items.removeLink')}
              >
                <Trash2 class="w-4 h-4" />
              </button>
            </div>
          </div>
        {/each}
      </div>
    {/if}
    
    <!-- Add Link Form -->
    {#if showAddLinkForm}
      <div class="mt-4 pt-4 border-t" style="border-color: var(--ds-border);">
        <h4 class="text-sm font-medium mb-3" style="color: var(--ds-text);">{t('items.addLink')}</h4>
        
        <div class="space-y-3">
          <div class="space-y-1">
            <label class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('items.linkType')}</label>
            <BasePicker
              bind:value={addLinkData.link_type_id}
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
          
          <div>
            <label class="block text-xs font-medium mb-1" style="color: var(--ds-text-subtle);">{t('items.targetItem')}</label>
            {#if addLinkData.target_id}
              <div class="flex items-center justify-between py-2">
                <div>
                  <div class="text-xs uppercase tracking-wide text-gray-400">
                    {addLinkData.target_type === 'test_case' ? t('items.testCase') : t('items.workItem')}
                  </div>
                  <div class="text-sm" style="color: var(--ds-text);">{addLinkData.target_title}</div>
                </div>
                <button
                  class="text-red-600 hover:text-red-800 text-xs cursor-pointer"
                  onclick={() => {
                    addLinkData.target_id = null;
                    addLinkData.target_title = '';
                    addLinkData.target_type = isTestLinkTypeSelected ? 'test_case' : 'item';
                  }}
                >
                  {t('common.clear')}
                </button>
              </div>
            {:else}
              <div class="relative" style="position: relative;">
                <input
                  type="text"
                  bind:value={searchQuery}
                  placeholder={searchPlaceholder}
                  class="w-full px-3 py-2 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:bg-gray-100 disabled:text-gray-400"
                  style="border-color: var(--ds-border); background-color: var(--ds-surface-raised); color: var(--ds-text);"
                  disabled={searchDisabled}
                />
                
                {#if searchDisabled}
                  <p class="text-xs text-gray-400 mt-2">{t('items.selectLinkTypeToSearch')}</p>
                {/if}
                
                {#if searching}
                  <div class="absolute right-3 top-2.5">
                    <div class="w-4 h-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin"></div>
                  </div>
                {/if}
                
                {#if searchResults.length > 0}
                  <div class="absolute z-50 w-full mt-1 border rounded shadow-lg max-h-40 overflow-y-auto" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
                    {#each searchResults as result}
                      <button
                        class="w-full text-left px-3 py-2 hover:bg-gray-50 cursor-pointer border-b last:border-b-0"
                        style="color: var(--ds-text); border-color: var(--ds-border);"
                        onclick={() => selectItem(result)}
                      >
                        <div class="flex items-center justify-between gap-2">
                          <div class="font-medium text-sm truncate">{result.title}</div>
                          <span class="text-[10px] uppercase tracking-wide px-1.5 py-0.5 rounded-full {result.type === 'test_case' ? 'bg-purple-100 text-purple-600' : 'bg-gray-100 text-gray-600'}">
                            {result.type === 'test_case' ? t('items.testCase') : t('items.workItem')}
                          </span>
                        </div>
                        <div class="text-xs text-gray-500 mt-0.5">
                          {#if result.type === 'test_case'}
                            {result.description || `Test Case #${result.id}`}
                          {:else}
                            {(result.workspace_name || 'Workspace')} · ID {result.id}
                          {/if}
                        </div>
                      </button>
                    {/each}
                  </div>
                {/if}
              </div>
            {/if}
          </div>
          
          <div class="flex gap-2 pt-2">
            <Button
              variant="primary"
              size="small"
              disabled={!addLinkData.link_type_id || !addLinkData.target_id}
              onclick={addLink}
            >
              {t('items.addLink')}
            </Button>
            <Button
              variant="secondary"
              size="small"
              onclick={cancelAddLink}
            >
              {t('common.cancel')}
            </Button>
          </div>
        </div>
      </div>
    {/if}
  </div>
</div>
{/if}

<!-- Child Work Items Section -->
{#if childItems.length > 0}
  <div class="mt-4">
    <div class="pt-2">
      <div class="flex items-center gap-2 mb-4">
        <FileText class="w-4 h-4" style="color: var(--ds-text-subtle);" />
        <h3 class="text-sm font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle); font-size: 11px;">{t('items.childWorkItems')}</h3>
      </div>

      {#if loadingChildItems}
        <div class="text-center py-4">
          <div class="text-sm" style="color: var(--ds-text-subtle);">{t('items.loadingChildItems')}</div>
        </div>
      {:else}
        <div class="space-y-2">
          {#each childItems as childItem}
            {@const childItemType = itemTypes.find(type => type.id === childItem.item_type_id)}
            <div
              class="group flex items-center justify-between px-4 py-3 rounded-lg border transition-colors"
              style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
            >
              <div class="flex items-center gap-3 flex-1 min-w-0">
                <!-- Item type icon -->
                {#if childItemType}
                  <div
                    class="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0"
                    style="background-color: {childItemType.color || '#6b7280'}20; color: {childItemType.color || '#6b7280'};"
                  >
                    <svelte:component this={iconMap[childItemType.icon] || FileText} class="w-3.5 h-3.5" />
                  </div>
                {:else}
                  <div
                    class="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0"
                    style="background-color: #6b728020; color: #6b7280;"
                  >
                    <FileText class="w-3.5 h-3.5" />
                  </div>
                {/if}
                <!-- Item key -->
                <LinkComponent
                  href="/workspaces/{childItem.workspace_id || workspaceId}/items/{childItem.id}"
                  class="text-xs font-mono whitespace-nowrap transition-colors cursor-pointer"
                  style="color: var(--ds-text-subtle);"
                >
                  {childItem.workspace_key || childItem.workspace?.key || workspace?.key || 'WORK'}-{childItem.id}
                </LinkComponent>
                <!-- Item title -->
                <LinkComponent
                  href="/workspaces/{childItem.workspace_id || workspaceId}/items/{childItem.id}"
                  class="text-sm hover:text-blue-600 cursor-pointer truncate"
                  style="color: var(--ds-text);"
                >
                  {childItem.title}
                </LinkComponent>
              </div>
              <!-- Right side: status badge -->
              <div class="flex items-center gap-2 flex-shrink-0">
                {#if childItem.status_name}
                  <span class="text-xs px-2 py-0.5 rounded-full" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                    {childItem.status_name}
                  </span>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .delete-button {
    animation: fadeIn 150ms ease-out;
  }

  @keyframes fadeIn {
    from {
      opacity: 0;
    }
    to {
      opacity: 1;
    }
  }
</style>
