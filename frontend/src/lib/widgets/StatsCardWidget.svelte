<script>
  import { FolderOpen, CheckCircle, Clock, AlertCircle } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { objectDisplayName } from '../utils/systemLabels.js';

  let { stats = {
    totalCollections: 0,
    itemsByStatusCategory: {},
    totalItems: 0
  }, statusCategories = [] } = $props();

  function getCategoryIcon(categoryName) {
    const name = categoryName.toLowerCase();
    if (name.includes('done') || name.includes('complete')) return CheckCircle;
    if (name.includes('progress') || name.includes('development')) return Clock;
    if (name.includes('todo') || name.includes('backlog')) return AlertCircle;
    return FolderOpen;
  }

  function getCategoryColor(category) {
    if (category.color) return category.color;
    const name = category.name.toLowerCase();
    if (name.includes('done') || name.includes('complete')) return '#10b981';
    if (name.includes('progress') || name.includes('development')) return '#3b82f6';
    if (name.includes('todo') || name.includes('backlog')) return '#6b7280';
    return '#8b5cf6';
  }
</script>

{#snippet metric(Icon, color, label, value)}
  <div class="min-w-0 basis-40 border-l border-ds-border pl-4">
    <dt class="flex items-center gap-2 text-xs text-ds-text-subtle">
      <Icon size={14} class="shrink-0" style={`color: ${color};`} aria-hidden="true" />
      <span class="wrap-anywhere">{label}</span>
    </dt>
    <dd class="mt-2 text-3xl leading-none font-semibold tracking-tight tabular-nums text-ds-text">
      {value}
    </dd>
  </div>
{/snippet}

<dl class="flex flex-wrap justify-between gap-x-6 gap-y-6 py-2">
  {@render metric(FolderOpen, 'var(--ds-icon-accent-blue)', t('widgets.stats.collections'), stats.totalCollections)}

  {#each statusCategories as category}
    {@render metric(
      getCategoryIcon(category.name),
      getCategoryColor(category),
      objectDisplayName(category, 'status_category'),
      stats.itemsByStatusCategory[category.name] || 0
    )}
  {/each}

  {#if stats.totalItems > 0}
    {@render metric(FolderOpen, 'var(--ds-icon-accent-purple)', t('widgets.stats.totalItems'), stats.totalItems)}
  {/if}
</dl>
