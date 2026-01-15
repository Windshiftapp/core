<script>
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  import { X, BarChart3, Package, GripVertical, Palette } from 'lucide-svelte';
  import { widgetRegistry, widgetCategories, getWidgetsByCategory } from '../services/widgetRegistry.js';
  import { gradients } from '../utils/gradients.js';
  import * as LucideIcons from 'lucide-svelte';

  export let isOpen = false;
  export let activeCategory = 'built-in';
  export let selectedGradient = 0;
  export let applyToAllViews = false;

  const dispatch = createEventDispatcher();

  // Get widgets by category
  $: builtInWidgets = getWidgetsByCategory(widgetCategories.BUILT_IN);
  $: additionalWidgets = getWidgetsByCategory(widgetCategories.ADDITIONAL);
  $: currentWidgets = activeCategory === 'built-in' ? builtInWidgets : additionalWidgets;

  // Navigation categories
  const categories = [
    {
      id: 'built-in',
      name: 'Built-in Widgets',
      icon: BarChart3,
      description: 'Core workspace widgets'
    },
    {
      id: 'additional',
      name: 'Additional Widgets',
      icon: Package,
      description: 'Extended functionality widgets'
    },
    {
      id: 'appearance',
      name: 'Appearance',
      icon: Palette,
      description: 'Customize colors and gradients'
    }
  ];

  // Handle gradient selection
  function selectGradient(index) {
    selectedGradient = index;
    dispatch('selectGradient', { gradient: index });
  }

  // Handle apply to all views toggle
  function handleApplyToAllViewsChange() {
    dispatch('applyToAllViewsChange', { applyToAllViews });
  }

  // Handle ESC key to close sidebar
  function handleKeydown(event) {
    if (event.key === 'Escape' && isOpen) {
      isOpen = false;
    }
  }

  onMount(() => {
    document.addEventListener('keydown', handleKeydown);
  });

  onDestroy(() => {
    document.removeEventListener('keydown', handleKeydown);
  });

  // Get Lucide icon component by name
  function getIconComponent(iconName) {
    return LucideIcons[iconName] || BarChart3;
  }
</script>

<!-- Backdrop overlay removed to allow clear view when dragging widgets -->

<!-- Sidebar -->
<div
  class="fixed top-0 left-0 h-full flex shadow-2xl z-50 transform transition-transform duration-300 ease-in-out"
  style="background-color: var(--ds-surface-card, #ffffff);"
  class:translate-x-0={isOpen}
  class:-translate-x-full={!isOpen}
>
  <!-- Left navigation (64px) -->
  <div class="w-16 border-r flex flex-col items-center py-4 gap-2" style="border-color: var(--ds-border); background-color: var(--ds-surface);">
    {#each categories as category}
      {@const isActive = activeCategory === category.id}
      <button
        class="w-12 h-12 rounded-lg flex items-center justify-center transition-all"
        style={isActive ? 'background: var(--ds-surface-raised); color: var(--ds-text); box-shadow: var(--shadow-sm);' : 'color: var(--ds-text-subtle);'}
        onmouseenter={(e) => { if (!isActive) e.currentTarget.style.cssText = 'background: var(--ds-background-neutral-hovered); color: var(--ds-text);'; }}
        onmouseleave={(e) => { if (!isActive) e.currentTarget.style.cssText = 'color: var(--ds-text-subtle);'; }}
        onclick={() => activeCategory = category.id}
        title={category.name}
      >
        <svelte:component this={category.icon} class="w-5 h-5" />
      </button>
    {/each}
  </div>

  <!-- Right content panel (384px) -->
  <div class="w-96 flex flex-col" style="background-color: var(--ds-surface-raised);">
    <!-- Header -->
    <div class="flex items-center justify-between px-6 py-4 border-b" style="border-color: var(--ds-border);">
      <div>
        <h2 class="text-lg font-semibold" style="color: var(--ds-text);">
          {categories.find(c => c.id === activeCategory)?.name || 'Widgets'}
        </h2>
        <p class="text-xs mt-0.5" style="color: var(--ds-text-subtle);">
          {categories.find(c => c.id === activeCategory)?.description || ''}
        </p>
      </div>
      <button
        class="p-1 rounded transition-colors"
        style="color: var(--ds-text-subtlest);"
        onmouseenter={(e) => e.currentTarget.style.cssText = 'color: var(--ds-text); background-color: var(--ds-background-neutral-hovered);'}
        onmouseleave={(e) => e.currentTarget.style.cssText = 'color: var(--ds-text-subtlest);'}
        onclick={() => isOpen = false}
        aria-label="Close sidebar"
      >
        <X class="w-5 h-5" />
      </button>
    </div>

    <!-- Widget cards / Appearance -->
    <div class="flex-1 overflow-y-auto p-6">
      {#if activeCategory === 'appearance'}
        <!-- Gradient Picker -->
        <div class="mb-6">
          <h3 class="text-sm font-medium mb-3" style="color: var(--ds-text);">Gradient Style</h3>
          <p class="text-sm mb-4" style="color: var(--ds-text-subtle);">Choose a color scheme for your workspace homepage</p>
        </div>

        <!-- Gradient Grid (7 columns to fit 19 gradients) -->
        <div class="grid grid-cols-7 gap-3">
          {#each gradients as gradient, index}
            <button
              onclick={() => selectGradient(index)}
              class="group relative w-[25px] h-[25px] rounded overflow-hidden transition-all hover:scale-110"
              class:ring-offset-2={selectedGradient === index}
              style={selectedGradient === index ? 'box-shadow: 0 0 0 2px var(--ds-border-focused); outline-offset: 2px;' : ''}
              title={gradient.name}
            >
              {#if index === 0}
                <!-- "None" option with X icon -->
                <div class="w-full h-full flex items-center justify-center" style="background-color: var(--ds-background-neutral);">
                  <X class="w-3 h-3" style="color: var(--ds-text-subtle);" />
                </div>
              {:else}
                <!-- Gradient Preview -->
                <div
                  class="w-full h-full"
                  style="background: {gradient.value};"
                ></div>
              {/if}

              <!-- Selected Indicator -->
              {#if selectedGradient === index}
                <div class="absolute inset-0 flex items-center justify-center bg-black/20">
                  <div class="w-3 h-3 bg-white rounded-full flex items-center justify-center">
                    <svg class="w-2 h-2" style="color: var(--ds-icon-accent-blue);" fill="currentColor" viewBox="0 0 20 20">
                      <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                    </svg>
                  </div>
                </div>
              {/if}
            </button>
          {/each}
        </div>

        <!-- Apply to All Views Toggle -->
        <div class="mt-6 p-4 rounded" style="background-color: var(--ds-background-neutral); border: 1px solid var(--ds-border);">
          <label class="flex items-start gap-3 cursor-pointer">
            <input
              type="checkbox"
              bind:checked={applyToAllViews}
              onchange={handleApplyToAllViewsChange}
              class="mt-0.5"
            />
            <div>
              <span class="text-sm font-medium block" style="color: var(--ds-text);">Apply gradient to all workspace views</span>
              <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">
                Board, List, Backlog, Map, and Tree
              </p>
            </div>
          </label>
        </div>
      {:else}
        <!-- Widget Cards -->
        <div class="space-y-3">
          {#each currentWidgets as widget}
            {@const IconComponent = getIconComponent(widget.icon)}
            <div
              class="widget-card p-3 rounded border transition-colors cursor-grab active:cursor-grabbing"
              style="border-color: var(--ds-border); background-color: var(--ds-surface);"
              onmouseenter={(e) => e.currentTarget.style.cssText = 'border-color: var(--ds-border-focused); background-color: var(--ds-background-neutral-hovered);'}
              onmouseleave={(e) => e.currentTarget.style.cssText = 'border-color: var(--ds-border); background-color: var(--ds-surface);'}
              data-widget-card
              data-widget-type={widget.type}
            >
              <div class="flex items-start gap-3">
                <!-- Icon preview -->
                <div class="w-10 h-10 rounded flex items-center justify-center flex-shrink-0" style="background: linear-gradient(to bottom right, var(--color-blue-500), var(--color-blue-600));">
                  <svelte:component this={IconComponent} class="w-5 h-5 text-white" />
                </div>

                <!-- Content -->
                <div class="flex-1 min-w-0">
                  <div class="flex items-center gap-2">
                    <h3 class="text-sm font-medium" style="color: var(--ds-text);">{widget.name}</h3>
                  </div>
                  <p class="text-xs mt-1" style="color: var(--ds-text-subtle);">{widget.description}</p>
                  <div class="flex items-center gap-2 mt-2">
                    <span class="text-xs px-2 py-0.5 rounded" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                      {widget.category}
                    </span>
                    <span class="text-xs" style="color: var(--ds-text-subtlest);">
                      Default: {widget.defaultWidth}/3 width
                    </span>
                  </div>
                </div>

                <!-- Drag handle -->
                <div class="cursor-grab active:cursor-grabbing flex-shrink-0" style="color: var(--ds-text-subtlest);">
                  <GripVertical class="w-5 h-5" />
                </div>
              </div>
            </div>
          {/each}
        </div>

        <!-- Help text -->
        <div class="mt-6 p-4 rounded" style="background-color: var(--ds-background-neutral); border: 1px solid var(--ds-border);">
          <p class="text-xs" style="color: var(--ds-text);">
            <strong>Tip:</strong> Drag widgets from here to any section on your workspace homepage to add them.
          </p>
        </div>
      {/if}
    </div>
  </div>
</div>

<style>
  .widget-card {
    user-select: none;
  }
</style>
