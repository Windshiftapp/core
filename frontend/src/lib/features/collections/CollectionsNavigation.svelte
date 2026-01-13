<script>
  import { onMount } from 'svelte';
  import { Tag, FolderOpen } from 'lucide-svelte';
  import { navigate, currentRoute } from '../../router.js';
  import { collectionCategoriesStore } from '../../stores/collectionCategories.js';
  import Button from '../../components/Button.svelte';
  import { getHexFromColorName } from '../../utils/colors.js';

  // Determine active view based on URL
  $: activeCategoryId = $currentRoute.params?.categoryId || null;
  $: isWorkspaceView = $currentRoute.path?.includes('/workspace');
  $: isAllGlobalActive = !isWorkspaceView && activeCategoryId === null;

  onMount(async () => {
    await collectionCategoriesStore.init();
  });

  function handleCategoryClick(categoryId) {
    if (categoryId === null) {
      navigate('/collections');
    } else {
      navigate(`/collections/category/${categoryId}`);
    }
  }

  function handleWorkspaceClick() {
    navigate('/collections/workspace');
  }

  function handleManageCategories() {
    const event = new CustomEvent('manage-collection-categories');
    document.dispatchEvent(event);
  }
</script>

<div class="w-64 border-r flex flex-col p-6" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
  <div class="mb-6">
    <div class="flex items-center gap-3 mb-2">
      <h2 class="text-xl font-semibold" style="color: var(--ds-text);">Collections</h2>
    </div>
    <p class="text-sm" style="color: var(--ds-text-subtle);">Saved queries and filters</p>
  </div>

  <nav class="flex-1 space-y-1">
    <!-- All Global Collections -->
    <button
      onclick={() => handleCategoryClick(null)}
      class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3"
      style={isAllGlobalActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
      onmouseenter={(e) => { if (!isAllGlobalActive) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
      onmouseleave={(e) => { if (!isAllGlobalActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
    >
      <div class="w-4 h-4 rounded bg-gradient-to-br from-purple-400 to-purple-600 flex-shrink-0"></div>
      <span>All Global</span>
    </button>

    <!-- Category List -->
    {#each $collectionCategoriesStore as category (category.id)}
      {@const isCatActive = activeCategoryId === category.id.toString()}
      <button
        onclick={() => handleCategoryClick(category.id)}
        class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3"
        style={isCatActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
        onmouseenter={(e) => { if (!isCatActive) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
        onmouseleave={(e) => { if (!isCatActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
        title={category.description || category.name}
      >
        <div
          class="w-4 h-4 rounded flex-shrink-0"
          style="background-color: {category.color?.startsWith('#') ? category.color : getHexFromColorName(category.color || 'indigo')};"
        ></div>
        <span class="truncate">{category.name}</span>
      </button>
    {/each}

    <!-- Divider -->
    <div class="my-3 border-t" style="border-color: var(--ds-border);"></div>

    <!-- Workspace Collections -->
    <button
      onclick={handleWorkspaceClick}
      class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3"
      style={isWorkspaceView ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
      onmouseenter={(e) => { if (!isWorkspaceView) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
      onmouseleave={(e) => { if (!isWorkspaceView) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
    >
      <FolderOpen class="w-4 h-4 flex-shrink-0" />
      <span>Workspace Collections</span>
    </button>
  </nav>

  <!-- Footer - Manage Categories -->
  <div class="pt-4 border-t" style="border-color: var(--ds-border);">
    <Button
      variant="default"
      icon={Tag}
      onclick={handleManageCategories}
      class="w-full justify-center"
    >
      Manage Categories
    </Button>
  </div>
</div>
