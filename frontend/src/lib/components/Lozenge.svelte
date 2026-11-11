<script>
  import { getLuminance, darkenColor, lightenColor, isGrayColor } from '../utils/colorUtils.js';
  import { themeStore } from '../stores/theme.svelte.js';
  import { namedColorHex } from '../utils/colors.js';

  /**
   * @type {{
   *   color?: string | null,
   *   text?: string,
   *   rounded?: string,
   *   size?: string,
   *   icon?: any,
   *   square?: boolean,
   *   customBg?: string | null,
   *   customBorder?: string | null,
   *   customText?: string | null,
   *   onGradient?: boolean,
   *   appearance?: string,
   *   dataTestid?: string,
   *   class?: string,
   *   children?: any,
   * }}
   */
  let {
    color: colorProp = null,
    text = '',
    rounded = 'rounded',
    size = 'sm',
    icon: Icon = null,
    square = false,
    customBg = null,
    customBorder = null,
    customText = null,
    onGradient = false,
    appearance = null,
    dataTestid = undefined,
    class: className = '',
    children = null
  } = $props();

  const APPEARANCE_TO_COLOR = {
    info: 'blue',
    success: 'green',
    warning: 'amber',
    error: 'red',
    new: 'purple',
    default: 'gray',
    inprogress: 'sky',
    moved: 'orange',
    removed: 'red'
  };
  const color = $derived(colorProp || APPEARANCE_TO_COLOR[appearance] || null);

  // Size classes
  const sizeClasses = {
    sm: 'px-2 py-0.5 text-xs',
    md: 'px-2.5 py-1 text-xs'
  };

  // Dark mode colors using 400-level shades (softer, less jarring).
  // Gray/zinc are handled separately via --ds-accent-gray.
  const darkColorStyles = {
    red: '#f87171', green: '#4ade80', blue: '#60a5fa',
    orange: '#fb923c', amber: '#fbbf24', yellow: '#facc15',
    lime: '#a3e635', emerald: '#34d399', teal: '#2dd4bf',
    cyan: '#22d3ee', sky: '#38bdf8', indigo: '#818cf8',
    violet: '#a78bfa', purple: '#c084fc', fuchsia: '#e879f9',
    pink: '#f472b6', rose: '#fb7185'
  };

  let sizeClass = $derived(sizeClasses[size] || sizeClasses.sm);

  // Computed style - uses semi-transparent backgrounds for dark mode support
  let computedStyle = $derived.by(() => {
    if (onGradient) {
      return 'background-color: transparent; border-color: white; color: white;';
    }
    if (customBg) {
      const luminance = getLuminance(customBg);
      const isGray = isGrayColor(customBg);
      let textBorderColor = customBg;
      let bgOpacity = '1A';
      if (luminance > 0.65) {
        textBorderColor = darkenColor(customBg, 0.5);
        bgOpacity = '30';
      } else if (luminance > 0.5) {
        textBorderColor = darkenColor(customBg, 0.3);
        bgOpacity = '20';
      }
      if (themeStore.isDarkMode && isGray) {
        textBorderColor = lightenColor(customBg, 1);
        bgOpacity = '30';
      }
      return `background-color: ${customBg}${bgOpacity}; border-color: ${customBorder || textBorderColor}; color: ${customText || textBorderColor};`;
    }
    // Gray resolves to the theme's neutral accent ink instead of the
    // hardcoded named palette, so light and dark modes get readable,
    // tokenized values. Wash strength mirrors the other tints.
    const isGray = color === 'zinc' || color === 'grey' || color === 'gray';
    if (isGray) {
      const wash = themeStore.isDarkMode ? 19 : 10;
      return `background-color: color-mix(in srgb, var(--ds-accent-gray) ${wash}%, transparent); border-color: var(--ds-accent-gray); color: var(--ds-accent-gray);`;
    }
    if (themeStore.isDarkMode) {
      const darkColor = darkColorStyles[color] || darkColorStyles.sky;
      return `background-color: ${darkColor}1A; border-color: ${darkColor}; color: ${darkColor};`;
    }
    const baseColor = namedColorHex[color] || namedColorHex.sky;
    return `background-color: ${baseColor}1A; border-color: ${baseColor}; color: ${baseColor};`;
  });
</script>

<span
  class="inline-flex items-center whitespace-nowrap {square ? '' : 'gap-1'} font-semibold border {rounded} {square ? 'w-4 h-4 flex-shrink-0' : sizeClass} {className}"
  style={computedStyle}
  dir="auto"
  data-testid={dataTestid}
>
  {#if Icon}
    <Icon size={12} />
  {/if}
  {#if text}{text}{/if}
  {@render children?.()}
</span>
