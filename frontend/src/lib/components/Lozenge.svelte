<script>
  import { getLuminance, darkenColor, lightenColor, isGrayColor } from '../utils/colorUtils.js';
  import { themeStore } from '../stores/theme.svelte.js';

  // Props
  let {
    color = null, // Full Catalyst color palette
    text = '',
    rounded = 'rounded', // 'rounded' | 'rounded-md'
    size = 'sm', // 'sm' | 'md'
    icon = null, // Optional Lucide icon component
    // Custom color props for dynamic colors (e.g., item type colors)
    customBg = null, // Hex color for background
    customBorder = null, // Hex color for border (defaults to customBg)
    customText = null // Hex color for text (defaults to customBg)
  } = $props();

  // Size classes
  const sizeClasses = {
    sm: 'px-2 py-0.5 text-xs',
    md: 'px-2.5 py-1 text-xs'
  };

  // Color mappings using hex values for dark mode support
  // Uses semi-transparent backgrounds (1A = ~10% opacity) that work in both themes
  const colorStyles = {
    red: '#ef4444',
    orange: '#f97316',
    amber: '#f59e0b',
    yellow: '#eab308',
    lime: '#84cc16',
    green: '#22c55e',
    emerald: '#10b981',
    teal: '#14b8a6',
    cyan: '#06b6d4',
    sky: '#0ea5e9',
    blue: '#3b82f6',
    indigo: '#6366f1',
    violet: '#8b5cf6',
    purple: '#a855f7',
    fuchsia: '#d946ef',
    pink: '#ec4899',
    rose: '#f43f5e',
    zinc: '#71717a',
    grey: '#71717a',
    gray: '#71717a'
  };

  let sizeClass = $derived(sizeClasses[size] || sizeClasses.sm);

  // Computed style - uses semi-transparent backgrounds for dark mode support
  let computedStyle = $derived.by(() => {
    if (customBg) {
      const luminance = getLuminance(customBg);
      const isGray = isGrayColor(customBg);
      // For light colors, darken text and border for visibility
      // Light colors (luminance > 0.65): darken by 50%
      // Medium-light (luminance > 0.5): darken by 30%
      let textBorderColor = customBg;
      let bgOpacity = '1A';
      if (luminance > 0.65) {
        textBorderColor = darkenColor(customBg, 0.5);
        bgOpacity = '30';
      } else if (luminance > 0.5) {
        textBorderColor = darkenColor(customBg, 0.3);
        bgOpacity = '20';
      }
      // In dark mode, lighten gray colors for better visibility
      if (themeStore.isDarkMode && isGray) {
        textBorderColor = lightenColor(customBg, 1);
        bgOpacity = '30';
      }
      return `background-color: ${customBg}${bgOpacity}; border-color: ${customBorder || textBorderColor}; color: ${customText || textBorderColor};`;
    }
    const baseColor = colorStyles[color] || colorStyles.sky;
    const isGray = color === 'zinc' || color === 'grey' || color === 'gray';
    // In dark mode, lighten gray colors for better visibility
    if (themeStore.isDarkMode && isGray) {
      const lightGray = lightenColor(baseColor, 1);
      return `background-color: ${baseColor}30; border-color: ${lightGray}; color: ${lightGray};`;
    }
    return `background-color: ${baseColor}1A; border-color: ${baseColor}; color: ${baseColor};`;
  });
</script>

<span
  class="inline-flex items-center gap-1 font-semibold border {rounded} {sizeClass}"
  style={computedStyle}
>
  {#if icon}
    <svelte:component this={icon} size={12} />
  {/if}
  {#if text}{text}{/if}
  <slot />
</span>
