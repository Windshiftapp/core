<script>
  import { statusCategoriesStore } from '../stores/statusCategories.svelte.js';
  import { getVisibleColor } from '../utils/colorUtils.js';
  import { themeStore } from '../stores/theme.svelte.js';

  let {
    status = null,       // Status object with { name, category_id } or { label, categoryColor }
    size = 'sm',         // 'sm' | 'md'
    showDot = true,      // Whether to show color dot indicator
    uppercase = true     // Whether to uppercase text
  } = $props();

  // Get category color - supports both category_id lookup and direct categoryColor property
  let categoryColor = $derived(
    status?.categoryColor
      ? status.categoryColor
      : status?.category_id
        ? statusCategoriesStore.getCategoryColor(status.category_id)
        : null
  );

  // Compute border color (fallback to design system border)
  let borderColor = $derived(categoryColor || 'var(--ds-border)');

  // Compute text color (adapt for dark mode)
  let textColor = $derived(
    categoryColor
      ? getVisibleColor(categoryColor, themeStore.isDarkMode)
      : 'var(--ds-text)'
  );

  // Size classes
  let sizeClasses = $derived({
    sm: 'px-2 py-0.5 text-xs',
    md: 'px-2.5 py-1 text-sm'
  }[size] || 'px-2 py-0.5 text-xs');

  let dotSize = $derived({
    sm: 'w-1.5 h-1.5',
    md: 'w-2 h-2'
  }[size] || 'w-1.5 h-1.5');

  // Get display text - supports both name and label properties
  let displayText = $derived(status?.name || status?.label || '');
</script>

{#if status}
  <span
    class="inline-flex items-center gap-1.5 rounded font-medium {sizeClasses}"
    class:uppercase
    style="border: 1px solid {borderColor}; color: {textColor};"
  >
    {#if showDot}
      <span class="{dotSize} rounded-full flex-shrink-0" style="background-color: {borderColor};"></span>
    {/if}
    {displayText}
  </span>
{/if}
