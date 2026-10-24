<script>
  import { onMount } from 'svelte';
  import { links as linksApi } from '../api/configuration.js';
  import X from '@lucide/svelte/icons/x';
  import Plus from '@lucide/svelte/icons/plus';
  import Search from '@lucide/svelte/icons/search';
  import Link2 from '@lucide/svelte/icons/link-2';
  import { errorToast } from '../stores/toasts.svelte.js';
  import Input from '../components/Input.svelte';
  import ItemTypeIcon from '../components/ItemTypeIcon.svelte';

  let {
    fieldId,
    itemId,
    fieldOptions = '{}',
    readonly = false,
    disabled = false,
    links: providedLinks = null,
    onChanged = null
  } = $props();

  let loadedLinks = $state([]);
  const links = $derived(providedLinks ?? loadedLinks);
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
    if (providedLinks === null && itemId && fieldId) {
      loadLinks();
    }
  });

  async function loadLinks() {
    if (!itemId || !fieldId) return;
    loading = true;
    try {
      const result = await linksApi.getFieldLinks(itemId, fieldId);
      loadedLinks = result || [];
    } catch (e) {
      console.error('Failed to load field links:', e);
      loadedLinks = [];
    } finally {
      loading = false;
    }
  }

  let searchTimeout;
  let searchSeq = 0;
  async function handleSearch(e) {
    searchQuery = e.target.value;
    clearTimeout(searchTimeout);
    if (!searchQuery.trim()) {
      searchSeq++; // Invalidate any in-flight request so it cannot land late.
      searchResults = [];
      searching = false;
      return;
    }
    searchTimeout = setTimeout(async () => {
      const seq = ++searchSeq;
      searching = true;
      try {
        const entityType = opts.allowed_entity_types?.[0] || 'item';
        const itemTypeIds = opts.allowed_item_type_ids || [];
        const results = await linksApi.search(searchQuery, entityType, 20, itemTypeIds) || [];
        // A newer keystroke superseded this request; drop the stale response.
        if (seq !== searchSeq) return;
        // Filter out already linked items
        const linkedIds = new Set(links.map(l => isMirror ? l.source_id : l.target_id));
        linkedIds.add(itemId); // Exclude self
        searchResults = results.filter(r => !linkedIds.has(r.id));
      } catch (e) {
        if (seq !== searchSeq) return;
        console.error('Search failed:', e);
        searchResults = [];
      } finally {
        if (seq === searchSeq) searching = false;
      }
    }, 300);
  }

  async function addLink(result) {
    try {
      const sourceType = 'item';
      const targetType = result.type || 'item';
      // Always describe the edit from the field owner's perspective. The
      // server resolves mirror fields to their primary field and performs the
      // single source/target swap there.
      const linkRequest = {
        link_type_id: opts.link_type_id,
        source_type: sourceType,
        source_id: itemId,
        target_type: targetType,
        target_id: result.id,
        custom_field_id: fieldId
      };

      await linksApi.create(linkRequest);

      searchQuery = '';
      searchResults = [];
      showSearch = false;
      if (onChanged) {
        await onChanged({ itemIds: affectedItemIds(linkRequest) });
      }
      else await loadLinks();
    } catch (e) {
      console.error('Failed to add link:', e);
      errorToast(e?.message || 'Failed to add link');
    }
  }

  async function removeLink(linkId) {
    try {
      const removedLink = links.find(link => Number(link.id) === Number(linkId));
      await linksApi.delete(linkId);
      if (onChanged) await onChanged({ itemIds: affectedItemIds(removedLink) });
      else await loadLinks();
    } catch (e) {
      console.error('Failed to remove link:', e);
      errorToast(e?.message || 'Failed to remove link');
    }
  }

  function affectedItemIds(link) {
    const ids = new Set([Number(itemId)]);
    if (link?.source_type === 'item') ids.add(Number(link.source_id));
    if (link?.target_type === 'item') ids.add(Number(link.target_id));
    return [...ids].filter(id => Number.isInteger(id) && id > 0);
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
        workspaceId: link.source_workspace_id
      };
    }
    return {
      id: link.target_id,
      title: link.target_title,
      type: link.target_type,
      typeIcon: link.target_item_type_icon,
      typeColor: link.target_item_type_color,
      typeName: link.target_item_type_name,
      workspaceId: link.target_workspace_id
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
          {#if displayItem.type === 'item'}
            <ItemTypeIcon
              icon={displayItem.typeIcon}
              color={displayItem.typeColor}
              title={displayItem.typeName}
            />
          {/if}
          {#if displayItem.type === 'item' && displayItem.workspaceId}
            <a
              href={`/workspaces/${displayItem.workspaceId}/items/${displayItem.id}`}
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
              class="opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100 transition-opacity cursor-pointer"
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
          class="linking-add inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs cursor-pointer transition-colors"
          style="color: var(--ds-text-subtle); border: 1px dashed var(--ds-border);"
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
          <Input
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
                class="linking-result w-full flex items-center gap-2 px-3 py-2 text-sm text-left cursor-pointer transition-colors"
                style="color: var(--ds-text);"
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

<style>
  .linking-add:hover,
  .linking-result:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
</style>
