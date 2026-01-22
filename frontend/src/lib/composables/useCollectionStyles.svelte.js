/**
 * Composable for managing collection map styling with gradient support.
 * Handles theming, glass effects, and background styles.
 */

/**
 * Creates reactive style variables for collection map with optional gradient support.
 *
 * @param {Function} getWorkspace - Function that returns the current workspace
 * @returns {Object} Reactive style objects and computed properties
 */
export function useCollectionStyles(getWorkspace) {
  // Reactive styles based on workspace
  let hasGradient = $derived.by(() => {
    const workspace = getWorkspace();
    return workspace?.gradient_start && workspace?.gradient_end;
  });

  let backgroundStyle = $derived.by(() => {
    const workspace = getWorkspace();
    if (workspace?.gradient_start && workspace?.gradient_end) {
      return `background: linear-gradient(135deg, ${workspace.gradient_start} 0%, ${workspace.gradient_end} 100%);`;
    }
    return 'background-color: var(--ds-surface);';
  });

  let textStyle = $derived.by(() => {
    const workspace = getWorkspace();
    if (workspace?.gradient_start && workspace?.gradient_end) {
      return 'color: var(--ds-text);';
    }
    return 'color: var(--ds-text);';
  });

  let subtleTextStyle = $derived.by(() => {
    const workspace = getWorkspace();
    if (workspace?.gradient_start && workspace?.gradient_end) {
      return 'color: var(--ds-text-subtle);';
    }
    return 'color: var(--ds-text-subtle);';
  });

  let glassTextStyle = $derived.by(() => {
    const workspace = getWorkspace();
    if (workspace?.gradient_start && workspace?.gradient_end) {
      return 'color: var(--ds-text);';
    }
    return 'color: var(--ds-text);';
  });

  let glassSubtleTextStyle = $derived.by(() => {
    const workspace = getWorkspace();
    if (workspace?.gradient_start && workspace?.gradient_end) {
      return 'color: var(--ds-text-subtle);';
    }
    return 'color: var(--ds-text-subtle);';
  });

  let cardBgStyle = $derived.by(() => {
    const workspace = getWorkspace();
    if (workspace?.gradient_start && workspace?.gradient_end) {
      return 'backdrop-filter: blur(12px); background-color: var(--ds-glass-bg); border-color: var(--ds-glass-border);';
    }
    return 'background-color: var(--ds-surface-card); border-color: var(--ds-border);';
  });

  let dropZoneBorderStyle = $derived.by(() => {
    const workspace = getWorkspace();
    if (workspace?.gradient_start && workspace?.gradient_end) {
      return 'border-color: var(--ds-glass-border); background-color: var(--ds-glass-bg);';
    }
    return 'border-color: var(--ds-border); background-color: var(--ds-surface-overlay);';
  });

  return {
    get hasGradient() { return hasGradient; },
    get backgroundStyle() { return backgroundStyle; },
    get textStyle() { return textStyle; },
    get subtleTextStyle() { return subtleTextStyle; },
    get glassTextStyle() { return glassTextStyle; },
    get glassSubtleTextStyle() { return glassSubtleTextStyle; },
    get cardBgStyle() { return cardBgStyle; },
    get dropZoneBorderStyle() { return dropZoneBorderStyle; }
  };
}

/**
 * Gets the appropriate background class/style for items based on gradient mode.
 *
 * @param {boolean} hasGradient - Whether gradient mode is active
 * @param {string} defaultBg - Default background color variable
 * @returns {string} CSS style string
 */
export function getItemBackground(hasGradient, defaultBg = 'var(--ds-surface-card)') {
  if (hasGradient) {
    return 'backdrop-filter: blur(12px); background-color: var(--ds-glass-bg);';
  }
  return `background-color: ${defaultBg};`;
}

/**
 * Gets the appropriate border style based on gradient mode.
 *
 * @param {boolean} hasGradient - Whether gradient mode is active
 * @returns {string} CSS style string
 */
export function getItemBorder(hasGradient) {
  if (hasGradient) {
    return 'border-color: var(--ds-glass-border);';
  }
  return 'border-color: var(--ds-border);';
}

/**
 * Generates the backdrop blur class for glass effect.
 *
 * @param {boolean} hasGradient - Whether gradient mode is active
 * @returns {string} CSS class string
 */
export function getBlurClass(hasGradient) {
  return hasGradient ? 'backdrop-blur-sm' : '';
}
