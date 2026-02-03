/**
 * Shared color utility for consistent color handling across the application.
 * Based on Catalyst/Tailwind color palette.
 */

// Available color names matching the Lozenge component
export const colorNames = [
  'red',
  'orange',
  'amber',
  'yellow',
  'lime',
  'green',
  'emerald',
  'teal',
  'cyan',
  'sky',
  'blue',
  'indigo',
  'violet',
  'purple',
  'fuchsia',
  'pink',
  'rose',
  'zinc',
];

// Map color names to hex values (500-level for display as solid colors)
export const colorToHex = {
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

// Map hex values back to color names (for migration/compatibility)
export const hexToColor = Object.entries(colorToHex).reduce((acc, [name, hex]) => {
  // Only use canonical names (not aliases) for reverse mapping
  if (!['grey', 'gray'].includes(name)) {
    acc[hex.toLowerCase()] = name;
  }
  return acc;
}, {});

/**
 * Get hex code from color name
 * @param {string} colorName - Color name (e.g., 'blue', 'red')
 * @returns {string} Hex code (e.g., '#3b82f6')
 */
export function getHexFromColorName(colorName) {
  return colorToHex[colorName] || colorToHex.zinc;
}

/**
 * Get color name from hex code (for migration purposes)
 * @param {string} hex - Hex code (e.g., '#3b82f6')
 * @returns {string} Color name (e.g., 'blue')
 */
export function getColorNameFromHex(hex) {
  return hexToColor[hex?.toLowerCase()] || 'zinc';
}

/**
 * Validate if a string is a valid color name
 * @param {string} colorName - Color name to validate
 * @returns {boolean}
 */
export function isValidColorName(colorName) {
  return colorNames.includes(colorName) || colorName === 'grey' || colorName === 'gray';
}
