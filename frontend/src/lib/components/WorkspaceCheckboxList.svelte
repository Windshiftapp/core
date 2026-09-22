<script>
  // Searchable, capped workspace checkbox list shared by admin forms that
  // restrict an entity to a set of workspaces (WI-1446). At large workspace
  // counts the list mounts a bounded window instead of every workspace.
  import { t } from '../stores/i18n.svelte.js';
  import Checkbox from './Checkbox.svelte';

  let {
    workspaces = [],
    selected = [],
    onToggle = () => {},
    idPrefix = null,
    emptyMessage = ''
  } = $props();

  const MAX_VISIBLE = 100;

  let search = $state('');

  let filtered = $derived.by(() => {
    const term = search.trim().toLowerCase();
    if (!term) return workspaces;
    return workspaces.filter(ws =>
      ws.name?.toLowerCase().includes(term) || ws.key?.toLowerCase().includes(term)
    );
  });
  let visible = $derived(filtered.slice(0, MAX_VISIBLE));

  function labelFor(ws) {
    return ws.key ? `${ws.key} - ${ws.name}` : (ws.name ?? '');
  }
</script>

<div class="space-y-2" data-testid="workspace-checkbox-list">
  <input
    bind:value={search}
    type="text"
    data-testid="workspace-checkbox-list-search"
    placeholder={t('nav.searchWorkspaces')}
    class="w-full px-2 py-1.5 rounded text-sm outline-none"
    style="background-color: var(--ds-background-input); border: 1px solid var(--ds-border); color: var(--ds-text);"
  />

  <div class="max-h-40 overflow-y-auto">
    {#if filtered.length === 0}
      <p class="text-xs" style="color: var(--ds-text-subtle);">
        {emptyMessage || t('nav.noWorkspacesMatch')}
      </p>
    {:else}
      {#each visible as ws (ws.id)}
        <Checkbox
          id={idPrefix ? `${idPrefix}-${ws.id}` : undefined}
          checked={selected.includes(ws.id)}
          onchange={(checked) => onToggle(ws.id, checked)}
          label={labelFor(ws)}
          class="py-1"
          size="small"
        />
      {/each}
      {#if visible.length < filtered.length}
        <div data-testid="workspace-checkbox-list-more-hint" class="py-1.5 text-xs text-center" style="color: var(--ds-text-subtle);">
          {t('pickers.showingOfTotal', { shown: visible.length, total: filtered.length })}
        </div>
      {/if}
    {/if}
  </div>
</div>
