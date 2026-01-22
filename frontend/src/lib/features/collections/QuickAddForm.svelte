<script>
  import { Plus, Package, ChevronDown } from 'lucide-svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { iconMap } from '../../utils/iconMap.js';

  let {
    parentId,
    state,
    workspaces = [],
    hasGradient = false,
    cardBgStyle = '',
    onUpdateField = () => {},
    onCreate = () => {},
    onCancel = () => {}
  } = $props();

  let selectedWorkspace = $derived(workspaces.find(w => w.id === state.workspaceId));
  let selectedItemType = $derived(state.availableTypes?.find(t => t.id === state.itemTypeId));

  // Dropdown management
  let showWorkspaceDropdown = $state(false);
  let showItemTypeDropdown = $state(false);

  function handleKeydown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      onCreate(parentId);
    } else if (e.key === 'Escape') {
      onCancel(parentId);
    }
  }

  function selectWorkspace(workspaceId) {
    onUpdateField(parentId, 'workspaceId', workspaceId);
    showWorkspaceDropdown = false;
  }

  function selectItemType(itemTypeId) {
    onUpdateField(parentId, 'itemTypeId', itemTypeId);
    showItemTypeDropdown = false;
  }
</script>

<div class="rounded shadow-md border" style={cardBgStyle}>
  <!-- Textarea area -->
  <div class="p-3 pb-2">
    <textarea
      value={state.title}
      data-quick-add-parent={parentId}
      oninput={(e) => onUpdateField(parentId, 'title', e.target.value)}
      onkeydown={handleKeydown}
      placeholder={t('collections.enterSummary')}
      rows="2"
      class="w-full px-0 py-0 text-sm resize-none border-0 focus:outline-none focus:ring-0"
      style="background-color: transparent; color: var(--ds-text); caret-color: var(--ds-text);"
    ></textarea>
  </div>

  <!-- Divider -->
  <div class="border-t mx-3" style="border-color: {hasGradient ? 'var(--ds-glass-border)' : 'var(--ds-border)'};"></div>

  <!-- Actions Footer -->
  <div class="p-3 pt-2 flex items-center gap-2 flex-wrap">
    <div class="flex items-center gap-2 flex-wrap">
      <!-- Workspace Selector -->
      <div class="relative">
        <button
          type="button"
          onclick={() => {
            showWorkspaceDropdown = !showWorkspaceDropdown;
            showItemTypeDropdown = false;
          }}
          class="w-7 h-7 rounded-md flex items-center justify-center border overflow-hidden transition-all hover:scale-105"
          style="{selectedWorkspace?.avatar_url ? '' : `background-color: ${selectedWorkspace?.color || 'var(--ds-interactive)'};`} border-color: var(--ds-border);"
          title={selectedWorkspace?.name || 'Select workspace'}
        >
          {#if selectedWorkspace?.avatar_url}
            <img src={selectedWorkspace.avatar_url} alt="{selectedWorkspace.name} avatar" class="w-full h-full object-cover" />
          {:else if selectedWorkspace?.icon}
            <svelte:component this={iconMap[selectedWorkspace.icon] || Package} class="w-3 h-3 text-white" />
          {:else}
            <Package class="w-3 h-3 text-white" />
          {/if}
        </button>

        {#if showWorkspaceDropdown}
          <div
            class="absolute z-50 mt-1 w-48 rounded-md shadow-lg border py-1"
            style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
          >
            {#each workspaces as ws}
              <button
                type="button"
                onclick={() => selectWorkspace(ws.id)}
                class="w-full px-3 py-2 text-left text-sm flex items-center gap-2 transition-colors"
                style="color: var(--ds-text);"
                onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-selected)'}
                onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
              >
                {#if ws.avatar_url}
                  <img src={ws.avatar_url} alt="" class="w-5 h-5 rounded object-cover" />
                {:else}
                  <div
                    class="w-5 h-5 rounded flex items-center justify-center"
                    style="background-color: {ws.color || 'var(--ds-interactive)'};"
                  >
                    <svelte:component this={iconMap[ws.icon] || Package} class="w-3 h-3 text-white" />
                  </div>
                {/if}
                <span class="truncate">{ws.name}</span>
              </button>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Item Type Selector -->
      {#if state.availableTypes?.length > 0}
        <div class="relative">
          <button
            type="button"
            onclick={() => {
              showItemTypeDropdown = !showItemTypeDropdown;
              showWorkspaceDropdown = false;
            }}
            class="h-7 px-2 rounded-md flex items-center gap-1.5 border text-sm transition-all hover:scale-105"
            style="border-color: var(--ds-border); color: var(--ds-text);"
            title={selectedItemType?.name || 'Select type'}
          >
            {#if selectedItemType}
              <div
                class="w-4 h-4 rounded flex items-center justify-center"
                style="background-color: {selectedItemType.color};"
              >
                <svelte:component this={iconMap[selectedItemType.icon] || Package} class="w-2.5 h-2.5 text-white" />
              </div>
              <span class="text-xs">{selectedItemType.name}</span>
            {:else}
              <span class="text-xs" style="color: var(--ds-text-subtle);">{t('collections.selectType')}</span>
            {/if}
            <ChevronDown class="w-3 h-3" style="color: var(--ds-text-subtle);" />
          </button>

          {#if showItemTypeDropdown}
            <div
              class="absolute z-50 mt-1 w-48 rounded-md shadow-lg border py-1"
              style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
            >
              {#each state.availableTypes as itemType}
                <button
                  type="button"
                  onclick={() => selectItemType(itemType.id)}
                  class="w-full px-3 py-2 text-left text-sm flex items-center gap-2 transition-colors"
                  style="color: var(--ds-text);"
                  onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-selected)'}
                  onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
                >
                  <div
                    class="w-5 h-5 rounded flex items-center justify-center"
                    style="background-color: {itemType.color};"
                  >
                    <svelte:component this={iconMap[itemType.icon] || Package} class="w-3 h-3 text-white" />
                  </div>
                  <span class="truncate">{itemType.name}</span>
                </button>
              {/each}
            </div>
          {/if}
        </div>
      {/if}

      <!-- Create Button -->
      <button
        type="button"
        onclick={() => onCreate(parentId)}
        class="h-7 px-3 rounded-md text-sm font-medium text-white transition-colors flex items-center gap-1"
        style="background-color: var(--ds-interactive);"
        onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-interactive-hovered)'}
        onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-interactive)'}
      >
        <Plus class="w-3.5 h-3.5" />
        {t('common.create')}
      </button>

      <!-- Cancel Button -->
      <button
        type="button"
        onclick={() => onCancel(parentId)}
        class="h-7 px-2 rounded-md text-sm transition-colors"
        style="color: var(--ds-text-subtle);"
        onmouseenter={(e) => e.currentTarget.style.color = 'var(--ds-text)'}
        onmouseleave={(e) => e.currentTarget.style.color = 'var(--ds-text-subtle)'}
      >
        {t('common.cancel')}
      </button>
    </div>
  </div>

  <!-- Error message -->
  {#if state.error}
    <div class="px-3 pb-3 text-xs" style="color: var(--ds-text-danger);">
      {state.error}
    </div>
  {/if}
</div>
