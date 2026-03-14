<script>
  import { onMount } from 'svelte';
  import { Tag } from 'lucide-svelte';
  import { navigate, currentRoute } from '../../router.js';
  import { categoriesStore } from '../../stores/categories.js';
  import Button from '../../components/Button.svelte';
  import { getHexFromColorName } from '../../utils/colors.js';
  import { t } from '../../stores/i18n.svelte.js';
  import SidebarHeader from '../../layout/SidebarHeader.svelte';
  
  // Get active category from URL params
  let activeCategoryId = $derived($currentRoute.params?.categoryId || null);
  let isAllActive = $derived(activeCategoryId === null);
  
  onMount(async () => {
    // Load categories when component mounts
    await categoriesStore.init();
  });
  
  function handleCategoryClick(categoryId) {
    if (categoryId === null) {
      navigate('/milestones');
    } else {
      navigate(`/milestones/category/${categoryId}`);
    }
  }
  
  function handleManageCategories() {
    // Emit event to parent to show category management modal
    const event = new CustomEvent('manage-categories');
    document.dispatchEvent(event);
  }
</script>

<!-- Milestone Navigation Sidebar -->
<div class="w-64 border-r flex flex-col p-6" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
  <!-- Header -->
  <SidebarHeader title={t('milestones.title')} description={t('milestones.subtitle')} noBorder />
  
  <!-- Navigation -->
  <nav class="flex-1 space-y-1">
    <!-- All Categories -->
    <button
      onclick={() => handleCategoryClick(null)}
      class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3"
      style={isAllActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
      onmouseenter={(e) => { if (!isAllActive) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
      onmouseleave={(e) => { if (!isAllActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
    >
      <div class="w-4 h-4 rounded bg-gradient-to-br from-purple-400 to-purple-600 flex-shrink-0"></div>
      <span>{t('milestones.allCategories')}</span>
    </button>

    <!-- Category List -->
    {#each $categoriesStore as category (category.id)}
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
  </nav>
  
  <!-- Footer - Manage Categories -->
  <div class="pt-4 border-t" style="border-color: var(--ds-border);">
    <Button
      variant="default"
      icon={Tag}
      onclick={handleManageCategories}
      class="w-full justify-center"
    >
      {t('milestones.manageCategories')}
    </Button>
  </div>
</div>
