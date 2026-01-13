<script>
  import { FolderOpen, CheckCircle, Clock, AlertCircle } from 'lucide-svelte';
  import StatCard from './StatCard.svelte';

  export let stats = {
    totalCollections: 0,
    itemsByStatusCategory: {},
    totalItems: 0
  };
  export let statusCategories = [];

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

<div class="flex items-center justify-between gap-4">
  <StatCard
    icon={FolderOpen}
    bgColor="var(--ds-accent-blue-subtler)"
    iconColor="var(--ds-icon-accent-blue)"
    label="Collections"
    value={stats.totalCollections}
  />

  {#each statusCategories as category}
    {@const color = getCategoryColor(category)}
    <StatCard
      icon={getCategoryIcon(category.name)}
      bgColor="{color}20"
      iconColor={color}
      label={category.name}
      value={stats.itemsByStatusCategory[category.name] || 0}
    />
  {/each}

  {#if stats.totalItems > 0}
    <StatCard
      icon={FolderOpen}
      bgColor="var(--ds-background-accent-purple-subtler)"
      iconColor="var(--ds-icon-accent-purple)"
      label="Total Items"
      value={stats.totalItems}
    />
  {/if}
</div>
