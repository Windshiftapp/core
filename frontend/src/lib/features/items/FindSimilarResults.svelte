<script>
  import { ExternalLink } from 'lucide-svelte';
  import Lozenge from '../../components/Lozenge.svelte';

  let { similarItems = [], summary = '', onnavigate = null } = $props();

  function getSimilarityColor(similarity) {
    switch (similarity) {
      case 'duplicate': return 'red';
      case 'closely_related': return 'orange';
      case 'somewhat_related': return 'blue';
      default: return 'gray';
    }
  }

  function getSimilarityLabel(similarity) {
    switch (similarity) {
      case 'duplicate': return 'Duplicate';
      case 'closely_related': return 'Closely Related';
      case 'somewhat_related': return 'Somewhat Related';
      default: return similarity;
    }
  }

  function handleNavigate(item) {
    onnavigate?.({ path: `/workspaces/${item.workspace_id}/items/${item.item_id}` });
  }
</script>

<div class="space-y-3">
  {#if summary}
    <p class="text-sm" style="color: var(--ds-text-subtle);">{summary}</p>
  {/if}

  {#if similarItems.length === 0}
    <p class="text-sm py-4 text-center" style="color: var(--ds-text-subtle);">No similar items found.</p>
  {:else}
    <div class="space-y-2">
      {#each similarItems as item}
        <div
          class="p-3 rounded-lg border transition-colors"
          style="border-color: var(--ds-border); background-color: var(--ds-surface);"
        >
          <div class="flex items-start justify-between gap-2">
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 mb-1">
                <span class="text-xs font-mono flex-shrink-0" style="color: var(--ds-text-subtle);">{item.item_key}</span>
                <Lozenge color={getSimilarityColor(item.similarity)} size="sm">
                  {getSimilarityLabel(item.similarity)}
                </Lozenge>
                {#if item.status_name}
                  <Lozenge color="gray" size="sm">{item.status_name}</Lozenge>
                {/if}
              </div>
              <p class="text-sm font-medium truncate" style="color: var(--ds-text);">{item.title}</p>
              {#if item.reason}
                <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{item.reason}</p>
              {/if}
            </div>
            <button
              class="flex-shrink-0 p-1.5 rounded transition-colors"
              style="color: var(--ds-text-subtle);"
              onmouseenter={(e) => { e.currentTarget.style.color = 'var(--ds-text)'; e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'; }}
              onmouseleave={(e) => { e.currentTarget.style.color = 'var(--ds-text-subtle)'; e.currentTarget.style.backgroundColor = 'transparent'; }}
              onclick={() => handleNavigate(item)}
              title="Open item"
            >
              <ExternalLink class="w-4 h-4" />
            </button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
