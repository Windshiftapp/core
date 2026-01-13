/**
 * Shared color utility functions for handling light/dark colors
 */

/**
 * Calculate luminance of a hex color (0-1 scale)
 * @param {string} hexColor - Hex color code (with or without #)
 * @returns {number} Luminance value between 0 and 1
 */
export function getLuminance(hexColor) {
  if (!hexColor) return 0.5;
  const hex = hexColor.replace('#', '');
  const r = parseInt(hex.substr(0, 2), 16);
  const g = parseInt(hex.substr(2, 2), 16);
  const b = parseInt(hex.substr(4, 2), 16);
  return (0.299 * r + 0.587 * g + 0.114 * b) / 255;
}

/**
 * Darken a hex color by a factor
 * @param {string} hexColor - Hex color code (with or without #)
 * @param {number} factor - Darkening factor (0-1, where 1 = black)
 * @returns {string} Darkened hex color
 */
export function darkenColor(hexColor, factor) {
  const hex = hexColor.replace('#', '');
  const r = Math.round(parseInt(hex.substr(0, 2), 16) * (1 - factor));
  const g = Math.round(parseInt(hex.substr(2, 2), 16) * (1 - factor));
  const b = Math.round(parseInt(hex.substr(4, 2), 16) * (1 - factor));
  return `#${r.toString(16).padStart(2, '0')}${g.toString(16).padStart(2, '0')}${b.toString(16).padStart(2, '0')}`;
}

/**
 * Check if a color is light (high luminance)
 * @param {string} hexColor - Hex color code
 * @returns {boolean} True if color is light
 */
export function isLightColor(hexColor) {
  return getLuminance(hexColor) > 0.65;
}

/**
 * Get a visible version of a color (darkened if too light in light mode, lightened if dark in dark mode)
 * @param {string} hexColor - Hex color code
 * @param {boolean} isDarkMode - Whether dark mode is active (optional, auto-detects if not provided)
 * @returns {string} The original color or adjusted version for visibility
 */
export function getVisibleColor(hexColor, isDarkMode = null) {
  if (!hexColor) return hexColor;

  // Auto-detect dark mode if not provided
  if (isDarkMode === null && typeof document !== 'undefined') {
    isDarkMode = document.documentElement.getAttribute('data-color-mode') === 'dark';
  }

  const luminance = getLuminance(hexColor);

  if (isDarkMode) {
    // In dark mode, lighten dark colors for visibility
    if (luminance < 0.5) {
      // Lighten more for darker colors
      const factor = luminance < 0.3 ? 0.6 : 0.4;
      return lightenColor(hexColor, factor);
    }
    return hexColor;
  }

  // In light mode, darken light colors for visibility
  if (luminance > 0.65) {
    return darkenColor(hexColor, 0.5);
  } else if (luminance > 0.5) {
    return darkenColor(hexColor, 0.3);
  }
  return hexColor;
}

/**
 * Lighten a hex color by a factor
 * @param {string} hexColor - Hex color code (with or without #)
 * @param {number} factor - Lightening factor (0-1, where 1 = white)
 * @returns {string} Lightened hex color
 */
export function lightenColor(hexColor, factor) {
  const hex = hexColor.replace('#', '');
  const r = Math.round(parseInt(hex.substr(0, 2), 16) + (255 - parseInt(hex.substr(0, 2), 16)) * factor);
  const g = Math.round(parseInt(hex.substr(2, 2), 16) + (255 - parseInt(hex.substr(2, 2), 16)) * factor);
  const b = Math.round(parseInt(hex.substr(4, 2), 16) + (255 - parseInt(hex.substr(4, 2), 16)) * factor);
  return `#${r.toString(16).padStart(2, '0')}${g.toString(16).padStart(2, '0')}${b.toString(16).padStart(2, '0')}`;
}

/**
 * Check if a color is gray/neutral (low saturation)
 * @param {string} hexColor - Hex color code
 * @returns {boolean} True if color is gray/neutral
 */
export function isGrayColor(hexColor) {
  if (!hexColor) return false;
  const hex = hexColor.replace('#', '');
  const r = parseInt(hex.substr(0, 2), 16);
  const g = parseInt(hex.substr(2, 2), 16);
  const b = parseInt(hex.substr(4, 2), 16);
  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  const saturation = max === 0 ? 0 : (max - min) / max;
  return saturation < 0.2;
}
