<script>
  import { useEventListener } from 'runed';
  import { Sparkles, CheckSquare, Compass, GripVertical, Bell, Clock, Eye, Target, Briefcase, Grip, ListChecks, Search } from '@lucide/svelte';
  import {
    DASHBOARD_GRID_COLUMNS,
    dashboardWidgetCategories,
    getDashboardWidgetsByCategory,
  } from '../services/dashboardWidgetRegistry.js';
  import DescriptionText from '../components/DescriptionText.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { isOpen = $bindable(false), activeCategory = $bindable('activity') } = $props();

  const iconMap = {
    Sparkles,
    CheckSquare,
    Compass,
    Bell,
    Clock,
    Eye,
    Target,
    Briefcase,
    Grip,
    ListChecks,
    Search,
  };

  let categories = $derived([
    {
      id: dashboardWidgetCategories.ACTIVITY,
      name: t('dashboard.customization.activity'),
      icon: Clock,
      description: t('dashboard.customization.activityDescription'),
    },
    {
      id: dashboardWidgetCategories.WORK,
      name: t('dashboard.customization.work'),
      icon: CheckSquare,
      description: t('dashboard.customization.workDescription'),
    },
    {
      id: dashboardWidgetCategories.NAVIGATION,
      name: t('dashboard.customization.navigation'),
      icon: Compass,
      description: t('dashboard.customization.navigationDescription'),
    },
  ]);

  let currentWidgets = $derived(getDashboardWidgetsByCategory(activeCategory));

  function handleKeydown(event) {
    if (event.key === 'Escape' && isOpen) {
      isOpen = false;
    }
  }

  useEventListener(() => document, 'keydown', handleKeydown);

  function getIconComponent(iconName) {
    return iconMap[iconName] || Sparkles;
  }
</script>

<div
  class="fixed top-0 left-0 h-full flex shadow-2xl z-50 transform transition-transform duration-300 ease-in-out"
  style="background-color: var(--ds-surface-card, #ffffff);"
  class:translate-x-0={isOpen}
  class:-translate-x-full={!isOpen}
>
  <!-- Left navigation -->
  <div
    class="w-16 border-r flex flex-col items-center py-4 gap-2"
    style="border-color: var(--ds-border); background-color: var(--ds-surface);"
  >
    {#each categories as category}
      {@const isActive = activeCategory === category.id}
      {@const CategoryIcon = category.icon}
      <button
        class="w-12 h-12 rounded-lg flex items-center justify-center transition-all"
        style={isActive
          ? 'background: var(--ds-surface-raised); color: var(--ds-text); box-shadow: var(--shadow-sm);'
          : 'color: var(--ds-text-subtle);'}
        onmouseenter={(e) => {
          if (!isActive)
            e.currentTarget.style.cssText =
              'background: var(--ds-background-neutral-hovered); color: var(--ds-text);';
        }}
        onmouseleave={(e) => {
          if (!isActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);';
        }}
        onclick={() => (activeCategory = category.id)}
        title={category.name}
      >
        <CategoryIcon class="w-5 h-5" />
      </button>
    {/each}
  </div>

  <!-- Right content panel -->
  <div class="w-96 flex flex-col" style="background-color: var(--ds-surface-raised);">
    <ModalHeader
      title={categories.find((c) => c.id === activeCategory)?.name || t('dashboard.customization.widgets')}
      subtitle={categories.find((c) => c.id === activeCategory)?.description || ''}
      onClose={() => (isOpen = false)}
    />

    <div class="flex-1 overflow-y-auto p-6">
      <div class="space-y-3">
        {#each currentWidgets as widget (widget.type)}
          {@const IconComponent = getIconComponent(widget.icon)}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="widget-card p-3 rounded border transition-colors cursor-grab active:cursor-grabbing"
            style="border-color: var(--ds-border); background-color: var(--ds-surface);"
            onmouseenter={(e) =>
              (e.currentTarget.style.cssText =
                'border-color: var(--ds-border-focused); background-color: var(--ds-background-neutral-hovered);')}
            onmouseleave={(e) =>
              (e.currentTarget.style.cssText =
                'border-color: var(--ds-border); background-color: var(--ds-surface);')}
            data-dashboard-widget-card
            data-widget-type={widget.type}
          >
            <div class="flex items-start gap-3">
              <div
                class="w-10 h-10 rounded flex items-center justify-center flex-shrink-0"
                style="background: linear-gradient(to bottom right, var(--color-blue-500), var(--color-blue-600));"
              >
                <IconComponent class="w-5 h-5 text-white" />
              </div>
              <div class="flex-1 min-w-0">
                <h3 class="text-sm font-medium" style="color: var(--ds-text);">{t(widget.nameKey)}</h3>
                <DescriptionText>{t(widget.descriptionKey)}</DescriptionText>
                <div class="flex items-center gap-2 mt-2">
                  <span
                    class="text-xs px-2 py-0.5 rounded"
                    style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
                  >
                    {categories.find((category) => category.id === widget.category)?.name || widget.category}
                  </span>
                  <span class="text-xs" style="color: var(--ds-text-subtlest);">
                    {t('widgets.defaultWidth', {
                      width: widget.defaultWidth,
                      columns: DASHBOARD_GRID_COLUMNS,
                    })}
                  </span>
                </div>
              </div>
              <div
                class="cursor-grab active:cursor-grabbing flex-shrink-0"
                style="color: var(--ds-text-subtlest);"
              >
                <GripVertical class="w-5 h-5" />
              </div>
            </div>
          </div>
        {/each}
      </div>

      <div
        class="mt-6 p-4 rounded"
        style="background-color: var(--ds-background-neutral); border: 1px solid var(--ds-border);"
      >
        <p class="text-xs" style="color: var(--ds-text);">
          <strong>{t('dashboard.customization.tipTitle')}:</strong> {t('dashboard.customization.tip')}
        </p>
      </div>
    </div>
  </div>
</div>

<style>
  .widget-card {
    user-select: none;
  }
</style>
