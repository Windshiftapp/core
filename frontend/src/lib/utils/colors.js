/**
 * Shared color utility for consistent color handling across the application.
 * Based on Catalyst/Tailwind color palette.
 */

// Map color names to hex values (500-level for display as solid colors)
const colorToHex = {
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
  // Aliases
  grey: '#71717a',
  gray: '#71717a',
};

/**
 * Get hex code from color name
 * @param {string} colorName - Color name (e.g., 'blue', 'red')
 * @returns {string} Hex code (e.g., '#3b82f6')
 */
export function getHexFromColorName(colorName) {
  return colorToHex[colorName] || colorToHex.zinc;
}
