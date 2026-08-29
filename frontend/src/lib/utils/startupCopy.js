// Startup can fail before a locale catalog has been loaded. Keep the small
// recovery surface eager so it never renders translation keys when the locale
// chunk is offline, stalled, or otherwise unavailable.
export const EAGER_STARTUP_COPY = Object.freeze({
  'common.loading': 'Loading...',
  'common.loadingSlow': 'This can take a moment on a slow connection.',
  'common.retry': 'Retry',
  'common.skipToMainContent': 'Skip to main content',
  'errors.failedToLoad': 'Failed to load',
  'errors.NETWORK_ERROR': 'Network error. Please check your connection.',
  'errors.TIMEOUT': 'The request timed out. Please try again.',
});

export function getStartupCopy(key, i18nReady, translate) {
  return i18nReady ? translate(key) : (EAGER_STARTUP_COPY[key] ?? key);
}
