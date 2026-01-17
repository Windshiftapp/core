<script>
  import { onMount } from 'svelte';
  import { Tag, Globe, Webhook, Layers, Mail } from 'lucide-svelte';
  import { navigate, currentRoute } from '../../router.js';
  import { channelCategoriesStore } from '../../stores/channelCategories.js';
  import Button from '../../components/Button.svelte';
  import { getHexFromColorName } from '../../utils/colors.js';
  import { t } from '../../stores/i18n.svelte.js';

  // Get active filters from URL params
  let activeCategoryId = $derived($currentRoute.params?.categoryId || null);
  let activeTypeFilter = $derived($currentRoute.params?.type || null);
  let isAllCategoriesActive = $derived(activeCategoryId === null && activeTypeFilter === null);

  // Channel type definitions (use $derived for reactive translations)
  let channelTypes = $derived([
    { id: null, label: t('channels.allTypes'), icon: Layers, color: 'from-gray-400 to-gray-600' },
    { id: 'portal', label: t('channels.portal'), icon: Globe, color: 'from-green-400 to-green-600' },
    { id: 'webhook', label: t('channels.webhook'), icon: Webhook, color: 'from-purple-400 to-purple-600' },
    { id: 'email', label: t('channels.email'), icon: Mail, color: 'from-blue-400 to-blue-600' }
  ]);

  onMount(async () => {
    // Load categories when component mounts
    await channelCategoriesStore.init();
  });

  function handleTypeClick(typeId) {
    if (typeId === null) {
      navigate('/channels');
    } else {
      navigate(`/channels/type/${typeId}`);
    }
  }

  function handleCategoryClick(categoryId) {
    if (categoryId === null) {
      navigate('/channels');
    } else {
      navigate(`/channels/category/${categoryId}`);
    }
  }

  function handleManageCategories() {
    // Emit event to parent to show category management modal
    const event = new CustomEvent('manage-channel-categories');
    document.dispatchEvent(event);
  }
</script>

<!-- Channel Navigation Sidebar -->
<div class="w-64 border-r flex flex-col p-6" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
  <!-- Header -->
  <div class="mb-6">
    <div class="flex items-center gap-3 mb-2">
      <h2 class="text-xl font-semibold" style="color: var(--ds-text);">{t('channels.title')}</h2>
    </div>
    <p class="text-sm" style="color: var(--ds-text-subtle);">{t('channels.subtitle')}</p>
  </div>

  <!-- Navigation -->
  <nav class="flex-1 space-y-4">
    <!-- Channel Types Section -->
    <div class="space-y-1">
      <div class="px-3 mb-2">
        <span class="text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('channels.types')}</span>
      </div>
      {#each channelTypes as type (type.id)}
        {@const isTypeActive = activeTypeFilter === type.id}
        <button
          onclick={() => handleTypeClick(type.id)}
          class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3"
          style={isTypeActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
          onmouseenter={(e) => { if (!isTypeActive) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
          onmouseleave={(e) => { if (!isTypeActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
        >
          <div class="w-4 h-4 rounded bg-gradient-to-br {type.color} flex-shrink-0 flex items-center justify-center">
            <svelte:component this={type.icon} class="w-2.5 h-2.5 text-white" />
          </div>
          <span>{type.label}</span>
        </button>
      {/each}
    </div>

    <!-- Categories Section -->
    <div class="space-y-1">
      <div class="px-3 mb-2">
        <span class="text-xs font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">{t('channels.categories')}</span>
      </div>
      <!-- All Channels -->
      <button
        onclick={() => handleCategoryClick(null)}
        class="w-full text-left cursor-pointer px-3 py-2 rounded-lg text-sm font-medium transition-all flex items-center gap-3"
        style={isAllCategoriesActive ? 'background: var(--ds-surface-selected); color: var(--ds-text);' : 'color: var(--ds-text-subtle);'}
        onmouseenter={(e) => { if (!isAllCategoriesActive) e.currentTarget.style.cssText = 'background: var(--ds-surface-hovered); color: var(--ds-text);'; }}
        onmouseleave={(e) => { if (!isAllCategoriesActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
      >
        <div class="w-4 h-4 rounded bg-gradient-to-br from-blue-400 to-blue-600 flex-shrink-0"></div>
        <span>{t('channels.allChannels')}</span>
      </button>

      <!-- Category List -->
      {#each $channelCategoriesStore as category (category.id)}
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
            style="background-color: {category.color?.startsWith('#') ? category.color : getHexFromColorName(category.color || 'blue')};"
          ></div>
          <span class="truncate">{category.name}</span>
        </button>
      {/each}
    </div>
  </nav>

  <!-- Footer - Manage Categories -->
  <div class="pt-4 border-t" style="border-color: var(--ds-border);">
    <Button
      variant="default"
      icon={Tag}
      onclick={handleManageCategories}
      class="w-full justify-center"
    >
      {t('channels.manageCategories')}
    </Button>
  </div>
</div>
