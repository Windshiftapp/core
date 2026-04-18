<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { X, Plus, Search, Link2 } from 'lucide-svelte';
  import { navigate } from '../router.js';

  let {
    fieldId,
    itemId,
    fieldOptions = '{}',
    readonly = false,
    disabled = false
  } = $props();

  let links = $state([]);
  let loading = $state(false);
  let showSearch = $state(false);
  let searchQuery = $state('');
  let searchResults = $state([]);
  let searching = $state(false);

  // Parse field options
  const opts = $derived((() => {
    try { return JSON.parse(fieldOptions || '{}'); } catch { return {}; }
  })());

  const isMirror = $derived(!!opts.mirror_of_field_id);

  onMount(() => {
    if (itemId && fieldId) {
      loadLinks();
    }
  });

  async function loadLinks() {
    if (!itemId || !fieldId) return;
    loading = true;
    try {
      const result = await api.links.getFieldLinks(itemId, fieldId);
      links = result || [];
    } catch (e) {
      console.error('Failed to load field links:', e);
      links = [];
    } finally {
      loading = false;
    }
  }

  let searchTimeout;
  async function handleSearch(e) {
    searchQuery = e.target.value;
    clearTimeout(searchTimeout);
    if (!searchQuery.trim()) {
      searchResults = [];
      return;
    }
    searchTimeout = setTimeout(async () => {
      searching = true;
      try {
        const entityType = opts.allowed_entity_types?.[0] || 'item';
        const itemTypeIds = opts.allowed_item_type_ids || [];
        searchResults = await api.links.search(searchQuery, entityType, 20, itemTypeIds) || [];
        // Filter out already linked items
        const linkedIds = new Set(links.map(l => isMirror ? l.source_id : l.target_id));
        linkedIds.add(itemId); // Exclude self
        searchResults = searchResults.filter(r => !linkedIds.has(r.id));
      } catch (e) {
        console.error('Search failed:', e);
        searchResults = [];
      } finally {
        searching = false;
      }
    }, 300);
  }

  async function addLink(result) {
    try {
      const sourceType = 'item';
      const targetType = result.type || 'item';

      await api.links.create({
        link_type_id: opts.link_type_id,
        source_type: isMirror ? targetType : sourceType,
        source_id: isMirror ? result.id : itemId,
        target_type: isMirror ? sourceType : targetType,
        target_id: isMirror ? itemId : result.id,
        custom_field_id: fieldId
      });

      searchQuery = '';
      searchResults = [];
      showSearch = false;
      await loadLinks();
    } catch (e) {
      console.error('Failed to add link:', e);
    }
  }

  async function removeLink(linkId) {
    try {
      await api.links.delete(linkId);
      await loadLinks();
    } catch (e) {
      console.error('Failed to remove link:', e);
    }
  }

  function getLinkDisplayItem(link) {
    if (isMirror) {
      return {
        id: link.source_id,
        title: link.source_title,
        type: link.source_type,
        typeIcon: link.source_item_type_icon,
        typeColor: link.source_item_type_color,
        typeName: link.source_item_type_name,
        workspaceKey: link.source_workspace_key
      };
    }
    return {
      id: link.target_id,
      title: link.target_title,
      type: link.target_type,
      typeIcon: link.target_item_type_icon,
      typeColor: link.target_item_type_color,
      typeName: link.target_item_type_name,
      workspaceKey: link.target_workspace_key
    };
  }
</script>

<div class="linking-field-picker">
  {#if loading}
    <span class="text-xs" style="color: var(--ds-text-subtle);">Loading...</span>
  {:else}
    <!-- Linked items -->
    <div class="flex flex-wrap gap-1.5">
      {#each links as link}
        {@const displayItem = getLinkDisplayItem(link)}
        <div class="inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs group"
          style="background: var(--ds-background-neutral); color: var(--ds-text); border: 1px solid var(--ds-border);">
          {#if displayItem.typeColor}
            <span class="w-2 h-2 rounded-full flex-shrink-0" style="background: {displayItem.typeColor};"></span>
          {/if}
          {#if displayItem.type === 'item' && displayItem.workspaceKey}
            <a
              href={`/workspaces/${displayItem.workspaceKey}/items/${displayItem.id}`}
              class="hover:underline cursor-pointer truncate max-w-[180px] no-underline"
              style="color: inherit;"
              title={displayItem.title}
            >
              {displayItem.title || `#${displayItem.id}`}
            </a>
          {:else}
            <span class="truncate max-w-[180px]" title={displayItem.title}>
              {displayItem.title || `#${displayItem.id}`}
            </span>
          {/if}
          {#if !readonly && !disabled}
            <button
              class="opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer"
              onclick={() => removeLink(link.id)}
              title="Remove link"
            >
              <X class="w-3 h-3" style="color: var(--ds-text-subtle);" />
            </button>
          {/if}
        </div>
      {/each}

      {#if !readonly && !disabled}
        <button
          class="inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs cursor-pointer transition-colors"
          style="color: var(--ds-text-subtle); border: 1px dashed var(--ds-border);"
          onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
          onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
          onclick={() => { showSearch = !showSearch; }}
        >
          <Plus class="w-3 h-3" />
          Add
        </button>
      {/if}
    </div>

    {#if links.length === 0 && (readonly || disabled)}
      <span class="text-xs" style="color: var(--ds-text-subtle);">None</span>
    {/if}

    <!-- Search dropdown -->
    {#if showSearch}
      <div class="mt-2 rounded-lg shadow-lg overflow-hidden" style="background: var(--ds-surface); border: 1px solid var(--ds-border);">
        <div class="flex items-center gap-2 px-3 py-2" style="border-bottom: 1px solid var(--ds-border);">
          <Search class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
          <!-- svelte-ignore a11y_autofocus -->
          <input
            type="text"
            class="w-full bg-transparent text-sm outline-none"
            style="color: var(--ds-text);"
            placeholder="Search items..."
            value={searchQuery}
            oninput={handleSearch}
            autofocus
          />
        </div>

        {#if searchResults.length > 0}
          <div class="max-h-48 overflow-y-auto">
            {#each searchResults as result}
              <button
                class="w-full flex items-center gap-2 px-3 py-2 text-sm text-left cursor-pointer transition-colors"
                style="color: var(--ds-text);"
                onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
                onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
                onclick={() => addLink(result)}
              >
                <Link2 class="w-3.5 h-3.5 flex-shrink-0" style="color: var(--ds-text-subtle);" />
                <span class="truncate">{result.title}</span>
                {#if result.workspace_name}
                  <span class="text-xs flex-shrink-0" style="color: var(--ds-text-subtle);">{result.workspace_name}</span>
                {/if}
              </button>
            {/each}
          </div>
        {:else if searchQuery && !searching}
          <div class="px-3 py-3 text-sm" style="color: var(--ds-text-subtle);">
            No results found
          </div>
        {:else if searching}
          <div class="px-3 py-3 text-sm" style="color: var(--ds-text-subtle);">
            Searching...
          </div>
        {/if}
      </div>
    {/if}
  {/if}
</div>
