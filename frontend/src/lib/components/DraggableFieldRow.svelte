<script>
  import DropIndicator from '../layout/DropIndicator.svelte';
  import DragHandleDots from './DragHandleDots.svelte';

  let {
    fieldId,
    fieldIndex,
    closestEdge = null,
    attributes = {},
    class: className = '',
    style = '',
    handleClass = 'w-4 h-4',
    handleColor = undefined,
    handleAttributes = {},
    children
  } = $props();
</script>

<div
  {...attributes}
  data-field-index={fieldIndex}
  data-field-id={fieldId}
  class={`relative group flex items-center ${className}`}
  {style}
>
  {#if closestEdge}
    <DropIndicator edge={closestEdge} gap={8} />
  {/if}

  <div
    {...handleAttributes}
    class="cursor-grab active:cursor-grabbing flex-shrink-0 p-1 rounded hover-bg transition-colors"
    style="touch-action: none;"
  >
    <DragHandleDots class={handleClass} color={handleColor} />
  </div>

  {@render children?.()}
</div>
