<script>
  import { backgroundCategories, getPresetsByCategory } from '../utils/backgroundImages.js';

  let {
    currentImageUrl = null,
    selectedCategory = 'abstract',
    onSelectImage = () => {},
    presetAspectRatio = '4 / 3',
    columns = 4,
    testIdPrefix = 'background-preset',
  } = $props();

  // svelte-ignore state_referenced_locally
  let localSelectedCategory = $state(selectedCategory);

  // Sync local category with prop
  $effect(() => {
    localSelectedCategory = selectedCategory;
  });

  function selectCategory(categoryId) {
    localSelectedCategory = categoryId;
  }
</script>

<!-- Category tabs -->
<div class="flex flex-wrap gap-2 mb-5">
  {#each backgroundCategories as category}
    <button
      type="button"
      class="category-tab px-3 py-1.5 text-sm font-medium rounded-md transition-colors"
      class:category-tab-selected={localSelectedCategory === category.id}
      onclick={() => selectCategory(category.id)}
      data-testid={`${testIdPrefix}-category-${category.id}`}
    >
      {category.name}
    </button>
  {/each}
</div>

<!-- Preset images grid -->
<div
  class="grid gap-3 mb-6"
  style={`grid-template-columns: repeat(${columns}, minmax(0, 1fr));`}
>
  {#each getPresetsByCategory(localSelectedCategory) as preset}
    <button
      type="button"
      onclick={() => onSelectImage(preset.url)}
      class="group relative rounded-lg overflow-hidden transition-all hover:scale-105"
      style={`aspect-ratio: ${presetAspectRatio};${currentImageUrl === preset.url ? ' box-shadow: 0 0 0 2px var(--ds-border-focused);' : ''}`}
      title={preset.name}
      aria-pressed={currentImageUrl === preset.url}
      data-testid={`${testIdPrefix}-${preset.id}`}
    >
      <img
        src={preset.thumbnail}
        alt={preset.name}
        class="w-full h-full object-cover"
        loading="lazy"
      />
      <div class="absolute inset-0 bg-black/0 group-hover:bg-black/20 transition-colors"></div>
      <div class="absolute bottom-0 left-0 right-0 p-2 bg-gradient-to-t from-black/60 to-transparent">
        <span class="text-xs text-white font-medium">{preset.name}</span>
      </div>
    </button>
  {/each}
</div>

<style>
  .category-tab {
    background-color: var(--ds-surface);
    color: var(--ds-text-subtle);
    border: 1px solid var(--ds-border);
  }

  .category-tab:hover {
    background-color: var(--ds-background-neutral-hovered);
    color: var(--ds-text);
  }

  .category-tab-selected {
    background-color: var(--ds-interactive) !important;
    color: white !important;
    border-color: var(--ds-interactive) !important;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
  }
</style>
