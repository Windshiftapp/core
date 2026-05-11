<script>
  import { getLuminance, darkenColor, lightenColor, isGrayColor } from '../utils/colorUtils.js';
  import { themeStore } from '../stores/theme.svelte.js';

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

  // Color mappings using hex values (500-level, used in light mode)
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

  // Dark mode colors using 400-level shades (softer, less jarring)
  const darkColorStyles = {
    red: '#f87171', green: '#4ade80', blue: '#60a5fa',
    orange: '#fb923c', amber: '#fbbf24', yellow: '#facc15',
    lime: '#a3e635', emerald: '#34d399', teal: '#2dd4bf',
    cyan: '#22d3ee', sky: '#38bdf8', indigo: '#818cf8',
    violet: '#a78bfa', purple: '#c084fc', fuchsia: '#e879f9',
    pink: '#f472b6', rose: '#fb7185',
    zinc: '#a1a1aa', grey: '#a1a1aa', gray: '#a1a1aa'
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
    const isGray = color === 'zinc' || color === 'grey' || color === 'gray';
    if (themeStore.isDarkMode) {
      const darkColor = darkColorStyles[color] || darkColorStyles.sky;
      if (isGray) {
        const lightGray = lightenColor(colorStyles[color] || colorStyles.zinc, 1);
        return `background-color: ${darkColor}30; border-color: ${lightGray}; color: ${lightGray};`;
      }
      return `background-color: ${darkColor}1A; border-color: ${darkColor}; color: ${darkColor};`;
    }
    const baseColor = colorStyles[color] || colorStyles.sky;
    return `background-color: ${baseColor}1A; border-color: ${baseColor}; color: ${baseColor};`;
  });
</script>

<span
  class="inline-flex items-center whitespace-nowrap {square ? '' : 'gap-1'} font-semibold border {rounded} {square ? 'w-4 h-4 flex-shrink-0' : sizeClass}"
  style={computedStyle}
>
  {#if Icon}
    <Icon size={12} />
  {/if}
  {#if text}{text}{/if}
  {@render children?.()}
</span>
