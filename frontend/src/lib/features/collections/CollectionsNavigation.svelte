<script>
  import { onMount } from 'svelte';
  import { Tag, FolderOpen } from '@lucide/svelte';
  import { navigate, currentRoute } from '../../router.js';
  import { t } from '../../stores/i18n.svelte.js';
  import NavigationSidebar from '../../layout/NavigationSidebar.svelte';
  import { collectionCategoriesStore } from '../../stores/collectionCategories.js';
  import Button from '../../components/Button.svelte';
  import { getHexFromColorName } from '../../utils/colors.js';

  // Determine active view based on URL
  let activeCategoryId = $derived($currentRoute.params?.categoryId || null);
  let isWorkspaceView = $derived($currentRoute.path?.includes('/workspace'));
  let isAllGlobalActive = $derived(!isWorkspaceView && activeCategoryId === null);

  onMount(async () => {
    await collectionCategoriesStore.init();
  });

  function categoryHref(categoryId) {
    return categoryId === null ? '/collections' : `/collections/category/${categoryId}`;
  }

  function handleManageCategories() {
    const event = new CustomEvent('manage-collection-categories');
    document.dispatchEvent(event);
  }
</script>

{#snippet sidebarFooter()}
  <div class="px-6 pb-6 pt-4 border-t" style="border-color: var(--ds-border);">
    <Button
      variant="default"
      icon={Tag}
      onclick={handleManageCategories}
      class="w-full justify-center"
      dataTestid="collection-categories-manage"
    >
      {t('collections.manageCategories')}
    </Button>
  </div>
{/snippet}

<NavigationSidebar title={t('collections.title')} description={t('collections.subtitle')} footer={sidebarFooter}>
  <nav class="flex-1 space-y-1">
    <!-- All Global Collections -->
    <a
      href={categoryHref(null)}
      class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3 no-underline"
      style={isAllGlobalActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
      onmouseenter={(e) => { if (!isAllGlobalActive) e.currentTarget.style.cssText = 'background: var(--ds-background-neutral-hovered); color: var(--ds-text);'; }}
      onmouseleave={(e) => { if (!isAllGlobalActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
    >
      <div class="w-4 h-4 rounded bg-gradient-to-br from-purple-400 to-purple-600 flex-shrink-0"></div>
      <span>{t('collections.allGlobal')}</span>
    </a>

    <!-- Category List -->
    {#each $collectionCategoriesStore as category (category.id)}
      {@const isCatActive = activeCategoryId === category.id.toString()}
      <a
        href={categoryHref(category.id)}
        class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3 no-underline"
        style={isCatActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
        onmouseenter={(e) => { if (!isCatActive) e.currentTarget.style.cssText = 'background: var(--ds-background-neutral-hovered); color: var(--ds-text);'; }}
        onmouseleave={(e) => { if (!isCatActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
        title={category.description || category.name}
      >
        <div
          class="w-4 h-4 rounded flex-shrink-0"
          style="background-color: {category.color?.startsWith('#') ? category.color : getHexFromColorName(category.color || 'indigo')};"
        ></div>
        <span class="truncate">{category.name}</span>
      </a>
    {/each}

    <!-- Divider -->
    <div class="my-3 border-t" style="border-color: var(--ds-border);"></div>

    <!-- Workspace Collections -->
    <a
      href="/collections/workspace"
      class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3 no-underline"
      style={isWorkspaceView ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
      onmouseenter={(e) => { if (!isWorkspaceView) e.currentTarget.style.cssText = 'background: var(--ds-background-neutral-hovered); color: var(--ds-text);'; }}
      onmouseleave={(e) => { if (!isWorkspaceView) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
    >
      <FolderOpen class="w-4 h-4 flex-shrink-0" />
      <span>{t('collections.workspaceCollections')}</span>
    </a>
  </nav>

</NavigationSidebar>
