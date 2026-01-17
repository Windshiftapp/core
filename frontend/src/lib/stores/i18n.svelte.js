/**
 * Internationalization (i18n) store for managing translations
 *
 * Supports:
 * - Multiple locales with lazy loading
 * - RTL support for Arabic
 * - String interpolation with {param} syntax
 * - Pluralization with _one, _other suffixes
 * - Backend error code translation
 * - localStorage persistence
 */

const STORAGE_KEY = 'windshift-locale';
const DEFAULT_LOCALE = 'en';

// Supported locales configuration
export const SUPPORTED_LOCALES = [
  { code: 'en', name: 'English', direction: 'ltr' },
  { code: 'de', name: 'Deutsch', direction: 'ltr' },
  { code: 'es', name: 'Español', direction: 'ltr' },
  { code: 'ar', name: 'العربية', direction: 'rtl' }
];

// Reactive state using Svelte 5 runes
let locale = $state(DEFAULT_LOCALE);
let translations = $state({});
let loading = $state(false);

// Derived state
const direction = $derived(
  SUPPORTED_LOCALES.find(l => l.code === locale)?.direction || 'ltr'
);

const isRTL = $derived(direction === 'rtl');

/**
 * Get a nested value from an object using dot notation
 * @param {object} obj - Object to traverse
 * @param {string} path - Dot-notated path (e.g., 'common.save')
 * @returns {string|undefined}
 */
function getNestedValue(obj, path) {
  return path.split('.').reduce((current, key) => {
    return current && typeof current === 'object' ? current[key] : undefined;
  }, obj);
}

/**
 * Interpolate parameters into a string
 * @param {string} str - String with {param} placeholders
 * @param {object} params - Parameters to interpolate
 * @returns {string}
 */
function interpolate(str, params = {}) {
  if (!str || typeof str !== 'string') return str;

  return str.replace(/\{(\w+)\}/g, (match, key) => {
    return params[key] !== undefined ? String(params[key]) : match;
  });
}

/**
 * Get translation for a key with optional interpolation and pluralization
 * @param {string} key - Translation key (dot notation)
 * @param {object} params - Parameters for interpolation (use 'count' for pluralization)
 * @returns {string}
 */
export function t(key, params = {}) {
  let value;

  // Handle pluralization
  if (params.count !== undefined) {
    const pluralKey = params.count === 1 ? `${key}_one` : `${key}_other`;
    value = getNestedValue(translations, pluralKey);
    if (!value) {
      // Fall back to base key if plural variant not found
      value = getNestedValue(translations, key);
    }
  } else {
    value = getNestedValue(translations, key);
  }

  // Return key if translation not found (helps identify missing translations)
  if (value === undefined) {
    console.warn(`Missing translation: ${key}`);
    return key;
  }

  return interpolate(value, params);
}

/**
 * Translate a backend error object
 * @param {Error|object} error - Error object with optional code and details
 * @returns {string}
 */
export function translateError(error) {
  if (!error) return t('errors.INTERNAL_ERROR');

  // Check if error has a code property
  const code = error.code || error.errorCode;

  if (code) {
    // Look up translation for error code
    const translation = getNestedValue(translations, `errors.${code}`);
    if (translation) {
      // Interpolate details if available
      return interpolate(translation, error.details || {});
    }
  }

  // Fall back to error message if available
  if (error.message) {
    return error.message;
  }

  // Final fallback
  return t('errors.INTERNAL_ERROR');
}

/**
 * Load translations for a locale
 * @param {string} localeCode - Locale code to load
 */
async function loadTranslations(localeCode) {
  loading = true;

  try {
    // Dynamic import for lazy loading
    const module = await import(`../locales/${localeCode}/index.js`);
    translations = module.default;
    locale = localeCode;

    // Persist to localStorage
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(STORAGE_KEY, localeCode);
    }

    // Update document direction
    if (typeof document !== 'undefined') {
      document.documentElement.dir = direction;
      document.documentElement.lang = localeCode;
    }
  } catch (err) {
    console.error(`Failed to load locale: ${localeCode}`, err);

    // Fall back to English if loading fails
    if (localeCode !== DEFAULT_LOCALE) {
      await loadTranslations(DEFAULT_LOCALE);
    }
  } finally {
    loading = false;
  }
}

/**
 * Initialize i18n with saved or default locale
 */
async function init() {
  let initialLocale = DEFAULT_LOCALE;

  // Check localStorage for saved preference
  if (typeof localStorage !== 'undefined') {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved && SUPPORTED_LOCALES.some(l => l.code === saved)) {
      initialLocale = saved;
    }
  }

  // Check browser language preference
  if (initialLocale === DEFAULT_LOCALE && typeof navigator !== 'undefined') {
    const browserLang = navigator.language?.split('-')[0];
    if (browserLang && SUPPORTED_LOCALES.some(l => l.code === browserLang)) {
      initialLocale = browserLang;
    }
  }

  await loadTranslations(initialLocale);
}

/**
 * Change the current locale
 * @param {string} localeCode - New locale code
 */
async function setLocale(localeCode) {
  if (!SUPPORTED_LOCALES.some(l => l.code === localeCode)) {
    console.warn(`Unsupported locale: ${localeCode}`);
    return;
  }

  if (localeCode === locale && Object.keys(translations).length > 0) {
    return; // Already loaded
  }

  await loadTranslations(localeCode);
}

/**
 * Get the current locale code
 */
function getLocale() {
  return locale;
}

/**
 * Check if translations are currently loading
 */
function isLoading() {
  return loading;
}

// Export the i18n store
export const i18n = {
  get locale() { return locale; },
  get direction() { return direction; },
  get isRTL() { return isRTL; },
  get loading() { return loading; },
  get supportedLocales() { return SUPPORTED_LOCALES; },
  init,
  setLocale,
  getLocale,
  isLoading,
  t,
  translateError
};

// Also export t and translateError directly for convenience
export { t as translate };
