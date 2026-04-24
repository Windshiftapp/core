<script>
  import { useEventListener } from 'runed';
  import { Sparkles, CheckSquare, Compass, GripVertical, Bell, Clock, Eye, Target, Briefcase, Grip, ListChecks } from 'lucide-svelte';
  import {
    dashboardWidgetCategories,
    getDashboardWidgetsByCategory,
  } from '../services/dashboardWidgetRegistry.js';
  import DescriptionText from '../components/DescriptionText.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';

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
  };

  const categories = [
    {
      id: dashboardWidgetCategories.ACTIVITY,
      name: 'Activity',
      icon: Clock,
      description: 'Briefings, activity streams, and notifications',
    },
    {
      id: dashboardWidgetCategories.WORK,
      name: 'Work',
      icon: CheckSquare,
      description: 'Items, milestones, and things assigned to you',
    },
    {
      id: dashboardWidgetCategories.NAVIGATION,
      name: 'Navigation',
      icon: Compass,
      description: 'Quick access to workspaces',
    },
  ];

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
        <svelte:component this={category.icon} class="w-5 h-5" />
      </button>
    {/each}
  </div>

  <!-- Right content panel -->
  <div class="w-96 flex flex-col" style="background-color: var(--ds-surface-raised);">
    <ModalHeader
      title={categories.find((c) => c.id === activeCategory)?.name || 'Widgets'}
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
                <svelte:component this={IconComponent} class="w-5 h-5 text-white" />
              </div>
              <div class="flex-1 min-w-0">
                <h3 class="text-sm font-medium" style="color: var(--ds-text);">{widget.name}</h3>
                <DescriptionText>{widget.description}</DescriptionText>
                <div class="flex items-center gap-2 mt-2">
                  <span
                    class="text-xs px-2 py-0.5 rounded"
                    style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
                  >
                    {widget.category}
                  </span>
                  <span class="text-xs" style="color: var(--ds-text-subtlest);">
                    Default: {widget.defaultWidth}/3 width
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
          <strong>Tip:</strong> Drag widgets from here into any section on your dashboard.
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
