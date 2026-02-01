<script>
  import { onMount } from 'svelte';
  import { api } from '../api.js';
  import { Workflow, Pencil, Trash2 } from 'lucide-svelte';
  import Tooltip from './Tooltip.svelte';
  import Spinner from './Spinner.svelte';
  import { t } from '../stores/i18n.svelte.js';

  export let itemId;
  export let onEdit = () => {};
  export let onDelete = () => {};

  let diagrams = [];
  let loading = true;
  let error = null;

  async function loadDiagrams() {
    try {
      loading = true;
      error = null;
      diagrams = await api.getDiagrams(itemId);
    } catch (err) {
      console.error('Failed to load diagrams:', err);
      error = t('components.diagram.loadError');
    } finally {
      loading = false;
    }
  }

  async function handleDelete(diagramId) {
    if (!confirm(t('components.diagram.confirmDelete'))) {
      return;
    }

    try {
      await api.deleteDiagram(diagramId);
      await loadDiagrams();
      onDelete(diagramId);
    } catch (err) {
      console.error('Failed to delete diagram:', err);
      alert(t('components.diagram.deleteError'));
    }
  }

  function formatDate(dateStr) {
    const date = new Date(dateStr);
    return date.toLocaleDateString() + ' ' + date.toLocaleTimeString();
  }

  onMount(() => {
    loadDiagrams();
  });

  export function refresh() {
    loadDiagrams();
  }
</script>

{#if loading}
  <div class="text-center py-8">
    <Spinner class="mx-auto" />
    <p class="text-sm mt-2" style="color: var(--ds-text-subtle);">{t('components.diagram.loading')}</p>
  </div>
{:else if diagrams && diagrams.length > 0}
  <div class="attachment-list space-y-1">
    {#each diagrams as diagram (diagram.id)}
      <div class="attachment-item rounded border p-2 transition-colors hover:bg-gray-50"
           style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2 flex-1 min-w-0">
            <!-- Diagram icon -->
            <div class="flex-shrink-0">
              <div class="flex items-center justify-center w-8 h-8">
                <Workflow class="w-5 h-5" style="color: var(--ds-text-subtle);" />
              </div>
            </div>

            <!-- Diagram info -->
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <p class="text-sm truncate" style="color: var(--ds-text);">
                  {diagram.name}
                </p>
              </div>
              <div class="flex items-center gap-2 text-xs" style="color: var(--ds-text-subtle);">
                {#if diagram.updated_by_name}
                  <span>{formatDate(diagram.updated_at)}</span>
                  <span>•</span>
                  <span>{diagram.updated_by_name}</span>
                {:else}
                  <span>{formatDate(diagram.created_at)}</span>
                  {#if diagram.creator_name}
                    <span>•</span>
                    <span>{diagram.creator_name}</span>
                  {/if}
                {/if}
              </div>
            </div>

            <!-- Actions -->
            <div class="flex items-center gap-1 flex-shrink-0">
              <Tooltip content={t('components.diagram.edit')}>
                {#snippet children()}
                  <button
                    onclick={() => onEdit(diagram)}
                    class="p-1 rounded hover:bg-blue-50 diagram-edit-btn"
                  >
                    <Pencil class="w-4 h-4" />
                  </button>
                {/snippet}
              </Tooltip>

              <Tooltip content={t('common.delete')}>
                {#snippet children()}
                  <button
                    onclick={() => handleDelete(diagram.id)}
                    class="p-1 rounded hover:bg-red-50 diagram-delete-btn"
                  >
                    <Trash2 class="w-4 h-4" />
                  </button>
                {/snippet}
              </Tooltip>
            </div>
          </div>
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
  .diagram-edit-btn {
    color: var(--ds-text-subtle);
  }
  .diagram-edit-btn:hover {
    color: #3b82f6;
  }
  .diagram-delete-btn {
    color: var(--ds-text-subtle);
  }
  .diagram-delete-btn:hover {
    color: #ef4444;
  }
</style>
