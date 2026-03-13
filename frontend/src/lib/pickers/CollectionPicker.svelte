<script>
  import { BasePicker } from '.';
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { FolderOpen, Globe } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = $bindable([]),
    placeholder = '',
    label = '',
    disabled = false,
    class: className = '',
    onChange = () => {},
    onCancel = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectCollections'));

  let collections = $state([]);
  let workspaces = $state([]);
  let loading = $state(false);
  let error = $state(null);

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    if (loading) return;

    try {
      loading = true;
      error = null;

      // Load collections and workspaces in parallel
      const [collectionsData, workspacesData] = await Promise.all([
        api.collections.getAll(),
        api.workspaces.getAll()
      ]);

      collections = collectionsData || [];
      workspaces = workspacesData || [];
    } catch (err) {
      console.error('Failed to load collections:', err);
      error = err.message || 'Failed to load collections';
      collections = [];
    } finally {
      loading = false;
    }
  }

  function getWorkspaceName(workspaceId) {
    if (!workspaceId) return t('common.global');
    const workspace = workspaces.find(w => w.id === workspaceId);
    return workspace ? workspace.key : `${t('workspaces.workspace')} ${workspaceId}`;
  }
</script>

<BasePicker
  bind:value
  items={collections}
  {loading}
  {error}
  placeholder={resolvedPlaceholder}
  {label}
  {disabled}
  class={className}
  multiple={true}
  searchFields={['name', 'description']}
  getValue={(collection) => collection?.id}
  getLabel={(collection) => collection?.name ?? ''}
  onChange={(value) => onChange(value)}
  onCancel={() => onCancel()}
>
  {#snippet chipSnippet({ item: collection })}
    <!-- Collection Icon -->
    <div class="w-3.5 h-3.5 rounded flex items-center justify-center flex-shrink-0"
         style="color: var(--ds-text-subtle);">
      {#if collection.workspace_id}
        <FolderOpen class="w-3 h-3" />
      {:else}
        <Globe class="w-3 h-3" />
      {/if}
    </div>
    <span class="font-medium truncate max-w-[150px]">{collection.name}</span>
  {/snippet}

  {#snippet itemSnippet({ item: collection, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <!-- Collection Icon -->
      <div class="flex-shrink-0">
        <div class="w-6 h-6 rounded-md flex items-center justify-center"
             style="background-color: var(--ds-surface); color: var(--ds-text-subtle);">
          {#if collection.workspace_id}
            <FolderOpen class="w-3.5 h-3.5" />
          {:else}
            <Globe class="w-3.5 h-3.5" />
          {/if}
        </div>
      </div>

      <!-- Collection Info -->
      <div class="flex flex-col min-w-0">
        <div class="flex items-center gap-2">
          <span class="font-medium truncate">{collection.name}</span>
          <span class="text-xs px-1.5 py-0.5 rounded"
                style="background-color: var(--ds-surface); color: var(--ds-text-subtle);">
            {getWorkspaceName(collection.workspace_id)}
          </span>
        </div>
        {#if collection.description}
          <span class="text-sm truncate" style="color: var(--ds-text-subtle);">{collection.description}</span>
        {/if}
      </div>
    </div>
  {/snippet}
</BasePicker>
