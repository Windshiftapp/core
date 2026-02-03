import { writable } from 'svelte/store';
import { api } from '../api.js';
import { gradients } from '../utils/gradients.js';

// Store for workspace gradient settings (using writable for compatibility with components using $ syntax)
export const workspaceGradientIndex = writable(0); // Default to 0 (None)
export const applyToAllViews = writable(false);
export const workspaceBackgroundImageUrl = writable(null); // Custom background image URL

// Internal runes-based state for useGradientStyles function
let _gradientIndex = $state(0);
let _applyToAll = $state(false);
let _backgroundImageUrl = $state(null);

// Sync stores to internal state
workspaceGradientIndex.subscribe((value) => {
  _gradientIndex = value;
});
applyToAllViews.subscribe((value) => {
  _applyToAll = value;
});
workspaceBackgroundImageUrl.subscribe((value) => {
  _backgroundImageUrl = value;
});

// Load gradient settings from workspace homepage layout
export async function loadWorkspaceGradient(workspaceId) {
  try {
    const layout = await api.workspaces.getHomepageLayout(workspaceId);
    // Default to 0 (None) if not set
    workspaceGradientIndex.set(layout?.gradient ?? 0);
    applyToAllViews.set(layout?.applyToAllViews ?? false);
    workspaceBackgroundImageUrl.set(layout?.backgroundImageUrl ?? null);
  } catch (error) {
    console.error('Failed to load workspace gradient:', error);
    // Default to no gradient/background
    workspaceGradientIndex.set(0);
    applyToAllViews.set(false);
    workspaceBackgroundImageUrl.set(null);
  }
}

// Get gradient CSS value from index
export function getGradientStyle(index) {
  // Index 0 is "None", no gradient
  if (index === 0 || !gradients[index]) {
    return null;
  }
  return gradients[index].value;
}

/**
 * Creates a reactive gradient styles object for use in Svelte 5 components.
 * Must be called at component initialization (top-level of <script>).
 *
 * Priority order:
 * 1. Background image (if set) → shows image with dark overlay
 * 2. Gradient (if index > 0) → shows gradient
 * 3. Default surface color
 *
 * @returns {Object} An object with reactive getters for all gradient-related styles
 *
 * @example
 * ```svelte
 * <script>
 *   import { useGradientStyles, loadWorkspaceGradient } from '../../stores/workspaceGradient.svelte.js';
 *
 *   const styles = useGradientStyles();
 *
 *   onMount(() => loadWorkspaceGradient(workspaceId));
 * </script>
 *
 * <div style="{styles.backgroundStyle}">
 *   <h1 style="{styles.textStyle}">Title</h1>
 *   <p style="{styles.subtleTextStyle}">Subtitle</p>
 *   <div style="{styles.glassStyle(12)}">Glass card</div>
 * </div>
 * ```
 */
export function useGradientStyles() {
  // Reactive gradient computation using Svelte 5 runes with internal state
  const gradientStyle = $derived(
    _applyToAll && _gradientIndex > 0 ? getGradientStyle(_gradientIndex) : null
  );

  // Background image takes priority over gradient
  const hasBackgroundImage = $derived(
    _applyToAll && _backgroundImageUrl !== null && _backgroundImageUrl !== ''
  );
  const hasGradient = $derived(!hasBackgroundImage && gradientStyle !== null);
  const hasCustomBackground = $derived(hasBackgroundImage || hasGradient);

  return {
    /** Whether a gradient is currently active (no background image) */
    get hasGradient() {
      return hasGradient;
    },

    /** Whether a background image is currently active */
    get hasBackgroundImage() {
      return hasBackgroundImage;
    },

    /** Whether any custom background (image or gradient) is active */
    get hasCustomBackground() {
      return hasCustomBackground;
    },

    /** The raw gradient CSS value (or null if no gradient) */
    get gradientStyle() {
      return gradientStyle;
    },

    /** The background image URL (or null if none) */
    get backgroundImageUrl() {
      return _backgroundImageUrl;
    },

    /** Background style for the main container */
    get backgroundStyle() {
      if (hasBackgroundImage) {
        // Background image with dark overlay for readability
        return `background: linear-gradient(rgba(0,0,0,0.3), rgba(0,0,0,0.3)), url(${_backgroundImageUrl}) center/cover no-repeat fixed;`;
      }
      if (hasGradient) {
        return `background: ${gradientStyle};`;
      }
      return 'background-color: var(--ds-surface);';
    },

    /** Text color for content directly on gradient/image background */
    get textStyle() {
      return hasCustomBackground ? 'color: white;' : 'color: var(--ds-text);';
    },

    /** Subtle text color for secondary content on gradient/image background */
    get subtleTextStyle() {
      return hasCustomBackground
        ? 'color: rgba(255, 255, 255, 0.8);'
        : 'color: var(--ds-text-subtle);';
    },

    /** Empty state text color (more subtle than subtleTextStyle) */
    get emptyStateStyle() {
      return hasCustomBackground
        ? 'color: rgba(255, 255, 255, 0.6);'
        : 'color: var(--ds-text-subtlest);';
    },

    /** Text color for content inside glass containers */
    get glassTextStyle() {
      return 'color: var(--ds-text);';
    },

    /** Subtle text color for content inside glass containers */
    get glassSubtleTextStyle() {
      return 'color: var(--ds-text-subtle);';
    },

    /** Drag handle color */
    get dragHandleStyle() {
      return 'color: var(--ds-text-subtlest);';
    },

    /**
     * Glass container style with configurable blur
     * @param {number} blur - Blur amount in pixels (default: 12)
     * @returns {string} CSS style string for glass effect
     */
    glassStyle(blur = 12) {
      return hasCustomBackground
        ? `background-color: var(--ds-glass-bg); border-color: var(--ds-glass-border); backdrop-filter: blur(${blur}px);`
        : 'background-color: var(--ds-surface-raised); border-color: var(--ds-border);';
    },

    /**
     * Card background style with configurable blur (for smaller elements like cards)
     * @param {number} blur - Blur amount in pixels (default: 8)
     * @returns {string} CSS style string for card background
     */
    cardStyle(blur = 8) {
      return hasCustomBackground
        ? `background-color: var(--ds-glass-bg); backdrop-filter: blur(${blur}px); border-color: var(--ds-glass-border);`
        : 'background-color: var(--ds-surface-raised); border-color: var(--ds-border);';
    },

    /**
     * Column background style (for board columns with higher blur)
     * @param {number} blur - Blur amount in pixels (default: 12)
     * @returns {string} CSS style string for column background
     */
    columnStyle(blur = 12) {
      return hasCustomBackground
        ? `backdrop-filter: blur(${blur}px); background-color: var(--ds-glass-bg); border-color: var(--ds-glass-border);`
        : 'background-color: var(--ds-surface); border-color: var(--ds-border);';
    },

    /**
     * Table container style
     * @param {number} blur - Blur amount in pixels (default: 12)
     * @returns {string} CSS style string for table container
     */
    tableStyle(blur = 12) {
      return hasCustomBackground
        ? `background-color: var(--ds-glass-bg); backdrop-filter: blur(${blur}px); border-color: var(--ds-glass-border);`
        : 'background-color: var(--ds-surface-raised); border-color: var(--ds-border);';
    },

    /** Table header background style */
    get tableHeaderStyle() {
      return 'background-color: var(--ds-surface);';
    },

    /** Border color for separators and dividers */
    get borderColor() {
      return hasCustomBackground ? 'var(--ds-glass-border)' : 'var(--ds-border)';
    },
  };
}
