/**
 * Theme store for managing dark/light mode preferences
 *
 * Supports three modes:
 * - 'light': Always use light theme
 * - 'dark': Always use dark theme
 * - 'system': Follow OS preference (default)
 */

const STORAGE_KEY = 'windshift-color-mode';

// Create reactive state using Svelte 5 runes
let colorMode = $state('system'); // 'light' | 'dark' | 'system'
let systemPreference = $state('light'); // detected OS preference
let activeTheme = $state(null); // current theme from backend

// Derived: actual theme to apply
const resolvedTheme = $derived(
	colorMode === 'system' ? systemPreference : colorMode
);

/**
 * Apply theme to document
 */
function applyTheme(theme) {
	if (typeof document !== 'undefined') {
		document.documentElement.dataset.colorMode = theme;
	}
}

/**
 * Apply nav colors based on current theme
 */
function applyNavColors() {
	if (typeof document === 'undefined' || !activeTheme) return;

	const root = document.documentElement;
	const isDark = resolvedTheme === 'dark';

	root.style.setProperty(
		'--nav-bg-color',
		isDark ? activeTheme.nav_background_color_dark : activeTheme.nav_background_color_light
	);
	root.style.setProperty(
		'--nav-text-color',
		isDark ? activeTheme.nav_text_color_dark : activeTheme.nav_text_color_light
	);
}

/**
 * Initialize theme store
 * - Reads from localStorage
 * - Sets up system preference detection
 * - Applies initial theme
 */
function init() {
	// Read stored preference
	if (typeof localStorage !== 'undefined') {
		const stored = localStorage.getItem(STORAGE_KEY);
		if (stored && ['light', 'dark', 'system'].includes(stored)) {
			colorMode = stored;
		}
	}

	// Set up system preference detection
	if (typeof window !== 'undefined') {
		const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
		systemPreference = mediaQuery.matches ? 'dark' : 'light';

		mediaQuery.addEventListener('change', (e) => {
			systemPreference = e.matches ? 'dark' : 'light';
			applyTheme(resolvedTheme);
			applyNavColors();
		});
	}

	// Apply initial theme
	applyTheme(resolvedTheme);
}

/**
 * Set color mode preference
 */
function setColorMode(mode) {
	if (!['light', 'dark', 'system'].includes(mode)) {
		console.warn(`Invalid color mode: ${mode}`);
		return;
	}

	colorMode = mode;

	// Persist to localStorage
	if (typeof localStorage !== 'undefined') {
		localStorage.setItem(STORAGE_KEY, mode);
	}

	// Apply theme immediately
	applyTheme(resolvedTheme);
	applyNavColors();
}

/**
 * Cycle through modes: light -> dark -> system -> light
 */
function cycleMode() {
	const modes = ['light', 'dark', 'system'];
	const currentIndex = modes.indexOf(colorMode);
	const nextIndex = (currentIndex + 1) % modes.length;
	setColorMode(modes[nextIndex]);
}

/**
 * Set the active theme (from backend)
 */
function setActiveTheme(theme) {
	activeTheme = theme;
	applyNavColors();
}

/**
 * Get current resolved theme
 */
function getResolvedTheme() {
	return resolvedTheme;
}

/**
 * Get current color mode setting
 */
function getColorMode() {
	return colorMode;
}

// Export the store
export const themeStore = {
	get colorMode() { return colorMode; },
	get resolvedTheme() { return resolvedTheme; },
	get isDarkMode() { return resolvedTheme === 'dark'; },
	get systemPreference() { return systemPreference; },
	get activeTheme() { return activeTheme; },
	init,
	setColorMode,
	cycleMode,
	setActiveTheme,
	getResolvedTheme,
	getColorMode,
};
