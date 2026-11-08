/**
 * Reactive UI locale code, mirrored from the i18n store.
 *
 * Components that need the active locale but must not import the i18n store
 * directly read this module. The i18n store is frequently mocked in tests
 * without every export, and importing it from widely used components makes
 * those mocks fail at import time.
 */
let current = $state('en');

/** @param {string} code */
export function setAppLocale(code) {
  current = code;
}

/** @returns {string} */
export function getAppLocale() {
  return current;
}
