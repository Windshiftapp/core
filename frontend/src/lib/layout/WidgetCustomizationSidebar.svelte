<script>
  import { useEventListener } from 'runed';
  import { GripVertical } from '@lucide/svelte';
  import DescriptionText from '../components/DescriptionText.svelte';
  import ModalHeader from '../dialogs/ModalHeader.svelte';

  let {
    isOpen = $bindable(false),
    activeCategory = $bindable(''),
    categories = [],
    widgets = [],
    iconMap = {},
    fallbackIcon,
    cardAttributes = {},
    fallbackTitle = '',
    tipLabel = '',
    tip = '',
  } = $props();

  const currentCategory = $derived(categories.find((category) => category.id === activeCategory));

  function handleKeydown(event) {
    if (event.key === 'Escape' && isOpen) isOpen = false;
  }

  useEventListener(() => document, 'keydown', handleKeydown);
</script>

<div
  class="fixed top-0 left-0 h-full flex shadow-2xl z-50 transform transition-transform duration-300 ease-in-out"
  style="background-color: var(--ds-surface-card, #ffffff);"
  class:translate-x-0={isOpen}
  class:-translate-x-full={!isOpen}
>
  <div
    class="w-16 border-r flex flex-col items-center py-4 gap-2"
    style="border-color: var(--ds-border); background-color: var(--ds-surface);"
  >
    {#each categories as category (category.id)}
      {@const isActive = activeCategory === category.id}
      {@const CategoryIcon = category.icon}
      <button
        class="w-12 h-12 rounded-lg flex items-center justify-center transition-all"
        style={isActive
          ? 'background: var(--ds-surface-raised); color: var(--ds-text); box-shadow: var(--shadow-sm);'
          : 'color: var(--ds-text-subtle);'}
        onmouseenter={(event) => {
          if (!isActive) event.currentTarget.style.cssText = 'background: var(--ds-background-neutral-hovered); color: var(--ds-text);';
        }}
        onmouseleave={(event) => {
          if (!isActive) event.currentTarget.style.cssText = 'color: var(--ds-text-subtle);';
        }}
        onclick={() => activeCategory = category.id}
        title={category.name}
        aria-label={category.name}
      >
        <CategoryIcon class="w-5 h-5" />
      </button>
    {/each}
  </div>

  <div class="w-96 flex flex-col" style="background-color: var(--ds-surface-raised);">
    <ModalHeader
      title={currentCategory?.name || fallbackTitle}
      subtitle={currentCategory?.description || ''}
      onClose={() => isOpen = false}
    />

    <div class="flex-1 overflow-y-auto p-6">
      <div class="space-y-3">
        {#each widgets as widget (widget.type)}
          {@const IconComponent = iconMap[widget.icon] || fallbackIcon}
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            {...cardAttributes}
            class="widget-card p-3 rounded border transition-colors cursor-grab active:cursor-grabbing"
            style="border-color: var(--ds-border); background-color: var(--ds-surface);"
            onmouseenter={(event) => event.currentTarget.style.cssText = 'border-color: var(--ds-border-focused); background-color: var(--ds-background-neutral-hovered);'}
            onmouseleave={(event) => event.currentTarget.style.cssText = 'border-color: var(--ds-border); background-color: var(--ds-surface);'}
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
                <h3 class="text-sm font-medium" style="color: var(--ds-text);">{widget.name}</h3>
                <DescriptionText>{widget.description}</DescriptionText>
                <div class="flex items-center gap-2 mt-2">
                  <span
                    class="text-xs px-2 py-0.5 rounded"
                    style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
                  >
                    {widget.categoryLabel}
                  </span>
                  <span class="text-xs" style="color: var(--ds-text-subtlest);">
                    {widget.widthLabel}
                  </span>
                </div>
              </div>
              <div class="cursor-grab active:cursor-grabbing flex-shrink-0" style="color: var(--ds-text-subtlest);">
                <GripVertical class="w-5 h-5" />
              </div>
            </div>
          </div>
        {/each}
      </div>

      <div class="mt-6 p-4 rounded" style="background-color: var(--ds-background-neutral); border: 1px solid var(--ds-border);">
        <p class="text-xs" style="color: var(--ds-text);">
          <strong>{tipLabel}:</strong> {tip}
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
