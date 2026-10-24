<script>
  import { onMount } from 'svelte';
  import Tag from '@tabler/icons-svelte-runes/icons/tag';
  import { navigate, currentRoute } from '../../router.js';
  import { categoriesStore } from '../../stores/categories.js';
  import { permissionStore, isSystemAdmin } from '../../stores/permissions.svelte.js';
  import Button from '../../components/Button.svelte';
  import { getHexFromColorName } from '../../utils/colors.js';
  import { t } from '../../stores/i18n.svelte.js';
  import NavigationSidebar from '../../layout/NavigationSidebar.svelte';
  
  // Get active category from URL params
  let activeCategoryId = $derived($currentRoute.params?.categoryId || null);
  let isAllActive = $derived(activeCategoryId === null);
  
  onMount(async () => {
    // Load categories when component mounts
    await categoriesStore.init();
  });
  
  function categoryHref(categoryId) {
    return categoryId === null ? '/milestones' : `/milestones/category/${categoryId}`;
  }
  
  function handleManageCategories() {
    // Emit event to parent to show category management modal
    const event = new CustomEvent('manage-categories');
    document.dispatchEvent(event);
  }

  const canManageCategories = $derived(
    $permissionStore.userPermissionKeys?.has('milestone.create') || $isSystemAdmin
  );
</script>

{#snippet sidebarFooter()}
  {#if canManageCategories}
    <div class="px-6 pb-6 pt-4 border-t" style="border-color: var(--ds-border);">
      <Button
        variant="default"
        icon={Tag}
        onclick={handleManageCategories}
        class="w-full justify-center"
      >
        {t('milestones.manageCategories')}
      </Button>
    </div>
  {/if}
{/snippet}

<!-- Milestone Navigation Sidebar -->
<NavigationSidebar title={t('milestones.title')} description={t('milestones.subtitle')} footer={sidebarFooter}>
  <!-- Navigation -->
  <nav class="flex-1 space-y-1">
    <!-- All Categories -->
    <a
      href={categoryHref(null)}
      class="nav-link w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3 no-underline"
      class:active={isAllActive}
    >
      <div class="w-4 h-4 rounded bg-gradient-to-br from-ds-nav-purple-from to-ds-nav-purple-to flex-shrink-0"></div>
      <span>{t('milestones.allCategories')}</span>
    </a>

    <!-- Category List -->
    {#each $categoriesStore as category (category.id)}
      {@const isCatActive = activeCategoryId === category.id.toString()}
      <a
        href={categoryHref(category.id)}
        class="nav-link w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3 no-underline"
        class:active={isCatActive}
        title={category.description || category.name}
      >
        <div
          class="w-4 h-4 rounded flex-shrink-0"
          style="background-color: {category.color?.startsWith('#') ? category.color : getHexFromColorName(category.color || 'indigo')};"
        ></div>
        <span class="truncate">{category.name}</span>
      </a>
    {/each}
  </nav>
  
</NavigationSidebar>


<style>
  .nav-link {
    color: var(--ds-text-subtle);
  }

  .nav-link:hover:not(.active) {
    background: var(--ds-background-neutral-hovered);
    color: var(--ds-text);
  }

  .nav-link.active {
    background: var(--ds-surface-selected);
    color: var(--ds-text);
  }
</style>
